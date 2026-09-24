// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestMoney(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		currency string
		scale    uint8
		want     string
		isZero   bool
		isPos    bool
		isNeg    bool
	}{
		{"HKD 100.500", "100.500", "HKD", 3, "100.5", false, true, false},
		{"HKD zero", "0", "HKD", 3, "0", true, false, false},
		{"CNY 50.25", "50.250", "CNY", 2, "50.25", false, true, false},
		{"negative", "-10.5", "HKD", 3, "-10.5", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := MustNewMoney(tt.value, tt.currency, tt.scale)
			if got := m.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if got := m.IsZero(); got != tt.isZero {
				t.Errorf("IsZero() = %v, want %v", got, tt.isZero)
			}
			if got := m.IsPositive(); got != tt.isPos {
				t.Errorf("IsPositive() = %v, want %v", got, tt.isPos)
			}
			if got := m.IsNegative(); got != tt.isNeg {
				t.Errorf("IsNegative() = %v, want %v", got, tt.isNeg)
			}
		})
	}
}

func TestMoneyAdd(t *testing.T) {
	a := MustNewMoney("10.5", "HKD", 3)
	b := MustNewMoney("5.25", "HKD", 3)
	got := a.Add(b)
	if !got.Decimal().Equal(decimal.NewFromFloat(15.75)) {
		t.Errorf("Add = %v, want 15.75", got.String())
	}
}

func TestMoneyAddMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Add with mismatched currencies did not panic")
		}
	}()
	a := MustNewMoney("10", "HKD", 3)
	b := MustNewMoney("5", "CNY", 3)
	_ = a.Add(b)
}

func TestMoneyMarshalJSON(t *testing.T) {
	m := MustNewMoney("123.456", "HKD", 3)
	b, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(b) != `"123.456"` {
		t.Errorf("MarshalJSON() = %s, want \"123.456\"", string(b))
	}
}

func TestPrice(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		tick    string
		want    string
		wantErr bool
	}{
		{"valid 100.5 tick 0.001", "100.5", "0.001", "100.5", false},
		{"not multiple of tick", "100.5005", "0.001", "", true},
		{"zero price", "0", "0.001", "0", false},
		{"zero tick", "100.5", "0", "100.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := MustNewPrice(tt.value, tt.tick)
			err := p.Validate(true)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && p.String() != tt.want {
				t.Errorf("String() = %q, want %q", p.String(), tt.want)
			}
		})
	}
}

func TestPriceNegative(t *testing.T) {
	p := MustNewPrice("-1", "0.001")
	if err := p.Validate(false); err == nil {
		t.Errorf("Validate() expected error for negative price")
	}
}

func TestQuantity(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		isInt   bool
		lot     uint64
		wantErr bool
	}{
		{"integer 100", "100", true, 100, false},
		{"integer lot ok", "500", true, 100, false},
		{"not multiple of lot", "550", true, 100, true},
		{"fractional", "100.5", false, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := MustNewQuantity(tt.value)
			if tt.isInt {
				if err := q.ValidateInteger(); err != nil {
					t.Errorf("ValidateInteger() unexpected error: %v", err)
				}
			}
			if tt.lot > 0 {
				err := q.ValidateLot(tt.lot)
				if (err != nil) != tt.wantErr {
					t.Errorf("ValidateLot() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestMoneyCheckScript(t *testing.T) {
	result, err := runMoneyCheck()
	if err != nil {
		t.Skipf("money-check not available: %v", err)
	}
	if result != 0 {
		t.Errorf("money-check failed with exit %d", result)
	}
}
