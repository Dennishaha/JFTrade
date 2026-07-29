package adk

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestStoreMaintenanceAdditionalCoverageBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("purge detects changed trigger and workflow candidates", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Triggers: []string{"missing-trigger"}}); !errors.Is(err, ErrCleanupCandidatesChanged) {
			t.Fatalf("PurgeDeletedConfigs missing trigger err = %v", err)
		}
		if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Workflows: []string{"missing-workflow"}}); !errors.Is(err, ErrCleanupCandidatesChanged) {
			t.Fatalf("PurgeDeletedConfigs missing workflow err = %v", err)
		}
		workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{ID: "purge-active-workflow", Name: "Active", Status: WorkflowStatusEnabled})
		if err != nil {
			t.Fatalf("SaveWorkflowDefinition: %v", err)
		}
		trigger, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{ID: "purge-active-trigger", WorkflowID: workflow.ID, Type: WorkflowTriggerTypeSchedule, Status: WorkflowTriggerStatusEnabled})
		if err != nil {
			t.Fatalf("SaveWorkflowTrigger: %v", err)
		}
		if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Triggers: []string{trigger.ID}}); !errors.Is(err, ErrCleanupCandidatesChanged) {
			t.Fatalf("PurgeDeletedConfigs active trigger err = %v", err)
		}
	})

	t.Run("purge surfaces delete failures", func(t *testing.T) {
		store := newBusinessStore(t)
		agent, err := store.SaveAgent(ctx, AgentWriteRequest{ID: "purge-delete-agent", Name: "Agent", Status: AgentStatusEnabled})
		if err != nil {
			t.Fatalf("SaveAgent: %v", err)
		}
		if err := store.DeleteAgent(ctx, agent.ID); err != nil {
			t.Fatalf("DeleteAgent: %v", err)
		}
		installStoreFailTrigger(t, store, "fail_purge_agent_delete", tableAgents, "DELETE", "purge agent delete failed")
		if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Agents: []string{agent.ID}}); err == nil || !strings.Contains(err.Error(), "purge agent delete failed") {
			t.Fatalf("PurgeDeletedConfigs delete agent err = %v", err)
		}
	})

	t.Run("purge surfaces trigger query and workflow delete failures", func(t *testing.T) {
		store := newBusinessStore(t)
		workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
			ID: "purge-delete-workflow", Name: "Delete Workflow", Status: WorkflowStatusEnabled, DeletedAt: new(nowString()),
		})
		if err != nil {
			t.Fatalf("SaveWorkflowDefinition: %v", err)
		}
		trigger, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
			ID: "purge-delete-trigger", WorkflowID: workflow.ID, Type: WorkflowTriggerTypeSchedule, Status: WorkflowTriggerStatusEnabled,
		})
		if err != nil {
			t.Fatalf("SaveWorkflowTrigger: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `UPDATE `+tableWorkflowTriggers+` SET payload_json = json_set(payload_json, '$.deletedAt', ?) WHERE id = ?`, nowString(), trigger.ID); err != nil {
			t.Fatalf("mark trigger deleted: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableWorkflowTriggers); err != nil {
			t.Fatalf("drop workflow triggers: %v", err)
		}
		if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Triggers: []string{trigger.ID}}); err == nil {
			t.Fatal("PurgeDeletedConfigs accepted dropped workflow trigger table")
		}

		store = newBusinessStore(t)
		workflow, err = store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
			ID: "purge-delete-workflow-fail", Name: "Delete Workflow Fail", Status: WorkflowStatusEnabled, DeletedAt: new(nowString()),
		})
		if err != nil {
			t.Fatalf("SaveWorkflowDefinition second: %v", err)
		}
		installStoreFailTrigger(t, store, "fail_purge_workflow_delete", tableWorkflows, "DELETE", "purge workflow delete failed")
		if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Workflows: []string{workflow.ID}}); err == nil || !strings.Contains(err.Error(), "purge workflow delete failed") {
			t.Fatalf("PurgeDeletedConfigs workflow delete err = %v", err)
		}
	})

	t.Run("compact database validates unavailable store", func(t *testing.T) {
		if err := (*Store)(nil).CompactDatabase(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("nil CompactDatabase err = %v", err)
		}
	})
}

func TestStoreHandoffAdditionalCoverageBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("handoff list surfaces query and payload errors", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableHandoffSegments+` (id, session_id, active, sequence_no, created_at, updated_at, payload_json) VALUES ('bad', 'session', 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '{')`); err != nil {
			t.Fatalf("insert bad handoff: %v", err)
		}
		if _, err := store.HandoffSegments(ctx, "session", false); err == nil {
			t.Fatal("HandoffSegments accepted malformed payload")
		}
		if _, err := store.HandoffSegmentsForRevision(ctx, "session", "ctx", false); err == nil {
			t.Fatal("HandoffSegmentsForRevision accepted malformed payload")
		}

		errorStore := newBusinessStore(t)
		if _, err := errorStore.db.ExecContext(ctx, `DROP TABLE `+tableHandoffSegments); err != nil {
			t.Fatalf("drop handoff table: %v", err)
		}
		if _, err := errorStore.HandoffSegments(ctx, "session", true); err == nil {
			t.Fatal("HandoffSegments succeeded after table drop")
		}
		if _, err := errorStore.HandoffSegmentsForRevision(ctx, "session", "ctx", true); err == nil {
			t.Fatal("HandoffSegmentsForRevision succeeded after table drop")
		}
	})

	t.Run("replace active handoff surfaces begin and superseded failures", func(t *testing.T) {
		store := newBusinessStore(t)
		jftradeCheckTestError(t, store.Close())
		if _, err := store.ReplaceActiveHandoffSegments(ctx, "session", HandoffSegment{SessionID: "session"}, nil); err == nil {
			t.Fatal("ReplaceActiveHandoffSegments closed store err = nil")
		}

		store = newBusinessStore(t)
		_, err := store.ReplaceActiveHandoffSegments(ctx, "session", HandoffSegment{SessionID: "session"}, []HandoffSegment{{ID: "blank-session"}})
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ReplaceActiveHandoffSegments blank superseded err = %v, want os.ErrNotExist", err)
		}
	})
}

func installStoreFailTrigger(t *testing.T, store *Store, name string, tableName string, op string, message string) {
	t.Helper()
	sql := `CREATE TRIGGER ` + name + ` BEFORE ` + op + ` ON ` + tableName + ` BEGIN SELECT RAISE(FAIL, '` + message + `'); END`
	if _, err := store.db.ExecContext(t.Context(), sql); err != nil {
		t.Fatalf("create trigger %s: %v", name, err)
	}
}
