package storage

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/c9s/bbgo/pkg/fixedpoint"
)

func TestParseStoredFixedRoundTripsFixedpointString(t *testing.T) {
	cases := []fixedpoint.Value{
		0,
		1,
		-1,
		10,
		-10,
		fixedpoint.Value(fixedpoint.DefaultPow),
		-fixedpoint.Value(fixedpoint.DefaultPow),
		123456789,
		-123456789,
		987654321012345,
		-987654321012345,
	}

	for _, value := range cases {
		text := value.String()
		parsed, err := parseStoredFixed(text)
		if err != nil {
			t.Fatalf("parseStoredFixed(%q) error = %v", text, err)
		}
		if parsed != value {
			t.Fatalf("parseStoredFixed(%q) = %v, want %v", text, parsed, value)
		}
	}

	random := rand.New(rand.NewSource(42))
	for range 2000 {
		value := fixedpoint.Value(random.Int63n(2_000_000_000_000_000) - 1_000_000_000_000_000)
		text := value.String()
		parsed, err := parseStoredFixed(text)
		if err != nil {
			t.Fatalf("parseStoredFixed(%q) error = %v", text, err)
		}
		if parsed != value {
			t.Fatalf("parseStoredFixed(%q) = %v, want %v", text, parsed, value)
		}
	}
}

func TestParseStoredFixedFallsBackToUpstreamParser(t *testing.T) {
	inputs := []string{"1e2", "10%", "inf", "-inf"}
	for _, input := range inputs {
		want, wantErr := fixedpoint.NewFromString(input)
		got, gotErr := parseStoredFixed(input)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("parseStoredFixed(%q) error = %v, want %v", input, gotErr, wantErr)
		}
		if got != want {
			t.Fatalf("parseStoredFixed(%q) = %v, want %v", input, got, want)
		}
	}
}
