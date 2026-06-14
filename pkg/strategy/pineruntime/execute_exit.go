package pineruntime

import (
	"fmt"
	"math"
	"strings"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeExitStatement(statement *strategyir.ExitStmt, scope *evaluationScope) (bool, error) {
	position := r.getPosition(r.symbol, scope.currentKlineTime)
	if position == nil {
		delete(r.trailingExits, trailingExitKey(statement))
		return false, nil
	}
	availableQuantity := math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	if availableQuantity <= 0 {
		delete(r.trailingExits, trailingExitKey(statement))
		return false, nil
	}
	direction := normalizeProtectDirection(statement.Direction)
	if direction == "" || direction == "both" {
		if position.Direction == "SHORT" {
			direction = "short"
		} else {
			direction = "long"
		}
	}
	allowLongExit := direction != "short"
	allowShortExit := direction != "long"
	if strings.TrimSpace(statement.TrailOffset) != "" {
		return r.executeTrailingExit(statement, scope, position, availableQuantity, direction)
	}
	stopPrice, hasStop, err := evaluateOptionalFloatExpression(statement.StopExpression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	limitPrice, hasLimit, err := evaluateOptionalFloatExpression(statement.LimitExpression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if !hasStop && !hasLimit {
		return false, nil
	}
	high, low, closePrice := currentBarPrices(scope)
	triggered := false
	triggerPrice := closePrice
	if allowLongExit && position.Direction == "LONG" {
		stopTriggered := hasStop && low <= stopPrice
		limitTriggered := hasLimit && high >= limitPrice
		if stopTriggered && limitTriggered {
			r.internalLog("strategy.exit bracket hit stop and limit in same bar; using stop-first")
		}
		switch {
		case stopTriggered:
			triggered = true
			triggerPrice = stopPrice
		case limitTriggered:
			triggered = true
			triggerPrice = limitPrice
		}
	}
	if allowShortExit && position.Direction == "SHORT" {
		stopTriggered := hasStop && high >= stopPrice
		limitTriggered := hasLimit && low <= limitPrice
		if stopTriggered && limitTriggered {
			r.internalLog("strategy.exit bracket hit stop and limit in same bar; using stop-first")
		}
		switch {
		case stopTriggered:
			triggered = true
			triggerPrice = stopPrice
		case limitTriggered:
			triggered = true
			triggerPrice = limitPrice
		}
	}
	if !triggered {
		return false, nil
	}
	quantityMode := strings.TrimSpace(statement.QuantityMode)
	if quantityMode == "" {
		quantityMode = "symbol_position_percent"
	}
	mode, ok := indicatorbinding.ParseQuantityMode(quantityMode)
	if !ok {
		return false, fmt.Errorf("pine line %d: unsupported exit quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}
	quantityExpr := strings.TrimSpace(statement.QuantityExpression)
	if quantityExpr == "" {
		quantityExpr = "100"
	}
	quantity, err := r.resolveOrderQuantity(&strategyir.OrderStmt{
		Range:              statement.Range,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       mode,
		QuantityExpression: quantityExpr,
	}, scope, position, availableQuantity, triggerPrice, mode)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if quantity <= 0 {
		return false, nil
	}
	if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
		r.internalLog("exit suppressed during warmup")
		return true, nil
	}
	if position.Direction == "LONG" {
		if err := r.submitOrder(types.SideTypeSell, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("LONG")
		return true, nil
	}
	if position.Direction == "SHORT" {
		if err := r.submitOrder(types.SideTypeBuy, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("SHORT")
		return true, nil
	}
	return false, nil
}

func (r *strategyRuntime) executeTrailingExit(
	statement *strategyir.ExitStmt,
	scope *evaluationScope,
	position *positionSnapshot,
	availableQuantity float64,
	direction string,
) (bool, error) {
	offsetTicks, err := evaluateFloatExpression(statement.TrailOffset, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if offsetTicks <= 0 || math.IsNaN(offsetTicks) || math.IsInf(offsetTicks, 0) {
		return false, fmt.Errorf("pine line %d: trail_offset must be positive", statement.Range.StartLine)
	}
	tickSize := r.marketTickSize()
	offset := offsetTicks * tickSize
	activationPrice := 0.0
	if strings.TrimSpace(statement.TrailPrice) != "" {
		activationPrice, err = evaluateFloatExpression(statement.TrailPrice, scope)
		if err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
	} else {
		points, evalErr := evaluateFloatExpression(statement.TrailPoints, scope)
		if evalErr != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, evalErr)
		}
		if points <= 0 || math.IsNaN(points) || math.IsInf(points, 0) {
			return false, fmt.Errorf("pine line %d: trail_points must be positive", statement.Range.StartLine)
		}
		basePrice := position.AveragePrice
		if basePrice <= 0 && scope.currentKline != nil {
			basePrice = scope.currentKline.Close.Float64()
		}
		if position.Direction == "SHORT" {
			activationPrice = basePrice - points*tickSize
		} else {
			activationPrice = basePrice + points*tickSize
		}
	}
	if activationPrice <= 0 || math.IsNaN(activationPrice) || math.IsInf(activationPrice, 0) {
		return false, fmt.Errorf("pine line %d: trailing activation price must be positive", statement.Range.StartLine)
	}

	key := trailingExitKey(statement)
	state := r.trailingExits[key]
	high, low, _ := currentBarPrices(scope)
	triggered := false
	triggerPrice := 0.0
	switch {
	case position.Direction == "LONG" && direction != "short":
		if !state.activated && high >= activationPrice {
			state.activated = true
			state.direction = "long"
			state.extreme = high
		}
		if state.activated {
			state.extreme = math.Max(state.extreme, high)
			state.stopPrice = state.extreme - offset
			triggered = low <= state.stopPrice
			triggerPrice = state.stopPrice
		}
	case position.Direction == "SHORT" && direction != "long":
		if !state.activated && low <= activationPrice {
			state.activated = true
			state.direction = "short"
			state.extreme = low
		}
		if state.activated {
			if state.extreme == 0 {
				state.extreme = low
			} else {
				state.extreme = math.Min(state.extreme, low)
			}
			state.stopPrice = state.extreme + offset
			triggered = high >= state.stopPrice
			triggerPrice = state.stopPrice
		}
	default:
		delete(r.trailingExits, key)
		return false, nil
	}
	r.trailingExits[key] = state
	if !triggered {
		return false, nil
	}
	r.internalLog("strategy.exit trailing stop triggered with closed-bar stop-first policy")
	quantityMode := strings.TrimSpace(statement.QuantityMode)
	if quantityMode == "" {
		quantityMode = "symbol_position_percent"
	}
	mode, ok := indicatorbinding.ParseQuantityMode(quantityMode)
	if !ok {
		return false, fmt.Errorf("pine line %d: unsupported exit quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}
	quantityExpr := strings.TrimSpace(statement.QuantityExpression)
	if quantityExpr == "" {
		quantityExpr = "100"
	}
	quantity, err := r.resolveOrderQuantity(&strategyir.OrderStmt{
		Range:              statement.Range,
		Intent:             strategyir.OrderIntentClose,
		QuantityMode:       mode,
		QuantityExpression: quantityExpr,
	}, scope, position, availableQuantity, triggerPrice, mode)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if quantity <= 0 {
		return false, nil
	}
	if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
		r.internalLog("trailing exit suppressed during warmup")
		return true, nil
	}
	side := types.SideTypeSell
	if position.Direction == "SHORT" {
		side = types.SideTypeBuy
	}
	if err := r.submitOrder(side, types.OrderTypeMarket, quantity, 0); err != nil {
		return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	delete(r.trailingExits, key)
	r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
	r.resetEntrySubmitCount(position.Direction)
	return true, nil
}

func trailingExitKey(statement *strategyir.ExitStmt) string {
	if statement == nil {
		return ""
	}
	return strings.TrimSpace(statement.ID) + "\x00" + strings.TrimSpace(statement.FromEntry)
}

func (r *strategyRuntime) marketTickSize() float64 {
	if r != nil && r.session != nil {
		if market, ok := r.session.Market(r.symbol); ok && !market.TickSize.IsZero() {
			return market.TickSize.Float64()
		}
	}
	return 0.01
}
