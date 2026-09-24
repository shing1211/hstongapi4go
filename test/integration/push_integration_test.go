// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

type mockServerAddrRouter struct {
	ln         net.Listener
	connCh     chan net.Conn
	stopCh     chan struct{}
	wg         sync.WaitGroup
	listenAddr string
}

func newMockPushServer() *mockServerAddrRouter {
	return &mockServerAddrRouter{
		connCh: make(chan net.Conn, 4),
		stopCh: make(chan struct{}),
	}
}

func (r *mockServerAddrRouter) addr() string {
	if r.ln == nil {
		return ""
	}
	return r.ln.Addr().String()
}

func (r *mockServerAddrRouter) start(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	r.ln = ln
	r.listenAddr = ln.Addr().String()

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

func (r *mockServerAddrRouter) stop() {
	close(r.stopCh)
	if r.ln != nil {
		_ = r.ln.Close()
	}
	r.wg.Wait()
}

func (r *mockServerAddrRouter) accept() net.Conn {
	select {
	case c := <-r.connCh:
		return c
	case <-r.stopCh:
		return nil
	}
}

func marshalPushFrame(t *testing.T, msgType types.NotifyMsgType, id string, payload proto.Message) []byte {
	t.Helper()
	a, err := anypb.New(payload)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	n := &pbmsg.PBNotify{
		NotifyMsgType: constant.NotifyMsgType(msgType),
		NotifyId:      id,
		NotifyTime:    uint64(time.Now().UnixMilli()),
		Payload:       a,
	}
	body, err := proto.Marshal(n)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	var buf bytes.Buffer
	if err := push.WriteFrame(&buf, push.Header{MsgType: push.MsgPush, SerialNo: 1}, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.Bytes()
}

func TestPushManager_ConnectAndReceive(t *testing.T) {
	if os.Getenv(envGate) != "1" {
		t.Skip("push integration test skipped: set " + envGate + "=1 to run")
	}

	svr := newMockPushServer()
	svr.start(t)
	defer svr.stop()

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, "tcp", svr.addr())
	}

	mgr := push.NewManager(nil,
		push.WithManagerDialFunc(dialer),
		push.WithReconnectBackoff(50*time.Millisecond, 200*time.Millisecond),
	)
	defer func() { _ = mgr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Dial(ctx, svr.addr()); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	conn := svr.accept()
	if conn == nil {
		t.Fatal("expected connection")
	}

	topicID := int(types.TopicBasicQot)
	sec := []*dto.Security{{DataType: 10000, Code: "0700.HK"}}
	_ = mgr.Subscribe(ctx, topicID, sec)

	frame := marshalPushFrame(t, types.BasicQotNotifyMsgType, "0700.HK", &hqnotify.BasicQotNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		BasicQot: &dto.BasicQot{LastPrice: 388.0, OpenPrice: 385.0},
	})
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Write(frame)

	select {
	case ev := <-mgr.Updates():
		if ev == nil {
			t.Error("received nil event")
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestPushManager_MultipleEmits(t *testing.T) {
	if os.Getenv(envGate) != "1" {
		t.Skip("push integration test skipped: set " + envGate + "=1 to run")
	}

	svr := newMockPushServer()
	svr.start(t)
	defer svr.stop()

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, "tcp", svr.addr())
	}

	mgr := push.NewManager(nil, push.WithManagerDialFunc(dialer))
	defer func() { _ = mgr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = mgr.Dial(ctx, svr.addr())
	conn := svr.accept()
	_ = mgr.Subscribe(ctx, int(types.TopicBasicQot), []*dto.Security{{DataType: 10000, Code: "0700.HK"}})

	for i := 0; i < 5; i++ {
		frame := marshalPushFrame(t, types.BasicQotNotifyMsgType, "0700.HK", &hqnotify.BasicQotNotify{
			Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
			BasicQot: &dto.BasicQot{LastPrice: float64(380 + i)},
		})
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, _ = conn.Write(frame)
	}

	for i := 0; i < 5; i++ {
		select {
		case <-mgr.Updates():
		case <-time.After(2 * time.Second):
			t.Errorf("timeout waiting for event %d", i+1)
			return
		}
	}
}

func TestPushManager_CloseCleanShutdown(t *testing.T) {
	if os.Getenv(envGate) != "1" {
		t.Skip("push integration test skipped: set " + envGate + "=1 to run")
	}

	svr := newMockPushServer()
	svr.start(t)

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		d := &net.Dialer{}
		return d.DialContext(ctx, "tcp", svr.addr())
	}

	mgr := push.NewManager(nil, push.WithManagerDialFunc(dialer))

	ctx := context.Background()
	_ = mgr.Dial(ctx, svr.addr())
	_ = svr.accept()

	_ = mgr.Close()
	svr.stop()

	select {
	case <-mgr.Done():
	case <-time.After(3 * time.Second):
		t.Error("Manager should have shut down")
	}
}
