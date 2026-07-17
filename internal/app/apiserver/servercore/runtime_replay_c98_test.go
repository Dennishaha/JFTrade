package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/shopspring/decimal"
)

func TestCoverage98StrategyDesignNormalizationKeepsRunnableDefaultsAndRejectsUnpersistableModels(t *testing.T) {
	definition, err := normalizeStrategyDesignDefinition(strategyDesignDefinition{
		ID:      " strategy-defaults ",
		Name:    " ",
		Runtime: strategyRuntimePinePlan,
		VisualModel: &strategyVisualModel{
			Nodes: nil,
			Edges: nil,
		},
	})
	if err != nil {
		t.Fatalf("normalize strategy design: %v", err)
	}
	if definition.Name != "strategy-defaults" || definition.Script == "" {
		t.Fatalf("defaulted strategy design = %#v", definition)
	}
	if definition.VisualModel == nil || definition.VisualModel.Engine != "logic-flow" || definition.VisualModel.Version != 1 || definition.VisualModel.Nodes == nil || definition.VisualModel.Edges == nil {
		t.Fatalf("normalized visual model = %#v", definition.VisualModel)
	}
	if script := defaultStrategyDesignPine(" "); !strings.Contains(script, `strategy("Pine Strategy"`) {
		t.Fatalf("blank default Pine strategy name = %q", script)
	}
	if got := nextStrategyDefinitionVersion("draft"); got != defaultStrategyVersion {
		t.Fatalf("draft version increment = %q, want %q", got, defaultStrategyVersion)
	}
	if got := syncStrategyScriptVersion(" ", "1.0.0"); got != " " {
		t.Fatalf("blank script version sync changed its input to %q", got)
	}

	unpersistable := strategyDesignDefinition{
		ID: "invalid-json",
		VisualModel: &strategyVisualModel{Nodes: []strategyVisualNode{{
			Properties: map[string]any{"runtimeOnly": make(chan struct{})},
		}}},
	}
	if strategyDesignDefinitionsEqual(unpersistable, strategyDesignDefinition{ID: "invalid-json"}) {
		t.Fatal("a non-serializable visual model must not compare equal for persistence")
	}
}

func TestCoverage98RuntimeRiskRejectionRecordsBothAuditAndPauseTransition(t *testing.T) {
	var nilExecutor *strategyLiveOrderExecutor
	if got := nilExecutor.currentRuntimeRiskSettings(); got != normalizeStrategyRuntimeRiskSettings(strategyRuntimeRiskSettings{}) {
		t.Fatalf("nil executor risk settings = %#v", got)
	}
	if got := ((*strategySymbolRuntime)(nil)).sellableQuantity("US.AAPL"); got != 0 {
		t.Fatalf("nil runner sellable quantity = %v", got)
	}

	var eventKinds []string
	var transitioned []string
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
		deps: strategyRuntimeManagerDeps{
			currentInstance: func(string) (managedStrategyInstance, bool) {
				return managedStrategyInstance{Binding: strategyInstanceBinding{RuntimeRisk: strategyRuntimeRiskSettings{Mode: "monitor"}}}, true
			},
			appendRuntimeEvent: func(_ string, _ string, kind string, _ string) error {
				eventKinds = append(eventKinds, kind)
				return nil
			},
			transitionInstance: func(_ string, status string, kind string, _ string) error {
				transitioned = append(transitioned, status+":"+kind)
				return nil
			},
			countRuntimeAudit: func(context.Context, runtimeactivity.AuditQuery) (int, error) {
				return 0, errors.New("audit store unavailable")
			},
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager: manager,
		instance: managedStrategyInstance{
			ID:      "runtime-risk-instance",
			Binding: strategyInstanceBinding{RuntimeRisk: strategyRuntimeRiskSettings{Mode: "enforce"}},
		},
	}
	if got := executor.currentRuntimeRiskSettings().Mode; got != "monitor" {
		t.Fatalf("persisted runtime risk settings were not preferred: %q", got)
	}
	if got := manager.todaySubmittedOrderCount("runtime-risk-instance", "US.AAPL", time.Now().UTC()); got != 0 {
		t.Fatalf("audit-store failure count = %d", got)
	}

	command := trdsrv.ExecutionOrderCommand{
		Symbol: "US.AAPL",
		Side:   "BUY",
		Query:  broker.PlaceOrderQuery{Symbol: "US.AAPL", Side: "BUY", Quantity: 2},
	}
	executor.recordRuntimeRiskDecision(strategyRuntimeRiskDecision{
		Matched: true, Rejected: true, PauseOnReject: true, Reason: "close_only", Detail: "rule=close_only",
	}, command)
	executor.recordRuntimeRiskDecision(strategyRuntimeRiskDecision{
		Matched: true, Reason: "daily_max_orders", Detail: "rule=daily_max_orders",
	}, command)
	if strings.Join(eventKinds, ",") != "risk_rejected,risk_monitor" {
		t.Fatalf("runtime risk event kinds = %#v", eventKinds)
	}
	if strings.Join(transitioned, ",") != "PAUSED:paused" {
		t.Fatalf("runtime risk pause transition = %#v", transitioned)
	}
}

func TestCoverage98WorkflowSnapshotAndLiveTradeReplayReachBusinessConsumers(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	now := time.Now().UTC().Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       12,
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
		Source:       "coverage-replay",
	})
	snapshot, err := server.workflowMarketSnapshot(t.Context(), "hk.00700")
	if err != nil {
		t.Fatalf("workflow cached market snapshot: %v", err)
	}
	if got := jftradeCheckedTypeAssertion[map[string]any](snapshot["snapshot"])["price"]; got != "321.4" {
		t.Fatalf("workflow snapshot price = %v", got)
	}

	// The manager has no matching runtime here, but a valid trade must still be
	// converted to the canonical strategy-runtime event without panicking.
	server.assistantSvc = nil
	server.strategyRuntimeManager = &strategyRuntimeManager{runtimes: map[string]*managedStrategyRuntime{}}
	server.handlePushMarketdataTick(marketTickSample{
		Kind:         "trade",
		InstrumentID: "HK.00700",
		Price:        decimal.RequireFromString("321.4"),
		Volume:       6,
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
	})
}

func TestCoverage98WorkflowSnapshotReturnsProviderFailureAfterStaleCache(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stale := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		QuoteAt:      stale.Format(time.RFC3339Nano),
		ObservedAt:   stale.Format(time.RFC3339Nano),
		Source:       "stale-cache",
	})
	if _, err := server.workflowMarketSnapshot(t.Context(), "HK.00700"); err == nil {
		t.Fatal("a stale workflow snapshot must surface the provider failure")
	}
}
