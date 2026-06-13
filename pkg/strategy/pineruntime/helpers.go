package pineruntime

import (
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func pineBarTime(kline *types.KLine) time.Time {
	if kline == nil {
		return time.Time{}
	}
	if !kline.StartTime.Time().IsZero() {
		return kline.StartTime.Time()
	}
	return kline.EndTime.Time()
}

func pineDayOfWeek(value time.Time) int {
	return int(value.Weekday()) + 1
}

func pineSymbolPrefix(symbol string) string {
	trimmed := strings.TrimSpace(symbol)
	if trimmed == "" {
		return ""
	}
	if index := strings.Index(trimmed, ":"); index > 0 {
		return trimmed[:index]
	}
	if index := strings.Index(trimmed, "."); index > 0 {
		return trimmed[:index]
	}
	return ""
}

func pineTimeframeUnit(interval types.Interval) string {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	switch {
	case strings.HasSuffix(value, "mo"), strings.HasSuffix(value, "mon"), strings.HasSuffix(value, "month"):
		return "month"
	case strings.HasSuffix(value, "w"):
		return "week"
	case strings.HasSuffix(value, "d"):
		return "day"
	case strings.HasSuffix(value, "h"):
		return "hour"
	case strings.HasSuffix(value, "m"):
		return "minute"
	default:
		return ""
	}
}

func pineTimeframeIsMinutes(interval types.Interval) bool {
	return pineTimeframeUnit(interval) == "minute"
}

func pineTimeframeIsIntraday(interval types.Interval) bool {
	switch pineTimeframeUnit(interval) {
	case "minute", "hour":
		return true
	default:
		return false
	}
}

func (r *strategyRuntime) cachedDivergenceRequirementKey(binding indicatorBinding, direction string, lookback int) (string, bool) {
	cacheKey := divergenceRequirementCacheKey{bindingKey: binding.Key, direction: direction, lookback: lookback}
	if r != nil && r.divergenceCache != nil {
		if cached, hit := r.divergenceCache[cacheKey]; hit {
			return cached, true
		}
	}
	key, ok := buildDivergenceRequirementKey(binding, direction, lookback)
	if !ok || r == nil || r.divergenceCache == nil {
		return key, ok
	}
	r.divergenceCache[cacheKey] = key
	return key, true
}
