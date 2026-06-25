package pine

import (
	"fmt"
	"regexp"
	"strings"
)

func (s *parseState) normalizeExpression(expression string) string {
	result, err := s.normalizeExpressionDepth(expression, 0, map[string]bool{})
	if err != nil && s.normalizationErr == nil {
		s.normalizationErr = err
	}
	return result
}

func (s *parseState) normalizeExpressionDepth(expression string, depth int, stack map[string]bool) (string, error) {
	result := strings.TrimSpace(expression)
	result = lowerInputCalls(result)
	result = s.resolveValueAliases(result)
	result = s.resolveSourceAliases(result)
	result, err := s.lowerObjectMethodCalls(result)
	if err != nil {
		return result, err
	}
	result, err = s.lowerCollectionHistoryReadCalls(result)
	if err != nil {
		return result, err
	}
	result, err = s.lowerCollectionReadCalls(result)
	if err != nil {
		return result, err
	}
	if unsupportedCallHistoryReference(result) {
		return result, fmt.Errorf("history references are supported only on identifiers or object fields; assign the function result first")
	}
	result, err = s.expandUDFCalls(result, depth, stack)
	if err != nil {
		return result, err
	}
	result = replaceSupportedRequestSecurity(result)
	result = strings.ReplaceAll(result, "strategy.position_avg_price", "position_avg_price")
	result = strings.ReplaceAll(result, "strategy.position_size", "position_size")
	result = strings.ReplaceAll(result, "strategy.equity", "equity")
	result = strings.ReplaceAll(result, "syminfo.tickerid", "syminfo_tickerid")
	result = strings.ReplaceAll(result, "syminfo.prefix", "syminfo_prefix")
	result = strings.ReplaceAll(result, "timeframe.period", "timeframe_period")
	result = strings.ReplaceAll(result, "timeframe.multiplier", "timeframe_multiplier")
	result = strings.ReplaceAll(result, "timeframe.isintraday", "timeframe_isintraday")
	result = strings.ReplaceAll(result, "timeframe.isminutes", "timeframe_isminutes")
	result = strings.ReplaceAll(result, "timeframe.isseconds", "timeframe_isseconds")
	result = strings.ReplaceAll(result, "timeframe.isdaily", "timeframe_isdaily")
	result = strings.ReplaceAll(result, "timeframe.isweekly", "timeframe_isweekly")
	result = strings.ReplaceAll(result, "timeframe.ismonthly", "timeframe_ismonthly")
	result = replaceStringNamespace(result)
	result = replacePineNamespaceConstants(result)
	result = replaceColorFunctions(result)
	result = replaceTAMovingAverageFunction(result, "ema", "EMA")
	result = replaceTAMovingAverageFunction(result, "sma", "SMA")
	result = replaceTAMovingAverageFunction(result, "rma", "SMMA")
	result = replaceTAMovingAverageFunction(result, "wma", "LWMA")
	result = replaceTAMovingAverageFunction(result, "hma", "HMA")
	result = replaceTAMovingAverageFunction(result, "vwma", "VWMA")
	result = replaceTASourceLengthFunction(result, "rsi", "rsi", "close", "14")
	result = replaceTAMacd(result)
	result = replaceTABollinger(result)
	result = replaceTAFunction(result, "atr", "atr(${period})")
	result = replaceTASourceLengthFunction(result, "stdev", "stdev", "close", "20")
	result = replaceTASourceLengthFunction(result, "variance", "variance", "close", "20")
	result = replaceTASourceLengthFunction(result, "cci", "cci", "hlc3", "20")
	result = replaceTAWindowFunction(result, "highest")
	result = replaceTAWindowFunction(result, "lowest")
	result = replaceTAWindowFunction(result, "change")
	result = replaceTAWindowFunction(result, "mom")
	result = replaceTAWindowFunction(result, "roc")
	result = replaceTAWindowFunction(result, "range")
	result = replaceTAWindowFunction(result, "mode")
	result = replaceTAWindowFunction(result, "rising")
	result = replaceTAWindowFunction(result, "falling")
	result = replaceTAWindowFunction(result, "sum")
	result = replaceTAExtremaBarsFunction(result, "highestbars")
	result = replaceTAExtremaBarsFunction(result, "lowestbars")
	result = replaceTASourceRequiredFunction(result, "cum", "cum")
	result = replaceTAFunction(result, "wpr", "williams_r(${period})")
	result = replaceTAAnchoredVWAP(result)
	result = replaceTimeframeNamespace(result)
	result = replaceTASourceOptionalFunction(result, "vwap", "vwap", "hlc3")
	result = replaceTASourceLengthFunction(result, "mfi", "mfi", "hlc3", "14")
	result = replaceTAStoch(result)
	result = replaceTAFunction(result, "dmi", "dmi(${left}, ${right})")
	result = replaceTAFunction(result, "supertrend", "supertrend(${left}, ${right})")
	result = replaceTAFunction(result, "sar", "sar(${left}, ${right}, ${third})")
	result = replaceTAStateFunction(result, "linreg")
	result = replaceTAStateFunction(result, "obv")
	result = replaceTAStateFunction(result, "pivothigh")
	result = replaceTAStateFunction(result, "pivotlow")
	result = replaceTAStateFunction(result, "kc")
	result = replaceTAStateFunction(result, "kcw")
	result = replaceTAStateFunction(result, "alma")
	result = replaceTAStateFunction(result, "bbw")
	result = replaceTAStateFunction(result, "cog")
	result = replaceTAStateFunction(result, "cmo")
	result = replaceTAStateFunction(result, "tsi")
	result = replaceTAStateFunction(result, "correlation")
	result = replaceTAStateFunction(result, "dev")
	result = replaceTAStateFunction(result, "median")
	result = replaceTAStateFunction(result, "percentile_linear_interpolation")
	result = replaceTAStateFunction(result, "percentile_nearest_rank")
	result = replaceTAStateFunction(result, "percentrank")
	result = replaceTAStateFunction(result, "swma")
	result = taOBVPattern.ReplaceAllString(result, "obv(close)")
	result = replaceTATr(result)
	result = replaceTAStateFunction(result, "barssince")
	result = replaceTAStateFunction(result, "valuewhen")
	result = replaceTAFunction(result, "crossover", "cross_over(${left}, ${right})")
	result = replaceTAFunction(result, "crossunder", "cross_under(${left}, ${right})")
	result = replaceTAFunction(result, "cross", "(cross_over(${left}, ${right}) || cross_under(${left}, ${right}))")
	result = replaceMathNamespace(result)
	for alias, target := range s.expressionAliases {
		result = s.cachedRegexp("word:"+alias, `\b`+regexp.QuoteMeta(alias)+`\b`).ReplaceAllString(result, target)
	}
	result = normalizeHistoryReferences(result)
	result = normalizeTernaryExpression(result)
	result = strings.ReplaceAll(result, " and ", " && ")
	result = strings.ReplaceAll(result, " or ", " || ")
	return stripWrappingParens(strings.TrimSpace(result)), nil
}

func (s *parseState) expandUDFCalls(expression string, depth int, stack map[string]bool) (string, error) {
	if s == nil || len(s.udfs) == 0 {
		return expression, nil
	}
	if depth > maxUDFExpansionDepth {
		return expression, fmt.Errorf("user-defined function expansion exceeded depth %d", maxUDFExpansionDepth)
	}
	result := expression
	for {
		changed := false
		for name, udf := range s.udfs {
			start := s.findUDFCallStart(result, name)
			if start < 0 {
				continue
			}
			open := start + len(name)
			for open < len(result) && (result[open] == ' ' || result[open] == '\t') {
				open++
			}
			close := matchingParen(result, open)
			if close < 0 {
				return result, fmt.Errorf("invalid call to user-defined function %q", name)
			}
			args := splitArguments(result[open+1 : close])
			if len(args) == 1 && strings.TrimSpace(args[0]) == "" {
				args = nil
			}
			if len(args) != len(udf.Args) {
				return result, fmt.Errorf("user-defined function %q expects %d arguments, got %d", name, len(udf.Args), len(args))
			}
			if stack[name] {
				return result, fmt.Errorf("recursive user-defined function %q is not supported by JFTrade yet", name)
			}
			stack[name] = true
			replacements := make(map[string]string, len(args))
			for index, arg := range args {
				normalizedArg, err := s.normalizeExpressionDepth(strings.TrimSpace(arg), depth+1, stack)
				if err != nil {
					delete(stack, name)
					return result, err
				}
				replacements[udf.Args[index]] = udfArgumentReplacement(normalizedArg)
			}
			body := udf.Body
			for _, argName := range udf.Args {
				body = s.cachedRegexp("word:"+argName, `\b`+regexp.QuoteMeta(argName)+`\b`).ReplaceAllString(body, replacements[argName])
			}
			expanded, err := s.expandUDFCalls(body, depth+1, stack)
			delete(stack, name)
			if err != nil {
				return result, err
			}
			result = result[:start] + "(" + expanded + ")" + result[close+1:]
			changed = true
			break
		}
		if !changed {
			return result, nil
		}
	}
}

func (s *parseState) findUDFCallStart(expression string, name string) int {
	pattern := s.cachedRegexp("call:"+name, `\b`+regexp.QuoteMeta(name)+`\s*\(`)
	matches := pattern.FindAllStringIndex(expression, -1)
	for _, match := range matches {
		if match[0] > 0 && expression[match[0]-1] == '.' {
			continue
		}
		return match[0]
	}
	return -1
}

func udfArgumentReplacement(value string) string {
	trimmed := strings.TrimSpace(value)
	if memberPattern.MatchString(trimmed) ||
		numberPattern.MatchString(trimmed) ||
		trimmed == "true" || trimmed == "false" || trimmed == "na" {
		return trimmed
	}
	return "(" + trimmed + ")"
}
