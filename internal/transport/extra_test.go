// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// roundTripFunc adapts a function to http.RoundTripper so a Transport can be
// driven without a real server for the failure paths.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// errReadCloser returns a read error, exercising the body-read failure path.
type errReadCloser struct{ err error }

func (b errReadCloser) Read([]byte) (int, error) { return 0, b.err }
func (errReadCloser) Close() error               { return nil }

func TestDo_BodyReadFailureIsConnectionError(t *testing.T) {
	readErr := errors.New("stream reset")
	tr := New(
		WithBaseURL("http://example.test"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       errReadCloser{err: readErr},
			}, nil
		})}),
	)

	err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil, want a body-read error")
	}
	if !errors.Is(err, readErr) {
		t.Fatalf("error %v does not wrap the read cause", err)
	}
	if got := errs.CategoryOf(err); got != errs.CategoryConnection {
		t.Fatalf("category = %q, want %q", got, errs.CategoryConnection)
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Fatalf("Error() = %q, want body-read context", err.Error())
	}
}

func TestDo_DialDeadlineIsTimeoutError(t *testing.T) {
	tr := New(
		WithBaseURL("http://example.test"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})}),
	)

	err := tr.Do(context.Background(), "hq/KL", "/hq/KL", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil, want a timeout error")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
		t.Fatalf("category = %q, want %q", got, errs.CategoryTimeout)
	}
}

func TestTimeoutSeconds_NeverBelowOne(t *testing.T) {
	short := New(WithDefaultTimeout(500 * time.Millisecond))
	if got := short.timeoutSeconds(context.Background()); got != 1 {
		t.Fatalf("timeoutSeconds(sub-second default) = %d, want 1", got)
	}

	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := short.timeoutSeconds(expired); got != 1 {
		t.Fatalf("timeoutSeconds(expired deadline) = %d, want 1", got)
	}
}

func TestClassifyFailure_CodeWithSeparatorOnly(t *testing.T) {
	code, message := classifyFailure("1011 :")
	if code != types.StatusServiceBusy {
		t.Fatalf("code = %q, want %q", code, types.StatusServiceBusy)
	}
	if message != errs.MessageForCode(types.StatusServiceBusy) {
		t.Fatalf("message = %q, want the table message", message)
	}

	if code, message := classifyFailure("   "); code != "" || message != "" {
		t.Fatalf("blank classifyFailure = (%q, %q), want empty", code, message)
	}
}

func TestDo_BodyReadDeadlineIsTimeout(t *testing.T) {
	tr := New(
		WithBaseURL("http://example.test"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       errReadCloser{err: context.DeadlineExceeded},
			}, nil
		})}),
	)

	err := tr.Do(context.Background(), "hq/KL", "/hq/KL", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil, want a read-deadline error")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
		t.Fatalf("category = %q, want %q", got, errs.CategoryTimeout)
	}
}

func TestDo_EncodeParamsFailure(t *testing.T) {
	tr := New(WithBaseURL("http://example.test"))
	err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", make(chan int), JSONCodec{}, nil)
	if err == nil {
		t.Fatal("Do returned nil for an unmarshalable params value")
	}
	if !strings.Contains(err.Error(), "encode params") {
		t.Fatalf("Error() = %q, want encode-params context", err.Error())
	}
}

func TestDo_BuildRequestFailure(t *testing.T) {
	tr := New(WithBaseURL("://bad"))
	err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", nil, nil, nil)
	if err == nil {
		t.Fatal("Do returned nil for an unparsable base URL")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Fatalf("Error() = %q, want build-request context", err.Error())
	}
}

func TestDo_DecodeResponseDataFailure(t *testing.T) {
	tr := New(
		WithBaseURL("http://example.test"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true,"err":"","data":{"code":123}}`)),
			}, nil
		})}),
	)

	var out struct {
		Code string `json:"code"`
	}
	err := tr.Do(context.Background(), "hq/BasicQot", "/hq/BasicQot", nil, JSONCodec{}, &out)
	if err == nil {
		t.Fatal("Do returned nil for a data payload that cannot decode into out")
	}
	if !strings.Contains(err.Error(), "decode response data") {
		t.Fatalf("Error() = %q, want decode-response-data context", err.Error())
	}
}

func TestBodySnippet(t *testing.T) {
	if got := bodySnippet(nil); got != "" {
		t.Fatalf("bodySnippet(nil) = %q, want empty", got)
	}
	if got := bodySnippet([]byte("  ")); got != "" {
		t.Fatalf("bodySnippet(blank) = %q, want empty", got)
	}

	long := bytes.Repeat([]byte("x"), 300)
	got := bodySnippet(long)
	if !strings.HasPrefix(got, ": ") || !strings.HasSuffix(got, "...") {
		t.Fatalf("bodySnippet(long) = %q, want a truncated snippet", got)
	}
	if len(got) > 256+3+2 {
		t.Fatalf("bodySnippet(long) length = %d, want it truncated", len(got))
	}
}
