// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

type Rate struct {
	dec decimal.Decimal
}

func MustNewRate(value string) Rate {
	dec, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("domain: Rate: invalid decimal string %q: %v", value, err))
	}
	return Rate{dec: dec}
}

func (r Rate) Decimal() decimal.Decimal { return r.dec }
func (r Rate) IsZero() bool             { return r.dec.IsZero() }
func (r Rate) IsPositive() bool         { return r.dec.IsPositive() && !r.dec.IsZero() }
func (r Rate) IsNegative() bool         { return r.dec.IsNegative() }

func (r Rate) String() string { return r.dec.String() }

func (r Rate) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, r.dec.String())), nil
}

func (r *Rate) UnmarshalJSON(data []byte) error {
	s := string(data)
	s = s[1 : len(s)-1]
	dec, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("domain: Rate: UnmarshalJSON: invalid decimal %q: %w", s, err)
	}
	r.dec = dec
	return nil
}
