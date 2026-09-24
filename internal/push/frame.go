// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// HeaderSize is the fixed size in bytes of every push/request/response header
// on the Gateway TCP channel.
const HeaderSize = 151

// Magic is the two-byte ASCII header flag ("HS") that prefixes every frame.
const Magic = "HS"

// BodySHA1Len is the size in bytes of the header's bodySHA1 signature field.
const BodySHA1Len = 128

// MaxBodyLen is the largest body ReadFrame accepts, 8 MiB. A header advertising
// a larger bodyLen is rejected before any body bytes are read, which bounds the
// memory a hostile or corrupt peer can make the reader allocate.
const MaxBodyLen = 8 << 20

// Byte offsets of each field within the 151-byte header (zero-based).
const (
	offHeaderFlag   = 0
	offMsgType      = 2
	offProtoFmtType = 4
	offProtoVer     = 5
	offSerialNo     = 6
	offBodyLen      = 10
	offBodySHA1     = 14
	offCompressAlgo = 142
	offReserved     = 143
)

// Sentinel errors returned by the framing helpers. They are wrapped with
// details, so callers test with errors.Is.
var (
	// ErrShortHeader reports a buffer or stream that ended before a complete
	// 151-byte header was available.
	ErrShortHeader = errors.New("push: short header")
	// ErrBadMagic reports a header whose szHeaderFlag is not "HS".
	ErrBadMagic = errors.New("push: bad header magic")
	// ErrInvalidBodyLen reports a negative bodyLen.
	ErrInvalidBodyLen = errors.New("push: invalid body length")
	// ErrBodyTooLarge reports a bodyLen above MaxBodyLen.
	ErrBodyTooLarge = errors.New("push: body too large")
	// ErrUnsupportedCompression reports a non-zero compressAlgorithm. The
	// protocol only defines 0 (no compression).
	ErrUnsupportedCompression = errors.New("push: unsupported compression algorithm")
)

// MsgType is the message kind carried in the header's msgType field.
type MsgType int16

const (
	// MsgHeartbeat is a keep-alive frame. It carries no body and is ignored by
	// the read loop.
	MsgHeartbeat MsgType = 0
	// MsgRequest is a client-to-Gateway request frame.
	MsgRequest MsgType = 1
	// MsgResponse is a Gateway-to-client response frame.
	MsgResponse MsgType = 2
	// MsgPush is a Gateway-to-client notification frame. Its body is a
	// PBNotify.
	MsgPush MsgType = 3
)

// String returns the human-readable name of the message type, or
// "MsgType(<n>)" for an unrecognized value.
func (m MsgType) String() string {
	switch m {
	case MsgHeartbeat:
		return "heartbeat"
	case MsgRequest:
		return "request"
	case MsgResponse:
		return "response"
	case MsgPush:
		return "push"
	default:
		return fmt.Sprintf("MsgType(%d)", int16(m))
	}
}

// Header is the decoded 151-byte wire header shared by heartbeat, request,
// response, and push frames. BodySHA1 holds the raw SHA1WithRSA signature of
// the body; it is preserved for opt-in verification (ADR 0005) and is not
// checked by this package.
type Header struct {
	// HeaderFlag is the ASCII header flag. It is always "HS" on the wire.
	HeaderFlag [2]byte
	// MsgType is the frame kind (see MsgType).
	MsgType MsgType
	// ProtoFmtType selects the body representation; 0 is protobuf.
	ProtoFmtType uint8
	// ProtoVer is the protocol format version; 0 is current.
	ProtoVer uint8
	// SerialNo is the sender's monotonic message serial number.
	SerialNo int32
	// BodyLen is the number of body bytes that follow the header.
	BodyLen int32
	// BodySHA1 is the 128-byte SHA1WithRSA signature of the body. It is zero
	// when the sender does not sign the frame.
	BodySHA1 [BodySHA1Len]byte
	// CompressAlgorithm selects body compression; 0 is none. ReadFrame rejects
	// any other value.
	CompressAlgorithm uint8
	// Reserved is the trailing 8-byte reserved field.
	Reserved int64
}

// EncodeHeader serialises h into its fixed 151-byte little-endian wire form.
// Every field of h is written verbatim; callers that need the header flag,
// message type, or body length normalized should use WriteFrame instead. The
// returned array is a value and safe to retain.
func EncodeHeader(h Header) [HeaderSize]byte {
	var b [HeaderSize]byte
	copy(b[offHeaderFlag:offHeaderFlag+2], h.HeaderFlag[:])
	binary.LittleEndian.PutUint16(b[offMsgType:offMsgType+2], uint16(h.MsgType)) // #nosec G115 -- unsigned encoding of a signed field; the wire form is fixed-width little-endian
	b[offProtoFmtType] = h.ProtoFmtType
	b[offProtoVer] = h.ProtoVer
	binary.LittleEndian.PutUint32(b[offSerialNo:offSerialNo+4], uint32(h.SerialNo)) // #nosec G115 -- unsigned encoding of a signed field; the wire form is fixed-width little-endian
	binary.LittleEndian.PutUint32(b[offBodyLen:offBodyLen+4], uint32(h.BodyLen))    // #nosec G115 -- unsigned encoding of a signed field; ReadFrame range-checks the decoded value before use
	copy(b[offBodySHA1:offBodySHA1+BodySHA1Len], h.BodySHA1[:])
	b[offCompressAlgo] = h.CompressAlgorithm
	binary.LittleEndian.PutUint64(b[offReserved:offReserved+8], uint64(h.Reserved)) // #nosec G115 -- unsigned encoding of a signed field; the wire form is fixed-width little-endian
	return b
}

// DecodeHeader parses the first HeaderSize bytes of b. It returns an error
// wrapping ErrShortHeader when b is shorter than HeaderSize and an error
// wrapping ErrBadMagic when the header flag is not "HS". Bytes beyond
// HeaderSize are ignored, so DecodeHeader may be called on a whole frame.
//
// DecodeHeader does not validate bodyLen or compressAlgorithm; ReadFrame does.
func DecodeHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, fmt.Errorf("%w: got %d bytes, need %d", ErrShortHeader, len(b), HeaderSize)
	}
	var h Header
	copy(h.HeaderFlag[:], b[offHeaderFlag:offHeaderFlag+2])
	if string(h.HeaderFlag[:]) != Magic {
		return Header{}, fmt.Errorf("%w: got %q, want %q", ErrBadMagic, string(h.HeaderFlag[:]), Magic)
	}
	h.MsgType = MsgType(int16(binary.LittleEndian.Uint16(b[offMsgType : offMsgType+2]))) // #nosec G115 -- decode of a fixed-width wire field; not an arithmetic conversion
	h.ProtoFmtType = b[offProtoFmtType]
	h.ProtoVer = b[offProtoVer]
	h.SerialNo = int32(binary.LittleEndian.Uint32(b[offSerialNo : offSerialNo+4])) // #nosec G115 -- decode of a fixed-width wire field; not an arithmetic conversion
	h.BodyLen = int32(binary.LittleEndian.Uint32(b[offBodyLen : offBodyLen+4]))    // #nosec G115 -- decode of a fixed-width wire field; ReadFrame range-checks BodyLen before use
	copy(h.BodySHA1[:], b[offBodySHA1:offBodySHA1+BodySHA1Len])
	h.CompressAlgorithm = b[offCompressAlgo]
	h.Reserved = int64(binary.LittleEndian.Uint64(b[offReserved : offReserved+8])) // #nosec G115 -- decode of a fixed-width wire field; not an arithmetic conversion
	return h, nil
}

// ReadFrame reads exactly one header and its body from r.
//
// It uses io.ReadFull for both the header and the body, so a short read yields
// io.ErrUnexpectedEOF (or io.EOF when nothing was read). It validates the magic
// flag, rejects a negative bodyLen with ErrInvalidBodyLen, rejects a bodyLen
// above MaxBodyLen with ErrBodyTooLarge, and rejects a non-zero
// compressAlgorithm with ErrUnsupportedCompression. The returned body slice is
// freshly allocated and owned by the caller; it is nil for a zero-length body
// such as a heartbeat.
func ReadFrame(r io.Reader) (Header, []byte, error) {
	var raw [HeaderSize]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return Header{}, nil, err
	}
	h, err := DecodeHeader(raw[:])
	if err != nil {
		return Header{}, nil, err
	}
	if h.BodyLen < 0 {
		return Header{}, nil, fmt.Errorf("%w: %d", ErrInvalidBodyLen, h.BodyLen)
	}
	if h.BodyLen > MaxBodyLen {
		return Header{}, nil, fmt.Errorf("%w: %d exceeds %d", ErrBodyTooLarge, h.BodyLen, MaxBodyLen)
	}
	if h.CompressAlgorithm != 0 {
		return Header{}, nil, fmt.Errorf("%w: %d", ErrUnsupportedCompression, h.CompressAlgorithm)
	}
	if h.BodyLen == 0 {
		return h, nil, nil
	}
	body := make([]byte, h.BodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return Header{}, nil, err
	}
	return h, body, nil
}

// WriteFrame writes h followed by body to w. The header flag is forced to
// "HS" and BodyLen is set to len(body), so callers must not pre-set either for
// the frame to be self-consistent; all other Header fields are written
// verbatim. A body larger than MaxBodyLen is rejected with ErrBodyTooLarge
// before any byte is written.
//
// WriteFrame writes the header and body with io.Writer.Write and reports
// io.ErrShortWrite if a writer reports a partial write without an error. It is
// used by tests and by the standalone mock Gateway (P09).
func WriteFrame(w io.Writer, h Header, body []byte) error {
	if len(body) > MaxBodyLen {
		return fmt.Errorf("%w: %d exceeds %d", ErrBodyTooLarge, len(body), MaxBodyLen)
	}
	h.HeaderFlag = [2]byte{Magic[0], Magic[1]}
	h.BodyLen = int32(len(body)) // #nosec G115 -- len(body) is bounded by MaxBodyLen immediately above, so it fits in int32
	raw := EncodeHeader(h)
	if err := writeFull(w, raw[:]); err != nil {
		return err
	}
	if len(body) > 0 {
		if err := writeFull(w, body); err != nil {
			return err
		}
	}
	return nil
}

// writeFull writes b to w and reports io.ErrShortWrite on a partial write.
func writeFull(w io.Writer, b []byte) error {
	n, err := w.Write(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return nil
}
