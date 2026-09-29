// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package auth holds the v-next session and token lifecycle.
//
// It provides:
//
//   - EncryptTradePassword: AES-192-ECB/PKCS7 encryption of the user-supplied
//     trade password, matching the fixed key published by HStong.
//   - Session: the login token, its expiry and refresh timestamps, and the
//     account it belongs to.
//   - TokenManager: issues and clears sessions against a SessionStore, with an
//     injectable Clock so expiry can be tested without wall-clock dependence.
//   - Authenticator: encrypts the password, calls an injected login function,
//     and stores the returned token.
//
// Rules (per ADR 0001, ADR 0005):
//
//   - No RSA signing or platform key handling; the Gateway owns all signing.
//   - The SDK never stores or logs the plaintext trade password.
//
// Not yet implemented, and therefore not claimed here:
//
//   - Automatic re-login when a token expires mid-operation. Authenticator
//     detects an expired session in MustBeAuthenticated and reports
//     errTokenExpired; deciding to re-login is the caller's call, not this
//     package's.
//   - A refresh action driven by Session.ShouldRefresh. The field and the
//     method exist; nothing calls ShouldRefresh, so the refresh window is
//     advisory. Callers read RefreshAt and decide.
//
// What this package does do, and previously only claimed to:
//
//   - Concurrent logins for the same account are coalesced. The first caller
//     performs the request and the rest wait for it and read the session it
//     produced, so N callers do not become N login requests against the
//     Gateway. Waiters honor context cancellation. A login that begins after
//     the flight ends performs its own request rather than reading a session
//     that may already be expiring.
//
// Do not read the presence of an unused method as evidence of an active
// behaviour: Session.ShouldRefresh, TokenManager.IsLoginInProgress,
// markLoginPending and clearLoginPending still have no caller outside tests, and
// the coalescing above is implemented by the flight in
// TokenManager.beginLogin/endLogin, not by those four.
//
// This package is the v-next session layer. It is composed by
// pkg/services.SessionService, and nothing under pkg/hstong/* reaches it; the
// released v0.1.x session path remains independent. See docs/threat-model.md
// for which controls are active and which are forward-looking.
package auth
