// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package resilience

import (
	"context"
	"sync"
	"time"
)

// Limit is a token-bucket policy for one scope (global or a single endpoint).
//
// TokensPerSecond is the steady refill rate and Burst is the bucket depth, that
// is the number of calls that may be admitted back-to-back after an idle
// period. A non-positive TokensPerSecond disables the scope: Wait never blocks
// for it. A non-positive Burst with a positive rate is normalized to 1.
type Limit struct {
	// TokensPerSecond is the steady-state refill rate. It must be positive for
	// the limit to apply.
	TokensPerSecond float64
	// Burst is the maximum number of tokens the bucket holds, i.e. the number
	// of immediate calls allowed. It is normalized to at least 1 when the rate
	// is positive.
	Burst int
}

// enabled reports whether the limit applies.
func (l Limit) enabled() bool {
	return l.TokensPerSecond > 0 && l.Burst > 0
}

// Waiter is the narrow interface the transport layer needs: a blocking admit
// call that honors context cancellation. *Limiter implements it.
type Waiter interface {
	// Wait blocks until the global limiter admits one call, or returns a
	// non-nil error when ctx is done first.
	Wait(ctx context.Context) error
}

// Limiter is a hierarchical token-bucket rate limiter with a global bucket and
// an optional per-endpoint bucket. Each WaitEndpoint call must pass both
// buckets, so an endpoint limit tightens the global limit rather than replacing
// it. A Limiter is safe for concurrent use.
//
// A zero-value Limit disables the corresponding scope, so
// NewLimiter(Limit{}, nil) admits every call immediately. This is the default
// behaviour of the SDK: the limiter is only consulted when a caller installs
// one with client.WithRateLimiter.
type Limiter struct {
	mu          sync.Mutex
	global      Limit
	perEndpoint map[string]Limit
	buckets     map[string]*bucket
	globalBkt   *bucket
	now         func() time.Time
}

// bucket is a single token bucket. It is guarded by its own mutex so buckets
// refill independently.
type bucket struct {
	mu     sync.Mutex
	limit  Limit
	tokens float64
	last   time.Time
	now    func() time.Time
}

// NewLimiter builds a Limiter from a global limit and optional per-endpoint
// limits. The per-endpoint map is copied; a nil map means no per-endpoint
// limits. Endpoints absent from the map are governed by the global limit only.
// Every limit is normalized as described on Limit.
func NewLimiter(global Limit, perEndpoint map[string]Limit) *Limiter {
	l := &Limiter{
		global:      normalizeLimit(global),
		perEndpoint: make(map[string]Limit, len(perEndpoint)),
		buckets:     make(map[string]*bucket),
		now:         time.Now,
	}
	for k, v := range perEndpoint {
		l.perEndpoint[k] = normalizeLimit(v)
	}
	if l.global.enabled() {
		l.globalBkt = l.newBucket(l.global)
	}
	return l
}

// normalizeLimit coerces a Limit into a usable form: a positive rate with a
// non-positive burst gets Burst 1, and a non-positive rate is left disabled.
func normalizeLimit(l Limit) Limit {
	if l.TokensPerSecond > 0 && l.Burst <= 0 {
		l.Burst = 1
	}
	return l
}

// nowTime is bound into every bucket so a test can replace the clock by
// assigning l.now before constructing buckets.
func (l *Limiter) nowTime() time.Time { return l.now() }

// newBucket creates a full bucket for limit. It must be called without holding
// l.mu when it is invoked from NewLimiter (which holds no lock yet).
func (l *Limiter) newBucket(limit Limit) *bucket {
	return &bucket{
		limit:  limit,
		tokens: float64(limit.Burst),
		last:   l.nowTime(),
		now:    l.nowTime,
	}
}

// Wait blocks until the global limiter admits one call. It returns
// ctx.Err() when ctx is done before a token is available, and nil immediately
// when the global limit is disabled.
func (l *Limiter) Wait(ctx context.Context) error {
	return l.globalBkt.wait(ctx)
}

// WaitEndpoint blocks until both the global limiter and the endpoint's own
// limiter (if configured) admit one call. The global bucket is charged first,
// then the endpoint bucket; a context cancellation returns ctx.Err(). An
// endpoint without a configured limit is governed by the global limit alone.
func (l *Limiter) WaitEndpoint(ctx context.Context, endpoint string) error {
	if err := l.globalBkt.wait(ctx); err != nil {
		return err
	}
	return l.bucketFor(endpoint).wait(ctx)
}

// bucketFor returns the bucket for endpoint, creating it on first use. A nil
// return means the endpoint has no limit.
func (l *Limiter) bucketFor(endpoint string) *bucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.buckets[endpoint]; ok {
		return b
	}
	limit, ok := l.perEndpoint[endpoint]
	if !ok || !limit.enabled() {
		l.buckets[endpoint] = nil
		return nil
	}
	b := l.newBucket(limit)
	l.buckets[endpoint] = b
	return b
}

// wait blocks for one token, or returns immediately for a nil bucket.
func (b *bucket) wait(ctx context.Context) error {
	if b == nil {
		return nil
	}
	for {
		b.mu.Lock()
		now := b.now()
		b.refill(now)
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		wait := time.Duration(float64(time.Second) * (1 - b.tokens) / b.limit.TokensPerSecond)
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		b.mu.Unlock()

		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
	}
}

// refill adds the tokens accrued since the last refill and caps the bucket at
// its burst depth. It must be called with b.mu held.
func (b *bucket) refill(now time.Time) {
	if b.last.IsZero() {
		b.last = now
		return
	}
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	b.last = now
	b.tokens += elapsed.Seconds() * b.limit.TokensPerSecond
	if b.tokens > float64(b.limit.Burst) {
		b.tokens = float64(b.limit.Burst)
	}
}

// sleepCtx sleeps for d, returning ctx.Err() when ctx is done first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
