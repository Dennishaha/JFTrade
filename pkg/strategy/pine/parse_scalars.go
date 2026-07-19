package pine

import (
	"strconv"
	"strings"
)

func parseNonNegativeIntConstant(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(stripWrappingParens(value)))
	return parsed, err == nil && parsed >= 0
}

func parseBoolConstant(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(stripWrappingParens(value))) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func normalizeStrategyCommissionType(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "strategy.commission.")
	switch normalized {
	case "percent", "cash_per_order", "cash_per_contract":
		return normalized, true
	default:
		return "", false
	}
}

func normalizeStrategyDefaultQtyMode(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "strategy.")
	switch normalized {
	case "", "fixed":
		return "fixed", true
	case "cash":
		return "cash", true
	case "percent_of_equity":
		return "percent_of_equity", true
	default:
		return "", false
	}
}

func parseStrategyPyramiding(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(stripWrappingParens(value)))
	if err != nil || parsed < 0 {
		return 1, false
	}
	if parsed == 0 {
		return 1, true
	}
	return parsed, true
}
