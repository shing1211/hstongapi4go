// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

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

// signedBody builds a BasicQotNotify PBNotify body.
func signedBody(t *testing.T, code string) []byte {
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
		Payload:       a,
	})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return body
}

// signBody returns a SHA1WithRSA signature of body.
func signBody(t *testing.T, key *rsa.PrivateKey, body []byte) []byte {
	t.Helper()
	sum := sha1.Sum(body)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, sum[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	return sig
}

// frameWith assembles a push frame carrying body and the given signature.
func frameWith(t *testing.T, body, sig []byte) []byte {
	t.Helper()
	hdr := push.Header{MsgType: push.MsgPush}
	copy(hdr.BodySHA1[:], sig)
	var buf bytes.Buffer
	if err := push.WriteFrame(&buf, hdr, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.Bytes()
}

// signedFrame assembles a push frame whose bodySHA1 is a SHA1WithRSA signature
// of body.
func signedFrame(t *testing.T, key *rsa.PrivateKey, body []byte) []byte {
	t.Helper()
	return frameWith(t, body, signBody(t, key, body))
}

// TestStreamVerifiesPushSignature proves the stream client passes the client's
// verification configuration through to the push channel: a correctly signed
// frame is delivered, and a tampered frame is dropped and surfaced on the
// subscription Errors channel.
func TestStreamVerifiesPushSignature(t *testing.T) {
	rec := newHTTPRecorder()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	gw := newFakeGateway(t)

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	t.Setenv(client.EnvVerifyPush, "true")
	c, err := client.New(
		client.WithBaseURL(srv.URL),
		client.WithPushAddr(gw.addr()),
		client.WithEnv(),
		client.WithPlatformPublicKey(string(pubPEM)),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if !c.VerifyPush() {
		t.Fatal("VerifyPush = false, want true")
	}

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

	body := signedBody(t, "0700.HK")
	if _, err := conn.Write(signedFrame(t, key, body)); err != nil {
		t.Fatalf("conn.Write: %v", err)
	}
	select {
	case ev := <-sub.Updates():
		if ev.ID != "0700.HK" {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a verified event")
	}

	tampered := append([]byte(nil), body...)
	tampered[len(tampered)-1] ^= 0xff
	// The signature is over the original body, so verification of the tampered
	// body must fail.
	if _, err := conn.Write(frameWith(t, tampered, signBody(t, key, body))); err != nil {
		t.Fatalf("conn.Write tampered: %v", err)
	}
	select {
	case err := <-sub.Errors():
		if !errors.Is(err, push.ErrSignatureMismatch) {
			t.Fatalf("Errors() = %v, want ErrSignatureMismatch", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a verification error")
	}
}
