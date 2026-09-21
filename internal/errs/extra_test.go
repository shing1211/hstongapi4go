// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package errs

import (
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestWrap pins the wrapping contract: Wrap preserves the cause for errors.Is /
// errors.As, fills the message from the code table, and classifies the code.
func TestWrap(t *testing.T) {
	cause := errors.New("dial refused")
	e := Wrap(cause, types.StatusCallFailed, "hq/BasicQot", "")

	if e.Category != CategoryAPI {
		t.Errorf("Category = %q, want %q", e.Category, CategoryAPI)
	}
	if e.Message != "call failed" {
		t.Errorf("Message = %q, want table message", e.Message)
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is through Wrap failed for the cause")
	}
	if !errors.Is(e, cause) || e.Unwrap() != cause {
		t.Error("Unwrap did not expose the cause")
	}

	noCode := Wrap(cause, "", "op", "")
	if noCode.Category != CategoryUnknown {
		t.Errorf("empty-code Category = %q, want %q", noCode.Category, CategoryUnknown)
	}
	if noCode.Err != cause {
		t.Error("empty-code Wrap dropped the cause")
	}

	nilCause := Wrap(nil, types.StatusServiceBusy, "op", "")
	if nilCause.Err != nil {
		t.Errorf("Wrap(nil) Err = %v, want nil", nilCause.Err)
	}
}

// TestErrorFallbackMessage exercises the branches of Error() that synthesize a
// message when Message is empty.
func TestErrorFallbackMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "code fills message",
			err:  &Error{Code: types.StatusServiceBusy, Op: "op"},
			want: "op: 1011 service busy, retry later",
		},
		{
			name: "cause fills message",
			err:  &Error{Err: errors.New("boom")},
			want: "boom",
		},
		{
			name: "generic fallback",
			err:  &Error{},
			want: "error",
		},
		{
			name: "cause already contained in message",
			err:  &Error{Message: "boom happened", Err: errors.New("boom")},
			want: "boom happened",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCategoryOfFillsFromCode covers an *Error that carries a code but no
// explicit category, as a hand-built value might.
func TestCategoryOfFillsFromCode(t *testing.T) {
	e := &Error{Code: types.StatusServiceBusy}
	if got := CategoryOf(e); got != CategoryRateLimit {
		t.Fatalf("CategoryOf(code-only) = %q, want %q", got, CategoryRateLimit)
	}
}
