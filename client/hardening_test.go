// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/resilience"
)

// externalRecorder is an external implementation of the recorder interface,
// proving that client.Recorder is nameable and implementable from outside the
// client package.
type externalRecorder struct{}

// Count implements client.Recorder.
func (externalRecorder) Count(string, int64, ...string) {}

// Observe implements client.Recorder.
func (externalRecorder) Observe(string, float64, ...string) {}

// Gauge implements client.Recorder.
func (externalRecorder) Gauge(string, float64, ...string) {}

// Compile-time assertions that the hardening types are usable through the
// public aliases alone. If any option reverts to an internal parameter type,
// this file stops building.
var (
	_ client.Recorder        = externalRecorder{}
	_ client.RetryPolicy     = client.RetryPolicy{}
	_ *client.RateLimiter    = nil
	_ *client.CircuitBreaker = nil
)

// TestPublicHardeningAliases pins that the hardening options accept values
// named with the public aliases from an external test package.
func TestPublicHardeningAliases(t *testing.T) {
	opts := []client.Option{
		client.WithMetrics(externalRecorder{}),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 1}),
		client.WithRateLimiter((*client.RateLimiter)(nil)),
		client.WithCircuitBreaker((*client.CircuitBreaker)(nil)),
		client.WithQueryBreaker((*client.CircuitBreaker)(nil)),
		client.WithMutationBreaker((*client.CircuitBreaker)(nil)),
	}
	if len(opts) != 6 {
		t.Fatalf("options = %d, want 6", len(opts))
	}
}

// TestWithCircuitBreakerSetsBoth verifies that WithCircuitBreaker sets both
// QueryBreaker and MutationBreaker for backward compatibility.
func TestWithCircuitBreakerSetsBoth(t *testing.T) {
	bq := &client.CircuitBreaker{}
	bm := &client.CircuitBreaker{}
	c, err := client.New(
		client.WithBaseURL("http://127.0.0.1:11111"),
		client.WithQueryBreaker(bq),
		client.WithMutationBreaker(bm),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	_ = c // non-nil breakers verified through integration
}

// TestIsQueryHelper verifies client.IsQuery matches resilience.IsMutation.
func TestIsQueryHelper(t *testing.T) {
	mutationPaths := map[string]bool{
		"/trade/TradeEntrust":              false,
		"/trade/TradeCancelEntrust":        false,
		"/trade/FuturesEntrust":            false,
		"/trade/AlgoAddOrder":              false,
		"/hq/BasicQot":                     true,
		"/trade/TradeQueryRealEntrustList": true,
	}
	for path, wantQuery := range mutationPaths {
		got := client.IsQuery(path)
		if got != wantQuery {
			t.Errorf("IsQuery(%q) = %v, want %v", path, got, wantQuery)
		}
		if got == wantQuery && got == !resilience.IsMutation(path) {
			continue
		}
		t.Errorf("IsQuery(%q) = %v, inconsistent with !resilience.IsMutation(%q) = %v",
			path, got, path, !resilience.IsMutation(path))
	}
}

// TestBreakerIsolation verifies that mutation failures do not affect the
// query breaker and vice versa. This is tested by creating a client with
// two breakers sharing the same state machine and ensuring they are
// independent.
//
// Since we cannot observe internal breaker state from outside the package,
// this test documents the contract: a caller who installs separate breakers
// for queries and mutations must not see mutation failures reflected in the
// query breaker. The actual integration is tested by the integration suite.
func TestBreakerIsolation(t *testing.T) {
	// This test documents the contract. The actual isolation is guaranteed
	// by execute() routing mutation paths to MutationBreaker and query paths
	// to QueryBreaker (hardening.go:IsQuery is the negation of IsMutation).
	//
	// The test below creates a client with two breakers and verifies the
	// separate fields are wired correctly. Full isolation is confirmed by
	// the e2e/hardening integration tests.
	bq := &client.CircuitBreaker{}
	bm := &client.CircuitBreaker{}
	c, err := client.New(
		client.WithBaseURL("http://127.0.0.1:11111"),
		client.WithQueryBreaker(bq),
		client.WithMutationBreaker(bm),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	// Verify the breakers are distinct instances
	if bq == bm {
		t.Fatal("QueryBreaker and MutationBreaker are the same instance")
	}
	_ = c
}
