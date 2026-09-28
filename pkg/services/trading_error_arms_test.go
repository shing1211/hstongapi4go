// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file covers the failure side of the trading surface: the branch each of
// the fifteen methods takes once Executor.Do returns an error, and the argument
// rejections that stop a call before the executor is consulted at all.
//
// It exists because that branch was the package's second-largest hole. A2
// measured the identical shape one layer down: twenty endpoints in
// pkg/hstong/trade, a fully covered happy path, and a `return ..., err`
// statement no test had ever reached, with the symptom that four mutations all sat
// at exactly 83.3%. Here thirteen methods carried the same unexercised statement,
// and four of them are the ones ADR 0003 is about. The two push-subscription
// methods arrived later (C15) and are rows here on the same terms: their retry
// class is asserted in trading_push_test.go, because theirs is the one route pair
// whose own GoDoc describes the opposite, but which op label, route and error
// shape each of them produces is this file's subject exactly as it is for the
// other thirteen.
//
// Two levels, both required. Level 1 drives a sequencedExecutor returning a
// sentinel: it proves the error reaches the caller unchanged, that the zero value
// is returned beside it, and that exactly one Do was recorded. Level 2 drives a
// real *client.Client over a wireRecorder answering with a real ok:false envelope
// and asserts the typed *errs.Error the transport actually builds — which is
// something a fake can never show, because a fake never builds a typed error. A
// service that laundered a Gateway rejection into an opaque one would pass every
// Level 1 row and fail here.
//
// No row sleeps. The one place a retry policy appears is the control at the end
// of the ADR 0003 file, which installs BaseBackoff 0 so the request counts are a
// property of the retry classification rather than of elapsed time.

var errTradingExecutorDown = errors.New("trading: scripted executor failure")

// tradingErrorCase is one row of the table: the op and route the method must
// reach, and the call itself. Each row asserts the zero value beside the error
// inside its own invoke, against the concrete return type, which is the only way
// a generic zero check means anything here.
type tradingErrorCase struct {
	name   string
	op     string
	route  client.Route
	invoke func(t *testing.T, ctx context.Context, svc *TradingService) error
}

// tradingErrorCases is the per-method table, one row per method in trading.go,
// with the zero-value assertion each method's signature makes necessary:
//
//   - the six that return a pointer return nil;
//   - the five that return a slice return nil, not an empty slice, so a caller
//     cannot tell a failed call from one that legitimately matched nothing;
//   - the three error-only mutations return the error itself.
func tradingErrorCases() []tradingErrorCase {
	account := tradingAccountID()
	entrustID := tradingEntrustID()
	order := tradingHKOrder()
	price := domain.MustNewPrice("387.05", "0.001")
	qty := domain.MustNewQuantity("100")

	return []tradingErrorCase{
		{
			name:  "Entrust",
			op:    opEntrust,
			route: client.RouteTradeEntrust,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				res, err := svc.Entrust(ctx, account, order)
				requireZero(t, res)
				return err
			},
		},
		{
			name:  "CancelEntrust",
			op:    opCancelEntrust,
			route: client.RouteTradeCancelEntrust,
			invoke: func(_ *testing.T, ctx context.Context, svc *TradingService) error {
				return svc.CancelEntrust(ctx, account, entrustID)
			},
		},
		{
			name:  "BatchCancelEntrust",
			op:    opBatchCancelEntrust,
			route: client.RouteTradeBatchCancelEntrust,
			invoke: func(_ *testing.T, ctx context.Context, svc *TradingService) error {
				return svc.BatchCancelEntrust(ctx, account, []domain.EntrustID{entrustID})
			},
		},
		{
			name:  "ChangeEntrust",
			op:    opChangeEntrust,
			route: client.RouteTradeChangeEntrust,
			invoke: func(_ *testing.T, ctx context.Context, svc *TradingService) error {
				return svc.ChangeEntrust(ctx, account, entrustID, price, qty)
			},
		},
		{
			name:  "MaxAvailableAsset",
			op:    opMaxAvailableAsset,
			route: client.RouteTradeQueryMaxAvailableAsset,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.MaxAvailableAsset(ctx, account, tradingHKSymbol(), price, types.EntrustTypeLimit)
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "RealEntrustList",
			op:    opRealEntrustList,
			route: client.RouteTradeQueryRealEntrustList,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.RealEntrustList(ctx, account, EntrustFilter{ExchangeType: types.ExchangeHK})
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "RealDeliverList",
			op:    opRealDeliverList,
			route: client.RouteTradeQueryRealDeliverList,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.RealDeliverList(ctx, account, DeliverFilter{ExchangeType: types.ExchangeHK})
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "RealCondOrderList",
			op:    opRealCondOrderList,
			route: client.RouteTradeQueryRealCondOrderList,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.RealCondOrderList(ctx, account, CondOrderFilter{ExchangeType: types.ExchangeHK})
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "HistoryEntrustList",
			op:    opHistoryEntrustList,
			route: client.RouteTradeQueryHistoryEntrustList,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.HistoryEntrustList(ctx, account, HistoryFilter{ExchangeType: types.ExchangeHK})
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "HistoryDeliverList",
			op:    opHistoryDeliverList,
			route: client.RouteTradeQueryHistoryDeliverList,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.HistoryDeliverList(ctx, account, HistoryFilter{ExchangeType: types.ExchangeHK})
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "HistoryCondOrderList",
			op:    opHistoryCondOrderList,
			route: client.RouteTradeQueryHistoryCondOrderList,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.HistoryCondOrderList(ctx, account, HistoryFilter{ExchangeType: types.ExchangeHK})
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "MarginFullInfo",
			op:    opMarginFullInfo,
			route: client.RouteTradeQueryMarginFullInfo,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.MarginFullInfo(ctx, account)
				requireZero(t, got)
				return err
			},
		},
		{
			name:  "BeforeAndAfterSupport",
			op:    opBeforeAndAfterSupport,
			route: client.RouteTradeQueryBeforeAndAfterSupport,
			invoke: func(t *testing.T, ctx context.Context, svc *TradingService) error {
				got, err := svc.BeforeAndAfterSupport(ctx, tradingHKSymbol())
				requireZero(t, got)
				return err
			},
		},
		// The two push-subscription rows carry no zero value to assert: both methods
		// return only an error, which is what makes "the reply is discarded" and
		// "the request is the whole result" expressible as a signature.
		{
			name:  "SubscribeOrders",
			op:    opTradeSubscribe,
			route: client.RouteTradeSubscribe,
			invoke: func(_ *testing.T, ctx context.Context, svc *TradingService) error {
				return svc.SubscribeOrders(ctx, account)
			},
		},
		{
			name:  "UnsubscribeOrders",
			op:    opTradeUnsubscribe,
			route: client.RouteTradeUnsubscribe,
			invoke: func(_ *testing.T, ctx context.Context, svc *TradingService) error {
				return svc.UnsubscribeOrders(ctx, account)
			},
		},
	}
}

// TestTradingExecutorFailurePropagates is Level 1: the executor's error reaches
// the caller unchanged, beside the zero value, after exactly one attempt.
//
// The zero-value assertion lives inside each invoke and is checked against the
// concrete return type. A method that returned a half-decoded payload beside its
// error would let a caller mistake a failure for an empty success, which is the
// failure mode the row exists to prevent — and the five slice-returning methods
// are the ones where that mistake is easiest to make, since an empty slice is a
// perfectly ordinary result.
func TestTradingExecutorFailurePropagates(t *testing.T) {
	for _, tc := range tradingErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: errTradingExecutorDown})
			svc := NewTradingService(exec)

			if err := tc.invoke(t, t.Context(), svc); !errors.Is(err, errTradingExecutorDown) {
				t.Fatalf("%s error = %v, want the executor's own error", tc.name, err)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestTradingGatewayRejectionArrivesTyped is Level 2: the same thirteen rows
// driven through a real client over a real HTTP server, with the Gateway
// answering "1007 : duplicate submission".
//
// The assertions are the ones a fake cannot make. internal/transport parses the
// documented "<code> : <text>" form back into an *errs.Error carrying the code
// and its category, and the service must hand that value to the caller with its
// own op attached. A wrapper that replaced it with a bare error, or that lost the
// op, would pass Level 1 and fail here.
//
// 1007 is chosen because it is a trading-state failure: a duplicate submission is
// not a malformed request, and the code a caller gets back has to say so, because
// retrying a duplicate is exactly the mistake the ADR 0003 reconciliation
// requirement exists to prevent.
func TestTradingGatewayRejectionArrivesTyped(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range tradingErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(code, text),
			})
			svc := NewTradingService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the Gateway rejection %q", tc.name, code)
			}
			errRejects(t, err, code, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryTrading {
				t.Errorf("CategoryOf = %q, want %q: a 1007 is a trading-state failure the caller "+
					"must reconcile rather than resubmit", got, errs.CategoryTrading)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}
			// A typed Gateway error is not retryable, so a client with no policy
			// installed still issues exactly one request — and the four mutations
			// issue one regardless. The count is asserted so a future change that
			// started swallowing the failure and reporting a zero value would be
			// visible here too.
			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests to %s = %d, want exactly 1", tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want exactly 1", got)
			}
		})
	}
}

// TestTradingGatewayRejectionIsNotLaundered is the specific laundering failure
// Level 2 is built to catch, stated on its own so a future change that starts
// wrapping the Gateway error in a local one fails with a readable message.
//
// A local rejection is StatusInvalidParam under CategoryAPI; a Gateway rejection
// is whatever the Gateway said. If a service ever replaced the transport's error
// with its own, a caller could no longer tell "I built this request wrong" from
// "the exchange rejected it", and the reconciliation ADR 0003 requires would be
// skipped. The two categories are compared side by side here.
func TestTradingGatewayRejectionIsNotLaundered(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeEntrust): gatewayFailure(types.StatusDuplicateSubmit, "duplicate submission"),
	})
	svc := NewTradingService(newWireExecutor(t, rec))

	_, err := svc.Entrust(t.Context(), tradingAccountID(), tradingHKOrder())
	errRejects(t, err, types.StatusDuplicateSubmit, opEntrust)
	if got := errs.CategoryOf(err); got == errs.CategoryAPI {
		t.Error("CategoryOf = api for a Gateway rejection: the service replaced the transport's " +
			"error with a local one, so a caller cannot tell a bad request from a rejected one")
	}

	// And the local rejection, for the comparison. Same method, same op, different
	// category and a different code: the typed pair is what makes them separable.
	// The order is an HK one with a bad lot, because a zero-valued order would pass
	// validateOrderForHK on its very first line — an empty market is not Hong Kong —
	// and would be sent to the very Gateway that just refused it.
	badLot := tradingHKOrder()
	badLot.Quantity = domain.MustNewQuantity("150")
	_, local := svc.Entrust(t.Context(), tradingAccountID(), badLot)
	assertInvalidParam(t, local, opEntrust)
	if errs.CategoryOf(local) == errs.CategoryOf(err) {
		t.Error("the local rejection and the Gateway rejection share a category, so nothing " +
			"distinguishes them for the caller")
	}
	if got := rec.total(); got != 1 {
		t.Errorf("total requests = %d, want 1: the local rejection must not have reached the "+
			"Gateway", got)
	}
}

// TestTradingCallIsCancellable pins that a cancelled context reaches the executor
// untouched and that a service neither swallows the cancellation nor turns it into
// a success. The error is matched through errors.Is on context.Canceled, never on
// a rendered message, and the zero value is asserted beside it.
func TestTradingCallIsCancellable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		invoke func(t *testing.T, ctx context.Context, svc *TradingService) error
	}{
		{"Entrust", func(t *testing.T, ctx context.Context, svc *TradingService) error {
			res, err := svc.Entrust(ctx, tradingAccountID(), tradingHKOrder())
			requireZero(t, res)
			return err
		}},
		{"MaxAvailableAsset", func(t *testing.T, ctx context.Context, svc *TradingService) error {
			got, err := svc.MaxAvailableAsset(ctx, tradingAccountID(), tradingHKSymbol(),
				domain.MustNewPrice("387.05", "0.001"), types.EntrustTypeLimit)
			requireZero(t, got)
			return err
		}},
		{"RealEntrustList", func(t *testing.T, ctx context.Context, svc *TradingService) error {
			got, err := svc.RealEntrustList(ctx, tradingAccountID(), EntrustFilter{ExchangeType: types.ExchangeHK})
			requireZero(t, got)
			return err
		}},
		{"MarginFullInfo", func(t *testing.T, ctx context.Context, svc *TradingService) error {
			got, err := svc.MarginFullInfo(ctx, tradingAccountID())
			requireZero(t, got)
			return err
		}},
		{"CancelEntrust", func(_ *testing.T, ctx context.Context, svc *TradingService) error {
			return svc.CancelEntrust(ctx, tradingAccountID(), tradingEntrustID())
		}},
		{"BatchCancelEntrust", func(_ *testing.T, ctx context.Context, svc *TradingService) error {
			return svc.BatchCancelEntrust(ctx, tradingAccountID(), []domain.EntrustID{tradingEntrustID()})
		}},
		{"ChangeEntrust", func(_ *testing.T, ctx context.Context, svc *TradingService) error {
			return svc.ChangeEntrust(ctx, tradingAccountID(), tradingEntrustID(),
				domain.MustNewPrice("387.05", "0.001"), domain.MustNewQuantity("100"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: context.Canceled})
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := tc.invoke(t, ctx, NewTradingService(exec))
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s on a cancelled context = %v, want context.Canceled", tc.name, err)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestTradingDoesNotRetryOnItsOwn is the half of the ADR 0003 story that belongs
// in this file rather than in the wire-level proof: a service must not grow a
// retry loop of its own, which is a different failure from a misclassified route.
//
// A fake executor has no retry logic, so this assertion is trivially true of the
// code as it stands — and that is the point being made. The substantive proof that
// the guarantee holds is in trading_adr0003_test.go, against a real client. This
// test is the guard against a future service adding a loop around its Do call: a
// retryable typed error is offered to the method, and the method is still recorded
// as having made exactly one call.
func TestTradingDoesNotRetryOnItsOwn(t *testing.T) {
	retryable := errs.New(types.StatusServiceBusy, opRealEntrustList, "service busy, retry later")
	if !errs.Retryable(retryable) {
		t.Fatal("test bug: the scripted error is not retryable, so this test would pass for the " +
			"wrong reason")
	}

	for _, tc := range tradingErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Two replies are scripted and only one is consumed: a second attempt
			// would consume the second and the fixture would still look healthy, so
			// the count is the assertion that matters here.
			exec := newSequencedExecutor(t,
				sequencedReply{err: retryable},
				sequencedReply{err: retryable},
			)
			svc := NewTradingService(exec)

			err := tc.invoke(t, t.Context(), svc)
			if !errors.Is(err, retryable) {
				t.Fatalf("%s error = %v, want the scripted retryable error", tc.name, err)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestTradingReportsAMalformedGatewayReply is the other half of "the error arm
// works": a reply the codec cannot decode is a typed error rather than a
// half-populated result.
//
// The transport hands the caller a value it could not fully decode only if the
// codec let it through. This row pins that it does not, for the one method whose
// out type is a single string, so a future change that swapped in a lenient
// decoder would be caught on the endpoint where it would first be noticed.
func TestTradingReportsAMalformedGatewayReply(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeQueryBeforeAndAfterSupport): `{"ok":true,"err":"","data":{"data":42}}`,
	})
	svc := NewTradingService(newWireExecutor(t, rec))

	got, err := svc.BeforeAndAfterSupport(t.Context(), tradingHKSymbol())
	if err == nil {
		t.Fatalf("BeforeAndAfterSupport on a reply whose data is a number = %+v and no error; "+
			"the value would be silently empty", got)
	}
	requireZero(t, got)
	if code, ok := errs.CodeOf(err); ok {
		t.Errorf("CodeOf = (%q, true), want (false): a decode failure is a local transport "+
			"error and must not be dressed as a Gateway status code", code)
	}
}

// TestTradingGatewayFailureWithNoCodeIsStillTyped covers the envelope form the
// Gateway uses when it has a message but no status code, because "err" is a
// free-text field and the "<code> : <text>" shape is a convention rather than a
// guarantee.
func TestTradingGatewayFailureWithNoCodeIsStillTyped(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeQueryMarginFullInfo): `{"ok":false,"err":"session expired"}`,
	})
	svc := NewTradingService(newWireExecutor(t, rec))

	got, err := svc.MarginFullInfo(t.Context(), tradingAccountID())
	if err == nil {
		t.Fatal("MarginFullInfo on a codeless ok:false envelope = nil error")
	}
	requireZero(t, got)
	if _, ok := errs.CodeOf(err); !ok {
		t.Log("CodeOf reports no code for this envelope: the text carried none, which is " +
			"acceptable as long as the failure is still a typed *errs.Error with the op attached")
	}
	assertInvalidParamCheck(t, err, opMarginFullInfo)
}

// assertInvalidParamCheck exists so the codeless-envelope row above can assert
// the op without asserting a code: the two properties are independent and a
// codeless rejection has only one of them.
func assertInvalidParamCheck(t *testing.T, err error, wantOp string) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(%T) yielded no *errs.Error", err)
	}
	if e.Op != wantOp {
		t.Errorf("Op = %q, want %q", e.Op, wantOp)
	}
}

// TestCancelAndBatchCancelDoNotDecodeTheirReply is a small but real observation
// about the three error-only mutations: none of them looks at what came back.
//
// A batch cancel reply names which cancels succeeded and which failed, and both
// methods discard it and return nil. A caller that cancels ten entrusts and is
// told "done" has no way to learn that four were refused. The reply is decoded
// into domain.CancelResultWire and then dropped, which is a behaviour change to
// make and therefore is recorded rather than fixed.
func TestCancelAndBatchCancelDoNotDecodeTheirReply(t *testing.T) {
	failing := json.RawMessage(`{"successEntrustId":["E-1"],` +
		`"failCancelEntrust":[{"failEntrustId":"E-2","remark":"already filled"}]}`)

	exec := newSequencedExecutor(t, sequencedReply{reply: failing})
	if err := NewTradingService(exec).BatchCancelEntrust(t.Context(), tradingAccountID(),
		[]domain.EntrustID{"E-1", "E-2"}); err != nil {
		t.Errorf("BatchCancelEntrust with one refused id = %v, want nil: the method returns "+
			"only the transport error and drops the per-id outcome", err)
	}
	requireCalls(t, exec, 1)
}
