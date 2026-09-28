// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- the Gateway's own push signature is SHA1WithRSA; this test reproduces the wire format, it does not choose an algorithm
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
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
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests here drive PushAdapter over the real internal/push client, over a
// net.Pipe installed by a substituted dialer. There is no listener, no port and no
// Gateway, so the frames the adapter converts are the bytes that would have arrived
// on the socket - which is the point: a test that handed the adapter a
// pre-built domain.PushUpdate would be asserting the test's own construction.
//
// Nothing sleeps and nothing waits on a clock. Frames are written to a pipe the
// test holds, a reconnect is produced by closing that pipe (so the read loop gets a
// real io.EOF rather than a synthesised one), and every goroutine is joined with a
// WaitGroup. No test calls recover().

// waitBound is the ceiling a test waits on an event. Reaching it is always a
// failure, never a way to reach a code path.
const waitBound = 30 * time.Second

// pipeDialer substitutes the push connection with in-memory pipes.
//
// The peer end of every pipe is retained so the test can write frames to it or
// close it; closing a peer is how a reconnect is produced here, because a closed
// pipe makes the push read loop return io.EOF, which is the real Gateway hangup
// the reconnect notice has to carry.
//
// dialed is the handshake that makes the read loop reachable without a timer: the
// dialer signals it on every dial, and a test takes the peer it is waiting for out
// of that signal. The channel is generously buffered and the send is non-blocking,
// so the read loop is never stalled by a test that has not asked for a dial yet.
type pipeDialer struct {
	mu       sync.Mutex
	dials    int
	consumed int
	peers    []net.Conn
	dialed   chan struct{}
}

// dial implements the adapter's DialFunc.
func (d *pipeDialer) dial(_ context.Context, _, _ string) (net.Conn, error) {
	local, peer := net.Pipe()
	d.mu.Lock()
	d.dials++
	d.peers = append(d.peers, peer)
	d.mu.Unlock()
	select {
	case d.dialed <- struct{}{}:
	default:
	}
	return local, nil
}

// awaitPeer returns the remote end of the i-th pipe, waiting on the dialer's own
// signal rather than on a clock. Reaching waitBound is a failure.
//
// The count of consumed signals is tracked, so asking for pipe 1 after pipe 0 waits
// for exactly one more dial rather than re-counting the one already taken - which
// would otherwise hang on a reconnected connection that had already happened.
func (d *pipeDialer) awaitPeer(t *testing.T, i int) net.Conn {
	t.Helper()
	for {
		d.mu.Lock()
		drained := d.consumed
		d.mu.Unlock()
		if drained > i {
			d.mu.Lock()
			peer := d.peers[i]
			d.mu.Unlock()
			return peer
		}
		select {
		case <-d.dialed:
			d.mu.Lock()
			d.consumed++
			d.mu.Unlock()
		case <-time.After(waitBound):
			t.Fatalf("the dialer never reached pipe %d; it ran %d time(s)", i, d.count())
		}
	}
}

// peerOf returns the remote end of the i-th pipe without waiting for it, for the
// tests that have already awaited that dial.
func (d *pipeDialer) peerOf(i int) net.Conn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peers[i]
}

// count reports how many times the dialer ran.
func (d *pipeDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

// adapterHarness is an adapter plus the pipe dialer behind it, with the read loop
// already running and a WaitGroup to join it.
type adapterHarness struct {
	adapter *PushAdapter
	dialer  *pipeDialer
	runDone sync.WaitGroup
}

// newAdapterHarness builds an adapter over a pipe dialer, registers cleanup that
// closes the adapter and then every retained pipe peer, and starts the read loop.
//
// Cleanup runs last-registered-first, so the adapter closes before the peers: no
// read is left blocked on a pipe whose peer has already gone.
func newAdapterHarness(t *testing.T, opts ...PushOption) *adapterHarness {
	t.Helper()
	d := &pipeDialer{dialed: make(chan struct{}, 256)}
	all := append([]PushOption{WithPushDialer(d.dial), WithPushReconnect(10*time.Millisecond, 50*time.Millisecond)}, opts...)
	a, err := NewPushAdapter(all...)
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	h := &adapterHarness{adapter: a, dialer: d}
	// Cleanup runs last-registered-first, so the adapter closes before the peers:
	// no read is left blocked on a pipe whose peer has already gone, and the read
	// loop has been joined before any pipe is touched.
	t.Cleanup(func() {
		_ = a.Close()
		h.runDone.Wait()
		d.mu.Lock()
		peers := d.peers
		d.peers = nil
		d.mu.Unlock()
		for _, p := range peers {
			_ = p.Close()
		}
	})
	h.runDone.Add(1)
	go func() {
		defer h.runDone.Done()
		_ = a.Run(context.Background())
	}()
	return h
}

// notifyFrameBody marshals a PBNotify carrying payload, with a deliberately bogus
// Any type_url - the vendored protos declare no proto package, so the push client
// dispatches on notifyMsgType and never consults the registry.
func notifyFrameBody(t *testing.T, msgType types.NotifyMsgType, id string, notifyTime uint64, payload proto.Message) []byte {
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
		NotifyMsgType: pbconstant.NotifyMsgType(msgType),
		NotifyId:      id,
		NotifyTime:    notifyTime,
		Payload:       a,
	})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return body
}

// pushFrame renders a complete push frame carrying body and sig.
func pushFrame(t *testing.T, body, sig []byte) []byte {
	t.Helper()
	hdr := push.Header{MsgType: push.MsgPush}
	copy(hdr.BodySHA1[:], sig)
	var buf strings.Builder
	if err := push.WriteFrame(&stringWriter{&buf}, hdr, body); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return []byte(buf.String())
}

// stringWriter adapts a strings.Builder to io.Writer.
type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// signBody returns a PKCS#1 v1.5 SHA1WithRSA signature of body, the algorithm the
// Gateway uses for the frame's bodySHA1 field.
func signBody(t *testing.T, key *rsa.PrivateKey, body []byte) []byte {
	t.Helper()
	sum := sha1.Sum(body)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, sum[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	return sig
}

// nextUpdate reads one delivered update, failing the test when none arrives.
func nextUpdate(t *testing.T, got <-chan *domain.PushUpdate) *domain.PushUpdate {
	t.Helper()
	select {
	case u, ok := <-got:
		if !ok {
			t.Fatal("the update channel closed, want an update")
		}
		return u
	case <-time.After(waitBound):
		t.Fatal("timed out waiting for a delivered update")
		return nil
	}
}

// nextAdapterError reads one error off the adapter's stream, failing when none
// arrives.
func nextAdapterError(t *testing.T, a *PushAdapter) error {
	t.Helper()
	select {
	case err, ok := <-a.Errors():
		if !ok {
			t.Fatal("the error stream closed, want an error")
		}
		return err
	case <-time.After(waitBound):
		t.Fatal("timed out waiting for an adapter error")
		return nil
	}
}

// ---------------------------------------------------------------------------
// The seam's conversion
// ---------------------------------------------------------------------------

// TestTheAdapterDeliversADomainEvent is the seam's whole purpose: a frame on the
// wire becomes a *domain.PushUpdate carrying a typed domain event and the envelope
// facts.
//
// The assertion is on the *type* of Event, not on a rendered string: the released
// layer hands the caller a Payload any and makes every accessor return a bool,
// which is the defect the v-next domain types exist to remove. A frame delivered
// here must need no bool to read.
func TestTheAdapterDeliversADomainEvent(t *testing.T) {
	h := newAdapterHarness(t)
	got := make(chan *domain.PushUpdate, 4)
	h.adapter.SubscribeTypes(types.BasicQotNotifyMsgType, func(u *domain.PushUpdate) { got <- u })

	body := notifyFrameBody(t, types.BasicQotNotifyMsgType, "00700.HK", 1_700_000_000_000,
		&hqnotify.BasicQotNotify{
			Security: &dto.Security{DataType: 10000, Code: "00700.HK"},
			BasicQot: &dto.BasicQot{LastPrice: 387.05, Volume: 1200},
		})
	if _, err := h.dialer.awaitPeer(t, 0).Write(pushFrame(t, body, nil)); err != nil {
		t.Fatalf("writing the frame: %v", err)
	}

	u := nextUpdate(t, got)
	if u.Type != types.BasicQotNotifyMsgType {
		t.Errorf("Type = %s, want BasicQotNotifyMsgType", u.Type.String())
	}
	if u.ID != "00700.HK" {
		t.Errorf("ID = %q, want the notifyId verbatim", u.ID)
	}
	if want := time.UnixMilli(1_700_000_000_000).UTC(); !u.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", u.Time, want)
	}
	if u.Time.Location() != time.UTC {
		t.Errorf("Time location = %v, want UTC", u.Time.Location())
	}
	if u.Event == nil {
		t.Fatal("Event is nil; domain.PushUpdate documents it as never nil")
	}
	quote, ok := u.Event.(domain.QuoteEvent)
	if !ok {
		t.Fatalf("Event = %T, want domain.QuoteEvent with no bool to check", u.Event)
	}
	if got, want := quote.LastPrice.String(), "387.05"; got != want {
		t.Errorf("LastPrice = %q, want %q", got, want)
	}
	if got, want := u.SecurityCode(), "00700.HK"; got != want {
		t.Errorf("SecurityCode = %q, want %q: the routing key must be readable from the envelope", got, want)
	}
}

// TestTheAdapterDeliversEveryPayloadType feeds one frame per payload type and
// requires a correctly typed event for each, so the seam is proven for the whole
// handler set the orchestrator installs rather than for one type.
func TestTheAdapterDeliversEveryPayloadType(t *testing.T) {
	cases := []struct {
		msgType types.NotifyMsgType
		payload proto.Message
		want    any
	}{
		{
			msgType: types.BasicQotNotifyMsgType,
			payload: &hqnotify.BasicQotNotify{Security: &dto.Security{Code: "00700.HK"}, BasicQot: &dto.BasicQot{LastPrice: 1}},
			want:    domain.QuoteEvent{},
		},
		{
			msgType: types.TickerNotifyMsgType,
			payload: &hqnotify.TickerNotify{Security: &dto.Security{Code: "00700.HK"}, Ticker: &dto.Ticker{Price: 1}},
			want:    domain.TickerEvent{},
		},
		{
			msgType: types.OrderBookNotifyMsgType,
			payload: &hqnotify.OrderBookFullNotify{Security: &dto.Security{Code: "00700.HK"}},
			want:    domain.OrderBookEvent{},
		},
		{
			msgType: types.BrokerQueueNotifyMsgType,
			payload: &hqnotify.BrokerNotify{Security: &dto.Security{Code: "00700.HK"}},
			want:    domain.BrokerEvent{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.msgType.String(), func(t *testing.T) {
			h := newAdapterHarness(t)
			got := make(chan *domain.PushUpdate, 2)
			h.adapter.SubscribeTypes(tc.msgType, func(u *domain.PushUpdate) { got <- u })

			body := notifyFrameBody(t, tc.msgType, "00700.HK", 1_700_000_000_000, tc.payload)
			if _, err := h.dialer.awaitPeer(t, 0).Write(pushFrame(t, body, nil)); err != nil {
				t.Fatalf("writing the frame: %v", err)
			}
			u := nextUpdate(t, got)
			if fmt.Sprintf("%T", u.Event) != fmt.Sprintf("%T", tc.want) {
				t.Errorf("Event = %T, want %T", u.Event, tc.want)
			}
		})
	}
}

// TestAnEmptyPayloadArrivesAsASystemEvent is the substitution's end-to-end half.
//
// Normalize would have returned (nil, nil) for this frame; the seam's contract is
// that a delivered update always carries an event, so the caller gets a
// SystemEvent with a code it can branch on instead of a nil it has to nil-check.
func TestAnEmptyPayloadArrivesAsASystemEvent(t *testing.T) {
	h := newAdapterHarness(t)
	got := make(chan *domain.PushUpdate, 2)
	h.adapter.SubscribeTypes(types.BasicQotNotifyMsgType, func(u *domain.PushUpdate) { got <- u })

	body := notifyFrameBody(t, types.BasicQotNotifyMsgType, "00700.HK", 1_700_000_000_000, nil)
	if _, err := h.dialer.awaitPeer(t, 0).Write(pushFrame(t, body, nil)); err != nil {
		t.Fatalf("writing the frame: %v", err)
	}

	u := nextUpdate(t, got)
	sys, ok := u.Event.(domain.SystemEvent)
	if !ok {
		t.Fatalf("Event = %T, want domain.SystemEvent", u.Event)
	}
	if sys.Code != "EMPTY_PAYLOAD" {
		t.Errorf("Code = %q, want %q", sys.Code, "EMPTY_PAYLOAD")
	}
	if sys.Level != "warn" {
		t.Errorf("Level = %q, want %q", sys.Level, "warn")
	}
	// The envelope survives the substitution: Type, ID and Time are frame facts and
	// are exactly what routing and staleness read.
	if u.ID != "00700.HK" || u.Type != types.BasicQotNotifyMsgType || u.Time.IsZero() {
		t.Errorf("envelope = %+v, want the frame's Type, ID and Time preserved", u)
	}
}

// TestANilHandlerDiscardsRatherThanDelivers covers the one reachable guard inside
// the seam's closure: SubscribeTypes with a nil handler still installs the closure,
// which must then discard frames rather than dereference a nil callback.
func TestANilHandlerDiscardsRatherThanDelivers(t *testing.T) {
	h := newAdapterHarness(t)
	h.adapter.SubscribeTypes(types.BasicQotNotifyMsgType, nil)

	// A frame still arrives and is discarded. The deterministic proof that it was
	// processed without panicking is the *next* frame for a type with a handler: the
	// read loop processes frames in wire order, so a panic or a stall in the nil
	// handler's frame would be visible there.
	got := make(chan *domain.PushUpdate, 2)
	h.adapter.SubscribeTypes(types.TickerNotifyMsgType, func(u *domain.PushUpdate) { got <- u })

	quiet := notifyFrameBody(t, types.BasicQotNotifyMsgType, "00700.HK", 1_700_000_000_000,
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "00700.HK"}})
	peer := h.dialer.awaitPeer(t, 0)
	if _, err := peer.Write(pushFrame(t, quiet, nil)); err != nil {
		t.Fatalf("writing the frame with no handler: %v", err)
	}
	loud := notifyFrameBody(t, types.TickerNotifyMsgType, "00700.HK", 1_700_000_000_001,
		&hqnotify.TickerNotify{Security: &dto.Security{Code: "00700.HK"}, Ticker: &dto.Ticker{Price: 2}})
	if _, err := peer.Write(pushFrame(t, loud, nil)); err != nil {
		t.Fatalf("writing the frame with a handler: %v", err)
	}
	if u := nextUpdate(t, got); u.Type != types.TickerNotifyMsgType {
		t.Errorf("delivered %s, want the TickerNotify frame that followed the discarded one", u.Type.String())
	}
}

// ---------------------------------------------------------------------------
// A4's T7: a real reconnect over a real socket failure
// ---------------------------------------------------------------------------

// TestTheReconnectCauseIsTheReadFailuresEOF is A4's T7, the end-to-end proof the
// released layer still owes.
//
// The peer end of the pipe is closed, so the push read loop returns io.EOF - the
// real Gateway hangup - and the client reconnects and fires its hook. The cause is
// asserted by identity, because that is the operational distinction a caller has to
// make: "the Gateway hung up, expect a gap" is not "the read deadline expired, the
// feed is stale".
//
// No sleep: the reconnect is awaited by waiting for the second dial, which is a
// fact about the dialer rather than about elapsed time.
func TestTheReconnectCauseIsTheReadFailuresEOF(t *testing.T) {
	h := newAdapterHarness(t)

	type reconnect struct {
		cause error
	}
	seen := make(chan reconnect, 4)
	h.adapter.OnReconnect(func(cause error) { seen <- reconnect{cause: cause} })
	h.adapter.OnReconnect(nil) // must be a no-op, not a nil hook in the fan-out

	h.dialer.awaitPeer(t, 0)
	if err := h.dialer.peerOf(0).Close(); err != nil {
		t.Fatalf("closing the peer: %v", err)
	}

	select {
	case got := <-seen:
		if got.cause != io.EOF {
			t.Errorf("reconnect cause = %v, want io.EOF", got.cause)
		}
	case <-time.After(waitBound):
		t.Fatal("timed out waiting for a reconnect")
	}

	// The second pipe is the reconnected connection; taking it is the proof that
	// the reconnect completed rather than only being signalled.
	h.dialer.awaitPeer(t, 1)
	if got := h.dialer.count(); got < 2 {
		t.Errorf("the dialer ran %d time(s), want a reconnect", got)
	}
}

// TestTheReconnectHookListGrowsAfterConstruction covers the adapter's own hook
// registry, which exists because the push client takes its hooks as a construction
// option and the orchestrator registers its hook after the adapter is built.
func TestTheReconnectHookListGrowsAfterConstruction(t *testing.T) {
	a, err := NewPushAdapter(WithPushDialer(func(context.Context, string, string) (net.Conn, error) {
		local, peer := net.Pipe()
		t.Cleanup(func() { _ = peer.Close() })
		return local, nil
	}))
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	var first, second int
	a.OnReconnect(func(error) { first++ })
	a.OnReconnect(func(error) { second++ })
	a.OnReconnect(nil)

	a.fireReconnect(io.EOF)
	if first != 1 || second != 1 {
		t.Errorf("hooks ran %d and %d times, want 1 each", first, second)
	}

	// Registering from inside a hook must not deadlock on the hook mutex.
	a.OnReconnect(func(error) { second++ })
	a.fireReconnect(io.ErrUnexpectedEOF)
	if first != 2 || second != 3 {
		t.Errorf("after registering from inside a hook: %d and %d, want 2 and 3", first, second)
	}
}

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

// TestAVerifiedFrameIsDeliveredAndATamperedOneIsReported covers the parity target
// from the released layer's verify_test.go, plus §9.1's requirement that the
// failure be reachable through a sentinel a caller outside this module can name.
func TestAVerifiedFrameIsDeliveredAndATamperedOneIsReported(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	encoded := x509.MarshalPKCS1PublicKey(&key.PublicKey)

	h := newAdapterHarness(t, WithPushVerification(encoded, true))
	if !h.adapter.VerificationEnabled() || !h.adapter.VerificationRequired() {
		t.Fatal("verification is not enabled and required after WithPushVerification")
	}
	got := make(chan *domain.PushUpdate, 4)
	h.adapter.SubscribeTypes(types.BasicQotNotifyMsgType, func(u *domain.PushUpdate) { got <- u })

	good := notifyFrameBody(t, types.BasicQotNotifyMsgType, "00700.HK", 1_700_000_000_000,
		&hqnotify.BasicQotNotify{Security: &dto.Security{Code: "00700.HK"}, BasicQot: &dto.BasicQot{LastPrice: 1}})
	tampered := append([]byte(nil), good...)
	tampered[len(tampered)-1] ^= 0xff

	peer := h.dialer.awaitPeer(t, 0)
	if _, err := peer.Write(pushFrame(t, tampered, signBody(t, key, good))); err != nil {
		t.Fatalf("writing the tampered frame: %v", err)
	}

	verdict := nextAdapterError(t, h.adapter)
	if !errors.Is(verdict, push.ErrSignatureMismatch) {
		t.Errorf("the verdict = %v, want push.ErrSignatureMismatch", verdict)
	}
	// The nameable half. A caller outside this module cannot spell internal/push's
	// sentinel, so the adapter pairs it with domain's - the one package both
	// pkg/services and pkg/transport can name - and pkg/services re-exports that.
	if !errors.Is(verdict, domain.ErrSignatureMismatch) {
		t.Errorf("errors.Is(verdict, domain.ErrSignatureMismatch) = false, want true: %v", verdict)
	}
	if got := errors.Unwrap(verdict); got == nil {
		t.Error("errors.Unwrap(verdict) = nil, want the push package's own error: a multi-%w wrap would " +
			"falsify the documented mechanism")
	}

	// No subscription existed when the verdict was raised other than the handler's,
	// and the verdict arrived on the stream regardless - which is the §7.1 hole on
	// the verification path: under the released layer a caller with no subscription
	// learns nothing about a bad frame.
	if _, err := peer.Write(pushFrame(t, good, signBody(t, key, good))); err != nil {
		t.Fatalf("writing the good frame: %v", err)
	}
	if u := nextUpdate(t, got); u.ID != "00700.HK" {
		t.Errorf("delivered %+v, want the valid frame that followed the dropped one", u)
	}
}

// TestAnUnparseableVerificationKeyFailsConnectNotConstruction pins §9.1's third
// decision: construction never fails, and the key parse is a connect-time check.
func TestAnUnparseableVerificationKeyFailsConnectNotConstruction(t *testing.T) {
	a, err := NewPushAdapter(
		WithPushAddr("127.0.0.1:1"),
		WithPushVerification([]byte("not-a-key"), true),
	)
	if err != nil {
		t.Fatalf("NewPushAdapter = %v, want nil: construction must not fail", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if err := a.Connect(context.Background()); !errors.Is(err, push.ErrUnsupportedKey) {
		t.Fatalf("Connect = %v, want ErrUnsupportedKey", err)
	}
	// The unusable key leaves verification off, which is the safe direction: a key
	// that cannot be parsed must not silently become a verifier that accepts
	// everything. Connect is what reports it.
	if a.VerificationEnabled() {
		t.Error("VerificationEnabled = true after an unparseable key: a verifier that cannot check " +
			"anything must not be installed")
	}
}

// TestTheVerificationTranslationKeepsBothTargets unit-tests the pairing directly,
// for both branches.
//
// The two branches exist because the failure modes differ: a signature mismatch is
// the one verdict a caller is expected to branch on and needs the nameable
// sentinel, while anything else - a decode failure, a read error - keeps the push
// package's own value and its text untouched.
func TestTheVerificationTranslationKeepsBothTargets(t *testing.T) {
	a, err := NewPushAdapter(WithPushDialer(func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unused")
	}))
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	t.Run("a signature mismatch gains the shared sentinel", func(t *testing.T) {
		cause := fmt.Errorf("push: reading header: %w", push.ErrSignatureMismatch)
		translated := &unverifiedError{cause: cause}
		if !errors.Is(translated, push.ErrSignatureMismatch) {
			t.Error("errors.Is(push.ErrSignatureMismatch) = false, want the internal sentinel to stay matchable")
		}
		if !errors.Is(translated, domain.ErrSignatureMismatch) {
			t.Error("errors.Is(domain.ErrSignatureMismatch) = false, want the nameable sentinel to match")
		}
		if got := errors.Unwrap(translated); got != cause {
			t.Errorf("errors.Unwrap = %v, want the cause itself", got)
		}
		if got := translated.Error(); got != cause.Error() {
			t.Errorf("Error() = %q, want %q: the wrapping must not change a log line", got, cause.Error())
		}
		if errors.Is(translated, io.EOF) {
			t.Error("errors.Is(io.EOF) = true, want false")
		}
	})

	t.Run("anything else is reported unchanged", func(t *testing.T) {
		cause := errors.New("push: read: connection reset")
		a.Report(cause)
		got := nextAdapterError(t, a)
		if !errors.Is(got, cause) {
			t.Errorf("reported error = %v, want %v", got, cause)
		}
		if errors.Is(got, domain.ErrSignatureMismatch) {
			t.Error("a read failure was dressed as a signature mismatch")
		}
	})

	t.Run("a nil error is not reported", func(t *testing.T) {
		a.Report(nil)
		if err := translatePushError(nil); err != nil {
			t.Errorf("translatePushError(nil) = %v, want nil", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Configuration and conversion units
// ---------------------------------------------------------------------------

// TestTheAdapterDefaults pins the option contract: the documented defaults, and a
// non-positive or empty value restoring each of them rather than being accepted.
func TestTheAdapterDefaults(t *testing.T) {
	a, err := NewPushAdapter()
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if got := a.Addr(); got != DefaultPushAddr {
		t.Errorf("Addr = %q, want %q", got, DefaultPushAddr)
	}
	if a.VerificationEnabled() {
		t.Error("verification is on by default; ADR 0005 keeps it opt-in")
	}
	if got, want := a.String(), "transport.PushAdapter{addr: "+DefaultPushAddr+"}"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if cfg := a.cfg; cfg.MinReconnect != DefaultPushMinReconnect || cfg.MaxReconnect != DefaultPushMaxReconnect {
		t.Errorf("backoff = %v/%v, want %v/%v", cfg.MinReconnect, cfg.MaxReconnect,
			DefaultPushMinReconnect, DefaultPushMaxReconnect)
	}
	if cfg := a.cfg; cfg.ReadDeadline != 0 {
		t.Errorf("ReadDeadline = %v, want 0: the deadline is off by default", cfg.ReadDeadline)
	}

	restored, err := NewPushAdapter(
		WithPushAddr(""),
		WithPushReconnect(0, 0),
		WithPushReadDeadline(-time.Second),
		WithPushDialer(nil),
	)
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	if got := restored.Addr(); got != DefaultPushAddr {
		t.Errorf("Addr after WithPushAddr(\"\") = %q, want %q", got, DefaultPushAddr)
	}
	if cfg := restored.cfg; cfg.MinReconnect != DefaultPushMinReconnect || cfg.MaxReconnect != DefaultPushMaxReconnect {
		t.Errorf("backoff after WithPushReconnect(0, 0) = %v/%v, want the defaults", cfg.MinReconnect, cfg.MaxReconnect)
	}
	if cfg := restored.cfg; cfg.ReadDeadline != 0 {
		t.Errorf("ReadDeadline after a negative value = %v, want 0", cfg.ReadDeadline)
	}

	// A max below min is raised to min rather than producing an inverted backoff.
	raised, err := NewPushAdapter(WithPushReconnect(2*time.Second, time.Millisecond))
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = raised.Close() })
	if got := raised.cfg.MaxReconnect; got != 2*time.Second {
		t.Errorf("MaxReconnect = %v, want it raised to MinReconnect (2s)", got)
	}

	configured, err := NewPushAdapter(WithPushAddr("127.0.0.1:22222"), WithPushReadDeadline(3*time.Second))
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = configured.Close() })
	if got := configured.Addr(); got != "127.0.0.1:22222" {
		t.Errorf("Addr = %q, want the configured address", got)
	}
	if got := configured.cfg.ReadDeadline; got != 3*time.Second {
		t.Errorf("ReadDeadline = %v, want 3s", got)
	}
	// A nil option is a no-op, not a panic.
	if _, err := NewPushAdapter(nil); err != nil {
		t.Errorf("NewPushAdapter(nil) = %v, want nil", err)
	}
}

// TestNotifyTimeMapsZeroToTheZeroTime pins the millisecond conversion, and the
// boundary that matters: a Gateway that sent no notifyTime must not read as
// 1970-01-01, because Stale and the out-of-order check both compare against it.
func TestNotifyTimeMapsZeroToTheZeroTime(t *testing.T) {
	if got := notifyTime(0); !got.IsZero() {
		t.Errorf("notifyTime(0) = %v, want the zero time", got)
	}
	got := notifyTime(1_700_000_000_123)
	want := time.UnixMilli(1_700_000_000_123).UTC()
	if !got.Equal(want) {
		t.Errorf("notifyTime = %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("notifyTime location = %v, want UTC", got.Location())
	}
	if got.Nanosecond() != 123_000_000 {
		t.Errorf("sub-millisecond component = %d ns, want the millisecond preserved exactly", got.Nanosecond())
	}
}

// TestThePushOptionsAreTheAdaptersOwn pins that the adapter translates its public
// configuration into the push client's options rather than sharing the client's - a
// signature naming internal/push's Option would make NewPushAdapter unspellable
// outside this module, which is the cost §4.2.1 rejects for the seam.
func TestThePushOptionsAreTheAdaptersOwn(t *testing.T) {
	opts := pushOptions(PushConfig{
		Addr:         "127.0.0.1:1",
		MinReconnect: time.Second,
		MaxReconnect: 2 * time.Second,
		ReadDeadline: time.Second,
	})
	if len(opts) != 3 {
		t.Errorf("pushOptions produced %d options, want 3 with no dialer and no key", len(opts))
	}
	withKey := pushOptions(PushConfig{Addr: "x", verifyKey: []byte("k"), verifyRequired: true})
	if len(withKey) != 4 {
		t.Errorf("pushOptions with a key produced %d options, want 4", len(withKey))
	}
	withDialer := pushOptions(PushConfig{Addr: "x", Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unused")
	}})
	if len(withDialer) != 4 {
		t.Errorf("pushOptions with a dialer produced %d options, want 4", len(withDialer))
	}
}

// TestCloseIsIdempotentAndClosesTheErrorStream covers the adapter's own lifecycle,
// which the orchestrator's Close depends on.
func TestCloseIsIdempotentAndClosesTheErrorStream(t *testing.T) {
	a, err := NewPushAdapter()
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := a.Close(); err != nil {
			t.Fatalf("Close %d: %v", i, err)
		}
	}
	if _, open := <-a.Errors(); open {
		t.Error("the error stream is still open after Close")
	}
}

// TestTheTranslationGoroutineEndsWhenTheUpstreamStreamCloses covers the upstream
// close, which is a different end from Close's own signal.
//
// The push client's stream closing under the adapter is what a transport that gives
// up the connection does; the adapter's goroutine has to notice and return rather
// than sit on a dead channel, and Close has to be able to join it either way.
func TestTheTranslationGoroutineEndsWhenTheUpstreamStreamCloses(t *testing.T) {
	a, err := NewPushAdapter()
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	// Closing the push client closes its stream, which is the case this covers; the
	// adapter's own done channel is still open, so the goroutine has to notice the
	// upstream close rather than its own signal.
	if err := a.client.Close(); err != nil {
		t.Fatalf("closing the push client: %v", err)
	}
	waitDone := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(waitBound):
		t.Fatal("the translation goroutine did not end when the upstream stream closed")
	}
}

// TestTheAdapterErrorStreamIsBoundedAndCountsItsLosses covers the drop-oldest path
// in emit.
//
// A lost diagnosis is recoverable and an unbounded queue on a flapping connection is a
// memory leak, so the stream is bounded - but a bounded queue is only defensible
// while the oldest entry is really the one that goes, which is what this pins.
func TestTheAdapterErrorStreamIsBoundedAndCountsItsLosses(t *testing.T) {
	a, err := NewPushAdapter()
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	// One more than the stream can hold, so the overflow is certain.
	total := cap(a.errc) + 1
	for i := 0; i < total; i++ {
		a.Report(fmt.Errorf("flapping %d", i))
	}
	if got := len(a.errc); got != cap(a.errc) {
		t.Errorf("the stream holds %d error(s) after %d reports, want the %d-entry capacity: it must "+
			"stay bounded", got, total, cap(a.errc))
	}
	// Drop-oldest: the oldest is gone and the newest is present.
	first, ok := <-a.errc
	if !ok {
		t.Fatal("the stream closed")
	}
	if !strings.Contains(first.Error(), "flapping 1") {
		t.Errorf("the oldest queued error is %q, want the one after it (flapping 1): the drop must be "+
			"the oldest, not the newest", first.Error())
	}
}

// TestANormalisationFailureIsReportedNotDelivered covers the one branch the wire
// cannot reach, directly, alongside the substitution it sits next to.
//
// The push client drives Decode from the same notifyMsgType it puts on the
// Notification, so a payload that does not match its declared type cannot arrive from
// a frame. It can arrive from a caller - a future decoder, a hand-built notification -
// and the alternative to reporting it is a swallowed failure and a nil event on a
// public API that promises there is none.
//
// The absent-payload case is here too because it is the arm next door and the two must
// not be confused: an absent payload is a delivered SystemEvent, and only an
// unclassifiable one is an error.
func TestANormalisationFailureIsReportedNotDelivered(t *testing.T) {
	a, err := NewPushAdapter()
	if err != nil {
		t.Fatalf("NewPushAdapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	var delivered []*domain.PushUpdate
	h := func(u *domain.PushUpdate) { delivered = append(delivered, u) }

	// A payload the type does not select cannot come off the wire.
	a.convert(&push.Notification{
		Type:    types.BasicQotNotifyMsgType,
		ID:      "00700.HK",
		Payload: &hqnotify.TickerNotify{},
	}, h)
	if len(delivered) != 0 {
		t.Errorf("an unclassifiable payload was delivered %d time(s), want 0", len(delivered))
	}
	if got := len(a.errc); got != 1 {
		t.Fatalf("the stream carries %d error(s), want 1: the failure must be reported, not swallowed", got)
	}
	reported := <-a.errc
	if !strings.Contains(reported.Error(), "does not match notifyMsgType") {
		t.Errorf("reported error = %q, want it to name the mismatch", reported.Error())
	}

	// An absent payload is the substitution, not a failure.
	a.convert(&push.Notification{Type: types.BasicQotNotifyMsgType, ID: "00700.HK"}, h)
	if len(delivered) != 1 {
		t.Fatalf("%d update(s) delivered, want 1: an absent payload is a delivered SystemEvent", len(delivered))
	}
	sys, ok := delivered[0].Event.(domain.SystemEvent)
	if !ok {
		t.Fatalf("Event = %T, want domain.SystemEvent", delivered[0].Event)
	}
	if sys.Code != "EMPTY_PAYLOAD" {
		t.Errorf("Code = %q, want %q", sys.Code, "EMPTY_PAYLOAD")
	}
	if len(a.errc) != 0 {
		t.Errorf("%d error(s) reported for the substitution, want 0", len(a.errc))
	}

	// A nil handler installs the closure and then discards every frame, including the
	// substitution: there is no caller to hand it to.
	a.convert(&push.Notification{Type: types.BasicQotNotifyMsgType, ID: "00700.HK"}, nil)
	if len(delivered) != 1 {
		t.Errorf("%d update(s) delivered, want the same 1: a nil handler must discard", len(delivered))
	}
	if len(a.errc) != 0 {
		t.Errorf("a nil handler produced %d error(s), want 0: there is nothing to normalise and no caller", len(a.errc))
	}
}
