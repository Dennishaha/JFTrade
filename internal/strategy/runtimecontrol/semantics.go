package runtimecontrol

import (
	"regexp"
	"strings"
)

type LiveExecutionLimitation struct {
	Code    string
	Message string
}

var (
	livePercentQuantityPattern = regexp.MustCompile(`(?i)\b(?:qty_percent|quantity_pct|quantityPct|QuantityPct|has_quantity_pct)\b`)
	liveDefaultEquityPattern   = regexp.MustCompile(`(?is)\bdefault_qty_type\s*=\s*strategy\.percent_of_equity\b`)
	liveCancelPattern          = regexp.MustCompile(`(?i)\bstrategy\.cancel(?:_all)?\s*\(`)
)

func LiveExecutionLimitations(script string) []LiveExecutionLimitation {
	executable := pineExecutableText(script)
	limitations := make([]LiveExecutionLimitation, 0, 2)
	if livePercentQuantityPattern.MatchString(executable) || liveDefaultEquityPattern.MatchString(executable) {
		limitations = append(limitations, LiveExecutionLimitation{
			Code:    "LIVE_PERCENT_QUANTITY_UNSUPPORTED",
			Message: "live execution does not support qty_percent or percent_of_equity quantities",
		})
	}
	if liveCancelPattern.MatchString(executable) {
		limitations = append(limitations, LiveExecutionLimitation{
			Code:    "LIVE_CANCEL_UNSUPPORTED",
			Message: "live execution does not support strategy.cancel or strategy.cancel_all",
		})
	}
	return limitations
}

func pineExecutableText(script string) string {
	var result strings.Builder
	inString := rune(0)
	escaped := false
	inLineComment := false
	runes := []rune(script)
	for index, current := range runes {
		if inLineComment {
			if current == '\n' {
				inLineComment = false
				result.WriteRune(current)
			}
			continue
		}
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == inString {
				inString = 0
			}
			continue
		}
		if current == '"' || current == '\'' {
			inString = current
			result.WriteRune(' ')
			continue
		}
		if current == '/' && index+1 < len(runes) && runes[index+1] == '/' {
			inLineComment = true
			result.WriteRune(' ')
			continue
		}
		result.WriteRune(current)
	}
	return result.String()
}
