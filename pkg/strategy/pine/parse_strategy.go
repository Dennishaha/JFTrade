package pine

import (
	"fmt"
	"regexp"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

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
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 id,
			Action:             action,
			Intent:             strategyir.OrderIntentEntry,
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
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 id,
			Action:             action,
			Intent:             strategyir.OrderIntentNet,
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
		comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(line.number, "strategy.close_all", args)
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
		comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(line.number, "strategy.close", args[1:])
		if err != nil {
			return nil, true, err
		}
		id := unquote(strings.TrimSpace(args[0]))
		action := strategyir.OrderActionSell
		if s.shortEntryIDs[id] {
			action = strategyir.OrderActionCover
		}
		quantityMode, quantityExpr := pineCloseQuantity(args[1:], id)
		orderType, limitExpr, _ := pineOrderPrices(args[1:])
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
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 id,
			Action:             action,
			Intent:             strategyir.OrderIntentClose,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			EntryPolicy:        "same_direction",
			OrderType:          orderType,
			LimitExpression:    limitExpr,
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
	triggerCount := 0
	for _, name := range []string{"stop", "limit", "trail_points", "trail_price"} {
		if _, ok := namedArgValue(orderArgs, name); ok {
			triggerCount++
		}
	}
	hasStop := hasNamedArg(orderArgs, "stop")
	hasLimit := hasNamedArg(orderArgs, "limit")
	hasTrailPoints := hasNamedArg(orderArgs, "trail_points")
	hasTrailPrice := hasNamedArg(orderArgs, "trail_price")
	hasTrail := hasTrailPoints || hasTrailPrice
	if hasTrailPoints && hasTrailPrice {
		return nil, fmt.Errorf("pine line %d: strategy.exit accepts trail_points or trail_price, not both", line.number)
	}
	if hasTrail && (hasStop || hasLimit) {
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
	if stopExpr != "" || limitExpr != "" {
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.exit", orderArgs, false)
		if err != nil {
			return nil, err
		}
		return &strategyir.ExitStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 exitID,
			FromEntry:          fromEntry,
			Direction:          direction,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			StopExpression:     stopExpr,
			LimitExpression:    limitExpr,
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
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
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.exit", orderArgs, false)
		if err != nil {
			return nil, err
		}
		statement := &strategyir.ExitStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 exitID,
			FromEntry:          fromEntry,
			Direction:          direction,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			TrailOffset:        trailOffsetExpr,
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
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

func pineProtectStatement(lineNumber int, direction string, mode string, percentage string, quantityMode string, quantityExpression string) *strategyir.ProtectStmt {
	return &strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: lineNumber, EndLine: lineNumber},
		Direction:            direction,
		Mode:                 mode,
		QuantityMode:         quantityMode,
		QuantityExpression:   quantityExpression,
		TimeValueExpression:  "1",
		TimeUnit:             "bar",
		PercentageExpression: percentage,
		WindowPolicy:         "continuous",
	}
}

func pineExitPercentage(expression string, pattern *regexp.Regexp) (string, bool) {
	normalized := stripWrappingParens(strings.TrimSpace(expression))
	match := pattern.FindStringSubmatch(normalized)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

func parseLogOrAlert(line parsedLine) (strategyir.Statement, bool) {
	lower := strings.ToLower(line.trimmed)
	if strings.HasPrefix(lower, "alert(") || strings.HasPrefix(lower, "notify(") {
		return &strategyir.NotifyStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, Message: firstStringArgument(line.trimmed)}, true
	}
	if strings.HasPrefix(lower, "log.info(") || strings.HasPrefix(lower, "log.warning(") || strings.HasPrefix(lower, "log.error(") {
		return &strategyir.LogStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, Message: firstStringArgument(line.trimmed)}, true
	}
	return nil, false
}
