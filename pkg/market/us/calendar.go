package us

import "time"

const (
	PreStartMinute        = 4 * 60
	RegularStartMinute    = 9*60 + 30
	RegularEndMinute      = 16 * 60
	EarlyRegularEndMinute = 13 * 60
	AfterEndMinute        = 20 * 60
	EarlyAfterEndMinute   = 17 * 60
	OvernightStartMinute  = 20 * 60
)

func IsTradingDay(at time.Time) bool {
	local := dayStart(at)
	if local.IsZero() {
		return false
	}
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	return !isFullDayHoliday(local)
}

func IsEarlyCloseDay(at time.Time) bool {
	local := dayStart(at)
	if !IsTradingDay(local) {
		return false
	}
	return isIndependenceDayEarlyClose(local) || isBlackFriday(local) || isChristmasEveEarlyClose(local)
}

func RegularSessionEndMinute(at time.Time) int {
	if IsEarlyCloseDay(at) {
		return EarlyRegularEndMinute
	}
	return RegularEndMinute
}

func AfterSessionEndMinute(at time.Time) int {
	if IsEarlyCloseDay(at) {
		return EarlyAfterEndMinute
	}
	return AfterEndMinute
}

func dayStart(at time.Time) time.Time {
	loc := Location()
	if at.IsZero() {
		return time.Time{}
	}
	local := at.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func isFullDayHoliday(local time.Time) bool {
	year := local.Year()
	return sameDay(local, observedFixedHoliday(year, time.January, 1)) ||
		sameDay(local, observedFixedHoliday(year+1, time.January, 1)) ||
		sameDay(local, nthWeekdayOfMonth(year, time.January, time.Monday, 3)) ||
		sameDay(local, nthWeekdayOfMonth(year, time.February, time.Monday, 3)) ||
		sameDay(local, goodFriday(year)) ||
		sameDay(local, lastWeekdayOfMonth(year, time.May, time.Monday)) ||
		sameDay(local, observedFixedHoliday(year, time.June, 19)) ||
		sameDay(local, observedFixedHoliday(year, time.July, 4)) ||
		sameDay(local, firstWeekdayOfMonth(year, time.September, time.Monday)) ||
		sameDay(local, nthWeekdayOfMonth(year, time.November, time.Thursday, 4)) ||
		sameDay(local, observedFixedHoliday(year, time.December, 25))
}

func isIndependenceDayEarlyClose(local time.Time) bool {
	year := local.Year()
	var earlyClose time.Time
	switch time.Date(year, time.July, 4, 0, 0, 0, 0, Location()).Weekday() {
	case time.Saturday, time.Sunday:
		earlyClose = time.Date(year, time.July, 2, 0, 0, 0, 0, Location())
	default:
		earlyClose = time.Date(year, time.July, 3, 0, 0, 0, 0, Location())
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

func sameDay(left, right time.Time) bool {
	return !left.IsZero() && !right.IsZero() &&
		left.Year() == right.Year() &&
		left.Month() == right.Month() &&
		left.Day() == right.Day()
}

func observedFixedHoliday(year int, month time.Month, day int) time.Time {
	base := time.Date(year, month, day, 0, 0, 0, 0, Location())
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
	first := time.Date(year, month, 1, 0, 0, 0, 0, Location())
	delta := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, delta+7*(nth-1))
}

func lastWeekdayOfMonth(year int, month time.Month, weekday time.Weekday) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, Location())
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
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, Location())
}
