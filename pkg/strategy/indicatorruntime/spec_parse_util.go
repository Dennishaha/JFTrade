package indicatorruntime

import (
	"log"
	"math"
	"strconv"
	"strings"
)

func parseOptionalAdvancedTimeUnit(parts []string, index int) (string, bool) {
	if len(parts) == index {
		return "", true
	}
	if len(parts) != index+1 {
		return "", false
	}
	timeUnit := normalizeIndicatorTimeUnit(parts[index])
	return timeUnit, timeUnit != ""
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed > 0
}

func parseNonNegativeInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed >= 0
}

func parseOHLCVSource(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open":
		return "open", true
	case "high":
		return "high", true
	case "low":
		return "low", true
	case "close":
		return "close", true
	case "volume":
		return "volume", true
	case "hl2":
		return "hl2", true
	case "hlc3":
		return "hlc3", true
	case "ohlc4":
		return "ohlc4", true
	default:
		return "", false
	}
}

func normalizeWindowFunction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func parseMovingAverageConfig(parts []string) (movingAverageConfig, bool) {
	if len(parts) == 2 {
		period, ok := parsePositiveInt(parts[1])
		if !ok {
			return movingAverageConfig{}, false
		}
		return movingAverageConfig{averageType: "MA", period: period}, true
	}
	if len(parts) == 3 {
		if period, ok := parsePositiveInt(parts[1]); ok {
			source, sourceOK := parseOHLCVSource(parts[2])
			if sourceOK {
				return movingAverageConfig{
					averageType: "MA",
					period:      period,
					source:      source,
				}, true
			}
			timeUnit, timeUnitOK := parseIndicatorTimeUnit(parts[2])
			if !timeUnitOK {
				return movingAverageConfig{}, false
			}
			return movingAverageConfig{
				averageType: "MA",
				period:      period,
				timeUnit:    timeUnit,
			}, true
		}
		period, ok := parsePositiveInt(parts[2])
		if !ok {
			return movingAverageConfig{}, false
		}
		return movingAverageConfig{
			averageType: normalizeMovingAverageType(parts[1]),
			period:      period,
		}, true
	}
	if len(parts) == 4 {
		period, ok := parsePositiveInt(parts[2])
		if !ok {
			return movingAverageConfig{}, false
		}
		source, sourceOK := parseOHLCVSource(parts[3])
		if sourceOK {
			return movingAverageConfig{
				averageType: normalizeMovingAverageType(parts[1]),
				period:      period,
				source:      source,
			}, true
		}
		timeUnit, timeUnitOK := parseIndicatorTimeUnit(parts[3])
		if !timeUnitOK {
			return movingAverageConfig{}, false
		}
		return movingAverageConfig{
			averageType: normalizeMovingAverageType(parts[1]),
			period:      period,
			timeUnit:    timeUnit,
		}, true
	}
	if len(parts) != 5 {
		return movingAverageConfig{}, false
	}
	period, ok := parsePositiveInt(parts[2])
	if !ok {
		return movingAverageConfig{}, false
	}
	source, sourceOK := parseOHLCVSource(parts[4])
	if !sourceOK {
		return movingAverageConfig{}, false
	}
	timeUnit, timeUnitOK := parseIndicatorTimeUnit(parts[3])
	if !timeUnitOK {
		return movingAverageConfig{}, false
	}
	return movingAverageConfig{
		averageType: normalizeMovingAverageType(parts[1]),
		period:      period,
		timeUnit:    timeUnit,
		source:      source,
	}, true
}

func normalizeSourceOrClose(value string) string {
	source, ok := parseOHLCVSource(value)
	if !ok || source == "" {
		return "close"
	}
	return source
}

func parseStopLossConfig(parts []string) (stopLossConfig, bool) {
	switch firstNonEmpty(parts[0]) {
	case "sl":
		if len(parts) != 5 {
			return stopLossConfig{}, false
		}
		timeValue, ok := parsePositiveInt(parts[2])
		if !ok {
			return stopLossConfig{}, false
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if err != nil || percentage <= 0 {
			return stopLossConfig{}, false
		}
		timeUnit, timeUnitOK := parseStopLossTimeUnit(parts[3])
		if !timeUnitOK {
			return stopLossConfig{}, false
		}
		return stopLossConfig{
			mode:         "stopLoss",
			direction:    normalizeStopLossDirection(parts[1]),
			timeValue:    timeValue,
			timeUnit:     timeUnit,
			percentage:   percentage,
			windowPolicy: "continuous",
		}, true
	case "risk":
		if len(parts) != 7 {
			return stopLossConfig{}, false
		}
		mode, ok := parseStopLossMode(parts[1])
		if !ok {
			return stopLossConfig{}, false
		}
		timeValue, ok := parsePositiveInt(parts[3])
		if !ok {
			return stopLossConfig{}, false
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)
		if err != nil || percentage <= 0 {
			return stopLossConfig{}, false
		}
		windowPolicy, ok := parseStopLossWindowPolicy(parts[6])
		if !ok {
			return stopLossConfig{}, false
		}
		timeUnit, timeUnitOK := parseStopLossTimeUnit(parts[4])
		if !timeUnitOK {
			return stopLossConfig{}, false
		}
		return stopLossConfig{
			mode:         mode,
			direction:    normalizeStopLossDirection(parts[2]),
			timeValue:    timeValue,
			timeUnit:     timeUnit,
			percentage:   percentage,
			windowPolicy: windowPolicy,
		}, true
	default:
		return stopLossConfig{}, false
	}
}

func parseStopLossTimeUnit(value string) (string, bool) {
	normalized := normalizeIndicatorTimeUnit(value)
	if normalized != "" {
		if _, ok := indicatorTimeUnitMinutes(normalized); ok {
			return normalized, true
		}
		switch normalized {
		case "day", "week", "month":
			return normalized, true
		default:
			return "", false
		}
	}
	trimmed := strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(trimmed); err == nil {
		trimmed = unquoted
	}
	switch strings.ToLower(strings.TrimSpace(trimmed)) {
	case "", "bar", "bars":
		return "", true
	default:
		return "", false
	}
}

func resolveBarCount(period int, timeUnit string, intervalMinutes int) int {
	if period <= 0 {
		return 0
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	if minutes, ok := indicatorTimeUnitMinutes(timeUnit); ok {
		return max(1, int(math.Ceil(float64(period*minutes)/float64(intervalMinutes))))
	}
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "":
		return period
	case "day":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerDay)/float64(intervalMinutes))))
	case "week":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerWeek)/float64(intervalMinutes))))
	case "month":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerMonth)/float64(intervalMinutes))))
	default:
		return period
	}
}

func indicatorTimeUnitMinutes(timeUnit string) (int, bool) {
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "minute":
		return 1, true
	case "hour":
		return 60, true
	default:
		normalized := normalizeIndicatorTimeUnit(timeUnit)
		if before, ok := strings.CutSuffix(normalized, "m"); ok {
			minutes, err := strconv.Atoi(before)
			if err == nil && minutes > 0 {
				return minutes, true
			}
		}
		return 0, false
	}
}

func normalizeMovingAverageType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "MA"
	}
}

func parseIndicatorTimeUnit(value string) (string, bool) {
	normalized := normalizeIndicatorTimeUnit(value)
	if normalized == "" {
		return "", false
	}
	if _, ok := indicatorTimeUnitMinutes(normalized); ok {
		return normalized, true
	}
	switch normalized {
	case "day", "week", "month":
		return normalized, true
	default:
		return "", false
	}
}

func normalizeIndicatorTimeUnit(value string) string {
	trimmed := strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(trimmed); err == nil {
		trimmed = unquoted
	}
	normalized := strings.ToLower(strings.TrimSpace(trimmed))
	switch normalized {
	case "", "bar", "bars":
		return ""
	case "m", "min", "mins", "minute", "minutes":
		return "minute"
	case "h", "hr", "hrs", "hour", "hours":
		return "hour"
	case "d", "day", "days":
		return "day"
	case "w", "week", "weeks":
		return "week"
	case "mo", "mon", "month", "months":
		return "month"
	default:
		if before, ok := strings.CutSuffix(normalized, "m"); ok {
			minutes, err := strconv.Atoi(before)
			if err == nil && minutes > 0 {
				switch minutes {
				case 1:
					return "minute"
				case 60:
					return "hour"
				default:
					return strconv.Itoa(minutes) + "m"
				}
			}
		}
		return ""
	}
}

func normalizeStopLossDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "long":
		return "long"
	case "short":
		return "short"
	default:
		return "auto"
	}
}

func normalizeStopLossMode(value string) string {
	switch strings.TrimSpace(value) {
	case "takeProfit":
		return "takeProfit"
	case "trailingStop":
		return "trailingStop"
	default:
		return "stopLoss"
	}
}

func parseStopLossMode(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "stopLoss", "takeProfit", "trailingStop":
		return strings.TrimSpace(value), true
	default:
		return "", false
	}
}

func normalizeStopLossWindowPolicy(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "session") {
		return "session"
	}
	return "continuous"
}

func parseStopLossWindowPolicy(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "continuous", "session":
		return strings.TrimSpace(value), true
	default:
		return "", false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
