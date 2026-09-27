// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests in this file cover the fail-closed half of the two closed code sets
// in this package. Before A8, exchangeType and entrustBs were checked for
// emptiness at nine sites and not checked at all at eight, so types.EntrustBS
// had two answers inside one SDK: pkg/hstong/future and pkg/hstong/algo refused
// a value outside {1,2,3,4} locally while pkg/hstong/trade forwarded it onto
// the wire of a mutation that is issued once and never retried.
//
// Every case is driven through the Manager against the recorder from
// assets_test.go, so "rejected" is proven by two independent facts: a typed
// 1016 error, and zero requests recorded. A guard that validated and then forgot
// to return would satisfy the first and fail the second.

// exchangeSite is one caller-reachable exchangeType field. call invokes the
// Manager method with the market under test, so a site that drops its guard
// shows up as a recorded request rather than only as a missing message.
type exchangeSite struct {
	// name identifies the request type in the test output.
	name string
	// op is the operation label the error must carry, so the two endpoints that
	// share a request type cannot be confused.
	op string
	// route is the Gateway path the call must reach when the market is accepted.
	route client.Route
	// call invokes the Manager method for the given market.
	call func(context.Context, *Manager, types.ExchangeType) error
	// optional is true for the one site where an absent market is documented as
	// meaningful, so only a supplied value is checked.
	optional bool
}

// exchangeSites enumerates every caller-supplied exchangeType in this package:
// the fifteen required fields and the one optional field, Positions. It is the
// work list for A8, and a test that only covers part of it would leave a site
// unvalidated, so each of the three table tests below runs all sixteen rows.
func exchangeSites() []exchangeSite {
	return []exchangeSite{
		{
			name: "EntrustRequest", op: opEntrust, route: client.RouteTradeEntrust,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				req := validEntrust()
				req.ExchangeType = x
				_, err := m.Entrust(ctx, req)
				return err
			},
		},
		{
			name: "CancelEntrustRequest", op: opCancelEntrust, route: client.RouteTradeCancelEntrust,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				req := validCancel()
				req.ExchangeType = x
				_, err := m.CancelEntrust(ctx, req)
				return err
			},
		},
		{
			name: "BatchCancelEntrustRequest", op: opBatchCancelEntrust, route: client.RouteTradeBatchCancelEntrust,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.BatchCancelEntrust(ctx, BatchCancelEntrustRequest{ExchangeType: x, EntrustIDs: []string{"ENT-1"}})
				return err
			},
		},
		{
			name: "ChangeEntrustRequest", op: opChangeEntrust, route: client.RouteTradeChangeEntrust,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				req := validChange()
				req.ExchangeType = x
				_, err := m.ChangeEntrust(ctx, req)
				return err
			},
		},
		{
			name: "MaxAvailableAssetRequest", op: opMaxAvailableAsset, route: client.RouteTradeQueryMaxAvailableAsset,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				req := validMaxAvailable()
				req.ExchangeType = x
				_, err := m.MaxAvailableAsset(ctx, req)
				return err
			},
		},
		{
			name: "RealEntrustListRequest", op: opRealEntrustList, route: client.RouteTradeQueryRealEntrustList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.RealEntrustList(ctx, RealEntrustListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "RealDeliverListRequest", op: opRealDeliverList, route: client.RouteTradeQueryRealDeliverList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.RealDeliverList(ctx, RealDeliverListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "CondOrderListRequest", op: opRealCondOrderList, route: client.RouteTradeQueryRealCondOrderList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.RealCondOrderList(ctx, CondOrderListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "HistoryEntrustListRequest", op: opHistoryEntrustList, route: client.RouteTradeQueryHistoryEntrustList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.HistoryEntrustList(ctx, HistoryEntrustListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "HistoryDeliverListRequest", op: opHistoryDeliverList, route: client.RouteTradeQueryHistoryDeliverList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.HistoryDeliverList(ctx, HistoryDeliverListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "HistoryCondOrderListRequest", op: opHistoryCondOrderList, route: client.RouteTradeQueryHistoryCondOrderList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.HistoryCondOrderList(ctx, HistoryCondOrderListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "BeforeAndAfterSupportRequest", op: opBeforeAndAfterSupport, route: client.RouteTradeQueryBeforeAndAfterSupport,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.BeforeAndAfterSupport(ctx, BeforeAndAfterSupportRequest{StockCode: "01810.HK", ExchangeType: x})
				return err
			},
		},
		{
			name: "MarginFundInfoRequest", op: opMarginFundInfo, route: client.RouteTradeQueryMarginFundInfo,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.MarginFundInfo(ctx, MarginFundInfoRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "RealFundJourListRequest", op: opRealFundJourList, route: client.RouteTradeQueryRealFundJourList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.RealFundJourList(ctx, FundJourListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "HistoryFundJourListRequest", op: opHistoryFundJourList, route: client.RouteTradeQueryHistoryFundJourList,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.HistoryFundJourList(ctx, HistoryFundJourListRequest{ExchangeType: x})
				return err
			},
		},
		{
			name: "PositionsRequest", op: opHoldsList, route: client.RouteTradeQueryHoldsList, optional: true,
			call: func(ctx context.Context, m *Manager, x types.ExchangeType) error {
				_, err := m.Positions(ctx, PositionsRequest{ExchangeType: x})
				return err
			},
		},
	}
}

// TestEveryRequestSiteRejectsAnUnrecognisedExchangeType is the fail-closed
// property for exchangeType across every site in this package, so a guard
// dropped from any single request type is caught. The values include the
// near-misses a caller is actually likely to send: the wrong case on a
// case-sensitive field ("k", "T", "V"), a neighbouring code, a numeric where a
// letter belongs, and the market name rather than its code.
func TestEveryRequestSiteRejectsAnUnrecognisedExchangeType(t *testing.T) {
	for _, site := range exchangeSites() {
		for _, bad := range []types.ExchangeType{"Z", "k", "V", "T", "X", "0", "1", "HK", "Shenzhen Connect", "K "} {
			t.Run(site.name+"/"+string(bad), func(t *testing.T) {
				rec, m := newManager(t, nil)

				err := site.call(context.Background(), m, bad)
				if err == nil {
					t.Fatalf("%s(exchangeType=%q) = nil error, want a local rejection", site.name, bad)
				}
				assertInvalid(t, err)
				assertOp(t, err, site.op)
				if got := rec.total(); got != 0 {
					t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
				}
			})
		}
	}
}

// TestEveryRequestSiteRejectsAnEmptyRequiredExchangeType pins the emptiness half
// at the fifteen required sites, and pins the one exemption. Eight of those
// fifteen carried no check at all before A8, so a guard that checked only the
// set would have stopped them here — but a caller who forgot the field must also
// be told the field is required, not handed a message about four market codes.
// That distinction is asserted here, at every site, so the two branches cannot
// be folded into one another.
func TestEveryRequestSiteRejectsAnEmptyRequiredExchangeType(t *testing.T) {
	for _, site := range exchangeSites() {
		t.Run(site.name, func(t *testing.T) {
			rec, m := newManager(t, nil)

			err := site.call(context.Background(), m, "")
			if site.optional {
				// The one documented exemption: no market on Positions asks the
				// Gateway for every market, so the request is sent.
				if err != nil {
					t.Fatalf("Positions(exchangeType=\"\") = %v, want the request to be sent", err)
				}
				if got := rec.total(); got != 1 {
					t.Fatalf("requests = %d, want 1: an absent optional exchangeType is not a rejection", got)
				}
				return
			}
			if err == nil {
				t.Fatalf("%s(exchangeType=\"\") = nil error, want a local rejection", site.name)
			}
			assertInvalid(t, err)
			assertOp(t, err, site.op)
			assertMessage(t, err, "exchangeType is required")
			assertMessageAbsent(t, err, "not one of K (Hong Kong)")
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
			}
		})
	}
}

// TestEveryRequestSiteAcceptsEveryDocumentedMarket is the control for the two
// tests above: closing a set must not narrow it.
//
// Every one of the sixteen sites is driven with all four documented markets
// rather than the "K" the rest of the suite uses throughout, so a guard
// hard-coded to the fixture's market, or one that accepted only the two
// uppercase markets, fails here rather than hiding behind the common fixture.
// Each market is read back off the recorded body, so a value dropped or
// rewritten in transit is caught as well. This is also the wire-level proof
// that the two lowercase Stock Connect codes are accepted on every site.
func TestEveryRequestSiteAcceptsEveryDocumentedMarket(t *testing.T) {
	for _, site := range exchangeSites() {
		for _, ex := range []struct {
			code types.ExchangeType
			want string
		}{
			{types.ExchangeHK, "K"},
			{types.ExchangeUS, "P"},
			{types.ExchangeShenzhenConnect, "v"},
			{types.ExchangeShanghaiConnect, "t"},
		} {
			t.Run(site.name+"/"+ex.want, func(t *testing.T) {
				rec, m := newManager(t, nil)

				if err := site.call(context.Background(), m, ex.code); err != nil {
					t.Fatalf("%s(exchangeType=%q) = %v, want nil", site.name, ex.code, err)
				}
				if got := rec.total(); got != 1 {
					t.Fatalf("requests = %d, want 1", got)
				}
				if body := paramsField(t, rec, site.route, "exchangeType"); body != ex.want {
					t.Fatalf("%s wire exchangeType = %q, want %q", site.name, body, ex.want)
				}
			})
		}
	}
}

// TestEveryDocumentedDirectionIsAcceptedAndSends is the acceptance side of the
// entrustBs set, asserted on the wire.
//
// This is the test that the "3 is an invalid code" mistake cannot survive. 3 is
// types.EntrustCloseShort and 4 is types.EntrustOpenShort, the short-selling
// directions, and a check written against only 1 and 2 would reject legitimate
// orders. Every row here is asserted against the recorded request body, so a
// narrowed set fails even though a widened one would not.
func TestEveryDocumentedDirectionIsAcceptedAndSends(t *testing.T) {
	for _, d := range []struct {
		code types.EntrustBS
		want string
	}{
		{types.EntrustBuy, "1"},
		{types.EntrustSell, "2"},
		{types.EntrustCloseShort, "3"},
		{types.EntrustOpenShort, "4"},
	} {
		t.Run("entrustBs="+d.want, func(t *testing.T) {
			rec, m := newManager(t, nil)

			req := validEntrust()
			req.EntrustBS = d.code
			if _, err := m.Entrust(context.Background(), req); err != nil {
				t.Fatalf("Entrust(entrustBs=%q) = %v, want nil", d.code, err)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("requests = %d, want 1", got)
			}
			if body := paramsField(t, rec, client.RouteTradeEntrust, "entrustBs"); body != d.want {
				t.Fatalf("wire entrustBs = %q, want %q", body, d.want)
			}
		})
	}
}

// TestValidatedDirectionSetRejectsEveryOutOfSetValue is the reject side of the
// same set, on the one site that carries a direction. The values are the
// near-misses a caller is likely to send: a neighbouring code, a zero, a
// negative, a padded digit, a numeric-looking decimal, the name rather than the
// code, and a non-ASCII digit that a Unicode-aware check would fold to "1".
func TestValidatedDirectionSetRejectsEveryOutOfSetValue(t *testing.T) {
	for _, bad := range []types.EntrustBS{"0", "5", "9", "-1", "1.0", "01", " 1", "1 ", "buy", "BUY", "Sell", "١"} {
		t.Run(string(bad), func(t *testing.T) {
			rec, m := newManager(t, nil)

			req := validEntrust()
			req.EntrustBS = bad
			_, err := m.Entrust(context.Background(), req)
			if err == nil {
				t.Fatalf("Entrust(entrustBs=%q) = nil error, want a local rejection", bad)
			}
			assertInvalid(t, err)
			assertOp(t, err, opEntrust)
			assertMessage(t, err, "3 (close short)")
			assertMessage(t, err, "4 (open short)")
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
			}
		})
	}
}

// TestCodeSetErrorMessageIsIdenticalAtEverySite is the no-drift guard. Both
// messages are built once and shared, so a site that grew its own wording would
// show up here as a mismatch. The assertions on the content matter as much as
// the equality: the market message has to name all four markets, because the
// field is case-sensitive and "v" and "t" are lowercase, and the direction
// message has to name 3 and 4 so it cannot be read as evidence that only buy and
// sell are accepted.
func TestCodeSetErrorMessageIsIdenticalAtEverySite(t *testing.T) {
	// The shared wording, read once from the builder every site routes through,
	// is the reference each endpoint must reproduce. The operation label is
	// per-endpoint by design, so it is the text after it that has to be
	// identical.
	refErr := validateExchange(opEntrust, "Z")
	if refErr == nil {
		t.Fatal(`validateExchange(op, "Z") = nil, want a rejection`)
	}
	want := strings.TrimPrefix(messageOf(t, refErr), opEntrust+": ")
	for _, code := range []string{"K (Hong Kong)", "P (US)", "v (Shenzhen Connect)", "t (Shanghai Connect)"} {
		if !strings.Contains(want, code) {
			t.Fatalf("exchangeType message %q does not name %q", want, code)
		}
	}

	for _, site := range exchangeSites() {
		t.Run(site.name, func(t *testing.T) {
			rec, m := newManager(t, nil)

			err := site.call(context.Background(), m, "Z")
			if err == nil {
				t.Fatalf("%s(exchangeType=%q) = nil error, want a local rejection", site.name, "Z")
			}
			got := strings.TrimPrefix(messageOf(t, err), site.op+": ")
			if got != want {
				t.Fatalf("exchangeType message at %s:\n got %q\nwant %q", site.name, got, want)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
			}
		})
	}

	rec, m := newManager(t, nil)
	req := validEntrust()
	req.EntrustBS = "9"
	_, err := m.Entrust(context.Background(), req)
	if err == nil {
		t.Fatal(`Entrust(entrustBs="9") = nil error, want a local rejection`)
	}
	// The rendered text carries the 1016 status code before the message, so the
	// comparison is made against the shared builder's own output rather than a
	// hand-written literal, which is what makes this a no-drift check instead
	// of a second copy of the wording.
	if got, wantBs := messageOf(t, err), messageOf(t, invalidDirection(opEntrust, "9")); got != wantBs {
		t.Fatalf("entrustBs message:\n got %q\nwant %q", got, wantBs)
	}
	if got := rec.total(); got != 0 {
		t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
	}
}

// TestPredicatesAcceptExactlyTheDocumentedSets pins the two predicates
// themselves, independently of any endpoint. A set that gains a value, loses
// one, or grows a case-insensitive comparison fails here, which keeps the
// endpoint tests above from being the only thing standing between a wrong set
// and a caller.
func TestPredicatesAcceptExactlyTheDocumentedSets(t *testing.T) {
	exchanges := []struct {
		code types.ExchangeType
		want bool
	}{
		{types.ExchangeHK, true},
		{types.ExchangeUS, true},
		{types.ExchangeShenzhenConnect, true},
		{types.ExchangeShanghaiConnect, true},
		{"", false},
		{"k", false},
		{"K ", false},
		{"Z", false},
	}
	for _, tc := range exchanges {
		t.Run("validExchange("+string(tc.code)+")", func(t *testing.T) {
			if got := validExchange(tc.code); got != tc.want {
				t.Errorf("validExchange(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}

	directions := []struct {
		code types.EntrustBS
		want bool
	}{
		{types.EntrustBuy, true},
		{types.EntrustSell, true},
		{types.EntrustCloseShort, true},
		{types.EntrustOpenShort, true},
		{"", false},
		{"0", false},
		{"5", false},
		{"1 ", false},
	}
	for _, tc := range directions {
		t.Run("validDirection("+string(tc.code)+")", func(t *testing.T) {
			if got := validDirection(tc.code); got != tc.want {
				t.Errorf("validDirection(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

// TestValidateExchangeSeparatesAbsenceFromAnUnknownValue covers the shared
// helper directly, so the two branches are pinned even if a future endpoint
// stops routing through it.
func TestValidateExchangeSeparatesAbsenceFromAnUnknownValue(t *testing.T) {
	rec, m := newManager(t, nil)
	_ = m

	if err := validateExchange(opEntrust, ""); err == nil {
		t.Fatal(`validateExchange(op, "") = nil, want a rejection`)
	} else {
		assertMessage(t, err, "exchangeType is required")
		assertMessageAbsent(t, err, "not one of K (Hong Kong)")
	}

	if err := validateExchange(opEntrust, "Z"); err == nil {
		t.Fatal(`validateExchange(op, "Z") = nil, want a rejection`)
	} else {
		assertMessage(t, err, "not one of K (Hong Kong)")
		assertMessageAbsent(t, err, "exchangeType is required")
	}

	if err := validateExchange(opEntrust, types.ExchangeHK); err != nil {
		t.Errorf("validateExchange(op, K) = %v, want nil", err)
	}

	if err := validateOptionalExchange(opHoldsList, ""); err != nil {
		t.Errorf(`validateOptionalExchange(op, "") = %v, want nil: an absent market is meaningful`, err)
	}
	if err := validateOptionalExchange(opHoldsList, "Z"); err == nil {
		t.Fatal(`validateOptionalExchange(op, "Z") = nil, want a rejection: a supplied market is still checked`)
	} else {
		assertMessage(t, err, "not one of K (Hong Kong)")
	}

	// The helper is pure: neither rejection shape may reach the Gateway.
	if got := rec.total(); got != 0 {
		t.Fatalf("requests = %d, want 0: the helper is called before any request", got)
	}
}

// assertOp asserts that err carries the operation label the caller should see,
// so the two endpoints that share a request type cannot be confused.
func assertOp(t *testing.T, err error, wantOp string) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("err is %T, want *errs.Error", err)
	}
	if e.Op != wantOp {
		t.Errorf("Op = %q, want %q", e.Op, wantOp)
	}
}

// assertMessage asserts that err's rendered text contains want.
func assertMessage(t *testing.T, err error, want string) {
	t.Helper()
	if got := messageOf(t, err); !strings.Contains(got, want) {
		t.Errorf("message %q does not contain %q", got, want)
	}
}

// assertMessageAbsent asserts that err's rendered text does not contain want.
func assertMessageAbsent(t *testing.T, err error, want string) {
	t.Helper()
	if got := messageOf(t, err); strings.Contains(got, want) {
		t.Errorf("message %q unexpectedly contains %q", got, want)
	}
}

// messageOf returns the rendered text of a locally raised error.
func messageOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want a rejection")
	}
	return err.Error()
}

// paramsField returns the string value of key inside the params object of the
// most recent request to route, read from the raw body rather than from a
// decoded struct so a value dropped or rewritten in transit cannot hide behind
// a type conversion.
func paramsField(t *testing.T, rec *recorder, route client.Route, key string) string {
	t.Helper()
	path := string(route)
	got, ok := rec.last(path)
	if !ok {
		t.Fatalf("no request reached %s", path)
	}
	var env struct {
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(got.body, &env); err != nil {
		t.Fatalf("decode envelope for %s: %v (body=%s)", path, err, got.body)
	}
	raw, err := json.Marshal(env.Params[key])
	if err != nil {
		t.Fatalf("re-encode %s %s: %v", path, key, err)
	}
	return strings.Trim(string(raw), `"`)
}
