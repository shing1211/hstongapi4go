// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/session"
	"github.com/shing1211/hstongapi4go/pkg/domain"
)

type fakeStore struct {
	sessions map[domain.AccountID]Session
}

func (f *fakeStore) SaveSession(ctx context.Context, s Session) error {
	f.sessions[s.AccountID] = s
	return nil
}

func (f *fakeStore) Session(ctx context.Context, id domain.AccountID) (Session, bool, error) {
	s, ok := f.sessions[id]
	return s, ok, nil
}

func (f *fakeStore) ClearSession(ctx context.Context, id domain.AccountID) error {
	delete(f.sessions, id)
	return nil
}

func (f *fakeStore) Close() error { return nil }

func TestAuthenticatorLoginSuccess(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	loginCalled := false
	loginFn := func(ctx context.Context, req LoginRequest) (string, error) {
		loginCalled = true
		if req.TradePassword == "" {
			t.Error("loginFn received empty encrypted password")
		}
		return "test-token-abc", nil
	}
	a := NewAuthenticator(store, loginFn,
		WithClock(fakeClock(1000)),
		WithTokenTTL(1*time.Hour),
	)

	ctx := context.Background()
	sess, err := a.Login(ctx, LoginRequest{
		AccountID:      "ACC123",
		TradePassword:  "hunter2",
		EncryptionSalt: "salt",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !loginCalled {
		t.Error("loginFn was not called")
	}
	if sess.Token != "test-token-abc" {
		t.Errorf("Token = %q, want %q", sess.Token, "test-token-abc")
	}
	if sess.AccountID != "ACC123" {
		t.Errorf("AccountID = %q, want %q", sess.AccountID, "ACC123")
	}
}

func TestAuthenticatorLoginFnError(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	wantErr := errors.New("gateway error")
	loginFn := func(ctx context.Context, req LoginRequest) (string, error) {
		return "", wantErr
	}
	a := NewAuthenticator(store, loginFn)

	ctx := context.Background()
	_, err := a.Login(ctx, LoginRequest{
		AccountID:      "ACC123",
		TradePassword:  "hunter2",
		EncryptionSalt: "salt",
	})
	if err != wantErr {
		t.Errorf("Login() error = %v, want %v", err, wantErr)
	}
}

func TestAuthenticatorLogout(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	store.sessions["ACC123"] = Session{AccountID: "ACC123", Token: "tok"}

	loginFn := func(ctx context.Context, req LoginRequest) (string, error) {
		return "tok", nil
	}
	a := NewAuthenticator(store, loginFn)

	ctx := context.Background()
	err := a.Logout(ctx, "ACC123")
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	_, ok := store.sessions["ACC123"]
	if ok {
		t.Error("session still present after Logout")
	}
}

func TestAuthenticatorGetSession(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	store.sessions["ACC123"] = Session{AccountID: "ACC123", Token: "tok", Expiry: 2000}

	loginFn := func(ctx context.Context, req LoginRequest) (string, error) {
		return "tok", nil
	}
	a := NewAuthenticator(store, loginFn, WithClock(fakeClock(1000)))

	ctx := context.Background()
	sess, ok, err := a.GetSession(ctx, "ACC123")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if !ok {
		t.Fatal("GetSession() ok = false, want true")
	}
	if sess.Token != "tok" {
		t.Errorf("Token = %q, want %q", sess.Token, "tok")
	}

	_, ok, err = a.GetSession(ctx, "ACC999")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if ok {
		t.Error("GetSession() for unknown account: ok = true, want false")
	}
}

func TestAuthenticatorMustBeAuthenticated(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	store.sessions["ACC123"] = Session{AccountID: "ACC123", Token: "tok", Expiry: 2000}

	loginFn := func(ctx context.Context, req LoginRequest) (string, error) {
		return "tok", nil
	}
	a := NewAuthenticator(store, loginFn, WithClock(fakeClock(1000)))

	ctx := context.Background()

	err := a.MustBeAuthenticated(ctx, "ACC123")
	if err != nil {
		t.Errorf("MustBeAuthenticated() error = %v, want nil", err)
	}

	err = a.MustBeAuthenticated(ctx, "ACC999")
	if err == nil {
		t.Error("MustBeAuthenticated() for unknown account: expected error, got nil")
	}
	if err != errNotLoggedIn {
		t.Errorf("MustBeAuthenticated() error = %v, want %v", err, errNotLoggedIn)
	}
}

func TestAuthenticatorMustBeAuthenticatedExpired(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	store.sessions["ACC123"] = Session{AccountID: "ACC123", Token: "tok", Expiry: 500}

	loginFn := func(ctx context.Context, req LoginRequest) (string, error) {
		return "tok", nil
	}
	a := NewAuthenticator(store, loginFn, WithClock(fakeClock(1000)))

	ctx := context.Background()
	err := a.MustBeAuthenticated(ctx, "ACC123")
	if err != errTokenExpired {
		t.Errorf("MustBeAuthenticated() error = %v, want %v", err, errTokenExpired)
	}
}

func TestAuthErrorError(t *testing.T) {
	e := &AuthError{Code: "test_code", Message: "test message"}
	if got := e.Error(); got != "test_code: test message" {
		t.Errorf("Error() = %q, want %q", got, "test_code: test message")
	}
}

func TestTokenManagerLoginInProgress(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	tm := NewTokenManager(store, WithClock(fakeClock(1000)))

	accountID := domain.AccountID("ACC123")

	if tm.IsLoginInProgress(accountID) {
		t.Error("IsLoginInProgress() = true, want false initially")
	}

	tm.markLoginPending(accountID)

	if !tm.IsLoginInProgress(accountID) {
		t.Error("IsLoginInProgress() = false, want true after markLoginPending")
	}

	tm.clearLoginPending(accountID)

	if tm.IsLoginInProgress(accountID) {
		t.Error("IsLoginInProgress() = true, want false after clearLoginPending")
	}
}

func TestTokenManagerClock(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	tm := NewTokenManager(store, WithClock(fakeClock(5000)))

	now := tm.clock.Now()
	if now != 5000 {
		t.Errorf("Now() = %d, want %d", now, 5000)
	}
}

func TestSessionManagerStateTransitions(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	sm := newSessionManager(store, fakeClock(1000))

	if sm.State() != session.LoggedOut {
		t.Errorf("initial state = %v, want loggedOut", sm.State())
	}

	ok := sm.BeginLogin()
	if !ok {
		t.Error("BeginLogin = false, want true")
	}
	if sm.State() != session.LoggingIn {
		t.Errorf("state after BeginLogin = %v, want loggingIn", sm.State())
	}

	sm.LoginSucceeded()
	if sm.State() != session.LoggedIn {
		t.Errorf("state after LoginSucceeded = %v, want loggedIn", sm.State())
	}

	sm.Logout()
	if sm.State() != session.LoggedOut {
		t.Errorf("state after Logout = %v, want loggedOut", sm.State())
	}
}

func TestSessionManagerBeginLoginFailsWhenLoggedIn(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	sm := newSessionManager(store, fakeClock(1000))

	sm.BeginLogin()
	sm.LoginSucceeded()
	ok := sm.BeginLogin()
	if ok {
		t.Error("BeginLogin when already logged in = true, want false")
	}
}

func TestSessionManagerLoginFailed(t *testing.T) {
	store := &fakeStore{sessions: make(map[domain.AccountID]Session)}
	sm := newSessionManager(store, fakeClock(1000))

	sm.BeginLogin()
	sm.LoginFailed()
	if sm.State() != session.LoggedOut {
		t.Errorf("state after LoginFailed = %v, want loggedOut", sm.State())
	}
}
