// Package codec implements the Futu OpenD wire frame: a fixed 44-byte header
// followed by a protobuf body.
//
//	| 'F' (1) | 'T' (1) | protoID (4 LE) | protoFmtType (1) | protoVer (1) |
//	| serialNo (4 LE) | bodyLen (4 LE) | bodySHA1 (20) | reserved (8) |
//	| body (bodyLen)                                                       |
package codec

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	HeaderLen        = 44
	ProtoFormatPB    = byte(0)
	defaultProtoVer  = byte(0)
	frameMagic0      = byte('F')
	frameMagic1      = byte('T')
	MaxFrameBodyLen  = 32 << 20
)

var (
	ErrFrameTooShort = errors.New("futu opend frame too short")
	ErrBadMagic      = errors.New("futu opend frame has invalid magic")
	ErrBadBodyHash   = errors.New("futu opend frame body hash mismatch")
	ErrBodyTooLarge  = errors.New("futu opend frame body too large")
)

// Header is the fixed 44-byte Futu OpenD frame header.
type Header struct {
	ProtoID      uint32
	ProtoFmtType byte
	ProtoVer     byte
	SerialNo     uint32
	BodyLen      uint32
	BodySHA1     [20]byte
}

// Frame is a decoded Futu OpenD packet.
type Frame struct {
	Header Header
	Body   []byte
}

// Encode builds a wire packet from a proto ID, serial number and encoded
// protobuf body.
func Encode(protoID, serialNo uint32, body []byte) ([]byte, error) {
	if len(body) > MaxFrameBodyLen {
		return nil, ErrBodyTooLarge
	}
	packet := make([]byte, HeaderLen+len(body))
	packet[0] = frameMagic0
	packet[1] = frameMagic1
	binary.LittleEndian.PutUint32(packet[2:6], protoID)
	packet[6] = ProtoFormatPB
	packet[7] = defaultProtoVer
	binary.LittleEndian.PutUint32(packet[8:12], serialNo)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(len(body)))
	hash := sha1.Sum(body)
	copy(packet[16:36], hash[:])
	// reserved 36..44 stays zero
	copy(packet[HeaderLen:], body)
	return packet, nil
}

// Decode validates and decodes a complete Futu OpenD wire packet.
func Decode(packet []byte) (Frame, error) {
	if len(packet) < HeaderLen {
		return Frame{}, ErrFrameTooShort
	}
	if packet[0] != frameMagic0 || packet[1] != frameMagic1 {
		return Frame{}, ErrBadMagic
	}
	bodyLen := binary.LittleEndian.Uint32(packet[12:16])
	if bodyLen > MaxFrameBodyLen {
		return Frame{}, ErrBodyTooLarge
	}
	if len(packet) != HeaderLen+int(bodyLen) {
		return Frame{}, fmt.Errorf("futu opend frame length mismatch: header bodyLen=%d packet=%d", bodyLen, len(packet))
	}
	var expected [20]byte
	copy(expected[:], packet[16:36])
	body := packet[HeaderLen:]
	actual := sha1.Sum(body)
	if expected != actual {
		return Frame{}, ErrBadBodyHash
	}
	header := Header{
		ProtoID:      binary.LittleEndian.Uint32(packet[2:6]),
		ProtoFmtType: packet[6],
		ProtoVer:     packet[7],
		SerialNo:     binary.LittleEndian.Uint32(packet[8:12]),
		BodyLen:      bodyLen,
		BodySHA1:     expected,
	}
	out := make([]byte, len(body))
	copy(out, body)
	return Frame{Header: header, Body: out}, nil
}
