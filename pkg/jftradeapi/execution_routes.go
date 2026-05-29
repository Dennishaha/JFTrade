package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

type executionPlaceOrderRequest struct {
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

type normalizedExecutionPlaceOrder struct {
	brokerID    string
	query       futu.BrokerPlaceOrderQuery
	submitOrder types.SubmitOrder
	symbol      string
	side        string
	orderType   string
	remark      string
	session     string
}

func (s *Server) serveExecutionRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/execution/orders" && r.Method == http.MethodGet:
		s.syncBrokerOrderUpdates(r.Context(), false)
		s.writeOK(w, s.executionOrders.listOrders())
	case (r.URL.Path == "/api/v1/execution/orders" || r.URL.Path == "/api/v1/execution/orders/preview") && r.Method == http.MethodPost:
		s.handleExecutionPlaceOrder(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/execution/orders/") && strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
		s.handleExecutionOrderEvents(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/execution/orders/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost:
		s.handleExecutionCancelOrder(w, r)
	default:
		return false
	}
	return true
}

func (s *Server) handleExecutionOrderEvents(w http.ResponseWriter, r *http.Request) {
	internalOrderID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/execution/orders/", "/events"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "internalOrderId is invalid")
		return
	}
	s.writeOK(w, s.executionOrders.orderEvents(strings.TrimSpace(internalOrderID)))
}

func (s *Server) handleExecutionPlaceOrder(w http.ResponseWriter, r *http.Request) {
	var payload executionPlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid execution order payload")
		return
	}
	request, err := s.normalizeExecutionPlaceOrder(payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	placedRecord, err := s.placeExecutionOrder(r.Context(), request)
	if err != nil {
		status, code := executionCommandError(err)
		s.writeError(w, status, code, err.Error())
		return
	}

	message := "order submitted to broker"
	internalOrderID := placedRecord.InternalOrderID
	status := placedRecord.Status
	s.writeOK(w, brokerOrderCommandResponse{
		Accepted:        true,
		Operation:       "PLACE",
		InternalOrderID: &internalOrderID,
		BrokerOrderID:   placedRecord.BrokerOrderID,
		BrokerOrderIDEx: placedRecord.BrokerOrderIDEx,
		OrderStatus:     &status,
		BrokerErrorCode: nil,
		Message:         message,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func executionSubmitOrderWithDefaults(order types.SubmitOrder) types.SubmitOrder {
	if strings.TrimSpace(string(order.TimeInForce)) == "" {
		order.TimeInForce = types.TimeInForce("DAY")
	}
	return order
}

func (s *Server) placeExecutionOrder(ctx context.Context, request normalizedExecutionPlaceOrder) (executionOrderSummaryResponse, error) {
	request.submitOrder = executionSubmitOrderWithDefaults(request.submitOrder)
	placed, err := s.brokerExecutionExchange().PlaceBrokerOrder(ctx, request.query, request.submitOrder)
	if err != nil {
		return executionOrderSummaryResponse{}, err
	}

	payloadData := map[string]any{
		"operation":          "PLACE",
		"brokerOrderId":      strconv.FormatUint(placed.Order.OrderID, 10),
		"brokerOrderIdEx":    placed.BrokerOrderIDEx,
		"tradingEnvironment": placed.TradingEnvironment,
		"accountId":          placed.AccountID,
		"market":             placed.Market,
		"symbol":             request.symbol,
		"side":               request.side,
		"orderType":          request.orderType,
		"requestedQuantity":  request.submitOrder.Quantity.Float64(),
		"requestedPrice":     executionOptionalFixedpointValue(request.submitOrder.Price),
	}
	if request.session != "" {
		payloadData["session"] = request.session
	}

	placedRecord := s.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           request.brokerID,
		BrokerOrderID:      strconv.FormatUint(placed.Order.OrderID, 10),
		BrokerOrderIDEx:    derefString(placed.BrokerOrderIDEx),
		TradingEnvironment: placed.TradingEnvironment,
		AccountID:          placed.AccountID,
		Market:             placed.Market,
		Symbol:             request.symbol,
		Side:               request.side,
		OrderType:          request.orderType,
		Status:             placed.Order.OriginalStatus,
		RequestedQuantity:  request.submitOrder.Quantity.Float64(),
		RequestedPrice:     executionOptionalFixedpointValue(request.submitOrder.Price),
		Remark:             request.remark,
		SubmittedAt:        placed.Order.CreationTime.Time().UTC().Format(time.RFC3339Nano),
		Payload:            payloadData,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	s.notifyExecutionOrderPlaced(placedRecord)
	return placedRecord, nil
}

func (s *Server) handleExecutionCancelOrder(w http.ResponseWriter, r *http.Request) {
	internalOrderID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/execution/orders/", "/cancel"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "internalOrderId is invalid")
		return
	}
	internalOrderID = strings.TrimSpace(internalOrderID)
	order, ok := s.executionOrders.order(internalOrderID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "execution order not found")
		return
	}
	if order.BrokerOrderID == nil || strings.TrimSpace(*order.BrokerOrderID) == "" {
		s.writeError(w, http.StatusConflict, "BROKER_ORDER_ID_MISSING", "execution order is missing broker order id")
		return
	}

	brokerOrderID, err := strconv.ParseUint(strings.TrimSpace(*order.BrokerOrderID), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "execution order has invalid broker order id")
		return
	}
	if order.Symbol == nil || strings.TrimSpace(*order.Symbol) == "" {
		s.writeError(w, http.StatusConflict, "SYMBOL_MISSING", "execution order is missing symbol")
		return
	}

	err = s.futuExchange().CancelBrokerOrders(r.Context(), futu.BrokerReadQuery{
		TradingEnvironment: order.TradingEnvironment,
		AccountID:          order.AccountID,
		Market:             order.Market,
	}, types.Order{
		SubmitOrder: types.SubmitOrder{Symbol: *order.Symbol},
		OrderID:     brokerOrderID,
	})
	if err != nil {
		status, code := executionCommandError(err)
		s.writeError(w, status, code, err.Error())
		return
	}

	updatedOrder, _ := s.executionOrders.markCancelRequested(internalOrderID, map[string]any{
		"operation":       "CANCEL",
		"brokerOrderId":   *order.BrokerOrderID,
		"brokerOrderIdEx": order.BrokerOrderIDEx,
		"symbol":          order.Symbol,
	})
	status := updatedOrder.Status
	s.writeOK(w, brokerOrderCommandResponse{
		Accepted:        true,
		Operation:       "CANCEL",
		InternalOrderID: &internalOrderID,
		BrokerOrderID:   updatedOrder.BrokerOrderID,
		BrokerOrderIDEx: updatedOrder.BrokerOrderIDEx,
		OrderStatus:     &status,
		BrokerErrorCode: nil,
		Message:         "cancel request submitted to broker",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) normalizeExecutionPlaceOrder(payload executionPlaceOrderRequest) (normalizedExecutionPlaceOrder, error) {
	brokerID := strings.ToLower(strings.TrimSpace(payload.BrokerID))
	if brokerID == "" {
		brokerID = "futu"
	}
	if brokerID != "futu" {
		return normalizedExecutionPlaceOrder{}, fmt.Errorf("execution orders currently support brokerId=futu only")
	}

	instrument, err := normalizeInstrumentInput(payload.Market, payload.Symbol, payload.Code)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, err
	}
	market := instrument.Market
	symbol := instrument.Symbol

	side, bbgoSide, err := normalizeExecutionSide(payload.Side)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, err
	}
	orderType, bbgoOrderType, err := normalizeExecutionOrderType(payload.OrderType)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, err
	}
	if payload.Quantity <= 0 {
		return normalizedExecutionPlaceOrder{}, fmt.Errorf("quantity must be greater than 0")
	}
	if requiresExecutionLimitPrice(orderType) && (payload.Price == nil || *payload.Price <= 0) {
		return normalizedExecutionPlaceOrder{}, fmt.Errorf("order type %s requires price", orderType)
	}
	if requiresExecutionStopPrice(orderType) && (payload.StopPrice == nil || *payload.StopPrice <= 0) {
		return normalizedExecutionPlaceOrder{}, fmt.Errorf("order type %s requires stopPrice", orderType)
	}

	timeInForce := strings.ToUpper(strings.TrimSpace(payload.TimeInForce))
	if timeInForce == "" {
		timeInForce = "DAY"
	}
	if timeInForce == "FOK" {
		return normalizedExecutionPlaceOrder{}, fmt.Errorf("futu execution does not support timeInForce FOK")
	}

	tradingEnvironment := strings.ToUpper(strings.TrimSpace(payload.TradingEnvironment))
	if tradingEnvironment == "" {
		tradingEnvironment = strings.ToUpper(strings.TrimSpace(payload.Env))
	}
	if tradingEnvironment == "" {
		tradingEnvironment = "SIMULATE"
	}

	session, fillOutsideRTH, err := normalizeExecutionOrderSession(market, orderType, payload.Session)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, err
	}

	remark := strings.TrimSpace(payload.Remark)
	if remark == "" {
		remark = strings.TrimSpace(payload.ClientOrderID)
	}

	submitOrder := types.SubmitOrder{
		ClientOrderID: strings.TrimSpace(payload.ClientOrderID),
		Symbol:        symbol,
		Side:          bbgoSide,
		Type:          bbgoOrderType,
		Quantity:      fixedpoint.NewFromFloat(payload.Quantity),
		Market:        types.Market{Exchange: futu.Name, Symbol: symbol, LocalSymbol: symbol},
		TimeInForce:   types.TimeInForce(timeInForce),
		Tag:           remark,
	}
	if payload.Price != nil {
		submitOrder.Price = fixedpoint.NewFromFloat(*payload.Price)
	}
	if payload.StopPrice != nil {
		submitOrder.StopPrice = fixedpoint.NewFromFloat(*payload.StopPrice)
	}

	var sessionPtr *string
	if session != "" {
		sessionCopy := session
		sessionPtr = &sessionCopy
	}

	return normalizedExecutionPlaceOrder{
		brokerID: brokerID,
		query: futu.BrokerPlaceOrderQuery{
			BrokerReadQuery: futu.BrokerReadQuery{
				TradingEnvironment: tradingEnvironment,
				AccountID:          strings.TrimSpace(payload.AccountID),
				Market:             market,
			},
			Session:        sessionPtr,
			FillOutsideRTH: fillOutsideRTH,
		},
		submitOrder: submitOrder,
		symbol:      symbol,
		side:        side,
		orderType:   orderType,
		remark:      remark,
		session:     session,
	}, nil
}

func normalizeExecutionOrderSession(market string, orderType string, rawSession string) (string, *bool, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	session := strings.ToUpper(strings.TrimSpace(rawSession))
	if market != "US" {
		if session != "" {
			return "", nil, fmt.Errorf("session is supported for US market orders only")
		}
		return "", nil, nil
	}
	if session == "" {
		session = "RTH"
	}
	switch session {
	case "RTH", "ETH", "ALL", "OVERNIGHT":
	default:
		return "", nil, fmt.Errorf("unsupported session %q", rawSession)
	}
	if !supportsExecutionFillOutsideRTH(orderType) {
		return session, nil, nil
	}
	fillOutsideRTH := session != "RTH"
	return session, &fillOutsideRTH, nil
}

func supportsExecutionFillOutsideRTH(orderType string) bool {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "LIMIT", "STOP_LIMIT":
		return true
	default:
		return false
	}
}

func normalizeExecutionSide(side string) (string, types.SideType, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "BUY":
		return "BUY", types.SideTypeBuy, nil
	case "SELL":
		return "SELL", types.SideTypeSell, nil
	default:
		return "", "", fmt.Errorf("unsupported side %q", side)
	}
}

func normalizeExecutionOrderType(orderType string) (string, types.OrderType, error) {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "", "LIMIT":
		return "LIMIT", types.OrderTypeLimit, nil
	case "MARKET":
		return "MARKET", types.OrderTypeMarket, nil
	case "STOP":
		return "STOP", types.OrderTypeStopMarket, nil
	case "STOP_LIMIT":
		return "STOP_LIMIT", types.OrderTypeStopLimit, nil
	default:
		return "", "", fmt.Errorf("unsupported orderType %q", orderType)
	}
}

func requiresExecutionLimitPrice(orderType string) bool {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "LIMIT", "STOP_LIMIT":
		return true
	default:
		return false
	}
}

func requiresExecutionStopPrice(orderType string) bool {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "STOP", "STOP_LIMIT":
		return true
	default:
		return false
	}
}

func executionCommandError(err error) (int, string) {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "required"),
		strings.Contains(message, "unsupported"),
		strings.Contains(message, "invalid"),
		strings.Contains(message, "must be"):
		return http.StatusBadRequest, "BAD_REQUEST"
	default:
		return http.StatusBadGateway, "BROKER_COMMAND_FAILED"
	}
}

func executionOptionalFixedpointValue(value fixedpoint.Value) *float64 {
	if value.Sign() <= 0 {
		return nil
	}
	floatValue := value.Float64()
	return &floatValue
}
