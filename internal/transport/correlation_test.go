// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// okEnvelope is a minimal successful Gateway response.
const okEnvelope = `{"ok":true,"err":"","data":{}}`

// captureIDs starts a server that records the correlation header of every
// request, so a test can assert on what the client actually put on the wire
// rather than on internal state.
func captureIDs(t *testing.T, header string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var ids []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ids = append(ids, r.Header.Get(header))
		mu.Unlock()
		writeJSON(t, w, okEnvelope)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(ids))
		copy(out, ids)
		return out
	}
}

// TestCorrelationIDHeaderIsOffByDefault is the ADR 0011 guarantee: with no
// option, the SDK puts nothing new on the wire.
func TestCorrelationIDHeaderIsOffByDefault(t *testing.T) {
	srv, ids := captureIDs(t, "X-Correlation-ID")
	tr := New(WithBaseURL(srv.URL))

	for range 3 {
		if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}

	for i, got := range ids() {
		if got != "" {
			t.Errorf("request %d carried X-Correlation-ID %q, want no header by default", i, got)
		}
	}
}

// TestCorrelationIDHeaderSetWhenEnabled asserts the header reaches the Gateway
// and that each value is a well-formed 128-bit hex identifier.
func TestCorrelationIDHeaderSetWhenEnabled(t *testing.T) {
	const header = "X-Correlation-ID"
	srv, ids := captureIDs(t, header)
	tr := New(WithBaseURL(srv.URL), WithCorrelationIDHeader(header))

	for range 3 {
		if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}

	got := ids()
	if len(got) != 3 {
		t.Fatalf("captured %d ids, want 3", len(got))
	}
	for i, id := range got {
		if id == "" {
			t.Errorf("request %d had an empty %s", i, header)
			continue
		}
		if len(id) != 32 {
			t.Errorf("request %d id %q has length %d, want 32 hex characters", i, id, len(id))
		}
		raw, err := hex.DecodeString(id)
		if err != nil {
			t.Errorf("request %d id %q is not hex: %v", i, id, err)
			continue
		}
		if len(raw) != 16 {
			t.Errorf("request %d id decodes to %d bytes, want 16", i, len(raw))
		}
		if id != lowerHex(id) {
			t.Errorf("request %d id %q is not lowercase hex", i, id)
		}
	}
}

// TestCorrelationIDIsUniquePerRequest is the property that makes the header
// useful: two requests must never share an identifier, or correlating a log
// line to a call becomes ambiguous.
func TestCorrelationIDIsUniquePerRequest(t *testing.T) {
	const header = "X-Correlation-ID"
	srv, ids := captureIDs(t, header)
	tr := New(WithBaseURL(srv.URL), WithCorrelationIDHeader(header))

	const n = 25
	for range n {
		if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}

	seen := make(map[string]int, n)
	for i, id := range ids() {
		if prev, dup := seen[id]; dup {
			t.Errorf("requests %d and %d share correlation id %q", prev, i, id)
			continue
		}
		seen[id] = i
	}
	if len(seen) != n {
		t.Errorf("got %d distinct ids across %d requests", len(seen), n)
	}
}

// TestCorrelationIDUniqueUnderConcurrency guards the same property when calls
// overlap, which is the normal case for a trading client.
func TestCorrelationIDUniqueUnderConcurrency(t *testing.T) {
	const header = "X-Correlation-ID"
	srv, ids := captureIDs(t, header)
	tr := New(WithBaseURL(srv.URL), WithCorrelationIDHeader(header))

	const workers, each = 8, 10
	var wg sync.WaitGroup
	errsCh := make(chan error, workers*each)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err != nil {
					errsCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		t.Fatalf("concurrent Do: %v", err)
	}

	got := ids()
	if len(got) != workers*each {
		t.Fatalf("captured %d ids, want %d", len(got), workers*each)
	}
	seen := make(map[string]struct{}, len(got))
	for _, id := range got {
		if id == "" {
			t.Fatal("a concurrent request carried an empty correlation id")
		}
		if _, dup := seen[id]; dup {
			t.Errorf("correlation id %q was reused across concurrent requests", id)
		}
		seen[id] = struct{}{}
	}
}

// TestCorrelationIDHeaderNameIsConfigurable covers a caller that already has a
// header convention, and pins that the configured name is the one used.
func TestCorrelationIDHeaderNameIsConfigurable(t *testing.T) {
	const header = "X-Shing-Request-Id"
	srv, ids := captureIDs(t, header)
	// Also assert the default name stays absent, so a custom name does not
	// accidentally enable the conventional one as well.
	standard, standardIDs := captureIDs(t, "X-Correlation-ID")
	_ = standard

	tr := New(WithBaseURL(srv.URL), WithCorrelationIDHeader(header))
	if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}

	got := ids()
	if len(got) != 1 || got[0] == "" {
		t.Fatalf("custom header not set: %v", got)
	}
	if len(standardIDs()) != 0 {
		t.Error("the default header name was set even though a custom name was configured")
	}
}

// TestCorrelationIDWhitespaceNameDisablesHeader pins the normalization: a
// whitespace-only name is not a usable header, so it must behave as off rather
// than producing a request net/http rejects.
func TestCorrelationIDWhitespaceNameDisablesHeader(t *testing.T) {
	srv, ids := captureIDs(t, "X-Correlation-ID")
	tr := New(WithBaseURL(srv.URL), WithCorrelationIDHeader("   \t  "))

	if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := ids(); len(got) != 1 || got[0] != "" {
		t.Errorf("whitespace header name produced %v, want the header absent", got)
	}
}

// TestCorrelationIDSurvivesFailedRequest asserts the identifier is attached
// before the request is sent, so a 5xx or malformed body is still traceable —
// which is when correlating a log line matters most.
func TestCorrelationIDSurvivesFailedRequest(t *testing.T) {
	const header = "X-Correlation-ID"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(header) == "" {
			t.Error("no correlation id on a request that is about to fail")
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	tr := New(WithBaseURL(srv.URL), WithCorrelationIDHeader(header))
	if err := tr.Do(context.Background(), "/x", "/x", nil, nil, nil); err == nil {
		t.Fatal("Do on a 500 = nil, want an error")
	}
}

// TestNewCorrelationIDFormat pins the generator itself, so a future change to
// the encoding is caught here rather than by a log reader.
func TestNewCorrelationIDFormat(t *testing.T) {
	const n = 50
	seen := make(map[string]struct{}, n)
	for range n {
		id, err := newCorrelationID()
		if err != nil {
			t.Fatalf("newCorrelationID: %v", err)
		}
		if len(id) != 32 {
			t.Fatalf("id %q has length %d, want 32", id, len(id))
		}
		if id != lowerHex(id) {
			t.Fatalf("id %q is not lowercase hex", id)
		}
		raw, err := hex.DecodeString(id)
		if err != nil {
			t.Fatalf("id %q is not hex: %v", id, err)
		}
		if len(raw) != 16 {
			t.Fatalf("id %q decodes to %d bytes, want 16", id, len(raw))
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("newCorrelationID returned %q twice", id)
		}
		seen[id] = struct{}{}
	}
}

// lowerHex returns s unchanged when it is already lowercase hex, and a
// distinguishable value otherwise, so it can be used as a cheap assertion.
func lowerHex(s string) string {
	b := []byte(s)
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return ""
		}
	}
	return s
}
