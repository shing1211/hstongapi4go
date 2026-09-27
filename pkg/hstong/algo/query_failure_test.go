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

// TestUnvalidatedCodesAreForwardedToTheGateway pins the one code that is still
// forwarded: targetStrategy. Its documented set is self-contradictory — "1005" is
// POV in the parameter table and INLINE in the request examples — so the SDK
// cannot decide which values are valid and only emptiness is rejected locally.
//
// This test previously also asserted that exchangeType and entrustBs were
// forwarded, and used "3" as its example of an unvalidated direction. That was
// wrong twice over: 3 is types.EntrustCloseShort, a documented short-selling
// direction, and exchangeType and entrustBs are closed sets that are now
// validated (A6). The test was inverted rather than deleted so that a future
// relaxation of those two fields is visible; TestLocalRejectionSendsNothing
// carries the fail-closed property and
// TestValidatedCodeSetsRejectEveryOutOfSetValue enumerates the accepted sets.
func TestUnvalidatedCodesAreForwardedToTheGateway(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathAddOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	params := validAddOrder()
	params.TargetStrategy = "9999"

	if _, err := m.AddOrder(context.Background(), params); err != nil {
		t.Fatalf("AddOrder: %v", err)
	}
	assertRequest(t, rec.last(pathAddOrder), pathAddOrder, `{
		"stockCode": "700.HK",
		"exchangeType": "K",
		"entrustType": "1",
		"entrustPrice": "350.5",
		"entrustAmount": "1000",
		"entrustBs": "1",
		"targetStrategy": "9999",
		"sessionType": "0",
		"strategyParam": {"maxVolume": "100", "sensitivity": "1"}
	}`)
}

// TestValidatedCodeSetsRejectEveryOutOfSetValue is the fail-closed property for
// the two sets A6 closed. It enumerates the accepted values and drives each
// request type through a Manager, so a value that escapes the check shows up as
// a request recorded against the server rather than as a local error. The
// out-of-set values are chosen to include the near-misses that a caller is
// actually likely to send: a wrong case ("k" for the lowercase-insensitive
// looking Hong Kong market, "V" for Shenzhen Connect), a neighbouring code, a
// numeric where a letter belongs, and the name rather than the code.
func TestValidatedCodeSetsRejectEveryOutOfSetValue(t *testing.T) {
	t.Run("exchangeType", func(t *testing.T) {
		for _, bad := range []types.ExchangeType{
			"z", "k", "p", "vX", "T", "V", "X", "K ", "0", "1", "HK", "Shenzhen Connect",
		} {
			t.Run(string(bad), func(t *testing.T) {
				m, rec := newManager(t, nil)

				p := validAddOrder()
				p.ExchangeType = bad
				if _, err := m.AddOrder(context.Background(), p); err == nil {
					t.Fatalf("AddOrder(exchangeType=%q) = nil error, want a local rejection", bad)
				} else if !errors.Is(err, algo.ErrInvalidParams) {
					t.Fatalf("AddOrder(exchangeType=%q): errors.Is(err, ErrInvalidParams) = false, err = %v", bad, err)
				}
				if got := rec.total(); got != 0 {
					t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
				}
			})
		}
	})

	t.Run("entrustBs", func(t *testing.T) {
		for _, bad := range []types.EntrustBS{
			"0", "5", "9", "-1", "1.0", "01", " 1", "buy", "BUY", "3 ", "١",
		} {
			t.Run(string(bad), func(t *testing.T) {
				m, rec := newManager(t, nil)

				p := validAddOrder()
				p.EntrustBS = bad
				if _, err := m.AddOrder(context.Background(), p); err == nil {
					t.Fatalf("AddOrder(entrustBs=%q) = nil error, want a local rejection", bad)
				} else if !errors.Is(err, algo.ErrInvalidParams) {
					t.Fatalf("AddOrder(entrustBs=%q): errors.Is(err, ErrInvalidParams) = false, err = %v", bad, err)
				}
				if got := rec.total(); got != 0 {
					t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
				}
			})
		}
	})
}

// TestEveryEndpointRejectsAnUnrecognisedExchangeType drives all seven request
// types, not just AddOrder, so a check dropped from any single validate method
// is caught. QueryOrderList is included in its optional shape: the field is
// present but unrecognised, which must be refused even though an absent one is
// accepted.
func TestEveryEndpointRejectsAnUnrecognisedExchangeType(t *testing.T) {
	rows := []struct {
		name string
		call func(context.Context, *algo.Manager) error
	}{
		{"AddOrder", func(ctx context.Context, m *algo.Manager) error {
			p := validAddOrder()
			p.ExchangeType = "Z"
			_, err := m.AddOrder(ctx, p)
			return err
		}},
		{"CancelOrder", func(ctx context.Context, m *algo.Manager) error {
			p := validCancelOrder()
			p.ExchangeType = "Z"
			_, err := m.CancelOrder(ctx, p)
			return err
		}},
		{"CancelEntrust", func(ctx context.Context, m *algo.Manager) error {
			p := validCancelEntrust()
			p.ExchangeType = "Z"
			_, err := m.CancelEntrust(ctx, p)
			return err
		}},
		{"ChangeOrder", func(ctx context.Context, m *algo.Manager) error {
			p := validChangeOrder()
			p.ExchangeType = "Z"
			_, err := m.ChangeOrder(ctx, p)
			return err
		}},
		{"ActionOrder", func(ctx context.Context, m *algo.Manager) error {
			p := validActionOrder()
			p.ExchangeType = "Z"
			_, err := m.ActionOrder(ctx, p)
			return err
		}},
		{"QueryOrderList", func(ctx context.Context, m *algo.Manager) error {
			p := validQueryOrderList()
			p.ExchangeType = "Z"
			_, err := m.QueryOrderList(ctx, p)
			return err
		}},
		{"QueryOrderList/optional and absent", func(ctx context.Context, m *algo.Manager) error {
			// The control case: exchangeType and stockCode both omitted is the
			// one QueryOrderList shape with no market in it at all, and it must
			// be sent rather than rejected.
			p := validQueryOrderList()
			p.ExchangeType = ""
			p.StockCode = ""
			_, err := m.QueryOrderList(ctx, p)
			return err
		}},
		{"QueryEntrustIDList", func(ctx context.Context, m *algo.Manager) error {
			p := validQueryEntrustIDList()
			p.ExchangeType = "Z"
			_, err := m.QueryEntrustIDList(ctx, p)
			return err
		}},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			m, rec := newManager(t, nil)

			err := row.call(context.Background(), m)
			if row.name == "QueryOrderList/optional and absent" {
				if err != nil {
					t.Fatalf("QueryOrderList with no exchangeType = %v, want the request to be sent", err)
				}
				if got := rec.total(); got != 1 {
					t.Fatalf("requests = %d, want 1: an omitted optional exchangeType is not a rejection", got)
				}
				return
			}
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

// TestEveryDocumentedCodeIsAccepted is the other half of the fail-closed
// contract: closing a set must not narrow it. All four markets and all four
// directions are driven through AddOrder and asserted on the wire, which also
// proves the short-selling directions 3 and 4 reach the Gateway rather than
// being refused by a set that mistakenly kept only buy and sell.
func TestEveryDocumentedCodeIsAccepted(t *testing.T) {
	exchanges := []struct {
		code types.ExchangeType
		want string
	}{
		{types.ExchangeHK, "K"},
		{types.ExchangeUS, "P"},
		{types.ExchangeShenzhenConnect, "v"},
		{types.ExchangeShanghaiConnect, "t"},
	}
	for _, ex := range exchanges {
		t.Run("exchangeType="+ex.want, func(t *testing.T) {
			m, rec := newManager(t, map[string]string{
				pathAddOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
			})

			p := validAddOrder()
			p.ExchangeType = ex.code
			if _, err := m.AddOrder(context.Background(), p); err != nil {
				t.Fatalf("AddOrder(exchangeType=%q): %v, want nil", ex.code, err)
			}
			assertRequest(t, rec.last(pathAddOrder), pathAddOrder, `{
				"stockCode": "700.HK",
				"exchangeType": "`+ex.want+`",
				"entrustType": "1",
				"entrustPrice": "350.5",
				"entrustAmount": "1000",
				"entrustBs": "1",
				"targetStrategy": "1",
				"sessionType": "0",
				"strategyParam": {"maxVolume": "100", "sensitivity": "1"}
			}`)
		})
	}

	directions := []struct {
		code types.EntrustBS
		want string
	}{
		{types.EntrustBuy, "1"},
		{types.EntrustSell, "2"},
		{types.EntrustCloseShort, "3"},
		{types.EntrustOpenShort, "4"},
	}
	for _, d := range directions {
		t.Run("entrustBs="+d.want, func(t *testing.T) {
			m, rec := newManager(t, map[string]string{
				pathAddOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
			})

			p := validAddOrder()
			p.EntrustBS = d.code
			if _, err := m.AddOrder(context.Background(), p); err != nil {
				t.Fatalf("AddOrder(entrustBs=%q): %v, want nil", d.code, err)
			}
			assertRequest(t, rec.last(pathAddOrder), pathAddOrder, `{
				"stockCode": "700.HK",
				"exchangeType": "K",
				"entrustType": "1",
				"entrustPrice": "350.5",
				"entrustAmount": "1000",
				"entrustBs": "`+d.want+`",
				"targetStrategy": "1",
				"sessionType": "0",
				"strategyParam": {"maxVolume": "100", "sensitivity": "1"}
			}`)
		})
	}
}

// TestUnrecognisedDefaultExchangeTypeIsRejected proves the default substituted
// by WithDefaultExchangeType is checked by the same rule as an explicit value.
// The default is applied inside the Manager method before validate runs, so
// without this the option would be a way to smuggle an unchecked market into
// every call that relies on it.
func TestUnrecognisedDefaultExchangeTypeIsRejected(t *testing.T) {
	m, rec := newManager(t, nil, algo.WithDefaultExchangeType("Z"))

	p := validCancelOrder()
	p.ExchangeType = ""
	if _, err := m.CancelOrder(context.Background(), p); err == nil {
		t.Fatal("CancelOrder with an unrecognised default exchangeType = nil error, want a local rejection")
	} else if !errors.Is(err, algo.ErrInvalidParams) {
		t.Fatalf("errors.Is(err, ErrInvalidParams) = false, err = %v", err)
	}
	if got := rec.total(); got != 0 {
		t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
	}
}
