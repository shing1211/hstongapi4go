// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

type LoginRequest struct {
	AccountID      domain.AccountID
	TradePassword  string
	EncryptionSalt string
}

type LoginResult struct {
	Session Session
	Error   error
}

type Authenticator struct {
	tokenManager *TokenManager
	loginFn      func(context.Context, LoginRequest) (string, error)
}

func NewAuthenticator(store SessionStore, loginFn func(context.Context, LoginRequest) (string, error), opts ...TokenManagerOption) *Authenticator {
	return &Authenticator{
		tokenManager: NewTokenManager(store, opts...),
		loginFn:      loginFn,
	}
}

// Login authenticates req.AccountID and stores the resulting token.
//
// Concurrent calls for the same account are coalesced into a single request to
// the Gateway: the first caller performs the login and the rest wait for it and
// then read the session it produced. Login is a money-moving operation, so N
// callers each sending a login is N login attempts against the platform and N
// chances to trip its rate limits, for no benefit. Waits honor ctx.
//
// A caller that arrives after the flight has finished performs its own login;
// it does not read a session that may be expiring.
func (a *Authenticator) Login(ctx context.Context, req LoginRequest) (Session, error) {
	done, leader := a.tokenManager.beginLogin(req.AccountID)
	if !leader {
		select {
		case <-done:
		case <-ctx.Done():
			return Session{}, ctx.Err()
		}
		return a.sessionFromStore(ctx, req.AccountID)
	}
	defer a.tokenManager.endLogin(req.AccountID)

	encrypted, err := EncryptTradePassword(req.TradePassword)
	if err != nil {
		return Session{}, err
	}
	token, err := a.loginFn(ctx, LoginRequest{
		AccountID:      req.AccountID,
		TradePassword:  encrypted,
		EncryptionSalt: req.EncryptionSalt,
	})
	if err != nil {
		return Session{}, err
	}
	if err := a.tokenManager.SaveToken(ctx, req.AccountID, token); err != nil {
		return Session{}, err
	}
	return a.sessionFromStore(ctx, req.AccountID)
}

// sessionFromStore reads back the session that was just written.
//
// The error is returned rather than discarded. It previously was not, which
// meant a store that failed the read after a successful save produced a zero
// Session and a nil error: the caller was told it had logged in, holding a token
// that did not exist, and the failure surfaced later as an opaque
// not_logged_in on an unrelated call.
func (a *Authenticator) sessionFromStore(ctx context.Context, accountID domain.AccountID) (Session, error) {
	sess, ok, err := a.tokenManager.GetSession(ctx, accountID)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, errSessionNotStored
	}
	return sess, nil
}

var errSessionNotStored = &AuthError{
	Code:    "session_not_stored",
	Message: "login succeeded but the session could not be read back from the store",
}

func (a *Authenticator) Logout(ctx context.Context, accountID domain.AccountID) error {
	return a.tokenManager.ClearToken(ctx, accountID)
}

func (a *Authenticator) GetSession(ctx context.Context, accountID domain.AccountID) (Session, bool, error) {
	return a.tokenManager.GetSession(ctx, accountID)
}

func (a *Authenticator) MustBeAuthenticated(ctx context.Context, accountID domain.AccountID) error {
	sess, ok, err := a.tokenManager.GetSession(ctx, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return errNotLoggedIn
	}
	if sess.IsExpired(a.tokenManager.clock.Now()) {
		return errTokenExpired
	}
	return nil
}

var errNotLoggedIn = &AuthError{Code: "not_logged_in", Message: "user is not logged in"}
var errTokenExpired = &AuthError{Code: "token_expired", Message: "session token has expired"}

type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string { return e.Code + ": " + e.Message }
