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

	if internalOrderID := s.findPlacedOrderLocked(input); internalOrderID != "" {
		summary := s.mergePlacedOrderLocked(internalOrderID, input, submittedAt, now)
		return cloneExecutionOrderSummary(summary)
	}

	s.nextOrderSeq++
	s.persistSequenceLocked("orders", s.nextOrderSeq)
	internalOrderID := fmt.Sprintf("exec-%06d", s.nextOrderSeq)
	summary := newPlacedOrderSummary(internalOrderID, input, submittedAt, now)
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
	s.appendEventLocked(internalOrderID, nil, summary.Status, input.EventType, input.Payload, now)
	return cloneExecutionOrderSummary(summary)
}

func (s *executionOrderStore) findPlacedOrderLocked(input executionPlacedOrderRecord) string {
	if internalOrderID := strings.TrimSpace(input.InternalOrderID); internalOrderID != "" {
		if _, ok := s.orders[internalOrderID]; ok {
			return internalOrderID
		}
	}
	if internalOrderID := s.findClientOrderIDLocked(
		input.BrokerID, input.TradingEnvironment, input.AccountID, input.ClientOrderID,
	); internalOrderID != "" {
		return internalOrderID
	}
	return s.findInternalOrderIDLocked(
		input.BrokerID,
		input.AccountID,
		input.TradingEnvironment,
		input.Market,
		input.BrokerOrderID,
		stringPointerOrNil(input.BrokerOrderIDEx),
	)
}

func newPlacedOrderSummary(
	internalOrderID string,
	input executionPlacedOrderRecord,
	submittedAt string,
	now string,
) executionOrderSummaryResponse {
	rawBrokerStatus := strings.TrimSpace(input.Status)
	status := canonicalPlacedRecordStatus(rawBrokerStatus)
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
		OrderKind:          input.OrderKind,
		ProductClass:       input.ProductClass,
		QuantityMode:       input.QuantityMode,
		ClientOrderID:      stringPointerOrNil(input.ClientOrderID),
		PreviewID:          stringPointerOrNil(input.PreviewID),
		NormalizedRequest:  firstNonEmptyString(input.NormalizedRequest, "{}"),
		RequestedAmount:    cloneFloat64Pointer(input.RequestedAmount),
	}
	if summary.OrderKind == "" {
		summary.OrderKind = broker.OrderKindSingle
	}
	if summary.ProductClass == "" {
		summary.ProductClass = broker.ProductClassUnknown
	}
	if summary.QuantityMode == "" {
		summary.QuantityMode = broker.QuantityModeUnits
	}
	summary.Legs = executionOrderLegsFromIntents(internalOrderID, input.Legs, status, now)
	applyExecutionLegSnapshots(&summary, input.LegSnapshots, now)
	return summary
}

func (s *executionOrderStore) prepareSubmission(input executionPlacedOrderRecord) (executionOrderSummaryResponse, bool, error) {
	s.submissionMu.Lock()
	defer s.submissionMu.Unlock()

	s.mu.RLock()
	existingID := s.findClientOrderIDLocked(
		input.BrokerID, input.TradingEnvironment, input.AccountID, input.ClientOrderID,
	)
	if existingID != "" {
		existing := cloneExecutionOrderSummary(s.orders[existingID])
		s.mu.RUnlock()
		return existing, false, nil
	}
	s.mu.RUnlock()

	input.Status = trdsrv.OrderStatusSubmitting
	input.EventType = "COMMAND_SUBMISSION_PREPARED"
	input.Payload = map[string]any{
		"clientOrderId": input.ClientOrderID, "previewId": input.PreviewID,
		"normalizedRequest": input.NormalizedRequest,
	}
	prepared := s.recordPlacedOrder(input)
	if s.persistence != nil {
		if err := s.persistence.persistOrder(prepared); err != nil {
			return prepared, false, fmt.Errorf("persist order before broker submission: %w", err)
		}
	}
	return prepared, true, nil
}

func (s *executionOrderStore) markSubmissionUnknown(internalOrderID string, submitErr error) executionOrderSummaryResponse {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	summary, ok := s.orders[internalOrderID]
	if !ok {
		s.mu.Unlock()
		return executionOrderSummaryResponse{}
	}
	previousStatus := summary.Status
	summary.Status = trdsrv.OrderStatusSubmissionUnknown
	message := "broker submission result is unknown"
	if submitErr != nil {
		message = submitErr.Error()
	}
	summary.LastError = &message
	source := "broker.submit"
	summary.LastErrorSource = &source
	summary.UpdatedAt = now
	for index := range summary.Legs {
		summary.Legs[index].Status = trdsrv.OrderStatusSubmissionUnknown
		summary.Legs[index].UpdatedAt = now
	}
	s.orders[internalOrderID] = summary
	s.persistOrderLocked(summary)
	s.appendEventLocked(internalOrderID, &previousStatus, summary.Status, "COMMAND_SUBMISSION_UNKNOWN", map[string]any{
		"error": message, "retryAllowed": false,
	}, now)
	cloned := cloneExecutionOrderSummary(summary)
	s.mu.Unlock()
	if s.persistence != nil {
		if err := s.persistence.persistOrder(cloned); err != nil {
			log.Printf("JFTrade persist submission-unknown state failed: %v", err)
		}
	}
	return cloned
}

func (s *executionOrderStore) mergePlacedOrderLocked(internalOrderID string, input executionPlacedOrderRecord, submittedAt string, createdAt string) executionOrderSummaryResponse {
	summary := s.orders[internalOrderID]
	previousStatus := summary.Status

	mergePlacedOrderIdentity(&summary, input)
	mergePlacedOrderRequest(&summary, internalOrderID, input, createdAt)
	mergePlacedOrderStatus(&summary, input, submittedAt, createdAt)
	s.orders[internalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
	s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, input.EventType, input.Payload, createdAt)
	return summary
}

func mergePlacedOrderIdentity(summary *executionOrderSummaryResponse, input executionPlacedOrderRecord) {
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
}

func mergePlacedOrderRequest(
	summary *executionOrderSummaryResponse,
	internalOrderID string,
	input executionPlacedOrderRecord,
	createdAt string,
) {
	if input.RequestedQuantity > 0 {
		requestedQuantity := input.RequestedQuantity
		if float64PointersDiffer(summary.RequestedQuantity, &requestedQuantity) {
			summary.RequestedQuantity = &requestedQuantity
		}
	}
	if input.RequestedPrice != nil && float64PointersDiffer(summary.RequestedPrice, input.RequestedPrice) {
		summary.RequestedPrice = cloneFloat64Pointer(input.RequestedPrice)
	}
	if input.RequestedAmount != nil {
		summary.RequestedAmount = cloneFloat64Pointer(input.RequestedAmount)
	}
	if input.OrderKind != "" {
		summary.OrderKind = input.OrderKind
	}
	if input.ProductClass != "" {
		summary.ProductClass = input.ProductClass
	}
	if input.QuantityMode != "" {
		summary.QuantityMode = input.QuantityMode
	}
	if value := strings.TrimSpace(input.ClientOrderID); value != "" {
		summary.ClientOrderID = &value
	}
	if value := strings.TrimSpace(input.PreviewID); value != "" {
		summary.PreviewID = &value
	}
	if value := strings.TrimSpace(input.NormalizedRequest); value != "" {
		summary.NormalizedRequest = value
	}
	if len(input.Legs) > 0 {
		summary.Legs = executionOrderLegsFromIntents(internalOrderID, input.Legs, summary.Status, createdAt)
	}
	applyExecutionLegSnapshots(summary, input.LegSnapshots, createdAt)
	if stringPointersDiffer(summary.Remark, stringPointerOrNil(input.Remark)) {
		summary.Remark = stringPointerOrNil(input.Remark)
	}
}

func mergePlacedOrderStatus(
	summary *executionOrderSummaryResponse,
	input executionPlacedOrderRecord,
	submittedAt string,
	createdAt string,
) {
	if summary.SubmittedAt == nil && submittedAt != "" {
		summary.SubmittedAt = stringPointerOrNil(submittedAt)
	}
	if rawBrokerStatus := strings.TrimSpace(input.Status); rawBrokerStatus != "" {
		incomingStatus := canonicalPlacedRecordStatus(rawBrokerStatus)
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
	for index := range summary.Legs {
		if reconciled, accepted := trdsrv.ReconcileCanonicalOrderStatus(
			summary.Legs[index].Status,
			trdsrv.OrderStatusCancelRequested,
		); accepted {
			summary.Legs[index].Status = reconciled
		}
		summary.Legs[index].UpdatedAt = now
	}
	s.orders[internalOrderID] = summary
	s.persistOrderLocked(summary)
	s.appendEventLocked(internalOrderID, stringPointerOrNil(previousStatus), summary.Status, "COMMAND_CANCEL_ACCEPTED", payload, now)
	return cloneExecutionOrderSummary(summary), true
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

func (s *executionOrderStore) findClientOrderIDLocked(brokerID, tradingEnvironment, accountID, clientOrderID string) string {
	clientOrderID = strings.TrimSpace(clientOrderID)
	if clientOrderID == "" {
		return ""
	}
	for internalOrderID, order := range s.orders {
		if !strings.EqualFold(strings.TrimSpace(order.BrokerID), strings.TrimSpace(brokerID)) ||
			!strings.EqualFold(strings.TrimSpace(order.TradingEnvironment), strings.TrimSpace(tradingEnvironment)) ||
			strings.TrimSpace(order.AccountID) != strings.TrimSpace(accountID) ||
			!strings.EqualFold(strings.TrimSpace(derefString(order.ClientOrderID)), clientOrderID) {
			continue
		}
		return internalOrderID
	}
	return ""
}

func canonicalPlacedRecordStatus(status string) string {
	if trdsrv.CanonicalStoredOrderStatus(status) == trdsrv.OrderStatusSubmissionUnknown {
		return trdsrv.OrderStatusSubmissionUnknown
	}
	return trdsrv.CanonicalBrokerOrderStatus(status)
}

func executionOrderLegsFromIntents(internalOrderID string, intents []broker.OrderLegIntent, status, now string) []trdsrv.ExecutionOrderLeg {
	legs := make([]trdsrv.ExecutionOrderLeg, 0, len(intents))
	for index, intent := range intents {
		ratio := max(intent.Ratio, 1)
		legs = append(legs, trdsrv.ExecutionOrderLeg{
			ID: fmt.Sprintf("%s-leg-%03d", internalOrderID, index+1), InternalOrderID: internalOrderID,
			Index: index, InstrumentID: strings.TrimSpace(intent.InstrumentID),
			ProductClass: intent.ProductClass, Side: strings.ToUpper(strings.TrimSpace(intent.Side)),
			Ratio: ratio, PredictionSide: strings.ToUpper(strings.TrimSpace(intent.PredictionSide)),
			RequestedQuantity: cloneFloat64Pointer(intent.Quantity), RequestedAmount: cloneFloat64Pointer(intent.Amount),
			RequestedPrice: cloneFloat64Pointer(intent.Price), Status: status,
			FilledQuantity: new(0.0), FilledAmount: new(0.0), UpdatedAt: now, CreatedAt: now,
		})
	}
	return legs
}

func applyExecutionLegSnapshots(
	summary *executionOrderSummaryResponse,
	snapshots []broker.OrderLegSnapshot,
	now string,
) {
	if summary == nil || len(snapshots) == 0 {
		return
	}
	for index, snapshot := range snapshots {
		targetIndex := -1
		for legIndex := range summary.Legs {
			if strings.EqualFold(summary.Legs[legIndex].InstrumentID, snapshot.InstrumentID) {
				targetIndex = legIndex
				break
			}
		}
		if targetIndex < 0 && index < len(summary.Legs) {
			targetIndex = index
		}
		if targetIndex < 0 {
			ratio := max(1, snapshot.Ratio)
			summary.Legs = append(summary.Legs, trdsrv.ExecutionOrderLeg{
				ID:              fmt.Sprintf("%s-leg-%03d", summary.InternalOrderID, len(summary.Legs)+1),
				InternalOrderID: summary.InternalOrderID, Index: len(summary.Legs),
				InstrumentID: snapshot.InstrumentID, ProductClass: snapshot.ProductClass,
				Side: snapshot.Side, Ratio: ratio, PredictionSide: snapshot.PredictionSide,
				Status:    trdsrv.CanonicalStoredOrderStatus(snapshot.Status),
				UpdatedAt: now, CreatedAt: now,
			})
			targetIndex = len(summary.Legs) - 1
		}
		leg := &summary.Legs[targetIndex]
		if snapshot.BrokerLegID != "" {
			leg.BrokerLegID = stringPointerOrNil(snapshot.BrokerLegID)
		}
		if snapshot.ProductClass != "" {
			leg.ProductClass = snapshot.ProductClass
		}
		if snapshot.Side != "" {
			leg.Side = strings.ToUpper(strings.TrimSpace(snapshot.Side))
		}
		if snapshot.Ratio > 0 {
			leg.Ratio = snapshot.Ratio
		}
		if snapshot.PredictionSide != "" {
			leg.PredictionSide = strings.ToUpper(strings.TrimSpace(snapshot.PredictionSide))
		}
		if snapshot.RequestedQuantity != 0 {
			leg.RequestedQuantity = new(snapshot.RequestedQuantity)
		}
		if snapshot.RequestedAmount != 0 {
			leg.RequestedAmount = new(snapshot.RequestedAmount)
		}
		if snapshot.RequestedPrice != 0 {
			leg.RequestedPrice = new(snapshot.RequestedPrice)
		}
		if snapshot.Status != "" {
			status := trdsrv.CanonicalBrokerOrderStatus(snapshot.Status)
			if status == trdsrv.OrderStatusUnknown {
				status = trdsrv.CanonicalStoredOrderStatus(snapshot.Status)
			}
			leg.Status = status
		}
		leg.FilledQuantity = new(snapshot.FilledQuantity)
		leg.FilledAmount = new(snapshot.FilledAmount)
		if snapshot.AveragePrice != 0 {
			leg.AveragePrice = new(snapshot.AveragePrice)
		}
		if snapshot.Fees != 0 {
			leg.Fees = new(snapshot.Fees)
		}
		if snapshot.Payout != 0 {
			leg.Payout = new(snapshot.Payout)
		}
		leg.UpdatedAt = now
	}
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
		// Keep every persistence item on the single FIFO worker.  Blocking here
		// applies backpressure to the in-memory mutation while preserving the
		// order in which snapshots, events, and sequence updates are committed.
		s.persistenceQueue <- item
		s.persistenceMu.Unlock()
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
