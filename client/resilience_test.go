// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/metrics"
	"github.com/shing1211/hstongapi4go/internal/resilience"
)

// failureServer counts requests and returns the given envelope body.
func failureServer(t *testing.T, body string) (*httptest.Server, *int32) {
	t.Helper()
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&count, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

// TestNew_HardeningOptionsInertByDefault pins that the resilience and
// observability hooks are off unless a caller installs them.
func TestNew_HardeningOptionsInertByDefault(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if c.cfg.Logger != nil || c.cfg.Metrics != nil || c.cfg.RetryPolicy != nil ||
		c.cfg.RateLimiter != nil || c.cfg.CircuitBreaker != nil || c.instruments != nil {
		t.Fatalf("defaults are not inert: %+v", c.cfg)
	}
}

func TestNew_HardeningOptionsApply(t *testing.T) {
	c, err := New(
		WithLogger(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))),
		WithMetrics(metrics.Nop{}),
		WithRetryPolicy(resilience.DefaultPolicy()),
		WithRateLimiter(resilience.NewLimiter(resilience.Limit{TokensPerSecond: 10, Burst: 1}, nil)),
		WithCircuitBreaker(resilience.NewBreaker(3, time.Second)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	if c.cfg.Logger == nil || c.cfg.Metrics == nil || c.cfg.RetryPolicy == nil ||
		c.cfg.RateLimiter == nil || c.cfg.CircuitBreaker == nil || c.instruments == nil {
		t.Fatalf("options not applied: %+v", c.cfg)
	}
}

func TestRetryPolicyRetriesQuery(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&count, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = fmt.Fprint(w, `{"ok":false,"err":"1011"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true,"err":"","data":null}`)
	}))
	t.Cleanup(srv.Close)

	c, err := New(
		WithBaseURL(srv.URL),
		WithRetryPolicy(resilience.Policy{MaxAttempts: 3, BaseBackoff: 0}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if err := c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, nil, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := atomic.LoadInt32(&count); got != 2 {
		t.Fatalf("requests = %d, want 2 (one retry)", got)
	}
}

func TestRetryPolicyNeverRetriesMutation(t *testing.T) {
	srv, count := failureServer(t, `{"ok":false,"err":"1011"}`)

	c, err := New(
		WithBaseURL(srv.URL),
		WithRetryPolicy(resilience.Policy{MaxAttempts: 5, BaseBackoff: 0}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	err = c.Do(context.Background(), "trade/TradeEntrust", RouteTradeEntrust, nil, nil, nil)
	if err == nil {
		t.Fatal("Do: err = nil, want a failure")
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("mutation requests = %d, want exactly 1", got)
	}
}

func TestCircuitBreakerOpensAndRefuses(t *testing.T) {
	srv, count := failureServer(t, `{"ok":false,"err":"1011"}`)

	breaker := resilience.NewBreaker(1, time.Minute)
	c, err := New(WithBaseURL(srv.URL), WithCircuitBreaker(breaker))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if err := c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, nil, nil, nil); err == nil {
		t.Fatal("first Do: err = nil, want a failure")
	}
	if breaker.State() != resilience.StateOpen {
		t.Fatalf("breaker state = %v, want open", breaker.State())
	}
	err = c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, nil, nil, nil)
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("second Do = %v, want ErrCircuitOpen", err)
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("requests = %d, want 1 (second call refused)", got)
	}
}

func TestRateLimiterGatesCalls(t *testing.T) {
	srv, count := failureServer(t, `{"ok":true,"err":"","data":null}`)

	limiter := resilience.NewLimiter(resilience.Limit{TokensPerSecond: 0.001, Burst: 1}, nil)
	c, err := New(WithBaseURL(srv.URL), WithRateLimiter(limiter))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if err := c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, nil, nil, nil); err != nil {
		t.Fatalf("first Do: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = c.Do(ctx, "hq/BasicQot", RouteHqBasicQot, nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Do = %v, want context.DeadlineExceeded", err)
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("requests = %d, want 1 (second call gated)", got)
	}
}

// countingRecorder counts measurements per metric name.
type countingRecorder struct {
	requests int
	errors   int
}

func (r *countingRecorder) Count(name string, _ int64, _ ...string) {
	switch name {
	case metrics.MetricHTTPRequests:
		r.requests++
	case metrics.MetricHTTPErrors:
		r.errors++
	}
}

func (r *countingRecorder) Observe(string, float64, ...string) {}

func (r *countingRecorder) Gauge(string, float64, ...string) {}

func TestMetricsRecorderReceivesMeasurements(t *testing.T) {
	srv, _ := failureServer(t, `{"ok":false,"err":"1017"}`)

	rec := &countingRecorder{}
	c, err := New(WithBaseURL(srv.URL), WithMetrics(rec))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if err := c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, nil, nil, nil); err == nil {
		t.Fatal("Do: err = nil, want a failure")
	}
	if rec.requests != 1 || rec.errors != 1 {
		t.Fatalf("recorded requests=%d errors=%d, want 1/1", rec.requests, rec.errors)
	}
}

func TestLoggerNeverLogsRequestPayload(t *testing.T) {
	srv, _ := failureServer(t, `{"ok":true,"err":"","data":null}`)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, err := New(WithBaseURL(srv.URL), WithLogger(logger))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	const secret = "hunter2-trade-password"
	if err := c.Do(context.Background(), "trade/TradeLogin", RouteTradeLogin,
		map[string]string{"password": secret}, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("logger leaked the request payload: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "op=trade/TradeLogin") {
		t.Fatalf("logger did not record the operation label: %s", buf.String())
	}
}
