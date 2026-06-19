package calendar

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market/hk"
	"github.com/jftrade/jftrade-main/pkg/market/sh"
	"github.com/jftrade/jftrade-main/pkg/market/sz"
	"github.com/jftrade/jftrade-main/pkg/market/us"
)

const BuiltinSourceID = "builtin_rules"

type BuiltinResolver struct {
	templates map[string]MarketTemplate
}

func NewBuiltinResolver() *BuiltinResolver {
	mainlandRegular := []SessionWindow{
		{Kind: SessionRegular, StartMinute: 9*60 + 30, EndMinute: 11*60 + 30},
		{Kind: SessionRegular, StartMinute: 13 * 60, EndMinute: 15 * 60},
	}
	return &BuiltinResolver{
		templates: map[string]MarketTemplate{
			"US": {
				MarketCode: "US",
				Timezone:   us.LocationName,
				RegularSessions: []SessionWindow{
					{Kind: SessionRegular, StartMinute: us.RegularStartMinute, EndMinute: us.RegularEndMinute},
				},
				ExtendedSessions: []SessionWindow{
					{Kind: SessionOvernight, StartMinute: 0, EndMinute: us.PreStartMinute},
					{Kind: SessionPre, StartMinute: us.PreStartMinute, EndMinute: us.RegularStartMinute},
					{Kind: SessionAfter, StartMinute: us.RegularEndMinute, EndMinute: us.AfterEndMinute},
				},
				SupportsExtendedHours:  true,
				OvernightCarryStartMin: us.OvernightStartMinute,
			},
			"HK": {
				MarketCode: "HK",
				Timezone:   hk.LocationName,
				RegularSessions: []SessionWindow{
					{Kind: SessionRegular, StartMinute: 9*60 + 30, EndMinute: 12 * 60},
					{Kind: SessionRegular, StartMinute: 13 * 60, EndMinute: 16 * 60},
				},
			},
			"CN": {
				MarketCode:      "CN",
				Timezone:        sh.LocationName,
				RegularSessions: CopySessions(mainlandRegular),
			},
			"SH": {
				MarketCode:      "SH",
				Timezone:        sh.LocationName,
				RegularSessions: CopySessions(mainlandRegular),
			},
			"SZ": {
				MarketCode:      "SZ",
				Timezone:        sz.LocationName,
				RegularSessions: CopySessions(mainlandRegular),
			},
		},
	}
}

func (r *BuiltinResolver) Template(marketCode string) (MarketTemplate, bool) {
	if r == nil {
		return MarketTemplate{}, false
	}
	code := NormalizeMarketCode(marketCode)
	template, ok := r.templates[code]
	return template, ok
}

func (r *BuiltinResolver) Schedule(marketCode string, day time.Time) (TradingDaySchedule, bool) {
	template, ok := r.Template(marketCode)
	if !ok || day.IsZero() {
		return TradingDaySchedule{}, false
	}
	localDay := DayStart(template, day)
	switch NormalizeMarketCode(template.MarketCode) {
	case "US":
		return builtinUSSchedule(template, localDay), true
	case "HK":
		return builtinWeekdaySchedule(template, localDay), true
	case "CN", "SH", "SZ":
		return builtinMainlandSchedule(template, localDay), true
	default:
		return TradingDaySchedule{}, false
	}
}

func builtinWeekdaySchedule(template MarketTemplate, day time.Time) TradingDaySchedule {
	switch day.Weekday() {
	case time.Saturday, time.Sunday:
		return ScheduleForDate(template, TradingDayClosed, day, BuiltinSourceID, "weekend", false, nil)
	default:
		return ScheduleForDate(template, TradingDayOpen, day, BuiltinSourceID, "", false, template.RegularSessions)
	}
}

func builtinMainlandSchedule(template MarketTemplate, day time.Time) TradingDaySchedule {
	if reason, ok := mainlandHolidayReason(day); ok {
		return ScheduleForDate(template, TradingDayClosed, day, BuiltinSourceID, reason, false, nil)
	}
	return builtinWeekdaySchedule(template, day)
}

func builtinUSSchedule(template MarketTemplate, day time.Time) TradingDaySchedule {
	switch day.Weekday() {
	case time.Saturday, time.Sunday:
		return ScheduleForDate(template, TradingDayClosed, day, BuiltinSourceID, "weekend", false, nil)
	}
	if sameDay(day, observedFixedHoliday(day.Year()+1, time.January, 1)) {
		return ScheduleForDate(template, TradingDayClosed, day, BuiltinSourceID, "new_years_day_observed", true, nil)
	}
	if reason, observed, ok := usHolidayReason(day); ok {
		return ScheduleForDate(template, TradingDayClosed, day, BuiltinSourceID, reason, observed, nil)
	}
	if reason, ok := usEarlyCloseReason(day); ok {
		sessions := []SessionWindow{
			{Kind: SessionOvernight, StartMinute: 0, EndMinute: us.PreStartMinute},
			{Kind: SessionPre, StartMinute: us.PreStartMinute, EndMinute: us.RegularStartMinute},
			{Kind: SessionRegular, StartMinute: us.RegularStartMinute, EndMinute: us.EarlyRegularEndMinute},
			{Kind: SessionAfter, StartMinute: us.EarlyRegularEndMinute, EndMinute: us.EarlyAfterEndMinute},
		}
		return ScheduleForDate(template, TradingDayEarlyClose, day, BuiltinSourceID, reason, false, sessions)
	}
	sessions := []SessionWindow{
		{Kind: SessionOvernight, StartMinute: 0, EndMinute: us.PreStartMinute},
		{Kind: SessionPre, StartMinute: us.PreStartMinute, EndMinute: us.RegularStartMinute},
		{Kind: SessionRegular, StartMinute: us.RegularStartMinute, EndMinute: us.RegularEndMinute},
		{Kind: SessionAfter, StartMinute: us.RegularEndMinute, EndMinute: us.AfterEndMinute},
	}
	return ScheduleForDate(template, TradingDayOpen, day, BuiltinSourceID, "", false, sessions)
}

// Mainland builtin fallback covers current production years while the official
// remote source is still being stabilized. The dates below reflect the 2025+
// State Council holiday arrangement principles plus known holiday dates for the
// affected lunar festivals in those years.
func mainlandHolidayReason(localDay time.Time) (string, bool) {
	yearHolidays, ok := mainlandHolidayClosures[localDay.Year()]
	if !ok {
		return "", false
	}
	reason, ok := yearHolidays[localDay.Format("2006-01-02")]
	return reason, ok
}

var mainlandHolidayClosures = map[int]map[string]string{
	2025: mainlandHolidayMap(
		"2025-01-01", "new_year_holiday",
		"2025-01-28", "spring_festival_holiday",
		"2025-01-29", "spring_festival_holiday",
		"2025-01-30", "spring_festival_holiday",
		"2025-01-31", "spring_festival_holiday",
		"2025-02-03", "spring_festival_holiday",
		"2025-02-04", "spring_festival_holiday",
		"2025-04-04", "qingming_festival_holiday",
		"2025-05-01", "labour_day_holiday",
		"2025-05-02", "labour_day_holiday",
		"2025-05-05", "labour_day_holiday",
		"2025-06-02", "dragon_boat_festival_holiday",
		"2025-10-01", "national_day_holiday",
		"2025-10-02", "national_day_holiday",
		"2025-10-03", "national_day_holiday",
		"2025-10-06", "national_day_holiday",
		"2025-10-07", "national_day_holiday",
		"2025-10-08", "national_day_holiday",
	),
	2026: mainlandHolidayMap(
		"2026-01-01", "new_year_holiday",
		"2026-01-02", "new_year_holiday",
		"2026-02-16", "spring_festival_holiday",
		"2026-02-17", "spring_festival_holiday",
		"2026-02-18", "spring_festival_holiday",
		"2026-02-19", "spring_festival_holiday",
		"2026-02-20", "spring_festival_holiday",
		"2026-02-23", "spring_festival_holiday",
		"2026-04-06", "qingming_festival_holiday",
		"2026-05-01", "labour_day_holiday",
		"2026-05-04", "labour_day_holiday",
		"2026-05-05", "labour_day_holiday",
		"2026-06-19", "dragon_boat_festival_holiday",
		"2026-09-25", "mid_autumn_festival_holiday",
		"2026-10-01", "national_day_holiday",
		"2026-10-02", "national_day_holiday",
		"2026-10-05", "national_day_holiday",
		"2026-10-06", "national_day_holiday",
		"2026-10-07", "national_day_holiday",
	),
	2027: mainlandHolidayMap(
		"2027-01-01", "new_year_holiday",
		"2027-02-05", "spring_festival_holiday",
		"2027-02-08", "spring_festival_holiday",
		"2027-02-09", "spring_festival_holiday",
		"2027-02-10", "spring_festival_holiday",
		"2027-02-11", "spring_festival_holiday",
		"2027-02-12", "spring_festival_holiday",
		"2027-04-05", "qingming_festival_holiday",
		"2027-05-03", "labour_day_holiday",
		"2027-05-04", "labour_day_holiday",
		"2027-05-05", "labour_day_holiday",
		"2027-06-09", "dragon_boat_festival_holiday",
		"2027-09-15", "mid_autumn_festival_holiday",
		"2027-10-01", "national_day_holiday",
		"2027-10-04", "national_day_holiday",
		"2027-10-05", "national_day_holiday",
		"2027-10-06", "national_day_holiday",
		"2027-10-07", "national_day_holiday",
	),
}

func mainlandHolidayMap(items ...string) map[string]string {
	result := make(map[string]string, len(items)/2)
	for i := 0; i+1 < len(items); i += 2 {
		result[items[i]] = items[i+1]
	}
	return result
}

func usHolidayReason(localDay time.Time) (string, bool, bool) {
	year := localDay.Year()
	checks := []struct {
		reason   string
		date     time.Time
		observed bool
	}{
		{"new_years_day", observedFixedHoliday(year, time.January, 1), observedFixedHoliday(year, time.January, 1).Day() != 1},
		{"martin_luther_king_jr_day", nthWeekdayOfMonth(year, time.January, time.Monday, 3), false},
		{"presidents_day", nthWeekdayOfMonth(year, time.February, time.Monday, 3), false},
		{"good_friday", goodFriday(year), false},
		{"memorial_day", lastWeekdayOfMonth(year, time.May, time.Monday), false},
		{"juneteenth", observedFixedHoliday(year, time.June, 19), observedFixedHoliday(year, time.June, 19).Day() != 19},
		{"independence_day", observedFixedHoliday(year, time.July, 4), observedFixedHoliday(year, time.July, 4).Day() != 4},
		{"labor_day", firstWeekdayOfMonth(year, time.September, time.Monday), false},
		{"thanksgiving", nthWeekdayOfMonth(year, time.November, time.Thursday, 4), false},
		{"christmas_day", observedFixedHoliday(year, time.December, 25), observedFixedHoliday(year, time.December, 25).Day() != 25},
	}
	for _, check := range checks {
		if sameDay(localDay, check.date) {
			return check.reason, check.observed, true
		}
	}
	return "", false, false
}

func usEarlyCloseReason(localDay time.Time) (string, bool) {
	switch {
	case isIndependenceDayEarlyClose(localDay):
		return "independence_day_early_close", true
	case isBlackFriday(localDay):
		return "black_friday_early_close", true
	case isChristmasEveEarlyClose(localDay):
		return "christmas_eve_early_close", true
	default:
		return "", false
	}
}

func sameDay(left, right time.Time) bool {
	return !left.IsZero() && !right.IsZero() &&
		left.Year() == right.Year() &&
		left.Month() == right.Month() &&
		left.Day() == right.Day()
}

func observedFixedHoliday(year int, month time.Month, day int) time.Time {
	base := time.Date(year, month, day, 0, 0, 0, 0, us.Location())
	switch base.Weekday() {
	case time.Saturday:
		return base.AddDate(0, 0, -1)
	case time.Sunday:
		return base.AddDate(0, 0, 1)
	default:
		return base
	}
}

func firstWeekdayOfMonth(year int, month time.Month, weekday time.Weekday) time.Time {
	return nthWeekdayOfMonth(year, month, weekday, 1)
}

func nthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, nth int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, us.Location())
	delta := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, delta+7*(nth-1))
}

func lastWeekdayOfMonth(year int, month time.Month, weekday time.Weekday) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, us.Location())
	delta := (int(last.Weekday()) - int(weekday) + 7) % 7
	return last.AddDate(0, 0, -delta)
}

func goodFriday(year int) time.Time {
	return easterSunday(year).AddDate(0, 0, -2)
}

func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, us.Location())
}

func isIndependenceDayEarlyClose(local time.Time) bool {
	year := local.Year()
	var earlyClose time.Time
	switch time.Date(year, time.July, 4, 0, 0, 0, 0, us.Location()).Weekday() {
	case time.Saturday, time.Sunday:
		earlyClose = time.Date(year, time.July, 2, 0, 0, 0, 0, us.Location())
	default:
		earlyClose = time.Date(year, time.July, 3, 0, 0, 0, 0, us.Location())
	}
	return sameDay(local, earlyClose) && earlyClose.Weekday() != time.Saturday && earlyClose.Weekday() != time.Sunday
}

func isBlackFriday(local time.Time) bool {
	thanksgiving := nthWeekdayOfMonth(local.Year(), time.November, time.Thursday, 4)
	return sameDay(local, thanksgiving.AddDate(0, 0, 1))
}

func isChristmasEveEarlyClose(local time.Time) bool {
	if local.Month() != time.December || local.Day() != 24 {
		return false
	}
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	return !sameDay(local, observedFixedHoliday(local.Year(), time.December, 25))
}

func MarketTemplates() map[string]MarketTemplate {
	resolver := NewBuiltinResolver()
	templates := make(map[string]MarketTemplate, len(resolver.templates))
	for code, template := range resolver.templates {
		template.RegularSessions = CopySessions(template.RegularSessions)
		template.ExtendedSessions = CopySessions(template.ExtendedSessions)
		templates[strings.ToUpper(code)] = template
	}
	return templates
}
