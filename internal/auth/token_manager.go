// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"sync"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

const defaultTokenTTL = 3 * time.Hour
const defaultRefreshWindow = 10 * time.Minute

type Clock interface {
	Now() int64
}

type realClock struct{}

func (realClock) Now() int64 { return time.Now().Unix() }

func RealClock() Clock { return realClock{} }

type TokenManager struct {
	store         SessionStore
	clock         Clock
	tokenTTL      time.Duration
	refreshWindow time.Duration

	mu      sync.Mutex
	pending map[domain.AccountID]bool
	// flights holds the completion signal for each in-flight login, so a caller
	// that arrives while a login is running can wait for its result instead of
	// starting a second one. An entry exists only for the duration of the
	// flight, and is removed when it ends, so a later Login starts its own.
	flights map[domain.AccountID]chan struct{}
}

func NewTokenManager(store SessionStore, opts ...TokenManagerOption) *TokenManager {
	tm := &TokenManager{
		store:         store,
		clock:         RealClock(),
		tokenTTL:      defaultTokenTTL,
		refreshWindow: defaultRefreshWindow,
		pending:       make(map[domain.AccountID]bool),
		flights:       make(map[domain.AccountID]chan struct{}),
	}
	for _, o := range opts {
		o(tm)
	}
	return tm
}

type TokenManagerOption func(*TokenManager)

func WithClock(c Clock) TokenManagerOption {
	return func(tm *TokenManager) { tm.clock = c }
}

func WithTokenTTL(ttl time.Duration) TokenManagerOption {
	return func(tm *TokenManager) { tm.tokenTTL = ttl }
}

func (tm *TokenManager) GetSession(ctx context.Context, accountID domain.AccountID) (Session, bool, error) {
	return tm.store.Session(ctx, accountID)
}

func (tm *TokenManager) SaveToken(ctx context.Context, accountID domain.AccountID, token string) error {
	now := tm.clock.Now()
	sess := Session{
		Token:     domain.SessionToken(token),
		AccountID: accountID,
		Expiry:    now + int64(tm.tokenTTL.Seconds()),
		RefreshAt: now + int64((tm.tokenTTL - tm.refreshWindow).Seconds()),
	}
	return tm.store.SaveSession(ctx, sess)
}

func (tm *TokenManager) ClearToken(ctx context.Context, accountID domain.AccountID) error {
	return tm.store.ClearSession(ctx, accountID)
}

func (tm *TokenManager) IsLoginInProgress(accountID domain.AccountID) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.pending[accountID]
}

func (tm *TokenManager) markLoginPending(accountID domain.AccountID) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.pending[accountID] = true
}

func (tm *TokenManager) clearLoginPending(accountID domain.AccountID) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.pending, accountID)
}

// beginLogin claims the login flight for accountID.
//
// It returns (done, leader). When leader is true the caller owns the flight and
// must call endLogin exactly once. When leader is false a flight is already
// running and done is closed when that flight ends, at which point the caller
// should re-read the session rather than logging in again.
//
// The two are decided under one lock so that exactly one caller can ever become
// the leader for a given account.
func (tm *TokenManager) beginLogin(accountID domain.AccountID) (done <-chan struct{}, leader bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.pending[accountID] {
		return tm.flights[accountID], false
	}
	tm.pending[accountID] = true
	ch := make(chan struct{})
	tm.flights[accountID] = ch
	return ch, true
}

// endLogin releases the flight and wakes every waiter.
//
// The entry is deleted before the channel is closed, so a Login that begins
// after this point starts a fresh flight rather than waiting on a completed one
// and returning a stale session. Closing under the lock is what makes that
// ordering safe: a waiter that has already taken the channel is guaranteed to
// observe the close.
func (tm *TokenManager) endLogin(accountID domain.AccountID) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	ch := tm.flights[accountID]
	delete(tm.flights, accountID)
	delete(tm.pending, accountID)
	if ch != nil {
		close(ch)
	}
}
