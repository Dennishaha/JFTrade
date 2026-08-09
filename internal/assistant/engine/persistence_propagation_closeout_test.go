package adk

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/skillsruntime"
)

func TestWorkflowTaskToolContractAndLifecycleReadFailures(t *testing.T) {
	readRuntime := newTestRuntime(t)
	if _, err := readRuntime.Store().DB().ExecContext(t.Context(), `DROP TABLE `+tableRuns); err != nil {
		t.Fatalf("drop run table: %v", err)
	}
	if err := readRuntime.markApprovalContinuationFailed(t.Context(), "missing", errors.New("continuation failed")); err == nil {
		t.Fatal("markApprovalContinuationFailed hid a run lookup failure")
	}

	timeoutRuntime := newTestRuntime(t)
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	run := mustSaveRun(t, timeoutRuntime, Run{
		ID: "cancel-reconcile-write-run", Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: old, CreatedAt: old, UpdatedAt: old, MaxDurationMs: 1, Usage: &RunUsage{},
	})
	installRunUpdateRejectTrigger(t, timeoutRuntime, run.ID, "reject_cancel_reconcile_write")
	if _, err := timeoutRuntime.CancelRun(t.Context(), "another-run"); err == nil || !strings.Contains(err.Error(), "persist timed-out run "+run.ID) {
		t.Fatalf("CancelRun reconciliation error = %v", err)
	}
}

func TestWorkflowResumePersistenceFailuresRemainObservable(t *testing.T) {
	t.Run("completion save failure becomes a durable parent failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-complete-write-agent", Name: "Resume Complete Write", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume complete write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "resume-complete-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_resumed_completion_write
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.id = '`+parent.ID+`' AND NEW.status = '`+RunStatusCompleted+`'
			BEGIN SELECT RAISE(FAIL, 'resumed completion write rejected'); END
		`); err != nil {
			t.Fatalf("create completion trigger: %v", err)
		}

		updated, err := runtime.continueParentWorkflowAfterChild(t.Context(), Run{
			ID: "resume-complete-write-child", ParentRunID: parent.ID, Status: RunStatusCompleted, Message: "child complete",
		})
		if err != nil || updated == nil || updated.Status != RunStatusFailed || !strings.Contains(updated.FailureReason, "resumed completion write rejected") {
			t.Fatalf("continueParentWorkflowAfterChild = %+v err=%v", updated, err)
		}
	})

	t.Run("completion and failure writes both failing return the storage error", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-double-write-agent", Name: "Resume Double Write", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume double write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "resume-double-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_resumed_terminal_writes
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.id = '`+parent.ID+`' AND NEW.status IN ('`+RunStatusCompleted+`', '`+RunStatusFailed+`')
			BEGIN SELECT RAISE(FAIL, 'resumed terminal write rejected'); END
		`); err != nil {
			t.Fatalf("create terminal trigger: %v", err)
		}

		updated, err := runtime.continueParentWorkflowAfterChild(t.Context(), Run{
			ID: "resume-double-write-child", ParentRunID: parent.ID, Status: RunStatusCompleted, Message: "child complete",
		})
		if err == nil || updated != nil || !strings.Contains(err.Error(), "resumed terminal write rejected") {
			t.Fatalf("continueParentWorkflowAfterChild = %+v err=%v", updated, err)
		}
	})
}

func TestWorkflowBlockerAndRuntimeInitializationFailureSemantics(t *testing.T) {
	originalBuiltinSpecs := skillsruntime.BuiltinSkillSpecs
	skillsruntime.BuiltinSkillSpecs = []skillsruntime.BuiltinSkillSpec{{Name: "startup-failure", BuildBundle: func() (map[string]string, error) {
		return nil, errors.New("builtin skill initialization rejected")
	}}}
	t.Cleanup(func() { skillsruntime.BuiltinSkillSpecs = originalBuiltinSpecs })
	root := t.TempDir()
	store, err := NewStore(
		filepath.Join(root, "adk.db"), filepath.Join(root, "secrets", "adk.json"), filepath.Join(root, "skills"),
	)
	if err == nil || store != nil || !strings.Contains(err.Error(), "builtin skill initialization rejected") {
		t.Fatalf("NewStore = store:%v err:%v", store, err)
	}
}
