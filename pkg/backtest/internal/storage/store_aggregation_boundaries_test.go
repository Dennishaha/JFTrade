package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func TestIntervalStorageValueCoversSupportedAndCustomIntervals(t *testing.T) {
	cases := []struct {
		interval types.Interval
		stored   int64
	}{
		{types.Interval1s, 1},
		{types.Interval3m, 180},
		{types.Interval5m, 300},
		{types.Interval15m, 900},
		{types.Interval30m, 1800},
		{types.Interval2h, 7200},
		{types.Interval4h, 14400},
		{types.Interval6h, 21600},
		{types.Interval12h, 43200},
		{types.Interval3d, 259200},
		{types.Interval2w, 1209600},
		{types.Interval1mo, 2592000},
		{types.Interval("45m"), 2700},
		{types.Interval("unsupported"), 0},
	}
	for _, tc := range cases {
		if got := intervalStorageValue(tc.interval); got != tc.stored {
			t.Fatalf("intervalStorageValue(%s) = %d, want %d", tc.interval, got, tc.stored)
		}
	}
}

func TestIntervalFromStorageValueCoversAllPersistedIntervals(t *testing.T) {
	cases := map[int64]types.Interval{
		1:       types.Interval1s,
		60:      types.Interval1m,
		180:     types.Interval3m,
		300:     types.Interval5m,
		900:     types.Interval15m,
		1800:    types.Interval30m,
		3600:    types.Interval1h,
		7200:    types.Interval2h,
		14400:   types.Interval4h,
		21600:   types.Interval6h,
		43200:   types.Interval12h,
		86400:   types.Interval1d,
		259200:  types.Interval3d,
		604800:  types.Interval1w,
		1209600: types.Interval2w,
		2592000: types.Interval1mo,
	}
	for stored, want := range cases {
		got, err := intervalFromStorageValue(stored)
		if err != nil {
			t.Fatalf("intervalFromStorageValue(%d): %v", stored, err)
		}
		if got != want {
			t.Fatalf("intervalFromStorageValue(%d) = %s, want %s", stored, got, want)
		}
	}
	if _, err := intervalFromStorageValue(2); err == nil || !strings.Contains(err.Error(), "unsupported stored interval value 2") {
		t.Fatalf("intervalFromStorageValue(2) error = %v", err)
	}
}

func TestReadSessionScopeNormalizationAndStorageTags(t *testing.T) {
	scopeCases := map[string]string{
		" legacy ":   klineSessionScopeLegacy,
		"REGULAR":    klineSessionScopeRegular,
		" extended ": klineSessionScopeExtended,
		"":           klineReadSessionScopeAuto,
		"overnight":  klineReadSessionScopeAuto,
	}
	for raw, want := range scopeCases {
		if got := normalizeReadSessionScopeName(raw); got != want {
			t.Fatalf("normalizeReadSessionScopeName(%q) = %q, want %q", raw, got, want)
		}
	}

	tagCases := map[string]string{
		klineSessionScopeRegular:  "r",
		klineSessionScopeExtended: "x",
		klineSessionScopeLegacy:   "l",
		"unknown":                 "l",
	}
	for raw, want := range tagCases {
		if got := klineSessionScopeStorageTag(raw); got != want {
			t.Fatalf("klineSessionScopeStorageTag(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestTradingPeriodIntervalHelpers(t *testing.T) {
	day := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	week := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		interval types.Interval
		unit     string
		end      time.Time
		next     time.Time
	}{
		{types.Interval1d, "day", day.AddDate(0, 0, 1).Add(-time.Millisecond), day.AddDate(0, 0, 1)},
		{types.Interval1w, "week", week.AddDate(0, 0, 7).Add(-time.Millisecond), week.AddDate(0, 0, 7)},
		{types.Interval1mo, "month", day.AddDate(0, 1, 0).Add(-time.Millisecond), day.AddDate(0, 1, 0)},
	}
	for _, tc := range cases {
		label := day
		if tc.interval == types.Interval1w {
			label = week
		}
		if !isTradingPeriodAggregationInterval(tc.interval) {
			t.Fatalf("%s should be a trading-period aggregation interval", tc.interval)
		}
		if got := tradingPeriodUnit(tc.interval); got != tc.unit {
			t.Fatalf("tradingPeriodUnit(%s) = %q, want %q", tc.interval, got, tc.unit)
		}
		if got := tradingPeriodLabelEnd(label, tc.interval); !got.Equal(tc.end) {
			t.Fatalf("tradingPeriodLabelEnd(%s) = %s, want %s", tc.interval, got, tc.end)
		}
		if got := shiftTradingPeriodLabel(label, tc.interval, 1); !got.Equal(tc.next) {
			t.Fatalf("shiftTradingPeriodLabel(%s) = %s, want %s", tc.interval, got, tc.next)
		}
	}

	if isTradingPeriodAggregationInterval(types.Interval4h) {
		t.Fatalf("4h should not use trading-period aggregation")
	}
	if got := tradingPeriodUnit(types.Interval4h); got != "" {
		t.Fatalf("tradingPeriodUnit(4h) = %q, want empty", got)
	}
	if got := tradingPeriodLabelEnd(week, types.Interval4h); !got.Equal(week) {
		t.Fatalf("tradingPeriodLabelEnd unsupported = %s, want original label", got)
	}
	if got := shiftTradingPeriodLabel(week, types.Interval4h, 2); !got.Equal(week) {
		t.Fatalf("shiftTradingPeriodLabel unsupported = %s, want original label", got)
	}
}

func TestAggregationBaseIntervalsAndExtendedDailyPriority(t *testing.T) {
	base := aggregationBaseIntervals(types.Interval1d)
	if len(base) == 0 || base[0] != types.Interval12h {
		t.Fatalf("daily aggregation bases = %#v, want largest lower interval first", base)
	}
	if got := aggregationBaseIntervals(types.Interval1m); got != nil {
		t.Fatalf("1m aggregation bases = %#v, want none", got)
	}
	if got := aggregationBaseIntervals(types.Interval("90s")); got != nil {
		t.Fatalf("90s aggregation bases = %#v, want none for non-minute multiple", got)
	}
	if !canAggregateFromLowerInterval(types.Interval15m) || canAggregateFromLowerInterval(types.Interval1m) {
		t.Fatalf("canAggregateFromLowerInterval has unexpected values")
	}

	prioritized := prioritizeDailyAggregationBaseIntervals([]types.Interval{types.Interval1d, types.Interval2h, types.Interval30m, types.Interval1h}, true)
	want := []types.Interval{types.Interval30m, types.Interval1h, types.Interval1d, types.Interval2h}
	for i := range want {
		if prioritized[i] != want[i] {
			t.Fatalf("prioritized[%d] = %s, want %s in %#v", i, prioritized[i], want[i], prioritized)
		}
	}
	unchanged := prioritizeDailyAggregationBaseIntervals([]types.Interval{types.Interval1d, types.Interval1h}, false)
	if unchanged[0] != types.Interval1d || unchanged[1] != types.Interval1h {
		t.Fatalf("regular daily base priority changed = %#v", unchanged)
	}
}

func TestAggregationBaseRangesRespectUSExtendedHours(t *testing.T) {
	since := time.Date(2026, time.June, 15, 13, 45, 0, 0, time.UTC)
	until := time.Date(2026, time.June, 16, 15, 0, 0, 0, time.UTC)

	regularSince, regularUntil := dailyAggregationBaseRange("US.AAPL", since, until, false)
	if !regularSince.Equal(time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)) ||
		!regularUntil.Equal(time.Date(2026, time.June, 15, 23, 59, 59, int(999*time.Millisecond), time.UTC)) {
		t.Fatalf("regular daily range = %s %s", regularSince, regularUntil)
	}

	extendedSince, extendedUntil := dailyAggregationBaseRange("US.AAPL", since, until, true)
	if !extendedSince.Equal(regularSince.Add(-6*time.Hour)) || !extendedUntil.Equal(regularUntil.Add(6*time.Hour)) {
		t.Fatalf("extended US daily range = %s %s, want +/-6h from regular", extendedSince, extendedUntil)
	}

	hkSince, hkUntil := dailyAggregationBaseRange("HK.00700", since, until, true)
	if !hkSince.Equal(regularSince) || !hkUntil.Equal(regularUntil) {
		t.Fatalf("HK extended daily range = %s %s, want regular range", hkSince, hkUntil)
	}
}
