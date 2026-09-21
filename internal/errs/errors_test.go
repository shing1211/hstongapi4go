// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package errs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

func TestCategoryString(t *testing.T) {
	tests := []struct {
		category Category
		want     string
	}{
		{CategorySuccess, "success"},
		{CategoryConnection, "connection"},
		{CategoryTimeout, "timeout"},
		{CategoryAPI, "api"},
		{CategoryAccount, "account"},
		{CategoryTrading, "trading"},
		{CategoryRateLimit, "rate_limit"},
		{CategoryUnknown, "unknown"},
		{Category(""), ""},
	}
	for _, tt := range tests {
		if got := tt.category.String(); got != tt.want {
			t.Errorf("Category(%q).String() = %q, want %q", string(tt.category), got, tt.want)
		}
	}
}

func TestCodeTableIsTotal(t *testing.T) {
	tests := []struct {
		code     types.StatusCode
		category Category
		message  string
	}{
		{types.StatusOK, CategorySuccess, "success"},
		{types.StatusUnknownError, CategoryAPI, "unknown system error"},
		{types.StatusSignatureError, CategoryAPI, "signature error"},
		{types.StatusEncryptionError, CategoryAPI, "data encryption error"},
		{types.StatusSocketNotInitialized, CategoryConnection, "socket not initialized"},
		{types.StatusEndpointDeprecated, CategoryAPI, "endpoint deprecated"},
		{types.StatusUserNotAuthorized, CategoryAccount, "user not authorized"},
		{types.StatusDuplicateSubmit, CategoryTrading, "duplicate submission"},
		{types.StatusCallFailed, CategoryAPI, "call failed"},
		{types.StatusEndpointNotFound, CategoryAPI, "endpoint not found"},
		{types.StatusIllegalRequest, CategoryAPI, "illegal request"},
		{types.StatusServiceBusy, CategoryRateLimit, "service busy, retry later"},
		{types.StatusNotLoggedIn, CategoryAccount, "not logged in"},
		{types.StatusKickedOffline, CategoryAccount, "session displaced by another login"},
		{types.StatusLoginTimeout, CategoryAccount, "login timeout"},
		{types.StatusCallTimeout, CategoryTimeout, "call timeout"},
		{types.StatusInvalidParam, CategoryAPI, "invalid parameter"},
		{types.StatusConnectFailed, CategoryConnection, "long-connection establishment failed"},
		{types.StatusReconnecting, CategoryConnection, "reconnecting, retry later"},
		{types.StatusFuturesLoginTimeout, CategoryAccount, "futures trade login timeout"},
		{types.StatusQueryProductInfoFailed, CategoryAPI, "query product info failed"},
		{types.StatusQueryContractInfoFailed, CategoryAPI, "query contract info failed"},
	}

	if len(tests) != len(codeTable) {
		t.Fatalf("table covers %d codes, want %d", len(tests), len(codeTable))
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := CategoryForCode(tt.code); got != tt.category {
				t.Errorf("CategoryForCode(%q) = %q, want %q", tt.code, got, tt.category)
			}
			if got := MessageForCode(tt.code); got != tt.message {
				t.Errorf("MessageForCode(%q) = %q, want %q", tt.code, got, tt.message)
			}
			if !KnownCode(tt.code) {
				t.Errorf("KnownCode(%q) = false, want true", tt.code)
			}
		})
	}
}

func TestCategoryForUnknownCode(t *testing.T) {
	if got := CategoryForCode(types.StatusCode("9999")); got != CategoryUnknown {
		t.Fatalf("CategoryForCode(9999) = %q, want %q", got, CategoryUnknown)
	}
	if got := MessageForCode(types.StatusCode("9999")); got != "9999" {
		t.Fatalf("MessageForCode(9999) = %q, want raw code", got)
	}
	if KnownCode(types.StatusCode("9999")) {
		t.Fatal("KnownCode(9999) = true, want false")
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"service busy 1011", New(types.StatusServiceBusy, "op", ""), true},
		{"call timeout 1015", New(types.StatusCallTimeout, "op", ""), true},
		{"connect failed 1017", New(types.StatusConnectFailed, "op", ""), true},
		{"reconnecting 1018", New(types.StatusReconnecting, "op", ""), true},
		{"not logged in 1012", New(types.StatusNotLoggedIn, "op", ""), false},
		{"kicked offline 1013", New(types.StatusKickedOffline, "op", ""), false},
		{"login timeout 1014", New(types.StatusLoginTimeout, "op", ""), false},
		{"futures login timeout 20033", New(types.StatusFuturesLoginTimeout, "op", ""), false},
		{"duplicate submit 1007", New(types.StatusDuplicateSubmit, "op", ""), false},
		{"deprecated 1005", New(types.StatusEndpointDeprecated, "op", ""), false},
		{"invalid param 1016", New(types.StatusInvalidParam, "op", ""), false},
		{"query product failed 40001", New(types.StatusQueryProductInfoFailed, "op", ""), false},
		{"connection category", Connection("op", errors.New("dial")), true},
		{"timeout category", Timeout("op", context.DeadlineExceeded), true},
		{"wrapped connection", fmt.Errorf("outer: %w", Connection("op", errors.New("dial"))), true},
		{"wrapped trading rejection", fmt.Errorf("outer: %w", New(types.StatusDuplicateSubmit, "op", "")), false},
		{"bare deadline exceeded", context.DeadlineExceeded, true},
		{"bare canceled", context.Canceled, true},
		{"wrapped deadline exceeded", fmt.Errorf("do: %w", context.DeadlineExceeded), true},
		{"plain error", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Retryable(tt.err); got != tt.want {
				t.Errorf("Retryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestReLoginRequired(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not logged in 1012", New(types.StatusNotLoggedIn, "op", ""), true},
		{"kicked offline 1013", New(types.StatusKickedOffline, "op", ""), true},
		{"login timeout 1014", New(types.StatusLoginTimeout, "op", ""), true},
		{"futures login timeout 20033", New(types.StatusFuturesLoginTimeout, "op", ""), true},
		{"wrapped 1012", fmt.Errorf("outer: %w", New(types.StatusNotLoggedIn, "op", "")), true},
		{"service busy 1011", New(types.StatusServiceBusy, "op", ""), false},
		{"duplicate submit 1007", New(types.StatusDuplicateSubmit, "op", ""), false},
		{"plain error", errors.New("boom"), false},
		{"connection", Connection("op", errors.New("dial")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReLoginRequired(tt.err); got != tt.want {
				t.Errorf("ReLoginRequired(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsDeprecated(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{New(types.StatusEndpointDeprecated, "op", ""), true},
		{fmt.Errorf("outer: %w", New(types.StatusEndpointDeprecated, "op", "")), true},
		{New(types.StatusCallFailed, "op", ""), false},
	}
	for _, tt := range tests {
		if got := IsDeprecated(tt.err); got != tt.want {
			t.Errorf("IsDeprecated(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestCodeOfAndCategoryOf(t *testing.T) {
	if code, ok := CodeOf(nil); ok || code != "" {
		t.Fatalf("CodeOf(nil) = (%q, %v), want (\"\", false)", code, ok)
	}
	if code, ok := CodeOf(Connection("op", errors.New("dial"))); ok || code != "" {
		t.Fatalf("CodeOf(connection) = (%q, %v), want (\"\", false)", code, ok)
	}

	base := New(types.StatusCallTimeout, "hq/BasicQot", "")
	wrapped := fmt.Errorf("outer: %w", base)

	code, ok := CodeOf(wrapped)
	if !ok || code != types.StatusCallTimeout {
		t.Fatalf("CodeOf(wrapped) = (%q, %v), want (%q, true)", code, ok, types.StatusCallTimeout)
	}
	if got := CategoryOf(wrapped); got != CategoryTimeout {
		t.Fatalf("CategoryOf(wrapped) = %q, want %q", got, CategoryTimeout)
	}
	if got := CategoryOf(nil); got != CategoryUnknown {
		t.Fatalf("CategoryOf(nil) = %q, want %q", got, CategoryUnknown)
	}
	if got := CategoryOf(errors.New("plain")); got != CategoryUnknown {
		t.Fatalf("CategoryOf(plain) = %q, want %q", got, CategoryUnknown)
	}
	if got := CategoryOf(context.Canceled); got != CategoryTimeout {
		t.Fatalf("CategoryOf(context.Canceled) = %q, want %q", got, CategoryTimeout)
	}
}

func TestErrorsIsAndAsThroughWraps(t *testing.T) {
	cause := errors.New("dial tcp 127.0.0.1:11111: connect: connection refused")
	base := Connection("hq/BasicQot", cause)
	wrapped := fmt.Errorf("transport: %w", base)

	if !errors.Is(wrapped, cause) {
		t.Fatal("errors.Is through *Error.Unwrap failed for the cause")
	}

	var typed *Error
	if !errors.As(wrapped, &typed) {
		t.Fatal("errors.As failed to find *Error")
	}
	if typed != base {
		t.Fatal("errors.As found the wrong *Error")
	}
	if typed.Category != CategoryConnection {
		t.Fatalf("category = %q, want %q", typed.Category, CategoryConnection)
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("cannot traverse from the wrapper to the cause")
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "code and message",
			err:  New(types.StatusNotLoggedIn, "trade/TradeEntrust", ""),
			want: "trade/TradeEntrust: 1012 not logged in",
		},
		{
			name: "transport failure with cause",
			err:  Connection("hq/BasicQot", errors.New("dial refused")),
			want: "hq/BasicQot: connection failed: dial refused",
		},
		{
			name: "timeout with same cause",
			err:  Timeout("hq/KL", context.DeadlineExceeded),
			want: "hq/KL: request timed out: context deadline exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}

	var nilErr *Error
	if got := nilErr.Error(); got != "<nil>" {
		t.Fatalf("nil *Error.Error() = %q, want \"<nil>\"", got)
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("nil *Error.Unwrap() != nil")
	}
}

func TestNewFillsMessageFromTable(t *testing.T) {
	e := New(types.StatusServiceBusy, "hq/Ticker", "")
	if e.Message != "service busy, retry later" {
		t.Fatalf("Message = %q, want table message", e.Message)
	}
	if e.Category != CategoryRateLimit {
		t.Fatalf("Category = %q, want %q", e.Category, CategoryRateLimit)
	}
	if e.Op != "hq/Ticker" {
		t.Fatalf("Op = %q, want hq/Ticker", e.Op)
	}
}

func TestErrorNeverLeaksCauseTextTwice(t *testing.T) {
	base := New(types.StatusCallFailed, "op", "call failed")
	got := base.Error()
	if strings.Count(got, "call failed") != 1 {
		t.Fatalf("Error() = %q, message duplicated", got)
	}
}
