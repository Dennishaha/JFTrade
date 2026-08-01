package market

import (
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

// SessionWindowInfo describes one exchange-calendar session window. StartAt is
// inclusive and EndAt is exclusive.
type SessionWindowInfo struct {
	Session     Session
	TradingDate string
	Timezone    string
	StartAt     time.Time
	EndAt       time.Time
}

// ResolveSessionWindow returns the exchange-calendar window containing at.
// Closed and unknown timestamps do not resolve to a session window.
func ResolveSessionWindow(symbol string, at time.Time) (SessionWindowInfo, bool) {
	if at.IsZero() {
		return SessionWindowInfo{}, false
	}
	profile, template, ok := sessionWindowProfile(symbol)
	if !ok {
		return SessionWindowInfo{}, false
	}
	session := ClassifySession(symbol, at)
	if session == SessionClosed || session == SessionUnknown {
		return SessionWindowInfo{}, false
	}
	if session == SessionOvernight {
		return resolveOvernightSessionWindow(symbol, profile, template, at)
	}
	schedule, ok := CurrentCalendarResolver().Schedule(profile.Market, at)
	if !ok || !marketcalendar.TradingDayHasSessions(schedule) {
		return SessionWindowInfo{}, false
	}
	local := at.In(profile.Location)
	minute := local.Hour()*60 + local.Minute()
	for _, candidate := range schedule.Sessions {
		if sessionFromCalendar(candidate.Kind) != session || minute < candidate.StartMinute || minute >= candidate.EndMinute {
			continue
		}
		return resolvedSessionWindow(template, schedule.Date, session, candidate), true
	}
	return SessionWindowInfo{}, false
}

// ResolveTradingDaySessionWindow returns a named session window for the local
// trading date containing tradingDay. For overnight, TradingDate is the day on
// which the 00:00-04:00 portion ends and the window starts on the prior day.
func ResolveTradingDaySessionWindow(symbol string, tradingDay time.Time, session Session) (SessionWindowInfo, bool) {
	if tradingDay.IsZero() || session == SessionClosed || session == SessionUnknown {
		return SessionWindowInfo{}, false
	}
	profile, template, ok := sessionWindowProfile(symbol)
	if !ok {
		return SessionWindowInfo{}, false
	}
	localDay := marketcalendar.DayStart(template, tradingDay)
	schedule, ok := CurrentCalendarResolver().Schedule(profile.Market, localDay)
	if !ok || !marketcalendar.TradingDayHasSessions(schedule) {
		return SessionWindowInfo{}, false
	}
	window, ok := marketcalendar.SessionWindowByKind(schedule, calendarSession(session))
	if !ok {
		return SessionWindowInfo{}, false
	}
	if session == SessionOvernight {
		start := localDay.AddDate(0, 0, -1).Add(time.Duration(template.OvernightCarryStartMin) * time.Minute)
		end := localDay.Add(time.Duration(window.EndMinute) * time.Minute)
		return newSessionWindowInfo(template, localDay, session, start, end), true
	}
	return resolvedSessionWindow(template, localDay, session, window), true
}

func sessionWindowProfile(symbol string) (Profile, marketcalendar.MarketTemplate, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok || profile.Location == nil {
		return Profile{}, marketcalendar.MarketTemplate{}, false
	}
	template, ok := CurrentCalendarResolver().Template(profile.Market)
	if !ok {
		return Profile{}, marketcalendar.MarketTemplate{}, false
	}
	return profile, template, true
}

func resolveOvernightSessionWindow(symbol string, profile Profile, template marketcalendar.MarketTemplate, at time.Time) (SessionWindowInfo, bool) {
	local := at.In(profile.Location)
	tradingDay := marketcalendar.DayStart(template, local)
	if local.Hour()*60+local.Minute() >= template.OvernightCarryStartMin {
		tradingDay = tradingDay.AddDate(0, 0, 1)
	}
	return ResolveTradingDaySessionWindow(symbol, tradingDay, SessionOvernight)
}

func resolvedSessionWindow(
	template marketcalendar.MarketTemplate,
	tradingDay time.Time,
	session Session,
	window marketcalendar.SessionWindow,
) SessionWindowInfo {
	localDay := marketcalendar.DayStart(template, tradingDay)
	start := localDay.Add(time.Duration(window.StartMinute) * time.Minute)
	end := localDay.Add(time.Duration(window.EndMinute) * time.Minute)
	return newSessionWindowInfo(template, localDay, session, start, end)
}

func newSessionWindowInfo(
	template marketcalendar.MarketTemplate,
	tradingDay time.Time,
	session Session,
	start time.Time,
	end time.Time,
) SessionWindowInfo {
	return SessionWindowInfo{
		Session:     session,
		TradingDate: marketcalendar.DayStart(template, tradingDay).Format("2006-01-02"),
		Timezone:    template.Timezone,
		StartAt:     start.UTC(),
		EndAt:       end.UTC(),
	}
}

func calendarSession(session Session) marketcalendar.SessionKind {
	switch session {
	case SessionPre:
		return marketcalendar.SessionPre
	case SessionRegular:
		return marketcalendar.SessionRegular
	case SessionAfter:
		return marketcalendar.SessionAfter
	case SessionOvernight:
		return marketcalendar.SessionOvernight
	default:
		return marketcalendar.SessionUnknown
	}
}
