package pine

import (
	"strings"
)

func (s *parseState) pineEntryQuantity(args []string) (string, string) {
	if quantityMode, quantityExpression, ok := pineExplicitQuantity(args); ok {
		return quantityMode, quantityExpression
	}
	mode := s.strategyMetadata.DefaultQtyMode
	if strings.TrimSpace(mode) == "" {
		mode = "fixed"
	}
	value := strings.TrimSpace(s.strategyMetadata.DefaultQtyValue)
	if value == "" {
		value = "1"
	}
	switch mode {
	case "percent_of_equity":
		return "account_position_percent", value
	case "cash":
		return "amount", value
	case "fixed":
		fallthrough
	default:
		return "shares", value
	}
}

func pineExplicitQuantity(args []string) (string, string, bool) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "qty_percent":
			return "account_position_percent", value, true
		case "qty":
			if percent, ok := pineEquityPercent(value); ok {
				return "account_position_percent", percent, true
			}
			if amount, ok := pineAmountQuantity(value); ok {
				return "amount", amount, true
			}
			return "shares", value, true
		}
	}
	if len(args) > 0 && !strings.Contains(args[0], "=") {
		value := args[0]
		if percent, ok := pineEquityPercent(value); ok {
			return "account_position_percent", percent, true
		}
		if amount, ok := pineAmountQuantity(value); ok {
			return "amount", amount, true
		}
		return "shares", value, true
	}
	return "", "", false
}

func pineAmountQuantity(value string) (string, bool) {
	normalized := stripWrappingParens(strings.TrimSpace(value))
	match := amountQuantityPattern.FindStringSubmatch(normalized)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

func pineCloseQuantity(args []string, entryID string) (string, string) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "qty_percent":
			return "symbol_position_percent", value
		case "qty":
			return "shares", value
		}
	}
	if len(args) > 0 && !strings.Contains(args[0], "=") {
		return "shares", args[0]
	}
	if len(args) > 1 && !strings.Contains(args[1], "=") {
		return "symbol_position_percent", args[1]
	}
	return "symbol_position_percent", "100"
}

func pineExitQuantity(args []string) (string, string) {
	return pineCloseQuantity(args, "")
}
