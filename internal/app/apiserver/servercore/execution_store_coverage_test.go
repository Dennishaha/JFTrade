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
	if fromFill.BrokerID != "futu" || fromFill.Status != trdsrv.OrderStatusPartiallyFilled {
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

func TestExecutionStoreRemainingPersistenceLifecycleBoundaries(t *testing.T) {
	var nilStore *executionOrderStore
	nilStore.startPersistenceWorker()
	nilStore.writePersistenceItem(executionPersistenceItem{})
	nilStore.configureSeenFillRetention(1)
	if err := nilStore.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}

	persistence, err := newExecutionOrderSQLiteStore(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("new persistence: %v", err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatalf("close persistence: %v", err)
	}
	store := newExecutionOrderStore()
	store.persistence = persistence
	store.writePersistenceItem(executionPersistenceItem{kind: "order", order: executionOrderSummaryResponse{InternalOrderID: "exec-fail"}})
}
