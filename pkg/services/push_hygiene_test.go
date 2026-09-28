// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// This is the key-hygiene guard for the push layer, and it is here for the same
// reason TestSessionDeclaresNoCredentialStorage is in session_test.go: gosec runs in
// CI and has rejected credential-shaped fields in this repository before, but a
// linter cannot tell a field that is only ever written with a ciphertext from one
// that is written with a plaintext, and it cannot tell a public key from a private
// one that happens to be stored beside it.
//
// So the source is parsed and the struct declarations are checked, for pkg/services
// and pkg/transport alike - the two packages the push seam spans, and the only two
// that can hold anything key-shaped on this path.
//
// Two rules, and they are different rules:
//
//   - No field may be named after a secret. There is no legitimate reason for one
//     on this path: the SDK holds no platform credentials and no developer private
//     key (AGENTS.md rule 4), and verification is a *public* key, which ADR 0005
//     records as reference data.
//   - A field whose name mentions a key must be unexported. A public key is not a
//     secret, so naming an exported field VerifyPublicKey would not be a leak - but
//     it would be a place where the next change puts a private key, and an exported
//     struct field is the only way to do that from outside the package. Keeping it
//     unexported makes key material reachable only through a constructor parameter,
//     which is what pkg/transport.WithPushVerification is.

// secretFieldNames may not appear as a struct field name anywhere on the push path.
// The match is exact, because "Password" as a substring would reject the word
// "PasswordPolicy" in prose-adjacent identifiers for no reason, and an exact list
// is what makes the rule readable in review.
var secretFieldNames = []string{
	"PrivateKey",
	"SecretKey",
	"Secret",
	"APIKey",
	"Credential",
	"Credentials",
	"Passphrase",
	"Salt",
	"Token",
	"Password",
}

// pushFilesAreTheTwoPackagesOnTheSeam names the files this guard reads. Both are
// parsed with the module root derived from this test's own location, so the guard
// does not depend on the working directory - the same rule internal/layering uses.
func TestPushFilesDeclareNoCredentialStorage(t *testing.T) {
	root := filepath.Clean(filepath.Join(mustGetwd(t), "..", ".."))
	targets := []string{
		filepath.Join(root, "pkg", "services", "push.go"),
		filepath.Join(root, "pkg", "transport", "push.go"),
	}

	sawPushConfig := false
	sawVerificationOption := false

	for _, path := range targets {
		name := strings.TrimPrefix(path, root+string(filepath.Separator))
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", name, err)
			}
			if file.Name.Name != "services" && file.Name.Name != "transport" {
				t.Fatalf("%s declares package %s, want services or transport: the guard is "+
					"reading the wrong file", name, file.Name.Name)
			}

			structs := 0
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok || st.Fields == nil {
						continue
					}
					structs++
					if ts.Name.Name == "PushConfig" {
						sawPushConfig = true
					}
					checkFields(t, name, ts.Name.Name, st)
				}
			}
			if structs == 0 {
				t.Fatalf("%s declares no struct type; the guard inspected nothing and every "+
					"assertion above would pass vacuously", name)
			}

			// The other half: key material has to enter through a parameter, and the
			// only one is the verification option. Naming it here is what stops a
			// second, unchecked way in from being added later.
			if file.Name.Name == "transport" && strings.Contains(string(source), "WithPushVerification") {
				sawVerificationOption = true
			}
		})
	}

	if !sawPushConfig {
		t.Error("no PushConfig struct was found; the rule that key material lives in an unexported " +
			"field of PushConfig has nothing to apply to")
	}
	if !sawVerificationOption {
		t.Error("pkg/transport/push.go no longer mentions WithPushVerification, so the one " +
			"checked way key material enters this package has gone")
	}
}

// checkFields applies both rules to one struct's fields.
func checkFields(t *testing.T, file, typeName string, st *ast.StructType) {
	t.Helper()
	for _, field := range st.Fields.List {
		for _, fieldName := range field.Names {
			name := fieldName.Name
			if name == "_" {
				continue
			}
			for _, banned := range secretFieldNames {
				if name == banned {
					t.Errorf("%s: %s declares a field named %s.\n"+
						"This package holds no platform credentials and no developer private key "+
						"(AGENTS.md rule 4). A public platform key is reference data, not a secret "+
						"(ADR 0005), and it belongs in a constructor parameter.", file, typeName, banned)
				}
			}
			if !strings.Contains(strings.ToLower(name), "key") {
				continue
			}
			if ast.IsExported(name) {
				t.Errorf("%s: %s declares an exported field named %s.\n"+
					"A public key is not a secret, so the name is not the problem - reachability is. "+
					"An exported struct field is the only way to assign key material from outside "+
					"the package, so keeping it unexported makes WithPushVerification the single "+
					"checked way in.", file, typeName, name)
			}
		}
	}
}

// TestTheVerificationSentinelIsTheDomainOne pins the other half of the key and
// boundary story: pkg/services' ErrSignatureMismatch is not a second sentinel but the
// domain one.
//
// The reasoning is §4.3's: a sentinel a caller is expected to branch on has to be
// nameable by that caller, and the one internal/push raises is not - `internal/` is
// unimportable outside this module. The adapter pairs the push sentinel with
// domain.ErrSignatureMismatch; pkg/services re-exports that same value so
// services.ErrSignatureMismatch and domain.ErrSignatureMismatch are one error. If a
// future change made them two, a caller matching one would silently stop matching
// the error the stream actually carries - which no other test here would notice.
func TestTheVerificationSentinelIsTheDomainOne(t *testing.T) {
	if ErrSignatureMismatch != domain.ErrSignatureMismatch {
		t.Errorf("services.ErrSignatureMismatch is not domain.ErrSignatureMismatch; " +
			"the adapter pairs the push sentinel with the domain one, so a services-local copy " +
			"would be an error the stream never carries")
	}
	if domain.ErrSignatureMismatch.Error() == "" {
		t.Error("domain.ErrSignatureMismatch has an empty message; a sentinel with no text cannot be " +
			"logged usefully")
	}
}

// mustGetwd returns the working directory, failing the test rather than returning an
// error a caller has to thread.
func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
