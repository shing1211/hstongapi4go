// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

// TestTickIsNotAPrice pins the type itself, because the defect P5 closes was a
// tick carried as a Price and nothing about the old tests would have noticed the
// type changing back. A Tick takes one argument; a Price takes a value and a
// tick. This line does not compile if Tick ever grows the second parameter.
func TestTickIsNotAPrice(t *testing.T) {
	// A tick is a scale. Constructing one must not require a grid for it to live
	// on, because that grid is the thing the SDK was inventing.
	tick := MustNewTick("0.0005")
	if got := tick.String(); got != "0.0005" {
		t.Errorf("MustNewTick(0.0005).String() = %q, want 0.0005", got)
	}
	want, err := decimal.NewFromString("0.0005")
	if err != nil {
		t.Fatalf("test bug: the comparison literal does not parse: %v", err)
	}
	if !tick.Decimal().Equal(want) {
		t.Errorf("Decimal() = %s, want 0.0005", tick.Decimal())
	}
}

// TestAFaithfulTickSurvives is the sharp case from design-tick-model.md section 1:
// as a Price, a 0.0005 tick was built with a 0.001 tick of its own, so validating
// it as a price failed - a spurious error on a value the Gateway had sent.
func TestAFaithfulTickSurvives(t *testing.T) {
	// The old shape, kept explicit: this is what a caller could not do.
	if err := MustNewPrice("0.0005", "0.001").Validate(true); err == nil {
		t.Fatal("a Price with a mismatched tick validated; the defect this type removes " +
			"is not reproducible, so the change is not measured")
	}
	// The new shape: there is no validation to fail, so the value simply survives.
	if got := MustNewTick("0.0005").String(); got != "0.0005" {
		t.Errorf("the tick did not survive: got %q, want 0.0005", got)
	}
}

func TestTickJSONRoundTrip(t *testing.T) {
	for _, in := range []string{"0", "0.0001", "0.001", "0.01", "0.2", "123.4567890123456789", "0.0000000000000000000000001"} {
		t.Run(in, func(t *testing.T) {
			raw, err := json.Marshal(MustNewTick(in))
			if err != nil {
				t.Fatalf("Marshal(%s): %v", in, err)
			}
			if want := `"` + in + `"`; string(raw) != want {
				t.Errorf("Marshal(%s) = %s, want %s (the digits the wire sent)", in, raw, want)
			}
			var back Tick
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("Unmarshal(%s): %v", raw, err)
			}
			if back.String() != in {
				t.Errorf("round trip = %q, want %q", back.String(), in)
			}
		})
	}
}

func TestTickZeroAndNegative(t *testing.T) {
	if !MustNewTick("0").IsZero() {
		t.Error("zero tick IsZero = false")
	}
	if MustNewTick("0").IsNegative() {
		t.Error("zero tick IsNegative = true")
	}
	if !MustNewTick("-0.001").IsNegative() {
		t.Error("negative tick IsNegative = false")
	}
	if MustNewTick("-0.001").IsZero() {
		t.Error("negative tick IsZero = true")
	}
}

func TestMustNewTickPanicsOnGarbage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustNewTick(\"0.00x5\") did not panic")
		}
	}()
	MustNewTick("0.00x5")
}

func TestNewTickReportsInsteadOfPanicking(t *testing.T) {
	if _, err := NewTick("0.0005"); err != nil {
		t.Errorf("NewTick(0.0005) = %v, want nil", err)
	}
	_, err := NewTick("0.00x5")
	if err == nil {
		t.Fatal("NewTick(0.00x5) = nil error, want a report")
	}
	// The non-panicking form is the one a caller should use on a wire field, so
	// it must not be the same failure by another name.
	if _, err := NewTick(""); err == nil {
		t.Error("NewTick(\"\") = nil error, want a report for an absent value")
	}
}

func TestTickUnmarshalRejectsGarbage(t *testing.T) {
	var tick Tick
	if err := json.Unmarshal([]byte(`"0.00x5"`), &tick); err == nil {
		t.Error("Unmarshal of a malformed tick = nil error, want a decode failure")
	}
}
