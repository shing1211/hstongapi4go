// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push_test

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

type raceRouter struct {
	mu   sync.Mutex
	addr string
}

func (r *raceRouter) set(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addr = addr
}

func (r *raceRouter) dial(ctx context.Context, addr string) (net.Conn, error) {
	r.mu.Lock()
	target := r.addr
	r.mu.Unlock()
	d := &net.Dialer{}
	return d.DialContext(ctx, "tcp", target)
}

type serverAddrRouter struct {
	mu         sync.Mutex
	listenAddr string
	ln         net.Listener
	connCh     chan net.Conn
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func newServerAddrRouter() *serverAddrRouter {
	return &serverAddrRouter{
		connCh: make(chan net.Conn, 4),
		stopCh: make(chan struct{}),
	}
}

func (r *serverAddrRouter) start(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	r.mu.Lock()
	r.ln = ln
	r.listenAddr = ln.Addr().String()
	r.mu.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			c, err := r.ln.Accept()
			if err != nil {
				select {
				case <-r.stopCh:
					return
				default:
					continue
				}
			}
			select {
			case r.connCh <- c:
			case <-r.stopCh:
				_ = c.Close()
				return
			}
		}
	}()
}

func (r *serverAddrRouter) accept() net.Conn {
	select {
	case c := <-r.connCh:
		return c
	case <-r.stopCh:
		return nil
	}
}

func (r *serverAddrRouter) stop() {
	r.mu.Lock()
	if r.ln != nil {
		_ = r.ln.Close()
	}
	r.mu.Unlock()
	close(r.stopCh)
	r.wg.Wait()
}

func (r *serverAddrRouter) addr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listenAddr
}

func TestManager_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	gw := newFakeGateway(t)
	t.Cleanup(func() { gw.stop() })

	router := &raceRouter{}
	router.set(gw.addr())

	mgr := push.NewManager(nil,
		push.WithManagerDialFunc(router.dial),
		push.WithReconnectBackoff(10*time.Millisecond, 50*time.Millisecond),
		push.WithReconnectMaxRetries(3),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Dial(ctx, gw.addr()); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = gw.accept(t)

	sec := []*dto.Security{{DataType: 10000, Code: "0700.HK"}}

	const nGoroutines = 10
	const nOps = 50
	var wg sync.WaitGroup
	var ops atomic.Int32

	for i := 0; i < nGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < nOps; j++ {
				topicID := (id*j + j) % 5
				if j%2 == 0 {
					_ = mgr.Subscribe(context.Background(), topicID, sec)
				} else {
					_ = mgr.Unsubscribe(context.Background(), topicID, sec)
				}
				ops.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if ops.Load() != nGoroutines*nOps {
		t.Errorf("ops = %d, want %d", ops.Load(), nGoroutines*nOps)
	}
}

func TestManager_CloseDuringReconnect(t *testing.T) {
	router := &raceRouter{}
	router.set("127.0.0.1:0")

	mgr := push.NewManager(nil,
		push.WithManagerDialFunc(router.dial),
		push.WithReconnectBackoff(10*time.Millisecond, 50*time.Millisecond),
		push.WithReconnectMaxRetries(100),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = mgr.Dial(ctx, router.addr)

	time.Sleep(50 * time.Millisecond)

	err := mgr.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-mgr.Done():
	case <-time.After(3 * time.Second):
		t.Error("Manager should have shut down")
	}
}

func TestManager_HeartbeatTimeoutTriggersReconnect(t *testing.T) {
	svr := newServerAddrRouter()
	svr.start(t)
	t.Cleanup(svr.stop)

	router := &raceRouter{}
	router.set(svr.addr())

	mgr := push.NewManager(nil,
		push.WithManagerDialFunc(router.dial),
		push.WithReconnectBackoff(50*time.Millisecond, 200*time.Millisecond),
		push.WithReconnectMaxRetries(3),
		push.WithHeartbeatInterval(50*time.Millisecond),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Dial(ctx, svr.addr()); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	conn1 := svr.accept()
	if conn1 == nil {
		t.Fatal("expected first connection")
	}

	conn1.Close()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			conn2 := svr.accept()
			if conn2 != nil {
				conn2.Close()
				goto done
			}
		case <-deadline:
			t.Fatal("reconnect did not happen within deadline")
		}
	}
done:
}

func TestManager_SubscribeDuringReconnect(t *testing.T) {
	svr := newServerAddrRouter()
	svr.start(t)
	t.Cleanup(svr.stop)

	router := &raceRouter{}
	router.set(svr.addr())

	mgr := push.NewManager(nil,
		push.WithManagerDialFunc(router.dial),
		push.WithReconnectBackoff(100*time.Millisecond, 200*time.Millisecond),
		push.WithReconnectMaxRetries(10),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = mgr.Dial(ctx, svr.addr())

	conn := svr.accept()
	conn.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Subscribe(context.Background(), 1, []*dto.Security{{DataType: 10000, Code: "0700.HK"}})
	}()

	conn2 := svr.accept()
	if conn2 == nil {
		t.Fatal("expected reconnect connection")
	}
	defer conn2.Close()

	wg.Wait()
}

func TestManager_UpdatesChannelFull(t *testing.T) {
	gw := newFakeGateway(t)
	t.Cleanup(func() { gw.stop() })

	mgr := push.NewManager(nil, push.WithManagerDialFunc(func(ctx context.Context, addr string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, "tcp", gw.addr())
	}))
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Dial(ctx, gw.addr()); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	conn := gw.accept(t)
	defer conn.Close()

	topicID := 1
	sec := []*dto.Security{{DataType: 10000, Code: "0700.HK"}}
	_ = mgr.Subscribe(ctx, topicID, sec)

	sendFrame := func() {
		frame := marshalManagerNotifyFrame(t, types.BasicQotNotifyMsgType, "0700.HK", uint64(time.Now().UnixNano()), &hqnotify.BasicQotNotify{
			Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
			BasicQot: &dto.BasicQot{LastPrice: 100.0},
		})
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, _ = conn.Write(frame)
	}

	for i := 0; i < 200; i++ {
		sendFrame()
	}
}

func TestManager_CloseIdempotentUnderRace(t *testing.T) {
	gw := newFakeGateway(t)
	t.Cleanup(func() { gw.stop() })

	mgr := push.NewManager(nil, push.WithManagerDialFunc(func(ctx context.Context, addr string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, "tcp", gw.addr())
	}))
	t.Cleanup(func() { _ = mgr.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = mgr.Dial(ctx, gw.addr())
	_ = gw.accept(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = mgr.Close()
			}
		}()
	}
	wg.Wait()
}
