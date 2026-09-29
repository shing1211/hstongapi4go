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
//   - Acting on the refresh window from inside this package. ShouldRefresh is
//     called by pkg/services, so the window is no longer advisory, but it is
//     pkg/services that decides to re-login on it - this package supplies the
//     predicate and does not schedule anything. A direct caller of this package
//     alone still reads RefreshAt and decides for itself.
//
// What this package does do, and previously only claimed to:
//
//   - Concurrent logins for the same account can be coalesced. Authenticator.Login
//     takes a per-account flight, so the first caller performs the request and
//     the rest wait for it and read the session it produced, rather than N
//     callers becoming N login requests against the Gateway. Waiters honor
//     context cancellation, and a login that begins after the flight ends
//     performs its own request rather than reading a session that may already be
//     expiring.
//
// # Who owns coalescing in production
//
// The above is capability, not reachability, and the two differ here. Production
// traffic is coalesced by SessionService.loginGate, which holds the gate for the
// whole Authenticator.Login call, so a caller joining a login in flight never
// reaches this package at all and the flight below only ever sees the single
// leader. NewSessionService builds one Authenticator, and no production path
// constructs one directly.
//
// The flight is kept as defence in depth for a direct caller that does not exist
// yet. It is unexported and un-composed, so it cannot become a second source of
// truth about who coalesces a login; pkg/services is the owner and asserts the
// properties in its own tests. See pkg/services/session.go and the R14 entry in
// docs/threat-model.md.
//
// Do not read the presence of a method as evidence of an active behaviour.
// TokenManager.IsLoginInProgress, markLoginPending and clearLoginPending still
// have no caller outside tests, and Session.ShouldRefresh has exactly one -
// pkg/services. The coalescing above is implemented by the flight in
// TokenManager.beginLogin/endLogin, not by those three.
//
// This package is the v-next session layer. It is composed by
// pkg/services.SessionService, and nothing under pkg/hstong/* reaches it; the
// released v0.1.x session path remains independent. See docs/threat-model.md
// for which controls are active and which are forward-looking.
package auth
