// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"sync"

	"github.com/shing1211/hstongapi4go/internal/crypto"
	"github.com/shing1211/hstongapi4go/internal/session"
	"github.com/shing1211/hstongapi4go/pkg/domain"
)

type Session struct {
	Token     domain.SessionToken
	AccountID domain.AccountID
	Expiry    int64
	RefreshAt int64
}

func (s Session) IsExpired(now int64) bool {
	return now >= s.Expiry
}

func (s Session) ShouldRefresh(now int64) bool {
	return now >= s.RefreshAt
}

type SessionStore interface {
	Session(ctx context.Context, accountID domain.AccountID) (Session, bool, error)
	SaveSession(ctx context.Context, sess Session) error
	ClearSession(ctx context.Context, accountID domain.AccountID) error
}

type inMemoryStore struct {
	mu       sync.Mutex
	sessions map[domain.AccountID]Session
}

func NewInMemoryStore() *inMemoryStore {
	return &inMemoryStore{sessions: make(map[domain.AccountID]Session)}
}

func (s *inMemoryStore) Session(ctx context.Context, accountID domain.AccountID) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[accountID]; ok {
		return sess, true, nil
	}
	return Session{}, false, nil
}

func (s *inMemoryStore) SaveSession(ctx context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.AccountID] = sess
	return nil
}

func (s *inMemoryStore) ClearSession(ctx context.Context, accountID domain.AccountID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, accountID)
	return nil
}

type sessionManager struct {
	store   SessionStore
	clock   Clock
	machine *session.Machine
}

func newSessionManager(store SessionStore, clock Clock) *sessionManager {
	return &sessionManager{
		store:   store,
		clock:   clock,
		machine: new(session.Machine),
	}
}

func (sm *sessionManager) State() session.State {
	return sm.machine.State()
}

func (sm *sessionManager) BeginLogin() bool {
	return sm.machine.BeginLogin()
}

func (sm *sessionManager) LoginSucceeded() {
	sm.machine.LoginSucceeded()
}

func (sm *sessionManager) LoginFailed() {
	sm.machine.LoginFailed()
}

func (sm *sessionManager) Logout() {
	sm.machine.Logout()
}

func EncryptTradePassword(plaintext string) (string, error) {
	return crypto.EncryptTradePassword(plaintext)
}
