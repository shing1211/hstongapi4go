// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/internal/resilience"
	inettransport "github.com/shing1211/hstongapi4go/internal/transport"
)

// Adapter wraps internal/transport.Transport with enterprise middleware:
// per-request deadlines, response-size caps, correlation IDs, pagination,
// and query-only retry.
type Adapter struct {
	// inner is the underlying HTTP executor.
	inner *inettransport.Transport
	// baseURL is the Gateway root used by the executor transport.
	baseURL string
	// deadline is the per-request timeout.
	deadline time.Duration
	// maxBytes is the maximum response body size in bytes. 0 means no cap.
	maxBytes int64
	// policy is the query-only retry policy. nil means no retry.
	policy *resilience.Policy
	// deadlineCancel stores the cancel function for the last
	// WithDeadline call so the underlying timer can be released.
	deadlineCancel context.CancelFunc
	// deadlineMu protects deadlineCancel for concurrent use.
	deadlineMu sync.Mutex
}

// NewAdapter creates an Adapter wrapping the existing Transport.
// deadline: per-request timeout (0 = use Transport's default).
// maxBytes: max response body size in bytes (0 = no cap).
// policy: query-only retry policy (nil = no retry).
func NewAdapter(inner *inettransport.Transport, deadline time.Duration, maxBytes int64, policy *resilience.Policy) *Adapter {
	return &Adapter{
		inner:    inner,
		baseURL:  inettransport.DefaultBaseURL,
		deadline: deadline,
		maxBytes: maxBytes,
		policy:   policy,
	}
}

// Do executes one HTTP request with enterprise middleware applied.
// It injects a correlation ID header, applies the deadline, enforces
// the response-size cap, and wraps the inner Transport.Do call.
// Mutations (checked via resilience.IsMutation) are never retried
// regardless of the policy.
func (a *Adapter) Do(ctx context.Context, route string, codec inettransport.Codec, params any, out any) error {
	corrID, err := generateCorrelationID()
	if err != nil {
		return errs.Wrap(err, "", route, "generate correlation id")
	}

	executor := a.executor(corrID)

	if resilience.IsMutation(route) {
		return executor.Do(ctx, route, route, params, codec, out)
	}

	if a.policy != nil {
		_, err = a.policy.DoRoute(ctx, route, func(ctx context.Context) error {
			return executor.Do(ctx, route, route, params, codec, out)
		})
		return err
	}

	return executor.Do(ctx, route, route, params, codec, out)
}

// executor returns a Transport that applies the correlation ID header
// and response-size cap via a custom HTTP client round tripper.
func (a *Adapter) executor(corrID string) *inettransport.Transport {
	client := a.httpClient(corrID)
	return inettransport.New(
		inettransport.WithBaseURL(a.baseURL),
		inettransport.WithHTTPClient(client),
		inettransport.WithDefaultTimeout(a.deadline),
	)
}

// httpClient returns an *http.Client whose round tripper injects the
// correlation ID header and enforces the response-size cap.
func (a *Adapter) httpClient(corrID string) *http.Client {
	return &http.Client{
		Transport: &middlewareRoundTripper{
			base:     http.DefaultTransport,
			corrID:   corrID,
			maxBytes: a.maxBytes,
		},
		Timeout: a.deadline,
	}
}

// middlewareRoundTripper wraps an http.RoundTripper to inject
// X-Correlation-ID and enforce response-size caps.
type middlewareRoundTripper struct {
	base     http.RoundTripper
	corrID   string
	maxBytes int64
}

func (r *middlewareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Correlation-ID", r.corrID)
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if r.maxBytes > 0 {
		resp.Body = http.MaxBytesReader(nil, resp.Body, r.maxBytes)
	}
	return resp, err
}

// generateCorrelationID produces a random 16-byte hex string for X-Correlation-ID.
func generateCorrelationID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// WithDeadline returns a context with the adapter's deadline applied,
// or the parent context unchanged if deadline is 0.
func (a *Adapter) WithDeadline(ctx context.Context) context.Context {
	if a.deadline <= 0 {
		return ctx
	}
	a.deadlineMu.Lock()
	defer a.deadlineMu.Unlock()
	if a.deadlineCancel != nil {
		a.deadlineCancel()
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, a.deadline)
	a.deadlineCancel = cancel
	return deadlineCtx
}
