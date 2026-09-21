// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the resilience helpers leave no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// retryableErr returns an error classified as retryable by errs.Retryable.
func retryableErr() error {
	return errs.Connection("test", errors.New("transient"))
}

// permanentErr returns an error classified as non-retryable by errs.Retryable.
func permanentErr() error {
	return errs.New(types.StatusInvalidParam, "test", "invalid parameter")
}

func TestPolicyMutationIsSingleAttempt(t *testing.T) {
	p := Policy{MaxAttempts: 5, BaseBackoff: 0, Jitter: false}
	calls := 0
	attempts, err := p.Do(context.Background(), ClassMutation, func(context.Context) error {
		calls++
		return retryableErr()
	})
	if attempts != 1 || calls != 1 {
		t.Fatalf("mutation class: attempts=%d calls=%d, want 1/1", attempts, calls)
	}
	if err == nil {
		t.Fatal("mutation class: err = nil, want the retryable failure")
	}
}

func TestPolicyDoRouteMutationIsSingleAttempt(t *testing.T) {
	for _, path := range MutationPaths() {
		p := Policy{MaxAttempts: 4, BaseBackoff: 0, Jitter: false}
		calls := 0
		attempts, err := p.DoRoute(context.Background(), path, func(context.Context) error {
			calls++
			return retryableErr()
		})
		if attempts != 1 || calls != 1 {
			t.Fatalf("%s: attempts=%d calls=%d, want 1/1", path, attempts, calls)
		}
		if err == nil {
			t.Fatalf("%s: err = nil, want the retryable failure", path)
		}
	}
}

func TestIsMutation(t *testing.T) {
	if !IsMutation("/trade/TradeEntrust") || !IsMutation("/trade/FuturesEntrust") || !IsMutation("/trade/AlgoAddOrder") {
		t.Fatal("IsMutation missed a documented mutation endpoint")
	}
	if IsMutation("/hq/BasicQot") || IsMutation("/trade/TradeQueryRealEntrustList") {
		t.Fatal("IsMutation classified a query endpoint as a mutation")
	}
	if ClassForPath("/hq/BasicQot") != ClassQuery || ClassForPath("/trade/TradeEntrust") != ClassMutation {
		t.Fatal("ClassForPath returned the wrong class")
	}
	paths := MutationPaths()
	if len(paths) != 12 {
		t.Fatalf("MutationPaths len = %d, want 12", len(paths))
	}
}

func TestPolicyRetriesRetryableQuery(t *testing.T) {
	p := Policy{MaxAttempts: 3, BaseBackoff: 0, Jitter: false}
	calls := 0
	attempts, err := p.DoRoute(context.Background(), "/hq/BasicQot", func(context.Context) error {
		calls++
		if calls < 3 {
			return retryableErr()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if attempts != 3 || calls != 3 {
		t.Fatalf("attempts=%d calls=%d, want 3/3", attempts, calls)
	}
}

func TestPolicyStopsOnNonRetryable(t *testing.T) {
	p := Policy{MaxAttempts: 5, BaseBackoff: 0, Jitter: false}
	calls := 0
	attempts, err := p.DoRoute(context.Background(), "/hq/BasicQot", func(context.Context) error {
		calls++
		return permanentErr()
	})
	if attempts != 1 || calls != 1 {
		t.Fatalf("attempts=%d calls=%d, want 1/1", attempts, calls)
	}
	if err == nil {
		t.Fatal("err = nil, want the permanent failure")
	}
}

func TestPolicyExhaustsBudget(t *testing.T) {
	p := Policy{MaxAttempts: 2, BaseBackoff: 0, Jitter: false}
	calls := 0
	attempts, err := p.Do(context.Background(), ClassQuery, func(context.Context) error {
		calls++
		return retryableErr()
	})
	if attempts != 2 || calls != 2 {
		t.Fatalf("attempts=%d calls=%d, want 2/2", attempts, calls)
	}
	if err == nil {
		t.Fatal("err = nil, want the last failure")
	}
}

func TestPolicyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	attempts, err := Policy{MaxAttempts: 3}.DoRoute(ctx, "/hq/BasicQot", func(context.Context) error {
		calls++
		return nil
	})
	if attempts != 0 || calls != 0 {
		t.Fatalf("attempts=%d calls=%d, want 0/0", attempts, calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestPolicyDelayBackoffAndJitter(t *testing.T) {
	p := Policy{BaseBackoff: 100 * time.Millisecond, MaxBackoff: 250 * time.Millisecond, Jitter: false}
	if got := p.Delay(1); got != 100*time.Millisecond {
		t.Fatalf("Delay(1) = %v, want 100ms", got)
	}
	if got := p.Delay(2); got != 200*time.Millisecond {
		t.Fatalf("Delay(2) = %v, want 200ms", got)
	}
	if got := p.Delay(3); got != 250*time.Millisecond {
		t.Fatalf("Delay(3) = %v, want 250ms", got)
	}
	if got := (Policy{BaseBackoff: 0}).Delay(1); got != 0 {
		t.Fatalf("Delay with no base = %v, want 0", got)
	}
	j := Policy{BaseBackoff: 100 * time.Millisecond, Jitter: true}
	for i := 0; i < 20; i++ {
		d := j.Delay(1)
		if d < 50*time.Millisecond || d > 100*time.Millisecond {
			t.Fatalf("jittered Delay(1) = %v, want within [50ms, 100ms]", d)
		}
	}
}

func TestLimiterBurstAndRefill(t *testing.T) {
	clock := time.Now()
	l := NewLimiter(Limit{TokensPerSecond: 1, Burst: 2}, nil)
	l.now = func() time.Time { return clock }

	ctx := context.Background()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("second Wait: %v", err)
	}

	short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if err := l.Wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third Wait = %v, want context.DeadlineExceeded", err)
	}

	clock = clock.Add(3 * time.Second)
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait after refill: %v", err)
	}
}

func TestLimiterPerEndpoint(t *testing.T) {
	clock := time.Now()
	l := NewLimiter(Limit{}, map[string]Limit{"/hq/BasicQot": {TokensPerSecond: 1, Burst: 1}})
	l.now = func() time.Time { return clock }

	ctx := context.Background()
	if err := l.WaitEndpoint(ctx, "/hq/BasicQot"); err != nil {
		t.Fatalf("first WaitEndpoint: %v", err)
	}
	short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if err := l.WaitEndpoint(short, "/hq/BasicQot"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second WaitEndpoint = %v, want context.DeadlineExceeded", err)
	}

	clock = clock.Add(time.Second)
	if err := l.WaitEndpoint(ctx, "/hq/BasicQot"); err != nil {
		t.Fatalf("WaitEndpoint after refill: %v", err)
	}
}

func TestLimiterDisabledAdmitsImmediately(t *testing.T) {
	l := NewLimiter(Limit{}, nil)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("Wait on disabled limiter: %v", err)
		}
		if err := l.WaitEndpoint(ctx, "/hq/BasicQot"); err != nil {
			t.Fatalf("WaitEndpoint on disabled limiter: %v", err)
		}
	}
}

func TestBreakerTransitions(t *testing.T) {
	clock := time.Now()
	b := NewBreaker(2, time.Minute)
	b.now = func() time.Time { return clock }

	if b.State() != StateClosed {
		t.Fatalf("initial state = %v, want closed", b.State())
	}
	if !b.Allow() {
		t.Fatal("closed breaker refused a call")
	}
	b.OnFailure()
	if b.State() != StateClosed {
		t.Fatalf("state after one failure = %v, want closed", b.State())
	}
	b.OnFailure()
	if b.State() != StateOpen {
		t.Fatalf("state after threshold failures = %v, want open", b.State())
	}
	if b.Allow() {
		t.Fatal("open breaker admitted a call before cooldown")
	}

	clock = clock.Add(time.Minute)
	if !b.Allow() {
		t.Fatal("breaker did not admit a half-open probe after cooldown")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state = %v, want half-open", b.State())
	}
	if b.Allow() {
		t.Fatal("half-open breaker admitted a second concurrent probe")
	}
	b.OnFailure()
	if b.State() != StateOpen {
		t.Fatalf("state after failed probe = %v, want open", b.State())
	}

	clock = clock.Add(time.Minute)
	if !b.Allow() {
		t.Fatal("breaker did not admit a probe after the second cooldown")
	}
	b.OnSuccess()
	if b.State() != StateClosed || b.Failures() != 0 {
		t.Fatalf("state after successful probe = %v failures=%d, want closed/0", b.State(), b.Failures())
	}
}

func TestBreakerDefaults(t *testing.T) {
	b := NewBreaker(0, 0)
	if b.threshold != DefaultFailureThreshold || b.cooldown != DefaultCooldown {
		t.Fatalf("defaults = %d/%v, want %d/%v", b.threshold, b.cooldown, DefaultFailureThreshold, DefaultCooldown)
	}
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateClosed:   "closed",
		StateOpen:     "open",
		StateHalfOpen: "half-open",
		State(99):     "unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", state, got, want)
		}
	}
}
