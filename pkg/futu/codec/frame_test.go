package codec

import (
	"errors"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	body := []byte{1, 2, 3, 4, 5}
	pkt, err := Encode(1001, 42, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt) != HeaderLen+len(body) {
		t.Fatalf("len = %d", len(pkt))
	}
	f, err := Decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if f.Header.ProtoID != 1001 || f.Header.SerialNo != 42 {
		t.Fatalf("header = %+v", f.Header)
	}
	if string(f.Body) != string(body) {
		t.Fatalf("body = %v", f.Body)
	}
}

func TestDecodeRejectsCorruptedBody(t *testing.T) {
	pkt, jftradeErr1 := Encode(2001, 1, []byte("hello"))
	jftradeCheckTestError(t, jftradeErr1)
	pkt[len(pkt)-1] ^= 0xff
	_, err := Decode(pkt)
	if !errors.Is(err, ErrBadBodyHash) {
		t.Fatalf("want ErrBadBodyHash, got %v", err)
	}
}

func TestDecodeRejectsBadMagic(t *testing.T) {
	pkt, jftradeErr2 := Encode(2001, 1, []byte{0})
	jftradeCheckTestError(t, jftradeErr2)
	pkt[0] = 'X'
	_, err := Decode(pkt)
	if !errors.Is(err, ErrBadMagic) {
		t.Fatalf("want ErrBadMagic, got %v", err)
	}
}

func TestDecodeRejectsShortFrame(t *testing.T) {
	_, err := Decode([]byte{1, 2, 3})
	if !errors.Is(err, ErrFrameTooShort) {
		t.Fatalf("want ErrFrameTooShort, got %v", err)
	}
}

func TestDecodeRejectsLengthMismatch(t *testing.T) {
	pkt, jftradeErr3 := Encode(1, 1, []byte{1, 2, 3})
	jftradeCheckTestError(t, jftradeErr3)
	// truncate body
	pkt = pkt[:HeaderLen+1]
	if _, err := Decode(pkt); err == nil {
		t.Fatal("want length mismatch error")
	}
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
