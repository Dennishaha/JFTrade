package marketdata

import (
	"errors"
	"testing"
)

func TestParseCandleSessionsNormalizesCSVAndRepeatedValues(t *testing.T) {
	sessions, err := ParseCandleSessions([]string{"overnight,regular", "extended", "regular"})
	if err != nil {
		t.Fatalf("ParseCandleSessions: %v", err)
	}
	if got := CandleSessionStrings(sessions); len(got) != 3 || got[0] != "regular" || got[1] != "extended" || got[2] != "overnight" {
		t.Fatalf("sessions = %#v", got)
	}
}

func TestParseCandleSessionsRejectsEmptyAndUnknownValues(t *testing.T) {
	for _, values := range [][]string{{""}, {"regular,invalid"}} {
		if _, err := ParseCandleSessions(values); !errors.Is(err, ErrInvalidCandleSessions) {
			t.Fatalf("ParseCandleSessions(%v) error = %v", values, err)
		}
	}
}

func TestResolveCandleSessionsDefaultsAndRejectsUnsupportedValues(t *testing.T) {
	available := []CandleSession{CandleSessionRegular, CandleSessionExtended}
	all, err := ResolveCandleSessions(nil, false, available)
	if err != nil || len(all) != 2 || all[0] != CandleSessionRegular || all[1] != CandleSessionExtended {
		t.Fatalf("default sessions = %#v, err=%v", all, err)
	}
	if _, err := ResolveCandleSessions([]CandleSession{CandleSessionOvernight}, true, available); !errors.Is(err, ErrInvalidCandleSessions) {
		t.Fatalf("unsupported session error = %v", err)
	}
	if _, err := ResolveCandleSessions(nil, true, available); !errors.Is(err, ErrInvalidCandleSessions) {
		t.Fatalf("empty requested sessions error = %v", err)
	}
	selected, err := ResolveCandleSessions([]CandleSession{CandleSessionExtended, CandleSessionRegular}, true, available)
	if err != nil || len(selected) != 2 || selected[0] != CandleSessionRegular || selected[1] != CandleSessionExtended {
		t.Fatalf("selected sessions = %#v, err=%v", selected, err)
	}
}

func TestFilterCandlesBySessionsPreservesUnknownAsRegular(t *testing.T) {
	candles := []map[string]any{
		{"at": "1", "session": "pre"},
		{"at": "2", "session": "regular"},
		{"at": "3"},
	}
	filtered := FilterCandlesBySessions(candles, []CandleSession{CandleSessionExtended})
	if len(filtered) != 1 || filtered[0]["at"] != "1" {
		t.Fatalf("filtered candles = %#v", filtered)
	}
	if got := FilterCandlesBySessions(candles, nil); len(got) != len(candles) {
		t.Fatalf("nil session filter = %#v", got)
	}
	for label, want := range map[string]CandleSession{
		"regular": CandleSessionRegular, "pre": CandleSessionExtended,
		"after": CandleSessionExtended, "extended": CandleSessionExtended,
		"overnight": CandleSessionOvernight,
	} {
		if got := CandleSessionForLabel(label); got != want {
			t.Fatalf("CandleSessionForLabel(%q) = %q, want %q", label, got, want)
		}
	}
	if got := CandleSessionForLabel("unknown"); got != "" {
		t.Fatalf("unknown label = %q", got)
	}
}

func TestNormalizeInstrumentFallsBackForUnqualifiedInput(t *testing.T) {
	market, symbol, id := normalizeInstrument("??", "code")
	if market != "??" || symbol != "CODE" || id != "??.CODE" {
		t.Fatalf("normalizeInstrument fallback = %q/%q/%q", market, symbol, id)
	}
}
