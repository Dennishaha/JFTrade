package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyAdaptersDelegateRefreshApplyCloseAndErrorMapping(t *testing.T) {
	catalogStore, err := NewStrategyCatalogStore(filepath.Join(t.TempDir(), "strategy-catalog.json"), "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	adapter := &strategyCatalogStoreAdapter{store: catalogStore}
	oldDefinition := stratsrv.Definition{
		ID:           "adapter-refresh-def",
		Name:         "Adapter Refresh",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Adapter Refresh\")\nlog.info(\"old\")",
	}
	instance, err := adapter.CreateInstance(oldDefinition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	newDefinition := oldDefinition
	newDefinition.Name = "Adapter Refresh v2"
	newDefinition.Version = "0.2.0"
	newDefinition.Script = "//@version=6\nstrategy(\"Adapter Refresh\")\nfast = ta.sma(close, 10)\nlog.info(str.tostring(fast))"
	refreshed, err := adapter.RefreshDefinition(instance.ID, newDefinition)
	if err != nil {
		t.Fatalf("RefreshDefinition: %v", err)
	}
	if refreshed.Definition.Version != "0.2.0" || !strings.Contains(refreshed.Params["script"].(string), "ta.sma") {
		t.Fatalf("refreshed instance = %#v", refreshed)
	}

	result, err := adapter.ApplyDefinitionToLinked(newDefinition)
	if err != nil {
		t.Fatalf("ApplyDefinitionToLinked: %v", err)
	}
	if result.DefinitionID != newDefinition.ID || result.TotalLinked != 1 || len(result.AlreadyLatest) != 1 {
		t.Fatalf("apply linked result = %#v", result)
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for _, tc := range []struct {
		err  error
		want error
	}{
		{os.ErrNotExist, stratsrv.ErrNotFound},
		{errStrategyInstanceBusy, stratsrv.ErrBusy},
		{errUnsupportedLegacyStrategyDefinition, stratsrv.ErrBadRequest},
	} {
		if got := mapStrategyStoreError(tc.err); !errors.Is(got, tc.want) {
			t.Fatalf("mapStrategyStoreError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
	if got := mapStrategyStoreError(nil); got != nil {
		t.Fatalf("mapStrategyStoreError(nil) = %v", got)
	}
}

func TestStrategyRuntimeAdapterExposesObservationSummaryAndNoopStop(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 30, 0, 0, time.UTC)
	manager := &strategyRuntimeManager{runtimes: map[string]*managedStrategyRuntime{
		"instance-adapter": {
			instanceID: "instance-adapter",
			definition: strategyDefinitionSummary{Name: "Adapter Runtime"},
			symbols: map[string]*strategySymbolRuntime{
				"US.AAPL": {symbol: "US.AAPL"},
				"US.MSFT": {symbol: "US.MSFT"},
			},
			lastClosedKLineAt: now,
			updatedAt:         now,
		},
	}}
	adapter := &strategyRuntimeManagerAdapter{mgr: manager}

	observation, ok := adapter.GetObservation("instance-adapter")
	if !ok || observation.ActualStatus != strategyStatusRunning || len(observation.ActiveSymbols) != 2 {
		t.Fatalf("GetObservation = %#v/%v", observation, ok)
	}
	if _, ok := adapter.GetObservation("missing"); ok {
		t.Fatal("missing runtime observation reported ok")
	}
	summary := adapter.RuntimeSummary()
	if summary.Status != "active" || summary.ActiveStrategies != 1 || len(summary.ActiveInstances) != 1 || summary.ActiveInstances[0].DefinitionName != "Adapter Runtime" {
		t.Fatalf("RuntimeSummary = %#v", summary)
	}
	if symbols := adapter.ActiveInstrumentIDs(); strings.Join(symbols, ",") != "US.AAPL,US.MSFT" {
		t.Fatalf("ActiveInstrumentIDs = %#v", symbols)
	}
	adapter.Stop("missing")
	if err := (noOpTradingOrderUpdateSubscription{}).Stop(); err != nil {
		t.Fatalf("no-op order update stop: %v", err)
	}
}
