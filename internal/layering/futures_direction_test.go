// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package layering

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestFuturesEntrustBSIsTwoValuesEverywhere guards the C1b resolution against
// drifting back apart.
//
// The failure it prevents is specific and had already happened once. Two sources
// disagreed about the futures `entrustBs` set - the vendor's two-value futures
// enum and a four-value set in SPEC §7.4 - and the resolution was *not* that one
// of them was wrong. It was that they describe different endpoints: the cash
// endpoint takes 1-4 and the futures endpoint takes 1-2. SPEC presented the cash
// four under a generic heading, which is what made the pair look contradictory,
// and the SDK adopted the wider set for futures for eleven releases on the
// strength of that misreading.
//
// So the invariant is not "the number is 2"; it is that the cash four and the
// futures two are both present, in the spec, attributed to different endpoints -
// and that the two validators agree with the futures half.
func TestFuturesEntrustBSIsTwoValuesEverywhere(t *testing.T) {
	root := moduleRoot(t)

	specRaw, err := os.ReadFile(filepath.Join(root, "docs", "SPEC.md"))
	if err != nil {
		t.Fatalf("reading docs/SPEC.md: %v", err)
	}
	// Scope to §7.4. A whole-file search is too coarse and the mutation run proved
	// it: `TradeEntrust` also names the cash endpoint's own section, and 空头平仓
	// also appears in the legacy dictionary, so deleting either from §7.4 was
	// invisible to a whole-file check. Both survived.
	spec := sectionOf(t, string(specRaw), "### 7.4 ")

	// 1. The section must name both endpoints, or the two sets look like one again.
	for _, want := range []string{"TradeEntrust", "FuturesEntrust"} {
		if !strings.Contains(spec, want) {
			t.Errorf("docs/SPEC.md §7.4 does not mention %s, so the cash and futures "+
				"entrustBs sets are not attributed to separate endpoints. That "+
				"attribution IS the resolution; without it the 4-vs-2 reading "+
				"looks like a contradiction again", want)
		}
	}
	// 2. Assert the two TABLES, not merely that the terms appear. The first
	//    version checked for 空头平仓 / 空头开仓 anywhere in the section and both
	//    deleting-the-row mutations survived, because §7.4 also quotes the
	//    vendor's own cash sentence, which contains both terms. The contract is
	//    the table shape: one table of four cash values, one of two futures
	//    values. That is what C1b is actually about.
	cash, futures := sectionTables(spec)
	if got := cash.sorted(); !equalStrings(got, []string{"1", "2", "3", "4"}) {
		t.Errorf("§7.4 cash table has values %v, want [1 2 3 4]. The futures narrowing "+
			"must not remove the cash set: POST /trade/TradeEntrust documents 3 and 4",
			got)
	}
	if got := futures.sorted(); !equalStrings(got, []string{"1", "2"}) {
		t.Errorf("§7.4 futures table has values %v, want [1 2]. The vendor documents "+
			"POST /trade/FuturesEntrust as \"1:买入,2:卖出\" and offers no third value",
			got)
	}
	// 3. It must carry the vendor's two, in the futures half.
	if !strings.Contains(spec, "1:买入,2:卖出") {
		t.Error("docs/SPEC.md no longer quotes the vendor's futures value set " +
			"(\"1:买入,2:卖出\"), so the futures half of the resolution is unsourced")
	}

	// 4. Neither validator may accept 3 or 4 on a futures mutation.
	for _, target := range []struct {
		file string
		fn   string
	}{
		{filepath.Join("pkg", "hstong", "future", "future.go"), "validateEntrustBS"},
		{filepath.Join("pkg", "services", "futures.go"), "futuresValidateEntrustBS"},
	} {
		rel, _ := filepath.Rel(root, filepath.Join(root, target.file))
		raw, err := os.ReadFile(filepath.Join(root, target.file))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		body := functionBody(t, string(raw), target.fn)
		for _, forbidden := range []string{"EntrustCloseShort", "EntrustOpenShort"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s: %s accepts %s. The futures endpoint documents only 1 and "+
					"2; 3 and 4 are cash values (docs/SPEC.md 7.4, tracker C1b). "+
					"Accepting them here is the exact defect the resolution removed",
					rel, target.fn, forbidden)
			}
		}
		for _, required := range []string{"EntrustBuy", "EntrustSell"} {
			if !strings.Contains(body, required) {
				t.Errorf("%s: %s no longer accepts %s; the futures set is {1,2}",
					rel, target.fn, required)
			}
		}
	}
}

// valueSet is the set of first-column values found in one Markdown table.
type valueSet map[string]bool

func (v valueSet) sorted() []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sectionTables returns the value sets of the two-value and four-value tables in
// a section, matched by size rather than by position so that reordering the
// prose does not break the check.
func sectionTables(section string) (cash, futures valueSet) {
	cash, futures = valueSet{}, valueSet{}
	for _, table := range tablesIn(section) {
		set := valueSet{}
		for _, row := range table {
			cells := strings.Split(row, "|")
			if len(cells) < 3 {
				continue
			}
			first := strings.Trim(strings.TrimSpace(cells[1]), "`")
			if first == "Value" {
				continue // header
			}
			set[first] = true
		}
		switch len(set) {
		case 4:
			cash = set
		case 2:
			futures = set
		}
	}
	return cash, futures
}

// tablesIn splits a section into its pipe tables, each a slice of data rows.
func tablesIn(section string) [][]string {
	var tables [][]string
	var current []string
	flush := func() {
		if len(current) > 0 {
			tables = append(tables, current)
			current = nil
		}
	}
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") {
			if strings.HasPrefix(trimmed, "|-") || strings.HasPrefix(trimmed, "| -") {
				continue // separator row
			}
			current = append(current, trimmed)
			continue
		}
		flush()
	}
	flush()
	return tables
}

// sectionOf returns the body of a heading up to the next heading of any level.
func sectionOf(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("heading %q not found; the guard is not checking what it was written for", heading)
	}
	rest := doc[start+len(heading):]
	for i, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(line, "#") {
			return strings.Join(strings.Split(rest, "\n")[:i], "\n")
		}
	}
	return rest
}

// functionBody returns the text of a top-level func, by brace counting from its
// signature. Deliberately simple: these are the only two functions in the
// repository whose body is a single switch, and a full AST pass would be more
// machinery than the invariant deserves.
func functionBody(t *testing.T, src, name string) string {
	t.Helper()
	lines := strings.Split(src, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "func "+name+"(") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("func %s not found; the guard is not checking what it was written for", name)
	}
	depth := 0
	seenBody := false
	var out []string
	for _, line := range lines[start:] {
		out = append(out, line)
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if strings.Contains(line, "{") {
			seenBody = true
		}
		if seenBody && depth <= 0 {
			break
		}
	}
	return strings.Join(out, "\n")
}
