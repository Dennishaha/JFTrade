// Package indicatorbinding provides shared parsing and normalization
// helpers for DSL indicator bindings.  These functions were previously
// duplicated between pkg/strategy/ir (planner) and
// pkg/strategy/dslruntime (runtime); they are now the single source of
// truth for indicator configuration parsing - runtime calculation
// remains in its owning package.
package indicatorbinding

import (
	"fmt"
	"strconv"
	"strings"
)

// --- function-call parsing ---

// ParseFunctionCall splits a DSL expression like "ma(EMA,14,m)" into its
// function name and a list of trimmed string arguments.
func ParseFunctionCall(value string) (string, []string, bool) {
	trimmed := strings.TrimSpace(value)
	openIndex := strings.Index(trimmed, "(")
	closeIndex := strings.LastIndex(trimmed, ")")
	if openIndex <= 0 || closeIndex != len(trimmed)-1 || closeIndex <= openIndex {
		return "", nil, false
	}
	name := strings.TrimSpace(trimmed[:openIndex])
	args := SplitArguments(trimmed[openIndex+1 : closeIndex])
	return name, args, true
}

// SplitArguments splits a comma-separated argument list, respecting nested
// parentheses and single/double/backtick quotes.
func SplitArguments(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	depth := 0
	quote := rune(0)
	for index, char := range trimmed {
		switch {
		case quote != 0:
			if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"' || char == '`':
			quote = char
		case char == '(':
			depth++
		case char == ')':
			if depth > 0 {
				depth--
			}
		case char == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(trimmed[start:index]))
			start = index + 1
		}
	}
	parts = append(parts, strings.TrimSpace(trimmed[start:]))
	return parts
}

// --- name normalization ---

// NormalizeFunctionName lower-cases and trims a DSL function name.
func NormalizeFunctionName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// --- moving-average type ---

// ParseMovingAverageType normalizes a moving-average type string (e.g.
// "ema", "sma").  Returns the canonical upper-case form and true on
// success.
func ParseMovingAverageType(value string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "MA", "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL":
		return strings.ToUpper(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

// NormalizeMovingAverageType returns the canonical MA type or "MA" when
// the input is unrecognised.
func NormalizeMovingAverageType(value string) string {
	parsed, ok := ParseMovingAverageType(value)
	if !ok {
		return "MA"
	}
	return parsed
}

// --- indicator time-unit ---

// ParseIndicatorTimeUnitValue normalizes a time-unit string (e.g. "m",
// "hours").  Returns the canonical lower-case unit and true on success.
// An empty string means "bar" (chart period).
func ParseIndicatorTimeUnitValue(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "bar", "bars":
		return "", true
	case "m", "min", "mins", "minute", "minutes":
		return "minute", true
	case "h", "hr", "hrs", "hour", "hours":
		return "hour", true
	case "d", "day", "days":
		return "day", true
	case "w", "week", "weeks":
		return "week", true
	case "mo", "mon", "month", "months":
		return "month", true
	default:
		return "", false
	}
}

// NormalizeIndicatorTimeUnit returns the canonical time unit or "" when
// the input is unrecognised.
func NormalizeIndicatorTimeUnit(value string) string {
	parsed, _ := ParseIndicatorTimeUnitValue(value)
	return parsed
}

// BuildMovingAverageKey returns a stable key like "ma:EMA:14:minute"
// used for indicator requirement de-duplication.
func BuildMovingAverageKey(averageType string, period int, timeUnit string) string {
	base := "ma:" + averageType + ":" + strconv.Itoa(period)
	if timeUnit == "" {
		return base
	}
	return base + ":" + timeUnit
}

// --- quantity mode ---

// ParseQuantityMode normalizes an order-quantity-mode string.
func ParseQuantityMode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accountpositionpercent", "account_position_percent":
		return "account_position_percent", true
	case "symbolpositionpercent", "symbol_position_percent", "positionpercent", "position_percent":
		return "symbol_position_percent", true
	case "cashpercent", "cash_percent":
		return "cash_percent", true
	case "marginbuyingpowerpercent", "margin_buying_power_percent":
		return "margin_buying_power_percent", true
	case "shortsellingpowerpercent", "short_selling_power_percent":
		return "short_selling_power_percent", true
	case "amount":
		return "amount", true
	case "share", "shares":
		return "shares", true
	default:
		return "", false
	}
}

// NormalizeQuantityMode returns the canonical quantity mode or "shares".
func NormalizeQuantityMode(value string) string {
	parsed, ok := ParseQuantityMode(value)
	if !ok {
		return "shares"
	}
	return parsed
}

// --- protect / risk management ---

// ParseProtectMode normalizes a protect/risk mode string.
func ParseProtectMode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stoploss", "stop_loss":
		return "stopLoss", true
	case "takeprofit", "take_profit":
		return "takeProfit", true
	case "trailingstop", "trailing_stop":
		return "trailingStop", true
	default:
		return "", false
	}
}

// NormalizeProtectMode returns the canonical protect mode or "stopLoss".
func NormalizeProtectMode(value string) string {
	parsed, ok := ParseProtectMode(value)
	if !ok {
		return "stopLoss"
	}
	return parsed
}

// ParseProtectDirection normalizes a protect direction string.
func ParseProtectDirection(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "long":
		return "long", true
	case "short":
		return "short", true
	case "auto", "both":
		return "auto", true
	default:
		return "", false
	}
}

// NormalizeProtectDirection returns the canonical protect direction or "auto".
func NormalizeProtectDirection(value string) string {
	parsed, ok := ParseProtectDirection(value)
	if !ok {
		return "auto"
	}
	return parsed
}

// ParseProtectWindowPolicy normalizes a window-policy string.
func ParseProtectWindowPolicy(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "continuous") {
		return "continuous", true
	}
	if strings.EqualFold(trimmed, "session") {
		return "session", true
	}
	return "", false
}

// NormalizeProtectWindowPolicy returns the canonical window policy or "continuous".
func NormalizeProtectWindowPolicy(value string) string {
	parsed, ok := ParseProtectWindowPolicy(value)
	if !ok {
		return "continuous"
	}
	return parsed
}

// --- numeric parsers ---

// ParsePositiveInt parses a string as a strictly positive integer.
func ParsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("positive integer is required")
	}
	return parsed, nil
}

// ParsePositiveFloat parses a string as a strictly positive float64.
func ParsePositiveFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("positive float is required")
	}
	return parsed, nil
}

// ParsePercentage strips a trailing '%' and parses the remaining value as
// a strictly positive float64.
func ParsePercentage(value string) (float64, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(value, "%"))
	return ParsePositiveFloat(trimmed)
}

// --- arg-count helpers ---

// ExpectOnePositiveIntArg validates that args contains exactly one positive
// integer and returns it.
func ExpectOnePositiveIntArg(line int, name string, args []string) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("dsl line %d: %s() requires exactly 1 positive integer argument", line, NormalizeFunctionName(name))
	}
	value, err := ParsePositiveInt(args[0])
	if err != nil {
		return 0, fmt.Errorf("dsl line %d: %s() argument must be a positive integer", line, NormalizeFunctionName(name))
	}
	return value, nil
}

// ExpectPositiveIntArgs validates that args contains exactly count positive
// integers and returns them.
func ExpectPositiveIntArgs(line int, name string, args []string, count int) ([]int, error) {
	if len(args) != count {
		return nil, fmt.Errorf("dsl line %d: %s() requires %d positive integer arguments", line, NormalizeFunctionName(name), count)
	}
	values := make([]int, 0, count)
	for _, arg := range args {
		value, err := ParsePositiveInt(arg)
		if err != nil {
			return nil, fmt.Errorf("dsl line %d: %s() arguments must be positive integers", line, NormalizeFunctionName(name))
		}
		values = append(values, value)
	}
	return values, nil
}

// IntsToStrings converts a slice of ints to a slice of their decimal
// string representations.
func IntsToStrings(values []int) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.Itoa(value))
	}
	return result
}
