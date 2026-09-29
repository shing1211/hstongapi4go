// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// readPathFilesWithPrices are the read-path files whose constructors P5 retyped to
// the zero tick. They are named by path relative to pkg/services because the guard
// has to read source: a runtime assertion would need a fixture per response type,
// and 31 construction sites across five response types is the wrong place to
// maintain one.
//
// A regexp or a literal count would not do either. Parsing the AST and checking
// the tick argument of every constructor call is the one check that fails the
// moment a single site is written with a non-zero grid, and does not care how the
// value is spelled.
var readPathFilesWithPrices = []string{
	"market.go",
	"../transport/mappers.go",
	"../domain/account.go",
	"../domain/trading.go",
}

// p5ReadPathCtorSites is how many constructor calls the four files are expected to
// make. It is a floor, not an exact figure: a check that only asserted "no
// non-zero ticks" would pass just as happily on a file emptied of constructors,
// which is a different defect wearing the same green tick.
const p5ReadPathCtorSites = 31

// TestReadPathPricesCarryTheZeroTick is the guard for design-tick-model.md 2.1:
// a price decoded from the wire carries the zero tick, because "I observed this
// price; I do not know this instrument's tick schedule" is the truthful state and
// a zero tick states it in the type rather than in a comment.
//
// It exists because restoring "0.001" to any one of the 31 sites is a silent
// regression: Price.Round would then floor a faithful 0.0005 to 0.000 on its way
// through, and no other test noticed. Verified by mutation - this test is what
// killed four of them, where the per-response-type assertions had left the market
// read path unpinned entirely.
func TestReadPathPricesCarryTheZeroTick(t *testing.T) {
	fset := token.NewFileSet()
	total := 0

	for _, rel := range readPathFilesWithPrices {
		file, err := parser.ParseFile(fset, rel, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// The callee is a selector in pkg/services and pkg/transport
			// (domain.MustNewPrice) and a bare identifier inside pkg/domain, which
			// is the same package. Accepting only the selector form silently
			// skipped 7 of the 31 sites - the whole of account.go and trading.go -
			// and the count floor is what caught it.
			var name string
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			case *ast.Ident:
				name = fun.Name
			default:
				return true
			}
			if name != "MustNewPrice" {
				// Only MustNewPrice takes a tick. MustNewQuantity takes one
				// argument and MustNewMoney takes (value, currency, scale) - a
				// scale, not a grid - so neither has a tick argument to be wrong
				// about, and counting them here would flag their arity instead.
				return true
			}
			if len(call.Args) != 2 {
				t.Errorf("%s: MustNewPrice is called with %d arguments, want 2 "+
					"(value, tick). A tick that moved position or gained a parameter "+
					"is a change this guard can no longer read",
					fset.Position(call.Pos()), len(call.Args))
				return true
			}
			total++
			// The tick is the second argument, and it is always a string
			// literal - the repo's money-check enforces that.
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: the tick argument is %T, not a literal; this guard can "+
					"only read a literal, so the call has drifted from the shape it "+
					"assumes", fset.Position(call.Pos()), call.Args[1])
				return true
			}
			tick, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Errorf("%s: tick argument %s does not unquote: %v",
					fset.Position(lit.Pos()), lit.Value, err)
				return true
			}
			if tick != "0" {
				t.Errorf("%s: MustNewPrice is constructed with tick %q, want \"0\". A "+
					"read-path price must not claim a grid the Gateway never promised: "+
					"Price.Round would floor a faithful 0.0005 to 0.000, and "+
					"Validate(true) would reject it as off-tick",
					fset.Position(call.Pos()), tick)
			}
			return true
		})
	}

	if total < p5ReadPathCtorSites {
		t.Errorf("the four read-path files make %d constructor calls, want at least %d. "+
			"A drop means a field was removed and this guard stopped covering it, which "+
			"is a silent hole rather than a simplification - raise or lower the floor "+
			"deliberately, not by editing a price", total, p5ReadPathCtorSites)
	}
}

// TestNoNonZeroTickLiteralRemainsInTheReadPath is the blunt companion: it does not
// care which constructor a literal is attached to, so a future helper that takes a
// tick and is not one of the three above is still covered.
func TestNoNonZeroTickLiteralRemainsInTheReadPath(t *testing.T) {
	for _, rel := range readPathFilesWithPrices {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, rel, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if v == "0.001" {
				t.Errorf("%s: the literal \"0.001\" appears in a read-path file as code. "+
					"It is either a tick argument this guard has just explained is wrong, "+
					"or a value the Gateway legitimately sent - in which case it belongs "+
					"in a test fixture, not in the mapper", fset.Position(lit.Pos()))
			}
			return true
		})
	}
}
