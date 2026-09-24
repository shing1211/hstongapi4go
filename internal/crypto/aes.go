// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"crypto/aes"
	"encoding/base64"
	"errors"
)

// tradePasswordKeyBase64 is the fixed AES key published in the official
// HStong Gateway trade/TradeLogin documentation. It is public reference data
// from the protocol specification, not a secret, and it decodes to a 24-byte
// AES-192 key.
const tradePasswordKeyBase64 = "m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY" // #nosec G101 -- published by HStong as protocol reference data, not a secret (ADR 0005)

// ErrInvalidKeyLength is returned when the trade-password key is not a valid
// AES key length (16, 24, or 32 bytes). The error never includes key material
// or the plaintext.
var ErrInvalidKeyLength = errors.New("crypto: invalid AES key length")

// EncryptTradePassword obfuscates a trade password for the Gateway's
// TradeLogin request and returns the standard, padded Base64 encoding of its
// AES-ECB/PKCS7 ciphertext.
//
// The transformation is deterministic: the same plaintext always yields the
// same output because ECB has no initialization vector. It returns an error
// only if the fixed protocol key fails to decode; the plaintext is never
// included in any error.
func EncryptTradePassword(plaintext string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(tradePasswordKeyBase64)
	if err != nil {
		return "", err
	}
	ciphertext, err := encryptECB(key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// encryptECB applies AES-ECB encryption to plaintext using key, padding the
// plaintext to a whole block with PKCS7 first. ECB is not provided as a mode
// by crypto/cipher, so each block is encrypted individually. It validates the
// key length and returns ErrInvalidKeyLength when it is not 16, 24, or 32
// bytes; the returned error never contains the plaintext.
func encryptECB(key, plaintext []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, ErrInvalidKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeyLength
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	out := make([]byte, len(padded))
	for start := 0; start < len(padded); start += block.BlockSize() {
		end := start + block.BlockSize()
		block.Encrypt(out[start:end], padded[start:end])
	}
	return out, nil
}

// pkcs7Pad appends PKCS7 padding to data so its length is a multiple of
// blockSize. When data is already a multiple of blockSize a full extra block
// is added, as required by PKCS7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}
