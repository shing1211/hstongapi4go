// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This suite drives the guard against throwaway trees built with --root
// semantics, never by editing the repository. That discipline is the point: the
// guard's verification plan is a table of mutations (design-parity-guard.md §9),
// and a guard that can only be mutation-tested by editing the repository
// produces weaker evidence and risks leaving a planted mutation behind.
//
// Two independent readers of the same tree keep the assertions honest. The guard
// parses the const block and the scan roots with go/ast; the helpers below read
// them as text with a regexp. A counting bug in one is very unlikely to be
// mirrored in the other, so agreeing on 51 declared and 51 referenced is a
// cross-check rather than a restatement.

const (
	routesFile = "client/routes.go"
	specFile   = "docs/SPEC.md"
)

// routeNameRE matches a declared constant in routes.go as text. The guard finds
// these with go/ast; this is deliberately a different method.
var routeNameRE = regexp.MustCompile(`^\s*(Route[A-Za-z0-9]+)\s+Route\s*=\s*"([^"]+)"`)

// clientRefRE matches a package-qualified reference in a scanned file as text.
// Requiring an uppercase letter after "Route" is what keeps `client.Routes`,
// `client.Route` (the type) and `client.Route(` (a conversion) out.
var clientRefRE = regexp.MustCompile(`client\.(Route[A-Z][A-Za-z0-9]*)`)

// repoRoot is the module root, derived from this test's working directory so
// the test does not depend on how it was invoked. It is computed independently
// of the guard's own resolveRoot, which is under test.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd is <root>/scripts/paritygate
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(routesFile))); err != nil {
		t.Fatalf("cannot find %s under %s: %v", routesFile, root, err)
	}
	return root
}

// scannedDirs are the paths copied into a fixture. They are exactly the inputs
// the guard reads, so a fixture is small and the copy is fast.
var scannedDirs = []string{"pkg/services", "pkg/transport", "internal/auth"}

// fixture copies the guard's inputs out of the real repository into a
// throwaway tree. Copying the real inputs rather than synthesizing them means
// every fixture case is exercised against the shape the guard actually meets in
// production, including SPEC's CRLF line endings in this working tree.
func fixture(t *testing.T) string {
	t.Helper()
	src := repoRoot(t)
	dst := t.TempDir()
	for _, rel := range append([]string{routesFile, specFile}, scannedDirs...) {
		copyPath(t, filepath.Join(src, filepath.FromSlash(rel)), filepath.Join(dst, filepath.FromSlash(rel)))
	}
	return dst
}

func copyPath(t *testing.T, src, dst string) {
	t.Helper()
	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat %s: %v", src, err)
	}
	if !info.IsDir() {
		data, readErr := os.ReadFile(src)
		if readErr != nil {
			t.Fatalf("read %s: %v", src, readErr)
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o700); mkErr != nil {
			t.Fatalf("mkdir for %s: %v", dst, mkErr)
		}
		if writeErr := os.WriteFile(dst, data, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", dst, writeErr)
		}
		return
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("readdir %s: %v", src, err)
	}
	for _, e := range entries {
		copyPath(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
}

// run evaluates and renders the guard against a tree, returning the report text
// and the exit code.
func run(root string, enforce bool) (string, int) {
	cfg := defaultConfig()
	cfg.Root = root
	cfg.Enforce = enforce
	return evaluate(cfg).report()
}

// readFile reads a file from a fixture tree.
func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// writeFile writes a file into a fixture tree.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// declaredAsText reads (name, path) pairs out of the fixture's const block with
// a regexp, independently of the guard's AST parse.
func declaredAsText(t *testing.T, root string) [][2]string {
	t.Helper()
	var out [][2]string
	for _, line := range strings.Split(readFile(t, root, routesFile), "\n") {
		if m := routeNameRE.FindStringSubmatch(strings.TrimSuffix(line, "\r")); m != nil {
			out = append(out, [2]string{m[1], m[2]})
		}
	}
	return out
}

// referencedAsText reads the distinct declared-looking names a scanned non-test
// file mentions, independently of the guard's AST walk.
func referencedAsText(t *testing.T, root string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, dir := range scannedDirs {
		base := filepath.Join(root, filepath.FromSlash(dir))
		filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return err
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			for _, m := range clientRefRE.FindAllStringSubmatch(string(data), -1) {
				names[m[1]] = true
			}
			return nil
		})
	}
	return names
}

// allRoutesSource returns a non-test source file in pkg/services that names
// every declared constant, which is how the full-parity case is built.
func allRoutesSource(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("package services\n\n")
	b.WriteString("import \"github.com/shing1211/hstongapi4go/client\"\n\n")
	b.WriteString("// parityFixtureAll names every declared route so the guard sees full parity.\n")
	b.WriteString("func parityFixtureAll() []client.Route {\n\treturn []client.Route{\n")
	for _, d := range declaredAsText(t, root) {
		b.WriteString("\t\tclient." + d[0] + ",\n")
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

// gappedFixture returns a fixture with a gap of exactly gap declared routes.
//
// The cases that need a non-zero gap used to inherit one from the real tree, and
// the real tree is now at full parity — C15 wired the last two endpoints — so they
// would otherwise have nothing to test. Deriving the gap instead of inheriting it
// is the right direction: it makes a case's premise explicit at the top of the
// case, and it stops every one of them from silently turning into a second copy of
// the full-parity case the moment the repository reaches parity.
//
// The gap is produced by replacing the scanned roots with a single file that names
// every declared constant except the last `gap` of them. The other two roots are
// emptied rather than deleted, because a missing scan root is a fatal integrity
// failure and this helper's callers are about the gap, not about that.
func gappedFixture(t *testing.T, gap int) string {
	t.Helper()
	if gap < 0 {
		t.Fatalf("a negative gap (%d) is not a tree this guard can produce", gap)
	}
	root := fixture(t)
	declared := declaredAsText(t, root)
	if gap >= len(declared) {
		t.Fatalf("a gap of %d would leave fewer than one referenced route, and the "+
			"walk's own vacuity check would then fail for the wrong reason", gap)
	}
	for _, dir := range scannedDirs {
		base := filepath.Join(root, filepath.FromSlash(dir))
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatalf("readdir %s: %v", base, err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".go") {
				if err := os.Remove(filepath.Join(base, e.Name())); err != nil {
					t.Fatalf("remove %s: %v", e.Name(), err)
				}
			}
		}
	}

	var b strings.Builder
	b.WriteString("package services\n\n")
	b.WriteString("import \"github.com/shing1211/hstongapi4go/client\"\n\n")
	fmt.Fprintf(&b, "// parityFixturePartial names every declared route except the last %d, so the\n"+
		"// guard sees a gap of exactly %d however far the real tree has advanced.\n"+
		"func parityFixturePartial() []client.Route {\n\treturn []client.Route{\n", gap, gap)
	for i, d := range declared {
		if i < len(declared)-gap {
			b.WriteString("\t\tclient." + d[0] + ",\n")
		}
	}
	b.WriteString("\t}\n}\n")
	writeFile(t, root, "pkg/services/parity_partial.go", b.String())
	return root
}

// TestFullParity covers V1: with every declared route named by a service, the
// gap is zero, report mode still exits 0, and enforcing mode exits 0 too. This
// is the case the C14 flip will move the real repository into, so it is proven
// here rather than assumed on the day the gate starts failing.
//
// Since C15 the real repository is in this case, so the synthetic file below adds
// nothing the tree does not already say. It is kept because the case is about the
// guard's contract at zero, not about the repository, and a case that reads the
// real tree can only ever be run when the tree happens to be at parity — which is
// exactly when it is least useful as a control.
func TestFullParity(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_all.go", allRoutesSource(t, root))

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode at full parity must exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0 mode=report enforce=off") {
		t.Errorf("want the machine-readable line to report 51/51 gap=0:\n%s", out)
	}
	if !strings.Contains(out, "Unwired (0 of 51 declared)") {
		t.Errorf("want an empty unwired list:\n%s", out)
	}

	out, code = run(root, true)
	if code != 0 {
		t.Fatalf("enforcing mode at full parity must exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0 mode=enforce enforce=on") {
		t.Errorf("want the enforcing machine-readable line to report gap=0:\n%s", out)
	}
	// PASS is permitted only here: gap 0 and enforcing.
	if !strings.Contains(out, "PASS: 51/51 endpoints named by a v-next service") {
		t.Errorf("want the enforcing zero-gap PASS line:\n%s", out)
	}
	if !strings.Contains(out, "PARITY GUARD - ENFORCING MODE") {
		t.Errorf("enforcing mode must name itself in the banner:\n%s", out)
	}
	if strings.Contains(out, "REPORT MODE") {
		t.Errorf("enforcing mode must not print the report banner:\n%s", out)
	}
	if strings.Contains(out, "does NOT mean v-next is at parity") {
		t.Errorf("the report-mode disclaimer belongs only to report mode:\n%s", out)
	}
}

// TestRealTreeIsNotVacuous is the non-vacuity proof for the real repository: the
// guard must find 51 declared routes and a non-zero number of them referenced. If
// either number were 0 the AST walk would be broken, and a broken walk reports
// "everything is wired" rather than failing.
//
// The 51 is a pinned literal, exactly as client/routes_test.go's wantRouteCount
// is: a commit that adds a route to client and SPEC must update it here too. The
// referenced count is not pinned -- it is cross-checked against an independent
// text scan, so the assertion survives the C-series closing the gap and keeps
// failing if the walk and the tree ever disagree.
func TestRealTreeIsNotVacuous(t *testing.T) {
	out, code := run(repoRoot(t), false)
	if code != 0 {
		t.Fatalf("report mode against the real tree must exit 0, got %d:\n%s", code, out)
	}
	declared := declaredAsText(t, repoRoot(t))
	if len(declared) != 51 {
		t.Fatalf("the const block declares %d routes, not 51; this test and "+
			"client/routes_test.go both pin that number", len(declared))
	}
	names := make(map[string]bool, len(declared))
	for _, d := range declared {
		names[d[0]] = true
	}
	referenced := referencedAsText(t, repoRoot(t))
	if len(referenced) == 0 {
		t.Fatal("the independent text scan found no references either; the fixture is wrong, not the guard")
	}
	for name := range referenced {
		if !names[name] {
			t.Errorf("the text scan saw %s, which is not a declared route constant; "+
				"the guard would have silently ignored it", name)
		}
	}
	wired := len(referenced)
	if !strings.Contains(out, "PARITY: "+strconv.Itoa(wired)+"/51 gap="+strconv.Itoa(51-wired)+" mode=report enforce=off") {
		t.Errorf("the guard and the independent scan disagree: want %d referenced of 51:\n%s", wired, out)
	}
	if !strings.Contains(out, "  Referenced ("+strconv.Itoa(wired)+" of 51), file:line for every counted reference:") {
		t.Errorf("want every counted reference listed, not a summary:\n%s", out)
	}
}

// TestRealTreeUnwiredSetIsTheKnownBacklog pins the real repository's gap, which
// is zero as of C15.
//
// # Why the snapshot has to move, and why it is still a snapshot
//
// This test is a baseline, not a derived assertion: it records what the guard
// measured against the real tree at the moment of writing, so a silent change in
// either direction shows up as a failing test rather than as a diff nobody reads.
// That is the same friction client/routes_test.go carries for an added route, and
// it is deliberate. It moved from 22 to 14 at C4 (eight futures reads), 14 to 11
// at C5 (three futures mutations), 11 to 4 at C7-C9 (all seven algo endpoints),
// 4 to 2 at C13 (the two trade-session endpoints), and 2 to 0 at C15 (the two
// trade-push subscription endpoints, SPEC 2.6 rows 32 and 33). C14 then has
// nothing left to close.
//
// # Why the test is still meaningful with an empty list
//
// It was not a presence-only check and it is not one now. The by-name section used
// to assert that each of the two backlog endpoints appeared in the unwired list;
// that assertion is now vacuous, because there is nothing to name. **The absence
// direction is therefore the whole test**, and it is a stronger property than the
// one it replaces: every one of the 51 declared constants must be absent from the
// unwired list. That is the assertion which would catch the guard's own worst
// failure — a reference walk that stopped crediting one file and reported 50/51 as
// a clean bill of health, or one that under-counted and so listed a wired route as
// unwired. Neither is visible in "the list is empty"; both are visible in "no
// declared name is in the list", and the second is checked against the declared
// set read independently by the regexp reader rather than against the guard's own
// parse, so a walk that silently lost a name from its parse cannot make the list
// and the expectation agree on the wrong set.
//
// The other three parts are unchanged in kind and still load-bearing: the per-group
// table (so a route wired in one group and not another is caught), the TOTAL row
// (so the arithmetic still adds up), and the enforce-mode expectations below (so
// the zero gap is proven to be a zero the gate accepts rather than a number nobody
// acted on).
func TestRealTreeUnwiredSetIsTheKnownBacklog(t *testing.T) {
	root := repoRoot(t)
	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}

	want := map[string][3]int{
		"Market pull":              {9, 9, 0},
		"Market subscription":      {2, 2, 0},
		"Trade session":            {2, 2, 0},
		"Trade assets / positions": {5, 5, 0},
		"Trade orders":             {13, 13, 0},
		"Trade push subscribe":     {2, 2, 0},
		"Algo / strategy":          {7, 7, 0},
		"Futures":                  {11, 11, 0},
		"TOTAL":                    {51, 51, 0},
	}
	got := groupRows(t, out)
	if len(got) != len(want) {
		t.Fatalf("want %d group rows, got %d:\n%s", len(want), len(got), out)
	}
	for title, columns := range want {
		row, ok := got[title]
		if !ok {
			t.Errorf("group %q is missing from the table:\n%s", title, out)
			continue
		}
		if row != columns {
			t.Errorf("group %q: want declared/wired/gap %v, got %v", title, columns, row)
		}
	}

	// The backlog arithmetic, stated rather than implied. A TOTAL of 51 wired and a
	// zero gap is only a zero gap if the declared side still totals 51, and the sum
	// of the eight per-group declared counts is that same number written a third
	// time - so this row catches a group header edited without the Total, which is
	// the one place the guard's own grouping and its count could disagree.
	sum := 0
	for _, title := range []string{
		"Market pull", "Market subscription", "Trade session", "Trade assets / positions",
		"Trade orders", "Trade push subscribe", "Algo / strategy", "Futures",
	} {
		sum += want[title][0]
	}
	if sum != want["TOTAL"][0] {
		t.Errorf("the per-group declared counts total %d, but the TOTAL row says %d; the "+
			"table above is not a coherent snapshot", sum, want["TOTAL"][0])
	}

	// The unwired header states the zero, and the section says so in words rather
	// than being blank. "(none)" is the guard's own rendering of an empty set, and
	// asserting it keeps a parser change that emitted nothing there from reading as
	// parity.
	if !strings.Contains(out, "  Unwired (0 of 51 declared), by SPEC group:") {
		t.Errorf("want the unwired header to state the gap:\n%s", out)
	}
	if !strings.Contains(out, "    (none)\n") {
		t.Errorf("want an empty unwired list rendered as \"(none)\":\n%s", out)
	}

	// The by-name list, empty. The parse is over the report's own names, so this is
	// the claim "the guard named nothing unwired" rather than "the string (none) is
	// present", which the row above already made.
	if names := requireUnwired(t, out, 0); len(names) != 0 {
		t.Errorf("the unwired list is not empty: %v", names)
	}

	// The absence direction, which is now the point of this test. Every declared
	// constant - all 51, listed out rather than derived, so a route added to client
	// and SPEC without a line here is a failing test and not a silently smaller
	// assertion - must be absent from the unwired list. The list is checked for the
	// name as a whole line ("      RouteX\\n"), because a substring test would also
	// match a route that merely appeared in the Referenced list further down.
	declared := map[string]bool{}
	for _, d := range declaredAsText(t, root) {
		declared[d[0]] = true
	}
	absent := []string{
		// Market pull, 9.
		"RouteHqBasicQot", "RouteHqOrderBook", "RouteHqKL", "RouteHqTimeShare",
		"RouteHqTicker", "RouteHqBroker", "RouteHqUsOptionChainCode",
		"RouteHqUsOptionChainExpireDate", "RouteHqUsOverNightTradeCodes",
		// Market subscription, 2.
		"RouteHqSubscribe", "RouteHqUnsubscribe",
		// Trade session, 2.
		"RouteTradeLogin", "RouteTradeLogout",
		// Trade assets / positions, 5.
		"RouteTradeQueryMarginFundInfo", "RouteTradeQueryHoldsList",
		"RouteTradeQueryRealFundJourList", "RouteTradeQueryHistoryFundJourList",
		"RouteHsRateQueryList",
		// Trade orders, 13.
		"RouteTradeEntrust", "RouteTradeCancelEntrust", "RouteTradeBatchCancelEntrust",
		"RouteTradeChangeEntrust", "RouteTradeQueryMaxAvailableAsset",
		"RouteTradeQueryRealEntrustList", "RouteTradeQueryRealDeliverList",
		"RouteTradeQueryRealCondOrderList", "RouteTradeQueryHistoryEntrustList",
		"RouteTradeQueryHistoryDeliverList", "RouteTradeQueryHistoryCondOrderList",
		"RouteTradeQueryMarginFullInfo", "RouteTradeQueryBeforeAndAfterSupport",
		// Trade push subscribe, 2 - the endpoints C15 wired.
		"RouteTradeSubscribe", "RouteTradeUnsubscribe",
		// Algo / strategy, 7.
		"RouteTradeAlgoAddOrder", "RouteTradeAlgoCancelOrder", "RouteTradeAlgoCancelEntrust",
		"RouteTradeAlgoChangeOrder", "RouteTradeAlgoActionOrder",
		"RouteTradeAlgoQueryOrderList", "RouteTradeAlgoQueryEntrustIdList",
		// Futures, 11.
		"RouteTradeFuturesQueryProductInfo", "RouteTradeFuturesQueryMaxBuySellAmount",
		"RouteTradeFuturesQueryFundInfo", "RouteTradeFuturesQueryHoldsList",
		"RouteTradeFuturesEntrust", "RouteTradeFuturesCancelEntrust",
		"RouteTradeFuturesModifyEntrust", "RouteTradeFuturesQueryRealEntrustList",
		"RouteTradeFuturesQueryHistoryEntrustList", "RouteTradeFuturesQueryRealDeliverList",
		"RouteTradeFuturesQueryHistoryDeliverList",
	}
	if len(absent) != 51 {
		t.Fatalf("the absence list has %d entries, not 51; it must cover every declared "+
			"route or a new one is checked by nothing", len(absent))
	}
	for _, name := range absent {
		if !declared[name] {
			t.Errorf("%s is not a declared route constant, so the absence list is stale", name)
		}
		if strings.Contains(out, "      "+name+"\n") {
			t.Errorf("%s is wired but appears in the unwired list:\n%s", name, out)
		}
	}

	// The enforce-mode expectations, against the real tree rather than a synthetic
	// one. This is C14's precondition, so it is proven here rather than on the day
	// the CI job starts failing: a zero gap the enforcing mode rejects, or a PASS
	// line it declines to print, would make the flip impossible and would be
	// discovered by CI rather than by this test.
	out, code = run(root, true)
	if code != 0 {
		t.Fatalf("enforcing mode against the real tree must exit 0 now that the gap is 0, "+
			"got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0 mode=enforce enforce=on") {
		t.Errorf("want the enforcing machine-readable line to report gap=0:\n%s", out)
	}
	if !strings.Contains(out, "PASS: 51/51 endpoints named by a v-next service; 0 NOT implemented") {
		t.Errorf("want the enforcing zero-gap PASS line:\n%s", out)
	}
	if !strings.Contains(out, "PARITY GUARD - ENFORCING MODE") {
		t.Errorf("enforcing mode must name itself in the banner:\n%s", out)
	}
	if strings.Contains(out, "does NOT mean v-next is at parity") {
		t.Errorf("the report-mode disclaimer belongs only to report mode:\n%s", out)
	}
}

// TestReportModeNeverReadsAsSuccess pins the vocabulary rules from the design's
// output requirements. A green exit in report mode is the gate working, not the
// layer being complete, and the words that would suggest otherwise are the whole
// class of misreading this bans.
//
// The vocabulary ban now covers the harder case, which it did not before C15: the
// tree is at zero gap, so the report says "51/51 endpoints named by a service" and
// "0 NOT implemented", and the temptation to read that as a pass is at its
// strongest. The guard must still refuse the words in report mode. The one place
// PASS is allowed is an *enforcing* run that found a zero gap, and
// TestFullParity asserts that it appears there - so the two tests together pin the
// boundary from both sides rather than one of them pinning it.
func TestReportModeNeverReadsAsSuccess(t *testing.T) {
	out, code := run(fixture(t), false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	lines := strings.SplitN(out, "\n", 3)
	if len(lines) < 2 {
		t.Fatalf("want at least two lines of report:\n%s", out)
	}
	if !strings.Contains(lines[0], "REPORT MODE") {
		t.Errorf("the first line must name the mode, so a skimmed log cannot read it as enforcement:\n%s", lines[0])
	}
	if !strings.Contains(lines[1], "NOT") {
		t.Errorf("the second line must state what is missing:\n%s", lines[1])
	}
	if !strings.Contains(out, "A green exit in report mode does NOT mean v-next is at parity.") {
		t.Errorf("want the unconditional report-mode disclaimer:\n%s", out)
	}
	if !strings.Contains(out, "exit=0 (report mode never fails on a gap)") {
		t.Errorf("want the echoed exit code:\n%s", out)
	}
	// The gap is zero, and it is still stated as a NOT-implemented count rather than
	// as a wired count. A reader who saw only "51/51 endpoints named by a service"
	// could not tell a report-mode run from an enforcing one.
	if !strings.Contains(out, "0 NOT implemented") {
		t.Errorf("the gap must be stated as a NOT-implemented count, never as a wired count:\n%s", out)
	}
	if !strings.Contains(out, "51/51 endpoints named by a service; 0 NOT implemented") {
		t.Errorf("want the coverage line to state the full figure and the zero gap:\n%s", out)
	}
	for _, banned := range []string{"PASS", "OK", "SUCCESS", "All endpoints", "all endpoints"} {
		if containsWord(out, banned) {
			t.Errorf("report mode must not use the word %q; it is reserved for an enforcing run that found a zero gap:\n%s", banned, out)
		}
	}
}

// containsWord reports whether s contains word as a standalone word, so a
// substring inside an unrelated identifier cannot trip a vocabulary ban.
func containsWord(s, word string) bool {
	for i := 0; i+len(word) <= len(s); i++ {
		if s[i:i+len(word)] != word {
			continue
		}
		if i > 0 && isWordByte(s[i-1]) {
			continue
		}
		if i+len(word) < len(s) && isWordByte(s[i+len(word)]) {
			continue
		}
		return true
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// TestEnforceModeFailsOnGap is the primary negative control: the guard must be
// shown to fail, or "exit 0" for a zero gap means nothing. V1 alone proves nothing
// on its own.
//
// The gap is built rather than inherited. This case used to read a gap off the real
// tree and had to be re-told the number every time the C-series closed one — it read
// 11, then 4, then 2, and at C15 the tree reached zero and there was nothing left to
// inherit. gappedFixture makes the premise a parameter, so the control survives the
// repository reaching parity, which is the only way a negative control can outlive
// the state it was written against.
func TestEnforceModeFailsOnGap(t *testing.T) {
	const gap = 2
	root := gappedFixture(t, gap)

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode with a non-zero gap must exit 0, got %d:\n%s", code, out)
	}
	requireUnwired(t, out, gap)

	out, code = run(root, true)
	if code != 1 {
		t.Fatalf("enforcing mode with a non-zero gap must exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "PARITY GUARD - ENFORCING MODE") {
		t.Errorf("want the enforcing banner:\n%s", out)
	}
	// The assertion is on the *shape* of the message -- it names the count, so a run
	// cannot fail on a gap it does not say out loud -- and on the count the fixture
	// actually produced, which is the premise of this case rather than a number
	// copied from wherever the tree happened to be.
	if !strings.Contains(out, fmt.Sprintf(
		"ERROR parity: %d declared endpoint(s) have no v-next service method; --enforce requires 0", gap)) {
		t.Errorf("want the fail-on-gap error naming the gap of %d:\n%s", gap, out)
	}
	if !strings.Contains(out, "exit=1 (--enforce requires every declared endpoint to be named by a v-next service)") {
		t.Errorf("want the enforcing exit line:\n%s", out)
	}
	if containsWord(out, "PASS") {
		t.Errorf("a failing enforcing run must never print PASS:\n%s", out)
	}
}

// TestRouteNamedOnlyInATestFile covers V3. A _test.go file that names a route is
// not an implementation, so the exclusion of test files is load-bearing rather
// than cosmetic: a test written before its implementation, or a fixture that names
// every route in order to assert on it, would otherwise be credited.
//
// The fixture is a deliberately gapped one, because the case needs a route that is
// unwired at baseline in order to show it stays unwired when only a test names it.
// While the real tree carried a gap this test could borrow one; now that the tree
// is at parity, borrowing would leave it with no premise at all — and it would have
// degenerated into a t.Skip that reads like a pass. gappedFixture states the gap,
// so the case is exercised whichever way the repository moves.
func TestRouteNamedOnlyInATestFile(t *testing.T) {
	const gap = 1
	base := gappedFixture(t, gap)
	out, code := run(base, false)
	if code != 0 {
		t.Fatalf("baseline run must exit 0, got %d:\n%s", code, out)
	}
	baseline := requireUnwired(t, out, gap)

	var target string
	for _, d := range declaredAsText(t, base) {
		if baseline[d[0]] {
			target = d[0]
			break
		}
	}
	if target == "" {
		t.Fatalf("the gapped fixture left no unwired route to exercise: %v", baseline)
	}

	root := gappedFixture(t, gap)
	writeFile(t, root, "pkg/services/parity_testonly_test.go",
		"package services\n\nimport \"github.com/shing1211/hstongapi4go/client\"\n\n"+
			"func parityTestOnly() client.Route {\n\treturn client."+target+"\n}\n")

	out, code = run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	if after := requireUnwired(t, out, gap); len(after) != len(baseline) {
		t.Errorf("a test-only reference changed the gap from %d to %d: %v -> %v",
			len(baseline), len(after), baseline, after)
	}
	if !strings.Contains(out, "      "+target) {
		t.Errorf("%s is named only in a _test.go file, so it must still be reported unwired:\n%s", target, out)
	}
	if strings.Contains(out, "parity_testonly_test.go:") {
		t.Errorf("a _test.go file must not appear in the scanned reference list:\n%s", out)
	}
}

// TestInlineConversionBlindSpot covers V4. A registered route reachable only as
// a client.Route("/path") conversion is simultaneously implemented and
// uncountable, so the guard would be wrong in both directions at once. The
// secondary diagnostic is what closes it.
func TestInlineConversionBlindSpot(t *testing.T) {
	base := fixture(t)
	out, _ := run(base, false)
	// The real tree is at full parity, which is the condition that makes this case
	// sharper than it was: a conversion of a *wired* route is added to a tree that
	// already credits it, so the count must not move by even one, and the diagnostic
	// has to be what carries the whole finding.
	requireUnwired(t, out, 0)

	target := "RouteHqBasicQot"
	var path string
	for _, d := range declaredAsText(t, base) {
		if d[0] == target {
			path = d[1]
		}
	}
	if path == "" {
		t.Fatalf("%s is not a declared route constant; the fixture is wrong, not the guard", target)
	}

	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_conversion.go",
		"package services\n\nimport \"github.com/shing1211/hstongapi4go/client\"\n\n"+
			"func parityConversion() client.Route {\n\treturn client.Route(\""+path+"\")\n}\n")

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	if !diagnosticAt(out, "WARN parity", "pkg/services/parity_conversion.go", `route "`+path+`" referenced without its named constant`) {
		t.Errorf("want the inline-conversion diagnostic at the conversion site:\n%s", out)
	}
	// The count is unchanged and the target is still absent from the unwired list:
	// the conversion is not credited, and a route the walk already credited is not
	// de-credited either.
	if !strings.Contains(out, "PARITY: 51/51 gap=0") {
		t.Errorf("a conversion must not change the coverage figure:\n%s", out)
	}
	requireUnwired(t, out, 0)
	if strings.Contains(out, "      "+target+"\n") {
		t.Errorf("%s is named by market.go and must not be listed as unwired just because "+
			"a second file reached it by conversion:\n%s", target, out)
	}

	// In enforcing mode the finding is re-labelled ERROR -- and since C14 it is
	// also fatal. What it was *not* is fatal, and this row used to assert exactly
	// that, and it had to be inverted rather than deleted.
	//
	// The design's verification plan calls this "the one class of secondary finding
	// that can make a green run mean something is unproven", and the case asserted
	// exit 1 for a long time -- but it passed for the wrong reason. While the real
	// tree carried a gap of 2, enforcing mode exited 1 because of the *gap*, and
	// the diagnostic contributed nothing; the assertion looked like it was covering
	// the diagnostic and was not. C15 corrected the row to measure what the code
	// actually did (exit 0, because report() set the code from the problem count
	// and the gap only) and recorded that the row is what C14 should decide about.
	//
	// C14 decided: an enforcing run that leaves a route reference unattributable
	// must fail, because a diagnostic is a statement that the walk did not account
	// for something, and a run that cannot say what it did not see is not a
	// measurement. The run below is at a zero gap, so the gap contributes nothing
	// to its exit code and the diagnostic is the only thing that can.
	//
	// The PASS line is the other half. It is keyed on the exit code rather than on
	// the gap, so this run -- a zero gap that nonetheless fails -- prints no PASS.
	// Both facts are asserted so a change to either is a failing test rather than a
	// surprise in a CI log.
	out, code = run(root, true)
	if code != 1 {
		t.Errorf("enforcing mode with a zero gap and one outstanding diagnostic exits %d, want 1: "+
			"a secondary diagnostic is a statement that the walk could not account for a route "+
			"reference, and C14 made it fatal in this mode. If this ever starts failing, the "+
			"exit contract in report() has changed:\n%s", code, out)
	}
	if !diagnosticAt(out, "ERROR parity", "pkg/services/parity_conversion.go", `referenced without its named constant`) {
		t.Errorf("want the conversion promoted to an ERROR-labelled line in enforcing mode:\n%s", out)
	}
	if !strings.Contains(out, "exit=1 (a secondary diagnostic is unresolved") {
		t.Errorf("want the exit line to name the diagnostic as the reason, so a red CI log is diagnosable:\n%s", out)
	}
	if containsWord(out, "PASS") {
		t.Errorf("a failing enforcing run must never print PASS, and the gap is 0 here, so "+
			"the diagnostic is the only thing that can have failed it:\n%s", out)
	}
}

// diagnosticAt reports whether the report carries a finding of the given verb and
// file whose text contains want. The line number is matched loosely so a test
// about the finding's substance does not break when the fixture's line count
// changes.
func diagnosticAt(out, verb, rel, want string) bool {
	prefix := "  " + verb + ": " + rel + ":"
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, want) {
			return true
		}
	}
	return false
}

// findings returns every WARN/ERROR line a report carries, whether a secondary
// diagnostic or an integrity problem, so a case that needs "this run had nothing
// to report beyond its numbers" can say so in one assertion rather than by
// matching prefixes inline. Both prefixes are collected because a narrowing that
// turned a diagnostic into a problem would otherwise read as the same silence.
func findings(out string) []string {
	var got []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "WARN parity:") || strings.HasPrefix(trimmed, "ERROR parity:") {
			got = append(got, trimmed)
		}
	}
	return got
}

// TestRouteTypedParameterIsNotADiagnostic pins the half of the client.Route
// field check that C14 narrowed away, and it is pinned on the real tree as well
// as on a fixture.
//
// A `client.Route`-typed parameter is a declaration of what a function accepts.
// It is not a reference to a route, and it cannot become one: a value reaches a
// parameter only at a call site, and a call site inside the scan roots is walked
// like any other code. The standing example is Executor.Do's own signature at
// pkg/services/executor.go:36 -- the seam every v-next call passes through, with
// all 51 routes credited at the call sites on the far side of it.
//
// This mattered because C14 made a secondary diagnostic fatal in enforcing mode.
// With the parameter half still reported, the real tree could never have had a
// green enforcing run: the diagnostic was a permanent false positive, and
// Executor is correct -- client.Client must keep satisfying it -- so "deal with
// the parameter" was not an available fix. The two halves are therefore proved in
// opposite directions by TestRouteTypedParameterIsNotADiagnostic and
// TestRouteTypedStructFieldIsADiagnostic, and neither can be re-widened without
// turning the other red.
func TestRouteTypedParameterIsNotADiagnostic(t *testing.T) {
	// Every shape a signature can take. An interface method's `route client.Route`
	// is the one most likely to leak back in: its *ast.Field has no names and its
	// type is an *ast.FuncType, so a walk keyed on the field's own type would see
	// nothing and one keyed on the field itself would see the parameter. The
	// `codec client.Codec` result is the over-widening control: a check that
	// reported any client-qualified field rather than the route type itself would
	// flag it.
	const body = "package services\n\n" +
		"import \"github.com/shing1211/hstongapi4go/client\"\n\n" +
		"type parityIface interface {\n\tDo(route client.Route) error\n}\n\n" +
		"type parityHolder struct{}\n\n" +
		"func (parityHolder) M(route client.Route) client.Route { return route }\n\n" +
		"func parityFn(route client.Route, codec client.Codec) client.Route { return route }\n\n" +
		"var parityLit = func(route client.Route) client.Route { return route }\n\n" +
		"var _ parityIface = parityHolder{}\n"

	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_parameter.go", body)

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	if got := findings(out); len(got) != 0 {
		t.Errorf("a client.Route-typed parameter is a signature, not a reference, so it must "+
			"not be reported; got %d finding(s):\n%s\n%s", len(got), strings.Join(got, "\n"), out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0") {
		t.Errorf("the fixture is the real tree plus an unreported signature, so coverage must "+
			"be unchanged:\n%s", out)
	}

	// The real tree, in the mode CI now runs. This is the assertion that would
	// have reddened the job on the day of the flip if the narrowing were reverted,
	// and it names the file so a re-widened check is a failing test rather than a
	// red log somebody has to diagnose.
	for _, enforce := range []bool{false, true} {
		out, code = run(repoRoot(t), enforce)
		got := findings(out)
		if len(got) != 0 {
			t.Errorf("enforce=%v: the real tree must carry no findings at all, got %d:\n%s\n%s",
				enforce, len(got), strings.Join(got, "\n"), out)
		}
		for _, f := range got {
			if strings.Contains(f, "executor.go") {
				t.Errorf("enforce=%v: Executor.Do's client.Route-typed parameter at "+
					"pkg/services/executor.go:36 is not a blind spot and must never be reported: %s",
					enforce, f)
			}
		}
		if code != 0 {
			t.Errorf("enforce=%v: the real tree must exit 0, got %d:\n%s", enforce, code, out)
		}
	}
}

// TestRouteTypedStructFieldIsADiagnostic is the half that survived the narrowing,
// and it is the direction that keeps enforcing mode honest: a route stored in a
// field is written somewhere the walk may never reach, so it can be neither
// credited nor withdrawn by name, and an enforcing run that cannot tell must fail.
//
// Every assertion here is about a fixture. Proving this on the real tree would
// mean putting a client.Route-typed field into pkg/services, which is the thing
// that must never ship, so the case builds a throwaway tree instead -- the same
// discipline the design's verification plan states, and the reason the guard's
// mutation table can outlive any single state of the repository.
func TestRouteTypedStructFieldIsADiagnostic(t *testing.T) {
	// A named field and an anonymous inline struct literal's field type, so both
	// spellings of a struct field are covered. The interface method below is the
	// control from the other test: same file, same type, parameter position, and
	// it must not be reported.
	const body = "package services\n\n" +
		"import \"github.com/shing1211/hstongapi4go/client\"\n\n" +
		"type parityStored struct {\n\troute client.Route\n\tname  string\n}\n\n" +
		"type parityIface interface {\n\tDo(route client.Route) error\n}\n\n" +
		"var _ = parityStored{}\n\n" +
		"var _ = struct{ route client.Route }{}\n\n" +
		"func parityUse(c client.Route) client.Route {\n\treturn c\n}\n"

	build := func(t *testing.T) string {
		t.Helper()
		root := fixture(t)
		writeFile(t, root, "pkg/services/parity_stored.go", body)
		return root
	}

	// Report mode: reported, not fatal. The two exit codes are different outcomes
	// and the design's report-mode contract covers only one of them, so this is
	// stated rather than assumed.
	root := build(t)
	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode never fails on a diagnostic, got %d:\n%s", code, out)
	}
	if !diagnosticAt(out, "WARN parity", "pkg/services/parity_stored.go", `a client.Route-typed struct field`) {
		t.Errorf("want the stored-route diagnostic in report mode:\n%s", out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0") {
		t.Errorf("a stored route must not move the coverage figure, and this tree is the real one:\n%s", out)
	}

	// Enforcing mode: the same finding, fatal. The gap is 0, so nothing in the
	// coverage figure can be what failed this run.
	root = build(t)
	out, code = run(root, true)
	if code != 1 {
		t.Fatalf("enforcing mode with an unresolved stored-route diagnostic must exit 1, got %d:\n%s", code, out)
	}
	if !diagnosticAt(out, "ERROR parity", "pkg/services/parity_stored.go", `a client.Route-typed struct field`) {
		t.Errorf("want the finding promoted to an ERROR-labelled line in enforcing mode:\n%s", out)
	}
	if !strings.Contains(out, "exit=1 (a secondary diagnostic is unresolved") {
		t.Errorf("want the exit line to name the diagnostic as the reason:\n%s", out)
	}
	if containsWord(out, "PASS") {
		t.Errorf("a zero-gap run that fails on a diagnostic must not print PASS:\n%s", out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0 mode=enforce enforce=on") {
		t.Errorf("the measurement itself is still valid and must still be printed:\n%s", out)
	}
}

// TestEnforcingModeWithABrokenInvariantPrintsNoPass covers the third way an
// enforcing run can fail, which nothing else in this suite reaches: a run whose
// inputs it could not read.
//
// open and wired are both 0 in that state, so a PASS line keyed on the gap
// renders as "PASS: 0/0 endpoints named by a v-next service" on a run that
// measured nothing at all -- the same ERROR/PASS contradiction C14 ends, reached
// through a different door. Keying PASS on the exit code closes both.
func TestEnforcingModeWithABrokenInvariantPrintsNoPass(t *testing.T) {
	root := fixture(t)
	spec := readFile(t, root, specFile)
	mutated := strings.Replace(spec, "**Total: 51 HTTP endpoints**", "**Total: 52 HTTP endpoints**", 1)
	if mutated == spec {
		t.Fatal("the fixture SPEC does not carry the Total line this test mutates")
	}
	writeFile(t, root, specFile, mutated)

	out, code := run(root, true)
	if code != 1 {
		t.Fatalf("enforcing mode must exit 1 on a broken invariant, got %d:\n%s", code, out)
	}
	if containsWord(out, "PASS") {
		t.Errorf("a run that could not measure must not print PASS, and \"PASS: 0/52\" is exactly "+
			"what it used to print here -- SPEC's total is readable, so the run returns before it "+
			"can be compared and open is 0:\n%s", out)
	}
	if !strings.Contains(out, "NOT MEASURED") {
		t.Errorf("want coverage reported as not measured:\n%s", out)
	}
	if !strings.Contains(out, "exit=1 (an integrity check failed") {
		t.Errorf("want the exit line to name the integrity failure:\n%s", out)
	}
}

// TestRoutePathLiteralIsReported covers the second secondary diagnostic: a
// service that builds the path itself and never mentions the constant is never
// credited, and the guard says so instead of silently reporting a gap.
func TestRoutePathLiteralIsReported(t *testing.T) {
	base := fixture(t)
	var path string
	for _, d := range declaredAsText(t, base) {
		if d[0] == "RouteHqTimeShare" {
			path = d[1]
		}
	}
	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_literal.go",
		"package services\n\nfunc parityLiteral() string {\n\treturn \""+path+"Request\"\n}\n")

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	if !diagnosticAt(out, "WARN parity", "pkg/services/parity_literal.go",
		`string literal "`+path+`Request" is the declared route`) {
		t.Errorf("want the path-literal diagnostic, alias form included:\n%s", out)
	}
}

// TestSPECCodeCountMismatch covers V5, and it is the control for the
// report/enforce boundary: report mode is not "never fails", it is "never fails
// on a gap". A SPEC that disagrees with the const block is a broken invariant
// and must redden CI before C14.
func TestSPECCodeCountMismatch(t *testing.T) {
	root := fixture(t)
	spec := readFile(t, root, specFile)
	mutated := strings.Replace(spec, "**Total: 51 HTTP endpoints**", "**Total: 52 HTTP endpoints**", 1)
	if mutated == spec {
		t.Fatal("the fixture SPEC does not carry the Total line this test mutates")
	}
	writeFile(t, root, specFile, mutated)

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("report mode must exit 1 on a SPEC/code count mismatch, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ERROR parity: "+specFile+":122 declares 52 HTTP endpoints; the "+routesFile+" const block declares 51") {
		t.Errorf("want the count-mismatch error naming both files:\n%s", out)
	}
	if !strings.Contains(out, "NOT MEASURED") {
		t.Errorf("a run that could not measure coverage must say so rather than print a count:\n%s", out)
	}
}

// TestGroupBreakdownMismatch covers V6. SPEC states the count three ways, so a
// hand-edited count is a coordinated edit rather than a one-line one.
func TestGroupBreakdownMismatch(t *testing.T) {
	root := fixture(t)
	spec := readFile(t, root, specFile)
	mutated := strings.Replace(spec, "+ 7 + 11)", "+ 7 + 10)", 1)
	if mutated == spec {
		t.Fatal("the fixture SPEC does not carry the breakdown this test mutates")
	}
	writeFile(t, root, specFile, mutated)

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("report mode must exit 1 on a breakdown mismatch, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ERROR parity: "+specFile+" group headers sum to 51 but line 122 breaks down as 50") {
		t.Errorf("want the group/breakdown mismatch error:\n%s", out)
	}
}

// TestConstMissingFromRegistry covers V7, the control for the registry check. A
// constant that is not in canonicalRoutes is a route client.Client.Do refuses at
// run time, and no test in the repository covers it: client/routes_test.go
// iterates the registry, so a constant omitted from it is invisible.
func TestConstMissingFromRegistry(t *testing.T) {
	root := fixture(t)
	routes := readFile(t, root, routesFile)
	mutated := strings.Replace(routes, "\tRouteHqBasicQot:                {},\n", "", 1)
	if mutated == routes {
		t.Fatal("the fixture route table does not carry the registry entry this test removes")
	}
	writeFile(t, root, routesFile, mutated)

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("report mode must exit 1 when a constant is missing from canonicalRoutes, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "is not in canonicalRoutes") {
		t.Errorf("want the registry error:\n%s", out)
	}
	if !strings.Contains(out, "errors.Is(route.Validate(), client.ErrUnknownRoute)") {
		t.Errorf("want the error to name the run-time consequence:\n%s", out)
	}
}

// TestRegistryEntryWithoutConst covers V8: a registry path with no constant can
// only be reached by conversion, so no service can name it and it would be
// permanently unwired.
func TestRegistryEntryWithoutConst(t *testing.T) {
	root := fixture(t)
	routes := readFile(t, root, routesFile)
	mutated := strings.Replace(routes, "var canonicalRoutes = map[Route]struct{}{\n",
		"var canonicalRoutes = map[Route]struct{}{\n\tRoute(\"/trade/Nope\"): {},\n", 1)
	if mutated == routes {
		t.Fatal("the fixture route table does not carry the registry literal this test extends")
	}
	writeFile(t, root, routesFile, mutated)

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("report mode must exit 1 for a registry entry with no constant, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, `ERROR parity: canonicalRoutes holds "/trade/Nope" with no constant`) {
		t.Errorf("want the registry-entry error:\n%s", out)
	}
}

// TestNoTotalLine covers V9. A parser that matches nothing must fail; it must
// never return an empty set that reads as "0 endpoints, all clear".
func TestNoTotalLine(t *testing.T) {
	root := fixture(t)
	spec := readFile(t, root, specFile)
	var kept []string
	for _, line := range strings.Split(spec, "\n") {
		if strings.HasPrefix(strings.TrimSuffix(line, "\r"), "**Total:") {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == len(strings.Split(spec, "\n")) {
		t.Fatal("the fixture SPEC does not carry the Total line this test removes")
	}
	writeFile(t, root, specFile, strings.Join(kept, "\n"))

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("report mode must exit 1 when the Total line is gone, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, `ERROR parity: `+specFile+` declares no "**Total: N HTTP endpoints**" line`) {
		t.Errorf("want the missing-Total error:\n%s", out)
	}
	if !strings.Contains(out, "NOT MEASURED") {
		t.Errorf("want coverage reported as not measured:\n%s", out)
	}
	// The banned phrases are checked against the report's own claims, not its
	// explanation of what it refuses to claim: the hint on this error quotes the
	// very phrase it must never assert.
	head := strings.SplitN(out, "\n", 3)
	if len(head) < 2 {
		t.Fatalf("want at least two lines of report:\n%s", out)
	}
	for _, banned := range []string{"0 endpoints", "all clear", "All endpoints", "gap=0"} {
		if strings.Contains(head[1], banned) {
			t.Errorf("the coverage line must not read as %q:\n%s", banned, out)
		}
	}
}

// TestCRLFAndLFProduceIdenticalOutput covers V10. docs/SPEC.md is CRLF in this
// working tree and LF on the runner, and it is the one file the guard parses as
// text, so a `$`-anchored pattern or a naive split would fail on one and pass on
// the other for a reason that has nothing to do with the code.
func TestCRLFAndLFProduceIdenticalOutput(t *testing.T) {
	root := fixture(t)
	spec := readFile(t, root, specFile)
	normalized := strings.ReplaceAll(spec, "\r\n", "\n")
	if !strings.Contains(spec, "\r\n") {
		t.Skip("this working tree already holds SPEC.md with LF endings; the CRLF side of the pair is covered by the same code path")
	}

	writeFile(t, root, specFile, normalized)
	lfOut, lfCode := run(root, false)

	writeFile(t, root, specFile, strings.ReplaceAll(normalized, "\n", "\r\n"))
	crlfOut, crlfCode := run(root, false)

	if lfCode != crlfCode {
		t.Errorf("CRLF and LF checkouts disagree on the exit code: %d vs %d", lfCode, crlfCode)
	}
	if lfCode != 0 {
		t.Fatalf("the LF fixture must exit 0, got %d:\n%s", lfCode, lfOut)
	}
	if lfOut != crlfOut {
		t.Errorf("CRLF and LF checkouts must produce byte-identical reports.\n--- LF ---\n%s\n--- CRLF ---\n%s", lfOut, crlfOut)
	}
	// The figure is stated so the comparison is anchored: a parser that silently
	// lost the routes on one line ending would produce two different reports and be
	// caught by the byte-equality check above, but two identical *wrong* reports
	// would not be caught by it alone.
	if !strings.Contains(crlfOut, "PARITY: 51/51 gap=0 mode=report enforce=off") {
		t.Errorf("the CRLF fixture must still reach 51/51 gap=0:\n%s", crlfOut)
	}
}

// TestShadowedClientIdentifier covers V11. A `client` that is not the module's
// client package makes every `client.RouteXxx` in the file ambiguous, and the
// walk would under-count silently. The guard must refuse to pass rather than
// report a smaller gap.
func TestShadowedClientIdentifier(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		kind string
	}{
		{"local variable", "func parityShadow() int {\n\tclient := 1\n\t_ = client\n\treturn client\n}\n", "local variable"},
		{"parameter", "func parityShadow(client int) int {\n\treturn client\n}\n", "parameter"},
		{"receiver", "type parityT struct{}\n\nfunc (client parityT) m() int {\n\treturn 1\n}\n", "receiver"},
		{"package-level var", "var client = 1\n", "const/var"},
		{"foreign import", "import client \"fmt\"\n\nvar _ = client.Sprintf\n", "import of \"fmt\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			writeFile(t, root, "pkg/services/parity_shadow.go", "package services\n\n"+tc.body)

			out, code := run(root, false)
			if code != 1 {
				t.Fatalf("a shadowed client identifier must exit 1, got %d:\n%s", code, out)
			}
			if !strings.Contains(out, `declares identifier "client" (`+tc.kind+`); the reference walk would under-count`) {
				t.Errorf("want the shadowing error naming the kind:\n%s", out)
			}
			if strings.Contains(out, "PARITY:") {
				t.Errorf("a failed integrity check must not print a machine-readable parity line, which would be read as a measurement:\n%s", out)
			}
		})
	}
}

// TestStructFieldNamedClientIsNotAnError pins the one binding that looks like
// shadowing and is not. A struct field is reached through its owner's selector
// (s.client), never as a bare identifier, so it cannot shadow the package name.
// pkg/services has three such fields today (AccountService.client,
// MarketService.client, TradingService.client), so a check that flagged them
// would make the guard unable to run against the real repository at all.
func TestStructFieldNamedClientIsNotAnError(t *testing.T) {
	out, code := run(repoRoot(t), false)
	if code != 0 {
		t.Fatalf("a struct field named client must not trip the shadow check, got %d:\n%s", code, out)
	}
	if strings.Contains(out, "the reference walk would under-count") {
		t.Errorf("no scanned file in the real tree shadows the client package:\n%s", out)
	}
}

// TestRangeAndTypeSwitchBindingsAreChecked covers the remaining binding forms a
// `client := 1` check would miss. A range clause or a type-switch binding is just
// as much a shadow as an assignment.
func TestRangeAndTypeSwitchBindingsAreChecked(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_shadow.go",
		"package services\n\nfunc parityRange() int {\n\tfor client := range []int{1} {\n\t\t_ = client\n\t}\n\tswitch client := interface{}(1).(type) {\n\tcase int:\n\t\treturn client\n\t}\n\treturn 0\n}\n")

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("a range or type-switch binding named client must exit 1, got %d:\n%s", code, out)
	}
	for _, kind := range []string{"range variable", "type switch variable"} {
		if !strings.Contains(out, `(`+kind+`)`) {
			t.Errorf("want the %s binding reported:\n%s", kind, out)
		}
	}
}

// TestAliasedClientImportIsNotCounted covers the mirror of the shadow check: the
// module's client package imported under a different name is a miscount, because
// a route referenced through that alias is invisible to the walk.
func TestAliasedClientImportIsNotCounted(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_alias.go",
		"package services\n\nimport hq \"github.com/shing1211/hstongapi4go/client\"\n\n"+
			"func parityAlias() hq.Route {\n\treturn hq.RouteHqBasicQot\n}\n")

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("an aliased import is a warning, not a broken invariant, got %d:\n%s", code, out)
	}
	if !diagnosticAt(out, "WARN parity", "pkg/services/parity_alias.go",
		`imports the client package as "hq"; route references through that alias are not counted`) {
		t.Errorf("want the alias diagnostic:\n%s", out)
	}
	// RouteHqBasicQot is still referenced by market.go, so the count must not
	// move: the diagnostic is about the alias being uncountable, not about a
	// route having been lost.
	if !strings.Contains(out, "PARITY: 51/51 gap=0") {
		t.Errorf("an aliased reference must not change the coverage figure:\n%s", out)
	}
}

// TestMissingScanRoot covers V12. A renamed directory must not read as "nothing
// referenced" and print a clean bill of health.
func TestMissingScanRoot(t *testing.T) {
	root := fixture(t)
	if err := os.RemoveAll(filepath.Join(root, "pkg", "services")); err != nil {
		t.Fatalf("removing pkg/services: %v", err)
	}

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("a missing scan root must exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ERROR parity: scan root pkg/services does not exist") {
		t.Errorf("want the missing-root error:\n%s", out)
	}
	for _, banned := range []string{"gap=0", "0 NOT implemented", "PARITY:"} {
		if strings.Contains(out, banned) {
			t.Errorf("a missing root must not produce the measurement %q:\n%s", banned, out)
		}
	}
}

// TestEmptyScanFailsToPass is the vacuity guard. A walk that inspects nothing
// reports a zero gap, which is exactly what full parity looks like, so the guard
// has to distinguish them -- the same reason internal/layering refuses to pass
// when its import walk finds nothing.
func TestEmptyScanFailsToPass(t *testing.T) {
	root := fixture(t)
	for _, dir := range scannedDirs {
		base := filepath.Join(root, filepath.FromSlash(dir))
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatalf("readdir %s: %v", base, err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".go") {
				if err := os.Remove(filepath.Join(base, e.Name())); err != nil {
					t.Fatalf("remove %s: %v", e.Name(), err)
				}
			}
		}
	}

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("an empty scan must exit 1, not report a clean bill of health, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "no non-test .go file found in any scan root") {
		t.Errorf("want the empty-scan error:\n%s", out)
	}
	if !strings.Contains(out, "no `client.RouteXxx` reference found in any scanned non-test file") {
		t.Errorf("want the vacuity error:\n%s", out)
	}
	for _, banned := range []string{"gap=0", "PASS", "PARITY:", "all endpoints"} {
		if containsWord(out, banned) {
			t.Errorf("a guard that scanned nothing must not print %q:\n%s", banned, out)
		}
	}
}

// TestMissingRouteFile covers the root-resolution contract: a root that does not
// hold client/routes.go is not a repository, and guessing would produce a wrong
// report rather than a failure.
func TestMissingRouteFile(t *testing.T) {
	out, code := run(t.TempDir(), false)
	if code != 1 {
		t.Fatalf("a root with no route table must exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "cannot locate the repository root") {
		t.Errorf("want the root-resolution error:\n%s", out)
	}
	if !strings.Contains(out, "a wrong root produces a wrong report, not a failure") {
		t.Errorf("want the hint explaining why the guard refuses to guess:\n%s", out)
	}
}

// TestMissingSPEC covers the other half of rule 5: the count's only source.
func TestMissingSPEC(t *testing.T) {
	root := fixture(t)
	if err := os.Remove(filepath.Join(root, "docs", "SPEC.md")); err != nil {
		t.Fatalf("removing SPEC.md: %v", err)
	}
	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("a missing SPEC must exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ERROR parity: "+specFile+" does not exist") {
		t.Errorf("want the missing-SPEC error:\n%s", out)
	}
}

// TestPositionalJoinDisagreementSuppressesGrouping covers the one condition that
// withholds output rather than failing. The SPEC rows and the const block are
// joined by position, so a path disagreement means the two orderings have
// diverged; printing a confidently wrong grouping would be worse than printing
// none, while the gap count stays valid.
func TestPositionalJoinDisagreementSuppressesGrouping(t *testing.T) {
	root := fixture(t)
	lines := strings.Split(readFile(t, root, specFile), "\n")
	// Swap the FuturesEntrust and FuturesCancelEntrust rows, keeping the row
	// numbers, so only the order diverges.
	first, second := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSuffix(line, "\r")
		if strings.Contains(trimmed, "POST http://127.0.0.1:11111/trade/FuturesEntrust`") {
			first = i
		}
		if strings.Contains(trimmed, "POST http://127.0.0.1:11111/trade/FuturesCancelEntrust`") {
			second = i
		}
	}
	if first < 0 || second < 0 {
		t.Fatal("the fixture SPEC does not carry the futures rows this test swaps")
	}
	lines[first], lines[second] = lines[second], lines[first]
	writeFile(t, root, specFile, strings.Join(lines, "\n"))

	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("a diverged ordering is not a broken invariant, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Group breakdown SUPPRESSED") {
		t.Errorf("want the group rendering suppressed:\n%s", out)
	}
	if strings.Contains(out, "  Group    ") {
		t.Errorf("no group table may be printed when the join disagreed:\n%s", out)
	}
	if !strings.Contains(out, "PARITY: 51/51 gap=0") {
		t.Errorf("the gap count does not depend on grouping and must survive:\n%s", out)
	}
}

// TestNoGroupRowsSilentlyDropped guards the one place a group header could be
// lost: a header whose title does not end in a count. That must be an error, not
// a group with a zero count, which would make the header sum disagree and take
// the whole report down on a cosmetic reword.
func TestNoGroupRowsSilentlyDropped(t *testing.T) {
	root := fixture(t)
	spec := readFile(t, root, specFile)
	mutated := strings.Replace(spec, "### 2.8 Futures — 11", "### 2.8 Futures, many", 1)
	if mutated == spec {
		t.Skip("this working tree's SPEC header shape differs from the fixture this test mutates")
	}
	writeFile(t, root, specFile, mutated)

	out, code := run(root, false)
	if code != 1 {
		t.Fatalf("a group header with no count must exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "does not end in a declared count") {
		t.Errorf("want the header-shape error:\n%s", out)
	}
}

// groupRowRE matches one row of the group table: a title and three counts. The
// table is parsed rather than string-matched so a column width change does not
// fail a test about the numbers.
var groupRowRE = regexp.MustCompile(`^  (.+?) +([0-9]+) +([0-9]+) +([0-9]+)$`)

// groupRows parses the declared/wired/gap columns of the group table out of a
// report, keyed by group title plus the TOTAL row.
func groupRows(t *testing.T, out string) map[string][3]int {
	t.Helper()
	rows := map[string][3]int{}
	for _, line := range strings.Split(out, "\n") {
		m := groupRowRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		nums := [3]int{}
		for i := 0; i < 3; i++ {
			v, err := strconv.Atoi(m[i+2])
			if err != nil {
				t.Fatalf("unparsable count %q: %v", m[i+2], err)
			}
			nums[i] = v
		}
		rows[strings.TrimSpace(m[1])] = nums
	}
	if len(rows) == 0 {
		t.Fatalf("no group rows parsed from the report:\n%s", out)
	}
	return rows
}

// unwiredNames parses the constant names out of a report's unwired section, so
// assertions about the gap can compare sets instead of counting substrings.
//
// An empty map is a legitimate result rather than a broken parse: the real tree
// reached full parity at C15, so "no unwired routes" is now a fact this guard
// reports and callers must be able to see. A case that needs a known non-empty
// gap uses requireUnwired, which states the count it depends on, rather than
// relying on this function to fail when the set comes back empty.
func unwiredNames(t *testing.T, out string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	in := false
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Unwired ("):
			in = true
		case in && strings.HasPrefix(trimmed, "Repo root:"):
			in = false
		case in && strings.HasPrefix(line, "      Route"):
			names[strings.Fields(trimmed)[0]] = true
		}
	}
	return names
}

// requireUnwired asserts the report's unwired section names exactly want routes,
// so a case that needs a known gap states it at the top rather than inheriting
// whatever the repository happens to have. It returns the names so the caller can
// assert which ones they are.
func requireUnwired(t *testing.T, out string, want int) map[string]bool {
	t.Helper()
	names := unwiredNames(t, out)
	if len(names) != want {
		t.Fatalf("the report names %d unwired route(s), want %d: %v\n%s", len(names), want, names, out)
	}
	return names
}
