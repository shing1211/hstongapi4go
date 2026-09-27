// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/hstong/algo"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// newRetryingManager is newManager with a retry policy whose backoff is zero, so
// a retry shows up as an extra recorded request instead of as elapsed time. No
// test in this package sleeps.
func newRetryingManager(t *testing.T, responses map[string]string, maxAttempts int, opts ...algo.Option) (*algo.Manager, *recorder) {
	t.Helper()
	rec := &recorder{responses: responses}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	c, err := client.New(
		client.WithBaseURL(srv.URL),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0}),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return algo.New(c, opts...), rec
}

// TestQueriesPropagateGatewayFailures asserts a read-only query surfaces a
// Gateway rejection as a typed error and returns no data. The mutations already
// had a failing-Gateway test; the two queries did not, so the entire error arm
// of both read-only methods was dark and a broken propagation path (a dropped
// error, or an empty slice returned alongside a failure) would have gone
// unnoticed.
func TestQueriesPropagateGatewayFailures(t *testing.T) {
	const busy = `{"ok":false,"err":"1011 service busy"}`

	t.Run("QueryOrderList", func(t *testing.T) {
		m, _ := newRetryingManager(t, map[string]string{pathQueryOrderList: busy}, 1)

		got, err := m.QueryOrderList(context.Background(), validQueryOrderList())
		if err == nil {
			t.Fatal("QueryOrderList = nil error, want a typed Gateway error")
		}
		if got != nil {
			t.Fatalf("orders = %#v, want nil alongside the error", got)
		}
		if code, ok := errs.CodeOf(err); !ok || code != types.StatusServiceBusy {
			t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusServiceBusy)
		}
		if !errs.Retryable(err) {
			t.Fatal("errs.Retryable(err) = false for a 1011 rejection, want true")
		}
		// A Gateway rejection must not masquerade as a local one.
		if errors.Is(err, algo.ErrInvalidParams) {
			t.Fatal("errors.Is(err, ErrInvalidParams) = true for a Gateway rejection")
		}
	})

	t.Run("QueryEntrustIDList", func(t *testing.T) {
		m, _ := newRetryingManager(t, map[string]string{pathQueryEntrustIDList: busy}, 1)

		got, err := m.QueryEntrustIDList(context.Background(), validQueryEntrustIDList())
		if err == nil {
			t.Fatal("QueryEntrustIDList = nil error, want a typed Gateway error")
		}
		if got != nil {
			t.Fatalf("entrust IDs = %#v, want nil alongside the error", got)
		}
		if code, ok := errs.CodeOf(err); !ok || code != types.StatusServiceBusy {
			t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusServiceBusy)
		}
		if errors.Is(err, algo.ErrInvalidParams) {
			t.Fatal("errors.Is(err, ErrInvalidParams) = true for a Gateway rejection")
		}
	})
}

// TestQueriesRetryWhileMutationsStaySingleAttempt is the ADR 0003 proof for this
// package: one client, one retry policy, the same retryable Gateway code. The
// two read-only queries are retried up to the budget; the five mutations issue
// exactly one request each. client/route_mutation_test.go holds the exhaustive
// route allowlist that classifies the algo mutation paths, so this test does not
// re-derive the classification, it exercises its effect.
func TestQueriesRetryWhileMutationsStaySingleAttempt(t *testing.T) {
	const busy = `{"ok":false,"err":"1011 service busy"}`
	const wantAttempts = 3

	t.Run("query is retried to the budget", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			path string
			call func(context.Context, *algo.Manager) error
		}{
			{"QueryOrderList", pathQueryOrderList, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.QueryOrderList(ctx, validQueryOrderList())
				return err
			}},
			{"QueryEntrustIDList", pathQueryEntrustIDList, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.QueryEntrustIDList(ctx, validQueryEntrustIDList())
				return err
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m, rec := newRetryingManager(t, map[string]string{tc.path: busy}, wantAttempts)

				if err := tc.call(context.Background(), m); err == nil {
					t.Fatal("call = nil error, want the Gateway rejection")
				}
				if got := rec.count(tc.path); got != wantAttempts {
					t.Fatalf("%s requests = %d, want %d: a read-only query is retryable", tc.path, got, wantAttempts)
				}
			})
		}
	})

	t.Run("mutation is issued once", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			path string
			call func(context.Context, *algo.Manager) error
		}{
			{"AddOrder", pathAddOrder, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.AddOrder(ctx, validAddOrder())
				return err
			}},
			{"CancelOrder", pathCancelOrder, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.CancelOrder(ctx, validCancelOrder())
				return err
			}},
			{"CancelEntrust", pathCancelEntrust, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.CancelEntrust(ctx, validCancelEntrust())
				return err
			}},
			{"ChangeOrder", pathChangeOrder, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.ChangeOrder(ctx, validChangeOrder())
				return err
			}},
			{"ActionOrder", pathActionOrder, func(ctx context.Context, m *algo.Manager) error {
				_, err := m.ActionOrder(ctx, validActionOrder())
				return err
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m, rec := newRetryingManager(t, map[string]string{tc.path: busy}, wantAttempts)

				if err := tc.call(context.Background(), m); err == nil {
					t.Fatal("call = nil error, want the Gateway rejection")
				}
				if got := rec.count(tc.path); got != 1 {
					t.Fatalf("%s requests = %d, want exactly 1 (ADR 0003)", tc.path, got)
				}
				if got := rec.total(); got != 1 {
					t.Fatalf("total requests = %d, want exactly 1", got)
				}
			})
		}
	})
}

// TestLocalRejectionSendsNothing covers the fail-closed contract end to end for
// every required field the existing suite did not drive through a Manager: a
// malformed argument produces no request at all, so there is no dangerous
// request to reject at the Gateway. Each row is a single-field mutation of an
// otherwise valid request, so a green row means that field's check is live.
func TestLocalRejectionSendsNothing(t *testing.T) {
	rows := []struct {
		name string
		call func(context.Context, *algo.Manager) error
	}{
		{"add/exchangeType", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.ExchangeType = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/entrustType", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.EntrustType = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/entrustPrice", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.EntrustPrice = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/entrustAmount", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.EntrustAmount = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/entrustBs", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.EntrustBS = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/targetStrategy", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.TargetStrategy = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/sessionType", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.SessionType = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/sessionType out of set", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.SessionType = "2"
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/maxVolume", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.StrategyParam.MaxVolume = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/maxVolume out of range", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.StrategyParam.MaxVolume = "0"
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/sensitivity", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.StrategyParam.Sensitivity = ""
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"add/sensitivity out of set", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.StrategyParam.Sensitivity = "7"
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"cancel/exchangeType", func(ctx context.Context, m *algo.Manager) error {
			p := validCancelOrder()
			p.ExchangeType = ""
			_, err := m.CancelOrder(ctx, p)
			return err
		}},
		{"cancelEntrust/orderId", func(ctx context.Context, m *algo.Manager) error {
			p := validCancelEntrust()
			p.OrderID = ""
			_, err := m.CancelEntrust(ctx, p)
			return err
		}},
		{"cancelEntrust/exchangeType", func(ctx context.Context, m *algo.Manager) error {
			p := validCancelEntrust()
			p.ExchangeType = ""
			_, err := m.CancelEntrust(ctx, p)
			return err
		}},
		{"change/entrustAmount", func(ctx context.Context, m *algo.Manager) error {
			p := validChangeOrder()
			p.EntrustAmount = ""
			_, err := m.ChangeOrder(ctx, p)
			return err
		}},
		{"change/entrustAmount not positive", func(ctx context.Context, m *algo.Manager) error {
			p := validChangeOrder()
			p.EntrustAmount = "0"
			_, err := m.ChangeOrder(ctx, p)
			return err
		}},
		{"entrustQuery/orderId", func(ctx context.Context, m *algo.Manager) error {
			p := validQueryEntrustIDList()
			p.OrderID = ""
			_, err := m.QueryEntrustIDList(ctx, p)
			return err
		}},
		{"entrustQuery/tradeDate", func(ctx context.Context, m *algo.Manager) error {
			p := validQueryEntrustIDList()
			p.TradeDate = ""
			_, err := m.QueryEntrustIDList(ctx, p)
			return err
		}},
		{"entrustQuery/exchangeType", func(ctx context.Context, m *algo.Manager) error {
			p := validQueryEntrustIDList()
			p.ExchangeType = ""
			_, err := m.QueryEntrustIDList(ctx, p)
			return err
		}},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			m, rec := newManager(t, nil)

			err := row.call(context.Background(), m)
			if err == nil {
				t.Fatal("error = nil, want a local rejection")
			}
			if !errors.Is(err, algo.ErrInvalidParams) {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = false, err = %v", err)
			}
			if code, ok := errs.CodeOf(err); ok {
				t.Fatalf("errs.CodeOf(err) = (%q, true), want no code for a request that was never sent", code)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
			}
		})
	}
}

// TestMoneyAndQuantitiesRoundTripVerbatim asserts the request path never
// reformats a monetary or quantitative value. The inputs below are chosen so
// that any parse through float64 — the classic way a decimal string silently
// loses or rounds a quantity — would change them: 19 and 23 significant digits,
// a value that is zero to every float64 precision yet positive as a decimal
// string, and a trailing zero that a formatter would strip. Every field is a
// string end to end, so the only permitted transformation is none.
func TestMoneyAndQuantitiesRoundTripVerbatim(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathAddOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	params := validAddOrder()
	params.EntrustPrice = "0.1234567890123456789"
	params.EntrustAmount = "12345678901234567890123"
	params.StrategyParam = algo.StrategyParam{
		OrigStartTime: "093000",
		OrigEndTime:   "160000",
		MaxVolume:     "99999999999999999999.00000000000000000001",
		MinAmount:     "0.00000000000000000001",
		Sensitivity:   algo.SensitivityPassive,
		ShowQty:       "1.10",
		QtyPercent:    "99",
		Interval:      3600,
	}

	if _, err := m.AddOrder(context.Background(), params); err != nil {
		t.Fatalf("AddOrder: %v", err)
	}

	assertRequest(t, rec.last(pathAddOrder), pathAddOrder, `{
		"stockCode": "700.HK",
		"exchangeType": "K",
		"entrustType": "1",
		"entrustPrice": "0.1234567890123456789",
		"entrustAmount": "12345678901234567890123",
		"entrustBs": "1",
		"targetStrategy": "1",
		"sessionType": "0",
		"strategyParam": {
			"origStartTime": "093000",
			"origEndTime": "160000",
			"maxVolume": "99999999999999999999.00000000000000000001",
			"minAmount": "0.00000000000000000001",
			"sensitivity": "3",
			"showQty": "1.10",
			"qtyPercent": "99",
			"interval": 3600
		}
	}`)
}

// TestUnvalidatedCodesAreForwardedToTheGateway pins the fail-open half of the
// code surface. entrustType, sessionType, sensitivity, and action are closed
// sets and are rejected locally, but targetStrategy, exchangeType, and
// entrustBs are only checked for emptiness, so an unknown value is forwarded
// unchanged and the Gateway decides. This is deliberate for targetStrategy —
// the documented code set is self-contradictory, since "1005" is POV in the
// parameter table and INLINE in the request examples — but exchangeType and
// entrustBs are documented closed sets that carry no local check at all. The
// test records the current behaviour so a change in either direction is visible;
// it is not an endorsement, and tightening it would be a production change
// outside a coverage pass.
func TestUnvalidatedCodesAreForwardedToTheGateway(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathAddOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	params := validAddOrder()
	params.TargetStrategy = "9999"
	params.ExchangeType = "Z"
	params.EntrustBS = "3"

	if _, err := m.AddOrder(context.Background(), params); err != nil {
		t.Fatalf("AddOrder: %v", err)
	}
	assertRequest(t, rec.last(pathAddOrder), pathAddOrder, `{
		"stockCode": "700.HK",
		"exchangeType": "Z",
		"entrustType": "1",
		"entrustPrice": "350.5",
		"entrustAmount": "1000",
		"entrustBs": "3",
		"targetStrategy": "9999",
		"sessionType": "0",
		"strategyParam": {"maxVolume": "100", "sensitivity": "1"}
	}`)
}
