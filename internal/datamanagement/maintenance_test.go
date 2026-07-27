package datamanagement

import (
	"context"
	"errors"
	"testing"
)

func TestMaintenanceRegistryDispatchesOnlyDeclaredCapabilities(t *testing.T) {
	wantErr := errors.New("compact failed")
	registry := NewMaintenanceRegistry(map[string]Target{
		"strategy": {
			Busy: BusyCheckerFunc(func(context.Context) string { return " active strategy " }),
			Purger: CandidatePurgerFunc(func(_ context.Context, candidates []CleanupCandidate) (int, error) {
				return len(candidates), nil
			}),
			Compactor: CompactorFunc(func(context.Context) error { return wantErr }),
		},
	})

	if got := registry.BusyReason(t.Context(), "strategy"); got != "active strategy" {
		t.Fatalf("busy reason = %q", got)
	}
	deleted, err := registry.Purge(t.Context(), "strategy", []CleanupCandidate{{ID: "one"}, {ID: "two"}})
	if err != nil || deleted != 2 {
		t.Fatalf("purge = %d, %v", deleted, err)
	}
	if err := registry.Compact(t.Context(), "strategy"); !errors.Is(err, wantErr) {
		t.Fatalf("compact error = %v, want %v", err, wantErr)
	}
}

func TestMaintenanceRegistryFailsClosedForMissingCapabilities(t *testing.T) {
	registry := NewMaintenanceRegistry(map[string]Target{"watchlist": {}})
	if got := registry.BusyReason(t.Context(), "missing"); got != "" {
		t.Fatalf("missing busy reason = %q", got)
	}
	if _, err := registry.Purge(t.Context(), "watchlist", nil); err == nil {
		t.Fatal("purge without capability succeeded")
	}
	if err := registry.Compact(t.Context(), "watchlist"); err == nil {
		t.Fatal("compact without capability succeeded")
	}
}

func TestBusyCheckersReturnTheFirstOwnedActivityReason(t *testing.T) {
	checkers := BusyCheckers{
		nil,
		BusyCheckerFunc(func(context.Context) string { return " " }),
		BusyCheckerFunc(func(context.Context) string { return " active backtest " }),
		BusyCheckerFunc(func(context.Context) string { return "active sync" }),
	}

	if reason := checkers.MaintenanceBusyReason(t.Context()); reason != "active backtest" {
		t.Fatalf("busy reason = %q, want first owned reason", reason)
	}
	if reason := (BusyCheckers{}).MaintenanceBusyReason(t.Context()); reason != "" {
		t.Fatalf("empty busy reason = %q", reason)
	}
}
