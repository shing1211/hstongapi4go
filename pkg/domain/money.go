// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

const defaultHKDScale = 3
const defaultCNYScale = 2

type Money struct {
	dec      decimal.Decimal
	currency string
	scale    uint8
}

func MustNewMoney(value string, currency string, scale uint8) Money {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("domain: Money: invalid decimal string %q: %v", value, err))
	}
	return Money{dec: dec, currency: currency, scale: scale}
}

func MustNewMoneyWithScale(value string, currency string, scale uint8) Money {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("domain: Money: invalid decimal string %q: %v", value, err))
	}
	dec = dec.Round(int32(scale))
	return Money{dec: dec, currency: currency, scale: scale}
}

func (m Money) Decimal() decimal.Decimal { return m.dec }
func (m Money) Currency() string         { return m.currency }
func (m Money) Scale() uint8             { return m.scale }

func (m Money) IsZero() bool     { return m.dec.IsZero() }
func (m Money) IsPositive() bool { return m.dec.IsPositive() && !m.dec.IsZero() }
func (m Money) IsNegative() bool { return m.dec.IsNegative() }

func (m Money) Add(other Money) Money {
	if m.currency != other.currency {
		panic(fmt.Sprintf("domain: Money: cannot add %s and %s", m.currency, other.currency))
	}
	return Money{dec: m.dec.Add(other.dec), currency: m.currency, scale: m.scale}
}

func (m Money) Sub(other Money) Money {
	if m.currency != other.currency {
		panic(fmt.Sprintf("domain: Money: cannot subtract %s from %s", other.currency, m.currency))
	}
	return Money{dec: m.dec.Sub(other.dec), currency: m.currency, scale: m.scale}
}

func (m Money) Mul(factor decimal.Decimal) Money {
	return Money{dec: m.dec.Mul(factor), currency: m.currency, scale: m.scale}
}

func (m Money) String() string {
	return m.dec.String()
}

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, m.dec.String())), nil
}

func (m *Money) UnmarshalJSON(data []byte) error {
	s := string(data)
	s = s[1 : len(s)-1]
	dec, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("domain: Money: UnmarshalJSON: invalid decimal %q: %w", s, err)
	}
	m.dec = dec
	return nil
}
