package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) parseStrategyAllowedEntryRisk(line parsedLine) error {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) != 1 {
		return fmt.Errorf("pine line %d: strategy.risk.allow_entry_in(direction) requires one argument", line.number)
	}
	direction, ok := normalizeStrategyAllowedEntryDirection(args[0])
	if !ok {
		return fmt.Errorf("pine line %d: strategy.risk.allow_entry_in direction %q is not supported", line.number, strings.TrimSpace(args[0]))
	}
	s.strategyMetadata.AllowedEntryDirection = direction
	return nil
}

func (s *parseState) parseStrategyMaxDrawdownRisk(line parsedLine) error {
	value, amountType, alertMessage, err := parseStrategyRiskAmountArgs(line.number, "strategy.risk.max_drawdown", splitArguments(callArgs(line.trimmed)))
	if err != nil {
		return err
	}
	s.strategyMetadata.MaxDrawdownValue = value
	s.strategyMetadata.MaxDrawdownType = amountType
	s.strategyMetadata.MaxDrawdownAlert = alertMessage
	return nil
}

func (s *parseState) parseStrategyMaxIntradayLossRisk(line parsedLine) error {
	value, amountType, alertMessage, err := parseStrategyRiskAmountArgs(line.number, "strategy.risk.max_intraday_loss", splitArguments(callArgs(line.trimmed)))
	if err != nil {
		return err
	}
	s.strategyMetadata.MaxIntradayLossValue = value
	s.strategyMetadata.MaxIntradayLossType = amountType
	s.strategyMetadata.MaxIntradayLossAlert = alertMessage
	return nil
}

func (s *parseState) parseStrategyMaxIntradayFilledOrdersRisk(line parsedLine) error {
	count, alertMessage, err := parseStrategyRiskCountArgs(line.number, "strategy.risk.max_intraday_filled_orders", splitArguments(callArgs(line.trimmed)))
	if err != nil {
		return err
	}
	s.strategyMetadata.MaxIntradayFilledOrders = count
	s.strategyMetadata.MaxIntradayFilledOrdersAlert = alertMessage
	return nil
}

func (s *parseState) parseStrategyMaxPositionSizeRisk(line parsedLine) error {
	contracts, err := parseStrategyRiskPositionSize(line.number, splitArguments(callArgs(line.trimmed)))
	if err != nil {
		return err
	}
	s.strategyMetadata.MaxPositionSize = contracts
	return nil
}

func (s *parseState) parseStrategyMaxConsLossDaysRisk(line parsedLine) error {
	count, alertMessage, err := parseStrategyRiskCountArgs(line.number, "strategy.risk.max_cons_loss_days", splitArguments(callArgs(line.trimmed)))
	if err != nil {
		return err
	}
	s.strategyMetadata.MaxConsLossDays = count
	s.strategyMetadata.MaxConsLossDaysAlert = alertMessage
	return nil
}

func (s *parseState) parseStrategyEntryCall(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) < 2 {
		return nil, fmt.Errorf("pine line %d: strategy.entry(id, direction, ...) requires at least two arguments", line.number)
	}
	id := unquote(strings.TrimSpace(args[0]))
	direction := strings.ToLower(strings.TrimSpace(args[1]))
	orderArgs := args[2:]
	if err := validateStrategyOrderArgs(line.number, "strategy.entry", orderArgs); err != nil {
		return nil, err
	}
	comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.entry", orderArgs, false)
	if err != nil {
		return nil, err
	}
	quantityMode, quantityExpr := s.pineEntryQuantity(orderArgs)
	orderType, limitExpr, stopExpr := pineOrderPrices(orderArgs)
	quantityExpr, orderType, limitExpr, stopExpr, whenExpr, err := s.normalizeStrategyOrderExpressions(line.number, "strategy.entry", orderArgs, quantityExpr, orderType, limitExpr, stopExpr)
	if err != nil {
		return nil, err
	}

	action := strategyir.OrderActionBuy
	if strings.Contains(direction, "short") {
		action = strategyir.OrderActionShort
		s.shortEntryIDs[id] = true
	} else {
		s.longEntryIDs[id] = true
	}

	return &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		ID:                 id,
		Action:             action,
		Intent:             strategyir.OrderIntentEntry,
		WhenExpression:     whenExpr,
		QuantityMode:       quantityMode,
		QuantityExpression: quantityExpr,
		EntryPolicy:        s.readEntryPolicyForLine(line.number),
		OrderType:          orderType,
		LimitExpression:    limitExpr,
		StopExpression:     stopExpr,
		Comment:            comment,
		AlertMessage:       alertMessage,
		DisableAlert:       disableAlert,
	}, nil
}

func (s *parseState) parseStrategyOrderCall(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) < 2 {
		return nil, fmt.Errorf("pine line %d: strategy.order(id, direction, ...) requires at least two arguments", line.number)
	}
	id := unquote(strings.TrimSpace(args[0]))
	orderArgs := args[2:]
	if err := validateStrategyOrderArgs(line.number, "strategy.order", orderArgs); err != nil {
		return nil, err
	}
	comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.order", orderArgs, false)
	if err != nil {
		return nil, err
	}
	direction := strings.ToLower(strings.TrimSpace(args[1]))
	quantityMode, quantityExpr := s.pineEntryQuantity(orderArgs)
	orderType, limitExpr, stopExpr := pineOrderPrices(orderArgs)
	quantityExpr, orderType, limitExpr, stopExpr, whenExpr, err := s.normalizeStrategyOrderExpressions(line.number, "strategy.order", orderArgs, quantityExpr, orderType, limitExpr, stopExpr)
	if err != nil {
		return nil, err
	}

	action := strategyir.OrderActionBuy
	if strings.Contains(direction, "short") {
		action = strategyir.OrderActionSell
	}
	return &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		ID:                 id,
		Action:             action,
		Intent:             strategyir.OrderIntentNet,
		WhenExpression:     whenExpr,
		QuantityMode:       quantityMode,
		QuantityExpression: quantityExpr,
		EntryPolicy:        "allow",
		OrderType:          orderType,
		LimitExpression:    limitExpr,
		StopExpression:     stopExpr,
		Comment:            comment,
		AlertMessage:       alertMessage,
		DisableAlert:       disableAlert,
	}, nil
}

func (s *parseState) parseStrategyCloseAllCall(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if err := rejectUnsupportedNamedArgs(line.number, "strategy.close_all", args, "immediately", "comment", "alert_message", "disable_alert"); err != nil {
		return nil, err
	}
	comment, alertMessage, disableAlert, immediate, err := pineCloseAllMetadata(line.number, args)
	if err != nil {
		return nil, err
	}
	return &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Intent:             strategyir.OrderIntentFlatten,
		QuantityMode:       "symbol_position_percent",
		QuantityExpression: "100",
		EntryPolicy:        "same_direction",
		OrderType:          "MARKET",
		Comment:            comment,
		AlertMessage:       alertMessage,
		DisableAlert:       disableAlert,
		Immediate:          immediate,
	}, nil
}

func (s *parseState) parseStrategyCloseCall(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) == 0 {
		return nil, fmt.Errorf("pine line %d: strategy.close(id) requires an entry id", line.number)
	}
	orderArgs := args[1:]
	if err := rejectUnsupportedNamedArgs(line.number, "strategy.close", orderArgs, "qty", "qty_percent", "limit", "stop", "comment", "alert_message", "immediately", "disable_alert", "when"); err != nil {
		return nil, err
	}
	comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(line.number, "strategy.close", orderArgs)
	if err != nil {
		return nil, err
	}
	id := unquote(strings.TrimSpace(args[0]))
	action := strategyir.OrderActionSell
	if s.shortEntryIDs[id] {
		action = strategyir.OrderActionCover
	}
	if err := rejectConflictingQuantityArgs(line.number, "strategy.close", orderArgs); err != nil {
		return nil, err
	}
	quantityMode, quantityExpr := pineCloseQuantity(orderArgs, id)
	orderType, limitExpr, stopExpr := pineOrderPrices(orderArgs)
	quantityExpr, orderType, limitExpr, stopExpr, whenExpr, err := s.normalizeStrategyOrderExpressions(line.number, "strategy.close", orderArgs, quantityExpr, orderType, limitExpr, stopExpr)
	if err != nil {
		return nil, err
	}
	return &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		ID:                 id,
		Action:             action,
		Intent:             strategyir.OrderIntentClose,
		WhenExpression:     whenExpr,
		QuantityMode:       quantityMode,
		QuantityExpression: quantityExpr,
		EntryPolicy:        "same_direction",
		OrderType:          orderType,
		LimitExpression:    limitExpr,
		StopExpression:     stopExpr,
		Comment:            comment,
		AlertMessage:       alertMessage,
		DisableAlert:       disableAlert,
		Immediate:          immediate,
	}, nil
}

func validateStrategyOrderArgs(lineNumber int, functionName string, args []string) error {
	if err := rejectUnsupportedOrderArgs(lineNumber, functionName, args); err != nil {
		return err
	}
	if err := rejectUnsupportedNamedArgs(lineNumber, functionName, args, "qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"); err != nil {
		return err
	}
	return rejectConflictingQuantityArgs(lineNumber, functionName, args)
}

func (s *parseState) normalizeStrategyOrderExpressions(
	lineNumber int,
	functionName string,
	orderArgs []string,
	quantityExpr string,
	orderType string,
	limitExpr string,
	stopExpr string,
) (string, string, string, string, string, error) {
	normalizedQuantity, err := s.normalizeRequiredExpression(lineNumber, "order quantity expression", quantityExpr)
	if err != nil {
		return "", "", "", "", "", err
	}
	normalizedLimit, err := s.normalizeOptionalExpression(lineNumber, "order limit expression", limitExpr)
	if err != nil {
		return "", "", "", "", "", err
	}
	normalizedStop, err := s.normalizeOptionalExpression(lineNumber, "order stop expression", stopExpr)
	if err != nil {
		return "", "", "", "", "", err
	}
	whenExpr, err := s.parseWhenExpression(lineNumber, functionName, orderArgs)
	if err != nil {
		return "", "", "", "", "", err
	}
	return normalizedQuantity, orderType, normalizedLimit, normalizedStop, whenExpr, nil
}

func (s *parseState) normalizeRequiredExpression(lineNumber int, fieldName string, expr string) (string, error) {
	normalized := s.normalizeExpression(expr)
	if err := s.takeNormalizationErr(lineNumber); err != nil {
		return "", err
	}
	if err := validateExpression(lineNumber, fieldName, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func (s *parseState) normalizeOptionalExpression(lineNumber int, fieldName string, expr string) (string, error) {
	normalized := s.normalizeExpression(expr)
	if err := s.takeNormalizationErr(lineNumber); err != nil {
		return "", err
	}
	if strings.TrimSpace(normalized) == "" {
		return normalized, nil
	}
	if err := validateExpression(lineNumber, fieldName, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func (s *parseState) normalizeOptionalNamedExpression(lineNumber int, fieldName string, args []string, name string) (string, error) {
	raw, ok := namedArgValue(args, name)
	if !ok {
		return "", nil
	}
	return s.normalizeRequiredExpression(lineNumber, fieldName, raw)
}

func (s *parseState) parseStrategyBracketExit(
	line parsedLine,
	exitID string,
	fromEntry string,
	direction string,
	orderArgs []string,
	quantityMode string,
	quantityExpr string,
	stopExpr string,
	limitExpr string,
	profitExpr string,
	lossExpr string,
) (strategyir.Statement, error) {
	metadata, err := pineExitMetadata(line.number, orderArgs)
	if err != nil {
		return nil, err
	}
	whenExpr, err := s.parseWhenExpression(line.number, "strategy.exit", orderArgs)
	if err != nil {
		return nil, err
	}
	return &strategyir.ExitStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		ID:                 exitID,
		FromEntry:          fromEntry,
		Direction:          direction,
		WhenExpression:     whenExpr,
		QuantityMode:       quantityMode,
		QuantityExpression: quantityExpr,
		ProfitExpression:   profitExpr,
		LossExpression:     lossExpr,
		StopExpression:     stopExpr,
		LimitExpression:    limitExpr,
		Comment:            metadata.comment,
		CommentProfit:      metadata.commentProfit,
		CommentLoss:        metadata.commentLoss,
		CommentTrailing:    metadata.commentTrailing,
		AlertMessage:       metadata.alertMessage,
		AlertProfit:        metadata.alertProfit,
		AlertLoss:          metadata.alertLoss,
		AlertTrailing:      metadata.alertTrailing,
		DisableAlert:       metadata.disableAlert,
	}, nil
}

func (s *parseState) parseStrategyTrailingExit(
	line parsedLine,
	exitID string,
	fromEntry string,
	direction string,
	orderArgs []string,
	quantityMode string,
	quantityExpr string,
) (strategyir.Statement, error) {
	trailOffset, hasOffset := namedArgValue(orderArgs, "trail_offset")
	if !hasOffset || strings.TrimSpace(trailOffset) == "" {
		return nil, fmt.Errorf("pine line %d: strategy.exit trailing stop requires trail_offset", line.number)
	}
	trailOffsetExpr, err := s.normalizeRequiredExpression(line.number, "exit trail_offset expression", trailOffset)
	if err != nil {
		return nil, err
	}
	metadata, err := pineExitMetadata(line.number, orderArgs)
	if err != nil {
		return nil, err
	}
	whenExpr, err := s.parseWhenExpression(line.number, "strategy.exit", orderArgs)
	if err != nil {
		return nil, err
	}
	statement := &strategyir.ExitStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		ID:                 exitID,
		FromEntry:          fromEntry,
		Direction:          direction,
		WhenExpression:     whenExpr,
		QuantityMode:       quantityMode,
		QuantityExpression: quantityExpr,
		TrailOffset:        trailOffsetExpr,
		Comment:            metadata.comment,
		CommentProfit:      metadata.commentProfit,
		CommentLoss:        metadata.commentLoss,
		CommentTrailing:    metadata.commentTrailing,
		AlertMessage:       metadata.alertMessage,
		AlertProfit:        metadata.alertProfit,
		AlertLoss:          metadata.alertLoss,
		AlertTrailing:      metadata.alertTrailing,
		DisableAlert:       metadata.disableAlert,
	}
	statement.TrailPoints, err = s.normalizeOptionalNamedExpression(line.number, "exit trail_points expression", orderArgs, "trail_points")
	if err != nil {
		return nil, err
	}
	statement.TrailPrice, err = s.normalizeOptionalNamedExpression(line.number, "exit trail_price expression", orderArgs, "trail_price")
	if err != nil {
		return nil, err
	}
	return statement, nil
}

func parseStrategyExitFromEntry(orderArgs []string) (string, []string) {
	if named, ok := namedArgValue(orderArgs, "from_entry"); ok {
		return unquote(strings.TrimSpace(named)), orderArgs
	}
	if len(orderArgs) > 0 && !strings.Contains(orderArgs[0], "=") {
		return unquote(strings.TrimSpace(orderArgs[0])), orderArgs[1:]
	}
	return "", orderArgs
}

func validateStrategyExitTriggers(lineNumber int, orderArgs []string) (bool, error) {
	triggerCount := 0
	for _, name := range []string{"stop", "limit", "profit", "loss", "trail_points", "trail_price"} {
		if _, ok := namedArgValue(orderArgs, name); ok {
			triggerCount++
		}
	}
	hasStop := hasNamedArg(orderArgs, "stop")
	hasLimit := hasNamedArg(orderArgs, "limit")
	hasProfit := hasNamedArg(orderArgs, "profit")
	hasLoss := hasNamedArg(orderArgs, "loss")
	hasTrailPoints := hasNamedArg(orderArgs, "trail_points")
	hasTrailPrice := hasNamedArg(orderArgs, "trail_price")
	hasTrail := hasTrailPoints || hasTrailPrice
	if hasTrailPoints && hasTrailPrice {
		return false, fmt.Errorf("pine line %d: strategy.exit accepts trail_points or trail_price, not both", lineNumber)
	}
	if hasTrail && (hasStop || hasLimit || hasProfit || hasLoss) {
		return false, fmt.Errorf("pine line %d: strategy.exit trail with stop/limit is not supported by JFTrade yet", lineNumber)
	}
	if triggerCount == 0 {
		return false, fmt.Errorf("pine line %d: strategy.exit advanced exit semantics are not supported by JFTrade yet", lineNumber)
	}
	return hasTrail, nil
}

func strategyExitDirection(fromEntry string) string {
	trimmed := strings.TrimSpace(fromEntry)
	if trimmed == "" {
		return "auto"
	}
	if strings.Contains(strings.ToLower(trimmed), "short") {
		return "short"
	}
	return "long"
}
