// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"github.com/shing1211/hstongapi4go/internal/metrics"
	"github.com/shing1211/hstongapi4go/internal/resilience"
)

// Recorder is the public name for the SDK's metrics recorder. It is an alias
// for the internal recorder interface, so external callers can implement it and
// name it without importing an internal package. Implementations must be safe
// for concurrent use; the methods are Count, Observe, and Gauge. A nil Recorder
// (the default) records nothing. See the observability guide for the metric
// names and labels the SDK emits.
type Recorder = metrics.Recorder

// RetryPolicy is the public name for the read-only retry configuration. It is
// an alias for the internal policy type, so external callers can construct a
// RetryPolicy value directly and pass it to WithRetryPolicy. The zero value
// makes exactly one attempt. Mutations are never retried regardless of the
// policy (docs/adr/0003-no-auto-retry-orders.md).
type RetryPolicy = resilience.Policy

// RateLimiter is the public name for the token-bucket rate limiter. It is an
// alias for the internal limiter type; its methods are Wait and WaitEndpoint.
// A nil *RateLimiter (the default) imposes no rate limit.
type RateLimiter = resilience.Limiter

// CircuitBreaker is the public name for the circuit breaker. It is an alias for
// the internal breaker type; its methods are Allow, OnSuccess, OnFailure, and
// State. A nil *CircuitBreaker (the default) disables breaking.
type CircuitBreaker = resilience.Breaker

// IsQuery reports whether path is a read-only ClassQuery endpoint. It is the
// negation of resilience.IsMutation and exists so callers outside the resilience
// package can classify a path without importing it.
func IsQuery(path string) bool {
	return !resilience.IsMutation(path)
}
