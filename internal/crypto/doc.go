// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package crypto implements the HStong Gateway's trade-password obfuscation.
//
// The official Gateway protocol requires the trade password sent to
// TradeLogin to be encrypted with AES in ECB mode using a fixed protocol key
// and PKCS7 padding, then Base64-encoded. This package provides exactly that
// transformation for the session manager.
//
// ECB is mandated by the wire protocol, not recommended by this package: it
// encrypts every 16-byte block independently, so equal plaintext blocks
// produce equal ciphertext blocks and it offers no semantic security. It must
// not be used as a general-purpose encryption scheme. Here it only obscures
// the password on the loopback hop between the SDK and the locally running
// Gateway.
//
// The protocol key is a fixed Base64 value taken verbatim from the published
// HStong documentation. It is public reference data, not a secret, and the
// same value is used by every SDK client.
package crypto
