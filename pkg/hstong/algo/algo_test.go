// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/hstong/algo"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind once every test has
// run (the Manager itself spawns none, and clients are closed by t.Cleanup).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Canonical route paths used by the assertions.
const (
	pathAddOrder           = "/trade/AlgoAddOrder"
	pathCancelOrder        = "/trade/AlgoCancelOrder"
	pathCancelEntrust      = "/trade/AlgoCancelEntrust"
	pathChangeOrder        = "/trade/AlgoChangeOrder"
	pathActionOrder        = "/trade/AlgoActionOrder"
	pathQueryOrderList     = "/trade/AlgoQueryOrderList"
	pathQueryEntrustIDList = "/trade/AlgoQueryEntrustIdList"
)

// recorded is one request observed by the test server.
type recorded struct {
	Method string
	Path   string
	Body   []byte
}

// recorder is an httptest handler that records every request and serves a
// per-path canned response. Access is mutex-guarded so it is race-safe.
type recorder struct {
	mu        sync.Mutex
	requests  []recorded
	responses map[string]string
}

// ServeHTTP records the request and writes the canned response for its path,
// or an empty successful envelope when none is configured.
func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)

	r.mu.Lock()
	r.requests = append(r.requests, recorded{Method: req.Method, Path: req.URL.Path, Body: body})
	resp, ok := r.responses[req.URL.Path]
	r.mu.Unlock()

	if !ok {
		resp = `{"ok":true,"err":"","data":{}}`
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, resp)
}

// count returns how many requests hit path.
func (r *recorder) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, req := range r.requests {
		if req.Path == path {
			n++
		}
	}
	return n
}

// total returns how many requests the server observed in all.
func (r *recorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// last returns the most recent request for path.
func (r *recorder) last(path string) recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.requests) - 1; i >= 0; i-- {
		if r.requests[i].Path == path {
			return r.requests[i]
		}
	}
	return recorded{}
}

// newManager starts a recorder-backed server and returns a Manager over it.
func newManager(t *testing.T, responses map[string]string, opts ...algo.Option) (*algo.Manager, *recorder) {
	t.Helper()
	rec := &recorder{responses: responses}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return algo.New(c, opts...), rec
}

// envelope is the Gateway request envelope with the payload kept raw so a test
// can assert its exact JSON.
type envelope struct {
	TimeoutSec any             `json:"timeout_sec"`
	Params     json.RawMessage `json:"params"`
}

// assertRequest asserts the canonical path, the POST method, a numeric
// timeout_sec, and the exact params of the recorded request.
func assertRequest(t *testing.T, rec recorded, wantPath, wantParams string) {
	t.Helper()
	if rec.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", rec.Method)
	}
	if rec.Path != wantPath {
		t.Fatalf("path = %q, want %q", rec.Path, wantPath)
	}
	var env envelope
	if err := json.Unmarshal(rec.Body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, rec.Body)
	}
	if _, ok := env.TimeoutSec.(float64); !ok {
		t.Fatalf("timeout_sec = %#v (%T), want a JSON number", env.TimeoutSec, env.TimeoutSec)
	}
	assertJSONEqual(t, env.Params, wantParams)
}

// assertJSONEqual compares two JSON documents structurally, ignoring key order
// and insignificant whitespace.
func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("decode got params: %v (%s)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want params: %v (%s)", err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("params mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// validAddOrder returns an AddOrderParams that passes local validation.
func validAddOrder() algo.AddOrderParams {
	return algo.AddOrderParams{
		StockCode:      "700.HK",
		ExchangeType:   types.ExchangeHK,
		EntrustType:    algo.EntrustTypeLimit,
		EntrustPrice:   "350.5",
		EntrustAmount:  "1000",
		EntrustBS:      types.EntrustBuy,
		TargetStrategy: algo.StrategyVWAP,
		SessionType:    algo.SessionTypeOff,
		StrategyParam: algo.StrategyParam{
			MaxVolume:   "100",
			Sensitivity: algo.SensitivityNeutral,
		},
	}
}

// validCancelOrder returns a CancelOrderParams that passes local validation.
func validCancelOrder() algo.CancelOrderParams {
	return algo.CancelOrderParams{OrderID: "MASTER-1", ExchangeType: types.ExchangeHK}
}

// validCancelEntrust returns a CancelEntrustParams that passes local validation.
func validCancelEntrust() algo.CancelEntrustParams {
	return algo.CancelEntrustParams{OrderID: "MASTER-1", EntrustID: "CHILD-9", ExchangeType: types.ExchangeHK}
}

// validChangeOrder returns a ChangeOrderParams that passes local validation.
func validChangeOrder() algo.ChangeOrderParams {
	return algo.ChangeOrderParams{
		OrderID:       "MASTER-1",
		StockCode:     "700.HK",
		ExchangeType:  types.ExchangeHK,
		EntrustPrice:  "360",
		EntrustAmount: "800",
		StrategyParam: algo.StrategyParam{Sensitivity: algo.SensitivityAggressive},
	}
}

// validActionOrder returns an ActionOrderParams that passes local validation.
func validActionOrder() algo.ActionOrderParams {
	return algo.ActionOrderParams{
		OrderID:        "MASTER-1",
		Action:         algo.ActionStart,
		TargetStrategy: algo.StrategyVWAP,
		ExchangeType:   types.ExchangeHK,
	}
}

// validQueryOrderList returns a QueryOrderListParams that passes local
// validation.
func validQueryOrderList() algo.QueryOrderListParams {
	return algo.QueryOrderListParams{
		PageNo:       "1",
		PageSize:     "30",
		StartDate:    "20260101",
		EndDate:      "20260131",
		ExchangeType: types.ExchangeHK,
		StockCode:    "700.HK",
	}
}

// validQueryEntrustIDList returns a QueryEntrustIDListParams that passes local
// validation.
func validQueryEntrustIDList() algo.QueryEntrustIDListParams {
	return algo.QueryEntrustIDListParams{
		OrderID:      "MASTER-1",
		TradeDate:    "20260105",
		ExchangeType: types.ExchangeHK,
	}
}

// TestManager_AddOrder asserts the add endpoint's path, method, numeric
// timeout_sec, exact params, and typed order-ID decoding.
func TestManager_AddOrder(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathAddOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	got, err := m.AddOrder(context.Background(), validAddOrder())
	if err != nil {
		t.Fatalf("AddOrder: %v", err)
	}
	if got != "MASTER-1" {
		t.Fatalf("order ID = %q, want %q", got, "MASTER-1")
	}
	assertRequest(t, rec.last(pathAddOrder), pathAddOrder, `{
		"stockCode": "700.HK",
		"exchangeType": "K",
		"entrustType": "1",
		"entrustPrice": "350.5",
		"entrustAmount": "1000",
		"entrustBs": "1",
		"targetStrategy": "1",
		"sessionType": "0",
		"strategyParam": {"maxVolume": "100", "sensitivity": "1"}
	}`)
}

// TestManager_CancelOrder asserts the master-cancel endpoint's request and
// decoding.
func TestManager_CancelOrder(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathCancelOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	got, err := m.CancelOrder(context.Background(), validCancelOrder())
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if got != "MASTER-1" {
		t.Fatalf("order ID = %q, want %q", got, "MASTER-1")
	}
	assertRequest(t, rec.last(pathCancelOrder), pathCancelOrder, `{
		"orderId": "MASTER-1",
		"exchangeType": "K"
	}`)
}

// TestManager_CancelEntrust asserts the child-cancel endpoint's request and
// decoding.
func TestManager_CancelEntrust(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathCancelEntrust: `{"ok":true,"err":"","data":{"data":"CHILD-9"}}`,
	})

	got, err := m.CancelEntrust(context.Background(), validCancelEntrust())
	if err != nil {
		t.Fatalf("CancelEntrust: %v", err)
	}
	if got != "CHILD-9" {
		t.Fatalf("entrust ID = %q, want %q", got, "CHILD-9")
	}
	assertRequest(t, rec.last(pathCancelEntrust), pathCancelEntrust, `{
		"orderId": "MASTER-1",
		"entrustId": "CHILD-9",
		"exchangeType": "K"
	}`)
}

// TestManager_ChangeOrder asserts the change endpoint's request and decoding.
func TestManager_ChangeOrder(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathChangeOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	got, err := m.ChangeOrder(context.Background(), validChangeOrder())
	if err != nil {
		t.Fatalf("ChangeOrder: %v", err)
	}
	if got != "MASTER-1" {
		t.Fatalf("order ID = %q, want %q", got, "MASTER-1")
	}
	assertRequest(t, rec.last(pathChangeOrder), pathChangeOrder, `{
		"orderId": "MASTER-1",
		"stockCode": "700.HK",
		"exchangeType": "K",
		"entrustPrice": "360",
		"entrustAmount": "800",
		"strategyParam": {"sensitivity": "2"}
	}`)
}

// TestManager_ActionOrder asserts the operate endpoint's request and decoding.
func TestManager_ActionOrder(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathActionOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
	})

	got, err := m.ActionOrder(context.Background(), validActionOrder())
	if err != nil {
		t.Fatalf("ActionOrder: %v", err)
	}
	if got != "MASTER-1" {
		t.Fatalf("order ID = %q, want %q", got, "MASTER-1")
	}
	assertRequest(t, rec.last(pathActionOrder), pathActionOrder, `{
		"orderId": "MASTER-1",
		"action": "1",
		"targetStrategy": "1",
		"exchangeType": "K"
	}`)
}

// TestManager_QueryOrderList asserts the master-order query's request and its
// typed decoding into []MasterOrder, including string money/quantities.
func TestManager_QueryOrderList(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathQueryOrderList: `{"ok":true,"err":"","data":{"algoOrderList":[{
			"orderId":"MASTER-1",
			"stockCode":"700.HK",
			"exchangeType":"K",
			"tradeDate":"20260105",
			"entrustType":"1",
			"entrustPrice":"350.5",
			"entrustAmount":"1000",
			"cumQty":"200",
			"leavesQty":"800",
			"status":"1",
			"entrustBs":"1",
			"targetStrategy":"1",
			"strategyStatus":"1",
			"strategyParam":{"maxVolume":"100","sensitivity":"1"},
			"roundLot":"100",
			"sendingTime":"20260105100000",
			"transactionTime":"20260105100001",
			"avgPx":"351"
		}]}}`,
	})

	got, err := m.QueryOrderList(context.Background(), validQueryOrderList())
	if err != nil {
		t.Fatalf("QueryOrderList: %v", err)
	}

	want := []algo.MasterOrder{{
		OrderID:         "MASTER-1",
		StockCode:       "700.HK",
		ExchangeType:    types.ExchangeHK,
		TradeDate:       "20260105",
		EntrustType:     algo.EntrustTypeLimit,
		EntrustPrice:    "350.5",
		EntrustAmount:   "1000",
		CumQty:          "200",
		LeavesQty:       "800",
		Status:          algo.StatusPartFilled,
		EntrustBS:       types.EntrustBuy,
		TargetStrategy:  algo.StrategyVWAP,
		StrategyStatus:  algo.StrategyStatusStart,
		StrategyParam:   algo.StrategyParam{MaxVolume: "100", Sensitivity: algo.SensitivityNeutral},
		RoundLot:        "100",
		SendingTime:     "20260105100000",
		TransactionTime: "20260105100001",
		AvgPx:           "351",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orders mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	assertRequest(t, rec.last(pathQueryOrderList), pathQueryOrderList, `{
		"pageNo": "1",
		"pageSize": "30",
		"startDate": "20260101",
		"endDate": "20260131",
		"exchangeType": "K",
		"stockCode": "700.HK"
	}`)
}

// TestManager_QueryEntrustIDList asserts the child-order query's request and
// typed decoding into []string.
func TestManager_QueryEntrustIDList(t *testing.T) {
	m, rec := newManager(t, map[string]string{
		pathQueryEntrustIDList: `{"ok":true,"err":"","data":{"entrustId":["CHILD-1","CHILD-2"]}}`,
	})

	got, err := m.QueryEntrustIDList(context.Background(), validQueryEntrustIDList())
	if err != nil {
		t.Fatalf("QueryEntrustIDList: %v", err)
	}
	want := []string{"CHILD-1", "CHILD-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entrust IDs = %#v, want %#v", got, want)
	}
	assertRequest(t, rec.last(pathQueryEntrustIDList), pathQueryEntrustIDList, `{
		"orderId": "MASTER-1",
		"tradeDate": "20260105",
		"exchangeType": "K"
	}`)
}

// TestAlgoMutationsAreSingleAttempt asserts each of the five mutations issues
// exactly one outbound request, even when the Gateway fails with a retryable
// code (ADR 0003).
func TestAlgoMutationsAreSingleAttempt(t *testing.T) {
	const retryableFailure = `{"ok":false,"err":"1011 service busy"}`

	cases := []struct {
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newManager(t, map[string]string{tc.path: retryableFailure})

			if err := tc.call(context.Background(), m); err == nil {
				t.Fatal("mutation = nil error, want error on Gateway failure")
			}
			if got := rec.count(tc.path); got != 1 {
				t.Fatalf("%s requests = %d, want exactly 1", tc.path, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want exactly 1", got)
			}
		})
	}
}

// TestManager_ValidationRejectsWithoutRequest asserts malformed arguments are
// rejected locally, wrapping ErrInvalidParams, before any request is sent.
func TestManager_ValidationRejectsWithoutRequest(t *testing.T) {
	missingStock := validAddOrder()
	missingStock.StockCode = ""

	badAmount := validAddOrder()
	badAmount.EntrustAmount = "0"

	badType := validAddOrder()
	badType.EntrustType = "9"

	missingOrder := validCancelOrder()
	missingOrder.OrderID = ""

	missingEntrust := validCancelEntrust()
	missingEntrust.EntrustID = ""

	badPrice := validChangeOrder()
	badPrice.EntrustPrice = "abc"

	badSensitivity := validChangeOrder()
	badSensitivity.StrategyParam.Sensitivity = "7"

	badAction := validActionOrder()
	badAction.Action = "9"

	missingStrategy := validActionOrder()
	missingStrategy.TargetStrategy = ""

	badPage := validQueryOrderList()
	badPage.PageNo = "0"

	badDate := validQueryOrderList()
	badDate.StartDate = "2026-01-01"

	missingExchange := validQueryOrderList()
	missingExchange.ExchangeType = ""

	badTradeDate := validQueryEntrustIDList()
	badTradeDate.TradeDate = "202601"

	cases := []struct {
		name string
		call func(context.Context, *algo.Manager) error
	}{
		{"add missing stockCode", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.AddOrder(ctx, missingStock)
			return err
		}},
		{"add zero amount", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.AddOrder(ctx, badAmount)
			return err
		}},
		{"add invalid entrustType", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.AddOrder(ctx, badType)
			return err
		}},
		{"cancel missing orderId", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.CancelOrder(ctx, missingOrder)
			return err
		}},
		{"cancel entrust missing entrustId", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.CancelEntrust(ctx, missingEntrust)
			return err
		}},
		{"change invalid price", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.ChangeOrder(ctx, badPrice)
			return err
		}},
		{"change invalid sensitivity", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.ChangeOrder(ctx, badSensitivity)
			return err
		}},
		{"action invalid code", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.ActionOrder(ctx, badAction)
			return err
		}},
		{"action missing strategy", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.ActionOrder(ctx, missingStrategy)
			return err
		}},
		{"query zero pageNo", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.QueryOrderList(ctx, badPage)
			return err
		}},
		{"query malformed date", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.QueryOrderList(ctx, badDate)
			return err
		}},
		{"query stockCode without exchange", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.QueryOrderList(ctx, missingExchange)
			return err
		}},
		{"entrust query malformed tradeDate", func(ctx context.Context, m *algo.Manager) error {
			_, err := m.QueryEntrustIDList(ctx, badTradeDate)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newManager(t, nil)
			err := tc.call(context.Background(), m)
			if err == nil {
				t.Fatal("error = nil, want validation error")
			}
			if !errors.Is(err, algo.ErrInvalidParams) {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = false, err = %v", err)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0 for a locally rejected call", got)
			}
		})
	}
}

// TestManager_DefaultExchangeType asserts a configured default fills an empty
// ExchangeType before the request is sent.
func TestManager_DefaultExchangeType(t *testing.T) {
	m, rec := newManager(t,
		map[string]string{pathCancelOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`},
		algo.WithDefaultExchangeType(types.ExchangeHK),
	)

	params := validCancelOrder()
	params.ExchangeType = ""
	if _, err := m.CancelOrder(context.Background(), params); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	assertRequest(t, rec.last(pathCancelOrder), pathCancelOrder, `{
		"orderId": "MASTER-1",
		"exchangeType": "K"
	}`)
}

// TestManager_GatewayFailureIsTyped asserts an ok:false envelope becomes a
// typed error carrying the Gateway status code.
func TestManager_GatewayFailureIsTyped(t *testing.T) {
	m, _ := newManager(t, map[string]string{
		pathActionOrder: `{"ok":false,"err":"1016 invalid parameter"}`,
	})

	_, err := m.ActionOrder(context.Background(), validActionOrder())
	if err == nil {
		t.Fatal("ActionOrder = nil error, want typed error")
	}
	if code, ok := errs.CodeOf(err); !ok || code != types.StatusInvalidParam {
		t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusInvalidParam)
	}
}
