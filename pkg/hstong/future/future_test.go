// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package future

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind once every test has
// run.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// recordedRequest is one HTTP request observed by the test server.
type recordedRequest struct {
	method string
	path   string
	body   []byte
}

// gatewayRecorder is an httptest handler that records every request and serves
// a per-path canned response. Access is mutex-guarded so it is safe under
// -race while the client's goroutine and the test share it.
type gatewayRecorder struct {
	mu        sync.Mutex
	requests  []recordedRequest
	responses map[string]string
}

// ServeHTTP records the request and writes the canned response for its path (or
// an empty success body when none is configured).
func (g *gatewayRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	g.mu.Lock()
	g.requests = append(g.requests, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
	resp, ok := g.responses[r.URL.Path]
	g.mu.Unlock()

	if !ok {
		resp = `{"ok":true,"err":"","data":{}}`
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, resp)
}

// byPath returns the requests whose path equals path.
func (g *gatewayRecorder) byPath(path string) []recordedRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []recordedRequest
	for _, r := range g.requests {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

// total returns the total number of recorded requests.
func (g *gatewayRecorder) total() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

// newTestServer starts a recorder-backed server and registers its shutdown.
func newTestServer(t *testing.T, responses map[string]string) (*gatewayRecorder, *httptest.Server) {
	t.Helper()
	rec := &gatewayRecorder{responses: responses}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	return rec, srv
}

// newTestManager builds a Manager pointed at baseURL and registers the client's
// close.
func newTestManager(t *testing.T, baseURL string, opts ...Option) *Manager {
	t.Helper()
	c, err := client.New(client.WithBaseURL(baseURL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return New(c, opts...)
}

// requestEnvelope is the Gateway HTTP request envelope.
type requestEnvelope struct {
	TimeoutSec int             `json:"timeout_sec"`
	Params     json.RawMessage `json:"params"`
}

// decodeEnvelope decodes a request body, failing the test when timeout_sec is
// absent or not a JSON integer (a quoted or fractional timeout_sec would fail
// to decode into int and is therefore caught here).
func decodeEnvelope(t *testing.T, body []byte) requestEnvelope {
	t.Helper()
	var env requestEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode request envelope: %v (body=%s)", err, body)
	}
	return env
}

// assertParamsEqual compares the envelope params with want by decoding both to
// generic values, so key order and whitespace do not matter.
func assertParamsEqual(t *testing.T, got json.RawMessage, want any) {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want params: %v", err)
	}
	var gotValue, wantValue any
	if len(got) > 0 {
		if err := json.Unmarshal(got, &gotValue); err != nil {
			t.Fatalf("decode got params: %v (params=%s)", err, got)
		}
	}
	if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
		t.Fatalf("decode want params: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("params = %s, want %s", got, wantJSON)
	}
}

// sampleOrderRow is a representative entrust/deliver row fixture.
const sampleOrderRow = `{"stockCode":"HSI2603","stockName":"HSI MAR 26",` +
	`"businessPrice":"20000.5","entrustBs":"1","entrustPrice":"19999",` +
	`"entrustAmount":"2","businessAmount":"1","date":"20260102",` +
	`"businessTime":"09:31:00","entrustTime":"09:30:00","queryParamStr":"c1",` +
	`"statusDesc":"部成","status":"7","entrustId":"E1","canBeCanceled":1,` +
	`"entrustType":"限价","entrustTypeNum":"0","isValid":1,"canBeUpdated":1,` +
	`"validType":"0","validTypeDesc":"当天有效","orderOptions":0,` +
	`"validTime":"2026/01/02"}`

// endpointCase drives one endpoint through the offline server and asserts the
// canonical request shape and typed response.
type endpointCase struct {
	name       string
	route      client.Route
	response   string
	wantParams any
	call       func(context.Context, *Manager) (any, error)
	check      func(*testing.T, any)
}

// TestManagerEndpoints asserts that every endpoint posts to its canonical path
// with a numeric timeout_sec and exact params, and decodes a typed response.
func TestManagerEndpoints(t *testing.T) {
	const orderListResponse = `{"ok":true,"err":"","data":{"data":[` + sampleOrderRow + `]}}`
	const orderHistoryResponse = `{"ok":true,"err":"","data":{"data":[` + sampleOrderRow + `],` +
		`"curPageNo":1,"curPageSize":20,"totalPageNo":3,"lastPage":0}}`

	cases := []endpointCase{
		{
			name:  "QueryProductInfo",
			route: client.RouteTradeFuturesQueryProductInfo,
			response: `{"ok":true,"err":"","data":{"productInfoVos":[` +
				`{"prodCode":"HSI","instCode":"FUT","lotSize":50,"decInPrice":0,` +
				`"contractSize":"50","priceDecimalPoint":"1","expiryDate":"2026-03-30","isSupportT1":0}]}}`,
			wantParams: QueryProductInfoRequest{StockCodes: []string{"HSI2603", "MHI2603"}},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryProductInfo(ctx, QueryProductInfoRequest{StockCodes: []string{"HSI2603", "MHI2603"}})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp, ok := got.(*QueryProductInfoResponse)
				if !ok {
					t.Fatalf("response type = %T", got)
				}
				if len(resp.ProductInfoVos) != 1 {
					t.Fatalf("ProductInfoVos len = %d, want 1", len(resp.ProductInfoVos))
				}
				p := resp.ProductInfoVos[0]
				if p.ProdCode != "HSI" || p.LotSize != 50 || p.ExpiryDate != "2026-03-30" || p.IsSupportT1 != 0 {
					t.Fatalf("decoded product = %+v", p)
				}
			},
		},
		{
			name:       "QueryMaxBuySellAmount",
			route:      client.RouteTradeFuturesQueryMaxBuySellAmount,
			response:   `{"ok":true,"err":"","data":{"positionStatus":1,"maxBuyAmount":100,"maxSellAmount":50,"initialMargin":"12345.67","ccy":"HKD"}}`,
			wantParams: QueryMaxBuySellAmountRequest{StockCode: "HSI2603"},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryMaxBuySellAmount(ctx, QueryMaxBuySellAmountRequest{StockCode: "HSI2603"})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*QueryMaxBuySellAmountResponse)
				if resp.PositionStatus != 1 || resp.MaxBuyAmount != 100 || resp.MaxSellAmount != 50 ||
					resp.InitialMargin != "12345.67" || resp.Ccy != "HKD" {
					t.Fatalf("decoded response = %+v", resp)
				}
			},
		},
		{
			name:       "QueryFundInfo",
			route:      client.RouteTradeFuturesQueryFundInfo,
			response:   `{"ok":true,"err":"","data":{"fundInfo":{"assetBalance":"100000.00","enableBalance":"80000.00","cashBal":"50000.00","iMargin":"1000.00","mMargin":"800.00","marginStatus":"1"}}}`,
			wantParams: struct{}{},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryFundInfo(ctx)
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*QueryFundInfoResponse)
				if resp.FundInfo.AssetBalance != "100000.00" || resp.FundInfo.EnableBalance != "80000.00" ||
					resp.FundInfo.IMargin != "1000.00" || resp.FundInfo.MarginStatus != "1" {
					t.Fatalf("decoded fundInfo = %+v", resp.FundInfo)
				}
			},
		},
		{
			name:       "QueryHoldsList",
			route:      client.RouteTradeFuturesQueryHoldsList,
			response:   `{"ok":true,"err":"","data":{"fundInfo":{"assetBalance":"1.00"},"holdsList":[{"stockCode":"HSI2603","stockName":"HSI","currentQty":"3","costPrice":"19000.5","profitLoss":"-12.5","dataType":"10010"}]}}`,
			wantParams: struct{}{},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryHoldsList(ctx)
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*QueryHoldsListResponse)
				if len(resp.HoldsList) != 1 {
					t.Fatalf("HoldsList len = %d, want 1", len(resp.HoldsList))
				}
				h := resp.HoldsList[0]
				if h.StockCode != "HSI2603" || h.CurrentQty != "3" || h.CostPrice != "19000.5" ||
					h.ProfitLoss != "-12.5" || h.DataType != "10010" {
					t.Fatalf("decoded hold = %+v", h)
				}
			},
		},
		{
			name:     "Entrust",
			route:    client.RouteTradeFuturesEntrust,
			response: `{"ok":true,"err":"","data":{"data":"E1"}}`,
			wantParams: EntrustRequest{
				StockCode:     "HSI2603",
				EntrustType:   "0",
				EntrustPrice:  "19999",
				EntrustAmount: "2",
				EntrustBS:     "1",
				ValidTimeType: "0",
			},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.Entrust(ctx, EntrustRequest{
					StockCode: "HSI2603", EntrustType: "0", EntrustPrice: "19999",
					EntrustAmount: "2", EntrustBS: "1", ValidTimeType: "0",
				})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				if resp := got.(*EntrustResponse); resp.Data != "E1" {
					t.Fatalf("Data = %q, want %q", resp.Data, "E1")
				}
			},
		},
		{
			name:       "CancelEntrust",
			route:      client.RouteTradeFuturesCancelEntrust,
			response:   `{"ok":true,"err":"","data":{"data":"E1"}}`,
			wantParams: CancelEntrustRequest{EntrustID: "E1", StockCode: "HSI2603"},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.CancelEntrust(ctx, CancelEntrustRequest{EntrustID: "E1", StockCode: "HSI2603"})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				if resp := got.(*EntrustResponse); resp.Data != "E1" {
					t.Fatalf("Data = %q, want %q", resp.Data, "E1")
				}
			},
		},
		{
			name:     "ModifyEntrust",
			route:    client.RouteTradeFuturesModifyEntrust,
			response: `{"ok":true,"err":"","data":{"data":"E1"}}`,
			wantParams: ModifyEntrustRequest{
				EntrustID: "E1", StockCode: "HSI2603", EntrustPrice: "20001",
				EntrustAmount: "3", EntrustBS: "1", ValidTimeType: "0",
			},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.ModifyEntrust(ctx, ModifyEntrustRequest{
					EntrustID: "E1", StockCode: "HSI2603", EntrustPrice: "20001",
					EntrustAmount: "3", EntrustBS: "1", ValidTimeType: "0",
				})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				if resp := got.(*EntrustResponse); resp.Data != "E1" {
					t.Fatalf("Data = %q, want %q", resp.Data, "E1")
				}
			},
		},
		{
			name:       "QueryRealEntrustList",
			route:      client.RouteTradeFuturesQueryRealEntrustList,
			response:   orderListResponse,
			wantParams: struct{}{},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryRealEntrustList(ctx)
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*EntrustListResponse)
				if len(resp.Data) != 1 {
					t.Fatalf("Data len = %d, want 1", len(resp.Data))
				}
				row := resp.Data[0]
				if row.StockCode != "HSI2603" || row.EntrustAmount != "2" || row.BusinessAmount != "1" ||
					row.CanBeCanceled != 1 || row.OrderOptions != 0 || row.EntrustID != "E1" {
					t.Fatalf("decoded row = %+v", row)
				}
			},
		},
		{
			name:       "QueryHistoryEntrustList",
			route:      client.RouteTradeFuturesQueryHistoryEntrustList,
			response:   orderHistoryResponse,
			wantParams: HistoryQueryRequest{PageNo: 2, PageSize: 20, StartDate: "20260101", EndDate: "20260131"},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryHistoryEntrustList(ctx, HistoryQueryRequest{
					PageNo: 2, PageSize: 20, StartDate: "20260101", EndDate: "20260131",
				})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*EntrustListResponse)
				if resp.CurPageNo != 1 || resp.CurPageSize != 20 || resp.TotalPageNo != 3 || resp.LastPage != 0 {
					t.Fatalf("decoded pagination = %+v", resp)
				}
				if len(resp.Data) != 1 || resp.Data[0].Status != "7" {
					t.Fatalf("decoded data = %+v", resp.Data)
				}
			},
		},
		{
			name:       "QueryRealDeliverList",
			route:      client.RouteTradeFuturesQueryRealDeliverList,
			response:   orderListResponse,
			wantParams: struct{}{},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryRealDeliverList(ctx)
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*DeliverListResponse)
				if len(resp.Data) != 1 || resp.Data[0].BusinessPrice != "20000.5" {
					t.Fatalf("decoded data = %+v", resp.Data)
				}
			},
		},
		{
			name:       "QueryHistoryDeliverList",
			route:      client.RouteTradeFuturesQueryHistoryDeliverList,
			response:   orderHistoryResponse,
			wantParams: HistoryQueryRequest{PageNo: 1, PageSize: 20},
			call: func(ctx context.Context, m *Manager) (any, error) {
				resp, err := m.QueryHistoryDeliverList(ctx, HistoryQueryRequest{})
				if err != nil {
					return nil, err
				}
				return resp, nil
			},
			check: func(t *testing.T, got any) {
				resp := got.(*DeliverListResponse)
				if resp.CurPageNo != 1 || len(resp.Data) != 1 {
					t.Fatalf("decoded response = %+v", resp)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rec, srv := newTestServer(t, map[string]string{string(tc.route): tc.response})
			m := newTestManager(t, srv.URL)

			got, err := tc.call(context.Background(), m)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}

			reqs := rec.byPath(string(tc.route))
			if len(reqs) != 1 {
				t.Fatalf("%s requests = %d, want exactly 1", tc.route, len(reqs))
			}
			if reqs[0].method != http.MethodPost {
				t.Fatalf("method = %q, want POST", reqs[0].method)
			}
			env := decodeEnvelope(t, reqs[0].body)
			if env.TimeoutSec < 1 {
				t.Fatalf("timeout_sec = %d, want >= 1", env.TimeoutSec)
			}
			assertParamsEqual(t, env.Params, tc.wantParams)

			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestHistoryQueryDefaults asserts the manager applies page defaults and honors
// the WithDefaultPageSize option.
func TestHistoryQueryDefaults(t *testing.T) {
	route := client.RouteTradeFuturesQueryHistoryEntrustList
	response := `{"ok":true,"err":"","data":{"data":[],"curPageNo":1,"curPageSize":20}}`

	t.Run("built-in defaults", func(t *testing.T) {
		rec, srv := newTestServer(t, map[string]string{string(route): response})
		m := newTestManager(t, srv.URL)

		if _, err := m.QueryHistoryEntrustList(context.Background(), HistoryQueryRequest{}); err != nil {
			t.Fatalf("QueryHistoryEntrustList: %v", err)
		}
		env := decodeEnvelope(t, rec.byPath(string(route))[0].body)
		assertParamsEqual(t, env.Params, HistoryQueryRequest{PageNo: 1, PageSize: 20})
	})

	t.Run("WithDefaultPageSize", func(t *testing.T) {
		rec, srv := newTestServer(t, map[string]string{string(route): response})
		m := newTestManager(t, srv.URL, WithDefaultPageSize(50))

		if _, err := m.QueryHistoryEntrustList(context.Background(), HistoryQueryRequest{}); err != nil {
			t.Fatalf("QueryHistoryEntrustList: %v", err)
		}
		env := decodeEnvelope(t, rec.byPath(string(route))[0].body)
		assertParamsEqual(t, env.Params, HistoryQueryRequest{PageNo: 1, PageSize: 50})
	})

	t.Run("oversized option falls back to default", func(t *testing.T) {
		rec, srv := newTestServer(t, map[string]string{string(route): response})
		m := newTestManager(t, srv.URL, WithDefaultPageSize(100))

		if _, err := m.QueryHistoryEntrustList(context.Background(), HistoryQueryRequest{}); err != nil {
			t.Fatalf("QueryHistoryEntrustList: %v", err)
		}
		env := decodeEnvelope(t, rec.byPath(string(route))[0].body)
		assertParamsEqual(t, env.Params, HistoryQueryRequest{PageNo: 1, PageSize: 20})
	})
}

// TestFutureMutationsAreSingleAttempt asserts each futures mutation issues
// exactly one outbound request, both on success and on a retryable gateway
// failure (1015), proving there is no auto-retry (ADR 0003).
func TestFutureMutationsAreSingleAttempt(t *testing.T) {
	cases := []struct {
		name  string
		route client.Route
		call  func(context.Context, *Manager) error
	}{
		{
			name:  "Entrust",
			route: client.RouteTradeFuturesEntrust,
			call: func(ctx context.Context, m *Manager) error {
				_, err := m.Entrust(ctx, EntrustRequest{
					StockCode: "HSI2603", EntrustType: "0", EntrustPrice: "19999",
					EntrustAmount: "2", EntrustBS: "1", ValidTimeType: "0",
				})
				return err
			},
		},
		{
			name:  "CancelEntrust",
			route: client.RouteTradeFuturesCancelEntrust,
			call: func(ctx context.Context, m *Manager) error {
				_, err := m.CancelEntrust(ctx, CancelEntrustRequest{EntrustID: "E1", StockCode: "HSI2603"})
				return err
			},
		},
		{
			name:  "ModifyEntrust",
			route: client.RouteTradeFuturesModifyEntrust,
			call: func(ctx context.Context, m *Manager) error {
				_, err := m.ModifyEntrust(ctx, ModifyEntrustRequest{
					EntrustID: "E1", StockCode: "HSI2603", EntrustPrice: "20001",
					EntrustAmount: "3", EntrustBS: "1", ValidTimeType: "0",
				})
				return err
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"/success", func(t *testing.T) {
			rec, srv := newTestServer(t, map[string]string{string(tc.route): `{"ok":true,"err":"","data":{"data":"E1"}}`})
			m := newTestManager(t, srv.URL)
			if err := tc.call(context.Background(), m); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("outbound requests = %d, want exactly 1", got)
			}
		})

		t.Run(tc.name+"/gateway-failure", func(t *testing.T) {
			rec, srv := newTestServer(t, map[string]string{string(tc.route): `{"ok":false,"err":"1015 call timeout"}`})
			m := newTestManager(t, srv.URL)
			err := tc.call(context.Background(), m)
			if err == nil {
				t.Fatalf("%s = nil error, want typed error", tc.name)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("outbound requests = %d, want exactly 1 (no auto-retry)", got)
			}
		})
	}
}

// TestValidationRejectsInvalidInput asserts malformed input is rejected with a
// typed "1016" error and that no request reaches the server.
func TestValidationRejectsInvalidInput(t *testing.T) {
	responses := map[string]string{}
	for _, r := range client.Routes() {
		responses[string(r)] = `{"ok":true,"err":"","data":{}}`
	}

	cases := []struct {
		name string
		call func(context.Context, *Manager) error
	}{
		{"product info empty list", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryProductInfo(ctx, QueryProductInfoRequest{})
			return err
		}},
		{"product info blank code", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryProductInfo(ctx, QueryProductInfoRequest{StockCodes: []string{"  "}})
			return err
		}},
		{"product info code with space", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryProductInfo(ctx, QueryProductInfoRequest{StockCodes: []string{"HSI 2603"}})
			return err
		}},
		{"max buy sell empty code", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryMaxBuySellAmount(ctx, QueryMaxBuySellAmountRequest{})
			return err
		}},
		{"entrust empty code", func(ctx context.Context, m *Manager) error {
			_, err := m.Entrust(ctx, EntrustRequest{EntrustAmount: "1", EntrustBS: "1", EntrustPrice: "1"})
			return err
		}},
		{"entrust bad direction", func(ctx context.Context, m *Manager) error {
			_, err := m.Entrust(ctx, EntrustRequest{StockCode: "HSI", EntrustAmount: "1", EntrustBS: "9", EntrustPrice: "1"})
			return err
		}},
		{"entrust zero amount", func(ctx context.Context, m *Manager) error {
			_, err := m.Entrust(ctx, EntrustRequest{StockCode: "HSI", EntrustAmount: "0", EntrustBS: "1", EntrustPrice: "1"})
			return err
		}},
		{"entrust non-numeric amount", func(ctx context.Context, m *Manager) error {
			_, err := m.Entrust(ctx, EntrustRequest{StockCode: "HSI", EntrustAmount: "1e3", EntrustBS: "1", EntrustPrice: "1"})
			return err
		}},
		{"entrust limit without price", func(ctx context.Context, m *Manager) error {
			_, err := m.Entrust(ctx, EntrustRequest{StockCode: "HSI", EntrustType: "0", EntrustAmount: "1", EntrustBS: "1"})
			return err
		}},
		{"entrust specified date without time", func(ctx context.Context, m *Manager) error {
			_, err := m.Entrust(ctx, EntrustRequest{StockCode: "HSI", EntrustType: "0", EntrustAmount: "1", EntrustBS: "1", EntrustPrice: "1", ValidTimeType: "4"})
			return err
		}},
		{"cancel empty entrust id", func(ctx context.Context, m *Manager) error {
			_, err := m.CancelEntrust(ctx, CancelEntrustRequest{StockCode: "HSI"})
			return err
		}},
		{"modify bad price", func(ctx context.Context, m *Manager) error {
			_, err := m.ModifyEntrust(ctx, ModifyEntrustRequest{EntrustID: "E1", StockCode: "HSI", EntrustPrice: "abc", EntrustAmount: "1", EntrustBS: "1"})
			return err
		}},
		{"history page size too large", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryHistoryEntrustList(ctx, HistoryQueryRequest{PageSize: 100})
			return err
		}},
		{"history bad date", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryHistoryDeliverList(ctx, HistoryQueryRequest{StartDate: "2026-01-01"})
			return err
		}},
		{"history impossible date", func(ctx context.Context, m *Manager) error {
			_, err := m.QueryHistoryDeliverList(ctx, HistoryQueryRequest{EndDate: "20261301"})
			return err
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rec, srv := newTestServer(t, responses)
			m := newTestManager(t, srv.URL)

			err := tc.call(context.Background(), m)
			if err == nil {
				t.Fatal("error = nil, want typed invalid-parameter error")
			}
			code, ok := errs.CodeOf(err)
			if !ok || code != types.StatusInvalidParam {
				t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusInvalidParam)
			}
			if !strings.Contains(err.Error(), "trade/Futures") {
				t.Fatalf("error %q does not name the futures operation", err)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("outbound requests = %d, want 0", got)
			}
		})
	}
}
