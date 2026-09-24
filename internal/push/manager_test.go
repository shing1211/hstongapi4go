// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push_test

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func TestManagerGoldenFrameDecode(t *testing.T) {
	gw := newFakeGateway(t)
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

	frame := marshalManagerNotifyFrame(t, types.BasicQotNotifyMsgType, "0700.HK", 1700000000000, &hqnotify.BasicQotNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		BasicQot: &dto.BasicQot{LastPrice: 388.0, OpenPrice: 385.0, HighPrice: 390.0, LowPrice: 384.0, Volume: 1000000},
	})
	writeFrame(t, conn, frame)

	select {
	case evt := <-mgr.Updates():
		if evt == nil {
			t.Fatal("received nil event")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestManagerCleanShutdown(t *testing.T) {
	mgr := push.NewManager(nil)

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case _, ok := <-mgr.Updates():
		if ok {
			t.Error("Updates channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Updates channel should be closed immediately after Close")
	}

	select {
	case _, ok := <-mgr.Errors():
		if ok {
			t.Error("Errors channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Errors channel should be closed immediately after Close")
	}
}

func TestManagerCloseIsIdempotent(t *testing.T) {
	mgr := push.NewManager(nil)
	if err := mgr.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

type managerAddrRouter struct {
	mu   sync.Mutex
	addr string
}

func (r *managerAddrRouter) set(addr string) {
	r.mu.Lock()
	r.addr = addr
	r.mu.Unlock()
}

func (r *managerAddrRouter) dial(ctx context.Context, addr string) (net.Conn, error) {
	r.mu.Lock()
	actualAddr := r.addr
	r.mu.Unlock()
	d := &net.Dialer{}
	return d.DialContext(ctx, "tcp", actualAddr)
}

func marshalManagerNotifyFrame(t *testing.T, typ types.NotifyMsgType, id string, ts uint64, payload proto.Message) []byte {
	t.Helper()
	var anyPayload *anypb.Any
	if payload != nil {
		a, err := anypb.New(payload)
		if err != nil {
			t.Fatalf("anypb.New: %v", err)
		}
		a.TypeUrl = "type.googleapis.com/does.not.Exist"
		anyPayload = a
	}
	n := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(typ),
		NotifyId:      id,
		NotifyTime:    ts,
		Payload:       anyPayload,
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
