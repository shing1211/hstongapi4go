// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/hstong/stream"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Environment variable names read by this suite. HSTONG_GATEWAY_URL,
// HSTONG_PUSH_ADDR, HSTONG_VERIFY_PUSH, and HSTONG_TRADE_PASSWORD are read by
// client.WithEnv through the client.Env* constants; envTradePassword is declared
// here only so the trade-only tests can gate on it without duplicating the
// client constant.
const (
	// envGate must be "1" for any test here to run.
	envGate = "HSTONG_INTEGRATION"
	// envTradePassword supplies the plaintext trade password; required for the
	// session and trade tests (client.EnvTradePassword).
	envTradePassword = client.EnvTradePassword
	// envTestSymbol selects the instrument under test (default 00700.HK).
	envTestSymbol = "HSTONG_TEST_SYMBOL"
	// envTestExchange selects the market (default K, Hong Kong).
	envTestExchange = "HSTONG_TEST_EXCHANGE"
	// envPlaceOrders must be "1" to enable the order mutation test.
	envPlaceOrders = "HSTONG_PLACE_ORDERS"

	// defaultSymbol is the instrument used when HSTONG_TEST_SYMBOL is unset.
	defaultSymbol = "00700.HK"
	// defaultExchange is the market used when HSTONG_TEST_EXCHANGE is unset.
	defaultExchange = "K"
	// probeTimeout bounds one read-only HTTP probe or login.
	probeTimeout = 30 * time.Second
	// streamTimeout is how long the streaming test waits for one push Event.
	streamTimeout = 10 * time.Second
	// orderTimeout bounds the opt-in order mutation and its cancel.
	orderTimeout = 30 * time.Second
)

// errHKOnly marks a case that only applies to the Hong Kong market, so the
// caller can skip it instead of failing when another exchange is configured.
var errHKOnly = errors.New("endpoint supports the Hong Kong market only")

// TestMain verifies that the suite leaves no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// requireIntegration skips the calling test unless HSTONG_INTEGRATION=1.
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv(envGate) != "1" {
		t.Skipf("integration test skipped: set %s=1 and run a local Gateway (see test/integration/README.md)", envGate)
	}
}

// requireTradePassword skips the calling test when HSTONG_TRADE_PASSWORD is
// unset, because the session and trade surfaces cannot be exercised without it.
func requireTradePassword(t *testing.T) {
	t.Helper()
	if os.Getenv(envTradePassword) == "" {
		t.Skipf("integration test skipped: %s is required for session and trade calls", envTradePassword)
	}
}

// newClient builds a Client from the HSTONG_* environment. Unset variables keep
// the SDK defaults, which already match the documented local Gateway addresses.
func newClient(t *testing.T) *client.Client {
	t.Helper()
	c, err := client.New(client.WithEnv())
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// envOr returns the value of key, or def when key is unset or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// testSymbol returns HSTONG_TEST_SYMBOL or its default.
func testSymbol() string { return envOr(envTestSymbol, defaultSymbol) }

// testExchange returns HSTONG_TEST_EXCHANGE or its default.
func testExchange() types.ExchangeType {
	return types.ExchangeType(envOr(envTestExchange, defaultExchange))
}

// testDataType maps the configured exchange to the instrument data type used in
// a dto.Security.
func testDataType() int32 {
	switch testExchange() {
	case types.ExchangeUS:
		return int32(types.DataTypeUSStock)
	default:
		return int32(types.DataTypeHKStock)
	}
}

// testSecurity returns the configured instrument as a dto.Security.
func testSecurity() *dto.Security {
	return &dto.Security{DataType: testDataType(), Code: testSymbol()}
}

// probeContext returns a context bounded by probeTimeout.
func probeContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), probeTimeout)
}

// doRaw issues one Gateway call with the JSON codec and returns the raw data
// payload so a test can inspect the exact wire representation. It never retries.
func doRaw(ctx context.Context, c *client.Client, route client.Route, params any) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.Do(ctx, route.Path(), route, params, c.JSON(), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// jsonKind classifies the top-level JSON token of raw as "number", "string",
// "object", "array", "boolean", "null", or "empty".
func jsonKind(raw json.RawMessage) string {
	b := bytes.TrimSpace(raw)
	if len(b) == 0 {
		return "empty"
	}
	switch b[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// isEntitlementError reports whether err looks like a missing-entitlement
// rejection, so a market test can skip rather than fail on an account that is
// not subscribed to a data package.
func isEntitlementError(err error) bool {
	if err == nil {
		return false
	}
	if code, ok := errs.CodeOf(err); ok && code == types.StatusUserNotAuthorized {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"not authorized", "unauthor", "no permission", "permission denied",
		"entitle", "授权",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// skipIfEntitlement skips t when err is an entitlement rejection and otherwise
// fails the test when err is non-nil.
func skipIfEntitlement(t *testing.T, what string, err error) {
	t.Helper()
	if isEntitlementError(err) {
		t.Skipf("%s: account is not entitled to this data (%v)", what, err)
	}
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// hasAnyKey reports whether m contains at least one of keys.
func hasAnyKey(m map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// truncate returns a bounded, single-line rendering of raw for a log or error.
func truncate(raw []byte) string {
	const maxLen = 200
	s := strings.TrimSpace(string(raw))
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// fetchRows fetches route, asserts the response is a JSON object with a
// container array, and returns the array's element objects. It skips the test
// when the account is not entitled, when the container is absent, or when the
// array is empty, because none of those lets the caller observe the wire shape.
func fetchRows(t *testing.T, ctx context.Context, c *client.Client, route client.Route, params any, container string) []map[string]json.RawMessage {
	t.Helper()
	raw, err := doRaw(ctx, c, route, params)
	skipIfEntitlement(t, string(route), err)
	if err != nil {
		t.Fatalf("%s: %v", route, err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("%s: data is not a JSON object: %v (payload: %s)", route, err, truncate(raw))
	}
	rowsRaw, ok := obj[container]
	if !ok {
		t.Skipf("%s: data has no %q array (payload: %s)", route, container, truncate(raw))
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(rowsRaw, &rows); err != nil {
		t.Fatalf("%s: %q is not an array of objects: %v", route, container, err)
	}
	if len(rows) == 0 {
		t.Skipf("%s: %q array is empty; no row to probe", route, container)
	}
	return rows
}

// assertKind logs the observed representation of row[field] and fails when it
// is not want. A missing field is informational only: the Gateway may omit
// zero-valued fields.
func assertKind(t *testing.T, route client.Route, container string, row map[string]json.RawMessage, field, want string) {
	t.Helper()
	raw, ok := row[field]
	if !ok {
		t.Logf("wire probe: %s %s[0].%s absent; representation not observed", route, container, field)
		return
	}
	kind := jsonKind(raw)
	t.Logf("wire probe: %s %s[0].%s observed as JSON %s (%s)", route, container, field, kind, truncate(raw))
	if kind == want {
		return
	}
	t.Errorf("wire/codec mismatch on %s field %s[0].%s: observed a JSON %s (%s), want a JSON %s. "+
		"pkg/hstong/market/market.go decodes this payload with encoding/json (docs/adr/0007-http-json-codec.md); "+
		"if the Gateway quotes an int64, correct that ADR and update the wrapper field type.",
		route, container, field, kind, truncate(raw), want)
}

// TestIntegration_Session logs in and issues one authenticated read.
func TestIntegration_Session(t *testing.T) {
	requireIntegration(t)
	requireTradePassword(t)

	c := newClient(t)
	sess := hstong.NewSessionManager(c)
	trd := trade.New(c, trade.WithSession(sess))

	ctx, cancel := probeContext(t)
	defer cancel()

	if err := sess.Login(ctx); err != nil {
		t.Fatalf("sess.Login: %v", err)
	}
	if !sess.IsLoggedIn() {
		t.Fatal("sess.IsLoggedIn() = false after a successful Login")
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), probeTimeout)
		defer ccancel()
		if err := sess.Logout(cctx); err != nil {
			t.Logf("sess.Logout: %v", err)
		}
	})

	fund, err := trd.MarginFundInfo(ctx, trade.MarginFundInfoRequest{ExchangeType: testExchange()})
	skipIfEntitlement(t, "TradeQueryMarginFundInfo after Login", err)
	t.Logf("session OK: assetBalance=%s enableBalance=%s", fund.AssetBalance, fund.EnableBalance)
}

// TestIntegration_WireCodecProbe confirms the int64 representation the real
// Gateway sends for market pulls. ADR 0007 assumes a JSON number because
// pkg/hstong/market decodes with encoding/json over int64 DTO fields; a quoted
// string would make the SDK fail this test with the offending field named.
func TestIntegration_WireCodecProbe(t *testing.T) {
	requireIntegration(t)

	c := newClient(t)
	mkt := market.New(c)
	ctx, cancel := probeContext(t)
	defer cancel()

	t.Run("BasicQot volume int64", func(t *testing.T) {
		req := market.BasicQotRequest{Security: []*dto.Security{testSecurity()}, MktTmType: 1}
		rows := fetchRows(t, ctx, c, client.RouteHqBasicQot, req, "basicQot")
		assertKind(t, client.RouteHqBasicQot, "basicQot", rows[0], "volume", "number")
		assertKind(t, client.RouteHqBasicQot, "basicQot", rows[0], "volumeStr", "string")

		r, err := mkt.BasicQot(ctx, req)
		if err != nil {
			t.Fatalf("typed BasicQot decode: %v", err)
		}
		t.Logf("typed decode OK: basicQot rows=%d volume=%d volumeStr=%q",
			len(r.BasicQot), r.BasicQot[0].GetVolume(), r.BasicQot[0].GetVolumeStr())
	})

	t.Run("Ticker volume/timestamp int64", func(t *testing.T) {
		req := market.TickerRequest{Security: testSecurity(), Limit: 10}
		rows := fetchRows(t, ctx, c, client.RouteHqTicker, req, "ticker")
		assertKind(t, client.RouteHqTicker, "ticker", rows[0], "volume", "number")
		assertKind(t, client.RouteHqTicker, "ticker", rows[0], "timestamp", "number")

		r, err := mkt.Ticker(ctx, req)
		if err != nil {
			t.Fatalf("typed Ticker decode: %v", err)
		}
		t.Logf("typed decode OK: ticker rows=%d volume=%d timestamp=%d",
			len(r.Ticker), r.Ticker[0].GetVolume(), r.Ticker[0].GetTimestamp())
	})
}

// TestIntegration_EnvelopeNesting settles the two envelope-shape questions P09
// flagged: whether /trade/TradeQueryMaxAvailableAsset nests the asset at
// data.data (the SDK's expectation) or returns it directly at data, and which
// of the three tolerated shapes /trade/TradeQueryHoldsList actually uses.
func TestIntegration_EnvelopeNesting(t *testing.T) {
	requireIntegration(t)
	requireTradePassword(t)

	c := newClient(t)
	sess := hstong.NewSessionManager(c)
	ctx, cancel := probeContext(t)
	defer cancel()

	if err := sess.Login(ctx); err != nil {
		t.Fatalf("sess.Login: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), probeTimeout)
		defer ccancel()
		if err := sess.Logout(cctx); err != nil {
			t.Logf("sess.Logout: %v", err)
		}
	})

	t.Run("MaxAvailableAsset data.data", func(t *testing.T) {
		req := trade.MaxAvailableAssetRequest{
			ExchangeType: testExchange(),
			StockCode:    testSymbol(),
			EntrustPrice: "1.00",
			EntrustType:  types.EntrustTypeLimit,
		}
		raw, err := doRaw(ctx, c, client.RouteTradeQueryMaxAvailableAsset, req)
		skipIfEntitlement(t, string(client.RouteTradeQueryMaxAvailableAsset), err)
		if err != nil {
			t.Fatalf("TradeQueryMaxAvailableAsset: %v", err)
		}

		var top map[string]json.RawMessage
		if err := json.Unmarshal(raw, &top); err != nil {
			t.Fatalf("data is not a JSON object: %v (payload: %s)", err, truncate(raw))
		}
		if sub, ok := top["data"]; ok && jsonKind(sub) == "object" {
			t.Logf("envelope OK: TradeQueryMaxAvailableAsset nests the asset at data.data, matching pkg/hstong/trade/orders.go")
			return
		}
		if hasAnyKey(top, "positionStatus", "longOpenAvailable", "cashAvailableAmount", "shortOpenPool") {
			t.Errorf("envelope disagreement: TradeQueryMaxAvailableAsset returns the asset object directly under data, "+
				"but pkg/hstong/trade/orders.go maxAvailableAssetResponse expects data.data, so the SDK silently decodes zeros. "+
				"Fix that struct (or the ADR) to match the observed shape: %s", truncate(raw))
			return
		}
		t.Skipf("cannot classify the TradeQueryMaxAvailableAsset data shape: %s", truncate(raw))
	})

	t.Run("HoldsList shape", func(t *testing.T) {
		req := trade.PositionsRequest{ExchangeType: testExchange()}
		raw, err := doRaw(ctx, c, client.RouteTradeQueryHoldsList, req)
		skipIfEntitlement(t, string(client.RouteTradeQueryHoldsList), err)
		if err != nil {
			t.Fatalf("TradeQueryHoldsList: %v", err)
		}

		switch jsonKind(raw) {
		case "array":
			t.Logf("HoldsList shape: bare array; the SDK tolerates it (pkg/hstong/trade/assets.go HoldsListResponse.UnmarshalJSON)")
		case "object":
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatalf("HoldsList data is not a JSON object: %v (payload: %s)", err, truncate(raw))
			}
			if _, ok := obj["holdsList"]; ok {
				t.Logf("HoldsList shape: wrapped {\"holdsList\":[...]}; the SDK tolerates it")
				return
			}
			if hasAnyKey(obj, "stockCode", "stockName", "currentAmount", "enableAmount") {
				t.Logf("HoldsList shape: single inline row (the reference documentation's shape); the SDK tolerates it")
				return
			}
			t.Errorf("unknown HoldsList shape: neither wrapped nor an inline row; update pkg/hstong/trade/assets.go HoldsListResponse.UnmarshalJSON: %s", truncate(raw))
		default:
			t.Skipf("cannot classify the HoldsList data shape (%s): %s", jsonKind(raw), truncate(raw))
		}
	})
}

// TestIntegration_MarketPull smokes the nine market pull endpoints. Endpoints
// the account is not entitled to are reported as skips, not failures.
func TestIntegration_MarketPull(t *testing.T) {
	requireIntegration(t)

	c := newClient(t)
	mkt := market.New(c)
	ctx, cancel := probeContext(t)
	defer cancel()

	ex := testExchange()

	cases := []struct {
		name string
		call func() error
	}{
		{"BasicQot", func() error {
			_, err := mkt.BasicQot(ctx, market.BasicQotRequest{Security: []*dto.Security{testSecurity()}, MktTmType: 1})
			return err
		}},
		{"OrderBook", func() error {
			_, err := mkt.OrderBook(ctx, market.OrderBookRequest{Security: testSecurity()})
			return err
		}},
		{"KL", func() error {
			_, err := mkt.KL(ctx, market.KLRequest{
				Security: testSecurity(), StartDate: 20250101, Direction: 0,
				ExRightFlag: 0, CycType: 2, Limit: 10,
			})
			return err
		}},
		{"TimeShare", func() error {
			_, err := mkt.TimeShare(ctx, market.TimeShareRequest{Security: testSecurity(), MktTmType: 1})
			return err
		}},
		{"Ticker", func() error {
			_, err := mkt.Ticker(ctx, market.TickerRequest{Security: testSecurity(), Limit: 10})
			return err
		}},
		{"Broker", func() error {
			if ex != types.ExchangeHK {
				return errHKOnly
			}
			_, err := mkt.Broker(ctx, market.BrokerRequest{Security: testSecurity()})
			return err
		}},
		{"UsOptionChainCode", func() error {
			_, err := mkt.UsOptionChainCode(ctx, market.UsOptionChainCodeRequest{
				SecurityCode: "AAPL", ExpireDate: "2026/12/18",
			})
			return err
		}},
		{"UsOptionChainExpireDate", func() error {
			_, err := mkt.UsOptionChainExpireDate(ctx, market.UsOptionChainExpireDateRequest{SecurityCode: "AAPL"})
			return err
		}},
		{"UsOverNightTradeCodes", func() error {
			_, err := mkt.UsOverNightTradeCodes(ctx, market.UsOverNightTradeCodesRequest{})
			return err
		}},
	}
	if len(cases) != 9 {
		t.Fatalf("market pull table = %d, want 9 (docs/SPEC.md 2.1)", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if errors.Is(err, errHKOnly) {
				t.Skipf("%s: configured exchange %q is not Hong Kong", tc.name, ex)
			}
			if isEntitlementError(err) {
				t.Skipf("%s: account is not entitled to this data (%v)", tc.name, err)
			}
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			t.Logf("%s OK for %s", tc.name, testSymbol())
		})
	}
}

// TestIntegration_Streaming connects the push channel, subscribes a quote topic
// and waits for at least one decoded Event before cancelling.
func TestIntegration_Streaming(t *testing.T) {
	requireIntegration(t)

	c := newClient(t)
	s := stream.New(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Connect(ctx); err != nil {
		t.Fatalf("stream.Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	sub, err := s.Subscribe(ctx, types.TopicBasicQot, testSecurity())
	if isEntitlementError(err) {
		t.Skipf("streaming: quote subscription is not entitled (%v)", err)
	}
	if err != nil {
		t.Fatalf("stream.Subscribe: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), probeTimeout)
		defer ccancel()
		if err := sub.Cancel(cctx); err != nil {
			t.Logf("subscription cancel: %v", err)
		}
	})

	select {
	case ev, ok := <-sub.Updates():
		if !ok {
			t.Fatal("quote Updates channel closed before an event arrived")
		}
		t.Logf("streaming OK: event type=%s id=%s payload=%T", ev.Type, ev.ID, ev.Payload)
	case err, ok := <-sub.Errors():
		if ok && err != nil {
			t.Fatalf("streaming: subscription error before an event: %v", err)
		}
		t.Fatal("quote Errors channel closed before an event arrived")
	case <-time.After(streamTimeout):
		t.Fatalf("streaming: no quote Event within %s for %s (topic %d). "+
			"The test environment is available Mon-Fri 09:00-18:00; a closed market or a missing quote entitlement will time out.",
			streamTimeout, testSymbol(), int(types.TopicBasicQot))
	}
}

// TestIntegration_TradeReads issues funds and positions reads (no mutations).
func TestIntegration_TradeReads(t *testing.T) {
	requireIntegration(t)
	requireTradePassword(t)

	c := newClient(t)
	sess := hstong.NewSessionManager(c)
	trd := trade.New(c, trade.WithSession(sess))
	ctx, cancel := probeContext(t)
	defer cancel()

	if err := sess.Login(ctx); err != nil {
		t.Fatalf("sess.Login: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), probeTimeout)
		defer ccancel()
		if err := sess.Logout(cctx); err != nil {
			t.Logf("sess.Logout: %v", err)
		}
	})

	t.Run("funds", func(t *testing.T) {
		f, err := trd.MarginFundInfo(ctx, trade.MarginFundInfoRequest{ExchangeType: testExchange()})
		skipIfEntitlement(t, "MarginFundInfo", err)
		t.Logf("funds OK: assetBalance=%s enableBalance=%s", f.AssetBalance, f.EnableBalance)
	})

	t.Run("positions", func(t *testing.T) {
		rows, err := trd.Positions(ctx, trade.PositionsRequest{ExchangeType: testExchange()})
		skipIfEntitlement(t, "Positions", err)
		t.Logf("positions OK: %d row(s)", len(rows))
	})
}

// TestIntegration_PlaceAndCancelOrder places one far-from-market limit order,
// asserts exactly one outbound TradeEntrust attempt and a non-empty entrust id,
// then cancels it. It is opt-in through HSTONG_PLACE_ORDERS=1 for safety, and
// the cancel is always attempted from cleanup even when the test fails first.
func TestIntegration_PlaceAndCancelOrder(t *testing.T) {
	requireIntegration(t)
	requireTradePassword(t)
	if os.Getenv(envPlaceOrders) != "1" {
		t.Skipf("order mutation test skipped: set %s=1 to place and cancel a real far-from-market limit order", envPlaceOrders)
	}

	counter := newCountingTransport()
	c, err := client.New(client.WithEnv(), client.WithHTTPClient(&http.Client{Transport: counter}))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	sess := hstong.NewSessionManager(c)
	trd := trade.New(c, trade.WithSession(sess))
	ctx, cancel := probeContext(t)
	defer cancel()

	if err := sess.Login(ctx); err != nil {
		t.Fatalf("sess.Login: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), probeTimeout)
		defer ccancel()
		if err := sess.Logout(cctx); err != nil {
			t.Logf("sess.Logout: %v", err)
		}
	})

	price, ok := farFromMarketBuyPrice(ctx, c)
	if !ok {
		t.Skipf("cannot determine a far-from-market price for %s (no quote); run during the Mon-Fri 09:00-18:00 test window", testSymbol())
	}

	req := trade.EntrustRequest{
		ExchangeType:  testExchange(),
		StockCode:     testSymbol(),
		EntrustAmount: "100",
		EntrustPrice:  price,
		EntrustBS:     types.EntrustBuy,
		EntrustType:   types.EntrustTypeLimit,
	}

	var placedID string
	t.Cleanup(func() {
		if placedID == "" {
			return
		}
		cctx, ccancel := context.WithTimeout(context.Background(), orderTimeout)
		defer ccancel()
		if _, cerr := trd.CancelEntrust(cctx, trade.CancelEntrustRequest{
			ExchangeType:  testExchange(),
			StockCode:     testSymbol(),
			EntrustAmount: req.EntrustAmount,
			EntrustPrice:  req.EntrustPrice,
			EntrustID:     placedID,
		}); cerr != nil {
			t.Errorf("cleanup: cancel of %s failed: %v", placedID, cerr)
		}
	})

	id, err := trd.Entrust(ctx, req)
	if err != nil {
		t.Fatalf("TradeEntrust: %v", err)
	}
	placedID = id
	if id == "" {
		t.Fatal("TradeEntrust returned an empty entrust id")
	}
	if got := counter.EntrustCalls(); got != 1 {
		t.Errorf("TradeEntrust issued %d outbound HTTP attempts, want exactly 1 (docs/adr/0003-no-auto-retry-orders.md)", got)
	}
	t.Logf("placed far-from-market buy %s x%s @ %s -> entrustId=%s (attempts=1)", testSymbol(), req.EntrustAmount, req.EntrustPrice, id)

	cid, cerr := trd.CancelEntrust(ctx, trade.CancelEntrustRequest{
		ExchangeType:  testExchange(),
		StockCode:     testSymbol(),
		EntrustAmount: req.EntrustAmount,
		EntrustPrice:  req.EntrustPrice,
		EntrustID:     id,
	})
	if cerr != nil {
		t.Fatalf("TradeCancelEntrust(%s): %v", id, cerr)
	}
	t.Logf("cancelled entrustId=%s -> cancelId=%s", id, cid)
	placedID = "" // the explicit cancel succeeded; cleanup is now a no-op
}

// farFromMarketBuyPrice quotes the configured symbol and returns a buy limit
// price at half the last price (rounded down to two decimals), which will not
// fill. It reports false when no usable quote is available.
func farFromMarketBuyPrice(ctx context.Context, c *client.Client) (string, bool) {
	mkt := market.New(c)
	r, err := mkt.BasicQot(ctx, market.BasicQotRequest{Security: []*dto.Security{testSecurity()}, MktTmType: 1})
	if err != nil || len(r.BasicQot) == 0 {
		return "", false
	}
	last := r.BasicQot[0].GetLastPrice()
	if last <= 0 {
		return "", false
	}
	p := math.Floor(last*0.5*100) / 100
	if p <= 0 {
		return "", false
	}
	return strconv.FormatFloat(p, 'f', 2, 64), true
}

// countingTransport counts outbound TradeEntrust attempts so the mutation test
// can assert exactly one was made.
type countingTransport struct {
	base    http.RoundTripper
	entrust atomic.Int32
}

// newCountingTransport returns a transport over http.DefaultTransport.
func newCountingTransport() *countingTransport {
	return &countingTransport{base: http.DefaultTransport}
}

// RoundTrip counts TradeEntrust requests and delegates to the base transport.
func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == string(client.RouteTradeEntrust) {
		c.entrust.Add(1)
	}
	return c.base.RoundTrip(req)
}

// CloseIdleConnections releases idle connections so the client's Close leaves
// no sockets behind.
func (c *countingTransport) CloseIdleConnections() {
	if closer, ok := c.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// EntrustCalls returns the number of TradeEntrust attempts observed.
func (c *countingTransport) EntrustCalls() int32 { return c.entrust.Load() }
