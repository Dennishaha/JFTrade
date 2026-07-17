package adk

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestStoreDataAdditionalCoverageBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("SaveRun and approval helpers surface lookup and payload failures", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if err := store.SaveRun(ctx, Run{ID: "goal-run", WorkMode: WorkModeLoop}); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("SaveRun err = %v, want %s failure", err, tableRuns)
		}
	})

	t.Run("SaveRunAndDenyPendingApprovals surfaces approval query failures", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableApprovals); err != nil {
			t.Fatalf("drop approvals table: %v", err)
		}
		if err := store.SaveRunAndDenyPendingApprovals(ctx, Run{ID: "run-deny", SessionID: "session", AgentID: "agent", Status: RunStatusRunning}); err == nil || !strings.Contains(err.Error(), tableApprovals) {
			t.Fatalf("SaveRunAndDenyPendingApprovals err = %v, want %s failure", err, tableApprovals)
		}
	})

	t.Run("SaveApprovalIfConfirmationAbsent falls back to insert errors when confirmation lookup misses", func(t *testing.T) {
		store := newBusinessStore(t)
		existing := Approval{
			ID: "approval-duplicate-id", RunID: "run", AgentID: "agent", Status: ApprovalStatusPending,
			ConfirmationCallID: "existing-confirmation", CreatedAt: nowString(), UpdatedAt: nowString(),
		}
		if err := store.SaveApproval(ctx, existing); err != nil {
			t.Fatalf("SaveApproval(existing): %v", err)
		}
		created, ok, err := store.SaveApprovalIfConfirmationAbsent(ctx, Approval{
			ID: "approval-duplicate-id", RunID: "run", AgentID: "agent", Status: ApprovalStatusPending,
			ConfirmationCallID: "new-confirmation",
		})
		if err == nil || ok || created.ID != "" {
			t.Fatalf("SaveApprovalIfConfirmationAbsent approval=%+v ok=%v err=%v, want duplicate insert error", created, ok, err)
		}
	})

	t.Run("approval lookup and audit helpers reject malformed payloads", func(t *testing.T) {
		store := newBusinessStore(t)
		now := nowString()
		if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"approval-bad-payload", "run", "agent", ApprovalStatusPending, `{"confirmationCallId":"bad-confirmation","status":["broken"]}`, now, now,
		); err != nil {
			t.Fatalf("insert malformed approval: %v", err)
		}
		if _, _, err := store.approvalByConfirmationCallID(ctx, "missing-confirmation"); err != nil {
			t.Fatalf("approvalByConfirmationCallID missing err = %v, want nil", err)
		}
		if _, _, err := store.approvalByConfirmationCallID(ctx, "bad-confirmation"); err == nil {
			t.Fatal("approvalByConfirmationCallID accepted malformed JSON payload")
		}
		if err := store.AddAuditEvent(ctx, AuditEvent{
			Kind:     "bad.audit",
			Metadata: map[string]any{"bad": make(chan int)},
		}); err == nil {
			t.Fatal("AddAuditEvent accepted unsupported metadata payload")
		}
	})

	t.Run("task and memory helpers cover nil patch, default scope and agent lookup errors", func(t *testing.T) {
		if err := applyTaskPatch(nil, "task", TaskPatchRequest{}); err != nil {
			t.Fatalf("applyTaskPatch(nil): %v", err)
		}

		store := newBusinessStore(t)
		entry, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "workspace note", Value: "hello"})
		if err != nil {
			t.Fatalf("SaveMemory workspace: %v", err)
		}
		if entry.Scope != "workspace" || entry.AgentID != "" {
			t.Fatalf("workspace memory entry = %+v, want workspace scope without agent", entry)
		}
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
			t.Fatalf("drop agents table: %v", err)
		}
		if _, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "agent note", Value: "x", Scope: "agent", AgentID: "agent-1"}); err == nil || !strings.Contains(err.Error(), tableAgents) {
			t.Fatalf("SaveMemory agent lookup err = %v, want %s failure", err, tableAgents)
		}
	})

	t.Run("task patches validate status and normalize child execution metadata", func(t *testing.T) {
		store := newBusinessStore(t)
		task, err := store.SaveTask(ctx, TaskWriteRequest{ID: "coverage-task-patch", Title: "Patch child execution"})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		invalidStatus := "waiting-for-human"
		if _, err := store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: &invalidStatus}); err == nil || !strings.Contains(err.Error(), "invalid task status") {
			t.Fatalf("invalid task status error = %v", err)
		}
		childAgent := " child-reviewer "
		childMode := " approval "
		updated, err := store.UpdateTask(ctx, task.ID, TaskPatchRequest{
			ChildAgentID:        &childAgent,
			ChildPermissionMode: &childMode,
		})
		if err != nil || updated.ChildAgentID != "child-reviewer" || updated.ChildPermissionMode != "approval" {
			t.Fatalf("normalized child task metadata = %+v/%v", updated, err)
		}
	})

	t.Run("approval resolution and terminal denial surface conditional write failures", func(t *testing.T) {
		store := newBusinessStore(t)
		approval := Approval{ID: "coverage-approval-update", RunID: "coverage-run", AgentID: "agent", Status: ApprovalStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()}
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_approval_resolution BEFORE UPDATE ON `+tableApprovals+` WHEN NEW.id = '`+approval.ID+`' BEGIN SELECT RAISE(FAIL, 'approval resolution write rejected'); END`); err != nil {
			t.Fatalf("create approval update trigger: %v", err)
		}
		if _, _, err := store.ResolvePendingApproval(ctx, approval.ID, ApprovalStatusApproved); err == nil || !strings.Contains(err.Error(), "approval resolution write rejected") {
			t.Fatalf("ResolvePendingApproval write error = %v", err)
		}

		store = newBusinessStore(t)
		approval = Approval{ID: "coverage-deny-update", RunID: "coverage-deny-run", AgentID: "agent", Status: ApprovalStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()}
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval terminal denial: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_terminal_denial BEFORE UPDATE ON `+tableApprovals+` WHEN NEW.id = '`+approval.ID+`' BEGIN SELECT RAISE(FAIL, 'terminal denial approval write rejected'); END`); err != nil {
			t.Fatalf("create terminal denial trigger: %v", err)
		}
		if err := store.SaveRunAndDenyPendingApprovals(ctx, Run{ID: approval.RunID, SessionID: "session", AgentID: approval.AgentID, Status: RunStatusFailed}); err == nil || !strings.Contains(err.Error(), "terminal denial approval write rejected") {
			t.Fatalf("SaveRunAndDenyPendingApprovals update error = %v", err)
		}
		stored, ok, err := store.Approval(ctx, approval.ID)
		if err != nil || !ok || stored.Status != ApprovalStatusPending {
			t.Fatalf("terminal denial rollback approval = %+v/%v/%v", stored, ok, err)
		}
	})

	t.Run("a conditional approval update that loses its race returns the stored record", func(t *testing.T) {
		store := newBusinessStore(t)
		approval := Approval{ID: "coverage-approval-race", RunID: "coverage-race-run", AgentID: "agent", Status: ApprovalStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()}
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		// SQLite RAISE(IGNORE) mirrors the observable result of a concurrent
		// resolver winning the conditional UPDATE: this call must not claim a
		// state transition that the database did not persist.
		if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER coverage98_ignore_approval_race BEFORE UPDATE ON `+tableApprovals+` WHEN NEW.id = '`+approval.ID+`' BEGIN SELECT RAISE(IGNORE); END`); err != nil {
			t.Fatalf("create approval race trigger: %v", err)
		}
		current, changed, err := store.ResolvePendingApproval(ctx, approval.ID, ApprovalStatusApproved)
		if err != nil || changed || current.ID != approval.ID || current.Status != ApprovalStatusPending {
			t.Fatalf("lost conditional approval update = %+v/%v/%v", current, changed, err)
		}
	})

	t.Run("root workflow terminal denial propagates preparation read failures", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if err := store.SaveRunAndDenyPendingApprovals(ctx, Run{ID: "coverage-root-deny", WorkMode: WorkModeLoop, Status: RunStatusFailed}); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("root workflow preparation error = %v", err)
		}
	})
}

func TestStoreEntityAdditionalCoverageBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("SaveAgent generates identifiers and rejects invalid status", func(t *testing.T) {
		store := newBusinessStore(t)
		agent, err := store.SaveAgent(ctx, AgentWriteRequest{})
		if err != nil {
			t.Fatalf("SaveAgent generated: %v", err)
		}
		if !strings.HasPrefix(agent.ID, "agent-") || agent.Status != AgentStatusEnabled {
			t.Fatalf("generated agent = %+v, want generated id and enabled status", agent)
		}
		if _, err := store.SaveAgent(ctx, AgentWriteRequest{ID: "agent-invalid-status", Status: "BROKEN"}); err == nil || !strings.Contains(err.Error(), "invalid agent status") {
			t.Fatalf("SaveAgent invalid status err = %v", err)
		}
		if err := store.DeleteAgent(ctx, "missing-agent"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteAgent missing err = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("provider deletion and default selection surface storage failures", func(t *testing.T) {
		store := newBusinessStore(t)
		provider, err := store.SaveProvider(ctx, ProviderWriteRequest{
			ID: "provider-delete-error", BaseURL: "https://example.test/v1", Model: "model", Enabled: true,
		})
		if err != nil {
			t.Fatalf("SaveProvider: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableProviders); err != nil {
			t.Fatalf("drop providers table: %v", err)
		}
		if err := store.DeleteProvider(ctx, provider.ID); err == nil || !strings.Contains(err.Error(), tableProviders) {
			t.Fatalf("DeleteProvider err = %v, want %s failure", err, tableProviders)
		}
	})

	t.Run("ensureDefaultProvider surfaces provider list failures", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableProviders); err != nil {
			t.Fatalf("drop providers table: %v", err)
		}
		if _, err := store.ensureDefaultProvider(ctx); err == nil || !strings.Contains(err.Error(), tableProviders) {
			t.Fatalf("ensureDefaultProvider err = %v, want %s failure", err, tableProviders)
		}
	})
}
