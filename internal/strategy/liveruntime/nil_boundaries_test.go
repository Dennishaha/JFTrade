package liveruntime

import (
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestNilRuntimeBoundariesReturnEmptyState(t *testing.T) {
	var manager *Manager
	manager.SetPineWorkerRunner(nil)
	if manager.currentPineWorkerRunner() != nil {
		t.Fatal("nil manager returned a Pine worker")
	}
	var runtime *symbolRuntime
	if runtime.brokerPositionsSnapshot() != nil || runtime.brokerAccountSnapshot() != nil {
		t.Fatal("nil strategy runtime returned account state")
	}
	if _, ok := manager.currentInstance("missing"); ok {
		t.Fatal("nil manager returned an instance")
	}
	if err := manager.appendRuntimeEvent("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.transitionInstance("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.reconcileRuntimeFailure("", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("nil manager placement error = nil")
	}
	if _, err := manager.cancelExecutionOrder(t.Context(), ""); err == nil {
		t.Fatal("nil manager cancellation error = nil")
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("nil manager close error = %v", err)
	}
}
