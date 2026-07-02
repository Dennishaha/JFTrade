package pine

import (
	"fmt"
	"strconv"
	"strings"

	strategyexpression "github.com/jftrade/jftrade-main/pkg/strategy/expression"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) rejectUnsupported(line parsedLine) error {
	if diagnostic, ok := unsupportedSyntaxDiagnostic(line); ok {
		return fmt.Errorf("pine line %d: %s", diagnostic.Line, diagnostic.Message)
	}
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.Contains(lower, "request.security("):
		if _, _, ok := supportedRequestSecurityTupleAssignmentLine(line.trimmed); ok {
			return nil
		}
		lowered := replaceSupportedRequestSecurity(line.trimmed)
		if match := assignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			lowered = s.normalizeExpression(strings.TrimSpace(match[4]))
			if err := s.takeNormalizationErr(line.number); err != nil {
				return err
			}
		}
		if strings.Contains(strings.ToLower(lowered), "request.security(") {
			if requestSecurityUsesTimeframeAlias(line.trimmed) {
				return nil
			}
			switch {
			case strings.Contains(lower, "barmerge.lookahead_on"):
				return fmt.Errorf("pine line %d: request.security() lookahead_on is not supported by JFTrade; use default lookahead_off", line.number)
			case strings.Contains(lower, "barmerge.gaps_on"):
				return fmt.Errorf("pine line %d: request.security() gaps_on is not supported by JFTrade; use default gaps_off", line.number)
			default:
				return fmt.Errorf("pine line %d: request.security() is supported only for syminfo.tickerid with OHLCV/hl2/hlc3/ohlc4 sources, source history, source-aware moving averages, supported static intraday advanced indicators, pure expressions, tuple whitelists, and v2.3 pure collection/object expressions without side effects", line.number)
			}
		}
	case strings.HasPrefix(lower, "runtime.error("):
		return fmt.Errorf("pine line %d: %s", line.number, firstStringArgument(line.trimmed))
	case strings.HasPrefix(lower, "import "):
		return fmt.Errorf("pine line %d: Pine libraries/imports are not supported by JFTrade yet", line.number)
	case (strings.Contains(lower, "array.") || strings.Contains(lower, "matrix.") || strings.Contains(lower, "map.")) && !supportedExecutableCollectionLine(line.trimmed):
		return fmt.Errorf("pine line %d: Pine collection namespaces array/matrix/map are not supported by JFTrade yet", line.number)
	default:
		return nil
	}
	return nil
}

func supportedRequestSecurityTupleAssignmentLine(line string) ([]string, []string, bool) {
	if match := generalTuplePattern.FindStringSubmatch(line); match != nil {
		aliases := splitArguments(match[1])
		if args, ok := requestSecurityCallArgs(strings.TrimSpace(match[3])); ok {
			lowered, loweredOK := lowerSupportedRequestSecurityTupleGeneral(args)
			if loweredOK && len(aliases) == len(lowered) {
				return aliases, lowered, true
			}
		}
	}
	match := tupleAssignmentPattern.FindStringSubmatch(line)
	if match == nil {
		return nil, nil, false
	}
	args, ok := requestSecurityCallArgs(strings.TrimSpace(match[5]))
	if !ok {
		return nil, nil, false
	}
	lowered, ok := lowerSupportedRequestSecurityTuple(args)
	if !ok {
		return nil, nil, false
	}
	aliases := tupleAssignmentAliases(match)
	return aliases, lowered, len(aliases) == len(lowered)
}

func unsupportedSyntaxDiagnostic(line parsedLine) (Diagnostic, bool) {
	if diagnostic, ok := publicDisabledHelperDiagnostic(line); ok {
		return diagnostic, true
	}
	lower := strings.ToLower(strings.TrimSpace(line.trimmed))
	switch {
	case strings.Contains(lower, "request.security("):
		if diagnostic, ok := requestSecurityUnsupportedDiagnostic(line); ok {
			return diagnostic, true
		}
		return Diagnostic{}, false
	case unsupportedTAFunctionName(lower) != "":
		name := unsupportedTAFunctionName(lower)
		return diagnosticForLine(DiagnosticSeverityError, "PINE_TA_FUNCTION_UNSUPPORTED", fmt.Sprintf("ta.%s() is not supported by JFTrade yet", name), line), true
	case strings.HasPrefix(lower, "import "), strings.HasPrefix(lower, "library("):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_DECLARATION_UNSUPPORTED", "Pine declarations, libraries, and methods are not executable in this JFTrade Pine v6 version", line), true
	case (strings.Contains(lower, "array.") || strings.Contains(lower, "matrix.") || strings.Contains(lower, "map.") || typedAssignmentPattern.MatchString(line.trimmed)) && !supportedExecutableCollectionLine(line.trimmed):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_COLLECTION_UNSUPPORTED", "Pine collection namespaces array/matrix/map are not executable in this JFTrade Pine v6 version", line), true
	case historyDiagnosticMessage(line.trimmed) != "":
		return diagnosticForLine(DiagnosticSeverityError, "PINE_HISTORY_REF_UNSUPPORTED", historyDiagnosticMessage(line.trimmed), line), true
	default:
		return Diagnostic{}, false
	}
}

func requestSecurityUnsupportedDiagnostic(line parsedLine) (Diagnostic, bool) {
	trimmed := strings.TrimSpace(line.trimmed)
	if _, _, ok := supportedRequestSecurityTupleAssignmentLine(trimmed); ok {
		return Diagnostic{}, false
	}
	if !strings.Contains(strings.ToLower(replaceSupportedRequestSecurity(trimmed)), "request.security(") {
		return Diagnostic{}, false
	}
	args, ok := requestSecurityArgsFromLine(trimmed)
	if !ok {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_UNSUPPORTED", "request.security() call could not be parsed", line), true
	}
	if len(args) < 3 {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_UNSUPPORTED", "request.security() requires symbol, timeframe, and expression arguments", line), true
	}
	for _, mergeArg := range args[3:] {
		lowerMerge := strings.ToLower(strings.TrimSpace(mergeArg))
		switch {
		case strings.Contains(lowerMerge, "barmerge.lookahead_on"):
			return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_LOOKAHEAD", "request.security() lookahead_on is not supported by JFTrade; use default lookahead_off", line), true
		case strings.Contains(lowerMerge, "barmerge.gaps_on"):
			return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_GAPS", "request.security() gaps_on is not supported by JFTrade; use default gaps_off", line), true
		case strings.HasPrefix(lowerMerge, "calc_bars_count="):
			return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_CALC_BARS_COUNT", "request.security() calc_bars_count is not supported by JFTrade", line), true
		}
	}
	if strings.TrimSpace(args[0]) != "syminfo.tickerid" {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL", "request.security() currently supports only syminfo.tickerid; dynamic or external symbols are not supported", line), true
	}
	if requestSecurityUsesTimeframeAlias(trimmed) {
		return Diagnostic{}, false
	}
	if _, ok := pineTimeframeUnit(unquote(strings.TrimSpace(args[1]))); !ok {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_DYNAMIC_TIMEFRAME", "request.security() currently supports only static timeframe strings", line), true
	}
	expression := strings.TrimSpace(args[2])
	if strings.Contains(strings.ToLower(expression), "request.security(") {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_NESTED", "nested request.security() calls are not supported by JFTrade", line), true
	}
	if requestSecurityExpressionHasSideEffect(expression) {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_SIDE_EFFECT", "request.security() expression must be pure; strategy, alert, visual, collection mutation, and reassignment side effects are not supported", line), true
	}
	if diagnostic, ok := requestSecurityTupleDiagnostic(line, expression); ok {
		return diagnostic, true
	}
	if requestSecurityExpressionHasUnsupportedTACall(expression) {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED", "request.security() expression is outside JFTrade's executable pure-expression subset", line), true
	}
	return Diagnostic{}, false
}

func requestSecurityTupleDiagnostic(line parsedLine, expression string) (Diagnostic, bool) {
	trimmed := strings.TrimSpace(expression)
	if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return Diagnostic{}, false
	}
	values := splitArguments(trimmed[1 : len(trimmed)-1])
	if len(values) < 2 || len(values) > 8 {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED", "request.security() tuple expressions support 2 to 8 values", line), true
	}
	aliases, ok := requestSecurityTupleAliasesFromLine(line.trimmed)
	if !ok {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_TUPLE_ASSIGNMENT", "request.security() tuple expressions must be assigned with matching tuple aliases", line), true
	}
	if len(aliases) != len(values) {
		return diagnosticForLine(DiagnosticSeverityError, "PINE_REQUEST_SECURITY_TUPLE_MISMATCH", fmt.Sprintf("request.security() tuple returns %d values but assignment has %d aliases", len(values), len(aliases)), line), true
	}
	return Diagnostic{}, false
}

func requestSecurityTupleAliasesFromLine(line string) ([]string, bool) {
	if match := generalTuplePattern.FindStringSubmatch(line); match != nil {
		rawNames := splitArguments(match[1])
		aliases := make([]string, 0, len(rawNames))
		for _, raw := range rawNames {
			aliases = append(aliases, strings.TrimSpace(raw))
		}
		return aliases, true
	}
	if match := tupleAssignmentPattern.FindStringSubmatch(line); match != nil {
		return tupleAssignmentAliases(match), true
	}
	return nil, false
}

func requestSecurityExpressionHasUnsupportedTACall(expression string) bool {
	lower := strings.ToLower(expression)
	for search := 0; search < len(lower); {
		index := strings.Index(lower[search:], "ta.")
		if index < 0 {
			return false
		}
		index += search
		open := strings.Index(lower[index:], "(")
		if open < 0 {
			return false
		}
		open += index
		name := strings.TrimSpace(lower[index+len("ta.") : open])
		close := matchingParen(expression, open)
		if close < 0 {
			return true
		}
		args := splitArguments(expression[open+1 : close])
		timeUnit := "minute"
		if replacement, ok := lowerRequestSecurityTACall(name, args, timeUnit); ok && replacement != "" {
			search = close + 1
			continue
		}
		return true
	}
	return false
}

func requestSecurityArgsFromLine(line string) ([]string, bool) {
	if match := assignmentPattern.FindStringSubmatch(line); match != nil {
		return requestSecurityCallArgs(strings.TrimSpace(match[4]))
	}
	if match := generalTuplePattern.FindStringSubmatch(line); match != nil {
		return requestSecurityCallArgs(strings.TrimSpace(match[3]))
	}
	if match := tupleAssignmentPattern.FindStringSubmatch(line); match != nil {
		return requestSecurityCallArgs(strings.TrimSpace(match[5]))
	}
	start := strings.Index(strings.ToLower(line), "request.security(")
	if start < 0 {
		return nil, false
	}
	open := start + len("request.security")
	close := matchingParen(line, open)
	if close < 0 {
		return nil, false
	}
	return splitArguments(line[open+1 : close]), true
}

func requestSecurityExpressionHasSideEffect(expression string) bool {
	lower := strings.ToLower(strings.TrimSpace(expression))
	if strings.Contains(lower, ":=") {
		return true
	}
	for _, denied := range []string{
		"strategy.", "alert(", "alertcondition(", "log.", "runtime.error(",
		"line.new(", "label.new(", "box.new(", "table.",
		"plot(", "plotshape(", "plotchar(", "hline(", "fill(", "bgcolor(", "barcolor(",
	} {
		if strings.Contains(lower, denied) {
			return true
		}
	}
	for _, mutator := range []string{
		".push(", ".pop(", ".shift(", ".unshift(", ".insert(", ".remove(", ".clear(", ".set(", ".fill(", ".put(",
		"array.push(", "array.pop(", "array.shift(", "array.unshift(", "array.insert(", "array.remove(", "array.clear(", "array.set(", "array.fill(",
		"map.put(", "map.remove(", "map.clear(",
		"matrix.set(", "matrix.fill(",
	} {
		if strings.Contains(lower, mutator) {
			return true
		}
	}
	return false
}

func unsupportedTAFunctionName(lower string) string {
	return ""
}

func historyDiagnosticMessage(expression string) string {
	if unsupportedCallHistoryReference(expression) {
		return "history references are supported only on identifiers or object fields; assign the function result first"
	}
	for _, match := range historyReferenceMatchesOutsideStringLiterals(expression) {
		if len(match) != 3 {
			continue
		}
		lookback, err := strconv.Atoi(strings.TrimSpace(match[2]))
		if err != nil || lookback < 0 {
			return "history reference lookback must be a non-negative integer"
		}
		if lookback > maxHistoryLookback {
			return fmt.Sprintf("history reference lookback %d exceeds JFTrade maximum %d", lookback, maxHistoryLookback)
		}
	}
	return ""
}

func unsupportedCallHistoryReference(expression string) bool {
	hasCallHistory := false
	rewriteOutsideStringLiterals(expression, func(segment string) string {
		if callHistoryPattern.MatchString(segment) {
			hasCallHistory = true
		}
		return segment
	})
	return hasCallHistory
}

func validateExpression(lineNumber int, label string, expression string) error {
	if _, err := strategyexpression.ParseExpression(expression); err != nil {
		return fmt.Errorf("pine line %d: invalid %s %q: %w", lineNumber, label, strings.TrimSpace(expression), err)
	}
	return nil
}

func parseStrategyDeclaration(line string) (strategyir.StrategyMetadata, []string) {
	metadata := strategyir.StrategyMetadata{
		Name:                  "Pine Strategy",
		DefaultQtyMode:        "fixed",
		DefaultQtyValue:       "1",
		Pyramiding:            1,
		AllowedEntryDirection: "all",
	}
	warnings := []string{}
	args := splitArguments(callArgs(line))
	if len(args) > 0 {
		if key, value, ok := splitNamedArg(args[0]); ok {
			if strings.EqualFold(key, "title") {
				if title := unquote(strings.TrimSpace(value)); title != "" {
					metadata.Name = title
				}
			}
		} else if title := unquote(strings.TrimSpace(args[0])); title != "" {
			metadata.Name = title
		}
	}
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "title":
			if title := unquote(strings.TrimSpace(value)); title != "" {
				metadata.Name = title
			}
		case "default_qty_type":
			if mode, ok := normalizeStrategyDefaultQtyMode(value); ok {
				metadata.DefaultQtyMode = mode
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy default_qty_type %q is not supported by JFTrade; using strategy.fixed", strings.TrimSpace(value)))
			}
		case "default_qty_value":
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				metadata.DefaultQtyValue = trimmed
			}
		case "pyramiding":
			if parsed, ok := parseStrategyPyramiding(value); ok {
				metadata.Pyramiding = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy pyramiding %q is not a supported constant integer; using 1", strings.TrimSpace(value)))
			}
		case "initial_capital":
			if parsed, ok := parsePositiveFloatConstant(value); ok {
				metadata.InitialCapital = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy initial_capital %q must be a positive constant number", strings.TrimSpace(value)))
			}
		case "commission_type":
			if parsed, ok := normalizeStrategyCommissionType(value); ok {
				metadata.CommissionType = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy commission_type %q is not supported by JFTrade", strings.TrimSpace(value)))
			}
		case "commission_value":
			if parsed, ok := parseNonNegativeFloatConstant(value); ok {
				metadata.CommissionValue = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy commission_value %q must be a non-negative constant number", strings.TrimSpace(value)))
			}
		case "slippage":
			if parsed, ok := parseNonNegativeIntConstant(value); ok {
				metadata.Slippage = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy slippage %q must be a non-negative constant integer", strings.TrimSpace(value)))
			}
		case "process_orders_on_close":
			if parsed, ok := parseBoolConstant(value); ok {
				metadata.ProcessOnClose = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy process_orders_on_close %q must be true or false", strings.TrimSpace(value)))
			}
		}
	}
	return metadata, warnings
}

func parsePositiveFloatConstant(value string) (float64, bool) {
	parsed, ok := parseNonNegativeFloatConstant(value)
	return parsed, ok && parsed > 0
}

func parseNonNegativeFloatConstant(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(stripWrappingParens(value)), 64)
	return parsed, err == nil && parsed >= 0
}

func isVisualOnlyCall(lower string) bool {
	return strings.HasPrefix(lower, "plot(") ||
		strings.HasPrefix(lower, "plotshape(") ||
		strings.HasPrefix(lower, "plotchar(") ||
		strings.HasPrefix(lower, "hline(") ||
		strings.HasPrefix(lower, "bgcolor(") ||
		strings.HasPrefix(lower, "barcolor(") ||
		strings.HasPrefix(lower, "fill(") ||
		strings.HasPrefix(lower, "alertcondition(") ||
		strings.HasPrefix(lower, "label.new(") ||
		strings.HasPrefix(lower, "line.new(") ||
		strings.HasPrefix(lower, "box.new(") ||
		strings.HasPrefix(lower, "table.")
}

func callName(line string) string {
	if index := strings.Index(line, "("); index > 0 {
		return strings.TrimSpace(line[:index])
	}
	return line
}
