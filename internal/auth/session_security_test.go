// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// mutableClock is a settable clock so token expiry can be exercised by moving
// time rather than by constructing fixed values.
type mutableClock struct {
	mu  sync.Mutex
	now int64
}

func (c *mutableClock) Now() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mutableClock) set(v int64) {
	c.mu.Lock()
	c.now = v
	c.mu.Unlock()
}

func (c *mutableClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now += int64(d.Seconds())
	c.mu.Unlock()
}

const testAccount = domain.AccountID("ACC-THREAT")

func newClockedAuth(t *testing.T, ttl time.Duration) (*Authenticator, *mutableClock) {
	t.Helper()
	clk := &mutableClock{now: 1_000}
	var issued int
	login := func(context.Context, LoginRequest) (string, error) {
		issued++
		return "token-" + string(rune('0'+issued)), nil
	}
	a := NewAuthenticator(NewInMemoryStore(), login,
		WithClock(clk), WithTokenTTL(ttl))
	return a, clk
}

// TestTokenReplayRejectedAfterRelogin asserts a second Login replaces the
// stored session, so a token captured from an earlier login no longer resolves
// to the current session.
func TestTokenReplayRejectedAfterRelogin(t *testing.T) {
	a, _ := newClockedAuth(t, 3*time.Hour)
	ctx := context.Background()

	first, err := a.Login(ctx, LoginRequest{AccountID: testAccount, TradePassword: "123456"})
	if err != nil {
		t.Fatalf("first Login: %v", err)
	}
	stale := first.Token

	second, err := a.Login(ctx, LoginRequest{AccountID: testAccount, TradePassword: "123456"})
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if second.Token == stale {
		t.Fatalf("re-login returned the same token %q; the session was not replaced", second.Token)
	}

	current, ok, err := a.GetSession(ctx, testAccount)
	if err != nil || !ok {
		t.Fatalf("GetSession ok=%v err=%v, want a session", ok, err)
	}
	if current.Token != second.Token {
		t.Fatalf("stored token = %q, want the re-login token %q", current.Token, second.Token)
	}
	if current.Token == stale {
		t.Fatal("the superseded token is still the stored session")
	}
}

// TestStaleTokenRejectedAfterLogout asserts a captured token cannot be used
// once the session is cleared.
func TestStaleTokenRejectedAfterLogout(t *testing.T) {
	a, _ := newClockedAuth(t, 3*time.Hour)
	ctx := context.Background()

	if _, err := a.Login(ctx, LoginRequest{AccountID: testAccount, TradePassword: "123456"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := a.Logout(ctx, testAccount); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, ok, _ := a.GetSession(ctx, testAccount); ok {
		t.Fatal("session still present after Logout")
	}
	if err := a.MustBeAuthenticated(ctx, testAccount); err == nil {
		t.Fatal("MustBeAuthenticated succeeded after Logout, want rejection")
	}
}

// TestClockDriftForwardExpiresToken covers the clock moving past expiry.
func TestClockDriftForwardExpiresToken(t *testing.T) {
	a, clk := newClockedAuth(t, 1*time.Hour)
	ctx := context.Background()

	if _, err := a.Login(ctx, LoginRequest{AccountID: testAccount, TradePassword: "123456"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := a.MustBeAuthenticated(ctx, testAccount); err != nil {
		t.Fatalf("MustBeAuthenticated before expiry: %v", err)
	}

	clk.advance(59 * time.Minute)
	if err := a.MustBeAuthenticated(ctx, testAccount); err != nil {
		t.Fatalf("MustBeAuthenticated one minute before expiry: %v", err)
	}

	clk.advance(2 * time.Minute) // now 61 minutes: past the 1h TTL
	if err := a.MustBeAuthenticated(ctx, testAccount); err == nil {
		t.Fatal("MustBeAuthenticated succeeded past expiry, want rejection")
	}
}

// TestClockDriftBackwardDoesNotCorruptState records the current behaviour when
// the wall clock moves backwards. A token already past its expiry becomes
// acceptable again, because IsExpired is a pure comparison against the supplied
// time and there is no monotonic component. This is a known limitation, pinned
// here so a future change to the model is a deliberate one.
func TestClockDriftBackwardDoesNotCorruptState(t *testing.T) {
	a, clk := newClockedAuth(t, 1*time.Hour)
	ctx := context.Background()

	if _, err := a.Login(ctx, LoginRequest{AccountID: testAccount, TradePassword: "123456"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	clk.advance(2 * time.Hour)
	if err := a.MustBeAuthenticated(ctx, testAccount); err == nil {
		t.Fatal("token unexpectedly valid far past expiry")
	}

	clk.set(1_000) // clock jumps back to the login instant
	if err := a.MustBeAuthenticated(ctx, testAccount); err != nil {
		t.Fatalf("MustBeAuthenticated after backward jump: %v", err)
	}
	if _, ok, _ := a.GetSession(ctx, testAccount); !ok {
		t.Fatal("session disappeared after a backward clock jump")
	}
}

func TestTokenManagerSaveComputesExpiryAndRefresh(t *testing.T) {
	clk := &mutableClock{now: 1_000}
	tm := NewTokenManager(NewInMemoryStore(), WithClock(clk), WithTokenTTL(3*time.Hour))
	ctx := context.Background()

	if err := tm.SaveToken(ctx, testAccount, "tok"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	sess, ok, err := tm.GetSession(ctx, testAccount)
	if err != nil || !ok {
		t.Fatalf("GetSession ok=%v err=%v", ok, err)
	}
	wantExpiry := int64(1_000 + (3 * time.Hour).Seconds())
	wantRefresh := int64(1_000 + (3*time.Hour - defaultRefreshWindow).Seconds())
	if sess.Expiry != wantExpiry {
		t.Errorf("Expiry = %d, want %d", sess.Expiry, wantExpiry)
	}
	if sess.RefreshAt != wantRefresh {
		t.Errorf("RefreshAt = %d, want %d", sess.RefreshAt, wantRefresh)
	}

	// The refresh window opens 10 minutes before expiry.
	clk.set(wantRefresh)
	if !sess.ShouldRefresh(clk.Now()) {
		t.Errorf("ShouldRefresh at RefreshAt = false, want true")
	}
	if sess.IsExpired(clk.Now()) {
		t.Errorf("IsExpired at RefreshAt = true, want false (still inside the TTL)")
	}
}
