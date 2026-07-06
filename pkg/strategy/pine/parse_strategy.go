package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

//nolint:funlen
func (s *parseState) parseStrategyCall(line parsedLine) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.HasPrefix(lower, "strategy.risk.allow_entry_in("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) != 1 {
			return nil, true, fmt.Errorf("pine line %d: strategy.risk.allow_entry_in(direction) requires one argument", line.number)
		}
		direction, ok := normalizeStrategyAllowedEntryDirection(args[0])
		if !ok {
			return nil, true, fmt.Errorf("pine line %d: strategy.risk.allow_entry_in direction %q is not supported", line.number, strings.TrimSpace(args[0]))
		}
		s.strategyMetadata.AllowedEntryDirection = direction
		return nil, true, nil
	case strings.HasPrefix(lower, "strategy.risk.max_drawdown("):
		value, amountType, alertMessage, err := parseStrategyRiskAmountArgs(line.number, "strategy.risk.max_drawdown", splitArguments(callArgs(line.trimmed)))
		if err != nil {
			return nil, true, err
		}
		s.strategyMetadata.MaxDrawdownValue = value
		s.strategyMetadata.MaxDrawdownType = amountType
		s.strategyMetadata.MaxDrawdownAlert = alertMessage
		return nil, true, nil
	case strings.HasPrefix(lower, "strategy.risk.max_intraday_loss("):
		value, amountType, alertMessage, err := parseStrategyRiskAmountArgs(line.number, "strategy.risk.max_intraday_loss", splitArguments(callArgs(line.trimmed)))
		if err != nil {
			return nil, true, err
		}
		s.strategyMetadata.MaxIntradayLossValue = value
		s.strategyMetadata.MaxIntradayLossType = amountType
		s.strategyMetadata.MaxIntradayLossAlert = alertMessage
		return nil, true, nil
	case strings.HasPrefix(lower, "strategy.risk.max_intraday_filled_orders("):
		count, alertMessage, err := parseStrategyRiskCountArgs(line.number, "strategy.risk.max_intraday_filled_orders", splitArguments(callArgs(line.trimmed)))
		if err != nil {
			return nil, true, err
		}
		s.strategyMetadata.MaxIntradayFilledOrders = count
		s.strategyMetadata.MaxIntradayFilledOrdersAlert = alertMessage
		return nil, true, nil
	case strings.HasPrefix(lower, "strategy.risk.max_position_size("):
		contracts, err := parseStrategyRiskPositionSize(line.number, splitArguments(callArgs(line.trimmed)))
		if err != nil {
			return nil, true, err
		}
		s.strategyMetadata.MaxPositionSize = contracts
		return nil, true, nil
	case strings.HasPrefix(lower, "strategy.risk.max_cons_loss_days("):
		count, alertMessage, err := parseStrategyRiskCountArgs(line.number, "strategy.risk.max_cons_loss_days", splitArguments(callArgs(line.trimmed)))
		if err != nil {
			return nil, true, err
		}
		s.strategyMetadata.MaxConsLossDays = count
		s.strategyMetadata.MaxConsLossDaysAlert = alertMessage
		return nil, true, nil
	case strings.HasPrefix(lower, "strategy.entry("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: strategy.entry(id, direction, ...) requires at least two arguments", line.number)
		}
		id := unquote(strings.TrimSpace(args[0]))
		direction := strings.ToLower(strings.TrimSpace(args[1]))
		if err := rejectUnsupportedOrderArgs(line.number, "strategy.entry", args[2:]); err != nil {
			return nil, true, err
		}
		if err := rejectUnsupportedNamedArgs(line.number, "strategy.entry", args[2:], "qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"); err != nil {
			return nil, true, err
		}
		if err := rejectConflictingQuantityArgs(line.number, "strategy.entry", args[2:]); err != nil {
			return nil, true, err
		}
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.entry", args[2:], false)
		if err != nil {
			return nil, true, err
		}
		quantityMode, quantityExpr := s.pineEntryQuantity(args[2:])
		orderType, limitExpr, stopExpr := pineOrderPrices(args[2:])
		action := strategyir.OrderActionBuy
		if strings.Contains(direction, "short") {
			action = strategyir.OrderActionShort
			s.shortEntryIDs[id] = true
		} else {
			s.longEntryIDs[id] = true
		}
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		stopExpr = s.normalizeExpression(stopExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(stopExpr) != "" {
			if err := validateExpression(line.number, "order stop expression", stopExpr); err != nil {
				return nil, true, err
			}
		}
		whenExpr, err := s.parseWhenExpression(line.number, "strategy.entry", args[2:])
		if err != nil {
			return nil, true, err
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
		}, true, nil
	case strings.HasPrefix(lower, "strategy.order("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: strategy.order(id, direction, ...) requires at least two arguments", line.number)
		}
		id := unquote(strings.TrimSpace(args[0]))
		if err := rejectUnsupportedOrderArgs(line.number, "strategy.order", args[2:]); err != nil {
			return nil, true, err
		}
		if err := rejectUnsupportedNamedArgs(line.number, "strategy.order", args[2:], "qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"); err != nil {
			return nil, true, err
		}
		if err := rejectConflictingQuantityArgs(line.number, "strategy.order", args[2:]); err != nil {
			return nil, true, err
		}
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.order", args[2:], false)
		if err != nil {
			return nil, true, err
		}
		direction := strings.ToLower(strings.TrimSpace(args[1]))
		quantityMode, quantityExpr := s.pineEntryQuantity(args[2:])
		orderType, limitExpr, stopExpr := pineOrderPrices(args[2:])
		action := strategyir.OrderActionBuy
		if strings.Contains(direction, "short") {
			action = strategyir.OrderActionSell
		}
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		stopExpr = s.normalizeExpression(stopExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(stopExpr) != "" {
			if err := validateExpression(line.number, "order stop expression", stopExpr); err != nil {
				return nil, true, err
			}
		}
		whenExpr, err := s.parseWhenExpression(line.number, "strategy.order", args[2:])
		if err != nil {
			return nil, true, err
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
		}, true, nil
	case strings.HasPrefix(lower, "strategy.close_all("):
		args := splitArguments(callArgs(line.trimmed))
		if err := rejectUnsupportedNamedArgs(line.number, "strategy.close_all", args, "immediately", "comment", "alert_message", "disable_alert"); err != nil {
			return nil, true, err
		}
		comment, alertMessage, disableAlert, immediate, err := pineCloseAllMetadata(line.number, args)
		if err != nil {
			return nil, true, err
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
		}, true, nil
	case strings.HasPrefix(lower, "strategy.close("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) == 0 {
			return nil, true, fmt.Errorf("pine line %d: strategy.close(id) requires an entry id", line.number)
		}
		if err := rejectUnsupportedNamedArgs(line.number, "strategy.close", args[1:], "qty", "qty_percent", "limit", "stop", "comment", "alert_message", "immediately", "disable_alert", "when"); err != nil {
			return nil, true, err
		}
		comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(line.number, "strategy.close", args[1:])
		if err != nil {
			return nil, true, err
		}
		id := unquote(strings.TrimSpace(args[0]))
		action := strategyir.OrderActionSell
		if s.shortEntryIDs[id] {
			action = strategyir.OrderActionCover
		}
		if err := rejectConflictingQuantityArgs(line.number, "strategy.close", args[1:]); err != nil {
			return nil, true, err
		}
		quantityMode, quantityExpr := pineCloseQuantity(args[1:], id)
		orderType, limitExpr, stopExpr := pineOrderPrices(args[1:])
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		stopExpr = s.normalizeExpression(stopExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(stopExpr) != "" {
			if err := validateExpression(line.number, "order stop expression", stopExpr); err != nil {
				return nil, true, err
			}
		}
		whenExpr, err := s.parseWhenExpression(line.number, "strategy.close", args[1:])
		if err != nil {
			return nil, true, err
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
		}, true, nil
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

//nolint:funlen
func (s *parseState) parseStrategyExit(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) < 1 {
		return nil, fmt.Errorf("pine line %d: strategy.exit(id, ...) requires an exit id", line.number)
	}
	exitID := unquote(strings.TrimSpace(args[0]))
	fromEntry := ""
	orderArgs := args[1:]
	if named, ok := namedArgValue(orderArgs, "from_entry"); ok {
		fromEntry = unquote(strings.TrimSpace(named))
	} else if len(orderArgs) > 0 && !strings.Contains(orderArgs[0], "=") {
		fromEntry = unquote(strings.TrimSpace(orderArgs[0]))
		orderArgs = orderArgs[1:]
	}
	if err := rejectUnsupportedOrderArgs(line.number, "strategy.exit", orderArgs); err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("pine line %d: strategy.exit accepts trail_points or trail_price, not both", line.number)
	}
	if hasTrail && (hasStop || hasLimit || hasProfit || hasLoss) {
		return nil, fmt.Errorf("pine line %d: strategy.exit trail with stop/limit is not supported by JFTrade yet", line.number)
	}
	if triggerCount == 0 {
		return nil, fmt.Errorf("pine line %d: strategy.exit advanced exit semantics are not supported by JFTrade yet", line.number)
	}
	fromEntryLower := strings.ToLower(fromEntry)
	direction := "long"
	if strings.Contains(fromEntryLower, "short") {
		direction = "short"
	}
	if strings.TrimSpace(fromEntry) == "" {
		direction = "auto"
	}
	if err := rejectUnsupportedNamedArgs(line.number, "strategy.exit", orderArgs, "from_entry", "qty", "qty_percent", "profit", "limit", "loss", "stop", "trail_price", "trail_points", "trail_offset", "oca_name", "oca_type", "comment", "comment_profit", "comment_loss", "comment_trailing", "alert_message", "alert_profit", "alert_loss", "alert_trailing", "disable_alert", "when"); err != nil {
		return nil, err
	}
	if err := rejectConflictingQuantityArgs(line.number, "strategy.exit", orderArgs); err != nil {
		return nil, err
	}
	quantityMode, quantityExpr := pineExitQuantity(orderArgs)
	quantityExpr = s.normalizeExpression(quantityExpr)
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, err
	}
	if err := validateExpression(line.number, "exit quantity expression", quantityExpr); err != nil {
		return nil, err
	}
	stopExpr := ""
	if raw, ok := namedArgValue(orderArgs, "stop"); ok {
		stopExpr = s.normalizeExpression(raw)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit stop expression", stopExpr); err != nil {
			return nil, err
		}
	}
	limitExpr := ""
	if raw, ok := namedArgValue(orderArgs, "limit"); ok {
		limitExpr = s.normalizeExpression(raw)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit limit expression", limitExpr); err != nil {
			return nil, err
		}
	}
	profitExpr := ""
	if raw, ok := namedArgValue(orderArgs, "profit"); ok {
		profitExpr = s.normalizeExpression(raw)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit profit expression", profitExpr); err != nil {
			return nil, err
		}
	}
	lossExpr := ""
	if raw, ok := namedArgValue(orderArgs, "loss"); ok {
		lossExpr = s.normalizeExpression(raw)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit loss expression", lossExpr); err != nil {
			return nil, err
		}
	}
	if stopExpr != "" || limitExpr != "" || profitExpr != "" || lossExpr != "" {
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
	if hasTrail {
		trailOffset, hasOffset := namedArgValue(orderArgs, "trail_offset")
		if !hasOffset || strings.TrimSpace(trailOffset) == "" {
			return nil, fmt.Errorf("pine line %d: strategy.exit trailing stop requires trail_offset", line.number)
		}
		trailOffsetExpr := s.normalizeExpression(trailOffset)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit trail_offset expression", trailOffsetExpr); err != nil {
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
		if raw, ok := namedArgValue(orderArgs, "trail_points"); ok {
			statement.TrailPoints = s.normalizeExpression(raw)
			if err := s.takeNormalizationErr(line.number); err != nil {
				return nil, err
			}
			if err := validateExpression(line.number, "exit trail_points expression", statement.TrailPoints); err != nil {
				return nil, err
			}
		}
		if raw, ok := namedArgValue(orderArgs, "trail_price"); ok {
			statement.TrailPrice = s.normalizeExpression(raw)
			if err := s.takeNormalizationErr(line.number); err != nil {
				return nil, err
			}
			if err := validateExpression(line.number, "exit trail_price expression", statement.TrailPrice); err != nil {
				return nil, err
			}
		}
		return statement, nil
	}
	return nil, fmt.Errorf("pine line %d: strategy.exit advanced exit semantics are not supported by JFTrade yet", line.number)
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
