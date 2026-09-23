// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

func TestEncryptTradePassword(t *testing.T) {
	got, err := EncryptTradePassword("123456")
	if err != nil {
		t.Fatalf("EncryptTradePassword(\"123456\") error = %v", err)
	}
	want := "W1U8iZIppSE+mBMtzy9vZQ=="
	if got != want {
		t.Errorf("EncryptTradePassword(\"123456\") = %q, want %q", got, want)
	}
}

func TestSessionIsExpired(t *testing.T) {
	tests := []struct {
		name    string
		expiry  int64
		now     int64
		expired bool
	}{
		{"not expired", 2000, 1000, false},
		{"exactly expired", 1000, 1000, true},
		{"past expiry", 999, 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := Session{Expiry: tt.expiry}
			if got := sess.IsExpired(tt.now); got != tt.expired {
				t.Errorf("IsExpired(%d) = %v, want %v", tt.now, got, tt.expired)
			}
		})
	}
}

func TestSessionShouldRefresh(t *testing.T) {
	sess := Session{RefreshAt: 1000}
	if sess.ShouldRefresh(999) {
		t.Errorf("ShouldRefresh(999) = true, want false")
	}
	if !sess.ShouldRefresh(1000) {
		t.Errorf("ShouldRefresh(1000) = false, want true")
	}
}

type fakeClock int64

func (c fakeClock) Now() int64 { return int64(c) }

func TestTokenManagerSaveGet(t *testing.T) {
	store := NewInMemoryStore()
	tm := NewTokenManager(store, WithClock(fakeClock(1000)), WithTokenTTL(3*time.Hour))

	ctx := context.Background()
	accountID := domain.AccountID("ACC123")

	err := tm.SaveToken(ctx, accountID, "token-abc")
	if err != nil {
		t.Fatalf("SaveToken error = %v", err)
	}

	sess, ok, err := tm.GetSession(ctx, accountID)
	if err != nil {
		t.Fatalf("GetSession error = %v", err)
	}
	if !ok {
		t.Fatal("GetSession: not found")
	}
	if sess.Token != "token-abc" {
		t.Errorf("Token = %q, want %q", sess.Token, "token-abc")
	}
	if sess.AccountID != accountID {
		t.Errorf("AccountID = %q, want %q", sess.AccountID, accountID)
	}
}

func TestTokenManagerClear(t *testing.T) {
	store := NewInMemoryStore()
	tm := NewTokenManager(store, WithClock(fakeClock(1000)))

	ctx := context.Background()
	accountID := domain.AccountID("ACC123")

	_ = tm.SaveToken(ctx, accountID, "token-xyz")
	_, ok, _ := tm.GetSession(ctx, accountID)
	if !ok {
		t.Fatal("expected session before clear")
	}

	_ = tm.ClearToken(ctx, accountID)
	_, ok, _ = tm.GetSession(ctx, accountID)
	if ok {
		t.Error("expected no session after clear")
	}
}

func TestInMemoryStoreConcurrency(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	accountID := domain.AccountID("ACC999")

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id domain.AccountID) {
			_ = store.SaveSession(ctx, Session{AccountID: id, Token: "tok"})
			_, _, _ = store.Session(ctx, id)
			done <- true
		}(accountID)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
