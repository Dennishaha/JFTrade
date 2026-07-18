package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestExecutionNotificationRemainingBoundaries(t *testing.T) {
	server := &Server{}
	server.notifyExecutionOrderLifecycle(executionOrderSummaryResponse{}, nil)
	server.notifyExecutionOrderLifecycle(executionOrderSummaryResponse{Status: "UNKNOWN"}, &executionOrderEventResponse{})

	if _, ok := executionNotificationForStatus(executionOrderSummaryResponse{Status: trdsrv.OrderStatusSubmitted}, &executionOrderEventResponse{EventType: "OTHER"}); ok {
		t.Fatal("unrelated submitted event produced notification")
	}
	partial, ok := executionNotificationForStatus(executionOrderSummaryResponse{Status: trdsrv.OrderStatusPartiallyFilled}, &executionOrderEventResponse{})
	if !ok || partial.Level != "info" || partial.Category != "broker.order.fill" {
		t.Fatalf("partial fill notification = %#v, %v", partial, ok)
	}
	if note := baseExecutionNotification(executionOrderSummaryResponse{}, "category"); note.BrokerID != "unknown" {
		t.Fatalf("default notification broker = %#v", note)
	}
	if got := executionOrderNotificationMessage(executionOrderSummaryResponse{InternalOrderID: "exec-empty"}); got != "exec-empty" {
		t.Fatalf("empty order message = %q", got)
	}
	symbol, side, brokerID := "AAPL", "BUY", "broker-1"
	quantity, filled := 2.0, 1.0
	message := executionOrderNotificationMessage(executionOrderSummaryResponse{
		TradingEnvironment: "SIMULATE",
		Symbol:             &symbol,
		Side:               &side,
		RequestedQuantity:  &quantity,
		FilledQuantity:     &filled,
		BrokerOrderID:      &brokerID,
	})
	for _, part := range []string{"SIMULATE", "AAPL", "BUY", "qty", "filled", "brokerOrderId"} {
		if !strings.Contains(message, part) {
			t.Fatalf("full order message %q missing %q", message, part)
		}
	}
}

func TestCatalogActivityRemainingDegradedBoundaries(t *testing.T) {
	store := newCatalogCoverageStore(t)
	store.data.Strategies = []managedStrategyInstance{{ID: "activity"}}
	runtimeStore := store.runtimeStore
	store.runtimeStore = nil
	if result, ok := store.strategyLogsPage("activity", strategyRuntimeLogQuery{Limit: -1, Offset: -1}); !ok || len(result.Logs) != 0 {
		t.Fatalf("nil runtime log page = %#v, %v", result, ok)
	}
	if result, ok := store.strategyAuditPage("activity", strategyRuntimeAuditQuery{Limit: -1, Offset: -1}); !ok || len(result.Entries) != 0 {
		t.Fatalf("nil runtime audit page = %#v, %v", result, ok)
	}
	store.runtimeStore = runtimeStore
	if err := runtimeStore.Close(); err != nil {
		t.Fatalf("close runtime store: %v", err)
	}
	if result, ok := store.strategyLogsPage("activity", strategyRuntimeLogQuery{}); !ok || result.Page.Total != 0 {
		t.Fatalf("degraded log page = %#v, %v", result, ok)
	}
	if result, ok := store.strategyAuditPage("activity", strategyRuntimeAuditQuery{}); !ok || result.Page.Total != 0 {
		t.Fatalf("degraded audit page = %#v, %v", result, ok)
	}
}

func TestDesignStoreRemainingCorruptAndClosedBoundaries(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "design.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO strategy_design_definitions (id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at) VALUES ('corrupt', 'Corrupt', '0.1.0', '', 'pinets', 'pine-v6', 'US.AAPL', '1m', '//@version=6
strategy("Corrupt")', '{', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', NULL)`); err != nil {
		t.Fatalf("insert corrupt definition: %v", err)
	}
	if got := store.listDefinitions(); len(got) != 0 {
		t.Fatalf("corrupt definitions list = %#v", got)
	}
	if _, ok, err := store.definition("corrupt"); err == nil || ok {
		t.Fatalf("corrupt definition = ok %v, err %v", ok, err)
	}
	if _, err := store.saveDefinition(strategyAdapterCoverageDefinition("corrupt")); err == nil {
		t.Fatal("expected corrupt existing save error")
	}
	if _, err := store.deleteDefinition("corrupt"); err == nil {
		t.Fatal("expected corrupt delete error")
	}

	bad := strategyAdapterCoverageDefinition("marshal-error")
	bad.VisualModel = &strategyVisualModel{Nodes: []strategyVisualNode{{Properties: map[string]any{"bad": make(chan int)}}}}
	if err := store.upsertDefinitionLocked(bad, nil); err == nil {
		t.Fatal("expected visual model marshal error")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close design store: %v", err)
	}
	if got := store.listDefinitions(); len(got) != 0 {
		t.Fatalf("closed definitions list = %#v", got)
	}
	if _, _, err := store.definition("closed"); err == nil {
		t.Fatal("expected closed definition error")
	}
	if _, err := store.saveDefinition(strategyAdapterCoverageDefinition("closed")); err == nil {
		t.Fatal("expected closed save error")
	}
	if _, err := store.deleteDefinition("closed"); err == nil {
		t.Fatal("expected closed delete error")
	}
	deletedAt := "now"
	if err := store.upsertDefinitionLocked(strategyAdapterCoverageDefinition("closed-upsert"), &deletedAt); err == nil {
		t.Fatal("expected closed upsert error")
	}
}

func TestRuntimeDependencyRemainingPureBoundaries(t *testing.T) {
	restoreRuntimeDependencyProbe(t,
		func(path string) (string, error) { return path, nil },
		func(context.Context, string, ...string) ([]byte, error) { return []byte("v22.0.0"), nil },
	)
	//nolint:staticcheck // Exercise the helper's explicit nil-context fallback.
	if got := checkNodeRuntimeDependency(nil, PineWorkerSettings{}); got["status"] != runtimeDependencyStatusOK {
		t.Fatalf("nil-context dependency = %#v", got)
	}
	configuredMissing := nodeMissingMessage("/missing/node", []string{"ignored"}, errors.New("missing"))
	if !strings.Contains(configuredMissing, "Configured") {
		t.Fatalf("configured missing message = %q", configuredMissing)
	}
	defaultMissing := nodeMissingMessage("", nil, errors.New("missing"))
	if !strings.Contains(defaultMissing, "Tried: node") {
		t.Fatalf("default missing message = %q", defaultMissing)
	}
	if got := summarizeDependencyCommandError(errors.New("boom"), nil); got != "boom" {
		t.Fatalf("empty command output summary = %q", got)
	}
	longOutput := strings.Repeat("x", 600)
	if got := summarizeDependencyCommandError(errors.New("boom"), []byte(longOutput)); len(got) > 506 {
		t.Fatalf("long command output was not truncated: %d", len(got))
	}
	for _, raw := range []string{"", "1.2.3.4", "1..2"} {
		if _, err := parseDependencyNodeVersion(raw); err == nil {
			t.Fatalf("invalid node version %q accepted", raw)
		}
	}
}
