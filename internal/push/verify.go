// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- the Gateway signs bodySHA1 with SHA1WithRSA; the algorithm is fixed by the wire protocol and verification is opt-in (ADR 0005)
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

// Sentinel errors returned by the signature verifier. They are wrapped with
// detail, so callers test with errors.Is.
var (
	// ErrNoPublicKey reports that verification was requested without a usable
	// public key.
	ErrNoPublicKey = errors.New("push: no platform public key configured")
	// ErrMissingSignature reports a frame whose 128-byte bodySHA1 field is all
	// zero, i.e. the sender did not sign it.
	ErrMissingSignature = errors.New("push: frame has no bodySHA1 signature")
	// ErrSignatureMismatch reports that the bodySHA1 signature did not verify
	// against the raw body and the configured key.
	ErrSignatureMismatch = errors.New("push: bodySHA1 signature verification failed")
	// ErrUnsupportedKey reports an encoded public key that is not a PEM
	// PUBLIC KEY block, a base64-encoded SPKI DER, or raw SPKI DER.
	ErrUnsupportedKey = errors.New("push: unsupported public key encoding")
)

// Verifier checks the SHA1WithRSA signature carried in a push frame's 128-byte
// bodySHA1 header field against the raw (uncompressed) body.
//
// Verification is opt-in and off by default (docs/adr/0005-key-model-and-push-
// verification.md). A Verifier is immutable after construction and safe for
// concurrent use.
type Verifier struct {
	key      *rsa.PublicKey
	required bool
}

// NewVerifier returns a Verifier over key. When required is true a frame with a
// missing or invalid signature must be dropped by the caller; when false the
// caller may still report the error but deliver the frame. A nil key makes
// every Verify call return ErrNoPublicKey.
func NewVerifier(key *rsa.PublicKey, required bool) *Verifier {
	return &Verifier{key: key, required: required}
}

// Required reports whether a failed verification must reject the frame.
func (v *Verifier) Required() bool {
	return v != nil && v.required
}

// Verify checks h.BodySHA1 over body. It returns nil on success,
// ErrMissingSignature when the signature is all zero, ErrNoPublicKey when no
// key was configured, and an error wrapping ErrSignatureMismatch when the
// signature does not verify.
func (v *Verifier) Verify(h Header, body []byte) error {
	if v == nil || v.key == nil {
		return ErrNoPublicKey
	}
	var zero [BodySHA1Len]byte
	if h.BodySHA1 == zero {
		return ErrMissingSignature
	}
	sum := sha1.Sum(body) // #nosec G401 -- mandated by the Gateway's SHA1WithRSA push signature; not a security choice by the SDK
	if err := rsa.VerifyPKCS1v15(v.key, crypto.SHA1, sum[:], h.BodySHA1[:]); err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureMismatch, err)
	}
	return nil
}

// ParsePublicKey decodes a platform RSA public key from any of the forms the
// SDK accepts:
//
//   - a PEM block, "PUBLIC KEY" (PKIX/SPKI) or "RSA PUBLIC KEY" (PKCS#1);
//   - a base64-encoded SPKI DER string, as stored in the bundled
//     pkg/types.PlatformPublicKeyTest and PlatformPublicKeyProd constants;
//   - raw SPKI DER bytes.
//
// It returns an error wrapping ErrUnsupportedKey when none of these yield an
// RSA key, and never returns a nil key with a nil error.
func ParsePublicKey(encoded []byte) (*rsa.PublicKey, error) {
	data := bytes.TrimSpace(encoded)
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrUnsupportedKey)
	}
	if block, _ := pem.Decode(data); block != nil {
		return parseDERPublicKey(block.Bytes)
	}
	if der, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		if key, err := parseDERPublicKey(der); err == nil {
			return key, nil
		}
	}
	return parseDERPublicKey(data)
}

// parseDERPublicKey parses PKIX (SPKI) DER, falling back to PKCS#1 DER, and
// requires an RSA key.
func parseDERPublicKey(der []byte) (*rsa.PublicKey, error) {
	if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
		rsaKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: not an RSA public key", ErrUnsupportedKey)
		}
		return rsaKey, nil
	}
	if rsaKey, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return rsaKey, nil
	}
	return nil, fmt.Errorf("%w: not a PEM PUBLIC KEY block, base64 SPKI, or SPKI DER", ErrUnsupportedKey)
}

// VerificationEnabled reports whether a verifier was configured.
func (c *Client) VerificationEnabled() bool {
	return c != nil && c.verifier != nil
}

// VerificationRequired reports whether verification failures must reject a
// frame. It is false when verification is disabled.
func (c *Client) VerificationRequired() bool {
	return c != nil && c.verifier.Required()
}

// verifyFrame applies the configured verifier to one push frame. It returns nil
// when verification is disabled or the signature verifies. A nil verifier
// never rejects a frame.
func (c *Client) verifyFrame(h Header, body []byte) error {
	if c.verifier == nil {
		return nil
	}
	return c.verifier.Verify(h, body)
}
