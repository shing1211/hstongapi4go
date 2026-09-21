// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package errs

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies that the errs package leaves no goroutines behind. The
// package spawns none, so this is cheap insurance that keeps the whole core
// honest as later phases add behavior here.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
