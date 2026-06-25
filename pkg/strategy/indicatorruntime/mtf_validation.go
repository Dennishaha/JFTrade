package indicatorruntime

import (
	"fmt"
	"strings"
)

func validateFixedTimeframeRequirements(requirements indicatorRequirements, intervalMinutes int) error {
	for _, config := range requirements.ma {
		if err := validateFixedTimeframe("moving average", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.securitySource {
		if err := validateFixedTimeframe("security source", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.rsiSource {
		if err := validateFixedTimeframe("rsi", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.stdevSource {
		if err := validateFixedTimeframe("stdev", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.variance {
		if err := validateFixedTimeframe("variance", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.stoch {
		if err := validateFixedTimeframe("stoch", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.cciSource {
		if err := validateFixedTimeframe("cci", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.mfi {
		if err := validateFixedTimeframe("mfi", config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	for _, config := range requirements.advanced {
		if err := validateFixedTimeframe(config.kind, config.timeUnit, intervalMinutes); err != nil {
			return err
		}
	}
	return nil
}

func validateFixedTimeframe(context string, timeUnit string, intervalMinutes int) error {
	normalized := normalizeIndicatorTimeUnit(timeUnit)
	if normalized == "" {
		return nil
	}
	targetMinutes, ok := comparableTimeUnitMinutes(normalized)
	if !ok || targetMinutes <= 0 {
		return fmt.Errorf("indicator %s fixed timeframe %q is not supported", context, strings.TrimSpace(timeUnit))
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	if targetMinutes < intervalMinutes {
		return fmt.Errorf(
			"indicator %s fixed timeframe %s is lower than strategy interval %s; JFTrade supports request.security() only at the current or a higher timeframe",
			context,
			formatIndicatorTimeUnit(normalized),
			formatIntervalMinutes(intervalMinutes),
		)
	}
	if targetMinutes < tradingSessionMinutesPerDay {
		if targetMinutes%intervalMinutes != 0 {
			return fmt.Errorf(
				"indicator %s fixed timeframe %s is not aligned with strategy interval %s; JFTrade aggregates MTF data from a single native interval",
				context,
				formatIndicatorTimeUnit(normalized),
				formatIntervalMinutes(intervalMinutes),
			)
		}
	}
	return nil
}

func comparableTimeUnitMinutes(timeUnit string) (int, bool) {
	if minutes, ok := indicatorTimeUnitMinutes(timeUnit); ok {
		return minutes, true
	}
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "day":
		return tradingSessionMinutesPerDay, true
	case "week":
		return tradingSessionMinutesPerWeek, true
	case "month":
		return tradingSessionMinutesPerMonth, true
	default:
		return 0, false
	}
}

func formatIndicatorTimeUnit(timeUnit string) string {
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "minute":
		return "1m"
	case "hour":
		return "60m"
	case "day":
		return "D"
	case "week":
		return "W"
	case "month":
		return "M"
	default:
		return normalizeIndicatorTimeUnit(timeUnit)
	}
}

func formatIntervalMinutes(minutes int) string {
	switch minutes {
	case tradingSessionMinutesPerDay:
		return "D"
	case tradingSessionMinutesPerWeek:
		return "W"
	case tradingSessionMinutesPerMonth:
		return "M"
	}
	if minutes <= 0 {
		return "1m"
	}
	return fmt.Sprintf("%dm", minutes)
}
