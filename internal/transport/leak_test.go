// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies that the transport package leaves no goroutines behind once
// every test, including the httptest servers, has run.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
