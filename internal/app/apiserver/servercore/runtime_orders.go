package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func (e *strategyNotifyOnlyOrderExecutor) SubmitOrders(_ context.Context, orders ...bbgotypes.SubmitOrder) (bbgotypes.OrderSlice, error) {
	createdOrders := make(bbgotypes.OrderSlice, 0, len(orders))
	for _, order := range orders {
		e.manager.recordSignal(e.instance.ID, time.Now().UTC())
		message := e.describeOrderSignal(order)
		e.manager.recordNotification(strategyRuntimeNotification{
			At:       time.Now().UTC().Format(time.RFC3339Nano),
			Level:    "info",
			Title:    "策略下单信号",
			Message:  message,
			Source:   "strategy.runtime",
			BrokerID: strategyRuntimeBrokerID(e.instance.Binding),
			Category: "strategy.order.signal",
		})
		jftradeErr4 := e.manager.appendRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("notify-only signal %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
			"signal_notified",
			message,
		)
		besteffort.LogError(jftradeErr4)
		createdOrders = append(createdOrders, bbgotypes.Order{SubmitOrder: order})
	}
	return createdOrders, nil
}

func (e *strategyNotifyOnlyOrderExecutor) CancelOrders(context.Context, ...bbgotypes.Order) error {
	return nil
}

func (e *strategyNotifyOnlyOrderExecutor) describeOrderSignal(order bbgotypes.SubmitOrder) string {
	marketPrice := e.runner.currentPrice()
	preparedPrice := marketPrice
	if order.Price.Sign() > 0 {
		preparedPrice = order.Price.Float64()
	}
	return fmt.Sprintf(
		"%s / %s: %s %s 股，预备下单价格 %s，当时市价 %s，仅通知模式",
		strategyRuntimeDisplayName(e.instance, e.runner),
		order.Symbol,
		strategyRuntimeSideLabel(order.Side),
		strategyRuntimeFormatNumber(order.Quantity.Float64()),
		strategyRuntimeFormatPrice(preparedPrice),
		strategyRuntimeFormatPrice(marketPrice),
	)
}

func (e *strategyLiveOrderExecutor) SubmitOrders(ctx context.Context, orders ...bbgotypes.SubmitOrder) (bbgotypes.OrderSlice, error) {
	placedOrders := make(bbgotypes.OrderSlice, 0, len(orders))
	for _, order := range orders {
		e.manager.recordSignal(e.instance.ID, time.Now().UTC())

		placeQuery := strategyRuntimeBrokerPlaceOrderQuery(e.instance.Binding, order.Symbol)
		placeQuery.Side = strings.ToUpper(string(order.Side))
		placeQuery.OrderType = strings.ToUpper(string(order.Type))
		placeQuery.Quantity = order.Quantity.Float64()
		if order.Price.Sign() > 0 {
			placeQuery.Price = new(order.Price.Float64())
		}
		if order.StopPrice.Sign() > 0 {
			placeQuery.StopPrice = new(order.StopPrice.Float64())
		}
		timeInForce := strings.ToUpper(string(order.TimeInForce))
		if timeInForce == "" {
			timeInForce = "DAY"
		}
		placeQuery.TimeInForce = &timeInForce
		remark := fmt.Sprintf("strategy runtime %s", e.instance.ID)
		placeQuery.Remark = &remark
		if order.ClientOrderID != "" {
			placeQuery.ClientOrderID = order.ClientOrderID
		}

		command := trdsrv.ExecutionOrderCommand{
			BrokerID:  strategyRuntimeBrokerID(e.instance.Binding),
			Query:     placeQuery,
			Symbol:    order.Symbol,
			Side:      strings.ToUpper(string(order.Side)),
			OrderType: strings.ToUpper(string(order.Type)),
			Remark:    remark,
		}
		decision := e.evaluateRuntimeRisk(command)
		e.recordRuntimeRiskDecision(decision, command)
		if decision.Rejected {
			return placedOrders, fmt.Errorf("runtime risk rejected order: %s", decision.Reason)
		}

		placed, err := e.manager.placeExecutionOrder(ctx, command)
		if err != nil {
			e.manager.recordError(e.instance.ID, err.Error(), time.Now().UTC())
			jftradeErr5 := e.manager.appendRuntimeEvent(
				e.instance.ID,
				fmt.Sprintf("live order failed %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
				"order_submit_failed",
				err.Error(),
			)
			besteffort.LogError(jftradeErr5)
			return placedOrders, err
		}
		e.manager.recordOrder(e.instance.ID, time.Now().UTC())
		jftradeErr6 := e.manager.appendRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("live order submitted %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
			"order_submitted",
			fmt.Sprintf("internalOrderId=%s", placed.InternalOrderID),
		)
		besteffort.LogError(jftradeErr6)
		e.trackOrder(order.ClientOrderID, placed.InternalOrderID)
		placedOrders = append(placedOrders, bbgotypes.Order{SubmitOrder: order})
	}
	return placedOrders, nil
}

func (e *strategyLiveOrderExecutor) CancelOrders(ctx context.Context, orders ...bbgotypes.Order) error {
	for _, order := range orders {
		clientOrderID := strings.TrimSpace(order.ClientOrderID)
		if clientOrderID == "" {
			continue
		}
		internalOrderID, ok := e.trackedInternalOrderID(clientOrderID)
		if !ok {
			continue
		}
		cancelled, err := e.manager.cancelExecutionOrder(ctx, internalOrderID)
		if err != nil {
			e.manager.recordError(e.instance.ID, err.Error(), time.Now().UTC())
			jftradeErr := e.manager.appendRuntimeEvent(
				e.instance.ID,
				fmt.Sprintf("live order cancel failed %s", clientOrderID),
				"order_cancel_failed",
				err.Error(),
			)
			besteffort.LogError(jftradeErr)
			return err
		}
		e.untrackOrder(clientOrderID)
		jftradeErr := e.manager.appendRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("live order cancel requested %s", clientOrderID),
			"order_cancel_requested",
			fmt.Sprintf("internalOrderId=%s", cancelled.InternalOrderID),
		)
		besteffort.LogError(jftradeErr)
	}
	return nil
}

func (e *strategyLiveOrderExecutor) trackOrder(clientOrderID string, internalOrderID string) {
	clientOrderID = strings.TrimSpace(clientOrderID)
	internalOrderID = strings.TrimSpace(internalOrderID)
	if clientOrderID == "" || internalOrderID == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.trackedInternalOrderIDs == nil {
		e.trackedInternalOrderIDs = map[string]string{}
	}
	e.trackedInternalOrderIDs[clientOrderID] = internalOrderID
}

func (e *strategyLiveOrderExecutor) trackedInternalOrderID(clientOrderID string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	internalOrderID, ok := e.trackedInternalOrderIDs[strings.TrimSpace(clientOrderID)]
	return internalOrderID, ok
}

func (e *strategyLiveOrderExecutor) untrackOrder(clientOrderID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.trackedInternalOrderIDs, strings.TrimSpace(clientOrderID))
}
