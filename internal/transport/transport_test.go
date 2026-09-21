// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// countingServer wraps handler, counts every inbound request, and closes the
// server at test cleanup.
func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *int32) {
	t.Helper()
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("writing response: %v", err)
	}
}

type quoteRequest struct {
	Code string `json:"code"`
}

type quoteData struct {
	Code  string      `json:"code"`
	Price string      `json:"price"`
	Qty   json.Number `json:"qty"`
}

func TestDo_JSONRoundTrip(t *testing.T) {
	srv, count := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if got := r.URL.Path; got != "/hq/BasicQot" {
			t.Errorf("path = %q, want /hq/BasicQot", got)
		}

		var env request
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decoding envelope: %v", err)
			return
		}
		if env.TimeoutSec != 10 {
			t.Errorf("timeout_sec = %d, want 10", env.TimeoutSec)
		}
		var params quoteRequest
		if err := json.Unmarshal(env.Params, &params); err != nil {
			t.Errorf("decoding params: %v", err)
			return
		}
		if params.Code != "AAPL" {
			t.Errorf("params.code = %q, want AAPL", params.Code)
		}

		writeJSON(t, w, `{"ok":true,"err":"","data":{"code":"AAPL","price":"123.45","qty":100}}`)
	})

	tr := New(WithBaseURL(srv.URL))
	var out quoteData
	if err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", quoteRequest{Code: "AAPL"}, JSONCodec{}, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.Code != "AAPL" || out.Price != "123.45" || out.Qty != json.Number("100") {
		t.Fatalf("out = %+v, want decoded data", out)
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestDo_OKFalseIsTypedError(t *testing.T) {
	tests := []struct {
		name            string
		errText         string
		wantCode        types.StatusCode
		wantKnownCode   bool
		wantRetryable   bool
		wantReLogin     bool
		wantMsgContains string
	}{
		{
			name:            "retryable service busy",
			errText:         "1011",
			wantCode:        types.StatusServiceBusy,
			wantKnownCode:   true,
			wantRetryable:   true,
			wantMsgContains: "service busy",
		},
		{
			name:            "re-login required",
			errText:         "1012",
			wantCode:        types.StatusNotLoggedIn,
			wantKnownCode:   true,
			wantRetryable:   false,
			wantReLogin:     true,
			wantMsgContains: "not logged in",
		},
		{
			name:            "code followed by gateway text",
			errText:         "1014 login expired",
			wantCode:        types.StatusLoginTimeout,
			wantKnownCode:   true,
			wantRetryable:   false,
			wantReLogin:     true,
			wantMsgContains: "login expired",
		},
		{
			name:            "trading rejection is never retryable",
			errText:         "1007",
			wantCode:        types.StatusDuplicateSubmit,
			wantKnownCode:   true,
			wantRetryable:   false,
			wantMsgContains: "duplicate submission",
		},
		{
			name:            "free text without a code",
			errText:         "gateway exploded",
			wantKnownCode:   false,
			wantRetryable:   false,
			wantMsgContains: "gateway exploded",
		},
		{
			name:            "empty error text",
			wantKnownCode:   false,
			wantRetryable:   false,
			wantMsgContains: "no error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := json.Marshal(response{OK: false, Err: tt.errText})
				writeJSON(t, w, string(body))
			})
			tr := New(WithBaseURL(srv.URL))

			err := tr.Do(context.Background(), "trade/TradeEntrust", "/trade/TradeEntrust", nil, nil, nil)
			if err == nil {
				t.Fatal("Do returned nil, want typed error")
			}

			var typed *errs.Error
			if !errors.As(err, &typed) {
				t.Fatalf("error type = %T, want *errs.Error", err)
			}
			code, ok := errs.CodeOf(err)
			if ok != tt.wantKnownCode {
				t.Fatalf("CodeOf known = %v, want %v (code %q)", ok, tt.wantKnownCode, code)
			}
			if tt.wantKnownCode && code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if got := errs.Retryable(err); got != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", got, tt.wantRetryable)
			}
			if got := errs.ReLoginRequired(err); got != tt.wantReLogin {
				t.Errorf("ReLoginRequired = %v, want %v", got, tt.wantReLogin)
			}
			if !strings.Contains(err.Error(), tt.wantMsgContains) {
				t.Errorf("Error() = %q, want to contain %q", err.Error(), tt.wantMsgContains)
			}
		})
	}
}

func TestDo_Non2xxIsTypedError(t *testing.T) {
	srv, count := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("gateway offline")); err != nil {
			t.Errorf("writing response: %v", err)
		}
	})
	tr := New(WithBaseURL(srv.URL))

	err := tr.Do(context.Background(), "hq/KL", "/hq/KL", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil, want typed error")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errs.Error", err)
	}
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "gateway offline") {
		t.Fatalf("Error() = %q, want HTTP status and body snippet", err.Error())
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("requests = %d, want exactly 1", got)
	}
}

func TestDo_MalformedJSONDoesNotPanic(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"ok":true,"data":`)
	})
	tr := New(WithBaseURL(srv.URL))

	err := tr.Do(context.Background(), "hq/Ticker", "/hq/Ticker", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil for malformed JSON")
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errs.Error", err)
	}
	if !strings.Contains(err.Error(), "hq/Ticker") {
		t.Fatalf("Error() = %q, want op context", err.Error())
	}
}

func TestDo_ContextCancellationHonored(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"ok":true,"err":"","data":{}}`)
	})
	tr := New(WithBaseURL(srv.URL))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := tr.Do(ctx, "hq/BasicQot", "/hq/BasicQot", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil after context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v does not wrap context.Canceled", err)
	}
	var typed *errs.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *errs.Error", err)
	}
}

func TestDo_ContextDeadlineShrinksEnvelopeTimeout(t *testing.T) {
	var seen int32 = -1
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		var env request
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decoding envelope: %v", err)
			return
		}
		atomic.StoreInt32(&seen, int32(env.TimeoutSec))
		writeJSON(t, w, `{"ok":true,"err":"","data":null}`)
	})
	tr := New(WithBaseURL(srv.URL), WithDefaultTimeout(30*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := tr.Do(ctx, "hq/KL", "/hq/KL", nil, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	got := atomic.LoadInt32(&seen)
	if got < 1 || got > 3 {
		t.Fatalf("timeout_sec = %d, want 1..3 (shrunk to context deadline)", got)
	}
}

func TestDo_NullAndMissingDataLeaveOutUntouched(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"null data", `{"ok":true,"err":"","data":null}`},
		{"missing data", `{"ok":true,"err":""}`},
		{"empty object", `{"ok":true,"err":"","data":{}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, tt.body)
			})
			tr := New(WithBaseURL(srv.URL))

			out := quoteData{Code: "UNCHANGED", Price: "1.00", Qty: json.Number("1")}
			if err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", nil, JSONCodec{}, &out); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if out.Code != "UNCHANGED" || out.Price != "1.00" || out.Qty != json.Number("1") {
				t.Fatalf("out = %+v, want untouched", out)
			}
		})
	}
}

// TestDo_SingleAttemptOnFailure encodes ADR 0003: the transport never retries,
// even for transient, retryable conditions.
func TestDo_SingleAttemptOnFailure(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "retryable gateway code 1011",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, `{"ok":false,"err":"1011"}`)
			},
		},
		{
			name: "connection-level HTTP 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, count := countingServer(t, tt.handler)
			tr := New(WithBaseURL(srv.URL), WithDefaultTimeout(time.Second))

			for i := 0; i < 3; i++ {
				if err := tr.Do(context.Background(), "trade/TradeEntrust", "/trade/TradeEntrust", nil, nil, nil); err == nil {
					t.Fatal("Do returned nil, want failure")
				}
			}
			if got := atomic.LoadInt32(count); got != 3 {
				t.Fatalf("requests = %d, want 3 (exactly one per Do call, no retries)", got)
			}
		})
	}
}

func TestDo_ConcurrentUseIsRaceClean(t *testing.T) {
	srv, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"ok":true,"err":"","data":{"code":"MSFT","price":"400.00","qty":5}}`)
	})
	tr := New(WithBaseURL(srv.URL))

	const goroutines = 64
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out quoteData
			if err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", quoteRequest{Code: "MSFT"}, JSONCodec{}, &out); err != nil {
				errCh <- err
				return
			}
			if out.Code != "MSFT" {
				errCh <- fmt.Errorf("out.Code = %q, want MSFT", out.Code)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Do: %v", err)
	}
}

func TestNew_Options(t *testing.T) {
	tr := New()
	if tr.baseURL != DefaultBaseURL {
		t.Fatalf("default baseURL = %q, want %q", tr.baseURL, DefaultBaseURL)
	}
	if tr.defaultTimeout != DefaultTimeout {
		t.Fatalf("default timeout = %v, want %v", tr.defaultTimeout, DefaultTimeout)
	}
	if tr.client.Timeout != DefaultTimeout {
		t.Fatalf("client timeout = %v, want %v", tr.client.Timeout, DefaultTimeout)
	}

	tr = New(WithBaseURL("http://example.test:9/"), WithDefaultTimeout(2*time.Second))
	if tr.baseURL != "http://example.test:9" {
		t.Fatalf("baseURL = %q, want trailing slash trimmed", tr.baseURL)
	}
	if tr.defaultTimeout != 2*time.Second || tr.client.Timeout != 2*time.Second {
		t.Fatalf("timeout = %v / client %v, want 2s", tr.defaultTimeout, tr.client.Timeout)
	}

	custom := &http.Client{Timeout: 7 * time.Second}
	tr = New(WithHTTPClient(custom), WithDefaultTimeout(2*time.Second))
	if tr.client != custom {
		t.Fatal("explicit client timeout must be preserved, client should be used unchanged")
	}
	if tr.defaultTimeout != 2*time.Second {
		t.Fatalf("envelope timeout = %v, want 2s", tr.defaultTimeout)
	}

	tr = New(WithBaseURL(""), WithDefaultTimeout(0))
	if tr.baseURL != DefaultBaseURL || tr.defaultTimeout != DefaultTimeout {
		t.Fatalf("empty options did not restore defaults: %q / %v", tr.baseURL, tr.defaultTimeout)
	}
}
