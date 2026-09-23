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
