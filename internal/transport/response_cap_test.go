// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResponseCapRejectsOversizedBody covers R7 in the threat model: the
// transport used to buffer an unbounded body with io.ReadAll, so a malformed or
// hostile Gateway response could force an arbitrarily large allocation.
func TestResponseCapRejectsOversizedBody(t *testing.T) {
	const cap = 512

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"blob":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", 4*cap)))
		_, _ = w.Write([]byte(`"}}`))
	}))
	defer srv.Close()

	tr := New(WithBaseURL(srv.URL), WithMaxResponseBytes(cap))

	var out map[string]any
	err := tr.Do(context.Background(), "oversized", "/oversized", nil, nil, &out)
	if err == nil {
		t.Fatal("Do() = nil, want an error for a body over the cap")
	}
	if !strings.Contains(err.Error(), "exceeds the 512 byte cap") {
		t.Errorf("Do() error = %v, want it to name the cap", err)
	}
}

// TestResponseCapAllowsBodyAtLimit guards the boundary: a body that exactly
// fills the cap must still succeed, so the limit is inclusive and the check is
// not off by one.
func TestResponseCapAllowsBodyAtLimit(t *testing.T) {
	payload := `{"ok":true,"data":{"v":"` + strings.Repeat("y", 64) + `"}}`
	capBytes := int64(len(payload))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	tr := New(WithBaseURL(srv.URL), WithMaxResponseBytes(capBytes))

	var out struct {
		V string `json:"v"`
	}
	if err := tr.Do(context.Background(), "atlimit", "/atlimit", nil, nil, &out); err != nil {
		t.Fatalf("Do() at exactly the cap = %v, want nil", err)
	}
	if len(out.V) != 64 {
		t.Errorf("decoded v length = %d, want 64", len(out.V))
	}
}

// TestDefaultResponseCapIsApplied verifies the constructor does not leave the
// cap unset, which would silently restore the unbounded behaviour this change
// removes.
func TestDefaultResponseCapIsApplied(t *testing.T) {
	tr := New()
	if tr.maxResponseBytes != DefaultMaxResponseBytes {
		t.Errorf("maxResponseBytes = %d, want DefaultMaxResponseBytes (%d)",
			tr.maxResponseBytes, DefaultMaxResponseBytes)
	}
}

// TestNonPositiveCapRestoresDefault documents that an accidental 0 does not
// disable the cap. Disabling must be a deliberate choice, not a zero value.
func TestNonPositiveCapRestoresDefault(t *testing.T) {
	for _, n := range []int64{0, -1} {
		tr := New(WithMaxResponseBytes(n))
		if tr.maxResponseBytes != DefaultMaxResponseBytes {
			t.Errorf("WithMaxResponseBytes(%d): maxResponseBytes = %d, want the default %d",
				n, tr.maxResponseBytes, DefaultMaxResponseBytes)
		}
	}
}

// TestLargerExplicitCapIsHonoured confirms the option is not clamped back to the
// default, so a caller with a genuinely large payload can raise the ceiling.
func TestLargerExplicitCapIsHonoured(t *testing.T) {
	const cap = 32 << 20
	tr := New(WithMaxResponseBytes(cap))
	if tr.maxResponseBytes != cap {
		t.Errorf("maxResponseBytes = %d, want %d", tr.maxResponseBytes, cap)
	}
}
