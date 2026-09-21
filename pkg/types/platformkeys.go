// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package types

// The platform RSA public keys below are bundled so that opt-in push-frame
// signature verification (SHA1WithRSA over the raw body bytes) works out of
// the box. They are base64-encoded SubjectPublicKeyInfo (SPKI) DER.
//
// These keys are *public reference data, not secrets*. They are copied
// verbatim from the published HStong documentation and are safe to commit. A
// caller may override them (for example after rotation) via the client's
// platform-key option. See ADR 0005.
const (
	// PlatformPublicKeyTest is the platform RSA public key for the test
	// environment (https://openapi-daily.hstong.com).
	PlatformPublicKeyTest = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCbRuA8hsbbzBKePEZZWaVtYpOjq2XaLZgAeVDlYqgy4lt4D+H2h+47AxVhYmS24O5lGuYD34ENlMoJphLrZkPbVBWJVHJZcRkpC0y36LFdFw7BSEA5+5+kdPFe8gR+wwXQ7sj9usESulRQcqrl38LoIz/vYUbYKsSe3dADfEgMKQIDAQAB"

	// PlatformPublicKeyProd is the platform RSA public key for the production
	// environment (https://openapi.hstong.com).
	PlatformPublicKeyProd = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDu7xSKk8VNr7WVsxIbltmpe4ViEVNP9QyjRvA2IBm7KCuE6FFyFABSubjhxeZ3joDuNlga0NVtd/qfPf2iursrSOmT00j2JWcR9pQ/mZ61aWEW8BNnkFvu0nijrw0A2z1XhyoaG1p4/pNWKiqcwpA1P23vRoDkfretUF/SlLtQGQIDAQAB"
)
