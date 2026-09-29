// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package layering

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The two documents this file keeps honest, and the one claim each carries that
// nothing in the build depends on.
const (
	authDocFile   = "internal/auth/doc.go"
	registerFile  = "docs/threat-model.md"
	registerTable = "## Risk register"
)

// callerlessRE marks the phrase that discloses a method with no non-test caller.
var callerlessRE = regexp.MustCompile(
	`(?:no caller outside tests|no non-test caller|nothing calls [A-Za-z0-9_.]+)`)

// It is the sentence, not the line, that carries the claim: a disclosure names
// its symbols first and puts the phrase on the clause that governs them, often
// several comment lines later. The sentence is therefore recovered by comment
// lines, not by a single regex over the raw text - and then narrowed to the clause
// that the phrase governs, because a disclosure sentence here also names methods
// it is *not* disclosing ("Session.ShouldRefresh has exactly one").
var (
	commentLineRE = regexp.MustCompile(`(?m)^// ?(.*)$`)
	sentenceRE    = regexp.MustCompile(`[^.!?]*[.]`)
	methodNameRE  = regexp.MustCompile(
		`(?:TokenManager|Session|Authenticator)\.([A-Z][A-Za-z0-9_]*)` +
			`|\b([a-z][A-Za-z0-9_]*[A-Z][A-Za-z0-9_]*)\b`)
)

// symbolRE pulls the method names out of a disclosure sentence.
//
// fixedCommitRE captures the hash a register row cites for its status. A row that
// resolves by removal has no commit, because nothing changed in the code; a row
// marked fixed has one, and a commit that exists without being cited is how a
// stale status presents.
var fixedCommitRE = regexp.MustCompile("`([0-9a-f]{7,40})`")

// TestAuthDocCallerClaimsAreAccurate recomputes the two claims that kept going
// quietly false while the code moved underneath them.
//
// The first is internal/auth/doc.go's disclosure that certain methods have no
// caller outside tests. That claim became false in two directions: a commit added
// coalescing to Authenticator.Login and then a caller for Session.ShouldRefresh in
// pkg/services, while the disclosure kept saying nothing called it. The second is
// the register's habit of citing a commit for a fixed row; it stayed stale for
// four days because a status is prose and nothing checks it.
//
// The checks are narrow on purpose. Modelling "no caller" as "the identifier
// appears in no non-test Go file" would fail on the doc comment that makes the
// claim and on the test files that exercise the method, so this resolves call
// expressions by AST instead of grepping text, and excludes the documenting file
// itself. A check that cannot be satisfied is one that gets deleted.
func TestAuthDocCallerClaimsAreAccurate(t *testing.T) {
	t.Run("doc.go discloses only methods with no production caller", testDocCallerClaims)
	t.Run("every fixed register row cites a commit that exists", testFixedRowCommits)
}

// testDocCallerClaims verifies each method doc.go names as caller-less really is
// uncalled outside its own tests and its own documentation.
func testDocCallerClaims(t *testing.T) {
	root := moduleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, authDocFile))
	if err != nil {
		t.Fatalf("reading %s: %v", authDocFile, err)
	}

	claims := callerlessClaims(string(raw))
	if len(claims) == 0 {
		t.Fatalf("no caller-less disclosure found in %s; the wording changed, so this "+
			"test no longer checks the claim. Re-word it in a form callerlessRE matches, "+
			"or update the pattern deliberately", authDocFile)
	}

	for _, name := range claims {
		if callers := nonTestCallers(t, root, name); len(callers) != 0 {
			t.Errorf("doc.go discloses %s as having no caller outside tests, but "+
				"these non-test files call it: %s", name, strings.Join(callers, ", "))
		}
	}
}

// callerlessClaims returns the methods doc.go discloses as having no non-test
// caller.
//
// A disclosure sentence names the methods it discloses and then governs them with
// the phrase, so only the text preceding the phrase is read for names. The rest of
// the sentence is a different claim about different methods - "Session.ShouldRefresh
// has exactly one" - and reading it would turn a true disclosure into three false
// failures.
func callerlessClaims(doc string) []string {
	// Comment lines carry the sentence across hard wraps, so rejoin them before
	// looking for sentence boundaries.
	var b strings.Builder
	for _, m := range commentLineRE.FindAllStringSubmatch(doc, -1) {
		b.WriteString(m[1])
		b.WriteString(" ")
	}
	prose := b.String()

	seen := map[string]bool{}
	var out []string
	for _, sentence := range sentenceRE.FindAllString(prose, -1) {
		loc := callerlessRE.FindStringIndex(sentence)
		if loc == nil {
			continue
		}
		for _, m := range methodNameRE.FindAllStringSubmatch(sentence[:loc[0]], -1) {
			for _, g := range m[1:] {
				if g == "" || seen[g] {
					continue
				}
				seen[g] = true
				out = append(out, g)
			}
		}
	}
	return out
}

// nonTestCallers returns the repository-relative files that call name, excluding
// test files, doc comments, and the file that makes the claim.
//
// Directories are skipped on the same grounds architecture_doc_test.go skips them:
// test, cmd, examples, docs, site, scripts, and internal/migration, the last
// because it exists only to be compiled and is not SDK code.
func nonTestCallers(t *testing.T, root, name string) []string {
	t.Helper()
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
			switch rel {
			case "test", "cmd", "examples", "docs", "site", "internal/migration":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(rel, "_test.go") ||
			rel == authDocFile {
			return nil
		}
		found, parseErr := fileCallsName(path, name)
		if parseErr != nil {
			return parseErr
		}
		if found {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	return out
}

// fileCallsName reports whether path contains a call expression whose function is
// a selector on name, or a plain call to name.
//
// Matching on call syntax rather than on the identifier keeps a method that is
// only ever mentioned - in a comment, in a doc string, in prose - from counting as
// a caller, which is precisely the distinction the disclosure is making.
func fileCallsName(path, name string) (bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return false, err
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel != nil && fn.Sel.Name == name {
				found = true
			}
		case *ast.Ident:
			if fn.Name == name {
				found = true
			}
		}
		return true
	})
	return found, nil
}

// testFixedRowCommits verifies that every hash a resolved register row cites is a
// commit this repository actually has, so a row cannot claim a fix that was never
// committed and a committed fix cannot sit uncited in the register.
func testFixedRowCommits(t *testing.T) {
	root := moduleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, registerFile))
	if err != nil {
		t.Fatalf("reading %s: %v", registerFile, err)
	}

	section := sectionBody(t, string(raw), registerTable)
	if section == "" {
		t.Fatalf("%s section %q not found; the heading changed, so this test no "+
			"longer reads the register", registerFile, registerTable)
	}

	// A shallow clone cannot resolve a historical commit, and CI checks out
	// with actions/checkout's default depth. Asking git whether an old hash
	// exists would then fail on citations that are perfectly correct, so the
	// check only applies where the object is actually reachable from HEAD.
	shallow := repoIsShallow(root)

	rows := 0
	var skipped int
	for _, row := range tableRows(section) {
		// The header row and the separator carry no hash, so counting rows that
		// cite a commit is enough.
		hashes := fixedCommitRE.FindAllStringSubmatch(row, -1)
		if len(hashes) == 0 {
			continue
		}
		rows++
		for _, h := range hashes {
			hash := h[1]
			switch commitReachability(root, hash) {
			case commitPresent:
			case commitAbsent:
				t.Errorf("register row cites commit %s, which is not in this "+
					"repository: %s", hash, truncate(row, 100))
			case commitUnreachable:
				// This clone is too shallow to resolve it, which says nothing about
				// whether the citation is correct. Not a failure, and not silent
				// either: the count is reported below so a run that verified less
				// than it looks is visible rather than indistinguishable from a
				// full pass.
				skipped++
			}
		}
	}
	if rows == 0 {
		t.Fatalf("no register row cites a commit; either the citations were " +
			"dropped or the table format changed, and this test is no longer checking anything")
	}
	if shallow && skipped > 0 {
		t.Logf("shallow clone: %d of %d citations could not be resolved and were "+
			"not checked; fetch full history to verify them", skipped, rows)
	}
}

// commitState is what a repository can say about a cited hash.
type commitState int

const (
	// commitPresent means the object exists and is a commit.
	commitPresent commitState = iota
	// commitAbsent means the repository does not contain it at all, which is the
	// case a citation error produces.
	commitAbsent
	// commitUnreachable means the object is not visible from this clone. A
	// shallow clone reports this for any commit outside its depth window, and it
	// is not evidence that the citation is wrong.
	commitUnreachable
)

// commitReachability classifies a cited hash without treating "this clone cannot
// see it" as "it does not exist".
//
// The two failures are indistinguishable from git alone: in a shallow clone, a
// citation that predates the fetched window and a citation that names nothing both
// produce "fatal: Needed a single revision" and exit 128. So the clone's own
// history boundary decides. A full clone is authoritative and reports absence
// as absence; a shallow clone cannot be, and reports anything it cannot resolve as
// unreachable instead of guessing.
//
// `^{commit}` is appended so a tag or a tree is not accepted where a commit is
// required.
func commitReachability(root, hash string) commitState {
	if commitResolvable(root, hash) {
		return commitPresent
	}
	if repoIsShallow(root) {
		return commitUnreachable
	}
	return commitAbsent
}

// commitResolvable reports whether hash names a commit visible from this clone.
func commitResolvable(root, hash string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", hash+"^{commit}")
	cmd.Dir = root
	return cmd.Run() == nil
}

// repoIsShallow reports whether root is a shallow clone, where citations older
// than the fetched window cannot be resolved.
func repoIsShallow(root string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-shallow-repository")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// truncate shortens a table row for a failure message.
func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
