package jftradeapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/pkg/broker"
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
	brokerID  string
	query     broker.PlaceOrderQuery
	symbol    string
	side      string
	orderType string
	remark    string
	session   string
}

type executionValidationError struct {
	message string
}

func (e executionValidationError) Error() string {
	return e.message
}

func executionValidationErrorf(format string, args ...any) error {
	return executionValidationError{message: fmt.Sprintf(format, args...)}
}

// handleExecutionOrders godoc
// @Summary 读取执行订单
// @Tags execution
// @Produce json
// @Param scope query string false "ACTIVE 表示仅活动订单"
// @Param brokerId query string false "Broker 标识"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场"
// @Success 200 {object} envelope{data=executionOrdersResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/execution/orders [get]
func (s *Server) handleExecutionOrders(c *gin.Context) {
	var query executionOrdersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid execution query")
		return
	}
	scope := strings.ToUpper(strings.TrimSpace(query.Scope))
	skipHistory := scope == "ACTIVE"
	s.syncBrokerOrderUpdatesWithScope(c.Request.Context(), false, skipHistory)
	s.writeOK(c, s.executionOrders.listOrdersFiltered(s.executionOrderListFilterFromQuery(query)))
}

func (s *Server) executionOrderListFilterFromQuery(query executionOrdersQuery) executionOrderListFilter {
	tradingEnvironment := strings.TrimSpace(query.TradingEnvironment)
	if tradingEnvironment == "" {
		tradingEnvironment = s.defaultTradingEnvironment()
	}
	return executionOrderListFilter{
		BrokerID:           strings.TrimSpace(query.BrokerID),
		TradingEnvironment: strings.ToUpper(strings.TrimSpace(tradingEnvironment)),
		AccountID:          strings.TrimSpace(query.AccountID),
		Market:             strings.ToUpper(strings.TrimSpace(query.Market)),
	}
}

func (s *Server) defaultTradingEnvironment() string {
	if s == nil || s.store == nil {
		return "SIMULATE"
	}
	return s.store.executionSettings().DefaultTradingEnvironment
}

// handleExecutionOrderEvents godoc
// @Summary 读取执行订单事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=executionOrderEventsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId}/events [get]
func (s *Server) handleExecutionOrderEvents(c *gin.Context) {
	var uri internalOrderURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "internalOrderId is invalid")
		return
	}
	internalOrderID := uri.InternalOrderID
	s.writeOK(c, s.executionOrders.orderEvents(strings.TrimSpace(internalOrderID)))
}

// handleExecutionPlaceOrder godoc
// @Summary 提交执行订单
// @Tags execution
// @Accept json
// @Produce json
// @Param request body executionPlaceOrderRequest true "执行订单"
// @Success 200 {object} envelope{data=brokerOrderCommandResponse}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/execution/orders [post]
func (s *Server) handleExecutionPlaceOrder(c *gin.Context) {
	var payload executionPlaceOrderRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid execution order payload")
		return
	}
	request, err := s.normalizeExecutionPlaceOrder(payload)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	placedRecord, err := s.placeExecutionOrder(c.Request.Context(), request)
	if err != nil {
		status, code := executionCommandError(err)
		s.writeError(c, status, code, err.Error())
		return
	}

	message := "order submitted to broker"
	internalOrderID := placedRecord.InternalOrderID
	status := placedRecord.Status
	s.writeOK(c, brokerOrderCommandResponse{
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

func (s *Server) placeExecutionOrder(ctx context.Context, request normalizedExecutionPlaceOrder) (executionOrderSummaryResponse, error) {
	placed, err := s.brokerExecutionExchange().PlaceBrokerOrder(ctx, request.query)
	if err != nil {
		return executionOrderSummaryResponse{}, err
	}

	brokerOrderID := placed.BrokerOrderID
	now := time.Now().UTC().Format(time.RFC3339Nano)

	payloadData := map[string]any{
		"operation":          "PLACE",
		"brokerOrderId":      brokerOrderID,
		"brokerOrderIdEx":    placed.BrokerOrderIDEx,
		"tradingEnvironment": placed.TradingEnvironment,
		"accountId":          placed.AccountID,
		"market":             placed.Market,
		"symbol":             request.symbol,
		"side":               request.side,
		"orderType":          request.orderType,
		"requestedQuantity":  request.query.Quantity,
		"requestedPrice":     request.query.Price,
	}
	if request.session != "" {
		payloadData["session"] = request.session
	}

	placedRecord := s.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           request.brokerID,
		BrokerOrderID:      brokerOrderID,
		BrokerOrderIDEx:    derefString(placed.BrokerOrderIDEx),
		TradingEnvironment: placed.TradingEnvironment,
		AccountID:          placed.AccountID,
		Market:             placed.Market,
		Symbol:             request.symbol,
		Side:               request.side,
		OrderType:          request.orderType,
		Status:             placed.Status,
		RequestedQuantity:  request.query.Quantity,
		RequestedPrice:     request.query.Price,
		Remark:             request.remark,
		SubmittedAt:        now,
		Payload:            payloadData,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	s.notifyExecutionOrderPlaced(placedRecord)
	return placedRecord, nil
}

// handleExecutionCancelOrder godoc
// @Summary 取消执行订单
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=brokerOrderCommandResponse}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId}/cancel [post]
func (s *Server) handleExecutionCancelOrder(c *gin.Context) {
	var uri internalOrderURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "internalOrderId is invalid")
		return
	}
	internalOrderID := strings.TrimSpace(uri.InternalOrderID)
	order, ok := s.executionOrders.order(internalOrderID)
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "execution order not found")
		return
	}
	if order.BrokerOrderID == nil || strings.TrimSpace(*order.BrokerOrderID) == "" {
		s.writeError(c, http.StatusConflict, "BROKER_ORDER_ID_MISSING", "execution order is missing broker order id")
		return
	}

	brokerOrderID, err := strconv.ParseUint(strings.TrimSpace(*order.BrokerOrderID), 10, 64)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "execution order has invalid broker order id")
		return
	}
	if order.Symbol == nil || strings.TrimSpace(*order.Symbol) == "" {
		s.writeError(c, http.StatusConflict, "SYMBOL_MISSING", "execution order is missing symbol")
		return
	}

	err = s.activeBroker().Trading().CancelOrders(c.Request.Context(), broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: order.TradingEnvironment,
		AccountID:          order.AccountID,
		Market:             order.Market,
	}, broker.CancelOrder{
		OrderID: brokerOrderID,
		Symbol:  *order.Symbol,
	})
	if err != nil {
		status, code := executionCommandError(err)
		s.writeError(c, status, code, err.Error())
		return
	}

	updatedOrder, _ := s.executionOrders.markCancelRequested(internalOrderID, map[string]any{
		"operation":       "CANCEL",
		"brokerOrderId":   *order.BrokerOrderID,
		"brokerOrderIdEx": order.BrokerOrderIDEx,
		"symbol":          order.Symbol,
	})
	status := updatedOrder.Status
	s.writeOK(c, brokerOrderCommandResponse{
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
		return normalizedExecutionPlaceOrder{}, executionValidationErrorf("execution orders currently support brokerId=futu only")
	}

	instrument, err := normalizeInstrumentInput(payload.Market, payload.Symbol, payload.Code)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, executionValidationErrorf("%s", err.Error())
	}
	market := instrument.Market
	symbol := instrument.Symbol

	side, err := normalizeExecutionSide(payload.Side)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, err
	}
	orderType, err := normalizeExecutionOrderType(payload.OrderType)
	if err != nil {
		return normalizedExecutionPlaceOrder{}, err
	}
	if payload.Quantity <= 0 {
		return normalizedExecutionPlaceOrder{}, executionValidationErrorf("quantity must be greater than 0")
	}
	if requiresExecutionLimitPrice(orderType) && (payload.Price == nil || *payload.Price <= 0) {
		return normalizedExecutionPlaceOrder{}, executionValidationErrorf("order type %s requires price", orderType)
	}
	if requiresExecutionStopPrice(orderType) && (payload.StopPrice == nil || *payload.StopPrice <= 0) {
		return normalizedExecutionPlaceOrder{}, executionValidationErrorf("order type %s requires stopPrice", orderType)
	}

	timeInForce := strings.ToUpper(strings.TrimSpace(payload.TimeInForce))
	if timeInForce == "" {
		timeInForce = "DAY"
	}
	if timeInForce == "FOK" {
		return normalizedExecutionPlaceOrder{}, executionValidationErrorf("futu execution does not support timeInForce FOK")
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

	var sessionPtr *string
	if session != "" {
		sessionCopy := session
		sessionPtr = &sessionCopy
	}

	var remarkPtr *string
	if remark != "" {
		remarkCopy := remark
		remarkPtr = &remarkCopy
	}

	placeQuery := broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{
			BrokerID:           brokerID,
			TradingEnvironment: tradingEnvironment,
			AccountID:          strings.TrimSpace(payload.AccountID),
			Market:             market,
		},
		Symbol:         symbol,
		Side:           side,
		OrderType:      orderType,
		Quantity:       payload.Quantity,
		Price:          payload.Price,
		TimeInForce:    &timeInForce,
		ClientOrderID:  strings.TrimSpace(payload.ClientOrderID),
		Remark:         remarkPtr,
		Session:        sessionPtr,
		FillOutsideRTH: fillOutsideRTH,
	}

	return normalizedExecutionPlaceOrder{
		brokerID:  brokerID,
		query:     placeQuery,
		symbol:    symbol,
		side:      side,
		orderType: orderType,
		remark:    remark,
		session:   session,
	}, nil
}

func normalizeExecutionOrderSession(market string, orderType string, rawSession string) (string, *bool, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	session := strings.ToUpper(strings.TrimSpace(rawSession))
	if market != "US" {
		if session != "" {
			return "", nil, executionValidationErrorf("session is supported for US market orders only")
		}
		return "", nil, nil
	}
	if session == "" {
		session = "RTH"
	}
	switch session {
	case "RTH", "ETH", "ALL", "OVERNIGHT":
	default:
		return "", nil, executionValidationErrorf("unsupported session %q", rawSession)
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

func normalizeExecutionSide(side string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "BUY":
		return "BUY", nil
	case "SELL":
		return "SELL", nil
	default:
		return "", executionValidationErrorf("unsupported side %q", side)
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
		return "", executionValidationErrorf("unsupported orderType %q", orderType)
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
	var validationErr executionValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, "BAD_REQUEST"
	}
	var brokerErr *broker.BrokerError
	if errors.As(err, &brokerErr) {
		switch strings.TrimSpace(brokerErr.Code) {
		case broker.ErrCodeAccountNotFound, broker.ErrCodeMarketNotSupported, broker.ErrCodeOrderNotFound:
			return http.StatusBadRequest, "BAD_REQUEST"
		case broker.ErrCodeTimeout:
			return http.StatusGatewayTimeout, "BROKER_TIMEOUT"
		case broker.ErrCodeRateLimited:
			return http.StatusTooManyRequests, "BROKER_RATE_LIMITED"
		case broker.ErrCodeNotConnected:
			return http.StatusBadGateway, "BROKER_NOT_CONNECTED"
		default:
			return http.StatusBadGateway, "BROKER_COMMAND_FAILED"
		}
	}
	return http.StatusBadGateway, "BROKER_COMMAND_FAILED"
}
