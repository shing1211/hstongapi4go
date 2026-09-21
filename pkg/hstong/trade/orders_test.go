// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// validEntrust is a limit order that passes EntrustRequest validation.
func validEntrust() EntrustRequest {
	return EntrustRequest{
		ExchangeType:  types.ExchangeHK,
		StockCode:     "01810.HK",
		EntrustAmount: "100",
		EntrustPrice:  "35.900",
		EntrustBS:     types.EntrustBuy,
		EntrustType:   types.EntrustTypeLimit,
	}
}

// assertInvalid asserts err is the typed "1016" invalid-parameter error.
func assertInvalid(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want invalid-parameter error")
	}
	code, ok := errs.CodeOf(err)
	if !ok || code != types.StatusInvalidParam {
		t.Fatalf("CodeOf(%v) = (%q, %v), want (%q, true)", err, code, ok, types.StatusInvalidParam)
	}
}

// TestOrders_Entrust asserts the canonical path, POST, exact params, and that
// the entrust identifier is decoded. It also proves one request is sent.
func TestOrders_Entrust(t *testing.T) {
	path := string(client.RouteTradeEntrust)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":"ENT-1"}}`,
	})

	id, err := m.Entrust(context.Background(), validEntrust())
	if err != nil {
		t.Fatalf("Entrust: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K","stockCode":"01810.HK","entrustAmount":"100","entrustPrice":"35.900","entrustBs":"1","entrustType":"3"}`)

	if id != "ENT-1" {
		t.Fatalf("id = %q, want ENT-1", id)
	}
	if got := rec.count(path); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

// TestOrders_EntrustConditional asserts a conditional order with validDays and
// condValue is accepted and all optional fields are forwarded.
func TestOrders_EntrustConditional(t *testing.T) {
	path := string(client.RouteTradeEntrust)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":"COND-1"}}`,
	})

	req := EntrustRequest{
		ExchangeType:  types.ExchangeUS,
		StockCode:     "AAPL",
		EntrustAmount: "10",
		EntrustPrice:  "180.5",
		EntrustBS:     types.EntrustSell,
		EntrustType:   types.EntrustTypeStopLossLimit,
		SessionType:   "3",
		ValidDays:     "5",
		CondValue:     "170",
		CondTrackType: "1",
	}
	if _, err := m.Entrust(context.Background(), req); err != nil {
		t.Fatalf("Entrust(conditional): %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"P","stockCode":"AAPL","entrustAmount":"10","entrustPrice":"180.5","entrustBs":"2","entrustType":"33","sessionType":"3","validDays":"5","condValue":"170","condTrackType":"1"}`)
}

// TestOrders_EntrustValidation asserts every documented rejection.
func TestOrders_EntrustValidation(t *testing.T) {
	cases := []struct {
		name string
		req  EntrustRequest
	}{
		{"missing exchangeType", func() EntrustRequest { r := validEntrust(); r.ExchangeType = ""; return r }()},
		{"missing stockCode", func() EntrustRequest { r := validEntrust(); r.StockCode = ""; return r }()},
		{"missing entrustBs", func() EntrustRequest { r := validEntrust(); r.EntrustBS = ""; return r }()},
		{"missing entrustType", func() EntrustRequest { r := validEntrust(); r.EntrustType = ""; return r }()},
		{"zero amount", func() EntrustRequest { r := validEntrust(); r.EntrustAmount = "0"; return r }()},
		{"negative amount", func() EntrustRequest { r := validEntrust(); r.EntrustAmount = "-1"; return r }()},
		{"non-numeric amount", func() EntrustRequest { r := validEntrust(); r.EntrustAmount = "abc"; return r }()},
		{"limit missing price", func() EntrustRequest { r := validEntrust(); r.EntrustPrice = ""; return r }()},
		{"conditional missing condValue", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeStopProfitLimit
			r.ValidDays = "5"
			return r
		}()},
		{"conditional missing validDays", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeStopProfitLimit
			r.CondValue = "30"
			return r
		}()},
		{"conditional validDays zero", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeStopProfitLimit
			r.CondValue = "30"
			r.ValidDays = "0"
			return r
		}()},
		{"conditional validDays over 100", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeStopProfitLimit
			r.CondValue = "30"
			r.ValidDays = "101"
			return r
		}()},
		{"conditional validDays non-numeric", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeStopProfitLimit
			r.CondValue = "30"
			r.ValidDays = "soon"
			return r
		}()},
		{"iceberg missing display", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeIcebergLimit
			return r
		}()},
		{"iceberg zero display", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeIcebergLimit
			r.IceBergDisplaySize = "0"
			return r
		}()},
		{"iceberg display over amount", func() EntrustRequest {
			r := validEntrust()
			r.EntrustType = types.EntrustTypeIcebergLimit
			r.IceBergDisplaySize = "101"
			return r
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newManager(t, nil)
			if _, err := m.Entrust(context.Background(), tc.req); err == nil {
				t.Fatal("Entrust = nil error, want rejection")
			} else {
				assertInvalid(t, err)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 (rejected before send)", got)
			}
		})
	}
}

// TestOrders_CancelEntrust asserts the cancel params and identifier decoding.
func TestOrders_CancelEntrust(t *testing.T) {
	path := string(client.RouteTradeCancelEntrust)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":"ENT-1"}}`,
	})

	id, err := m.CancelEntrust(context.Background(), CancelEntrustRequest{
		ExchangeType:  types.ExchangeHK,
		StockCode:     "01810.HK",
		EntrustAmount: "100",
		EntrustPrice:  "35.900",
		EntrustID:     "ENT-1",
		EntrustType:   types.EntrustTypeLimit,
	})
	if err != nil {
		t.Fatalf("CancelEntrust: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K","stockCode":"01810.HK","entrustAmount":"100","entrustPrice":"35.900","entrustId":"ENT-1","entrustType":"3"}`)

	if id != "ENT-1" {
		t.Fatalf("id = %q, want ENT-1", id)
	}
}

// TestOrders_CancelEntrustValidation asserts the required fields.
func TestOrders_CancelEntrustValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  CancelEntrustRequest
	}{
		{"missing exchangeType", CancelEntrustRequest{StockCode: "X", EntrustID: "1"}},
		{"missing stockCode", CancelEntrustRequest{ExchangeType: types.ExchangeHK, EntrustID: "1"}},
		{"missing entrustId", CancelEntrustRequest{ExchangeType: types.ExchangeHK, StockCode: "X"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newManager(t, nil)
			if _, err := m.CancelEntrust(context.Background(), tc.req); err == nil {
				t.Fatal("CancelEntrust = nil error, want rejection")
			} else {
				assertInvalid(t, err)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0", got)
			}
		})
	}
}

// TestOrders_BatchCancelEntrust asserts params and the per-order outcome
// decoding.
func TestOrders_BatchCancelEntrust(t *testing.T) {
	path := string(client.RouteTradeBatchCancelEntrust)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"successEntrustId":["A"],"failCancelEntrust":[{"failEntrustId":"B","remark":"too late"}]}}`,
	})

	out, err := m.BatchCancelEntrust(context.Background(), BatchCancelEntrustRequest{
		ExchangeType: types.ExchangeHK,
		EntrustIDs:   []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("BatchCancelEntrust: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K","entrustId":["A","B"]}`)

	if len(out.SuccessEntrustID) != 1 || out.SuccessEntrustID[0] != "A" {
		t.Fatalf("SuccessEntrustID = %v", out.SuccessEntrustID)
	}
	if len(out.FailCancelEntrust) != 1 || out.FailCancelEntrust[0].FailEntrustID != "B" {
		t.Fatalf("FailCancelEntrust = %+v", out.FailCancelEntrust)
	}

	// exchangeType is required.
	rec2, m2 := newManager(t, nil)
	if _, err := m2.BatchCancelEntrust(context.Background(), BatchCancelEntrustRequest{}); err == nil {
		t.Fatal("BatchCancelEntrust without exchangeType = nil error")
	} else {
		assertInvalid(t, err)
	}
	if got := rec2.total(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

// TestOrders_ChangeEntrust asserts the change params and identifier decoding.
func TestOrders_ChangeEntrust(t *testing.T) {
	path := string(client.RouteTradeChangeEntrust)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":"ENT-2"}}`,
	})

	id, err := m.ChangeEntrust(context.Background(), ChangeEntrustRequest{
		ExchangeType:  types.ExchangeHK,
		StockCode:     "01810.HK",
		EntrustAmount: "200",
		EntrustPrice:  "36.000",
		EntrustID:     "ENT-1",
		EntrustType:   types.EntrustTypeLimit,
	})
	if err != nil {
		t.Fatalf("ChangeEntrust: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K","stockCode":"01810.HK","entrustAmount":"200","entrustPrice":"36.000","entrustId":"ENT-1","entrustType":"3"}`)

	if id != "ENT-2" {
		t.Fatalf("id = %q, want ENT-2", id)
	}
}

// TestOrders_ChangeEntrustValidation asserts the change-specific rejections.
func TestOrders_ChangeEntrustValidation(t *testing.T) {
	base := func() ChangeEntrustRequest {
		return ChangeEntrustRequest{
			ExchangeType:  types.ExchangeHK,
			StockCode:     "01810.HK",
			EntrustAmount: "200",
			EntrustPrice:  "36.000",
			EntrustID:     "ENT-1",
			EntrustType:   types.EntrustTypeLimit,
		}
	}
	for _, tc := range []struct {
		name string
		req  ChangeEntrustRequest
	}{
		{"missing amount", func() ChangeEntrustRequest { r := base(); r.EntrustAmount = "0"; return r }()},
		{"missing entrustId", func() ChangeEntrustRequest { r := base(); r.EntrustID = ""; return r }()},
		{"conditional missing condValue", func() ChangeEntrustRequest {
			r := base()
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.ValidDays = "3"
			return r
		}()},
		{"conditional missing validDays", func() ChangeEntrustRequest {
			r := base()
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.CondValue = "30"
			return r
		}()},
		{"conditional validDays over 100", func() ChangeEntrustRequest {
			r := base()
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.CondValue = "30"
			r.ValidDays = "101"
			return r
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newManager(t, nil)
			if _, err := m.ChangeEntrust(context.Background(), tc.req); err == nil {
				t.Fatal("ChangeEntrust = nil error, want rejection")
			} else {
				assertInvalid(t, err)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0", got)
			}
		})
	}
}

// TestOrders_Reads covers the nine order queries: canonical path, POST, exact
// params, and typed decoding.
func TestOrders_Reads(t *testing.T) {
	t.Run("MaxAvailableAsset", func(t *testing.T) {
		path := string(client.RouteTradeQueryMaxAvailableAsset)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":{"positionStatus":"0","longOpenAvailable":"100","cashAvailableAmount":"5000"}}}`,
		})
		out, err := m.MaxAvailableAsset(context.Background(), MaxAvailableAssetRequest{
			ExchangeType: types.ExchangeHK,
			StockCode:    "01810.HK",
			EntrustPrice: "35.900",
			EntrustType:  types.EntrustTypeLimit,
		})
		if err != nil {
			t.Fatalf("MaxAvailableAsset: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","stockCode":"01810.HK","entrustPrice":"35.900","entrustType":"3"}`)
		if out.PositionStatus != "0" || out.LongOpenAvailable != "100" {
			t.Fatalf("out = %+v", out)
		}
	})

	t.Run("RealEntrustList", func(t *testing.T) {
		path := string(client.RouteTradeQueryRealEntrustList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[{"entrustId":"E1","status":"2","canBeCanceled":1,"canBeUpdated":0,"fareVo":{"fare0":"1.00","faret":"2.00"}}]}}`,
		})
		rows, err := m.RealEntrustList(context.Background(), RealEntrustListRequest{
			ExchangeType:  types.ExchangeHK,
			QueryCount:    DefaultPageSize,
			QueryParamStr: "0",
		})
		if err != nil {
			t.Fatalf("RealEntrustList: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","queryCount":20,"queryParamStr":"0"}`)
		if len(rows) != 1 || rows[0].EntrustID != "E1" || rows[0].CanBeCanceled != 1 {
			t.Fatalf("rows = %+v", rows)
		}
		if rows[0].FareVo == nil || rows[0].FareVo.FareT != "2.00" {
			t.Fatalf("fareVo = %+v", rows[0].FareVo)
		}
	})

	t.Run("RealDeliverList", func(t *testing.T) {
		path := string(client.RouteTradeQueryRealDeliverList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[{"entrustId":"E1","businessAmount":"10"}]}}`,
		})
		rows, err := m.RealDeliverList(context.Background(), RealDeliverListRequest{
			ExchangeType:  types.ExchangeHK,
			QueryCount:    DefaultPageSize,
			QueryParamStr: "0",
		})
		if err != nil {
			t.Fatalf("RealDeliverList: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","queryCount":20,"queryParamStr":"0"}`)
		if len(rows) != 1 || rows[0].BusinessAmount != "10" {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("RealCondOrderList", func(t *testing.T) {
		path := string(client.RouteTradeQueryRealCondOrderList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[{"condOrderId":"C1","status":"1","entrustType":"31","entrustAmount":"100"}],"curPageNo":1,"curPageSize":1,"totalPages":1}}`,
		})
		page, err := m.RealCondOrderList(context.Background(), CondOrderListRequest{
			ExchangeType: types.ExchangeHK,
			PageNo:       1,
			PageSize:     20,
		})
		if err != nil {
			t.Fatalf("RealCondOrderList: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","pageNo":1,"pageSize":20}`)
		if len(page.Data) != 1 || page.Data[0].CondOrderID != "C1" || page.TotalPages != 1 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("HistoryEntrustList", func(t *testing.T) {
		path := string(client.RouteTradeQueryHistoryEntrustList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[{"entrustId":"E1"}]}}`,
		})
		rows, err := m.HistoryEntrustList(context.Background(), HistoryEntrustListRequest{
			ExchangeType:  types.ExchangeHK,
			QueryCount:    DefaultPageSize,
			QueryParamStr: "0",
			StartDate:     "20230101",
			EndDate:       "20230102",
		})
		if err != nil {
			t.Fatalf("HistoryEntrustList: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","queryCount":20,"queryParamStr":"0","startDate":"20230101","endDate":"20230102"}`)
		if len(rows) != 1 || rows[0].EntrustID != "E1" {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("HistoryDeliverList", func(t *testing.T) {
		path := string(client.RouteTradeQueryHistoryDeliverList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[{"entrustId":"E1","businessAmount":"5"}]}}`,
		})
		rows, err := m.HistoryDeliverList(context.Background(), HistoryDeliverListRequest{
			ExchangeType:  types.ExchangeHK,
			QueryCount:    DefaultPageSize,
			QueryParamStr: "0",
			StartDate:     "20230101",
			EndDate:       "20230102",
		})
		if err != nil {
			t.Fatalf("HistoryDeliverList: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","queryCount":20,"queryParamStr":"0","startDate":"20230101","endDate":"20230102"}`)
		if len(rows) != 1 || rows[0].BusinessAmount != "5" {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("HistoryCondOrderList", func(t *testing.T) {
		path := string(client.RouteTradeQueryHistoryCondOrderList)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":[{"condOrderId":"C1"}],"curPageNo":1,"curPageSize":1,"totalPages":3}}`,
		})
		page, err := m.HistoryCondOrderList(context.Background(), HistoryCondOrderListRequest{
			ExchangeType: types.ExchangeHK,
			PageNo:       "1",
			PageSize:     "20",
			StartTime:    "2021-01-01 19:21:21",
			EndTime:      "2021-01-02 19:21:21",
		})
		if err != nil {
			t.Fatalf("HistoryCondOrderList: %v", err)
		}
		assertCall(t, rec, path, `{"exchangeType":"K","pageNo":"1","pageSize":"20","startTime":"2021-01-01 19:21:21","endTime":"2021-01-02 19:21:21"}`)
		if len(page.Data) != 1 || page.TotalPages != 3 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("MarginFullInfo", func(t *testing.T) {
		path := string(client.RouteTradeQueryMarginFullInfo)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"marginAllow":"1","marginInitRatio":"0.5","rate":[{"currencyCode":"HKD","interestRateWithinMortgage":"0.05"}]}}`,
		})
		out, err := m.MarginFullInfo(context.Background(), MarginFullInfoRequest{
			DataType:  "10000",
			StockCode: "01810.HK",
		})
		if err != nil {
			t.Fatalf("MarginFullInfo: %v", err)
		}
		assertCall(t, rec, path, `{"dataType":"10000","stockCode":"01810.HK"}`)
		if out.MarginAllow != "1" || len(out.Rate) != 1 || out.Rate[0].InterestRateWithinMortgage != "0.05" {
			t.Fatalf("out = %+v", out)
		}
	})

	t.Run("BeforeAndAfterSupport", func(t *testing.T) {
		path := string(client.RouteTradeQueryBeforeAndAfterSupport)
		rec, m := newManager(t, map[string]string{
			path: `{"ok":true,"err":"","data":{"data":"1"}}`,
		})
		got, err := m.BeforeAndAfterSupport(context.Background(), BeforeAndAfterSupportRequest{
			StockCode:    "01810.HK",
			ExchangeType: types.ExchangeHK,
		})
		if err != nil {
			t.Fatalf("BeforeAndAfterSupport: %v", err)
		}
		assertCall(t, rec, path, `{"stockCode":"01810.HK","exchangeType":"K"}`)
		if got != "1" {
			t.Fatalf("got = %q, want 1", got)
		}
	})
}

// TestOrders_MutationsAreSingleAttempt asserts each order mutation issues
// exactly one outbound request per call and never retries (ADR 0003).
func TestOrders_MutationsAreSingleAttempt(t *testing.T) {
	cases := []struct {
		name string
		path client.Route
		call func(*Manager) error
	}{
		{
			name: "Entrust",
			path: client.RouteTradeEntrust,
			call: func(m *Manager) error {
				_, err := m.Entrust(context.Background(), validEntrust())
				return err
			},
		},
		{
			name: "CancelEntrust",
			path: client.RouteTradeCancelEntrust,
			call: func(m *Manager) error {
				_, err := m.CancelEntrust(context.Background(), CancelEntrustRequest{
					ExchangeType: types.ExchangeHK,
					StockCode:    "01810.HK",
					EntrustID:    "ENT-1",
				})
				return err
			},
		},
		{
			name: "BatchCancelEntrust",
			path: client.RouteTradeBatchCancelEntrust,
			call: func(m *Manager) error {
				_, err := m.BatchCancelEntrust(context.Background(), BatchCancelEntrustRequest{
					ExchangeType: types.ExchangeHK,
					EntrustIDs:   []string{"ENT-1"},
				})
				return err
			},
		},
		{
			name: "ChangeEntrust",
			path: client.RouteTradeChangeEntrust,
			call: func(m *Manager) error {
				_, err := m.ChangeEntrust(context.Background(), ChangeEntrustRequest{
					ExchangeType:  types.ExchangeHK,
					StockCode:     "01810.HK",
					EntrustAmount: "100",
					EntrustPrice:  "36.000",
					EntrustID:     "ENT-1",
					EntrustType:   types.EntrustTypeLimit,
				})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, m := newManager(t, nil)
			if err := tc.call(m); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got := rec.count(string(tc.path)); got != 1 {
				t.Fatalf("%s requests = %d, want exactly 1", tc.path, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("%s total requests = %d, want exactly 1", tc.path, got)
			}
		})
	}
}
