package adk

import (
	"strings"
	"testing"
)

func TestPurgeDeletedConfigsCascadesConfigurationButKeepsHistory(t *testing.T) {
	store := newBusinessStore(t)
	ctx := t.Context()
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{ID: "purge-agent", Name: "Purge Agent", Status: AgentStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{ID: "purge-workflow", Name: "Purge Workflow", Status: WorkflowStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	trigger, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{ID: "purge-trigger", WorkflowID: workflow.ID, Type: WorkflowTriggerTypeWebhook, Status: WorkflowTriggerStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	logItem, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{ID: "purge-log", WorkflowID: workflow.ID, TriggerID: trigger.ID, TriggerType: trigger.Type, Status: "COMPLETED", RunID: "historical-run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteWorkflowDefinition(ctx, workflow.ID); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{
		Agents: []string{agent.ID}, Workflows: []string{workflow.ID}, Triggers: []string{trigger.ID},
	})
	if err != nil || deleted != 3 {
		t.Fatalf("PurgeDeletedConfigs = %d, %v", deleted, err)
	}
	if _, ok, err := store.Agent(ctx, agent.ID); err != nil || ok {
		t.Fatalf("agent remains ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.WorkflowDefinition(ctx, workflow.ID); err != nil || ok {
		t.Fatalf("workflow remains ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.WorkflowTrigger(ctx, trigger.ID); err != nil || ok {
		t.Fatalf("trigger remains ok=%v err=%v", ok, err)
	}
	if stored, ok, err := store.WorkflowTriggerLog(ctx, logItem.ID); err != nil || !ok || stored.RunID != "historical-run" {
		t.Fatalf("historical log = %+v ok=%v err=%v", stored, ok, err)
	}
}

func TestCompactDatabaseReclaimsFreePages(t *testing.T) {
	store := newBusinessStore(t)
	ctx := t.Context()
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE maintenance_blob (id INTEGER PRIMARY KEY, payload TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("x", 8192)
	for range 256 {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO maintenance_blob(payload) VALUES (?)`, payload); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM maintenance_blob`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := store.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before == 0 {
		t.Fatal("expected free pages before compact")
	}
	if err := store.CompactDatabase(ctx); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := store.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("freelist after compact = %d", after)
	}
}
