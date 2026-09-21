// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package market

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind once every test,
// including the httptest servers, has run.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// recordedRequest is one captured Gateway call.
type recordedRequest struct {
	method      string
	path        string
	contentType string
	body        []byte
}

// gatewayRecorder is an httptest handler that records every request and serves
// a per-path canned response. Access is mutex-guarded so it is safe under
// -race.
type gatewayRecorder struct {
	mu        sync.Mutex
	reqs      []recordedRequest
	responses map[string]string
}

// ServeHTTP records the request and writes the canned response for its path.
func (g *gatewayRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	g.mu.Lock()
	g.reqs = append(g.reqs, recordedRequest{
		method:      r.Method,
		path:        r.URL.Path,
		contentType: r.Header.Get("Content-Type"),
		body:        body,
	})
	resp := g.responses[r.URL.Path]
	g.mu.Unlock()

	if resp == "" {
		resp = `{"ok":true,"err":"","data":null}`
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, resp)
}

// requests returns a copy of the recorded requests.
func (g *gatewayRecorder) requests() []recordedRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]recordedRequest, len(g.reqs))
	copy(out, g.reqs)
	return out
}

// newTestManager starts a recorder-backed Gateway and returns a Manager over it.
// The server and client are closed on test cleanup.
func newTestManager(t *testing.T, responses map[string]string) (*Manager, *gatewayRecorder) {
	t.Helper()
	rec := &gatewayRecorder{responses: responses}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return New(c), rec
}

// envelope is the decoded Gateway request envelope. TimeoutSec is a
// json.Number so a quoted value fails decoding, proving the field is numeric.
type envelope struct {
	TimeoutSec json.Number     `json:"timeout_sec"`
	Params     json.RawMessage `json:"params"`
}

// assertWire asserts a single recorded request matches the canonical path,
// method, content type, numeric timeout_sec, and exact params JSON.
func assertWire(t *testing.T, rec *gatewayRecorder, path, wantParams string) {
	t.Helper()
	reqs := rec.requests()
	if len(reqs) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(reqs))
	}
	got := reqs[0]
	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != path {
		t.Errorf("path = %q, want %q", got.path, path)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}

	var env envelope
	if err := json.Unmarshal(got.body, &env); err != nil {
		t.Fatalf("decode request envelope: %v (body=%s)", err, got.body)
	}
	if env.TimeoutSec.String() != "10" {
		t.Errorf("timeout_sec = %q, want numeric 10", env.TimeoutSec.String())
	}
	if string(env.Params) != wantParams {
		t.Errorf("params = %s, want %s", env.Params, wantParams)
	}
}

// TestManager_Endpoints exercises every market pull endpoint against a mock
// Gateway and asserts both the outbound wire form and the typed decoding.
func TestManager_Endpoints(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		wantParams string
		response   string
		call       func(context.Context, *Manager) (any, error)
		check      func(*testing.T, any)
	}{
		{
			name:       "BasicQot",
			path:       "/hq/BasicQot",
			wantParams: `{"security":[{"dataType":10000,"code":"0700.HK"}],"mktTmType":1}`,
			response: `{"ok":true,"err":"","data":{"basicQot":[{
				"security":{"dataType":10000,"code":"0700.HK"},
				"isSuspended":false,"openPrice":300.5,"highPrice":305.2,"lowPrice":299.8,
				"lastPrice":303.4,"lastClosePrice":301.0,"priceSpread":0.2,
				"volume":12345678,"turnover":3740000000.5,"turnoverRate":0.42,"amplitude":1.8,
				"secStatus":1,"listTime":"20040616","lotSize":100,
				"tradeTime":"2026-09-21 10:00:00","marketTime":"2026-09-21 10:00:00",
				"stockStatus":"1","volumeStr":"12345678","mktTmType":1,
				"futureExData":{"expiryDate":"2026-12-30","expiryDateDistance":100,
					"preSettle":"300.0","openInterest":"1000","preOpenInterest":"900",
					"dailyIncrement":"100","multiplier":"10","mainContractCode":"HSImain.HK"},
				"optionExData":{"strikePrice":"300","openInterest":"500","expireDate":"20260116",
					"premium":"5.5","intrinsicValue":"3.4","dueDate":"20260116","iv":"0.25",
					"timeValue":"2.1","actualLeverage":"15.2","delta":"0.5","gamma":"0.02",
					"vega":"0.1","theta":"-0.05","rho":"0.01"}
			}]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.BasicQot(ctx, BasicQotRequest{
					Security:  []*dto.Security{{DataType: 10000, Code: "0700.HK"}},
					MktTmType: 1,
				})
			},
			check: func(t *testing.T, got any) {
				resp, ok := got.(BasicQotResponse)
				if !ok {
					t.Fatalf("response type = %T, want BasicQotResponse", got)
				}
				if len(resp.BasicQot) != 1 {
					t.Fatalf("len(BasicQot) = %d, want 1", len(resp.BasicQot))
				}
				q := resp.BasicQot[0]
				if q.GetSecurity().GetCode() != "0700.HK" {
					t.Errorf("Security.Code = %q, want 0700.HK", q.GetSecurity().GetCode())
				}
				if q.GetVolume() != 12345678 {
					t.Errorf("Volume = %d, want 12345678", q.GetVolume())
				}
				if q.GetLastPrice() != 303.4 {
					t.Errorf("LastPrice = %v, want 303.4", q.GetLastPrice())
				}
				if q.GetFutureExData().GetMultiplier() != "10" {
					t.Errorf("FutureExData.Multiplier = %q, want 10", q.GetFutureExData().GetMultiplier())
				}
				if q.GetOptionExData().GetDelta() != "0.5" {
					t.Errorf("OptionExData.Delta = %q, want 0.5", q.GetOptionExData().GetDelta())
				}
			},
		},
		{
			name:       "OrderBook",
			path:       "/hq/OrderBook",
			wantParams: `{"security":{"dataType":10000,"code":"0700.HK"},"mktTmType":1,"depthBookType":3}`,
			response: `{"ok":true,"err":"","data":{
				"security":{"dataType":10000,"code":"0700.HK"},
				"orderBookAskList":[{"level":1,"price":303.6,"volume":1000,"orederCount":5,"brokeId":"HKEX"}],
				"orderBookBidList":[{"level":1,"price":303.4,"volume":2000,"orederCount":8,"brokeId":"HKEX"}],
				"spreadLevel":0.2}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.OrderBook(ctx, OrderBookRequest{
					Security:      &dto.Security{DataType: 10000, Code: "0700.HK"},
					MktTmType:     1,
					DepthBookType: 3,
				})
			},
			check: func(t *testing.T, got any) {
				resp := got.(OrderBookResponse)
				if resp.TickSize != 0.2 {
					t.Errorf("TickSize = %v, want 0.2", resp.TickSize)
				}
				if len(resp.OrderBookAskList) != 1 || len(resp.OrderBookBidList) != 1 {
					t.Fatalf("ask/bid lengths = %d/%d, want 1/1",
						len(resp.OrderBookAskList), len(resp.OrderBookBidList))
				}
				if resp.OrderBookAskList[0].GetVolume() != 1000 {
					t.Errorf("ask volume = %d, want 1000", resp.OrderBookAskList[0].GetVolume())
				}
				if resp.OrderBookBidList[0].GetLevel() != 1 {
					t.Errorf("bid level = %d, want 1", resp.OrderBookBidList[0].GetLevel())
				}
			},
		},
		{
			name:       "KL",
			path:       "/hq/KL",
			wantParams: `{"security":{"dataType":20000,"code":"AAPL"},"startDate":20260101,"direction":1,"exRightFlag":1,"cycType":5,"limit":100}`,
			response: `{"ok":true,"err":"","data":{
				"security":{"dataType":20000,"code":"AAPL"},
				"kline":[{"date":"2026-09-18","highPrice":230.5,"openPrice":228.0,
					"lowPrice":227.5,"closePrice":229.9,"lastClosePrice":228.2,
					"volume":55000000,"turnover":12600000000.5,
					"timestamp":1758163200,"time":"16:00"}]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.KL(ctx, KLRequest{
					Security:    &dto.Security{DataType: 20000, Code: "AAPL"},
					StartDate:   20260101,
					Direction:   1,
					ExRightFlag: 1,
					CycType:     5,
					Limit:       100,
				})
			},
			check: func(t *testing.T, got any) {
				resp := got.(KLResponse)
				if len(resp.Kline) != 1 {
					t.Fatalf("len(Kline) = %d, want 1", len(resp.Kline))
				}
				k := resp.Kline[0]
				if k.GetDate() != "2026-09-18" {
					t.Errorf("Date = %q, want 2026-09-18", k.GetDate())
				}
				if k.GetVolume() != 55000000 {
					t.Errorf("Volume = %d, want 55000000", k.GetVolume())
				}
				if k.GetTimestamp() != 1758163200 {
					t.Errorf("Timestamp = %d, want 1758163200", k.GetTimestamp())
				}
			},
		},
		{
			name:       "TimeShare",
			path:       "/hq/TimeShare",
			wantParams: `{"security":{"dataType":20000,"code":"AAPL"},"mktTmType":1}`,
			response: `{"ok":true,"err":"","data":{
				"security":{"dataType":20000,"code":"AAPL"},
				"timeShare":[{"time":"09:30","price":229.1,"lastClosePrice":228.2,
					"avgPrice":229.0,"volume":100000,"turnover":22900000.0}]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.TimeShare(ctx, TimeShareRequest{
					Security:  &dto.Security{DataType: 20000, Code: "AAPL"},
					MktTmType: 1,
				})
			},
			check: func(t *testing.T, got any) {
				resp := got.(TimeShareResponse)
				if len(resp.TimeShare) != 1 {
					t.Fatalf("len(TimeShare) = %d, want 1", len(resp.TimeShare))
				}
				ts := resp.TimeShare[0]
				if ts.GetPrice() != 229.1 {
					t.Errorf("Price = %v, want 229.1", ts.GetPrice())
				}
				if ts.GetVolume() != 100000 {
					t.Errorf("Volume = %d, want 100000", ts.GetVolume())
				}
			},
		},
		{
			name:       "Ticker",
			path:       "/hq/Ticker",
			wantParams: `{"security":{"dataType":20000,"code":"AAPL"},"limit":50,"mktTmType":1}`,
			response: `{"ok":true,"err":"","data":{
				"security":{"dataType":20000,"code":"AAPL"},
				"ticker":[{"time":"10:00:01","side":1,"price":229.5,"volume":300,
					"turnover":68850.0,"type":1,"timestamp":1758448801,"mktTmType":1}]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.Ticker(ctx, TickerRequest{
					Security:  &dto.Security{DataType: 20000, Code: "AAPL"},
					Limit:     50,
					MktTmType: 1,
				})
			},
			check: func(t *testing.T, got any) {
				resp := got.(TickerResponse)
				if len(resp.Ticker) != 1 {
					t.Fatalf("len(Ticker) = %d, want 1", len(resp.Ticker))
				}
				tk := resp.Ticker[0]
				if tk.GetSide() != 1 {
					t.Errorf("Side = %d, want 1", tk.GetSide())
				}
				if tk.GetVolume() != 300 {
					t.Errorf("Volume = %d, want 300", tk.GetVolume())
				}
				if tk.GetTimestamp() != 1758448801 {
					t.Errorf("Timestamp = %d, want 1758448801", tk.GetTimestamp())
				}
			},
		},
		{
			name:       "Broker",
			path:       "/hq/Broker",
			wantParams: `{"security":{"dataType":10000,"code":"0700.HK"}}`,
			response: `{"ok":true,"err":"","data":{
				"security":{"dataType":10000,"code":"0700.HK"},
				"brokerAskList":[{"level":1,"item":"B","type":66,"name":"Broker A"}],
				"brokerBidList":[{"level":1,"item":"S","type":83,"name":"Broker B"}]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.Broker(ctx, BrokerRequest{
					Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
				})
			},
			check: func(t *testing.T, got any) {
				resp := got.(BrokerResponse)
				if len(resp.BrokerAskList) != 1 || len(resp.BrokerBidList) != 1 {
					t.Fatalf("ask/bid lengths = %d/%d, want 1/1",
						len(resp.BrokerAskList), len(resp.BrokerBidList))
				}
				if resp.BrokerAskList[0].GetItem() != "B" {
					t.Errorf("ask item = %q, want B", resp.BrokerAskList[0].GetItem())
				}
				if resp.BrokerBidList[0].GetType() != 83 {
					t.Errorf("bid type = %d, want 83", resp.BrokerBidList[0].GetType())
				}
			},
		},
		{
			name:       "UsOptionChainCode",
			path:       "/hq/UsOptionChainCode",
			wantParams: `{"securityCode":"AAPL","expireDate":"2026/01/16","flagInOut":1,"optionType":"C"}`,
			response:   `{"ok":true,"err":"","data":{"optionCode":["AAPL260116C00200000","AAPL260116P00200000"]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.UsOptionChainCode(ctx, UsOptionChainCodeRequest{
					SecurityCode: "AAPL",
					ExpireDate:   "2026/01/16",
					FlagInOut:    1,
					OptionType:   "C",
				})
			},
			check: func(t *testing.T, got any) {
				resp := got.(UsOptionChainCodeResponse)
				if len(resp.OptionCode) != 2 {
					t.Fatalf("len(OptionCode) = %d, want 2", len(resp.OptionCode))
				}
				if resp.OptionCode[0] != "AAPL260116C00200000" {
					t.Errorf("OptionCode[0] = %q", resp.OptionCode[0])
				}
			},
		},
		{
			name:       "UsOptionChainExpireDate",
			path:       "/hq/UsOptionChainExpireDate",
			wantParams: `{"securityCode":"AAPL"}`,
			response:   `{"ok":true,"err":"","data":{"expireDate":["2026/01/16","2026/02/20"]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.UsOptionChainExpireDate(ctx, UsOptionChainExpireDateRequest{SecurityCode: "AAPL"})
			},
			check: func(t *testing.T, got any) {
				resp := got.(UsOptionChainExpireDateResponse)
				if len(resp.ExpireDate) != 2 {
					t.Fatalf("len(ExpireDate) = %d, want 2", len(resp.ExpireDate))
				}
				if resp.ExpireDate[1] != "2026/02/20" {
					t.Errorf("ExpireDate[1] = %q", resp.ExpireDate[1])
				}
			},
		},
		{
			name:       "UsOverNightTradeCodes",
			path:       "/hq/UsOverNightTradeCodes",
			wantParams: `{}`,
			response:   `{"ok":true,"err":"","data":{"securityCodes":["AAPL","TSLA","NVDA"]}}`,
			call: func(ctx context.Context, m *Manager) (any, error) {
				return m.UsOverNightTradeCodes(ctx, UsOverNightTradeCodesRequest{})
			},
			check: func(t *testing.T, got any) {
				resp := got.(UsOverNightTradeCodesResponse)
				if len(resp.SecurityCodes) != 3 {
					t.Fatalf("len(SecurityCodes) = %d, want 3", len(resp.SecurityCodes))
				}
				if resp.SecurityCodes[0] != "AAPL" {
					t.Errorf("SecurityCodes[0] = %q", resp.SecurityCodes[0])
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newTestManager(t, map[string]string{tc.path: tc.response})

			got, err := tc.call(context.Background(), m)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			assertWire(t, rec, tc.path, tc.wantParams)
			tc.check(t, got)
		})
	}
}

// TestTicker_RejectsLimitOutOfRange asserts the 1..MaxTickerLimit guard rejects
// out-of-range limits with a typed StatusInvalidParam error and sends no
// request.
func TestTicker_RejectsLimitOutOfRange(t *testing.T) {
	for _, limit := range []int32{-1, 0, MaxTickerLimit + 1, 1000} {
		t.Run("limit="+strconv.Itoa(int(limit)), func(t *testing.T) {
			m, rec := newTestManager(t, nil)

			_, err := m.Ticker(context.Background(), TickerRequest{
				Security: &dto.Security{DataType: 20000, Code: "AAPL"},
				Limit:    limit,
			})
			if err == nil {
				t.Fatalf("Ticker(limit=%d) = nil error, want typed rejection", limit)
			}
			code, ok := errs.CodeOf(err)
			if !ok || code != types.StatusInvalidParam {
				t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusInvalidParam)
			}
			if !strings.Contains(err.Error(), "limit") {
				t.Errorf("error %q does not mention the limit", err)
			}
			if n := len(rec.requests()); n != 0 {
				t.Fatalf("recorded %d requests, want 0", n)
			}
		})
	}
}

// TestBasicQot_RejectsEmptySecurity asserts an empty or nil security list is
// rejected with a typed StatusInvalidParam error and sends no request.
func TestBasicQot_RejectsEmptySecurity(t *testing.T) {
	cases := map[string][]*dto.Security{
		"nil":   nil,
		"empty": {},
		"nil element": {
			nil,
		},
	}
	for name, secs := range cases {
		t.Run(name, func(t *testing.T) {
			m, rec := newTestManager(t, nil)

			_, err := m.BasicQot(context.Background(), BasicQotRequest{Security: secs})
			if err == nil {
				t.Fatal("BasicQot = nil error, want typed rejection")
			}
			code, ok := errs.CodeOf(err)
			if !ok || code != types.StatusInvalidParam {
				t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusInvalidParam)
			}
			if n := len(rec.requests()); n != 0 {
				t.Fatalf("recorded %d requests, want 0", n)
			}
		})
	}
}

// TestSingleSecurity_RejectsNil asserts every single-security endpoint rejects a
// nil security locally.
func TestSingleSecurity_RejectsNil(t *testing.T) {
	calls := map[string]func(context.Context, *Manager) error{
		"OrderBook": func(ctx context.Context, m *Manager) error {
			_, err := m.OrderBook(ctx, OrderBookRequest{})
			return err
		},
		"KL": func(ctx context.Context, m *Manager) error {
			_, err := m.KL(ctx, KLRequest{})
			return err
		},
		"TimeShare": func(ctx context.Context, m *Manager) error {
			_, err := m.TimeShare(ctx, TimeShareRequest{})
			return err
		},
		"Ticker": func(ctx context.Context, m *Manager) error {
			_, err := m.Ticker(ctx, TickerRequest{Limit: 10})
			return err
		},
		"Broker": func(ctx context.Context, m *Manager) error {
			_, err := m.Broker(ctx, BrokerRequest{})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			m, rec := newTestManager(t, nil)
			err := call(context.Background(), m)
			if err == nil {
				t.Fatalf("%s = nil error, want typed rejection", name)
			}
			code, ok := errs.CodeOf(err)
			if !ok || code != types.StatusInvalidParam {
				t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusInvalidParam)
			}
			if n := len(rec.requests()); n != 0 {
				t.Fatalf("recorded %d requests, want 0", n)
			}
		})
	}
}

// TestOptionChain_RejectsEmptySecurityCode asserts both option-chain endpoints
// reject an empty underlying code locally.
func TestOptionChain_RejectsEmptySecurityCode(t *testing.T) {
	m, rec := newTestManager(t, nil)
	ctx := context.Background()

	if _, err := m.UsOptionChainCode(ctx, UsOptionChainCodeRequest{ExpireDate: "2026/01/16"}); err == nil {
		t.Fatal("UsOptionChainCode = nil error, want typed rejection")
	}
	if _, err := m.UsOptionChainExpireDate(ctx, UsOptionChainExpireDateRequest{}); err == nil {
		t.Fatal("UsOptionChainExpireDate = nil error, want typed rejection")
	}
	if n := len(rec.requests()); n != 0 {
		t.Fatalf("recorded %d requests, want 0", n)
	}
}
