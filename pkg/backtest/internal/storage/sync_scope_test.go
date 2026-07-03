package storage

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestFilterSyncedKLinesBySessionScopeKeepsOnlyRegularIntradayBars(t *testing.T) {
	premarketStart := time.Date(2026, time.May, 20, 13, 0, 0, 0, time.UTC)
	regularStart := time.Date(2026, time.May, 20, 13, 30, 0, 0, time.UTC)
	klines := []bbgotypes.KLine{
		{
			Symbol:    "US.AAPL",
			Interval:  bbgotypes.Interval1m,
			StartTime: bbgotypes.Time(premarketStart),
			EndTime:   bbgotypes.Time(premarketStart.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(101),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(100.5),
			Volume:    fixedpoint.NewFromInt(1000),
		},
		{
			Symbol:    "US.AAPL",
			Interval:  bbgotypes.Interval1m,
			StartTime: bbgotypes.Time(regularStart),
			EndTime:   bbgotypes.Time(regularStart.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(101),
			High:      fixedpoint.NewFromFloat(102),
			Low:       fixedpoint.NewFromFloat(100),
			Close:     fixedpoint.NewFromFloat(101.5),
			Volume:    fixedpoint.NewFromInt(1000),
		},
	}

	filtered := filterSyncedKLinesBySessionScope("US.AAPL", bbgotypes.Interval1m, klineSessionScopeRegular, klines)
	if len(filtered) != 1 {
		t.Fatalf("filtered regular intraday bar count = %d, want 1", len(filtered))
	}
	if !filtered[0].StartTime.Time().Equal(regularStart) {
		t.Fatalf("filtered regular intraday start = %s, want %s", filtered[0].StartTime.Time(), regularStart)
	}
}

func TestFilterSyncedKLinesBySessionScopeDoesNotDropDailyBars(t *testing.T) {
	dayStart := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	klines := []bbgotypes.KLine{{
		Symbol:    "US.AAPL",
		Interval:  bbgotypes.Interval1d,
		StartTime: bbgotypes.Time(dayStart),
		EndTime:   bbgotypes.Time(dayStart.Add(24*time.Hour - time.Millisecond)),
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(102),
		Low:       fixedpoint.NewFromFloat(99),
		Close:     fixedpoint.NewFromFloat(101),
		Volume:    fixedpoint.NewFromInt(1000),
	}}

	filtered := filterSyncedKLinesBySessionScope("US.AAPL", bbgotypes.Interval1d, klineSessionScopeRegular, klines)
	if len(filtered) != 1 {
		t.Fatalf("filtered daily bar count = %d, want 1", len(filtered))
	}
}

func TestSyncWriteSessionScope(t *testing.T) {
	tests := []struct {
		name           string
		symbol         string
		interval       bbgotypes.Interval
		requestedScope string
		want           string
	}{
		{name: "legacy passthrough", symbol: "US.AAPL", interval: bbgotypes.Interval1m, requestedScope: klineSessionScopeLegacy, want: klineSessionScopeLegacy},
		{name: "regular passthrough", symbol: "US.AAPL", interval: bbgotypes.Interval1m, requestedScope: klineSessionScopeRegular, want: klineSessionScopeRegular},
		{name: "us one hour keeps extended", symbol: "US.AAPL", interval: bbgotypes.Interval1h, requestedScope: klineSessionScopeExtended, want: klineSessionScopeExtended},
		{name: "us daily falls back to regular", symbol: "US.AAPL", interval: bbgotypes.Interval1d, requestedScope: klineSessionScopeExtended, want: klineSessionScopeRegular},
		{name: "non us intraday falls back to regular", symbol: "HK.00700", interval: bbgotypes.Interval1m, requestedScope: klineSessionScopeExtended, want: klineSessionScopeRegular},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := syncWriteSessionScope(tt.symbol, tt.interval, tt.requestedScope); got != tt.want {
				t.Fatalf("syncWriteSessionScope(%s, %s, %s) = %s, want %s", tt.symbol, tt.interval, tt.requestedScope, got, tt.want)
			}
		})
	}
}
