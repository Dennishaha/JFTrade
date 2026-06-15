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

func pineBarCloseTime(kline *types.KLine, interval types.Interval) (time.Time, bool) {
	if kline == nil {
		return time.Time{}, false
	}
	if !kline.EndTime.Time().IsZero() {
		return kline.EndTime.Time(), true
	}
	start := pineBarTime(kline)
	if start.IsZero() {
		return time.Time{}, false
	}
	if duration, ok := pineIntervalDuration(interval); ok {
		return start.Add(duration), true
	}
	return time.Time{}, false
}

func (s *evaluationScope) runtimeInterval() types.Interval {
	if s == nil {
		return ""
	}
	if s.runtime != nil && s.runtime.interval != "" {
		return s.runtime.interval
	}
	if s.currentKline != nil {
		return s.currentKline.Interval
	}
	return ""
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
	case strings.HasSuffix(value, "s"):
		return "second"
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
	case "second", "minute", "hour":
		return true
	default:
		return false
	}
}

func pineTimeframeMultiplier(interval types.Interval) int {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	if value == "" {
		return 1
	}
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 1
	}
	result := 0
	for _, char := range value[:end] {
		result = result*10 + int(char-'0')
	}
	if result <= 0 {
		return 1
	}
	return result
}

func pineIntervalDuration(interval types.Interval) (time.Duration, bool) {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	if value == "" {
		return 0, false
	}
	unit := value[len(value)-1:]
	numberText := value[:len(value)-1]
	multiplier := 1
	if numberText != "" {
		parsed := 0
		for _, char := range numberText {
			if char < '0' || char > '9' {
				return 0, false
			}
			parsed = parsed*10 + int(char-'0')
		}
		if parsed > 0 {
			multiplier = parsed
		}
	}
	switch unit {
	case "s":
		return time.Duration(multiplier) * time.Second, true
	case "m":
		return time.Duration(multiplier) * time.Minute, true
	case "h":
		return time.Duration(multiplier) * time.Hour, true
	case "d":
		return time.Duration(multiplier) * 24 * time.Hour, true
	case "w":
		return time.Duration(multiplier) * 7 * 24 * time.Hour, true
	default:
		return 0, false
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
