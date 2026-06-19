package calendar

import "time"

func SnapshotSchedule(snapshot CalendarSnapshot, marketCode string, day time.Time) (TradingDaySchedule, bool) {
	marketCode = NormalizeMarketCode(marketCode)
	for _, schedule := range snapshot.Schedules {
		if NormalizeMarketCode(schedule.MarketCode) != marketCode {
			continue
		}
		if sameDay(schedule.Date, day) {
			return schedule, true
		}
	}
	return TradingDaySchedule{}, false
}

func SnapshotCoversDay(snapshot CalendarSnapshot, template MarketTemplate, day time.Time) bool {
	if snapshot.From.IsZero() || snapshot.To.IsZero() || day.IsZero() {
		return false
	}
	normalizedDay := DayStart(template, day)
	from := DayStart(template, snapshot.From)
	to := DayStart(template, snapshot.To)
	return !normalizedDay.Before(from) && !normalizedDay.After(to)
}
