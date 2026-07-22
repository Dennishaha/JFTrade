package servercore

import (
	"math"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestBrokerSnapshotCoveredFillQuantityLedgerBoundaries(t *testing.T) {
	fill := broker.OrderFillSnapshot{FilledQuantity: 2, FilledAt: "2026-07-22T02:00:00Z"}
	var nilStore *executionOrderStore
	if got := nilStore.brokerSnapshotCoveredFillQuantityLocked("order", fill); got != 0 {
		t.Fatalf("nil store credit = %v, want 0", got)
	}
	store := newExecutionOrderStore()
	if got := store.brokerSnapshotCoveredFillQuantityLocked("", fill); got != 0 {
		t.Fatalf("empty order id credit = %v, want 0", got)
	}
	if got := store.brokerSnapshotCoveredFillQuantityLocked("order", broker.OrderFillSnapshot{}); got != 0 {
		t.Fatalf("empty fill credit = %v, want 0", got)
	}

	store.events["order"] = []executionOrderEventResponse{
		{EventType: "COMMAND_PLACE_ACCEPTED", PayloadJSON: `{}`, CreatedAt: "2026-07-22T01:00:00Z"},
		{EventType: "BROKER_PUSH_ORDER", PayloadJSON: `{`, CreatedAt: "2026-07-22T01:00:00Z"},
		{EventType: "BROKER_PUSH_ORDER", PayloadJSON: `{"filledQuantity":0}`, CreatedAt: "2026-07-22T01:00:00Z"},
		{EventType: "BROKER_PUSH_ORDER", PayloadJSON: `{"filledQuantity":5,"updatedAt":"2026-07-22T01:00:00Z"}`, CreatedAt: "2026-07-22T01:00:00Z"},
		{EventType: "BROKER_PUSH_ORDER", PayloadJSON: `{"filledQuantity":8,"updatedAt":"2026-07-22T03:00:00Z"}`, CreatedAt: "2026-07-22T03:00:00Z"},
		{EventType: "BROKER_PUSH_UPDATED", PayloadJSON: `{"filledQuantity":10,"updatedAt":"2026-07-22T03:30:00Z"}`, CreatedAt: "2026-07-22T03:30:00Z"},
		{EventType: "BROKER_FILL_RECEIVED", PayloadJSON: `{`},
		{EventType: "BROKER_FILL_RECEIVED", PayloadJSON: `{"filledQuantity":0}`},
		{EventType: "BROKER_FILL_RECEIVED", PayloadJSON: `{"filledQuantity":4,"filledAt":"2026-07-22T02:30:00Z"}`},
	}
	if got := store.brokerSnapshotCoveredFillQuantityLocked("order", fill); got != 2 {
		t.Fatalf("capped snapshot credit = %v, want 2", got)
	}
	fill.FilledQuantity = 10
	if got := store.brokerSnapshotCoveredFillQuantityLocked("order", fill); got != 6 {
		t.Fatalf("partial snapshot credit = %v, want 6", got)
	}
	store.events["order"] = append(store.events["order"], executionOrderEventResponse{
		EventType: "BROKER_FILL_RECEIVED", PayloadJSON: `{"filledQuantity":6,"filledAt":"2026-07-22T03:00:00Z"}`,
	})
	if got := store.brokerSnapshotCoveredFillQuantityLocked("order", fill); got != 0 {
		t.Fatalf("exhausted snapshot credit = %v, want 0", got)
	}
}

func TestBrokerEventCoverageTimestampBoundaries(t *testing.T) {
	if !brokerEventCanCoverFill("", "2026-07-22T02:00:00Z") {
		t.Fatal("empty snapshot timestamp should conservatively cover fill")
	}
	if !brokerEventCanCoverFill("bad", "also-bad") {
		t.Fatal("malformed timestamps should conservatively cover fill")
	}
	if brokerEventCanCoverFill("2026-07-22T01:00:00Z", "2026-07-22T02:00:00Z") {
		t.Fatal("older snapshot should not cover newer fill")
	}
	if !brokerEventCanCoverFill("2026-07-22T02:00:00Z", "2026-07-22T02:00:00Z") {
		t.Fatal("same-time snapshot should cover fill")
	}
}

func TestBrokerFeeAndRuntimeNilBoundaries(t *testing.T) {
	store := newExecutionOrderStore()
	if _, event, changed := store.recordBrokerOrderFee("futu", broker.OrderFeeSnapshot{}); event != nil || changed {
		t.Fatalf("empty fee result = event=%+v changed=%v", event, changed)
	}
	amount := 1.5
	if _, event, changed := store.recordBrokerOrderFee("futu", broker.OrderFeeSnapshot{
		BrokerOrderIDEx: "missing-order", FeeAmount: &amount,
	}); event != nil || changed {
		t.Fatalf("unknown-order fee result = event=%+v changed=%v", event, changed)
	}

	var manager *strategyRuntimeManager
	manager.setPineWorkerRunner(nil)
	if manager.currentPineWorkerRunner() != nil {
		t.Fatal("nil manager returned a Pine worker")
	}
	var runtime *strategySymbolRuntime
	if runtime.brokerPositionsSnapshot() != nil || runtime.brokerAccountSnapshot() != nil {
		t.Fatal("nil strategy runtime returned account state")
	}

	server := &Server{serverRuntimes: serverRuntimes{strategyRuntimeManager: &strategyRuntimeManager{}}}
	server.handlePushMarketdataTick(mdsrv.Tick{
		Kind: mdsrv.TickKindTrade, InstrumentID: "US.AAPL", VolumeDelta: math.NaN(),
	})
}
