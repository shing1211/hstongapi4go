// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
)

// TestManager_SubscribeOrders asserts the canonical path, POST, numeric
// timeout_sec, and the exact empty params of /trade/TradeSubscribe.
func TestManager_SubscribeOrders(t *testing.T) {
	path := string(client.RouteTradeSubscribe)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"success":true}}`,
	})

	if err := m.SubscribeOrders(context.Background()); err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}
	assertCall(t, rec, path, `{}`)
}

// TestManager_UnsubscribeOrders asserts the canonical path, POST, numeric
// timeout_sec, and the exact empty params of /trade/TradeUnsubscribe.
func TestManager_UnsubscribeOrders(t *testing.T) {
	path := string(client.RouteTradeUnsubscribe)
	rec, m := newManager(t, map[string]string{
		path: `{"ok":true,"err":"","data":{"success":true}}`,
	})

	if err := m.UnsubscribeOrders(context.Background()); err != nil {
		t.Fatalf("UnsubscribeOrders: %v", err)
	}
	assertCall(t, rec, path, `{}`)
}

// fakeSession counts EnsureLoggedIn calls and declines every re-login so the
// Manager's session handling is observable.
type fakeSession struct {
	ensured int
}

// EnsureLoggedIn records one consultation and reports the session as live.
func (f *fakeSession) EnsureLoggedIn(context.Context) error { f.ensured++; return nil }

// ReLogin reports that it handled nothing.
func (f *fakeSession) ReLogin(context.Context, error) (bool, error) { return false, nil }

// TestManager_SubscribeOrdersEnsuresSession asserts that a trade session is
// consulted before the subscribe request, reflecting the documented requirement
// that order push needs a logged-in trading session.
func TestManager_SubscribeOrdersEnsuresSession(t *testing.T) {
	path := string(client.RouteTradeSubscribe)
	rec := &recorder{responses: map[string]string{
		path: `{"ok":true,"err":"","data":{"success":true}}`,
	}}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	sess := &fakeSession{}
	m := New(c, WithSession(sess))
	if err := m.SubscribeOrders(context.Background()); err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}
	if sess.ensured != 1 {
		t.Fatalf("EnsureLoggedIn calls = %d, want 1", sess.ensured)
	}
	assertCall(t, rec, path, `{}`)
}
