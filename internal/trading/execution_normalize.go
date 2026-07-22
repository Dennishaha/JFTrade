package trading

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func (s *Service) normalizeExecutionOrder(payload ExecutionPlaceRequest) (ExecutionOrderCommand, error) {
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
	orderKind, productClass, quantityMode, err := normalizeExecutionProduct(&payload, instrument)
	if err != nil {
		return ExecutionOrderCommand{}, err
	}
	if err := validateExecutionPrices(payload, orderType); err != nil {
		return ExecutionOrderCommand{}, err
	}
	timeInForce, environment, session, fillOutsideRTH, err := s.normalizeExecutionTerms(
		payload, productClass, instrument.Market, orderType,
	)
	if err != nil {
		return ExecutionOrderCommand{}, err
	}
	remark := strings.TrimSpace(payload.Remark)
	if remark == "" {
		remark = strings.TrimSpace(payload.ClientOrderID)
	}
	brokerID, _, err := s.resolveExecutionBroker(payload.BrokerID)
	if err != nil {
		return ExecutionOrderCommand{}, err
	}
	command := ExecutionOrderCommand{
		BrokerID: brokerID, Symbol: instrument.Symbol, Side: side, OrderType: orderType,
		Remark: remark, Session: session, OrderKind: orderKind, ProductClass: productClass,
		QuantityMode: quantityMode, PreviewID: strings.TrimSpace(payload.PreviewID),
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{
				BrokerID: brokerID, TradingEnvironment: environment,
				AccountID: strings.TrimSpace(payload.AccountID), Market: instrument.Market,
			},
			Symbol: instrument.Symbol, ProductClass: productClass, QuantityMode: quantityMode,
			Side: side, OrderType: orderType, Quantity: payload.Quantity,
			Amount: payload.Amount, PredictionSide: payload.PredictionSide,
			Price: payload.Price, StopPrice: payload.StopPrice, TimeInForce: executionStringPointer(timeInForce),
			ClientOrderID: strings.TrimSpace(payload.ClientOrderID), Remark: executionStringPointer(remark),
			Session: executionStringPointer(session), FillOutsideRTH: fillOutsideRTH,
		},
	}
	normalized, err := json.Marshal(command.Query)
	if err != nil {
		return ExecutionOrderCommand{}, requestErrorf("normalize execution request: %v", err)
	}
	command.NormalizedRequest = string(normalized)
	return command, nil
}

func normalizeExecutionProduct(
	payload *ExecutionPlaceRequest,
	instrument market.Instrument,
) (broker.OrderKind, broker.ProductClass, broker.QuantityMode, error) {
	orderKind := payload.OrderKind
	if orderKind == "" {
		orderKind = broker.OrderKindSingle
	}
	productClass := payload.ProductClass
	if productClass == "" {
		productClass = broker.ProductClassEquity
	}
	if orderKind != broker.OrderKindSingle && orderKind != broker.OrderKindEventSingle {
		return "", "", "", requestErrorf("orderKind %q must use the combo execution endpoint", orderKind)
	}
	quantityMode := broker.QuantityModeUnits
	if productClass == broker.ProductClassOption || productClass == broker.ProductClassFuture {
		quantityMode = broker.QuantityModeContracts
	}
	if productClass == broker.ProductClassEventContract || orderKind == broker.OrderKindEventSingle {
		orderKind = broker.OrderKindEventSingle
		productClass = broker.ProductClassEventContract
		quantityMode = broker.QuantityModeAmount
		if payload.QuantityMode != "" && payload.QuantityMode != quantityMode {
			return "", "", "", requestErrorf("event-contract quantityMode must be amount")
		}
		if err := normalizePredictionOrder(payload, instrument); err != nil {
			return "", "", "", err
		}
	} else {
		if payload.QuantityMode != "" && payload.QuantityMode != quantityMode {
			return "", "", "", requestErrorf("quantityMode %q is invalid for productClass %q", payload.QuantityMode, productClass)
		}
		if payload.Amount != nil {
			return "", "", "", requestErrorf("amount is supported for event contracts only")
		}
		if strings.TrimSpace(payload.PredictionSide) != "" {
			return "", "", "", requestErrorf("predictionSide is supported for event contracts only")
		}
		if !finitePositive(payload.Quantity) {
			return "", "", "", requestErrorf("quantity must be greater than 0")
		}
	}
	if quantityMode == broker.QuantityModeContracts && payload.Quantity != float64(int64(payload.Quantity)) {
		return "", "", "", requestErrorf("option and future quantity must be an integer number of contracts")
	}
	return orderKind, productClass, quantityMode, nil
}

func normalizePredictionOrder(payload *ExecutionPlaceRequest, instrument market.Instrument) error {
	if !strings.EqualFold(instrument.Market, "US") {
		return requestErrorf("prediction contracts must use market US")
	}
	if payload.Amount == nil || !finitePositive(*payload.Amount) {
		return requestErrorf("event-contract amount must be greater than 0")
	}
	predictionSide := strings.ToUpper(strings.TrimSpace(payload.PredictionSide))
	if predictionSide != "YES" && predictionSide != "NO" {
		return requestErrorf("predictionSide must be YES or NO")
	}
	if payload.Price == nil || math.IsNaN(*payload.Price) || math.IsInf(*payload.Price, 0) || *payload.Price < 0.01 || *payload.Price > 0.99 {
		return requestErrorf("event-contract price must be between 0.01 and 0.99")
	}
	payload.PredictionSide = predictionSide
	return nil
}

func validateExecutionPrices(payload ExecutionPlaceRequest, orderType string) error {
	if payload.Price != nil && !finitePositive(*payload.Price) {
		return requestErrorf("price must be greater than 0 when provided")
	}
	if payload.StopPrice != nil && !finitePositive(*payload.StopPrice) {
		return requestErrorf("stopPrice must be greater than 0 when provided")
	}
	if requiresLimitPrice(orderType) && payload.Price == nil {
		return requestErrorf("order type %s requires price", orderType)
	}
	if requiresStopPrice(orderType) && payload.StopPrice == nil {
		return requestErrorf("order type %s requires stopPrice", orderType)
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (s *Service) normalizeExecutionTerms(
	payload ExecutionPlaceRequest,
	productClass broker.ProductClass,
	marketCode string,
	orderType string,
) (string, string, string, *bool, error) {
	timeInForce := strings.ToUpper(strings.TrimSpace(payload.TimeInForce))
	if timeInForce == "" {
		timeInForce = "DAY"
	}
	if timeInForce == "FOK" {
		return "", "", "", nil, requestErrorf("execution does not support timeInForce FOK")
	}
	environment := strings.TrimSpace(payload.TradingEnvironment)
	if environment == "" {
		environment = payload.Env
	}
	environment = s.executionEnvironment(environment)
	if productClass == broker.ProductClassOption && strings.TrimSpace(payload.Session) != "" {
		return "", "", "", nil, requestErrorf("US options do not support stock extended-hours sessions")
	}
	var session string
	var fillOutsideRTH *bool
	if productClass != broker.ProductClassOption && productClass != broker.ProductClassEventContract {
		var err error
		session, fillOutsideRTH, err = normalizeExecutionSession(marketCode, orderType, payload.Session)
		if err != nil {
			return "", "", "", nil, err
		}
	}
	return timeInForce, environment, session, fillOutsideRTH, nil
}

func executionCommandHash(command ExecutionOrderCommand) string {
	query := command.Query
	query.Remark = nil
	content, _ := json.Marshal(map[string]any{
		"brokerId": command.BrokerID, "orderKind": command.OrderKind,
		"productClass": command.ProductClass, "query": query, "legs": command.Legs,
	})
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func executionCommandResponse(operation, message string, order ExecutionOrder) ExecutionCommandResponse {
	return ExecutionCommandResponse{
		Accepted: true, Operation: operation, InternalOrderID: new(order.InternalOrderID),
		BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
		OrderStatus: new(order.Status), Message: message, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
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
	return session, new(session != "RTH"), nil
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
