// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestWithCorrelationIDHeaderPlumbsThrough guards the seam between the public
// option and internal/transport. The transport-level tests prove the header is
// generated correctly; this proves client.New actually forwards the configured
// name, which is the link most likely to be broken by a refactor that adds an
// option to one layer and forgets the other.
func TestWithCorrelationIDHeaderPlumbsThrough(t *testing.T) {
	const header = "X-Correlation-ID"
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get(header))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"err":"","data":{}}`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithBaseURL(srv.URL), WithCorrelationIDHeader(header))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Do(t.Context(), "op", RouteHqBasicQot, nil, c.JSON(), nil); err != nil {
		t.Fatalf("Do: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), seen...)
	mu.Unlock()

	if len(got) != 1 {
		t.Fatalf("captured %d headers, want 1", len(got))
	}
	if got[0] == "" {
		t.Fatalf("%s was not forwarded to the transport", header)
	}
	if len(got[0]) != 32 {
		t.Errorf("correlation id %q has length %d, want 32 hex characters", got[0], len(got[0]))
	}
}

// TestCorrelationIDHeaderOffByDefaultAtClientLevel is the ADR 0011 guarantee at
// the public surface: a Client built with no correlation option must not put the
// header on the wire.
func TestCorrelationIDHeaderOffByDefaultAtClientLevel(t *testing.T) {
	const header = "X-Correlation-ID"
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get(header))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"err":"","data":{}}`))
	}))
	t.Cleanup(srv.Close)

	c, err := New(WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()

	for range 2 {
		if err := c.Do(t.Context(), "op", RouteHqBasicQot, nil, c.JSON(), nil); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}

	mu.Lock()
	got := append([]string(nil), seen...)
	mu.Unlock()

	for i, id := range got {
		if id != "" {
			t.Errorf("request %d carried %s %q, want the header absent by default", i, header, id)
		}
	}
}
