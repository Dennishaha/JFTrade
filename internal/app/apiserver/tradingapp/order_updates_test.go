package tradingapp

import (
	"errors"
	"testing"
	"time"

	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestNewOrderUpdatesWorkerConstructsWorker(t *testing.T) {
	worker := NewOrderUpdatesWorker(nil, nil, trdsrv.OrderUpdatesConfig{})
	if worker == nil {
		t.Fatal("NewOrderUpdatesWorker returned nil")
	}
}

func TestBrokerOrderMappingsPreserveLifecycleFields(t *testing.T) {
	price := 101.25
	amount := 250.0
	payout := 75.0
	filledQuantity := 1.0
	externalID := "ORD-EXT-1"
	fillExternalID := "FILL-EXT-1"
	fromBroker := broker.OrderSnapshot{
		AccountID: "SIM-001", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "101", BrokerOrderIDEx: &externalID, Symbol: "US.AAPL", SymbolName: new("Apple"),
		Side: "BUY", OrderType: "LIMIT", Status: "SUBMITTED", Quantity: 2, FilledQuantity: &filledQuantity,
		OrderKind: broker.OrderKindEventParlay, ProductClass: broker.ProductClassEventContract,
		QuantityMode: broker.QuantityModeAmount, Amount: &amount,
		Legs: []broker.OrderLegSnapshot{{
			BrokerLegID: "LEG-1", InstrumentID: "US.EVENT.ONE",
			ProductClass: broker.ProductClassEventContract, PredictionSide: "YES",
			Status: "SUBMITTED", RequestedAmount: amount,
		}},
		Price: &price, FilledAveragePrice: &price, SubmittedAt: "2026-07-01T10:00:00Z", UpdatedAt: "2026-07-01T10:01:00Z",
		Remark: new("lifecycle"), LastError: new("none"), TimeInForce: new("DAY"), Currency: new("USD"),
	}

	mapped := tradingOrdersFromBroker("futu", []broker.OrderSnapshot{fromBroker})
	if len(mapped) != 1 || mapped[0].BrokerOrderIDEx == nil || *mapped[0].BrokerOrderIDEx != externalID {
		t.Fatalf("broker order mapping = %#v", mapped)
	}
	if mapped[0].OrderKind != broker.OrderKindEventParlay ||
		mapped[0].Amount == nil || *mapped[0].Amount != amount || len(mapped[0].Legs) != 1 {
		t.Fatalf("broker-neutral lifecycle fields were lost = %#v", mapped[0])
	}
	if back := brokerOrderFromTrading(mapped[0]); back.BrokerOrderID != fromBroker.BrokerOrderID ||
		back.BrokerOrderIDEx == nil || *back.BrokerOrderIDEx != externalID ||
		back.TimeInForce == nil || *back.TimeInForce != "DAY" ||
		back.Amount == nil || *back.Amount != amount || len(back.Legs) != 1 {
		t.Fatalf("trading order round trip = %#v", back)
	}

	fill := trdsrv.Fill{
		AccountID: "SIM-001", TradingEnvironment: "SIMULATE", Market: "US", BrokerOrderID: "101", BrokerOrderIDEx: &externalID,
		BrokerFillID: "900", BrokerFillIDEx: &fillExternalID, Symbol: "US.AAPL", SymbolName: new("Apple"), Side: "BUY",
		FilledQuantity: 1, FillPrice: &price, FilledAt: "2026-07-01T10:01:00Z", Status: new("FILLED"), Payout: &payout,
	}
	mappedFill := brokerFillFromTrading(fill)
	if mappedFill.BrokerFillIDEx == nil || *mappedFill.BrokerFillIDEx != fillExternalID ||
		mappedFill.FillPrice == nil || *mappedFill.FillPrice != price ||
		mappedFill.Payout == nil || *mappedFill.Payout != payout {
		t.Fatalf("broker fill mapping = %#v", mappedFill)
	}
}

func TestBrokerOrderQueryTrimsRuntimeIdentifiers(t *testing.T) {
	query := brokerOrderQuery(trdsrv.OrderQuery{
		BrokerID: " futu ", AccountID: " SIM-001 ", TradingEnvironment: " SIMULATE ", Market: " US ",
	})
	if query.BrokerID != "futu" || query.AccountID != "SIM-001" ||
		query.TradingEnvironment != "SIMULATE" || query.Market != "US" {
		t.Fatalf("broker read query normalization = %#v", query)
	}
}

func TestExecutionOrderUpdatesIgnoreMissingLedger(t *testing.T) {
	var nilUpdates *ExecutionOrderUpdates
	nilUpdates.ApplyOrder(t.Context(), "futu", trdsrv.Order{}, trdsrv.OrderWriteMetadata{})
	nilUpdates.ApplyFill(t.Context(), "futu", trdsrv.Fill{})
	nilUpdates.ApplyFees(t.Context(), "futu", []broker.OrderFeeSnapshot{{BrokerOrderIDEx: "missing"}})

	updates := NewExecutionOrderUpdates(nil, nil)
	updates.ApplyOrder(t.Context(), "futu", trdsrv.Order{}, trdsrv.OrderWriteMetadata{})
	updates.ApplyFill(t.Context(), "futu", trdsrv.Fill{})
	updates.ApplyFees(t.Context(), "futu", nil)
}

func TestOrderUpdateSourceDegradesCleanlyWithoutActiveFutuRuntime(t *testing.T) {
	source := NewOrderUpdateSource(OrderUpdateSourceOptions{})

	if accounts, err := source.DiscoverAccounts(t.Context()); !errors.Is(err, trdsrv.ErrOrderUpdateSourceInactive) || accounts != nil {
		t.Fatalf("DiscoverAccounts without active broker = %#v / %v", accounts, err)
	}
	query := trdsrv.OrderQuery{BrokerID: " futu ", AccountID: " SIM-001 ", TradingEnvironment: " SIMULATE ", Market: " US "}
	if orders, err := source.CurrentOrders(t.Context(), query); err != nil || orders != nil {
		t.Fatalf("CurrentOrders without market data = %#v / %v", orders, err)
	}
	if orders, err := source.HistoryOrders(t.Context(), query, time.Now().Add(-time.Hour), time.Now()); err != nil || orders != nil {
		t.Fatalf("HistoryOrders without market data = %#v / %v", orders, err)
	}
	subscription, err := source.Subscribe(t.Context(), nil, nil, nil)
	if err != nil {
		t.Fatalf("Subscribe without Futu exchange = %v", err)
	}
	if err := subscription.Stop(); err != nil {
		t.Fatalf("no-op subscription Stop = %v", err)
	}
}

func TestExecutionOrderUpdatesPersistBrokerLifecycleFields(t *testing.T) {
	price := 101.25
	amount := 250.0
	payout := 75.0
	filledQuantity := 1.0
	externalID := "ORD-EXT-1"
	fillsID := "FILL-EXT-1"
	order := trdsrv.Order{
		BrokerID:  "futu",
		AccountID: "SIM-001", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "101", BrokerOrderIDEx: &externalID, Symbol: "US.AAPL", SymbolName: new("Apple"),
		Side: "BUY", OrderType: "LIMIT", Status: "SUBMITTED", Quantity: 2, FilledQuantity: &filledQuantity,
		OrderKind: broker.OrderKindEventParlay, ProductClass: broker.ProductClassEventContract,
		QuantityMode: broker.QuantityModeAmount, Amount: &amount,
		Legs: []broker.OrderLegSnapshot{{
			BrokerLegID: "LEG-1", InstrumentID: "US.EVENT.ONE",
			ProductClass: broker.ProductClassEventContract, PredictionSide: "YES",
			Status: "SUBMITTED", RequestedAmount: amount,
		}},
		Price: &price, FilledAveragePrice: &price, SubmittedAt: "2026-07-01T10:00:00Z", UpdatedAt: "2026-07-01T10:01:00Z",
		Remark: new("lifecycle"), LastError: new("none"), TimeInForce: new("DAY"), Currency: new("USD"),
	}

	fill := trdsrv.Fill{
		AccountID: "SIM-001", TradingEnvironment: "SIMULATE", Market: "US", BrokerOrderID: "101", BrokerOrderIDEx: &externalID,
		BrokerFillID: "900", BrokerFillIDEx: &fillsID, Symbol: "US.AAPL", SymbolName: new("Apple"), Side: "BUY",
		FilledQuantity: 1, FillPrice: &price, FilledAt: "2026-07-01T10:01:00Z", Status: new("FILLED"),
		Payout: &payout,
	}
	store := tradingstore.NewInMemory()
	updates := NewExecutionOrderUpdates(store, nil)
	metadata := trdsrv.OrderWriteMetadata{DiscoveredEventType: "BROKER_DISCOVERED", UpdatedEventType: "BROKER_UPDATED", Source: "broker", SourceDetail: "poll"}
	updates.ApplyOrder(t.Context(), "futu", order, metadata)
	updates.ApplyOrder(t.Context(), "futu", order, metadata)
	orders := store.AllOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("duplicate broker order push created %d records", len(orders))
	}
	updates.ApplyFill(t.Context(), "futu", fill)
	updates.ApplyFill(t.Context(), "futu", fill)
	updated, ok := store.Order(orders[0].InternalOrderID)
	if !ok || updated.Payout == nil || *updated.Payout != payout {
		t.Fatalf("prediction payout was not persisted on parent order = %#v", updated)
	}
	if events := store.Events(orders[0].InternalOrderID).Events; len(events) != 2 {
		t.Fatalf("duplicate broker fill push events = %#v", events)
	}
}
