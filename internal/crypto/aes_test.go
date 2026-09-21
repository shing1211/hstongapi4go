// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestKeyIsAES192(t *testing.T) {
	key, err := base64.StdEncoding.DecodeString(tradePasswordKeyBase64)
	if err != nil {
		t.Fatalf("decoding protocol key: %v", err)
	}
	if len(key) != 24 {
		t.Fatalf("protocol key length = %d, want 24 (AES-192)", len(key))
	}
}

func TestEncryptTradePassword(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
		want      string
		wantLen   int
	}{
		{
			name:      "doc vector",
			plaintext: "123456",
			want:      "W1U8iZIppSE+mBMtzy9vZQ==",
			wantLen:   16,
		},
		{
			name:      "empty string pads to full block",
			plaintext: "",
			want:      "oTOStZkbw47Ck3kJ3D9RwA==",
			wantLen:   16,
		},
		{
			name:      "exactly one block pads an extra block",
			plaintext: "1234567890abcdef",
			want:      "l6h2WJu7rtha3fVr81Y9uKEzkrWZG8OOwpN5Cdw/UcA=",
			wantLen:   32,
		},
		{
			name:      "multi block",
			plaintext: "correct horse battery staple",
			want:      "+rMq6MQR8CakUXiC5VS1pSbnjIRw5has0gKtU13fibU=",
			wantLen:   32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncryptTradePassword(tt.plaintext)
			if err != nil {
				t.Fatalf("EncryptTradePassword(%q) error = %v", tt.plaintext, err)
			}
			if got != tt.want {
				t.Fatalf("EncryptTradePassword(%q) = %q, want %q", tt.plaintext, got, tt.want)
			}

			decoded, err := base64.StdEncoding.DecodeString(got)
			if err != nil {
				t.Fatalf("output %q is not valid standard Base64: %v", got, err)
			}
			if len(decoded) != tt.wantLen {
				t.Fatalf("ciphertext length = %d, want %d", len(decoded), tt.wantLen)
			}
			if len(decoded)%16 != 0 {
				t.Fatalf("ciphertext length %d is not a multiple of the AES block size", len(decoded))
			}
			if strings.Contains(got, tt.plaintext) && tt.plaintext != "" {
				t.Fatalf("output %q leaks plaintext %q", got, tt.plaintext)
			}
		})
	}
}

func TestEncryptTradePassword_Deterministic(t *testing.T) {
	const plaintext = "s3cret-passphrase"
	first, err := EncryptTradePassword(plaintext)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := EncryptTradePassword(plaintext)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Fatalf("ECB output not deterministic: %q != %q", first, second)
	}
}

func TestEncryptTradePassword_NoPlaintextInError(t *testing.T) {
	const secret = "hunter2-do-not-log"

	for _, keyLen := range []int{0, 1, 16 - 1, 24 + 1, 32 + 1} {
		key := make([]byte, keyLen)
		_, err := encryptECB(key, []byte(secret))
		if !errors.Is(err, ErrInvalidKeyLength) {
			t.Fatalf("encryptECB with %d-byte key error = %v, want ErrInvalidKeyLength", keyLen, err)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error %q leaks the plaintext", err)
		}
	}
}
