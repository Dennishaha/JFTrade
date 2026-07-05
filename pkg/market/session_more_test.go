package market

import (
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func TestMarketSessionBoundaryGuardClauses(t *testing.T) {
	original := CurrentCalendarResolver()
	calendarResolverMu.Lock()
	activeCalendarResolver = nil
	calendarResolverMu.Unlock()
	t.Cleanup(func() { SetCalendarResolver(original) })

	if got := CurrentCalendarResolver(); got == nil {
		t.Fatal("nil active resolver should fall back to default resolver")
	}
	calendarResolverMu.Lock()
	activeCalendarResolver = nil
	calendarResolverMu.Unlock()
	if previous := SwapCalendarResolver(nil); previous == nil {
		t.Fatal("SwapCalendarResolver should report default resolver when active resolver is nil")
	}

	if got := ClassifySession("HK.00700", time.Now()); got != SessionUnknown {
		t.Fatalf("ClassifySession non-US = %s, want unknown", got)
	}
	if got := ClassifySession("US.AAPL", time.Time{}); got != SessionUnknown {
		t.Fatalf("ClassifySession zero time = %s, want unknown", got)
	}
	if got := sessionFromCalendar(marketcalendar.SessionClosed); got != SessionClosed {
		t.Fatalf("sessionFromCalendar closed = %s", got)
	}
	if IsRegularTradingTime("BAD", time.Now()) {
		t.Fatal("invalid symbol should not be regular trading time")
	}
	if IsRegularTradingTime("US.AAPL", time.Time{}) {
		t.Fatal("zero time should not be regular trading time")
	}
	if got, ok := TradingDayKey("BAD", time.Now(), true); ok || got != "" {
		t.Fatalf("TradingDayKey bad symbol = %q/%v", got, ok)
	}
	if got, ok := TradingPeriodKey("BAD", time.Now(), "day", true); ok || got != "" {
		t.Fatalf("TradingPeriodKey bad symbol = %q/%v", got, ok)
	}
}

func TestMarketSessionBucketClampsAtSessionEnd(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	at := time.Date(2026, 6, 15, 15, 55, 0, 0, nyLoc)

	start, end, ok := SessionAwareIntradayBucketBounds("US.AAPL", at, 15*time.Minute, false)
	if !ok {
		t.Fatal("expected regular session bucket near close")
	}
	if !start.Equal(time.Date(2026, 6, 15, 19, 45, 0, 0, time.UTC)) {
		t.Fatalf("bucket start = %s", start)
	}
	if !end.Equal(time.Date(2026, 6, 15, 19, 59, 59, int(time.Second-time.Millisecond), time.UTC)) {
		t.Fatalf("bucket end = %s, want regular close minus 1ms", end)
	}
}

func TestMarketSundayOvernightCarryWindow(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	template := marketcalendar.MarketTemplate{
		MarketCode:             "US",
		Timezone:               "America/New_York",
		SupportsExtendedHours:  true,
		OvernightCarryStartMin: 20 * 60,
	}
	sunday := marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayOpen, time.Date(2026, 6, 14, 0, 0, 0, 0, nyLoc), "unit", "", false, []marketcalendar.SessionWindow{
		{Kind: marketcalendar.SessionOvernight, StartMinute: 20 * 60, EndMinute: 24 * 60},
	})
	monday := marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayOpen, time.Date(2026, 6, 15, 0, 0, 0, 0, nyLoc), "unit", "", false, []marketcalendar.SessionWindow{
		{Kind: marketcalendar.SessionPre, StartMinute: 4 * 60, EndMinute: 9*60 + 30},
		{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
	})
	original := SwapCalendarResolver(&testCalendarResolver{
		template: template,
		schedules: map[string]marketcalendar.TradingDaySchedule{
			"2026-06-14": sunday,
			"2026-06-15": monday,
		},
	})
	t.Cleanup(func() { SetCalendarResolver(original) })

	at := time.Date(2026, 6, 14, 20, 30, 0, 0, nyLoc)

	start, end, ok := sessionWindowBounds("US.AAPL", at, true)
	if !ok {
		t.Fatal("expected Sunday overnight carry window")
	}
	if !start.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) || !end.Equal(time.Date(2026, 6, 15, 4, 0, 0, 0, time.UTC)) {
		t.Fatalf("Sunday overnight window = %s - %s", start, end)
	}
}

func TestMarketLabelAndResolverFailureBoundaries(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	localDay := time.Date(2026, 6, 17, 12, 0, 0, 0, nyLoc)
	if got, ok := tradingPeriodLabelStartFromLocalDay(localDay, "week"); !ok || !got.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("week label start = %s/%v", got, ok)
	}
	if _, _, _, ok := resolveTradingDaySchedule("BAD", localDay); ok {
		t.Fatal("unknown market should not resolve trading schedule")
	}
	if _, _, _, ok := resolveTradingDaySchedule("US", time.Time{}); ok {
		t.Fatal("zero time should not resolve trading schedule")
	}

	profile, ok := ProfileForSymbol("US.AAPL")
	if !ok {
		t.Fatal("missing US profile")
	}
	if _, _, ok := regularSessionWindowBounds(profile, time.Time{}); ok {
		t.Fatal("zero time should not resolve regular session bounds")
	}

	template := marketcalendar.MarketTemplate{
		MarketCode:             "US",
		Timezone:               "America/New_York",
		SupportsExtendedHours:  true,
		OvernightCarryStartMin: 20 * 60,
	}
	original := SwapCalendarResolver(&testCalendarResolver{template: template, schedules: map[string]marketcalendar.TradingDaySchedule{}})
	t.Cleanup(func() { SetCalendarResolver(original) })
	if _, _, ok := regularSessionWindowBounds(profile, localDay); ok {
		t.Fatal("missing schedule should not resolve regular session bounds")
	}
	if _, _, ok := usExtendedSessionWindowBounds("US.AAPL", localDay); ok {
		t.Fatal("missing schedule should not resolve extended session bounds")
	}
}
