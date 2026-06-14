package servercore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// newTradingService 创建交易服务并注入运行期依赖。
func (s *Server) newTradingService() *trdsrv.Service {
	fallbackMarket := strings.ToUpper(strings.TrimSpace(s.store.Integration().Config.TradeMarket))
	if fallbackMarket == "" {
		fallbackMarket = "HK"
	}
	orderUpdates := trdsrv.NewOrderUpdatesWorker(
		&tradingOrderUpdateSource{server: s},
		&tradingExecutionOrderUpdates{server: s},
		trdsrv.OrderUpdatesConfig{
			BrokerID:       "futu",
			FallbackMarket: fallbackMarket,
			HistoryLookback: func() int {
				return s.store.ExecutionSettings().BrokerOrderHistoryLookbackDays
			},
		},
	)
	return trdsrv.NewService(
		trdsrv.WithActiveBroker(s.activeBroker),
		trdsrv.WithDefaultMarket(func() string {
			return s.store.Integration().Config.TradeMarket
		}),
		trdsrv.WithDefaultTradingEnvironment(s.defaultTradingEnvironment),
		trdsrv.WithBrokerRuntime(s.brokerRuntime),
		trdsrv.WithOrderUpdates(orderUpdates),
		trdsrv.WithListOrders(s.listExecutionOrders),
		trdsrv.WithPlaceOrder(s.placeExecutionOrder),
		trdsrv.WithCancelOrder(s.cancelExecutionOrder),
		trdsrv.WithGetOrderEvents(s.getExecutionOrderEvents),
	)
}

func (s *Server) defaultTradingEnvironment() string {
	if s == nil || s.store == nil {
		return "SIMULATE"
	}
	return s.store.ExecutionSettings().DefaultTradingEnvironment
}

func (s *Server) listExecutionOrders(ctx context.Context, filter trdsrv.ExecutionOrderFilter) (trdsrv.ExecutionOrders, error) {
	return s.executionOrders.listOrdersFiltered(filter), nil
}

func (s *Server) placeExecutionOrder(ctx context.Context, request trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	placed, err := s.brokerExecutionExchange().PlaceBrokerOrder(ctx, request.Query)
	if err != nil {
		return trdsrv.ExecutionOrder{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	payloadData := map[string]any{
		"operation": "PLACE", "brokerOrderId": placed.BrokerOrderID,
		"brokerOrderIdEx": placed.BrokerOrderIDEx, "tradingEnvironment": placed.TradingEnvironment,
		"accountId": placed.AccountID, "market": placed.Market, "symbol": request.Symbol,
		"side": request.Side, "orderType": request.OrderType,
		"requestedQuantity": request.Query.Quantity, "requestedPrice": request.Query.Price,
	}
	if request.Session != "" {
		payloadData["session"] = request.Session
	}
	record := s.executionOrders.recordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: request.BrokerID, BrokerOrderID: placed.BrokerOrderID,
		BrokerOrderIDEx:    derefString(placed.BrokerOrderIDEx),
		TradingEnvironment: placed.TradingEnvironment, AccountID: placed.AccountID,
		Market: placed.Market, Symbol: request.Symbol, Side: request.Side,
		OrderType: request.OrderType, Status: placed.Status,
		RequestedQuantity: request.Query.Quantity, RequestedPrice: request.Query.Price,
		Remark: request.Remark, SubmittedAt: now, Payload: payloadData,
		EventType: "COMMAND_PLACE_ACCEPTED",
	})
	s.notifyExecutionOrderPlaced(record)
	return record, nil
}

func (s *Server) cancelExecutionOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	internalOrderID = strings.TrimSpace(internalOrderID)
	order, ok := s.executionOrders.order(internalOrderID)
	if !ok {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order not found")
	}
	if order.BrokerOrderID == nil || strings.TrimSpace(*order.BrokerOrderID) == "" {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order is missing broker order id")
	}

	brokerOrderID, err := strconv.ParseUint(strings.TrimSpace(*order.BrokerOrderID), 10, 64)
	if err != nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order has invalid broker order id")
	}
	if order.Symbol == nil || strings.TrimSpace(*order.Symbol) == "" {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order is missing symbol")
	}

	err = s.activeBroker().Trading().CancelOrders(ctx, broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: order.TradingEnvironment,
		AccountID:          order.AccountID,
		Market:             order.Market,
	}, broker.CancelOrder{
		OrderID: brokerOrderID,
		Symbol:  *order.Symbol,
	})
	if err != nil {
		return trdsrv.ExecutionOrder{}, err
	}

	updatedOrder, _ := s.executionOrders.markCancelRequested(internalOrderID, map[string]any{
		"operation":       "CANCEL",
		"brokerOrderId":   *order.BrokerOrderID,
		"brokerOrderIdEx": order.BrokerOrderIDEx,
		"symbol":          order.Symbol,
	})
	return updatedOrder, nil
}

// getExecutionOrderEvents 获取订单事件。
func (s *Server) getExecutionOrderEvents(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrderEvents, error) {
	return s.executionOrders.orderEvents(strings.TrimSpace(internalOrderID)), nil
}
