// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package hstong provides the public, typed managers for the HStong (华盛)
// Quant OpenAPI Gateway. A manager owns one surface of the Gateway — session,
// market data, trading, futures, or algo — and holds a shared *client.Client
// that performs the HTTP calls. Managers are the recommended way for
// applications to use the SDK; the lower-level client package remains available
// for endpoints a manager does not yet cover.
//
// # Session
//
// Trading requires an explicit login before any trade, futures, or algo call.
// SessionManager drives the trade session against the local Gateway:
//
//	sm := hstong.NewSessionManager(c)
//	if err := sm.Login(ctx); err != nil {
//		return err
//	}
//	defer sm.Logout(ctx)
//
//	// Before any authenticated call, or after an error that
//	// errs.ReLoginRequired reports, ensure the session is live:
//	handled, err := sm.ReLogin(ctx, callErr)
//
// Login encrypts the configured trade password with the protocol's
// AES-ECB/PKCS7 transformation before it leaves the process; the plaintext is
// never logged and never appears in an error. EnsureLoggedIn coalesces
// concurrent callers into a single login request, and StartKeepAlive polls cheaply
// on an interval to extend the three-hour token.
//
// Managers are safe for concurrent use.
package hstong
