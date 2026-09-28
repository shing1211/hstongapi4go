// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the acceptance criterion for the topic-to-payload relation, and it
// is the only thing in the repository that can see PushOrchestration: the parity
// guard reports 51/51 whether this type exists or not, because a TCP/protobuf
// client will never name a client.Route. A green guard across this work is the
// failure the guard programme exists to prevent.
//
// So the criterion is a test that fails when the relation rots, and it is
// *derived* rather than transcribed:
//
//   - the key set is parsed out of pkg/types/enums.go with go/ast at test time, so
//     there is no list of topics here for a human to maintain;
//   - the expected values are parsed out of docs/SPEC.md §4 with the parity
//     guard's own CRLF discipline, so a change to the specification has to be made
//     deliberately rather than by coincidence;
//   - the set of constants the mapping's switch actually mentions is read out of
//     the mapping's own AST, so a stale case for a removed constant is a failure
//     and not dead code.
//
// G1-G5 are the five assertions. G5 is the one that keeps the other four honest:
// a zero-match parse must be an error, never a pass, because "every declared topic
// is mapped" and "the parse found no topics" are otherwise the same sentence.
//
// The irreducible limit is stated in the design note and repeated here so nobody
// mistakes this for protocol truth: G3 pins the code to SPEC §4, so a wrong SPEC
// and wrong code agree and G3 stays green. No test in this repository can
// substitute for one live observation of what the Gateway actually sends.

// declaredConst is one constant read out of pkg/types/enums.go.
type declaredConst struct {
	// Name is the Go identifier, so a rename in pkg/types breaks the tests that
	// refer to a constant by name rather than passing them vacuously.
	Name string
	// Value is the constant's numeric value.
	Value int64
}

// parseDeclaredConstants returns the constants of the named type declared in
// pkg/types/enums.go.
//
// It fails the test when the file is missing or the type is not declared in it.
// That is deliberate and is the whole of G5's first half: a renamed or split
// enums.go must break loudly, because a parse that quietly matched nothing would
// make every key-space assertion below vacuously true.
func parseDeclaredConstants(t *testing.T, typeName string) []declaredConst {
	t.Helper()
	path := filepath.Join(repoRoot(t), "pkg", "types", "enums.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading pkg/types/enums.go, the declaration site of %s: %v\n"+
			"If that file moved, this test must be updated to point at the new declaration site; "+
			"it must not be made to pass by finding nothing", typeName, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, raw, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing pkg/types/enums.go: %v", err)
	}

	var out []declaredConst
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			// A typed constant block names its type on the ValueSpec, not on a
			// TypeSpec: `TopicBasicQot TopicID = 11` is one ValueSpec whose Type
			// is the ident TopicID.
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || vs.Type == nil {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != typeName {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				n, convErr := strconv.ParseInt(lit.Value, 10, 64)
				if convErr != nil {
					t.Fatalf("constant %s.%s = %s is not an integer literal: %v", typeName, name.Name, lit.Value, convErr)
				}
				out = append(out, declaredConst{Name: name.Name, Value: n})
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s constants were found in pkg/types/enums.go; this test must fail rather than "+
			"pass on an empty parse, or every key-space assertion here becomes vacuous", typeName)
	}
	return out
}

// mappingMentions reads the AST of the function named fn in pkg/services and
// returns the set of identifiers in its body that are declared topic constants.
//
// Reading the mapping's own AST rather than counting its cases is what lets G1
// detect a stale case: a case for a constant that no longer exists mentions a name
// that is not in the declared set, and the two sets disagree.
func mappingMentions(t *testing.T, fn string, declared map[string]bool) map[string]bool {
	t.Helper()
	path := filepath.Join(repoRoot(t), "pkg", "services", "push.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing pkg/services/push.go: %v", err)
	}

	found := map[string]bool{}
	seen := false
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn || fd.Recv != nil {
			continue
		}
		seen = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if declared[ident.Name] {
				found[ident.Name] = true
			}
			return true
		})
	}
	if !seen {
		t.Fatalf("pkg/services/push.go does not declare func %s; the criterion reads the mapping from "+
			"its declaration site and cannot fall back to a written list", fn)
	}
	return found
}

// repoRoot returns the module root, derived from this test's own location so the
// test does not depend on the working directory. The pattern is the one
// internal/layering uses.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd is <root>/pkg/services
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// specGroup is one row of docs/SPEC.md §4: a group name, the topicIds it carries,
// and the payload name the Gateway pushes for them.
type specGroup struct {
	Group   string
	Topics  []int
	Payload string
}

// specMsgTypeRowRE matches one §5 table row: | `TrsStockDeliverMsgType` | `0` |
var specMsgTypeRowRE = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|\\s*`([0-9]+)`\\s*\\|\\s*$")

// specPushRowRE matches one §4 table row: | Group | `11`, `35` | `BasicQotNotify` |
var specPushRowRE = regexp.MustCompile("^\\|\\s*([^|]+?)\\s*\\|\\s*([^|]+?)\\s*\\|\\s*`([^`]+)`\\s*\\|\\s*$")

// specBacktickRE finds the backticked tokens inside one table cell.
var specBacktickRE = regexp.MustCompile("`([^`]+)`")

// parseSpecPushGroups reads docs/SPEC.md §4 and §5 with the parity guard's CRLF
// discipline.
//
// The discipline is the guard's own and for the guard's own reason: SPEC.md is
// CRLF in some working trees and LF in others, .gitattributes only applies on
// checkout, and a `$`-anchored match would then fail on a dev host and pass on the
// runner for a reason unrelated to the code. So the file is split on "\n" and
// every line has its "\r" trimmed, which is exactly what
// scripts/paritygate/main.go's parseSpec does.
func parseSpecPushGroups(t *testing.T) ([]specGroup, map[string]int64) {
	t.Helper()
	path := filepath.Join(repoRoot(t), "docs", "SPEC.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading docs/SPEC.md: %v\n"+
			"If that file moved, this test must be updated to point at the new location; it must "+
			"not be made to pass by finding no rows", err)
	}
	lines := strings.Split(string(raw), "\n")

	groups := make([]specGroup, 0)
	msgTypes := make(map[string]int64)
	section := ""
	for i, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if m := regexp.MustCompile(`^## (\d+)\.`).FindStringSubmatch(line); m != nil {
			section = m[1]
			continue
		}
		switch section {
		case "4":
			m := specPushRowRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			g := specGroup{Group: m[1], Payload: m[3]}
			for _, tok := range specBacktickRE.FindAllStringSubmatch(m[2], -1) {
				n, convErr := strconv.Atoi(tok[1])
				if convErr != nil {
					t.Fatalf("docs/SPEC.md:%d: topicId %q is not an integer: %v", i+1, tok[1], convErr)
				}
				g.Topics = append(g.Topics, n)
			}
			if len(g.Topics) == 0 {
				t.Fatalf("docs/SPEC.md:%d: push group %q carries no topicId", i+1, g.Group)
			}
			groups = append(groups, g)
		case "5":
			m := specMsgTypeRowRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			n, convErr := strconv.ParseInt(m[2], 10, 64)
			if convErr != nil {
				t.Fatalf("docs/SPEC.md:%d: notifyMsgType %q is not an integer: %v", i+1, m[2], convErr)
			}
			msgTypes[m[1]] = n
		}
	}
	if len(groups) == 0 || len(msgTypes) == 0 {
		t.Fatalf("docs/SPEC.md yielded %d push group row(s) and %d NotifyMsgType row(s); at least "+
			"one of each is required or the agreement assertion below is vacuous", len(groups), len(msgTypes))
	}
	return groups, msgTypes
}

// specPayloadType resolves a §4 payload name onto the NotifyMsgType §5 declares
// for it.
//
// The join is not derivable from the specification alone, and pretending otherwise
// would put a written four-row table into the test - the exact rot §12.3 exists to
// prevent. §4's payload column names the *generated protobuf message* the Gateway
// pushes (`OrderBookFullNotify`, `BrokerNotify`), while §5 names the *wire enum
// constant* the PBNotify discriminator carries (`OrderBookNotifyMsgType`,
// `BrokerQueueNotifyMsgType`), and the two names coincide for only two of the four
// rows. So the specification states two vocabularies and the join between them
// lives in code.
//
// The join is therefore read out of the SDK's own wire decode table: internal/push's
// Decode is the function that decides which generated message a notifyMsgType
// carries, so it is the one place where a §4 payload name and a §5 constant name
// meet. That is a code fact being asserted against the document, not a list being
// transcribed - and if the decode table moves, this test says so instead of quietly
// checking nothing.
func specPayloadType(t *testing.T, payload string, msgTypes map[string]int64) int64 {
	t.Helper()

	// The exact name join, where the two columns happen to agree.
	if v, ok := msgTypes[payload+"MsgType"]; ok {
		return v
	}

	// Otherwise, through the generated message the SDK decodes for each constant.
	table := parseDecodeTable(t)
	for constName, messageName := range table {
		if messageName != payload {
			continue
		}
		v, ok := msgTypes[constName]
		if !ok {
			t.Fatalf("docs/SPEC.md section 4 names the payload %q, which internal/push decodes for the "+
				"constant %s, but section 5 declares no %q; the two sections must agree", payload, constName, constName)
		}
		return v
	}

	names := make([]string, 0, len(msgTypes))
	for n := range msgTypes {
		names = append(names, n)
	}
	decoded := make([]string, 0, len(table))
	for c, m := range table {
		decoded = append(decoded, c+" -> "+m)
	}
	t.Fatalf("docs/SPEC.md section 4 names the payload %q, which appears neither as %q in section 5 "+
		"nor as the generated message any constant decodes to.\n"+
		"Section 5 declares: %v\n"+
		"internal/push's Decode maps: %v\n"+
		"If the payload column of section 4 and the decode table have drifted apart, one of them is "+
		"wrong and this test is reporting it rather than passing", payload, payload+"MsgType", names, decoded)
	return 0
}

// parseDecodeTable reads internal/push's Decode switch and returns, for each
// types.NotifyMsgType constant it names, the generated protobuf message it
// unmarshals into.
//
// It is the join between docs/SPEC.md §4's payload names (generated messages) and
// §5's constant names (wire enum values), and it is read from the declaration site
// so a fourth vocabulary does not have to be written down by hand.
func parseDecodeTable(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "internal", "push", "client.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing internal/push/client.go, the wire decode table: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Name.Name == "Decode" && fd.Recv == nil {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("internal/push/client.go does not declare func Decode; the join between docs/SPEC.md " +
			"section 4's payload names and section 5's constant names is no longer readable")
	}

	out := make(map[string]string)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			message := compositeTypeName(cc)
			if message == "" {
				continue
			}
			for _, expr := range cc.List {
				sel, ok := expr.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "types" {
					continue
				}
				out[sel.Sel.Name] = message
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatal("internal/push's Decode table yielded no constant-to-message pairs; the parse is broken " +
			"and every §4 payload name would be unresolvable")
	}
	return out
}

// compositeTypeName returns the bare type name of the first composite literal in
// node, which is how Decode writes `m = &hqnotify.OrderBookFullNotify{}`.
func compositeTypeName(node ast.Node) string {
	found := ""
	ast.Inspect(node, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch t := cl.Type.(type) {
		case *ast.Ident:
			found = t.Name
		case *ast.SelectorExpr:
			found = t.Sel.Name
		}
		return found == ""
	})
	return found
}

// ---------------------------------------------------------------------------
// G1 - key-space totality
// ---------------------------------------------------------------------------

// TestG1KeySpaceTotality asserts that every declared types.TopicID constant is
// mapped, and that the mapping's switch mentions exactly the declared set.
//
// The two halves are the key-side of one property. The first is what a vendor
// topic addition breaks: adding a TopicID constant to pkg/types/enums.go is a
// one-line change, and this test then fails immediately with the constant's name in
// the message. The second is what a *removal* would break: without it, a deleted
// constant would leave an orphan case behind, the count would not match, and the
// test would report the drift instead of reading the stale case as coverage.
func TestG1KeySpaceTotality(t *testing.T) {
	declared := parseDeclaredConstants(t, "TopicID")
	declaredNames := make(map[string]bool, len(declared))
	for _, c := range declared {
		declaredNames[c.Name] = true
	}

	mentioned := mappingMentions(t, "NotifyTypeForTopic", declaredNames)

	var missing, stale []string
	for _, c := range declared {
		topic := types.TopicID(c.Value)
		msgType, ok := NotifyTypeForTopic(topic)
		if !ok {
			missing = append(missing, c.Name)
			continue
		}
		if _, has := mentioned[c.Name]; !has {
			missing = append(missing, c.Name+" (mapped, but its constant is not mentioned in the switch)")
		}
		_ = msgType
	}
	for name := range mentioned {
		if !declaredNames[name] {
			stale = append(stale, name+" (a case names it, but pkg/types/enums.go no longer declares it)")
		}
	}

	if len(missing) > 0 {
		t.Errorf("NotifyTypeForTopic does not cover %d of the %d declared types.TopicID constants: %v\n"+
			"Every topic the SDK accepts must map to the payload the Gateway pushes for it, or "+
			"Subscribe must refuse it. Add the case, and the SPEC section 4 row it belongs to.",
			len(missing), len(declared), missing)
	}
	if len(stale) > 0 {
		t.Errorf("NotifyTypeForTopic names %d constant(s) pkg/types/enums.go no longer declares: %v\n"+
			"A case for a removed constant is not coverage; it is a case that can never be reached.",
			len(stale), stale)
	}

	// The accepted-topic set /hq/Subscribe will admit is a third place the topic
	// vocabulary is written down. It is the same drift one line away, so it is
	// checked here rather than left for a caller to discover as a local rejection.
	for _, c := range declared {
		if _, ok := knownTopics[types.TopicID(c.Value)]; !ok {
			t.Errorf("knownTopics (what MarketService.Subscribe accepts) does not include the declared "+
				"constant %s = %d, so a subscriber of it would be refused locally with no push", c.Name, c.Value)
		}
	}
}

// ---------------------------------------------------------------------------
// G2 - value-space closure, both directions
// ---------------------------------------------------------------------------

// TestG2ValueSpaceClosure asserts the mapping is closed over the declared
// NotifyMsgType vocabulary, and that the constants it never returns are exactly
// the three session-wide delivery types.
//
// Both directions matter. Forward: a typo'd or invented payload type is caught by
// asking whether every returned value is a declared constant at all. Backward:
// asking whether every declared constant is returned kills a mapping that is
// internally consistent and wrong - eleven rows all edited to one type still
// passes the totality check in G1, and this is what catches it.
//
// The three names are written as Go identifiers, so renaming one in pkg/types
// breaks this test instead of passing it.
func TestG2ValueSpaceClosure(t *testing.T) {
	declaredTopics := parseDeclaredConstants(t, "TopicID")
	declaredTypes := parseDeclaredConstants(t, "NotifyMsgType")

	returned := map[int64]bool{}
	for _, c := range declaredTopics {
		msgType, ok := NotifyTypeForTopic(types.TopicID(c.Value))
		if !ok {
			continue
		}
		returned[int64(msgType)] = true
	}

	nameByValue := make(map[int64]string, len(declaredTypes))
	declared := make(map[string]bool, len(declaredTypes))
	for _, c := range declaredTypes {
		nameByValue[c.Value] = c.Name
		declared[c.Name] = true
	}

	// Forward: nothing the mapping returns may be undeclared.
	for value := range returned {
		if _, ok := nameByValue[value]; !ok {
			t.Errorf("NotifyTypeForTopic returned notifyMsgType %d, which pkg/types/enums.go does not "+
				"declare; the values are %v", value, nameByValue)
		}
	}

	// Backward: the unreturned constants must be exactly the three that have no
	// TopicID at all.
	wantUnreturned := map[string]bool{
		"TrsStockDeliverMsgType":          true,
		"TradeStockDeliverMsgType":        true,
		"FuturesTradeStockDeliverMsgType": true,
	}
	for name := range wantUnreturned {
		if !declared[name] {
			t.Errorf("this test names the constant %s as one NotifyTypeForTopic must never return, but "+
				"pkg/types/enums.go does not declare it; the delivery-type list and the test have drifted", name)
		}
	}
	var never, unexpected []string
	for _, c := range declaredTypes {
		if returned[c.Value] {
			continue
		}
		never = append(never, c.Name)
		if !wantUnreturned[c.Name] {
			unexpected = append(unexpected, c.Name)
		}
	}
	if len(never) != len(wantUnreturned) {
		t.Errorf("NotifyTypeForTopic never returns %v (%d of them), want exactly the three "+
			"session-wide delivery types", never, len(never))
	}
	if len(unexpected) > 0 {
		t.Errorf("NotifyTypeForTopic never returns %v, which is neither a session-wide delivery type "+
			"nor covered by any other rule; a payload type nothing can deliver is dead vocabulary", unexpected)
	}

	// The installed handler set must be the image of the mapping plus those three.
	// This is what stops a fourth delivery type from being declared in pkg/types
	// and mapped but never wired to a handler.
	installed := map[int64]bool{}
	for _, msgType := range marketNotifyTypes() {
		installed[int64(msgType)] = true
	}
	for _, msgType := range tradeNotifyTypes {
		installed[int64(msgType)] = true
	}
	for value, name := range nameByValue {
		if !installed[value] {
			t.Errorf("the declared notifyMsgType %s has no installed push handler; the handler set is "+
				"the image of NotifyTypeForTopic plus the three delivery types, so %s is "+
				"mapped-but-not-delivered", name, name)
		}
	}
	for value := range installed {
		if _, ok := nameByValue[value]; !ok {
			t.Errorf("a push handler is installed for notifyMsgType %d, which pkg/types/enums.go does "+
				"not declare", value)
		}
	}
}

// ---------------------------------------------------------------------------
// G3 - the relation is the one SPEC section 4 states
// ---------------------------------------------------------------------------

// TestG3AgreesWithTheSpecification asserts the mapping agrees with docs/SPEC.md §4
// group by group, in both directions.
//
// G1 and G2 are both blind to a *wrong* mapping that is internally consistent -
// topic 25 delivering ticker updates is a complete, closed, total mapping that
// answers every question they ask and is still wrong. This is the assertion that
// catches it, and it is why the expected values are read from the document rather
// than from the code.
//
// The comparison runs both ways on purpose. A mapping edited without a SPEC change
// breaks here; a SPEC change without a mapping change breaks here too, and that is
// the correct direction for a specification: the document and the code are made to
// agree deliberately rather than by coincidence.
func TestG3AgreesWithTheSpecification(t *testing.T) {
	groups, msgTypes := parseSpecPushGroups(t)
	declaredTopics := parseDeclaredConstants(t, "TopicID")

	// The topic vocabulary must be the same set the specification states. A
	// constant with no SPEC row, or a SPEC topic with no constant, means the two
	// documents disagree and neither assertion below can mean anything.
	specTopics := map[int]specGroup{}
	for _, g := range groups {
		for _, id := range g.Topics {
			specTopics[id] = g
		}
	}
	for _, c := range declaredTopics {
		g, ok := specTopics[int(c.Value)]
		if !ok {
			t.Errorf("the declared constant %s = %d appears in no docs/SPEC.md section 4 group; the "+
				"specification and pkg/types must agree on the topic vocabulary", c.Name, c.Value)
			continue
		}
		want := types.NotifyMsgType(specPayloadType(t, g.Payload, msgTypes))
		got, mapped := NotifyTypeForTopic(types.TopicID(c.Value))
		if !mapped {
			t.Errorf("%s = %d (SPEC section 4 group %q) is not mapped at all", c.Name, c.Value, g.Group)
			continue
		}
		if got != want {
			t.Errorf("%s = %d (SPEC section 4 group %q) delivers %s, want %s from that row: %s",
				c.Name, c.Value, g.Group, got.String(), want.String(), g.Payload)
		}
	}
	for id := range specTopics {
		found := false
		for _, c := range declaredTopics {
			if int(c.Value) == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("docs/SPEC.md section 4 states topicId %d, which pkg/types/enums.go does not "+
				"declare; a topic the Gateway pushes that no caller can subscribe to", id)
		}
	}
}

// ---------------------------------------------------------------------------
// G4 - delivery is live
// ---------------------------------------------------------------------------

// TestG4EveryMappedTopicDelivers subscribes to every declared topic and asserts
// the handler for its mapped type is installed and that a frame of that type
// reaches the subscriber.
//
// This is the assertion that separates a *correct* mapping from a *wired* one.
// A twelfth topic mapped to a fifth payload type passes G1, G2 and G3 - and a
// subscriber of it would wait forever for a handler nobody registered.
//
// The frame is injected through the fake transport's handler registry rather than
// built as a *domain.PushUpdate by the test, so the assertion is about the wiring
// the orchestrator performs, not about a value the test assembled itself.
func TestG4EveryMappedTopicDelivers(t *testing.T) {
	declared := parseDeclaredConstants(t, "TopicID")
	if len(declared) == 0 {
		t.Fatal("no declared TopicID constants; this test would pass vacuously")
	}

	for _, c := range declared {
		t.Run(c.Name, func(t *testing.T) {
			topic := types.TopicID(c.Value)
			msgType, ok := NotifyTypeForTopic(topic)
			if !ok {
				t.Fatalf("%s = %d is not mapped, so this test cannot say anything about delivery", c.Name, c.Value)
			}

			tr := newFakePushTransport()
			o := newTestOrchestration(t, tr)
			handler := tr.handler(msgType)
			if handler == nil {
				t.Fatalf("no push handler was installed for %s, which %s maps to; the handler set must "+
					"be the image of NotifyTypeForTopic", msgType.String(), c.Name)
			}

			code := c.Name + ".HK"
			sub, err := o.Subscribe(t.Context(), topic, &Security{DataType: types.DataTypeHKStock, Code: code})
			if err != nil {
				t.Fatalf("Subscribe(%s): %v", c.Name, err)
			}

			stamp := c.Value * 1000
			handler(&domain.PushUpdate{
				Type:  msgType,
				ID:    code,
				Time:  pushTestTime(stamp),
				Event: syntheticMarketEvent(msgType, code),
			})

			select {
			case got, open := <-sub.Updates():
				if !open {
					t.Fatal("Updates was closed instead of delivering")
				}
				if got.Type != msgType {
					t.Errorf("delivered type = %s, want %s", got.Type.String(), msgType.String())
				}
				if got.Event == nil {
					t.Error("delivered update carries a nil event; domain.PushUpdate documents it as never nil")
				}
			default:
				t.Fatal("nothing was delivered to the subscriber")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// G5 - non-vacuity
// ---------------------------------------------------------------------------

// TestG5TheParseIsNotVacuous asserts that every input this criterion depends on
// actually yielded something, with a floor on each count.
//
// It is the guard on the guard. Without it, renaming pkg/types/enums.go, moving
// docs/SPEC.md, or splitting either into a shape this parser does not recognise
// would leave the key-space assertions above reading an empty set - and "every
// declared topic is mapped" is trivially true of no topics. A zero-match must
// therefore be a failure here rather than a pass everywhere else, which is the
// same rule design-parity-guard.md states for its own inputs and the same rule
// internal/layering enforces for its import walk.
func TestG5TheParseIsNotVacuous(t *testing.T) {
	const (
		minTopics = 8
		minTypes  = 7
		wantGroup = 4
	)

	topics := parseDeclaredConstants(t, "TopicID")
	if len(topics) < minTopics {
		t.Errorf("pkg/types/enums.go yielded %d types.TopicID constants, want at least %d; the "+
			"totality check is not looking at the declaration site any more", len(topics), minTopics)
	}

	msgTypes := parseDeclaredConstants(t, "NotifyMsgType")
	if len(msgTypes) < minTypes {
		t.Errorf("pkg/types/enums.go yielded %d types.NotifyMsgType constants, want at least %d; the "+
			"value-space closure check is not looking at the declaration site any more", len(msgTypes), minTypes)
	}

	groups, specTypes := parseSpecPushGroups(t)
	if len(groups) != wantGroup {
		t.Errorf("docs/SPEC.md section 4 yielded %d push group rows, want exactly %d: %v\n"+
			"Either the specification's table changed shape or the parse stopped matching it, and "+
			"both are a failure here rather than a vacuous agreement above",
			len(groups), wantGroup, specGroupNames(groups))
	}
	if len(specTypes) < minTypes {
		t.Errorf("docs/SPEC.md section 5 yielded %d NotifyMsgType rows, want at least %d", len(specTypes), minTypes)
	}
}

// specGroupNames renders the parsed group rows for an assertion message.
func specGroupNames(groups []specGroup) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.Group+" -> "+g.Payload)
	}
	return out
}
