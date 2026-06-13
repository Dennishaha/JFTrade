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
		return false, nil
	}
	availableQuantity := math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	if availableQuantity <= 0 {
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
