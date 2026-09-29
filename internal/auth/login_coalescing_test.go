// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// flakyStore is an in-memory SessionStore whose read can be made to fail, which
// is the condition the old Login swallowed.
type flakyStore struct {
	mu       sync.Mutex
	sessions map[domain.AccountID]Session
	saveErr  error
	readErr  error
	// discardSaves makes SaveSession report success while persisting nothing,
	// which is the "save succeeded, read finds nothing" case the old Login also
	// swallowed.
	discardSaves bool
	reads        int
	saves        int
}

func newFlakyStore() *flakyStore {
	return &flakyStore{sessions: make(map[domain.AccountID]Session)}
}

func (s *flakyStore) Session(_ context.Context, id domain.AccountID) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if s.readErr != nil {
		return Session{}, false, s.readErr
	}
	sess, ok := s.sessions[id]
	return sess, ok, nil
}

func (s *flakyStore) SaveSession(_ context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	if !s.discardSaves {
		s.sessions[sess.AccountID] = sess
	}
	return nil
}

func (s *flakyStore) ClearSession(_ context.Context, id domain.AccountID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func testLoginReq(id domain.AccountID) LoginRequest {
	return LoginRequest{AccountID: id, TradePassword: "pw", EncryptionSalt: "salt"}
}

// TestLoginPropagatesThePostSaveReadError is the defect this change fixes.
//
// Login used to end with `sess, _, _ := GetSession(...)`, discarding both the
// error and the found flag. A store that saved the token and then failed the
// read produced a zero Session and a nil error, so the caller believed it had
// logged in while holding a token that was not there. The failure surfaced later
// as an opaque not_logged_in on some unrelated call.
func TestLoginPropagatesThePostSaveReadError(t *testing.T) {
	store := newFlakyStore()
	store.readErr = errors.New("store read failed")

	var calls atomic.Int64
	a := NewAuthenticator(store, func(context.Context, LoginRequest) (string, error) {
		calls.Add(1)
		return "tok", nil
	})

	sess, err := a.Login(context.Background(), testLoginReq("acct-1"))
	if err == nil {
		t.Fatalf("Login returned (Session{}, nil) with a failing store: "+
			"reads=%d saves=%d loginCalls=%d. A caller that got nil here would "+
			"believe it was authenticated holding a token that does not exist",
			store.reads, store.saves, calls.Load())
	}
	if sess.Token != "" {
		t.Errorf("Login returned a token (%q) alongside an error; want a zero Session", sess.Token)
	}
	if !strings.Contains(err.Error(), "store read failed") {
		t.Errorf("Login error = %q, want it to carry the underlying store error", err)
	}
}

// TestLoginReportsASessionThatWasNotStored covers the other half: the read
// succeeds but finds nothing, which the discarded found-flag also swallowed.
func TestLoginReportsASessionThatWasNotStored(t *testing.T) {
	store := newFlakyStore()
	store.discardSaves = true // SaveSession reports success, persists nothing
	a := NewAuthenticator(store, func(context.Context, LoginRequest) (string, error) {
		return "tok", nil
	})

	_, err := a.Login(context.Background(), testLoginReq("acct-1"))
	if err == nil {
		t.Fatal("Login returned nil error although the store holds no session")
	}
	if !errors.Is(err, errSessionNotStored) {
		t.Errorf("Login error = %v, want errSessionNotStored", err)
	}
}

// TestLoginFlightGrantsLeadershipExactlyOnce tests the mechanism directly,
// which is the part that can be asserted deterministically.
func TestLoginFlightGrantsLeadershipExactlyOnce(t *testing.T) {
	tm := NewTokenManager(newFlakyStore())
	const id domain.AccountID = "acct-1"

	done, leader := tm.beginLogin(id)
	if !leader {
		t.Fatal("the first beginLogin was not granted leadership")
	}

	waitDone, leader2 := tm.beginLogin(id)
	if leader2 {
		t.Error("a second beginLogin was granted leadership while a flight was open")
	}
	if waitDone != done {
		t.Error("the waiter was handed a different channel than the leader holds")
	}

	tm.endLogin(id)
	select {
	case <-done:
	default:
		t.Error("endLogin did not release the waiters")
	}

	// A flight must not outlive itself: a later Login must be able to lead,
	// or it would wait on a completed flight and read a stale session.
	if _, leader3 := tm.beginLogin(id); !leader3 {
		t.Error("beginLogin after endLogin was not granted leadership")
	}
	tm.endLogin(id)

	// A completed flight must also leave no state behind. Leadership alone does
	// not catch this: a stale entry is overwritten by the next flight, so the
	// only symptom is that the map grows with every account ever logged into and
	// never shrinks. That is the shape of a slow leak, so it is asserted
	// directly.
	tm.mu.Lock()
	flights, pending := len(tm.flights), len(tm.pending)
	tm.mu.Unlock()
	if flights != 0 || pending != 0 {
		t.Errorf("after all flights ended: %d flight entries and %d pending entries "+
			"remain, want 0 and 0. A completed login must not retain state", flights, pending)
	}
}

// TestConcurrentLoginsNeverOverlap is the property that matters and that is
// deterministic: while one login is in flight, no other login request may be
// started.
//
// An earlier version of this test asserted that N concurrent callers produce
// exactly one call to loginFn, and it failed. The test was wrong, not the code:
// a caller that arrives *after* the flight has closed correctly performs its own
// login, and nothing can guarantee all N callers are inside the flight before
// the leader finishes without a sleep. What coalescing does guarantee is
// exclusivity, and that is what this asserts - with an atomic high-water mark on
// concurrent in-flight requests, so no timing is involved in the verdict.
func TestConcurrentLoginsNeverOverlap(t *testing.T) {
	store := newFlakyStore()
	var calls, inflight, peak atomic.Int64
	firstEntered := make(chan struct{})
	release := make(chan struct{})

	a := NewAuthenticator(store, func(context.Context, LoginRequest) (string, error) {
		ordinal := calls.Add(1)
		cur := inflight.Add(1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		defer inflight.Add(-1)
		// Only the very first request blocks, keyed off the call ordinal rather
		// than the in-flight count: after the first login releases, a later login
		// also sees inflight == 1, and closing on that panicked with "close of
		// closed channel". The gate is closed exactly once below.
		if ordinal == 1 {
			close(firstEntered)
			<-release
		}
		return "tok", nil
	})

	const callers = 12
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Login(context.Background(), testLoginReq("acct-1")); err != nil {
				t.Errorf("coalesced Login: %v", err)
			}
		}()
	}

	<-firstEntered // a login is in flight, so the flight is open
	close(release) // release it exactly once
	wg.Wait()

	if got := peak.Load(); got > 1 {
		t.Errorf("%d login requests were in flight at once, want at most 1. "+
			"Concurrent logins are concurrent money-moving requests against the "+
			"Gateway; coalescing exists so that never happens", got)
	}
	if got := calls.Load(); got == 0 {
		t.Error("loginFn was never called")
	}
	if got := calls.Load(); got > callers {
		t.Errorf("loginFn called %d times for %d callers", got, callers)
	}
}

// TestLoginAfterAFlightEndsStartsItsOwn guards the other side of coalescing. If
// a later Login joined a completed flight it would read back a session that is
// already expiring, so a fresh login must really perform a fresh request.
func TestLoginAfterAFlightEndsStartsItsOwn(t *testing.T) {
	store := newFlakyStore()
	var calls atomic.Int64
	a := NewAuthenticator(store, func(context.Context, LoginRequest) (string, error) {
		calls.Add(1)
		return "tok", nil
	})

	for i := 0; i < 3; i++ {
		if _, err := a.Login(context.Background(), testLoginReq("acct-1")); err != nil {
			t.Fatalf("sequential Login %d: %v", i, err)
		}
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("loginFn called %d times for 3 sequential logins, want 3: "+
			"coalescing must not outlive the flight it belongs to", got)
	}
}

// TestCoalescingIsPerAccount checks the flight is keyed by account, so a
// concurrent login for a different account is not made to wait.
func TestCoalescingIsPerAccount(t *testing.T) {
	store := newFlakyStore()
	var calls atomic.Int64
	a := NewAuthenticator(store, func(context.Context, LoginRequest) (string, error) {
		calls.Add(1)
		return "tok", nil
	})

	var wg sync.WaitGroup
	for _, id := range []domain.AccountID{"acct-1", "acct-2", "acct-3"} {
		wg.Add(1)
		go func(id domain.AccountID) {
			defer wg.Done()
			if _, err := a.Login(context.Background(), testLoginReq(id)); err != nil {
				t.Errorf("Login(%s): %v", id, err)
			}
		}(id)
	}
	wg.Wait()
	if got := calls.Load(); got != 3 {
		t.Errorf("loginFn called %d times for 3 distinct accounts, want 3: "+
			"the flight is per-account and must not serialise unrelated logins", got)
	}
}

// TestWaiterHonoursContextCancellation checks a waiter is not stuck behind a
// leader that never finishes.
func TestWaiterHonoursContextCancellation(t *testing.T) {
	store := newFlakyStore()
	blocked := make(chan struct{})
	entered := make(chan struct{})

	a := NewAuthenticator(store, func(context.Context, LoginRequest) (string, error) {
		close(entered)
		<-blocked
		return "tok", nil
	})

	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		// The leader's own context is not cancelled; it is released below.
		_, _ = a.Login(context.Background(), testLoginReq("acct-1"))
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Login(ctx, testLoginReq("acct-1"))
	if err == nil {
		t.Fatal("a waiter with a cancelled context returned nil error, want ctx.Err()")
	}
	// The specific cause matters. An earlier version of this test asserted only
	// that *some* error came back, which a mutation that removed the wait
	// entirely survived: the waiter then read the store before the leader had
	// saved anything and got errSessionNotStored, which is a non-nil error and
	// so satisfied "an error was returned". Asserting the cause is what makes
	// the waiter observable.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("waiter error = %v, want context.Canceled. A waiter must report the "+
			"cancellation, not whatever it finds in the store", err)
	}

	close(blocked)
	<-leaderDone
}
