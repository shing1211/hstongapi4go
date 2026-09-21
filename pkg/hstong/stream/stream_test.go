// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/shing1211/hstongapi4go/client"
	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/hstong/stream"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// httpRecorder is an httptest handler that counts calls per path and returns a
// success envelope.
type httpRecorder struct {
	mu     sync.Mutex
	counts map[string]int
}

func newHTTPRecorder() *httpRecorder {
	return &httpRecorder{counts: make(map[string]int)}
}

func (r *httpRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	_, _ = io.ReadAll(req.Body)
	r.mu.Lock()
	r.counts[req.URL.Path]++
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true,"err":"","data":null}`)
}

func (r *httpRecorder) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[path]
}

func (r *httpRecorder) waitCount(t *testing.T, path string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.count(path) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s called %d times, want >= %d", path, r.count(path), want)
}

// fakeGateway is a local TCP listener that hands accepted connections to the
// test and exits cleanly on stop.
type fakeGateway struct {
	ln     net.Listener
	connCh chan net.Conn
	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

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

func (g *fakeGateway) addr() string { return g.ln.Addr().String() }

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

// addrRouter routes dials to the currently registered address.
type addrRouter struct {
	mu   sync.Mutex
	addr string
}

func (r *addrRouter) set(addr string) {
	r.mu.Lock()
	r.addr = addr
	r.mu.Unlock()
}

func (r *addrRouter) dial(ctx context.Context, network, _ string) (net.Conn, error) {
	r.mu.Lock()
	addr := r.addr
	r.mu.Unlock()
	d := &net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

// basicQotFrame builds a goldens BasicQot push frame with a bogus Any type_url.
func basicQotFrame(t *testing.T, code string, ts uint64) []byte {
	t.Helper()
	a, err := anypb.New(&hqnotify.BasicQotNotify{
		Security: &dto.Security{DataType: 10000, Code: code},
		BasicQot: &dto.BasicQot{LastPrice: 388.0},
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	a.TypeUrl = "type.googleapis.com/does.not.Exist"
	body, err := proto.Marshal(&pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(types.BasicQotNotifyMsgType),
		NotifyId:      code,
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

func TestSubscribeReceivesAndCancel(t *testing.T) {
	rec := newHTTPRecorder()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	gw := newFakeGateway(t)

	c, err := client.New(client.WithBaseURL(srv.URL), client.WithPushAddr(gw.addr()))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	s := stream.New(c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	sub, err := s.Subscribe(ctx, types.TopicBasicQot, &dto.Security{DataType: 10000, Code: "0700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec.waitCount(t, "/hq/Subscribe", 1)

	conn := gw.accept(t)
	defer conn.Close()
	if _, err := conn.Write(basicQotFrame(t, "0700.HK", 1700000000000)); err != nil {
		t.Fatalf("conn.Write: %v", err)
	}

	select {
	case ev, ok := <-sub.Updates():
		if !ok {
			t.Fatal("updates channel closed before an event arrived")
		}
		if ev.Type != types.BasicQotNotifyMsgType || ev.ID != "0700.HK" {
			t.Fatalf("event = %+v", ev)
		}
		q, ok := ev.BasicQot()
		if !ok || q.GetSecurity().GetCode() != "0700.HK" {
			t.Fatalf("BasicQot() = %#v (ok=%v)", q, ok)
		}
		if ev.Time.IsZero() {
			t.Errorf("event time is zero, want the frame timestamp")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
	}

	if err := sub.Cancel(ctx); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	rec.waitCount(t, "/hq/Unsubscribe", 1)

	select {
	case _, ok := <-sub.Updates():
		if ok {
			t.Fatal("updates channel still open after Cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("updates channel was not closed by Cancel")
	}
}

func TestReconnectResubscribes(t *testing.T) {
	rec := newHTTPRecorder()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	router := &addrRouter{}
	gw1 := newFakeGateway(t)
	router.set(gw1.addr())

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	s := stream.New(c,
		stream.WithDialer(router.dial),
		stream.WithReconnect(10*time.Millisecond, 50*time.Millisecond),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	sub, err := s.Subscribe(ctx, types.TopicBasicQot, &dto.Security{DataType: 10000, Code: "0700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec.waitCount(t, "/hq/Subscribe", 1)

	conn1 := gw1.accept(t)
	if _, err := conn1.Write(basicQotFrame(t, "0700.HK", 1)); err != nil {
		t.Fatalf("conn1.Write: %v", err)
	}
	select {
	case ev := <-sub.Updates():
		if ev.ID != "0700.HK" {
			t.Fatalf("first event = %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the first event")
	}

	_ = conn1.Close()
	gw1.stop()

	gw2 := newFakeGateway(t)
	router.set(gw2.addr())

	select {
	case err := <-sub.Errors():
		if !errors.Is(err, stream.ErrReconnected) {
			t.Fatalf("reconnect notice = %v, want ErrReconnected", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reconnect notice on Errors()")
	}

	rec.waitCount(t, "/hq/Subscribe", 2)

	conn2 := gw2.accept(t)
	defer conn2.Close()
	if _, err := conn2.Write(basicQotFrame(t, "0005.HK", 2)); err != nil {
		t.Fatalf("conn2.Write: %v", err)
	}
	select {
	case ev := <-sub.Updates():
		if ev.ID != "0005.HK" {
			t.Fatalf("post-reconnect event = %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the post-reconnect event")
	}
}

func TestSubscribeValidation(t *testing.T) {
	rec := newHTTPRecorder()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	s := stream.New(c)
	t.Cleanup(func() { _ = s.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := s.Subscribe(ctx, types.TopicID(99), &dto.Security{Code: "0700.HK"}); err == nil {
		t.Fatal("unknown topic accepted")
	}
	if _, err := s.Subscribe(ctx, types.TopicBasicQot); err == nil {
		t.Fatal("empty security list accepted")
	}
	if got := rec.count("/hq/Subscribe"); got != 0 {
		t.Fatalf("sent %d subscribe requests for invalid input, want 0", got)
	}
}
