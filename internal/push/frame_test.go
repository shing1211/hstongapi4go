// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// sampleHeader returns a fully populated header for round-trip tests.
func sampleHeader() Header {
	var sig [BodySHA1Len]byte
	for i := range sig {
		sig[i] = byte(i)
	}
	return Header{
		HeaderFlag:        [2]byte{'H', 'S'},
		MsgType:           MsgPush,
		ProtoFmtType:      0,
		ProtoVer:          0,
		SerialNo:          123456,
		BodyLen:           7,
		BodySHA1:          sig,
		CompressAlgorithm: 0,
		Reserved:          42,
	}
}

// rawFrame encodes h and appends body without WriteFrame's normalization, so
// tests can construct malformed frames.
func rawFrame(h Header, body []byte) []byte {
	raw := EncodeHeader(h)
	return append(raw[:], body...)
}

func TestEncodeDecodeHeaderRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		h    Header
	}{
		{"push", sampleHeader()},
		{"heartbeat", Header{HeaderFlag: [2]byte{'H', 'S'}, MsgType: MsgHeartbeat}},
		{"request", Header{HeaderFlag: [2]byte{'H', 'S'}, MsgType: MsgRequest, SerialNo: -5, Reserved: -1}},
		{"response", Header{HeaderFlag: [2]byte{'H', 'S'}, MsgType: MsgResponse, ProtoVer: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := EncodeHeader(tt.h)
			got, err := DecodeHeader(raw[:])
			if err != nil {
				t.Fatalf("DecodeHeader: %v", err)
			}
			if got != tt.h {
				t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, tt.h)
			}
		})
	}
}

func TestEncodeHeaderLayout(t *testing.T) {
	h := Header{
		HeaderFlag:        [2]byte{'H', 'S'},
		MsgType:           MsgPush,
		ProtoFmtType:      0,
		ProtoVer:          0,
		SerialNo:          0x01020304,
		BodyLen:           0x05060708,
		CompressAlgorithm: 0,
		Reserved:          0x1112131415161718,
	}
	raw := EncodeHeader(h)
	if got := string(raw[0:2]); got != Magic {
		t.Errorf("bytes 0..2 = %q, want %q", got, Magic)
	}
	if got := binary.LittleEndian.Uint16(raw[2:4]); got != uint16(MsgPush) {
		t.Errorf("msgType = %d, want %d", got, int16(MsgPush))
	}
	if raw[4] != 0 || raw[5] != 0 {
		t.Errorf("protoFmtType/protoVer = %d/%d, want 0/0", raw[4], raw[5])
	}
	if got := binary.LittleEndian.Uint32(raw[6:10]); got != 0x01020304 {
		t.Errorf("serialNo = %#x, want 0x01020304", got)
	}
	if got := binary.LittleEndian.Uint32(raw[10:14]); got != 0x05060708 {
		t.Errorf("bodyLen = %#x, want 0x05060708", got)
	}
	if raw[142] != 0 {
		t.Errorf("compressAlgorithm = %d, want 0", raw[142])
	}
	if got := binary.LittleEndian.Uint64(raw[143:151]); got != 0x1112131415161718 {
		t.Errorf("reserved = %#x", got)
	}
}

func TestDecodeHeaderShortAndBadMagic(t *testing.T) {
	if _, err := DecodeHeader(make([]byte, HeaderSize-1)); !errors.Is(err, ErrShortHeader) {
		t.Fatalf("short header err = %v, want ErrShortHeader", err)
	}
	raw := EncodeHeader(sampleHeader())
	raw[0] = 'X'
	if _, err := DecodeHeader(raw[:]); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("bad magic err = %v, want ErrBadMagic", err)
	}
}

func TestReadFrameShortReads(t *testing.T) {
	full := EncodeHeader(sampleHeader())
	partialHeader := full[:HeaderSize-1]
	tests := []struct {
		name  string
		input []byte
		want  error
	}{
		{"empty", nil, io.EOF},
		{"partial header", []byte("H"), io.ErrUnexpectedEOF},
		{"header only truncated", partialHeader, io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrame(bytes.NewReader(tt.input))
			if tt.want == io.EOF {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("err = %v, want io.EOF", err)
				}
				return
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
			}
		})
	}
}

func TestReadFrameValidation(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{
			"negative bodyLen",
			rawFrame(Header{HeaderFlag: [2]byte{'H', 'S'}, MsgType: MsgPush, BodyLen: -1}, nil),
			ErrInvalidBodyLen,
		},
		{
			"oversized bodyLen",
			rawFrame(Header{HeaderFlag: [2]byte{'H', 'S'}, MsgType: MsgPush, BodyLen: MaxBodyLen + 1}, nil),
			ErrBodyTooLarge,
		},
		{
			"compressed body",
			rawFrame(Header{HeaderFlag: [2]byte{'H', 'S'}, MsgType: MsgPush, BodyLen: 1, CompressAlgorithm: 1}, []byte{0}),
			ErrUnsupportedCompression,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrame(bytes.NewReader(tt.raw))
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWriteReadFrameSymmetry(t *testing.T) {
	tests := []struct {
		name string
		h    Header
		body []byte
	}{
		{"push with body", sampleHeader(), []byte("notify-body")},
		{"heartbeat no body", Header{MsgType: MsgHeartbeat}, nil},
		{"empty body", Header{MsgType: MsgPush, SerialNo: 9}, []byte{}},
		{"large-ish body", Header{MsgType: MsgRequest, Reserved: 7}, bytes.Repeat([]byte{0xAB}, 4096)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, tt.h, tt.body); err != nil {
				t.Fatalf("WriteFrame: %v", err)
			}
			got, body, err := ReadFrame(&buf)
			if err != nil {
				t.Fatalf("ReadFrame: %v", err)
			}
			if got.HeaderFlag != [2]byte{'H', 'S'} {
				t.Errorf("header flag = %q, want HS", string(got.HeaderFlag[:]))
			}
			if got.MsgType != tt.h.MsgType {
				t.Errorf("MsgType = %v, want %v", got.MsgType, tt.h.MsgType)
			}
			if got.SerialNo != tt.h.SerialNo {
				t.Errorf("SerialNo = %d, want %d", got.SerialNo, tt.h.SerialNo)
			}
			if got.Reserved != tt.h.Reserved {
				t.Errorf("Reserved = %d, want %d", got.Reserved, tt.h.Reserved)
			}
			if got.BodySHA1 != tt.h.BodySHA1 {
				t.Errorf("BodySHA1 mismatch")
			}
			if got.BodyLen != int32(len(tt.body)) {
				t.Errorf("BodyLen = %d, want %d", got.BodyLen, len(tt.body))
			}
			if !bytes.Equal(body, tt.body) {
				t.Errorf("body = %q, want %q", body, tt.body)
			}
			if buf.Len() != 0 {
				t.Errorf("%d trailing bytes", buf.Len())
			}
		})
	}
}

func TestWriteFrameRejectsOversizedBody(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrame(&buf, Header{MsgType: MsgPush}, make([]byte, MaxBodyLen+1))
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes before rejecting", buf.Len())
	}
}

func TestMsgTypeString(t *testing.T) {
	tests := map[MsgType]string{
		MsgHeartbeat: "heartbeat",
		MsgRequest:   "request",
		MsgResponse:  "response",
		MsgPush:      "push",
		MsgType(99):  "MsgType(99)",
	}
	for in, want := range tests {
		if got := in.String(); got != want {
			t.Errorf("MsgType(%d).String() = %q, want %q", int16(in), got, want)
		}
	}
}
