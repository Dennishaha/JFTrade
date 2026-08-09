package tradingapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type fakeGatewayStore struct {
	prepareRecord trdsrv.ExecutionPlacedOrderRecord
	prepareOrder  trdsrv.ExecutionOrder
	prepareFresh  bool
	prepareErr    error
	unknownID     string
	placed        []trdsrv.ExecutionPlacedOrderRecord
	orders        map[string]trdsrv.ExecutionOrder
	cancelled     map[string]bool
}

func newFakeGatewayStore() *fakeGatewayStore {
	return &fakeGatewayStore{
		orders:    map[string]trdsrv.ExecutionOrder{},
		cancelled: map[string]bool{},
	}
}

func (s *fakeGatewayStore) PrepareSubmission(record trdsrv.ExecutionPlacedOrderRecord) (trdsrv.ExecutionOrder, bool, error) {
	s.prepareRecord = record
	if s.prepareErr != nil {
		return trdsrv.ExecutionOrder{}, false, s.prepareErr
	}
	return s.prepareOrder, s.prepareFresh, nil
}

func (s *fakeGatewayStore) MarkSubmissionUnknown(internalOrderID string, _ error) trdsrv.ExecutionOrder {
	s.unknownID = internalOrderID
	return trdsrv.ExecutionOrder{InternalOrderID: internalOrderID}
}

func (s *fakeGatewayStore) RecordPlacedOrder(record trdsrv.ExecutionPlacedOrderRecord) trdsrv.ExecutionOrder {
	s.placed = append(s.placed, record)
	order := trdsrv.ExecutionOrder{
		InternalOrderID: record.InternalOrderID, BrokerID: record.BrokerID,
		BrokerOrderID: ptr(record.BrokerOrderID), Status: record.Status, Symbol: ptr(record.Symbol),
	}
	s.orders[record.InternalOrderID] = order
	return order
}

func (s *fakeGatewayStore) Order(internalOrderID string) (trdsrv.ExecutionOrder, bool) {
	order, ok := s.orders[internalOrderID]
	return order, ok
}

func (s *fakeGatewayStore) MarkCancelRequested(internalOrderID string, _ any) (trdsrv.ExecutionOrder, bool) {
	s.cancelled[internalOrderID] = true
	order, ok := s.orders[internalOrderID]
	return order, ok
}

type gatewayTrading struct {
	placeResult    *broker.PlaceOrderResult
	placeErr       error
	cancelErr      error
	cancelled      []broker.CancelOrder
	comboResult    *broker.ComboOrderResult
	comboPlaceErr  error
	comboCancelErr error
	cancelComboID  string
}

func (t *gatewayTrading) PlaceOrder(context.Context, broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	return t.placeResult, t.placeErr
}

func (t *gatewayTrading) CancelOrders(_ context.Context, _ broker.ReadQuery, orders ...broker.CancelOrder) error {
	t.cancelled = append(t.cancelled, orders...)
	return t.cancelErr
}

func (t *gatewayTrading) PreviewComboOrder(context.Context, broker.ComboOrderIntent) (*broker.ProductRuleResult, error) {
	return nil, nil
}

func (t *gatewayTrading) PlaceComboOrder(context.Context, broker.ComboOrderIntent) (*broker.ComboOrderResult, error) {
	return t.comboResult, t.comboPlaceErr
}

func (t *gatewayTrading) CancelComboOrder(_ context.Context, _ broker.ReadQuery, brokerOrderID string) error {
	t.cancelComboID = brokerOrderID
	return t.comboCancelErr
}

type gatewayBroker struct {
	id      string
	trading broker.TradingService
}

func (b *gatewayBroker) ID() string { return b.id }
func (b *gatewayBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: b.id}
}
func (b *gatewayBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) { return nil, nil }
func (b *gatewayBroker) Trading() broker.TradingService                             { return b.trading }
func (b *gatewayBroker) MarketData() broker.MarketDataReader                        { return nil }

func (b *gatewayBroker) PreviewComboOrder(context.Context, broker.ComboOrderIntent) (*broker.ProductRuleResult, error) {
	return nil, nil
}

func (b *gatewayBroker) PlaceComboOrder(ctx context.Context, intent broker.ComboOrderIntent) (*broker.ComboOrderResult, error) {
	trading, ok := b.trading.(broker.ComboTradingService)
	if !ok {
		return nil, errors.New("combo trading service is unavailable")
	}
	return trading.PlaceComboOrder(ctx, intent)
}

func (b *gatewayBroker) CancelComboOrder(ctx context.Context, query broker.ReadQuery, brokerOrderID string) error {
	trading, ok := b.trading.(broker.ComboTradingService)
	if !ok {
		return errors.New("combo trading service is unavailable")
	}
	return trading.CancelComboOrder(ctx, query, brokerOrderID)
}

type plainBroker struct {
	id      string
	trading broker.TradingService
}

func (b *plainBroker) ID() string { return b.id }
func (b *plainBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: b.id}
}
func (b *plainBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) { return nil, nil }
func (b *plainBroker) Trading() broker.TradingService                             { return b.trading }
func (b *plainBroker) MarketData() broker.MarketDataReader                        { return nil }

func TestExecutionGatewayPlaceOrderBoundaries(t *testing.T) {
	store := newFakeGatewayStore()
	trading := &gatewayTrading{}
	gateway := NewExecutionGateway(ExecutionGatewayDependencies{
		ResolveBroker: func(id string) broker.Broker { return &gatewayBroker{id: id, trading: trading} },
		Orders:        func() ExecutionOrderStore { return store },
	})
	command := trdsrv.ExecutionOrderCommand{
		BrokerID: "futu", Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
		Query: broker.PlaceOrderQuery{ReadQuery: broker.ReadQuery{BrokerID: "other"}},
	}
	if _, err := gateway.PlaceOrder(context.Background(), command); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("broker mismatch error = %v", err)
	}
	command.Query.BrokerID = "futu"
	noStore := NewExecutionGateway(ExecutionGatewayDependencies{})
	if _, err := noStore.PlaceOrder(context.Background(), command); !errors.Is(err, trdsrv.ErrOrderStoreUnavailable) {
		t.Fatalf("missing store error = %v", err)
	}
	store.prepareErr = errors.New("prepare failed")
	if _, err := gateway.PlaceOrder(context.Background(), command); !errors.Is(err, store.prepareErr) {
		t.Fatalf("prepare error = %v", err)
	}
	store.prepareErr = nil
	store.prepareOrder = trdsrv.ExecutionOrder{InternalOrderID: "prepared"}
	store.prepareFresh = false
	placed, err := gateway.PlaceOrder(context.Background(), command)
	if err != nil || placed.InternalOrderID != "prepared" || len(store.placed) != 0 {
		t.Fatalf("stale submission = %#v, %v", placed, err)
	}
	store.prepareFresh = true
	noTradingGateway := NewExecutionGateway(ExecutionGatewayDependencies{
		ResolveBroker: func(id string) broker.Broker { return &gatewayBroker{id: id} },
		Orders:        func() ExecutionOrderStore { return store },
	})
	if _, err := noTradingGateway.PlaceOrder(context.Background(), command); err == nil || store.unknownID != "prepared" {
		t.Fatalf("unavailable trading error = %v, unknown=%q", err, store.unknownID)
	}
	ex := "EXT-1"
	store.prepareOrder = trdsrv.ExecutionOrder{InternalOrderID: "prepared"}
	trading.placeResult = &broker.PlaceOrderResult{
		BrokerOrderID: "9001", BrokerOrderIDEx: &ex, TradingEnvironment: "SIMULATE",
		AccountID: "acct-1", Market: "US", Status: "SUBMITTED",
	}
	notified := false
	gateway.dependencies.NotifyPlaced = func(trdsrv.ExecutionOrder) { notified = true }
	command.Session = "T1"
	placed, err = gateway.PlaceOrder(context.Background(), command)
	if err != nil || placed.BrokerOrderID == nil || *placed.BrokerOrderID != "9001" ||
		len(store.placed) != 1 || !notified {
		t.Fatalf("successful placement = %#v, %v", placed, err)
	}
	payload, ok := store.placed[0].Payload.(map[string]any)
	if !ok || payload["session"] != "T1" {
		t.Fatalf("placed record session = %#v", store.placed[0])
	}
}

func TestExecutionGatewayCancelOrderBoundaries(t *testing.T) {
	store := newFakeGatewayStore()
	trading := &gatewayTrading{}
	gateway := NewExecutionGateway(ExecutionGatewayDependencies{
		ResolveBroker: func(id string) broker.Broker { return &gatewayBroker{id: id, trading: trading} },
		Orders:        func() ExecutionOrderStore { return store },
	})
	noStore := NewExecutionGateway(ExecutionGatewayDependencies{})
	if _, err := noStore.CancelOrder(context.Background(), "o1"); !errors.Is(err, trdsrv.ErrOrderStoreUnavailable) {
		t.Fatalf("missing store cancel error = %v", err)
	}
	terminal := trdsrv.OrderStatusFilled
	store.orders["o1"] = trdsrv.ExecutionOrder{Status: terminal}
	if _, err := gateway.CancelOrder(context.Background(), "o1"); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal cancel error = %v", err)
	}
	store.orders["o1"] = trdsrv.ExecutionOrder{Status: "SUBMITTED"}
	if _, err := gateway.CancelOrder(context.Background(), "o1"); err == nil || !strings.Contains(err.Error(), "missing broker order id") {
		t.Fatalf("missing broker id error = %v", err)
	}
	store.orders["o1"] = trdsrv.ExecutionOrder{Status: "SUBMITTED", BrokerOrderID: ptr("not-a-number")}
	if _, err := gateway.CancelOrder(context.Background(), "o1"); err == nil || !strings.Contains(err.Error(), "invalid broker order id") {
		t.Fatalf("invalid broker id error = %v", err)
	}
	store.orders["o1"] = trdsrv.ExecutionOrder{
		Status: "SUBMITTED", BrokerOrderID: ptr("9001"), BrokerID: "futu",
	}
	if _, err := gateway.CancelOrder(context.Background(), "o1"); err == nil || !strings.Contains(err.Error(), "missing symbol") {
		t.Fatalf("missing symbol error = %v", err)
	}
	store.orders["o1"] = trdsrv.ExecutionOrder{
		Status: "SUBMITTED", BrokerOrderID: ptr("9001"), BrokerID: "futu", Symbol: ptr("US.AAPL"),
	}
	trading.cancelErr = errors.New("cancel failed")
	if _, err := gateway.CancelOrder(context.Background(), "o1"); !errors.Is(err, trading.cancelErr) {
		t.Fatalf("broker cancel error = %v", err)
	}
	trading.cancelErr = nil
	trading.cancelled = nil
	updated, err := gateway.CancelOrder(context.Background(), "o1")
	if err != nil || !store.cancelled["o1"] || updated.InternalOrderID != "" {
		t.Fatalf("successful cancel = %#v, %v", updated, err)
	}
	if len(trading.cancelled) != 1 || trading.cancelled[0].BrokerOrderID != "9001" {
		t.Fatalf("cancel order payload = %#v", trading.cancelled)
	}
}

func TestExecutionGatewayPlaceComboBoundaries(t *testing.T) {
	store := newFakeGatewayStore()
	trading := &gatewayTrading{}
	gateway := NewExecutionGateway(ExecutionGatewayDependencies{
		ResolveBroker: func(id string) broker.Broker { return &plainBroker{id: id, trading: trading} },
		Orders:        func() ExecutionOrderStore { return store },
	})
	intent := broker.ComboOrderIntent{
		ReadQuery: broker.ReadQuery{BrokerID: "futu", AccountID: "acct-1", Market: "US"},
		OrderKind: broker.OrderKindEventParlay, Legs: []broker.OrderLegIntent{{
			InstrumentID: "US.EVENT.ONE", Side: "BUY", Quantity: ptr(100.0),
		}},
	}
	if _, err := gateway.PlaceCombo(context.Background(), intent); err == nil || !strings.Contains(err.Error(), "combo trading service is unavailable") {
		t.Fatalf("non-combo broker error = %v", err)
	}
	gateway.dependencies.ResolveBroker = func(id string) broker.Broker {
		return &gatewayBroker{id: id, trading: trading}
	}
	store.prepareOrder = trdsrv.ExecutionOrder{InternalOrderID: "combo-1"}
	store.prepareFresh = true
	trading.comboResult = &broker.ComboOrderResult{BrokerOrderID: "C-1", Status: "SUBMITTED"}
	notified := false
	gateway.dependencies.NotifyPlaced = func(trdsrv.ExecutionOrder) { notified = true }
	placed, err := gateway.PlaceCombo(context.Background(), intent)
	if err != nil || placed.BrokerOrderID == nil || *placed.BrokerOrderID != "C-1" ||
		len(store.placed) != 1 || !notified {
		t.Fatalf("successful combo placement = %#v, %v", placed, err)
	}
	if store.placed[0].OrderType != "COMBO" || store.placed[0].Symbol != "US.EVENT.ONE" {
		t.Fatalf("combo placed record = %#v", store.placed[0])
	}
	store.prepareFresh = false
	if placed, err := gateway.PlaceCombo(context.Background(), intent); err != nil || placed.InternalOrderID != "combo-1" {
		t.Fatalf("stale combo submission = %#v, %v", placed, err)
	}
	store.prepareFresh = true
	store.prepareErr = errors.New("combo prepare failed")
	if _, err := gateway.PlaceCombo(context.Background(), intent); !errors.Is(err, store.prepareErr) {
		t.Fatalf("combo prepare error = %v", err)
	}
	store.prepareErr = nil
	trading.comboPlaceErr = errors.New("combo place failed")
	if _, err := gateway.PlaceCombo(context.Background(), intent); !errors.Is(err, trading.comboPlaceErr) || store.unknownID != "combo-1" {
		t.Fatalf("combo place error = %v unknown=%q", err, store.unknownID)
	}
}

func TestExecutionGatewayCancelComboBoundaries(t *testing.T) {
	store := newFakeGatewayStore()
	gateway := NewExecutionGateway(ExecutionGatewayDependencies{Orders: func() ExecutionOrderStore { return store }})
	noStore := NewExecutionGateway(ExecutionGatewayDependencies{})
	if _, err := noStore.CancelCombo(context.Background(), "c1"); !errors.Is(err, trdsrv.ErrOrderStoreUnavailable) {
		t.Fatalf("missing store combo cancel error = %v", err)
	}
	gateway.dependencies.ResolveBroker = func(id string) broker.Broker { return &plainBroker{id: id} }
	if _, err := gateway.CancelCombo(context.Background(), "c1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing combo error = %v", err)
	}
	store.orders["c1"] = trdsrv.ExecutionOrder{BrokerID: "futu", BrokerOrderID: ptr("C-1")}
	if _, err := gateway.CancelCombo(context.Background(), "c1"); err == nil || !strings.Contains(err.Error(), "combo trading service is unavailable") {
		t.Fatalf("non-combo cancel error = %v", err)
	}
	trading := &gatewayTrading{}
	gateway.dependencies.ResolveBroker = func(id string) broker.Broker { return &gatewayBroker{id: id, trading: trading} }
	store.orders["c1"] = trdsrv.ExecutionOrder{BrokerID: "futu"}
	if _, err := gateway.CancelCombo(context.Background(), "c1"); err == nil || !strings.Contains(err.Error(), "missing broker order id") {
		t.Fatalf("missing combo broker id error = %v", err)
	}
	store.orders["c1"] = trdsrv.ExecutionOrder{BrokerID: "futu", BrokerOrderID: ptr("C-1")}
	trading.comboCancelErr = errors.New("combo cancel failed")
	if _, err := gateway.CancelCombo(context.Background(), "c1"); !errors.Is(err, trading.comboCancelErr) {
		t.Fatalf("combo cancel error = %v", err)
	}
	trading.comboCancelErr = nil
	updated, err := gateway.CancelCombo(context.Background(), "c1")
	if err != nil || !store.cancelled["c1"] || trading.cancelComboID != "C-1" {
		t.Fatalf("successful combo cancel = %#v, %v", updated, err)
	}
}

func TestExecutionGatewayHelpers(t *testing.T) {
	if got := firstNonEmptyString("", "futu", "bbgo"); got != "futu" {
		t.Fatalf("firstNonEmptyString = %q", got)
	}
	if got := firstNonEmptyString(); got != "" {
		t.Fatalf("empty firstNonEmptyString = %q", got)
	}
	if got := derefString(nil); got != "" {
		t.Fatalf("nil derefString = %q", got)
	}
	value := "v"
	if got := derefString(&value); got != "v" {
		t.Fatalf("derefString = %q", got)
	}
}
