package calendar

import (
	"testing"
	"time"
)

func TestBuiltinCalendarBoundaryCoverage(t *testing.T) {
	resolver := NewBuiltinResolver()
	hk, ok := resolver.Template("HK")
	if !ok {
		t.Fatal("HK template missing")
	}
	weekend := builtinWeekdaySchedule(hk, time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC))
	if weekend.Status != TradingDayClosed || weekend.Reason != "weekend" {
		t.Fatalf("HK weekend schedule = %+v", weekend)
	}

	us, ok := resolver.Template("US")
	if !ok {
		t.Fatal("US template missing")
	}
	if schedule := builtinUSSchedule(us, time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)); schedule.Status != TradingDayClosed || schedule.Reason != "weekend" {
		t.Fatalf("US weekend schedule = %+v", schedule)
	}
	if schedule := builtinUSSchedule(us, time.Date(2026, time.January, 6, 0, 0, 0, 0, time.UTC)); schedule.Status != TradingDayOpen || len(schedule.Sessions) == 0 {
		t.Fatalf("US normal weekday schedule = %+v", schedule)
	}
	if reason, ok := mainlandHolidayReason(time.Date(2040, time.January, 2, 0, 0, 0, 0, time.UTC)); ok || reason != "" {
		t.Fatalf("unexpected unlisted mainland holiday = %q,%v", reason, ok)
	}
}

func TestCalendarHelperBoundaryCoverage(t *testing.T) {
	if got := observedFixedHoliday(2023, time.January, 1); got.Day() != 2 {
		t.Fatalf("Sunday observed holiday = %s, want Jan 2", got)
	}
	if isIndependenceDayEarlyClose(time.Date(2026, time.July, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("2026-07-03 should not be an independence early close")
	}
	if isChristmasEveEarlyClose(time.Date(2022, time.December, 24, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("weekend Christmas Eve should not be an early close")
	}
	if got := LoadLocation(MarketTemplate{Timezone: "Not/A_Real_Zone"}); got != time.UTC {
		t.Fatalf("invalid timezone location = %v, want UTC", got)
	}
}
