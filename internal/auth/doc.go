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
//   - Automatic re-login when a token expires mid-operation.
//   - Re-login backoff. The public pkg/hstong.SessionManager has a
//     single-flight EnsureLoggedIn; Authenticator has no equivalent yet.
//   - A refresh action driven by Session.ShouldRefresh. Callers read RefreshAt
//     and decide.
//
// The primitives for the last two exist but nothing composes them:
// Session.ShouldRefresh, TokenManager.IsLoginInProgress, markLoginPending, and
// clearLoginPending have no caller outside tests. Concurrent logins are
// therefore not coalesced, and the refresh window is never acted on.
// Authenticator does check Session.IsExpired, so it re-logs-in on an expired
// token; the single-flight wrapper and the refresh trigger are what is missing.
// Do not read the presence of those methods as evidence that either behaviour
// is active.
//
// This package is not yet wired into the released client. See
// docs/threat-model.md for which controls are active and which are
// forward-looking.
package auth
