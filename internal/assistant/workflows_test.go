package assistant

import (
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestNextWorkflowScheduleRunUsesFiveFieldCronAndTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	config := map[string]any{"cron": "0 8 * * 1-5", "timezone": "Asia/Shanghai"}

	next, err := nextWorkflowScheduleRun(config, time.Date(2026, 7, 1, 7, 59, 0, 0, location).UTC())
	if err != nil {
		t.Fatalf("nextWorkflowScheduleRun weekday: %v", err)
	}
	if want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next weekday run = %s, want %s", next, want)
	}

	next, err = nextWorkflowScheduleRun(config, time.Date(2026, 7, 3, 8, 1, 0, 0, location).UTC())
	if err != nil {
		t.Fatalf("nextWorkflowScheduleRun weekend rollover: %v", err)
	}
	if want := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next rollover run = %s, want %s", next, want)
	}

	if _, err := nextWorkflowScheduleRun(map[string]any{"cron": "0 0 8 * * 1-5"}, time.Now()); err == nil {
		t.Fatal("nextWorkflowScheduleRun accepted six-field cron, want error")
	}
}

func TestEvaluateMarketThresholdTriggerEdgesAndCooldown(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	trigger := jfadk.WorkflowTrigger{
		Config: map[string]any{
			"instrumentIds": []string{"US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"value":         100,
			"edge":          "cross_up",
			"cooldownSec":   900,
		},
	}

	matches, changed := evaluateMarketThresholdTrigger(trigger, []map[string]any{marketThresholdEvent("US.AAPL", 99)}, now)
	if len(matches) != 0 || !changed {
		t.Fatalf("first below-threshold event matches=%+v changed=%v, want no match with state change", matches, changed)
	}
	matches, changed = evaluateMarketThresholdTrigger(trigger, []map[string]any{marketThresholdEvent("US.AAPL", 101)}, now.Add(time.Second))
	if len(matches) != 1 || !changed {
		t.Fatalf("cross-up event matches=%+v changed=%v, want one match", matches, changed)
	}
	threshold, ok := matches[0]["threshold"].(map[string]any)
	if !ok || threshold["instrumentId"] != "US.AAPL" || threshold["edge"] != "cross_up" {
		t.Fatalf("matched threshold payload = %+v", matches[0]["threshold"])
	}

	above := jfadk.WorkflowTrigger{
		Config: map[string]any{
			"instrumentIds": []string{"US.AAPL"},
			"snapshotPath":  "snapshot.price",
			"operator":      ">",
			"value":         100,
			"edge":          "above",
			"cooldownSec":   900,
		},
	}
	matches, _ = evaluateMarketThresholdTrigger(above, []map[string]any{marketThresholdEvent("US.AAPL", 101)}, now)
	if len(matches) != 1 {
		t.Fatalf("above threshold first match = %+v, want one", matches)
	}
	matches, _ = evaluateMarketThresholdTrigger(above, []map[string]any{marketThresholdEvent("US.AAPL", 102)}, now.Add(time.Second))
	if len(matches) != 0 {
		t.Fatalf("above threshold cooldown match = %+v, want none", matches)
	}
	matches, _ = evaluateMarketThresholdTrigger(above, []map[string]any{marketThresholdEvent("US.AAPL", 103)}, now.Add(901*time.Second))
	if len(matches) != 1 {
		t.Fatalf("above threshold after cooldown match = %+v, want one", matches)
	}
}

func TestWorkflowWebhookTriggerSecretLifecycle(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-webhook-agent", Name: "Workflow Webhook Agent", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID:             "workflow-webhook-secret",
		Name:           "Webhook Secret",
		Status:         jfadk.WorkflowStatusEnabled,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeLoop,
		PromptTemplate: "run webhook",
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	created, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-webhook-secret-trigger",
		Type:   jfadk.WorkflowTriggerTypeWebhook,
		Status: jfadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger create: %v", err)
	}
	if created.Secret == "" || !created.Trigger.HasSecret || created.Trigger.SecretHash != "" {
		t.Fatalf("created webhook trigger = %+v secret=%q, want one-time secret and sanitized trigger", created.Trigger, created.Secret)
	}
	raw, ok, err := runtime.Store().WorkflowTrigger(ctx, created.Trigger.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTrigger raw ok=%v err=%v", ok, err)
	}
	if raw.SecretHash == "" || !verifyWorkflowSecret(created.Secret, raw.SecretHash) {
		t.Fatalf("stored webhook hash not verifiable: %+v", raw)
	}

	updated, err := service.SaveWorkflowTrigger(ctx, workflow.ID, created.Trigger.ID, jfadk.WorkflowTriggerWriteRequest{
		Type:   jfadk.WorkflowTriggerTypeWebhook,
		Title:  "Webhook Secret Updated",
		Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{"source": "unit-test"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger update: %v", err)
	}
	if updated.Secret != "" {
		t.Fatalf("update secret = %q, want empty without reset", updated.Secret)
	}
	if _, err := service.RunWorkflowWebhook(ctx, created.Trigger.ID, "not-the-secret", nil); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("RunWorkflowWebhook invalid secret err = %v, want secret rejection", err)
	}

	reset, err := service.SaveWorkflowTrigger(ctx, workflow.ID, created.Trigger.ID, jfadk.WorkflowTriggerWriteRequest{
		Type:        jfadk.WorkflowTriggerTypeWebhook,
		Status:      jfadk.WorkflowTriggerStatusEnabled,
		ResetSecret: true,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger reset: %v", err)
	}
	if reset.Secret == "" || reset.Secret == created.Secret {
		t.Fatalf("reset secret = %q, want new one-time secret", reset.Secret)
	}
}

func TestSaveWorkflowRoundTripsCanvasGraph(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-canvas-agent", Name: "Workflow Canvas Agent", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID:             "workflow-canvas-roundtrip",
		Name:           "Canvas Round Trip",
		Status:         jfadk.WorkflowStatusDisabled,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeLoop,
		PromptTemplate: "run canvas",
		CanvasGraph: &jfadk.WorkflowCanvasGraph{
			Version: "adk-workflow-canvas/v1",
			Nodes: []jfadk.WorkflowCanvasNode{
				{ID: "start", Type: "start", Position: jfadk.WorkflowCanvasPoint{X: 80, Y: 250}},
				{ID: "agent", Type: "agent", Position: jfadk.WorkflowCanvasPoint{X: 385, Y: 250}},
			},
			Edges: []jfadk.WorkflowCanvasEdge{
				{ID: "start->agent", Source: "start", Target: "agent", Type: "smoothstep"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	stored, err := service.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if stored.CanvasGraph == nil || len(stored.CanvasGraph.Nodes) != 2 || stored.CanvasGraph.Edges[0].Target != "agent" {
		t.Fatalf("stored canvas graph = %+v", stored.CanvasGraph)
	}
}

func TestRunWorkflowStoresResultAndNodeTrace(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-trace-agent", Name: "Workflow Trace Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID:             "workflow-trace",
		Name:           "Workflow Trace",
		Status:         jfadk.WorkflowStatusEnabled,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeChat,
		PromptTemplate: "run {{ .symbol }}",
		DefaultInputs:  map[string]any{"symbol": "US.AAPL"},
		CanvasGraph:    workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	result, err := service.RunWorkflow(ctx, workflow.ID, map[string]any{"symbol": "US.MSFT"})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if result.Log.Result == nil || !strings.Contains(result.Log.Result.Markdown, "ok") {
		t.Fatalf("workflow result = %+v, want markdown containing child reply", result.Log.Result)
	}
	if len(result.Log.NodeRuns) < 3 {
		t.Fatalf("node runs = %+v, want Start/Agent/Monitor trace", result.Log.NodeRuns)
	}
	if result.Log.NodeRuns[0].NodeID != "start" || result.Log.NodeRuns[1].NodeID != "agent:primary" {
		t.Fatalf("node run order = %+v", result.Log.NodeRuns)
	}
	if result.Log.NodeRuns[1].Outputs["reply"] != "ok" {
		t.Fatalf("agent outputs = %+v, want reply ok", result.Log.NodeRuns[1].Outputs)
	}
	stored, ok, err := runtime.Store().WorkflowTriggerLog(ctx, result.Log.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTriggerLog ok=%v err=%v", ok, err)
	}
	if stored.Result == nil || !strings.Contains(stored.Result.Markdown, "ok") || len(stored.NodeRuns) < 3 {
		t.Fatalf("stored log = %+v, want result and node trace", stored)
	}
}

func TestRunWorkflowWithoutCanvasGraphFailsInsteadOfChatFallback(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-no-canvas-agent", Name: "Workflow No Canvas Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID: "workflow-no-canvas", Name: "Workflow No Canvas", Status: jfadk.WorkflowStatusEnabled,
		AgentID: agent.ID, WorkMode: jfadk.WorkModeChat, PromptTemplate: "run",
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	result, err := service.RunWorkflow(ctx, workflow.ID, nil)
	if err == nil || result.Response != nil || result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed {
		t.Fatalf("RunWorkflow result=%+v err=%v, want failed log without response", result, err)
	}
	if !strings.Contains(err.Error(), "canvas graph is required") || !strings.Contains(result.Log.Error, "canvas graph is required") {
		t.Fatalf("RunWorkflow err=%v log=%+v, want canvas graph error", err, result.Log)
	}
}

func TestRunWorkflowCanvasCompilesAndStoresNodeOutputs(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-canvas-agent", Name: "Workflow Canvas Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent parent: %v", err)
	}
	child, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-canvas-child", Name: "Workflow Canvas Child", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent child: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID:             "workflow-canvas",
		Name:           "Workflow Canvas",
		Status:         jfadk.WorkflowStatusEnabled,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeLoop,
		PromptTemplate: "fallback {{ .symbol }}",
		DefaultInputs:  map[string]any{"symbol": "US.AAPL"},
		CanvasGraph: &jfadk.WorkflowCanvasGraph{
			Version: "adk-workflow-canvas/v1",
			Nodes: []jfadk.WorkflowCanvasNode{
				{ID: "start", Type: "start", Position: jfadk.WorkflowCanvasPoint{}},
				{ID: "research", Type: "agent", Position: jfadk.WorkflowCanvasPoint{}, Data: map[string]any{
					"title": "Research", "agentId": child.ID, "promptTemplate": "research {{ .symbol }}",
				}},
				{ID: "report", Type: "agent", Position: jfadk.WorkflowCanvasPoint{}, Data: map[string]any{
					"title": "Report", "promptTemplate": "report {{ .symbol }}",
				}},
				{ID: "monitor", Type: "monitor", Position: jfadk.WorkflowCanvasPoint{}},
			},
			Edges: []jfadk.WorkflowCanvasEdge{
				{ID: "start-research", Source: "start", Target: "research"},
				{ID: "research-report", Source: "research", Target: "report"},
				{ID: "report-monitor", Source: "report", Target: "monitor"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	result, err := service.RunWorkflow(ctx, workflow.ID, map[string]any{"symbol": "US.MSFT"})
	if err != nil {
		t.Fatalf("RunWorkflow canvas: %v", err)
	}
	if result.Log.Status != jfadk.WorkflowTriggerLogStatusSucceeded || result.Response == nil {
		t.Fatalf("canvas result = %+v, want succeeded response", result)
	}
	if result.Response.Run.WorkflowEngine != jfadk.WorkflowEngineADK2Canvas {
		t.Fatalf("workflow engine = %q, want canvas", result.Response.Run.WorkflowEngine)
	}
	if len(result.Response.Run.ChildRunIDs) != 2 {
		t.Fatalf("child run ids = %+v, want two canvas child runs", result.Response.Run.ChildRunIDs)
	}
	research := workflowNodeRunByID(result.Log.NodeRuns, "research")
	report := workflowNodeRunByID(result.Log.NodeRuns, "report")
	if research == nil || report == nil {
		t.Fatalf("node runs = %+v, want research and report nodes", result.Log.NodeRuns)
	}
	if research.Inputs["agentId"] != child.ID || research.Inputs["message"] != "research US.MSFT" {
		t.Fatalf("research inputs = %+v, want rendered child agent message", research.Inputs)
	}
	if research.Outputs["reply"] != "ok" || report.Outputs["reply"] != "ok" {
		t.Fatalf("node outputs research=%+v report=%+v, want replies", research.Outputs, report.Outputs)
	}
}

func workflowNodeRunByID(runs []jfadk.WorkflowNodeRun, id string) *jfadk.WorkflowNodeRun {
	for index := range runs {
		if runs[index].NodeID == id {
			return &runs[index]
		}
	}
	return nil
}
