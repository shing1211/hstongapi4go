// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// guidePath is docs/MIGRATION.md relative to this package.
const guidePath = "../MIGRATION.md"

// goBlockRE extracts a fenced go block, keeping only the body.
var goBlockRE = regexp.MustCompile("(?s)```go\n(.*?)```")

// TestEverySampleInTheGuideIsPublished ties the published guide to the compiled
// code, which is the whole reason the samples are a package and not prose.
//
// Each ```go block in the guide must contain the body of a Sample* function that
// exists here. Three failure modes are caught, and none of them is caught by
// anything else:
//
//   - a sample in the guide that was never compiled, or was edited in the guide
//     and drifted from the code (the function signature is matched, so a
//     signature edit on one side only fails)
//   - a Sample* function with no counterpart in the guide, which is stale code
//     that will drift
//   - an ellipsis placeholder, which is the specific defect the first draft of
//     this guide shipped with
func TestEverySampleInTheGuideIsPublished(t *testing.T) {
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("reading the guide: %v", err)
	}
	guide := string(raw)

	published := publishedSampleBodies(t)

	var blocks []string
	for _, m := range goBlockRE.FindAllStringSubmatch(guide, -1) {
		blocks = append(blocks, m[1])
	}
	if len(blocks) == 0 {
		t.Fatal("the guide has no ```go block, so this test would pass vacuously; " +
			"either the samples moved or the extraction is broken")
	}

	for i, body := range blocks {
		if strings.Contains(body, "...") {
			t.Errorf("go block %d contains an ellipsis. A sample with a placeholder is not "+
				"runnable, which is the defect this package exists to prevent. Publish the "+
				"whole statement, or point at the Sample* function by name instead", i+1)
		}
		if !hasSampleSignature(body, published) {
			t.Errorf("go block %d does not correspond to any Sample* function in this "+
				"package:\n%s", i+1, indent(body))
		}
	}

	// Every Sample* function must be published, or it is stale code.
	for _, sig := range published {
		if !guidePublishes(sig) {
			t.Errorf("Sample%s is compiled here but not published in the guide:\n  func %s",
				sig.name, sig.line)
		}
	}
}

// sampleSig is a published sample's function name and its parameter list, which
// together are enough to locate its body.
type sampleSig struct {
	name string
	line string
	full string
}

var sampleFuncRE = regexp.MustCompile(`(?m)^func (Sample[A-Za-z0-9_]*)\(([^\n]*)$`)

// publishedSampleBodies reads this file and returns one sampleSig per Sample*
// function, keyed by name, with the full declaration line used for matching.
func publishedSampleBodies(t *testing.T) map[string]sampleSig {
	t.Helper()
	self, err := os.ReadFile("samples.go")
	if err != nil {
		t.Fatalf("reading samples.go: %v", err)
	}
	out := map[string]sampleSig{}
	for _, m := range sampleFuncRE.FindAllStringSubmatch(string(self), -1) {
		name := m[1]
		line := strings.TrimSpace(m[0])
		full := extractFunction(string(self), "func "+name+"(")
		if full == "" {
			t.Fatalf("could not extract the body of %s from samples.go", name)
		}
		out[name] = sampleSig{name: name, line: line, full: full}
	}
	if len(out) == 0 {
		t.Fatal("no Sample* functions found in samples.go; the regex or the naming has drifted")
	}
	return out
}

// hasSampleSignature reports whether a guide block is the published form of one
// of the Sample* functions.
//
// It compares the **whole normalised function**, not the signature, and the
// difference is not cosmetic. A signature-only matcher was tried first and two
// mutations survived it: changing a type inside a sample body in the guide, and
// rewording a statement. Both left the guide telling a reader to paste code that
// would not build or would behave differently, with every check green.
//
// Whole-body comparison is possible only because the guide publishes complete
// functions rather than excerpts. The cost is that editing a sample in the guide
// now requires editing the code too - which is the intended behaviour.
func hasSampleSignature(body string, published map[string]sampleSig) bool {
	guide := normalizeCode(body)
	for _, sig := range published {
		if guide == normalizeCode(sig.full) {
			return true
		}
	}
	return false
}

// extractFunction returns the text of the top-level func whose declaration line
// starts with prefix, from that line to the first following line that is exactly
// "}". Every Sample function is gofmt'd and top level, so that is sufficient and
// far more predictable than brace counting.
func extractFunction(src, prefix string) string {
	lines := strings.Split(src, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start; i < len(lines); i++ {
		// Column 0 exactly. A nested closing brace trims to "}" too, and
		// matching it would truncate the function at its first loop or if-block.
		if lines[i] == "}" {
			return strings.Join(lines[start:i+1], "\n")
		}
	}
	return ""
}

// normalizeCode strips line comments and collapses whitespace, so that a
// reformatting is invisible and a change of statement is not.
func normalizeCode(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString(" ")
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// guidePublishes reports whether the guide shows a function whose name is sig.
func guidePublishes(sig sampleSig) bool {
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), "func "+sig.name+"(")
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}

// TestNoPlaceholdersInAnySample guards the specific regression: the first draft of
// this guide contained market.BasicQotRequest{...}, which is not Go.
func TestNoPlaceholdersInAnySample(t *testing.T) {
	self, err := os.ReadFile("samples.go")
	if err != nil {
		t.Fatalf("reading samples.go: %v", err)
	}
	for i, line := range strings.Split(string(self), "\n") {
		if strings.Contains(line, "...") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			t.Errorf("samples.go:%d contains an ellipsis: %s", i+1, strings.TrimSpace(line))
		}
	}
}

// TestTheGuideIsNotAPlaceholderItself catches the other regression in the first
// draft: a section that says it "will be populated when v1.0 is released" while
// claiming to be the migration guide.
func TestTheGuideIsNotAPlaceholderItself(t *testing.T) {
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("reading the guide: %v", err)
	}
	guide := string(raw)
	for _, phrase := range []string{
		"will be populated when v1.0 is released",
		"All items below represent current intentions",
		"(planned)",
	} {
		if strings.Contains(guide, phrase) {
			t.Errorf("the guide still contains the placeholder phrase %q. A migration guide "+
				"marked planned is a document nobody can act on, and this one is a "+
				"supported document", strconv.Quote(phrase))
		}
	}
}
