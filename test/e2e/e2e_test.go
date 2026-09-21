// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/algo"
	"github.com/shing1211/hstongapi4go/pkg/hstong/future"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/hstong/stream"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
	"github.com/shing1211/hstongapi4go/pkg/types"
	"github.com/shing1211/hstongapi4go/test/mockgateway"
)

// TestMain verifies the suite leaves no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// endpoint is one row of the 51-endpoint e2e table. call drives one typed
// manager method and returns an error when the call or its result assertion
// fails.
type endpoint struct {
	name string
	call func(context.Context) error
}

// check returns an error when cond is false. It keeps the table closures
// declarative.
func check(cond bool, format string, args ...any) error {
	if cond {
		return nil
	}
	return fmt.Errorf(format, args...)
}

// hkSecurity returns a Hong Kong security fixture.
func hkSecurity() *dto.Security {
	return &dto.Security{DataType: int32(types.DataTypeHKStock), Code: "00700.HK"}
}

// newMockClient starts a mock and returns a client pointed at it. Both are
// released at test cleanup.
func newMockClient(t *testing.T) (*mockgateway.Server, *client.Client) {
	t.Helper()
	srv := mockgateway.New(mockgateway.WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	cli, err := client.New(
		client.WithBaseURL(srv.HTTPBaseURL()),
		client.WithPushAddr(srv.PushAddr()),
		client.WithTradePassword("123456"),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return srv, cli
}

// TestE2E_AllEndpoints calls all 51 endpoints through their SDK manager methods.
func TestE2E_AllEndpoints(t *testing.T) {
	srv, cli := newMockClient(t)
	_ = srv

	mkt := market.New(cli)
	trd := trade.New(cli)
	fut := future.New(cli)
	alg := algo.New(cli)
	sess := hstong.NewSessionManager(cli)

	ctx := context.Background()
	if err := sess.Login(ctx); err != nil {
		t.Fatalf("priming login: %v", err)
	}

	cases := []endpoint{
		// --- Market pull (9) --------------------------------------------
		{"POST /hq/BasicQot", func(ctx context.Context) error {
			r, err := mkt.BasicQot(ctx, market.BasicQotRequest{Security: []*dto.Security{hkSecurity()}, MktTmType: 1})
			if err != nil {
				return err
			}
			return check(len(r.BasicQot) == 1 && r.BasicQot[0].GetLastPrice() == 388.0,
				"BasicQot = %+v", r.BasicQot)
		}},
		{"POST /hq/OrderBook", func(ctx context.Context) error {
			r, err := mkt.OrderBook(ctx, market.OrderBookRequest{Security: hkSecurity()})
			if err != nil {
				return err
			}
			return check(len(r.OrderBookBidList) > 0 && len(r.OrderBookAskList) > 0,
				"OrderBook bid=%d ask=%d", len(r.OrderBookBidList), len(r.OrderBookAskList))
		}},
		{"POST /hq/KL", func(ctx context.Context) error {
			r, err := mkt.KL(ctx, market.KLRequest{Security: hkSecurity(), StartDate: 20260101, CycType: 2, Limit: 10})
			if err != nil {
				return err
			}
			return check(len(r.Kline) > 0, "KL kline is empty")
		}},
		{"POST /hq/TimeShare", func(ctx context.Context) error {
			r, err := mkt.TimeShare(ctx, market.TimeShareRequest{Security: hkSecurity()})
			if err != nil {
				return err
			}
			return check(len(r.TimeShare) > 0, "TimeShare is empty")
		}},
		{"POST /hq/Ticker", func(ctx context.Context) error {
			r, err := mkt.Ticker(ctx, market.TickerRequest{Security: hkSecurity(), Limit: 10})
			if err != nil {
				return err
			}
			return check(len(r.Ticker) > 0, "Ticker is empty")
		}},
		{"POST /hq/Broker", func(ctx context.Context) error {
			r, err := mkt.Broker(ctx, market.BrokerRequest{Security: hkSecurity()})
			if err != nil {
				return err
			}
			return check(len(r.BrokerBidList) > 0, "Broker is empty")
		}},
		{"POST /hq/UsOptionChainCode", func(ctx context.Context) error {
			r, err := mkt.UsOptionChainCode(ctx, market.UsOptionChainCodeRequest{SecurityCode: "AAPL", ExpireDate: "2026/10/16"})
			if err != nil {
				return err
			}
			return check(len(r.OptionCode) > 0, "UsOptionChainCode is empty")
		}},
		{"POST /hq/UsOptionChainExpireDate", func(ctx context.Context) error {
			r, err := mkt.UsOptionChainExpireDate(ctx, market.UsOptionChainExpireDateRequest{SecurityCode: "AAPL"})
			if err != nil {
				return err
			}
			return check(len(r.ExpireDate) > 0, "UsOptionChainExpireDate is empty")
		}},
		{"POST /hq/UsOverNightTradeCodes", func(ctx context.Context) error {
			r, err := mkt.UsOverNightTradeCodes(ctx, market.UsOverNightTradeCodesRequest{})
			if err != nil {
				return err
			}
			return check(len(r.SecurityCodes) > 0, "UsOverNightTradeCodes is empty")
		}},

		// --- Market subscription (2) ------------------------------------
		{"POST /hq/Subscribe", func(ctx context.Context) error {
			return mkt.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
		}},
		{"POST /hq/Unsubscribe", func(ctx context.Context) error {
			return mkt.Unsubscribe(ctx, types.TopicBasicQot, hkSecurity())
		}},

		// --- Trade session (2) ------------------------------------------
		{"POST /trade/TradeLogin", func(ctx context.Context) error {
			return sess.Login(ctx)
		}},
		{"POST /trade/TradeLogout", func(ctx context.Context) error {
			return sess.Logout(ctx)
		}},

		// --- Trade assets / positions (5) -------------------------------
		{"POST /trade/TradeQueryMarginFundInfo", func(ctx context.Context) error {
			r, err := trd.MarginFundInfo(ctx, trade.MarginFundInfoRequest{ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(r.AssetBalance == "1000000.00", "AssetBalance = %q", r.AssetBalance)
		}},
		{"POST /trade/TradeQueryHoldsList", func(ctx context.Context) error {
			r, err := trd.Positions(ctx, trade.PositionsRequest{ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(len(r) > 0, "Positions is empty")
		}},
		{"POST /trade/TradeQueryRealFundJourList", func(ctx context.Context) error {
			r, err := trd.RealFundJourList(ctx, trade.FundJourListRequest{ExchangeType: types.ExchangeHK, QueryCount: 20, QueryParamStr: "0"})
			if err != nil {
				return err
			}
			return check(len(r) == 2, "RealFundJourList rows = %d, want 2", len(r))
		}},
		{"POST /trade/TradeQueryHistoryFundJourList", func(ctx context.Context) error {
			r, err := trd.HistoryFundJourList(ctx, trade.HistoryFundJourListRequest{
				ExchangeType: types.ExchangeHK, QueryCount: 20, QueryParamStr: "0",
				StartDate: "20260901", EndDate: "20260921",
			})
			if err != nil {
				return err
			}
			return check(len(r) == 2, "HistoryFundJourList rows = %d, want 2", len(r))
		}},
		{"POST /hs/rate/queryList", func(ctx context.Context) error {
			r, err := trd.ExchangeRate(ctx, trade.ExchangeRateRequest{RateType: "0"})
			if err != nil {
				return err
			}
			return check(r["HKD"]["USD"] == "0.1279", "HKD/USD = %q", r["HKD"]["USD"])
		}},

		// --- Trade orders (13) ------------------------------------------
		{"POST /trade/TradeEntrust", func(ctx context.Context) error {
			id, err := trd.Entrust(ctx, trade.EntrustRequest{
				ExchangeType: types.ExchangeHK, StockCode: "00700.HK", EntrustAmount: "100",
				EntrustPrice: "387.00", EntrustBS: types.EntrustBuy, EntrustType: types.EntrustTypeLimit,
			})
			if err != nil {
				return err
			}
			return check(id == "E20260921001", "entrust id = %q", id)
		}},
		{"POST /trade/TradeCancelEntrust", func(ctx context.Context) error {
			id, err := trd.CancelEntrust(ctx, trade.CancelEntrustRequest{
				ExchangeType: types.ExchangeHK, StockCode: "00700.HK", EntrustID: "E20260921001",
			})
			if err != nil {
				return err
			}
			return check(id != "", "cancel id is empty")
		}},
		{"POST /trade/TradeBatchCancelEntrust", func(ctx context.Context) error {
			r, err := trd.BatchCancelEntrust(ctx, trade.BatchCancelEntrustRequest{ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(len(r.SuccessEntrustID) == 2, "batch success = %d", len(r.SuccessEntrustID))
		}},
		{"POST /trade/TradeChangeEntrust", func(ctx context.Context) error {
			id, err := trd.ChangeEntrust(ctx, trade.ChangeEntrustRequest{
				ExchangeType: types.ExchangeHK, StockCode: "00700.HK", EntrustAmount: "100",
				EntrustPrice: "388.00", EntrustID: "E20260921001",
			})
			if err != nil {
				return err
			}
			return check(id != "", "change id is empty")
		}},
		{"POST /trade/TradeQueryMaxAvailableAsset", func(ctx context.Context) error {
			r, err := trd.MaxAvailableAsset(ctx, trade.MaxAvailableAssetRequest{
				ExchangeType: types.ExchangeHK, StockCode: "00700.HK",
				EntrustPrice: "388.00", EntrustType: types.EntrustTypeLimit,
			})
			if err != nil {
				return err
			}
			return check(r.PositionStatus == "0", "PositionStatus = %q", r.PositionStatus)
		}},
		{"POST /trade/TradeQueryRealEntrustList", func(ctx context.Context) error {
			r, err := trd.RealEntrustList(ctx, trade.RealEntrustListRequest{ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(len(r) == 20, "RealEntrustList rows = %d, want 20", len(r))
		}},
		{"POST /trade/TradeQueryRealDeliverList", func(ctx context.Context) error {
			r, err := trd.RealDeliverList(ctx, trade.RealDeliverListRequest{ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(len(r) > 0, "RealDeliverList is empty")
		}},
		{"POST /trade/TradeQueryRealCondOrderList", func(ctx context.Context) error {
			r, err := trd.RealCondOrderList(ctx, trade.CondOrderListRequest{ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(r.CurPageNo == 1, "CurPageNo = %d", r.CurPageNo)
		}},
		{"POST /trade/TradeQueryHistoryEntrustList", func(ctx context.Context) error {
			r, err := trd.HistoryEntrustList(ctx, trade.HistoryEntrustListRequest{
				ExchangeType: types.ExchangeHK, QueryCount: 20, QueryParamStr: "0",
				StartDate: "20260901", EndDate: "20260921",
			})
			if err != nil {
				return err
			}
			return check(len(r) > 0, "HistoryEntrustList is empty")
		}},
		{"POST /trade/TradeQueryHistoryDeliverList", func(ctx context.Context) error {
			r, err := trd.HistoryDeliverList(ctx, trade.HistoryDeliverListRequest{
				ExchangeType: types.ExchangeHK, QueryCount: 20, QueryParamStr: "0",
				StartDate: "20260901", EndDate: "20260921",
			})
			if err != nil {
				return err
			}
			return check(len(r) > 0, "HistoryDeliverList is empty")
		}},
		{"POST /trade/TradeQueryHistoryCondOrderList", func(ctx context.Context) error {
			r, err := trd.HistoryCondOrderList(ctx, trade.HistoryCondOrderListRequest{
				ExchangeType: types.ExchangeHK,
				StartTime:    "2026-09-01 00:00:00", EndTime: "2026-09-21 23:59:59",
			})
			if err != nil {
				return err
			}
			return check(r.CurPageNo == 1, "CurPageNo = %d", r.CurPageNo)
		}},
		{"POST /trade/TradeQueryMarginFullInfo", func(ctx context.Context) error {
			r, err := trd.MarginFullInfo(ctx, trade.MarginFullInfoRequest{DataType: "10000", StockCode: "00700.HK"})
			if err != nil {
				return err
			}
			return check(r.MarginAllow == "1", "MarginAllow = %q", r.MarginAllow)
		}},
		{"POST /trade/TradeQueryBeforeAndAfterSupport", func(ctx context.Context) error {
			v, err := trd.BeforeAndAfterSupport(ctx, trade.BeforeAndAfterSupportRequest{
				StockCode: "00700.HK", ExchangeType: types.ExchangeHK,
			})
			if err != nil {
				return err
			}
			return check(v == "1", "support = %q", v)
		}},

		// --- Trade push subscribe (2) -----------------------------------
		{"POST /trade/TradeSubscribe", func(ctx context.Context) error {
			return trd.SubscribeOrders(ctx)
		}},
		{"POST /trade/TradeUnsubscribe", func(ctx context.Context) error {
			return trd.UnsubscribeOrders(ctx)
		}},

		// --- Algo (7) ---------------------------------------------------
		{"POST /trade/AlgoAddOrder", func(ctx context.Context) error {
			id, err := alg.AddOrder(ctx, algo.AddOrderParams{
				StockCode: "00700.HK", ExchangeType: types.ExchangeHK,
				EntrustType: algo.EntrustTypeLimit, EntrustPrice: "388.00", EntrustAmount: "1000",
				EntrustBS: types.EntrustBuy, TargetStrategy: algo.StrategyVWAP, SessionType: algo.SessionTypeOff,
				StrategyParam: algo.StrategyParam{MaxVolume: "100", Sensitivity: algo.SensitivityNeutral},
			})
			if err != nil {
				return err
			}
			return check(id == "MA20260921001", "algo id = %q", id)
		}},
		{"POST /trade/AlgoCancelOrder", func(ctx context.Context) error {
			id, err := alg.CancelOrder(ctx, algo.CancelOrderParams{OrderID: "MA20260921001", ExchangeType: types.ExchangeHK})
			if err != nil {
				return err
			}
			return check(id != "", "algo cancel id is empty")
		}},
		{"POST /trade/AlgoCancelEntrust", func(ctx context.Context) error {
			id, err := alg.CancelEntrust(ctx, algo.CancelEntrustParams{
				OrderID: "MA20260921001", EntrustID: "CH20260921001", ExchangeType: types.ExchangeHK,
			})
			if err != nil {
				return err
			}
			return check(id != "", "algo cancel entrust id is empty")
		}},
		{"POST /trade/AlgoChangeOrder", func(ctx context.Context) error {
			id, err := alg.ChangeOrder(ctx, algo.ChangeOrderParams{
				OrderID: "MA20260921001", StockCode: "00700.HK", ExchangeType: types.ExchangeHK,
				EntrustPrice: "389.00", EntrustAmount: "800",
			})
			if err != nil {
				return err
			}
			return check(id != "", "algo change id is empty")
		}},
		{"POST /trade/AlgoActionOrder", func(ctx context.Context) error {
			id, err := alg.ActionOrder(ctx, algo.ActionOrderParams{
				OrderID: "MA20260921001", Action: algo.ActionStart,
				TargetStrategy: algo.StrategyVWAP, ExchangeType: types.ExchangeHK,
			})
			if err != nil {
				return err
			}
			return check(id != "", "algo action id is empty")
		}},
		{"POST /trade/AlgoQueryOrderList", func(ctx context.Context) error {
			r, err := alg.QueryOrderList(ctx, algo.QueryOrderListParams{
				PageNo: "1", PageSize: "20", StartDate: "20260901", EndDate: "20260921",
			})
			if err != nil {
				return err
			}
			return check(len(r) == 1, "algo order list = %d", len(r))
		}},
		{"POST /trade/AlgoQueryEntrustIdList", func(ctx context.Context) error {
			r, err := alg.QueryEntrustIDList(ctx, algo.QueryEntrustIDListParams{
				OrderID: "MA20260921001", TradeDate: "20260921", ExchangeType: types.ExchangeHK,
			})
			if err != nil {
				return err
			}
			return check(len(r) == 2, "algo entrust ids = %d", len(r))
		}},

		// --- Futures (11) -----------------------------------------------
		{"POST /trade/FuturesQueryProductInfo", func(ctx context.Context) error {
			r, err := fut.QueryProductInfo(ctx, future.QueryProductInfoRequest{StockCodes: []string{"HSI2609.HK"}})
			if err != nil {
				return err
			}
			return check(len(r.ProductInfoVos) == 1, "product infos = %d", len(r.ProductInfoVos))
		}},
		{"POST /trade/FuturesQueryMaxBuySellAmount", func(ctx context.Context) error {
			r, err := fut.QueryMaxBuySellAmount(ctx, future.QueryMaxBuySellAmountRequest{StockCode: "HSI2609.HK"})
			if err != nil {
				return err
			}
			return check(r.MaxBuyAmount == 10 && r.Ccy == "HKD", "max = %d/%d ccy=%q", r.MaxBuyAmount, r.MaxSellAmount, r.Ccy)
		}},
		{"POST /trade/FuturesQueryFundInfo", func(ctx context.Context) error {
			r, err := fut.QueryFundInfo(ctx)
			if err != nil {
				return err
			}
			return check(r.FundInfo.AssetBalance == "1000000.00", "assetBalance = %q", r.FundInfo.AssetBalance)
		}},
		{"POST /trade/FuturesQueryHoldsList", func(ctx context.Context) error {
			r, err := fut.QueryHoldsList(ctx)
			if err != nil {
				return err
			}
			return check(len(r.HoldsList) == 1, "futures holds = %d", len(r.HoldsList))
		}},
		{"POST /trade/FuturesEntrust", func(ctx context.Context) error {
			r, err := fut.Entrust(ctx, future.EntrustRequest{
				StockCode: "HSI2609.HK", EntrustType: "0", EntrustPrice: "25000",
				EntrustAmount: "1", EntrustBS: "1",
			})
			if err != nil {
				return err
			}
			return check(r.Data == "F20260921001", "futures entrust = %q", r.Data)
		}},
		{"POST /trade/FuturesCancelEntrust", func(ctx context.Context) error {
			r, err := fut.CancelEntrust(ctx, future.CancelEntrustRequest{EntrustID: "F20260921001", StockCode: "HSI2609.HK"})
			if err != nil {
				return err
			}
			return check(r.Data != "", "futures cancel is empty")
		}},
		{"POST /trade/FuturesModifyEntrust", func(ctx context.Context) error {
			r, err := fut.ModifyEntrust(ctx, future.ModifyEntrustRequest{
				EntrustID: "F20260921001", StockCode: "HSI2609.HK",
				EntrustPrice: "25100", EntrustAmount: "1", EntrustBS: "1",
			})
			if err != nil {
				return err
			}
			return check(r.Data != "", "futures modify is empty")
		}},
		{"POST /trade/FuturesQueryRealEntrustList", func(ctx context.Context) error {
			r, err := fut.QueryRealEntrustList(ctx)
			if err != nil {
				return err
			}
			return check(len(r.Data) == 1, "futures real entrust = %d", len(r.Data))
		}},
		{"POST /trade/FuturesQueryHistoryEntrustList", func(ctx context.Context) error {
			r, err := fut.QueryHistoryEntrustList(ctx, future.HistoryQueryRequest{})
			if err != nil {
				return err
			}
			return check(r.CurPageNo == 1, "futures history entrust page = %d", r.CurPageNo)
		}},
		{"POST /trade/FuturesQueryRealDeliverList", func(ctx context.Context) error {
			r, err := fut.QueryRealDeliverList(ctx)
			if err != nil {
				return err
			}
			return check(len(r.Data) == 1, "futures real deliver = %d", len(r.Data))
		}},
		{"POST /trade/FuturesQueryHistoryDeliverList", func(ctx context.Context) error {
			r, err := fut.QueryHistoryDeliverList(ctx, future.HistoryQueryRequest{})
			if err != nil {
				return err
			}
			return check(r.CurPageNo == 1, "futures history deliver page = %d", r.CurPageNo)
		}},
	}

	if len(cases) != 51 {
		t.Fatalf("e2e endpoint table = %d, want 51 (docs/SPEC.md §2)", len(cases))
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if err := c.call(ctx); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		})
	}
}

// TestE2E_SessionReLogin asserts an injected 1014 surfaces as a typed error and
// drives the session's single-flight re-login.
func TestE2E_SessionReLogin(t *testing.T) {
	srv, cli := newMockClient(t)
	sess := hstong.NewSessionManager(cli)
	trd := trade.New(cli, trade.WithSession(sess))
	ctx := context.Background()

	if err := sess.Login(ctx); err != nil {
		t.Fatalf("Login: %v", err)
	}
	srv.Inject("/trade/TradeQueryMarginFundInfo", mockgateway.ErrorInjection{
		Code: "1014", Message: "login timeout", Times: 1,
	})

	_, err := trd.MarginFundInfo(ctx, trade.MarginFundInfoRequest{ExchangeType: types.ExchangeHK})
	if err == nil {
		t.Fatal("expected an injected 1014 error")
	}
	if code, ok := errs.CodeOf(err); !ok || code != types.StatusLoginTimeout {
		t.Fatalf("code = %q (ok=%v), want 1014", code, ok)
	}
	if !errs.ReLoginRequired(err) {
		t.Fatalf("ReLoginRequired(%v) = false, want true", err)
	}
	if !sess.IsLoggedIn() {
		t.Fatal("session was not restored by the re-login")
	}
}

// TestE2E_ErrorMapping asserts injected ok:false envelopes surface as typed
// *errs.Error values with the documented Retryable/ReLoginRequired flags.
func TestE2E_ErrorMapping(t *testing.T) {
	srv, cli := newMockClient(t)
	mkt := market.New(cli)
	ctx := context.Background()

	t.Run("1011 retryable", func(t *testing.T) {
		srv.Inject("/hq/BasicQot", mockgateway.ErrorInjection{Code: "1011", Message: "busy"})
		_, err := mkt.BasicQot(ctx, market.BasicQotRequest{Security: []*dto.Security{hkSecurity()}})
		if err == nil {
			t.Fatal("expected error")
		}
		var typed *errs.Error
		if !errors.As(err, &typed) {
			t.Fatalf("error %T is not *errs.Error", err)
		}
		if code, _ := errs.CodeOf(err); code != types.StatusServiceBusy {
			t.Fatalf("code = %q, want 1011", code)
		}
		if !errs.Retryable(err) {
			t.Fatal("1011 must be Retryable")
		}
		if errs.ReLoginRequired(err) {
			t.Fatal("1011 must not require re-login")
		}
	})

	t.Run("1014 re-login required", func(t *testing.T) {
		srv.Inject("/hq/BasicQot", mockgateway.ErrorInjection{Code: "1014", Message: "login timeout"})
		_, err := mkt.BasicQot(ctx, market.BasicQotRequest{Security: []*dto.Security{hkSecurity()}})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errs.ReLoginRequired(err) {
			t.Fatalf("ReLoginRequired(%v) = false, want true", err)
		}
		if errs.Retryable(err) {
			t.Fatal("1014 must not be Retryable")
		}
	})
}

// TestE2E_Push connects the stream client, subscribes a quote topic and the
// trade push, has the mock emit frames (with an unrecognisable Any type_url),
// and asserts typed Events arrive.
func TestE2E_Push(t *testing.T) {
	srv := mockgateway.New(
		mockgateway.WithPushAddr("127.0.0.1:0"),
		mockgateway.WithPushTypeURL("type.googleapis.com/does.not.Exist"),
	)
	srv.StartT(t)

	cli, err := client.New(
		client.WithBaseURL(srv.HTTPBaseURL()),
		client.WithPushAddr(srv.PushAddr()),
		client.WithTradePassword("123456"),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	trd := trade.New(cli)
	s := stream.New(cli, stream.WithTradeManager(trd))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	quoteSub, err := s.Subscribe(ctx, types.TopicBasicQot, &dto.Security{DataType: int32(types.DataTypeHKStock), Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe quote: %v", err)
	}
	tradeSub, err := s.SubscribeTrade(ctx)
	if err != nil {
		t.Fatalf("SubscribeTrade: %v", err)
	}
	if !srv.TradeSubscribed() {
		t.Fatal("mock did not record the trade subscription")
	}
	waitForClients(t, srv, 1)

	if err := srv.EmitQuote(&dto.Security{DataType: int32(types.DataTypeHKStock), Code: "00700.HK"},
		&dto.BasicQot{LastPrice: 388.0}); err != nil {
		t.Fatalf("EmitQuote: %v", err)
	}
	select {
	case ev, ok := <-quoteSub.Updates():
		if !ok {
			t.Fatal("quote updates channel closed")
		}
		q, ok := ev.BasicQot()
		if !ok {
			t.Fatalf("quote payload type = %T", ev.Payload)
		}
		if q.GetSecurity().GetCode() != "00700.HK" || q.GetBasicQot().GetLastPrice() != 388.0 {
			t.Fatalf("quote payload = %+v", q)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the quote event")
	}

	if err := srv.EmitTradeDeliver(&tradenotify.TradeStockDeliverNotify{
		FundAccount: "F0001", StockCode: "00700.HK", EntrustNo: "E20260921001",
		EntrustStatus: "2", MatchNo: "M0001",
	}); err != nil {
		t.Fatalf("EmitTradeDeliver: %v", err)
	}
	select {
	case ev, ok := <-tradeSub.Updates():
		if !ok {
			t.Fatal("trade updates channel closed")
		}
		d, ok := ev.TradeDeliver()
		if !ok {
			t.Fatalf("trade payload type = %T", ev.Payload)
		}
		if d.GetStockCode() != "00700.HK" || d.GetEntrustNo() != "E20260921001" {
			t.Fatalf("trade payload = %+v", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the trade event")
	}
}

// waitForClients polls until the mock has at least n connected push clients.
func waitForClients(t *testing.T, srv *mockgateway.Server, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if srv.ClientCount() >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("push clients = %d, want >= %d", srv.ClientCount(), n)
}
