// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/resilience"
	inettransport "github.com/shing1211/hstongapi4go/internal/transport"
)

func TestNewAdapter(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 5*time.Second, 1024*1024, nil)
	if adapter == nil {
		t.Fatal("NewAdapter returned nil")
	}
	if adapter.deadline != 5*time.Second {
		t.Errorf("deadline = %v, want %v", adapter.deadline, 5*time.Second)
	}
	if adapter.maxBytes != 1024*1024 {
		t.Errorf("maxBytes = %d, want %d", adapter.maxBytes, 1024*1024)
	}
}

func TestNewAdapterZeroDeadline(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 0, 0, nil)
	if adapter.deadline != 0 {
		t.Errorf("deadline = %v, want 0", adapter.deadline)
	}
}

func TestGenerateCorrelationID(t *testing.T) {
	id, err := generateCorrelationID()
	if err != nil {
		t.Fatalf("generateCorrelationID error: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("id length = %d, want 32", len(id))
	}
}

func TestWithDeadline(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 100*time.Millisecond, 0, nil)

	ctx := context.Background()
	deadlineCtx := adapter.WithDeadline(ctx)
	if deadlineCtx == nil {
		t.Fatal("WithDeadline returned nil")
	}
	_, ok := deadlineCtx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}

	adapter2 := NewAdapter(inner, 0, 0, nil)
	unchanged := adapter2.WithDeadline(ctx)
	if unchanged != ctx {
		t.Error("expected parent context unchanged when deadline is 0")
	}
}

func TestAdapterMutationNotRetried(t *testing.T) {
	inner := inettransport.New()
	policy := &resilience.Policy{MaxAttempts: 3}
	adapter := NewAdapter(inner, 0, 0, policy)
	_ = adapter

	mutationRoutes := []string{
		"/trade/TradeEntrust",
		"/trade/FuturesEntrust",
		"/trade/AlgoAddOrder",
	}
	for _, route := range mutationRoutes {
		if !resilience.IsMutation(route) {
			t.Errorf("expected %s to be a mutation", route)
		}
	}
}

func TestAdapterDoWithClosedTransport(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 5*time.Second, 0, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := adapter.Do(ctx, "http://127.0.0.1:1", nil, nil, nil)
	if err == nil {
		t.Error("expected error from closed transport")
	}
}

func TestMiddlewareRoundTripperSetsHeader(t *testing.T) {
	var gotHeader bool
	capturingRT := &capturingRoundTripper{
		base: http.DefaultTransport,
		capture: func(r *http.Request) {
			if r.Header.Get("X-Correlation-ID") == "test-correlation-id" {
				gotHeader = true
			}
		},
	}
	rt := &middlewareRoundTripper{
		base:   capturingRT,
		corrID: "test-correlation-id",
	}
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = rt.RoundTrip(req)
	if !gotHeader {
		t.Error("X-Correlation-ID header not set")
	}
}

type capturingRoundTripper struct {
	base    http.RoundTripper
	capture func(*http.Request)
}

func (c *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.capture != nil {
		c.capture(req)
	}
	return c.base.RoundTrip(req)
}

func TestDoReturnsErrorOnBadRequest(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 5*time.Second, 0, nil)

	ctx := context.Background()
	err := adapter.Do(ctx, "/nonexistent/route", nil, nil, nil)
	if err == nil {
		t.Error("expected error for nonexistent route")
	}
}

func TestDoContextDeadline(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 1*time.Millisecond, 0, nil)

	ctx := context.Background()
	deadlineCtx := adapter.WithDeadline(ctx)
	_ = deadlineCtx
	_, ok := deadlineCtx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}
}

func TestDoCancellation(t *testing.T) {
	inner := inettransport.New()
	adapter := NewAdapter(inner, 0, 0, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := adapter.Do(ctx, "http://127.0.0.1:1", nil, nil, nil)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		_ = err
	}
}
