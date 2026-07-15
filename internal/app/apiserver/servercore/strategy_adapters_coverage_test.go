package servercore

import (
	"errors"
	"path/filepath"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func strategyAdapterCoverageDefinition(id string) stratsrv.Definition {
	return stratsrv.Definition{
		ID:           id,
		Name:         "Coverage",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Coverage\")",
	}
}

func TestStrategyDesignAdapterSaveErrorMappings(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "design.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	adapter := &strategyDesignStoreAdapter{store: store}
	legacy := strategyAdapterCoverageDefinition("legacy")
	legacy.Runtime = "legacy-go"
	if _, err := adapter.SaveDefinition(legacy); !errors.Is(err, stratsrv.ErrBadRequest) {
		t.Fatalf("legacy save error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close design store: %v", err)
	}
	if _, err := adapter.SaveDefinition(strategyAdapterCoverageDefinition("closed")); err == nil || errors.Is(err, stratsrv.ErrBadRequest) {
		t.Fatalf("closed save error = %v", err)
	}
}

func TestStrategyCatalogAdapterMutationErrorMappings(t *testing.T) {
	store := newCatalogCoverageStore(t)
	adapter := &strategyCatalogStoreAdapter{store: store}
	missingBinding := stratsrv.InstanceBinding{Symbols: []string{"US.AAPL"}, Interval: "1m", ExecutionMode: strategyExecutionModeNotifyOnly}

	if _, err := adapter.UpdateInstanceRuntimeRisk("missing", stratsrv.RuntimeRiskSettings{}); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("risk missing error = %v", err)
	}
	if _, err := adapter.TransitionInstance("missing", strategyStatusRunning); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("transition missing error = %v", err)
	}
	if _, err := adapter.RefreshDefinition("missing", strategyAdapterCoverageDefinition("def")); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("refresh missing error = %v", err)
	}

	if err := store.runtimeStore.Close(); err != nil {
		t.Fatalf("close catalog runtime store: %v", err)
	}
	if _, err := adapter.CreateInstance(strategyAdapterCoverageDefinition("create"), missingBinding); err == nil {
		t.Fatal("expected closed create persistence error")
	}

	arbitrary := errors.New("arbitrary strategy store error")
	if got := mapStrategyStoreError(arbitrary); !errors.Is(got, arbitrary) {
		t.Fatalf("default mapped error = %v", got)
	}
}

func TestStrategyCatalogAdapterRefreshInstanceDefinitionBoundaries(t *testing.T) {
	designStore, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "design.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	defer func() { _ = designStore.Close() }()

	store := newCatalogCoverageStore(t)
	adapter := &strategyCatalogStoreAdapter{store: store, designStore: designStore}
	store.data.Strategies = []managedStrategyInstance{{
		ID:         "missing-definition",
		Status:     strategyStatusStopped,
		Definition: strategyDefinitionSummary{StrategyID: "missing"},
	}}
	if _, err := adapter.RefreshInstanceDefinition("missing-definition"); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("missing definition refresh error = %v", err)
	}

	definition, err := designStore.saveDefinition(strategyAdapterCoverageDefinition("linked"))
	if err != nil {
		t.Fatalf("save linked definition: %v", err)
	}
	store.data.Strategies = []managedStrategyInstance{{
		ID:         "busy",
		Status:     strategyStatusRunning,
		Definition: strategyDefinitionSummary{StrategyID: definition.ID, Version: "0.0.1"},
		Params:     map[string]any{"definitionId": definition.ID},
	}}
	if _, err := adapter.RefreshInstanceDefinition("busy"); !errors.Is(err, stratsrv.ErrBusy) {
		t.Fatalf("busy refresh error = %v", err)
	}

	if err := designStore.Close(); err != nil {
		t.Fatalf("close design store: %v", err)
	}
	store.data.Strategies = []managedStrategyInstance{{
		ID:         "definition-error",
		Status:     strategyStatusStopped,
		Definition: strategyDefinitionSummary{StrategyID: "linked"},
	}}
	if _, err := adapter.RefreshInstanceDefinition("definition-error"); err == nil || errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("definition query error = %v", err)
	}
}

func TestStrategyCatalogAdapterEnrichmentPersistenceFailures(t *testing.T) {
	designStore, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "design.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	adapter := &strategyCatalogStoreAdapter{designStore: designStore}
	item := strategyListItem{ID: "item", Status: strategyStatusStopped, Definition: strategyDefinitionSummary{StrategyID: "missing", Version: "1.0.0"}}
	status := adapter.buildDefinitionSyncStatus(item)
	if status == nil || !status.IsLatest {
		t.Fatalf("missing definition sync = %#v", status)
	}
	if err := designStore.Close(); err != nil {
		t.Fatalf("close design store: %v", err)
	}
	if status := adapter.buildDefinitionSyncStatus(item); status == nil {
		t.Fatal("definition lookup error should preserve baseline sync status")
	}

	store := newCatalogCoverageStore(t)
	if err := store.runtimeStore.Close(); err != nil {
		t.Fatalf("close runtime store: %v", err)
	}
	adapter = &strategyCatalogStoreAdapter{store: store}
	enriched := adapter.enrichItem(strategyListItem{ID: "persisted-error"})
	if enriched.ID != "persisted-error" {
		t.Fatalf("enriched item = %#v", enriched)
	}
}
