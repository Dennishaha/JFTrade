package backtest

import (
	"testing"
	"time"
)

func TestResolveBacktestTimeRangeUsesMarketDateAndDST(t *testing.T) {
	start, end, startDate, endDate, timezone, err := resolveBacktestTimeRange(
		"US.AAPL",
		"2026-03-08",
		"2026-03-08",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("resolveBacktestTimeRange: %v", err)
	}
	if want := time.Date(2026, time.March, 8, 5, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
	if want := time.Date(2026, time.March, 9, 4, 0, 0, 0, time.UTC).Add(-time.Nanosecond); !end.Equal(want) {
		t.Fatalf("end = %s, want %s", end, want)
	}
	if end.Sub(start)+time.Nanosecond != 23*time.Hour {
		t.Fatalf("DST day duration = %s, want 23h", end.Sub(start)+time.Nanosecond)
	}
	if startDate != "2026-03-08" || endDate != "2026-03-08" || timezone != "America/New_York" {
		t.Fatalf("labels/timezone = %q %q %q", startDate, endDate, timezone)
	}
}

func TestResolveBacktestTimeRangeUsesHongKongCalendarDay(t *testing.T) {
	start, end, _, _, timezone, err := resolveBacktestTimeRange(
		"HK.00700",
		"2026-01-01",
		"2026-01-01",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("resolveBacktestTimeRange: %v", err)
	}
	if want := time.Date(2025, time.December, 31, 16, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
	if want := time.Date(2026, time.January, 1, 16, 0, 0, 0, time.UTC).Add(-time.Nanosecond); !end.Equal(want) {
		t.Fatalf("end = %s, want %s", end, want)
	}
	if timezone != "Asia/Hong_Kong" {
		t.Fatalf("timezone = %q", timezone)
	}
}

func TestResolveBacktestTimeRangeNormalizesLegacyTimestamps(t *testing.T) {
	start, end, startDate, endDate, timezone, err := resolveBacktestTimeRange(
		"US.AAPL",
		"",
		"",
		"2026-06-20T09:30:00+08:00",
		"2026-06-20T10:30:00+08:00",
	)
	if err != nil {
		t.Fatalf("resolveBacktestTimeRange: %v", err)
	}
	if start.Format(time.RFC3339Nano) != "2026-06-20T01:30:00Z" || end.Format(time.RFC3339Nano) != "2026-06-20T02:30:00Z" {
		t.Fatalf("normalized range = %s to %s", start, end)
	}
	if startDate != "" || endDate != "" || timezone != "America/New_York" {
		t.Fatalf("legacy metadata = %q %q %q", startDate, endDate, timezone)
	}
}
