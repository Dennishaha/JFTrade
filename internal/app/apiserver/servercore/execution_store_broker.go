package servercore

import (
	"fmt"
	"strings"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *executionOrderStore) upsertBrokerOrderWithSource(brokerID string, snapshot broker.OrderSnapshot, discoveredEventType string, updatedEventType string, source string, sourceDetail string) (executionOrderSummaryResponse, *executionOrderEventResponse, bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	internalOrderID := s.findInternalOrderIDLocked(brokerID, snapshot.AccountID, snapshot.TradingEnvironment, snapshot.Market, snapshot.BrokerOrderID, snapshot.BrokerOrderIDEx)
	if internalOrderID == "" {
		summary := s.brokerOrderSummaryFromSnapshot(s.allocateInternalOrderIDLocked(), brokerID, snapshot, now, source, sourceDetail)
		order, event := s.persistBrokerOrderLocked(summary, nil, discoveredEventType, snapshot, firstNonEmptyString(summary.UpdatedAt, now))
		return order, event, true
	}

	summary := s.orders[internalOrderID]
	if brokerOrderSnapshotStale(summary, snapshot) {
		return executionOrderSummaryResponse{}, nil, false
	}
	previousStatus := summary.Status
	changed := applyBrokerOrderSnapshot(&summary, snapshot)
	if !changed {
		return executionOrderSummaryResponse{}, nil, false
	}
	normalizeBrokerOrderSummary(&summary, snapshot, now, source, sourceDetail)
	order, event := s.persistBrokerOrderLocked(summary, stringPointerOrNil(previousStatus), updatedEventType, snapshot, summary.UpdatedAt)
	return order, event, true
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
	if s.registerFillKeyLocked(fillKey, now) {
		return executionOrderSummaryResponse{}, nil, false
	}

	internalOrderID := s.findInternalOrderIDLocked(brokerID, fill.AccountID, fill.TradingEnvironment, fill.Market, fill.BrokerOrderID, fill.BrokerOrderIDEx)
	if internalOrderID == "" {
		summary := brokerOrderSummaryFromFill(s.allocateInternalOrderIDLocked(), brokerID, fill, now)
		order, event := s.persistBrokerOrderLocked(summary, nil, "BROKER_FILL_RECEIVED", fill, firstNonEmptyString(summary.UpdatedAt, now))
		return order, event, true
	}

	summary := s.orders[internalOrderID]
	previousStatus, previousFilled, newFilled := applyBrokerOrderFill(&summary, fill, now)
	order, event := s.persistBrokerOrderLocked(summary, stringPointerOrNil(previousStatus), "BROKER_FILL_RECEIVED", fill, summary.UpdatedAt)
	return order, event, previousStatus != summary.Status || newFilled != previousFilled
}

func (s *executionOrderStore) recordBrokerOrderFee(
	brokerID string,
	fee broker.OrderFeeSnapshot,
) (executionOrderSummaryResponse, *executionOrderEventResponse, bool) {
	feeAmount := brokerOrderFeeAmount(fee)
	if feeAmount == nil || strings.TrimSpace(fee.BrokerOrderIDEx) == "" {
		return executionOrderSummaryResponse{}, nil, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	brokerOrderIDEx := strings.TrimSpace(fee.BrokerOrderIDEx)
	internalOrderID := s.findInternalOrderIDLocked(
		brokerID,
		fee.AccountID,
		fee.TradingEnvironment,
		fee.Market,
		"",
		&brokerOrderIDEx,
	)
	if internalOrderID == "" {
		return executionOrderSummaryResponse{}, nil, false
	}
	summary := s.orders[internalOrderID]
	if !float64PointersDiffer(summary.Fees, feeAmount) {
		return cloneExecutionOrderSummary(summary), nil, false
	}
	summary.Fees = cloneFloat64Pointer(feeAmount)
	summary.UpdatedAt = now
	order, event := s.persistBrokerOrderLocked(
		summary,
		stringPointerOrNil(summary.Status),
		"BROKER_ORDER_FEES_UPDATED",
		fee,
		now,
	)
	return order, event, true
}

func brokerOrderFeeAmount(fee broker.OrderFeeSnapshot) *float64 {
	if fee.FeeAmount != nil {
		return cloneFloat64Pointer(fee.FeeAmount)
	}
	if len(fee.FeeItems) == 0 {
		return nil
	}
	total := 0.0
	for _, item := range fee.FeeItems {
		total += item.Value
	}
	return &total
}

func (s *executionOrderStore) allocateInternalOrderIDLocked() string {
	s.nextOrderSeq++
	s.persistSequenceLocked("orders", s.nextOrderSeq)
	return fmt.Sprintf("exec-%06d", s.nextOrderSeq)
}

func (s *executionOrderStore) persistBrokerOrderLocked(summary executionOrderSummaryResponse, previousStatus *string, eventType string, payload any, createdAt string) (executionOrderSummaryResponse, *executionOrderEventResponse) {
	s.orders[summary.InternalOrderID] = summary
	s.linkBrokerOrderLocked(summary)
	s.persistOrderLocked(summary)
	event := s.appendEventLocked(summary.InternalOrderID, previousStatus, summary.Status, eventType, payload, createdAt)
	return cloneExecutionOrderSummary(summary), new(cloneExecutionOrderEvent(event))
}

func (s *executionOrderStore) registerFillKeyLocked(fillKey string, now string) bool {
	if fillKey == "" {
		return false
	}
	if _, exists := s.seenFillKeys[fillKey]; exists {
		return true
	}
	s.seenFillKeys[fillKey] = now
	s.persistSeenFillKeyLocked(fillKey, now)
	return false
}

func (s *executionOrderStore) brokerOrderSummaryFromSnapshot(internalOrderID string, brokerID string, snapshot broker.OrderSnapshot, now string, source string, sourceDetail string) executionOrderSummaryResponse {
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
		OrderKind:          snapshot.OrderKind,
		ProductClass:       snapshot.ProductClass,
		QuantityMode:       snapshot.QuantityMode,
		Symbol:             stringPointerOrNil(snapshot.Symbol),
		Side:               stringPointerOrNil(snapshot.Side),
		OrderType:          stringPointerOrNil(snapshot.OrderType),
		Status:             trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus),
		RawBrokerStatus:    stringPointerOrNil(rawBrokerStatus),
		RequestedQuantity:  new(snapshot.Quantity),
		RequestedAmount:    cloneFloat64Pointer(snapshot.Amount),
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
	if summary.OrderKind == "" {
		summary.OrderKind = broker.OrderKindSingle
	}
	if summary.ProductClass == "" {
		summary.ProductClass = broker.ProductClassUnknown
	}
	if summary.QuantityMode == "" {
		summary.QuantityMode = broker.QuantityModeUnits
	}
	applyExecutionLegSnapshots(&summary, snapshot.Legs, summary.UpdatedAt)
	return summary
}

func applyBrokerOrderSnapshot(summary *executionOrderSummaryResponse, snapshot broker.OrderSnapshot) bool {
	changed := false
	changed = applyBrokerOrderSnapshotStatus(summary, snapshot) || changed
	changed = applyBrokerOrderSnapshotIdentity(summary, snapshot) || changed
	changed = applyBrokerOrderSnapshotQuantities(summary, snapshot) || changed
	legsBefore := marshalExecutionPayload(summary.Legs)
	applyExecutionLegSnapshots(summary, snapshot.Legs, firstNonEmptyString(snapshot.UpdatedAt, summary.UpdatedAt))
	changed = legsBefore != marshalExecutionPayload(summary.Legs) || changed
	if value := strings.TrimSpace(snapshot.SubmittedAt); value != "" && stringPointersDiffer(summary.SubmittedAt, &value) {
		summary.SubmittedAt = stringPointerOrNil(value)
		changed = true
	}
	if value := strings.TrimSpace(snapshot.UpdatedAt); value != "" {
		if executionTimestampAdvances(summary.UpdatedAt, value) {
			summary.UpdatedAt = value
			changed = true
		}
	}
	return changed
}

func applyBrokerOrderSnapshotStatus(summary *executionOrderSummaryResponse, snapshot broker.OrderSnapshot) bool {
	changed := false
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
	if stringPointersDiffer(summary.LastError, snapshot.LastError) {
		summary.LastError = cloneStringPointer(snapshot.LastError)
		changed = true
	}
	if snapshot.LastError != nil {
		summary.LastErrorSource = executionStringPointerOrNil("broker.sync")
	} else {
		summary.LastErrorSource = nil
	}
	return changed
}

func applyBrokerOrderSnapshotIdentity(summary *executionOrderSummaryResponse, snapshot broker.OrderSnapshot) bool {
	changed := false
	if snapshot.OrderKind != "" && summary.OrderKind != snapshot.OrderKind {
		summary.OrderKind = snapshot.OrderKind
		changed = true
	}
	incomingProductKnown := snapshot.ProductClass != "" && snapshot.ProductClass != broker.ProductClassUnknown
	currentProductUnknown := summary.ProductClass == "" || summary.ProductClass == broker.ProductClassUnknown
	if snapshot.ProductClass != "" &&
		(incomingProductKnown || currentProductUnknown) &&
		summary.ProductClass != snapshot.ProductClass {
		summary.ProductClass = snapshot.ProductClass
		changed = true
	}
	// OpenD's single-order payload does not carry a product type. Do not let its
	// generic units fallback erase a contracts/amount mode locked by preview.
	if snapshot.QuantityMode != "" &&
		(incomingProductKnown || currentProductUnknown) &&
		summary.QuantityMode != snapshot.QuantityMode {
		summary.QuantityMode = snapshot.QuantityMode
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
	return changed
}

func applyBrokerOrderSnapshotQuantities(summary *executionOrderSummaryResponse, snapshot broker.OrderSnapshot) bool {
	changed := false
	if snapshot.Quantity > 0 && float64PointersDiffer(summary.RequestedQuantity, &snapshot.Quantity) {
		summary.RequestedQuantity = new(snapshot.Quantity)
		changed = true
	}
	if snapshot.Price != nil && float64PointersDiffer(summary.RequestedPrice, snapshot.Price) {
		summary.RequestedPrice = cloneFloat64Pointer(snapshot.Price)
		changed = true
	}
	if snapshot.Amount != nil && float64PointersDiffer(summary.RequestedAmount, snapshot.Amount) {
		summary.RequestedAmount = cloneFloat64Pointer(snapshot.Amount)
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
	return changed
}

func normalizeBrokerOrderSummary(summary *executionOrderSummaryResponse, snapshot broker.OrderSnapshot, now string, source string, sourceDetail string) {
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
}

func brokerOrderSummaryFromFill(internalOrderID string, brokerID string, fill broker.OrderFillSnapshot, now string) executionOrderSummaryResponse {
	rawBrokerStatus := strings.TrimSpace(derefString(fill.Status))
	status := trdsrv.CanonicalBrokerOrderStatus(rawBrokerStatus)
	if fill.Payout != nil {
		status = trdsrv.OrderStatusFilled
	}
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
		Payout:             cloneFloat64Pointer(fill.Payout),
		Remark:             nil,
		LastError:          nil,
		LastErrorCode:      nil,
		LastErrorSource:    executionStringPointerOrNil("broker.push"),
		SubmittedAt:        stringPointerOrNil(firstNonEmptyString(fill.FilledAt, now)),
		UpdatedAt:          firstNonEmptyString(fill.FilledAt, now),
		CreatedAt:          now,
	}
	return summary
}

func applyBrokerOrderFill(summary *executionOrderSummaryResponse, fill broker.OrderFillSnapshot, now string) (string, float64, float64) {
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
	if fill.Payout != nil {
		status = trdsrv.OrderStatusFilled
	}
	if status == trdsrv.OrderStatusUnknown {
		status = trdsrv.OrderStatusPartiallyFilled
		if summary.RequestedQuantity != nil && newFilled >= *summary.RequestedQuantity && *summary.RequestedQuantity > 0 {
			status = trdsrv.OrderStatusFilled
		}
	}
	summary.FilledQuantity = new(newFilled)
	if fill.Payout != nil {
		summary.Payout = cloneFloat64Pointer(fill.Payout)
	}
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
	if updatedAt := firstNonEmptyString(fill.FilledAt, now); executionTimestampAdvances(summary.UpdatedAt, updatedAt) {
		summary.UpdatedAt = updatedAt
	}
	if summary.SubmittedAt == nil {
		summary.SubmittedAt = stringPointerOrNil(firstNonEmptyString(fill.FilledAt, now))
	}
	if summary.Source == "" {
		summary.Source = "broker"
	}
	if summary.SourceDetail == "" {
		summary.SourceDetail = "broker.fill"
	}
	return previousStatus, previousFilled, newFilled
}
