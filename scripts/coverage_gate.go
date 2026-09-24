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
	}
	gate := 85.0
	failed := false

	for _, pkg := range pkgs {
		cmd := exec.Command("go", "test", "-coverprofile=coverage_tmp.out", "-covermode=atomic", "-cover", pkg.path) // #nosec G204 -- fixed argv from a hardcoded package list in a developer script; no external input
		cmd.Dir = "."
		out, _ := cmd.CombinedOutput()
		pct := extractCoverage(string(out))
		if pct < gate {
			fmt.Printf("FAIL: %s %.1f%%\n", pkg.name, pct)
			failed = true
		} else {
			fmt.Printf("PASS: %s %.1f%%\n", pkg.name, pct)
		}
		_ = os.Remove("coverage_tmp.out")
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("All packages meet the 85% coverage gate")
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
