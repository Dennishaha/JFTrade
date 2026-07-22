package futu

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

const futuWallClockLayout = "2006-01-02 15:04:05"

func futuRequestLocation(symbol string, tradeMarket int32) *time.Location {
	if profile, ok := market.ProfileForSymbol(symbol); ok && profile.Location != nil {
		return profile.Location
	}
	authority := runtimeMarketAuthority(tradeMarket)
	prefix := authority
	if authority == "CN" {
		prefix = "SH"
	}
	if profile, ok := market.ProfileForSymbol(prefix + "._"); ok && profile.Location != nil {
		return profile.Location
	}
	return time.UTC
}

func formatFutuRequestTime(at time.Time, symbol string) string {
	return at.In(futuRequestLocation(symbol, 0)).Format(futuWallClockLayout)
}

func normalizeTradeFilterTimeInput(value string, symbol string, tradeMarket int32) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.In(futuRequestLocation(symbol, tradeMarket)).Format(futuWallClockLayout)
		}
	}
	for _, layout := range []string{
		futuWallClockLayout,
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
	} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.Format(futuWallClockLayout)
		}
	}
	return trimmed
}
