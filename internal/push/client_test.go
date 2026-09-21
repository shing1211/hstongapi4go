// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind, including the
// fake Gateway accept loops and the push read loop.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeGateway is a local TCP listener that hands accepted connections to the
// test and exits cleanly on stop, so goleak stays quiet.
type fakeGateway struct {
	ln     net.Listener
	connCh chan net.Conn
	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

// newFakeGateway starts a fake Gateway listener on an ephemeral local port.
func newFakeGateway(t *testing.T) *fakeGateway {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	g := &fakeGateway{ln: ln, connCh: make(chan net.Conn, 8), stopCh: make(chan struct{})}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case g.connCh <- c:
			case <-g.stopCh:
				_ = c.Close()
				return
			}
		}
	}()
	t.Cleanup(g.stop)
	return g
}

// addr returns the listener address.
func (g *fakeGateway) addr() string { return g.ln.Addr().String() }

// accept waits for the next accepted connection and fails the test on timeout.
func (g *fakeGateway) accept(t *testing.T) net.Conn {
	t.Helper()
	select {
	case c := <-g.connCh:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("fake gateway: timed out waiting for a connection")
		return nil
	}
}

// stop closes the listener and drains accepted connections.
func (g *fakeGateway) stop() {
	g.once.Do(func() {
		close(g.stopCh)
		_ = g.ln.Close()
	})
	g.wg.Wait()
	for {
		select {
		case c := <-g.connCh:
			_ = c.Close()
		default:
			return
		}
	}
}

// addrRouter is a dialer that always connects to the currently registered
// address, letting a reconnect test move the target between listeners.
type addrRouter struct {
	mu   sync.Mutex
	addr string
}

// set replaces the dial target.
func (r *addrRouter) set(addr string) {
	r.mu.Lock()
	r.addr = addr
	r.mu.Unlock()
}

// dial implements push.DialFunc.
func (r *addrRouter) dial(ctx context.Context, network, _ string) (net.Conn, error) {
	r.mu.Lock()
	addr := r.addr
	r.mu.Unlock()
	d := &net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

// notifyFrame is one golden push frame to encode.
type notifyFrame struct {
	typ     types.NotifyMsgType
	id      string
	ts      uint64
	payload proto.Message
}

// marshalNotifyFrame builds a full push frame with a goldens payload. The Any
// type_url is deliberately bogus to prove the client decodes by notifyMsgType
// rather than by the global type registry.
func marshalNotifyFrame(t *testing.T, f notifyFrame) []byte {
	t.Helper()
	var payload *anypb.Any
	if f.payload != nil {
		a, err := anypb.New(f.payload)
		if err != nil {
			t.Fatalf("anypb.New: %v", err)
		}
		a.TypeUrl = "type.googleapis.com/does.not.Exist"
		payload = a
	}
	n := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(f.typ),
		NotifyId:      f.id,
		NotifyTime:    f.ts,
		Payload:       payload,
	}
	body, err := proto.Marshal(n)
	if err != nil {
		t.Fatalf("proto.Marshal PBNotify: %v", err)
	}
	var buf bytes.Buffer
	if err := push.WriteFrame(&buf, push.Header{MsgType: push.MsgPush, SerialNo: 1}, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.Bytes()
}

// marshalHeartbeat builds an empty heartbeat frame.
func marshalHeartbeat(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := push.WriteFrame(&buf, push.Header{MsgType: push.MsgHeartbeat, SerialNo: 2}, nil); err != nil {
		t.Fatalf("WriteFrame heartbeat: %v", err)
	}
	return buf.Bytes()
}

// writeFrame writes raw to conn and fails the test on error.
func writeFrame(t *testing.T, conn net.Conn, raw []byte) {
	t.Helper()
	if _, err := conn.Write(raw); err != nil {
		t.Fatalf("conn.Write: %v", err)
	}
}

// waitNotification receives one notification or fails on timeout.
func waitNotification(t *testing.T, ch <-chan *push.Notification) *push.Notification {
	t.Helper()
	select {
	case n := <-ch:
		return n
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a notification")
		return nil
	}
}

// waitNotificationsByType collects exactly n notifications and indexes them by
// notify type, failing on a timeout or a duplicate type. Delivery order across
// notify types is intentionally undefined -- each type is served by its own
// dispatcher goroutine -- so callers must key by type rather than assert a
// cross-type sequence.
func waitNotificationsByType(t *testing.T, ch <-chan *push.Notification, n int) map[types.NotifyMsgType]*push.Notification {
	t.Helper()
	got := make(map[types.NotifyMsgType]*push.Notification, n)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for len(got) < n {
		select {
		case notif := <-ch:
			if _, dup := got[notif.Type]; dup {
				t.Fatalf("duplicate notification for type %d", notif.Type)
			}
			got[notif.Type] = notif
		case <-timer.C:
			t.Fatalf("timed out: received %d/%d notifications", len(got), n)
		}
	}
	return got
}

func TestClientDecodesTypedNotifications(t *testing.T) {
	gw := newFakeGateway(t)
	pc := push.New(push.WithAddr(gw.addr()), push.WithReconnect(10*time.Millisecond, 50*time.Millisecond))
	t.Cleanup(func() { _ = pc.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan *push.Notification, 16)
	for _, typ := range []types.NotifyMsgType{
		types.BasicQotNotifyMsgType,
		types.TickerNotifyMsgType,
		types.OrderBookNotifyMsgType,
		types.BrokerQueueNotifyMsgType,
		types.TradeStockDeliverMsgType,
	} {
		pc.SubscribeTypes(typ, func(n *push.Notification) { got <- n })
	}

	if err := pc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	go func() { _ = pc.Run(ctx) }()

	conn := gw.accept(t)
	defer conn.Close()

	frames := []notifyFrame{
		{types.BasicQotNotifyMsgType, "0700.HK", 1700000000000, &hqnotify.BasicQotNotify{
			Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
			BasicQot: &dto.BasicQot{LastPrice: 388.0},
		}},
		{types.TickerNotifyMsgType, "0005.HK", 1700000000001, &hqnotify.TickerNotify{
			Security: &dto.Security{DataType: 10000, Code: "0005.HK"},
			Ticker:   &dto.Ticker{Price: 60.5, Volume: 1000},
		}},
		{types.OrderBookNotifyMsgType, "0700.HK", 1700000000002, &hqnotify.OrderBookFullNotify{
			Security:      &dto.Security{DataType: 10000, Code: "0700.HK"},
			Side:          1,
			OrderBookList: []*dto.OrderBook{{Level: 1, Price: 387.8, Volume: 200}},
		}},
		{types.BrokerQueueNotifyMsgType, "0700.HK", 1700000000003, &hqnotify.BrokerNotify{
			Security:   &dto.Security{DataType: 10000, Code: "0700.HK"},
			Side:       0,
			BrokerList: []*dto.Broker{{Level: 1, Name: "BROKER"}},
		}},
		{types.TradeStockDeliverMsgType, "0700.HK", 1700000000004, &tradenotify.TradeStockDeliverNotify{
			FundAccount:   "FUND-1",
			StockCode:     "0700.HK",
			BusinessPrice: "388.0",
			MatchNo:       "M-1",
		}},
	}
	for _, f := range frames {
		writeFrame(t, conn, marshalNotifyFrame(t, f))
	}

	byType := waitNotificationsByType(t, got, len(frames))

	n0 := byType[types.BasicQotNotifyMsgType]
	if n0 == nil || n0.ID != "0700.HK" || n0.Time != 1700000000000 {
		t.Fatalf("basic qot envelope = %+v", n0)
	}
	q, ok := n0.Payload.(*hqnotify.BasicQotNotify)
	if !ok || q.GetSecurity().GetCode() != "0700.HK" || q.GetBasicQot().GetLastPrice() != 388.0 {
		t.Fatalf("BasicQot payload = %#v (ok=%v)", n0.Payload, ok)
	}

	n1 := byType[types.TickerNotifyMsgType]
	if n1 == nil {
		t.Fatal("no TickerNotify notification received")
	}
	tk, ok := n1.Payload.(*hqnotify.TickerNotify)
	if !ok || tk.GetTicker().GetVolume() != 1000 {
		t.Fatalf("Ticker payload = %#v (ok=%v)", n1.Payload, ok)
	}

	n2 := byType[types.OrderBookNotifyMsgType]
	if n2 == nil {
		t.Fatal("no OrderBookFullNotify notification received")
	}
	ob, ok := n2.Payload.(*hqnotify.OrderBookFullNotify)
	if !ok || len(ob.GetOrderBookList()) != 1 || ob.GetSide() != 1 {
		t.Fatalf("OrderBook payload = %#v (ok=%v)", n2.Payload, ok)
	}

	n3 := byType[types.BrokerQueueNotifyMsgType]
	if n3 == nil {
		t.Fatal("no BrokerNotify notification received")
	}
	br, ok := n3.Payload.(*hqnotify.BrokerNotify)
	if !ok || br.GetBrokerList()[0].GetName() != "BROKER" {
		t.Fatalf("Broker payload = %#v (ok=%v)", n3.Payload, ok)
	}

	n4 := byType[types.TradeStockDeliverMsgType]
	if n4 == nil {
		t.Fatal("no TradeStockDeliverNotify notification received")
	}
	td, ok := n4.Payload.(*tradenotify.TradeStockDeliverNotify)
	if !ok || td.GetFundAccount() != "FUND-1" || td.GetMatchNo() != "M-1" {
		t.Fatalf("TradeDeliver payload = %#v (ok=%v)", n4.Payload, ok)
	}
}

func TestClientIgnoresHeartbeat(t *testing.T) {
	gw := newFakeGateway(t)
	pc := push.New(push.WithAddr(gw.addr()))
	t.Cleanup(func() { _ = pc.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan *push.Notification, 4)
	pc.SubscribeTypes(types.BasicQotNotifyMsgType, func(n *push.Notification) { got <- n })

	if err := pc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	go func() { _ = pc.Run(ctx) }()

	conn := gw.accept(t)
	defer conn.Close()

	writeFrame(t, conn, marshalHeartbeat(t))
	writeFrame(t, conn, marshalNotifyFrame(t, notifyFrame{
		types.BasicQotNotifyMsgType, "0700.HK", 1,
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0700.HK"}},
	}))
	writeFrame(t, conn, marshalNotifyFrame(t, notifyFrame{
		types.BasicQotNotifyMsgType, "0005.HK", 2,
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0005.HK"}},
	}))

	// The read loop processes frames in wire order and the type's dispatcher
	// preserves that order, so a heartbeat that produced a notification would
	// arrive before the first push. Waiting for the two real notifications in
	// order is a deterministic substitute for a sleep-based absence check.
	if n := waitNotification(t, got); n.ID != "0700.HK" {
		t.Fatalf("first notification = %+v, want 0700.HK", n)
	}
	if n := waitNotification(t, got); n.ID != "0005.HK" {
		t.Fatalf("second notification = %+v, want 0005.HK", n)
	}
}

func TestClientReconnectsAndFiresHook(t *testing.T) {
	router := &addrRouter{}
	gw1 := newFakeGateway(t)
	router.set(gw1.addr())

	reconnected := make(chan error, 4)
	pc := push.New(
		push.WithAddr("unused:0"),
		push.WithDialer(router.dial),
		push.WithReconnect(10*time.Millisecond, 50*time.Millisecond),
		push.WithOnReconnect(func(err error) { reconnected <- err }),
	)
	t.Cleanup(func() { _ = pc.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan *push.Notification, 8)
	pc.SubscribeTypes(types.BasicQotNotifyMsgType, func(n *push.Notification) { got <- n })

	if err := pc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	go func() { _ = pc.Run(ctx) }()

	conn1 := gw1.accept(t)
	writeFrame(t, conn1, marshalNotifyFrame(t, notifyFrame{
		types.BasicQotNotifyMsgType, "0700.HK", 1,
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0700.HK"}},
	}))
	if n := waitNotification(t, got); n.ID != "0700.HK" {
		t.Fatalf("first notification = %+v", n)
	}

	_ = conn1.Close()
	gw1.stop()

	gw2 := newFakeGateway(t)
	router.set(gw2.addr())

	select {
	case <-reconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("OnReconnect hook did not fire")
	}

	conn2 := gw2.accept(t)
	defer conn2.Close()
	writeFrame(t, conn2, marshalNotifyFrame(t, notifyFrame{
		types.BasicQotNotifyMsgType, "0005.HK", 2,
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0005.HK"}},
	}))
	if n := waitNotification(t, got); n.ID != "0005.HK" {
		t.Fatalf("post-reconnect notification = %+v", n)
	}
}

func TestClientCloseIsIdempotentAndTerminal(t *testing.T) {
	pc := push.New(push.WithAddr("127.0.0.1:1"))
	if err := pc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := pc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := pc.Connect(context.Background()); !errors.Is(err, push.ErrClosed) {
		t.Fatalf("Connect after Close = %v, want ErrClosed", err)
	}
	if err := pc.Run(context.Background()); !errors.Is(err, push.ErrClosed) {
		t.Fatalf("Run after Close = %v, want ErrClosed", err)
	}
	// SubscribeTypes after Close must be a no-op and must not panic.
	pc.SubscribeTypes(types.BasicQotNotifyMsgType, func(*push.Notification) {})
}

func TestDecodeUnknownType(t *testing.T) {
	if _, err := push.Decode(types.NotifyMsgType(4242), nil); err == nil {
		t.Fatal("Decode of an unknown type succeeded")
	}
}

func TestDecodeEmptyPayload(t *testing.T) {
	got, err := push.Decode(types.BasicQotNotifyMsgType, nil)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if _, ok := got.(*hqnotify.BasicQotNotify); !ok {
		t.Fatalf("Decode returned %T, want *hqnotify.BasicQotNotify", got)
	}
}
