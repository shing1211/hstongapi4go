// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
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

func TestProtoJSONCodecRoundTrip(t *testing.T) {
	in := &dto.Security{DataType: 10000, Code: "00700"}
	raw, err := ProtoJSONCodec{}.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(raw); !strings.Contains(got, `"dataType":10000`) || !strings.Contains(got, `"code":"00700"`) {
		t.Fatalf("Marshal = %s, want canonical protojson for Security", got)
	}

	var out dto.Security
	if err := (ProtoJSONCodec{}).Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.GetDataType() != in.GetDataType() || out.GetCode() != in.GetCode() {
		t.Fatalf("round trip = %+v, want %+v", &out, in)
	}
}

func TestProtoJSONCodecRejectsUnknownFields(t *testing.T) {
	var out dto.Security
	err := (ProtoJSONCodec{}).Unmarshal([]byte(`{"dataType":1,"code":"x","mystery":true}`), &out)
	if err == nil {
		t.Fatal("Unmarshal accepted an unknown field; DiscardUnknown must be false")
	}
}

func TestProtoJSONCodecRejectsNonProto(t *testing.T) {
	var sample codecSample
	_, err := ProtoJSONCodec{}.Marshal(sample)
	if !errors.Is(err, ErrNotProtoMessage) {
		t.Fatalf("Marshal error = %v, want ErrNotProtoMessage", err)
	}
	if !strings.Contains(err.Error(), "transport.codecSample") {
		t.Fatalf("Marshal error %q does not name the offending type", err)
	}

	err = (ProtoJSONCodec{}).Unmarshal([]byte(`{}`), &sample)
	if !errors.Is(err, ErrNotProtoMessage) {
		t.Fatalf("Unmarshal error = %v, want ErrNotProtoMessage", err)
	}

	err = (ProtoJSONCodec{}).Unmarshal([]byte(`null`), nil)
	if !errors.Is(err, ErrNotProtoMessage) {
		t.Fatalf("Unmarshal(nil) error = %v, want ErrNotProtoMessage", err)
	}
}
