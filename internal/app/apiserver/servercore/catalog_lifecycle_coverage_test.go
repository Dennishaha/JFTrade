package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func catalogCoverageDefinition(id, version string) strategyDesignDefinition {
	return strategyDesignDefinition{
		ID: id, Name: "Coverage", Version: version,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL", Interval: "1m", Script: "//@version=6\nstrategy(\"Coverage\")",
	}
}

func TestCatalogLifecycleRemainingValidationAndMissingBoundaries(t *testing.T) {
	store := &strategyCatalogStore{data: strategyCatalogFile{Strategies: []managedStrategyInstance{
		{ID: "busy", Status: strategyStatusRunning},
		{ID: "stopped", Status: strategyStatusStopped, Definition: strategyDefinitionSummary{StrategyID: "definition", Version: "1"}},
	}}}
	invalid := catalogCoverageDefinition("invalid", "1")
	invalid.SourceFormat = "visual"
	if _, err := store.instantiateStrategy(invalid, strategyInstanceBinding{}); err == nil {
		t.Fatal("invalid instantiate error = nil")
	}
	if _, err := store.updateStrategyBinding("busy", strategyInstanceBinding{}); !errors.Is(err, errStrategyInstanceBusy) {
		t.Fatalf("busy binding error = %v", err)
	}
	if _, err := store.updateStrategyBinding("missing", strategyInstanceBinding{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing binding error = %v", err)
	}
	if _, err := store.updateStrategyRuntimeRisk("missing", strategyRuntimeRiskSettings{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing risk error = %v", err)
	}
	if _, err := store.refreshStrategyDefinition("missing", catalogCoverageDefinition("definition", "2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing refresh error = %v", err)
	}
	if _, err := store.refreshStrategyDefinition("stopped", invalid); err == nil {
		t.Fatal("invalid refresh definition error = nil")
	}
	if _, err := store.applyDefinitionToLinkedStrategies(invalid); err == nil {
		t.Fatal("invalid apply definition error = nil")
	}
	if _, err := store.deleteStrategy("busy"); !errors.Is(err, errStrategyInstanceBusy) {
		t.Fatalf("busy delete error = %v", err)
	}
	if _, err := store.deleteStrategy("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing delete error = %v", err)
	}
	if _, err := store.transitionStrategy("missing", strategyStatusStopped, "stop", ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing transition error = %v", err)
	}
	if err := store.appendStrategyRuntimeEvent("missing", "", "", ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing runtime event error = %v", err)
	}
	if err := store.reconcileStrategyRuntimeFailure("stopped", ""); err != nil {
		t.Fatalf("stopped reconcile error = %v", err)
	}
	if err := store.reconcileStrategyRuntimeFailure("missing", ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing reconcile error = %v", err)
	}
	unchangedStore := &strategyCatalogStore{data: strategyCatalogFile{Strategies: []managedStrategyInstance{{ID: "stopped", Status: strategyStatusStopped}}}}
	if changed, err := unchangedStore.reconcileRuntimeStatesOnStartup(); err != nil || changed != 0 {
		t.Fatalf("unchanged startup reconcile = %d, %v", changed, err)
	}

	if changed, err := store.refreshStrategyDefinitionLocked(nil, catalogCoverageDefinition("definition", "2"), nil, time.Now()); err != nil || changed {
		t.Fatalf("nil locked refresh = %v, %v", changed, err)
	}
	busy := managedStrategyInstance{Status: strategyStatusRunning}
	if _, err := store.refreshStrategyDefinitionLocked(&busy, catalogCoverageDefinition("definition", "2"), nil, time.Now()); !errors.Is(err, errStrategyInstanceBusy) {
		t.Fatalf("busy locked refresh error = %v", err)
	}
	latest := managedStrategyInstance{Status: strategyStatusStopped, Definition: strategyDefinitionSummary{Version: "2"}}
	if changed, err := store.refreshStrategyDefinitionLocked(&latest, catalogCoverageDefinition("definition", "2"), nil, time.Now()); err != nil || changed {
		t.Fatalf("latest locked refresh = %v, %v", changed, err)
	}
}

func TestCatalogLifecycleRemainingPersistenceFailures(t *testing.T) {
	store, err := NewStrategyCatalogStore(filepath.Join(t.TempDir(), "catalog.json"), filepath.Join(t.TempDir(), "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	definition := catalogCoverageDefinition("definition", "2")
	store.data.Strategies = []managedStrategyInstance{
		{ID: "binding", Status: strategyStatusStopped, Params: map[string]any{}},
		{ID: "risk", Status: strategyStatusStopped, Params: map[string]any{}},
		{ID: "refresh", Status: strategyStatusStopped, Definition: strategyDefinitionSummary{StrategyID: "definition", Version: "1"}, Params: map[string]any{"definitionId": "definition"}},
		{ID: "apply", Status: strategyStatusStopped, Definition: strategyDefinitionSummary{StrategyID: "definition", Version: "1"}, Params: map[string]any{"definitionId": "definition"}},
		{ID: "delete", Status: strategyStatusStopped},
		{ID: "transition", Status: strategyStatusStopped},
		{ID: "event", Status: strategyStatusStopped},
		{ID: "reconcile", Status: strategyStatusRunning},
		{ID: "startup", Status: strategyStatusPaused},
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.instantiateStrategy(catalogCoverageDefinition("new", "1"), strategyInstanceBinding{}); err == nil {
		t.Fatal("closed instantiate error = nil")
	}
	if _, err := store.updateStrategyBinding("binding", strategyInstanceBinding{}); err == nil {
		t.Fatal("closed binding update error = nil")
	}
	if _, err := store.updateStrategyRuntimeRisk("risk", strategyRuntimeRiskSettings{}); err == nil {
		t.Fatal("closed risk update error = nil")
	}
	if _, err := store.refreshStrategyDefinition("refresh", definition); err == nil {
		t.Fatal("closed definition refresh error = nil")
	}
	if _, err := store.applyDefinitionToLinkedStrategies(definition); err == nil {
		t.Fatal("closed linked apply error = nil")
	}
	if _, err := store.deleteStrategy("delete"); err == nil {
		t.Fatal("closed delete error = nil")
	}
	if _, err := store.transitionStrategy("transition", strategyStatusRunning, "start", ""); err == nil {
		t.Fatal("closed transition error = nil")
	}
	if err := store.appendStrategyRuntimeEvent("event", "runtime event", "runtime", "detail"); err != nil {
		t.Fatalf("runtime event degradation should remain non-fatal: %v", err)
	}
	if err := store.reconcileStrategyRuntimeFailure("reconcile", "failure"); err == nil {
		t.Fatal("closed failure reconcile error = nil")
	}
	if _, err := store.reconcileRuntimeStatesOnStartup(); err == nil {
		t.Fatal("closed startup reconcile error = nil")
	}
}
