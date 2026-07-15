package servercore

import (
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestAssistantWorkflowToolManagerAvailableAndUnavailablePaths(t *testing.T) {
	ctx := t.Context()
	unavailable := assistantWorkflowToolManager{}
	if _, err := unavailable.service(); err == nil {
		t.Fatal("unavailable workflow service error = nil")
	}
	assertUnavailable := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	_, err := unavailable.ListWorkflows(ctx, "", 10, 0)
	assertUnavailable("ListWorkflows", err)
	_, err = unavailable.GetWorkflow(ctx, "workflow")
	assertUnavailable("GetWorkflow", err)
	_, err = unavailable.SaveWorkflow(ctx, "workflow", jfadk.WorkflowDefinitionWriteRequest{})
	assertUnavailable("SaveWorkflow", err)
	_, err = unavailable.DeleteWorkflow(ctx, "workflow")
	assertUnavailable("DeleteWorkflow", err)
	_, err = unavailable.ListWorkflowTriggers(ctx, "workflow")
	assertUnavailable("ListWorkflowTriggers", err)
	_, err = unavailable.GetWorkflowTrigger(ctx, "workflow", "trigger")
	assertUnavailable("GetWorkflowTrigger", err)
	_, err = unavailable.SaveWorkflowTrigger(ctx, "workflow", "trigger", jfadk.WorkflowTriggerWriteRequest{})
	assertUnavailable("SaveWorkflowTrigger", err)
	_, err = unavailable.DeleteWorkflowTrigger(ctx, "workflow", "trigger")
	assertUnavailable("DeleteWorkflowTrigger", err)
	_, err = unavailable.ListWorkflowRuns(ctx, "workflow", "trigger", "", 10, 0)
	assertUnavailable("ListWorkflowRuns", err)
	_, err = unavailable.GetWorkflowRun(ctx, "log")
	assertUnavailable("GetWorkflowRun", err)
	_, err = unavailable.StartWorkflow(ctx, "workflow", nil)
	assertUnavailable("StartWorkflow", err)
	_, err = unavailable.StartWorkflowTrigger(ctx, "trigger", nil)
	assertUnavailable("StartWorkflowTrigger", err)

	settings, err := NewSettingsStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	agent, err := server.adkRuntime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "manager-agent", Name: "Manager Agent", ProviderID: testADKProviderID,
		Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeChat,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := assistantWorkflowToolManager{server: server}
	if service, err := manager.service(); err != nil || service == nil {
		t.Fatalf("service = %v, %v", service, err)
	}
	payload := jfadk.WorkflowDefinitionWriteRequest{
		ID: "manager-workflow", Name: "Manager Workflow", Status: jfadk.WorkflowStatusEnabled,
		AgentID: agent.ID, WorkMode: jfadk.WorkModeChat, PromptTemplate: "Review {{ .symbol }}",
		DefaultInputs: map[string]any{"symbol": "US.AAPL"},
		CanvasGraph: &jfadk.WorkflowCanvasGraph{
			Version: "adk-workflow-canvas/v1",
			Nodes: []jfadk.WorkflowCanvasNode{
				{ID: "start", Type: "start"},
				{ID: "agent:primary", Type: "agent"},
				{ID: "monitor", Type: "monitor"},
			},
			Edges: []jfadk.WorkflowCanvasEdge{
				{ID: "start-agent", Source: "start", Target: "agent:primary"},
				{ID: "agent-monitor", Source: "agent:primary", Target: "monitor"},
			},
		},
	}
	workflow, err := manager.SaveWorkflow(ctx, "", payload)
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	if page, err := manager.ListWorkflows(ctx, jfadk.WorkflowStatusEnabled, 10, 0); err != nil || page.Total == 0 || len(page.Items) == 0 {
		t.Fatalf("ListWorkflows = %+v, %v", page, err)
	}
	if got, err := manager.GetWorkflow(ctx, workflow.ID); err != nil || got.ID != workflow.ID {
		t.Fatalf("GetWorkflow = %+v, %v", got, err)
	}

	trigger, err := manager.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID: "manager-trigger", Type: jfadk.WorkflowTriggerTypeWebhook, Title: "Manager Trigger", Status: jfadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}
	if triggers, err := manager.ListWorkflowTriggers(ctx, workflow.ID); err != nil || len(triggers) == 0 {
		t.Fatalf("ListWorkflowTriggers = %+v, %v", triggers, err)
	}
	if got, err := manager.GetWorkflowTrigger(ctx, workflow.ID, trigger.ID); err != nil || got.ID != trigger.ID {
		t.Fatalf("GetWorkflowTrigger = %+v, %v", got, err)
	}

	started, err := manager.StartWorkflow(ctx, workflow.ID, map[string]any{"symbol": "US.MSFT"})
	if err != nil || !started.Accepted || started.Log.ID == "" {
		t.Fatalf("StartWorkflow = %+v, %v", started, err)
	}
	triggered, err := manager.StartWorkflowTrigger(ctx, trigger.ID, map[string]any{"symbol": "US.NVDA"})
	if err != nil || !triggered.Accepted || triggered.Log.ID == "" {
		t.Fatalf("StartWorkflowTrigger = %+v, %v", triggered, err)
	}
	if page, err := manager.ListWorkflowRuns(ctx, workflow.ID, "", "", 10, 0); err != nil || page.Total < 2 || len(page.Items) < 2 {
		t.Fatalf("ListWorkflowRuns = %+v, %v", page, err)
	}
	if got, err := manager.GetWorkflowRun(ctx, started.Log.ID); err != nil || got.ID != started.Log.ID {
		t.Fatalf("GetWorkflowRun = %+v, %v", got, err)
	}
	if deleted, err := manager.DeleteWorkflowTrigger(ctx, workflow.ID, trigger.ID); err != nil || deleted.ID != trigger.ID {
		t.Fatalf("DeleteWorkflowTrigger = %+v, %v", deleted, err)
	}
	if deleted, err := manager.DeleteWorkflow(ctx, workflow.ID); err != nil || deleted.ID != workflow.ID {
		t.Fatalf("DeleteWorkflow = %+v, %v", deleted, err)
	}
}

func TestADKAdapterRemainingInputBoundaries(t *testing.T) {
	var nilServer *Server
	if _, err := nilServer.adkWatchlistList(t.Context(), WatchlistListInput{}); err == nil {
		t.Fatal("nil watchlist error = nil")
	}
	nilServer.populateADKBrokerToolDeps(nil)
	nilServer.populateADKStrategyToolDeps(nil)
	nilServer.populateADKBacktestToolDeps(nil)
	if _, ok := resolveADKWatchlistGroup(nil, "missing"); ok {
		t.Fatal("missing watchlist group resolved")
	}

	settings, err := NewSettingsStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	if _, err := server.adkBrokerOrders(t.Context(), BrokerReadInput{Scope: "invalid"}); err == nil {
		t.Fatal("invalid orders scope error = nil")
	}
	if _, err := server.adkBrokerFills(t.Context(), BrokerReadInput{Scope: "invalid"}); err == nil {
		t.Fatal("invalid fills scope error = nil")
	}
	if _, err := server.adkBrokerCashFlows(t.Context(), BrokerReadInput{}); err == nil {
		t.Fatal("missing clearingDate error = nil")
	}
	if _, err := server.adkBrokerFees(t.Context(), BrokerReadInput{}); err == nil {
		t.Fatal("missing orderIdEx error = nil")
	}
	if _, err := server.adkBrokerMarginRatios(t.Context(), BrokerReadInput{Market: "invalid", Symbols: []string{"AAPL"}}); err == nil {
		t.Fatal("invalid margin symbol error = nil")
	}
	if _, err := server.adkBrokerMarginRatios(t.Context(), BrokerReadInput{Market: "US"}); err == nil {
		t.Fatal("missing margin symbol error = nil")
	}
	if _, err := server.adkUpdateStrategyInstanceMode("missing", "paper"); err == nil {
		t.Fatal("missing strategy instance mode error = nil")
	}
	if _, err := nilServer.adkSaveStrategyDraft(StrategyDraftInput{}); err == nil {
		t.Fatal("nil strategy draft error = nil")
	}
	if _, err := nilServer.adkSaveStrategyDefinition(StrategyDefinitionInput{}); err == nil {
		t.Fatal("nil strategy definition error = nil")
	}
	if _, err := server.adkSaveStrategyDefinition(StrategyDefinitionInput{DefinitionID: "missing"}); err == nil {
		t.Fatal("missing strategy definition error = nil")
	}
	if _, err := strategyVisualModelFromInput(func() {}); err == nil {
		t.Fatal("unmarshalable visual model error = nil")
	}
	if got := mergeADKBrokerValues([]string{"", "AAPL", "aapl"}); len(got) != 1 || got[0] != "AAPL" {
		t.Fatalf("merged broker values = %#v", got)
	}

	var emptyOptimization assistantOptimizationRuns
	if _, ok := emptyOptimization.Get("missing"); ok {
		t.Fatal("nil optimization run found")
	}
	emptyOptimization.Cancel("missing")
	(&assistantOptimizationRuns{server: server}).Cancel("missing")
}

func TestADKToolDepsAuditAndBasicClosures(t *testing.T) {
	settings, err := NewSettingsStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	deps := server.adkToolDeps()
	deps.RecordAudit(t.Context(), "coverage", "subject", "detail", map[string]any{"ok": true})
	if !deps.ADKEnabled() || deps.SystemStatus() == nil || deps.PluginCatalog() == nil || deps.ManagedAccounts() == nil || deps.DefaultTradeMarket() == "" {
		t.Fatal("basic ADK dependencies returned incomplete values")
	}
	_, _ = deps.FutuOpenDHealth(t.Context())
	_, _, _ = deps.MarketSubscriptions(t.Context())
	_, _ = deps.MarketSnapshot(t.Context(), "US", "AAPL")
	_, _ = deps.MarketCandles(t.Context(), "US", "AAPL", "1d", 1)
	_ = deps.BrokerEnabled()
	_ = deps.BrokerFunds(t.Context(), broker.ReadQuery{}, 0)
	_ = deps.BrokerPositions(t.Context(), broker.ReadQuery{}, 0)
	_, _ = deps.MarketDepth(t.Context(), "US", "AAPL", 1)
	_ = deps.RiskEvents()
	deps.CancelBacktest("missing")
}
