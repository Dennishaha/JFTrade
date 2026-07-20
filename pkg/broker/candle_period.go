package broker

import (
	"fmt"
	"strings"
)

var canonicalHistoricalCandlePeriods = []string{
	"1m", "3m", "5m", "10m", "15m", "30m", "1h", "1d", "1w", "1mo",
}

func SupportedHistoricalCandlePeriods() []string {
	return append([]string(nil), canonicalHistoricalCandlePeriods...)
}

func NormalizeCandlePeriod(period string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "tick", "ticker", "k_tick":
		return "tick", nil
	case "1m", "1min", "k_1m":
		return "1m", nil
	case "3m", "3min", "k_3m":
		return "3m", nil
	case "5m", "5min", "k_5m":
		return "5m", nil
	case "10m", "10min", "k_10m":
		return "10m", nil
	case "15m", "15min", "k_15m":
		return "15m", nil
	case "30m", "30min", "k_30m":
		return "30m", nil
	case "60m", "60min", "1h", "1hour", "k_60m":
		return "1h", nil
	case "1d", "day", "daily", "d", "k_day":
		return "1d", nil
	case "1w", "week", "weekly", "w", "k_week":
		return "1w", nil
	case "1mo", "month", "mth", "monthly", "k_month":
		return "1mo", nil
	default:
		return "", fmt.Errorf("unsupported period %q", period)
	}
}
