package pineruntime

import (
	"fmt"
	"math"
	"strings"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeProtectStatement(statement *strategyir.ProtectStmt, scope *evaluationScope) (bool, error) {
	requirement, err := r.resolveProtectRequirement(statement)
	if err != nil {
		return false, err
	}
	rawSnapshot, ok := scope.indicators[requirement.key]
	if !ok || rawSnapshot == nil {
		r.internalLog("waiting for indicator " + requirement.key)
		return false, nil
	}
	position := r.getPosition(r.symbol, scope.currentKlineTime)
	if position == nil {
		return false, nil
	}
	shouldExitLong := requirement.allowLongExit && position.Direction == "LONG" && readBool(rawSnapshot, "longTriggered")
	shouldExitShort := requirement.allowShortExit && position.Direction == "SHORT" && readBool(rawSnapshot, "shortTriggered")
	availableQuantity := math.Floor(absFloat(firstPositiveFloat(absFloat(position.AvailableQuantity), absFloat(position.Quantity))))
	quantity := 0.0
	if shouldExitLong || shouldExitShort {
		quantityMode := strings.TrimSpace(statement.QuantityMode)
		if quantityMode == "" {
			quantityMode = "symbol_position_percent"
		}
		quantityExpr := strings.TrimSpace(statement.QuantityExpression)
		if quantityExpr == "" {
			quantityExpr = "100"
		}
		mode, ok := indicatorbinding.ParseQuantityMode(quantityMode)
		if !ok {
			return false, fmt.Errorf("pine line %d: unsupported exit quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
		}
		closePrice := 0.0
		if scope.currentKline != nil {
			closePrice = scope.currentKline.Close.Float64()
		}
		quantity, err = r.resolveOrderQuantity(&strategyir.OrderStmt{
			Range:              statement.Range,
			Action:             strategyir.OrderActionSell,
			Intent:             strategyir.OrderIntentClose,
			QuantityMode:       mode,
			QuantityExpression: quantityExpr,
		}, scope, position, availableQuantity, closePrice, mode)
		if err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		if quantity <= 0 {
			return false, nil
		}
	}
	if shouldExitLong {
		if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
			r.internalLog("protect exit suppressed during warmup")
			return true, nil
		}
		if err := r.submitOrder(types.SideTypeSell, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("LONG")
		return true, nil
	}
	if shouldExitShort {
		if r.isPlaceBlockedDuringWarmup(scope.currentKlineTime) {
			r.internalLog("protect exit suppressed during warmup")
			return true, nil
		}
		if err := r.submitOrder(types.SideTypeBuy, types.OrderTypeMarket, quantity, 0); err != nil {
			return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		r.emitOrderMetadata(statement.Comment, statement.AlertMessage, statement.DisableAlert)
		r.resetEntrySubmitCount("SHORT")
		return true, nil
	}
	return false, nil
}

func (r *strategyRuntime) resolveProtectRequirement(statement *strategyir.ProtectStmt) (cachedProtectRequirement, error) {
	if cached, ok := r.protectCache[statement]; ok {
		return cached, cached.err
	}
	key, err := buildProtectRequirementKey(statement)
	cached := cachedProtectRequirement{key: key, err: err}
	if err == nil {
		direction := normalizeProtectDirection(statement.Direction)
		cached.allowLongExit = direction != "short"
		cached.allowShortExit = direction != "long"
	}
	r.protectCache[statement] = cached
	return cached, err
}
