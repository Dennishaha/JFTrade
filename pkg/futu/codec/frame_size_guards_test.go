package codec

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestFrameSizeGuardsCoverEncodeAndDecode(t *testing.T) {
	tooLarge := make([]byte, MaxFrameBodyLen+1)
	if _, err := Encode(1, 1, tooLarge); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("Encode oversized body error = %v, want ErrBodyTooLarge", err)
	}

	packet := make([]byte, HeaderLen)
	packet[0], packet[1] = frameMagic0, frameMagic1
	binary.LittleEndian.PutUint32(packet[12:16], MaxFrameBodyLen+1)
	if _, err := Decode(packet); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("Decode oversized declared body error = %v, want ErrBodyTooLarge", err)
	}
}
