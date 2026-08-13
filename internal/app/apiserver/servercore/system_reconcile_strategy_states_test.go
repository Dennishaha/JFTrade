package servercore

import (
	"path/filepath"
	"strings"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestNewServerReconcilesPersistedActiveStrategyStates(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	initialStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore initial: %v", err)
	}
	initialServer := newTestServer(t, initialStore)
	instanceID := createCatalogInstanceForTest(t, initialServer, stratsrv.ManagedInstance{
		Definition: stratsrv.DefinitionSummary{
			StrategyID: "demo-plugin",
			Name:       "Demo Plugin",
			Version:    "1.0.0",
		},
		Status: strategyStatusRunning,
	})

	reloadedStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore reload: %v", err)
	}
	reloadedServer := newTestServer(t, reloadedStore)

	strategy, ok := reloadedServer.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatal("expected reconciled strategy to exist")
	}
	if strategy.Status != strategyStatusStopped {
		t.Fatalf("reconciled status = %s, want %s", strategy.Status, strategyStatusStopped)
	}
	logs, ok := reloadedServer.stores.StrategyCatalog.GetLogs(instanceID, stratsrv.LogQuery{})
	if !ok || len(logs.Logs) == 0 || !strings.Contains(logs.Logs[0], "reconciled strategy state") {
		t.Fatalf("expected reconciliation log, got %+v", logs)
	}
	audit, ok := reloadedServer.stores.StrategyCatalog.GetAudit(instanceID, stratsrv.AuditQuery{})
	if !ok {
		t.Fatal("expected reconciled strategy audit to exist")
	}
	found := false
	for _, entry := range audit.Entries {
		if entry.Kind == "reconciled" && strings.Contains(entry.Detail, "stale running state") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reconciliation audit entry, got %+v", audit.Entries)
	}

	strategyRuntime := reloadedServer.sysSvc.Status().StrategyRuntime
	if strategyRuntime == nil {
		t.Fatalf("expected strategyRuntime summary, got %+v", reloadedServer.sysSvc.Status().StrategyRuntime)
	}
	if got := strategyRuntime.ActiveStrategies; got != 0 {
		t.Fatalf("activeStrategies after restart = %d, want 0", got)
	}
	if got := strategyRuntime.Status; got != "idle" {
		t.Fatalf("runtime status after restart = %v, want idle", got)
	}
}
