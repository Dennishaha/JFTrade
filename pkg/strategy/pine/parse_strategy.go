package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) parseStrategyCall(line parsedLine) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.HasPrefix(lower, "strategy.risk.allow_entry_in("):
		return nil, true, s.parseStrategyAllowedEntryRisk(line)
	case strings.HasPrefix(lower, "strategy.risk.max_drawdown("):
		return nil, true, s.parseStrategyMaxDrawdownRisk(line)
	case strings.HasPrefix(lower, "strategy.risk.max_intraday_loss("):
		return nil, true, s.parseStrategyMaxIntradayLossRisk(line)
	case strings.HasPrefix(lower, "strategy.risk.max_intraday_filled_orders("):
		return nil, true, s.parseStrategyMaxIntradayFilledOrdersRisk(line)
	case strings.HasPrefix(lower, "strategy.risk.max_position_size("):
		return nil, true, s.parseStrategyMaxPositionSizeRisk(line)
	case strings.HasPrefix(lower, "strategy.risk.max_cons_loss_days("):
		return nil, true, s.parseStrategyMaxConsLossDaysRisk(line)
	case strings.HasPrefix(lower, "strategy.entry("):
		statement, err := s.parseStrategyEntryCall(line)
		return statement, true, err
	case strings.HasPrefix(lower, "strategy.order("):
		statement, err := s.parseStrategyOrderCall(line)
		return statement, true, err
	case strings.HasPrefix(lower, "strategy.close_all("):
		statement, err := s.parseStrategyCloseAllCall(line)
		return statement, true, err
	case strings.HasPrefix(lower, "strategy.close("):
		statement, err := s.parseStrategyCloseCall(line)
		return statement, true, err
	case strings.HasPrefix(lower, "strategy.exit("):
		statement, err := s.parseStrategyExit(line)
		if err != nil {
			return nil, true, err
		}
		return statement, true, nil
	case strings.HasPrefix(lower, "strategy.cancel_all("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) > 0 {
			return nil, true, fmt.Errorf("pine line %d: strategy.cancel_all arguments are not supported by JFTrade yet", line.number)
		}
		return &strategyir.CancelStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, All: true}, true, nil
	case strings.HasPrefix(lower, "strategy.cancel("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) != 1 {
			return nil, true, fmt.Errorf("pine line %d: strategy.cancel(id) requires one order id", line.number)
		}
		return &strategyir.CancelStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, ID: unquote(strings.TrimSpace(args[0]))}, true, nil
	default:
		return nil, false, nil
	}
}

func normalizeStrategyAllowedEntryDirection(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "strategy.direction.")
	normalized = strings.TrimPrefix(normalized, "strategy.")
	switch normalized {
	case "", "all":
		return "all", true
	case "long":
		return "long", true
	case "short":
		return "short", true
	default:
		return "", false
	}
}

func normalizeStrategyRiskAmountType(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "strategy.")
	switch normalized {
	case "percent_of_equity", "cash":
		return normalized, true
	default:
		return "", false
	}
}

func parseStrategyRiskAmountArgs(lineNumber int, fn string, args []string) (float64, string, string, error) {
	if len(args) < 2 {
		return 0, "", "", fmt.Errorf("pine line %d: %s(value, type[, alert_message]) requires at least two arguments", lineNumber, fn)
	}
	value, ok := parsePositiveFloatConstant(args[0])
	if !ok {
		return 0, "", "", fmt.Errorf("pine line %d: %s value %q must be a positive constant number", lineNumber, fn, strings.TrimSpace(args[0]))
	}
	amountType, ok := normalizeStrategyRiskAmountType(args[1])
	if !ok {
		return 0, "", "", fmt.Errorf("pine line %d: %s type %q is not supported", lineNumber, fn, strings.TrimSpace(args[1]))
	}
	alertMessage := ""
	remaining := args[2:]
	if named, ok := namedArgValue(remaining, "alert_message"); ok {
		alertMessage = unquote(strings.TrimSpace(named))
	} else if len(remaining) > 0 && !strings.Contains(remaining[0], "=") {
		alertMessage = unquote(strings.TrimSpace(remaining[0]))
		remaining = remaining[1:]
	}
	for _, arg := range remaining {
		key, _, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(key, "alert_message") {
			continue
		}
		return 0, "", "", fmt.Errorf("pine line %d: %s supports only value, type, and optional alert_message", lineNumber, fn)
	}
	return value, amountType, alertMessage, nil
}

func parseStrategyRiskCountArgs(lineNumber int, fn string, args []string) (int, string, error) {
	if len(args) == 0 {
		return 0, "", fmt.Errorf("pine line %d: %s(count[, alert_message]) requires at least one argument", lineNumber, fn)
	}
	count, ok := parseNonNegativeIntConstant(args[0])
	if !ok || count <= 0 {
		return 0, "", fmt.Errorf("pine line %d: %s count %q must be a positive constant integer", lineNumber, fn, strings.TrimSpace(args[0]))
	}
	alertMessage := ""
	remaining := args[1:]
	if named, ok := namedArgValue(remaining, "alert_message"); ok {
		alertMessage = unquote(strings.TrimSpace(named))
	} else if len(remaining) > 0 && !strings.Contains(remaining[0], "=") {
		alertMessage = unquote(strings.TrimSpace(remaining[0]))
		remaining = remaining[1:]
	}
	for _, arg := range remaining {
		key, _, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(key, "alert_message") {
			continue
		}
		return 0, "", fmt.Errorf("pine line %d: %s supports only count and optional alert_message", lineNumber, fn)
	}
	return count, alertMessage, nil
}

func parseStrategyRiskPositionSize(lineNumber int, args []string) (float64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("pine line %d: strategy.risk.max_position_size(contracts) requires one argument", lineNumber)
	}
	contracts, ok := parsePositiveFloatConstant(args[0])
	if !ok {
		return 0, fmt.Errorf("pine line %d: strategy.risk.max_position_size contracts %q must be a positive constant number", lineNumber, strings.TrimSpace(args[0]))
	}
	return contracts, nil
}

func (s *parseState) parseStrategyExit(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) < 1 {
		return nil, fmt.Errorf("pine line %d: strategy.exit(id, ...) requires an exit id", line.number)
	}
	exitID := unquote(strings.TrimSpace(args[0]))
	orderArgs := args[1:]
	fromEntry, orderArgs := parseStrategyExitFromEntry(orderArgs)
	if err := rejectUnsupportedOrderArgs(line.number, "strategy.exit", orderArgs); err != nil {
		return nil, err
	}
	hasTrail, err := validateStrategyExitTriggers(line.number, orderArgs)
	if err != nil {
		return nil, err
	}
	direction := strategyExitDirection(fromEntry)
	if err := rejectUnsupportedNamedArgs(line.number, "strategy.exit", orderArgs, "from_entry", "qty", "qty_percent", "profit", "limit", "loss", "stop", "trail_price", "trail_points", "trail_offset", "oca_name", "oca_type", "comment", "comment_profit", "comment_loss", "comment_trailing", "alert_message", "alert_profit", "alert_loss", "alert_trailing", "disable_alert", "when"); err != nil {
		return nil, err
	}
	if err := rejectConflictingQuantityArgs(line.number, "strategy.exit", orderArgs); err != nil {
		return nil, err
	}
	quantityMode, quantityExpr := pineExitQuantity(orderArgs)
	quantityExpr, err = s.normalizeRequiredExpression(line.number, "exit quantity expression", quantityExpr)
	if err != nil {
		return nil, err
	}
	stopExpr, err := s.normalizeOptionalNamedExpression(line.number, "exit stop expression", orderArgs, "stop")
	if err != nil {
		return nil, err
	}
	limitExpr, err := s.normalizeOptionalNamedExpression(line.number, "exit limit expression", orderArgs, "limit")
	if err != nil {
		return nil, err
	}
	profitExpr, err := s.normalizeOptionalNamedExpression(line.number, "exit profit expression", orderArgs, "profit")
	if err != nil {
		return nil, err
	}
	lossExpr, err := s.normalizeOptionalNamedExpression(line.number, "exit loss expression", orderArgs, "loss")
	if err != nil {
		return nil, err
	}
	if stopExpr != "" || limitExpr != "" || profitExpr != "" || lossExpr != "" {
		return s.parseStrategyBracketExit(line, exitID, fromEntry, direction, orderArgs, quantityMode, quantityExpr, stopExpr, limitExpr, profitExpr, lossExpr)
	}
	if hasTrail {
		return s.parseStrategyTrailingExit(line, exitID, fromEntry, direction, orderArgs, quantityMode, quantityExpr)
	}
	// validateStrategyExitTriggers guarantees every valid non-trailing exit has
	// at least one bracket trigger, so it always belongs to the bracket path.
	return s.parseStrategyBracketExit(line, exitID, fromEntry, direction, orderArgs, quantityMode, quantityExpr, stopExpr, limitExpr, profitExpr, lossExpr)
}

func (s *parseState) parseWhenExpression(lineNumber int, functionName string, args []string) (string, error) {
	raw, ok := namedArgValue(args, "when")
	if !ok {
		return "", nil
	}
	whenExpr := s.normalizeExpression(raw)
	if err := s.takeNormalizationErr(lineNumber); err != nil {
		return "", err
	}
	if err := validateExpression(lineNumber, functionName+" when expression", whenExpr); err != nil {
		return "", err
	}
	return whenExpr, nil
}

func parseLogOrAlert(line parsedLine) (strategyir.Statement, bool) {
	lower := strings.ToLower(line.trimmed)
	if strings.HasPrefix(lower, "alert(") {
		return &strategyir.NotifyStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, Message: firstStringArgument(line.trimmed)}, true
	}
	if strings.HasPrefix(lower, "log.info(") || strings.HasPrefix(lower, "log.warning(") || strings.HasPrefix(lower, "log.error(") {
		return &strategyir.LogStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, Message: firstStringArgument(line.trimmed)}, true
	}
	return nil, false
}
