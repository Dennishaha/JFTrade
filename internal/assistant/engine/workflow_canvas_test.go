package adk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowCanvasCompilerSequentialFanOutAndJoin(t *testing.T) {
	workflow := WorkflowDefinition{
		ID: "canvas", Name: "Canvas", AgentID: "parent", WorkMode: WorkModeLoop,
		PromptTemplate: "fallback prompt", PermissionMode: PermissionModeApproval,
		CanvasGraph: &WorkflowCanvasGraph{
			Nodes: []WorkflowCanvasNode{
				canvasNode("start", "start", nil),
				canvasNode("fetch", "agent", map[string]any{"title": "Fetch", "promptTemplate": "fetch {{ .symbol }}", "agentId": "researcher"}),
				canvasNode("risk", "agent", map[string]any{"title": "Risk"}),
				canvasNode("report", "agent", map[string]any{"title": "Report", "providerId": "provider-b", "model": "model-b"}),
				canvasNode("monitor", "monitor", nil),
			},
			Edges: []WorkflowCanvasEdge{
				{ID: "start-fetch", Source: "start", Target: "fetch"},
				{ID: "start-risk", Source: "start", Target: "risk"},
				{ID: "fetch-report", Source: "fetch", Target: "report"},
				{ID: "risk-report", Source: "risk", Target: "report"},
				{ID: "report-monitor", Source: "report", Target: "monitor"},
			},
		},
	}
	steps, err := compileWorkflowCanvasSteps(workflow, "fallback", "objective")
	if err != nil {
		t.Fatalf("compileWorkflowCanvasSteps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("steps = %+v, want 3", steps)
	}
	byID := map[string]workflowStep{}
	for _, step := range steps {
		byID[step.DependencyID] = step
		if step.PlanSource != workflowPlanSourceCanvas || step.WorkflowMode != WorkModeLoop || step.ModeHint != WorkModeChat {
			t.Fatalf("step metadata = %+v, want canvas loop/chat", step)
		}
	}
	if byID["fetch"].ChildAgentID != "researcher" || byID["fetch"].Message != "fetch {{ .symbol }}" {
		t.Fatalf("fetch step = %+v", byID["fetch"])
	}
	if len(byID["risk"].DependsOn) != 0 || byID["risk"].Message != "fallback" {
		t.Fatalf("risk step = %+v, want start dependency only and fallback message", byID["risk"])
	}
	report := byID["report"]
	if strings.Join(report.DependsOn, ",") != "fetch,risk" || report.ChildProviderID != "provider-b" || report.ChildModel != "model-b" {
		t.Fatalf("report step = %+v, want fan-in dependencies and overrides", report)
	}
}

func TestWorkflowCanvasCompilerRejectsInvalidGraphs(t *testing.T) {
	cases := []struct {
		name  string
		graph *WorkflowCanvasGraph
		want  string
	}{
		{
			name: "missing canvas",
			want: "graph is required",
		},
		{
			name:  "empty canvas",
			graph: &WorkflowCanvasGraph{},
			want:  "no executable agent",
		},
		{
			name: "no agent",
			graph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("monitor", "monitor", nil)},
				Edges: []WorkflowCanvasEdge{{ID: "start-monitor", Source: "start", Target: "monitor"}},
			},
			want: "no executable agent",
		},
		{
			name: "cycle",
			graph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("a", "agent", nil), canvasNode("b", "agent", nil)},
				Edges: []WorkflowCanvasEdge{{ID: "start-a", Source: "start", Target: "a"}, {ID: "a-b", Source: "a", Target: "b"}, {ID: "b-a", Source: "b", Target: "a"}},
			},
			want: "cycle",
		},
		{
			name: "unknown endpoint",
			graph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("a", "agent", nil)},
				Edges: []WorkflowCanvasEdge{{ID: "missing", Source: "start", Target: "missing"}},
			},
			want: "unknown target",
		},
		{
			name: "self edge",
			graph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("a", "agent", nil)},
				Edges: []WorkflowCanvasEdge{{ID: "self", Source: "a", Target: "a"}},
			},
			want: "must not connect a node to itself",
		},
		{
			name: "disconnected agent",
			graph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("a", "agent", nil), canvasNode("b", "agent", nil)},
				Edges: []WorkflowCanvasEdge{{ID: "start-a", Source: "start", Target: "a"}},
			},
			want: "not reachable",
		},
		{
			name: "unsupported type",
			graph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("tool", "tool", nil)},
			},
			want: "unsupported type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workflow := WorkflowDefinition{AgentID: "agent", CanvasGraph: tc.graph}
			_, err := compileWorkflowCanvasSteps(workflow, "message", "objective")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("compileWorkflowCanvasSteps err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestRunCanvasWorkflowExecutesAReachableAgentGraph(t *testing.T) {
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "canvas-run-agent", Name: "Canvas Run Agent", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeLessApproval,
	})

	response, err := runtime.RunCanvasWorkflow(context.Background(), WorkflowCanvasRunRequest{
		Workflow: WorkflowDefinition{
			ID: "run-canvas", Name: "Run Canvas", AgentID: agent.ID,
			CanvasGraph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{
					canvasNode("start", "start", nil),
					canvasNode("research", "agent", map[string]any{"title": "Research", "message": "Summarize the requested market signal."}),
				},
				Edges: []WorkflowCanvasEdge{{ID: "start-research", Source: "start", Target: "research"}},
			},
		},
		Message:   "研究一个市场信号",
		Objective: "给出研究结论",
	})
	if err != nil {
		t.Fatalf("RunCanvasWorkflow: %v", err)
	}
	if response.Run.Status != RunStatusCompleted || response.Run.WorkflowStatus != workflowStatusComplete || response.Run.WorkflowEngine != WorkflowEngineADK2Canvas {
		t.Fatalf("canvas run = %+v", response.Run)
	}
	if len(response.Run.WorkflowPlan) != 1 || response.Run.WorkflowPlan[0].PlanSource != workflowPlanSourceCanvas || response.Run.WorkflowPlan[0].Status != "DONE" {
		t.Fatalf("canvas plan = %+v", response.Run.WorkflowPlan)
	}
	if len(response.Run.ChildRunIDs) != 1 || strings.TrimSpace(response.Reply) == "" {
		t.Fatalf("canvas response = %+v", response)
	}
}

func TestRunCanvasWorkflowPausesForAChildInputRequest(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "canvas-input-agent", Name: "Canvas Input Agent", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeAll,
	})

	response, err := runtime.RunCanvasWorkflow(t.Context(), WorkflowCanvasRunRequest{
		Workflow: WorkflowDefinition{
			ID: "canvas-input", Name: "Canvas Input", AgentID: agent.ID,
			CanvasGraph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{
					canvasNode("start", "start", nil),
					canvasNode("ask", "agent", map[string]any{
						"title": "Collect user choice", "message": "@input.required choose a risk profile",
					}),
				},
				Edges: []WorkflowCanvasEdge{{ID: "start-ask", Source: "start", Target: "ask"}},
			},
		},
		Message:   "Collect a user choice before continuing.",
		Objective: "Capture the user's risk profile.",
	})
	if err != nil {
		t.Fatalf("RunCanvasWorkflow: %v", err)
	}
	if response.Run.Status != RunStatusPendingInput || response.Run.WorkflowStatus != workflowStatusPaused || response.InputRequest == nil {
		t.Fatalf("pending canvas response = %+v", response)
	}
	if len(response.Run.ChildRunIDs) != 1 || len(response.Run.WorkflowPlan) != 1 || response.Run.WorkflowPlan[0].Status != "BLOCKED" {
		t.Fatalf("pending canvas plan = %+v", response.Run)
	}
	childID := response.Run.ChildRunIDs[0]
	child, ok, err := runtime.Store().Run(t.Context(), childID)
	if err != nil || !ok || child.Status != RunStatusPendingInput || child.InputRequest == nil || child.InputRequest.ID != response.InputRequest.ID {
		t.Fatalf("persisted input child = %+v ok=%v err=%v", child, ok, err)
	}
	if runtime.adkRuns[response.Run.ID] == nil || runtime.adkRuns[childID] == nil {
		t.Fatalf("paused canvas execution must remain resumable: parent=%p child=%p", runtime.adkRuns[response.Run.ID], runtime.adkRuns[childID])
	}
}

func TestRunCanvasWorkflowFailsClosedWhenTheChildProviderIsUnavailable(t *testing.T) {
	runtime := newTestRuntime(t)
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider temporarily unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(unavailable.Close)
	providerID := "canvas-unavailable-provider"
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: providerID, DisplayName: "Unavailable canvas provider", BaseURL: unavailable.URL,
		Model: "test-model", APIKey: "sk-test", Enabled: true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "canvas-unavailable-agent", Name: "Canvas Unavailable Agent", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat, PermissionMode: PermissionModeLessApproval,
	})

	response, err := runtime.RunCanvasWorkflow(t.Context(), WorkflowCanvasRunRequest{
		Workflow: WorkflowDefinition{
			ID: "canvas-unavailable", Name: "Canvas Unavailable", AgentID: agent.ID,
			CanvasGraph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{
					canvasNode("start", "start", nil),
					canvasNode("research", "agent", map[string]any{"title": "Research", "message": "Fetch the current market summary."}),
				},
				Edges: []WorkflowCanvasEdge{{ID: "start-research", Source: "start", Target: "research"}},
			},
		},
		Message: "Fetch the current market summary.",
	})
	if err != nil {
		t.Fatalf("RunCanvasWorkflow must project provider failure into the run state: %v", err)
	}
	if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed || response.Run.FailureReason == "" {
		t.Fatalf("provider outage response = %+v", response)
	}
	stored, ok, err := runtime.Store().Run(t.Context(), response.Run.ID)
	if err != nil || !ok || stored.Status != RunStatusFailed || stored.WorkflowStatus != workflowStatusFailed {
		t.Fatalf("stored provider outage run = %+v ok=%v err=%v", stored, ok, err)
	}
}

func canvasNode(id string, nodeType string, data map[string]any) WorkflowCanvasNode {
	return WorkflowCanvasNode{ID: id, Type: nodeType, Position: WorkflowCanvasPoint{}, Data: data}
}
