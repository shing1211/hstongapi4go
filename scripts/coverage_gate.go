// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	pkgs := []struct {
		path string
		name string
	}{
		{"./pkg/domain/...", "github.com/shing1211/hstongapi4go/pkg/domain"},
		{"./internal/auth/...", "github.com/shing1211/hstongapi4go/internal/auth"},
		{"./internal/transport/...", "github.com/shing1211/hstongapi4go/internal/transport"},
		{"./pkg/hstong", "github.com/shing1211/hstongapi4go/pkg/hstong"},
		{"./pkg/hstong/stream", "github.com/shing1211/hstongapi4go/pkg/hstong/stream"},
		{"./pkg/hstong/trade", "github.com/shing1211/hstongapi4go/pkg/hstong/trade"},
		{"./pkg/hstong/algo", "github.com/shing1211/hstongapi4go/pkg/hstong/algo"},
		{"./pkg/types", "github.com/shing1211/hstongapi4go/pkg/types"},
		{"./pkg/transport", "github.com/shing1211/hstongapi4go/pkg/transport"},
		{"./internal/push", "github.com/shing1211/hstongapi4go/internal/push"},
	}
	gate := 85.0
	failed := false

	for _, pkg := range pkgs {
		cmd := exec.Command("go", "test", "-coverprofile=coverage_tmp.out", "-covermode=atomic", "-cover", pkg.path) // #nosec G204 -- fixed argv from a hardcoded package list in a developer script; no external input
		cmd.Dir = "."
		out, runErr := cmd.CombinedOutput()
		output := string(out)
		_ = os.Remove("coverage_tmp.out")

		// A failed `go test` produces no "coverage:" line, so comparing anyway
		// reported the package as "0.0%" — indistinguishable from a genuine
		// coverage drop, and with the actual failure discarded. A flaky test on
		// a loaded runner therefore looked like a coverage regression. Report the
		// two conditions separately and show the tail so the cause is visible.
		if runErr != nil {
			failed = true
			fmt.Printf("ERROR: %s: go test failed: %v\n", pkg.name, runErr)
			if tail := tailLines(output, 25); tail != "" {
				fmt.Printf("%s\n", tail)
			}
			continue
		}

		pct := extractCoverage(output)
		if pct < gate {
			fmt.Printf("FAIL: %s %.1f%% (below %.1f%%)\n", pkg.name, pct, gate)
			failed = true
			continue
		}
		fmt.Printf("PASS: %s %.1f%%\n", pkg.name, pct)
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("All packages meet the 85% coverage gate")
}

// tailLines returns the last n lines of s, or the whole string when it is
// shorter. Used to surface the end of a failed test run, where the useful
// detail is in the final lines.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func extractCoverage(output string) float64 {
	idx := strings.Index(output, "coverage: ")
	if idx < 0 {
		return 0
	}
	rest := output[idx+len("coverage: "):]
	end := strings.Index(rest, "%")
	if end < 0 {
		return 0
	}
	var pct float64
	if _, err := fmt.Sscanf(rest[:end], "%f", &pct); err != nil {
		return 0
	}
	return pct
}
