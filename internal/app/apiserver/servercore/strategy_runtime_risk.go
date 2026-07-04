package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	"github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

type strategyRuntimeRiskDecision = runtimecontrol.RiskDecision

func (e *strategyLiveOrderExecutor) evaluateRuntimeRisk(command trdsrv.ExecutionOrderCommand) strategyRuntimeRiskDecision {
	settings := e.currentRuntimeRiskSettings()
	context := runtimecontrol.RiskContext{}
	if e != nil && e.manager != nil {
		context.TodaySubmittedOrderCount = e.manager.todaySubmittedOrderCount(e.instance.ID, command.Symbol, time.Now().UTC())
	}
	if e != nil && e.runner != nil {
		context.CurrentPrice = e.runner.currentPrice()
		context.SellableQuantity = e.runner.sellableQuantity(command.Symbol)
	}
	return runtimecontrol.EvaluateRisk(
		strategyRuntimeRiskSettingsToControl(settings),
		strategyRuntimeOrderIntentFromCommand(command),
		context,
	)
}

func strategyRuntimeOrderIntentFromCommand(command trdsrv.ExecutionOrderCommand) runtimecontrol.OrderIntent {
	return runtimecontrol.OrderIntent{
		Symbol:   command.Symbol,
		Side:     command.Side,
		Quantity: command.Query.Quantity,
		Price:    command.Query.Price,
	}
}

func strategyRuntimeRiskSettingsToControl(input strategyRuntimeRiskSettings) runtimecontrol.RiskSettings {
	return runtimecontrol.RiskSettings{
		Mode:             input.Mode,
		CloseOnly:        input.CloseOnly,
		MaxOrderQuantity: input.MaxOrderQuantity,
		MaxOrderNotional: input.MaxOrderNotional,
		DailyMaxOrders:   input.DailyMaxOrders,
		PauseOnReject:    input.PauseOnReject,
	}
}

func (e *strategyLiveOrderExecutor) currentRuntimeRiskSettings() strategyRuntimeRiskSettings {
	if e == nil {
		return normalizeStrategyRuntimeRiskSettings(strategyRuntimeRiskSettings{})
	}
	if e.manager != nil {
		if instance, ok := e.manager.currentInstance(e.instance.ID); ok {
			return normalizeStrategyRuntimeRiskSettings(instance.Binding.RuntimeRisk)
		}
	}
	return normalizeStrategyRuntimeRiskSettings(e.instance.Binding.RuntimeRisk)
}

func (e *strategyLiveOrderExecutor) recordRuntimeRiskDecision(decision strategyRuntimeRiskDecision, command trdsrv.ExecutionOrderCommand) {
	if !decision.Matched || e == nil || e.manager == nil {
		return
	}
	if decision.Rejected {
		message := fmt.Sprintf("runtime risk rejected %s %s %s: %s", command.Symbol, command.Side, strategyRuntimeFormatNumber(command.Query.Quantity), decision.Reason)
		e.manager.recordError(e.instance.ID, message, time.Now().UTC())
		jftradeErr2 := e.manager.appendRuntimeEvent(e.instance.ID, message, "risk_rejected", decision.Detail)
		jftradeLogError(jftradeErr2)
		if decision.PauseOnReject {
			e.manager.stopStrategy(e.instance.ID)
			jftradeErr3 := e.manager.transitionInstance(e.instance.ID, strategyStatusPaused, "paused", "runtime risk rejected order with pauseOnReject")
			jftradeLogError(jftradeErr3)
		}
		return
	}
	message := fmt.Sprintf("runtime risk monitor matched %s %s %s: %s", command.Symbol, command.Side, strategyRuntimeFormatNumber(command.Query.Quantity), decision.Reason)
	jftradeErr1 := e.manager.appendRuntimeEvent(e.instance.ID, message, "risk_monitor", decision.Detail)
	jftradeLogError(jftradeErr1)
}

func (r *strategySymbolRuntime) sellableQuantity(symbol string) float64 {
	if r == nil {
		return 0
	}
	return runtimecontrol.SellableQuantity(strategyRuntimePositionsToControl(cloneStrategyRuntimePositions(r.cachedPositions)), symbol)
}

func (m *strategyRuntimeManager) todaySubmittedOrderCount(instanceID string, symbol string, now time.Time) int {
	if m == nil || m.deps.countRuntimeAudit == nil {
		return 0
	}
	fromAt := marketDayStartUTC(symbol, now)
	count, err := m.deps.countRuntimeAudit(context.Background(), runtimeactivity.AuditQuery{
		InstanceID: strings.TrimSpace(instanceID),
		Kind:       "order_submitted",
		FromAt:     &fromAt,
	})
	if err != nil {
		return 0
	}
	return count
}

func marketDayStartUTC(symbol string, now time.Time) time.Time {
	return runtimecontrol.MarketDayStartUTC(symbol, now)
}
