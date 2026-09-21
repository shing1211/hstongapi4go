// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/shing1211/hstongapi4go/client"
	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/hstong/stream"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// fakeTradePusher counts the Gateway subscribe/unsubscribe calls the stream
// client issues for order push. It satisfies stream.TradePusher.
type fakeTradePusher struct {
	mu           sync.Mutex
	subscribes   int
	unsubscribes int
}

// SubscribeOrders records one subscribe call.
func (f *fakeTradePusher) SubscribeOrders(context.Context) error {
	f.mu.Lock()
	f.subscribes++
	f.mu.Unlock()
	return nil
}

// UnsubscribeOrders records one unsubscribe call.
func (f *fakeTradePusher) UnsubscribeOrders(context.Context) error {
	f.mu.Lock()
	f.unsubscribes++
	f.mu.Unlock()
	return nil
}

// counts returns the subscribe and unsubscribe totals.
func (f *fakeTradePusher) counts() (subscribes, unsubscribes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subscribes, f.unsubscribes
}

// tradeDeliverFrame builds a goldens TradeStockDeliverNotify push frame for
// notifyType. The Any type_url is deliberately bogus: the push decoder selects
// the concrete type from the wire enum, not the registry.
func tradeDeliverFrame(t *testing.T, notifyType types.NotifyMsgType, id string, ts uint64, msg *tradenotify.TradeStockDeliverNotify) []byte {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	a.TypeUrl = "type.googleapis.com/does.not.Exist"
	body, err := proto.Marshal(&pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(notifyType),
		NotifyId:      id,
		NotifyTime:    ts,
		Payload:       a,
	})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	var buf bytes.Buffer
	if err := push.WriteFrame(&buf, push.Header{MsgType: push.MsgPush}, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.Bytes()
}

// readEvent waits for one event on sub, failing the test on timeout or a close.
func readEvent(t *testing.T, sub *stream.Subscription) stream.Event {
	t.Helper()
	select {
	case ev, ok := <-sub.Updates():
		if !ok {
			t.Fatal("updates channel closed before an event arrived")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
		return stream.Event{}
	}
}

// newTradeStream starts a recorder-backed HTTP server and a local TCP push
// gateway, builds a client pointed at both, and returns a connected stream
// client. A non-nil tp is attached with stream.WithTradeManager.
func newTradeStream(t *testing.T, tp stream.TradePusher) (*stream.Client, *httpRecorder, *fakeGateway) {
	t.Helper()
	rec := newHTTPRecorder()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	gw := newFakeGateway(t)

	c, err := client.New(client.WithBaseURL(srv.URL), client.WithPushAddr(gw.addr()))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var opts []stream.Option
	if tp != nil {
		opts = append(opts, stream.WithTradeManager(tp))
	}
	s := stream.New(c, opts...)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = s.Close()
	})
	return s, rec, gw
}

// TestSubscribeTradeDecodesTypes asserts that one trade subscription receives
// the three order-status notify types and that the typed accessors split the
// stock (0, 1) and futures (2) variants.
func TestSubscribeTradeDecodesTypes(t *testing.T) {
	tp := &fakeTradePusher{}
	s, _, gw := newTradeStream(t, tp)

	sub, err := s.SubscribeTrade(context.Background())
	if err != nil {
		t.Fatalf("SubscribeTrade: %v", err)
	}
	if subs, unsubs := tp.counts(); subs != 1 || unsubs != 0 {
		t.Fatalf("subscribe/unsubscribe = %d/%d, want 1/0", subs, unsubs)
	}

	conn := gw.accept(t)
	defer conn.Close()

	cases := []struct {
		notifyType types.NotifyMsgType
		id         string
	}{
		{types.TrsStockDeliverMsgType, "TRS-1"},
		{types.TradeStockDeliverMsgType, "00700.HK"},
		{types.FuturesTradeStockDeliverMsgType, "HSI2603"},
	}
	for i, tc := range cases {
		msg := &tradenotify.TradeStockDeliverNotify{
			FundAccount:   "F123",
			StockCode:     tc.id,
			EntrustStatus: "2",
			EntrustNo:     "E1",
			RecordNo:      "E1",
		}
		if _, err := conn.Write(tradeDeliverFrame(t, tc.notifyType, tc.id, uint64(1700000000000+i), msg)); err != nil {
			t.Fatalf("conn.Write: %v", err)
		}

		ev := readEvent(t, sub)
		if ev.Type != tc.notifyType {
			t.Fatalf("event type = %d, want %d", ev.Type, tc.notifyType)
		}
		if ev.ID != tc.id {
			t.Fatalf("event ID = %q, want %q", ev.ID, tc.id)
		}
		if ev.Time.IsZero() {
			t.Errorf("event time is zero, want the frame timestamp")
		}

		if tc.notifyType == types.FuturesTradeStockDeliverMsgType {
			m, ok := ev.FuturesTradeDeliver()
			if !ok || m.GetStockCode() != tc.id {
				t.Fatalf("FuturesTradeDeliver() = %#v (ok=%v)", m, ok)
			}
			t.Logf("notifyType=%d id=%s futures.TradeStockDeliverNotify{fundAccount=%s stockCode=%s entrustStatus=%s}",
				ev.Type, ev.ID, m.GetFundAccount(), m.GetStockCode(), m.GetEntrustStatus())
			if _, ok := ev.TradeDeliver(); ok {
				t.Fatalf("TradeDeliver accepted a futures event (type 2)")
			}
			continue
		}
		m, ok := ev.TradeDeliver()
		if !ok || m.GetStockCode() != tc.id {
			t.Fatalf("TradeDeliver() = %#v (ok=%v)", m, ok)
		}
		t.Logf("notifyType=%d id=%s stock.TradeStockDeliverNotify{fundAccount=%s stockCode=%s entrustStatus=%s}",
			ev.Type, ev.ID, m.GetFundAccount(), m.GetStockCode(), m.GetEntrustStatus())
		if _, ok := ev.FuturesTradeDeliver(); ok {
			t.Fatalf("FuturesTradeDeliver accepted a stock event (type %d)", ev.Type)
		}
	}
}

// TestSubscribeTradeScenarios drives the documented order-status scenarios
// (place, change, cancel, fill) through the push channel and asserts the
// decoded fields, including that matchNo is set only for a fill.
func TestSubscribeTradeScenarios(t *testing.T) {
	tp := &fakeTradePusher{}
	s, _, gw := newTradeStream(t, tp)

	sub, err := s.SubscribeTrade(context.Background())
	if err != nil {
		t.Fatalf("SubscribeTrade: %v", err)
	}

	conn := gw.accept(t)
	defer conn.Close()

	scenarios := []struct {
		name           string
		notifyType     types.NotifyMsgType
		entrustStatus  string
		matchNo        string
		entrustNo      string
		entrustAmount  string
		leftAmount     string
		businessAmount string
		businessPrice  string
	}{
		{"place", types.TradeStockDeliverMsgType, "2", "", "E100", "1000", "1000", "", ""},
		{"change", types.TradeStockDeliverMsgType, "A", "", "E101", "2000", "2000", "", ""},
		{"cancel", types.TradeStockDeliverMsgType, "6", "", "E102", "1000", "1000", "", ""},
		{"fill", types.TradeStockDeliverMsgType, "8", "M9001", "E103", "1000", "0", "1000", "320.50"},
		{"futures-place", types.FuturesTradeStockDeliverMsgType, "2", "", "F200", "5", "5", "", ""},
		{"futures-fill", types.FuturesTradeStockDeliverMsgType, "8", "M9100", "F201", "5", "0", "5", "19850"},
	}

	for i, sc := range scenarios {
		msg := &tradenotify.TradeStockDeliverNotify{
			FundAccount:    "F123",
			StockCode:      "00700.HK",
			StockName:      "Tencent",
			EntrustBs:      "1",
			ExchangeType:   "K",
			EntrustStatus:  sc.entrustStatus,
			EntrustNo:      sc.entrustNo,
			RecordNo:       sc.entrustNo, // recordNo == entrustNo at placement
			EntrustAmount:  sc.entrustAmount,
			LeftAmount:     sc.leftAmount,
			MatchNo:        sc.matchNo,
			BusinessAmount: sc.businessAmount,
			BusinessPrice:  sc.businessPrice,
		}
		if _, err := conn.Write(tradeDeliverFrame(t, sc.notifyType, "00700.HK", uint64(1700000001000+i), msg)); err != nil {
			t.Fatalf("%s: conn.Write: %v", sc.name, err)
		}

		ev := readEvent(t, sub)
		if ev.Type != sc.notifyType {
			t.Fatalf("%s: event type = %d, want %d", sc.name, ev.Type, sc.notifyType)
		}

		var m *tradenotify.TradeStockDeliverNotify
		if sc.notifyType == types.FuturesTradeStockDeliverMsgType {
			got, ok := ev.FuturesTradeDeliver()
			if !ok {
				t.Fatalf("%s: FuturesTradeDeliver() ok=false", sc.name)
			}
			m = got
		} else {
			got, ok := ev.TradeDeliver()
			if !ok {
				t.Fatalf("%s: TradeDeliver() ok=false", sc.name)
			}
			m = got
		}

		if m.GetEntrustStatus() != sc.entrustStatus {
			t.Errorf("%s: entrustStatus = %q, want %q", sc.name, m.GetEntrustStatus(), sc.entrustStatus)
		}
		if m.GetEntrustNo() != sc.entrustNo || m.GetRecordNo() != sc.entrustNo {
			t.Errorf("%s: entrustNo/recordNo = %q/%q, want %q/%q",
				sc.name, m.GetEntrustNo(), m.GetRecordNo(), sc.entrustNo, sc.entrustNo)
		}
		if m.GetEntrustAmount() != sc.entrustAmount || m.GetLeftAmount() != sc.leftAmount {
			t.Errorf("%s: entrustAmount/leftAmount = %q/%q, want %q/%q",
				sc.name, m.GetEntrustAmount(), m.GetLeftAmount(), sc.entrustAmount, sc.leftAmount)
		}
		if m.GetBusinessAmount() != sc.businessAmount || m.GetBusinessPrice() != sc.businessPrice {
			t.Errorf("%s: businessAmount/businessPrice = %q/%q, want %q/%q",
				sc.name, m.GetBusinessAmount(), m.GetBusinessPrice(), sc.businessAmount, sc.businessPrice)
		}
		if m.GetMatchNo() != sc.matchNo {
			t.Errorf("%s: matchNo = %q, want %q", sc.name, m.GetMatchNo(), sc.matchNo)
		}
		if fill := m.GetMatchNo() != ""; fill != (m.GetBusinessAmount() != "" && m.GetLeftAmount() == "0") {
			t.Errorf("%s: matchNo/businessAmount/leftAmount inconsistent: %q/%q/%q",
				sc.name, m.GetMatchNo(), m.GetBusinessAmount(), m.GetLeftAmount())
		}

		t.Logf("scenario=%-13s notifyType=%d status=%s entrustNo=%s recordNo=%s matchNo=%q entrustAmount=%s leftAmount=%s businessAmount=%s businessPrice=%s",
			sc.name, ev.Type, m.GetEntrustStatus(), m.GetEntrustNo(), m.GetRecordNo(), m.GetMatchNo(),
			m.GetEntrustAmount(), m.GetLeftAmount(), m.GetBusinessAmount(), m.GetBusinessPrice())
	}
}

// TestSubscribeTradeCancel asserts Cancel triggers the HTTP unsubscribe once,
// closes both channels, and is idempotent.
func TestSubscribeTradeCancel(t *testing.T) {
	tp := &fakeTradePusher{}
	s, _, _ := newTradeStream(t, tp)

	sub, err := s.SubscribeTrade(context.Background())
	if err != nil {
		t.Fatalf("SubscribeTrade: %v", err)
	}

	if err := sub.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if subs, unsubs := tp.counts(); subs != 1 || unsubs != 1 {
		t.Fatalf("subscribe/unsubscribe = %d/%d, want 1/1", subs, unsubs)
	}

	select {
	case _, ok := <-sub.Updates():
		if ok {
			t.Fatal("updates channel still open after Cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("updates channel was not closed by Cancel")
	}
	select {
	case _, ok := <-sub.Errors():
		if ok {
			t.Fatal("errors channel still open after Cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("errors channel was not closed by Cancel")
	}

	if err := sub.Cancel(context.Background()); err != nil {
		t.Fatalf("second Cancel: %v", err)
	}
	if subs, unsubs := tp.counts(); subs != 1 || unsubs != 1 {
		t.Fatalf("second Cancel changed counts: subscribe/unsubscribe = %d/%d", subs, unsubs)
	}
}

// TestSubscribeTradeWithoutTradeManager asserts the documented push-only mode:
// no HTTP subscribe is sent, a decoded event still arrives, and Cancel sends no
// HTTP unsubscribe.
func TestSubscribeTradeWithoutTradeManager(t *testing.T) {
	s, rec, gw := newTradeStream(t, nil)

	sub, err := s.SubscribeTrade(context.Background())
	if err != nil {
		t.Fatalf("SubscribeTrade: %v", err)
	}
	if got := rec.count(string(client.RouteTradeSubscribe)); got != 0 {
		t.Fatalf("push-only SubscribeTrade sent %d HTTP subscribe calls, want 0", got)
	}

	conn := gw.accept(t)
	defer conn.Close()
	msg := &tradenotify.TradeStockDeliverNotify{
		StockCode:     "00700.HK",
		EntrustStatus: "8",
		EntrustNo:     "E103",
		RecordNo:      "E103",
		MatchNo:       "M9001",
	}
	if _, err := conn.Write(tradeDeliverFrame(t, types.TradeStockDeliverMsgType, "00700.HK", 1700000002000, msg)); err != nil {
		t.Fatalf("conn.Write: %v", err)
	}
	ev := readEvent(t, sub)
	m, ok := ev.TradeDeliver()
	if !ok || m.GetMatchNo() != "M9001" {
		t.Fatalf("TradeDeliver() = %#v (ok=%v)", m, ok)
	}

	if err := sub.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := rec.count(string(client.RouteTradeUnsubscribe)); got != 0 {
		t.Fatalf("push-only Cancel sent %d HTTP unsubscribe calls, want 0", got)
	}
}
