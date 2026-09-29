// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

type Price struct {
	dec  decimal.Decimal
	tick decimal.Decimal
}

func MustNewPrice(value string, tick string) Price {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("domain: Price: invalid decimal string %q: %v", value, err))
	}
	t, err := decimal.NewFromString(tick)
	if err != nil {
		panic(fmt.Sprintf("domain: Price: invalid tick string %q: %v", tick, err))
	}
	return Price{dec: dec, tick: t}
}

func (p Price) Decimal() decimal.Decimal { return p.dec }
func (p Price) Tick() decimal.Decimal    { return p.tick }

func (p Price) IsZero() bool     { return p.dec.IsZero() }
func (p Price) IsNegative() bool { return p.dec.IsNegative() }

func (p Price) Validate(step bool) error {
	if p.dec.IsNegative() {
		return fmt.Errorf("domain: Price: negative price %s", p.dec.String())
	}
	if step && !p.tick.IsZero() {
		remainder := p.dec.Mod(p.tick)
		if !remainder.IsZero() {
			return fmt.Errorf("domain: Price: %s is not a multiple of tick %s", p.dec.String(), p.tick.String())
		}
	}
	return nil
}

// ValidateOn checks p against an externally supplied tick rather than against the
// one the Price carries.
//
// It exists because Validate(step=true) is satisfied by the caller's own tick,
// which the caller chose and which therefore cannot reject anything the caller
// did not already rule out. Order validation must compare the price against the
// instrument's grid, and that grid belongs to the instrument, not to the order.
// See TickSchedule and design-tick-model.md §2.2.
//
// The sign check always runs. An empty tick, or one decimal cannot parse, means
// "unknown", and an unknown tick skips the step check rather than inventing a
// grid: substituting a default here would reintroduce the defect ValidateOn
// exists to remove. The returned error therefore means the price is invalid on
// its own terms, or is on a grid the caller named.
func (p Price) ValidateOn(tick string) error {
	if p.dec.IsNegative() {
		return fmt.Errorf("domain: Price: negative price %s", p.dec.String())
	}
	if tick == "" {
		return nil
	}
	grid, err := decimal.NewFromString(tick)
	if err != nil {
		return fmt.Errorf("domain: Price: invalid tick %q: %w", tick, err)
	}
	if grid.IsZero() || grid.IsNegative() {
		return nil
	}
	remainder := p.dec.Mod(grid)
	if !remainder.IsZero() {
		return fmt.Errorf("domain: Price: %s is not a multiple of tick %s", p.dec.String(), grid.String())
	}
	return nil
}

func (p Price) Round() Price {
	if p.tick.IsZero() {
		return p
	}
	return Price{dec: p.dec.Div(p.tick).Floor().Mul(p.tick), tick: p.tick}
}

func (p Price) String() string { return p.dec.String() }

func (p Price) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, p.dec.String())), nil
}

func (p *Price) UnmarshalJSON(data []byte) error {
	s := string(data)
	s = s[1 : len(s)-1]
	dec, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("domain: Price: UnmarshalJSON: invalid decimal %q: %w", s, err)
	}
	p.dec = dec
	return nil
}
