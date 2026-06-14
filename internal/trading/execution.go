package trading

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

type ExecutionPlaceRequest struct {
	BrokerID           string   `json:"brokerId"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	Env                string   `json:"env"`
	AccountID          string   `json:"accountId"`
	Market             string   `json:"market"`
	Code               string   `json:"code"`
	Symbol             string   `json:"symbol"`
	Side               string   `json:"side"`
	OrderType          string   `json:"orderType"`
	TimeInForce        string   `json:"timeInForce"`
	Session            string   `json:"session"`
	Quantity           float64  `json:"quantity"`
	Price              *float64 `json:"price"`
	StopPrice          *float64 `json:"stopPrice"`
	ClientOrderID      string   `json:"clientOrderId"`
	Remark             string   `json:"remark"`
}

type ExecutionOrderCommand struct {
	BrokerID  string
	Query     broker.PlaceOrderQuery
	Symbol    string
	Side      string
	OrderType string
	Remark    string
	Session   string
}

type ExecutionOrderFilter struct {
	BrokerID           string
	TradingEnvironment string
	AccountID          string
	Market             string
}

type ExecutionOrder struct {
	InternalOrderID    string   `json:"internalOrderId"`
	BrokerID           string   `json:"brokerId"`
	BrokerOrderID      *string  `json:"brokerOrderId"`
	BrokerOrderIDEx    *string  `json:"brokerOrderIdEx"`
	Source             string   `json:"source"`
	SourceDetail       string   `json:"sourceDetail"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	AccountID          string   `json:"accountId"`
	Market             string   `json:"market"`
	Symbol             *string  `json:"symbol"`
	Side               *string  `json:"side"`
	OrderType          *string  `json:"orderType"`
	Status             string   `json:"status"`
	RequestedQuantity  *float64 `json:"requestedQuantity"`
	RequestedPrice     *float64 `json:"requestedPrice"`
	FilledQuantity     *float64 `json:"filledQuantity"`
	FilledAveragePrice *float64 `json:"filledAveragePrice"`
	Remark             *string  `json:"remark"`
	LastError          *string  `json:"lastError"`
	LastErrorCode      *string  `json:"lastErrorCode"`
	LastErrorSource    *string  `json:"lastErrorSource"`
	SubmittedAt        *string  `json:"submittedAt"`
	UpdatedAt          string   `json:"updatedAt"`
	CreatedAt          string   `json:"createdAt"`
}

type ExecutionOrderEvent struct {
	ID              string  `json:"id"`
	InternalOrderID string  `json:"internalOrderId"`
	EventType       string  `json:"eventType"`
	PreviousStatus  *string `json:"previousStatus"`
	NextStatus      string  `json:"nextStatus"`
	PayloadJSON     string  `json:"payloadJson"`
	CreatedAt       string  `json:"createdAt"`
}

type ExecutionOrders struct {
	Orders []ExecutionOrder `json:"orders"`
}

type ExecutionOrderEvents struct {
	InternalOrderID string                `json:"internalOrderId"`
	Events          []ExecutionOrderEvent `json:"events"`
}

type ExecutionPlacedOrderRecord struct {
	BrokerID           string
	BrokerOrderID      string
	BrokerOrderIDEx    string
	TradingEnvironment string
	AccountID          string
	Market             string
	Symbol             string
	Side               string
	OrderType          string
	Status             string
	RequestedQuantity  float64
	RequestedPrice     *float64
	Remark             string
	SubmittedAt        string
	Payload            any
	EventType          string
	Message            string
}

type ExecutionCommandResponse struct {
	Accepted        bool    `json:"accepted"`
	Operation       string  `json:"operation"`
	InternalOrderID *string `json:"internalOrderId"`
	BrokerOrderID   *string `json:"brokerOrderId"`
	BrokerOrderIDEx *string `json:"brokerOrderIdEx"`
	OrderStatus     *string `json:"orderStatus"`
	BrokerErrorCode *string `json:"brokerErrorCode"`
	Message         string  `json:"message"`
	CheckedAt       string  `json:"checkedAt"`
}

type ExecutionPreview struct {
	PreviewAt          string   `json:"previewAt"`
	BrokerID           string   `json:"brokerId"`
	Symbol             string   `json:"symbol"`
	Side               string   `json:"side"`
	OrderType          string   `json:"orderType"`
	Quantity           float64  `json:"quantity"`
	Price              *float64 `json:"price"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	AccountID          string   `json:"accountId"`
	Market             string   `json:"market"`
	PreviewValid       bool     `json:"previewValid"`
}

type RequestError struct {
	message string
}

func (e RequestError) Error() string { return e.message }

func IsRequestError(err error) bool {
	var target RequestError
	return errors.As(err, &target)
}

func (s *Service) ExecutionFilter(brokerID, environment, accountID, marketCode string) ExecutionOrderFilter {
	if strings.TrimSpace(environment) == "" && s.defaultTradingEnvironment != nil {
		environment = s.defaultTradingEnvironment()
	}
	return ExecutionOrderFilter{
		BrokerID: strings.TrimSpace(brokerID), TradingEnvironment: strings.ToUpper(strings.TrimSpace(environment)),
		AccountID: strings.TrimSpace(accountID), Market: strings.ToUpper(strings.TrimSpace(marketCode)),
	}
}

func (s *Service) ListExecutionOrders(ctx context.Context, filter ExecutionOrderFilter, activeOnly bool) (ExecutionOrders, error) {
	s.SyncOrderUpdates(ctx, false, activeOnly)
	return s.listOrders(ctx, filter)
}

func (s *Service) ExecutionOrdersSnapshot(ctx context.Context) (ExecutionOrders, error) {
	return s.listOrders(ctx, ExecutionOrderFilter{})
}

func (s *Service) ExecutionOrderEvents(ctx context.Context, id string) (ExecutionOrderEvents, error) {
	return s.getOrderEvents(ctx, strings.TrimSpace(id))
}

func (s *Service) PreviewExecutionOrder(req ExecutionPlaceRequest) (ExecutionPreview, error) {
	command, err := s.normalizeExecutionOrder(req)
	if err != nil {
		return ExecutionPreview{}, err
	}
	return ExecutionPreview{
		PreviewAt: now(), BrokerID: command.BrokerID, Symbol: command.Symbol, Side: command.Side,
		OrderType: command.OrderType, Quantity: command.Query.Quantity, Price: command.Query.Price,
		TradingEnvironment: command.Query.TradingEnvironment, AccountID: command.Query.AccountID,
		Market: command.Query.Market, PreviewValid: true,
	}, nil
}

func (s *Service) CreateExecutionOrder(ctx context.Context, req ExecutionPlaceRequest) (ExecutionCommandResponse, error) {
	command, err := s.normalizeExecutionOrder(req)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	order, err := s.placeOrder(ctx, command)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("PLACE", "order submitted to broker", order), nil
}

func (s *Service) CancelExecutionOrder(ctx context.Context, id string) (ExecutionCommandResponse, error) {
	order, err := s.cancelOrder(ctx, strings.TrimSpace(id))
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("CANCEL", "cancel request submitted to broker", order), nil
}

func (s *Service) normalizeExecutionOrder(payload ExecutionPlaceRequest) (ExecutionOrderCommand, error) {
	brokerID := strings.ToLower(strings.TrimSpace(payload.BrokerID))
	if brokerID == "" {
		brokerID = "futu"
	}
	if brokerID != "futu" {
		return ExecutionOrderCommand{}, requestErrorf("execution orders currently support brokerId=futu only")
	}
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: payload.Market, Symbol: payload.Symbol, Code: payload.Code,
	})
	if err != nil {
		return ExecutionOrderCommand{}, requestErrorf("%s", err.Error())
	}
	side, err := normalizeExecutionSide(payload.Side)
	if err != nil {
		return ExecutionOrderCommand{}, err
	}
	orderType, err := normalizeExecutionOrderType(payload.OrderType)
	if err != nil {
		return ExecutionOrderCommand{}, err
	}
	if payload.Quantity <= 0 {
		return ExecutionOrderCommand{}, requestErrorf("quantity must be greater than 0")
	}
	if requiresLimitPrice(orderType) && (payload.Price == nil || *payload.Price <= 0) {
		return ExecutionOrderCommand{}, requestErrorf("order type %s requires price", orderType)
	}
	if requiresStopPrice(orderType) && (payload.StopPrice == nil || *payload.StopPrice <= 0) {
		return ExecutionOrderCommand{}, requestErrorf("order type %s requires stopPrice", orderType)
	}
	timeInForce := strings.ToUpper(strings.TrimSpace(payload.TimeInForce))
	if timeInForce == "" {
		timeInForce = "DAY"
	}
	if timeInForce == "FOK" {
		return ExecutionOrderCommand{}, requestErrorf("futu execution does not support timeInForce FOK")
	}
	environment := strings.ToUpper(strings.TrimSpace(payload.TradingEnvironment))
	if environment == "" {
		environment = strings.ToUpper(strings.TrimSpace(payload.Env))
	}
	if environment == "" && s.defaultTradingEnvironment != nil {
		environment = strings.ToUpper(strings.TrimSpace(s.defaultTradingEnvironment()))
	}
	if environment == "" {
		environment = "SIMULATE"
	}
	session, fillOutsideRTH, err := normalizeExecutionSession(instrument.Market, orderType, payload.Session)
	if err != nil {
		return ExecutionOrderCommand{}, err
	}
	remark := strings.TrimSpace(payload.Remark)
	if remark == "" {
		remark = strings.TrimSpace(payload.ClientOrderID)
	}
	return ExecutionOrderCommand{
		BrokerID: brokerID, Symbol: instrument.Symbol, Side: side, OrderType: orderType,
		Remark: remark, Session: session,
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{
				BrokerID: brokerID, TradingEnvironment: environment,
				AccountID: strings.TrimSpace(payload.AccountID), Market: instrument.Market,
			},
			Symbol: instrument.Symbol, Side: side, OrderType: orderType, Quantity: payload.Quantity,
			Price: payload.Price, TimeInForce: executionStringPointer(timeInForce),
			ClientOrderID: strings.TrimSpace(payload.ClientOrderID), Remark: executionStringPointer(remark),
			Session: executionStringPointer(session), FillOutsideRTH: fillOutsideRTH,
		},
	}, nil
}

func executionCommandResponse(operation, message string, order ExecutionOrder) ExecutionCommandResponse {
	status := order.Status
	internalOrderID := order.InternalOrderID
	return ExecutionCommandResponse{
		Accepted: true, Operation: operation, InternalOrderID: &internalOrderID,
		BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
		OrderStatus: &status, Message: message, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func requestErrorf(format string, args ...any) error {
	return RequestError{message: fmt.Sprintf(format, args...)}
}

func normalizeExecutionSide(side string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "BUY":
		return "BUY", nil
	case "SELL":
		return "SELL", nil
	default:
		return "", requestErrorf("unsupported side %q", side)
	}
}

func normalizeExecutionOrderType(orderType string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "", "LIMIT":
		return "LIMIT", nil
	case "MARKET":
		return "MARKET", nil
	case "STOP":
		return "STOP", nil
	case "STOP_LIMIT":
		return "STOP_LIMIT", nil
	default:
		return "", requestErrorf("unsupported orderType %q", orderType)
	}
}

func normalizeExecutionSession(marketCode, orderType, raw string) (string, *bool, error) {
	session := strings.ToUpper(strings.TrimSpace(raw))
	if strings.ToUpper(strings.TrimSpace(marketCode)) != "US" {
		if session != "" {
			return "", nil, requestErrorf("session is supported for US market orders only")
		}
		return "", nil, nil
	}
	if session == "" {
		session = "RTH"
	}
	switch session {
	case "RTH", "ETH", "ALL", "OVERNIGHT":
	default:
		return "", nil, requestErrorf("unsupported session %q", raw)
	}
	if orderType != "LIMIT" && orderType != "STOP_LIMIT" {
		return session, nil, nil
	}
	fillOutsideRTH := session != "RTH"
	return session, &fillOutsideRTH, nil
}

func requiresLimitPrice(orderType string) bool {
	return orderType == "LIMIT" || orderType == "STOP_LIMIT"
}

func requiresStopPrice(orderType string) bool {
	return orderType == "STOP" || orderType == "STOP_LIMIT"
}

func executionStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
