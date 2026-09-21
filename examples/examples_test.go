// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package examples contains only tests. The runnable example programs live in
// the subdirectories of this directory.
//
// TestExamplesBuild walks every examples/*/main.go and parses it with
// go/parser, asserting that it declares "package main" and defines a "func
// main()". This is deliberately a static check rather than a compilation of the
// examples: `go build ./examples/...` already compiles them in CI, so the test
// guards the two invariants a bare build does not state — that each example is
// an executable command, and that every example file carries the SPDX header.
package examples

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spdxHeader is the license header every hand-written Go file must carry
// (AGENTS.md).
const spdxHeader = "SPDX-License-Identifier: Apache-2.0"

// TestExamplesBuild checks every examples/*/main.go for a main package, a main
// function, and the SPDX header.
func TestExamplesBuild(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("*", "main.go"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no examples/*/main.go found; the walk is looking in the wrong directory")
	}

	fset := token.NewFileSet()
	for _, path := range matches {
		path := path
		t.Run(filepath.Dir(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if !strings.Contains(string(raw), spdxHeader) {
				t.Errorf("%s is missing the %q header", path, spdxHeader)
			}

			file, err := parser.ParseFile(fset, path, raw, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if file.Name == nil || file.Name.Name != "main" {
				t.Fatalf("%s: package = %v, want main", path, file.Name)
			}

			var hasMain bool
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Name != nil && fn.Name.Name == "main" && fn.Recv == nil {
					hasMain = true
					break
				}
			}
			if !hasMain {
				t.Fatalf("%s: no func main() found", path)
			}
		})
	}
}
