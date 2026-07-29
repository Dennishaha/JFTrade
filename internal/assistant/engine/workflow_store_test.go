package adk

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWorkflowStoreCRUDSoftDeleteAndLogs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	for _, table := range []string{tableWorkflows, tableWorkflowTriggers, tableWorkflowTriggerLog} {
		var count int
		if err := store.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table); err != nil {
			t.Fatalf("check workflow schema table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("workflow schema table %s count = %d, want 1", table, count)
		}
	}

	workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
		ID:             "wf-store-test",
		Name:           "Store Test Workflow",
		Status:         WorkflowStatusEnabled,
		AgentID:        "agent-store-test",
		WorkMode:       WorkModeLoop,
		PromptTemplate: "run {{ .workflow.name }}",
		DefaultInputs:  map[string]any{"market": "US"},
		CanvasGraph: &WorkflowCanvasGraph{
			Version: "adk-workflow-canvas/v1",
			Nodes: []WorkflowCanvasNode{
				{ID: "start", Type: "start", Position: WorkflowCanvasPoint{X: 80, Y: 250}},
				{ID: "agent", Type: "agent", Position: WorkflowCanvasPoint{X: 385, Y: 250}},
			},
			Edges: []WorkflowCanvasEdge{
				{ID: "start->agent", Source: "start", Target: "agent", Type: "smoothstep"},
			},
			Viewport: map[string]any{"x": 0, "y": 0, "zoom": 1},
		},
		Tags: []string{"store", "workflow"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowDefinition: %v", err)
	}
	if workflow.ID == "" || workflow.CreatedAt == "" || workflow.UpdatedAt == "" {
		t.Fatalf("saved workflow missing identity/timestamps: %+v", workflow)
	}

	listed, total, err := store.ListWorkflowDefinitionsPage(ctx, WorkflowStatusEnabled, 10, 0)
	if err != nil {
		t.Fatalf("ListWorkflowDefinitionsPage: %v", err)
	}
	if total != 1 || len(listed) != 1 || listed[0].ID != workflow.ID {
		t.Fatalf("listed workflows total=%d items=%+v, want saved workflow", total, listed)
	}
	if listed[0].CanvasGraph == nil || len(listed[0].CanvasGraph.Nodes) != 2 || listed[0].CanvasGraph.Edges[0].Source != "start" {
		t.Fatalf("listed workflow canvas graph = %+v, want saved graph", listed[0].CanvasGraph)
	}
	storedWorkflow, ok, err := store.WorkflowDefinition(ctx, workflow.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowDefinition ok=%v err=%v", ok, err)
	}
	if storedWorkflow.CanvasGraph == nil || storedWorkflow.CanvasGraph.Version != "adk-workflow-canvas/v1" {
		t.Fatalf("stored workflow canvas graph = %+v", storedWorkflow.CanvasGraph)
	}

	trigger, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
		ID:         "trigger-store-test",
		WorkflowID: workflow.ID,
		Type:       WorkflowTriggerTypeSchedule,
		Title:      "Daily",
		Status:     WorkflowTriggerStatusEnabled,
		Config:     map[string]any{"cron": "0 8 * * 1-5", "timezone": "Asia/Shanghai"},
		NextRunAt:  "2026-07-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}

	due, err := store.ListDueWorkflowScheduleTriggers(ctx, "2026-07-01T00:00:00Z", 20)
	if err != nil {
		t.Fatalf("ListDueWorkflowScheduleTriggers: %v", err)
	}
	if len(due) != 1 || due[0].ID != trigger.ID {
		t.Fatalf("due triggers = %+v, want %s", due, trigger.ID)
	}

	enabledByType, err := store.ListEnabledWorkflowTriggersByType(ctx, WorkflowTriggerTypeSchedule)
	if err != nil {
		t.Fatalf("ListEnabledWorkflowTriggersByType: %v", err)
	}
	if len(enabledByType) != 1 || enabledByType[0].ID != trigger.ID {
		t.Fatalf("enabled schedule triggers = %+v, want %s", enabledByType, trigger.ID)
	}

	log, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{
		WorkflowID:  workflow.ID,
		TriggerID:   trigger.ID,
		TriggerType: WorkflowTriggerTypeSchedule,
		Status:      WorkflowTriggerLogStatusPendingApproval,
		RunID:       "run-store-test",
		SessionID:   "session-store-test",
		Inputs:      map[string]any{"market": "US"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}

	logs, total, err := store.ListWorkflowTriggerLogsPage(ctx, workflow.ID, trigger.ID, WorkflowTriggerLogStatusPendingApproval, 10, 0)
	if err != nil {
		t.Fatalf("ListWorkflowTriggerLogsPage: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].ID != log.ID || logs[0].RunID != "run-store-test" {
		t.Fatalf("workflow logs total=%d items=%+v, want saved log", total, logs)
	}

	activeLogs, err := store.ListActiveWorkflowTriggerLogs(ctx, trigger.ID)
	if err != nil {
		t.Fatalf("ListActiveWorkflowTriggerLogs: %v", err)
	}
	if len(activeLogs) != 1 || activeLogs[0].ID != log.ID {
		t.Fatalf("active logs = %+v, want %s", activeLogs, log.ID)
	}

	deleted, err := store.DeleteWorkflowDefinition(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("DeleteWorkflowDefinition: %v", err)
	}
	if deleted.Status != WorkflowStatusDisabled || deleted.DeletedAt == nil {
		t.Fatalf("deleted workflow = %+v, want disabled with deletedAt", deleted)
	}

	listed, total, err = store.ListWorkflowDefinitionsPage(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("ListWorkflowDefinitionsPage after delete: %v", err)
	}
	if total != 0 || len(listed) != 0 {
		t.Fatalf("listed workflows after delete total=%d items=%+v, want none", total, listed)
	}

	storedTrigger, ok, err := store.WorkflowTrigger(ctx, trigger.ID)
	if err != nil || !ok {
		t.Fatalf("WorkflowTrigger after workflow delete ok=%v err=%v", ok, err)
	}
	if storedTrigger.Status != WorkflowTriggerStatusDisabled || storedTrigger.DeletedAt != nil {
		t.Fatalf("trigger after workflow delete = %+v, want disabled but not deleted", storedTrigger)
	}
}
