// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package layering holds the machine-checked package boundary rules for this
// module. It contains no production code; the rules are asserted by a test that
// parses the repository's own imports.
//
// The rules live here rather than in .golangci.yml because golangci-lint's
// depguard linter cannot express them. In v2.9 depguard's `files` field honours
// only the `$all` and `$test` tokens: a path glob such as `pkg/hstong/**` is
// accepted without complaint and then matches nothing, so the rule passes
// forever while enforcing nothing. That failure mode is worse than having no
// rule, because it looks like enforcement. A test that parses imports has no
// such gap, and its failure can be verified by planting a violating import.
package layering

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePrefix = "github.com/shing1211/hstongapi4go/"

// skipDirs are never walked. gen/ is generated code, which AGENTS.md forbids
// editing, and cmd/ plus examples/ are entry points rather than library layers.
var skipDirs = map[string]bool{
	"gen":       true,
	".gitnexus": true,
	"site":      true,
}

// imports maps each package directory (slash-separated, relative to the module
// root) to the set of module-internal packages it imports.
type importSet map[string]map[string]bool

// loadImports walks the module and records, for every package, the
// module-internal packages it imports. Test files are included: a layering rule
// that holds for production code but is violated by a test still obscures the
// boundary, and internal/push already has tests in both package forms.
func loadImports(t *testing.T) importSet {
	t.Helper()
	root := moduleRoot(t)
	out := importSet{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			if skipDirs[rel] || strings.HasPrefix(rel, "scripts/") {
				return fs.SkipDir
			}
			if strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "examples/") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		pkg := filepath.ToSlash(rel)
		if pkg == "." {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		set, ok := out[pkg]
		if !ok {
			set = map[string]bool{}
			out[pkg] = set
		}
		for _, imp := range file.Imports {
			if imp.Path == nil {
				continue
			}
			p := strings.TrimPrefix(imp.Path.Value, `"`+modulePrefix)
			if p != imp.Path.Value && p != "" {
				p = strings.TrimSuffix(p, `"`)
				set[p] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no imports found; the walk is broken and every rule below would pass vacuously")
	}
	return out
}

// moduleRoot returns the repository root, derived from this test's location so
// the test does not depend on the working directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd is <root>/internal/layering
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// rule is one boundary assertion: packages matching prefix must not import
// any of forbidden.
type rule struct {
	name      string
	prefix    string
	forbidden []string
	why       string
}

var rules = []rule{
	{
		name:   "released surface does not reach the v-next layer",
		prefix: "pkg/hstong",
		forbidden: []string{
			"pkg/domain",
			"pkg/services",
			"pkg/transport",
		},
		why: "a caller using pkg/hstong/* must not be affected by a refactor of the " +
			"unwired v-next layer, and the two SDK implementations must not quietly merge",
	},
	{
		name:      "transport does not import services",
		prefix:    "pkg/transport",
		forbidden: []string{"pkg/services"},
		why: "ADR 0010 rule 6: the interface is declared in services and implemented " +
			"in transport, so this edge would invert the intended direction",
	},
	{
		name:   "the HTTP executor stays at the base of the stack",
		prefix: "internal/transport",
		forbidden: []string{
			"pkg/domain",
			"pkg/services",
			"pkg/transport",
			"pkg/hstong",
		},
		why: "ADR 0010: internal/transport is below every public layer, so it must " +
			"not depend on any of them",
	},
	{
		name:   "services reaches the Gateway only through client.Client",
		prefix: "pkg/services",
		forbidden: []string{
			"internal/transport",
			"internal/push",
		},
		why: "the indirection through client.Client is where the rate limiter, circuit " +
			"breakers, metrics, and tracing spans live; calling the executor directly " +
			"would bypass all of them (docs/VNEXT.md 5.4)",
	},
	{
		name:      "push does not call back up into the managers",
		prefix:    "internal/push",
		forbidden: []string{"pkg/hstong"},
		why: "the dependency runs from pkg/hstong/stream down to internal/push, never " +
			"the other way",
	},
	{
		name:      "domain does not depend on the wire or the released surface",
		prefix:    "pkg/domain",
		forbidden: []string{"client", "internal/", "pkg/hstong", "pkg/services", "pkg/transport"},
		why:       "ADR 0010: domain is the innermost layer and must know nothing above it",
	},
}

// TestLayeringRules asserts every boundary in the table above.
//
// Two properties are deliberate. The rule list is checked for being non-empty and
// the import walk is asserted to have found something, so a future refactor that
// breaks the walk cannot turn this into a test that passes because it inspected
// nothing. And the known deviation of pkg/domain importing gen/hq/dto is
// deliberately absent: it is real, recorded in ARCHITECTURE.md section 6, and
// encoding it here would fail every build until it is fixed.
func TestLayeringRules(t *testing.T) {
	if len(rules) == 0 {
		t.Fatal("no layering rules are defined; this test would pass vacuously")
	}
	imports := loadImports(t)

	for _, r := range rules {
		t.Run(r.name, func(t *testing.T) {
			var violations []string
			for pkg, deps := range imports {
				if pkg != r.prefix && !strings.HasPrefix(pkg, r.prefix+"/") {
					continue
				}
				for _, forbidden := range r.forbidden {
					matched := forbidden
					if strings.HasSuffix(forbidden, "/") {
						matched = strings.TrimSuffix(forbidden, "/")
					}
					for dep := range deps {
						if dep == matched || strings.HasPrefix(dep, matched+"/") {
							violations = append(violations, pkg+" imports "+dep)
						}
					}
				}
			}
			if len(violations) > 0 {
				sort.Strings(violations)
				t.Errorf("%s must not import %s, because %s.\nViolations:\n  %s",
					r.prefix, strings.Join(r.forbidden, ", "), r.why,
					strings.Join(violations, "\n  "))
			}
		})
	}
}

// TestLayeringRulesCoverTheKnownPackages guards against a rule silently matching
// no package at all, which is the failure mode that made a depguard-based version
// of this check useless: a rule whose prefix matches nothing passes forever.
// Each rule's prefix must correspond to at least one real package.
func TestLayeringRulesCoverTheKnownPackages(t *testing.T) {
	imports := loadImports(t)
	for _, r := range rules {
		found := false
		for pkg := range imports {
			if pkg == r.prefix || strings.HasPrefix(pkg, r.prefix+"/") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rule %q has prefix %q, which matches no package in the module; "+
				"it would pass without checking anything", r.name, r.prefix)
		}
	}
}

// TestExecutorInterfaceIsDeclaredByServices records the dependency inversion
// ADR 0010 rule 6 asks for, in the place where a reader will look for it.
// pkg/services must own the request-path interface rather than importing one,
// which is what lets the layer be tested without a Gateway.
func TestExecutorInterfaceIsDeclaredByServices(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join(root, "pkg", "services"), nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing pkg/services: %v", err)
	}

	found := false
	for name, pkg := range pkgs {
		for _, file := range pkg.Files {
			if !found {
				for _, decl := range file.Decls {
					if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
						for _, spec := range genDecl.Specs {
							if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == "Executor" {
								if _, isIface := ts.Type.(*ast.InterfaceType); isIface {
									found = true
								}
							}
						}
					}
				}
			}
		}
		_ = name
	}
	if !found {
		t.Error("pkg/services does not declare an Executor interface; the request path " +
			"must be declared in the dependent package (ADR 0010 rule 6)")
	}
}
