// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command paritygate answers one question: which of the endpoints documented in
// docs/SPEC.md has no v-next service method?
//
// It runs in two modes. Report mode (the default) prints the gap and exits 0
// for any gap, so a run in the middle of the v-next work cannot redden CI. It
// still exits 1 on a broken invariant, because "report mode" means "never fails
// on a gap", not "never fails". Enforcing mode (--enforce) additionally exits 1
// when the gap is non-zero, or when a secondary diagnostic is outstanding.
//
// C14 is what makes the enforcing mode the CI default, and it is also the change
// that made a diagnostic fatal in that mode. The reason is one the log can show
// rather than argue about: before it, an enforcing run at a zero gap with one
// outstanding diagnostic printed "ERROR parity: ..." and "PASS: 51/51 ..." on
// the same run and exited 0. A run that contradicts itself is worse than a run
// that fails, because a reader has no way to know which half to believe.
//
// Making the diagnostic fatal was only safe once the diagnostic stopped firing
// on a permanent false positive. It used to fire on a client.Route-typed
// function or method *parameter* -- specifically Executor.Do's signature at
// pkg/services/executor.go:36, which every v-next call passes through -- and
// that is a declaration of what a function accepts, not a reference to a route.
// A value enters a parameter only at a call site, and call sites inside the scan
// roots are walked like any other code, so suppressing it loses nothing. A
// client.Route-typed *struct field* is the opposite and is still reported: a
// route stored in a field is put there somewhere the walk cannot see.
//
// The canonical route set is parsed out of client/routes.go rather than obtained
// from client.Routes(). That is deliberate: the registry would then come from
// the compiled binary rather than from the tree under test, which would make
// this command impossible to exercise against a fixture -- and the whole point of
// a guard is that its failure modes are testable. Parsing also means the guard
// has no dependency on any other package in this module, so a compile error
// elsewhere in the tree can never be misreported as a parity result.
//
// It is a standalone command rather than a Go test because report mode's entire
// output would be t.Log, and `go test` discards the log of a test that passes
// unless -v is passed. Neither the CI build job nor the race job is verbose, so
// a test-based guard would print nothing to anyone and its only signal would be
// a green tick carrying no information -- the exact failure report mode exists
// to prevent.
//
// Stdlib only, per AGENTS.md hard rule 8. go/parser and go/ast are the same
// technique internal/layering uses to assert a package boundary without adding
// a type-aware dependency.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	// pkgIdent is the local name of the module's client package inside the
	// scanned roots. Every counted reference is a selector expression whose
	// receiver is an *ast.Ident with this name, which is what distinguishes the
	// package identifier from a field selector such as s.client.
	pkgIdent = "client"

	// clientImportPath is the only import allowed to bind pkgIdent. A different
	// path bound to `client` would make every reference in that file
	// misattributed, so the guard refuses to run rather than report it.
	clientImportPath = `"` + "github.com/shing1211/hstongapi4go/client" + `"`

	// routesRel is the file that declares the canonical route set, relative to
	// the repository root.
	routesRel = "client/routes.go"

	// specRel is the file that states the canonical endpoint count, relative to
	// the repository root. AGENTS.md hard rule 5 makes it the only place that
	// number may be written, so its absence is fatal.
	specRel = "docs/SPEC.md"
)

// Integrity failures. Each is a broken invariant rather than a parity gap, and
// each is fatal in both modes. They are sentinels rather than formatted strings
// so a test can assert on the condition instead of on prose.
var (
	// errNoRouteConstants is distinct from "this repository has no routes":
	// either outcome makes every route read as unwired, and that is the one
	// reading a guard must never produce.
	errNoRouteConstants = errors.New("no `Route`-typed constants found in " + routesRel)

	// errNoRegistry and errManyRegistries are the two ways the registry parse
	// can silently yield a partial or empty set.
	errNoRegistry     = errors.New("no `canonicalRoutes` map literal found in " + routesRel)
	errManyRegistries = errors.New("more than one `canonicalRoutes` map literal found in " + routesRel)

	// errNoSpecTotal is the single most important invariant in the guard: a
	// parser that finds no Total line must fail, never return an empty set that
	// reads as "0 endpoints, all clear".
	errNoSpecTotal    = errors.New(specRel + ` declares no "**Total: N HTTP endpoints**" line`)
	errManySpecTotals = errors.New(specRel + ` declares more than one "**Total: N HTTP endpoints**" line`)

	errNoSpecGroups  = errors.New(specRel + " declares no `### 2.x` group headers")
	errNoSpecRows    = errors.New(specRel + " declares no numbered route rows")
	errNoBreakdown   = errors.New(specRel + " declares no `(a + b + ...)` breakdown on the Total line")
	errNoSPECFile    = errors.New(specRel + " does not exist")
	errNoRoutesFile  = errors.New(routesRel + " does not exist")
	errNoRoot        = errors.New("cannot locate the repository root")
	errNoReferences  = errors.New("no `" + pkgIdent + ".RouteXxx` reference found in any scanned non-test file")
	errNoSourcesFile = errors.New("no non-test .go file found in any scan root")
)

var (
	// totalLineRE finds the one line stating the canonical endpoint count. It
	// is deliberately not $-anchored: docs/SPEC.md is CRLF in some checkouts
	// and LF in others, and a $-anchored pattern would fail on one and pass on
	// the other for a reason that has nothing to do with the code. Every line is
	// also TrimSuffix'd of "\r" before matching.
	totalLineRE = regexp.MustCompile(`\*\*Total: ([0-9]+) HTTP endpoints\*\*`)

	// groupHeaderRE finds the `### 2.x <title> <count>` header prefix. The title
	// and the count are split afterwards, field by field, rather than by a
	// second capture group: SPEC writes "### 2.8 Futures — 11" with an em dash
	// and the const block writes "// Futures (11)", so the separator is not
	// load-bearing and a cosmetic reformat cannot fail CI. For the same reason
	// the guard never matches group *names* between the two files.
	groupHeaderRE = regexp.MustCompile(`^### 2\.([0-9]+) (.+)$`)

	// countFieldRE matches a field that is entirely digits, which is how the
	// trailing count is told apart from a title that happens to end in a digit.
	countFieldRE = regexp.MustCompile(`^[0-9]+$`)

	// routeRowRE finds a numbered SPEC route row: "| 12 | `/trade/TradeLogin`
	// | `POST http://...` |". It is an interpreted string because the pattern
	// contains backticks.
	routeRowRE = regexp.MustCompile("^\\| *([0-9]+) *\\| *`([^`]+)` *\\|")

	// breakdownRE finds the "(9 + 2 + 2 + 5 + 13 + 2 + 7 + 11)" arithmetic that
	// sits on the same line as the Total.
	breakdownRE = regexp.MustCompile(`\(([0-9]+(?: *\+ *[0-9]+)*)\)`)
)

// titleTrimCutset is stripped from the end of a header title so the em dash and
// any other separator the author used does not end up in the report.
const titleTrimCutset = " \t-–—:·|"

// defaultScanRoots are the packages a v-next service method may live in.
//
// pkg/services is the primary root. pkg/transport and internal/auth are joined
// to it for a reason worth stating: internal/auth takes an injected loginFn and
// names no route today, so composing it is the moment RouteTradeLogin first
// enters the v-next layer, and a pkg/services-only scan would report a gap no
// task can close. pkg/domain is excluded because internal/layering already
// forbids it from importing client, so it can never name a route. Nothing else
// is in scope: pkg/hstong/* references all 51 between them and is deprecated, so
// including it would make the guard permanently green and worthless.
//
// Test files are excluded from every root, and that exclusion is load-bearing
// rather than cosmetic: a test written before its implementation -- or a fixture
// that names every route in order to assert on it -- would otherwise be counted
// as an implementation.
var defaultScanRoots = []string{"pkg/services", "pkg/transport", "internal/auth"}

// config is every input the command reads. Nothing else touches the
// filesystem, so a test can drive the whole command against a throwaway tree by
// setting Root.
type config struct {
	// Root is the repository root. Empty means "resolve it from this source
	// file's location, then from the working directory".
	Root string
	// Enforce turns on fail-on-gap.
	Enforce bool
	// Routes and Spec are Root-relative paths.
	Routes string
	Spec   string
	// Roots are the Root-relative package directories scanned for references.
	Roots []string
}

// defaultConfig returns the configuration a bare `go run ./scripts/paritygate`
// uses: the resolved repository root, report mode, and the canonical paths.
func defaultConfig() config {
	return config{
		Routes: routesRel,
		Spec:   specRel,
		Roots:  defaultScanRoots,
	}
}

func main() {
	cfg := defaultConfig()
	fs := flag.NewFlagSet("paritygate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.Root, "root", "", "repository root to read (default: resolved from the source file, then the working directory)")
	fs.BoolVar(&cfg.Enforce, "enforce", false, "exit 1 when any declared endpoint has no v-next service method, or when a secondary diagnostic is outstanding")
	// --enforce is a flag and not an environment variable on purpose. It
	// appears in the `run:` line of the CI step, so the mode is greppable in
	// the workflow and readable in the log, and default-off makes the safe
	// direction the default: an exported shell variable can never make a
	// developer's local run enforcing by accident.
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "paritygate: unexpected argument %q\n", fs.Arg(0))
		os.Exit(2)
	}
	out, code := evaluate(cfg).report()
	fmt.Print(out)
	os.Exit(code)
}

// routeDecl is one `RouteXxx Route = "/path"` constant, in declaration order.
// Declaration order is load-bearing: the SPEC route rows are numbered 1..N in
// the same order, which is how group boundaries are joined without matching
// group names across two different naming conventions.
type routeDecl struct {
	name string
	path string
	line int
}

// registryEntry is one key of the canonicalRoutes map literal. name is empty
// for a key written as a bare path, which is a registry entry no service can
// name by constant.
type registryEntry struct {
	name string
	path string
	line int
}

// reference is one counted `client.RouteXxx` selector expression, with the
// position that justifies counting it. Every reference is printed, because a
// guard that shows its work is auditable and one that does not is not.
type reference struct {
	name string
	rel  string
	line int
}

// diagnostic is a secondary finding: something the reference walk cannot resolve
// on its own. Diagnostics are warnings in report mode, which reports them and
// exits 0 anyway, and errors in enforcing mode, which fails on them: a run that
// cannot say which route references it did not see must not be green.
type diagnostic struct {
	rel  string
	line int
	what string
}

// group is one `### 2.x` SPEC header and the declared endpoints it covers.
type group struct {
	number   int
	title    string
	declared int
	start    int
}

// problem is an integrity failure: a broken invariant rather than a parity gap.
// Any problem means exit 1 in both modes, because the alternative is a report
// that says nothing is wrong while being unable to tell.
type problem struct {
	err  error
	hint string
}

// specInfo is what the guard reads out of SPEC.md.
type specInfo struct {
	total     int
	totalLine int
	breakdown []int
	groups    []group
	rows      []string
	rowNums   []int
}

// analysis is everything the report is rendered from. evaluate never prints, so
// every field a test asserts on is a plain value rather than captured output.
type analysis struct {
	cfg config
	// root is the resolved repository root, empty when it could not be found.
	root string
	// measured records that the route set, the registry and the SPEC count were
	// all read successfully. When it is false the report states that coverage
	// is NOT MEASURED rather than printing "0/0 ... 0 NOT implemented", which
	// would read as a clean bill of health.
	measured bool
	decls    []routeDecl
	registry []registryEntry
	refs     []reference
	diags    []diagnostic
	spec     specInfo
	// grouped records that the positional SPEC-to-const join agreed. When it did
	// not, the group rendering is suppressed: a confidently wrong grouping is
	// worse than none, while the gap count itself stays valid.
	grouped  bool
	joinNote string
	// groupOf maps a declared-constant index to its group index, filled in by
	// joinGroups. It is only meaningful when grouped is true.
	groupOf   []int
	wiredName map[string]bool
	problems  []problem
}

// evaluate performs every check and never fails; report turns the result into
// the exit code.
func evaluate(cfg config) analysis {
	a := analysis{cfg: normalize(cfg)}
	if a.root = resolveRoot(a.cfg.Root, a.cfg.Routes); a.root == "" {
		a.fail(errNoRoot, "neither this source file's location nor the working directory contains "+a.cfg.Routes+
			"; a wrong root produces a wrong report, not a failure")
		return a
	}

	decls, registry, err := parseRoutes(filepath.Join(a.root, a.cfg.Routes))
	if err != nil {
		a.fail(err, hintFor(err))
		return a
	}
	if len(decls) == 0 {
		a.fail(errNoRouteConstants, "a const-block parse that matched nothing is indistinguishable from a repository with no routes")
		return a
	}
	a.decls, a.registry = decls, registry

	// Check 1: the const block and the registry agree, in both directions.
	a.checkRegistry()

	spec, err := parseSpec(filepath.Join(a.root, a.cfg.Spec))
	if err != nil {
		a.fail(err, hintFor(err))
		return a
	}
	a.spec = spec

	// Check 2: the declared count against SPEC, then the three copies SPEC
	// states it in.
	if spec.total != len(decls) {
		a.fail(fmt.Errorf("%s:%d declares %d HTTP endpoints; the %s const block declares %d",
			a.cfg.Spec, spec.totalLine, spec.total, a.cfg.Routes, len(decls)),
			"the route set is the client const block and SPEC states the number (AGENTS.md rule 5), "+
				"so the two cannot disagree without a red run")
		return a
	}
	if sumDeclared(spec.groups) != spec.total {
		a.fail(fmt.Errorf("%s group headers sum to %d but line %d declares %d",
			a.cfg.Spec, sumDeclared(spec.groups), spec.totalLine, spec.total),
			"SPEC states both the group counts and the Total; they are one number written twice")
		return a
	}
	if err := checkBreakdown(spec); err != nil {
		a.fail(err, "SPEC states the same count three ways (group headers, breakdown, Total); "+
			"a commit that edits one of them is not a coherent commit")
		return a
	}

	// The reference walk. Its own integrity conditions are fatal in both modes:
	// a walk that cannot see the references would report a zero gap, and a zero
	// gap is what full parity looks like.
	a.scan()

	// Coverage is reported only when every integrity condition held. A number
	// computed from a partial walk, a missing scan root, or a disagreeing SPEC
	// is the one output this guard must never print, because it would read as a
	// measurement while being a guess.
	a.measured = len(a.problems) == 0
	if a.measured {
		a.joinGroups()
	}
	return a
}

// normalize fills in the canonical paths and scan roots so a test only has to
// set Root.
func normalize(cfg config) config {
	if cfg.Routes == "" {
		cfg.Routes = routesRel
	}
	if cfg.Spec == "" {
		cfg.Spec = specRel
	}
	if len(cfg.Roots) == 0 {
		cfg.Roots = defaultScanRoots
	}
	return cfg
}

// fail records an integrity failure. Any failure is fatal in both modes.
func (a *analysis) fail(err error, hint string) {
	a.problems = append(a.problems, problem{err: err, hint: hint})
}

// hintFor supplies the reason an input cannot be trusted. These are the
// conditions under which a report would be confidently wrong rather than
// obviously broken, so each hint names what the guard would have concluded
// without the check.
func hintFor(err error) string {
	switch {
	case errors.Is(err, errNoRegistry):
		return "a rename or a split of the registry would produce a silently empty set, and an empty set reports every route as unwired"
	case errors.Is(err, errManyRegistries):
		return "the guard parses the first registry it finds; more than one means it may have parsed the wrong one"
	case errors.Is(err, errNoSpecTotal):
		return "that line is the canonical count (AGENTS.md rule 5); a zero-match must never read as \"0 endpoints, all clear\""
	case errors.Is(err, errManySpecTotals):
		return "the count must be stated exactly once, or the guard cannot tell which copy is canonical"
	case errors.Is(err, errNoSPECFile):
		return "AGENTS.md rule 5 makes this the only source of the count"
	case errors.Is(err, errNoRoutesFile):
		return "the canonical route set is the const block in this file"
	case errors.Is(err, errNoRouteConstants):
		return "an empty const-block parse reports every route as unwired"
	case errors.Is(err, errNoSpecGroups), errors.Is(err, errNoSpecRows), errors.Is(err, errNoBreakdown):
		return "SPEC states the count three ways; a missing one means the guard is parsing a shape that no longer exists"
	default:
		return ""
	}
}

// checkRegistry is check 1: every declared constant is a registered route, and
// every registered route has a constant.
//
// The first direction is what client.Client.Do consults -- Do calls
// route.Validate, which reads canonicalRoutes -- so a constant missing from the
// registry is a route that fails at run time with client.ErrUnknownRoute. No
// test covers it: client/routes_test.go iterates the registry, so a constant
// omitted from it is invisible. The second direction is the reference walk's
// blind spot promoted to a table error: a registry path with no constant is
// reachable only by conversion, so no service can name it and it would be
// permanently unwired.
func (a *analysis) checkRegistry() {
	byName := make(map[string]registryEntry, len(a.registry))
	byPath := make(map[string]bool, len(a.registry))
	for _, e := range a.registry {
		if e.name != "" {
			byName[e.name] = e
		}
		byPath[e.path] = true
	}
	declared := make(map[string]bool, len(a.decls))
	for _, d := range a.decls {
		declared[d.name] = true
		_, named := byName[d.name]
		if !named && !byPath[d.path] {
			a.fail(fmt.Errorf("%s:%d %s = %q is not in canonicalRoutes", a.cfg.Routes, d.line, d.name, d.path),
				"client.Client.Do would refuse it: errors.Is(route.Validate(), client.ErrUnknownRoute)")
		}
	}
	for _, e := range a.registry {
		if e.name == "" {
			a.fail(fmt.Errorf("canonicalRoutes holds %q with no constant in %s", e.path, a.cfg.Routes),
				"no service can name it; add a Route constant or drop the entry")
			continue
		}
		if !declared[e.name] {
			a.fail(fmt.Errorf("%s:%d canonicalRoutes names %s, which no constant in the const block declares",
				a.cfg.Routes, e.line, e.name),
				"the registry and the const block must declare the same set")
		}
	}
}

// parseRoutes reads both halves of the canonical route set out of
// client/routes.go: the `Route`-typed constants, in declaration order, and the
// canonicalRoutes map literal.
//
// Only constants with an explicit `Route` type are taken. The untyped string
// constants in that file (aliasSuffixMsgType and friends) are not routes, and
// requiring the type is what keeps a reworded alias from being counted as a
// 52nd endpoint.
func parseRoutes(path string) ([]routeDecl, []registryEntry, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil, errNoRoutesFile
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}

	var decls []routeDecl
	registries := 0
	var registry []registryEntry

	for _, d := range file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch gen.Tok {
		case token.CONST:
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || !isRouteType(vs.Type) {
					continue
				}
				if len(vs.Names) != 1 || len(vs.Values) != 1 {
					return nil, nil, fmt.Errorf("%s: a Route constant must declare one name with one value", path)
				}
				lit, ok := vs.Values[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return nil, nil, fmt.Errorf("%s: a Route constant must be initialized with a string literal", path)
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: unquoting %s: %w", path, lit.Value, err)
				}
				decls = append(decls, routeDecl{
					name: vs.Names[0].Name,
					path: value,
					line: fset.Position(vs.Pos()).Line,
				})
			}
		case token.VAR:
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "canonicalRoutes" {
					continue
				}
				registries++
				entries, err := parseRegistry(fset, vs)
				if err != nil {
					return nil, nil, err
				}
				registry = entries
			}
		}
	}

	switch {
	case registries == 0:
		return nil, nil, errNoRegistry
	case registries > 1:
		return nil, nil, errManyRegistries
	}
	return decls, registry, nil
}

// isRouteType reports whether a const spec carries an explicit `Route` type.
func isRouteType(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "Route"
}

// parseRegistry reads the canonicalRoutes map literal. A key written as a bare
// string literal, or as a Route conversion, has no constant name, which is
// recorded as an empty name so check 1 can report it.
func parseRegistry(fset *token.FileSet, vs *ast.ValueSpec) ([]registryEntry, error) {
	if len(vs.Values) != 1 {
		return nil, errors.New("canonicalRoutes must be initialized with one composite literal")
	}
	lit, ok := vs.Values[0].(*ast.CompositeLit)
	if !ok {
		return nil, errors.New("canonicalRoutes must be initialized with a composite literal")
	}
	entries := make([]registryEntry, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return nil, errors.New("every canonicalRoutes element must be a key: value pair")
		}
		entry := registryEntry{line: fset.Position(kv.Pos()).Line}
		switch key := kv.Key.(type) {
		case *ast.Ident:
			entry.name = key.Name
		case *ast.BasicLit:
			value, err := unquoteString(key)
			if err != nil {
				return nil, err
			}
			entry.path = value
		case *ast.CallExpr:
			if len(key.Args) != 1 {
				return nil, errors.New("a conversion key in canonicalRoutes must convert one string literal")
			}
			arg, ok := key.Args[0].(*ast.BasicLit)
			if !ok {
				return nil, errors.New("a conversion key in canonicalRoutes must convert a string literal")
			}
			value, err := unquoteString(arg)
			if err != nil {
				return nil, err
			}
			entry.path = value
		default:
			return nil, fmt.Errorf("unsupported canonicalRoutes key of type %T", kv.Key)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// unquoteString unquotes a Go string literal, reporting the position of the
// caller on failure.
func unquoteString(lit *ast.BasicLit) (string, error) {
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", fmt.Errorf("unquoting %s: %w", lit.Value, err)
	}
	return value, nil
}

// parseSpec reads the canonical count, the group headers, the breakdown
// arithmetic, and the numbered route rows out of docs/SPEC.md.
//
// Line endings are normalised here and only here. docs/SPEC.md is CRLF in some
// working trees and LF in others -- it is the one file this guard parses as
// text, and .gitattributes only applies on checkout -- so a `$`-anchored match
// or a naive strings.Split would fail on a dev host and pass on the runner for
// a reason unrelated to the code. Splitting on "\n" and TrimSuffix'ing "\r"
// makes the parse line-ending agnostic, and SPEC contains non-ASCII (the em
// dash), so the match is over runes.
func parseSpec(path string) (specInfo, error) {
	// #nosec G304 -- path is filepath.Join(root, "docs/SPEC.md"): a fixed relative
	// filename joined to the repository root the developer supplied via --root. Same
	// threat model as the G204 annotation in scripts/coverage_gate.go: a local
	// developer tool with no external input, and the root is the very thing the
	// operator asked it to read. Annotating rather than restructuring, because
	// "refuse to read a file whose path came from a flag" is not a property this
	// script can have.
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return specInfo{}, errNoSPECFile
		}
		return specInfo{}, err
	}
	lines := strings.Split(string(raw), "\n")
	info := specInfo{}
	totals := 0

	for i, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if m := totalLineRE.FindStringSubmatch(line); m != nil {
			totals++
			if totals > 1 {
				return specInfo{}, errManySpecTotals
			}
			n, convErr := strconv.Atoi(m[1])
			if convErr != nil {
				return specInfo{}, fmt.Errorf("%s:%d: %w", specRel, i+1, convErr)
			}
			info.total, info.totalLine = n, i+1
			mb := breakdownRE.FindStringSubmatch(line)
			if mb == nil {
				return specInfo{}, errNoBreakdown
			}
			for _, term := range strings.Split(mb[1], "+") {
				v, convErr := strconv.Atoi(strings.TrimSpace(term))
				if convErr != nil {
					return specInfo{}, fmt.Errorf("%s:%d breakdown term %q: %w", specRel, i+1, term, convErr)
				}
				info.breakdown = append(info.breakdown, v)
			}
		}
		if m := groupHeaderRE.FindStringSubmatch(line); m != nil {
			n, convErr := strconv.Atoi(m[1])
			if convErr != nil {
				return specInfo{}, fmt.Errorf("%s:%d: %w", specRel, i+1, convErr)
			}
			fields := strings.Fields(m[2])
			if len(fields) == 0 || !countFieldRE.MatchString(fields[len(fields)-1]) {
				return specInfo{}, fmt.Errorf("%s:%d group header %q does not end in a declared count",
					specRel, i+1, m[2])
			}
			declared, convErr := strconv.Atoi(fields[len(fields)-1])
			if convErr != nil {
				return specInfo{}, fmt.Errorf("%s:%d: %w", specRel, i+1, convErr)
			}
			title := strings.TrimRight(strings.Join(fields[:len(fields)-1], " "), titleTrimCutset)
			info.groups = append(info.groups, group{number: n, title: title, declared: declared})
		}
		if m := routeRowRE.FindStringSubmatch(line); m != nil {
			n, convErr := strconv.Atoi(m[1])
			if convErr != nil {
				return specInfo{}, fmt.Errorf("%s:%d: %w", specRel, i+1, convErr)
			}
			info.rowNums = append(info.rowNums, n)
			info.rows = append(info.rows, m[2])
		}
	}

	if totals == 0 {
		return specInfo{}, errNoSpecTotal
	}
	if len(info.groups) == 0 {
		return specInfo{}, errNoSpecGroups
	}
	if len(info.rows) == 0 {
		return specInfo{}, errNoSpecRows
	}
	for i, g := range info.groups {
		if g.number != i+1 {
			return specInfo{}, fmt.Errorf("%s declares group header 2.%d out of order", specRel, g.number)
		}
	}
	return info, nil
}

// sumDeclared totals the declared endpoints across the group headers.
func sumDeclared(groups []group) int {
	total := 0
	for _, g := range groups {
		total += g.declared
	}
	return total
}

// checkBreakdown verifies the third copy of the count: the arithmetic on the
// Total line. SPEC states the same number three ways -- eight group headers, the
// breakdown, and the Total -- and requiring all three to agree makes a
// hand-edited count a four-place edit (this file, the const block, and the two
// Go literals in client) rather than a one-place one.
func checkBreakdown(info specInfo) error {
	if len(info.breakdown) != len(info.groups) {
		return fmt.Errorf("%s line %d breaks down as %d terms but %d group headers are declared",
			specRel, info.totalLine, len(info.breakdown), len(info.groups))
	}
	sum := 0
	for _, v := range info.breakdown {
		sum += v
	}
	for i, v := range info.breakdown {
		if v != info.groups[i].declared {
			return fmt.Errorf("%s group headers sum to %d but line %d breaks down as %d",
				specRel, sumDeclared(info.groups), info.totalLine, sum)
		}
	}
	return nil
}

// scan walks the scan roots and counts the references, collecting the secondary
// diagnostics from the same pass.
func (a *analysis) scan() {
	declaredNames := make(map[string]bool, len(a.decls))
	declaredPaths := make(map[string]bool, len(a.decls))
	for _, d := range a.decls {
		declaredNames[d.name] = true
		declaredPaths[d.path] = true
	}
	a.wiredName = make(map[string]bool, len(a.refs))
	files := 0

	for _, rel := range a.cfg.Roots {
		dir := filepath.Join(a.root, filepath.FromSlash(rel))
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			a.fail(fmt.Errorf("scan root %s does not exist", rel),
				"a renamed directory must not read as \"nothing referenced\"")
			continue
		}
		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// testdata is ignored by the go tool, so its contents are not
				// package code and must not be credited.
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			refs, diags, probs := scanFile(path, slashRelative(a.root, path), declaredNames, declaredPaths)
			for _, p := range probs {
				a.fail(p.err, p.hint)
			}
			a.refs = append(a.refs, refs...)
			a.diags = append(a.diags, diags...)
			for _, r := range refs {
				a.wiredName[r.name] = true
			}
			files++
			return nil
		})
		if walkErr != nil {
			a.fail(fmt.Errorf("walking %s: %w", rel, walkErr),
				"an unreadable scan root must not read as \"nothing referenced\"")
		}
	}

	if files == 0 {
		a.fail(errNoSourcesFile, "a scan root with no source would report every route as unwired")
	}
	if len(a.refs) == 0 {
		a.fail(errNoReferences, "a walk that found nothing cannot tell full parity from a broken walk, "+
			"and a zero gap is exactly what full parity looks like")
	}
}

// scanFile walks one parsed file. It returns the counted references, the
// secondary diagnostics, and any integrity problem.
func scanFile(path, rel string, declaredNames, declaredPaths map[string]bool) ([]reference, []diagnostic, []problem) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, []problem{{
			err:  fmt.Errorf("%s: %w", rel, err),
			hint: "an unparseable scanned file must not read as \"nothing referenced\"",
		}}
	}

	var refs []reference
	var diags []diagnostic
	var probs []problem

	// A string literal that is the argument of a client.Route conversion is
	// reported once, as a conversion, rather than twice.
	conversionLits := map[token.Pos]bool{}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			// A counted reference is a selector whose receiver is the bare
			// identifier `client` and whose selector is a declared constant
			// name. This is what separates the package qualifier from a field
			// selector such as s.client, and what keeps `client.Route` (the
			// type) and `client.Routes` (the function) out of the count.
			if isPkgIdent(node.X) && declaredNames[node.Sel.Name] {
				refs = append(refs, reference{
					name: node.Sel.Name,
					rel:  rel,
					line: fset.Position(node.Pos()).Line,
				})
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok || !isPkgIdent(sel.X) || sel.Sel.Name != "Route" || len(node.Args) != 1 {
				return true
			}
			lit, ok := node.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, unqErr := unquoteString(lit)
			if unqErr != nil || !declaredPaths[value] {
				return true
			}
			conversionLits[lit.Pos()] = true
			diags = append(diags, diagnostic{
				rel:  rel,
				line: fset.Position(node.Pos()).Line,
				what: fmt.Sprintf("route %q referenced without its named constant, as a %s.%s conversion; the route still counts as unwired", value, pkgIdent, sel.Sel.Name),
			})
		case *ast.BasicLit:
			if node.Kind != token.STRING || conversionLits[node.Pos()] {
				return true
			}
			value, unqErr := unquoteString(node)
			if unqErr != nil {
				return true
			}
			if canonical, ok := aliasPath(value, declaredPaths); ok {
				diags = append(diags, diagnostic{
					rel:  rel,
					line: fset.Position(node.Pos()).Line,
					what: fmt.Sprintf("string literal %q is the declared route %q (or one of the two Gateway aliases); a service that builds the path itself is never credited by name", value, canonical),
				})
			}
		}
		return true
	})

	checkStoredRoutes(fset, file, rel, &diags)
	probs = append(probs, checkClientBindings(fset, file, rel, &diags)...)
	return refs, diags, probs
}

// checkStoredRoutes reports every client.Route-typed *struct field* declared in a
// scanned file. It is the one shape where a route reference can exist in the
// scanned set and still be unattributable, and it is fatal in enforcing mode
// because of that: a route can be withdrawn into a field, or a route can be
// implemented through one, and neither direction is resolvable from the
// declaration.
//
// # Why a function or method parameter is not reported
//
// The distinction is mechanical rather than a matter of taste. Both a parameter
// and a struct field are an *ast.Field, so a walk that matched on the node type
// alone covered both -- and the parameter half fired permanently on
// pkg/services/executor.go:36, which is Executor.Do's own signature:
//
//	Do(ctx context.Context, op string, route client.Route, params any, codec client.Codec, out any) error
//
// That declaration states what the interface *accepts*. It is not a reference to
// a route, and it cannot become one: a value reaches a parameter only at a call
// site, and every call site inside the scan roots is walked by scanFile like any
// other code. The 51 routes are credited at those call sites, on the far side of
// this parameter, which is the point of it. Reporting the signature therefore
// reported a blind spot that does not exist -- and since Executor is correct and
// client.Client must keep satisfying it, "deal with the parameter" was not an
// available fix, so a fatal diagnostic would have reddened CI permanently.
//
// The field half is the opposite. A stored value is written somewhere the walk
// may never visit, and nothing in the declaration says what.
//
// The walk is over *ast.StructType rather than *ast.Field, so which of the two is
// being examined is decided by the node the walk descends into rather than by
// string-matching the type, and an interface method's `route client.Route` --
// which is a *ast.FuncType parameter, the suppressed case -- cannot leak in.
func checkStoredRoutes(fset *token.FileSet, file *ast.File, rel string, diags *[]diagnostic) {
	ast.Inspect(file, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, f := range st.Fields.List {
			sel, ok := f.Type.(*ast.SelectorExpr)
			if !ok || !isPkgIdent(sel.X) || sel.Sel.Name != "Route" {
				continue
			}
			*diags = append(*diags, diagnostic{
				rel:  rel,
				line: fset.Position(f.Pos()).Line,
				what: fmt.Sprintf("a %s.%s-typed struct field: a route stored in a field is written somewhere the walk may not reach, so it can be neither credited nor withdrawn by name",
					pkgIdent, sel.Sel.Name),
			})
		}
		return true
	})
}

// isPkgIdent reports whether expr is the bare identifier `client`.
func isPkgIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == pkgIdent
}

// aliasPath reports whether value is a declared route path or one of the two
// Gateway alias forms client.NormalizePath accepts, returning the canonical path
// it maps to.
func aliasPath(value string, declaredPaths map[string]bool) (string, bool) {
	if declaredPaths[value] {
		return value, true
	}
	for _, suffix := range []string{"RequestMsgType", "Request"} {
		if trimmed := strings.TrimSuffix(value, suffix); trimmed != value && declaredPaths[trimmed] {
			return trimmed, true
		}
	}
	return "", false
}

// checkClientBindings is the fatal self-check of the reference walk.
//
// If a scanned file binds the identifier `client` to anything other than this
// module's client package, every `client.RouteXxx` in that file is ambiguous and
// the walk would under-count silently, reporting a smaller gap than the truth.
// The guard must refuse to pass rather than report a smaller gap, because a
// guard that reports everything as wired after scanning nothing is worse than no
// guard. The same reasoning internal/layering uses to refuse to pass when its
// import walk finds nothing.
//
// Struct field names are deliberately NOT checked. A field is reached through
// its owner's selector (`s.client`), never as a bare identifier, so it cannot
// shadow the package name -- and pkg/services has three of them today
// (AccountService.client, MarketService.client, TradingService.client). Only
// declarations that actually bind a name in scope are checked: function
// signatures (which is where a receiver, parameter, or result lives), package-
// level declarations, and local assignments, range clauses, and type switches.
func checkClientBindings(fset *token.FileSet, file *ast.File, rel string, diags *[]diagnostic) []problem {
	var probs []problem
	report := func(pos token.Pos, kind string) {
		probs = append(probs, problem{
			err: fmt.Errorf("%s:%d declares identifier %q (%s); the reference walk would under-count",
				rel, fset.Position(pos).Line, pkgIdent, kind),
			hint: "a `client` that is not this module's client package makes every `" + pkgIdent + ".RouteXxx` in the file ambiguous",
		})
	}
	checkFieldList := func(fl *ast.FieldList, kind string) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, name := range f.Names {
				if name.Name == pkgIdent {
					report(name.Pos(), kind)
				}
			}
		}
	}
	checkDef := func(id *ast.Ident, kind string) {
		if id != nil && id.Name == pkgIdent {
			report(id.Pos(), kind)
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ImportSpec:
			bound := ""
			if node.Name != nil {
				bound = node.Name.Name
			} else if node.Path != nil {
				bound = lastPathSegment(node.Path.Value)
			}
			switch {
			case node.Path != nil && node.Path.Value == clientImportPath && bound != pkgIdent:
				// A miscount, not a miscredit: a route referenced through the
				// alias is not counted at all, so the gap would be overstated.
				*diags = append(*diags, diagnostic{
					rel:  rel,
					line: fset.Position(node.Pos()).Line,
					what: fmt.Sprintf("imports the client package as %q; route references through that alias are not counted", bound),
				})
			case node.Path != nil && bound == pkgIdent && node.Path.Value != clientImportPath:
				report(node.Pos(), "import of "+node.Path.Value)
			}
		case *ast.FuncDecl:
			// A method name lives in its type's namespace and cannot shadow a
			// package identifier; a package-level function name can.
			if node.Recv == nil && node.Name.Name == pkgIdent {
				report(node.Name.Pos(), "package-level func")
			}
			checkFieldList(node.Recv, "receiver")
			checkFieldList(node.Type.Params, "parameter")
			checkFieldList(node.Type.Results, "result")
		case *ast.FuncLit:
			checkFieldList(node.Type.Params, "parameter")
			checkFieldList(node.Type.Results, "result")
		case *ast.ValueSpec:
			for _, name := range node.Names {
				checkDef(name, "const/var")
			}
		case *ast.TypeSpec:
			checkDef(node.Name, "type")
		case *ast.AssignStmt:
			if node.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range node.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					checkDef(id, "local variable")
				}
			}
		case *ast.RangeStmt:
			if node.Tok != token.DEFINE {
				return true
			}
			if id, ok := node.Key.(*ast.Ident); ok {
				checkDef(id, "range variable")
			}
			if id, ok := node.Value.(*ast.Ident); ok {
				checkDef(id, "range variable")
			}
		case *ast.TypeSwitchStmt:
			assign, ok := node.Assign.(*ast.AssignStmt)
			if !ok || assign.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range assign.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					checkDef(id, "type switch variable")
				}
			}
		}
		return true
	})
	return probs
}

// lastPathSegment returns the default local name of an import path, which is
// its last slash-separated element.
func lastPathSegment(quoted string) string {
	trimmed := strings.Trim(quoted, `"`)
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}

// joinGroups assigns each declared constant to the SPEC group that covers it,
// positionally. The SPEC route rows are numbered 1..N in the same order as the
// const block, so a path disagreement at any position means the two orderings
// have diverged; the group rendering is then suppressed rather than printed
// confidently and wrongly. The gap count does not depend on grouping and stays
// valid either way.
func (a *analysis) joinGroups() {
	if len(a.spec.rows) != a.spec.total {
		a.joinNote = fmt.Sprintf("%s has %d numbered route rows but declares %d endpoints",
			a.cfg.Spec, len(a.spec.rows), a.spec.total)
		return
	}
	for i, row := range a.spec.rows {
		if a.decls[i].path != row {
			a.joinNote = fmt.Sprintf("%s row %d is %q but %s:%d declares %s = %q",
				a.cfg.Spec, a.spec.rowNums[i], row, a.cfg.Routes, a.decls[i].line, a.decls[i].name, a.decls[i].path)
			return
		}
	}
	a.groupOf = make([]int, len(a.decls))
	start := 0
	for gi := range a.spec.groups {
		a.spec.groups[gi].start = start
		for k := start; k < start+a.spec.groups[gi].declared; k++ {
			a.groupOf[k] = gi
		}
		start += a.spec.groups[gi].declared
	}
	if start != len(a.decls) {
		a.joinNote = fmt.Sprintf("group headers cover %d constants but the const block declares %d", start, len(a.decls))
		return
	}
	a.grouped = true
}

// unwired returns the indexes of the declared constants no service names, in
// declaration order.
func (a *analysis) unwired() []int {
	out := make([]int, 0, len(a.decls))
	for i, d := range a.decls {
		if !a.wiredName[d.name] {
			out = append(out, i)
		}
	}
	return out
}

// wiredIn returns how many of a group's constants are referenced and how many
// are not.
func (a *analysis) wiredIn(g group) (int, int) {
	n := 0
	for _, d := range a.decls[g.start : g.start+g.declared] {
		if a.wiredName[d.name] {
			n++
		}
	}
	return n, g.declared - n
}

// gapIn counts the unwired constants in a group.
func (a *analysis) gapIn(g group) int {
	_, gap := a.wiredIn(g)
	return gap
}

// groupOfIndex returns the SPEC group covering the i-th declared constant.
func (a *analysis) groupOfIndex(i int) group {
	if len(a.groupOf) == 0 {
		return group{}
	}
	if i < 0 || i >= len(a.groupOf) {
		return group{}
	}
	return a.spec.groups[a.groupOf[i]]
}

// slashRelative returns path relative to root in slash form, so the report's
// file:line columns are stable across platforms.
func slashRelative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// hasFile reports whether root contains the named file. A directory of that
// name does not count.
func hasFile(root, rel string) bool {
	if root == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

// resolveRoot finds the repository root. An explicit --root wins; otherwise this
// source file's location is used (which `go run` resolves to the real source
// path), then the working directory. It returns "" rather than a guess when no
// candidate holds the route file: a wrong root produces a wrong report, not a
// failure, so guessing is the one option that must not be taken.
func resolveRoot(explicit, routesRelPath string) string {
	if explicit != "" {
		if hasFile(explicit, routesRelPath) {
			return filepath.Clean(explicit)
		}
		return ""
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		// <root>/scripts/paritygate/main.go -> <root>
		candidate := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		if hasFile(candidate, routesRelPath) {
			return candidate
		}
	}
	if wd, err := os.Getwd(); err == nil && hasFile(wd, routesRelPath) {
		return filepath.Clean(wd)
	}
	return ""
}

// report renders the whole output and returns the exit code.
//
// # The exit contract
//
// Before C14: exit 1 if and only if an integrity check failed, or --enforce was
// given and the gap was non-zero. A secondary diagnostic never entered the
// expression.
//
// After C14: exit 1 if and only if an integrity check failed; or --enforce was
// given and either the gap is non-zero or a secondary diagnostic is outstanding.
//
// The added clause is what makes an enforcing run's PASS line trustworthy. A
// diagnostic is a statement that the walk could not account for a route
// reference, so leaving it out of the exit code allowed one run to print
// "ERROR parity: ..." and "PASS: 51/51 ..." together and exit 0 -- the log
// contradicting itself, which is the precise failure the vocabulary rules above
// exist to prevent. It holds the diagnostic to the same standard as an integrity
// failure: a run that cannot say what it did not see is not a measurement.
//
// # Report mode never fails on a gap
//
// Neither the gap nor a diagnostic is fatal outside enforcing mode, and this is
// the invariant report mode exists to protect. A report-mode run that exits 0 for
// 22 unwired endpoints, and a report-mode run that exits 0 while printing a
// diagnostic, are both correct -- both are loudly not-parity runs. A
// report-mode run that exits 0 on a corrupt SPEC is not, and that is why
// "report mode" never meant "never fails".
func (a analysis) report() (string, int) {
	var b strings.Builder

	open, wired := 0, 0
	if a.measured {
		open = len(a.unwired())
		wired = len(a.wiredName)
	}
	code := 0
	if len(a.problems) > 0 || (a.cfg.Enforce && (open > 0 || len(a.diags) > 0)) {
		code = 1
	}

	// The banner names the mode on the first line and the outcome on the second,
	// so a skimmed log cannot be read as success. SPEC's em dash is deliberately
	// not reproduced: this output has to stay legible in a Windows console,
	// whose active code page renders U+2014 as "?". The wording is the
	// requirement; the glyph is not.
	if a.cfg.Enforce {
		b.WriteString("PARITY GUARD - ENFORCING MODE (this run fails on a coverage gap)\n")
	} else {
		b.WriteString("PARITY GUARD - REPORT MODE (informational; this run cannot fail on a coverage gap)\n")
	}
	if a.measured {
		fmt.Fprintf(&b, "  v-next service coverage: %d/%d endpoints named by a service; %d NOT implemented\n",
			wired, a.spec.total, open)
	} else {
		b.WriteString("  v-next service coverage: NOT MEASURED - the canonical route set or the SPEC count could not be read\n")
	}
	b.WriteString("\n")

	// What was read is printed in every case, so a failure is diagnosable and a
	// fixture run is unmistakable in the log (design note §9).
	inputs := func() {
		fmt.Fprintf(&b, "  Repo root: %s\n", a.root)
		fmt.Fprintf(&b, "  Scanned: %s (test files excluded)\n", strings.Join(a.cfg.Roots, ", "))
		if a.measured {
			fmt.Fprintf(&b, "  Route set: %s (%d declared)   SPEC: %s (declares %d)\n",
				a.cfg.Routes, len(a.decls), a.cfg.Spec, a.spec.total)
		}
	}
	if !a.measured {
		inputs()
		b.WriteString("\n")
	}

	if a.measured {
		if a.grouped {
			b.WriteString("  Group                              declared  wired   gap\n")
			for _, g := range a.spec.groups {
				w, gap := a.wiredIn(g)
				fmt.Fprintf(&b, "  %-30s %8d %6d %5d\n", g.title, g.declared, w, gap)
			}
			fmt.Fprintf(&b, "  %-30s %8d %6d %5d\n", "TOTAL", a.spec.total, wired, open)
		} else {
			fmt.Fprintf(&b, "  Group breakdown SUPPRESSED: %s\n", a.joinNote)
			b.WriteString("  The gap total is unaffected; only the per-group attribution is withheld.\n")
		}
		b.WriteString("\n")

		unwired := a.unwired()
		fmt.Fprintf(&b, "  Unwired (%d of %d declared), by SPEC group:\n", len(unwired), a.spec.total)
		if len(unwired) == 0 {
			b.WriteString("    (none)\n")
		}
		headed := make(map[int]bool, len(a.spec.groups))
		for _, i := range unwired {
			g := a.groupOfIndex(i)
			if a.grouped && !headed[a.groupOf[i]] {
				headed[a.groupOf[i]] = true
				fmt.Fprintf(&b, "    %s (%d)\n", g.title, a.gapIn(g))
			}
			fmt.Fprintf(&b, "      %-42s %s\n", a.decls[i].name, a.decls[i].path)
		}
		b.WriteString("\n")

		inputs()

		refs := append([]reference(nil), a.refs...)
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].rel != refs[j].rel {
				return refs[i].rel < refs[j].rel
			}
			if refs[i].line != refs[j].line {
				return refs[i].line < refs[j].line
			}
			return refs[i].name < refs[j].name
		})
		paths := make(map[string]string, len(a.decls))
		for _, d := range a.decls {
			paths[d.name] = d.path
		}
		fmt.Fprintf(&b, "  Referenced (%d of %d), file:line for every counted reference:\n", len(refs), a.spec.total)
		for _, r := range refs {
			fmt.Fprintf(&b, "    %s:%d  %-42s %s\n", r.rel, r.line, r.name, paths[r.name])
		}
		b.WriteString("\n")
	}

	if len(a.diags) > 0 {
		b.WriteString("  Secondary diagnostics - what the reference walk cannot resolve on its own:\n")
		diags := append([]diagnostic(nil), a.diags...)
		sort.Slice(diags, func(i, j int) bool {
			if diags[i].rel != diags[j].rel {
				return diags[i].rel < diags[j].rel
			}
			if diags[i].line != diags[j].line {
				return diags[i].line < diags[j].line
			}
			return diags[i].what < diags[j].what
		})
		for _, d := range diags {
			verb := "WARN parity"
			if a.cfg.Enforce {
				verb = "ERROR parity"
			}
			fmt.Fprintf(&b, "  %s: %s:%d %s\n", verb, d.rel, d.line, d.what)
		}
		b.WriteString("\n")
	}

	for _, p := range a.problems {
		fmt.Fprintf(&b, "  ERROR parity: %v\n", p.err)
		if p.hint != "" {
			fmt.Fprintf(&b, "       -> %s\n", p.hint)
		}
	}
	if len(a.problems) > 0 {
		b.WriteString("\n")
	}
	if a.measured && a.cfg.Enforce && open > 0 {
		fmt.Fprintf(&b, "  ERROR parity: %d declared endpoint(s) have no v-next service method; --enforce requires 0\n", open)
	}

	if a.measured {
		mode, enforce := "report", "off"
		if a.cfg.Enforce {
			mode, enforce = "enforce", "on"
		}
		fmt.Fprintf(&b, "  PARITY: %d/%d gap=%d mode=%s enforce=%s\n", wired, a.spec.total, open, mode, enforce)
	}

	switch {
	case !a.cfg.Enforce:
		// Unconditional in report mode: a green exit here means the gate
		// worked, not that the v-next layer is complete. The words PASS, OK,
		// SUCCESS and "all endpoints" appear only in an enforcing run that
		// found a zero gap, so a skimmed log cannot be read as partial success.
		b.WriteString("  A green exit in report mode does NOT mean v-next is at parity.\n")
		if code == 0 {
			b.WriteString("  exit=0 (report mode never fails on a gap)\n")
		} else {
			b.WriteString("  exit=1 (an integrity check failed; report mode never fails on a gap, but it does fail on a broken invariant)\n")
		}
	case code == 0:
		// The one place PASS is allowed, and it is keyed on the exit code rather
		// than on the gap for two reasons that only C14's contract separates. A
		// zero gap with an outstanding diagnostic is a failure, and printing PASS
		// there is the ERROR/PASS contradiction C14 exists to end. And a run that
		// could not measure has open == 0 too, so keying on the gap alone also
		// printed a PASS line on a run that measured nothing -- observed as
		// "PASS: 0/52" on a SPEC/code count mismatch, where the SPEC total is
		// known but the run returns before it can be compared.
		fmt.Fprintf(&b, "  PASS: %d/%d endpoints named by a v-next service; 0 NOT implemented\n", wired, a.spec.total)
		b.WriteString("  exit=0 (every declared endpoint is named by a v-next service)\n")
	default:
		reason := "a secondary diagnostic is unresolved; --enforce requires the reference walk to account for every route reference"
		switch {
		case len(a.problems) > 0:
			reason = "an integrity check failed; a run that cannot account for its own inputs is not a parity result"
		case open > 0:
			reason = "--enforce requires every declared endpoint to be named by a v-next service"
		}
		fmt.Fprintf(&b, "  exit=1 (%s)\n", reason)
	}
	return b.String(), code
}
