// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/auth"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the test suite for session.go: the two trade-session endpoints and
// the composition of internal/auth that reaches them.
//
// Three properties make it its own file rather than a section of an existing
// one, and each rules out the approach the B-series established for the other
// services:
//
//   - **No row sleeps, and no row waits on a deadline.** The suite's centrepiece
//     is a concurrency property - N concurrent logins produce one request - and a
//     fake that answers immediately cannot express "these callers are all waiting".
//     So time enters only through a settable clock, injected with
//     WithSessionClock, and interleaving is controlled by parking a leader inside
//     the one request it is entitled to and by taking the gate's own mutex from
//     this file, which is legal because the tests are in package services.
//     AGENTS.md records that a coverage figure which moves between runs of one
//     commit is a defect; a timing-based suite would be exactly that.
//   - **Both error levels are exercised for both methods.** The fake level says
//     what the service did with an outcome; the real-HTTP level says what
//     internal/transport built out of it. A fake never builds a typed error, so a
//     suite with only the fake level cannot assert a code, a category, or that a
//     body was actually written to the wire.
//   - **The credential is treated as an absence everywhere it could appear.** Not
//     in the request body, not in an error string, not in the returned struct, and
//     not in a log. Those are four separate places, and one assertion would not
//     cover them.
//
// testsupport_test.go is used unchanged: sequencedExecutor and requireCalls for
// the fake level, wireRecorder and newWireExecutor for the real one, and
// errRejects / assertInvalidParam / requireZero for the typed-error shape. Every
// double declared here is session-prefixed, because a duplicate declaration in a
// package whose tests are all in-package is a compile error that takes every other
// run in pkg/services down with it.

// ---------------------------------------------------------------------------
// Doubles and shared fixtures
// ---------------------------------------------------------------------------

const (
	// sessionAccount is the local session key every test in this file uses. It
	// never appears in a request body, and TestSessionParamsAreNeverNil and
	// TestSessionLoginAndLogoutHappyPath assert that it does not.
	sessionAccount = domain.AccountID("ACC-SESSION")
	// sessionPassword is the vendor's documented demo password. It is not a
	// secret, and neither is it used anywhere but here.
	sessionPassword = "123456"
	// sessionCiphertext is HStong's published known-answer vector for
	// sessionPassword: Base64(AES-192-ECB/PKCS7(password)). Asserting it is what
	// proves the password reached the wire encrypted, exactly once - a second
	// encryption would produce a different string and fail the assertion.
	sessionCiphertext = "W1U8iZIppSE+mBMtzy9vZQ=="
)

// sessionClock is a settable auth.Clock. Time is the only thing these tests move
// and they move it by assignment, so no row of this suite can be made to pass by
// a machine being fast or slow.
type sessionClock struct {
	mu  sync.Mutex
	now int64
}

func (c *sessionClock) Now() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *sessionClock) set(v int64) {
	c.mu.Lock()
	c.now = v
	c.mu.Unlock()
}

func (c *sessionClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now += int64(d.Seconds())
	c.mu.Unlock()
}

// sessionStore is an auth.SessionStore a test can inspect and can make fail on a
// chosen call.
//
// failReadOn arms a failure on the n-th read (1-based), which is how the three
// distinct read arms are told apart: read 1 is Login's own pre-flight check, read
// 2 is the leader's re-read, and read 3 is the one internal/auth performs after
// saving and discards the error from. reads, when non-nil, signals every read
// that got past the armed failure, so a test can wait for a caller to be past its
// store check without sleeping.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[domain.AccountID]auth.Session
	reads    chan struct{}

	readCount  int
	failReadOn int
	readErr    error

	saveErr   error
	clearErr  error
	saveCall  int
	clearCall int
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[domain.AccountID]auth.Session)}
}

func (s *sessionStore) Session(_ context.Context, accountID domain.AccountID) (auth.Session, bool, error) {
	s.mu.Lock()
	s.readCount++
	fail := s.failReadOn > 0 && s.readCount == s.failReadOn
	reads := s.reads
	err := s.readErr
	sess, ok := s.sessions[accountID]
	s.mu.Unlock()

	if !fail && reads != nil {
		// Non-blocking, so a test that armed less signal capacity than it has
		// callers cannot wedge a read; a dropped signal costs a failed assertion,
		// never a hung goroutine.
		select {
		case reads <- struct{}{}:
		default:
		}
	}
	if fail {
		return auth.Session{}, false, err
	}
	return sess, ok, nil
}

func (s *sessionStore) SaveSession(_ context.Context, sess auth.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCall++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.sessions[sess.AccountID] = sess
	return nil
}

func (s *sessionStore) ClearSession(_ context.Context, accountID domain.AccountID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearCall++
	if s.clearErr != nil {
		return s.clearErr
	}
	delete(s.sessions, accountID)
	return nil
}

// holds reports what the store currently holds for accountID.
func (s *sessionStore) holds(accountID domain.AccountID) (auth.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[accountID]
	return sess, ok
}

// clears reports how many times the store was cleared.
func (s *sessionStore) clears() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clearCall
}

// saves reports how many times the store was written.
func (s *sessionStore) saves() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCall
}

// sessionParkExecutor is an Executor whose Do parks until it is released.
//
// It exists for one reason: a single-flight property cannot be observed with a
// fake that answers immediately, because "the other callers arrived while the
// first was still inside the request" is the very thing under test. Parking the
// leader is what makes the interleaving a fact rather than a hope.
type sessionParkExecutor struct {
	mu      sync.Mutex
	ops     []string
	routes  []client.Route
	params  []any
	reply   any
	err     error
	release chan struct{}
	once    sync.Once
}

func newSessionParkExecutor() *sessionParkExecutor {
	return &sessionParkExecutor{release: make(chan struct{})}
}

func (e *sessionParkExecutor) JSON() client.Codec { return nil }

func (e *sessionParkExecutor) Do(_ context.Context, op string, route client.Route, params any, _ client.Codec, out any) error {
	e.mu.Lock()
	e.ops = append(e.ops, op)
	e.routes = append(e.routes, route)
	e.params = append(e.params, params)
	reply, replyErr := e.reply, e.err
	e.mu.Unlock()

	<-e.release

	if replyErr != nil {
		return replyErr
	}
	if out == nil || reply == nil {
		return nil
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (e *sessionParkExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.ops)
}

func (e *sessionParkExecutor) callAt(i int) sequencedCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return sequencedCall{op: e.ops[i], route: e.routes[i], params: e.params[i]}
}

// unblock lets every parked request finish. It is safe to call more than once,
// because a test may park, fail an assertion, and still need the goroutines it
// started to finish.
func (e *sessionParkExecutor) unblock() { e.once.Do(func() { close(e.release) }) }

// awaitCalls blocks until the executor has recorded at least n calls, or gives up
// after a bounded number of yields.
//
// The bound is the point. A plain channel receive would be correct but would hang
// until go test's own panic timeout if a defect stopped the code from calling Do at
// all, and a ten-minute hang is a far worse failure to read than a message. A
// bounded yield loop fails in well under a second and says what it was waiting
// for. It is not a sleep, and it is not an assumption about how long work takes: it
// bounds how long the test waits for a goroutine the test itself started, and every
// assertion downstream is about the count the executor recorded.
func (e *sessionParkExecutor) awaitCalls(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < 2_000_000; i++ {
		if e.callCount() >= n {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("the executor recorded %d calls, want at least %d: the code under test never "+
		"issued the request it was expected to issue", e.callCount(), n)
}

// sessionAck is a scripted session reply. The body is a raw JSON message so the
// reply is decoded by encoding/json through the same path a real reply takes,
// rather than being hand-built into the wire type - which is the only way the
// absent-versus-false rule on the success field is actually exercised.
func sessionAck(body string) sequencedReply {
	return sequencedReply{reply: json.RawMessage(body)}
}

// sessionBusy is the rejection a retry policy treats as transient. It is the
// same code the rest of this package's ADR 0003 proofs use, so a session row and
// a mutation row are being driven by the same stimulus.
func sessionBusy(op string) error {
	return errs.New(types.StatusServiceBusy, op, "service busy, retry later")
}

// sessionNewService builds a SessionService over a fake with a clock this file
// controls, which is the only reason any expiry assertion in this suite is
// deterministic.
func sessionNewService(f *sequencedExecutor, clk *sessionClock, opts ...SessionOption) *SessionService {
	base := []SessionOption{WithSessionClient(f), WithSessionClock(clk)}
	return NewSessionService(f, append(base, opts...)...)
}

// sessionWireService builds a SessionService over a real client on a recorder,
// with the same fixed clock.
func sessionWireService(exec Executor, opts ...SessionOption) *SessionService {
	base := []SessionOption{WithSessionClock(&sessionClock{now: 1_000})}
	return NewSessionService(exec, append(base, opts...)...)
}

// readSessionSource reads this package's own session.go as text, for the static
// half of the credential property.
func readSessionSource() (string, error) {
	raw, err := os.ReadFile("session.go")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// drainStoreReads empties a sessionStore's read signal. It is called once a
// leader is parked inside its request: the leader cannot read again until it is
// released, so whatever is in the channel is exactly what it did on the way in,
// and the tokens after the drain belong to the callers this test is about to
// start.
func drainStoreReads(reads chan struct{}) {
drain:
	for {
		select {
		case <-reads:
		default:
			break drain
		}
	}
}

// awaitWaiters blocks until the gate reports n callers queued behind the login in
// flight for accountID, and fails the test rather than hanging if they never get
// there.
//
// It is the barrier that makes the single-flight assertions exact, and it is worth
// being explicit about why nothing weaker would do. A caller that has read the
// store and not yet taken the gate's mutex has registered nothing, and no
// bookkeeping can retroactively include it; only a count taken under that same
// mutex says whether it has been through. Waiting on the store reads is not
// enough, because a read happens before the gate rather than in it.
//
// The count can only reach n while the leader's entry is present, because the
// join path is the only one that increments it - and the leader's entry cannot be
// removed while the leader is parked inside its request. So "every caller has
// joined" is a fact about the code's state rather than about how fast the machine
// is, and the request count below is then a measurement rather than a hope.
func awaitWaiters(t *testing.T, g *loginGate, accountID domain.AccountID, n int) {
	t.Helper()
	for i := 0; i < 2_000_000; i++ {
		if g.waiting(accountID) >= n {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("%d caller(s) never joined the login in flight for %s; the gate is not "+
		"coalescing them", n, accountID)
}

// ---------------------------------------------------------------------------
// Routing and the two request bodies
// ---------------------------------------------------------------------------

// TestSessionRoutesAndBodies is the positive path for both endpoints at the
// real-HTTP level: the op label, the route, and the body are the ones the method
// names, and the bodies are asserted as bytes rather than as structs because a
// struct assertion cannot see a key that is missing from the envelope.
func TestSessionRoutesAndBodies(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeLogin):  gatewaySuccess(`{"success":true}`),
		string(client.RouteTradeLogout): gatewaySuccess(`{"success":true}`),
	})
	svc := sessionWireService(newWireExecutor(t, rec))

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := svc.Logout(t.Context(), sessionAccount); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	loginBody := rec.lastBody(t, string(client.RouteTradeLogin))
	if !strings.Contains(loginBody, `"password":"`+sessionCiphertext+`"`) {
		t.Errorf("the login body does not carry the encrypted password as \"password\":\n%s", loginBody)
	}
	// The key is password, not tradePassword: both vendor SDKs and the released
	// pkg/hstong/session.go send password, and docs/protocol.md's curl example is
	// the only source in this repository that says otherwise. Sending the doc's
	// spelling would be a request the Gateway cannot read.
	if strings.Contains(loginBody, "tradePassword") {
		t.Errorf("the login body carries tradePassword, which no vendor or released layer sends:\n%s", loginBody)
	}

	// The logout body must be an empty object and not an absent key.
	logoutBody := rec.lastBody(t, string(client.RouteTradeLogout))
	if !strings.Contains(logoutBody, `"params":{}`) {
		t.Errorf("the logout body does not carry an empty params object:\n%s", logoutBody)
	}
	if got := rec.total(); got != 2 {
		t.Errorf("requests = %d, want 2: one per endpoint", got)
	}
}

// TestSessionParamsAreNeverNil states the struct{}{} rule as a test rather than a
// comment, at the only level that can see the difference: the recorded body.
//
// A nil params is dropped from the envelope by internal/transport, because
// request.Params carries omitempty, so a logout that passed nil would send an
// envelope with no params key at all. That is a different request from the one
// both vendors send, and it is invisible at the fake level.
func TestSessionParamsAreNeverNil(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeLogin):  gatewaySuccess(`{"success":true}`),
		string(client.RouteTradeLogout): gatewaySuccess(`{"success":true}`),
	})
	svc := sessionWireService(newWireExecutor(t, rec))

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := svc.Logout(t.Context(), sessionAccount); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	for _, route := range []client.Route{client.RouteTradeLogin, client.RouteTradeLogout} {
		if body := rec.lastBody(t, string(route)); !strings.Contains(body, `"params"`) {
			t.Errorf("the %s body has no params key at all, so the params were nil rather than "+
				"an empty object:\n%s", route, body)
		}
	}
}

// ---------------------------------------------------------------------------
// The happy path
// ---------------------------------------------------------------------------

// TestSessionLoginAndLogoutHappyPath is the fake-level positive row for both
// methods. The reply bodies are the two dialects this repository's sources
// disagree about, so a build that only ever decoded one of them fails here rather
// than in a production Gateway.
func TestSessionLoginAndLogoutHappyPath(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	f := newSequencedExecutor(t, sessionAck(`{"success":true}`), sessionAck(`{"success":true}`))
	svc := sessionNewService(f, clk)

	sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sess.AccountID != sessionAccount {
		t.Errorf("AccountID = %q, want %q", sess.AccountID, sessionAccount)
	}
	// The Gateway's acknowledgement carried no string, so the token is the zero
	// SessionToken. That is the honest value on this dialect; a fabricated
	// identifier would read as a credential the SDK does not hold.
	if !sess.Token.IsZero() {
		t.Errorf("Token = %q, want the zero SessionToken: the reply carried no data string", sess.Token)
	}
	if sess.ExpiresAt <= sess.RefreshAt {
		t.Errorf("ExpiresAt = %d, RefreshAt = %d, want the refresh point to open before the "+
			"expiry", sess.ExpiresAt, sess.RefreshAt)
	}
	if sess.ExpiresAt != 1_000+int64((3*time.Hour).Seconds()) {
		t.Errorf("ExpiresAt = %d, want %d: the expiry is the token TTL measured from the injected "+
			"clock, not from the wall clock", sess.ExpiresAt, 1_000+int64((3*time.Hour).Seconds()))
	}
	if err := svc.Logout(t.Context(), sessionAccount); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	requireCalls(t, f, 2)
}

// TestSessionLoginAcceptsTheStringDialect is the second reply dialect on its own,
// because the two are separate decisions and folding them into one table would
// let the boolean row mask the string one.
func TestSessionLoginAcceptsTheStringDialect(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	f := newSequencedExecutor(t, sessionAck(`{"data":"session-token-abc"}`))
	svc := sessionNewService(f, clk)

	sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("Login on the Java vendor's CommonStringVo dialect: %v", err)
	}
	if sess.Token != "session-token-abc" {
		t.Errorf("Token = %q, want the string the reply carried", sess.Token)
	}
	if sess.AccountID != sessionAccount {
		t.Errorf("AccountID = %q, want %q: the account is the local session key and is carried "+
			"through from the argument, never read off the reply", sess.AccountID, sessionAccount)
	}
}

// TestSessionAccountIDNeverReachesTheWire states the account rule as an
// assertion on both request bodies. It is the same rule the cash, futures and
// algo services follow, and here it is load bearing: an account identifier in a
// session body would be an identifier the Gateway never asked for.
func TestSessionAccountIDNeverReachesTheWire(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	f := newSequencedExecutor(t, sessionAck(`{"success":true}`), sessionAck(`{"success":true}`))
	svc := sessionNewService(f, clk)

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := svc.Logout(t.Context(), sessionAccount); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	for i := 0; i < 2; i++ {
		params := f.callAt(t, i).params
		if params == nil {
			t.Fatalf("call %d passed nil params: internal/transport drops a nil params from the "+
				"envelope, so the key would be absent rather than {}", i)
		}
		rendered, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("rendering call %d params: %v", i, err)
		}
		if strings.Contains(string(rendered), string(sessionAccount)) {
			t.Errorf("call %d put the account id on the wire: %s", i, rendered)
		}
	}
}

// TestSessionLoginRequestCarriesTheCiphertextOnce is the encryption control.
//
// internal/auth encrypts the password before it calls the injected login function,
// so a service that encrypted again - the obvious mistake when the parameter is
// called tradePassword and the field is called password - would send a ciphertext
// of a ciphertext. The published known-answer vector pins it, and a second
// encryption produces a different string, so this cannot pass by accident.
func TestSessionLoginRequestCarriesTheCiphertextOnce(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	f := newSequencedExecutor(t, sessionAck(`{"success":true}`))
	svc := sessionNewService(f, clk)

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}
	params, ok := f.lastParams(t).(sessionLoginWireRequest)
	if !ok {
		t.Fatalf("params = %T, want the login request body", f.lastParams(t))
	}
	if params.Password != sessionCiphertext {
		t.Errorf("Password = %q, want the published known-answer ciphertext %q: the password must "+
			"be encrypted exactly once, by internal/auth", params.Password, sessionCiphertext)
	}
}

// ---------------------------------------------------------------------------
// Local rejections, at zero requests
// ---------------------------------------------------------------------------

// TestSessionLocalRejectionsCostNoRequest is the fail-closed property. Every
// executor here is built with *no* scripted reply, so a request that escaped the
// validation would hit the fixture's own t.Fatalf rather than be quietly absorbed
// - a rejection that cost a request fails the test instead of passing it.
func TestSessionLocalRejectionsCostNoRequest(t *testing.T) {
	clk := &sessionClock{now: 1_000}

	t.Run("zero account id", func(t *testing.T) {
		f := newSequencedExecutor(t)
		svc := sessionNewService(f, clk)

		sess, err := svc.Login(t.Context(), "", sessionPassword)
		requireZero(t, sess)
		assertInvalidParam(t, err, opTradeLogin)
		requireCalls(t, f, 0)

		assertInvalidParam(t, svc.Logout(t.Context(), ""), opTradeLogout)
		requireCalls(t, f, 0)
	})

	for _, blank := range []string{"", "   ", "\t\n"} {
		t.Run("blank trade password "+fmt.Sprintf("%q", blank), func(t *testing.T) {
			f := newSequencedExecutor(t)
			svc := sessionNewService(f, clk)

			sess, err := svc.Login(t.Context(), sessionAccount, blank)
			requireZero(t, sess)
			e := assertInvalidParam(t, err, opTradeLogin)
			if e.Category != errs.CategoryAPI {
				t.Errorf("Category = %q, want %q: a local rejection must never carry a Gateway "+
					"trading or account code for a request the Gateway never saw",
					e.Category, errs.CategoryAPI)
			}
			requireCalls(t, f, 0)
		})
	}
}

// ---------------------------------------------------------------------------
// Error arms, at both levels
// ---------------------------------------------------------------------------

// TestSessionLoginGatewayErrorArms is the fake-level table for a transport-level
// failure. The typed shape is asserted because a caller branching on a login
// failure branches on a code.
func TestSessionLoginGatewayErrorArms(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	f := newSequencedExecutor(t, sequencedReply{err: sessionBusy(opTradeLogin)})
	svc := sessionNewService(f, clk)

	sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	requireZero(t, sess)
	errRejects(t, err, types.StatusServiceBusy, opTradeLogin)
	if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
		t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
	}
	requireCalls(t, f, 1)
}

// TestSessionLoginBodyErrorArms is the other fake-level arm: the request reached
// the Gateway and came back ok, but the body is not an acknowledgement.
//
// The rows here are the ones that must *not* carry a status code. A refused
// credential is not a documented code, so handing the caller a Gateway trading or
// account code for it would send them looking in the wrong place entirely - and
// the two predicates a caller actually branches on come out false, which is the
// property that matters.
func TestSessionLoginBodyErrorArms(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"an explicit refusal", `{"success":false}`},
		{"a body with neither field", `{}`},
		{"a refusal that also carries a string", `{"success":false,"data":"tok"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := &sessionClock{now: 1_000}
			f := newSequencedExecutor(t, sessionAck(tt.body))
			svc := sessionNewService(f, clk)

			sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			requireZero(t, sess)
			if err == nil {
				t.Fatalf("Login on %s = nil error, want a rejection", tt.body)
			}
			if code, ok := errs.CodeOf(err); ok {
				t.Errorf("CodeOf = (%q, true), want no code: a refused body is not a status code", code)
			}
			// Re-logging in on this signal would send the same password again, and
			// retrying would too; the caller's next move is a different credential.
			if errs.ReLoginRequired(err) {
				t.Error("ReLoginRequired = true, want false: a refused credential is not a session failure")
			}
			if errs.Retryable(err) {
				t.Error("Retryable = true, want false: asking again unchanged will fail the same way")
			}
			if got := errs.CategoryOf(err); got != errs.CategoryUnknown {
				t.Errorf("CategoryOf = %q, want %q: no documented category describes a refused body",
					got, errs.CategoryUnknown)
			}
			requireCalls(t, f, 1)
		})
	}
}

// TestSessionSeparatesARefusedCredentialFromARetryableRejection is the pair the
// GoDoc on Login promises, asserted at the real-HTTP level because only there does
// a typed error get built at all.
//
// The failures need opposite responses from a caller - change the password versus
// try again later - and the only thing that tells them apart is the category,
// because a refused credential has no status code at all. So the assertion is on
// the categories and the two predicates, never on a rendered message: a reworded
// message must not be able to merge the two.
func TestSessionSeparatesARefusedCredentialFromARetryableRejection(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantRetryable bool
		wantCategory  errs.Category
		wantCode      types.StatusCode
		wantReLogin   bool
	}{
		{
			name:          "a refused credential",
			body:          gatewaySuccess(`{"success":false}`),
			wantRetryable: false,
			wantCategory:  errs.CategoryUnknown,
			wantReLogin:   false,
		},
		{
			name:          "a rate limit",
			body:          gatewayFailure(types.StatusServiceBusy, "service busy, retry later"),
			wantRetryable: true,
			wantCategory:  errs.CategoryRateLimit,
			wantCode:      types.StatusServiceBusy,
			wantReLogin:   false,
		},
		{
			name:          "a not-logged-in rejection",
			body:          gatewayFailure(types.StatusNotLoggedIn, "not logged in"),
			wantRetryable: false,
			wantCategory:  errs.CategoryAccount,
			wantCode:      types.StatusNotLoggedIn,
			wantReLogin:   true,
		},
		{
			name:          "a displaced session",
			body:          gatewayFailure(types.StatusKickedOffline, "session displaced"),
			wantRetryable: false,
			wantCategory:  errs.CategoryAccount,
			wantCode:      types.StatusKickedOffline,
			wantReLogin:   true,
		},
		{
			name:          "a login timeout",
			body:          gatewayFailure(types.StatusLoginTimeout, "login timeout"),
			wantRetryable: false,
			wantCategory:  errs.CategoryAccount,
			wantCode:      types.StatusLoginTimeout,
			wantReLogin:   true,
		},
		{
			name:          "a futures login timeout",
			body:          gatewayFailure(types.StatusFuturesLoginTimeout, "futures trade login timeout"),
			wantRetryable: false,
			wantCategory:  errs.CategoryAccount,
			wantCode:      types.StatusFuturesLoginTimeout,
			wantReLogin:   true,
		},
		{
			name:          "a user-not-authorized rejection",
			body:          gatewayFailure(types.StatusUserNotAuthorized, "user not yet authorized"),
			wantRetryable: false,
			wantCategory:  errs.CategoryAccount,
			wantCode:      types.StatusUserNotAuthorized,
			wantReLogin:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeLogin): tt.body,
			})
			svc := sessionWireService(newWireExecutor(t, rec))

			sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			requireZero(t, sess)
			if err == nil {
				t.Fatal("Login = nil error, want the rejection")
			}
			if got := errs.CategoryOf(err); got != tt.wantCategory {
				t.Errorf("CategoryOf = %q, want %q", got, tt.wantCategory)
			}
			if got := errs.Retryable(err); got != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", got, tt.wantRetryable)
			}
			if got := errs.ReLoginRequired(err); got != tt.wantReLogin {
				t.Errorf("ReLoginRequired = %v, want %v: 1006 is an authorization state rather "+
					"than a session that a re-login would fix, which is why this row differs "+
					"from the four session codes above it", got, tt.wantReLogin)
			}
			code, ok := errs.CodeOf(err)
			if ok != (tt.wantCode != "") || (ok && code != tt.wantCode) {
				t.Errorf("CodeOf = (%q, %v), want (%q, present=%v)", code, ok, tt.wantCode, tt.wantCode != "")
			}
		})
	}
}

// TestSessionLogoutErrorArms is the fake-level table for Logout, and every row
// also asserts what happened to the stored session. That second assertion matters
// more than the error: a Logout that failed but dropped the local session would
// leave a caller unable to tell whether it is still logged in, which is exactly
// the state the released layer's "left unchanged so the caller may reconcile"
// protects.
func TestSessionLogoutErrorArms(t *testing.T) {
	t.Run("no session means no request", func(t *testing.T) {
		// No scripted reply, so a request would fail the fixture.
		f := newSequencedExecutor(t)
		svc := sessionNewService(f, &sessionClock{now: 1_000})

		if err := svc.Logout(t.Context(), sessionAccount); err != nil {
			t.Fatalf("Logout with no session = %v, want nil: it must be idempotent", err)
		}
		requireCalls(t, f, 0)
	})

	t.Run("a transport failure keeps the session", func(t *testing.T) {
		sentinel := errors.New("connection reset by peer")
		store := newSessionStore()
		f := newSequencedExecutor(t, sessionAck(`{"success":true}`), sequencedReply{err: sentinel})
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))
		if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
			t.Fatalf("the priming Login: %v", err)
		}

		if err := svc.Logout(t.Context(), sessionAccount); !errors.Is(err, sentinel) {
			t.Errorf("Logout = %v, want the transport error back unchanged", err)
		}
		if _, held := store.holds(sessionAccount); !held {
			t.Error("a transport failure dropped the session: the Gateway's answer is unknown, so " +
				"the caller must be left able to reconcile")
		}
		if got := store.clears(); got != 0 {
			t.Errorf("the store was cleared %d time(s), want 0", got)
		}
	})

	t.Run("a refused body keeps the session", func(t *testing.T) {
		store := newSessionStore()
		f := newSequencedExecutor(t, sessionAck(`{"success":true}`), sessionAck(`{"success":false}`))
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))
		if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
			t.Fatalf("the priming Login: %v", err)
		}

		err := svc.Logout(t.Context(), sessionAccount)
		if err == nil {
			t.Fatal("Logout on {\"success\":false} = nil error, want a rejection")
		}
		if code, ok := errs.CodeOf(err); ok {
			t.Errorf("CodeOf = (%q, true), want no code for a refused body", code)
		}
		if _, held := store.holds(sessionAccount); !held {
			t.Error("the session was dropped by a refused logout: the caller must be able to reconcile")
		}
		if got := store.clears(); got != 0 {
			t.Errorf("the store was cleared %d time(s), want 0", got)
		}
	})

	t.Run("an acknowledged logout clears the session", func(t *testing.T) {
		store := newSessionStore()
		f := newSequencedExecutor(t, sessionAck(`{"success":true}`), sessionAck(`{"success":true}`))
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))
		if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
			t.Fatalf("the priming Login: %v", err)
		}
		if err := svc.Logout(t.Context(), sessionAccount); err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if _, held := store.holds(sessionAccount); held {
			t.Error("the session survived an acknowledged logout: a token captured before it must " +
				"stop resolving to a live session")
		}
		if got := store.clears(); got != 1 {
			t.Errorf("the store was cleared %d time(s), want exactly 1", got)
		}
	})
}

// TestSessionLogoutGatewayErrorArms is Logout's real-HTTP arm, so the typed error
// is one internal/transport built rather than one the fake handed over.
func TestSessionLogoutGatewayErrorArms(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeLogin):  gatewaySuccess(`{"success":true}`),
		string(client.RouteTradeLogout): gatewayFailure(types.StatusCallFailed, "call failed"),
	})
	svc := sessionWireService(newWireExecutor(t, rec))

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("the priming Login: %v", err)
	}
	errRejects(t, svc.Logout(t.Context(), sessionAccount), types.StatusCallFailed, opTradeLogout)
	if got := rec.count(string(client.RouteTradeLogout)); got != 1 {
		t.Errorf("requests to /trade/TradeLogout = %d, want 1 with the default policy", got)
	}
}

// TestSessionStoreFailureArms covers the store errors, which are the only errors
// in this file the Gateway never sees.
//
// The third row is the one that matters. internal/auth's Authenticator.Login
// discards the error from the read it performs *after* saving the session, so a
// store that fails there returns a zero session and a nil error. Left alone that
// reaches a caller as a session for the empty account, and each of the service's
// forty-nine other methods would then fail with a Gateway rejection that says
// nothing about the real cause. The service's own check on the way out turns it
// into a clear error instead.
func TestSessionStoreFailureArms(t *testing.T) {
	sentinel := errors.New("session store unavailable")

	t.Run("an unreadable store costs no request", func(t *testing.T) {
		store := newSessionStore()
		store.failReadOn, store.readErr = 1, sentinel
		// No scripted reply: a request would fail the fixture.
		f := newSequencedExecutor(t)
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))

		sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		requireZero(t, sess)
		if !errors.Is(err, sentinel) {
			t.Errorf("Login = %v, want the store error: an unreadable store must be reported "+
				"rather than turned into a credential on the wire", err)
		}
		requireCalls(t, f, 0)
	})

	t.Run("an unreadable store also stops a logout before the wire", func(t *testing.T) {
		// Logout's only store call is its own lookup, so read 1 is that one: with
		// no session readable there is nothing to reconcile against and no reason
		// to ask the Gateway to end something the SDK does not know it holds.
		store := newSessionStore()
		store.failReadOn, store.readErr = 1, sentinel
		f := newSequencedExecutor(t)
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))

		if err := svc.Logout(t.Context(), sessionAccount); !errors.Is(err, sentinel) {
			t.Errorf("Logout = %v, want the store error", err)
		}
		requireCalls(t, f, 0)
	})

	t.Run("an unwritable store fails a login the Gateway accepted", func(t *testing.T) {
		store := newSessionStore()
		store.saveErr = sentinel
		f := newSequencedExecutor(t, sessionAck(`{"success":true}`))
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))

		sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		requireZero(t, sess)
		if !errors.Is(err, sentinel) {
			t.Errorf("Login = %v, want the store error", err)
		}
		// The request *was* sent, because the store is not consulted until the
		// Gateway has answered. This is the one row in this file where a
		// credential-bearing request reaches the wire and the call still fails.
		requireCalls(t, f, 1)
		if got := store.saves(); got != 1 {
			t.Errorf("the store was written %d time(s), want 1", got)
		}
	})

	t.Run("a store that cannot be read back is reported, not returned as a zero session", func(t *testing.T) {
		store := newSessionStore()
		// Read 1 is Login's pre-flight check, read 2 is the leader's re-read, and
		// read 3 is the one internal/auth makes after saving and discards.
		store.failReadOn, store.readErr = 3, sentinel
		f := newSequencedExecutor(t, sessionAck(`{"success":true}`))
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))

		sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		requireZero(t, sess)
		if err == nil {
			t.Fatal("Login = a nil error. internal/auth discards the error from the read it makes " +
				"after saving, so the service must not hand back the zero session it received")
		}
		if !strings.Contains(err.Error(), "session store") {
			t.Errorf("error = %q, want it to name the session store as the thing that failed", err)
		}
	})

	t.Run("an unclearable store fails an acknowledged logout", func(t *testing.T) {
		store := newSessionStore()
		store.clearErr = sentinel
		f := newSequencedExecutor(t, sessionAck(`{"success":true}`), sessionAck(`{"success":true}`))
		svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))
		if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
			t.Fatalf("the priming Login: %v", err)
		}
		if err := svc.Logout(t.Context(), sessionAccount); !errors.Is(err, sentinel) {
			t.Errorf("Logout = %v, want the store error: the local clear failed, so reporting "+
				"success would leave a session the Gateway has already ended", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Single-flight
// ---------------------------------------------------------------------------

// TestSessionConcurrentLoginsIssueExactlyOneRequest is the centrepiece.
//
// The interleaving is built, not hoped for. The leader is parked inside the one
// request it is entitled to, and this file waits on the gate's own wait count to
// learn that every other caller has taken the join path - which it can only do
// while the leader's entry is still present, and the leader cannot remove that
// entry while it is parked. So by the time the leader is released, every caller
// is provably a joiner, and the request count is a measurement rather than a
// hope.
//
// Three properties are asserted, and the third is what makes the first worth
// having: every caller gets the leader's result, so a joiner is not merely quiet,
// it is correct.
func TestSessionConcurrentLoginsIssueExactlyOneRequest(t *testing.T) {
	const joiners = 8
	clk := &sessionClock{now: 1_000}
	exec := newSessionParkExecutor()
	exec.reply = json.RawMessage(`{"success":true}`)
	t.Cleanup(exec.unblock)

	store := newSessionStore()
	store.reads = make(chan struct{}, 4*joiners)
	svc := NewSessionService(exec, WithSessionClock(clk), WithSessionStore(store))

	type outcome struct {
		sess *domain.TradeSession
		err  error
	}
	results := make(chan outcome, joiners+1)

	// The leader. It parks inside Do and stays there until this file says so.
	go func() {
		sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		results <- outcome{sess: sess, err: err}
	}()
	exec.awaitCalls(t, 1)
	// The leader is parked, so it cannot read again: whatever is in the channel
	// is exactly its own pre-flight read and the leader's re-read.
	drainStoreReads(store.reads)

	// Take the gate, so a caller that has finished its store check cannot get any
	// further. Everything below is a fact about the code's control flow rather than
	// about the scheduler.
	svc.gate.mu.Lock()

	var wg sync.WaitGroup
	for i := 0; i < joiners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			results <- outcome{sess: sess, err: err}
		}()
	}
	for i := 0; i < joiners; i++ {
		<-store.reads
	}
	svc.gate.mu.Unlock()

	// The barrier. Every caller has now taken the join path, and only the join
	// path increments the count, so none of them is a leader.
	awaitWaiters(t, svc.gate, sessionAccount, joiners)
	if got := exec.callCount(); got != 1 {
		t.Errorf("requests so far = %d, want 1: a caller that had passed its store check must "+
			"not be able to become a second leader", got)
	}

	exec.unblock()
	wg.Wait()

	leader := <-results
	if leader.err != nil {
		t.Fatalf("the leader's Login: %v", leader.err)
	}
	if leader.sess == nil {
		t.Fatal("the leader received a nil session beside a nil error")
	}
	for i := 0; i < joiners; i++ {
		got := <-results
		if got.err != nil {
			t.Fatalf("joiner %d: %v; a joiner must receive the leader's outcome, not its own", i, got.err)
		}
		if got.sess == nil {
			t.Fatalf("joiner %d received a nil session beside a nil error", i)
		}
		if got.sess.Token != leader.sess.Token || got.sess.ExpiresAt != leader.sess.ExpiresAt {
			t.Errorf("joiner %d session = %+v, want the leader's %+v: a joiner must observe the "+
				"same session, not merely an equivalent one", i, got.sess, leader.sess)
		}
	}
	if got := exec.callCount(); got != 1 {
		t.Fatalf("requests to /trade/TradeLogin = %d, want exactly 1 for %d concurrent callers",
			got, joiners+1)
	}
	if got := exec.callAt(0).route; got != client.RouteTradeLogin {
		t.Errorf("the single request went to %s, want %s", got, client.RouteTradeLogin)
	}
	if got := exec.callAt(0).op; got != opTradeLogin {
		t.Errorf("op = %q, want %q", got, opTradeLogin)
	}
	// The gate is empty again, which is what a later caller relies on when it
	// finds no entry and starts a fresh attempt rather than adopting a stale
	// outcome.
	if got := svc.gate.waiting(sessionAccount); got != 0 {
		t.Errorf("waiting = %d, want 0 once the attempt is published", got)
	}
}

// TestSessionJoinersReceiveTheLeadersFailure is the other half of single-flight: a
// coalesced failure must reach every caller, and reach them as the same error
// rather than as N re-attempts.
//
// The failure path is the harder one and is why this test is not folded into the
// one above. A leader that succeeded leaves a session in the store, so a caller
// that wrongly became a second leader would re-read it and cost no request - the
// request count would not notice. A leader that failed leaves nothing, so the same
// defect shows up as a second request. This is A2's "a rule asserted on one branch
// is half a rule" in its purest form, and the assertion only means something here.
func TestSessionJoinersReceiveTheLeadersFailure(t *testing.T) {
	const joiners = 6
	clk := &sessionClock{now: 1_000}
	exec := newSessionParkExecutor()
	exec.err = sessionBusy(opTradeLogin)
	t.Cleanup(exec.unblock)

	store := newSessionStore()
	store.reads = make(chan struct{}, 4*joiners)
	svc := NewSessionService(exec, WithSessionClock(clk), WithSessionStore(store))

	type outcome struct {
		sess *domain.TradeSession
		err  error
	}
	results := make(chan outcome, joiners+1)

	go func() {
		_, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		results <- outcome{err: err}
	}()
	exec.awaitCalls(t, 1)
	drainStoreReads(store.reads)
	svc.gate.mu.Lock()

	var wg sync.WaitGroup
	for i := 0; i < joiners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			results <- outcome{sess: sess, err: err}
		}()
	}
	for i := 0; i < joiners; i++ {
		<-store.reads
	}
	svc.gate.mu.Unlock()
	awaitWaiters(t, svc.gate, sessionAccount, joiners)

	exec.unblock()
	wg.Wait()

	errRejects(t, (<-results).err, types.StatusServiceBusy, opTradeLogin)
	for i := 0; i < joiners; i++ {
		got := <-results
		requireZero(t, got.sess)
		errRejects(t, got.err, types.StatusServiceBusy, opTradeLogin)
	}
	if n := exec.callCount(); n != 1 {
		t.Errorf("requests to /trade/TradeLogin = %d, want 1: a coalesced failure must not be "+
			"re-attempted once per waiter, and on this branch a re-attempt is visible as a "+
			"second request because a failed login leaves no session behind", n)
	}
}

// TestSessionJoinerCancellationIsItsOwn pins the one outcome a joiner can have
// that the leader cannot: its own context ending while it waits.
//
// The arrangement is what makes the assertion safe. The joiner's context is
// cancelled while it is still pinned at the gate, so by the time it reaches the
// select the ctx.Done case is already closed and cannot lose a race against
// attempt.done - which is also open, because the leader is parked. There is no
// window in which the joiner could be said to have "probably" observed one case.
func TestSessionJoinerCancellationIsItsOwn(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	exec := newSessionParkExecutor()
	exec.reply = json.RawMessage(`{"success":true}`)
	t.Cleanup(exec.unblock)

	store := newSessionStore()
	store.reads = make(chan struct{}, 8)
	svc := NewSessionService(exec, WithSessionClock(clk), WithSessionStore(store))

	leaderDone := make(chan error, 1)
	go func() {
		_, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		leaderDone <- err
	}()
	exec.awaitCalls(t, 1)
	drainStoreReads(store.reads)

	ctx, cancel := context.WithCancel(t.Context())
	joinerDone := make(chan error, 1)
	svc.gate.mu.Lock()
	go func() {
		_, err := svc.Login(ctx, sessionAccount, sessionPassword)
		joinerDone <- err
	}()
	<-store.reads
	svc.gate.mu.Unlock()
	// The joiner has taken the join path, so the cancellation is already in
	// effect by the time it reaches the select - and its context being done cannot
	// be confused with the leader's outcome, which is the point of the row.
	awaitWaiters(t, svc.gate, sessionAccount, 1)
	cancel()

	if err := <-joinerDone; !errors.Is(err, context.Canceled) {
		t.Errorf("the joiner got %v, want context.Canceled: its own cancellation is not a Gateway "+
			"failure and must stay matchable with errors.Is", err)
	}
	if n := exec.callCount(); n != 1 {
		t.Errorf("requests = %d, want 1: a cancelled joiner must not fall through to a request", n)
	}

	// The leader is untouched: a joiner's cancellation is not the leader's outcome.
	exec.unblock()
	if err := <-leaderDone; err != nil {
		t.Errorf("the leader got %v, want nil: a joiner's cancellation must not become the "+
			"leader's outcome", err)
	}
	if n := exec.callCount(); n != 1 {
		t.Errorf("requests = %d, want 1 after the leader finished", n)
	}
}

// TestSessionDifferentAccountsLogInIndependently states the scope of the
// coalescing: it is per account, so a slow login for one account cannot make
// another wait.
//
// A single global gate would pass every other test in this file and would be wrong
// here: it would serialise two independent credentials behind one slow Gateway,
// so the single-flight property would come at the cost of a cross-account stall.
func TestSessionDifferentAccountsLogInIndependently(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	f := newSequencedExecutor(t,
		sessionAck(`{"data":"token-a"}`),
		sessionAck(`{"data":"token-b"}`),
	)
	svc := sessionNewService(f, clk)

	first, err := svc.Login(t.Context(), domain.AccountID("ACC-A"), sessionPassword)
	if err != nil {
		t.Fatalf("Login(ACC-A): %v", err)
	}
	second, err := svc.Login(t.Context(), domain.AccountID("ACC-B"), sessionPassword)
	if err != nil {
		t.Fatalf("Login(ACC-B): %v", err)
	}
	if first.AccountID != "ACC-A" || second.AccountID != "ACC-B" {
		t.Errorf("AccountIDs = %q, %q, want the two sessions filed under the keys they were given",
			first.AccountID, second.AccountID)
	}
	if first.Token == second.Token {
		t.Errorf("both accounts hold the token %q; each login must read its own reply", first.Token)
	}
	requireCalls(t, f, 2)
}

// TestSessionLeaderReusesASessionThatAppearedUnderIt is the stale-read row, and
// it is the one that makes the request count a property of the code rather than
// of a favourable interleaving.
//
// Login checks the store before it takes the gate, so a caller can read "no
// session" and then find the gate free because somebody else logged in in
// between. Without the leader's re-read that caller becomes a second leader and
// puts the password on the wire a second time for a session it already has.
//
// The window is built rather than raced for: the gate is held, the caller is
// started and observed to have missed the store, a session is then written into
// that store from the test, and only then is the gate released.
//
// The executor here scripts a reply rather than being left empty, and that is a
// correction rather than a style choice. An empty fixture answers an unexpected
// request with t.Fatalf, which is fine for a call on the test's own goroutine and
// illegal for one on a goroutine the test spawned - it Goexits that goroutine
// without the test ever returning, so the row *hangs* where it should fail. An
// earlier draft of this test did exactly that, and a hang is a far worse way to
// find out that a defect is detected. With a scripted reply the defect produces a
// real assertion failure naming the wrong token, and the request count is
// reported as a count.
func TestSessionLeaderReusesASessionThatAppearedUnderIt(t *testing.T) {
	store := newSessionStore()
	store.reads = make(chan struct{}, 8)
	// Two replies, so a second request cannot reach the fixture's Fatalf either.
	f := newSequencedExecutor(t,
		sessionAck(`{"data":"from-the-wire"}`),
		sessionAck(`{"data":"from-the-wire-again"}`),
	)
	svc := sessionNewService(f, &sessionClock{now: 1_000}, WithSessionStore(store))

	type outcome struct {
		sess *domain.TradeSession
		err  error
	}
	results := make(chan outcome, 1)

	svc.gate.mu.Lock()
	go func() {
		sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		results <- outcome{sess: sess, err: err}
	}()
	// The caller's pre-flight read, which misses: nothing is stored yet.
	<-store.reads
	if _, held := store.holds(sessionAccount); held {
		t.Fatal("the store already held a session; the test did not set up its own premise")
	}

	// Somebody else establishes one while our caller is pinned at the gate. The
	// timestamps are far enough ahead that the fixed clock reads it as live, which
	// is the point: the re-read must find a *usable* session, not merely any.
	written := auth.Session{
		Token:     "session-from-another-caller",
		AccountID: sessionAccount,
		Expiry:    1_000_000,
		RefreshAt: 999_000,
	}
	if err := store.SaveSession(t.Context(), written); err != nil {
		t.Fatalf("seeding the store: %v", err)
	}
	svc.gate.mu.Unlock()

	got := <-results
	if got.err != nil {
		t.Fatalf("Login: %v", got.err)
	}
	if got.sess == nil {
		t.Fatal("Login = a nil session beside a nil error")
	}
	if got.sess.Token != written.Token {
		t.Errorf("Token = %q, want the session that appeared under this caller (%q): a leader "+
			"must re-read before it spends a request, and the reply's own token is what a "+
			"missing re-read would have returned", got.sess.Token, written.Token)
	}
	if got.sess.ExpiresAt != written.Expiry || got.sess.RefreshAt != written.RefreshAt {
		t.Errorf("the reused session = %+v, want the stored %+v", got.sess, written)
	}
	requireCalls(t, f, 0)
}

// TestSessionLosingTheRaceDoesNotResendTheCredential is the same property stated
// as a count over callers that interleave freely, so it holds for every
// interleaving rather than for the one a scheduler happened to produce. It is a
// second control rather than a duplicate: the row above forces one specific
// interleaving, and this one does not choose.
func TestSessionLosingTheRaceDoesNotResendTheCredential(t *testing.T) {
	const callers = 24
	clk := &sessionClock{now: 1_000}
	exec := newSessionParkExecutor()
	exec.reply = json.RawMessage(`{"success":true}`)
	t.Cleanup(exec.unblock)

	svc := NewSessionService(exec, WithSessionClock(clk), WithSessionStore(newSessionStore()))

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// No gate pinning here: the point is that the callers interleave
			// freely and the request count must still come out at one.
			_, _ = svc.Login(t.Context(), sessionAccount, sessionPassword)
		}()
	}
	exec.awaitCalls(t, 1)
	exec.unblock()
	wg.Wait()

	if got := exec.callCount(); got != 1 {
		t.Fatalf("requests to /trade/TradeLogin = %d, want exactly 1 across %d concurrent callers: "+
			"a caller that read an empty store and then lost the race must find the session the "+
			"leader stored rather than send the credential again", got, callers)
	}
}

// ---------------------------------------------------------------------------
// Expiry, the refresh window, and clock drift
// ---------------------------------------------------------------------------

// TestSessionRefreshPolicy walks a session across its whole life with a clock this
// file sets by assignment. Nothing here sleeps and no row depends on the machine's
// speed: internal/auth's token manager was given the same clock, so the timestamps
// on the stored session and the service's refresh decision come from one source.
//
// The four rows are the four states the GoDoc on Login names: not yet due, due,
// expired, and inside the refresh window by a single second.
func TestSessionRefreshPolicy(t *testing.T) {
	const loginAt = 1_000
	// internal/auth's refresh window is ten minutes, not one: the numbers have to
	// be derived from the same constants the token manager used or the boundary
	// rows would be testing a window that does not exist.
	const refreshWindow = 10 * time.Minute
	refreshAt := loginAt + int64((3*time.Hour - refreshWindow).Seconds())
	expiresAt := loginAt + int64((3 * time.Hour).Seconds())

	tests := []struct {
		name      string
		now       int64
		wantCalls int
		whyIt     string
	}{
		{
			name:      "a live session is reused and costs no request",
			now:       loginAt + 30*60,
			wantCalls: 1,
			whyIt:     "the second Login is inside the refresh window, so it must not re-authenticate",
		},
		{
			name:      "one second inside the refresh window is still reused",
			now:       refreshAt - 1,
			wantCalls: 1,
			whyIt:     "the refresh point opens exactly at RefreshAt, not one second earlier",
		},
		{
			name:      "at the refresh point the session is re-established",
			now:       refreshAt,
			wantCalls: 2,
			whyIt:     "ShouldRefresh is inclusive, so the boundary itself is a refresh",
		},
		{
			name:      "past the expiry the session is re-established",
			now:       expiresAt + 60,
			wantCalls: 2,
			whyIt:     "an expired session is also past its refresh point",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newSessionStore()
			// Two scripted replies, because the refreshing rows issue a second
			// request and the reusing rows must leave the second one untouched.
			f := newSequencedExecutor(t, sessionAck(`{"data":"first"}`), sessionAck(`{"data":"second"}`))
			svc := sessionNewService(f, &sessionClock{now: loginAt}, WithSessionStore(store))

			first, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			if err != nil {
				t.Fatalf("the first Login: %v", err)
			}

			clk, ok := svc.clock.(*sessionClock)
			if !ok {
				t.Fatalf("the service's clock is %T, want the injected test clock", svc.clock)
			}
			clk.set(tt.now)

			second, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			if err != nil {
				t.Fatalf("the second Login: %v", err)
			}
			if got := f.callCount(); got != tt.wantCalls {
				t.Fatalf("requests = %d, want %d: %s", got, tt.wantCalls, tt.whyIt)
			}
			if tt.wantCalls == 1 {
				if second.Token != first.Token || second.ExpiresAt != first.ExpiresAt {
					t.Errorf("the reused session = %+v, want the one already held %+v", second, first)
				}
				return
			}
			if second.Token != "second" {
				t.Errorf("Token = %q, want %q from the refresh request", second.Token, "second")
			}
			if second.ExpiresAt <= first.ExpiresAt {
				t.Errorf("the refreshed session expires at %d, want a new expiry after %d",
					second.ExpiresAt, first.ExpiresAt)
			}
		})
	}
}

// TestSessionClockDriftInBothDirections covers the two directions separately,
// because they fail differently.
//
// Forward is the ordinary case: a session that has outlived its TTL is
// re-established. Backward is the interesting one. internal/auth's own security
// suite pins that a token past its expiry becomes acceptable again when the wall
// clock jumps back, because IsExpired is a pure comparison with no monotonic
// component; the service must not paper over that by refusing to reuse a session
// it has already stored, which would turn a clock step into an unexpected
// re-authentication on a live session.
func TestSessionClockDriftInBothDirections(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	store := newSessionStore()
	f := newSequencedExecutor(t, sessionAck(`{"data":"first"}`), sessionAck(`{"data":"second"}`))
	svc := sessionNewService(f, clk, WithSessionStore(store))

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("the priming Login: %v", err)
	}
	// Forward, past the expiry: the session is re-established.
	clk.advance(4 * time.Hour)

	refreshed, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("Login after drifting forward: %v", err)
	}
	if refreshed.Token != "second" {
		t.Errorf("Token = %q, want %q: a session past its expiry must be re-established",
			refreshed.Token, "second")
	}

	// Backward, to before even the first login: the same session is usable again
	// and nothing is corrupted or re-sent.
	clk.set(500)
	back, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("Login after drifting backward: %v", err)
	}
	if back.Token != "second" || back.ExpiresAt != refreshed.ExpiresAt {
		t.Errorf("the session after a backward drift = %+v, want the stored %+v unchanged",
			back, refreshed)
	}
	if got := f.callCount(); got != 2 {
		t.Errorf("requests = %d, want 2: a backward clock step must not re-authenticate", got)
	}
	if _, held := store.holds(sessionAccount); !held {
		t.Error("the session disappeared after a backward clock step")
	}
}

// TestSessionRefreshRejectedIsSurfacedAndNotRetried is the refresh-rejection row.
//
// A refresh the Gateway refuses must reach the caller, must cost exactly one
// attempt, and must leave the service able to try again - the three properties
// that together say "retried by the caller, never by the SDK".
func TestSessionRefreshRejectedIsSurfacedAndNotRetried(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	store := newSessionStore()
	f := newSequencedExecutor(t,
		sessionAck(`{"data":"first"}`),
		sequencedReply{err: sessionBusy(opTradeLogin)},
		sessionAck(`{"data":"third"}`),
	)
	svc := sessionNewService(f, clk, WithSessionStore(store))

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("the first Login: %v", err)
	}
	clk.advance(4 * time.Hour)

	sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	requireZero(t, sess)
	errRejects(t, err, types.StatusServiceBusy, opTradeLogin)
	if got := f.callCount(); got != 2 {
		t.Fatalf("requests = %d, want 2: a refused refresh is surfaced after one attempt and is "+
			"not retried by the SDK", got)
	}
	// A rate limit says nothing about the session, so the stored one is kept.
	if _, held := store.holds(sessionAccount); !held {
		t.Error("a rate limit on a refresh discarded the stored session: a transient failure is " +
			"not evidence about the session")
	}

	// The next call starts a fresh attempt rather than reporting the service as
	// permanently broken.
	third, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("the third Login: %v", err)
	}
	if third.Token != "third" {
		t.Errorf("Token = %q, want %q", third.Token, "third")
	}
	if got := f.callCount(); got != 3 {
		t.Errorf("requests = %d, want 3", got)
	}
}

// TestSessionDiscardsASessionTheGatewayRefused is the counterpart, and it is the
// row that keeps a dead session from being handed back.
//
// The bug it prevents is specific and total: if a session the Gateway has refused
// were kept, the next Login would find it in its store, find it inside its
// three-hour TTL, and return it - with no request and no error - until the TTL ran
// out. The service would be unable to log in again for three hours and nothing
// would say why. So an account-category rejection discards the session, and the
// next call re-authenticates.
func TestSessionDiscardsASessionTheGatewayRefused(t *testing.T) {
	// Every documented account-category code is a row, because that set is the
	// vocabulary the distinction is expressed in: 1006, 1012, 1013, 1014, 20033.
	for _, code := range []types.StatusCode{
		types.StatusUserNotAuthorized,
		types.StatusNotLoggedIn,
		types.StatusKickedOffline,
		types.StatusLoginTimeout,
		types.StatusFuturesLoginTimeout,
	} {
		t.Run(string(code), func(t *testing.T) {
			clk := &sessionClock{now: 1_000}
			store := newSessionStore()
			f := newSequencedExecutor(t,
				sessionAck(`{"data":"first"}`),
				sequencedReply{err: errs.New(code, opTradeLogin, "session no longer valid")},
				sessionAck(`{"data":"third"}`),
			)
			svc := sessionNewService(f, clk, WithSessionStore(store))

			if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
				t.Fatalf("the first Login: %v", err)
			}
			clk.advance(4 * time.Hour)

			sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			requireZero(t, sess)
			errRejects(t, err, code, opTradeLogin)
			if got := f.callCount(); got != 2 {
				t.Fatalf("requests = %d, want 2", got)
			}
			if _, held := store.holds(sessionAccount); held {
				t.Fatalf("the session the Gateway refused with %q is still held. The next Login "+
					"would return it - inside its TTL, with no request and no error - and the "+
					"service could not log in again until the TTL expired.", code)
			}

			third, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
			if err != nil {
				t.Fatalf("the third Login: %v", err)
			}
			if third.Token != "third" {
				t.Errorf("Token = %q, want %q: after a discarded session the next call must "+
					"re-authenticate", third.Token, "third")
			}
		})
	}
}

// TestSessionKeepsTheSessionWhenTheCredentialIsRefused is the row that keeps the
// two failure kinds apart, and it is a separate test because the two disagree
// about what the store should look like afterwards.
//
// A refused credential carries no status code and lands in CategoryUnknown, so
// the stored session is *not* discarded - there is nothing about the session in
// the answer, and throwing it away on a bad password would end a working session
// because someone mistyped. The stale session is still not reusable, because it
// is past its refresh point, so the next call re-authenticates anyway; what
// differs is that the service did not destroy state on the strength of a
// credential failure.
func TestSessionKeepsTheSessionWhenTheCredentialIsRefused(t *testing.T) {
	clk := &sessionClock{now: 1_000}
	store := newSessionStore()
	f := newSequencedExecutor(t,
		sessionAck(`{"data":"first"}`),
		sessionAck(`{"success":false}`),
		sessionAck(`{"data":"third"}`),
	)
	svc := sessionNewService(f, clk, WithSessionStore(store))

	if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
		t.Fatalf("the first Login: %v", err)
	}
	clk.advance(4 * time.Hour)

	sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	requireZero(t, sess)
	if err == nil {
		t.Fatal("Login on a refused body = nil error, want a rejection")
	}
	if got := f.callCount(); got != 2 {
		t.Fatalf("requests = %d, want 2: a refused credential is not retried", got)
	}
	if _, held := store.holds(sessionAccount); !held {
		t.Error("a refused credential discarded the stored session: a bad password is not evidence " +
			"that the session is bad, and discarding it would end a working session")
	}

	third, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("the third Login: %v", err)
	}
	if third.Token != "third" {
		t.Errorf("Token = %q, want %q", third.Token, "third")
	}
	if got := f.callCount(); got != 3 {
		t.Errorf("requests = %d, want 3", got)
	}
}

// TestSessionUnusableClassification pins the predicate itself rather than
// observing it through a service, so the two exclusions are stated as claims.
//
// The exclusions are the interesting rows. A rate limit and a timeout are not
// evidence about a session, and treating either as such would destroy a working
// session because the Gateway happened to be busy.
func TestSessionUnusableClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
		why  string
	}{
		{"nil", nil, false, "a nil cause is never a reason to discard a session"},
		{"not logged in", errs.New(types.StatusNotLoggedIn, "op", "x"), true, "1012"},
		{"user not authorized", errs.New(types.StatusUserNotAuthorized, "op", "x"), true,
			"1006 is a real answer about the account's authorization state"},
		{"kicked offline", errs.New(types.StatusKickedOffline, "op", "x"), true, "1013"},
		{"login timeout", errs.New(types.StatusLoginTimeout, "op", "x"), true, "1014"},
		{"futures login timeout", errs.New(types.StatusFuturesLoginTimeout, "op", "x"), true, "20033"},
		{"rate limit", errs.New(types.StatusServiceBusy, "op", "x"), false,
			"1011 says the Gateway was busy, not that the session is bad"},
		{"timeout", errs.Timeout("op", context.DeadlineExceeded), false,
			"a timeout says the SDK never learned the answer"},
		{"connection", errs.Connection("op", errors.New("dial tcp")), false,
			"a dropped connection is not evidence about the session"},
		{"a refused body", errs.New("", "op", "trade login was rejected"), false,
			"a refused credential says nothing about the session"},
		{"trading state", errs.New(types.StatusDuplicateSubmit, "op", "x"), false,
			"1007 is trading state, not a session failure"},
		{"an untagged error", errors.New("plain"), false,
			"an error the SDK did not classify is not a reason to destroy state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionUnusable(tt.err); got != tt.want {
				t.Errorf("sessionUnusable(%v) = %v, want %v: %s", tt.err, got, tt.want, tt.why)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The credential
// ---------------------------------------------------------------------------

// TestSessionPasswordIsNeverOnTheWireInAnyForm asserts the absence of the
// plaintext in all four places it could appear, at the real-HTTP level.
//
// Four is the number of places, not one assertion repeated: the request body, the
// error string, the returned struct, and a log. A caller who sees a password in
// any of them has been told something the SDK promised it would never say, and
// the log is the one that outlives the process.
//
// The positive half matters as much as the negative: the body must carry the
// published ciphertext, or "the password is absent" would be satisfied by a
// service that sent nothing at all.
func TestSessionPasswordIsNeverOnTheWireInAnyForm(t *testing.T) {
	var logBuf bytes.Buffer
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeLogin): gatewaySuccess(`{"data":"session-token-abc"}`),
	})
	exec := newWireExecutor(t, rec, client.WithLogger(slog.New(slog.NewTextHandler(&logBuf, nil))))
	svc := sessionWireService(exec)

	// 1. The request body.
	sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	body := rec.lastBody(t, string(client.RouteTradeLogin))
	if strings.Contains(body, sessionPassword) {
		t.Errorf("the request body carries the plaintext password:\n%s", body)
	}
	if !strings.Contains(body, sessionCiphertext) {
		t.Errorf("the request body does not carry the encrypted password:\n%s", body)
	}

	// 2. The error string. The Gateway refuses, so there is an error to render.
	refusing := newWireRecorder(map[string]string{
		string(client.RouteTradeLogin): gatewaySuccess(`{"success":false}`),
	})
	refusingSvc := sessionWireService(newWireExecutor(t, refusing))
	_, loginErr := refusingSvc.Login(t.Context(), sessionAccount, sessionPassword)
	if loginErr == nil {
		t.Fatal("Login on a refused body = nil error, want a rejection to render")
	}
	if strings.Contains(loginErr.Error(), sessionPassword) {
		t.Errorf("the error string carries the plaintext password: %q", loginErr.Error())
	}
	if strings.Contains(fmt.Sprintf("%+v", loginErr), sessionPassword) {
		t.Errorf("the error value carries the plaintext password: %+v", loginErr)
	}

	// 3. The returned struct.
	rendered, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("rendering the returned session: %v", err)
	}
	if strings.Contains(string(rendered), sessionPassword) {
		t.Errorf("the returned session carries the plaintext password: %s", rendered)
	}
	if strings.Contains(fmt.Sprintf("%+v", sess), sessionPassword) {
		t.Errorf("the returned session value carries the plaintext password: %+v", sess)
	}

	// 4. A log, through the SDK's own logger.
	if strings.Contains(logBuf.String(), sessionPassword) {
		t.Errorf("a log line carries the plaintext password:\n%s", logBuf.String())
	}
	if strings.Contains(logBuf.String(), sessionCiphertext) {
		t.Errorf("a log line carries the encrypted password:\n%s", logBuf.String())
	}
}

// TestSessionDeclaresNoCredentialStorage is the static half of the same property,
// and it is what stops the next change from quietly reintroducing a place to keep
// a secret.
//
// gosec runs in CI and has rejected credential-shaped fields in this repository
// before, but a linter cannot tell a field that is only ever written with a
// ciphertext from one that is written with a plaintext. So this package's own
// source is read as text and its struct declarations are checked: no field may be
// named after a secret, and the one field named after the password is allowed
// only on the wire request body.
//
// The trade password is a *parameter* of Login rather than a field anywhere, and
// that is a deliberate choice rather than an accident: a parameter is not stored
// state, so there is no struct here for one to be written into.
func TestSessionDeclaresNoCredentialStorage(t *testing.T) {
	source, err := readSessionSource()
	if err != nil {
		t.Fatalf("reading session.go: %v", err)
	}
	// A declaration line is a tab, an exported name, and a type - so a word in
	// prose, of which this package's GoDoc has a great many, cannot trip the check.
	secretish := []string{
		"Secret", "SecretKey", "APIKey", "PrivateKey", "Salt",
		"Credentials", "Credential", "Passphrase", "Token",
	}
	sawPasswordField := false
	for i, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(line, "\t") || trimmed == line {
			continue
		}
		name, _, found := strings.Cut(trimmed, " ")
		if !found || name == "" || name[0] < 'A' || name[0] > 'Z' {
			continue
		}
		for _, banned := range secretish {
			if name == banned {
				t.Errorf("session.go:%d declares a field named %s. The trade password is a "+
					"parameter of Login and the token is read straight off the reply; neither "+
					"needs a place to be stored.", i+1, banned)
			}
		}
		if name == "Password" {
			sawPasswordField = true
			if !strings.Contains(trimmed, `json:"password"`) {
				t.Errorf("session.go:%d declares a Password field that is not the wire body: "+
					"the only field allowed to carry that name is the request body, and what it "+
					"carries is the ciphertext", i+1)
			}
		}
	}
	if !sawPasswordField {
		t.Error("no Password field was found at all; the login request body must carry the " +
			"encrypted password under the Gateway's own key")
	}
}

// TestSessionPredicatesAgreeWithInternalAuth is the control on the one place this
// layer duplicates internal/auth's logic.
//
// domain.TradeSession.IsExpired and ShouldRefresh are the same two integer
// comparisons internal/auth makes on its own Session, and they cannot be shared:
// internal/auth is an internal package, so pkg/domain must not import it, and a
// caller outside the module can only see the domain copy. Duplication without a
// control is how the two drift, so they are run against each other over the
// boundaries that matter - either side of each timestamp, and the zero-timestamp
// case a hand-built session would never produce.
func TestSessionPredicatesAgreeWithInternalAuth(t *testing.T) {
	internal := []auth.Session{
		{Expiry: 1_000, RefreshAt: 500},
		{Expiry: 2_000, RefreshAt: 1_000},
		{Expiry: 2_000, RefreshAt: 1_999},
		{Expiry: 0, RefreshAt: 0},
	}
	clocks := []int64{0, 1, 499, 500, 501, 999, 1_000, 1_001, 1_999, 2_000, 2_001, 9_999}

	for _, isess := range internal {
		dsess := domain.TradeSession{ExpiresAt: isess.Expiry, RefreshAt: isess.RefreshAt}
		for _, now := range clocks {
			if got, want := dsess.IsExpired(now), isess.IsExpired(now); got != want {
				t.Errorf("expiry=%d refreshAt=%d now=%d: domain.IsExpired = %v, internal/auth's = %v",
					isess.Expiry, isess.RefreshAt, now, got, want)
			}
			if got, want := dsess.ShouldRefresh(now), isess.ShouldRefresh(now); got != want {
				t.Errorf("expiry=%d refreshAt=%d now=%d: domain.ShouldRefresh = %v, internal/auth's = %v",
					isess.Expiry, isess.RefreshAt, now, got, want)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

// TestSessionServiceConstructionResolvesTheCollaborators covers the three options
// and the defaults, because the constructor's shape is unusual: the options
// resolve into the service's fields and the authenticator is built from them
// afterwards, so a test that only ever passed one option would not notice the
// wiring reading a field before it was written.
func TestSessionServiceConstructionResolvesTheCollaborators(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		svc := NewSessionService(newSequencedExecutor(t))
		if svc.clock == nil {
			t.Error("clock = nil, want a default")
		}
		if svc.store == nil {
			t.Error("store = nil, want a default")
		}
		if svc.auth == nil {
			t.Error("auth = nil, want a composed authenticator")
		}
		if svc.gate == nil {
			t.Error("gate = nil, want a gate")
		}
		if got := svc.clock.Now(); got <= 0 {
			t.Errorf("the default clock reads %d, want a real time", got)
		}
	})

	t.Run("options win in order", func(t *testing.T) {
		first := &sessionClock{now: 111}
		second := &sessionClock{now: 222}
		storeA, storeB := newSessionStore(), newSessionStore()
		svc := NewSessionService(newSequencedExecutor(t),
			WithSessionClock(first), WithSessionStore(storeA),
			WithSessionClock(second), WithSessionStore(storeB),
		)
		if svc.clock != second {
			t.Errorf("clock = %v, want the last option's clock", svc.clock)
		}
		if svc.store != storeB {
			t.Error("store is not the last option's store; the last option must win")
		}
	})

	t.Run("a nil option restores the default", func(t *testing.T) {
		svc := NewSessionService(newSequencedExecutor(t), WithSessionClock(nil), WithSessionStore(nil))
		if svc.clock == nil || svc.store == nil {
			t.Fatalf("clock = %v, store = %v; a nil option must restore the defaults", svc.clock, svc.store)
		}
		// And the restored store is a working one rather than a nil interface
		// value, which the row above alone would not prove.
		sess, err := svc.Login(t.Context(), "", sessionPassword)
		requireZero(t, sess)
		assertInvalidParam(t, err, opTradeLogin)
	})

	t.Run("the client option replaces the request path", func(t *testing.T) {
		first := newSequencedExecutor(t, sequencedReply{err: errors.New("the replaced path")})
		second := newSequencedExecutor(t, sessionAck(`{"success":true}`))
		svc := NewSessionService(first,
			WithSessionClient(second), WithSessionClock(&sessionClock{now: 1_000}))
		if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
		if got := second.callCount(); got != 1 {
			t.Errorf("the replacement path recorded %d calls, want 1", got)
		}
		if got := first.callCount(); got != 0 {
			t.Errorf("the replaced path recorded %d calls, want 0", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Retry classification
// ---------------------------------------------------------------------------

// TestSessionRequestsRetryAndOrderMutationsDoNot states the retry behaviour of
// both session routes and, in the same test, the contrast with an order mutation.
//
// The two session rows are the interesting half. Neither route is in
// internal/resilience's mutation set, so the client classifies them as queries and
// a policy re-sends them - which is correct: a login is not an order mutation, so
// ADR 0003 does not apply, and re-logging in re-establishes the same session. A
// reader who assumed otherwise would add an unnecessary exclusion.
//
// The contrast row is what makes the session rows a measurement rather than a
// claim. Five requests can only come from a policy that is installed and firing,
// and one request for an order mutation under the identical policy is what says
// the two classes are being told apart rather than the policy being absent.
func TestSessionRequestsRetryAndOrderMutationsDoNot(t *testing.T) {
	const maxAttempts = 5
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")
	policy := client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})

	t.Run("a login retries", func(t *testing.T) {
		// Every path answers busy, not just the one under test, so a service that
		// quietly issued a request somewhere else would show up in total().
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeLogin):  busy,
			string(client.RouteTradeLogout): busy,
		})
		svc := sessionWireService(newWireExecutor(t, rec, policy))

		sess, err := svc.Login(t.Context(), sessionAccount, sessionPassword)
		requireZero(t, sess)
		errRejects(t, err, types.StatusServiceBusy, opTradeLogin)
		if got := rec.count(string(client.RouteTradeLogin)); got != maxAttempts {
			t.Errorf("requests to /trade/TradeLogin = %d, want %d. A session route is not an order "+
				"mutation, so a retry policy applies to it; the single-attempt rows elsewhere in "+
				"this package prove nothing unless this one does retry.", got, maxAttempts)
		}
		if got := rec.total(); got != maxAttempts {
			t.Errorf("total requests = %d, want %d", got, maxAttempts)
		}
	})

	t.Run("a logout retries", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeLogin):  gatewaySuccess(`{"success":true}`),
			string(client.RouteTradeLogout): busy,
		})
		svc := sessionWireService(newWireExecutor(t, rec, policy))

		if _, err := svc.Login(t.Context(), sessionAccount, sessionPassword); err != nil {
			t.Fatalf("the priming Login: %v", err)
		}
		errRejects(t, svc.Logout(t.Context(), sessionAccount), types.StatusServiceBusy, opTradeLogout)
		if got := rec.count(string(client.RouteTradeLogout)); got != maxAttempts {
			t.Errorf("requests to /trade/TradeLogout = %d, want %d: re-logging out a session that "+
				"is already gone is the same no-op twice", got, maxAttempts)
		}
	})

	t.Run("an order mutation does not", func(t *testing.T) {
		paths := map[string]string{
			string(client.RouteTradeLogin):  busy,
			string(client.RouteTradeLogout): busy,
		}
		for _, tc := range tradingMutationAttempts() {
			paths[string(tc.route)] = busy
		}
		rec := newWireRecorder(paths)
		svc := NewTradingService(newWireExecutor(t, rec, policy))

		for _, tc := range tradingMutationAttempts() {
			t.Run(tc.name, func(t *testing.T) {
				if err := tc.invoke(t.Context(), svc); err == nil {
					t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
				}
				if got := rec.count(string(tc.route)); got != 1 {
					t.Errorf("requests to %s = %d, want exactly 1 under the identical policy that "+
						"sent the two session routes %d times: ADR 0003's contrast is the only "+
						"evidence that the session rows are measuring the retry class rather than "+
						"a policy that never fired", tc.route, got, maxAttempts)
				}
			})
		}
	})
}
