package pine

import (
	"fmt"
	"strconv"
	"strings"

	strategyexpression "github.com/jftrade/jftrade-main/pkg/strategy/expression"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func rejectUnsupported(line parsedLine) error {
	if diagnostic, ok := unsupportedSyntaxDiagnostic(line); ok {
		return fmt.Errorf("pine line %d: %s", diagnostic.Line, diagnostic.Message)
	}
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.Contains(lower, "request.security("):
		if strings.Contains(strings.ToLower(replaceSupportedRequestSecurity(line.trimmed)), "request.security(") {
			if requestSecurityUsesTimeframeAlias(line.trimmed) {
				return nil
			}
			switch {
			case strings.Contains(lower, "barmerge.lookahead_on"):
				return fmt.Errorf("pine line %d: request.security() lookahead_on is not supported by JFTrade; use default lookahead_off", line.number)
			case strings.Contains(lower, "barmerge.gaps_on"):
				return fmt.Errorf("pine line %d: request.security() gaps_on is not supported by JFTrade; use default gaps_off", line.number)
			default:
				return fmt.Errorf("pine line %d: request.security() is supported only for syminfo.tickerid with OHLCV/hl2/hlc3/ohlc4 sources, source history, source-aware moving averages, supported static intraday advanced indicators, v1.4 pure expressions, or v1.5 common TA pure expressions without side effects", line.number)
			}
		}
	case strings.HasPrefix(lower, "runtime.error("):
		return fmt.Errorf("pine line %d: %s", line.number, firstStringArgument(line.trimmed))
	case strings.HasPrefix(lower, "import "):
		return fmt.Errorf("pine line %d: Pine libraries/imports are not supported by JFTrade yet", line.number)
	case strings.Contains(lower, "array."), strings.Contains(lower, "matrix."), strings.Contains(lower, "map."):
		return fmt.Errorf("pine line %d: Pine collection namespaces array/matrix/map are not supported by JFTrade yet", line.number)
	default:
		return nil
	}
	return nil
}

func unsupportedSyntaxDiagnostic(line parsedLine) (Diagnostic, bool) {
	lower := strings.ToLower(strings.TrimSpace(line.trimmed))
	switch {
	case unsupportedTAFunctionName(lower) != "":
		name := unsupportedTAFunctionName(lower)
		return diagnosticForLine(DiagnosticSeverityError, "PINE_TA_FUNCTION_UNSUPPORTED", fmt.Sprintf("ta.%s() is not supported by JFTrade yet", name), line), true
	case strings.HasPrefix(lower, "while "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_WHILE_UNSUPPORTED", "while loops are parsed but not executable in this JFTrade Pine v6 version", line), true
	case strings.HasPrefix(lower, "type "), strings.HasPrefix(lower, "method "), strings.HasPrefix(lower, "import "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_DECLARATION_UNSUPPORTED", "Pine declarations, libraries, and methods are not executable in this JFTrade Pine v6 version", line), true
	case strings.Contains(lower, "array."), strings.Contains(lower, "matrix."), strings.Contains(lower, "map."):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_COLLECTION_UNSUPPORTED", "Pine collection namespaces array/matrix/map are not executable in this JFTrade Pine v6 version", line), true
	case historyDiagnosticMessage(line.trimmed) != "":
		return diagnosticForLine(DiagnosticSeverityError, "PINE_HISTORY_REF_UNSUPPORTED", historyDiagnosticMessage(line.trimmed), line), true
	default:
		return Diagnostic{}, false
	}
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

func parseStrategyTitle(line string) string {
	match := strategyTitlePattern.FindStringSubmatch(line)
	if match == nil {
		return ""
	}
	return unquote(strings.TrimSpace(match[1]))
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
