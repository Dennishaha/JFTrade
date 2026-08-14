package productfeatures

import (
	"strings"
	"testing"
)

func TestNormalizeCandleOptionsAcceptsSessionsAndAdjustments(t *testing.T) {
	sessions, adjustment, err := normalizeCandleOptions([]string{" Regular ", "EXTENDED"}, " FORWARD ")
	if err != nil || adjustment != "forward" || strings.Join(sessions, ",") != "regular,extended" {
		t.Fatalf("normalizeCandleOptions(valid) = %#v/%q/%v", sessions, adjustment, err)
	}
	if sessions, adjustment, err := normalizeCandleOptions(nil, ""); err != nil || adjustment != "none" || sessions != nil {
		t.Fatalf("normalizeCandleOptions(default) = %#v/%q/%v", sessions, adjustment, err)
	}
}

func TestNormalizeCandleOptionsRejectsUnsupportedValues(t *testing.T) {
	if _, _, err := normalizeCandleOptions([]string{"pre-market"}, "none"); err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("unsupported session error = %v", err)
	}
	if _, _, err := normalizeCandleOptions([]string{"regular"}, "split"); err == nil || !strings.Contains(err.Error(), "adjustment") {
		t.Fatalf("unsupported adjustment error = %v", err)
	}
}
