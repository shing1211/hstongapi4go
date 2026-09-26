// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests in this file cover the remaining decision points of the trade
// surface: the request defaults each paging method applies, the required fields
// the read methods reject locally, the order types for which the docs allow an
// empty entrustPrice, the inline-position decode error, and the fetch-error path
// of Paginate.
//
// They also pin the money contract in both directions. A sibling task replaced
// %.3f-formatting with verbatim decimal text after a 0.0005 tick was silently
// rounded to 0.001; the trade surface carries prices as Go strings end to end and
// must never reintroduce a fixed scale. Every assertion here is exact-string, and
// several of the values are deliberately chosen so that any float64 round trip
// would be visible.

// TestPriceOptional asserts the full documented contract: a market, iceberg
// market, or hidden market order may omit the price, as may any conditional
// order, and the two remaining types may not.
//
// The predicate is exercised directly as well as through Entrust because a
// change to it is a change to what the SDK will accept from a caller, which is
// worth pinning independently of the wire assertions below.
func TestPriceOptional(t *testing.T) {
	for _, tt := range []struct {
		orderType types.EntrustType
		want      bool
	}{
		// Market, iceberg market, and hidden market orders may omit the price.
		{types.EntrustTypeMarket, true},
		{types.EntrustTypeIcebergMarket, true},
		{types.EntrustTypeHiddenMarket, true},
		// Every conditional order may omit it too, through the default arm that
		// defers to isConditional. The trigger value, not the order price, is
		// what such an order is waiting on.
		{types.EntrustTypeStopProfitLimit, true},
		{types.EntrustTypeStopProfitMarket, true},
		{types.EntrustTypeStopLossLimit, true},
		{types.EntrustTypeStopLossMarket, true},
		{types.EntrustTypeTrailingStopLimit, true},
		{types.EntrustTypeTrailingStopMarket, true},
		// The remaining documented types require a price.
		{types.EntrustTypeLimit, false},
		{types.EntrustTypeIcebergLimit, false},
		// An unrecognised code is treated as requiring a price, so an unknown
		// order type fails closed rather than sending a priceless order.
		{types.EntrustType("99"), false},
		{types.EntrustType(""), false},
	} {
		t.Run(string(tt.orderType)+"/"+strconv.FormatBool(tt.want), func(t *testing.T) {
			if got := priceOptional(tt.orderType); got != tt.want {
				t.Errorf("priceOptional(%q) = %v, want %v", tt.orderType, got, tt.want)
			}
		})
	}
}

// TestManager_MarketOrdersOmitThePrice asserts that an order whose type allows
// an empty entrustPrice is sent without the field, that a supplied price is
// still forwarded, and that a limit order without a price is still rejected.
//
// Without the rejection case the first two assertions would hold even if
// priceOptional answered true for every type.
func TestManager_MarketOrdersOmitThePrice(t *testing.T) {
	const path = string(client.RouteTradeEntrust)

	for _, tc := range []struct {
		name       string
		req        EntrustRequest
		wantParams string
	}{
		{
			name: "market order omits the price",
			req: func() EntrustRequest {
				r := validEntrust()
				r.ExchangeType = types.ExchangeUS
				r.StockCode = "AAPL"
				r.EntrustType = types.EntrustTypeMarket
				r.EntrustPrice = ""
				return r
			}(),
			wantParams: `{"exchangeType":"P","stockCode":"AAPL","entrustAmount":"100","entrustBs":"1","entrustType":"5"}`,
		},
		{
			name: "hidden market order omits the price",
			req: func() EntrustRequest {
				r := validEntrust()
				r.ExchangeType = types.ExchangeUS
				r.StockCode = "AAPL"
				r.EntrustType = types.EntrustTypeHiddenMarket
				r.EntrustPrice = ""
				return r
			}(),
			wantParams: `{"exchangeType":"P","stockCode":"AAPL","entrustAmount":"100","entrustBs":"1","entrustType":"10"}`,
		},
		{
			name: "iceberg market order omits the price and sends the display size",
			req: func() EntrustRequest {
				r := validEntrust()
				r.ExchangeType = types.ExchangeUS
				r.StockCode = "AAPL"
				r.EntrustType = types.EntrustTypeIcebergMarket
				r.EntrustPrice = ""
				r.IceBergDisplaySize = "10"
				return r
			}(),
			wantParams: `{"exchangeType":"P","stockCode":"AAPL","entrustAmount":"100","entrustBs":"1","entrustType":"8","iceBergDisplaySize":"10"}`,
		},
		{
			name: "market order still forwards a price when one is given",
			req: func() EntrustRequest {
				r := validEntrust()
				r.ExchangeType = types.ExchangeUS
				r.StockCode = "AAPL"
				r.EntrustType = types.EntrustTypeMarket
				r.EntrustPrice = "180.25"
				return r
			}(),
			wantParams: `{"exchangeType":"P","stockCode":"AAPL","entrustAmount":"100","entrustPrice":"180.25","entrustBs":"1","entrustType":"5"}`,
		},
		{
			name: "conditional order omits the price",
			req: func() EntrustRequest {
				r := validEntrust()
				r.EntrustType = types.EntrustTypeTrailingStopLimit
				r.EntrustPrice = ""
				r.CondValue = "30"
				r.ValidDays = "10"
				r.CondTrackType = "1"
				return r
			}(),
			wantParams: `{"exchangeType":"K","stockCode":"01810.HK","entrustAmount":"100","entrustBs":"1","entrustType":"35","validDays":"10","condValue":"30","condTrackType":"1"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newManager(t, map[string]string{
				path: `{"ok":true,"err":"","data":{"data":"ENT-1"}}`,
			})
			if _, err := m.Entrust(context.Background(), tc.req); err != nil {
				t.Fatalf("Entrust: %v", err)
			}
			assertCall(t, rec, path, tc.wantParams)
		})
	}

	// The control: a limit order still needs a price, so a priceOptional that
	// answered true for everything would be caught here.
	t.Run("limit order without a price is still rejected", func(t *testing.T) {
		rec, m := newManager(t, nil)
		req := validEntrust()
		req.EntrustPrice = ""
		if _, err := m.Entrust(context.Background(), req); err == nil {
			t.Fatal("Entrust(limit, no price) = nil error, want rejection")
		} else {
			assertInvalid(t, err)
		}
		if got := rec.total(); got != 0 {
			t.Fatalf("requests = %d, want 0", got)
		}
	})
}

// TestManager_OrderPricesCrossTheWireVerbatim is the money guard for the order
// path.
//
// The Gateway's price is a decimal string and the SDK must not re-scale it. Each
// value below is chosen so that a fixed-scale format would be visible: 0.0005 is
// the tick that %.3f used to round up to 0.001, and 1.00000000000000000001
// carries twenty fractional digits that no float64 can represent, so any
// conversion to binary floating point and back would visibly change it.
//
// The assertion reads the raw request body rather than the decoded params,
// because a text substitution that happened to be idempotent would be invisible
// after a JSON decode.
func TestManager_OrderPricesCrossTheWireVerbatim(t *testing.T) {
	// 0.0005 and a twenty-digit value are unreachable through a float64; 180.25
	// and 0.1 are the cases a binary float could still represent.
	prices := []string{"0.0005", "0.1", "1.00000000000000000001", "180.25", "35.9005", "1234.567890123456789"}

	for _, tc := range []struct {
		name  string
		route client.Route
		body  string
		call  func(*Manager, string) error
	}{
		{
			name:  "Entrust",
			route: client.RouteTradeEntrust,
			body:  `{"ok":true,"err":"","data":{"data":"ENT-1"}}`,
			call: func(m *Manager, price string) error {
				r := validEntrust()
				r.EntrustPrice = price
				_, err := m.Entrust(context.Background(), r)
				return err
			},
		},
		{
			name:  "ChangeEntrust",
			route: client.RouteTradeChangeEntrust,
			body:  `{"ok":true,"err":"","data":{"data":"ENT-1"}}`,
			call: func(m *Manager, price string) error {
				r := validChange()
				r.EntrustPrice = price
				_, err := m.ChangeEntrust(context.Background(), r)
				return err
			},
		},
		{
			name:  "MaxAvailableAsset",
			route: client.RouteTradeQueryMaxAvailableAsset,
			body:  `{"ok":true,"err":"","data":{"data":{"positionStatus":"0"}}}`,
			call: func(m *Manager, price string) error {
				r := validMaxAvailable()
				r.EntrustPrice = price
				_, err := m.MaxAvailableAsset(context.Background(), r)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, price := range prices {
				rec, m := newManager(t, map[string]string{string(tc.route): tc.body})
				if err := tc.call(m, price); err != nil {
					t.Fatalf("%s(price=%q): %v", tc.name, price, err)
				}
				got, ok := rec.last(string(tc.route))
				if !ok {
					t.Fatalf("no request reached %s", tc.route)
				}
				want := []byte(`"entrustPrice":"` + price + `"`)
				if !bytes.Contains(got.body, want) {
					t.Fatalf("price %q did not reach the wire verbatim.\n want a body containing: %s\n            got: %s",
						price, want, got.body)
				}
			}
		})
	}
}

// TestManager_ResponseMoneyDecodesVerbatim is the decode-side half of the money
// contract: amounts with more precision than a float64 carries must reach the
// caller byte-for-byte.
func TestManager_ResponseMoneyDecodesVerbatim(t *testing.T) {
	const body = `{"ok":true,"err":"","data":{"data":[` +
		`{"entrustId":"E1","businessPrice":"1234.56789012345678901","businessBalance":"0.000000000000000001","entrustPrice":"0.0005"},` +
		`{"entrustId":"E2","businessPrice":"0.1","businessBalance":"-100.0","entrustPrice":"1.00000000000000000001"}` +
		`]}}`

	t.Run("deliver rows", func(t *testing.T) {
		_, m := newManager(t, map[string]string{
			string(client.RouteTradeQueryRealDeliverList): body,
		})
		rows, err := m.RealDeliverList(context.Background(), RealDeliverListRequest{ExchangeType: types.ExchangeHK})
		if err != nil {
			t.Fatalf("RealDeliverList: %v", err)
		}
		want := [][3]string{
			{"1234.56789012345678901", "0.000000000000000001", "0.0005"},
			{"0.1", "-100.0", "1.00000000000000000001"},
		}
		if len(rows) != len(want) {
			t.Fatalf("rows = %d, want %d", len(rows), len(want))
		}
		for i, w := range want {
			if rows[i].BusinessPrice != w[0] || rows[i].BusinessBalance != w[1] || rows[i].EntrustPrice != w[2] {
				t.Errorf("row %d = (%q, %q, %q), want %v", i,
					rows[i].BusinessPrice, rows[i].BusinessBalance, rows[i].EntrustPrice, w)
			}
		}
	})

	t.Run("fund-journey rows", func(t *testing.T) {
		_, m := newManager(t, map[string]string{
			string(client.RouteTradeQueryRealFundJourList): `{"ok":true,"err":"","data":{"data":[` +
				`{"businessBalance":"-0.000000000000000001","type":"1"}]}}`,
		})
		rows, err := m.RealFundJourList(context.Background(), FundJourListRequest{ExchangeType: types.ExchangeHK})
		if err != nil {
			t.Fatalf("RealFundJourList: %v", err)
		}
		if len(rows) != 1 || rows[0].BusinessBalance != "-0.000000000000000001" {
			t.Fatalf("rows = %+v, want the exact negative eighteen-digit amount", rows)
		}
	})
}

// TestManager_CondOrderListPagingDefaults asserts the page defaults each
// conditional-order query applies before sending: RealCondOrderList normalises a
// non-positive page number to 1 and clamps the size, HistoryCondOrderList
// substitutes the documented defaults for empty strings.
func TestManager_CondOrderListPagingDefaults(t *testing.T) {
	t.Run("real query normalises page number and size", func(t *testing.T) {
		const path = string(client.RouteTradeQueryRealCondOrderList)
		for _, tc := range []struct {
			name       string
			req        CondOrderListRequest
			wantParams string
		}{
			{
				name:       "zero page number and size",
				req:        CondOrderListRequest{ExchangeType: types.ExchangeHK},
				wantParams: `{"exchangeType":"K","pageNo":1,"pageSize":20}`,
			},
			{
				name:       "negative page number",
				req:        CondOrderListRequest{ExchangeType: types.ExchangeHK, PageNo: -5, PageSize: 5},
				wantParams: `{"exchangeType":"K","pageNo":1,"pageSize":5}`,
			},
			{
				name:       "size over the cap",
				req:        CondOrderListRequest{ExchangeType: types.ExchangeHK, PageNo: 2, PageSize: 1000},
				wantParams: `{"exchangeType":"K","pageNo":2,"pageSize":99}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				rec, m := newManager(t, map[string]string{
					path: `{"ok":true,"err":"","data":{"data":[],"curPageNo":1,"curPageSize":0,"totalPages":0}}`,
				})
				page, err := m.RealCondOrderList(context.Background(), tc.req)
				if err != nil {
					t.Fatalf("RealCondOrderList: %v", err)
				}
				if page == nil {
					t.Fatal("RealCondOrderList = nil page, want a non-nil empty page")
				}
				assertCall(t, rec, path, tc.wantParams)
			})
		}
	})

	t.Run("history query substitutes string defaults", func(t *testing.T) {
		const path = string(client.RouteTradeQueryHistoryCondOrderList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[],"curPageNo":1,"curPageSize":0,"totalPages":0}}`,
		})
		page, err := m.HistoryCondOrderList(context.Background(), HistoryCondOrderListRequest{
			ExchangeType: types.ExchangeHK,
		})
		if err != nil {
			t.Fatalf("HistoryCondOrderList: %v", err)
		}
		if page == nil {
			t.Fatal("HistoryCondOrderList = nil page, want a non-nil empty page")
		}
		// startTime and endTime carry no omitempty, so they are always on the
		// wire even when the caller left them empty.
		assertCall(t, rec, path, `{"exchangeType":"K","pageNo":"1","pageSize":"20","startTime":"","endTime":""}`)
	})
}

// TestManager_HistoryCondOrderListPageSizeIsNotClamped pins a documented
// asymmetry between the two conditional-order queries.
//
// RealCondOrderList carries the page size as an int and clamps it to
// [1, MaxPageSize]. HistoryCondOrderList carries it as the string the wire
// format documents and only substitutes a default when it is empty; a value the
// caller supplies is sent verbatim. That is the documented behaviour rather than
// an oversight, so it is pinned here so a future clamp cannot be added, and
// removed, unnoticed.
func TestManager_HistoryCondOrderListPageSizeIsNotClamped(t *testing.T) {
	const path = string(client.RouteTradeQueryHistoryCondOrderList)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":[]}}`,
	})

	page, err := m.HistoryCondOrderList(context.Background(), HistoryCondOrderListRequest{
		ExchangeType: types.ExchangeHK,
		PageNo:       "3",
		PageSize:     "1000",
	})
	if err != nil {
		t.Fatalf("HistoryCondOrderList: %v", err)
	}
	if page == nil {
		t.Fatal("HistoryCondOrderList = nil page, want a non-nil empty page")
	}
	assertCall(t, rec, path, `{"exchangeType":"K","pageNo":"3","pageSize":"1000","startTime":"","endTime":""}`)
}

// TestManager_ReadMethodsRejectBeforeSend asserts that the read methods with
// local required-field checks reject before anything reaches the Gateway. An
// order-side check that fired after the send would be a mutation on the wire for
// a request the SDK already knew was invalid.
func TestManager_ReadMethodsRejectBeforeSend(t *testing.T) {
	for _, tc := range []struct {
		name  string
		op    string
		route client.Route
		call  func(*Manager) error
	}{
		{
			name:  "MaxAvailableAsset without entrustPrice",
			op:    opMaxAvailableAsset,
			route: client.RouteTradeQueryMaxAvailableAsset,
			call: func(m *Manager) error {
				r := validMaxAvailable()
				r.EntrustPrice = ""
				_, err := m.MaxAvailableAsset(context.Background(), r)
				return err
			},
		},
		{
			name:  "MaxAvailableAsset without entrustType",
			op:    opMaxAvailableAsset,
			route: client.RouteTradeQueryMaxAvailableAsset,
			call: func(m *Manager) error {
				r := validMaxAvailable()
				r.EntrustType = ""
				_, err := m.MaxAvailableAsset(context.Background(), r)
				return err
			},
		},
		{
			name:  "RealCondOrderList without exchangeType",
			op:    opRealCondOrderList,
			route: client.RouteTradeQueryRealCondOrderList,
			call: func(m *Manager) error {
				_, err := m.RealCondOrderList(context.Background(), CondOrderListRequest{})
				return err
			},
		},
		{
			name:  "HistoryCondOrderList without exchangeType",
			op:    opHistoryCondOrderList,
			route: client.RouteTradeQueryHistoryCondOrderList,
			call: func(m *Manager) error {
				_, err := m.HistoryCondOrderList(context.Background(), HistoryCondOrderListRequest{})
				return err
			},
		},
		{
			name:  "MarginFullInfo without dataType",
			op:    opMarginFullInfo,
			route: client.RouteTradeQueryMarginFullInfo,
			call: func(m *Manager) error {
				_, err := m.MarginFullInfo(context.Background(), MarginFullInfoRequest{StockCode: "01810.HK"})
				return err
			},
		},
		{
			name:  "MarginFullInfo without stockCode",
			op:    opMarginFullInfo,
			route: client.RouteTradeQueryMarginFullInfo,
			call: func(m *Manager) error {
				_, err := m.MarginFullInfo(context.Background(), MarginFullInfoRequest{DataType: "10000"})
				return err
			},
		},
		{
			name:  "BeforeAndAfterSupport without stockCode",
			op:    opBeforeAndAfterSupport,
			route: client.RouteTradeQueryBeforeAndAfterSupport,
			call: func(m *Manager) error {
				_, err := m.BeforeAndAfterSupport(context.Background(), BeforeAndAfterSupportRequest{
					ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name:  "BeforeAndAfterSupport without exchangeType",
			op:    opBeforeAndAfterSupport,
			route: client.RouteTradeQueryBeforeAndAfterSupport,
			call: func(m *Manager) error {
				_, err := m.BeforeAndAfterSupport(context.Background(), BeforeAndAfterSupportRequest{
					StockCode: "01810.HK",
				})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newManager(t, nil)
			err := tc.call(m)
			if err == nil {
				t.Fatalf("%s = nil error, want a local rejection", tc.name)
			}
			e := errRejects(t, err, types.StatusInvalidParam, tc.op)
			if e.Category != errs.CategoryAPI {
				t.Errorf("Category = %q, want %q", e.Category, errs.CategoryAPI)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 (rejected before send)", got)
			}
		})
	}
}

// TestHoldsListResponseUnmarshalInlineDecodeError covers the third decode in
// UnmarshalJSON, the one that reads a single inline position object.
//
// A Gateway that sends a bare object with a wrongly typed field must produce a
// decode error rather than an empty position list: silently reporting no
// positions would tell a caller the account is flat when the truth is unknown.
func TestHoldsListResponseUnmarshalInlineDecodeError(t *testing.T) {
	for _, body := range []string{
		`{"stockCode":123}`,
		`{"stockName":7,"currentAmount":"100"}`,
		`{"exchangeType":["K"]}`,
	} {
		t.Run(body, func(t *testing.T) {
			var got HoldsListResponse
			err := json.Unmarshal([]byte(body), &got)
			if err == nil {
				t.Fatalf("Unmarshal(%s) = nil, want a decode error", body)
			}
			var typeErr *json.UnmarshalTypeError
			if !errors.As(err, &typeErr) {
				t.Fatalf("errors.As(%T) yielded no *json.UnmarshalTypeError", err)
			}
			if len(got.HoldsList) != 0 {
				t.Errorf("HoldsList len = %d, want 0", len(got.HoldsList))
			}
		})
	}
}

// TestPaginate_ReturnsRowsGatheredSoFarOnFetchError asserts the documented
// contract that a mid-walk failure returns the rows already collected together
// with the error, rather than discarding them or retrying.
//
// Dropping the rows would force a caller to re-walk from cursor "0" over a
// paginated endpoint whose earlier pages may already have rolled off.
func TestPaginate_ReturnsRowsGatheredSoFarOnFetchError(t *testing.T) {
	t.Run("first page fails", func(t *testing.T) {
		want := errors.New("scripted: first page unavailable")
		calls := 0
		fetch := func(_ context.Context, _ int, _ string) (Page[int], error) {
			calls++
			return Page[int]{}, want
		}

		got, err := Paginate(context.Background(), DefaultPageSize, fetch)
		if !errors.Is(err, want) {
			t.Fatalf("Paginate error = %v, want %v", err, want)
		}
		if got != nil {
			t.Errorf("Paginate = %v, want nil", got)
		}
		if calls != 1 {
			t.Errorf("fetch calls = %d, want 1: a failed fetch must not be retried", calls)
		}
	})

	t.Run("third page fails after two full pages", func(t *testing.T) {
		want := errors.New("scripted: page three unavailable")
		const size = 3
		calls := 0
		fetch := func(_ context.Context, _ int, cursor string) (Page[int], error) {
			calls++
			if cursor == "c2" {
				return Page[int]{}, want
			}
			items := make([]int, size)
			for i := range items {
				items[i] = i
			}
			return Page[int]{Items: items, Cursor: "c" + strconv.Itoa(calls)}, nil
		}

		got, err := Paginate(context.Background(), size, fetch)
		if !errors.Is(err, want) {
			t.Fatalf("Paginate error = %v, want %v", err, want)
		}
		if len(got) != 2*size {
			t.Fatalf("rows = %d, want %d: the pages already walked must be returned", len(got), 2*size)
		}
		if calls != 3 {
			t.Errorf("fetch calls = %d, want 3: the walk must stop at the failure", calls)
		}
	})
}
