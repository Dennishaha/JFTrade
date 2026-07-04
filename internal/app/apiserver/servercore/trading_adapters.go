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
		trdsrv.WithBrokerRuntimeProvider(&serverTradingBrokerRuntimeProvider{server: s}),
		trdsrv.WithDefaultMarket(func() string {
			return s.store.Integration().Config.TradeMarket
		}),
		trdsrv.WithDefaultTradingEnvironment(s.defaultTradingEnvironment),
		trdsrv.WithOrderUpdates(orderUpdates),
		trdsrv.WithPreTradeRiskGateway(s.preTradeRiskGateway),
		trdsrv.WithOrderStore(&serverTradingOrderStore{store: s.executionOrders}),
		trdsrv.WithOrderGateway(&serverTradingOrderGateway{server: s}),
	)
}

type serverTradingBrokerRuntimeProvider struct {
	server *Server
}

func (p *serverTradingBrokerRuntimeProvider) ActiveBroker() broker.Broker {
	return p.server.activeBroker()
}

func (p *serverTradingBrokerRuntimeProvider) Runtime(ctx context.Context) map[string]any {
	return p.server.brokerRuntime(ctx)
}

type serverTradingOrderStore struct {
	store *executionOrderStore
}

func (p *serverTradingOrderStore) ListOrders(_ context.Context, filter trdsrv.ExecutionOrderFilter) (trdsrv.ExecutionOrders, error) {
	return p.store.listOrdersFiltered(filter), nil
}

func (p *serverTradingOrderStore) OrderEvents(_ context.Context, internalOrderID string) (trdsrv.ExecutionOrderEvents, error) {
	return p.store.orderEvents(strings.TrimSpace(internalOrderID)), nil
}

type serverTradingOrderGateway struct {
	server *Server
}

func (p *serverTradingOrderGateway) PlaceOrder(ctx context.Context, request trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	return p.server.placeExecutionOrder(ctx, request)
}

func (p *serverTradingOrderGateway) CancelOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	return p.server.cancelExecutionOrder(ctx, internalOrderID)
}

var (
	_ trdsrv.BrokerRuntimeProvider = (*serverTradingBrokerRuntimeProvider)(nil)
	_ trdsrv.OrderStore            = (*serverTradingOrderStore)(nil)
	_ trdsrv.OrderGateway          = (*serverTradingOrderGateway)(nil)
)

func (s *Server) defaultTradingEnvironment() string {
	if s == nil || s.store == nil {
		return "SIMULATE"
	}
	return s.store.ExecutionSettings().DefaultTradingEnvironment
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
		"rawBrokerStatus": placed.Status,
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
	if trdsrv.IsCanonicalTerminalOrderStatus(order.Status) {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order is already terminal (%s)", order.Status)
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
