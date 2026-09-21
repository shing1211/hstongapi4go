// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
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

// newTestKey generates a throwaway RSA-1024 key for signature tests.
func newTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return key
}

// notifyBody marshals a PBNotify with the given payload and a deliberately
// bogus Any type_url, matching the client's notifyMsgType dispatch.
func notifyBody(t *testing.T, typ types.NotifyMsgType, id string, payload proto.Message) []byte {
	t.Helper()
	var a *anypb.Any
	if payload != nil {
		var err error
		a, err = anypb.New(payload)
		if err != nil {
			t.Fatalf("anypb.New: %v", err)
		}
		a.TypeUrl = "type.googleapis.com/does.not.Exist"
	}
	body, err := proto.Marshal(&pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(typ),
		NotifyId:      id,
		Payload:       a,
	})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return body
}

// signBody returns a PKCS#1 v1.5 SHA1WithRSA signature of body.
func signBody(t *testing.T, key *rsa.PrivateKey, body []byte) []byte {
	t.Helper()
	sum := sha1.Sum(body)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, sum[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	return sig
}

// assembleFrame writes a push frame carrying body and sig.
func assembleFrame(t *testing.T, body, sig []byte) []byte {
	t.Helper()
	hdr := push.Header{MsgType: push.MsgPush}
	copy(hdr.BodySHA1[:], sig)
	var buf bytes.Buffer
	if err := push.WriteFrame(&buf, hdr, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.Bytes()
}

func TestParsePublicKeyBundledConstants(t *testing.T) {
	for name, encoded := range map[string]string{
		"test": types.PlatformPublicKeyTest,
		"prod": types.PlatformPublicKeyProd,
	} {
		key, err := push.ParsePublicKey([]byte(encoded))
		if err != nil {
			t.Fatalf("%s key: ParsePublicKey: %v", name, err)
		}
		if key.N.BitLen() != 1024 || key.E != 65537 {
			t.Fatalf("%s key = %d-bit e=%d, want 1024-bit e=65537", name, key.N.BitLen(), key.E)
		}
	}
}

func TestParsePublicKeyPEMAndDER(t *testing.T) {
	key := newTestKey(t)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	for name, encoded := range map[string][]byte{
		"pem":       pemBlock,
		"raw-der":   der,
		"base64":    []byte(base64.StdEncoding.EncodeToString(der)),
		"pkcs1-der": x509.MarshalPKCS1PublicKey(&key.PublicKey),
	} {
		got, err := push.ParsePublicKey(encoded)
		if err != nil {
			t.Fatalf("%s: ParsePublicKey: %v", name, err)
		}
		if got.N.Cmp(key.N) != 0 {
			t.Fatalf("%s: parsed key does not match", name)
		}
	}
}

func TestParsePublicKeyRejectsInvalid(t *testing.T) {
	for name, encoded := range map[string][]byte{
		"empty":   nil,
		"garbage": []byte("not a key"),
	} {
		if _, err := push.ParsePublicKey(encoded); !errors.Is(err, push.ErrUnsupportedKey) {
			t.Fatalf("%s: err = %v, want ErrUnsupportedKey", name, err)
		}
	}
}

func TestVerifierRoundTrip(t *testing.T) {
	key := newTestKey(t)
	body := []byte("hello push body")
	sig := signBody(t, key, body)

	var hdr push.Header
	copy(hdr.BodySHA1[:], sig)

	v := push.NewVerifier(&key.PublicKey, true)
	if !v.Required() {
		t.Fatal("Required() = false, want true")
	}
	if err := v.Verify(hdr, body); err != nil {
		t.Fatalf("Verify(valid): %v", err)
	}

	tampered := append([]byte(nil), body...)
	tampered[0] ^= 0xff
	if err := v.Verify(hdr, tampered); !errors.Is(err, push.ErrSignatureMismatch) {
		t.Fatalf("Verify(tampered body) = %v, want ErrSignatureMismatch", err)
	}

	var zero push.Header
	if err := v.Verify(zero, body); !errors.Is(err, push.ErrMissingSignature) {
		t.Fatalf("Verify(zero sig) = %v, want ErrMissingSignature", err)
	}

	if err := push.NewVerifier(nil, true).Verify(hdr, body); !errors.Is(err, push.ErrNoPublicKey) {
		t.Fatalf("Verify(nil key) = %v, want ErrNoPublicKey", err)
	}
	if push.NewVerifier(&key.PublicKey, false).Required() {
		t.Fatal("Required() = true, want false")
	}
}

func TestClientVerificationDeliversSignedFrame(t *testing.T) {
	key := newTestKey(t)
	gw := newFakeGateway(t)
	pc := push.New(
		push.WithAddr(gw.addr()),
		push.WithReconnect(10*time.Millisecond, 50*time.Millisecond),
		push.WithVerification(x509.MarshalPKCS1PublicKey(&key.PublicKey), true),
	)
	t.Cleanup(func() { _ = pc.Close() })
	if !pc.VerificationEnabled() || !pc.VerificationRequired() {
		t.Fatal("verification not enabled/required after WithVerification")
	}

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
	body := notifyBody(t, types.BasicQotNotifyMsgType, "0700.HK",
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0700.HK"}})
	writeFrame(t, conn, assembleFrame(t, body, signBody(t, key, body)))

	if n := waitNotification(t, got); n.ID != "0700.HK" {
		t.Fatalf("notification = %+v", n)
	}
}

func TestClientVerificationDropsTamperedFrame(t *testing.T) {
	key := newTestKey(t)
	gw := newFakeGateway(t)
	pc := push.New(
		push.WithAddr(gw.addr()),
		push.WithReconnect(10*time.Millisecond, 50*time.Millisecond),
		push.WithVerification(x509.MarshalPKCS1PublicKey(&key.PublicKey), true),
	)
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

	body := notifyBody(t, types.BasicQotNotifyMsgType, "0700.HK",
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0700.HK"}})
	sig := signBody(t, key, body)
	tampered := append([]byte(nil), body...)
	tampered[len(tampered)-1] ^= 0xff
	writeFrame(t, conn, assembleFrame(t, tampered, sig))

	select {
	case err := <-pc.Errors():
		if !errors.Is(err, push.ErrSignatureMismatch) {
			t.Fatalf("Errors() = %v, want ErrSignatureMismatch", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a verification error")
	}

	// A valid frame written after the tampered one must be the next
	// notification: the read loop processes frames in wire order, so a dropped
	// frame cannot be delivered later. This is a deterministic substitute for a
	// sleep-based absence check.
	good := notifyBody(t, types.BasicQotNotifyMsgType, "0005.HK",
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0005.HK"}})
	writeFrame(t, conn, assembleFrame(t, good, signBody(t, key, good)))
	if n := waitNotification(t, got); n.ID != "0005.HK" {
		t.Fatalf("notification = %+v, want the valid 0005.HK frame after the dropped one", n)
	}
}

func TestClientVerificationOptionalReportsButDelivers(t *testing.T) {
	key := newTestKey(t)
	gw := newFakeGateway(t)
	pc := push.New(
		push.WithAddr(gw.addr()),
		push.WithReconnect(10*time.Millisecond, 50*time.Millisecond),
		push.WithVerification(x509.MarshalPKCS1PublicKey(&key.PublicKey), false),
	)
	t.Cleanup(func() { _ = pc.Close() })
	if pc.VerificationRequired() {
		t.Fatal("VerificationRequired() = true, want false")
	}

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
	body := notifyBody(t, types.BasicQotNotifyMsgType, "0700.HK",
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0700.HK"}})
	writeFrame(t, conn, assembleFrame(t, body, make([]byte, push.BodySHA1Len)))
	// An all-zero signature is missing; the optional policy still delivers the
	// frame and reports the problem.
	select {
	case n := <-got:
		if n.ID != "0700.HK" {
			t.Fatalf("notification = %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the optional-verification frame")
	}
	select {
	case err := <-pc.Errors():
		if !errors.Is(err, push.ErrMissingSignature) {
			t.Fatalf("Errors() = %v, want ErrMissingSignature", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a verification error")
	}
}

func TestClientVerificationDisabledAcceptsUnsignedFrame(t *testing.T) {
	gw := newFakeGateway(t)
	pc := push.New(
		push.WithAddr(gw.addr()),
		push.WithReconnect(10*time.Millisecond, 50*time.Millisecond),
	)
	t.Cleanup(func() { _ = pc.Close() })
	if pc.VerificationEnabled() {
		t.Fatal("VerificationEnabled() = true, want false by default")
	}

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
	body := notifyBody(t, types.BasicQotNotifyMsgType, "0700.HK",
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "0700.HK"}})
	// A non-zero bogus signature must be ignored when verification is off.
	bogus := bytes.Repeat([]byte{0xAB}, push.BodySHA1Len)
	writeFrame(t, conn, assembleFrame(t, body, bogus))

	if n := waitNotification(t, got); n.ID != "0700.HK" {
		t.Fatalf("notification = %+v", n)
	}
}

func TestClientVerificationBadKeyFailsConnect(t *testing.T) {
	pc := push.New(push.WithAddr("127.0.0.1:1"), push.WithVerification([]byte("not-a-key"), true))
	t.Cleanup(func() { _ = pc.Close() })
	if err := pc.Connect(context.Background()); !errors.Is(err, push.ErrUnsupportedKey) {
		t.Fatalf("Connect = %v, want ErrUnsupportedKey", err)
	}
}
