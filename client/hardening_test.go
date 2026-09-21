// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"testing"

	"github.com/shing1211/hstongapi4go/client"
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

// TestPublicHardeningAliases pins that the four hardening options accept values
// named with the public aliases from an external test package.
func TestPublicHardeningAliases(t *testing.T) {
	opts := []client.Option{
		client.WithMetrics(externalRecorder{}),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 1}),
		client.WithRateLimiter((*client.RateLimiter)(nil)),
		client.WithCircuitBreaker((*client.CircuitBreaker)(nil)),
	}
	if len(opts) != 4 {
		t.Fatalf("options = %d, want 4", len(opts))
	}
}
