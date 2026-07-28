package indicatorwarmup

import (
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// RuntimeOptions controls market-session assumptions used by warmup estimates.
type RuntimeOptions struct {
	IncludeExtendedHours bool
}

func resolveIntervalMinutes(interval types.Interval) int {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	if value == "" {
		return 1
	}
	var unit string
	switch {
	case strings.HasSuffix(value, "mo"):
		unit = "mo"
		value = strings.TrimSuffix(value, "mo")
	case strings.HasSuffix(value, "min"):
		unit = "min"
		value = strings.TrimSuffix(value, "min")
	case strings.HasSuffix(value, "m"):
		unit = "m"
		value = strings.TrimSuffix(value, "m")
	case strings.HasSuffix(value, "h"):
		unit = "h"
		value = strings.TrimSuffix(value, "h")
	case strings.HasSuffix(value, "d"):
		unit = "d"
		value = strings.TrimSuffix(value, "d")
	case strings.HasSuffix(value, "w"):
		unit = "w"
		value = strings.TrimSuffix(value, "w")
	default:
		return 1
	}
	amount, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || amount <= 0 {
		return 1
	}
	switch unit {
	case "min", "m":
		return amount
	case "h":
		return amount * 60
	case "d":
		return amount * tradingSessionMinutesPerDay
	case "w":
		return amount * tradingSessionMinutesPerWeek
	case "mo":
		return amount * tradingSessionMinutesPerMonth
	default:
		return 1
	}
}
