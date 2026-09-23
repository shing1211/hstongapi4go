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
}

func NewTokenManager(store SessionStore, opts ...TokenManagerOption) *TokenManager {
	tm := &TokenManager{
		store:         store,
		clock:         RealClock(),
		tokenTTL:      defaultTokenTTL,
		refreshWindow: defaultRefreshWindow,
		pending:       make(map[domain.AccountID]bool),
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

type fixedClock int64

func (fixedClock) Now() int64 { return 0 }
