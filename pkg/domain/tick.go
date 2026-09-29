// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Tick is a minimum price increment: a scale, not a price.
//
// It exists because a tick is not a price, and representing one as the other makes
// the type invent a grid the Gateway never promised. OrderBookResponse.TickSize
// used to be a domain.Price built as MustNewPrice(<the tick>, "0.001") - a price
// whose own tick was a different price's tick - so a caller validating it as a
// price got a spurious error on a faithful 0.0005 tick, and the two numbers
// necessarily disagreed whenever the instrument was not on a 0.001 grid.
// (docs/runs/2026-09-26-vnext-parity-wire/design-tick-model.md section 1)
//
// Validate and Round are deliberately not defined here. A tick is not on its own
// grid, so there is nothing for it to be a multiple of and nothing to round it
// to; defining either would reintroduce the same modelling error through a new
// door. The 0.0005 case therefore cannot fail validation, because no validation
// exists to fail.
//
// Whether the wire field this is read from is a tick or a bid-ask spread is
// unresolved by anything in this repository - the vendor's own field
// documentation says "minimum price unit" (tick) and the only fixture is equally
// consistent with both readings. The type is correct under either, and the
// decision that would discriminate is recorded in section 3.1 of the same note.
// No test asserts the semantics, because asserting an unverified belief in a test
// is how it becomes an unquestioned fact.
type Tick struct {
	dec decimal.Decimal
}

// MustNewTick returns the Tick for value, panicking on a value decimal cannot
// parse. It takes one argument, not two: a tick has no tick of its own, which is
// the whole reason this type exists.
func MustNewTick(value string) Tick {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("domain: Tick: invalid decimal string %q: %v", value, err))
	}
	return Tick{dec: dec}
}

// NewTick is MustNewTick without the panic, returning the error instead. It is
// for a wire field the caller has not yet checked, where a decode error should be
// reportable rather than fatal.
func NewTick(value string) (Tick, error) {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		return Tick{}, fmt.Errorf("domain: Tick: invalid decimal string %q: %w", value, err)
	}
	return Tick{dec: dec}, nil
}

func (t Tick) Decimal() decimal.Decimal { return t.dec }

func (t Tick) IsZero() bool     { return t.dec.IsZero() }
func (t Tick) IsNegative() bool { return t.dec.IsNegative() }

func (t Tick) String() string { return t.dec.String() }

func (t Tick) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, t.dec.String())), nil
}

func (t *Tick) UnmarshalJSON(data []byte) error {
	s := string(data)
	s = s[1 : len(s)-1]
	dec, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("domain: Tick: UnmarshalJSON: invalid decimal %q: %w", s, err)
	}
	t.dec = dec
	return nil
}
