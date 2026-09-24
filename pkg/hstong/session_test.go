// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package hstong

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/crypto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind once every test,
// including keep-alive shutdown, has run.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// defaultLoginOK is the successful /trade/TradeLogin response body.
const defaultLoginOK = `{"ok":true,"err":"","data":{"success":true}}`

// gatewayRecorder is an httptest handler that records every request path and
// body and serves a per-path canned response. All access is mutex-guarded so it
// is safe under -race while the client's goroutine and the test share it.
type gatewayRecorder struct {
	mu        sync.Mutex
	paths     []string
	bodies    [][]byte
	responses map[string]string
	delay     time.Duration
}

// ServeHTTP records the request and writes the canned response for its path (or
// defaultLoginOK when none is configured).
func (g *gatewayRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	g.mu.Lock()
	g.paths = append(g.paths, r.URL.Path)
	g.bodies = append(g.bodies, body)
	resp, ok := g.responses[r.URL.Path]
	delay := g.delay
	g.mu.Unlock()

	if !ok {
		resp = defaultLoginOK
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, resp)
}

// count returns how many requests hit path.
func (g *gatewayRecorder) count(path string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, p := range g.paths {
		if p == path {
			n++
		}
	}
	return n
}

// lastBody returns the most recent request body for path, or nil.
func (g *gatewayRecorder) lastBody(path string) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := len(g.paths) - 1; i >= 0; i-- {
		if g.paths[i] == path {
			return g.bodies[i]
		}
	}
	return nil
}

// newGateway starts a recorder-backed test server and registers its shutdown.
func newGateway(t *testing.T, responses map[string]string, delay time.Duration) (*gatewayRecorder, *httptest.Server) {
	t.Helper()
	rec := &gatewayRecorder{responses: responses, delay: delay}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	return rec, srv
}

// newTestClient builds a client pointed at baseURL with the given trade
// password and registers its close. An empty password leaves it unset.
func newTestClient(t *testing.T, baseURL, password string) *client.Client {
	t.Helper()
	c, err := client.New(client.WithBaseURL(baseURL), client.WithTradePassword(password))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// countingTransport records, per URL path, the requests a client issues. It
// counts at RoundTrip entry, so the count is fixed the instant a request is
// handed to the transport. A server-side recorder, by contrast, can observe an
// already-aborted in-flight request after the issuing goroutine has exited,
// which makes it unsuitable for proving that a loop stopped issuing requests.
type countingTransport struct {
	base  http.RoundTripper
	mu    sync.Mutex
	paths []string
}

// RoundTrip records the request path and forwards to the wrapped transport.
func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.paths = append(t.paths, req.URL.Path)
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}

// count returns how many requests were issued to path.
func (t *countingTransport) count(path string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, p := range t.paths {
		if p == path {
			n++
		}
	}
	return n
}

// CloseIdleConnections releases the wrapped transport's idle connections so the
// client's Close does not leave goroutines behind for goleak.
func (t *countingTransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// newCountingTestClient builds a client pointed at baseURL whose transport
// counts issued requests. The returned counter distinguishes requests the
// client actually sent from requests a server merely happened to record.
func newCountingTestClient(t *testing.T, baseURL, password string) (*client.Client, *countingTransport) {
	t.Helper()

	var base = http.DefaultTransport
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		base = tr.Clone()
	}
	ct := &countingTransport{base: base}

	c, err := client.New(
		client.WithBaseURL(baseURL),
		client.WithTradePassword(password),
		client.WithHTTPClient(&http.Client{Transport: ct, Timeout: client.DefaultTimeout}),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, ct
}

// waitKeepAliveStopped blocks until the keep-alive loop goroutine has exited,
// using the same done channel StopKeepAlive waits on. It makes the sample taken
// after context cancellation deterministic instead of relying on a fixed sleep.
func waitKeepAliveStopped(t *testing.T, sm *SessionManager) {
	t.Helper()
	sm.kaMu.Lock()
	done := sm.kaDone
	sm.kaMu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("keep-alive loop did not stop after cancellation")
	}
}

// waitFor blocks until cond is true or timeout elapses.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

// TestSessionManager_LoginSuccess asserts a successful login moves the manager
// to LoggedIn, posts to /trade/TradeLogin, and sends the encrypted password,
// never the plaintext.
func TestSessionManager_LoginSuccess(t *testing.T) {
	const password = "123456"

	rec, srv := newGateway(t, nil, 0)
	c := newTestClient(t, srv.URL, password)
	sm := NewSessionManager(c)

	if err := sm.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !sm.IsLoggedIn() {
		t.Fatal("IsLoggedIn() = false, want true after successful Login")
	}
	if got := rec.count(string(client.RouteTradeLogin)); got != 1 {
		t.Fatalf("TradeLogin requests = %d, want 1", got)
	}

	wantEncrypted, err := crypto.EncryptTradePassword(password)
	if err != nil {
		t.Fatalf("EncryptTradePassword: %v", err)
	}

	var env struct {
		Params struct {
			Password string `json:"password"`
		} `json:"params"`
	}
	body := rec.lastBody(string(client.RouteTradeLogin))
	if len(body) == 0 {
		t.Fatal("TradeLogin request body is empty")
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode login envelope: %v (body=%s)", err, body)
	}
	if env.Params.Password != wantEncrypted {
		t.Fatalf("password = %q, want encrypted %q", env.Params.Password, wantEncrypted)
	}
	if env.Params.Password == password {
		t.Fatal("password was sent in plaintext")
	}
}

// TestSessionManager_LoginRejected asserts an ok:false envelope becomes a typed
// error and leaves the manager logged out.
func TestSessionManager_LoginRejected(t *testing.T) {
	const password = "123456"

	rec, srv := newGateway(t, map[string]string{
		string(client.RouteTradeLogin): `{"ok":false,"err":"1012 not logged in"}`,
	}, 0)
	c := newTestClient(t, srv.URL, password)
	sm := NewSessionManager(c)

	err := sm.Login(context.Background())
	if err == nil {
		t.Fatal("Login = nil error, want typed error")
	}
	code, ok := errs.CodeOf(err)
	if !ok || code != types.StatusNotLoggedIn {
		t.Fatalf("CodeOf(err) = (%q, %v), want (%q, true)", code, ok, types.StatusNotLoggedIn)
	}
	if !errs.ReLoginRequired(err) {
		t.Fatal("ReLoginRequired(err) = false, want true for 1012")
	}
	if sm.IsLoggedIn() {
		t.Fatal("IsLoggedIn() = true, want false after rejected login")
	}
	if rec.count(string(client.RouteTradeLogin)) != 1 {
		t.Fatalf("TradeLogin requests = %d, want 1", rec.count(string(client.RouteTradeLogin)))
	}
	if body := string(rec.lastBody(string(client.RouteTradeLogin))); strings.Contains(body, password) {
		t.Fatalf("request/error leaked the plaintext password: %s", body)
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("error leaked the plaintext password: %v", err)
	}
}

// TestSessionManager_LoginMissingPassword asserts Login and EnsureLoggedIn fail
// without a configured password and without sending any request.
func TestSessionManager_LoginMissingPassword(t *testing.T) {
	rec, srv := newGateway(t, nil, 0)
	c := newTestClient(t, srv.URL, "")
	sm := NewSessionManager(c)

	if err := sm.Login(context.Background()); err == nil {
		t.Fatal("Login = nil error, want error for missing password")
	}
	if err := sm.EnsureLoggedIn(context.Background()); err == nil {
		t.Fatal("EnsureLoggedIn = nil error, want error for missing password")
	}
	if sm.IsLoggedIn() {
		t.Fatal("IsLoggedIn() = true, want false")
	}
	if got := rec.count(string(client.RouteTradeLogin)); got != 0 {
		t.Fatalf("TradeLogin requests = %d, want 0", got)
	}
}

// TestSessionManager_LogoutWhenLoggedOut asserts Logout is a no-op that sends
// no request, and that the logged-in path posts exactly one logout.
func TestSessionManager_LogoutWhenLoggedOut(t *testing.T) {
	rec, srv := newGateway(t, nil, 0)
	c := newTestClient(t, srv.URL, "123456")
	sm := NewSessionManager(c)

	if err := sm.Logout(context.Background()); err != nil {
		t.Fatalf("Logout while logged out = %v, want nil", err)
	}
	if got := rec.count(string(client.RouteTradeLogout)); got != 0 {
		t.Fatalf("TradeLogout requests = %d, want 0", got)
	}
	if got := rec.count(string(client.RouteTradeLogin)); got != 0 {
		t.Fatalf("TradeLogin requests = %d, want 0", got)
	}

	if err := sm.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := sm.Logout(context.Background()); err != nil {
		t.Fatalf("Logout while logged in = %v, want nil", err)
	}
	if sm.IsLoggedIn() {
		t.Fatal("IsLoggedIn() = true, want false after Logout")
	}
	if got := rec.count(string(client.RouteTradeLogout)); got != 1 {
		t.Fatalf("TradeLogout requests = %d, want 1", got)
	}
}

// TestSessionManager_EnsureLoggedInConcurrent asserts 32 racing callers produce
// exactly one /trade/TradeLogin request and all observe success.
func TestSessionManager_EnsureLoggedInConcurrent(t *testing.T) {
	const (
		goroutines = 32
		password   = "123456"
	)

	rec, srv := newGateway(t, nil, 25*time.Millisecond)
	c := newTestClient(t, srv.URL, password)
	sm := NewSessionManager(c)

	start := make(chan struct{})
	errCh := make(chan error, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := sm.EnsureLoggedIn(context.Background()); err != nil {
				errCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("EnsureLoggedIn: %v", err)
	}
	if got := rec.count(string(client.RouteTradeLogin)); got != 1 {
		t.Fatalf("TradeLogin requests = %d, want exactly 1", got)
	}
	if !sm.IsLoggedIn() {
		t.Fatal("IsLoggedIn() = false, want true")
	}
}

// TestSessionManager_ReLogin asserts ReLogin forces exactly one login for a
// re-login error and is a no-op for an unrelated error.
func TestSessionManager_ReLogin(t *testing.T) {
	t.Run("relogin cause is handled", func(t *testing.T) {
		rec, srv := newGateway(t, nil, 0)
		c := newTestClient(t, srv.URL, "123456")
		sm := NewSessionManager(c)

		cause := errs.New(types.StatusLoginTimeout, "trade/TradeEntrust", "")
		handled, err := sm.ReLogin(context.Background(), cause)
		if err != nil {
			t.Fatalf("ReLogin: %v", err)
		}
		if !handled {
			t.Fatal("ReLogin handled = false, want true for 1014")
		}
		if got := rec.count(string(client.RouteTradeLogin)); got != 1 {
			t.Fatalf("TradeLogin requests = %d, want exactly 1", got)
		}
		if !sm.IsLoggedIn() {
			t.Fatal("IsLoggedIn() = false, want true after ReLogin")
		}
	})

	t.Run("unrelated cause is ignored", func(t *testing.T) {
		rec, srv := newGateway(t, nil, 0)
		c := newTestClient(t, srv.URL, "123456")
		sm := NewSessionManager(c)

		handled, err := sm.ReLogin(context.Background(), errors.New("boom"))
		if err != nil {
			t.Fatalf("ReLogin: %v", err)
		}
		if handled {
			t.Fatal("ReLogin handled = true, want false for unrelated error")
		}
		if got := rec.count(string(client.RouteTradeLogin)); got != 0 {
			t.Fatalf("TradeLogin requests = %d, want 0", got)
		}
	})
}

// TestSessionManager_KeepAlive asserts the poll fires repeatedly and then
// stops growing after StopKeepAlive and after context cancellation.
func TestSessionManager_KeepAlive(t *testing.T) {
	const password = "123456"
	keepAlivePath := string(client.RouteTradeQueryMarginFundInfo)

	t.Run("StopKeepAlive halts the loop", func(t *testing.T) {
		_, srv := newGateway(t, nil, 0)
		c, issued := newCountingTestClient(t, srv.URL, password)
		sm := NewSessionManager(c, WithKeepAliveInterval(10*time.Millisecond))
		t.Cleanup(sm.StopKeepAlive)

		sm.StartKeepAlive(context.Background())
		waitFor(t, func() bool { return issued.count(keepAlivePath) >= 2 }, 2*time.Second)

		// StopKeepAlive joins the loop, so no further request can be issued
		// once it returns; the sample is deterministic.
		sm.StopKeepAlive()
		stopped := issued.count(keepAlivePath)
		time.Sleep(80 * time.Millisecond)
		if got := issued.count(keepAlivePath); got != stopped {
			t.Fatalf("keep-alive requests grew from %d to %d after StopKeepAlive", stopped, got)
		}
		// Idempotent: a second stop must not panic or block.
		sm.StopKeepAlive()
	})

	t.Run("context cancellation halts the loop", func(t *testing.T) {
		_, srv := newGateway(t, nil, 0)
		c, issued := newCountingTestClient(t, srv.URL, password)
		sm := NewSessionManager(c, WithKeepAliveInterval(10*time.Millisecond))
		t.Cleanup(sm.StopKeepAlive)

		ctx, cancel := context.WithCancel(context.Background())
		sm.StartKeepAlive(ctx)
		waitFor(t, func() bool { return issued.count(keepAlivePath) >= 2 }, 2*time.Second)

		// Wait for the loop to observe cancellation and exit before sampling,
		// rather than sleeping for an arbitrary fixed duration.
		cancel()
		waitKeepAliveStopped(t, sm)
		stopped := issued.count(keepAlivePath)
		time.Sleep(80 * time.Millisecond)
		if got := issued.count(keepAlivePath); got != stopped {
			t.Fatalf("keep-alive requests grew from %d to %d after context cancel", stopped, got)
		}
		sm.StopKeepAlive()
	})
}
