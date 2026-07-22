package servercore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionOrderStoreSortingFilteringAndMissingOrderBoundaries(t *testing.T) {
	store := newExecutionOrderStore()
	store.orders["exec-000001"] = executionOrderSummaryResponse{
		InternalOrderID:    "exec-000001",
		BrokerID:           "futu",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Status:             "SUBMITTED",
		UpdatedAt:          "2026-07-03T08:00:00Z",
		CreatedAt:          "2026-07-03T07:00:00Z",
	}
	store.orders["exec-000002"] = executionOrderSummaryResponse{
		InternalOrderID:    "exec-000002",
		BrokerID:           "futu",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Status:             "SUBMITTED",
		UpdatedAt:          "2026-07-03T08:00:00Z",
		CreatedAt:          "2026-07-03T07:30:00Z",
	}
	store.orders["exec-000003"] = executionOrderSummaryResponse{
		InternalOrderID:    "exec-000003",
		BrokerID:           "futu",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Status:             "SUBMITTED",
		UpdatedAt:          "2026-07-03T08:00:00Z",
		CreatedAt:          "2026-07-03T07:30:00Z",
	}

	filtered := store.listOrdersFiltered(executionOrderListFilter{BrokerID: "FUTU"})
	if len(filtered.Orders) != 3 {
		t.Fatalf("filtered orders len = %d, want 3", len(filtered.Orders))
	}
	if filtered.Orders[0].InternalOrderID != "exec-000003" || filtered.Orders[1].InternalOrderID != "exec-000002" || filtered.Orders[2].InternalOrderID != "exec-000001" {
		t.Fatalf("sorted orders = %#v, want createdAt/internalOrderID descending tie-breaks", filtered.Orders)
	}

	if executionOrderMatchesListFilter(store.orders["exec-000001"], executionOrderListFilter{BrokerID: "ib"}) {
		t.Fatal("broker mismatch unexpectedly matched")
	}
	if executionOrderMatchesListFilter(store.orders["exec-000001"], executionOrderListFilter{Market: "US"}) {
		t.Fatal("market mismatch unexpectedly matched")
	}

	if _, ok := store.order("missing-order"); ok {
		t.Fatal("missing order unexpectedly found")
	}
}

func TestExecutionOrderStorePlacedOrderPreservesMissingBrokerAndRepairsSparseSummary(t *testing.T) {
	store := newExecutionOrderStore()

	defaulted := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           " \t ",
		BrokerOrderID:      "NEW-DEFAULT",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		RequestedQuantity:  1,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	if defaulted.BrokerID != "" || defaulted.Status != "SUBMITTED" {
		t.Fatalf("placed order with missing broker = %#v", defaulted)
	}

	legacyRequestedQuantity := 100.0
	legacyRequestedPrice := 88.5
	legacyFilledQuantity := 0.0
	store.orders["exec-legacy"] = executionOrderSummaryResponse{
		InternalOrderID:    "exec-legacy",
		BrokerID:           "futu",
		BrokerOrderID:      stringPointerOrNil("LEGACY-100"),
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             stringPointerOrNil("HK.00005"),
		Side:               stringPointerOrNil("BUY"),
		OrderType:          stringPointerOrNil("LIMIT"),
		Status:             "",
		RequestedQuantity:  &legacyRequestedQuantity,
		RequestedPrice:     &legacyRequestedPrice,
		FilledQuantity:     &legacyFilledQuantity,
		SubmittedAt:        nil,
		UpdatedAt:          "",
		CreatedAt:          "",
		Source:             "",
		SourceDetail:       "",
	}
	store.linkBrokerOrderLocked(store.orders["exec-legacy"])

	updatedPrice := 90.25
	merged := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "LEGACY-100",
		BrokerOrderIDEx:    "LEGACY-100-EX",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "SELL",
		OrderType:          "MARKET",
		Status:             "SUBMITTED",
		RequestedQuantity:  200,
		RequestedPrice:     &updatedPrice,
		Remark:             "replayed after reconnect",
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	if merged.InternalOrderID != "exec-legacy" {
		t.Fatalf("merged order id = %q, want exec-legacy", merged.InternalOrderID)
	}
	if merged.BrokerOrderIDEx == nil || *merged.BrokerOrderIDEx != "LEGACY-100-EX" {
		t.Fatalf("merged brokerOrderIDEx = %#v", merged.BrokerOrderIDEx)
	}
	if merged.Symbol == nil || *merged.Symbol != "HK.00700" || merged.Side == nil || *merged.Side != "SELL" || merged.OrderType == nil || *merged.OrderType != "MARKET" {
		t.Fatalf("merged order identity = %#v", merged)
	}
	if merged.RequestedQuantity == nil || *merged.RequestedQuantity != 200 || merged.RequestedPrice == nil || *merged.RequestedPrice != updatedPrice {
		t.Fatalf("merged order economics = %#v", merged)
	}
	if merged.SubmittedAt == nil || *merged.SubmittedAt == "" || merged.UpdatedAt == "" || merged.CreatedAt == "" {
		t.Fatalf("merged order timestamps = %#v", merged)
	}
	if merged.Source != "system" || merged.SourceDetail != "command.place" || merged.Status != "BROKER_ACCEPTED" {
		t.Fatalf("merged order source/status = %#v", merged)
	}
	events := store.orderEvents("exec-legacy")
	if len(events.Events) != 1 || events.Events[0].EventType != "COMMAND_PLACE_ACCEPTED" {
		t.Fatalf("merged order events = %#v", events.Events)
	}
}

func TestExecutionOrderStoreBrokerSyncPreservesMissingBrokerAndRepairsIncompleteSummary(t *testing.T) {
	store := newExecutionOrderStore()

	discovered, event, changed := store.upsertBrokerOrderWithSource(" \t ", broker.OrderSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "DISCOVERED-1",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		Quantity:           1,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "", "")
	if !changed || event == nil {
		t.Fatalf("discovered broker order changed=%v event=%#v", changed, event)
	}
	if discovered.BrokerID != "" || discovered.Source != "broker" || discovered.SourceDetail != "broker.current" {
		t.Fatalf("discovered broker order = %#v", discovered)
	}

	legacyRequestedQuantity := 1.0
	legacyRequestedPrice := 50.0
	legacyLastErrorCode := "LEGACY-ERROR"
	store.orders["exec-repair"] = executionOrderSummaryResponse{
		InternalOrderID:    "exec-repair",
		BrokerID:           "futu",
		BrokerOrderID:      stringPointerOrNil("SYNC-REPAIR"),
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-002",
		Market:             "US",
		Status:             "SUBMITTED",
		RequestedQuantity:  &legacyRequestedQuantity,
		RequestedPrice:     &legacyRequestedPrice,
		LastErrorCode:      &legacyLastErrorCode,
		UpdatedAt:          "",
		CreatedAt:          "2026-07-02T00:00:00Z",
		Source:             "",
		SourceDetail:       "",
	}
	store.linkBrokerOrderLocked(store.orders["exec-repair"])

	repairOrderIDEx := "SYNC-REPAIR-EX"
	repairedPrice := 51.5
	repairedFilledQuantity := 0.5
	repairedAveragePrice := 51.5
	repairedRemark := "snapshot repaired sparse cache entry"
	repaired, updateEvent, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID:          "SIM-002",
		TradingEnvironment: "SIMULATE",
		Market:             "US",
		BrokerOrderID:      "SYNC-REPAIR",
		BrokerOrderIDEx:    &repairOrderIDEx,
		Symbol:             "US.AAPL",
		Side:               "SELL",
		OrderType:          "LIMIT",
		Status:             "FILLED_PART",
		Quantity:           2,
		Price:              &repairedPrice,
		FilledQuantity:     &repairedFilledQuantity,
		FilledAveragePrice: &repairedAveragePrice,
		Remark:             &repairedRemark,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "", "")
	if !changed || updateEvent == nil || updateEvent.EventType != "BROKER_SYNC_UPDATED" {
		t.Fatalf("repaired broker sync changed=%v event=%#v", changed, updateEvent)
	}
	if repaired.BrokerOrderIDEx == nil || *repaired.BrokerOrderIDEx != repairOrderIDEx {
		t.Fatalf("repaired brokerOrderIDEx = %#v", repaired.BrokerOrderIDEx)
	}
	if repaired.Symbol == nil || *repaired.Symbol != "US.AAPL" || repaired.Side == nil || *repaired.Side != "SELL" || repaired.OrderType == nil || *repaired.OrderType != "LIMIT" {
		t.Fatalf("repaired order identity = %#v", repaired)
	}
	if repaired.RequestedQuantity == nil || *repaired.RequestedQuantity != 2 || repaired.RequestedPrice == nil || *repaired.RequestedPrice != repairedPrice {
		t.Fatalf("repaired requested economics = %#v", repaired)
	}
	if repaired.FilledQuantity == nil || *repaired.FilledQuantity != repairedFilledQuantity || repaired.FilledAveragePrice == nil || *repaired.FilledAveragePrice != repairedAveragePrice {
		t.Fatalf("repaired filled economics = %#v", repaired)
	}
	if repaired.Remark == nil || *repaired.Remark != repairedRemark || repaired.SubmittedAt == nil || *repaired.SubmittedAt != "2026-07-02T00:00:00Z" || repaired.UpdatedAt == "" {
		t.Fatalf("repaired broker metadata = %#v", repaired)
	}
	if repaired.LastErrorCode != nil || repaired.LastErrorSource != nil || repaired.Source != "broker" || repaired.SourceDetail != "broker.current" {
		t.Fatalf("repaired broker diagnostics/source = %#v", repaired)
	}
}

func TestExecutionOrderStorePersistenceWorkerQueueAndFallbackPaths(t *testing.T) {
	t.Run("queued worker flushes and close is idempotent", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "execution-orders.db")
		persistence, err := newExecutionOrderSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
		}

		store := newExecutionOrderStore()
		store.persistence = persistence
		store.startPersistenceWorker()
		store.startPersistenceWorker()
		store.enqueuePersistence(executionPersistenceItem{kind: "sequence", seqName: "orders", seqValue: 7})

		if err := store.Close(); err != nil {
			t.Fatalf("store.Close(): %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("second store.Close(): %v", err)
		}

		reloaded, err := newExecutionOrderSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("reload persistence: %v", err)
		}
		defer func() { jftradeCheckTestError(t, reloaded.Close()) }()

		sequences, err := reloaded.loadSequences()
		if err != nil {
			t.Fatalf("loadSequences(): %v", err)
		}
		if sequences["orders"] != 7 {
			t.Fatalf("persisted order sequence = %d, want 7", sequences["orders"])
		}
	})

	t.Run("nil queue skips and full queue preserves FIFO order with backpressure", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "execution-orders.db")
		persistence, err := newExecutionOrderSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
		}

		store := newExecutionOrderStore()
		store.persistence = persistence
		store.enqueuePersistence(executionPersistenceItem{kind: "sequence", seqName: "ignored", seqValue: 3})

		sequences, err := persistence.loadSequences()
		if err != nil {
			t.Fatalf("loadSequences() after nil queue: %v", err)
		}
		if len(sequences) != 0 {
			t.Fatalf("nil queue unexpectedly persisted sequences: %#v", sequences)
		}

		store.persistenceQueue = make(chan executionPersistenceItem, 1)
		store.persistenceQueue <- executionPersistenceItem{kind: "sequence", seqName: "events", seqValue: 10}
		enqueued := make(chan struct{})
		go func() {
			store.enqueuePersistence(executionPersistenceItem{kind: "sequence", seqName: "events", seqValue: 11})
			close(enqueued)
		}()
		select {
		case <-enqueued:
			t.Fatal("full persistence queue bypassed FIFO backpressure")
		case <-time.After(25 * time.Millisecond):
		}

		store.persistenceWG.Add(1)
		go store.runPersistenceWorker(store.persistenceQueue)
		select {
		case <-enqueued:
		case <-time.After(2 * time.Second):
			t.Fatal("enqueue did not resume after persistence worker drained the queue")
		}
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close(): %v", err)
		}

		reloaded, err := newExecutionOrderSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("reload persistence: %v", err)
		}
		defer func() { jftradeCheckTestError(t, reloaded.Close()) }()
		sequences, err = reloaded.loadSequences()
		if err != nil {
			t.Fatalf("loadSequences() after backpressure writes: %v", err)
		}
		if sequences["events"] != 11 {
			t.Fatalf("FIFO persisted event sequence = %d, want 11", sequences["events"])
		}
	})
}

func TestExecutionOrderSQLiteStoreIgnoresBlankIdentifiersAndZeroCutoff(t *testing.T) {
	persistence, err := newExecutionOrderSQLiteStore(filepath.Join(t.TempDir(), "execution-orders.db"))
	if err != nil {
		t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
	}
	defer func() { jftradeCheckTestError(t, persistence.Close()) }()

	if err := persistence.persistSeenFillKey(" \t ", "2026-07-03T00:00:00Z"); err != nil {
		t.Fatalf("persistSeenFillKey(blank): %v", err)
	}
	if err := persistence.persistSequence(" \t ", 42); err != nil {
		t.Fatalf("persistSequence(blank): %v", err)
	}
	if err := persistence.deleteSeenFillKeysBefore(time.Time{}); err != nil {
		t.Fatalf("deleteSeenFillKeysBefore(zero): %v", err)
	}

	fillKeys, err := persistence.loadSeenFillKeys()
	if err != nil {
		t.Fatalf("loadSeenFillKeys(): %v", err)
	}
	sequences, err := persistence.loadSequences()
	if err != nil {
		t.Fatalf("loadSequences(): %v", err)
	}
	if len(fillKeys) != 0 || len(sequences) != 0 {
		t.Fatalf("blank writes unexpectedly persisted fillKeys=%#v sequences=%#v", fillKeys, sequences)
	}
}
