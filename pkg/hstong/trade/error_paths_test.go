// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests in this file cover the failure side of the trade surface: the branch
// every endpoint takes once Manager.call returns an error, the two session paths
// inside call, and the ADR 0003 single-attempt guarantee.
//
// They exist because that branch was the package's largest hole. Fourteen of the
// twenty endpoints carried a `return ..., err` statement that no test had ever
// reached, which is why the four mutations all sat at exactly 83.3% and the
// cursor list queries at exactly 80.0%: the identical percentages were one
// untested branch, not two.
//
// Every case is driven by a canned ok:false envelope from httptest or by a
// scripted Session. Nothing here sleeps, polls, or waits on a deadline: the
// retry policy installed in the ADR 0003 test has a zero backoff, so the number
// of HTTP requests it produces is a property of the retry classification rather
// than of how long a run took. The same commit therefore measures the same
// coverage on every run, which is the discipline internal/push adopted after its
// reconnect tests made the gate a coin flip.

// gatewayFailure renders an ok:false envelope in the "1012 : not logged in" form
// the Gateway sends: a documented status code, a separator, and free text.
func gatewayFailure(code types.StatusCode, text string) string {
	return fmt.Sprintf(`{"ok":false,"err":%q}`, string(code)+" : "+text)
}

// requireZero fails when v is not the zero value of T. Every method here must
// hand back the zero value beside an error, so a caller cannot mistake a
// partially decoded payload for a successful response.
func requireZero[T any](t *testing.T, v T) {
	t.Helper()
	var zero T
	if !reflect.DeepEqual(v, zero) {
		t.Fatalf("value returned alongside the error = %+v, want the zero value", v)
	}
}

// errRejects asserts that err is the typed *errs.Error for wantCode under wantOp
// and returns it. It matches only through errors.As and the typed fields, never
// on the rendered message, so a reworded message cannot change the outcome.
func errRejects(t *testing.T, err error, wantCode types.StatusCode, wantOp string) *errs.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want the Gateway rejection %q", wantCode)
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(%T) yielded no *errs.Error", err)
	}
	if e.Code != wantCode {
		t.Errorf("Code = %q, want %q", e.Code, wantCode)
	}
	if e.Op != wantOp {
		t.Errorf("Op = %q, want %q", e.Op, wantOp)
	}
	return e
}

// newInstrumentedManager starts a recorder-backed test server, builds a client
// with copts applied on top of the recorder's base URL, and returns the recorder
// plus a Manager over that client with opts applied. newManager covers the
// default configuration; this variant exists for the cases that have to install
// a retry policy or a Session to reach the branch under test.
func newInstrumentedManager(t *testing.T, responses map[string]string, copts []client.Option, opts ...Option) (*recorder, *Manager) {
	t.Helper()
	rec := &recorder{responses: responses}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	all := make([]client.Option, 0, len(copts)+1)
	all = append(all, client.WithBaseURL(srv.URL))
	all = append(all, copts...)

	c, err := client.New(all...)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return rec, New(c, opts...)
}

// TestManager_CallFailurePropagates asserts that every endpoint surfaces the
// Gateway's typed rejection unchanged and returns the zero value beside it.
//
// The rejection is "1007 duplicate submission", a CategoryTrading failure that
// is not retryable, so the assertion is not merely counting attempts: a Manager
// that silently retried, or wrapped the error into something unmatchable, would
// fail here.
func TestManager_CallFailurePropagates(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range []struct {
		name   string
		op     string
		route  client.Route
		invoke func(*testing.T, *Manager) error
	}{
		{
			name:  "Entrust",
			op:    opEntrust,
			route: client.RouteTradeEntrust,
			invoke: func(t *testing.T, m *Manager) error {
				id, err := m.Entrust(context.Background(), validEntrust())
				requireZero(t, id)
				return err
			},
		},
		{
			name:  "CancelEntrust",
			op:    opCancelEntrust,
			route: client.RouteTradeCancelEntrust,
			invoke: func(t *testing.T, m *Manager) error {
				id, err := m.CancelEntrust(context.Background(), validCancel())
				requireZero(t, id)
				return err
			},
		},
		{
			name:  "BatchCancelEntrust",
			op:    opBatchCancelEntrust,
			route: client.RouteTradeBatchCancelEntrust,
			invoke: func(t *testing.T, m *Manager) error {
				out, err := m.BatchCancelEntrust(context.Background(), BatchCancelEntrustRequest{
					ExchangeType: types.ExchangeHK,
					EntrustIDs:   []string{"ENT-1"},
				})
				requireZero(t, out)
				return err
			},
		},
		{
			name:  "ChangeEntrust",
			op:    opChangeEntrust,
			route: client.RouteTradeChangeEntrust,
			invoke: func(t *testing.T, m *Manager) error {
				id, err := m.ChangeEntrust(context.Background(), validChange())
				requireZero(t, id)
				return err
			},
		},
		{
			name:  "MaxAvailableAsset",
			op:    opMaxAvailableAsset,
			route: client.RouteTradeQueryMaxAvailableAsset,
			invoke: func(t *testing.T, m *Manager) error {
				out, err := m.MaxAvailableAsset(context.Background(), validMaxAvailable())
				requireZero(t, out)
				return err
			},
		},
		{
			name:  "RealEntrustList",
			op:    opRealEntrustList,
			route: client.RouteTradeQueryRealEntrustList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.RealEntrustList(context.Background(), RealEntrustListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "RealDeliverList",
			op:    opRealDeliverList,
			route: client.RouteTradeQueryRealDeliverList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.RealDeliverList(context.Background(), RealDeliverListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "RealCondOrderList",
			op:    opRealCondOrderList,
			route: client.RouteTradeQueryRealCondOrderList,
			invoke: func(t *testing.T, m *Manager) error {
				page, err := m.RealCondOrderList(context.Background(), CondOrderListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, page)
				return err
			},
		},
		{
			name:  "HistoryEntrustList",
			op:    opHistoryEntrustList,
			route: client.RouteTradeQueryHistoryEntrustList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.HistoryEntrustList(context.Background(), HistoryEntrustListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "HistoryDeliverList",
			op:    opHistoryDeliverList,
			route: client.RouteTradeQueryHistoryDeliverList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.HistoryDeliverList(context.Background(), HistoryDeliverListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "HistoryCondOrderList",
			op:    opHistoryCondOrderList,
			route: client.RouteTradeQueryHistoryCondOrderList,
			invoke: func(t *testing.T, m *Manager) error {
				page, err := m.HistoryCondOrderList(context.Background(), HistoryCondOrderListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, page)
				return err
			},
		},
		{
			name:  "MarginFullInfo",
			op:    opMarginFullInfo,
			route: client.RouteTradeQueryMarginFullInfo,
			invoke: func(t *testing.T, m *Manager) error {
				out, err := m.MarginFullInfo(context.Background(), MarginFullInfoRequest{DataType: "10000", StockCode: "01810.HK"})
				requireZero(t, out)
				return err
			},
		},
		{
			name:  "BeforeAndAfterSupport",
			op:    opBeforeAndAfterSupport,
			route: client.RouteTradeQueryBeforeAndAfterSupport,
			invoke: func(t *testing.T, m *Manager) error {
				flag, err := m.BeforeAndAfterSupport(context.Background(), BeforeAndAfterSupportRequest{
					StockCode:    "01810.HK",
					ExchangeType: types.ExchangeHK,
				})
				requireZero(t, flag)
				return err
			},
		},
		{
			name:  "MarginFundInfo",
			op:    opMarginFundInfo,
			route: client.RouteTradeQueryMarginFundInfo,
			invoke: func(t *testing.T, m *Manager) error {
				out, err := m.MarginFundInfo(context.Background(), MarginFundInfoRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, out)
				return err
			},
		},
		{
			name:  "Positions",
			op:    opHoldsList,
			route: client.RouteTradeQueryHoldsList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.Positions(context.Background(), PositionsRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "RealFundJourList",
			op:    opRealFundJourList,
			route: client.RouteTradeQueryRealFundJourList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.RealFundJourList(context.Background(), FundJourListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "HistoryFundJourList",
			op:    opHistoryFundJourList,
			route: client.RouteTradeQueryHistoryFundJourList,
			invoke: func(t *testing.T, m *Manager) error {
				rows, err := m.HistoryFundJourList(context.Background(), HistoryFundJourListRequest{ExchangeType: types.ExchangeHK})
				requireZero(t, rows)
				return err
			},
		},
		{
			name:  "ExchangeRate",
			op:    opRateQueryList,
			route: client.RouteHsRateQueryList,
			invoke: func(t *testing.T, m *Manager) error {
				rates, err := m.ExchangeRate(context.Background(), ExchangeRateRequest{})
				requireZero(t, rates)
				return err
			},
		},
		{
			name:  "SubscribeOrders",
			op:    opTradeSubscribe,
			route: client.RouteTradeSubscribe,
			invoke: func(_ *testing.T, m *Manager) error {
				return m.SubscribeOrders(context.Background())
			},
		},
		{
			name:  "UnsubscribeOrders",
			op:    opTradeUnsubscribe,
			route: client.RouteTradeUnsubscribe,
			invoke: func(_ *testing.T, m *Manager) error {
				return m.UnsubscribeOrders(context.Background())
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newInstrumentedManager(t, map[string]string{
				string(tc.route): gatewayFailure(code, text),
			}, nil)

			if err := tc.invoke(t, m); err == nil {
				t.Fatalf("%s = nil error, want the Gateway rejection %q", tc.name, code)
			} else {
				errRejects(t, err, code, tc.op)
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

// scriptedSession is a Session whose outcomes the test dictates and whose
// consultations it counts. Every field is mutex-guarded so the fake is safe
// under -race. It never sleeps, so no assertion depends on wall-clock timing.
type scriptedSession struct {
	mu            sync.Mutex
	ensureErr     error
	ensureCalls   int
	reloginCalls  int
	reloginCauses []error
	reloginErr    error
	handled       bool
}

// EnsureLoggedIn records the consultation and returns the scripted error.
func (s *scriptedSession) EnsureLoggedIn(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCalls++
	return s.ensureErr
}

// ReLogin records the cause it was handed and returns the scripted outcome.
func (s *scriptedSession) ReLogin(_ context.Context, cause error) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloginCalls++
	s.reloginCauses = append(s.reloginCauses, cause)
	return s.handled, s.reloginErr
}

// counts returns the consultation counters under the lock.
func (s *scriptedSession) counts() (ensure, relogin int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureCalls, s.reloginCalls
}

// causes returns a copy of the recorded ReLogin causes under the lock.
func (s *scriptedSession) causes() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]error(nil), s.reloginCauses...)
}

// errNotLive stands in for a login that cannot be established. It is a distinct
// value rather than a Gateway status because it never reaches the Gateway.
var errNotLive = errors.New("scripted: session cannot be established")

// TestManager_CallStopsWhenSessionIsNotLive asserts that a Manager with a
// Session does not send the request when the session cannot be established.
//
// Sending it anyway would place an order on a connection the SDK already knows
// is not authenticated, and the caller's only evidence would be a Gateway
// rejection attributed to the endpoint rather than to the login.
func TestManager_CallStopsWhenSessionIsNotLive(t *testing.T) {
	sess := &scriptedSession{ensureErr: errNotLive}
	rec, m := newInstrumentedManager(t, nil, nil, WithSession(sess))

	id, err := m.Entrust(context.Background(), validEntrust())
	requireZero(t, id)

	if !errors.Is(err, errNotLive) {
		t.Fatalf("Entrust error = %v, want the login failure %v", err, errNotLive)
	}
	if got := rec.total(); got != 0 {
		t.Fatalf("requests = %d, want 0 (the order must not be sent before login)", got)
	}
	ensure, relogin := sess.counts()
	if ensure != 1 {
		t.Errorf("EnsureLoggedIn calls = %d, want 1", ensure)
	}
	if relogin != 0 {
		t.Errorf("ReLogin calls = %d, want 0 (a login that never succeeded cannot be refreshed)", relogin)
	}
}

// TestManager_CallAsksSessionToReLoginWithoutRepeatingTheRequest asserts the
// best-effort re-login in Manager.call: on a "1012 not logged in" rejection the
// session is asked to re-login once, the caller still sees the original endpoint
// error, and — the part that matters for ADR 0003 — the endpoint request itself
// is not issued a second time.
func TestManager_CallAsksSessionToReLoginWithoutRepeatingTheRequest(t *testing.T) {
	const path = string(client.RouteTradeEntrust)

	sess := &scriptedSession{handled: true, reloginErr: errors.New("scripted: re-login also failed")}
	rec, m := newInstrumentedManager(t, map[string]string{
		path: gatewayFailure(types.StatusNotLoggedIn, "not logged in"),
	}, nil, WithSession(sess))

	id, err := m.Entrust(context.Background(), validEntrust())
	requireZero(t, id)
	errRejects(t, err, types.StatusNotLoggedIn, opEntrust)

	ensure, relogin := sess.counts()
	if ensure != 1 {
		t.Errorf("EnsureLoggedIn calls = %d, want 1", ensure)
	}
	if relogin != 1 {
		t.Errorf("ReLogin calls = %d, want 1", relogin)
	}

	// The session must be handed the same failure the caller is, so it can tell a
	// displaced session from a rejected order.
	causes := sess.causes()
	if len(causes) != 1 {
		t.Fatalf("ReLogin causes = %d, want 1", len(causes))
	}
	errRejects(t, causes[0], types.StatusNotLoggedIn, opEntrust)

	// The re-login outcome is deliberately not surfaced: the endpoint error is the
	// one the caller must reconcile, and the re-login ran best-effort.
	if got := rec.count(path); got != 1 {
		t.Fatalf("requests to %s = %d, want exactly 1: re-logging in must not resubmit the order (ADR 0003)", path, got)
	}
}

// TestManager_MutationRoutesIssueExactlyOneAttemptUnderARetryPolicy is the ADR
// 0003 proof for this package.
//
// A retry policy of five attempts is installed and the Gateway rejects every
// request with "1011 service busy", a failure errs.Retryable reports as
// retryable. A read-only route therefore has every reason to send five requests.
// A trade mutation must still send exactly one: resubmitting an order that may
// already have reached the Gateway is the failure mode ADR 0003 exists to
// prevent. The policy's backoff is zero, so the request counts are decided by
// the retry classification and not by how long the test took to run.
func TestManager_MutationRoutesIssueExactlyOneAttemptUnderARetryPolicy(t *testing.T) {
	const wantQueryAttempts = 5
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")
	retrying := []client.Option{
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: wantQueryAttempts}),
	}

	for _, tc := range []struct {
		name   string
		op     string
		route  client.Route
		invoke func(*Manager) error
	}{
		{
			name:  "Entrust",
			op:    opEntrust,
			route: client.RouteTradeEntrust,
			invoke: func(m *Manager) error {
				_, err := m.Entrust(context.Background(), validEntrust())
				return err
			},
		},
		{
			name:  "CancelEntrust",
			op:    opCancelEntrust,
			route: client.RouteTradeCancelEntrust,
			invoke: func(m *Manager) error {
				_, err := m.CancelEntrust(context.Background(), validCancel())
				return err
			},
		},
		{
			name:  "BatchCancelEntrust",
			op:    opBatchCancelEntrust,
			route: client.RouteTradeBatchCancelEntrust,
			invoke: func(m *Manager) error {
				_, err := m.BatchCancelEntrust(context.Background(), BatchCancelEntrustRequest{
					ExchangeType: types.ExchangeHK,
					EntrustIDs:   []string{"ENT-1"},
				})
				return err
			},
		},
		{
			name:  "ChangeEntrust",
			op:    opChangeEntrust,
			route: client.RouteTradeChangeEntrust,
			invoke: func(m *Manager) error {
				_, err := m.ChangeEntrust(context.Background(), validChange())
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newInstrumentedManager(t, map[string]string{
				string(tc.route): busy,
			}, retrying)

			if err := tc.invoke(m); err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			} else {
				errRejects(t, err, types.StatusServiceBusy, tc.op)
			}

			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests to %s = %d, want exactly 1: a rejected order must never be resent (ADR 0003)",
					tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want exactly 1", got)
			}
		})
	}

	// Control. Without it the assertions above would pass just as well against a
	// policy that was never applied, which would make them worthless: the point
	// is that the policy does retry a query and declines to retry a mutation.
	t.Run("read-only route retries under the same policy", func(t *testing.T) {
		rec, m := newInstrumentedManager(t, map[string]string{
			string(client.RouteTradeQueryRealEntrustList): busy,
		}, retrying)

		rows, err := m.RealEntrustList(context.Background(), RealEntrustListRequest{ExchangeType: types.ExchangeHK})
		if err == nil {
			t.Fatal("RealEntrustList = nil error, want the rejection")
		}
		requireZero(t, rows)
		errRejects(t, err, types.StatusServiceBusy, opRealEntrustList)

		if got := rec.total(); got != wantQueryAttempts {
			t.Fatalf("total requests = %d, want %d: the control must show the policy really does retry a query",
				got, wantQueryAttempts)
		}
	})
}
