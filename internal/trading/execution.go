package trading

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
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
