package jftradeapi

import (
	"testing"
	"time"
)

func TestNormalizeCandlePeriodMapsAliases(t *testing.T) {
	period, err := normalizeCandlePeriod("  k_60m ")
	if err != nil {
		t.Fatalf("normalizeCandlePeriod: %v", err)
	}
	if period != "1h" {
		t.Fatalf("period = %q", period)
	}
}

func TestParseQueryTimeFallsBackOnInvalidInput(t *testing.T) {
	fallback := time.Date(2026, time.May, 22, 10, 30, 0, 0, time.UTC)
	if got := parseQueryTime("invalid", fallback); !got.Equal(fallback) {
		t.Fatalf("parseQueryTime fallback = %s, want %s", got, fallback)
	}
}

func TestKLineQueryWindowUsesExplicitBounds(t *testing.T) {
	expectedBegin := time.Date(2026, time.May, 21, 9, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, time.May, 21, 16, 0, 0, 0, time.UTC)

	beginAt, endAt := kLineQueryWindow(map[string][]string{
		"fromTime": {expectedBegin.Format(time.RFC3339Nano)},
		"toTime":   {expectedEnd.Format(time.RFC3339Nano)},
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

	beginAt, resolvedEndAt := kLineQueryWindow(map[string][]string{
		"fromTime": {endAt.Add(time.Hour).Format(time.RFC3339Nano)},
		"toTime":   {endAt.Format(time.RFC3339Nano)},
	}, time.Minute, 2)

	expectedBegin := endAt.Add(-36 * time.Hour)
	if !resolvedEndAt.Equal(endAt) {
		t.Fatalf("endAt = %s, want %s", resolvedEndAt, endAt)
	}
	if !beginAt.Equal(expectedBegin) {
		t.Fatalf("beginAt = %s, want %s", beginAt, expectedBegin)
	}
}
