// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests in this file pin the failure and drop-oldest paths of Client. They
// exist because the reconnect loop's coverage previously depended on how many
// dial attempts fitted inside a test deadline: the same commit measured between
// 83% and 87% from run to run, which made the coverage gate a coin flip. Every
// case here is driven by an explicit handshake, a pre-cancelled context, or a
// substituted dialer, so the result does not depend on wall-clock timing.

// pipeConn returns one end of an in-memory connection plus a cleanup that closes
// both ends, so a test never leaks the peer's file descriptor.
func pipeConn(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, server
}

// TestOptionNormalization pins every fallback New applies to option values. These
// branches are the difference between a client dialling 127.0.0.1:11112 and one
// dialling an empty string, and between a reconnect loop that backs off and one
// that spins.
func TestOptionNormalization(t *testing.T) {
	// Options is exported, so a caller can install a raw Option that bypasses the
	// With... helpers. New must still normalize whatever it is handed.
	clearAddr := func(o *Options) { o.Addr = "" }
	zeroMin := func(o *Options) { o.MinReconnect = 0 }
	inverted := func(o *Options) {
		o.MinReconnect = time.Hour
		o.MaxReconnect = time.Second
	}

	t.Run("empty address restores the default", func(t *testing.T) {
		c := New(clearAddr)
		t.Cleanup(func() { _ = c.Close() })

		if got := c.Addr(); got != DefaultAddr {
			t.Errorf("Addr() = %q, want %q", got, DefaultAddr)
		}
	})

	t.Run("WithAddr empty restores the default", func(t *testing.T) {
		c := New(WithAddr("127.0.0.1:19999"), WithAddr(""))
		t.Cleanup(func() { _ = c.Close() })

		if got := c.Addr(); got != DefaultAddr {
			t.Errorf("Addr() = %q, want %q", got, DefaultAddr)
		}
	})

	t.Run("non-positive minimum backoff restores the default", func(t *testing.T) {
		c := New(zeroMin)
		t.Cleanup(func() { _ = c.Close() })

		if got := c.opts.MinReconnect; got != DefaultMinReconnect {
			t.Errorf("MinReconnect = %v, want %v", got, DefaultMinReconnect)
		}
	})

	t.Run("maximum below the minimum is raised to the minimum", func(t *testing.T) {
		c := New(inverted)
		t.Cleanup(func() { _ = c.Close() })

		if got, min := c.opts.MaxReconnect, c.opts.MinReconnect; got != min {
			t.Errorf("MaxReconnect = %v, want it raised to MinReconnect %v", got, min)
		}
	})

	t.Run("nil option is ignored", func(t *testing.T) {
		c := New(nil, WithAddr("127.0.0.1:19998"))
		t.Cleanup(func() { _ = c.Close() })

		if got := c.Addr(); got != "127.0.0.1:19998" {
			t.Errorf("Addr() = %q, want the address set after the nil option", got)
		}
	})
}

// TestWithDialerNilRestoresDefaultDialer asserts that clearing the dialer falls
// back to a real net.Dialer instead of leaving a nil function that would panic on
// the first reconnect.
func TestWithDialerNilRestoresDefaultDialer(t *testing.T) {
	replaced := false
	c := New(
		WithAddr("127.0.0.1:1"),
		WithDialer(func(context.Context, string, string) (net.Conn, error) {
			replaced = true
			return nil, errors.New("replaced dialer must not be used")
		}),
		WithDialer(nil),
	)
	t.Cleanup(func() { _ = c.Close() })

	if c.opts.Dial == nil {
		t.Fatal("WithDialer(nil) left a nil dialer; New must install the default")
	}
	if err := c.Connect(context.Background()); err == nil {
		t.Error("Connect = nil, want the default dialer to fail against 127.0.0.1:1")
	}
	if replaced {
		t.Error("the replaced dialer was called; WithDialer(nil) did not clear it")
	}
}

// TestConnectReusesLiveConnection asserts Connect is idempotent while a
// connection is already held. Redialing here would drop a healthy push stream and
// silently lose subscriptions.
func TestConnectReusesLiveConnection(t *testing.T) {
	conn, _ := pipeConn(t)

	dials := 0
	c := New(
		WithAddr("127.0.0.1:1"),
		WithDialer(func(context.Context, string, string) (net.Conn, error) {
			dials++
			return conn, nil
		}),
	)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	if dials != 1 {
		t.Errorf("dialed %d times, want 1: a repeated Connect must reuse the live connection", dials)
	}
}

// TestConnectDiscardsConnectionWhenClosedDuringDial covers the window between
// dialing and storing the connection. If Close lands in that window the fresh
// connection must be dropped, not parked on a closed client where nothing will
// ever close it.
func TestConnectDiscardsConnectionWhenClosedDuringDial(t *testing.T) {
	var c *Client
	c = New(
		WithAddr("127.0.0.1:1"),
		WithDialer(func(context.Context, string, string) (net.Conn, error) {
			_ = c.Close()
			conn, _ := pipeConn(t)
			return conn, nil
		}),
	)

	if err := c.Connect(context.Background()); !errors.Is(err, ErrClosed) {
		t.Errorf("Connect = %v, want ErrClosed", err)
	}
	if got := c.currentConn(); got != nil {
		t.Error("a connection was stored on a closed client")
	}
}

// TestConnectKeepsFirstConnectionWhenRaced covers the second nil check, where
// another Connect wins the race while this dial is in flight. The losing
// connection must be closed rather than leaked, because nothing owns it after
// Connect returns.
func TestConnectKeepsFirstConnectionWhenRaced(t *testing.T) {
	winner, _ := pipeConn(t)
	loser, loserPeer := pipeConn(t)

	var c *Client
	c = New(
		WithAddr("127.0.0.1:1"),
		WithDialer(func(context.Context, string, string) (net.Conn, error) {
			// Simulate the interleaving deterministically: the competing Connect
			// stores its connection during this dial, before the second nil check.
			c.setConn(winner)
			return loser, nil
		}),
	)
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect = %v, want nil", err)
	}
	if got := c.currentConn(); got != winner {
		t.Error("Connect stored the losing connection; the first live connection must win")
	}
	// A closed loser reports EOF on the peer, which is how the test observes that
	// the discarded connection was actually closed rather than leaked.
	if _, err := loserPeer.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read from the losing peer = %v, want io.EOF (the loser was closed)", err)
	}
}

// TestRunRejectsUnparsableVerificationKey covers the Run entry check for a
// verification key that failed to parse. Run must surface the parse error rather
// than dialling a stream it can never verify.
func TestRunRejectsUnparsableVerificationKey(t *testing.T) {
	c := New(WithAddr("127.0.0.1:1"), WithVerification([]byte("not a key"), true))
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Run(context.Background()); err == nil {
		t.Fatal("Run with an unparsable verification key = nil, want the parse error")
	}
}

// TestRunReportsDialFailureAndStopsOnCancel covers Run's dial-failure path: the
// error must reach Errors, and cancelling the context while Run waits out the
// backoff must end it promptly rather than after the full delay.
//
// The backoff is set to 30s and the context is cancelled as soon as the error is
// observed, so the test waits on a channel rather than a clock. It would take 30s
// to fail if the cancellation were not honoured.
func TestRunReportsDialFailureAndStopsOnCancel(t *testing.T) {
	const dialErr = "dial refused"
	c := New(
		WithAddr("127.0.0.1:1"),
		WithReconnect(30*time.Second, 30*time.Second),
		WithDialer(func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New(dialErr)
		}),
	)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	select {
	case err := <-c.Errors():
		if err == nil || err.Error() != dialErr {
			t.Errorf("Errors() yielded %v, want %q", err, dialErr)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not report the dial failure")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// TestReadLoopSkipsUnknownMessageType covers the read loop's catch-all branch. An
// unrecognised msgType must be skipped rather than treated as a push frame, or a
// future Gateway message would be decoded as a protobuf notification.
func TestReadLoopSkipsUnknownMessageType(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	conn, peer := pipeConn(t)

	body := []byte("a body this SDK does not interpret")
	h := sampleHeader()
	h.MsgType = MsgType(99)
	h.BodyLen = int32(len(body))
	raw := rawFrame(h, body)

	go func() {
		_, _ = peer.Write(raw)
		_ = peer.Close()
	}()

	if err := c.readLoop(conn); err == nil {
		t.Error("readLoop = nil, want the error raised by the closed peer")
	}
}

// TestDecodeRejectsMalformedInput covers both decode entry points rejecting bytes
// that are not a valid message, so a corrupt frame is reported instead of being
// dispatched as a zero-valued notification.
func TestDecodeRejectsMalformedInput(t *testing.T) {
	malformed := []byte{0xff, 0xff, 0xff, 0xff}

	if _, err := decodeNotification(malformed); err == nil {
		t.Error("decodeNotification on a malformed body = nil, want an error")
	}
	if _, err := Decode(types.TickerNotifyMsgType, malformed); err == nil {
		t.Error("Decode on a malformed payload = nil, want an error")
	}
}

// TestDispatchIgnoresNilAndUnregisteredType covers the two early returns in
// dispatch. Neither may panic: a nil notification reaches dispatch when a decode
// path yields nothing, and an unregistered type is routine during subscribe and
// reconnect races.
func TestDispatchIgnoresNilAndUnregisteredType(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	c.dispatch(nil)
	c.dispatch(&Notification{Type: types.TickerNotifyMsgType})
}

// TestDispatchDropsOldestWhenQueueFull covers the drop-oldest policy for a handler
// that has fallen behind. Blocking the handler and waiting for it to actually
// start keeps the queue full deterministically; without that handshake the test
// would race the dispatcher goroutine and measure coverage instead of behaviour.
func TestDispatchDropsOldestWhenQueueFull(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	const typ = types.TickerNotifyMsgType
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	c.SubscribeTypes(typ, func(*Notification) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	})

	c.dispatch(&Notification{Type: typ, ID: "park"})
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		close(release)
		t.Fatal("handler never received the first notification")
	}

	for i := range dispatchBuffer + 2 {
		c.dispatch(&Notification{Type: typ, ID: fmt.Sprintf("n%d", i)})
	}

	c.mu.Lock()
	queued := len(c.handlers[typ].ch)
	c.mu.Unlock()
	if queued != dispatchBuffer {
		t.Errorf("handler queue holds %d notifications, want it capped at %d", queued, dispatchBuffer)
	}
	close(release)
}

// TestReportIgnoresNilAndStopsAfterClose covers both of report's guards. Enqueuing
// a nil error would hand callers a nil to match on, and reporting after Close
// would send on a closed channel and panic the read loop.
func TestReportIgnoresNilAndStopsAfterClose(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	c.report(nil)
	if got := len(c.Errors()); got != 0 {
		t.Errorf("error channel holds %d entries after report(nil), want 0", got)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c.report(errors.New("after close"))
}

// TestReportDropsOldestWhenFull covers the error channel's drop-oldest policy. A
// reader that falls behind must lose the stalest transport error, not the newest
// one, and the channel must never grow past its bound.
func TestReportDropsOldestWhenFull(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	total := errorBuffer + 4
	for i := range total {
		c.report(fmt.Errorf("probe %d", i))
	}
	if got := len(c.Errors()); got != errorBuffer {
		t.Fatalf("error channel holds %d entries, want it capped at %d", got, errorBuffer)
	}

	var newest error
	for range errorBuffer {
		if err, ok := <-c.Errors(); ok && err != nil {
			newest = err
		}
	}
	if newest == nil {
		t.Fatal("error channel yielded no errors")
	}
	if want := fmt.Sprintf("probe %d", total-1); newest.Error() != want {
		t.Errorf("newest retained error = %q, want %q", newest.Error(), want)
	}
}

// TestSleepCtx covers the non-positive duration shortcut, which decides whether
// Run yields immediately or spins when the backoff has been normalized to zero.
func TestSleepCtx(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		d    time.Duration
		want bool
	}{
		{"zero duration on a live context", context.Background(), 0, true},
		{"negative duration on a live context", context.Background(), -time.Second, true},
		{"zero duration on a cancelled context", cancelled, 0, false},
		{"positive duration on a cancelled context", cancelled, time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sleepCtx(tt.ctx, tt.d); got != tt.want {
				t.Errorf("sleepCtx(ctx, %v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

// TestNextBackoffStaysWithinBounds pins the backoff ceiling and the
// overflow guard. An uncapped backoff would pin a reconnecting fleet at the
// maximum delay forever, and the overflow path must return the input unchanged
// rather than a negative duration that would make sleepCtx return immediately.
func TestNextBackoffStaysWithinBounds(t *testing.T) {
	const max = 8 * time.Second

	cur := time.Second
	for range 8 {
		got := nextBackoff(cur, max)
		if got <= 0 || got > max {
			t.Fatalf("nextBackoff(%v, %v) = %v, want a delay in (0, %v]", cur, max, got, max)
		}
		cur = got
	}

	if got := nextBackoff(max, max); got <= 0 || got > max {
		t.Errorf("nextBackoff(%v, %v) = %v, want a delay in (0, %v]", max, max, got, max)
	}
	if got := nextBackoff(0, max); got != 0 {
		t.Errorf("nextBackoff(0, %v) = %v, want 0: an overflowed double returns cur unchanged", max, got)
	}
}

// truncWriter reports a partial write with no error, which io.Writer permits and
// which writeFull must convert into io.ErrShortWrite.
type truncWriter struct{}

func (truncWriter) Write(b []byte) (int, error) { return len(b) / 2, nil }

// errPeerGone stands in for a peer that disconnects mid-frame.
var errPeerGone = errors.New("peer closed the connection")

// failingWriter rejects every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errPeerGone }

// headerOnlyWriter accepts the header and fails on the body, reproducing a peer
// that acknowledges the header and then disappears.
type headerOnlyWriter struct {
	mu      sync.Mutex
	written bool
}

func (w *headerOnlyWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(b) == HeaderSize && !w.written {
		w.written = true
		return len(b), nil
	}
	return 0, errPeerGone
}

// TestWriteFullReportsPartialWrite covers the short-write conversion. Treating a
// truncated frame as sent would desynchronise the stream and make every later
// frame undecodable.
func TestWriteFullReportsPartialWrite(t *testing.T) {
	if err := writeFull(truncWriter{}, make([]byte, 16)); !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("writeFull = %v, want io.ErrShortWrite", err)
	}
}

// TestWriteFramePropagatesWriteFailure covers both write sites in WriteFrame. A
// header that fails must not be followed by a body write, and a body that fails
// must be reported rather than leaving a header-only frame on the wire.
func TestWriteFramePropagatesWriteFailure(t *testing.T) {
	t.Run("header write fails", func(t *testing.T) {
		if err := WriteFrame(failingWriter{}, sampleHeader(), nil); !errors.Is(err, errPeerGone) {
			t.Errorf("WriteFrame = %v, want errPeerGone", err)
		}
	})

	t.Run("body write fails after a good header", func(t *testing.T) {
		err := WriteFrame(&headerOnlyWriter{}, sampleHeader(), make([]byte, 7))
		if !errors.Is(err, errPeerGone) {
			t.Errorf("WriteFrame = %v, want errPeerGone", err)
		}
	})
}
