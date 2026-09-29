// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package layering

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// architectureDoc is the document this file keeps honest.
const architectureDoc = "ARCHITECTURE.md"

// docPackageRowRE captures a row of section 5's table: a backticked package path
// and whether the row claims it has a production importer.
var docPackageRowRE = strings.NewReplacer()

// TestVNextImportersInSection5AreAccurate recomputes the fact ARCHITECTURE.md
// section 5 asserts, rather than trusting the document.
//
// Section 5 is the section most likely to go quietly false, because nothing in
// the build depends on it and it is not derived from a test. It had already
// drifted twice by the time this was written: it claimed `pkg/transport` was
// imported by `pkg/services/account.go`, which D3 removed when it closed the wire
// leak, and it claimed `internal/auth` had no production importer, which C13
// made false when `SessionService` began composing it. Both were plausible on
// reading and wrong against the source.
//
// The check is deliberately narrow - it verifies the *importer* claim per package
// and nothing else in the document - because a wider check would need to model
// prose, and a check that cannot be satisfied is one that gets deleted.
func TestVNextImportersInSection5AreAccurate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), architectureDoc))
	if err != nil {
		t.Fatalf("reading %s: %v", architectureDoc, err)
	}
	doc := string(raw)

	section := sectionBody(t, doc, "## 5. ")
	if section == "" {
		t.Fatal("section 5 not found; the heading changed, so this test no longer checks " +
			"the table it was written for")
	}

	checked := 0
	for _, row := range tableRows(section) {
		pkg, claims, ok := parseImportRow(row)
		if !ok {
			continue
		}
		checked++
		importers := productionImportersOf(t, pkg)
		hasNonTest := false
		for _, imp := range importers {
			if !strings.HasSuffix(imp, "_test.go") {
				hasNonTest = true
				break
			}
		}
		if claims && !hasNonTest {
			t.Errorf("ARCHITECTURE.md section 5 says %s is imported by SDK production "+
				"code, but no non-test file imports it. Actual importers: %v",
				pkg, importers)
		}
		if !claims && hasNonTest {
			t.Errorf("ARCHITECTURE.md section 5 says nothing in the SDK imports %s, but "+
				"these non-test files do: %v", pkg, importers)
		}
	}
	// A row-parsing change - a cell moved, a bold marker dropped, a backtick
	// added - would leave the loop running over nothing and this test green. The
	// same lesson as TestLayeringRulesCoverTheKnownPackages, in a different file.
	if checked < 4 {
		t.Errorf("section 5's table yielded %d checkable package rows, want at least 4 "+
			"(pkg/domain, pkg/services, pkg/transport, internal/auth). The table's "+
			"shape has probably changed and this test is no longer checking it",
			checked)
	}
}

// sectionBody returns the text of a section from its heading to the next heading
// of the same or higher level.
func sectionBody(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	if start < 0 {
		return ""
	}
	rest := doc[start+len(heading):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		return rest[:next]
	}
	return rest
}

// tableRows returns the pipe-delimited rows of the first table in a section.
func tableRows(section string) []string {
	var rows []string
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if strings.HasPrefix(trimmed, "|-") || strings.HasPrefix(trimmed, "| -") {
			continue
		}
		rows = append(rows, trimmed)
	}
	return rows
}

// parseImportRow reads a row as (package, claimsProductionImporter). The table
// has two shapes: "Yes, ..." and "**No.** ...", plus rows about removed symbols
// and about pkg/domain, which are skipped because they are not importer claims.
func parseImportRow(row string) (pkg string, claims bool, ok bool) {
	cells := strings.Split(row, "|")
	// Section 5's table has exactly two columns - package, then the importer
	// claim - so the claim is the third cell, index 2. Getting this wrong parses
	// zero rows and passes vacuously, which is why the caller asserts a floor.
	if len(cells) < 3 {
		return "", false, false
	}
	pkg = strings.Trim(strings.TrimSpace(cells[1]), "`")
	if !strings.HasPrefix(pkg, "pkg/") && !strings.HasPrefix(pkg, "internal/") {
		return "", false, false
	}
	// A symbol row such as `internal/push.Manager` is a retirement note, not an
	// importer claim.
	if strings.Count(pkg, ".") > strings.Count(pkg, "/") {
		return "", false, false
	}
	body := strings.TrimSpace(cells[2])
	lowered := strings.ToLower(body)
	switch {
	case strings.HasPrefix(lowered, "**no"):
		return pkg, false, true
	case strings.HasPrefix(lowered, "**yes"), strings.HasPrefix(lowered, "yes"):
		return pkg, true, true
	}
	return "", false, false
}

// productionImportersOf lists the repository's non-test Go files that import pkg,
// which is the same measure section 5 was written against: SDK source only, not
// the coverage gate, the mock gateway, the examples or the documentation
// samples.
func productionImportersOf(t *testing.T, pkg string) []string {
	t.Helper()
	root := moduleRoot(t)
	want := `"` + modulePrefix + pkg + `"`

	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[rel] || strings.HasPrefix(rel, "scripts/") {
				return filepath.SkipDir
			}
			if rel == "test" || rel == "cmd" || rel == "examples" || rel == "docs" || rel == "site" {
				return filepath.SkipDir
			}
			// internal/migration is the compiled documentation-samples package
			// that keeps MIGRATION.md honest. It is not SDK code: nothing
			// depends on it, it has no callers, and it exists only to be
			// compiled. Counting its imports would make every v-next package
			// look reachable and would make section 5's claim false for a
			// package the SDK does not actually use. ARCHITECTURE.md section 5
			// states the same exclusion, so the two agree by construction
			// rather than by coincidence.
			if rel == "internal/migration" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			if imp.Path != nil && imp.Path.Value == want {
				out = append(out, rel)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	return out
}

var _ = docPackageRowRE
