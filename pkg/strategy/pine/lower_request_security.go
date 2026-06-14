package pine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func replaceSupportedRequestSecurity(expression string) string {
	prefix := "request.security("
	for {
		start := strings.Index(strings.ToLower(expression), prefix)
		if start < 0 {
			return expression
		}
		open := start + len(prefix) - 1
		close := matchingParen(expression, open)
		if close < 0 {
			return expression
		}
		args := splitArguments(expression[open+1 : close])
		replacement, ok := lowerSupportedRequestSecurity(args)
		if !ok {
			return expression
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func lowerSupportedRequestSecurity(args []string) (string, bool) {
	if len(args) < 3 || strings.TrimSpace(args[0]) != "syminfo.tickerid" {
		return "", false
	}
	timeUnit, ok := pineTimeframeUnit(unquote(strings.TrimSpace(args[1])))
	if !ok {
		return "", false
	}
	if !supportedRequestSecurityMergeArgs(args[3:]) {
		return "", false
	}
	inner := strings.TrimSpace(args[2])
	if strings.Contains(strings.ToLower(inner), "request.security(") {
		return "", false
	}
	if source, lookback, ok := supportedRequestSecuritySourceHistory(inner); ok {
		if lookback > 0 {
			return fmt.Sprintf("security_source(%s, %s, %d)", source, timeUnit, lookback), true
		}
		return fmt.Sprintf("security_source(%s, %s)", source, timeUnit), true
	}
	name, innerArgs, ok := parseTACall(inner)
	if !ok && strings.EqualFold(inner, "ta.obv") {
		name, innerArgs, ok = "obv", []string{"close"}, true
	}
	if !ok {
		if replacement, ok := lowerPureRequestSecurityExpression(inner, timeUnit); ok {
			return replacement, true
		}
		return "", false
	}
	if replacement, ok := lowerAdvancedRequestSecurity(name, innerArgs, timeUnit); ok {
		return replacement, true
	}
	if len(innerArgs) < 2 {
		if replacement, ok := lowerPureRequestSecurityExpression(inner, timeUnit); ok {
			return replacement, true
		}
		return "", false
	}
	source, ok := supportedRequestSecuritySource(strings.TrimSpace(innerArgs[0]))
	if !ok {
		if replacement, ok := lowerPureRequestSecurityExpression(inner, timeUnit); ok {
			return replacement, true
		}
		return "", false
	}
	maType, ok := pineMovingAverageType(name)
	if !ok {
		if replacement, ok := lowerPureRequestSecurityExpression(inner, timeUnit); ok {
			return replacement, true
		}
		return "", false
	}
	if source == "close" {
		return fmt.Sprintf("ma(%s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit), true
	}
	return fmt.Sprintf("ma(%s, %s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit, source), true
}

func lowerPureRequestSecurityExpression(expression string, timeUnit string) (string, bool) {
	if !requestSecurityExpressionIsPure(expression) {
		return "", false
	}
	result := strings.TrimSpace(expression)
	if strings.HasPrefix(result, "[") || strings.Contains(result, "=>") {
		return "", false
	}
	placeholders := make([]string, 0)
	addPlaceholder := func(value string) string {
		token := fmt.Sprintf("__pine_mtf_placeholder_%d__", len(placeholders))
		placeholders = append(placeholders, value)
		return token
	}
	var ok bool
	result, ok = maskPureRequestSecurityTACalls(result, timeUnit, addPlaceholder)
	if !ok {
		return "", false
	}
	result, ok = maskPureRequestSecuritySourceHistory(result, timeUnit, addPlaceholder)
	if !ok {
		return "", false
	}
	result = replacePureRequestSecuritySources(result, timeUnit)
	for index, value := range placeholders {
		result = strings.ReplaceAll(result, fmt.Sprintf("__pine_mtf_placeholder_%d__", index), value)
	}
	if strings.Contains(strings.ToLower(result), "ta.") || strings.Contains(strings.ToLower(result), "request.security(") {
		return "", false
	}
	return "(" + result + ")", true
}

func requestSecurityExpressionIsPure(expression string) bool {
	lower := strings.ToLower(strings.TrimSpace(expression))
	for _, denied := range []string{
		"strategy.", "alert(", "alertcondition(", "log.", "runtime.error(",
		"array.", "matrix.", "map.", "line.", "label.", "box.", "table.",
		"plot(", "plotshape(", "plotchar(", "hline(", "fill(", "bgcolor(", "barcolor(",
		":=", "request.security(",
	} {
		if strings.Contains(lower, denied) {
			return false
		}
	}
	return true
}

func maskPureRequestSecurityTACalls(expression string, timeUnit string, addPlaceholder func(string) string) (string, bool) {
	result := expression
	for {
		start := strings.Index(strings.ToLower(result), "ta.")
		if start < 0 {
			return result, true
		}
		open := strings.Index(result[start:], "(")
		if open < 0 {
			if strings.HasPrefix(strings.ToLower(result[start:]), "ta.obv") {
				replacement, ok := lowerRequestSecurityTACall("obv", []string{"close"}, timeUnit)
				if !ok {
					return "", false
				}
				end := start + len("ta.obv")
				result = result[:start] + addPlaceholder(replacement) + result[end:]
				continue
			}
			return "", false
		}
		open += start
		close := matchingParen(result, open)
		if close < 0 {
			return "", false
		}
		name := strings.ToLower(strings.TrimSpace(result[start+len("ta.") : open]))
		args := splitArguments(result[open+1 : close])
		replacement, ok := lowerRequestSecurityTACall(name, args, timeUnit)
		if !ok {
			return "", false
		}
		result = result[:start] + addPlaceholder(replacement) + result[close+1:]
	}
}

func lowerRequestSecurityTACall(name string, args []string, timeUnit string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if maType, ok := pineMovingAverageType(name); ok {
		source, period := pineSourceLengthArgs(args, "close", "20")
		if _, ok := supportedRequestSecuritySource(source); !ok {
			return "", false
		}
		if source == "close" {
			return fmt.Sprintf("ma(%s, %s, %s)", maType, period, timeUnit), true
		}
		return fmt.Sprintf("ma(%s, %s, %s, %s)", maType, period, timeUnit, source), true
	}
	switch name {
	case "rsi":
		source, period := pineSourceLengthArgs(args, "close", "14")
		if _, ok := supportedRequestSecuritySource(source); !ok {
			return "", false
		}
		return fmt.Sprintf("rsi(%s, %s, %s)", source, period, timeUnit), true
	case "macd":
		if len(args) != 4 {
			return "", false
		}
		source, ok := supportedRequestSecuritySource(strings.TrimSpace(args[0]))
		if !ok {
			return "", false
		}
		return fmt.Sprintf("macd(%s, %s, %s, %s, %s)", strings.TrimSpace(args[1]), strings.TrimSpace(args[2]), strings.TrimSpace(args[3]), timeUnit, source), true
	case "atr":
		period := "14"
		if len(args) == 1 {
			period = strings.TrimSpace(args[0])
		} else if len(args) != 0 {
			return "", false
		}
		return fmt.Sprintf("atr(%s, %s)", period, timeUnit), true
	case "bb":
		if len(args) != 3 {
			return "", false
		}
		source, ok := supportedRequestSecuritySource(strings.TrimSpace(args[0]))
		if !ok {
			return "", false
		}
		return fmt.Sprintf("bollinger(%s, %s, %s, %s)", strings.TrimSpace(args[1]), strings.TrimSpace(args[2]), timeUnit, source), true
	case "supertrend":
		if len(args) != 2 {
			return "", false
		}
		return fmt.Sprintf("supertrend(%s, %s, %s)", strings.TrimSpace(args[0]), strings.TrimSpace(args[1]), timeUnit), true
	case "linreg", "obv", "pivothigh", "pivotlow", "kc", "kcw", "alma",
		"cmo", "tsi", "correlation", "dev", "median", "percentile_linear_interpolation",
		"percentile_nearest_rank", "percentrank", "swma":
		return lowerAdvancedRequestSecurity(name, args, timeUnit)
	default:
		return "", false
	}
}

func maskPureRequestSecuritySourceHistory(expression string, timeUnit string, addPlaceholder func(string) string) (string, bool) {
	ok := true
	result := rewriteOutsideStringLiterals(expression, func(segment string) string {
		return historyReferencePattern.ReplaceAllStringFunc(segment, func(match string) string {
			parts := historyReferencePattern.FindStringSubmatch(match)
			if len(parts) != 3 {
				return match
			}
			source, sourceOK := supportedRequestSecuritySource(parts[1])
			lookback, err := strconv.Atoi(strings.TrimSpace(parts[2]))
			if !sourceOK || err != nil || lookback < 0 || lookback > maxHistoryLookback {
				ok = false
				return match
			}
			return addPlaceholder(fmt.Sprintf("security_source(%s, %s, %d)", source, timeUnit, lookback))
		})
	})
	return result, ok
}

func replacePureRequestSecuritySources(expression string, timeUnit string) string {
	return rewriteOutsideStringLiterals(expression, func(segment string) string {
		for _, source := range []string{"ohlc4", "hlc3", "hl2", "volume", "open", "high", "low", "close"} {
			pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(source) + `\b`)
			segment = pattern.ReplaceAllString(segment, fmt.Sprintf("security_source(%s, %s)", strings.ToLower(source), timeUnit))
		}
		return segment
	})
}

func lowerAdvancedRequestSecurity(name string, args []string, timeUnit string) (string, bool) {
	if !supportedAdvancedRequestSecurityTimeUnit(timeUnit) {
		return "", false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "linreg":
		if len(args) != 3 {
			return "", false
		}
	case "obv":
		if len(args) == 0 {
			args = []string{"close"}
		}
		if len(args) != 1 {
			return "", false
		}
	case "pivothigh", "pivotlow":
		if len(args) == 2 {
			source := "high"
			if name == "pivotlow" {
				source = "low"
			}
			args = append([]string{source}, args...)
		}
		if len(args) != 3 {
			return "", false
		}
	case "kc", "kcw":
		if len(args) == 3 {
			args = append(args, "true")
		}
		if len(args) != 4 {
			return "", false
		}
	case "alma":
		if len(args) != 4 {
			return "", false
		}
	case "cmo", "dev", "median", "percentrank":
		if len(args) != 2 {
			return "", false
		}
	case "tsi":
		if len(args) != 3 {
			return "", false
		}
	case "correlation":
		if len(args) != 3 {
			return "", false
		}
		if _, ok := supportedRequestSecuritySource(strings.TrimSpace(args[1])); !ok {
			return "", false
		}
	case "percentile_linear_interpolation", "percentile_nearest_rank":
		if len(args) != 3 {
			return "", false
		}
	case "swma":
		if len(args) != 1 {
			return "", false
		}
	default:
		return "", false
	}
	if _, ok := supportedRequestSecuritySource(strings.TrimSpace(args[0])); !ok {
		return "", false
	}
	return fmt.Sprintf("%s(%s, %s)", name, strings.Join(args, ", "), timeUnit), true
}

func supportedAdvancedRequestSecurityTimeUnit(timeUnit string) bool {
	normalized := strings.ToLower(unquote(strings.TrimSpace(timeUnit)))
	return normalized == "minute" || normalized == "hour" || strings.HasSuffix(normalized, "m")
}

func supportedRequestSecurityMergeArgs(args []string) bool {
	for index, arg := range args {
		key, value, named := splitNamedArg(arg)
		if !named {
			switch index {
			case 0:
				key = "gaps"
				value = arg
			case 1:
				key = "lookahead"
				value = arg
			default:
				return false
			}
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "gaps":
			if !strings.EqualFold(strings.TrimSpace(value), "barmerge.gaps_off") {
				return false
			}
		case "lookahead":
			if !strings.EqualFold(strings.TrimSpace(value), "barmerge.lookahead_off") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func supportedRequestSecuritySourceHistory(expression string) (string, int, bool) {
	if source, ok := supportedRequestSecuritySource(expression); ok {
		return source, 0, true
	}
	matches := historyReferencePattern.FindStringSubmatch(strings.TrimSpace(expression))
	if len(matches) != 3 || strings.TrimSpace(matches[0]) != strings.TrimSpace(expression) {
		return "", 0, false
	}
	source, ok := supportedRequestSecuritySource(matches[1])
	if !ok {
		return "", 0, false
	}
	lookback, err := strconv.Atoi(strings.TrimSpace(matches[2]))
	if err != nil || lookback < 0 || lookback > maxHistoryLookback {
		return "", 0, false
	}
	return source, lookback, true
}

func supportedRequestSecuritySource(expression string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(expression)) {
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

func requestSecurityUsesTimeframeAlias(expression string) bool {
	prefix := "request.security("
	start := strings.Index(strings.ToLower(expression), prefix)
	if start < 0 {
		return false
	}
	open := start + len(prefix) - 1
	close := matchingParen(expression, open)
	if close < 0 {
		return false
	}
	args := splitArguments(expression[open+1 : close])
	if len(args) < 2 {
		return false
	}
	timeframe := strings.TrimSpace(args[1])
	return identifierPattern.MatchString(timeframe)
}
