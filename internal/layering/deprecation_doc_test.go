// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package layering

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v01xPackages are the released v0.1.x manager packages that ADR 0011 says must
// carry a deprecation notice once v-next reaches feature parity.
var v01xPackages = []string{"algo", "future", "market", "stream", "trade"}

// TestV01xPackagesCarryTheDeprecationNotice checks the notice ADR 0011 promised
// is actually present in each released manager package.
//
// The notice is prose, not a // Deprecated: marker, and that is a deliberate
// decision rather than an omission: staticcheck is enabled in this repository,
// so a marker would trip SA1019 on this repository's own examples and on
// internal/migration/samples.go - the file whose whole job is showing the old
// call shape. Adding one during a v0.1.x patch line would also make every
// downstream consumer's build emit warnings they did not ask for. The marker
// ships with v1.0.0.
//
// This test asserts the marker is absent, so that the day someone adds it in a
// patch release, this fails and asks whether it was meant.
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
					"section; ADR 0011 requires a deprecation notice directing "+
					"consumers to pkg/services", pkg)
			}
			if !strings.Contains(doc, "MIGRATION.md") {
				t.Errorf("pkg/hstong/%s's notice does not point at docs/MIGRATION.md, "+
					"so a reader has no next step", pkg)
			}
			// Asserted as the whole sentence, not as a bare "v1.0.0" substring.
			// A bare substring check is satisfied by any incidental mention of the
			// version anywhere in the file, which is exactly the kind of assertion
			// that survives a mutation removing the sentence it was written for.
			if !strings.Contains(doc, "machine-readable marker ships with v1.0.0") {
				t.Errorf("pkg/hstong/%s's notice does not say that the "+
					"machine-readable marker ships with v1.0.0. Without that the "+
					"notice states a deprecation with no date, which is the one thing "+
					"a reader needs to plan a migration", pkg)
			}
			if hasDeprecationMarker(doc) {
				t.Errorf("pkg/hstong/%s carries a // Deprecated: marker. This test "+
					"records the decision to keep the v0.1.x notice prose-only until "+
					"v1.0.0; if the marker was added deliberately, update this test and "+
					"the eight call sites it will break (six examples, "+
					"internal/migration/samples.go, scripts/coverage_gate.go)", pkg)
			}
		})
	}
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
