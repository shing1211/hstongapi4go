// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the failure side of account.go: the branch each of the five
// methods takes once its request path returns an error, plus the local validation
// that keeps a malformed call off the wire.
//
// It exists because that branch was the package's last hole. A2 measured the
// identical shape one layer down — twenty endpoints in pkg/hstong/trade, a fully
// covered happy path, and a `return ..., err` statement no test had ever reached,
// with the symptom that four mutations all sat at exactly 83.3%. Here all five
// methods carried the same unexercised statement.
//
// Four levels, all required, each reaching something the level above cannot.
//
//   - Level 1 is the sequencedExecutor returning a sentinel: the error is
//     propagated unchanged, the zero value comes back beside it, and exactly one
//     Do was recorded.
//   - Level 1b is the *partial* mid-walk failure, the one place in account.go
//     where an error and a non-zero result are returned together. A caller that
//     checks err alone is fine; a caller that treats a non-empty slice as success
//     is not, so the behaviour is pinned in both directions rather than left to be
//     discovered.
//   - Level 2 is a real *client.Client over a server answering a real ok:false
//     envelope, asserting the typed *errs.Error the transport actually builds. A
//     fake never builds a typed error, so a service that laundered a Gateway
//     rejection into an opaque one would pass every Level 1 row and fail here.
//   - The control is a retry policy with a zero backoff, showing the "exactly one
//     request" assertions are a decision rather than the absence of one.
//
// No row sleeps. The one place a retry policy appears installs MaxAttempts with
// BaseBackoff 0, so the request counts are a property of the retry classification
// rather than of elapsed time — the discipline internal/push needed after its
// reconnect tests made a coverage figure a coin flip.

// errAccountExecutorDown is the sentinel the Level 1 rows return. It is compared
// with errors.Is, never on a rendered message.
var errAccountExecutorDown = errors.New("account: scripted executor failure")

// accountErrorCase is one row of the Level 1 and Level 2 tables: the op and route
// the method must reach, and the call itself.
//
// Each row asserts the zero value beside the error inside its own invoke, against
// the concrete result type, which is the only way a generic zero check is
// meaningful here.
type accountErrorCase struct {
	name string
	op   string
	// route is the endpoint the method must reach.
	route client.Route
	// callsWhenCancelled is how many Do calls a cancelled context costs. It is
	// 1 for the three single-shot methods, which hand the context to the executor
	// and let the request path report the cancellation, and 0 for the two cursor
	// walks, because walkFundJourPages checks ctx.Err() at the top of its loop and
	// returns before spending a request. The asymmetry is a property of the code
	// and is pinned rather than averaged away: a walk that had already fetched a
	// page would not be free to stop.
	callsWhenCancelled int
	invoke             func(t *testing.T, ctx context.Context, svc *AccountService) error
}

// accountErrorCases is the per-method table, one row per method in account.go
// that has a rejection arm.
func accountErrorCases() []accountErrorCase {
	return []accountErrorCase{
		{
			name: "MarginFundInfo", op: opMarginFundInfo,
			route: client.RouteTradeQueryMarginFundInfo, callsWhenCancelled: 1,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService) error {
				t.Helper()
				bal, err := svc.MarginFundInfo(ctx, accountFixtureID(), MarginFundInfoRequest{})
				requireZero(t, bal)
				return err
			},
		},
		{
			name: "HoldsList", op: opHoldsList,
			route: client.RouteTradeQueryHoldsList, callsWhenCancelled: 1,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService) error {
				t.Helper()
				positions, err := svc.HoldsList(ctx, accountFixtureID(), HoldsFilter{})
				requireZero(t, positions)
				return err
			},
		},
		{
			name: "RealFundJourList", op: opRealFundJourList,
			route: client.RouteTradeQueryRealFundJourList, callsWhenCancelled: 0,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService) error {
				t.Helper()
				entries, err := svc.RealFundJourList(ctx, accountFixtureID(),
					FundJourFilter{}, domain.Pagination{})
				requireZero(t, entries)
				return err
			},
		},
		{
			name: "HistoryFundJourList", op: opHistoryFundJourList,
			route: client.RouteTradeQueryHistoryFundJourList, callsWhenCancelled: 0,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService) error {
				t.Helper()
				entries, err := svc.HistoryFundJourList(ctx, accountFixtureID(),
					FundJourFilter{}, domain.Pagination{})
				requireZero(t, entries)
				return err
			},
		},
		{
			name: "RateQueryList", op: opRateQueryList,
			route: client.RouteHsRateQueryList, callsWhenCancelled: 1,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService) error {
				t.Helper()
				rates, err := svc.RateQueryList(ctx)
				requireZero(t, rates)
				return err
			},
		},
	}
}

// accountValidatedCase is one row of the zero-accountID table. It is a separate
// type from accountErrorCase because these methods reject *before* the executor is
// consulted, so the assertion is that zero Do calls were recorded — the opposite
// of the table above.
type accountValidatedCase struct {
	name string
	op   string
	run  func(t *testing.T, ctx context.Context, svc *AccountService)
}

// accountValidatedCases are the four of the five methods that take an accountID
// and reject a zero one.
//
// RateQueryList is absent because it takes no accountID and has no validation arm
// at all. The plan's row for it says so explicitly, and a row here would pin a
// check the code does not have.
func accountValidatedCases() []accountValidatedCase {
	zeroID := domain.AccountID("")
	return []accountValidatedCase{
		{
			name: "MarginFundInfo", op: opMarginFundInfo,
			run: func(t *testing.T, ctx context.Context, svc *AccountService) {
				t.Helper()
				bal, err := svc.MarginFundInfo(ctx, zeroID, MarginFundInfoRequest{
					ExchangeType: types.ExchangeHK,
				})
				assertInvalidParam(t, err, opMarginFundInfo)
				requireZero(t, bal)
			},
		},
		{
			name: "HoldsList", op: opHoldsList,
			run: func(t *testing.T, ctx context.Context, svc *AccountService) {
				t.Helper()
				positions, err := svc.HoldsList(ctx, zeroID, HoldsFilter{
					ExchangeType: types.ExchangeHK,
				})
				assertInvalidParam(t, err, opHoldsList)
				requireZero(t, positions)
			},
		},
		{
			name: "RealFundJourList", op: opRealFundJourList,
			run: func(t *testing.T, ctx context.Context, svc *AccountService) {
				t.Helper()
				entries, err := svc.RealFundJourList(ctx, zeroID, FundJourFilter{
					ExchangeType: types.ExchangeHK, StartDate: "20260101", EndDate: "20260926",
				}, domain.Pagination{PageSize: 10, Cursor: "resume-here"})
				assertInvalidParam(t, err, opRealFundJourList)
				requireZero(t, entries)
			},
		},
		{
			name: "HistoryFundJourList", op: opHistoryFundJourList,
			run: func(t *testing.T, ctx context.Context, svc *AccountService) {
				t.Helper()
				entries, err := svc.HistoryFundJourList(ctx, zeroID, FundJourFilter{
					ExchangeType: types.ExchangeHK, StartDate: "20260101", EndDate: "20260926",
				}, domain.Pagination{PageSize: 10, Cursor: "resume-here"})
				assertInvalidParam(t, err, opHistoryFundJourList)
				requireZero(t, entries)
			},
		},
	}
}

// TestAccountMethodsRejectAZeroAccountIDWithoutARequest is the fail-closed row.
//
// The order matters and is the point: a zero accountID is refused locally, so the
// Gateway never sees a call it would have to reconcile. Zero recorded Do calls is
// the assertion that distinguishes a local rejection from a Gateway rejection
// wearing the same code, and the filter and pagination in each row are fully
// populated so a row cannot pass by reaching the executor and being rejected there.
func TestAccountMethodsRejectAZeroAccountIDWithoutARequest(t *testing.T) {
	for _, tc := range accountValidatedCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			tc.run(t, t.Context(), NewAccountService(exec))
			requireCalls(t, exec, 0)
		})
	}
}

// TestAccountMethodsAcceptANonZeroAccountID is the control for the table above.
//
// A validator that rejected every accountID — or one whose check was accidentally
// inverted — would satisfy TestAccountMethodsRejectAZeroAccountIDWithoutARequest
// while making the methods unusable. The ordinary value is shown to pass through
// the same entry point, with the recorded call as the evidence that validation
// really was skipped rather than merely outvoted.
func TestAccountMethodsAcceptANonZeroAccountID(t *testing.T) {
	for _, tc := range accountValidatedCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: accountReplyFor(t, tc.name)})
			svc := NewAccountService(exec)

			switch tc.name {
			case "MarginFundInfo":
				if _, err := svc.MarginFundInfo(t.Context(), accountFixtureID(),
					MarginFundInfoRequest{}); err != nil {
					t.Fatalf("MarginFundInfo with a non-zero accountID = %v, want nil", err)
				}
			case "HoldsList":
				if _, err := svc.HoldsList(t.Context(), accountFixtureID(),
					HoldsFilter{}); err != nil {
					t.Fatalf("HoldsList with a non-zero accountID = %v, want nil", err)
				}
			case "RealFundJourList":
				if _, err := svc.RealFundJourList(t.Context(), accountFixtureID(),
					FundJourFilter{}, domain.Pagination{}); err != nil {
					t.Fatalf("RealFundJourList with a non-zero accountID = %v, want nil", err)
				}
			case "HistoryFundJourList":
				if _, err := svc.HistoryFundJourList(t.Context(), accountFixtureID(),
					FundJourFilter{}, domain.Pagination{}); err != nil {
					t.Fatalf("HistoryFundJourList with a non-zero accountID = %v, want nil", err)
				}
			default:
				t.Fatalf("no positive-path row for %q; this table and "+
					"accountValidatedCases have drifted apart", tc.name)
			}
			accountExpectCall(t, exec, tc.op, accountRouteFor(t, tc.name))
			requireCalls(t, exec, 1)
		})
	}
}

// accountReplyFor is the reply each method needs so the positive control reaches
// a mapper without panicking. Every one of them populates every field, because
// four of the five delegate to a domain mapper that panics on an empty decimal
// (finding F2). It is a json.RawMessage rather than a string so the sequenced
// executor marshals it as a document; a []byte would be marshalled as a base64
// JSON string and every row would fail on a decode error.
func accountReplyFor(t *testing.T, method string) json.RawMessage {
	t.Helper()
	switch method {
	case "MarginFundInfo":
		return marginFundInfoBody()
	case "HoldsList":
		return holdsListBody()
	case "RealFundJourList", "HistoryFundJourList":
		return fundJourPageBody(accountFundJourRow("100.001", ""))
	case "RateQueryList":
		return json.RawMessage(accountRateFixtures)
	default:
		t.Fatalf("no reply fixture for %q", method)
		return nil
	}
}

// accountRouteFor is the endpoint each method must reach, keyed by the same names
// accountValidatedCases uses.
func accountRouteFor(t *testing.T, method string) client.Route {
	t.Helper()
	switch method {
	case "MarginFundInfo":
		return client.RouteTradeQueryMarginFundInfo
	case "HoldsList":
		return client.RouteTradeQueryHoldsList
	case "RealFundJourList":
		return client.RouteTradeQueryRealFundJourList
	case "HistoryFundJourList":
		return client.RouteTradeQueryHistoryFundJourList
	case "RateQueryList":
		return client.RouteHsRateQueryList
	default:
		t.Fatalf("no route for %q", method)
		return ""
	}
}

// TestAccountExecutorFailurePropagates is Level 1: the executor's error reaches
// the caller unchanged, the zero value comes back beside it, and exactly one Do
// was recorded.
func TestAccountExecutorFailurePropagates(t *testing.T) {
	for _, tc := range accountErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: errAccountExecutorDown})

			err := tc.invoke(t, t.Context(), NewAccountService(exec))
			if !errors.Is(err, errAccountExecutorDown) {
				t.Fatalf("%s = %v, want the executor's own error", tc.name, err)
			}
			accountExpectCall(t, exec, tc.op, tc.route)
			requireCalls(t, exec, 1)
		})
	}
}

// TestAccountGatewayRejectionArrivesTyped is Level 2: a real ok:false envelope
// becomes a typed *errs.Error carrying the Gateway's code, its category, and the
// service's own op.
//
// This is the row that proves the services do not launder a typed Gateway error
// into an opaque one — something a fake cannot show, because a fake never builds a
// typed error at all. A caller branching on Op to decide whether to re-authenticate
// is misinformed by a service that filled the wrong one in.
func TestAccountGatewayRejectionArrivesTyped(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range accountErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(code, text),
			})
			svc := NewAccountService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the Gateway rejection %q", tc.name, code)
			}
			errRejects(t, err, code, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryTrading {
				t.Errorf("CategoryOf = %q, want %q: a %s is a trading-state failure the caller "+
					"must reconcile before resubmitting", got, errs.CategoryTrading, code)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests to %s = %d, want exactly 1", tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want exactly 1", got)
			}
		})
	}
}

// TestAccountReadRetriesUnderARetryPolicy is the control for the single-request
// assertions above, and without it those assertions would be worth very little.
//
// Level 2 asserts a single HTTP request, and a single request is also what a
// client with no retry policy installed produces — so a client that silently
// ignored its policy would pass every row above. This installs MaxAttempts 5 with
// a zero backoff and a retryable rejection ("1011 service busy", which
// errs.Retryable reports as retryable) and shows all five account routes taking
// all five. The policy is therefore demonstrably live, and the one request in the
// table above is a decision rather than a default.
//
// Every route in this package is a read: none appears in resilience.mutationPaths,
// so ADR 0003 does not apply and all five are legitimately retryable.
func TestAccountReadRetriesUnderARetryPolicy(t *testing.T) {
	const wantAttempts = 5

	for _, tc := range accountErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(types.StatusServiceBusy, "service busy, retry later"),
			})
			svc := NewAccountService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: wantAttempts})))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			}
			errRejects(t, err, types.StatusServiceBusy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}
			if got := rec.total(); got != wantAttempts {
				t.Fatalf("total requests = %d, want %d: this control exists to show the retry "+
					"policy really is applied, or the single-request rows above prove nothing",
					got, wantAttempts)
			}
		})
	}
}

// TestAccountCallIsCancellable pins that a cancelled context is reported as a
// cancellation and never as a success. The error is matched through errors.Is on
// context.Canceled, never on a rendered message.
//
// The Do-call count differs between the rows, and the difference is the point.
// The three single-shot methods hand the context to the executor, so the request
// path is what reports the cancellation and exactly one call is recorded. The two
// cursor walks check ctx.Err() at the top of walkFundJourPages' loop and return
// before spending a request, so they cost nothing. A caller cancelling a long walk
// therefore saves a round trip per page, and that is a promise the count holds
// rather than an accident of the assertion.
func TestAccountCallIsCancellable(t *testing.T) {
	for _, tc := range accountErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: context.Canceled})
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := tc.invoke(t, ctx, NewAccountService(exec))
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s on a cancelled context = %v, want context.Canceled", tc.name, err)
			}
			if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
				t.Errorf("CategoryOf = %q, want %q for a cancelled call", got, errs.CategoryTimeout)
			}
			requireCalls(t, exec, tc.callsWhenCancelled)
		})
	}
}

// TestAccountMalformedReplyIsReportedAndNotDressedAsAGatewayCode covers the third
// way a request can fail: the Gateway says ok:true and then sends a data member the
// endpoint cannot decode.
//
// A method that ignored the decode failure would return an empty result and no
// error, and the caller would read that as a successful empty book. The zero value
// is asserted, and so is the absence of a status code: a local transport failure
// must not be dressed as a Gateway code, or a caller retrying on a code would chase
// a fault the Gateway never reported.
func TestAccountMalformedReplyIsReportedAndNotDressedAsAGatewayCode(t *testing.T) {
	for _, tc := range accountErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(`42`),
			})
			svc := NewAccountService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s on a data member of 42 = nil error; the value would be silently empty",
					tc.name)
			}
			if code, ok := errs.CodeOf(err); ok {
				t.Errorf("CodeOf = (%q, true), want (false): a decode failure is a local "+
					"transport error and must not carry a Gateway status code", code)
			}
			// The op still identifies which method failed, which is what a caller
			// branches on to decide whether to re-authenticate.
			var typed *errs.Error
			if !errors.As(err, &typed) {
				t.Errorf("errors.As(%T) yielded no *errs.Error, so the failure is not typed", err)
			} else if typed.Op != tc.op {
				t.Errorf("Op = %q, want %q", typed.Op, tc.op)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The mid-walk failure — an error and a non-zero result together
// ---------------------------------------------------------------------------

// TestFundJourMidWalkFailureReturnsTheCompletedRows is the Level 1b row and the
// one behaviour in account.go a caller cannot guess: a cursor walk that fails on
// its second page returns the rows of the first *and* the error.
//
// The design is deliberate and documented on walkFundJourPages — a partial outage
// should be visible without discarding completed pages — so the test pins it in
// both directions. Discarding the rows would lose completed work; dropping the
// error would turn a partial result into what looks like a complete one.
func TestFundJourMidWalkFailureReturnsTheCompletedRows(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t,
				sequencedReply{reply: fundJourPageBody(
					accountFundJourRow("100.001", "cursor-A"),
					accountFundJourRow("200.002", "cursor-B"),
				)},
				sequencedReply{err: errAccountExecutorDown},
			)

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 2})
			if !errors.Is(err, errAccountExecutorDown) {
				t.Fatalf("%s = %v, want the executor's own error from the second page", req.name, err)
			}
			accountExpectCall(t, exec, req.op, req.route)
			requireCalls(t, exec, 2)
			if len(entries) != 2 {
				t.Fatalf("entries = %d, want the 2 rows the first page completed: a partial "+
					"outage must not discard them", len(entries))
			}
			if got := entries[0].BusinessBalance.String(); got != "100.001" {
				t.Errorf("entries[0].BusinessBalance = %q, want 100.001", got)
			}
			// The completed page was real, so the walk must have sent its cursor.
			if got, want := accountJourCursors(t, exec), []string{"", "cursor-B"}; !reflect.DeepEqual(got, want) {
				t.Errorf("queryParamStr per request = %v, want %v", got, want)
			}
		})
	}
}

// TestFundJourFirstPageFailureReturnsNothing is the contrast row: when the *first*
// page fails there is nothing completed to return, so the zero value comes back.
//
// Together the two rows pin that the partial result is a property of the walk's
// progress and not of the error itself.
func TestFundJourFirstPageFailureReturnsNothing(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: errAccountExecutorDown})

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 2})
			if !errors.Is(err, errAccountExecutorDown) {
				t.Fatalf("%s = %v, want the executor's own error", req.name, err)
			}
			accountExpectCall(t, exec, req.op, req.route)
			requireZero(t, entries)
			requireCalls(t, exec, 1)
		})
	}
}

// TestFundJourMidWalkFailureOverTheWire is Level 2 for the same behaviour: the
// completed rows must survive a real HTTP client and a real typed Gateway
// rejection, not just a fake's sentinel.
//
// It needs a server whose answer changes between requests, which the shared
// wireRecorder cannot express — its table is keyed by path, and both pages of a
// cursor walk share one — so this file brings its own sequenced server. The two
// assertions that matter are that the second request really was made, so the walk
// really did reach a second page, and that the rows of the first came back.
func TestFundJourMidWalkFailureOverTheWire(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			srv, exec := newSequencedWireServer(t, []string{
				gatewaySuccess(string(fundJourPageBody(
					accountFundJourRow("100.001", "cursor-A"),
					accountFundJourRow("200.002", "cursor-B"),
				))),
				gatewayFailure(types.StatusServiceBusy, "service busy, retry later"),
			})

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 2})
			if err == nil {
				t.Fatalf("%s = nil error, want the second page's Gateway rejection", req.name)
			}
			errRejects(t, err, types.StatusServiceBusy, req.op)
			if got, ok := errs.CodeOf(err); !ok || got != types.StatusServiceBusy {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, types.StatusServiceBusy)
			}
			if got := srv.total(); got != 2 {
				t.Fatalf("requests = %d, want 2: the walk must reach a second page before failing", got)
			}
			if want := []string{string(req.route), string(req.route)}; !reflect.DeepEqual(srv.pathsSeen(), want) {
				t.Errorf("paths seen = %v, want two requests to %s", srv.pathsSeen(), req.route)
			}
			if len(entries) != 2 {
				t.Fatalf("entries = %d, want the 2 rows of the first page, which completed "+
					"successfully over a real connection", len(entries))
			}
			if got := entries[1].BusinessBalance.String(); got != "200.002" {
				t.Errorf("entries[1].BusinessBalance = %q, want 200.002", got)
			}
		})
	}
}

// TestFundJourMidWalkFailureRetriesOnlyTheFailingPage is the retry-policy row for
// a walk: the policy applies per fetch, so a failure on page two costs the retry
// budget there while page one's rows are still returned.
//
// It is asserted with a request count rather than a clock, and the walk's own
// behaviour is the interesting part: six requests (one for page one, five for page
// two) and still one completed row. A walk that re-fetched the whole thing, or
// discarded the first page when the second failed, would show a different count.
func TestFundJourMidWalkFailureRetriesOnlyTheFailingPage(t *testing.T) {
	const attempts = 5

	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")
			replies := make([]string, 0, 1+attempts)
			replies = append(replies, gatewaySuccess(
				string(fundJourPageBody(accountFundJourRow("100.001", "cursor-A")))))
			for i := 0; i < attempts; i++ {
				replies = append(replies, busy)
			}
			srv, exec := newSequencedWireServer(t, replies,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: attempts}))

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 1})
			if err == nil {
				t.Fatalf("%s = nil error, want the second page's rejection", req.name)
			}
			errRejects(t, err, types.StatusServiceBusy, req.op)
			if got, want := srv.total(), 1+attempts; got != want {
				t.Errorf("requests = %d, want %d: the completed page is not re-fetched and the "+
					"failing one takes the whole budget", got, want)
			}
			if len(entries) != 1 {
				t.Errorf("entries = %d, want the 1 row of the completed page", len(entries))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A server whose answer changes between requests
// ---------------------------------------------------------------------------

// sequencedWireServer answers the nth request from a fixed list of whole envelope
// bodies and counts what it saw. An exhausted list keeps answering with the last
// body, and the count assertion in each test is what notices.
//
// It is the one shape the shared wireRecorder cannot express. Every field is
// guarded, because a fixture that is not race-safe turns go test -race into a coin
// flip.
type sequencedWireServer struct {
	mu       sync.Mutex
	bodies   []string
	requests int
	paths    []string
}

// ServeHTTP records the request and answers with this request's body.
func (s *sequencedWireServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	idx := s.requests
	s.requests++
	s.paths = append(s.paths, req.URL.Path)
	body := s.bodies[len(s.bodies)-1]
	if idx < len(s.bodies) {
		body = s.bodies[idx]
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// total returns how many requests reached the server.
func (s *sequencedWireServer) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

// pathsSeen returns the request paths in arrival order.
func (s *sequencedWireServer) pathsSeen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

// newSequencedWireServer starts the server and returns it together with a real
// *client.Client over it as an Executor, with copts applied on top of the base
// URL. Server and client are both closed through t.Cleanup, so no test has to
// remember to.
//
// It never points at 127.0.0.1:11111: a services test has no business talking to a
// live Gateway.
func newSequencedWireServer(t *testing.T, bodies []string, copts ...client.Option) (*sequencedWireServer, Executor) {
	t.Helper()
	if len(bodies) == 0 {
		t.Fatal("test bug: the sequenced server was given no bodies, so it would panic on " +
			"the first request instead of answering it")
	}
	srv := &sequencedWireServer{bodies: bodies}

	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	all := make([]client.Option, 0, len(copts)+1)
	all = append(all, client.WithBaseURL(httpSrv.URL))
	all = append(all, copts...)

	c, err := client.New(all...)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return srv, c
}

// TestTheSequencedServerAnswersInOrder guards the fixture the two mid-walk rows
// above depend on.
//
// Without it, "the second body is the failure" would be an assumption: a server
// that replayed body one forever would let those rows pass while the second page
// was never really a failure. The rows also depend on an exhausted list
// re-answering with the last body, which the retry row's six requests would
// otherwise leave untested.
func TestTheSequencedServerAnswersInOrder(t *testing.T) {
	wide := `{"HKD":{"USD":"7.8","CNY":"0.9"},"CNY":{"HKD":"1.09"},"USD":{"HKD":"0.12"},"GBP":{}}`
	narrow := `{"HKD":{"USD":"7.8"}}`

	srv, exec := newSequencedWireServer(t, []string{gatewaySuccess(wide), gatewaySuccess(narrow)})
	svc := NewAccountService(exec)

	first, err := svc.RateQueryList(t.Context())
	if err != nil {
		t.Fatalf("RateQueryList, first call: %v", err)
	}
	second, err := svc.RateQueryList(t.Context())
	if err != nil {
		t.Fatalf("RateQueryList, second call: %v", err)
	}
	if len(first) != 4 {
		t.Errorf("the first call returned %d rates, want 4: the first body was not served", len(first))
	}
	if len(second) != 1 {
		t.Errorf("the second call returned %d rates, want 1: the second body was not served", len(second))
	}
	if got := srv.total(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// TestTheSequencedServerReplaysTheLastBodyWhenExhausted is the second half of the
// fixture contract.
//
// The retry row scripts one success and five failures and then asserts six
// requests, so the list runs out before the client stops asking. Without the
// replay rule the fixture would index out of range on request two and the failure
// would be a panic in a helper rather than a named assertion.
func TestTheSequencedServerReplaysTheLastBodyWhenExhausted(t *testing.T) {
	srv, exec := newSequencedWireServer(t, []string{
		gatewaySuccess(`{"HKD":{"USD":"7.8","CNY":"0.9"},"CNY":{"HKD":"1.09"},"USD":{"HKD":"0.12"},"GBP":{}}`),
	})
	svc := NewAccountService(exec)

	for i, want := range []int{4, 4, 4} {
		rates, err := svc.RateQueryList(t.Context())
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if len(rates) != want {
			t.Errorf("call %d returned %d rates, want %d: the last body must be replayed", i, len(rates), want)
		}
	}
	if got := srv.total(); got != 3 {
		t.Errorf("requests = %d, want 3", got)
	}
	if got, want := srv.pathsSeen(), []string{
		string(client.RouteHsRateQueryList),
		string(client.RouteHsRateQueryList),
		string(client.RouteHsRateQueryList),
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("paths seen = %v, want three requests to %s", got, client.RouteHsRateQueryList)
	}
}
