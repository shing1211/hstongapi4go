// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package resilience provides the optional hardening layer of the HStong
// (华盛) SDK: a stdlib-only token-bucket rate limiter, a retry policy with a
// hard mutation guard, and a three-state circuit breaker.
//
// Nothing in this package is active unless a caller opts in through the client
// options (client.WithRateLimiter, client.WithRetryPolicy,
// client.WithCircuitBreaker). The SDK's default behaviour remains one HTTP
// attempt per call with no rate limit and no breaker
// (docs/adr/0003-no-auto-retry-orders.md, docs/adr/0004-minimal-dependencies.md).
//
// # Mutation safety
//
// Order, futures, and algo mutation endpoints are a closed set in code
// (see IsMutation and MutationPaths). Policy.DoRoute derives the retry class
// from the endpoint path, and a mutation path is issued exactly once regardless
// of the configured attempt budget. Retrying a mutation is never authorized by
// a caller flag.
//
// # Concurrency
//
// Limiter and Breaker are safe for concurrent use. Policy is an immutable value
// and is safe to share.
package resilience
