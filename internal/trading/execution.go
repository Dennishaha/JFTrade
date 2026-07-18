package trading

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

type ExecutionPlaceRequest struct {
	BrokerID           string                  `json:"brokerId"`
	TradingEnvironment string                  `json:"tradingEnvironment"`
	Env                string                  `json:"env"`
	AccountID          string                  `json:"accountId"`
	Market             string                  `json:"market"`
	Code               string                  `json:"code"`
	Symbol             string                  `json:"symbol"`
	Side               string                  `json:"side"`
	OrderType          string                  `json:"orderType"`
	TimeInForce        string                  `json:"timeInForce"`
	Session            string                  `json:"session"`
	Quantity           float64                 `json:"quantity"`
	Price              *float64                `json:"price"`
	StopPrice          *float64                `json:"stopPrice"`
	Amount             *float64                `json:"amount"`
	PredictionSide     string                  `json:"predictionSide"`
	ProductClass       broker.ProductClass     `json:"productClass"`
	OrderKind          broker.OrderKind        `json:"orderKind"`
	QuantityMode       broker.QuantityMode     `json:"quantityMode"`
	PreviewID          string                  `json:"previewId"`
	RFQID              string                  `json:"rfqId"`
	QuoteExpiresAt     *time.Time              `json:"quoteExpiresAt"`
	Legs               []broker.OrderLegIntent `json:"legs"`
	ClientOrderID      string                  `json:"clientOrderId"`
	Remark             string                  `json:"remark"`
}

type ExecutionOrderCommand struct {
	BrokerID          string
	Query             broker.PlaceOrderQuery
	Symbol            string
	Side              string
	OrderType         string
	Remark            string
	Session           string
	OrderKind         broker.OrderKind
	ProductClass      broker.ProductClass
	QuantityMode      broker.QuantityMode
	PreviewID         string
	RFQID             string
	QuoteExpiresAt    *time.Time
	Legs              []broker.OrderLegIntent
	NormalizedRequest string
}

type ExecutionOrderFilter struct {
	BrokerID           string
	TradingEnvironment string
	AccountID          string
	Market             string
}

type ExecutionOrder struct {
	InternalOrderID    string              `json:"internalOrderId"`
	BrokerID           string              `json:"brokerId"`
	BrokerOrderID      *string             `json:"brokerOrderId"`
	BrokerOrderIDEx    *string             `json:"brokerOrderIdEx"`
	Source             string              `json:"source"`
	SourceDetail       string              `json:"sourceDetail"`
	TradingEnvironment string              `json:"tradingEnvironment"`
	AccountID          string              `json:"accountId"`
	Market             string              `json:"market"`
	OrderKind          broker.OrderKind    `json:"orderKind"`
	ProductClass       broker.ProductClass `json:"productClass"`
	QuantityMode       broker.QuantityMode `json:"quantityMode"`
	ClientOrderID      *string             `json:"clientOrderId"`
	PreviewID          *string             `json:"previewId"`
	NormalizedRequest  string              `json:"normalizedRequest,omitempty"`
	RequestedAmount    *float64            `json:"requestedAmount"`
	Fees               *float64            `json:"fees"`
	Payout             *float64            `json:"payout"`
	Legs               []ExecutionOrderLeg `json:"legs,omitempty"`
	Symbol             *string             `json:"symbol"`
	Side               *string             `json:"side"`
	OrderType          *string             `json:"orderType"`
	Status             string              `json:"status"`
	RawBrokerStatus    *string             `json:"rawBrokerStatus"`
	RequestedQuantity  *float64            `json:"requestedQuantity"`
	RequestedPrice     *float64            `json:"requestedPrice"`
	FilledQuantity     *float64            `json:"filledQuantity"`
	FilledAveragePrice *float64            `json:"filledAveragePrice"`
	Remark             *string             `json:"remark"`
	LastError          *string             `json:"lastError"`
	LastErrorCode      *string             `json:"lastErrorCode"`
	LastErrorSource    *string             `json:"lastErrorSource"`
	SubmittedAt        *string             `json:"submittedAt"`
	UpdatedAt          string              `json:"updatedAt"`
	CreatedAt          string              `json:"createdAt"`
}

type ExecutionOrderLeg struct {
	ID                string              `json:"id"`
	InternalOrderID   string              `json:"internalOrderId"`
	Index             int                 `json:"index"`
	BrokerLegID       *string             `json:"brokerLegId,omitempty"`
	InstrumentID      string              `json:"instrumentId"`
	ProductClass      broker.ProductClass `json:"productClass"`
	Side              string              `json:"side"`
	Ratio             int                 `json:"ratio"`
	PredictionSide    string              `json:"predictionSide,omitempty"`
	RequestedQuantity *float64            `json:"requestedQuantity,omitempty"`
	RequestedAmount   *float64            `json:"requestedAmount,omitempty"`
	RequestedPrice    *float64            `json:"requestedPrice,omitempty"`
	Status            string              `json:"status"`
	FilledQuantity    *float64            `json:"filledQuantity,omitempty"`
	FilledAmount      *float64            `json:"filledAmount,omitempty"`
	AveragePrice      *float64            `json:"averagePrice,omitempty"`
	Fees              *float64            `json:"fees,omitempty"`
	Payout            *float64            `json:"payout,omitempty"`
	UpdatedAt         string              `json:"updatedAt"`
	CreatedAt         string              `json:"createdAt"`
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

type ExecutionOrderDetails struct {
	Order        ExecutionOrder        `json:"order"`
	RecentEvents []ExecutionOrderEvent `json:"recentEvents"`
	CheckedAt    string                `json:"checkedAt"`
}

type ExecutionPlacedOrderRecord struct {
	InternalOrderID    string
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
	RequestedAmount    *float64
	OrderKind          broker.OrderKind
	ProductClass       broker.ProductClass
	QuantityMode       broker.QuantityMode
	ClientOrderID      string
	PreviewID          string
	NormalizedRequest  string
	Legs               []broker.OrderLegIntent
	LegSnapshots       []broker.OrderLegSnapshot
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
	PreviewID          string              `json:"previewId"`
	PreviewAt          string              `json:"previewAt"`
	ExpiresAt          string              `json:"expiresAt"`
	CapabilityVersion  string              `json:"capabilityVersion"`
	BrokerID           string              `json:"brokerId"`
	Symbol             string              `json:"symbol"`
	Side               string              `json:"side"`
	OrderType          string              `json:"orderType"`
	Quantity           float64             `json:"quantity"`
	Price              *float64            `json:"price"`
	Amount             *float64            `json:"amount"`
	PredictionSide     string              `json:"predictionSide,omitempty"`
	ProductClass       broker.ProductClass `json:"productClass"`
	OrderKind          broker.OrderKind    `json:"orderKind"`
	QuantityMode       broker.QuantityMode `json:"quantityMode"`
	RequestHash        string              `json:"requestHash"`
	TradingEnvironment string              `json:"tradingEnvironment"`
	AccountID          string              `json:"accountId"`
	Market             string              `json:"market"`
	PreviewValid       bool                `json:"previewValid"`
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
	return s.orderStore.ListOrders(ctx, filter)
}

func (s *Service) ExecutionOrdersSnapshot(ctx context.Context) (ExecutionOrders, error) {
	return s.orderStore.ListOrders(ctx, ExecutionOrderFilter{})
}

func (s *Service) ExecutionOrderEvents(ctx context.Context, id string) (ExecutionOrderEvents, error) {
	return s.orderStore.OrderEvents(ctx, strings.TrimSpace(id))
}

func (s *Service) ExecutionOrderDetails(ctx context.Context, id string) (ExecutionOrderDetails, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ExecutionOrderDetails{}, requestErrorf("internalOrderId is required")
	}
	// A receipt refresh must bypass the active-order cache so a just-filled or
	// just-cancelled order does not remain stale for the cache TTL.
	s.SyncOrderUpdates(ctx, true, true)
	orders, err := s.orderStore.ListOrders(ctx, ExecutionOrderFilter{})
	if err != nil {
		return ExecutionOrderDetails{}, err
	}
	var order *ExecutionOrder
	for index := range orders.Orders {
		if orders.Orders[index].InternalOrderID == id {
			order = &orders.Orders[index]
			break
		}
	}
	if order == nil {
		return ExecutionOrderDetails{}, ErrExecutionOrderNotFound
	}
	if !IsCanonicalTerminalOrderStatus(order.Status) && executionOrderHasBrokerReference(*order) {
		s.SyncExecutionOrderHistory(ctx, *order)
		orders, err = s.orderStore.ListOrders(ctx, ExecutionOrderFilter{})
		if err != nil {
			return ExecutionOrderDetails{}, err
		}
		for index := range orders.Orders {
			if orders.Orders[index].InternalOrderID == id {
				order = &orders.Orders[index]
				break
			}
		}
	}
	events, err := s.orderStore.OrderEvents(ctx, id)
	if err != nil {
		return ExecutionOrderDetails{}, err
	}
	recentEvents := events.Events
	if len(recentEvents) > 10 {
		recentEvents = recentEvents[len(recentEvents)-10:]
	}
	return ExecutionOrderDetails{
		Order:        *order,
		RecentEvents: append([]ExecutionOrderEvent(nil), recentEvents...),
		CheckedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func executionOrderHasBrokerReference(order ExecutionOrder) bool {
	return strings.TrimSpace(executionStringValue(order.BrokerOrderID)) != "" ||
		strings.TrimSpace(executionStringValue(order.BrokerOrderIDEx)) != ""
}

func executionStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) PreviewExecutionOrder(req ExecutionPlaceRequest) (ExecutionPreview, error) {
	return s.PreviewExecutionOrderContext(context.Background(), req)
}

func (s *Service) PreviewExecutionOrderContext(
	ctx context.Context,
	req ExecutionPlaceRequest,
) (ExecutionPreview, error) {
	command, err := s.normalizeExecutionOrder(req)
	if err != nil {
		return ExecutionPreview{}, err
	}
	if requiresLockedExecutionPreview(command) && strings.TrimSpace(command.Query.ClientOrderID) == "" {
		return ExecutionPreview{}, requestErrorf("clientOrderId is required for derivative and event-contract previews")
	}
	if err := s.validateProductOrderPreview(ctx, command); err != nil {
		return ExecutionPreview{}, err
	}
	previewAt := time.Now().UTC()
	requestHash := executionCommandHash(command)
	previewID := "preview-" + requestHash[:20]
	expiresAt := previewAt.Add(5 * time.Minute)
	preview := ExecutionPreview{
		PreviewID: previewID, PreviewAt: previewAt.Format(time.RFC3339Nano),
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		BrokerID:          command.BrokerID, Symbol: command.Symbol, Side: command.Side,
		OrderType: command.OrderType, Quantity: command.Query.Quantity, Price: command.Query.Price,
		Amount: command.Query.Amount, PredictionSide: command.Query.PredictionSide,
		ProductClass: command.ProductClass, OrderKind: command.OrderKind, QuantityMode: command.QuantityMode,
		RequestHash:        requestHash,
		TradingEnvironment: command.Query.TradingEnvironment, AccountID: command.Query.AccountID,
		Market: command.Query.Market, PreviewValid: true,
	}
	if s.previewStore != nil {
		if err := s.previewStore.SavePreview(ExecutionPreviewRecord{
			PreviewID: previewID, RequestHash: requestHash, BrokerID: command.BrokerID,
			CapabilityVersion: broker.BuiltinCapabilityCatalog.Version, AccountID: command.Query.AccountID,
			ExpiresAt: expiresAt.Format(time.RFC3339Nano), NormalizedRequest: command.NormalizedRequest,
			CreatedAt: previewAt.Format(time.RFC3339Nano),
		}); err != nil {
			return ExecutionPreview{}, err
		}
	}
	return preview, nil
}

func (s *Service) CreateExecutionOrder(ctx context.Context, req ExecutionPlaceRequest) (ExecutionCommandResponse, error) {
	command, err := s.normalizeExecutionOrder(req)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	if requiresLockedExecutionPreview(command) {
		if strings.TrimSpace(command.PreviewID) == "" {
			return ExecutionCommandResponse{}, requestErrorf("previewId is required for derivative and event-contract orders")
		}
		if strings.TrimSpace(command.Query.ClientOrderID) == "" {
			return ExecutionCommandResponse{}, requestErrorf("clientOrderId is required for idempotent derivative and event-contract submission")
		}
	}
	if strings.TrimSpace(command.PreviewID) != "" && strings.TrimSpace(command.Query.ClientOrderID) == "" {
		return ExecutionCommandResponse{}, requestErrorf("clientOrderId is required when previewId is supplied")
	}
	if s.previewStore != nil && strings.TrimSpace(command.PreviewID) != "" {
		if err := s.previewStore.ConsumePreview(
			command.PreviewID, command.BrokerID, command.Query.AccountID,
			executionCommandHash(command), command.Query.ClientOrderID,
		); err != nil {
			return ExecutionCommandResponse{}, requestErrorf("execution preview is invalid: %v", err)
		}
	}
	order, err := s.PlaceExecutionOrder(ctx, command)
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("PLACE", "order submitted to broker", order), nil
}

func requiresLockedExecutionPreview(command ExecutionOrderCommand) bool {
	return command.OrderKind == broker.OrderKindEventSingle ||
		command.ProductClass == broker.ProductClassOption ||
		command.ProductClass == broker.ProductClassFuture ||
		command.ProductClass == broker.ProductClassEventContract
}

func (s *Service) validateProductOrderPreview(ctx context.Context, command ExecutionOrderCommand) error {
	if !requiresLockedExecutionPreview(command) {
		return nil
	}
	selected := s.brokerRuntime.ActiveBroker()
	if resolver, ok := s.brokerRuntime.(interface{ ResolveBroker(string) broker.Broker }); ok {
		selected = resolver.ResolveBroker(command.BrokerID)
	} else if selected != nil && !strings.EqualFold(selected.ID(), command.BrokerID) {
		selected = nil
	}
	if selected == nil {
		return requestErrorf("brokerId %q is not available", command.BrokerID)
	}
	provider, ok := selected.(broker.ProductRuleProvider)
	if !ok {
		return requestErrorf("broker %q does not support product rule previews", command.BrokerID)
	}
	segment := broker.MarketSegmentDerivatives
	if command.ProductClass == broker.ProductClassEventContract {
		segment = broker.MarketSegmentPrediction
		if err := validatePredictionTradingAccount(ctx, selected, command.Query.AccountID); err != nil {
			return err
		}
	}
	if command.ProductClass == broker.ProductClassFuture &&
		strings.EqualFold(command.Query.TradingEnvironment, "REAL") {
		if err := validateFuturesTradingAuthority(ctx, selected, command.Query.AccountID); err != nil {
			return err
		}
	}
	quantity := command.Query.Quantity
	result, err := provider.ValidateProductOrder(ctx, broker.ProductRuleQuery{
		ReadQuery: command.Query.ReadQuery,
		FeatureID: broker.FeatureExecutionOrderPreview,
		Instrument: broker.Instrument{
			InstrumentID:  command.Symbol,
			Code:          strings.TrimPrefix(command.Symbol, command.Query.Market+"."),
			ProductClass:  command.ProductClass,
			MarketSegment: segment,
			QuoteMarket:   command.Query.Market,
			TradeMarket:   command.Query.Market,
			QuantityMode:  command.QuantityMode,
		},
		OrderKind: command.OrderKind,
		OrderType: command.OrderType,
		Session:   command.Session,
		Quantity:  &quantity,
		Amount:    command.Query.Amount,
		Price:     command.Query.Price,
		Legs:      command.Legs,
	})
	if err != nil {
		return err
	}
	if result == nil || !result.Allowed {
		reason := "product-rule preview rejected"
		if result != nil && strings.TrimSpace(result.Reason) != "" {
			reason = result.Reason
		}
		return requestErrorf("%s", reason)
	}
	return nil
}

func validateFuturesTradingAuthority(ctx context.Context, selected broker.Broker, accountID string) error {
	accounts, err := selected.DiscoverAccounts(ctx)
	if err != nil {
		return requestErrorf("futures authority could not be verified: %v", err)
	}
	for _, account := range accounts {
		if accountID != "" && account.ID != accountID {
			continue
		}
		if containsExecutionAuthority(account.MarketAuthorities, "FUTURES") {
			return nil
		}
	}
	return requestErrorf("REAL futures orders require FUTURES account authority")
}

func validatePredictionTradingAccount(ctx context.Context, selected broker.Broker, accountID string) error {
	accounts, err := selected.DiscoverAccounts(ctx)
	if err != nil {
		return requestErrorf("prediction account eligibility could not be verified: %v", err)
	}
	for _, account := range accounts {
		if accountID != "" && account.ID != accountID {
			continue
		}
		firm := ""
		if account.SecurityFirm != nil {
			firm = strings.ToUpper(strings.TrimSpace(*account.SecurityFirm))
		}
		if firm != "FUTUINC" {
			continue
		}
		if len(account.MarketAuthorities) == 0 || containsExecutionAuthority(account.MarketAuthorities, "US") {
			return nil
		}
	}
	return requestErrorf("prediction market requires an eligible Moomoo US account")
}

func containsExecutionAuthority(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

// PlaceExecutionOrder is the shared command boundary for manual and strategy
// orders. Every broker submission must pass through the pre-trade gateway.
func (s *Service) PlaceExecutionOrder(ctx context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
	if s.preTradeRisk != nil {
		decision := s.preTradeRisk.EvaluatePlaceOrder(ctx, command)
		if !decision.Allows() {
			return ExecutionOrder{}, RiskRejectedError{Decision: decision}
		}
	}
	order, err := s.orderGateway.PlaceOrder(ctx, command)
	if err != nil {
		return ExecutionOrder{}, err
	}
	return order, nil
}

func (s *Service) CancelExecutionOrder(ctx context.Context, id string) (ExecutionCommandResponse, error) {
	order, err := s.orderGateway.CancelOrder(ctx, strings.TrimSpace(id))
	if err != nil {
		return ExecutionCommandResponse{}, err
	}
	return executionCommandResponse("CANCEL", "cancel request submitted to broker", order), nil
}

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
	quantityMode := payload.QuantityMode
	if quantityMode == "" {
		quantityMode = broker.QuantityModeUnits
	}
	if productClass == broker.ProductClassOption || productClass == broker.ProductClassFuture {
		quantityMode = broker.QuantityModeContracts
	}
	if productClass == broker.ProductClassEventContract || orderKind == broker.OrderKindEventSingle {
		orderKind = broker.OrderKindEventSingle
		productClass = broker.ProductClassEventContract
		quantityMode = broker.QuantityModeAmount
		if err := normalizePredictionOrder(payload, instrument); err != nil {
			return "", "", "", err
		}
	} else if payload.Quantity <= 0 {
		return "", "", "", requestErrorf("quantity must be greater than 0")
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
	if payload.Amount == nil || *payload.Amount <= 0 {
		return requestErrorf("event-contract amount must be greater than 0")
	}
	predictionSide := strings.ToUpper(strings.TrimSpace(payload.PredictionSide))
	if predictionSide != "YES" && predictionSide != "NO" {
		return requestErrorf("predictionSide must be YES or NO")
	}
	if payload.Price == nil || *payload.Price < 0.01 || *payload.Price > 0.99 {
		return requestErrorf("event-contract price must be between 0.01 and 0.99")
	}
	payload.PredictionSide = predictionSide
	return nil
}

func validateExecutionPrices(payload ExecutionPlaceRequest, orderType string) error {
	if requiresLimitPrice(orderType) && (payload.Price == nil || *payload.Price <= 0) {
		return requestErrorf("order type %s requires price", orderType)
	}
	if requiresStopPrice(orderType) && (payload.StopPrice == nil || *payload.StopPrice <= 0) {
		return requestErrorf("order type %s requires stopPrice", orderType)
	}
	return nil
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
