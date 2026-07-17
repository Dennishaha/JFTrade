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
	result, err := s.preprocessNormalizedExpression(result, depth, stack)
	if err != nil {
		return result, err
	}
	result = applyExpressionNamespaceRewrites(result)
	result = applyExpressionFunctionRewrites(result)
	result = s.applyExpressionAliasRewrites(result)
	return finalizeNormalizedExpression(result), nil
}

func (s *parseState) preprocessNormalizedExpression(expression string, depth int, stack map[string]bool) (string, error) {
	result := lowerInputCalls(expression)
	result = s.resolveValueAliases(result)
	result = s.resolveSourceAliases(result)

	var err error
	result, err = s.lowerObjectMethodCalls(result)
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
	return replaceSupportedRequestSecurity(result), nil
}

func applyExpressionNamespaceRewrites(expression string) string {
	result := expression
	for _, replacement := range []struct{ old, new string }{
		{"strategy.position_avg_price", "position_avg_price"},
		{"strategy.position_size", "position_size"},
		{"strategy.equity", "equity"},
		{"syminfo.tickerid", "syminfo_tickerid"},
		{"syminfo.prefix", "syminfo_prefix"},
		{"timeframe.period", "timeframe_period"},
		{"timeframe.multiplier", "timeframe_multiplier"},
		{"timeframe.isintraday", "timeframe_isintraday"},
		{"timeframe.isminutes", "timeframe_isminutes"},
		{"timeframe.isseconds", "timeframe_isseconds"},
		{"timeframe.isdaily", "timeframe_isdaily"},
		{"timeframe.isweekly", "timeframe_isweekly"},
		{"timeframe.ismonthly", "timeframe_ismonthly"},
	} {
		result = strings.ReplaceAll(result, replacement.old, replacement.new)
	}
	result = replaceStringNamespace(result)
	result = replacePineNamespaceConstants(result)
	result = replaceColorFunctions(result)
	return result
}

func applyExpressionFunctionRewrites(expression string) string {
	result := applyMovingAverageRewrites(expression)
	result = applyCoreIndicatorRewrites(result)
	result = applyWindowIndicatorRewrites(result)
	result = applyStateIndicatorRewrites(result)
	result = applyCrossRewrites(result)
	return replaceMathNamespace(result)
}

func applyMovingAverageRewrites(expression string) string {
	result := expression
	for _, item := range []struct{ name, kind string }{
		{"ema", "EMA"},
		{"sma", "SMA"},
		{"rma", "SMMA"},
		{"wma", "LWMA"},
		{"hma", "HMA"},
		{"vwma", "VWMA"},
	} {
		result = replaceTAMovingAverageFunction(result, item.name, item.kind)
	}
	return result
}

func applyCoreIndicatorRewrites(expression string) string {
	result := expression
	result = replaceTASourceLengthFunction(result, "rsi", "rsi", "close", "14")
	result = replaceTAMacd(result)
	result = replaceTABollinger(result)
	result = replaceTAFunction(result, "atr", "atr(${period})")
	result = replaceTASourceLengthFunction(result, "stdev", "stdev", "close", "20")
	result = replaceTASourceLengthFunction(result, "variance", "variance", "close", "20")
	result = replaceTASourceLengthFunction(result, "cci", "cci", "hlc3", "20")
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
	result = taOBVPattern.ReplaceAllString(result, "obv(close)")
	return replaceTATr(result)
}

func applyWindowIndicatorRewrites(expression string) string {
	result := expression
	for _, name := range []string{"highest", "lowest", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum"} {
		result = replaceTAWindowFunction(result, name)
	}
	for _, name := range []string{"highestbars", "lowestbars"} {
		result = replaceTAExtremaBarsFunction(result, name)
	}
	return result
}

func applyStateIndicatorRewrites(expression string) string {
	result := expression
	for _, name := range []string{
		"linreg", "obv", "pivothigh", "pivotlow", "kc", "kcw", "alma", "bbw", "cog", "cmo",
		"tsi", "correlation", "dev", "median", "percentile_linear_interpolation",
		"percentile_nearest_rank", "percentrank", "swma", "barssince", "valuewhen",
	} {
		result = replaceTAStateFunction(result, name)
	}
	return result
}

func applyCrossRewrites(expression string) string {
	result := expression
	result = replaceTAFunction(result, "crossover", "cross_over(${left}, ${right})")
	result = replaceTAFunction(result, "crossunder", "cross_under(${left}, ${right})")
	return replaceTAFunction(result, "cross", "(cross_over(${left}, ${right}) || cross_under(${left}, ${right}))")
}

func (s *parseState) applyExpressionAliasRewrites(expression string) string {
	result := expression
	for alias, target := range s.expressionAliases {
		result = s.cachedRegexp("word:"+alias, `\b`+regexp.QuoteMeta(alias)+`\b`).ReplaceAllString(result, target)
	}
	return result
}

func finalizeNormalizedExpression(expression string) string {
	result := normalizeHistoryReferences(expression)
	result = normalizeTernaryExpression(result)
	result = strings.ReplaceAll(result, " and ", " && ")
	result = strings.ReplaceAll(result, " or ", " || ")
	return stripWrappingParens(strings.TrimSpace(result))
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
