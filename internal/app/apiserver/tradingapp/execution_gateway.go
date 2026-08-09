package tradingapp

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

// ExecutionOrderStore is the durable ledger surface used by the execution
// gateway. Keeping it local keeps tradingapp free of concrete store types.
type ExecutionOrderStore interface {
	PrepareSubmission(trdsrv.ExecutionPlacedOrderRecord) (trdsrv.ExecutionOrder, bool, error)
	MarkSubmissionUnknown(string, error) trdsrv.ExecutionOrder
	RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord) trdsrv.ExecutionOrder
	Order(string) (trdsrv.ExecutionOrder, bool)
	MarkCancelRequested(string, any) (trdsrv.ExecutionOrder, bool)
}

// ExecutionGatewayDependencies supplies the broker and ledger callbacks that
// keep the execution gateway independent from the HTTP composition root.
type ExecutionGatewayDependencies struct {
	ResolveBroker func(string) broker.Broker
	Orders        func() ExecutionOrderStore
	NotifyPlaced  func(trdsrv.ExecutionOrder)
}

// ExecutionGateway implements the trading service order and combo order
// gateways against an application-provided broker registry and ledger.
type ExecutionGateway struct {
	dependencies ExecutionGatewayDependencies
}

var _ trdsrv.OrderGateway = (*ExecutionGateway)(nil)
var _ trdsrv.ComboOrderGateway = (*ExecutionGateway)(nil)

func NewExecutionGateway(dependencies ExecutionGatewayDependencies) *ExecutionGateway {
	return &ExecutionGateway{dependencies: dependencies}
}

func (g *ExecutionGateway) PlaceOrder(ctx context.Context, request trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
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
	selected := g.resolveBroker(request.BrokerID)
	orders := g.orders()
	if orders == nil {
		return trdsrv.ExecutionOrder{}, trdsrv.ErrOrderStoreUnavailable
	}
	prepared, fresh, err := orders.PrepareSubmission(trdsrv.ExecutionPlacedOrderRecord{
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
	var placed *broker.PlaceOrderResult
	if selected != nil && selected.Trading() != nil {
		placed, err = selected.Trading().PlaceOrder(ctx, request.Query)
	} else {
		err = fmt.Errorf("broker %q trading service is unavailable", request.BrokerID)
	}
	if err != nil {
		orders.MarkSubmissionUnknown(prepared.InternalOrderID, err)
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
	record := orders.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
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
	if g.dependencies.NotifyPlaced != nil {
		g.dependencies.NotifyPlaced(record)
	}
	return record, nil
}

func (g *ExecutionGateway) CancelOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	internalOrderID = strings.TrimSpace(internalOrderID)
	orders := g.orders()
	if orders == nil {
		return trdsrv.ExecutionOrder{}, trdsrv.ErrOrderStoreUnavailable
	}
	order, ok := orders.Order(internalOrderID)
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
	selected := g.resolveBroker(order.BrokerID)
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
	if selected == nil || selected.Trading() == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("broker %q trading service is unavailable", order.BrokerID)
	}
	if err := selected.Trading().CancelOrders(ctx, cancelQuery, cancelOrder); err != nil {
		return trdsrv.ExecutionOrder{}, err
	}
	updatedOrder, _ := orders.MarkCancelRequested(internalOrderID, map[string]any{
		"operation":       "CANCEL",
		"brokerOrderId":   *order.BrokerOrderID,
		"brokerOrderIdEx": order.BrokerOrderIDEx,
		"symbol":          order.Symbol,
	})
	return updatedOrder, nil
}

func (g *ExecutionGateway) PlaceCombo(ctx context.Context, intent broker.ComboOrderIntent) (trdsrv.ExecutionOrder, error) {
	selected := g.resolveBroker(intent.BrokerID)
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
	orders := g.orders()
	if orders == nil {
		return trdsrv.ExecutionOrder{}, trdsrv.ErrOrderStoreUnavailable
	}
	prepared, fresh, err := orders.PrepareSubmission(trdsrv.ExecutionPlacedOrderRecord{
		BrokerID: intent.BrokerID, TradingEnvironment: intent.TradingEnvironment,
		AccountID: intent.AccountID, Market: intent.Market, Symbol: symbol, Side: side,
		OrderType: "COMBO", RequestedQuantity: quantity, RequestedPrice: intent.Price,
		RequestedAmount: intent.Amount, OrderKind: intent.OrderKind, ProductClass: intent.ProductClass,
		QuantityMode: ComboOrderQuantityMode(intent.OrderKind), ClientOrderID: intent.ClientOrderID,
		PreviewID: intent.PreviewID, NormalizedRequest: NormalizedBrokerComboIntent(intent), Legs: intent.Legs,
	})
	if err != nil {
		return trdsrv.ExecutionOrder{}, err
	}
	if !fresh {
		return prepared, nil
	}
	placed, err := service.PlaceComboOrder(ctx, intent)
	if err != nil {
		orders.MarkSubmissionUnknown(prepared.InternalOrderID, err)
		return trdsrv.ExecutionOrder{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	record := orders.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		InternalOrderID: prepared.InternalOrderID, BrokerID: intent.BrokerID, BrokerOrderID: placed.BrokerOrderID,
		BrokerOrderIDEx:    placed.BrokerOrderID,
		TradingEnvironment: intent.TradingEnvironment, AccountID: intent.AccountID,
		Market: intent.Market, Symbol: symbol, Side: side, OrderType: "COMBO",
		Status: placed.Status, RequestedQuantity: quantity, RequestedPrice: intent.Price,
		RequestedAmount: intent.Amount, OrderKind: intent.OrderKind, ProductClass: intent.ProductClass,
		QuantityMode: ComboOrderQuantityMode(intent.OrderKind), ClientOrderID: intent.ClientOrderID,
		PreviewID: intent.PreviewID, NormalizedRequest: NormalizedBrokerComboIntent(intent),
		Legs: intent.Legs, LegSnapshots: placed.Legs, SubmittedAt: now,
		Payload: map[string]any{
			"operation": "PLACE_COMBO", "brokerOrderId": placed.BrokerOrderID,
			"orderKind": intent.OrderKind, "legs": intent.Legs,
		},
		EventType: "COMMAND_COMBO_PLACE_ACCEPTED",
	})
	if g.dependencies.NotifyPlaced != nil {
		g.dependencies.NotifyPlaced(record)
	}
	return record, nil
}

func (g *ExecutionGateway) CancelCombo(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	orders := g.orders()
	if orders == nil {
		return trdsrv.ExecutionOrder{}, trdsrv.ErrOrderStoreUnavailable
	}
	order, ok := orders.Order(strings.TrimSpace(internalOrderID))
	if !ok {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("execution order not found")
	}
	selected := g.resolveBroker(order.BrokerID)
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
	updated, _ := orders.MarkCancelRequested(internalOrderID, map[string]any{
		"operation": "CANCEL_COMBO", "brokerOrderId": brokerOrderID,
	})
	return updated, nil
}

func (g *ExecutionGateway) resolveBroker(id string) broker.Broker {
	if g == nil || g.dependencies.ResolveBroker == nil {
		return nil
	}
	return g.dependencies.ResolveBroker(id)
}

func (g *ExecutionGateway) orders() ExecutionOrderStore {
	if g == nil || g.dependencies.Orders == nil {
		return nil
	}
	return g.dependencies.Orders()
}

func ComboOrderQuantityMode(kind broker.OrderKind) broker.QuantityMode {
	if kind == broker.OrderKindEventParlay {
		return broker.QuantityModeAmount
	}
	return broker.QuantityModeContracts
}

func NormalizedBrokerComboIntent(intent broker.ComboOrderIntent) string {
	content, err := json.Marshal(intent)
	if err != nil {
		return "{}"
	}
	return string(content)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
