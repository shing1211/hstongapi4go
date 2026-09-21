// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package hstong

import (
	"context"
	"sync"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/crypto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/internal/session"
)

// Operation labels used in errors raised by SessionManager. They never carry
// request payloads or credentials.
const (
	opTradeLogin  = "trade/TradeLogin"
	opTradeLogout = "trade/TradeLogout"
)

// DefaultKeepAliveInterval is the keep-alive poll period applied when no
// WithKeepAliveInterval option is supplied. It is deliberately conservative:
// the trading token lives for three hours and is extended by activity, so a
// half-hour poll refreshes it long before expiry while generating little load.
const DefaultKeepAliveInterval = 30 * time.Minute

// tradeLoginParams is the JSON body of /trade/TradeLogin. Password is the
// Base64 of the AES-ECB/PKCS7 ciphertext produced by crypto.EncryptTradePassword,
// never the plaintext.
type tradeLoginParams struct {
	Password string `json:"password"`
}

// commonBoolResponse is the shared {"success":bool} body returned by session
// and other simple Gateway endpoints.
type commonBoolResponse struct {
	Success bool `json:"success"`
}

// SessionOption configures a SessionManager. Options are applied in order on
// top of the defaults inside NewSessionManager; the last option that sets a
// field wins.
type SessionOption func(*sessionConfig)

// sessionConfig holds the resolved SessionManager settings.
type sessionConfig struct {
	keepAliveInterval time.Duration
	keepAliveRoute    client.Route
}

// defaultSessionConfig returns the configuration applied before any option.
func defaultSessionConfig() sessionConfig {
	return sessionConfig{
		keepAliveInterval: DefaultKeepAliveInterval,
		keepAliveRoute:    client.RouteTradeQueryMarginFundInfo,
	}
}

// WithKeepAliveInterval sets the period between keep-alive polls. The default
// is DefaultKeepAliveInterval. A non-positive value restores the default.
func WithKeepAliveInterval(interval time.Duration) SessionOption {
	return func(c *sessionConfig) {
		if interval <= 0 {
			c.keepAliveInterval = DefaultKeepAliveInterval
			return
		}
		c.keepAliveInterval = interval
	}
}

// WithKeepAliveRoute sets the endpoint polled to keep the trading token alive.
// The default is client.RouteTradeQueryMarginFundInfo, a cheap read that proves
// the session is still valid. An empty route restores the default. The route
// must be registered in the client package; an unregistered route makes each
// poll return an error, which the keep-alive loop ignores.
func WithKeepAliveRoute(route client.Route) SessionOption {
	return func(c *sessionConfig) {
		if route == "" {
			c.keepAliveRoute = client.RouteTradeQueryMarginFundInfo
			return
		}
		c.keepAliveRoute = route
	}
}

// SessionManager owns the trade session against the Gateway. It performs
// login and logout, coalesces concurrent logins into a single request, forces a
// re-login when an error indicates the session is invalid, and optionally polls
// a cheap endpoint to extend the token.
//
// All methods are safe for concurrent use. A SessionManager must not be copied
// after first use.
type SessionManager struct {
	client            *client.Client
	keepAliveInterval time.Duration
	keepAliveRoute    client.Route

	state session.Machine

	// mu guards state transitions that involve waiting for an in-flight login,
	// together with loginErr and cond. It is held only for bookkeeping, never
	// across an HTTP call, so a slow Gateway never blocks IsLoggedIn.
	mu       sync.Mutex
	cond     *sync.Cond
	loginErr error

	kaMu     sync.Mutex
	kaCancel context.CancelFunc
	kaDone   chan struct{}
}

// NewSessionManager returns a SessionManager over c with opts applied in order
// on top of the defaults. c must be non-nil and is not owned by the manager:
// closing it is the caller's responsibility. The returned manager starts
// LoggedOut; call Login or EnsureLoggedIn before authenticated calls.
func NewSessionManager(c *client.Client, opts ...SessionOption) *SessionManager {
	cfg := defaultSessionConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.keepAliveInterval <= 0 {
		cfg.keepAliveInterval = DefaultKeepAliveInterval
	}
	if cfg.keepAliveRoute == "" {
		cfg.keepAliveRoute = client.RouteTradeQueryMarginFundInfo
	}

	s := &SessionManager{
		client:            c,
		keepAliveInterval: cfg.keepAliveInterval,
		keepAliveRoute:    cfg.keepAliveRoute,
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// Login performs a trade login whether or not a session is believed to exist.
// It encrypts the configured trade password and posts it to /trade/TradeLogin.
//
// If no trade password is configured (client.TradePassword is empty), Login
// returns a typed error without sending a request. A Gateway rejection or a
// {"success":false} body moves the manager back to LoggedOut and is returned as
// a typed *errs.Error; the plaintext password is never included in any error.
//
// Login serializes with EnsureLoggedIn and ReLogin: it waits for an in-flight
// login to settle, then performs its own attempt.
func (s *SessionManager) Login(ctx context.Context) error {
	s.mu.Lock()
	for s.state.State() == session.LoggingIn {
		s.cond.Wait()
	}
	s.state.Logout()
	if !s.state.BeginLogin() {
		s.mu.Unlock()
		return errs.New("", opTradeLogin, "session login is already in progress")
	}
	s.mu.Unlock()

	return s.finishLogin(ctx)
}

// Logout posts /trade/TradeLogout and, on success, moves the manager to
// LoggedOut. It is idempotent: when the manager is not LoggedIn it returns nil
// without sending a request. A Gateway error is returned and the state is left
// unchanged so the caller may reconcile.
func (s *SessionManager) Logout(ctx context.Context) error {
	if !s.IsLoggedIn() {
		return nil
	}
	if err := s.client.Do(ctx, opTradeLogout, client.RouteTradeLogout, nil, s.client.JSON(), nil); err != nil {
		return err
	}

	s.mu.Lock()
	s.state.Logout()
	s.loginErr = nil
	s.cond.Broadcast()
	s.mu.Unlock()
	return nil
}

// IsLoggedIn reports whether the manager currently believes it holds a valid
// trading token.
func (s *SessionManager) IsLoggedIn() bool {
	return s.state.State() == session.LoggedIn
}

// EnsureLoggedIn returns nil immediately when the manager is already LoggedIn.
// Otherwise it performs a single-flight login: exactly one caller issues the
// /trade/TradeLogin request while every other concurrent caller waits and then
// observes that same outcome. On success all callers return nil; on failure all
// callers return the leader's error. A later call after a failure starts a
// fresh attempt.
//
// EnsureLoggedIn is the preferred gate before an authenticated call. It does
// not by itself re-login a session the Gateway has rejected; use ReLogin for
// that, driven by errs.ReLoginRequired.
func (s *SessionManager) EnsureLoggedIn(ctx context.Context) error {
	s.mu.Lock()
	if s.state.State() == session.LoggedIn {
		s.mu.Unlock()
		return nil
	}
	if s.state.State() == session.LoggingIn || !s.state.BeginLogin() {
		for s.state.State() == session.LoggingIn {
			s.cond.Wait()
		}
		err := s.loginErr
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	return s.finishLogin(ctx)
}

// ReLogin forces a fresh login when cause indicates the session is no longer
// valid. It reports whether it handled the error:
//
//   - When errs.ReLoginRequired(cause) is true it discards the current state
//     (waiting for any in-flight login to settle), forces a new login, and
//     returns (true, err) where err is the login's outcome.
//   - Otherwise it returns (false, nil) and sends no request.
//
// A nil cause is never a re-login trigger.
func (s *SessionManager) ReLogin(ctx context.Context, cause error) (handled bool, err error) {
	if !errs.ReLoginRequired(cause) {
		return false, nil
	}

	s.mu.Lock()
	for s.state.State() == session.LoggingIn {
		s.cond.Wait()
	}
	s.state.Logout()
	s.mu.Unlock()

	return true, s.EnsureLoggedIn(ctx)
}

// StartKeepAlive starts a goroutine that, once per keep-alive interval, ensures
// the session is logged in and then calls the keep-alive route with the JSON
// codec, ignoring its response. The loop stops when StopKeepAlive is called or
// when ctx is cancelled.
//
// Calling StartKeepAlive while a loop is already running is a no-op; the
// existing loop, and its interval, are kept. To change the interval or restart,
// call StopKeepAlive first. StartKeepAlive is safe for concurrent use.
func (s *SessionManager) StartKeepAlive(ctx context.Context) {
	s.kaMu.Lock()
	defer s.kaMu.Unlock()
	if s.kaCancel != nil {
		return
	}

	kaCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.kaCancel = cancel
	s.kaDone = done
	go s.keepAliveLoop(kaCtx, done)
}

// StopKeepAlive stops the keep-alive loop started by StartKeepAlive and waits
// for its goroutine to exit. It is idempotent and safe to call concurrently and
// when no loop is running. The loop also stops on its own when the context
// passed to StartKeepAlive is cancelled.
func (s *SessionManager) StopKeepAlive() {
	s.kaMu.Lock()
	cancel := s.kaCancel
	done := s.kaDone
	s.kaCancel = nil
	s.kaDone = nil
	s.kaMu.Unlock()

	if cancel == nil || done == nil {
		return
	}
	cancel()
	<-done
}

// finishLogin performs the HTTP login for a caller that has already moved the
// state machine into LoggingIn, publishes the outcome, and wakes every waiter.
func (s *SessionManager) finishLogin(ctx context.Context) error {
	err := s.doLogin(ctx)

	s.mu.Lock()
	if err == nil {
		s.state.LoginSucceeded()
	} else {
		s.state.LoginFailed()
	}
	s.loginErr = err
	s.cond.Broadcast()
	s.mu.Unlock()

	return err
}

// doLogin encrypts the configured trade password and posts it to the Gateway.
// It never includes the plaintext password in an error.
func (s *SessionManager) doLogin(ctx context.Context) error {
	password := s.client.TradePassword()
	if password == "" {
		return errs.New("", opTradeLogin, "trade password is not configured")
	}

	encrypted, err := crypto.EncryptTradePassword(password)
	if err != nil {
		return errs.Wrap(err, "", opTradeLogin, "encrypt trade password")
	}

	var resp commonBoolResponse
	if err := s.client.Do(ctx, opTradeLogin, client.RouteTradeLogin, tradeLoginParams{Password: encrypted}, s.client.JSON(), &resp); err != nil {
		return err
	}
	if !resp.Success {
		return errs.New("", opTradeLogin, "trade login was rejected")
	}
	return nil
}

// keepAliveLoop polls until ctx is cancelled or done is closed.
func (s *SessionManager) keepAliveLoop(ctx context.Context, done chan struct{}) {
	defer func() {
		close(done)
		s.kaMu.Lock()
		if s.kaDone == done {
			s.kaCancel = nil
			s.kaDone = nil
		}
		s.kaMu.Unlock()
	}()

	ticker := time.NewTicker(s.keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.keepAliveOnce(ctx)
		}
	}
}

// keepAliveOnce ensures the session is logged in and issues one keep-alive
// request. Every outcome, including errors, is intentionally ignored: the poll
// is best-effort and must never terminate the loop or surface to the caller.
func (s *SessionManager) keepAliveOnce(ctx context.Context) {
	if err := s.EnsureLoggedIn(ctx); err != nil {
		return
	}
	_ = s.client.Do(ctx, s.keepAliveRoute.String(), s.keepAliveRoute, nil, s.client.JSON(), nil)
}
