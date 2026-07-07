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
