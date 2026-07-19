package servercore

import (
	"context"
	"encoding/json"
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
		trdsrv.WithExecutionPreviewStore(&serverTradingOrderStore{store: s.executionOrders}),
		trdsrv.WithPredictionQuoteStore(&serverTradingOrderStore{store: s.executionOrders}),
		trdsrv.WithOrderGateway(&serverTradingOrderGateway{server: s}),
		trdsrv.WithComboOrderGateway(&serverTradingOrderGateway{server: s}),
	)
}

type serverTradingBrokerRuntimeProvider struct {
	server *Server
}

func (p *serverTradingBrokerRuntimeProvider) ActiveBroker() broker.Broker {
	return p.server.activeBroker()
}

func (p *serverTradingBrokerRuntimeProvider) ResolveBroker(id string) broker.Broker {
	return p.server.resolveBroker(id)
}

func (p *serverTradingBrokerRuntimeProvider) Runtime(ctx context.Context) *trdsrv.BrokerRuntimeResponse {
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

func (p *serverTradingOrderStore) SavePreview(record trdsrv.ExecutionPreviewRecord) error {
	if p == nil || p.store == nil || p.store.persistence == nil {
		return nil
	}
	return p.store.persistence.savePreview(record)
}

func (p *serverTradingOrderStore) ConsumePreview(
	previewID, brokerID, accountID, requestHash, clientOrderID string,
) error {
	if p == nil || p.store == nil || p.store.persistence == nil {
		return nil
	}
	return p.store.persistence.consumePreview(previewID, brokerID, accountID, requestHash, clientOrderID)
}

func (p *serverTradingOrderStore) SavePredictionQuote(
	_ context.Context,
	record broker.PredictionQuoteRecord,
) error {
	if p == nil || p.store == nil || p.store.persistence == nil {
		return fmt.Errorf("prediction quote persistence is unavailable")
	}
	return p.store.persistence.savePredictionQuote(record)
}

func (p *serverTradingOrderStore) ValidatePredictionQuote(
	_ context.Context,
	quoteID, brokerID, accountID, environment, mvc, legsHash string,
) (broker.PredictionQuoteRecord, error) {
	if p == nil || p.store == nil || p.store.persistence == nil {
		return broker.PredictionQuoteRecord{}, fmt.Errorf("prediction quote persistence is unavailable")
	}
	return p.store.persistence.predictionQuote(
		quoteID, brokerID, accountID, environment, mvc, legsHash,
	)
}

func (p *serverTradingOrderStore) ConsumePredictionQuote(
	_ context.Context,
	quoteID, brokerID, accountID, environment, mvc, legsHash, previewID, clientOrderID string,
) error {
	if p == nil || p.store == nil || p.store.persistence == nil {
		return fmt.Errorf("prediction quote persistence is unavailable")
	}
	return p.store.persistence.consumePredictionQuote(
		quoteID, brokerID, accountID, environment, mvc, legsHash, previewID, clientOrderID,
	)
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
	_ trdsrv.ExecutionPreviewStore = (*serverTradingOrderStore)(nil)
	_ broker.PredictionQuoteStore  = (*serverTradingOrderStore)(nil)
	_ trdsrv.OrderGateway          = (*serverTradingOrderGateway)(nil)
	_ trdsrv.ComboOrderGateway     = (*serverTradingOrderGateway)(nil)
)

func (p *serverTradingOrderGateway) PlaceCombo(ctx context.Context, intent broker.ComboOrderIntent) (trdsrv.ExecutionOrder, error) {
	selected := p.server.resolveBroker(intent.BrokerID)
	service, ok := selected.(broker.ComboTradingService)
	if !ok {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("broker %q combo trading service is unavailable", intent.BrokerID)
	}
	symbol := ""
	side := ""
	quantity := 0.0
	if len(intent.Legs) > 0 {
		symbol = intent.Legs[0].InstrumentID
		side = intent.Legs[0].Side
		if intent.Legs[0].Quantity != nil {
			quantity = *intent.Legs[0].Quantity
		}
	}
	prepared, fresh, err := p.server.executionOrders.prepareSubmission(trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: intent.BrokerID, TradingEnvironment: intent.TradingEnvironment,
		AccountID: intent.AccountID, Market: intent.Market, Symbol: symbol, Side: side,
		OrderType: "COMBO", RequestedQuantity: quantity, RequestedPrice: intent.Price,
		RequestedAmount: intent.Amount, OrderKind: intent.OrderKind, ProductClass: intent.ProductClass,
		QuantityMode: comboOrderQuantityMode(intent.OrderKind), ClientOrderID: intent.ClientOrderID,
		PreviewID: intent.PreviewID, NormalizedRequest: normalizedBrokerComboIntent(intent), Legs: intent.Legs,
	})
	if err != nil {
		return trdsrv.ExecutionOrder{}, err
	}
	if !fresh {
		return prepared, nil
	}
	placed, err := service.PlaceComboOrder(ctx, intent)
	if err != nil {
		p.server.executionOrders.markSubmissionUnknown(prepared.InternalOrderID, err)
		return trdsrv.ExecutionOrder{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := p.server.executionOrders.recordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		InternalOrderID: prepared.InternalOrderID, BrokerID: intent.BrokerID, BrokerOrderID: placed.BrokerOrderID,
		BrokerOrderIDEx:    placed.BrokerOrderID,
		TradingEnvironment: intent.TradingEnvironment, AccountID: intent.AccountID,
		Market: intent.Market, Symbol: symbol, Side: side, OrderType: "COMBO",
		Status: placed.Status, RequestedQuantity: quantity, RequestedPrice: intent.Price,
		RequestedAmount: intent.Amount, OrderKind: intent.OrderKind, ProductClass: intent.ProductClass,
		QuantityMode: comboOrderQuantityMode(intent.OrderKind), ClientOrderID: intent.ClientOrderID,
		PreviewID: intent.PreviewID, NormalizedRequest: normalizedBrokerComboIntent(intent),
		Legs: intent.Legs, LegSnapshots: placed.Legs, SubmittedAt: now,
		Payload: map[string]any{
			"operation": "PLACE_COMBO", "brokerOrderId": placed.BrokerOrderID,
			"orderKind": intent.OrderKind, "legs": intent.Legs,
		},
		EventType: "COMMAND_COMBO_PLACE_ACCEPTED",
	})
	p.server.notifyExecutionOrderPlaced(record)
	return record, nil
}

func (p *serverTradingOrderGateway) CancelCombo(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	order, ok := p.server.executionOrders.order(strings.TrimSpace(internalOrderID))
	if !ok {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order not found")
	}
	selected := p.server.resolveBroker(order.BrokerID)
	service, ok := selected.(broker.ComboTradingService)
	if !ok {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("broker %q combo trading service is unavailable", order.BrokerID)
	}
	brokerOrderID := derefString(order.BrokerOrderIDEx)
	if brokerOrderID == "" {
		brokerOrderID = derefString(order.BrokerOrderID)
	}
	if brokerOrderID == "" {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution combo is missing broker order id")
	}
	if err := service.CancelComboOrder(ctx, broker.ReadQuery{
		BrokerID: order.BrokerID, TradingEnvironment: order.TradingEnvironment,
		AccountID: order.AccountID, Market: order.Market,
	}, brokerOrderID); err != nil {
		return trdsrv.ExecutionOrder{}, err
	}
	updated, _ := p.server.executionOrders.markCancelRequested(internalOrderID, map[string]any{
		"operation": "CANCEL_COMBO", "brokerOrderId": brokerOrderID,
	})
	return updated, nil
}

func comboOrderQuantityMode(kind broker.OrderKind) broker.QuantityMode {
	if kind == broker.OrderKindEventParlay {
		return broker.QuantityModeAmount
	}
	return broker.QuantityModeContracts
}

func normalizedBrokerComboIntent(intent broker.ComboOrderIntent) string {
	content, err := json.Marshal(intent)
	if err != nil {
		return "{}"
	}
	return string(content)
}

func (s *Server) defaultTradingEnvironment() string {
	if s == nil || s.store == nil {
		return "SIMULATE"
	}
	return s.store.ExecutionSettings().DefaultTradingEnvironment
}

func (s *Server) placeExecutionOrder(ctx context.Context, request trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	requestBrokerID := strings.TrimSpace(request.BrokerID)
	queryBrokerID := strings.TrimSpace(request.Query.BrokerID)
	if requestBrokerID != "" && queryBrokerID != "" && !strings.EqualFold(requestBrokerID, queryBrokerID) {
		return trdsrv.ExecutionOrder{}, fmt.Errorf(
			"execution broker %q does not match query broker %q",
			requestBrokerID,
			queryBrokerID,
		)
	}
	request.BrokerID = firstNonEmptyString(requestBrokerID, queryBrokerID)
	request.Query.BrokerID = request.BrokerID
	selected := s.resolveBroker(request.BrokerID)
	var placed *broker.PlaceOrderResult
	var err error
	prepared, fresh, err := s.executionOrders.prepareSubmission(trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: request.BrokerID, TradingEnvironment: request.Query.TradingEnvironment,
		AccountID: request.Query.AccountID, Market: request.Query.Market, Symbol: request.Symbol,
		Side: request.Side, OrderType: request.OrderType, RequestedQuantity: request.Query.Quantity,
		RequestedPrice: request.Query.Price, RequestedAmount: request.Query.Amount,
		OrderKind: request.OrderKind, ProductClass: request.ProductClass, QuantityMode: request.QuantityMode,
		ClientOrderID: request.Query.ClientOrderID, PreviewID: request.PreviewID,
		NormalizedRequest: request.NormalizedRequest, Legs: request.Legs, Remark: request.Remark,
	})
	if err != nil {
		return trdsrv.ExecutionOrder{}, err
	}
	if !fresh {
		return prepared, nil
	}
	if selected != nil && selected.Trading() != nil {
		placed, err = selected.Trading().PlaceOrder(ctx, request.Query)
	} else {
		err = fmt.Errorf("broker %q trading service is unavailable", request.BrokerID)
	}
	if err != nil {
		s.executionOrders.markSubmissionUnknown(prepared.InternalOrderID, err)
		return trdsrv.ExecutionOrder{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	payloadData := map[string]any{
		"operation": "PLACE", "brokerOrderId": placed.BrokerOrderID,
		"brokerOrderIdEx": placed.BrokerOrderIDEx, "tradingEnvironment": placed.TradingEnvironment,
		"accountId": placed.AccountID, "market": placed.Market, "symbol": request.Symbol,
		"side": request.Side, "orderType": request.OrderType,
		"requestedQuantity": request.Query.Quantity, "requestedPrice": request.Query.Price,
		"requestedAmount": request.Query.Amount, "predictionSide": request.Query.PredictionSide,
		"orderKind": request.OrderKind, "productClass": request.ProductClass,
		"rawBrokerStatus": placed.Status,
	}
	if request.Session != "" {
		payloadData["session"] = request.Session
	}
	record := s.executionOrders.recordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		InternalOrderID: prepared.InternalOrderID, BrokerID: request.BrokerID, BrokerOrderID: placed.BrokerOrderID,
		BrokerOrderIDEx:    derefString(placed.BrokerOrderIDEx),
		TradingEnvironment: placed.TradingEnvironment, AccountID: placed.AccountID,
		Market: placed.Market, Symbol: request.Symbol, Side: request.Side,
		OrderType: request.OrderType, Status: placed.Status,
		RequestedQuantity: request.Query.Quantity, RequestedPrice: request.Query.Price,
		RequestedAmount: request.Query.Amount, OrderKind: request.OrderKind,
		ProductClass: request.ProductClass, QuantityMode: request.QuantityMode,
		ClientOrderID: request.Query.ClientOrderID, PreviewID: request.PreviewID,
		NormalizedRequest: request.NormalizedRequest, Legs: request.Legs,
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

	selected := s.resolveBroker(order.BrokerID)
	cancelQuery := broker.ReadQuery{
		BrokerID:           order.BrokerID,
		TradingEnvironment: order.TradingEnvironment,
		AccountID:          order.AccountID,
		Market:             order.Market,
	}
	cancelOrder := broker.CancelOrder{
		OrderID:       brokerOrderID,
		BrokerOrderID: *order.BrokerOrderID,
		Symbol:        *order.Symbol,
	}
	if selected != nil && selected.Trading() != nil {
		err = selected.Trading().CancelOrders(ctx, cancelQuery, cancelOrder)
	} else {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("broker %q trading service is unavailable", order.BrokerID)
	}
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
