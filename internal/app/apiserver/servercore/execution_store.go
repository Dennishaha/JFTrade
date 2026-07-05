package servercore

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *executionOrderStore) listOrders() executionOrdersResponse {
	return s.listOrdersFiltered(executionOrderListFilter{})
}

func (s *executionOrderStore) listOrdersFiltered(filter executionOrderListFilter) executionOrdersResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orders := make([]executionOrderSummaryResponse, 0, len(s.orders))
	for _, order := range s.orders {
		if !executionOrderMatchesListFilter(order, filter) {
			continue
		}
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

func executionOrderMatchesListFilter(order executionOrderSummaryResponse, filter executionOrderListFilter) bool {
	if filter.BrokerID != "" && !strings.EqualFold(strings.TrimSpace(order.BrokerID), filter.BrokerID) {
		return false
	}
	if filter.TradingEnvironment != "" && !strings.EqualFold(strings.TrimSpace(order.TradingEnvironment), filter.TradingEnvironment) {
		return false
	}
	if filter.AccountID != "" && strings.TrimSpace(order.AccountID) != filter.AccountID {
		return false
	}
	if filter.Market != "" && !strings.EqualFold(strings.TrimSpace(order.Market), filter.Market) {
		return false
	}
	return true
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
	s.persistSequenceLocked("orders", s.nextOrderSeq)
	internalOrderID := fmt.Sprintf("exec-%06d", s.nextOrderSeq)
	rawBrokerStatus := strings.TrimSpace(input.Status)
	status := trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus)
	if rawBrokerStatus == "" {
		status = trdsrv.OrderStatusSubmitted
	}
	summary := executionOrderSummaryResponse{
		InternalOrderID:    internalOrderID,
		BrokerID:           strings.TrimSpace(input.BrokerID),
		BrokerOrderID:      stringPointerOrNil(input.BrokerOrderID),
		BrokerOrderIDEx:    stringPointerOrNil(input.BrokerOrderIDEx),
		Source:             "system",
		SourceDetail:       "command.place",
		TradingEnvironment: strings.TrimSpace(input.TradingEnvironment),
		AccountID:          strings.TrimSpace(input.AccountID),
		Market:             strings.TrimSpace(input.Market),
		Symbol:             stringPointerOrNil(input.Symbol),
		Side:               stringPointerOrNil(input.Side),
		OrderType:          stringPointerOrNil(input.OrderType),
		Status:             status,
		RawBrokerStatus:    stringPointerOrNil(rawBrokerStatus),
		RequestedQuantity:  new(input.RequestedQuantity),
		RequestedPrice:     cloneFloat64Pointer(input.RequestedPrice),
		FilledQuantity:     new(0.0),
		FilledAveragePrice: nil,
		Remark:             stringPointerOrNil(input.Remark),
		LastError:          nil,
		LastErrorCode:      nil,
		LastErrorSource:    nil,
		SubmittedAt:        stringPointerOrNil(submittedAt),
		UpdatedAt:          now,
		CreatedAt:          now,
	}
	if summary.BrokerID == "" {
		summary.BrokerID = "futu"
	}
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
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
	if rawBrokerStatus := strings.TrimSpace(input.Status); rawBrokerStatus != "" {
		incomingStatus := trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus)
		if reconciled, accepted := trdsrv.ReconcileCanonicalOrderStatus(summary.Status, incomingStatus); accepted {
			summary.Status = reconciled
			summary.RawBrokerStatus = stringPointerOrNil(rawBrokerStatus)
		}
	} else if summary.Status == "" || summary.Status == trdsrv.OrderStatusUnknown {
		summary.Status = trdsrv.OrderStatusSubmitted
	}
	if summary.UpdatedAt == "" {
		summary.UpdatedAt = createdAt
	}
	if summary.CreatedAt == "" {
		summary.CreatedAt = createdAt
	}
	if summary.Source == "" || summary.Source == "broker" {
		summary.Source = "system"
	}
	if summary.SourceDetail == "" || strings.HasPrefix(summary.SourceDetail, "broker.") {
		summary.SourceDetail = "command.place"
	}

	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
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
	if reconciled, accepted := trdsrv.ReconcileCanonicalOrderStatus(summary.Status, trdsrv.OrderStatusCancelRequested); accepted {
		summary.Status = reconciled
	}
	summary.UpdatedAt = now
	s.orders[internalOrderID] = summary
	s.persistOrderLocked(summary)
	s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, "COMMAND_CANCEL_ACCEPTED", payload, now)
	return cloneExecutionOrderSummary(summary), true
}

func (s *executionOrderStore) upsertBrokerOrderWithSource(brokerID string, snapshot broker.OrderSnapshot, discoveredEventType string, updatedEventType string, source string, sourceDetail string) (executionOrderSummaryResponse, *executionOrderEventResponse, bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	internalOrderID := s.findInternalOrderIDLocked(brokerID, snapshot.AccountID, snapshot.TradingEnvironment, snapshot.Market, snapshot.BrokerOrderID, snapshot.BrokerOrderIDEx)
	if internalOrderID == "" {
		s.nextOrderSeq++
		s.persistSequenceLocked("orders", s.nextOrderSeq)
		internalOrderID = fmt.Sprintf("exec-%06d", s.nextOrderSeq)
		filledQuantity := cloneFloat64Pointer(snapshot.FilledQuantity)
		if filledQuantity == nil {
			filledQuantity = new(0.0)
		}
		rawBrokerStatus := strings.TrimSpace(snapshot.Status)
		summary := executionOrderSummaryResponse{
			InternalOrderID:    internalOrderID,
			BrokerID:           strings.TrimSpace(brokerID),
			BrokerOrderID:      stringPointerOrNil(snapshot.BrokerOrderID),
			BrokerOrderIDEx:    cloneStringPointer(snapshot.BrokerOrderIDEx),
			Source:             firstNonEmptyString(source, "broker"),
			SourceDetail:       firstNonEmptyString(sourceDetail, "broker.current"),
			TradingEnvironment: snapshot.TradingEnvironment,
			AccountID:          snapshot.AccountID,
			Market:             snapshot.Market,
			Symbol:             stringPointerOrNil(snapshot.Symbol),
			Side:               stringPointerOrNil(snapshot.Side),
			OrderType:          stringPointerOrNil(snapshot.OrderType),
			Status:             trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus),
			RawBrokerStatus:    stringPointerOrNil(rawBrokerStatus),
			RequestedQuantity:  new(snapshot.Quantity),
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
		s.persistOrderLocked(summary)
		event := s.appendEventLocked(internalOrderID, nil, summary.Status, discoveredEventType, snapshot, firstNonEmptyString(summary.UpdatedAt, now))
		return cloneExecutionOrderSummary(summary), new(cloneExecutionOrderEvent(event)), true
	}

	summary := s.orders[internalOrderID]
	if brokerOrderSnapshotStale(summary, snapshot) {
		return executionOrderSummaryResponse{}, nil, false
	}
	changed := false
	previousStatus := summary.Status
	if rawBrokerStatus := strings.TrimSpace(snapshot.Status); rawBrokerStatus != "" {
		incomingStatus := trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus)
		if reconciled, accepted := trdsrv.ReconcileCanonicalOrderStatus(summary.Status, incomingStatus); accepted {
			if summary.Status != reconciled {
				summary.Status = reconciled
				changed = true
			}
			if stringPointersDiffer(summary.RawBrokerStatus, &rawBrokerStatus) {
				summary.RawBrokerStatus = stringPointerOrNil(rawBrokerStatus)
				changed = true
			}
		}
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
		summary.RequestedQuantity = new(snapshot.Quantity)
		changed = true
	}
	if snapshot.Price != nil && float64PointersDiffer(summary.RequestedPrice, snapshot.Price) {
		summary.RequestedPrice = cloneFloat64Pointer(snapshot.Price)
		changed = true
	}
	if snapshot.FilledQuantity != nil && *snapshot.FilledQuantity >= optionalFloat64(summary.FilledQuantity) && float64PointersDiffer(summary.FilledQuantity, snapshot.FilledQuantity) {
		summary.FilledQuantity = cloneFloat64Pointer(snapshot.FilledQuantity)
		changed = true
	}
	if snapshot.FilledAveragePrice != nil && optionalFloat64(snapshot.FilledQuantity) >= optionalFloat64(summary.FilledQuantity) && float64PointersDiffer(summary.FilledAveragePrice, snapshot.FilledAveragePrice) {
		summary.FilledAveragePrice = cloneFloat64Pointer(snapshot.FilledAveragePrice)
		changed = true
	}
	if value := strings.TrimSpace(snapshot.SubmittedAt); value != "" && stringPointersDiffer(summary.SubmittedAt, &value) {
		summary.SubmittedAt = stringPointerOrNil(value)
		changed = true
	}
	if value := strings.TrimSpace(snapshot.UpdatedAt); value != "" {
		if executionTimestampAdvances(summary.UpdatedAt, value) || (changed && summary.UpdatedAt != value) {
			summary.UpdatedAt = value
			changed = true
		}
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
	if summary.Source == "" {
		summary.Source = firstNonEmptyString(source, "broker")
	}
	if summary.SourceDetail == "" {
		summary.SourceDetail = firstNonEmptyString(sourceDetail, "broker.current")
	}
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
	event := s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, updatedEventType, snapshot, summary.UpdatedAt)
	return cloneExecutionOrderSummary(summary), new(cloneExecutionOrderEvent(event)), true
}

func executionTimestampAdvances(current, incoming string) bool {
	current = strings.TrimSpace(current)
	incoming = strings.TrimSpace(incoming)
	if incoming == "" || incoming == current {
		return false
	}
	if current == "" {
		return true
	}
	currentTime, currentErr := time.Parse(time.RFC3339Nano, current)
	incomingTime, incomingErr := time.Parse(time.RFC3339Nano, incoming)
	if currentErr != nil || incomingErr != nil {
		return true
	}
	return incomingTime.After(currentTime)
}

func brokerOrderSnapshotStale(current executionOrderSummaryResponse, snapshot broker.OrderSnapshot) bool {
	currentTimeText := strings.TrimSpace(current.UpdatedAt)
	incomingTimeText := strings.TrimSpace(snapshot.UpdatedAt)
	if currentTimeText == "" || incomingTimeText == "" || currentTimeText == incomingTimeText {
		return false
	}
	currentTime, currentErr := time.Parse(time.RFC3339Nano, currentTimeText)
	incomingTime, incomingErr := time.Parse(time.RFC3339Nano, incomingTimeText)
	if currentErr != nil || incomingErr != nil || !incomingTime.Before(currentTime) {
		return false
	}
	if snapshot.FilledQuantity != nil && *snapshot.FilledQuantity > optionalFloat64(current.FilledQuantity) {
		return false
	}
	if rawStatus := strings.TrimSpace(snapshot.Status); rawStatus != "" {
		incomingStatus := trdsrv.CanonicalBrokerOrderStatus(rawStatus)
		if reconciled, accepted := trdsrv.ReconcileCanonicalOrderStatus(current.Status, incomingStatus); accepted && reconciled != current.Status {
			return false
		}
	}
	return true
}

func (s *executionOrderStore) recordBrokerOrderFill(brokerID string, fill broker.OrderFillSnapshot) (executionOrderSummaryResponse, *executionOrderEventResponse, bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	fillKey := executionFillLookupKey(brokerID, fill.AccountID, fill.TradingEnvironment, fill.Market, fill.BrokerFillID, fill.BrokerFillIDEx)
	if fillKey != "" {
		if _, exists := s.seenFillKeys[fillKey]; exists {
			return executionOrderSummaryResponse{}, nil, false
		}
		s.seenFillKeys[fillKey] = now
		s.persistSeenFillKeyLocked(fillKey, now)
	}

	internalOrderID := s.findInternalOrderIDLocked(brokerID, fill.AccountID, fill.TradingEnvironment, fill.Market, fill.BrokerOrderID, fill.BrokerOrderIDEx)
	if internalOrderID == "" {
		s.nextOrderSeq++
		s.persistSequenceLocked("orders", s.nextOrderSeq)
		internalOrderID = fmt.Sprintf("exec-%06d", s.nextOrderSeq)
		rawBrokerStatus := strings.TrimSpace(derefString(fill.Status))
		status := trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus)
		if status == trdsrv.OrderStatusUnknown {
			status = trdsrv.OrderStatusPartiallyFilled
		}
		summary := executionOrderSummaryResponse{
			InternalOrderID:    internalOrderID,
			BrokerID:           strings.TrimSpace(brokerID),
			BrokerOrderID:      stringPointerOrNil(fill.BrokerOrderID),
			BrokerOrderIDEx:    cloneStringPointer(fill.BrokerOrderIDEx),
			Source:             "broker",
			SourceDetail:       "broker.fill",
			TradingEnvironment: fill.TradingEnvironment,
			AccountID:          fill.AccountID,
			Market:             fill.Market,
			Symbol:             stringPointerOrNil(fill.Symbol),
			Side:               stringPointerOrNil(fill.Side),
			OrderType:          nil,
			Status:             status,
			RawBrokerStatus:    stringPointerOrNil(rawBrokerStatus),
			RequestedQuantity:  nil,
			RequestedPrice:     nil,
			FilledQuantity:     new(fill.FilledQuantity),
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
		s.persistOrderLocked(summary)
		event := s.appendEventLocked(internalOrderID, nil, summary.Status, "BROKER_FILL_RECEIVED", fill, firstNonEmptyString(summary.UpdatedAt, now))
		return cloneExecutionOrderSummary(summary), new(cloneExecutionOrderEvent(event)), true
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
	rawBrokerStatus := strings.TrimSpace(derefString(fill.Status))
	status := trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus)
	if status == trdsrv.OrderStatusUnknown {
		status = trdsrv.OrderStatusPartiallyFilled
		if summary.RequestedQuantity != nil && newFilled >= *summary.RequestedQuantity && *summary.RequestedQuantity > 0 {
			status = trdsrv.OrderStatusFilled
		}
	}
	summary.FilledQuantity = new(newFilled)
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
	if reconciled, accepted := trdsrv.ReconcileCanonicalOrderStatus(summary.Status, status); accepted {
		summary.Status = reconciled
		if rawBrokerStatus != "" {
			summary.RawBrokerStatus = stringPointerOrNil(rawBrokerStatus)
		}
	}
	summary.UpdatedAt = firstNonEmptyString(fill.FilledAt, now)
	if summary.SubmittedAt == nil {
		summary.SubmittedAt = stringPointerOrNil(firstNonEmptyString(fill.FilledAt, now))
	}
	if summary.Source == "" {
		summary.Source = "broker"
	}
	if summary.SourceDetail == "" {
		summary.SourceDetail = "broker.fill"
	}
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
	event := s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, "BROKER_FILL_RECEIVED", fill, summary.UpdatedAt)
	return cloneExecutionOrderSummary(summary), new(cloneExecutionOrderEvent(event)), previousStatus != summary.Status || newFilled != previousFilled
}

func (s *executionOrderStore) appendEventLocked(internalOrderID string, previousStatus *string, nextStatus string, eventType string, payload any, createdAt string) executionOrderEventResponse {
	s.nextEventSeq++
	s.persistSequenceLocked("events", s.nextEventSeq)
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
	s.persistEventLocked(event)
	return event
}

func (s *executionOrderStore) persistOrderLocked(order executionOrderSummaryResponse) {
	s.enqueuePersistence(executionPersistenceItem{kind: "order", order: cloneExecutionOrderSummary(order)})
}

func (s *executionOrderStore) persistEventLocked(event executionOrderEventResponse) {
	s.enqueuePersistence(executionPersistenceItem{kind: "event", event: cloneExecutionOrderEvent(event)})
}

func (s *executionOrderStore) persistSeenFillKeyLocked(fillKey string, createdAt string) {
	s.enqueuePersistence(executionPersistenceItem{kind: "fill", fillKey: strings.TrimSpace(fillKey), createdAt: createdAt})
}

func (s *executionOrderStore) persistSequenceLocked(name string, value uint64) {
	s.enqueuePersistence(executionPersistenceItem{kind: "sequence", seqName: name, seqValue: value})
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

func (s *executionOrderStore) startPersistenceWorker() {
	if s == nil || s.persistence == nil {
		return
	}
	s.persistenceMu.Lock()
	defer s.persistenceMu.Unlock()
	if s.persistenceQueue != nil {
		return
	}
	s.persistenceQueue = make(chan executionPersistenceItem, defaultExecutionPersistenceQueueSize)
	s.persistenceWG.Add(1)
	go s.runPersistenceWorker(s.persistenceQueue)
}

func (s *executionOrderStore) runPersistenceWorker(queue <-chan executionPersistenceItem) {
	defer s.persistenceWG.Done()
	for item := range queue {
		s.writePersistenceItem(item)
	}
}

func (s *executionOrderStore) enqueuePersistence(item executionPersistenceItem) {
	if s == nil || s.persistence == nil {
		return
	}
	s.persistenceMu.Lock()
	if s.persistenceClosed || s.persistenceQueue == nil {
		s.persistenceMu.Unlock()
		return
	}
	select {
	case s.persistenceQueue <- item:
		s.persistenceMu.Unlock()
	default:
		s.persistenceWG.Add(1)
		s.persistenceMu.Unlock()
		go func() {
			defer s.persistenceWG.Done()
			s.writePersistenceItem(item)
		}()
	}
}

func (s *executionOrderStore) writePersistenceItem(item executionPersistenceItem) {
	if s == nil || s.persistence == nil {
		return
	}
	var err error
	switch item.kind {
	case "order":
		err = s.persistence.persistOrder(item.order)
	case "event":
		err = s.persistence.persistEvent(item.event)
	case "fill":
		err = s.persistence.persistSeenFillKey(item.fillKey, item.createdAt)
	case "sequence":
		err = s.persistence.persistSequence(item.seqName, item.seqValue)
	case "deleteSeenFillsBefore":
		var cutoff time.Time
		cutoff, err = time.Parse(time.RFC3339Nano, item.cutoff)
		if err == nil {
			err = s.persistence.deleteSeenFillKeysBefore(cutoff)
		}
	}
	if err != nil {
		log.Printf("JFTrade execution order persistence degraded (%s): %v", item.kind, err)
	}
}

func (s *executionOrderStore) configureSeenFillRetention(days int) {
	if s == nil {
		return
	}
	normalized := days
	if normalized < 1 {
		normalized = 90
	}
	if normalized > 3650 {
		normalized = 3650
	}
	cutoff := time.Now().UTC().Add(-time.Duration(normalized) * 24 * time.Hour)

	s.mu.Lock()
	s.seenFillRetentionDays = normalized
	for key, createdAt := range s.seenFillKeys {
		if executionTimestampBefore(createdAt, cutoff) {
			delete(s.seenFillKeys, key)
		}
	}
	s.enqueuePersistence(executionPersistenceItem{
		kind:   "deleteSeenFillsBefore",
		cutoff: cutoff.Format(time.RFC3339Nano),
	})
	s.mu.Unlock()
}

func executionTimestampBefore(value string, cutoff time.Time) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return false
	}
	return parsed.Before(cutoff)
}

func (s *executionOrderStore) Close() error {
	if s == nil {
		return nil
	}
	s.persistenceMu.Lock()
	if s.persistenceClosed {
		s.persistenceMu.Unlock()
		return nil
	}
	s.persistenceClosed = true
	if s.persistenceQueue != nil {
		close(s.persistenceQueue)
	}
	s.persistenceMu.Unlock()

	s.persistenceWG.Wait()
	if s.persistence != nil {
		return s.persistence.Close()
	}
	return nil
}
