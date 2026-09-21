// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind after every test.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// recorded is one captured HTTP request.
type recorded struct {
	method string
	path   string
	body   []byte
}

// recorder is an httptest handler that records every request and serves a
// per-path canned response. All access is mutex-guarded so it is safe under
// -race.
type recorder struct {
	mu        sync.Mutex
	requests  []recorded
	responses map[string]string
}

// ServeHTTP records the request and writes the canned response for its path, or
// a generic ok envelope when none is configured.
func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)

	r.mu.Lock()
	r.requests = append(r.requests, recorded{method: req.Method, path: req.URL.Path, body: body})
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
		if req.path == path {
			n++
		}
	}
	return n
}

// total returns how many requests were recorded.
func (r *recorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// last returns the most recent request for path.
func (r *recorder) last(path string) (recorded, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.requests) - 1; i >= 0; i-- {
		if r.requests[i].path == path {
			return r.requests[i], true
		}
	}
	return recorded{}, false
}

// newManager starts a recorder-backed test server, builds a client pointed at
// it (closed on cleanup), and returns a Manager over that client with no
// session.
func newManager(t *testing.T, responses map[string]string) (*recorder, *Manager) {
	t.Helper()
	rec := &recorder{responses: responses}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return rec, New(c)
}

// assertCall asserts that the most recent request to path was a POST whose
// envelope carries a numeric timeout_sec and whose params equal wantParams
// semantically. An empty wantParams skips the params comparison.
func assertCall(t *testing.T, rec *recorder, path, wantParams string) {
	t.Helper()

	got, ok := rec.last(path)
	if !ok {
		t.Fatalf("no request reached %s", path)
	}
	if got.method != http.MethodPost {
		t.Fatalf("%s method = %s, want POST", path, got.method)
	}

	var env struct {
		TimeoutSec json.Number     `json:"timeout_sec"`
		Params     json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(got.body, &env); err != nil {
		t.Fatalf("decode envelope %s: %v (body=%s)", path, err, got.body)
	}
	if env.TimeoutSec == "" {
		t.Fatalf("%s timeout_sec missing: %s", path, got.body)
	}
	if _, err := env.TimeoutSec.Int64(); err != nil {
		t.Fatalf("%s timeout_sec = %q, want a JSON number", path, env.TimeoutSec)
	}
	if wantParams == "" {
		return
	}

	var gotParams, want any
	if err := json.Unmarshal(env.Params, &gotParams); err != nil {
		t.Fatalf("decode %s params: %v (params=%s)", path, err, env.Params)
	}
	if err := json.Unmarshal([]byte(wantParams), &want); err != nil {
		t.Fatalf("decode want params %q: %v", wantParams, err)
	}
	if !reflect.DeepEqual(gotParams, want) {
		t.Fatalf("%s params = %s, want %s", path, env.Params, wantParams)
	}
}

// TestManager_MarginFundInfo asserts the canonical path, POST, numeric
// timeout_sec, exact params, and typed decoding.
func TestManager_MarginFundInfo(t *testing.T) {
	path := string(client.RouteTradeQueryMarginFundInfo)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"assetBalance":"1000.50","buyPowerUs":"2000","holdsBalance":"1.00","marketValue":"500"}}`,
	})

	out, err := m.MarginFundInfo(context.Background(), MarginFundInfoRequest{ExchangeType: types.ExchangeHK})
	if err != nil {
		t.Fatalf("MarginFundInfo: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K"}`)

	if out.AssetBalance != "1000.50" {
		t.Fatalf("AssetBalance = %q, want %q", out.AssetBalance, "1000.50")
	}
	if out.BuyPowerUs != "2000" {
		t.Fatalf("BuyPowerUs = %q, want %q", out.BuyPowerUs, "2000")
	}
	if out.HoldsBalance != "1.00" || out.MarketValue != "500" {
		t.Fatalf("deprecated fields not decoded: %+v", out)
	}
}

// TestManager_Positions asserts a holdsList array decodes and that the
// deprecated fields are carried.
func TestManager_Positions(t *testing.T) {
	path := string(client.RouteTradeQueryHoldsList)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"holdsList":[{"stockCode":"00700.HK","stockName":"Tencent","currentAmount":"100","lastPrice":"300.5","marketValueRate":"0.1"}]}}`,
	})

	rows, err := m.Positions(context.Background(), PositionsRequest{ExchangeType: types.ExchangeHK})
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K"}`)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].StockCode != "00700.HK" || rows[0].MarketValueRate != "0.1" {
		t.Fatalf("row = %+v", rows[0])
	}
}

// TestManager_PositionsInline asserts the current reference page's flattened
// single-object form also decodes.
func TestManager_PositionsInline(t *testing.T) {
	path := string(client.RouteTradeQueryHoldsList)
	_, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"stockCode":"AAPL","stockName":"Apple","currentAmount":"5"}}`,
	})

	rows, err := m.Positions(context.Background(), PositionsRequest{})
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	if len(rows) != 1 || rows[0].StockCode != "AAPL" {
		t.Fatalf("rows = %+v, want one AAPL row", rows)
	}
}

// TestManager_RealFundJourList asserts exact params and decoding.
func TestManager_RealFundJourList(t *testing.T) {
	path := string(client.RouteTradeQueryRealFundJourList)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":[{"businessBalance":"-100.0","type":"1","queryParamStr":"c1"}]}}`,
	})

	rows, err := m.RealFundJourList(context.Background(), FundJourListRequest{
		ExchangeType:  types.ExchangeHK,
		QueryCount:    2,
		QueryParamStr: "0",
	})
	if err != nil {
		t.Fatalf("RealFundJourList: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K","queryCount":2,"queryParamStr":"0"}`)

	if len(rows) != 1 || rows[0].BusinessBalance != "-100.0" {
		t.Fatalf("rows = %+v", rows)
	}
}

// TestManager_HistoryFundJourList asserts the date range is sent and decoded.
func TestManager_HistoryFundJourList(t *testing.T) {
	path := string(client.RouteTradeQueryHistoryFundJourList)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"data":[{"businessBalance":"10.0","time":"2023-01-02"}]}}`,
	})

	rows, err := m.HistoryFundJourList(context.Background(), HistoryFundJourListRequest{
		ExchangeType:  types.ExchangeHK,
		QueryCount:    DefaultPageSize,
		QueryParamStr: "0",
		StartDate:     "20230101",
		EndDate:       "20230102",
	})
	if err != nil {
		t.Fatalf("HistoryFundJourList: %v", err)
	}
	assertCall(t, rec, path, `{"exchangeType":"K","queryCount":20,"queryParamStr":"0","startDate":"20230101","endDate":"20230102"}`)

	if len(rows) != 1 || rows[0].Time != "2023-01-02" {
		t.Fatalf("rows = %+v", rows)
	}
}

// TestManager_ExchangeRate asserts the dynamic map decodes and RateType is
// forwarded.
func TestManager_ExchangeRate(t *testing.T) {
	path := string(client.RouteHsRateQueryList)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"HKD":{"USD":"0.12718763"},"USD":{"HKD":"7.83510000"}}}`,
	})

	rates, err := m.ExchangeRate(context.Background(), ExchangeRateRequest{RateType: "1"})
	if err != nil {
		t.Fatalf("ExchangeRate: %v", err)
	}
	assertCall(t, rec, path, `{"rateType":"1"}`)

	if rates["HKD"]["USD"] != "0.12718763" || rates["USD"]["HKD"] != "7.83510000" {
		t.Fatalf("rates = %+v", rates)
	}
}
