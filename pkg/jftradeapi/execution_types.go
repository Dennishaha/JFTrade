package jftradeapi

import (
	"sync"
)

// brokerOrderCommandResponse is the JSON response for a broker command (place/cancel).
type brokerOrderCommandResponse struct {
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

type executionOrderSummaryResponse struct {
	InternalOrderID    string   `json:"internalOrderId"`
	BrokerID           string   `json:"brokerId"`
	BrokerOrderID      *string  `json:"brokerOrderId"`
	BrokerOrderIDEx    *string  `json:"brokerOrderIdEx"`
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

type executionOrderEventResponse struct {
	ID              string  `json:"id"`
	InternalOrderID string  `json:"internalOrderId"`
	EventType       string  `json:"eventType"`
	PreviousStatus  *string `json:"previousStatus"`
	NextStatus      string  `json:"nextStatus"`
	PayloadJSON     string  `json:"payloadJson"`
	CreatedAt       string  `json:"createdAt"`
}

type executionOrdersResponse struct {
	Orders []executionOrderSummaryResponse `json:"orders"`
}

type executionOrderEventsResponse struct {
	InternalOrderID string                        `json:"internalOrderId"`
	Events          []executionOrderEventResponse `json:"events"`
}

type executionPlacedOrderRecord struct {
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

type executionOrderStore struct {
	mu                 sync.RWMutex
	nextOrderSeq       uint64
	nextEventSeq       uint64
	orders             map[string]executionOrderSummaryResponse
	events             map[string][]executionOrderEventResponse
	brokerOrderIndex   map[string]string
	brokerOrderExIndex map[string]string
	seenFillKeys       map[string]struct{}
}

func newExecutionOrderStore() *executionOrderStore {
	return &executionOrderStore{
		orders:             make(map[string]executionOrderSummaryResponse),
		events:             make(map[string][]executionOrderEventResponse),
		brokerOrderIndex:   make(map[string]string),
		brokerOrderExIndex: make(map[string]string),
		seenFillKeys:       make(map[string]struct{}),
	}
}
