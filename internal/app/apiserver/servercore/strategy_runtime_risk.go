package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

type strategyRuntimeRiskDecision struct {
	matched       bool
	rejected      bool
	pauseOnReject bool
	reason        string
	detail        string
}

func (e *strategyLiveOrderExecutor) evaluateRuntimeRisk(command trdsrv.ExecutionOrderCommand) strategyRuntimeRiskDecision {
	settings := e.currentRuntimeRiskSettings()
	if settings.Mode == "off" {
		return strategyRuntimeRiskDecision{}
	}

	reason := e.runtimeRiskRejectReason(settings, command)
	if reason == "" {
		return strategyRuntimeRiskDecision{}
	}

	detail := fmt.Sprintf(
		"rule=%s symbol=%s side=%s qty=%s mode=%s closeOnly=%t maxQty=%s maxNotional=%s dailyMaxOrders=%s",
		reason,
		command.Symbol,
		command.Side,
		strategyRuntimeFormatNumber(command.Query.Quantity),
		settings.Mode,
		settings.CloseOnly,
		strategyRuntimeOptionalFloatLabel(settings.MaxOrderQuantity),
		strategyRuntimeOptionalFloatLabel(settings.MaxOrderNotional),
		strategyRuntimeOptionalIntLabel(settings.DailyMaxOrders),
	)
	return strategyRuntimeRiskDecision{
		matched:       true,
		rejected:      settings.Mode == "enforce",
		pauseOnReject: settings.PauseOnReject,
		reason:        reason,
		detail:        detail,
	}
}

func (e *strategyLiveOrderExecutor) currentRuntimeRiskSettings() strategyRuntimeRiskSettings {
	if e == nil {
		return normalizeStrategyRuntimeRiskSettings(strategyRuntimeRiskSettings{})
	}
	if e != nil && e.server != nil && e.server.strategyStore != nil {
		if instance, ok := e.server.strategyStore.strategy(e.instance.ID); ok {
			return normalizeStrategyRuntimeRiskSettings(instance.Binding.RuntimeRisk)
		}
	}
	return normalizeStrategyRuntimeRiskSettings(e.instance.Binding.RuntimeRisk)
}

func (e *strategyLiveOrderExecutor) runtimeRiskRejectReason(settings strategyRuntimeRiskSettings, command trdsrv.ExecutionOrderCommand) string {
	side := strings.ToUpper(strings.TrimSpace(command.Side))
	if settings.CloseOnly {
		if side != "SELL" {
			return "close_only"
		}
		if sellable := e.runner.sellableQuantity(command.Symbol); command.Query.Quantity > sellable+0.0000001 {
			return "close_only_insufficient_position"
		}
	}
	if settings.MaxOrderQuantity != nil && command.Query.Quantity > *settings.MaxOrderQuantity {
		return "max_order_quantity"
	}
	if settings.MaxOrderNotional != nil {
		price := 0.0
		if command.Query.Price != nil {
			price = *command.Query.Price
		}
		if price <= 0 && e.runner != nil {
			price = e.runner.currentPrice()
		}
		if price <= 0 {
			return "max_order_notional_missing_price"
		}
		if command.Query.Quantity*price > *settings.MaxOrderNotional {
			return "max_order_notional"
		}
	}
	if settings.DailyMaxOrders != nil && e.manager.todaySubmittedOrderCount(e.instance.ID, time.Now().UTC()) >= *settings.DailyMaxOrders {
		return "daily_max_orders"
	}
	return ""
}

func (e *strategyLiveOrderExecutor) recordRuntimeRiskDecision(decision strategyRuntimeRiskDecision, command trdsrv.ExecutionOrderCommand) {
	if !decision.matched {
		return
	}
	if decision.rejected {
		message := fmt.Sprintf("runtime risk rejected %s %s %s: %s", command.Symbol, command.Side, strategyRuntimeFormatNumber(command.Query.Quantity), decision.reason)
		e.manager.recordError(e.instance.ID, message, time.Now().UTC())
		jftradeErr2 := e.server.strategyStore.appendStrategyRuntimeEvent(e.instance.ID, message, "risk_rejected", decision.detail)
		jftradeLogError(jftradeErr2)
		if decision.pauseOnReject {
			e.manager.stopStrategy(e.instance.ID)
			_, jftradeErr3 := e.server.strategyStore.transitionStrategy(e.instance.ID, strategyStatusPaused, "paused", "runtime risk rejected order with pauseOnReject")
			jftradeLogError(jftradeErr3)
		}
		return
	}
	message := fmt.Sprintf("runtime risk monitor matched %s %s %s: %s", command.Symbol, command.Side, strategyRuntimeFormatNumber(command.Query.Quantity), decision.reason)
	jftradeErr1 := e.server.strategyStore.appendStrategyRuntimeEvent(e.instance.ID, message, "risk_monitor", decision.detail)
	jftradeLogError(jftradeErr1)
}

func (r *strategySymbolRuntime) sellableQuantity(symbol string) float64 {
	if r == nil {
		return 0
	}
	positions := cloneStrategyRuntimePositions(r.cachedPositions)
	total := 0.0
	for _, position := range positions {
		if strategyRuntimePositionMatchesSymbol(position, symbol) {
			total += position.SellableQuantity
		}
	}
	return total
}

func (m *strategyRuntimeManager) todaySubmittedOrderCount(instanceID string, now time.Time) int {
	if m == nil || m.server == nil || m.server.strategyRuntimeStore == nil {
		return 0
	}
	count, err := m.server.strategyRuntimeStore.CountAudit(context.Background(), strategyRuntimeAuditQuery{
		InstanceID: strings.TrimSpace(instanceID),
		Kind:       "order_submitted",
		FromAt:     new(time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		return 0
	}
	return count
}

func strategyRuntimeOptionalFloatLabel(value *float64) string {
	if value == nil {
		return "none"
	}
	return strategyRuntimeFormatNumber(*value)
}

func strategyRuntimeOptionalIntLabel(value *int) string {
	if value == nil {
		return "none"
	}
	return fmt.Sprintf("%d", *value)
}
