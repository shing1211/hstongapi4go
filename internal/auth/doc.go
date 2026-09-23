// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package auth handles session and token lifecycle management for the v-next SDK.
//
// It provides:
//
//   - TradePassword: AES-ECB encryption of the user-supplied trade password
//     (PKCS7 padding, no IV — matching the legacy platform protocol).
//   - Session: encapsulates the login token, refresh timestamp, expiry, and
//     account number returned by the Gateway.
//   - TokenManager: issues new sessions, refreshes expiring sessions, and
//     enforces a single concurrent login per AccountID.
//   - injectable Clock so that token expiry can be tested without wall-clock
//     dependence.
//
// Rules (per ADR 0001, ADR 0005):
//
//   - No RSA signing or platform key handling; the Gateway owns all signing.
//   - The SDK never stores or logs the plaintext trade password.
//   - Re-login is automatic when the token expires during an active operation,
//     with a backoff to prevent thundering-herd on shared token expiry.
//   - This package is the only place in the v-next layer that uses
//     decimal.Decimal at the wire bridge (for fee/money calculations returned
//     by the login response); all other wire-to-domain conversion uses string.
package auth
