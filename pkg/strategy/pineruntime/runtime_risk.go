package pineruntime

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const pineRuntimeRiskEpsilon = 1e-9

func (r *strategyRuntime) syncRiskState(currentTime time.Time) {
	if r == nil || currentTime.IsZero() {
		return
	}
	dayStart := r.marketDayStart(currentTime)
	if r.riskState.currentDayStart.IsZero() || !dayStart.Equal(r.riskState.currentDayStart) {
		r.finalizePreviousTradingDay()
		r.riskState.currentDayStart = dayStart
		r.riskState.currentDayStartEquity = 0
		r.riskState.currentDayLatestEquity = 0
		r.riskState.dailyFilledOrders = 0
		r.riskState.intradayLossTriggered = false
	}
	equity := r.getTotalAccountValue()
	if equity <= 0 {
		return
	}
	if r.riskState.peakEquity <= 0 || equity > r.riskState.peakEquity {
		r.riskState.peakEquity = equity
	}
	if r.riskState.currentDayStartEquity <= 0 {
		r.riskState.currentDayStartEquity = equity
	}
	r.riskState.currentDayLatestEquity = equity
	if r.maxDrawdownValue > 0 &&
		!r.riskState.drawdownTriggered &&
		pineRiskThresholdReached(r.riskState.peakEquity-equity, r.riskState.peakEquity, r.maxDrawdownValue, r.maxDrawdownType) {
		r.riskState.drawdownTriggered = true
		r.emitRiskTrigger("strategy.risk.max_drawdown", r.maxDrawdownAlert, fmt.Sprintf("equity=%0.4f peak=%0.4f", equity, r.riskState.peakEquity))
	}
	if r.maxIntradayLossValue > 0 &&
		!r.riskState.intradayLossTriggered &&
		pineRiskThresholdReached(r.riskState.currentDayStartEquity-equity, r.riskState.currentDayStartEquity, r.maxIntradayLossValue, r.maxIntradayLossType) {
		r.riskState.intradayLossTriggered = true
		r.emitRiskTrigger("strategy.risk.max_intraday_loss", r.maxIntradayLossAlert, fmt.Sprintf("equity=%0.4f dayStart=%0.4f", equity, r.riskState.currentDayStartEquity))
	}
}

func (r *strategyRuntime) recordFilledOrder(currentTime time.Time) {
	if r == nil {
		return
	}
	r.syncRiskState(currentTime)
	r.riskState.dailyFilledOrders++
	if r.maxIntradayFilledOrders > 0 && r.riskState.dailyFilledOrders == r.maxIntradayFilledOrders {
		r.emitRiskTrigger(
			"strategy.risk.max_intraday_filled_orders",
			r.maxIntradayFilledOrdersAlert,
			fmt.Sprintf("filledOrders=%d", r.riskState.dailyFilledOrders),
		)
	}
}

func (r *strategyRuntime) finalizePreviousTradingDay() {
	if r == nil || r.riskState.currentDayStart.IsZero() {
		return
	}
	if r.maxConsLossDays <= 0 {
		return
	}
	startEquity := r.riskState.currentDayStartEquity
	endEquity := r.riskState.currentDayLatestEquity
	if startEquity <= 0 || endEquity <= 0 {
		return
	}
	if endEquity < startEquity-pineRuntimeRiskEpsilon {
		r.riskState.consecutiveLossDays++
	} else {
		r.riskState.consecutiveLossDays = 0
	}
	if !r.riskState.consLossDaysTriggered && r.riskState.consecutiveLossDays >= r.maxConsLossDays {
		r.riskState.consLossDaysTriggered = true
		r.emitRiskTrigger(
			"strategy.risk.max_cons_loss_days",
			r.maxConsLossDaysAlert,
			fmt.Sprintf("consecutiveLossDays=%d", r.riskState.consecutiveLossDays),
		)
	}
}

func (r *strategyRuntime) marketDayStart(currentTime time.Time) time.Time {
	if r == nil {
		return currentTime.UTC()
	}
	symbol := strings.TrimSpace(r.symbol)
	if start, ok := market.TradingDayBoundaryStart(symbol, currentTime, r.strategy != nil && r.strategy.UseExtendedHours); ok {
		return start
	}
	location := time.UTC
	if profile, ok := market.ProfileForSymbol(symbol); ok && profile.Location != nil {
		location = profile.Location
	}
	local := currentTime.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UTC()
}

func pineRiskThresholdReached(loss float64, baseline float64, threshold float64, amountType string) bool {
	if loss <= pineRuntimeRiskEpsilon || threshold <= 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(amountType)) {
	case "cash":
		return loss+pineRuntimeRiskEpsilon >= threshold
	case "percent_of_equity":
		return baseline > 0 && (loss*100)+pineRuntimeRiskEpsilon >= baseline*threshold
	default:
		return false
	}
}

func (r *strategyRuntime) enforceOrderRiskBoundaries(
	action strategyir.OrderAction,
	intent strategyir.OrderIntent,
	position *positionSnapshot,
	quantity float64,
) (float64, bool) {
	if r == nil || quantity <= 0 {
		return quantity, false
	}
	quantity = r.capOrderQuantityToMaxPosition(action, intent, position, quantity)
	if quantity <= 0 {
		return 0, true
	}
	if blocked, reason := r.shouldBlockRiskIncreasingOrder(action, intent, position, quantity); blocked {
		r.internalLog(reason)
		return 0, true
	}
	return quantity, false
}

func (r *strategyRuntime) capOrderQuantityToMaxPosition(
	action strategyir.OrderAction,
	intent strategyir.OrderIntent,
	position *positionSnapshot,
	quantity float64,
) float64 {
	if r == nil || r.maxPositionSize <= 0 || quantity <= 0 {
		return quantity
	}
	if intent == strategyir.OrderIntentClose || intent == strategyir.OrderIntentFlatten {
		return quantity
	}
	currentQuantity := 0.0
	if position != nil {
		currentQuantity = position.Quantity
	}
	nextQuantity := projectedPositionQuantity(currentQuantity, action, quantity)
	if math.Abs(nextQuantity) <= r.maxPositionSize+pineRuntimeRiskEpsilon || math.Abs(nextQuantity) <= math.Abs(currentQuantity)+pineRuntimeRiskEpsilon {
		return quantity
	}
	cappedQuantity := nextQuantity
	if cappedQuantity > r.maxPositionSize {
		cappedQuantity = r.maxPositionSize
	}
	if cappedQuantity < -r.maxPositionSize {
		cappedQuantity = -r.maxPositionSize
	}
	allowedDelta := math.Floor(math.Abs(cappedQuantity - currentQuantity))
	if allowedDelta <= 0 {
		r.internalLog("order blocked by strategy.risk.max_position_size")
		return 0
	}
	if allowedDelta < quantity {
		r.internalLog(fmt.Sprintf("order quantity reduced by strategy.risk.max_position_size to %.0f", allowedDelta))
		return allowedDelta
	}
	return quantity
}

func (r *strategyRuntime) shouldBlockRiskIncreasingOrder(
	action strategyir.OrderAction,
	intent strategyir.OrderIntent,
	position *positionSnapshot,
	quantity float64,
) (bool, string) {
	if r == nil || quantity <= 0 {
		return false, ""
	}
	if intent == strategyir.OrderIntentClose || intent == strategyir.OrderIntentFlatten {
		return false, ""
	}
	currentQuantity := 0.0
	if position != nil {
		currentQuantity = position.Quantity
	}
	nextQuantity := projectedPositionQuantity(currentQuantity, action, quantity)
	if math.Abs(nextQuantity) <= math.Abs(currentQuantity)+pineRuntimeRiskEpsilon {
		return false, ""
	}
	switch {
	case r.maxIntradayFilledOrders > 0 && r.riskState.dailyFilledOrders >= r.maxIntradayFilledOrders:
		return true, "order blocked by strategy.risk.max_intraday_filled_orders"
	case r.riskState.drawdownTriggered:
		return true, "order blocked by strategy.risk.max_drawdown"
	case r.riskState.intradayLossTriggered:
		return true, "order blocked by strategy.risk.max_intraday_loss"
	case r.riskState.consLossDaysTriggered:
		return true, "order blocked by strategy.risk.max_cons_loss_days"
	default:
		return false, ""
	}
}

func projectedPositionQuantity(currentQuantity float64, action strategyir.OrderAction, quantity float64) float64 {
	switch action {
	case strategyir.OrderActionBuy, strategyir.OrderActionCover:
		return currentQuantity + quantity
	case strategyir.OrderActionSell, strategyir.OrderActionShort:
		return currentQuantity - quantity
	default:
		return currentQuantity
	}
}

func (r *strategyRuntime) emitRiskTrigger(name string, alert string, detail string) {
	if r == nil {
		return
	}
	message := strings.TrimSpace(name)
	if detail = strings.TrimSpace(detail); detail != "" {
		message += " triggered: " + detail
	} else {
		message += " triggered"
	}
	r.internalLog(message)
	if trimmed := strings.TrimSpace(alert); trimmed != "" {
		r.notify(trimmed)
	}
}
