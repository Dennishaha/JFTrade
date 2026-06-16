package servercore

import (
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	"testing"
	"time"
)

func TestNormalizeCandlePeriodMapsAliases(t *testing.T) {
	period, err := httpserver.NormalizeCandlePeriod("  k_60m ")
	if err != nil {
		t.Fatalf("normalizeCandlePeriod: %v", err)
	}
	if period != "1h" {
		t.Fatalf("period = %q", period)
	}
}

func TestParseQueryTimeFallsBackOnInvalidInput(t *testing.T) {
	defaultTime := time.Date(2026, time.May, 22, 10, 30, 0, 0, time.UTC)
	if got := httpserver.ParseQueryTime("invalid", defaultTime); !got.Equal(defaultTime) {
		t.Fatalf("parseQueryTime default = %s, want %s", got, defaultTime)
	}
}

func TestKLineQueryWindowUsesExplicitBounds(t *testing.T) {
	expectedBegin := time.Date(2026, time.May, 21, 9, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, time.May, 21, 16, 0, 0, 0, time.UTC)

	beginAt, endAt := kLineQueryWindow(marketCandlesQuery{
		FromTime: newOptionalTimeValue(expectedBegin),
		ToTime:   newOptionalTimeValue(expectedEnd),
	}, time.Minute, 10)

	if !beginAt.Equal(expectedBegin) {
		t.Fatalf("beginAt = %s, want %s", beginAt, expectedBegin)
	}
	if !endAt.Equal(expectedEnd) {
		t.Fatalf("endAt = %s, want %s", endAt, expectedEnd)
	}
}

func TestKLineQueryWindowResetsInvalidBeginToDefaultLookback(t *testing.T) {
	endAt := time.Date(2026, time.May, 21, 16, 0, 0, 0, time.UTC)

	beginAt, resolvedEndAt := kLineQueryWindow(marketCandlesQuery{
		FromTime: newOptionalTimeValue(endAt.Add(time.Hour)),
		ToTime:   newOptionalTimeValue(endAt),
	}, time.Minute, 2)

	expectedBegin := endAt.Add(-36 * time.Hour)
	if !resolvedEndAt.Equal(endAt) {
		t.Fatalf("endAt = %s, want %s", resolvedEndAt, endAt)
	}
	if !beginAt.Equal(expectedBegin) {
		t.Fatalf("beginAt = %s, want %s", beginAt, expectedBegin)
	}
}
