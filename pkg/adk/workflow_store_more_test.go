package adk

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func newWorkflowStoreBoundaryStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "adk.db"), filepath.Join(root, "secrets.json"), filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })
	return store
}

func TestWorkflowStoreAdditionalBoundaryBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("workflow definition surfaces marshal lookup and delete errors", func(t *testing.T) {
		store := newWorkflowStoreBoundaryStore(t)
		if _, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
			Name:          "bad workflow",
			DefaultInputs: map[string]any{"nan": math.NaN()},
		}); err == nil {
			t.Fatal("SaveWorkflowDefinition accepted non-JSON payload")
		}

		if _, ok, err := store.WorkflowDefinition(ctx, "missing-workflow"); err != nil || ok {
			t.Fatalf("WorkflowDefinition missing ok=%v err=%v, want false/nil", ok, err)
		}
		if _, err := store.DeleteWorkflowDefinition(ctx, "missing-workflow"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteWorkflowDefinition missing err = %v, want os.ErrNotExist", err)
		}

		saved, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
			ID:     "wf-extra-boundary",
			Name:   "Workflow Boundary",
			Status: WorkflowStatusEnabled,
		})
		if err != nil {
			t.Fatalf("SaveWorkflowDefinition: %v", err)
		}
		if _, err := store.DeleteWorkflowDefinition(ctx, saved.ID); err != nil {
			t.Fatalf("DeleteWorkflowDefinition first: %v", err)
		}
		if _, err := store.DeleteWorkflowDefinition(ctx, saved.ID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteWorkflowDefinition second err = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("workflow definition and trigger listings surface table errors", func(t *testing.T) {
		store := newWorkflowStoreBoundaryStore(t)
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableWorkflows); err != nil {
			t.Fatalf("drop workflows: %v", err)
		}
		if _, ok, err := store.WorkflowDefinition(ctx, "wf"); err == nil || ok {
			t.Fatalf("WorkflowDefinition dropped table ok=%v err=%v, want error", ok, err)
		}
		if _, _, err := store.ListWorkflowDefinitionsPage(ctx, "", 10, 0); err == nil {
			t.Fatal("ListWorkflowDefinitionsPage succeeded after dropping workflows table")
		}
	})

	t.Run("workflow trigger surfaces marshal lookup list and delete errors", func(t *testing.T) {
		store := newWorkflowStoreBoundaryStore(t)
		if _, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
			WorkflowID: "wf",
			Type:       WorkflowTriggerTypeSchedule,
			Config:     map[string]any{"nan": math.NaN()},
		}); err == nil {
			t.Fatal("SaveWorkflowTrigger accepted non-JSON payload")
		}
		if _, err := store.DeleteWorkflowTrigger(ctx, "missing-trigger"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteWorkflowTrigger missing err = %v, want os.ErrNotExist", err)
		}
		if _, ok, err := store.WorkflowTrigger(ctx, "missing-trigger"); err != nil || ok {
			t.Fatalf("WorkflowTrigger missing ok=%v err=%v, want false/nil", ok, err)
		}

		saved, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
			ID:         "trigger-extra-boundary",
			WorkflowID: "wf",
			Type:       WorkflowTriggerTypeSchedule,
			Status:     WorkflowTriggerStatusEnabled,
			NextRunAt:  "2026-07-05T00:00:00Z",
		})
		if err != nil {
			t.Fatalf("SaveWorkflowTrigger: %v", err)
		}
		if _, err := store.DeleteWorkflowTrigger(ctx, saved.ID); err != nil {
			t.Fatalf("DeleteWorkflowTrigger first: %v", err)
		}
		if _, err := store.DeleteWorkflowTrigger(ctx, saved.ID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteWorkflowTrigger second err = %v, want os.ErrNotExist", err)
		}

		errorStore := newWorkflowStoreBoundaryStore(t)
		if _, err := errorStore.db.ExecContext(ctx, `DROP TABLE `+tableWorkflowTriggers); err != nil {
			t.Fatalf("drop workflow_triggers: %v", err)
		}
		if _, ok, err := errorStore.WorkflowTrigger(ctx, saved.ID); err == nil || ok {
			t.Fatalf("WorkflowTrigger dropped table ok=%v err=%v, want error", ok, err)
		}
		if _, err := errorStore.ListWorkflowTriggers(ctx, "wf"); err == nil {
			t.Fatal("ListWorkflowTriggers succeeded after dropping trigger table")
		}
		if _, err := errorStore.ListEnabledWorkflowTriggersByType(ctx, WorkflowTriggerTypeSchedule); err == nil {
			t.Fatal("ListEnabledWorkflowTriggersByType succeeded after dropping trigger table")
		}
		if _, err := errorStore.ListDueWorkflowScheduleTriggers(ctx, "2026-07-05T00:00:00Z", 10); err == nil {
			t.Fatal("ListDueWorkflowScheduleTriggers succeeded after dropping trigger table")
		}
	})

	t.Run("workflow trigger logs surface marshal lookup and list errors", func(t *testing.T) {
		store := newWorkflowStoreBoundaryStore(t)
		if _, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{
			WorkflowID: "wf",
			TriggerID:  "trigger",
			Inputs:     map[string]any{"nan": math.NaN()},
		}); err == nil {
			t.Fatal("SaveWorkflowTriggerLog accepted non-JSON payload")
		}
		if _, ok, err := store.WorkflowTriggerLog(ctx, "missing-log"); err != nil || ok {
			t.Fatalf("WorkflowTriggerLog missing ok=%v err=%v, want false/nil", ok, err)
		}

		errorStore := newWorkflowStoreBoundaryStore(t)
		if _, err := errorStore.db.ExecContext(ctx, `DROP TABLE `+tableWorkflowTriggerLog); err != nil {
			t.Fatalf("drop workflow_trigger_log: %v", err)
		}
		if _, ok, err := errorStore.WorkflowTriggerLog(ctx, "log"); err == nil || ok {
			t.Fatalf("WorkflowTriggerLog dropped table ok=%v err=%v, want error", ok, err)
		}
		if _, _, err := errorStore.ListWorkflowTriggerLogsPage(ctx, "", "", "", 10, 0); err == nil {
			t.Fatal("ListWorkflowTriggerLogsPage succeeded after dropping log table")
		}
		if _, err := errorStore.ListActiveWorkflowTriggerLogs(ctx, "trigger"); err == nil {
			t.Fatal("ListActiveWorkflowTriggerLogs succeeded after dropping log table")
		}
	})
}
