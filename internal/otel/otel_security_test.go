// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

//go:build otel

package otel_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/shing1211/hstongapi4go/internal/otel"
	"github.com/shing1211/hstongapi4go/internal/transport"
)

func newRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return rec
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }

func TestEndSpanRedactsKeyShapedSecrets(t *testing.T) {
	const secret = "sup3rs3cret"
	cases := []struct {
		name string
		err  error
	}{
		{"keyed query", errors.New("login rejected: password=" + secret)},
		{"keyed json", errors.New(`body {"tradePassword":"` + secret + `"}`)},
		{"masked body", errors.New(`gateway returned HTTP 500: {"password":"***"}`)},
		{"no error", nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := newRecorder(t)
			_, span := otel.StartSpan(context.Background(), "test")
			otel.EndSpan(span, tt.err)

			spans := rec.Ended()
			if len(spans) != 1 {
				t.Fatalf("recorded %d spans, want 1", len(spans))
			}
			for _, ev := range spans[0].Events() {
				for _, a := range ev.Attributes {
					if v := a.Value.Emit(); strings.Contains(v, secret) {
						t.Fatalf("span attribute leaked the secret: %s=%s", a.Key, v)
					}
				}
			}
			if d := spans[0].Status().Description; strings.Contains(d, secret) {
				t.Fatalf("span status leaked the secret: %q", d)
			}
		})
	}
}

// TestSafeErrorLeavesUnkeyedTextAlone pins the documented boundary of
// SafeError: redaction is key-driven, so a secret embedded in free text with
// no recognisable key cannot be detected and is passed through unchanged.
// Callers must not rely on SafeError to scrub arbitrary prose.
func TestSafeErrorLeavesUnkeyedTextAlone(t *testing.T) {
	err := errors.New("login failed for sup3rs3cret")
	if got := otel.SafeError(err).Error(); got != err.Error() {
		t.Fatalf("SafeError(%q) = %q, want it unchanged", err.Error(), got)
	}
	if otel.SafeError(nil) != nil {
		t.Fatal("SafeError(nil) != nil, want nil")
	}
}

// TestTransportErrorIsSafeForSpans proves the end-to-end property: a Gateway or
// proxy that echoes a credential in a non-2xx body cannot get that credential
// into the error a span would record.
func TestTransportErrorIsSafeForSpans(t *testing.T) {
	const secret = "sup3rs3cret"
	rec := newRecorder(t)

	body := `{"error":"rejected","tradePassword":"` + secret + `"}`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Body:       nopCloser{strings.NewReader(body)},
			Header:     make(http.Header),
		}, nil
	})}
	tr := transport.New(transport.WithHTTPClient(client))

	_, span := otel.StartSpan(context.Background(), "transport")
	err := tr.Do(context.Background(), "op", "/hq/BasicQot", struct{}{}, transport.JSONCodec{}, &struct{}{})
	otel.EndSpan(span, err)

	if err == nil {
		t.Fatal("Do() error = nil, want a non-2xx error")
	}
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	for _, ev := range spans[0].Events() {
		if strings.Contains(ev.Name, secret) {
			t.Fatalf("span event leaked the echoed secret: %q", ev.Name)
		}
	}
	if strings.Contains(spans[0].Status().Description, secret) {
		t.Fatalf("span status leaked the echoed secret: %q", spans[0].Status().Description)
	}
}
