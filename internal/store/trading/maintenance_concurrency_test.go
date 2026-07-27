package trading

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestExecutionMaintenanceReportsBusyAndCompactsAvailableDatabase(t *testing.T) {
	degraded := NewInMemory()
	if err := degraded.CompactMaintenanceResource(t.Context()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("degraded compact error = %v", err)
	}
	degraded.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		InternalOrderID: "active-order",
		Status:          trdsrv.OrderStatusSubmitted,
	})
	if reason := degraded.MaintenanceBusyReason(t.Context()); !strings.Contains(reason, "非终态") {
		t.Fatalf("active order busy reason = %q", reason)
	}

	store, err := New(filepath.Join(t.TempDir(), "execution.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		InternalOrderID: "terminal-order",
		Status:          trdsrv.OrderStatusFilled,
	})
	if reason := store.MaintenanceBusyReason(t.Context()); reason != "" {
		t.Fatalf("terminal order busy reason = %q", reason)
	}
	if err := store.CompactMaintenanceResource(t.Context()); err != nil {
		t.Fatalf("CompactMaintenanceResource: %v", err)
	}
}

func TestExecutionStoreConcurrentReadsWritesAndDurableReload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "execution.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const writers = 24
	var wg sync.WaitGroup
	for index := range writers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			store.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
				ClientOrderID: fmt.Sprintf("client-%02d", index),
				BrokerID:      "futu",
				AccountID:     "SIM-1",
				Market:        "US",
				Status:        trdsrv.OrderStatusSubmitted,
			})
		}()
		go func() {
			defer wg.Done()
			_ = store.AllOrders()
		}()
	}
	wg.Wait()
	if got := len(store.AllOrders().Orders); got != writers {
		t.Fatalf("concurrent order count = %d, want %d", got, writers)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := New(dbPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	if got := len(reloaded.AllOrders().Orders); got != writers {
		t.Fatalf("reloaded order count = %d, want %d", got, writers)
	}
}
