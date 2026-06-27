package pineruntime

import (
	"fmt"
	"strings"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) resolveKLineSession(kline types.KLine) market.Session {
	if r.session != nil && r.session.Exchange != nil {
		if resolver, ok := r.session.Exchange.(klineSessionResolver); ok {
			if session, ok := resolver.ResolveKLineSession(kline); ok {
				return session
			}
		}
	}
	strategySymbol := ""
	if r.strategy != nil {
		strategySymbol = r.strategy.Symbol
	}
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(strategySymbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return market.SessionUnknown
	}
	return market.ClassifySession(resolvedSymbol, observedAt)
}

func (r *strategyRuntime) isPlaceBlockedDuringWarmup(currentKlineTime time.Time) bool {
	return !r.strategy.WarmupUntil.IsZero() && !currentKlineTime.IsZero() && currentKlineTime.Before(r.strategy.WarmupUntil)
}

func (r *strategyRuntime) log(message string) {
	bbgo2.Notify("pine strategy %s: %s", r.displayName, strings.TrimSpace(message))
}

func (r *strategyRuntime) internalLog(message string) {
	if bbgo2.IsBackTesting {
		return
	}
	r.log(message)
}

func (r *strategyRuntime) notify(message string) {
	bbgo2.Notify("pine strategy %s: %s", r.displayName, strings.TrimSpace(message))
}

func (r *strategyRuntime) cachedPosition(symbol string, barTime time.Time) (*positionSnapshot, bool) {
	if r == nil || !r.positionCache.valid {
		return nil, false
	}
	if r.positionCache.symbol != symbol || !r.positionCache.barTime.Equal(barTime) {
		return nil, false
	}
	return r.positionCache.value, true
}

func (r *strategyRuntime) storeCachedPosition(symbol string, barTime time.Time, snapshot *positionSnapshot) {
	if r == nil {
		return
	}
	r.positionCache = cachedPositionSnapshot{
		barTime: barTime,
		symbol:  symbol,
		value:   snapshot,
		valid:   true,
	}
}

func (r *strategyRuntime) clearPositionCache() {
	if r == nil {
		return
	}
	r.positionCache = cachedPositionSnapshot{}
}

func normalizeRuntimePyramiding(program *strategyir.Program) int {
	if program == nil || program.Metadata.Pyramiding <= 0 {
		return 1
	}
	return program.Metadata.Pyramiding
}

func normalizeRuntimeAllowedEntryDirection(program *strategyir.Program) string {
	if program == nil {
		return "all"
	}
	switch strings.ToLower(strings.TrimSpace(program.Metadata.AllowedEntryDirection)) {
	case "long":
		return "long"
	case "short":
		return "short"
	default:
		return "all"
	}
}

func normalizeRuntimeRiskAmountType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "percent_of_equity", "cash":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeRuntimePositiveFloat(value float64) float64 {
	if value > 0 {
		return value
	}
	return 0
}

func normalizeRuntimePositiveInt(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func shouldExecuteWhenExpression(lineNumber int, expression string, scope *evaluationScope) (bool, error) {
	if strings.TrimSpace(expression) == "" {
		return true, nil
	}
	allowed, err := evaluateBoolExpression(expression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: %w", lineNumber, err)
	}
	return allowed, nil
}

func (r *strategyRuntime) applySyntheticPositionBasis(symbol string, snapshot *positionSnapshot) {
	if r == nil || snapshot == nil || snapshot.AveragePrice > 0 || snapshot.Quantity == 0 || r.syntheticPositionBases == nil {
		return
	}
	basis, ok := r.syntheticPositionBases[strings.ToUpper(strings.TrimSpace(symbol))]
	if !ok || basis.AveragePrice <= 0 || basis.Quantity == 0 {
		return
	}
	if snapshot.Quantity > 0 && basis.Quantity > 0 || snapshot.Quantity < 0 && basis.Quantity < 0 {
		snapshot.AveragePrice = basis.AveragePrice
	}
}

func (r *strategyRuntime) recordSyntheticPositionFill(
	symbol string,
	action strategyir.OrderAction,
	quantity float64,
	price float64,
	priorPosition *positionSnapshot,
) {
	if r == nil || quantity <= 0 || price <= 0 {
		return
	}
	if r.syntheticPositionBases == nil {
		r.syntheticPositionBases = map[string]syntheticPositionBasis{}
	}
	key := strings.ToUpper(strings.TrimSpace(symbol))
	currentQuantity := 0.0
	currentAverage := 0.0
	if priorPosition != nil {
		currentQuantity = priorPosition.Quantity
		currentAverage = priorPosition.AveragePrice
	}
	if currentAverage <= 0 {
		if basis, ok := r.syntheticPositionBases[key]; ok {
			if currentQuantity > 0 && basis.Quantity > 0 || currentQuantity < 0 && basis.Quantity < 0 {
				currentAverage = basis.AveragePrice
			}
		}
	}
	delta := 0.0
	switch action {
	case strategyir.OrderActionBuy, strategyir.OrderActionCover:
		delta = quantity
	case strategyir.OrderActionSell, strategyir.OrderActionShort:
		delta = -quantity
	default:
		return
	}
	nextQuantity := currentQuantity + delta
	switch {
	case absFloat(nextQuantity) <= pineRuntimeRiskEpsilon:
		delete(r.syntheticPositionBases, key)
	case absFloat(currentQuantity) <= pineRuntimeRiskEpsilon || currentQuantity*delta > 0:
		weightedNotional := absFloat(currentQuantity)*currentAverage + absFloat(delta)*price
		r.syntheticPositionBases[key] = syntheticPositionBasis{
			Quantity:     nextQuantity,
			AveragePrice: weightedNotional / absFloat(nextQuantity),
		}
	case absFloat(delta) < absFloat(currentQuantity):
		r.syntheticPositionBases[key] = syntheticPositionBasis{
			Quantity:     nextQuantity,
			AveragePrice: currentAverage,
		}
	default:
		r.syntheticPositionBases[key] = syntheticPositionBasis{
			Quantity:     nextQuantity,
			AveragePrice: price,
		}
	}
}
