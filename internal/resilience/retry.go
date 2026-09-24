// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package resilience

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
)

// Class classifies an operation for retry purposes. Query operations may be
// retried by a Policy; mutation operations are never retried.
type Class string

const (
	// ClassQuery is the retry class of read-only endpoints: market data,
	// assets and positions, fund-journey and order/entrust/deliver queries,
	// futures queries, and algo queries.
	ClassQuery Class = "query"
	// ClassMutation is the retry class of order, futures, and algo mutations.
	// A Policy issues exactly one attempt for this class, at every
	// configuration (docs/adr/0003-no-auto-retry-orders.md).
	ClassMutation Class = "mutation"
)

// mutationPaths is the closed set of order-mutation endpoints. The strings
// mirror the canonical client.Route paths; this package deliberately does not
// import client so it can be reused by the transport without a cycle. A caller
// cannot extend or shrink this set.
var mutationPaths = []string{
	// Trade.
	"/trade/TradeEntrust",
	"/trade/TradeCancelEntrust",
	"/trade/TradeBatchCancelEntrust",
	"/trade/TradeChangeEntrust",
	// Futures.
	"/trade/FuturesEntrust",
	"/trade/FuturesCancelEntrust",
	"/trade/FuturesModifyEntrust",
	// Algo.
	"/trade/AlgoAddOrder",
	"/trade/AlgoCancelOrder",
	"/trade/AlgoCancelEntrust",
	"/trade/AlgoChangeOrder",
	"/trade/AlgoActionOrder",
}

// mutationSet is mutationPaths as a lookup set.
var mutationSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(mutationPaths))
	for _, p := range mutationPaths {
		m[p] = struct{}{}
	}
	return m
}()

// aliasSuffixMsgType and aliasSuffixRequest are the two alias suffixes the
// Gateway accepts for every route. They are duplicated from client/routes.go
// rather than imported: client depends on this package, so importing it back
// would create an import cycle. client.NormalizePath remains the authority for
// turning an alias into a canonical path; this copy exists so mutation
// classification cannot be bypassed by using an alias form.
const (
	aliasSuffixMsgType = "RequestMsgType"
	aliasSuffixRequest = "Request"
)

// stripAliasSuffix removes a trailing alias suffix from p. RequestMsgType is
// stripped before Request so "/XRequestMsgType" reduces to "/X" in one call.
func stripAliasSuffix(p string) string {
	if stripped := strings.TrimSuffix(p, aliasSuffixMsgType); stripped != p {
		return stripped
	}
	return strings.TrimSuffix(p, aliasSuffixRequest)
}

// IsMutation reports whether path is one of the closed set of order-mutation
// endpoints. It is the authoritative mutation test; callers must not replace it
// with a flag. The two Gateway alias forms are reduced to their canonical path
// first, so aliasing a mutation cannot silently reclassify it as a retryable
// query.
func IsMutation(path string) bool {
	_, ok := mutationSet[stripAliasSuffix(path)]
	return ok
}

// MutationPaths returns a sorted copy of the closed mutation-endpoint set.
// Callers may modify the returned slice without affecting the set.
func MutationPaths() []string {
	out := append([]string(nil), mutationPaths...)
	sort.Strings(out)
	return out
}

// ClassForPath returns ClassMutation for a mutation endpoint and ClassQuery
// for everything else. Alias forms of a mutation are recognised as mutations.
func ClassForPath(path string) Class {
	if IsMutation(path) {
		return ClassMutation
	}
	return ClassQuery
}

// AttemptFunc performs exactly one attempt of an operation. It must honor
// cancellation of the supplied context. A returned error is inspected with
// errs.Retryable to decide whether the Policy makes another attempt.
type AttemptFunc func(ctx context.Context) error

// Policy is the retry configuration for read-only queries. The zero value
// makes exactly one attempt. A Policy is an immutable value and safe to share.
type Policy struct {
	// MaxAttempts is the total number of attempts, including the first. A
	// value below 1 is treated as 1.
	MaxAttempts int
	// BaseBackoff is the delay before the second attempt. Each further delay
	// doubles. A non-positive value disables waiting between attempts.
	BaseBackoff time.Duration
	// MaxBackoff caps the exponential delay. A non-positive value imposes no
	// cap beyond the natural growth.
	MaxBackoff time.Duration
	// Jitter, when true, spreads each delay by adding a random duration in
	// [0, delay). Jitter is ignored when BaseBackoff is non-positive.
	Jitter bool
}

// DefaultPolicy returns a conservative retry policy: three attempts with
// exponential backoff from 100ms to 2s and jitter enabled. It is intended for
// read-only queries only.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts: 3,
		BaseBackoff: 100 * time.Millisecond,
		MaxBackoff:  2 * time.Second,
		Jitter:      true,
	}
}

// Do executes fn with retries as governed by p and class.
//
// A ClassMutation operation is issued exactly once regardless of p.MaxAttempts:
// order, futures, and algo mutations are never auto-retried
// (docs/adr/0003-no-auto-retry-orders.md). A read-only operation is retried
// only while the error satisfies errs.Retryable and the attempt budget is not
// exhausted. Do returns the number of attempts made and the last error; it
// returns (0, ctx.Err()) when ctx is already done and (attempts, ctx.Err()) when
// ctx is cancelled between attempts. Prefer DoRoute, which derives the class
// from the endpoint path.
func (p Policy) Do(ctx context.Context, class Class, fn AttemptFunc) (int, error) {
	if fn == nil {
		return 0, errors.New("resilience: nil attempt function")
	}
	max := p.MaxAttempts
	if class == ClassMutation {
		max = 1
	}
	if max < 1 {
		max = 1
	}

	attempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return attempts, err
		}
		attempts++
		err := fn(ctx)
		if err == nil {
			return attempts, nil
		}
		if attempts >= max || class == ClassMutation || !errs.Retryable(err) {
			return attempts, err
		}
		if serr := sleepCtx(ctx, p.Delay(attempts)); serr != nil {
			return attempts, serr
		}
	}
}

// DoRoute derives the retry class from path with ClassForPath and calls Do. It
// is the safe integration point: a path in the closed mutation set can never be
// retried, so a caller cannot enable global retry for an order mutation.
func (p Policy) DoRoute(ctx context.Context, path string, fn AttemptFunc) (int, error) {
	return p.Do(ctx, ClassForPath(path), fn)
}

// Delay returns the backoff before attempt n+1, where n is the number of
// attempts already made (so Delay(1) is the delay after the first failed
// attempt). The delay is BaseBackoff doubled n-1 times, capped at MaxBackoff,
// with optional jitter. It returns 0 when BaseBackoff is non-positive.
func (p Policy) Delay(n int) time.Duration {
	if p.BaseBackoff <= 0 || n < 1 {
		return 0
	}
	d := p.BaseBackoff
	for i := 1; i < n; i++ {
		if p.MaxBackoff > 0 && d >= p.MaxBackoff {
			break
		}
		d *= 2
		if d <= 0 {
			d = p.MaxBackoff
			break
		}
	}
	if p.MaxBackoff > 0 && d > p.MaxBackoff {
		d = p.MaxBackoff
	}
	if p.Jitter && d > 0 {
		half := d / 2
		if half > 0 {
			d = half + time.Duration(rand.Int63n(int64(half)+1)) // #nosec G404 -- retry jitter only; not a security-sensitive random value
		}
	}
	return d
}
