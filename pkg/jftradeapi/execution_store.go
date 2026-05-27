package jftradeapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

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

func (s *executionOrderStore) listOrders() executionOrdersResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orders := make([]executionOrderSummaryResponse, 0, len(s.orders))
	for _, order := range s.orders {
		orders = append(orders, cloneExecutionOrderSummary(order))
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].UpdatedAt != orders[j].UpdatedAt {
			return orders[i].UpdatedAt > orders[j].UpdatedAt
		}
		if orders[i].CreatedAt != orders[j].CreatedAt {
			return orders[i].CreatedAt > orders[j].CreatedAt
		}
		return orders[i].InternalOrderID > orders[j].InternalOrderID
	})
	return executionOrdersResponse{Orders: orders}
}

func (s *executionOrderStore) orderEvents(internalOrderID string) executionOrderEventsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[internalOrderID]
	cloned := make([]executionOrderEventResponse, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, cloneExecutionOrderEvent(event))
	}
	return executionOrderEventsResponse{InternalOrderID: internalOrderID, Events: cloned}
}

func (s *executionOrderStore) order(internalOrderID string) (executionOrderSummaryResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[internalOrderID]
	if !ok {
		return executionOrderSummaryResponse{}, false
	}
	return cloneExecutionOrderSummary(order), true
}

func (s *executionOrderStore) recordPlacedOrder(input executionPlacedOrderRecord) executionOrderSummaryResponse {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	submittedAt := strings.TrimSpace(input.SubmittedAt)
	if submittedAt == "" {
		submittedAt = now
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if internalOrderID := s.findInternalOrderIDLocked(
		input.BrokerID,
		input.AccountID,
		input.TradingEnvironment,
		input.Market,
		input.BrokerOrderID,
		stringPointerOrNil(input.BrokerOrderIDEx),
	); internalOrderID != "" {
		summary := s.mergePlacedOrderLocked(internalOrderID, input, submittedAt, now)
		return cloneExecutionOrderSummary(summary)
	}

	s.nextOrderSeq++
	internalOrderID := fmt.Sprintf("exec-%06d", s.nextOrderSeq)
	requestedQuantity := input.RequestedQuantity
	filledQuantity := 0.0
	summary := executionOrderSummaryResponse{
		InternalOrderID:    internalOrderID,
		BrokerID:           strings.TrimSpace(input.BrokerID),
		BrokerOrderID:      stringPointerOrNil(input.BrokerOrderID),
		BrokerOrderIDEx:    stringPointerOrNil(input.BrokerOrderIDEx),
		TradingEnvironment: strings.TrimSpace(input.TradingEnvironment),
		AccountID:          strings.TrimSpace(input.AccountID),
		Market:             strings.TrimSpace(input.Market),
		Symbol:             stringPointerOrNil(input.Symbol),
		Side:               stringPointerOrNil(input.Side),
		OrderType:          stringPointerOrNil(input.OrderType),
		Status:             strings.TrimSpace(input.Status),
		RequestedQuantity:  &requestedQuantity,
		RequestedPrice:     cloneFloat64Pointer(input.RequestedPrice),
		FilledQuantity:     &filledQuantity,
		FilledAveragePrice: nil,
		Remark:             stringPointerOrNil(input.Remark),
		LastError:          nil,
		LastErrorCode:      nil,
		LastErrorSource:    nil,
		SubmittedAt:        stringPointerOrNil(submittedAt),
		UpdatedAt:          now,
		CreatedAt:          now,
	}
	if summary.Status == "" {
		summary.Status = "SUBMITTED"
	}
	if summary.BrokerID == "" {
		summary.BrokerID = "futu"
	}
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.appendEventLocked(internalOrderID, nil, summary.Status, input.EventType, input.Payload, now)
	return cloneExecutionOrderSummary(summary)
}

func (s *executionOrderStore) mergePlacedOrderLocked(internalOrderID string, input executionPlacedOrderRecord, submittedAt string, createdAt string) executionOrderSummaryResponse {
	summary := s.orders[internalOrderID]
	previousStatus := summary.Status

	if value := strings.TrimSpace(input.BrokerID); value != "" {
		summary.BrokerID = value
	}
	if stringPointersDiffer(summary.BrokerOrderID, stringPointerOrNil(input.BrokerOrderID)) {
		summary.BrokerOrderID = stringPointerOrNil(input.BrokerOrderID)
	}
	if stringPointersDiffer(summary.BrokerOrderIDEx, stringPointerOrNil(input.BrokerOrderIDEx)) {
		summary.BrokerOrderIDEx = stringPointerOrNil(input.BrokerOrderIDEx)
	}
	if value := strings.TrimSpace(input.TradingEnvironment); value != "" {
		summary.TradingEnvironment = value
	}
	if value := strings.TrimSpace(input.AccountID); value != "" {
		summary.AccountID = value
	}
	if value := strings.TrimSpace(input.Market); value != "" {
		summary.Market = value
	}
	if stringPointersDiffer(summary.Symbol, stringPointerOrNil(input.Symbol)) {
		summary.Symbol = stringPointerOrNil(input.Symbol)
	}
	if stringPointersDiffer(summary.Side, stringPointerOrNil(input.Side)) {
		summary.Side = stringPointerOrNil(input.Side)
	}
	if stringPointersDiffer(summary.OrderType, stringPointerOrNil(input.OrderType)) {
		summary.OrderType = stringPointerOrNil(input.OrderType)
	}
	if input.RequestedQuantity > 0 {
		requestedQuantity := input.RequestedQuantity
		if float64PointersDiffer(summary.RequestedQuantity, &requestedQuantity) {
			summary.RequestedQuantity = &requestedQuantity
		}
	}
	if input.RequestedPrice != nil && float64PointersDiffer(summary.RequestedPrice, input.RequestedPrice) {
		summary.RequestedPrice = cloneFloat64Pointer(input.RequestedPrice)
	}
	if stringPointersDiffer(summary.Remark, stringPointerOrNil(input.Remark)) {
		summary.Remark = stringPointerOrNil(input.Remark)
	}
	if summary.SubmittedAt == nil && submittedAt != "" {
		summary.SubmittedAt = stringPointerOrNil(submittedAt)
	}
	if summary.Status == "" {
		summary.Status = firstNonEmptyString(input.Status, "SUBMITTED")
	}
	if summary.UpdatedAt == "" {
		summary.UpdatedAt = createdAt
	}
	if summary.CreatedAt == "" {
		summary.CreatedAt = createdAt
	}

	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, input.EventType, input.Payload, createdAt)
	return summary
}

func (s *executionOrderStore) markCancelRequested(internalOrderID string, payload any) (executionOrderSummaryResponse, bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	summary, ok := s.orders[internalOrderID]
	if !ok {
		return executionOrderSummaryResponse{}, false
	}
	previousStatus := summary.Status
	summary.Status = "CANCEL_REQUESTED"
	summary.UpdatedAt = now
	s.orders[internalOrderID] = summary
	s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, "COMMAND_CANCEL_ACCEPTED", payload, now)
	return cloneExecutionOrderSummary(summary), true
}

func (s *executionOrderStore) upsertBrokerOrder(brokerID string, snapshot futu.BrokerOrderSnapshot, discoveredEventType string, updatedEventType string) (executionOrderSummaryResponse, *executionOrderEventResponse, bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	internalOrderID := s.findInternalOrderIDLocked(brokerID, snapshot.AccountID, snapshot.TradingEnvironment, snapshot.Market, snapshot.BrokerOrderID, snapshot.BrokerOrderIDEx)
	if internalOrderID == "" {
		s.nextOrderSeq++
		internalOrderID = fmt.Sprintf("exec-%06d", s.nextOrderSeq)
		requestedQuantity := snapshot.Quantity
		filledQuantity := cloneFloat64Pointer(snapshot.FilledQuantity)
		if filledQuantity == nil {
			zero := 0.0
			filledQuantity = &zero
		}
		summary := executionOrderSummaryResponse{
			InternalOrderID:    internalOrderID,
			BrokerID:           strings.TrimSpace(brokerID),
			BrokerOrderID:      stringPointerOrNil(snapshot.BrokerOrderID),
			BrokerOrderIDEx:    cloneStringPointer(snapshot.BrokerOrderIDEx),
			TradingEnvironment: snapshot.TradingEnvironment,
			AccountID:          snapshot.AccountID,
			Market:             snapshot.Market,
			Symbol:             stringPointerOrNil(snapshot.Symbol),
			Side:               stringPointerOrNil(snapshot.Side),
			OrderType:          stringPointerOrNil(snapshot.OrderType),
			Status:             firstNonEmptyString(snapshot.Status, "SUBMITTED"),
			RequestedQuantity:  &requestedQuantity,
			RequestedPrice:     cloneFloat64Pointer(snapshot.Price),
			FilledQuantity:     filledQuantity,
			FilledAveragePrice: cloneFloat64Pointer(snapshot.FilledAveragePrice),
			Remark:             cloneStringPointer(snapshot.Remark),
			LastError:          cloneStringPointer(snapshot.LastError),
			LastErrorCode:      nil,
			LastErrorSource:    executionStringPointerOrNil("broker.sync"),
			SubmittedAt:        stringPointerOrNil(firstNonEmptyString(snapshot.SubmittedAt, now)),
			UpdatedAt:          firstNonEmptyString(snapshot.UpdatedAt, now),
			CreatedAt:          now,
		}
		if summary.BrokerID == "" {
			summary.BrokerID = "futu"
		}
		s.orders[internalOrderID] = summary
		s.linkBrokerOrderLocked(summary)
		event := s.appendEventLocked(internalOrderID, nil, summary.Status, discoveredEventType, snapshot, firstNonEmptyString(summary.UpdatedAt, now))
		clonedEvent := cloneExecutionOrderEvent(event)
		return cloneExecutionOrderSummary(summary), &clonedEvent, true
	}

	summary := s.orders[internalOrderID]
	changed := false
	previousStatus := summary.Status
	if value := strings.TrimSpace(snapshot.Status); value != "" && value != summary.Status {
		summary.Status = value
		changed = true
	}
	if value := strings.TrimSpace(snapshot.Market); value != "" && value != summary.Market {
		summary.Market = value
		changed = true
	}
	if value := strings.TrimSpace(snapshot.AccountID); value != "" && value != summary.AccountID {
		summary.AccountID = value
		changed = true
	}
	if value := strings.TrimSpace(snapshot.TradingEnvironment); value != "" && value != summary.TradingEnvironment {
		summary.TradingEnvironment = value
		changed = true
	}
	if stringPointersDiffer(summary.Symbol, stringPointerOrNil(snapshot.Symbol)) {
		summary.Symbol = stringPointerOrNil(snapshot.Symbol)
		changed = true
	}
	if stringPointersDiffer(summary.Side, stringPointerOrNil(snapshot.Side)) {
		summary.Side = stringPointerOrNil(snapshot.Side)
		changed = true
	}
	if stringPointersDiffer(summary.OrderType, stringPointerOrNil(snapshot.OrderType)) {
		summary.OrderType = stringPointerOrNil(snapshot.OrderType)
		changed = true
	}
	if stringPointersDiffer(summary.BrokerOrderIDEx, snapshot.BrokerOrderIDEx) {
		summary.BrokerOrderIDEx = cloneStringPointer(snapshot.BrokerOrderIDEx)
		changed = true
	}
	if stringPointersDiffer(summary.Remark, snapshot.Remark) {
		summary.Remark = cloneStringPointer(snapshot.Remark)
		changed = true
	}
	if stringPointersDiffer(summary.LastError, snapshot.LastError) {
		summary.LastError = cloneStringPointer(snapshot.LastError)
		changed = true
	}
	if snapshot.LastError != nil {
		summary.LastErrorSource = executionStringPointerOrNil("broker.sync")
	} else {
		summary.LastErrorSource = nil
	}
	if snapshot.Quantity > 0 && float64PointersDiffer(summary.RequestedQuantity, &snapshot.Quantity) {
		requestedQuantity := snapshot.Quantity
		summary.RequestedQuantity = &requestedQuantity
		changed = true
	}
	if snapshot.Price != nil && float64PointersDiffer(summary.RequestedPrice, snapshot.Price) {
		summary.RequestedPrice = cloneFloat64Pointer(snapshot.Price)
		changed = true
	}
	if snapshot.FilledQuantity != nil && float64PointersDiffer(summary.FilledQuantity, snapshot.FilledQuantity) {
		summary.FilledQuantity = cloneFloat64Pointer(snapshot.FilledQuantity)
		changed = true
	}
	if snapshot.FilledAveragePrice != nil && float64PointersDiffer(summary.FilledAveragePrice, snapshot.FilledAveragePrice) {
		summary.FilledAveragePrice = cloneFloat64Pointer(snapshot.FilledAveragePrice)
		changed = true
	}
	if value := strings.TrimSpace(snapshot.SubmittedAt); value != "" && stringPointersDiffer(summary.SubmittedAt, &value) {
		summary.SubmittedAt = stringPointerOrNil(value)
		changed = true
	}
	if value := strings.TrimSpace(snapshot.UpdatedAt); value != "" && value != summary.UpdatedAt {
		summary.UpdatedAt = value
		changed = true
	}
	if !changed {
		return executionOrderSummaryResponse{}, nil, false
	}
	if summary.UpdatedAt == "" {
		summary.UpdatedAt = now
	}
	if summary.SubmittedAt == nil {
		summary.SubmittedAt = stringPointerOrNil(summary.CreatedAt)
	}
	if snapshot.LastError == nil {
		summary.LastErrorCode = nil
	}
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	event := s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, updatedEventType, snapshot, summary.UpdatedAt)
	clonedEvent := cloneExecutionOrderEvent(event)
	return cloneExecutionOrderSummary(summary), &clonedEvent, true
}

func (s *executionOrderStore) recordBrokerOrderFill(brokerID string, fill futu.BrokerOrderFillSnapshot) (executionOrderSummaryResponse, *executionOrderEventResponse, bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	fillKey := executionFillLookupKey(brokerID, fill.AccountID, fill.TradingEnvironment, fill.Market, fill.BrokerFillID, fill.BrokerFillIDEx)
	if fillKey != "" {
		if _, exists := s.seenFillKeys[fillKey]; exists {
			return executionOrderSummaryResponse{}, nil, false
		}
		s.seenFillKeys[fillKey] = struct{}{}
	}

	internalOrderID := s.findInternalOrderIDLocked(brokerID, fill.AccountID, fill.TradingEnvironment, fill.Market, fill.BrokerOrderID, fill.BrokerOrderIDEx)
	if internalOrderID == "" {
		s.nextOrderSeq++
		internalOrderID = fmt.Sprintf("exec-%06d", s.nextOrderSeq)
		filledQuantity := fill.FilledQuantity
		status := firstNonEmpty(derefString(fill.Status), "FILLED_PART")
		if status == "" {
			status = "FILLED_PART"
		}
		summary := executionOrderSummaryResponse{
			InternalOrderID:    internalOrderID,
			BrokerID:           strings.TrimSpace(brokerID),
			BrokerOrderID:      stringPointerOrNil(fill.BrokerOrderID),
			BrokerOrderIDEx:    cloneStringPointer(fill.BrokerOrderIDEx),
			TradingEnvironment: fill.TradingEnvironment,
			AccountID:          fill.AccountID,
			Market:             fill.Market,
			Symbol:             stringPointerOrNil(fill.Symbol),
			Side:               stringPointerOrNil(fill.Side),
			OrderType:          nil,
			Status:             status,
			RequestedQuantity:  nil,
			RequestedPrice:     nil,
			FilledQuantity:     &filledQuantity,
			FilledAveragePrice: cloneFloat64Pointer(fill.FillPrice),
			Remark:             nil,
			LastError:          nil,
			LastErrorCode:      nil,
			LastErrorSource:    executionStringPointerOrNil("broker.push"),
			SubmittedAt:        stringPointerOrNil(firstNonEmptyString(fill.FilledAt, now)),
			UpdatedAt:          firstNonEmptyString(fill.FilledAt, now),
			CreatedAt:          now,
		}
		if summary.BrokerID == "" {
			summary.BrokerID = "futu"
		}
		s.orders[internalOrderID] = summary
		s.linkBrokerOrderLocked(summary)
		event := s.appendEventLocked(internalOrderID, nil, summary.Status, "BROKER_FILL_RECEIVED", fill, firstNonEmptyString(summary.UpdatedAt, now))
		clonedEvent := cloneExecutionOrderEvent(event)
		return cloneExecutionOrderSummary(summary), &clonedEvent, true
	}

	summary := s.orders[internalOrderID]
	previousStatus := summary.Status
	previousFilled := optionalFloat64(summary.FilledQuantity)
	previousAverage := optionalFloat64(summary.FilledAveragePrice)
	newFilled := previousFilled + fill.FilledQuantity
	filledAverage := previousAverage
	if fill.FillPrice != nil && newFilled > 0 {
		filledAverage = ((previousAverage * previousFilled) + (*fill.FillPrice * fill.FilledQuantity)) / newFilled
	}
	status := summary.Status
	if fillStatus := strings.TrimSpace(derefString(fill.Status)); fillStatus != "" {
		status = fillStatus
	} else if summary.RequestedQuantity != nil && newFilled >= *summary.RequestedQuantity && *summary.RequestedQuantity > 0 {
		status = "FILLED_ALL"
	} else {
		status = "FILLED_PART"
	}
	filledQuantity := newFilled
	summary.FilledQuantity = &filledQuantity
	if fill.FillPrice != nil {
		summary.FilledAveragePrice = &filledAverage
	}
	if strings.TrimSpace(fill.Symbol) != "" {
		summary.Symbol = stringPointerOrNil(fill.Symbol)
	}
	if strings.TrimSpace(fill.Side) != "" {
		summary.Side = stringPointerOrNil(fill.Side)
	}
	if summary.BrokerOrderIDEx == nil {
		summary.BrokerOrderIDEx = cloneStringPointer(fill.BrokerOrderIDEx)
	}
	if strings.TrimSpace(fill.Market) != "" {
		summary.Market = fill.Market
	}
	if strings.TrimSpace(fill.AccountID) != "" {
		summary.AccountID = fill.AccountID
	}
	if strings.TrimSpace(fill.TradingEnvironment) != "" {
		summary.TradingEnvironment = fill.TradingEnvironment
	}
	summary.LastError = nil
	summary.LastErrorCode = nil
	summary.LastErrorSource = nil
	summary.Status = status
	summary.UpdatedAt = firstNonEmptyString(fill.FilledAt, now)
	if summary.SubmittedAt == nil {
		summary.SubmittedAt = stringPointerOrNil(firstNonEmptyString(fill.FilledAt, now))
	}
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	event := s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, "BROKER_FILL_RECEIVED", fill, summary.UpdatedAt)
	clonedEvent := cloneExecutionOrderEvent(event)
	return cloneExecutionOrderSummary(summary), &clonedEvent, previousStatus != summary.Status || newFilled != previousFilled
}

func (s *executionOrderStore) appendEventLocked(internalOrderID string, previousStatus *string, nextStatus string, eventType string, payload any, createdAt string) executionOrderEventResponse {
	s.nextEventSeq++
	event := executionOrderEventResponse{
		ID:              fmt.Sprintf("evt-%06d", s.nextEventSeq),
		InternalOrderID: internalOrderID,
		EventType:       strings.TrimSpace(eventType),
		PreviousStatus:  cloneStringPointer(previousStatus),
		NextStatus:      strings.TrimSpace(nextStatus),
		PayloadJSON:     marshalExecutionPayload(payload),
		CreatedAt:       createdAt,
	}
	s.events[internalOrderID] = append(s.events[internalOrderID], event)
	return event
}

func (s *executionOrderStore) findInternalOrderIDLocked(brokerID string, accountID string, tradingEnvironment string, market string, brokerOrderID string, brokerOrderIDEx *string) string {
	if key := executionBrokerLookupKey(brokerID, tradingEnvironment, accountID, market, brokerOrderID); key != "" {
		if internalOrderID, ok := s.brokerOrderIndex[key]; ok {
			return internalOrderID
		}
	}
	if key := executionBrokerLookupKey(brokerID, tradingEnvironment, accountID, market, derefString(brokerOrderIDEx)); key != "" {
		if internalOrderID, ok := s.brokerOrderExIndex[key]; ok {
			return internalOrderID
		}
	}
	return ""
}

func (s *executionOrderStore) linkBrokerOrderLocked(order executionOrderSummaryResponse) {
	if key := executionBrokerLookupKey(order.BrokerID, order.TradingEnvironment, order.AccountID, order.Market, derefString(order.BrokerOrderID)); key != "" {
		s.brokerOrderIndex[key] = order.InternalOrderID
	}
	if key := executionBrokerLookupKey(order.BrokerID, order.TradingEnvironment, order.AccountID, order.Market, derefString(order.BrokerOrderIDEx)); key != "" {
		s.brokerOrderExIndex[key] = order.InternalOrderID
	}
}

func executionBrokerLookupKey(brokerID string, tradingEnvironment string, accountID string, market string, brokerOrderID string) string {
	brokerOrderID = strings.TrimSpace(brokerOrderID)
	if brokerOrderID == "" {
		return ""
	}
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(brokerID)),
		strings.ToUpper(strings.TrimSpace(tradingEnvironment)),
		strings.TrimSpace(accountID),
		strings.ToUpper(strings.TrimSpace(market)),
		brokerOrderID,
	}, "|")
}

func executionFillLookupKey(brokerID string, accountID string, tradingEnvironment string, market string, brokerFillID string, brokerFillIDEx *string) string {
	fillID := strings.TrimSpace(brokerFillID)
	if fillID == "" {
		fillID = strings.TrimSpace(derefString(brokerFillIDEx))
	}
	if fillID == "" {
		return ""
	}
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(brokerID)),
		strings.ToUpper(strings.TrimSpace(tradingEnvironment)),
		strings.TrimSpace(accountID),
		strings.ToUpper(strings.TrimSpace(market)),
		fillID,
	}, "|")
}

func marshalExecutionPayload(payload any) string {
	if payload == nil {
		return "{}"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func cloneExecutionOrderSummary(in executionOrderSummaryResponse) executionOrderSummaryResponse {
	return executionOrderSummaryResponse{
		InternalOrderID:    in.InternalOrderID,
		BrokerID:           in.BrokerID,
		BrokerOrderID:      cloneStringPointer(in.BrokerOrderID),
		BrokerOrderIDEx:    cloneStringPointer(in.BrokerOrderIDEx),
		TradingEnvironment: in.TradingEnvironment,
		AccountID:          in.AccountID,
		Market:             in.Market,
		Symbol:             cloneStringPointer(in.Symbol),
		Side:               cloneStringPointer(in.Side),
		OrderType:          cloneStringPointer(in.OrderType),
		Status:             in.Status,
		RequestedQuantity:  cloneFloat64Pointer(in.RequestedQuantity),
		RequestedPrice:     cloneFloat64Pointer(in.RequestedPrice),
		FilledQuantity:     cloneFloat64Pointer(in.FilledQuantity),
		FilledAveragePrice: cloneFloat64Pointer(in.FilledAveragePrice),
		Remark:             cloneStringPointer(in.Remark),
		LastError:          cloneStringPointer(in.LastError),
		LastErrorCode:      cloneStringPointer(in.LastErrorCode),
		LastErrorSource:    cloneStringPointer(in.LastErrorSource),
		SubmittedAt:        cloneStringPointer(in.SubmittedAt),
		UpdatedAt:          in.UpdatedAt,
		CreatedAt:          in.CreatedAt,
	}
}

func cloneExecutionOrderEvent(in executionOrderEventResponse) executionOrderEventResponse {
	return executionOrderEventResponse{
		ID:              in.ID,
		InternalOrderID: in.InternalOrderID,
		EventType:       in.EventType,
		PreviousStatus:  cloneStringPointer(in.PreviousStatus),
		NextStatus:      in.NextStatus,
		PayloadJSON:     in.PayloadJSON,
		CreatedAt:       in.CreatedAt,
	}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringPointersDiffer(left *string, right *string) bool {
	return derefString(left) != derefString(right)
}

func float64PointersDiffer(left *float64, right *float64) bool {
	return optionalFloat64(left) != optionalFloat64(right)
}

func optionalFloat64(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func executionStringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
