package futu

import (
	"fmt"
	"strings"
	"time"
)

type TradingSessionWindow struct {
	StartMinute int
	EndMinute   int
}

type TradingProfile struct {
	Market   string
	Location *time.Location
	Sessions []TradingSessionWindow
}

var hongKongLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Hong_Kong")
	if err != nil {
		return time.UTC
	}
	return loc
}()

var shanghaiLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return loc
}()

var tradingProfiles = map[string]TradingProfile{
	"US": {
		Market:   "US",
		Location: usEasternLocation,
		Sessions: []TradingSessionWindow{{StartMinute: 9*60 + 30, EndMinute: 16 * 60}},
	},
	"HK": {
		Market:   "HK",
		Location: hongKongLocation,
		Sessions: []TradingSessionWindow{
			{StartMinute: 9*60 + 30, EndMinute: 12 * 60},
			{StartMinute: 13 * 60, EndMinute: 16 * 60},
		},
	},
	"SH": {
		Market:   "SH",
		Location: shanghaiLocation,
		Sessions: []TradingSessionWindow{
			{StartMinute: 9*60 + 30, EndMinute: 11*60 + 30},
			{StartMinute: 13 * 60, EndMinute: 15 * 60},
		},
	},
	"SZ": {
		Market:   "SZ",
		Location: shanghaiLocation,
		Sessions: []TradingSessionWindow{
			{StartMinute: 9*60 + 30, EndMinute: 11*60 + 30},
			{StartMinute: 13 * 60, EndMinute: 15 * 60},
		},
	},
}

func TradingProfileForSymbol(symbol string) (TradingProfile, bool) {
	profile, ok := tradingProfiles[tradingProfileMarket(symbol)]
	return profile, ok
}

func TradingMinutesPerRegularDay(symbol string) (int, bool) {
	profile, ok := TradingProfileForSymbol(symbol)
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

func TradingMinutesPerTradingDay(symbol string, includeExtendedHours bool) (int, bool) {
	if !includeExtendedHours || tradingProfileMarket(symbol) != "US" {
		return TradingMinutesPerRegularDay(symbol)
	}
	// US extended replay currently models a continuous trading day from
	// 20:00 ET of the previous calendar day to 20:00 ET of the trading day.
	return 24 * 60, true
}

func IsRegularTradingTime(symbol string, at time.Time) bool {
	profile, ok := TradingProfileForSymbol(symbol)
	if !ok || at.IsZero() {
		return false
	}
	local := at.In(profile.Location)
	minutes := local.Hour()*60 + local.Minute()
	for _, session := range profile.Sessions {
		if minutes >= session.StartMinute && minutes < session.EndMinute {
			return true
		}
	}
	return false
}

func RegularTradingPeriodKey(symbol string, at time.Time, unit string) (string, bool) {
	profile, ok := TradingProfileForSymbol(symbol)
	if !ok || at.IsZero() || !IsRegularTradingTime(symbol, at) {
		return "", false
	}
	local := at.In(profile.Location)
	return tradingPeriodKeyFromLocalDay(profile, local, unit)
}

func TradingDayKey(symbol string, at time.Time, includeExtendedHours bool) (string, bool) {
	profile, ok := TradingProfileForSymbol(symbol)
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

func TradingPeriodKey(symbol string, at time.Time, unit string, includeExtendedHours bool) (string, bool) {
	profile, ok := TradingProfileForSymbol(symbol)
	if !ok {
		return "", false
	}
	localDay, ok := tradingPeriodLocalDay(profile, symbol, at, includeExtendedHours)
	if !ok {
		return "", false
	}
	return tradingPeriodKeyFromLocalDay(profile, localDay, unit)
}

func TradingPeriodLabelStart(symbol string, at time.Time, unit string, includeExtendedHours bool) (time.Time, bool) {
	profile, ok := TradingProfileForSymbol(symbol)
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
	profile, ok := TradingProfileForSymbol(symbol)
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

func SessionAwareIntradayBucketBounds(symbol string, at time.Time, interval time.Duration, includeExtendedHours bool) (time.Time, time.Time, bool) {
	if at.IsZero() || interval <= 0 {
		return time.Time{}, time.Time{}, false
	}

	sessionStart, sessionEndExclusive, ok := sessionWindowBounds(symbol, at, includeExtendedHours)
	if !ok || at.Before(sessionStart) || !at.Before(sessionEndExclusive) {
		return time.Time{}, time.Time{}, false
	}

	offset := at.Sub(sessionStart)
	bucketStart := sessionStart.Add((offset / interval) * interval)
	bucketEndExclusive := bucketStart.Add(interval)
	if bucketEndExclusive.After(sessionEndExclusive) {
		bucketEndExclusive = sessionEndExclusive
	}
	return bucketStart.UTC(), bucketEndExclusive.Add(-time.Millisecond).UTC(), true
}

func sessionWindowBounds(symbol string, at time.Time, includeExtendedHours bool) (time.Time, time.Time, bool) {
	market := tradingProfileMarket(symbol)
	if includeExtendedHours && market == "US" {
		return usExtendedSessionWindowBounds(symbol, at)
	}

	profile, ok := TradingProfileForSymbol(symbol)
	if !ok || at.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	return regularSessionWindowBounds(profile, at)
}

func regularSessionWindowBounds(profile TradingProfile, at time.Time) (time.Time, time.Time, bool) {
	if at.IsZero() {
		return time.Time{}, time.Time{}, false
	}

	local := at.In(profile.Location)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return time.Time{}, time.Time{}, false
	}

	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location)
	minutes := local.Hour()*60 + local.Minute()
	for _, session := range profile.Sessions {
		if minutes >= session.StartMinute && minutes < session.EndMinute {
			startAt := dayStart.Add(time.Duration(session.StartMinute) * time.Minute)
			endAt := dayStart.Add(time.Duration(session.EndMinute) * time.Minute)
			return startAt.UTC(), endAt.UTC(), true
		}
	}
	return time.Time{}, time.Time{}, false
}

func usExtendedSessionWindowBounds(symbol string, at time.Time) (time.Time, time.Time, bool) {
	if at.IsZero() || tradingProfileMarket(symbol) != "US" {
		return time.Time{}, time.Time{}, false
	}

	session := ClassifyMarketSession(symbol, at)
	if session == MarketSessionClosed || session == MarketSessionUnknown {
		return time.Time{}, time.Time{}, false
	}

	local := at.In(usEasternLocation)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, usEasternLocation)
	minutes := local.Hour()*60 + local.Minute()

	switch session {
	case MarketSessionOvernight:
		if minutes >= 20*60 {
			return dayStart.Add(20 * time.Hour).UTC(), dayStart.Add(24 * time.Hour).UTC(), true
		}
		return dayStart.UTC(), dayStart.Add(4 * time.Hour).UTC(), true
	case MarketSessionPre:
		return dayStart.Add(4 * time.Hour).UTC(), dayStart.Add(9*time.Hour + 30*time.Minute).UTC(), true
	case MarketSessionRegular:
		return dayStart.Add(9*time.Hour + 30*time.Minute).UTC(), dayStart.Add(16 * time.Hour).UTC(), true
	case MarketSessionAfter:
		return dayStart.Add(16 * time.Hour).UTC(), dayStart.Add(20 * time.Hour).UTC(), true
	default:
		return time.Time{}, time.Time{}, false
	}
}

func tradingPeriodKeyFromLocalDay(profile TradingProfile, localDay time.Time, unit string) (string, bool) {
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

func tradingPeriodLocalDay(profile TradingProfile, symbol string, at time.Time, includeExtendedHours bool) (time.Time, bool) {
	if at.IsZero() {
		return time.Time{}, false
	}
	local := at.In(profile.Location)
	if !includeExtendedHours {
		if !IsRegularTradingTime(symbol, at) {
			return time.Time{}, false
		}
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location), true
	}

	if tradingProfileMarket(symbol) != "US" {
		if !IsRegularTradingTime(symbol, at) {
			return time.Time{}, false
		}
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location), true
	}

	session := ClassifyMarketSession(symbol, at)
	switch session {
	case MarketSessionClosed, MarketSessionUnknown:
		return time.Time{}, false
	case MarketSessionOvernight:
		if local.Hour()*60+local.Minute() >= 20*60 {
			local = local.AddDate(0, 0, 1)
		}
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location), true
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

func tradingProfileMarket(symbol string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(symbol))
	switch {
	case strings.HasPrefix(trimmed, "US."):
		return "US"
	case strings.HasPrefix(trimmed, "HK."):
		return "HK"
	case strings.HasPrefix(trimmed, "SH."):
		return "SH"
	case strings.HasPrefix(trimmed, "SZ."):
		return "SZ"
	default:
		return ""
	}
}

func startOfWeek(at time.Time) time.Time {
	weekday := int(at.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	dayStart := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
	return dayStart.AddDate(0, 0, -(weekday - 1))
}

func FormatTradingProfile(profile TradingProfile) string {
	parts := make([]string, 0, len(profile.Sessions))
	for _, session := range profile.Sessions {
		parts = append(parts, fmt.Sprintf("%02d:%02d-%02d:%02d", session.StartMinute/60, session.StartMinute%60, session.EndMinute/60, session.EndMinute%60))
	}
	return fmt.Sprintf("%s@%s[%s]", profile.Market, profile.Location.String(), strings.Join(parts, ","))
}
