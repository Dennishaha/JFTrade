package market

import (
	"strings"
	"testing"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type testCalendarResolver struct {
	template  marketcalendar.MarketTemplate
	schedules map[string]marketcalendar.TradingDaySchedule
}

func (r *testCalendarResolver) Template(marketCode string) (marketcalendar.MarketTemplate, bool) {
	if strings.EqualFold(marketCode, r.template.MarketCode) {
		return r.template, true
	}
	return marketcalendar.MarketTemplate{}, false
}

func (r *testCalendarResolver) Schedule(marketCode string, day time.Time) (marketcalendar.TradingDaySchedule, bool) {
	if !strings.EqualFold(marketCode, r.template.MarketCode) {
		return marketcalendar.TradingDaySchedule{}, false
	}
	key := marketcalendar.DayStart(r.template, day).Format("2006-01-02")
	schedule, ok := r.schedules[key]
	return schedule, ok
}

func TestMarketInputMatchingAndCalendarResolverLifecycle(t *testing.T) {
	parsed, err := ParseQualifiedInstrumentSymbol("SH.600519")
	if err != nil {
		t.Fatalf("ParseQualifiedInstrumentSymbol: %v", err)
	}
	if !MarketInputMatchesParsedSymbol("CN", parsed) {
		t.Fatal("CN market should accept SH-qualified China symbol")
	}
	if !MarketInputMatchesParsedSymbol("CNSH", parsed) {
		t.Fatal("CNSH market should accept SH-qualified China symbol")
	}
	if MarketInputMatchesParsedSymbol("CNSZ", parsed) {
		t.Fatal("CNSZ market should reject SH-qualified China symbol")
	}
	if MarketInputMatchesParsedSymbol("bad-market", parsed) {
		t.Fatal("unsupported market should not match parsed symbol")
	}

	original := CurrentCalendarResolver()
	custom := &testCalendarResolver{
		template: marketcalendar.MarketTemplate{
			MarketCode:             "US",
			Timezone:               "America/New_York",
			SupportsExtendedHours:  true,
			OvernightCarryStartMin: 20 * 60,
		},
		schedules: map[string]marketcalendar.TradingDaySchedule{},
	}

	SetCalendarResolver(custom)
	if got := CurrentCalendarResolver(); got != custom {
		t.Fatalf("CurrentCalendarResolver = %#v, want custom resolver", got)
	}

	previous := SwapCalendarResolver(nil)
	if previous != custom {
		t.Fatalf("SwapCalendarResolver previous = %#v, want custom resolver", previous)
	}
	if got := CurrentCalendarResolver(); got == custom || got == nil {
		t.Fatalf("CurrentCalendarResolver after reset swap = %#v, want non-nil default resolver", got)
	}

	SetCalendarResolver(custom)
	ResetCalendarResolver()
	if got := CurrentCalendarResolver(); got == custom || got == nil {
		t.Fatalf("CurrentCalendarResolver after Reset = %#v, want non-nil default resolver", got)
	}

	SetCalendarResolver(original)
}

func TestTradingMinuteHelpersAndExtendedSessionFlags(t *testing.T) {
	if !IsExtendedSession(SessionPre) || !IsExtendedSession(SessionAfter) || !IsExtendedSession(SessionOvernight) {
		t.Fatal("expected pre/after/overnight sessions to count as extended")
	}
	if IsExtendedSession(SessionRegular) || IsExtendedSession(SessionClosed) {
		t.Fatal("regular/closed sessions should not count as extended")
	}

	if got, ok := TradingMinutesPerDay("US.AAPL", true); !ok || got != 24*60 {
		t.Fatalf("TradingMinutesPerDay US extended = %d, %v; want 1440, true", got, ok)
	}
	if got, ok := TradingMinutesPerTradingDay("US.AAPL", false); !ok || got != 390 {
		t.Fatalf("TradingMinutesPerTradingDay US regular = %d, %v; want 390, true", got, ok)
	}
	if got, ok := TradingMinutesPerDay("HK.00700", true); !ok || got != 330 {
		t.Fatalf("TradingMinutesPerDay HK regular = %d, %v; want 330, true", got, ok)
	}
	if got, ok := TradingMinutesPerTradingDay("SH.600519", false); !ok || got != 240 {
		t.Fatalf("TradingMinutesPerTradingDay SH regular = %d, %v; want 240, true", got, ok)
	}
	if got, ok := TradingMinutesPerTradingDay("CN.600519", false); ok || got != 0 {
		t.Fatalf("TradingMinutesPerTradingDay invalid symbol = %d, %v; want 0, false", got, ok)
	}
}

func TestCalendarAndTradingPeriodLabels(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	at := time.Date(2026, 6, 14, 20, 30, 0, 0, nyLoc)

	if got, ok := TradingDayKey("US.AAPL", at, true); !ok || got != "2026-06-15" {
		t.Fatalf("TradingDayKey = %q, %v; want 2026-06-15, true", got, ok)
	}
	if got, ok := TradingDayLabelStart("US.AAPL", at, true); !ok || !got.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("TradingDayLabelStart = %s, %v", got, ok)
	}

	if got, ok := CalendarPeriodKey("US.AAPL", time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC), "week"); !ok || got != "2026-06-08" {
		t.Fatalf("CalendarPeriodKey week = %q, %v; want 2026-06-08, true", got, ok)
	}
	if got, ok := CalendarPeriodKey("US.AAPL", time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC), "month"); !ok || got != "2026-06" {
		t.Fatalf("CalendarPeriodKey month = %q, %v; want 2026-06, true", got, ok)
	}
	if got, ok := CalendarPeriodKey("BAD", time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC), "day"); ok || got != "" {
		t.Fatalf("CalendarPeriodKey bad symbol = %q, %v; want empty, false", got, ok)
	}

	if got, ok := TradingPeriodLabelStartForDate("US.AAPL", time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC), "week"); !ok || !got.Equal(time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("TradingPeriodLabelStartForDate week = %s, %v", got, ok)
	}
	if got, ok := TradingPeriodLabelStartForDate("US.AAPL", time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC), "month"); !ok || !got.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("TradingPeriodLabelStartForDate month = %s, %v", got, ok)
	}
	if got, ok := TradingPeriodLabelStartForDate("US.AAPL", time.Date(2026, 6, 14, 21, 0, 0, 0, time.UTC), "quarter"); ok || !got.IsZero() {
		t.Fatalf("TradingPeriodLabelStartForDate invalid unit = %s, %v; want zero, false", got, ok)
	}
}

func TestParseInstrumentValidationAndProfileFormatting(t *testing.T) {
	if _, err := ParseInstrument(InstrumentInput{Market: "US", Symbol: "HK.00700"}); err == nil || !strings.Contains(err.Error(), "does not match symbol") {
		t.Fatalf("ParseInstrument market mismatch err = %v", err)
	}
	if _, err := ParseInstrument(InstrumentInput{Market: "US", Symbol: "AAPL", Code: "MSFT"}); err == nil || !strings.Contains(err.Error(), "does not match symbol") {
		t.Fatalf("ParseInstrument code mismatch err = %v", err)
	}
	if _, err := ParseInstrument(InstrumentInput{Symbol: "US."}); err == nil || !strings.Contains(err.Error(), "MARKET.CODE") {
		t.Fatalf("ParseInstrument malformed symbol err = %v", err)
	}

	profile, ok := ProfileForSymbol("HK.00700")
	if !ok {
		t.Fatal("expected HK profile")
	}
	formatted := FormatProfile(profile)
	if !strings.Contains(formatted, "HK@Asia/Hong_Kong") || !strings.Contains(formatted, "09:30-12:00") || !strings.Contains(formatted, "13:00-16:00") {
		t.Fatalf("FormatProfile = %q", formatted)
	}
}
