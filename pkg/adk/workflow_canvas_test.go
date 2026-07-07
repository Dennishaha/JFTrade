package adk

import (
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

func canvasNode(id string, nodeType string, data map[string]any) WorkflowCanvasNode {
	return WorkflowCanvasNode{ID: id, Type: nodeType, Position: WorkflowCanvasPoint{}, Data: data}
}
