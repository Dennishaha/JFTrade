package market

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

var (
	defaultCalendarResolver marketcalendar.Resolver = marketcalendar.NewBuiltinResolver()
	calendarResolverMu      sync.RWMutex
	activeCalendarResolver  marketcalendar.Resolver = defaultCalendarResolver
)

func CurrentCalendarResolver() marketcalendar.Resolver {
	calendarResolverMu.RLock()
	defer calendarResolverMu.RUnlock()
	if activeCalendarResolver == nil {
		return defaultCalendarResolver
	}
	return activeCalendarResolver
}

func SetCalendarResolver(resolver marketcalendar.Resolver) {
	calendarResolverMu.Lock()
	defer calendarResolverMu.Unlock()
	if resolver == nil {
		activeCalendarResolver = defaultCalendarResolver
		return
	}
	activeCalendarResolver = resolver
}

func SwapCalendarResolver(resolver marketcalendar.Resolver) marketcalendar.Resolver {
	calendarResolverMu.Lock()
	defer calendarResolverMu.Unlock()
	previous := activeCalendarResolver
	if previous == nil {
		previous = defaultCalendarResolver
	}
	if resolver == nil {
		activeCalendarResolver = defaultCalendarResolver
	} else {
		activeCalendarResolver = resolver
	}
	return previous
}

func ResetCalendarResolver() {
	SetCalendarResolver(nil)
}

func ClassifySession(symbol string, at time.Time) Session {
	if !IsUSSymbol(symbol) || at.IsZero() {
		return SessionUnknown
	}
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return SessionUnknown
	}
	template, schedule, local, ok := resolveTradingDaySchedule(profile.Market, at)
	if !ok {
		return SessionUnknown
	}
	minutes := local.Hour()*60 + local.Minute()
	if local.Weekday() == time.Sunday && minutes >= template.OvernightCarryStartMin {
		if nextSchedule, ok := scheduleForMarketDay(profile.Market, local.AddDate(0, 0, 1)); ok && marketcalendar.TradingDayHasSessions(nextSchedule) {
			return SessionOvernight
		}
	}
	if !marketcalendar.TradingDayHasSessions(schedule) {
		return SessionClosed
	}
	if session, ok := marketcalendar.SessionForMinute(schedule, minutes); ok {
		return sessionFromCalendar(session)
	}
	if minutes >= template.OvernightCarryStartMin {
		if nextSchedule, ok := scheduleForMarketDay(profile.Market, local.AddDate(0, 0, 1)); ok && marketcalendar.TradingDayHasSessions(nextSchedule) {
			return SessionOvernight
		}
	}
	return SessionClosed
}

func sessionFromCalendar(session marketcalendar.SessionKind) Session {
	switch session {
	case marketcalendar.SessionClosed:
		return SessionClosed
	case marketcalendar.SessionPre:
		return SessionPre
	case marketcalendar.SessionRegular:
		return SessionRegular
	case marketcalendar.SessionAfter:
		return SessionAfter
	case marketcalendar.SessionOvernight:
		return SessionOvernight
	default:
		return SessionUnknown
	}
}

func IsExtendedSession(session Session) bool {
	return session == SessionPre || session == SessionAfter || session == SessionOvernight
}

func ShouldUseRegularCloseAsPreviousClose(symbol string, session Session, regularClose decimal.Decimal) bool {
	return IsUSSymbol(symbol) && session != SessionRegular && regularClose.GreaterThan(decimal.Zero)
}

func tradingMinutesPerRegularDay(symbol string) (int, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return 0, false
	}
	minutes := 0
	for _, session := range profile.Sessions {
		if session.EndMinute > session.StartMinute {
			minutes += session.EndMinute - session.StartMinute
		}
	}
	if minutes <= 0 {
		return 0, false
	}
	return minutes, true
}

func TradingMinutesPerDay(symbol string, includeExtendedHours bool) (int, bool) {
	return TradingMinutesPerTradingDay(symbol, includeExtendedHours)
}

func TradingMinutesPerTradingDay(symbol string, includeExtendedHours bool) (int, bool) {
	if !includeExtendedHours || !IsUSSymbol(symbol) {
		return tradingMinutesPerRegularDay(symbol)
	}
	return 24 * 60, true
}

func IsRegularTradingTime(symbol string, at time.Time) bool {
	profile, ok := ProfileForSymbol(symbol)
	if !ok || at.IsZero() {
		return false
	}
	_, schedule, local, ok := resolveTradingDaySchedule(profile.Market, at)
	if !ok || !marketcalendar.TradingDayHasSessions(schedule) {
		return false
	}
	minutes := local.Hour()*60 + local.Minute()
	for _, session := range schedule.Sessions {
		if session.Kind == marketcalendar.SessionRegular && minutes >= session.StartMinute && minutes < session.EndMinute {
			return true
		}
	}
	return false
}

func TradingDayKey(symbol string, at time.Time, includeExtendedHours bool) (string, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return "", false
	}
	localDay, ok := tradingPeriodLocalDay(profile, symbol, at, includeExtendedHours)
	if !ok {
		return "", false
	}
	return localDay.Format("2006-01-02"), true
}

func TradingDayLabelStart(symbol string, at time.Time, includeExtendedHours bool) (time.Time, bool) {
	return TradingPeriodLabelStart(symbol, at, "day", includeExtendedHours)
}

// TradingDayBoundaryStart returns the actual UTC instant where the market
// trading day containing at begins. US extended-hours trading rolls at the
// calendar template's overnight carry boundary on the previous local day.
func TradingDayBoundaryStart(symbol string, at time.Time, includeExtendedHours bool) (time.Time, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return time.Time{}, false
	}
	localDay, ok := tradingPeriodLocalDay(profile, symbol, at, includeExtendedHours)
	if !ok {
		return time.Time{}, false
	}
	if includeExtendedHours && IsUSSymbol(symbol) {
		previousDay := localDay.AddDate(0, 0, -1)
		carryStart := overnightCarryStartMinute(profile)
		return time.Date(previousDay.Year(), previousDay.Month(), previousDay.Day(), carryStart/60, carryStart%60, 0, 0, profile.Location).UTC(), true
	}
	return time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, profile.Location).UTC(), true
}

func TradingPeriodKey(symbol string, at time.Time, unit string, includeExtendedHours bool) (string, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return "", false
	}
	localDay, ok := tradingPeriodLocalDay(profile, symbol, at, includeExtendedHours)
	if !ok {
		return "", false
	}
	return tradingPeriodKeyFromLocalDay(profile, localDay, unit)
}

// CalendarPeriodKey returns the canonical market-local calendar period key
// without requiring the timestamp to fall inside a trading session.
func CalendarPeriodKey(symbol string, at time.Time, unit string) (string, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok || profile.Location == nil || at.IsZero() {
		return "", false
	}
	return tradingPeriodKeyFromLocalDay(profile, at.In(profile.Location), unit)
}

func TradingPeriodLabelStart(symbol string, at time.Time, unit string, includeExtendedHours bool) (time.Time, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return time.Time{}, false
	}
	localDay, ok := tradingPeriodLocalDay(profile, symbol, at, includeExtendedHours)
	if !ok {
		return time.Time{}, false
	}
	return tradingPeriodLabelStartFromLocalDay(localDay, unit)
}

func TradingPeriodLabelStartForDate(symbol string, at time.Time, unit string) (time.Time, bool) {
	profile, ok := ProfileForSymbol(symbol)
	if !ok || at.IsZero() {
		return time.Time{}, false
	}
	dayLabel := time.Date(at.UTC().Year(), at.UTC().Month(), at.UTC().Day(), 0, 0, 0, 0, time.UTC)
	localDay, err := time.ParseInLocation("2006-01-02", dayLabel.Format("2006-01-02"), profile.Location)
	if err != nil {
		return time.Time{}, false
	}
	key, ok := tradingPeriodKeyFromLocalDay(profile, localDay, unit)
	if !ok {
		return time.Time{}, false
	}
	return tradingPeriodLabelStartFromKey(key, unit)
}

// SessionAwareIntradayBucketBounds returns the closed UTC bucket range
// [start, end] for storage/query code that treats K-line end timestamps as
// inclusive. Internally session windows are still evaluated as [start, end).
func SessionAwareIntradayBucketBounds(symbol string, at time.Time, interval time.Duration, includeExtendedHours bool) (time.Time, time.Time, bool) {
	if at.IsZero() || interval <= 0 {
		return time.Time{}, time.Time{}, false
	}
	sessionStart, sessionEndExclusive, ok := sessionWindowBounds(symbol, at, includeExtendedHours)
	if !ok || at.Before(sessionStart) || !at.Before(sessionEndExclusive) {
		return time.Time{}, time.Time{}, false
	}
	offset := at.Sub(sessionStart)
	bucketStart := sessionStart.Add(offset.Truncate(interval))
	bucketEndExclusive := bucketStart.Add(interval)
	if bucketEndExclusive.After(sessionEndExclusive) {
		bucketEndExclusive = sessionEndExclusive
	}
	return bucketStart.UTC(), bucketEndExclusive.Add(-time.Millisecond).UTC(), true
}

func sessionWindowBounds(symbol string, at time.Time, includeExtendedHours bool) (time.Time, time.Time, bool) {
	if includeExtendedHours && IsUSSymbol(symbol) {
		return usExtendedSessionWindowBounds(symbol, at)
	}
	profile, ok := ProfileForSymbol(symbol)
	if !ok || at.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	return regularSessionWindowBounds(profile, at)
}

func regularSessionWindowBounds(profile Profile, at time.Time) (time.Time, time.Time, bool) {
	if at.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	template, schedule, local, ok := resolveTradingDaySchedule(profile.Market, at)
	if !ok || !marketcalendar.TradingDayHasSessions(schedule) {
		return time.Time{}, time.Time{}, false
	}
	minutes := local.Hour()*60 + local.Minute()
	for _, session := range schedule.Sessions {
		if session.Kind == marketcalendar.SessionRegular && minutes >= session.StartMinute && minutes < session.EndMinute {
			dayStart := marketcalendar.DayStart(template, local)
			startAt := dayStart.Add(time.Duration(session.StartMinute) * time.Minute)
			endAt := dayStart.Add(time.Duration(session.EndMinute) * time.Minute)
			return startAt.UTC(), endAt.UTC(), true
		}
	}
	return time.Time{}, time.Time{}, false
}

func usExtendedSessionWindowBounds(symbol string, at time.Time) (time.Time, time.Time, bool) {
	if at.IsZero() || !IsUSSymbol(symbol) {
		return time.Time{}, time.Time{}, false
	}
	session := ClassifySession(symbol, at)
	if session == SessionClosed || session == SessionUnknown {
		return time.Time{}, time.Time{}, false
	}
	profile, ok := ProfileForSymbol(symbol)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	template, schedule, local, ok := resolveTradingDaySchedule(profile.Market, at)
	if !ok || !marketcalendar.TradingDayHasSessions(schedule) {
		return time.Time{}, time.Time{}, false
	}
	minutes := local.Hour()*60 + local.Minute()
	dayStart := marketcalendar.DayStart(template, local)
	switch session {
	case SessionOvernight:
		if minutes >= template.OvernightCarryStartMin {
			return dayStart.Add(time.Duration(template.OvernightCarryStartMin) * time.Minute).UTC(), dayStart.Add(24 * time.Hour).UTC(), true
		}
		if overnight, ok := marketcalendar.SessionWindowByKind(schedule, marketcalendar.SessionOvernight); ok {
			return dayStart.Add(time.Duration(overnight.StartMinute) * time.Minute).UTC(), dayStart.Add(time.Duration(overnight.EndMinute) * time.Minute).UTC(), true
		}
		return time.Time{}, time.Time{}, false
	case SessionPre:
		if pre, ok := marketcalendar.SessionWindowByKind(schedule, marketcalendar.SessionPre); ok {
			return dayStart.Add(time.Duration(pre.StartMinute) * time.Minute).UTC(), dayStart.Add(time.Duration(pre.EndMinute) * time.Minute).UTC(), true
		}
		return time.Time{}, time.Time{}, false
	case SessionRegular:
		if regular, ok := marketcalendar.SessionWindowByKind(schedule, marketcalendar.SessionRegular); ok {
			return dayStart.Add(time.Duration(regular.StartMinute) * time.Minute).UTC(), dayStart.Add(time.Duration(regular.EndMinute) * time.Minute).UTC(), true
		}
		return time.Time{}, time.Time{}, false
	case SessionAfter:
		if after, ok := marketcalendar.SessionWindowByKind(schedule, marketcalendar.SessionAfter); ok {
			return dayStart.Add(time.Duration(after.StartMinute) * time.Minute).UTC(), dayStart.Add(time.Duration(after.EndMinute) * time.Minute).UTC(), true
		}
		return time.Time{}, time.Time{}, false
	default:
		return time.Time{}, time.Time{}, false
	}
}

func tradingPeriodKeyFromLocalDay(profile Profile, localDay time.Time, unit string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "day":
		dayStart := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, profile.Location)
		return dayStart.Format("2006-01-02"), true
	case "week":
		weekStart := startOfWeek(localDay)
		return weekStart.Format("2006-01-02"), true
	case "month":
		monthStart := time.Date(localDay.Year(), localDay.Month(), 1, 0, 0, 0, 0, profile.Location)
		return monthStart.Format("2006-01"), true
	default:
		return "", false
	}
}

func tradingPeriodLocalDay(profile Profile, symbol string, at time.Time, includeExtendedHours bool) (time.Time, bool) {
	if at.IsZero() {
		return time.Time{}, false
	}
	local := at.In(profile.Location)
	if !includeExtendedHours || !IsUSSymbol(symbol) {
		if !IsRegularTradingTime(symbol, at) {
			return time.Time{}, false
		}
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location), true
	}
	session := ClassifySession(symbol, at)
	switch session {
	case SessionClosed, SessionUnknown:
		return time.Time{}, false
	case SessionOvernight:
		if local.Hour()*60+local.Minute() >= overnightCarryStartMinute(profile) {
			local = local.AddDate(0, 0, 1)
		}
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location), true
}

func overnightCarryStartMinute(profile Profile) int {
	if template, ok := CurrentCalendarResolver().Template(profile.Market); ok {
		return template.OvernightCarryStartMin
	}
	return 20 * 60
}

func tradingPeriodLabelStartFromLocalDay(localDay time.Time, unit string) (time.Time, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "day":
		return time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, time.UTC), true
	case "week":
		weekStart := startOfWeek(localDay)
		return time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC), true
	case "month":
		return time.Date(localDay.Year(), localDay.Month(), 1, 0, 0, 0, 0, time.UTC), true
	default:
		return time.Time{}, false
	}
}

func tradingPeriodLabelStartFromKey(key string, unit string) (time.Time, bool) {
	normalizedUnit := strings.ToLower(strings.TrimSpace(unit))
	var (
		labelAt time.Time
		err     error
	)
	switch normalizedUnit {
	case "day", "week":
		labelAt, err = time.ParseInLocation("2006-01-02", key, time.UTC)
	case "month":
		labelAt, err = time.ParseInLocation("2006-01", key, time.UTC)
		if err == nil {
			labelAt = time.Date(labelAt.Year(), labelAt.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
	default:
		return time.Time{}, false
	}
	if err != nil {
		return time.Time{}, false
	}
	return labelAt.UTC(), true
}

func startOfWeek(at time.Time) time.Time {
	weekday := int(at.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	dayStart := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
	return dayStart.AddDate(0, 0, -(weekday - 1))
}

func resolveTradingDaySchedule(marketCode string, at time.Time) (marketcalendar.MarketTemplate, marketcalendar.TradingDaySchedule, time.Time, bool) {
	resolver := CurrentCalendarResolver()
	template, ok := resolver.Template(marketCode)
	if !ok || at.IsZero() {
		return marketcalendar.MarketTemplate{}, marketcalendar.TradingDaySchedule{}, time.Time{}, false
	}
	local := at.In(marketcalendar.LoadLocation(template))
	schedule, ok := resolver.Schedule(marketCode, local)
	if !ok {
		return marketcalendar.MarketTemplate{}, marketcalendar.TradingDaySchedule{}, time.Time{}, false
	}
	return template, schedule, local, true
}

func scheduleForMarketDay(marketCode string, day time.Time) (marketcalendar.TradingDaySchedule, bool) {
	resolver := CurrentCalendarResolver()
	template, ok := resolver.Template(marketCode)
	if !ok || day.IsZero() {
		return marketcalendar.TradingDaySchedule{}, false
	}
	return resolver.Schedule(marketCode, marketcalendar.DayStart(template, day))
}

func marketInputMatchesParsedSymbol(market string, parsed Instrument) bool {
	resolvedMarket, preferredPrefix, err := NormalizeMarketInput(market)
	if err != nil {
		return false
	}
	if resolvedMarket != parsed.Market {
		return false
	}
	if preferredPrefix == "" {
		return true
	}
	return preferredPrefix == parsed.Prefix
}

func FormatProfile(profile Profile) string {
	parts := make([]string, 0, len(profile.Sessions))
	for _, session := range profile.Sessions {
		parts = append(parts, fmt.Sprintf("%02d:%02d-%02d:%02d", session.StartMinute/60, session.StartMinute%60, session.EndMinute/60, session.EndMinute%60))
	}
	return fmt.Sprintf("%s@%s[%s]", profile.Market, profile.Location.String(), strings.Join(parts, ","))
}
