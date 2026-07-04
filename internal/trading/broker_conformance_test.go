package trading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFakeBrokerConformanceAcceptedPartialFullAndOutOfOrderUpdates(t *testing.T) {
	harness := newFakeBrokerConformanceHarness()
	service := NewService(WithOrderStore(harness), WithOrderGateway(harness))
	price := 100.0

	created, err := service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", OrderType: "LIMIT", Quantity: 10, Price: &price,
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder: %v", err)
	}
	if created.OrderStatus == nil || *created.OrderStatus != OrderStatusBrokerAccepted {
		t.Fatalf("created status = %#v, want %s", created.OrderStatus, OrderStatusBrokerAccepted)
	}

	harness.ApplyFill("1001", "fill-1", 4, 100)
	details := mustConformanceOrderDetails(t, service, *created.InternalOrderID)
	if details.Order.Status != OrderStatusPartiallyFilled || optionalExecutionFloat(details.Order.FilledQuantity) != 4 {
		t.Fatalf("partial details = %#v", details.Order)
	}

	harness.ApplyOrder(fakeBrokerOrderUpdate{BrokerOrderID: "1001", RawStatus: "SUBMITTED", FilledQuantity: new(1.0), UpdatedAt: harness.now.Add(-time.Minute)})
	details = mustConformanceOrderDetails(t, service, *created.InternalOrderID)
	if details.Order.Status != OrderStatusPartiallyFilled || optionalExecutionFloat(details.Order.FilledQuantity) != 4 {
		t.Fatalf("stale update regressed order = %#v", details.Order)
	}

	harness.ApplyFill("1001", "fill-2", 6, 101)
	details = mustConformanceOrderDetails(t, service, *created.InternalOrderID)
	if details.Order.Status != OrderStatusFilled || optionalExecutionFloat(details.Order.FilledQuantity) != 10 {
		t.Fatalf("filled details = %#v", details.Order)
	}
	if details.Order.FilledAveragePrice == nil || *details.Order.FilledAveragePrice != 100.6 {
		t.Fatalf("filled average = %#v, want 100.6", details.Order.FilledAveragePrice)
	}

	harness.ApplyOrder(fakeBrokerOrderUpdate{BrokerOrderID: "1001", RawStatus: "CANCELLED_ALL", UpdatedAt: harness.now.Add(time.Minute)})
	details = mustConformanceOrderDetails(t, service, *created.InternalOrderID)
	if details.Order.Status != OrderStatusFilled {
		t.Fatalf("terminal status regressed = %#v", details.Order)
	}
}

func TestFakeBrokerConformanceCancelAcceptedAndCancelRejected(t *testing.T) {
	harness := newFakeBrokerConformanceHarness()
	service := NewService(WithOrderStore(harness), WithOrderGateway(harness))
	price := 88.0

	first, err := service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		Market: "US", Symbol: "MSFT", Side: "BUY", Quantity: 2, Price: &price,
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder first: %v", err)
	}
	cancelled, err := service.CancelExecutionOrder(t.Context(), *first.InternalOrderID)
	if err != nil {
		t.Fatalf("CancelExecutionOrder first: %v", err)
	}
	if cancelled.OrderStatus == nil || *cancelled.OrderStatus != OrderStatusCancelRequested {
		t.Fatalf("cancel requested = %#v", cancelled)
	}
	harness.ApplyOrder(fakeBrokerOrderUpdate{BrokerOrderID: "1001", RawStatus: "CANCELLED_ALL"})
	firstDetails := mustConformanceOrderDetails(t, service, *first.InternalOrderID)
	if firstDetails.Order.Status != OrderStatusCancelled {
		t.Fatalf("cancel accepted details = %#v", firstDetails.Order)
	}
	if _, err := service.CancelExecutionOrder(t.Context(), *first.InternalOrderID); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal cancel error = %v", err)
	}

	second, err := service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		Market: "US", Symbol: "MSFT", Side: "SELL", Quantity: 1, Price: &price,
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder second: %v", err)
	}
	if _, err := service.CancelExecutionOrder(t.Context(), *second.InternalOrderID); err != nil {
		t.Fatalf("CancelExecutionOrder second: %v", err)
	}
	harness.RejectCancel(*second.InternalOrderID, "broker refused cancel because order is locked")
	secondDetails := mustConformanceOrderDetails(t, service, *second.InternalOrderID)
	if secondDetails.Order.Status != OrderStatusCancelRequested {
		t.Fatalf("cancel rejected should not become terminal = %#v", secondDetails.Order)
	}
	if secondDetails.Order.LastError == nil || !strings.Contains(*secondDetails.Order.LastError, "refused cancel") {
		t.Fatalf("cancel reject error = %#v", secondDetails.Order.LastError)
	}
	if !conformanceHasEvent(secondDetails.RecentEvents, "BROKER_CANCEL_REJECTED") {
		t.Fatalf("cancel reject event missing = %#v", secondDetails.RecentEvents)
	}
}

func TestFakeBrokerConformancePlaceRejectedPushBeforeQueryAndUnsupportedCapability(t *testing.T) {
	harness := newFakeBrokerConformanceHarness()
	service := NewService(WithOrderStore(harness), WithOrderGateway(harness))
	price := 55.0

	harness.nextPlaceStatus = "SUBMIT_FAILED"
	rejected, err := service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		Market: "US", Symbol: "TSLA", Side: "BUY", Quantity: 1, Price: &price,
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder rejected status: %v", err)
	}
	rejectedDetails := mustConformanceOrderDetails(t, service, *rejected.InternalOrderID)
	if rejectedDetails.Order.Status != OrderStatusRejected || rejectedDetails.Order.RawBrokerStatus == nil || *rejectedDetails.Order.RawBrokerStatus != "SUBMIT_FAILED" {
		t.Fatalf("rejected details = %#v", rejectedDetails.Order)
	}

	harness.ApplyOrder(fakeBrokerOrderUpdate{
		BrokerOrderID:      "push-before-query",
		BrokerOrderIDEx:    "push-before-query-ex",
		RawStatus:          "ACCEPTED",
		AccountID:          "ACC-PUSH",
		Market:             "US",
		Symbol:             "US.NVDA",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Quantity:           3,
		Price:              &price,
		TradingEnvironment: "SIMULATE",
	})
	snapshot, err := service.ExecutionOrdersSnapshot(t.Context())
	if err != nil {
		t.Fatalf("ExecutionOrdersSnapshot: %v", err)
	}
	var discovered *ExecutionOrder
	for index := range snapshot.Orders {
		if snapshot.Orders[index].BrokerOrderID != nil && *snapshot.Orders[index].BrokerOrderID == "push-before-query" {
			discovered = &snapshot.Orders[index]
			break
		}
	}
	if discovered == nil || discovered.Status != OrderStatusBrokerAccepted || discovered.InternalOrderID == "" {
		t.Fatalf("push-before-query discovered = %#v", discovered)
	}

	harness.rejectNextPlace = errors.New("broker capability unsupported: stop-limit")
	_, err = service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		Market: "US", Symbol: "TSLA", Side: "BUY", OrderType: "STOP_LIMIT", Quantity: 1, Price: &price, StopPrice: &price,
	})
	if err == nil || !strings.Contains(err.Error(), "capability unsupported") {
		t.Fatalf("unsupported capability error = %v", err)
	}
}

type fakeBrokerConformanceHarness struct {
	mu sync.Mutex

	nextOrderSeq int
	nextEventSeq int
	now          time.Time

	nextPlaceStatus  string
	rejectNextPlace  error
	rejectNextCancel error

	orders      map[string]ExecutionOrder
	events      map[string][]ExecutionOrderEvent
	brokerIndex map[string]string
	seenFills   map[string]struct{}
}

type fakeBrokerOrderUpdate struct {
	BrokerOrderID      string
	BrokerOrderIDEx    string
	RawStatus          string
	AccountID          string
	TradingEnvironment string
	Market             string
	Symbol             string
	Side               string
	OrderType          string
	Quantity           float64
	Price              *float64
	FilledQuantity     *float64
	FilledAveragePrice *float64
	UpdatedAt          time.Time
	LastError          string
}

func newFakeBrokerConformanceHarness() *fakeBrokerConformanceHarness {
	return &fakeBrokerConformanceHarness{
		now:         time.Date(2026, 7, 4, 1, 2, 3, 0, time.UTC),
		orders:      make(map[string]ExecutionOrder),
		events:      make(map[string][]ExecutionOrderEvent),
		brokerIndex: make(map[string]string),
		seenFills:   make(map[string]struct{}),
	}
}

func (h *fakeBrokerConformanceHarness) PlaceOrder(_ context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rejectNextPlace != nil {
		err := h.rejectNextPlace
		h.rejectNextPlace = nil
		return ExecutionOrder{}, err
	}
	h.nextOrderSeq++
	brokerOrderID := fmt.Sprintf("%d", 1000+h.nextOrderSeq)
	brokerOrderIDEx := fmt.Sprintf("fake-ex-%06d", h.nextOrderSeq)
	rawStatus := strings.TrimSpace(h.nextPlaceStatus)
	h.nextPlaceStatus = ""
	if rawStatus == "" {
		rawStatus = "SUBMITTED"
	}
	now := h.timestamp(0)
	order := ExecutionOrder{
		InternalOrderID:    fmt.Sprintf("exec-fake-%06d", h.nextOrderSeq),
		BrokerID:           firstConformanceString(command.BrokerID, "futu"),
		BrokerOrderID:      conformanceStringPointer(brokerOrderID),
		BrokerOrderIDEx:    conformanceStringPointer(brokerOrderIDEx),
		Source:             "system",
		SourceDetail:       "command.place",
		TradingEnvironment: firstConformanceString(command.Query.TradingEnvironment, "SIMULATE"),
		AccountID:          command.Query.AccountID,
		Market:             command.Query.Market,
		Symbol:             conformanceStringPointer(command.Symbol),
		Side:               conformanceStringPointer(command.Side),
		OrderType:          conformanceStringPointer(command.OrderType),
		Status:             CanonicalBrokerOrderStatus(rawStatus),
		RawBrokerStatus:    conformanceStringPointer(rawStatus),
		RequestedQuantity:  new(command.Query.Quantity),
		RequestedPrice:     cloneConformanceFloat(command.Query.Price),
		FilledQuantity:     new(0.0),
		Remark:             conformanceStringPointer(command.Remark),
		SubmittedAt:        conformanceStringPointer(now),
		UpdatedAt:          now,
		CreatedAt:          now,
	}
	h.storeOrderLocked(order)
	h.appendEventLocked(order.InternalOrderID, nil, order.Status, "COMMAND_PLACE_ACCEPTED", map[string]any{
		"rawBrokerStatus": rawStatus,
		"brokerOrderId":   brokerOrderID,
	}, now)
	return cloneConformanceOrder(order), nil
}

func (h *fakeBrokerConformanceHarness) CancelOrder(_ context.Context, internalOrderID string) (ExecutionOrder, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rejectNextCancel != nil {
		err := h.rejectNextCancel
		h.rejectNextCancel = nil
		return ExecutionOrder{}, err
	}
	order, ok := h.orders[strings.TrimSpace(internalOrderID)]
	if !ok {
		return ExecutionOrder{}, ErrExecutionOrderNotFound
	}
	if IsCanonicalTerminalOrderStatus(order.Status) {
		return ExecutionOrder{}, fmt.Errorf("execution order is already terminal (%s)", order.Status)
	}
	previous := order.Status
	if reconciled, accepted := ReconcileCanonicalOrderStatus(order.Status, OrderStatusCancelRequested); accepted {
		order.Status = reconciled
	}
	order.UpdatedAt = h.timestamp(1)
	h.storeOrderLocked(order)
	h.appendEventLocked(order.InternalOrderID, &previous, order.Status, "COMMAND_CANCEL_ACCEPTED", map[string]any{
		"brokerOrderId": derefConformanceString(order.BrokerOrderID),
	}, order.UpdatedAt)
	return cloneConformanceOrder(order), nil
}

func (h *fakeBrokerConformanceHarness) ListOrders(_ context.Context, filter ExecutionOrderFilter) (ExecutionOrders, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	orders := make([]ExecutionOrder, 0, len(h.orders))
	for _, order := range h.orders {
		if conformanceOrderMatchesFilter(order, filter) {
			orders = append(orders, cloneConformanceOrder(order))
		}
	}
	sort.Slice(orders, func(i, j int) bool {
		return orders[i].UpdatedAt > orders[j].UpdatedAt
	})
	return ExecutionOrders{Orders: orders}, nil
}

func (h *fakeBrokerConformanceHarness) OrderEvents(_ context.Context, internalOrderID string) (ExecutionOrderEvents, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	events := h.events[strings.TrimSpace(internalOrderID)]
	cloned := make([]ExecutionOrderEvent, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, cloneConformanceEvent(event))
	}
	return ExecutionOrderEvents{InternalOrderID: strings.TrimSpace(internalOrderID), Events: cloned}, nil
}

func (h *fakeBrokerConformanceHarness) ApplyOrder(update fakeBrokerOrderUpdate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	update = h.normalizeOrderUpdate(update)
	internalOrderID := h.brokerIndex[h.brokerKey(update)]
	if internalOrderID == "" {
		internalOrderID = h.internalOrderIDByBrokerOrderIDLocked(update.BrokerOrderID)
	}
	if internalOrderID == "" {
		h.nextOrderSeq++
		internalOrderID = fmt.Sprintf("exec-fake-%06d", h.nextOrderSeq)
		now := h.timestamp(2)
		order := ExecutionOrder{
			InternalOrderID:    internalOrderID,
			BrokerID:           "futu",
			BrokerOrderID:      conformanceStringPointer(update.BrokerOrderID),
			BrokerOrderIDEx:    conformanceStringPointer(update.BrokerOrderIDEx),
			Source:             "broker",
			SourceDetail:       "broker.push",
			TradingEnvironment: update.TradingEnvironment,
			AccountID:          update.AccountID,
			Market:             update.Market,
			Symbol:             conformanceStringPointer(update.Symbol),
			Side:               conformanceStringPointer(update.Side),
			OrderType:          conformanceStringPointer(update.OrderType),
			Status:             CanonicalBrokerOrderStatus(update.RawStatus),
			RawBrokerStatus:    conformanceStringPointer(update.RawStatus),
			RequestedQuantity:  new(update.Quantity),
			RequestedPrice:     cloneConformanceFloat(update.Price),
			FilledQuantity:     cloneConformanceFloat(update.FilledQuantity),
			FilledAveragePrice: cloneConformanceFloat(update.FilledAveragePrice),
			SubmittedAt:        conformanceStringPointer(now),
			UpdatedAt:          now,
			CreatedAt:          now,
		}
		if order.FilledQuantity == nil {
			order.FilledQuantity = new(0.0)
		}
		h.storeOrderLocked(order)
		h.appendEventLocked(internalOrderID, nil, order.Status, "BROKER_PUSH_DISCOVERED", update, order.UpdatedAt)
		return
	}

	order := h.orders[internalOrderID]
	previous := order.Status
	changed := false
	if update.RawStatus != "" {
		incoming := CanonicalBrokerOrderStatus(update.RawStatus)
		if reconciled, accepted := ReconcileCanonicalOrderStatus(order.Status, incoming); accepted {
			if order.Status != reconciled {
				order.Status = reconciled
				changed = true
			}
			if derefConformanceString(order.RawBrokerStatus) != update.RawStatus {
				order.RawBrokerStatus = conformanceStringPointer(update.RawStatus)
				changed = true
			}
		}
	}
	if update.FilledQuantity != nil && *update.FilledQuantity >= optionalExecutionFloat(order.FilledQuantity) {
		order.FilledQuantity = cloneConformanceFloat(update.FilledQuantity)
		changed = true
	}
	if update.FilledAveragePrice != nil && update.FilledQuantity != nil && *update.FilledQuantity >= optionalExecutionFloat(order.FilledQuantity) {
		order.FilledAveragePrice = cloneConformanceFloat(update.FilledAveragePrice)
		changed = true
	}
	if update.LastError != "" {
		order.LastError = conformanceStringPointer(update.LastError)
		changed = true
	}
	incomingUpdatedAt := update.UpdatedAt.Format(time.RFC3339Nano)
	if executionConformanceTimestampAdvances(order.UpdatedAt, incomingUpdatedAt) {
		order.UpdatedAt = incomingUpdatedAt
		changed = true
	}
	if !changed {
		return
	}
	h.storeOrderLocked(order)
	h.appendEventLocked(internalOrderID, &previous, order.Status, "BROKER_PUSH_ORDER", update, order.UpdatedAt)
}

func (h *fakeBrokerConformanceHarness) ApplyFill(brokerOrderID string, brokerFillID string, quantity float64, price float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.seenFills[brokerFillID]; ok {
		return
	}
	h.seenFills[brokerFillID] = struct{}{}
	internalOrderID := h.internalOrderIDByBrokerOrderIDLocked(brokerOrderID)
	if internalOrderID == "" {
		return
	}
	order := h.orders[internalOrderID]
	previous := order.Status
	previousFilled := optionalExecutionFloat(order.FilledQuantity)
	previousAverage := optionalExecutionFloat(order.FilledAveragePrice)
	nextFilled := previousFilled + quantity
	nextAverage := ((previousAverage * previousFilled) + (price * quantity)) / nextFilled
	order.FilledQuantity = new(nextFilled)
	order.FilledAveragePrice = new(nextAverage)
	incomingStatus := OrderStatusPartiallyFilled
	if order.RequestedQuantity != nil && nextFilled >= *order.RequestedQuantity {
		incomingStatus = OrderStatusFilled
	}
	if reconciled, accepted := ReconcileCanonicalOrderStatus(order.Status, incomingStatus); accepted {
		order.Status = reconciled
	}
	order.LastError = nil
	order.UpdatedAt = h.timestamp(3)
	h.storeOrderLocked(order)
	h.appendEventLocked(internalOrderID, &previous, order.Status, "BROKER_FILL_RECEIVED", map[string]any{
		"brokerOrderId": brokerOrderID,
		"brokerFillId":  brokerFillID,
		"quantity":      quantity,
		"price":         price,
	}, order.UpdatedAt)
}

func (h *fakeBrokerConformanceHarness) RejectCancel(internalOrderID string, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	order := h.orders[strings.TrimSpace(internalOrderID)]
	previous := order.Status
	order.LastError = conformanceStringPointer(message)
	order.LastErrorSource = conformanceStringPointer("broker.cancel")
	order.UpdatedAt = h.timestamp(4)
	h.storeOrderLocked(order)
	h.appendEventLocked(internalOrderID, &previous, order.Status, "BROKER_CANCEL_REJECTED", map[string]any{"message": message}, order.UpdatedAt)
}

func (h *fakeBrokerConformanceHarness) normalizeOrderUpdate(update fakeBrokerOrderUpdate) fakeBrokerOrderUpdate {
	if update.AccountID == "" {
		update.AccountID = "ACC-1"
	}
	if update.TradingEnvironment == "" {
		update.TradingEnvironment = "SIMULATE"
	}
	if update.Market == "" {
		update.Market = "US"
	}
	if update.RawStatus == "" {
		update.RawStatus = "SUBMITTED"
	}
	if update.Symbol == "" {
		update.Symbol = "US.AAPL"
	}
	if update.Side == "" {
		update.Side = "BUY"
	}
	if update.OrderType == "" {
		update.OrderType = "LIMIT"
	}
	if update.Quantity == 0 {
		update.Quantity = 1
	}
	if update.UpdatedAt.IsZero() {
		update.UpdatedAt = h.now.Add(30 * time.Second)
	}
	return update
}

func (h *fakeBrokerConformanceHarness) storeOrderLocked(order ExecutionOrder) {
	h.orders[order.InternalOrderID] = cloneConformanceOrder(order)
	if key := h.brokerKey(fakeBrokerOrderUpdate{
		BrokerOrderID:      derefConformanceString(order.BrokerOrderID),
		AccountID:          order.AccountID,
		TradingEnvironment: order.TradingEnvironment,
		Market:             order.Market,
	}); key != "" {
		h.brokerIndex[key] = order.InternalOrderID
	}
}

func (h *fakeBrokerConformanceHarness) appendEventLocked(internalOrderID string, previous *string, next string, eventType string, payload any, createdAt string) {
	h.nextEventSeq++
	encoded, _ := json.Marshal(payload)
	h.events[internalOrderID] = append(h.events[internalOrderID], ExecutionOrderEvent{
		ID:              fmt.Sprintf("evt-fake-%06d", h.nextEventSeq),
		InternalOrderID: internalOrderID,
		EventType:       eventType,
		PreviousStatus:  cloneConformanceString(previous),
		NextStatus:      next,
		PayloadJSON:     string(encoded),
		CreatedAt:       createdAt,
	})
}

func (h *fakeBrokerConformanceHarness) brokerKey(update fakeBrokerOrderUpdate) string {
	if strings.TrimSpace(update.BrokerOrderID) == "" {
		return ""
	}
	return strings.Join([]string{
		"futu",
		strings.ToUpper(strings.TrimSpace(update.TradingEnvironment)),
		strings.TrimSpace(update.AccountID),
		strings.ToUpper(strings.TrimSpace(update.Market)),
		strings.TrimSpace(update.BrokerOrderID),
	}, "|")
}

func (h *fakeBrokerConformanceHarness) internalOrderIDByBrokerOrderIDLocked(brokerOrderID string) string {
	for key, internalOrderID := range h.brokerIndex {
		if strings.HasSuffix(key, "|"+strings.TrimSpace(brokerOrderID)) {
			return internalOrderID
		}
	}
	return ""
}

func (h *fakeBrokerConformanceHarness) timestamp(offsetSeconds int) string {
	return h.now.Add(time.Duration(h.nextEventSeq+offsetSeconds) * time.Second).Format(time.RFC3339Nano)
}

func mustConformanceOrderDetails(t *testing.T, service *Service, internalOrderID string) ExecutionOrderDetails {
	t.Helper()
	details, err := service.ExecutionOrderDetails(t.Context(), internalOrderID)
	if err != nil {
		t.Fatalf("ExecutionOrderDetails(%s): %v", internalOrderID, err)
	}
	return details
}

func conformanceOrderMatchesFilter(order ExecutionOrder, filter ExecutionOrderFilter) bool {
	if filter.BrokerID != "" && !strings.EqualFold(order.BrokerID, filter.BrokerID) {
		return false
	}
	if filter.TradingEnvironment != "" && !strings.EqualFold(order.TradingEnvironment, filter.TradingEnvironment) {
		return false
	}
	if filter.AccountID != "" && order.AccountID != filter.AccountID {
		return false
	}
	if filter.Market != "" && !strings.EqualFold(order.Market, filter.Market) {
		return false
	}
	return true
}

func conformanceHasEvent(events []ExecutionOrderEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func executionConformanceTimestampAdvances(current string, incoming string) bool {
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

func firstConformanceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func optionalExecutionFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func conformanceStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func cloneConformanceOrder(order ExecutionOrder) ExecutionOrder {
	order.BrokerOrderID = cloneConformanceString(order.BrokerOrderID)
	order.BrokerOrderIDEx = cloneConformanceString(order.BrokerOrderIDEx)
	order.Symbol = cloneConformanceString(order.Symbol)
	order.Side = cloneConformanceString(order.Side)
	order.OrderType = cloneConformanceString(order.OrderType)
	order.RawBrokerStatus = cloneConformanceString(order.RawBrokerStatus)
	order.RequestedQuantity = cloneConformanceFloat(order.RequestedQuantity)
	order.RequestedPrice = cloneConformanceFloat(order.RequestedPrice)
	order.FilledQuantity = cloneConformanceFloat(order.FilledQuantity)
	order.FilledAveragePrice = cloneConformanceFloat(order.FilledAveragePrice)
	order.Remark = cloneConformanceString(order.Remark)
	order.LastError = cloneConformanceString(order.LastError)
	order.LastErrorCode = cloneConformanceString(order.LastErrorCode)
	order.LastErrorSource = cloneConformanceString(order.LastErrorSource)
	order.SubmittedAt = cloneConformanceString(order.SubmittedAt)
	return order
}

func cloneConformanceEvent(event ExecutionOrderEvent) ExecutionOrderEvent {
	event.PreviousStatus = cloneConformanceString(event.PreviousStatus)
	return event
}

func cloneConformanceString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneConformanceFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func derefConformanceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
