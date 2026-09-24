// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"sync"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

const defaultRetryDelay = 500 * time.Millisecond
const defaultMaxRetryDelay = 30 * time.Second

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
	mu           sync.Mutex
}

func NewAuthenticator(store SessionStore, loginFn func(context.Context, LoginRequest) (string, error), opts ...TokenManagerOption) *Authenticator {
	return &Authenticator{
		tokenManager: NewTokenManager(store, opts...),
		loginFn:      loginFn,
	}
}

func (a *Authenticator) Login(ctx context.Context, req LoginRequest) (Session, error) {
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
	sess, _, _ := a.tokenManager.GetSession(ctx, req.AccountID)
	return sess, nil
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
