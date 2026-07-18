package servercore

import (
	"errors"
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCoverage98OrderUpdateSourceDegradesCleanlyWithoutActiveFutuRuntime(t *testing.T) {
	server := newTradingAdapterCoverageServer(t)
	source := &tradingOrderUpdateSource{server: server}

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

	readQuery := brokerOrderQuery(query)
	if readQuery.BrokerID != "futu" || readQuery.AccountID != "SIM-001" || readQuery.TradingEnvironment != "SIMULATE" || readQuery.Market != "US" {
		t.Fatalf("broker read query normalization = %#v", readQuery)
	}
}

func TestCoverage98OrderUpdateMappingsRetainBrokerLifecycleFields(t *testing.T) {
	price := 101.25
	amount := 250.0
	payout := 75.0
	filledQuantity := 1.0
	externalID := "ORD-EXT-1"
	fillsID := "FILL-EXT-1"
	fromBroker := broker.OrderSnapshot{
		AccountID: "SIM-001", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "101", BrokerOrderIDEx: &externalID, Symbol: "US.AAPL", SymbolName: stringPointerOrNil("Apple"),
		Side: "BUY", OrderType: "LIMIT", Status: "SUBMITTED", Quantity: 2, FilledQuantity: &filledQuantity,
		OrderKind: broker.OrderKindEventParlay, ProductClass: broker.ProductClassEventContract,
		QuantityMode: broker.QuantityModeAmount, Amount: &amount,
		Legs: []broker.OrderLegSnapshot{{
			BrokerLegID: "LEG-1", InstrumentID: "US.EVENT.ONE",
			ProductClass: broker.ProductClassEventContract, PredictionSide: "YES",
			Status: "SUBMITTED", RequestedAmount: amount,
		}},
		Price: &price, FilledAveragePrice: &price, SubmittedAt: "2026-07-01T10:00:00Z", UpdatedAt: "2026-07-01T10:01:00Z",
		Remark: stringPointerOrNil("coverage"), LastError: stringPointerOrNil("none"), TimeInForce: stringPointerOrNil("DAY"), Currency: stringPointerOrNil("USD"),
	}
	mapped := tradingOrdersFromBroker("futu", []broker.OrderSnapshot{fromBroker})
	if len(mapped) != 1 || mapped[0].BrokerOrderIDEx == nil || *mapped[0].BrokerOrderIDEx != externalID || mapped[0].Price == nil || *mapped[0].Price != price {
		t.Fatalf("broker order mapping = %#v", mapped)
	}
	if mapped[0].BrokerID != "futu" ||
		mapped[0].OrderKind != broker.OrderKindEventParlay ||
		mapped[0].ProductClass != broker.ProductClassEventContract ||
		mapped[0].QuantityMode != broker.QuantityModeAmount ||
		mapped[0].Amount == nil || *mapped[0].Amount != amount ||
		len(mapped[0].Legs) != 1 {
		t.Fatalf("broker-neutral lifecycle fields were lost = %#v", mapped[0])
	}
	if back := brokerOrderFromTrading(mapped[0]); back.BrokerOrderID != fromBroker.BrokerOrderID ||
		back.BrokerOrderIDEx == nil || *back.BrokerOrderIDEx != externalID ||
		back.TimeInForce == nil || *back.TimeInForce != "DAY" ||
		back.OrderKind != broker.OrderKindEventParlay ||
		back.Amount == nil || *back.Amount != amount ||
		len(back.Legs) != 1 {
		t.Fatalf("trading order round trip = %#v", back)
	}

	fill := trdsrv.Fill{
		AccountID: "SIM-001", TradingEnvironment: "SIMULATE", Market: "US", BrokerOrderID: "101", BrokerOrderIDEx: &externalID,
		BrokerFillID: "900", BrokerFillIDEx: &fillsID, Symbol: "US.AAPL", SymbolName: stringPointerOrNil("Apple"), Side: "BUY",
		FilledQuantity: 1, FillPrice: &price, FilledAt: "2026-07-01T10:01:00Z", Status: stringPointerOrNil("FILLED"),
		Payout: &payout,
	}
	if mappedFill := brokerFillFromTrading(fill); mappedFill.BrokerFillIDEx == nil ||
		*mappedFill.BrokerFillIDEx != fillsID ||
		mappedFill.FillPrice == nil || *mappedFill.FillPrice != price ||
		mappedFill.Payout == nil || *mappedFill.Payout != payout {
		t.Fatalf("broker fill mapping = %#v", mappedFill)
	}

	// Pushes may arrive after server shutdown. Nil update sinks are intentionally
	// harmless, while duplicate broker updates do not create duplicate records.
	var nilUpdates *tradingExecutionOrderUpdates
	nilUpdates.ApplyOrder(t.Context(), "futu", mapped[0], trdsrv.OrderWriteMetadata{})
	nilUpdates.ApplyFill(t.Context(), "futu", fill)

	server := newTradingAdapterCoverageServer(t)
	updates := &tradingExecutionOrderUpdates{server: server}
	metadata := trdsrv.OrderWriteMetadata{DiscoveredEventType: "BROKER_DISCOVERED", UpdatedEventType: "BROKER_UPDATED", Source: "broker", SourceDetail: "poll"}
	updates.ApplyOrder(t.Context(), "futu", mapped[0], metadata)
	updates.ApplyOrder(t.Context(), "futu", mapped[0], metadata)
	orders := server.executionOrders.listOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("duplicate broker order push created %d records", len(orders))
	}
	updates.ApplyFill(t.Context(), "futu", fill)
	updates.ApplyFill(t.Context(), "futu", fill)
	updated, ok := server.executionOrders.order(orders[0].InternalOrderID)
	if !ok || updated.Payout == nil || *updated.Payout != payout {
		t.Fatalf("prediction payout was not persisted on parent order = %#v", updated)
	}
	if events := server.executionOrders.orderEvents(orders[0].InternalOrderID).Events; len(events) != 2 {
		t.Fatalf("duplicate broker fill push events = %#v", events)
	}
}
