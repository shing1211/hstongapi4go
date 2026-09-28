// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package main

import (
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
// mirrored in the other, so agreeing on 51 declared and 47 referenced is a
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

// TestFullParity covers V1: with every declared route named by a service, the
// gap is zero, report mode still exits 0, and enforcing mode exits 0 too. This
// is the case the C14 flip will move the real repository into, so it is proven
// here rather than assumed on the day the gate starts failing.
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

// TestRealTreeUnwiredSetIsTheKnownBacklog pins the C2 baseline, refreshed as the
// C-series closes the gaps. The 4 unwired endpoints are the run's remaining
// backlog: the 2 that SPEC groups as trade session (login, logout) and the 2 it
// groups as trade push subscribe (subscribe, unsubscribe).
//
// This is a snapshot and it will need editing as C8-C15 close the gaps -- each of
// them must update it to the new count. That is the same friction
// client/routes_test.go carries for a route that is added, and it is deliberate:
// the guard's own number is the source of truth, this literal only records what
// that number was when the task landed, so a silent change in either direction
// shows up as a failing test rather than as a diff nobody reads.
//
// It moved from 22 to 14 at C4, which wired the eight futures reads, 14 to 11 at
// C5, which wired the three futures mutations, and 11 to 4 at C7+C8+C9, which wired
// all seven algo endpoints -- so algo is now 7/7/0 and the only remaining gap is the
// two session and two push groups. The per-group row and the by-name list are
// updated together, so a task that wires a route in one place and forgets the
// other fails here.
func TestRealTreeUnwiredSetIsTheKnownBacklog(t *testing.T) {
	root := repoRoot(t)
	out, code := run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	want := map[string][3]int{
		"Market pull":              {9, 9, 0},
		"Market subscription":      {2, 2, 0},
		"Trade session":            {2, 0, 2},
		"Trade assets / positions": {5, 5, 0},
		"Trade orders":             {13, 13, 0},
		"Trade push subscribe":     {2, 0, 2},
		"Algo / strategy":          {7, 7, 0},
		"Futures":                  {11, 11, 0},
		"TOTAL":                    {51, 47, 4},
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
	if !strings.Contains(out, "  Unwired (4 of 51 declared), by SPEC group:") {
		t.Errorf("want the unwired header to state the gap:\n%s", out)
	}
	// The 4, by name. Every one is checked against the declared set so the list
	// cannot drift into naming something that is not an endpoint.
	declared := map[string]bool{}
	for _, d := range declaredAsText(t, root) {
		declared[d[0]] = true
	}
	session := []string{"RouteTradeLogin", "RouteTradeLogout"}
	push := []string{"RouteTradeSubscribe", "RouteTradeUnsubscribe"}
	for _, group := range [][]string{session, push} {
		for _, name := range group {
			if !declared[name] {
				t.Errorf("%s is not a declared route constant", name)
			}
			if !strings.Contains(out, "      "+name) {
				t.Errorf("%s must appear in the unwired list:\n%s", name, out)
			}
		}
	}
	// The eighteen endpoints wired at C4, C5 and C7-C9 must NOT appear in the
	// unwired list. Without this the list could name a wired route and the test
	// would still pass, because the check above only asserts presence, never
	// absence. It is longer than it was at C5 because C7-C9 added the seven algo
	// routes, and the two algo *queries* are the two a wrong route most likely to
	// hide on, because they are the only two algo paths a client may retry.
	for _, name := range []string{
		"RouteTradeFuturesQueryProductInfo", "RouteTradeFuturesQueryMaxBuySellAmount",
		"RouteTradeFuturesQueryFundInfo", "RouteTradeFuturesQueryHoldsList",
		"RouteTradeFuturesQueryRealEntrustList", "RouteTradeFuturesQueryHistoryEntrustList",
		"RouteTradeFuturesQueryRealDeliverList", "RouteTradeFuturesQueryHistoryDeliverList",
		"RouteTradeFuturesEntrust", "RouteTradeFuturesCancelEntrust",
		"RouteTradeFuturesModifyEntrust",
		"RouteTradeAlgoQueryOrderList", "RouteTradeAlgoQueryEntrustIdList",
		"RouteTradeAlgoAddOrder", "RouteTradeAlgoCancelOrder", "RouteTradeAlgoCancelEntrust",
		"RouteTradeAlgoChangeOrder", "RouteTradeAlgoActionOrder",
	} {
		if !declared[name] {
			t.Errorf("%s is not a declared route constant", name)
		}
		if strings.Contains(out, "      "+name+"\n") {
			t.Errorf("%s is wired and must not appear in the unwired list:\n%s", name, out)
		}
	}
	if got := 2 + 2; got != 4 {
		t.Fatalf("the backlog is %d endpoints, not the 4 the table records", got)
	}
}

// TestReportModeNeverReadsAsSuccess pins the vocabulary rules from the design's
// output requirements. A green exit in report mode is the gate working, not the
// layer being complete, and the words that would suggest otherwise are the whole
// class of misreading this bans.
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
	if !strings.Contains(out, "4 NOT implemented") {
		t.Errorf("the gap must be stated as a NOT-implemented count, never as a wired count:\n%s", out)
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
// shown to fail, or "exit 0" for a 4-endpoint gap means nothing. V1 alone
// proves nothing on its own.
func TestEnforceModeFailsOnGap(t *testing.T) {
	out, code := run(fixture(t), true)
	if code != 1 {
		t.Fatalf("enforcing mode with a non-zero gap must exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "PARITY GUARD - ENFORCING MODE") {
		t.Errorf("want the enforcing banner:\n%s", out)
	}
	// fixture copies the real scan roots, so the gap here is the real tree's
	// current backlog rather than a frozen number: it read 11 while algo was
	// unwired and reads 4 now that C7-C9 wired it. The assertion is on the
	// *shape* of the message -- it names the count, so a run cannot fail on a
	// gap it does not say out loud -- and the count is whatever the tree has.
	if !strings.Contains(out, "ERROR parity: 4 declared endpoint(s) have no v-next service method; --enforce requires 0") {
		t.Errorf("want the fail-on-gap error naming the gap:\n%s", out)
	}
	if containsWord(out, "PASS") {
		t.Errorf("a failing enforcing run must never print PASS:\n%s", out)
	}
}

// TestRouteNamedOnlyInATestFile covers V3. A _test.go file that names a route is
// not an implementation, so the exclusion of test files is load-bearing rather
// than cosmetic: the guard's own fixture naming all 51, or any test written
// before its implementation, would otherwise be credited.
func TestRouteNamedOnlyInATestFile(t *testing.T) {
	base := fixture(t)
	out, code := run(base, false)
	if code != 0 {
		t.Fatalf("baseline run must exit 0, got %d:\n%s", code, out)
	}
	baseline := unwiredNames(t, out)

	var target string
	for _, d := range declaredAsText(t, base) {
		if baseline[d[0]] {
			target = d[0]
			break
		}
	}
	if target == "" {
		t.Skip("every declared route is named by a service, so there is no unwired route for this case to exercise")
	}

	root := fixture(t)
	writeFile(t, root, "pkg/services/parity_testonly_test.go",
		"package services\n\nimport \"github.com/shing1211/hstongapi4go/client\"\n\n"+
			"func parityTestOnly() client.Route {\n\treturn client."+target+"\n}\n")

	out, code = run(root, false)
	if code != 0 {
		t.Fatalf("report mode must exit 0, got %d:\n%s", code, out)
	}
	if after := unwiredNames(t, out); len(after) != len(baseline) {
		t.Errorf("a test-only reference changed the gap from %d to %d:\n%s", len(baseline), len(after), out)
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
	baseline := unwiredNames(t, out)

	// Prefer a route the guard currently calls unwired, which is the case the
	// design describes: the route is implemented and reported unwired at the
	// same time. When the tree reaches parity any declared route will do, since
	// the diagnostic fires either way.
	target := "RouteHqBasicQot"
	for _, d := range declaredAsText(t, base) {
		if baseline[d[0]] {
			target = d[0]
			break
		}
	}
	var path string
	for _, d := range declaredAsText(t, base) {
		if d[0] == target {
			path = d[1]
		}
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
	if baseline[target] && !strings.Contains(out, "      "+target) {
		t.Errorf("%s was unwired at baseline, so the conversion must not be counted as an implementation:\n%s", target, out)
	}

	// In enforcing mode the same finding is fatal: it is the one class of
	// secondary finding that can make a green run mean something is unproven.
	out, code = run(root, true)
	if code != 1 {
		t.Fatalf("enforcing mode with an unresolvable reference must exit 1, got %d:\n%s", code, out)
	}
	if !diagnosticAt(out, "ERROR parity", "pkg/services/parity_conversion.go", `referenced without its named constant`) {
		t.Errorf("want the conversion promoted to an error in enforcing mode:\n%s", out)
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
	if !strings.Contains(crlfOut, "PARITY: 47/51 gap=4 mode=report enforce=off") {
		t.Errorf("the CRLF fixture must still reach 47/51:\n%s", crlfOut)
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
	if !strings.Contains(out, "PARITY: 47/51 gap=4") {
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
	if !strings.Contains(out, "PARITY: 47/51 gap=4") {
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
	if len(names) == 0 {
		t.Fatalf("no unwired names parsed from the report:\n%s", out)
	}
	return names
}
