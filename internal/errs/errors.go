// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package errs

import (
	"context"
	"errors"
	"strings"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Category is a coarse classification of an Error, used by the resilience
// layer to decide whether a failure is worth retrying and by callers to branch
// on the kind of failure without inspecting the raw status code. It is a
// string so it round-trips cleanly through logs and metrics.
type Category string

const (
	// CategorySuccess is the category of the success code ("0000"). A success
	// code is never wrapped as an error in practice; the category exists so the
	// status-code table is total.
	CategorySuccess Category = "success"
	// CategoryConnection marks transport-level failures: dial errors, a
	// socket that was never initialized, a failed long-connection setup, or a
	// reconnecting Gateway. Connection errors are generally retryable.
	CategoryConnection Category = "connection"
	// CategoryTimeout marks calls that exceeded their deadline, either at the
	// Gateway ("1015") or locally. Timeouts are generally retryable.
	CategoryTimeout Category = "timeout"
	// CategoryAPI marks protocol-, request-, or server-side failures that a
	// caller cannot fix by retrying: bad signatures, illegal or invalid
	// requests, unknown endpoints, deprecated endpoints, and failed queries.
	CategoryAPI Category = "api"
	// CategoryAccount marks session and authorization failures ("1006",
	// "1012", "1013", "1014", "20033") that require re-login or authorization.
	CategoryAccount Category = "account"
	// CategoryTrading marks trading-state failures such as a duplicate
	// submission ("1007"), where the caller must reconcile before resubmitting.
	CategoryTrading Category = "trading"
	// CategoryRateLimit marks transient load shedding ("1011") where the
	// Gateway asks the caller to retry later.
	CategoryRateLimit Category = "rate_limit"
	// CategoryUnknown is the category of an unclassified failure, including any
	// status code the SDK does not document.
	CategoryUnknown Category = "unknown"
)

// String returns the category's wire/log form. The zero Category returns the
// empty string.
func (c Category) String() string {
	return string(c)
}

// codeInfo pairs a status code's category with its human-readable message.
type codeInfo struct {
	category Category
	message  string
}

// codeTable is the single mapping from every documented Gateway status code
// (docs/SPEC.md §6) to its Category and human-readable message. It is the
// authoritative table for CategoryForCode and MessageForCode.
var codeTable = map[types.StatusCode]codeInfo{
	types.StatusOK:                      {CategorySuccess, "success"},
	types.StatusUnknownError:            {CategoryAPI, "unknown system error"},
	types.StatusSignatureError:          {CategoryAPI, "signature error"},
	types.StatusEncryptionError:         {CategoryAPI, "data encryption error"},
	types.StatusSocketNotInitialized:    {CategoryConnection, "socket not initialized"},
	types.StatusEndpointDeprecated:      {CategoryAPI, "endpoint deprecated"},
	types.StatusUserNotAuthorized:       {CategoryAccount, "user not authorized"},
	types.StatusDuplicateSubmit:         {CategoryTrading, "duplicate submission"},
	types.StatusCallFailed:              {CategoryAPI, "call failed"},
	types.StatusEndpointNotFound:        {CategoryAPI, "endpoint not found"},
	types.StatusIllegalRequest:          {CategoryAPI, "illegal request"},
	types.StatusServiceBusy:             {CategoryRateLimit, "service busy, retry later"},
	types.StatusNotLoggedIn:             {CategoryAccount, "not logged in"},
	types.StatusKickedOffline:           {CategoryAccount, "session displaced by another login"},
	types.StatusLoginTimeout:            {CategoryAccount, "login timeout"},
	types.StatusCallTimeout:             {CategoryTimeout, "call timeout"},
	types.StatusInvalidParam:            {CategoryAPI, "invalid parameter"},
	types.StatusConnectFailed:           {CategoryConnection, "long-connection establishment failed"},
	types.StatusReconnecting:            {CategoryConnection, "reconnecting, retry later"},
	types.StatusFuturesLoginTimeout:     {CategoryAccount, "futures trade login timeout"},
	types.StatusQueryProductInfoFailed:  {CategoryAPI, "query product info failed"},
	types.StatusQueryContractInfoFailed: {CategoryAPI, "query contract info failed"},
}

// Error is the typed error returned by the SDK for every Gateway or
// transport-level failure. It never contains credentials or request bodies:
// only the operation, status code, category, message, and wrapped cause are
// retained, so it is safe to log and to expose to callers.
//
// Error values are produced by New, Wrap, Connection, and Timeout and are
// traversable with errors.Is and errors.As.
type Error struct {
	// Code is the Gateway status code, or the empty StatusCode for
	// transport-level failures that never reached the Gateway. It may also be
	// empty when the Gateway reported a failure without a recognizable code.
	Code types.StatusCode
	// Category is the coarse classification of Code (or of the underlying
	// transport failure). It is never empty for values created by this package.
	Category Category
	// Message is a human-readable description of the failure. It never includes
	// secrets.
	Message string
	// Op identifies the endpoint or operation that failed, for example
	// "trade/TradeEntrust". It is included in Error() to give context.
	Op string
	// Err is the wrapped cause, if any. Unwrap exposes it to errors.Is and
	// errors.As.
	Err error
}

// Error implements the error interface. It renders the operation, status code,
// message, and (when distinct) the wrapped cause. The output contains no
// request payloads or credentials.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	if e.Op != "" {
		b.WriteString(e.Op)
		b.WriteString(": ")
	}
	if e.Code != "" && e.Code != types.StatusOK {
		b.WriteString(string(e.Code))
		b.WriteString(" ")
	}
	msg := e.Message
	if msg == "" {
		switch {
		case e.Code != "":
			msg = MessageForCode(e.Code)
		case e.Err != nil:
			msg = e.Err.Error()
		default:
			msg = "error"
		}
	}
	b.WriteString(msg)
	if e.Err != nil {
		cause := e.Err.Error()
		if cause != "" && cause != msg && !strings.Contains(msg, cause) {
			b.WriteString(": ")
			b.WriteString(cause)
		}
	}
	return b.String()
}

// Unwrap returns the wrapped cause, enabling errors.Is and errors.As to
// traverse the chain. It returns nil when there is no cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// New returns an *Error for a Gateway status code. When message is empty it is
// filled from the documented message table via MessageForCode. The category is
// always derived from code (CategoryUnknown for codes the SDK does not
// document).
func New(code types.StatusCode, op, message string) *Error {
	if message == "" {
		message = MessageForCode(code)
	}
	return &Error{
		Code:     code,
		Category: CategoryForCode(code),
		Message:  message,
		Op:       op,
	}
}

// Wrap returns an *Error that wraps err as its cause, preserving errors.Is and
// errors.As traversal. code, op, and message have the same semantics as New;
// err may be nil. When code is empty the category is CategoryUnknown.
func Wrap(err error, code types.StatusCode, op, message string) *Error {
	e := New(code, op, message)
	e.Err = err
	return e
}

// Connection returns an *Error classified as CategoryConnection for a
// transport-level failure that never reached the Gateway or could not be
// delivered, such as a dial failure or a malformed response. The cause is
// wrapped and exposed via Unwrap.
func Connection(op string, err error) *Error {
	return &Error{
		Category: CategoryConnection,
		Message:  "connection failed",
		Op:       op,
		Err:      err,
	}
}

// Timeout returns an *Error classified as CategoryTimeout for a call that
// exceeded its deadline before a response was obtained. The cause is wrapped
// and exposed via Unwrap. Timeouts are retryable for query endpoints only;
// mutations must never be retried (ADR 0003).
func Timeout(op string, err error) *Error {
	return &Error{
		Category: CategoryTimeout,
		Message:  "request timed out",
		Op:       op,
		Err:      err,
	}
}

// KnownCode reports whether code is one of the documented Gateway status codes
// in the mapping table.
func KnownCode(code types.StatusCode) bool {
	_, ok := codeTable[code]
	return ok
}

// CategoryForCode returns the Category for a documented status code, or
// CategoryUnknown for the empty code and for any undocumented code.
func CategoryForCode(code types.StatusCode) Category {
	if info, ok := codeTable[code]; ok {
		return info.category
	}
	return CategoryUnknown
}

// MessageForCode returns the documented human-readable message for code, or the
// raw code when it is not documented.
func MessageForCode(code types.StatusCode) string {
	if info, ok := codeTable[code]; ok {
		return info.message
	}
	return string(code)
}

// CodeOf extracts the Gateway status code from err, traversing wrapped errors
// with errors.As. The boolean is false when err is nil, does not wrap an
// *Error, or carries no code (transport-level failures).
func CodeOf(err error) (types.StatusCode, bool) {
	if err == nil {
		return "", false
	}
	var e *Error
	if errors.As(err, &e) && e != nil && e.Code != "" {
		return e.Code, true
	}
	return "", false
}

// CategoryOf returns the Category of err, traversing wrapped errors with
// errors.As. A bare context.DeadlineExceeded or context.Canceled is reported as
// CategoryTimeout; any other untagged error is CategoryUnknown.
func CategoryOf(err error) Category {
	if err == nil {
		return CategoryUnknown
	}
	var e *Error
	if errors.As(err, &e) && e != nil {
		if e.Category != "" {
			return e.Category
		}
		return CategoryForCode(e.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return CategoryTimeout
	}
	return CategoryUnknown
}

// Retryable reports whether err describes a condition that the resilience layer
// may retry for read-only query endpoints. It is true for the documented
// transient codes ("1011" service busy, "1015" call timeout, "1017" failed
// long-connection setup, "1018" reconnecting), for CategoryConnection and
// CategoryTimeout failures, and for context.DeadlineExceeded and
// context.Canceled. It is false for every other condition, in particular every
// trading rejection and all account/session failures. This helper never
// authorizes retrying an order mutation: mutation endpoints are excluded from
// retry unconditionally (ADR 0003).
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var e *Error
	if errors.As(err, &e) && e != nil {
		if e.Category == CategoryConnection || e.Category == CategoryTimeout {
			return true
		}
		switch e.Code {
		case types.StatusServiceBusy,
			types.StatusCallTimeout,
			types.StatusConnectFailed,
			types.StatusReconnecting:
			return true
		}
	}
	return false
}

// ReLoginRequired reports whether err indicates that the session is no longer
// valid and a single-flight re-login is required: "1012" (not logged in),
// "1013" (session displaced), "1014" (login timeout), and "20033" (futures
// trade login timeout).
func ReLoginRequired(err error) bool {
	code, ok := CodeOf(err)
	if !ok {
		return false
	}
	switch code {
	case types.StatusNotLoggedIn,
		types.StatusKickedOffline,
		types.StatusLoginTimeout,
		types.StatusFuturesLoginTimeout:
		return true
	default:
		return false
	}
}

// IsDeprecated reports whether err is the "1005" endpoint-deprecated status.
func IsDeprecated(err error) bool {
	code, ok := CodeOf(err)
	return ok && code == types.StatusEndpointDeprecated
}
