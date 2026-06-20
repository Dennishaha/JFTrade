package market

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
	"github.com/jftrade/jftrade-main/pkg/market/hk"
	"github.com/jftrade/jftrade-main/pkg/market/sh"
	"github.com/jftrade/jftrade-main/pkg/market/sz"
	"github.com/jftrade/jftrade-main/pkg/market/us"
)

type MarketCode string

const (
	MarketUS MarketCode = "US"
	MarketHK MarketCode = "HK"
	MarketCN MarketCode = "CN"
	MarketSH MarketCode = "SH"
	MarketSZ MarketCode = "SZ"
)

type Session string

const (
	SessionUnknown   Session = "unknown"
	SessionClosed    Session = "closed"
	SessionPre       Session = "pre"
	SessionRegular   Session = "regular"
	SessionAfter     Session = "after"
	SessionOvernight Session = "overnight"
)

type TradingWindow struct {
	StartMinute int
	EndMinute   int
}

type Precision struct {
	Price int
	Quote int
}

type MarketDescriptor struct {
	Code                   string
	ResolvedMarket         string
	PreferredPrefix        string
	DisplayName            string
	QuoteCurrency          string
	PricePrecision         int
	QuotePrecision         int
	TickSize               float64
	SupportsExtendedHours  bool
	RequiresExchangePrefix bool
	Aliases                []string
	RegularSessions        []TradingWindow
}

type Profile struct {
	Market                 string
	ResolvedMarket         string
	PreferredPrefix        string
	DisplayName            string
	QuoteCurrency          string
	PricePrecision         int
	QuotePrecision         int
	TickSize               float64
	Aliases                []string
	Location               *time.Location
	Sessions               []TradingWindow
	ExtendedHours          bool
	RequiresExchangePrefix bool
}

type Instrument struct {
	Market string
	Prefix string
	Code   string
	Symbol string
}

type InstrumentInput struct {
	Market       string
	Symbol       string
	Code         string
	InstrumentID string
}

var profiles = map[string]Profile{
	us.Code: {
		Market:          us.Code,
		ResolvedMarket:  us.ResolvedMarket,
		PreferredPrefix: us.PreferredPrefix,
		DisplayName:     "US",
		QuoteCurrency:   "USD",
		PricePrecision:  2,
		QuotePrecision:  2,
		TickSize:        0.01,
		Aliases:         []string{"NYSE", "NASDAQ"},
		Location:        us.Location(),
		Sessions:        convertWindowPairs(us.RegularWindows),
		ExtendedHours:   true,
	},
	hk.Code: {
		Market:          hk.Code,
		ResolvedMarket:  hk.ResolvedMarket,
		PreferredPrefix: hk.PreferredPrefix,
		DisplayName:     "Hong Kong",
		QuoteCurrency:   "HKD",
		PricePrecision:  3,
		QuotePrecision:  3,
		TickSize:        0.001,
		Aliases:         []string{"HKEX"},
		Location:        hk.Location(),
		Sessions:        convertWindowPairs(hk.RegularWindows),
	},
	sh.Code: {
		Market:                 sh.Code,
		ResolvedMarket:         sh.ResolvedMarket,
		PreferredPrefix:        sh.PreferredPrefix,
		DisplayName:            "Shanghai",
		QuoteCurrency:          "CNY",
		PricePrecision:         2,
		QuotePrecision:         2,
		TickSize:               0.01,
		Aliases:                []string{"CNSH"},
		Location:               sh.Location(),
		Sessions:               convertWindowPairs(sh.RegularWindows),
		RequiresExchangePrefix: true,
	},
	sz.Code: {
		Market:                 sz.Code,
		ResolvedMarket:         sz.ResolvedMarket,
		PreferredPrefix:        sz.PreferredPrefix,
		DisplayName:            "Shenzhen",
		QuoteCurrency:          "CNY",
		PricePrecision:         2,
		QuotePrecision:         2,
		TickSize:               0.01,
		Aliases:                []string{"CNSZ"},
		Location:               sz.Location(),
		Sessions:               convertWindowPairs(sz.RegularWindows),
		RequiresExchangePrefix: true,
	},
}

var marketDescriptorOrder = []string{"HK", "US", "CN", "SH", "SZ"}

var (
	defaultCalendarResolver marketcalendar.Resolver = marketcalendar.NewBuiltinResolver()
	calendarResolverMu      sync.RWMutex
	activeCalendarResolver  marketcalendar.Resolver = defaultCalendarResolver
)

func convertWindowPairs(windows [][2]int) []TradingWindow {
	result := make([]TradingWindow, 0, len(windows))
	for _, window := range windows {
		result = append(result, TradingWindow{StartMinute: window[0], EndMinute: window[1]})
	}
	return result
}

func NormalizeMarketInput(market string) (resolvedMarket string, preferredPrefix string, err error) {
	normalized := strings.ToUpper(strings.TrimSpace(market))
	switch normalized {
	case "":
		return "", "", nil
	case "CN":
		return "CN", "", nil
	case "CNSH":
		return "CN", "SH", nil
	case "CNSZ":
		return "CN", "SZ", nil
	case "SG", "JP", "AU", "MY", "CA":
		return normalized, normalized, nil
	}
	if profile, ok := profiles[normalized]; ok {
		return profile.ResolvedMarket, profile.PreferredPrefix, nil
	}
	return "", "", fmt.Errorf("unsupported market %q", market)
}

func ParseInstrument(input InstrumentInput) (Instrument, error) {
	resolvedMarket, preferredPrefix, err := NormalizeMarketInput(input.Market)
	if err != nil {
		return Instrument{}, err
	}

	normalizedSymbol := strings.ToUpper(strings.TrimSpace(input.InstrumentID))
	if normalizedSymbol == "" {
		normalizedSymbol = strings.ToUpper(strings.TrimSpace(input.Symbol))
	}
	normalizedSymbol = strings.ReplaceAll(normalizedSymbol, ":", ".")
	normalizedCode := strings.ToUpper(strings.TrimSpace(input.Code))

	if normalizedSymbol == "" && normalizedCode == "" {
		return Instrument{}, fmt.Errorf("symbol or code is required")
	}

	if normalizedSymbol != "" {
		parsed, err := ParseQualifiedInstrumentSymbol(normalizedSymbol)
		if err == nil {
			if normalizedCode != "" && !strings.EqualFold(normalizedCode, parsed.Code) {
				return Instrument{}, fmt.Errorf("code %q does not match symbol %q", input.Code, input.Symbol)
			}
			if resolvedMarket != "" && !marketInputMatchesParsedSymbol(input.Market, parsed) {
				return Instrument{}, fmt.Errorf("market %q does not match symbol %q", input.Market, input.Symbol)
			}
			return parsed, nil
		}
		if strings.Contains(normalizedSymbol, ".") {
			return Instrument{}, err
		}
		if normalizedCode != "" && !strings.EqualFold(normalizedCode, normalizedSymbol) {
			return Instrument{}, fmt.Errorf("code %q does not match symbol %q", input.Code, input.Symbol)
		}
		normalizedCode = normalizedSymbol
	}

	if resolvedMarket == "" {
		return Instrument{}, fmt.Errorf("market is required when symbol has no market prefix")
	}
	if preferredPrefix == "" {
		return Instrument{}, fmt.Errorf("market %q requires an exchange-qualified symbol like SH.600519 or SZ.000001", input.Market)
	}

	return Instrument{
		Market: resolvedMarket,
		Prefix: preferredPrefix,
		Code:   normalizedCode,
		Symbol: preferredPrefix + "." + normalizedCode,
	}, nil
}

func ParseQualifiedInstrumentSymbol(symbol string) (Instrument, error) {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	normalized = strings.ReplaceAll(normalized, ":", ".")
	parts := strings.SplitN(normalized, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return Instrument{}, fmt.Errorf("symbol %q must be in MARKET.CODE form", symbol)
	}
	resolvedMarket, preferredPrefix, err := NormalizeMarketInput(parts[0])
	if err != nil {
		return Instrument{}, err
	}
	prefix := strings.ToUpper(strings.TrimSpace(parts[0]))
	code := strings.ToUpper(strings.TrimSpace(parts[1]))
	if preferredPrefix == "" {
		return Instrument{}, fmt.Errorf("market %q requires an exchange-qualified symbol like SH.600519 or SZ.000001", prefix)
	}
	if preferredPrefix != prefix {
		prefix = preferredPrefix
	}
	return Instrument{
		Market: resolvedMarket,
		Prefix: prefix,
		Code:   code,
		Symbol: prefix + "." + code,
	}, nil
}

func MarketInputMatchesParsedSymbol(market string, parsed Instrument) bool {
	return marketInputMatchesParsedSymbol(market, parsed)
}

func ProfileForSymbol(symbol string) (Profile, bool) {
	profile, ok := profiles[SymbolMarket(symbol)]
	return profile, ok
}

func MarketDescriptors() []MarketDescriptor {
	result := make([]MarketDescriptor, 0, len(marketDescriptorOrder))
	for _, code := range marketDescriptorOrder {
		if code == "CN" {
			result = append(result, MarketDescriptor{
				Code:                   "CN",
				ResolvedMarket:         "CN",
				DisplayName:            "China A Shares",
				QuoteCurrency:          "CNY",
				PricePrecision:         2,
				QuotePrecision:         2,
				TickSize:               0.01,
				RequiresExchangePrefix: true,
				Aliases:                []string{"SH", "SZ", "CNSH", "CNSZ"},
			})
			continue
		}
		if profile, ok := profiles[code]; ok {
			result = append(result, descriptorFromProfile(profile))
		}
	}
	return result
}

func descriptorFromProfile(profile Profile) MarketDescriptor {
	return MarketDescriptor{
		Code:                   profile.Market,
		ResolvedMarket:         profile.ResolvedMarket,
		PreferredPrefix:        profile.PreferredPrefix,
		DisplayName:            profile.DisplayName,
		QuoteCurrency:          profile.QuoteCurrency,
		PricePrecision:         profile.PricePrecision,
		QuotePrecision:         profile.QuotePrecision,
		TickSize:               profile.TickSize,
		SupportsExtendedHours:  profile.ExtendedHours,
		RequiresExchangePrefix: profile.RequiresExchangePrefix,
		Aliases:                append([]string(nil), profile.Aliases...),
		RegularSessions:        append([]TradingWindow(nil), profile.Sessions...),
	}
}

func SymbolMarket(symbol string) string {
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

func IsUSSymbol(symbol string) bool {
	return SymbolMarket(symbol) == "US"
}

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
	if !IsUSSymbol(symbol) {
		return SessionUnknown
	}
	if at.IsZero() {
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
	if !includeExtendedHours {
		if !IsRegularTradingTime(symbol, at) {
			return time.Time{}, false
		}
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, profile.Location), true
	}

	if !IsUSSymbol(symbol) {
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
