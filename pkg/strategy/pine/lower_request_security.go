package pine

import (
	"fmt"
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
	if !ok || len(innerArgs) < 2 {
		return "", false
	}
	source, ok := supportedRequestSecuritySource(strings.TrimSpace(innerArgs[0]))
	if !ok {
		return "", false
	}
	maType, ok := pineMovingAverageType(name)
	if !ok {
		return "", false
	}
	if source == "close" {
		return fmt.Sprintf("ma(%s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit), true
	}
	return fmt.Sprintf("ma(%s, %s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit, source), true
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
