// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

type codecSample struct {
	Code  string      `json:"code"`
	Price string      `json:"price"`
	Qty   json.Number `json:"qty"`
}

func TestJSONCodecRoundTrip(t *testing.T) {
	in := codecSample{Code: "AAPL", Price: "123.45", Qty: json.Number("100")}
	raw, err := JSONCodec{}.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"price":"123.45"`) {
		t.Fatalf("Marshal produced %s; money must be a JSON string", raw)
	}

	var out codecSample
	if err := (JSONCodec{}).Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Code != in.Code || out.Price != in.Price || out.Qty != in.Qty {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}
