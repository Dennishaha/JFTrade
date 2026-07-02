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

func TestMarketResolverAndSessionBoundaryFallbacks(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	original := SwapCalendarResolver(&testCalendarResolver{
		template: marketcalendar.MarketTemplate{
			MarketCode:             "US",
			Timezone:               "America/New_York",
			SupportsExtendedHours:  true,
			OvernightCarryStartMin: 20 * 60,
		},
		schedules: map[string]marketcalendar.TradingDaySchedule{},
	})
	t.Cleanup(func() { SetCalendarResolver(original) })

	if got := ClassifySession("US.AAPL", time.Date(2026, 6, 15, 10, 0, 0, 0, nyLoc)); got != SessionUnknown {
		t.Fatalf("ClassifySession with missing schedule = %s, want unknown", got)
	}
	if IsRegularTradingTime("US.AAPL", time.Date(2026, 6, 15, 10, 0, 0, 0, nyLoc)) {
		t.Fatal("missing schedule should not be regular trading time")
	}
	if _, ok := scheduleForMarketDay("US", time.Time{}); ok {
		t.Fatal("zero market day should not resolve schedule")
	}
	if _, _, ok := sessionWindowBounds("US.AAPL", time.Time{}, true); ok {
		t.Fatal("zero time should not resolve session window")
	}
	if _, _, ok := usExtendedSessionWindowBounds("HK.00700", time.Date(2026, 6, 15, 10, 0, 0, 0, nyLoc)); ok {
		t.Fatal("non-US symbol should not use US extended window")
	}
}

func TestUSAfterHoursWindowsAndPeriodLabelBoundaries(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	preStart, preEnd, ok := sessionWindowBounds("US.AAPL", time.Date(2026, 6, 15, 8, 0, 0, 0, nyLoc), true)
	if !ok {
		t.Fatal("expected US pre-market window")
	}
	if !preStart.Equal(time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)) || !preEnd.Equal(time.Date(2026, 6, 15, 13, 30, 0, 0, time.UTC)) {
		t.Fatalf("pre-market window = %s - %s", preStart, preEnd)
	}
	regularStart, regularEnd, ok := sessionWindowBounds("US.AAPL", time.Date(2026, 6, 15, 10, 0, 0, 0, nyLoc), true)
	if !ok {
		t.Fatal("expected US regular window under extended mode")
	}
	if !regularStart.Equal(time.Date(2026, 6, 15, 13, 30, 0, 0, time.UTC)) || !regularEnd.Equal(time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("regular window = %s - %s", regularStart, regularEnd)
	}
	afterStart, afterEnd, ok := sessionWindowBounds("US.AAPL", time.Date(2026, 6, 12, 17, 0, 0, 0, nyLoc), true)
	if !ok {
		t.Fatal("expected US after-hours window")
	}
	if !afterStart.Equal(time.Date(2026, 6, 12, 20, 0, 0, 0, time.UTC)) || !afterEnd.Equal(time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("after-hours window = %s - %s", afterStart, afterEnd)
	}
	if _, _, ok := sessionWindowBounds("US.AAPL", time.Date(2026, 6, 12, 20, 0, 0, 0, nyLoc), true); ok {
		t.Fatal("Friday 20:00 should be closed because next trading day is not Saturday")
	}
	if _, _, ok := SessionAwareIntradayBucketBounds("US.AAPL", time.Date(2026, 6, 12, 17, 30, 0, 0, nyLoc), 0, true); ok {
		t.Fatal("zero interval should not produce intraday bucket")
	}
	overnightStart, overnightEnd, ok := sessionWindowBounds("US.AAPL", time.Date(2026, 6, 15, 3, 0, 0, 0, nyLoc), true)
	if !ok {
		t.Fatal("expected US overnight window before pre-market")
	}
	if !overnightStart.Equal(time.Date(2026, 6, 15, 4, 0, 0, 0, time.UTC)) || !overnightEnd.Equal(time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("overnight window = %s - %s", overnightStart, overnightEnd)
	}
	at := time.Date(2026, 6, 14, 20, 30, 0, 0, nyLoc)
	if got, ok := TradingPeriodKey("US.AAPL", at, "week", true); !ok || got != "2026-06-15" {
		t.Fatalf("TradingPeriodKey week = %q, %v; want 2026-06-15 true", got, ok)
	}
	if got, ok := TradingPeriodKey("US.AAPL", at, "month", true); !ok || got != "2026-06" {
		t.Fatalf("TradingPeriodKey month = %q, %v; want 2026-06 true", got, ok)
	}
	if got, ok := TradingPeriodKey("US.AAPL", at, "quarter", true); ok || got != "" {
		t.Fatalf("TradingPeriodKey invalid unit = %q, %v; want empty false", got, ok)
	}
	if got, ok := TradingPeriodLabelStart("US.AAPL", at, "month", true); !ok || !got.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("TradingPeriodLabelStart month = %s, %v", got, ok)
	}
	if got, ok := TradingPeriodLabelStart("US.AAPL", at, "quarter", true); ok || !got.IsZero() {
		t.Fatalf("TradingPeriodLabelStart invalid unit = %s, %v; want zero false", got, ok)
	}
	if got := sessionFromCalendar(marketcalendar.SessionKind("auction")); got != SessionUnknown {
		t.Fatalf("sessionFromCalendar unknown = %s, want unknown", got)
	}
}

func TestNormalizeMarketInputAdditionalAliases(t *testing.T) {
	resolved, prefix, err := NormalizeMarketInput(" sg ")
	if err != nil || resolved != "SG" || prefix != "SG" {
		t.Fatalf("NormalizeMarketInput(SG) = %q/%q err=%v", resolved, prefix, err)
	}
	if _, _, err := NormalizeMarketInput("mars"); err == nil || !strings.Contains(err.Error(), "unsupported market") {
		t.Fatalf("NormalizeMarketInput unsupported err = %v", err)
	}
	if _, err := ParseInstrument(InstrumentInput{}); err == nil || !strings.Contains(err.Error(), "symbol or code is required") {
		t.Fatalf("ParseInstrument empty err = %v", err)
	}
	if _, err := ParseQualifiedInstrumentSymbol("AAPL"); err == nil || !strings.Contains(err.Error(), "MARKET.CODE") {
		t.Fatalf("ParseQualifiedInstrumentSymbol unqualified err = %v", err)
	}
}

func TestCustomCalendarSessionWindowsAndClosedDays(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	template := marketcalendar.MarketTemplate{
		MarketCode:             "US",
		Timezone:               "America/New_York",
		SupportsExtendedHours:  true,
		OvernightCarryStartMin: 20 * 60,
	}
	openDay := marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayOpen, time.Date(2026, 6, 15, 0, 0, 0, 0, nyLoc), "unit", "", false, []marketcalendar.SessionWindow{
		{Kind: marketcalendar.SessionOvernight, StartMinute: 0, EndMinute: 4 * 60},
		{Kind: marketcalendar.SessionPre, StartMinute: 4 * 60, EndMinute: 9*60 + 30},
		{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
		{Kind: marketcalendar.SessionAfter, StartMinute: 16 * 60, EndMinute: 20 * 60},
	})
	closedDay := marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayClosed, time.Date(2026, 6, 16, 0, 0, 0, 0, nyLoc), "unit", "holiday", false, nil)
	original := SwapCalendarResolver(&testCalendarResolver{
		template: template,
		schedules: map[string]marketcalendar.TradingDaySchedule{
			"2026-06-15": openDay,
			"2026-06-16": closedDay,
		},
	})
	t.Cleanup(func() { SetCalendarResolver(original) })

	if got := ClassifySession("US.AAPL", time.Date(2026, 6, 15, 12, 0, 0, 0, nyLoc)); got != SessionRegular {
		t.Fatalf("ClassifySession regular = %s, want %s", got, SessionRegular)
	}
	if got := ClassifySession("US.AAPL", time.Date(2026, 6, 16, 12, 0, 0, 0, nyLoc)); got != SessionClosed {
		t.Fatalf("ClassifySession closed schedule = %s, want %s", got, SessionClosed)
	}
	if got, ok := TradingDayKey("US.AAPL", time.Date(2026, 6, 16, 12, 0, 0, 0, nyLoc), true); ok || got != "" {
		t.Fatalf("TradingDayKey closed day = %q, %v; want empty false", got, ok)
	}
	if _, _, ok := SessionAwareIntradayBucketBounds("US.AAPL", time.Date(2026, 6, 16, 12, 0, 0, 0, nyLoc), time.Hour, true); ok {
		t.Fatal("closed schedule should not produce a bucket")
	}
}

func TestSessionWindowBoundsRejectsMissingSessionWindows(t *testing.T) {
	nyLoc := mustLocation(t, "America/New_York")
	template := marketcalendar.MarketTemplate{
		MarketCode:             "US",
		Timezone:               "America/New_York",
		SupportsExtendedHours:  true,
		OvernightCarryStartMin: 20 * 60,
	}
	cases := []struct {
		name     string
		at       time.Time
		sessions []marketcalendar.SessionWindow
	}{
		{
			name: "classified pre without pre window",
			at:   time.Date(2026, 6, 15, 5, 0, 0, 0, nyLoc),
			sessions: []marketcalendar.SessionWindow{
				{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
			},
		},
		{
			name: "classified after without after window",
			at:   time.Date(2026, 6, 15, 17, 0, 0, 0, nyLoc),
			sessions: []marketcalendar.SessionWindow{
				{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
			},
		},
		{
			name: "classified overnight without overnight window",
			at:   time.Date(2026, 6, 15, 2, 0, 0, 0, nyLoc),
			sessions: []marketcalendar.SessionWindow{
				{Kind: marketcalendar.SessionRegular, StartMinute: 9*60 + 30, EndMinute: 16 * 60},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := SwapCalendarResolver(&testCalendarResolver{
				template: template,
				schedules: map[string]marketcalendar.TradingDaySchedule{
					"2026-06-15": marketcalendar.ScheduleForDate(template, marketcalendar.TradingDayOpen, tc.at, "unit", "", false, tc.sessions),
				},
			})
			t.Cleanup(func() { SetCalendarResolver(original) })

			if _, _, ok := sessionWindowBounds("US.AAPL", tc.at, true); ok {
				t.Fatal("missing classified session window should not return bounds")
			}
		})
	}
}

func TestNonUSExtendedRequestsUseRegularWindowsAndLabelValidation(t *testing.T) {
	hkLoc := mustLocation(t, "Asia/Hong_Kong")
	at := time.Date(2026, 6, 12, 13, 30, 0, 0, hkLoc)

	start, end, ok := sessionWindowBounds("HK.00700", at, true)
	if !ok {
		t.Fatal("HK extended request should use regular market windows")
	}
	if !start.Equal(time.Date(2026, 6, 12, 5, 0, 0, 0, time.UTC)) || !end.Equal(time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("HK regular afternoon window = %s - %s", start, end)
	}
	if got, ok := TradingDayBoundaryStart("BAD", at, true); ok || !got.IsZero() {
		t.Fatalf("TradingDayBoundaryStart bad symbol = %s, %v; want zero false", got, ok)
	}
	if got, ok := TradingPeriodLabelStart("BAD", at, "day", true); ok || !got.IsZero() {
		t.Fatalf("TradingPeriodLabelStart bad symbol = %s, %v; want zero false", got, ok)
	}
	if got, ok := tradingPeriodLabelStartFromLocalDay(at, "quarter"); ok || !got.IsZero() {
		t.Fatalf("tradingPeriodLabelStartFromLocalDay invalid unit = %s, %v; want zero false", got, ok)
	}
	if got, ok := tradingPeriodLabelStartFromKey("not-a-day", "day"); ok || !got.IsZero() {
		t.Fatalf("tradingPeriodLabelStartFromKey invalid day = %s, %v; want zero false", got, ok)
	}
	if got, ok := tradingPeriodLabelStartFromKey("2026-13", "month"); ok || !got.IsZero() {
		t.Fatalf("tradingPeriodLabelStartFromKey invalid month = %s, %v; want zero false", got, ok)
	}
	if got, ok := tradingPeriodLabelStartFromKey("2026-06-12", "quarter"); ok || !got.IsZero() {
		t.Fatalf("tradingPeriodLabelStartFromKey invalid unit = %s, %v; want zero false", got, ok)
	}
}
