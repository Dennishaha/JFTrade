package servercore

import (
	"path/filepath"
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionStoreRemainingMergeTimestampAndFillKeyBoundaries(t *testing.T) {
	store := newExecutionOrderStore()
	existing := executionOrderSummaryResponse{
		InternalOrderID:    "exec-existing",
		BrokerID:           "futu",
		BrokerOrderID:      stringPointerOrNil("broker-1"),
		TradingEnvironment: "SIMULATE",
		AccountID:          "account",
		Market:             "US",
		Status:             trdsrv.OrderStatusUnknown,
	}
	store.orders[existing.InternalOrderID] = existing
	store.linkBrokerOrderLocked(existing)
	merged := store.mergePlacedOrderLocked(existing.InternalOrderID, executionPlacedOrderRecord{}, "now", "created")
	if merged.Status != trdsrv.OrderStatusSubmitted {
		t.Fatalf("blank merge status = %q", merged.Status)
	}

	if !executionTimestampAdvances("", "incoming") {
		t.Fatal("empty current timestamp should advance")
	}
	if !executionTimestampAdvances("bad-current", "bad-incoming") {
		t.Fatal("unparseable timestamps should advance")
	}
	if store.registerFillKeyLocked("", "now") {
		t.Fatal("blank fill key should not be registered as duplicate")
	}
}

func TestExecutionStoreRemainingSnapshotIdentityAndFillDefaults(t *testing.T) {
	summary := executionOrderSummaryResponse{}
	changed := applyBrokerOrderSnapshotIdentity(&summary, broker.OrderSnapshot{
		Market:             "US",
		AccountID:          "account",
		TradingEnvironment: "SIMULATE",
	})
	if !changed || summary.Market != "US" || summary.AccountID != "account" || summary.TradingEnvironment != "SIMULATE" {
		t.Fatalf("identity update = %#v changed=%v", summary, changed)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	fromFill := brokerOrderSummaryFromFill("exec-fill", "", broker.OrderFillSnapshot{}, now)
	if fromFill.BrokerID != "" || fromFill.Status != trdsrv.OrderStatusPartiallyFilled {
		t.Fatalf("fill defaults = %#v", fromFill)
	}

	ex := "broker-order-ex"
	price := 12.5
	fill := broker.OrderFillSnapshot{
		BrokerOrderIDEx: &ex,
		FilledQuantity:  1,
		FillPrice:       &price,
		FilledAt:        now,
	}
	fillTarget := executionOrderSummaryResponse{Status: trdsrv.OrderStatusSubmitted}
	applyBrokerOrderFill(&fillTarget, fill, now)
	if fillTarget.BrokerOrderIDEx == nil || *fillTarget.BrokerOrderIDEx != ex || fillTarget.SubmittedAt == nil || fillTarget.Source != "broker" || fillTarget.SourceDetail != "broker.fill" {
		t.Fatalf("applied fill defaults = %#v", fillTarget)
	}
}

func TestExecutionStorePersistsParentBrokerFeesWithoutInventingLegAllocation(t *testing.T) {
	store := newExecutionOrderStore()
	order := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID: "coverage", BrokerOrderID: "101", BrokerOrderIDEx: "ORDER-EX-101",
		TradingEnvironment: "SIMULATE", AccountID: "account", Market: "US",
		Status: "FILLED_ALL", OrderKind: broker.OrderKindOptionCombo,
		ProductClass: broker.ProductClassOption,
		Legs: []broker.OrderLegIntent{{
			InstrumentID: "US.AAPL260717C00200000",
			ProductClass: broker.ProductClassOption,
			Side:         "BUY", Ratio: 1,
		}},
	})
	updated, event, changed := store.recordBrokerOrderFee("coverage", broker.OrderFeeSnapshot{
		AccountID: "account", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderIDEx: "ORDER-EX-101",
		FeeItems: []broker.OrderFeeItemSnapshot{
			{Title: "commission", Value: 1.25},
			{Title: "platform", Value: 0.75},
		},
	})
	if !changed || event == nil || event.EventType != "BROKER_ORDER_FEES_UPDATED" ||
		updated.InternalOrderID != order.InternalOrderID ||
		updated.Fees == nil || *updated.Fees != 2 {
		t.Fatalf("parent fee update = %#v event=%#v changed=%v", updated, event, changed)
	}
	if len(updated.Legs) != 1 || updated.Legs[0].Fees != nil {
		t.Fatalf("broker parent fee was incorrectly allocated to legs = %#v", updated.Legs)
	}
	if _, duplicateEvent, duplicateChanged := store.recordBrokerOrderFee("coverage", broker.OrderFeeSnapshot{
		AccountID: "account", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderIDEx: "ORDER-EX-101", FeeAmount: new(2.0),
	}); duplicateChanged || duplicateEvent != nil {
		t.Fatalf("duplicate fee update changed=%v event=%#v", duplicateChanged, duplicateEvent)
	}
}

func TestBrokerSnapshotDoesNotDowngradePreviewLockedProduct(t *testing.T) {
	summary := executionOrderSummaryResponse{
		OrderKind:    broker.OrderKindSingle,
		ProductClass: broker.ProductClassOption,
		QuantityMode: broker.QuantityModeContracts,
	}
	changed := applyBrokerOrderSnapshotIdentity(&summary, broker.OrderSnapshot{
		OrderKind:    broker.OrderKindSingle,
		ProductClass: broker.ProductClassUnknown,
		QuantityMode: broker.QuantityModeUnits,
		Market:       "US",
	})
	if !changed {
		t.Fatal("market update should still be applied")
	}
	if summary.ProductClass != broker.ProductClassOption ||
		summary.QuantityMode != broker.QuantityModeContracts {
		t.Fatalf("preview-locked product was downgraded: %#v", summary)
	}
}

func TestExecutionStoreRemainingPersistenceLifecycleBoundaries(t *testing.T) {
	var nilStore *executionOrderStore
	nilStore.startPersistenceWorker()
	nilStore.writePersistenceItem(executionPersistenceItem{kind: "order", order: executionOrderSummaryResponse{InternalOrderID: "exec-nil"}})
	nilStore.configureSeenFillRetention(1)
	if err := nilStore.Close(); err != nil {
		t.Fatalf("nil store Close: %v", err)
	}

	store := newExecutionOrderStore()
	store.startPersistenceWorker()
	if store.persistenceQueue != nil {
		t.Fatal("startPersistenceWorker without persistence should not create a queue")
	}
	store.writePersistenceItem(executionPersistenceItem{kind: "order", order: executionOrderSummaryResponse{InternalOrderID: "exec-no-persistence"}})
	if len(store.orders) != 0 {
		t.Fatalf("writePersistenceItem without persistence mutated orders = %#v", store.orders)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store Close: %v", err)
	}
	if !store.persistenceClosed {
		t.Fatal("store Close should mark persistenceClosed")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second store Close: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "closed.db")
	persistence, err := newExecutionOrderSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new persistence: %v", err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatalf("close persistence: %v", err)
	}
	closedStore := newExecutionOrderStore()
	closedStore.persistence = persistence
	closedStore.writePersistenceItem(executionPersistenceItem{kind: "order", order: executionOrderSummaryResponse{InternalOrderID: "exec-fail"}})

	reloaded, err := newExecutionOrderStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	if orders := reloaded.listOrders(); len(orders.Orders) != 0 {
		t.Fatalf("write to closed persistence was stored: %#v", orders.Orders)
	}
}
