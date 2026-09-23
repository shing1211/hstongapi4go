// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

type Quantity struct {
	dec decimal.Decimal
}

func MustNewQuantity(value string) Quantity {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("domain: Quantity: invalid decimal string %q: %v", value, err))
	}
	return Quantity{dec: dec}
}

func (q Quantity) Decimal() decimal.Decimal { return q.dec }
func (q Quantity) IsZero() bool             { return q.dec.IsZero() }
func (q Quantity) IsPositive() bool         { return q.dec.IsPositive() && !q.dec.IsZero() }
func (q Quantity) IsNegative() bool         { return q.dec.IsNegative() }

func (q Quantity) ValidateInteger() error {
	if !q.dec.IsInteger() {
		return fmt.Errorf("domain: Quantity: %s is not an integer", q.dec.String())
	}
	return nil
}

func (q Quantity) ValidateLot(lot uint64) error {
	if lot == 0 {
		return nil
	}
	lotDec := decimal.NewFromInt(int64(lot))
	remainder := q.dec.Mod(lotDec)
	if !remainder.IsZero() {
		return fmt.Errorf("domain: Quantity: %s is not a multiple of lot %d", q.dec.String(), lot)
	}
	return nil
}

func (q Quantity) String() string { return q.dec.String() }

func (q Quantity) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, q.dec.String())), nil
}

func (q *Quantity) UnmarshalJSON(data []byte) error {
	s := string(data)
	s = s[1 : len(s)-1]
	dec, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("domain: Quantity: UnmarshalJSON: invalid decimal %q: %w", s, err)
	}
	q.dec = dec
	return nil
}
