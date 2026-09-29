// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package layering

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// v01xPackages are the released v0.1.x manager packages that ADR 0011 says must
// carry a deprecation notice once v-next reaches feature parity.
var v01xPackages = []string{"algo", "future", "market", "stream", "trade"}

// v01xManagers maps each released v0.1.x manager package to the file and
// declaration that carries its deprecation marker. The marker belongs on the
// manager type rather than the package comment, so that `go doc` surfaces it on
// the identifier a caller actually writes.
var v01xManagers = map[string]string{
	"algo":   "Manager",
	"future": "Manager",
	"market": "Manager",
	"stream": "Client",
	"trade":  "Manager",
}

// TestV01xPackagesCarryTheDeprecationNotice checks the two things ADR 0011 asks
// for: a notice in the package documentation pointing at pkg/services, and a
// machine-readable marker on the manager type.
//
// The marker was prose-only through v0.1.x and landed in v1.0.0. An earlier
// version of this test asserted the marker was *absent*; it now asserts it is
// present, so removing it is a deliberate act that fails the build rather than a
// silent regression.
func TestV01xPackagesCarryTheDeprecationNotice(t *testing.T) {
	root := moduleRoot(t)

	for _, pkg := range v01xPackages {
		t.Run(pkg, func(t *testing.T) {
			docPath := filepath.Join(root, "pkg", "hstong", pkg, "doc.go")
			raw, err := os.ReadFile(docPath)
			if err != nil {
				t.Fatalf("reading %s: %v", docPath, err)
			}
			doc := string(raw)

			if !strings.Contains(doc, "Relationship to pkg/services") {
				t.Errorf("pkg/hstong/%s has no \"Relationship to pkg/services\" "+
					"section; ADR 0011 requires a notice directing consumers to "+
					"pkg/services", pkg)
			}
			if !strings.Contains(doc, "MIGRATION.md") {
				t.Errorf("pkg/hstong/%s's notice does not point at docs/MIGRATION.md, "+
					"so a reader has no next step", pkg)
			}

			decl := v01xManagers[pkg]
			if !managerCarriesMarker(t, root, pkg, decl) {
				t.Errorf("no // Deprecated: marker on %s in pkg/hstong/%s. v1.0.0 "+
					"introduced the marker, so its removal is a change to the "+
					"deprecation policy and should be a deliberate one", decl, pkg)
			}
		})
	}
}

// managerCarriesMarker reports whether the named type declaration is preceded by
// a // Deprecated: marker in the package's non-test sources.
func managerCarriesMarker(t *testing.T, root, pkg, decl string) bool {
	t.Helper()
	dir := filepath.Join(root, "pkg", "hstong", pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	declRe := regexp.MustCompile(`^type ` + regexp.QuoteMeta(decl) + ` struct`)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			if !declRe.MatchString(line) {
				continue
			}
			// Walk back over the doc comment, allowing for the blank comment
			// line GoDoc convention.
			for k := i - 1; k >= 0; k-- {
				trimmed := strings.TrimSpace(lines[k])
				if !strings.HasPrefix(trimmed, "//") {
					return false
				}
				if hasDeprecationMarker(lines[k]) {
					return true
				}
			}
			return false
		}
	}
	t.Fatalf("type %s not found in pkg/hstong/%s; the guard is not checking what "+
		"it was written for", decl, pkg)
	return false
}

// hasDeprecationMarker reports whether the file contains a real Go deprecation
// marker.
//
// The check is line-anchored on purpose. The notice has to *explain* what a
// // Deprecated: marker would do, so the file legitimately contains that
// substring inside prose - written as "// // Deprecated: marker would make
// staticcheck ...", where the text after the comment prefix begins with another
// "//". A plain strings.Contains check fires on that prose and, having done so,
// cannot tell the two apart. A real marker is a comment line whose text starts
// with "Deprecated: ", so matching the anchored form is what actually separates
// "this package is marked deprecated" from "this package explains why it is not
// marked deprecated".
func hasDeprecationMarker(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		// The space after "//" has to go too, or "// Deprecated: x" reduces to
		// " Deprecated: x" and never matches the prefix.
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "//"))
		if strings.HasPrefix(trimmed, "Deprecated: ") {
			return true
		}
	}
	return false
}

// TestMigrationSamplesKeepUsingTheOldShape is the other half of the decision
// above. The migration guide exists to show the v0.1.x call shape as the "before"
// column, so that package must keep importing pkg/hstong even while the notice
// is being added everywhere else - and a future cleanup that "modernises" it
// would silently destroy the guide's value.
func TestMigrationSamplesKeepUsingTheOldShape(t *testing.T) {
	root := moduleRoot(t)
	samples, err := os.ReadFile(filepath.Join(root, "internal", "migration", "samples.go"))
	if err != nil {
		t.Fatalf("reading internal/migration/samples.go: %v", err)
	}
	text := string(samples)
	if !strings.Contains(text, `"`+modulePrefix+`pkg/hstong/market"`) {
		t.Error("internal/migration/samples.go no longer imports pkg/hstong/market. " +
			"The \"before\" column of docs/MIGRATION.md is the v0.1.x shape; " +
			"converting the samples to the v-next shape removes the comparison " +
			"the guide exists to make.")
	}
	if !strings.Contains(text, "services.NewStack") {
		t.Error("internal/migration/samples.go no longer references services.NewStack, " +
			"so the \"after\" column of docs/MIGRATION.md is missing.")
	}
}
