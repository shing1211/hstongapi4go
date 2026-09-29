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
//
// # Relationship to pkg/services
//
// These managers remain the supported default and are not scheduled for
// removal. The v-next layer in pkg/services covers the same endpoints; see
// docs/MIGRATION.md for a side-by-side migration, and
// docs/adr/0011-v01x-compatibility.md for the compatibility guarantee.
// Migration is opt-in — services.NewStack is a separate constructor, and nothing
// about the default changes when you upgrade.
//
// pkg/services reached feature parity on 2026-09-26, which is the condition
// ADR 0011 sets for announcing a deprecation. Each sub-package carries a notice
// saying so, and the machine-readable // Deprecated: marker landed with v1.0.0 on
// each sub-package's Manager type.
//
// The marker is quieter than a deprecation usually is: staticcheck's SA1019
// fires only where a deprecated identifier is written out, so `market.New(c)` -
// what most callers write - produces no warning at all. Nothing here is
// scheduled for removal. This surface still works, is still the default, and the
// marker is advisory; the machine-readable form was deferred to v1.0.0
// specifically so that a v0.1.x patch release would not make every existing
// consumer's build emit warnings they did not ask for.
package hstong
