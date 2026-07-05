package exchangecalendar

import (
	"strings"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func policyForMarket(settings jfsettings.ExchangeCalendarSettings, market string) jfsettings.ExchangeCalendarSourcePolicy {
	normalizedMarket := normalizeMarket(market)
	for _, policy := range settings.SourcePolicies {
		if normalizeMarket(policy.Market) == normalizedMarket {
			return policy
		}
	}
	if normalizedMarket == "SH" || normalizedMarket == "SZ" {
		for _, policy := range settings.SourcePolicies {
			if normalizeMarket(policy.Market) == "CN" {
				return policy
			}
		}
	}
	return jfsettings.ExchangeCalendarSourcePolicy{
		Market:            normalizedMarket,
		FallbackToBuiltin: true,
	}
}

func sourceEnabled(settings jfsettings.ExchangeCalendarSettings, sourceID string) bool {
	if sourceID == ManualOverrideSourceID || sourceID == BuiltinSourceID {
		return true
	}
	for _, policy := range settings.SourcePolicies {
		if len(policy.EnabledSourceIDs) == 0 {
			continue
		}
		for _, id := range policy.EnabledSourceIDs {
			if strings.TrimSpace(id) == sourceID {
				return true
			}
		}
	}
	return false
}

func manualOverrideSchedule(settings jfsettings.ExchangeCalendarSettings, builtin *marketcalendar.BuiltinResolver, market string, day time.Time) (marketcalendar.TradingDaySchedule, bool) {
	if builtin == nil {
		return marketcalendar.TradingDaySchedule{}, false
	}
	template, ok := builtin.Template(market)
	if !ok {
		return marketcalendar.TradingDaySchedule{}, false
	}
	dayKey := marketcalendar.DayStart(template, day).Format("2006-01-02")
	for _, override := range settings.ManualOverrides {
		overrideMarket := normalizeMarket(override.Market)
		if overrideMarket != normalizeMarket(market) && (overrideMarket != "CN" || (normalizeMarket(market) != "SH" && normalizeMarket(market) != "SZ")) {
			continue
		}
		if strings.TrimSpace(override.Date) != dayKey {
			continue
		}
		return marketcalendar.TradingDaySchedule{
			MarketCode: normalizeMarket(market),
			Date:       marketcalendar.DayStart(template, day),
			Status:     manualStatus(override.Status),
			Sessions:   manualSessions(override.Sessions),
			Reason:     strings.TrimSpace(override.Reason),
			SourceID:   ManualOverrideSourceID,
			Observed:   override.Observed,
		}, true
	}
	return marketcalendar.TradingDaySchedule{}, false
}

func manualStatus(status string) marketcalendar.TradingDayStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(marketcalendar.TradingDayOpen):
		return marketcalendar.TradingDayOpen
	case string(marketcalendar.TradingDayClosed):
		return marketcalendar.TradingDayClosed
	case string(marketcalendar.TradingDayEarlyClose):
		return marketcalendar.TradingDayEarlyClose
	case string(marketcalendar.TradingDaySpecial):
		return marketcalendar.TradingDaySpecial
	default:
		return marketcalendar.TradingDayUnknown
	}
}

func manualSessions(sessions []jfsettings.ExchangeCalendarSessionWindow) []marketcalendar.SessionWindow {
	normalized := make([]marketcalendar.SessionWindow, 0, len(sessions))
	for _, session := range sessions {
		normalized = append(normalized, marketcalendar.SessionWindow{
			Kind:        manualSessionKind(session.Kind),
			StartMinute: session.StartMinute,
			EndMinute:   session.EndMinute,
		})
	}
	return marketcalendar.NormalizeSessions(normalized)
}

func manualSessionKind(kind string) marketcalendar.SessionKind {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(marketcalendar.SessionPre):
		return marketcalendar.SessionPre
	case string(marketcalendar.SessionRegular):
		return marketcalendar.SessionRegular
	case string(marketcalendar.SessionAfter):
		return marketcalendar.SessionAfter
	case string(marketcalendar.SessionOvernight):
		return marketcalendar.SessionOvernight
	case string(marketcalendar.SessionClosed):
		return marketcalendar.SessionClosed
	default:
		return marketcalendar.SessionUnknown
	}
}
