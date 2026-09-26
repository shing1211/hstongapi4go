// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/push"
)

// The tests in this file pin the failure and guard paths of Client. They exist
// because the package sat 0.6pp above the 85% coverage gate, which is a margin
// thin enough to fail a release on a coverage wobble rather than on a defect.
//
// No test here sleeps to reach a path, and no test waits for a dial or a
// reconnect to happen. Every asynchronous edge is crossed by an explicit
// handshake:
//
//   - the push connection is a net.Pipe installed by a substituted dialer, so
//     there is no listener, no port, and no Gateway;
//   - a mid-call state change is driven from the server hook, which runs before
//     the reply is written, so the client cannot observe the reply until the
//     hook has returned;
//   - a state change on the Connect path is made from inside the dialer, which
//     runs on the Connect goroutine itself, so no second goroutine is involved;
//   - a loop that must terminate is awaited on its own exit, never on a clock.
//
// waitBound is the ceiling a test waits on. Hitting it is always a failure; no
// test uses it to get somewhere.

// waitBound is the ceiling any test waits for an event. Reaching it is a
// failure, never a way to reach a code path.
const waitBound = 30 * time.Second

// subscribePath is the Gateway route the market manager posts to.
const subscribePath = "/hq/Subscribe"

// Gateway response envelopes used by the scripted server.
const (
	// okEnvelope is a successful response with no payload.
	okEnvelope = `{"ok":true,"err":"","data":null}`
	// failEnvelope is a failed response carrying a documented status code
	// (1011, service busy), which the transport turns into a typed errs.Error.
	failEnvelope = `{"ok":false,"err":"1011"}`
)

// alwaysOK replies with a success envelope to every request.
func alwaysOK(string, int) string { return okEnvelope }

// hookServer is a Gateway stand-in whose per-path reply is scripted. Calls are
// counted per path, and the 1-based count is passed to reply so a test can let
// the first call succeed and fail every one after it.
//
// before runs after the call is counted and before the reply is written. That
// ordering is the point: the client cannot observe the reply until before has
// returned, so a state change made there is a handshake rather than a race.
type hookServer struct {
	mu     sync.Mutex
	counts map[string]int
	reply  func(path string, call int) string
	before func(path string, call int)
	url    string
}

// newHookServer starts a scripted Gateway. A nil reply defaults to alwaysOK and
// a nil before installs no hook.
func newHookServer(t *testing.T, reply func(path string, call int) string, before func(path string, call int)) *hookServer {
	t.Helper()
	if reply == nil {
		reply = alwaysOK
	}
	h := &hookServer{counts: make(map[string]int), reply: reply, before: before}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	h.url = srv.URL
	return h
}

func (h *hookServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	_, _ = io.ReadAll(req.Body)
	h.mu.Lock()
	h.counts[req.URL.Path]++
	call := h.counts[req.URL.Path]
	reply, before := h.reply, h.before
	h.mu.Unlock()

	if before != nil {
		before(req.URL.Path, call)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, reply(req.URL.Path, call))
}

// count reports how many requests reached path.
func (h *hookServer) count(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[path]
}

// pipeDialer substitutes the push connection with in-memory pipes, so no test
// opens a TCP listener or contacts a Gateway. The peer end of every pipe is
// retained and closed by the test, which leaves the push read loop parked in
// ReadFrame instead of spinning through reconnect backoff.
//
// before runs on the dialing goroutine before the pipe is handed over. It is
// the only place a test may reach the client from inside the dial, which is
// what makes the "closed during dial" window reproducible without a race.
type pipeDialer struct {
	mu     sync.Mutex
	dials  int
	peers  []net.Conn
	before func(call int) error
}

// dial implements push.DialFunc.
func (d *pipeDialer) dial(_ context.Context, _, _ string) (net.Conn, error) {
	d.mu.Lock()
	d.dials++
	call := d.dials
	before := d.before
	d.mu.Unlock()

	if before != nil {
		if err := before(call); err != nil {
			return nil, err
		}
	}

	local, peer := net.Pipe()
	d.mu.Lock()
	d.peers = append(d.peers, peer)
	d.mu.Unlock()
	return local, nil
}

// count reports how many times the dialer ran.
func (d *pipeDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

// peer returns the remote end of the i-th pipe, which the test reads to learn
// whether the local end was actually closed.
func (d *pipeDialer) peer(i int) net.Conn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peers[i]
}

// closePeers closes every retained pipe peer. Call it after the stream client
// is closed, so no file descriptor outlives the test.
func (d *pipeDialer) closePeers() {
	d.mu.Lock()
	peers := d.peers
	d.peers = nil
	d.mu.Unlock()
	for _, p := range peers {
		_ = p.Close()
	}
}

// newTestClient builds a stream Client over a *client.Client pointed at srvURL
// with d as its push dialer, and registers cleanup for both. Callers may pass
// additional options, typically WithTradeManager.
func newTestClient(t *testing.T, srvURL string, d *pipeDialer, opts ...Option) *Client {
	t.Helper()
	c, err := client.New(client.WithBaseURL(srvURL), client.WithPushAddr(push.DefaultAddr))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	all := make([]Option, 0, len(opts)+1)
	all = append(all, WithDialer(d.dial))
	all = append(all, opts...)
	s := New(c, all...)

	// Cleanup runs last-registered-first: close the stream client (which stops
	// the push goroutines and so ends any further dials) before the pipe peers.
	t.Cleanup(func() {
		_ = s.Close()
		d.closePeers()
	})
	return s
}

// newBareClient returns a Client with no underlying *client.Client, no push
// client, and no goroutines. It is the right starting point for a test of a
// method that only touches the subscription map or is handed its own push
// client.
func newBareClient(t *testing.T) *Client {
	t.Helper()
	s := &Client{subs: make(map[*Subscription]struct{}), done: make(chan struct{})}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// hkSecurity is the instrument every market subscription in these tests uses.
func hkSecurity() *dto.Security { return &dto.Security{DataType: 10000, Code: "0700.HK"} }

// subscriptionCount reports how many subscriptions are registered.
func subscriptionCount(s *Client) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

// scriptedPusher stands in for a *trade.Manager. subErr maps a 1-based
// SubscribeOrders call index to the error that call must return, so a test can
// let the first subscribe succeed and fail the reconnect-time resubscribe.
// onSub runs on the SubscribeOrders goroutine, which is the caller's own
// goroutine, so a hook there is a same-goroutine handshake.
type scriptedPusher struct {
	mu     sync.Mutex
	subs   int
	unsubs int
	subErr map[int]error
	onSub  func(call int)
}

// SubscribeOrders records one subscribe call and returns the scripted error for
// that call index, if any.
func (p *scriptedPusher) SubscribeOrders(context.Context) error {
	p.mu.Lock()
	p.subs++
	call := p.subs
	err := p.subErr[call]
	hook := p.onSub
	p.mu.Unlock()

	if hook != nil {
		hook(call)
	}
	return err
}

// UnsubscribeOrders records one unsubscribe call.
func (p *scriptedPusher) UnsubscribeOrders(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.unsubs++
	return nil
}

// counts reports the subscribe and unsubscribe totals.
func (p *scriptedPusher) counts() (subs, unsubs int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.subs, p.unsubs
}

// nextError reads one error from a subscription, failing the test when none
// arrives.
func nextError(t *testing.T, sub *Subscription) error {
	t.Helper()
	select {
	case err, ok := <-sub.errs:
		if !ok {
			t.Fatal("subscription error channel closed, want an error")
		}
		return err
	case <-time.After(waitBound):
		t.Fatal("timed out waiting for a subscription error")
		return nil
	}
}

// drainErrors returns the errors already queued on a subscription without
// blocking, so a test can assert that nothing *else* was reported.
func drainErrors(sub *Subscription) []error {
	var out []error
	for {
		select {
		case err, ok := <-sub.errs:
			if !ok {
				return out
			}
			out = append(out, err)
		default:
			return out
		}
	}
}

// assertReconnectNotice checks that err is the ErrReconnected notice raised by
// onReconnect.
//
// onReconnect builds the notice as fmt.Errorf("%w: %v", ErrReconnected, cause),
// so the wrap target is ErrReconnected and the reconnect cause is rendered into
// the message text rather than wrapped. That contradicts the ErrReconnected
// doc comment, which says the error that ended the connection "is wrapped and
// available with errors.Unwrap". This test pins the behaviour as implemented so
// a future change to either the notice or the doc is a visible diff; the
// discrepancy is reported as a finding rather than fixed here, because this is a
// coverage task.
func assertReconnectNotice(t *testing.T, err, cause error) {
	t.Helper()
	if !errors.Is(err, ErrReconnected) {
		t.Errorf("reconnect notice = %v, want ErrReconnected", err)
	}
	if got := errors.Unwrap(err); got != ErrReconnected {
		t.Errorf("reconnect notice wraps %v, want ErrReconnected (the cause is rendered as text)", got)
	}
	if !strings.Contains(err.Error(), cause.Error()) {
		t.Errorf("reconnect notice %q does not mention the reconnect cause %q", err, cause)
	}
}
