package adk

import (
	"context"
	"testing"
)

func TestCoverage98CanvasWorkflowAppliesExplicitProviderAndModelOverrides(t *testing.T) {
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "coverage98-canvas-override-agent",
		Name:           "Canvas Override Agent",
		Model:          "agent-default-model",
		Status:         AgentStatusEnabled,
		WorkMode:       WorkModeChat,
		PermissionMode: PermissionModeLessApproval,
	})

	response, err := runtime.RunCanvasWorkflow(context.Background(), WorkflowCanvasRunRequest{
		Workflow: WorkflowDefinition{
			ID:             "coverage98-canvas-overrides",
			Name:           "Canvas Override Contract",
			AgentID:        agent.ID,
			ProviderID:     testProviderID,
			Model:          "canvas-selected-model",
			PermissionMode: PermissionModeApproval,
			CanvasGraph: &WorkflowCanvasGraph{
				Nodes: []WorkflowCanvasNode{
					canvasNode("start", "start", nil),
					canvasNode("review", "agent", map[string]any{"title": "Review", "message": "Review the supplied market note."}),
				},
				Edges: []WorkflowCanvasEdge{{ID: "start-review", Source: "start", Target: "review"}},
			},
		},
		Message:   "Review a market note.",
		Objective: "Keep the explicit execution model auditable.",
	})
	if err != nil {
		t.Fatalf("RunCanvasWorkflow: %v", err)
	}
	if response.Run.Status != RunStatusCompleted || response.Run.ProviderID != testProviderID || response.Run.Model != "canvas-selected-model" || response.Run.PermissionMode != PermissionModeApproval {
		t.Fatalf("canvas override run = %+v", response.Run)
	}
	if len(response.Run.ChildRunIDs) != 1 {
		t.Fatalf("canvas override child runs = %#v", response.Run.ChildRunIDs)
	}
	child, ok, err := runtime.Store().Run(t.Context(), response.Run.ChildRunIDs[0])
	if err != nil || !ok || child.ProviderID != testProviderID || child.Model != "canvas-selected-model" || child.PermissionMode != PermissionModeApproval {
		t.Fatalf("canvas override child = %+v ok=%v err=%v", child, ok, err)
	}
}
