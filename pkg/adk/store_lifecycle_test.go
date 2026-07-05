package adk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestDeleteProviderFailsWhenReferencedByAgent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:          "openai",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     "openai",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	err := runtime.Store().DeleteProvider(ctx, "openai")
	if err == nil || !strings.Contains(err.Error(), "used by agent") {
		t.Fatalf("DeleteProvider error = %v, want used by agent", err)
	}
}

func TestProvidersMaintainDefaultSelectionAndCreatedOrder(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if err := runtime.Store().DeleteProvider(ctx, testProviderID); err != nil {
		t.Fatalf("DeleteProvider test provider: %v", err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-older", DisplayName: "Older", APIKey: "sk-older", Enabled: true,
	})
	time.Sleep(10 * time.Millisecond)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-newer", DisplayName: "Newer", APIKey: "sk-newer", Enabled: true,
	})

	providers, err := runtime.Store().ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) < 2 {
		t.Fatalf("providers len = %d, want at least 2", len(providers))
	}
	if providers[0].ID != "provider-older" || providers[1].ID != "provider-newer" {
		t.Fatalf("provider order = [%s %s], want [provider-older provider-newer]", providers[0].ID, providers[1].ID)
	}
	if !providers[0].Default || providers[1].Default {
		t.Fatalf("provider defaults = %+v, want older default only", providers[:2])
	}
	if !providers[0].HasAPIKey || !providers[1].HasAPIKey {
		t.Fatalf("providers api key visibility = %+v, want both true", providers[:2])
	}

	updatedDefault, err := runtime.Store().SetDefaultProvider(ctx, "provider-newer")
	if err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}
	if !updatedDefault.Default {
		t.Fatalf("updated default = %+v, want default", updatedDefault)
	}
	providers, err = runtime.Store().ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders after default: %v", err)
	}
	if providers[0].ID != "provider-newer" || !providers[0].Default || providers[1].Default {
		t.Fatalf("provider order/default after set = %+v, want newer default first", providers[:2])
	}
	if err := runtime.Store().DeleteProvider(ctx, "provider-newer"); err != nil {
		t.Fatalf("DeleteProvider default: %v", err)
	}
	providers, err = runtime.Store().ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders after delete default: %v", err)
	}
	if len(providers) == 0 || providers[0].ID != "provider-older" || !providers[0].Default {
		t.Fatalf("providers after default delete = %+v, want older promoted", providers)
	}

	if err := runtime.Store().DeleteProvider(ctx, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteProvider blank error = %v, want os.ErrNotExist", err)
	}
	if err := runtime.Store().DeleteProvider(ctx, "provider-missing"); err != nil {
		t.Fatalf("DeleteProvider missing = %v, want nil", err)
	}
}

func TestDeleteSessionRemovesApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent", "test")
	run := mustSaveRun(t, runtime, Run{ID: "run-test", SessionID: session.ID, AgentID: "agent", Status: RunStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()})
	approval := Approval{ID: "approval-test", RunID: run.ID, AgentID: "agent", ToolName: "strategy.save_draft", Status: ApprovalStatusPending}
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "task-test", Title: "test", Status: "TODO", AgentID: "agent", RunID: run.ID})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	appendADKEvent(t, runtime, "agent", session.ID, newAssistantEvent(run.ID, []*genai.Part{{Text: "done"}}, time.Unix(40, 0)))

	if err := runtime.Store().DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, err := runtime.Store().Approval(ctx, approval.ID); err != nil || ok {
		t.Fatalf("approval still exists: ok=%v err=%v", ok, err)
	}
	if _, ok, err := runtime.Store().Run(ctx, run.ID); err != nil || ok {
		t.Fatalf("run still exists: ok=%v err=%v", ok, err)
	}
	if _, ok, err := runtime.Store().Task(ctx, task.ID); err != nil || ok {
		t.Fatalf("task still exists: ok=%v err=%v", ok, err)
	}
	messages := mustMessages(t, runtime, session.ID)
	if len(messages) != 0 {
		t.Fatalf("messages = %+v, want empty after deleting session", messages)
	}
	if _, ok, err := runtime.Store().Session(ctx, session.ID); err != nil || ok {
		t.Fatalf("session still exists: ok=%v err=%v", ok, err)
	}
}

func TestSaveRunDoesNotRegressTerminalLifecycle(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	cancelledAt := nowString()
	terminal := mustSaveRun(t, runtime, Run{
		ID: "run-terminal-monotonic", SessionID: "session-terminal-monotonic", AgentID: "agent",
		Status: RunStatusCancelled, Message: "cancelled", ErrorCode: "RUN_CANCELLED",
		CreatedAt: now, UpdatedAt: now, CompletedAt: &cancelledAt, CancelledAt: &cancelledAt,
	})
	stale := terminal
	stale.Status = RunStatusRunning
	stale.Message = "stale running snapshot"
	stale.ErrorCode = ""
	stale.CompletedAt = nil
	stale.CancelledAt = nil
	if err := runtime.Store().SaveRun(ctx, stale); err != nil {
		t.Fatalf("SaveRun stale snapshot: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, terminal.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusCancelled || stored.ErrorCode != "RUN_CANCELLED" || stored.CancelledAt == nil {
		t.Fatalf("terminal run regressed = %+v", stored)
	}

	failedAt := nowString()
	failed := mustSaveRun(t, runtime, Run{
		ID: "run-terminal-different-terminal", SessionID: "session-terminal-monotonic", AgentID: "agent",
		Status: RunStatusFailed, Message: "failed first", ErrorCode: "PROVIDER_ERROR",
		CreatedAt: now, UpdatedAt: now, CompletedAt: &failedAt,
	})
	laterCancelled := failed
	laterCancelled.Status = RunStatusCancelled
	laterCancelled.Message = "stale cancellation"
	laterCancelled.ErrorCode = "RUN_CANCELLED"
	laterCancelled.CancelledAt = &failedAt
	if err := runtime.Store().SaveRun(ctx, laterCancelled); err != nil {
		t.Fatalf("SaveRun different terminal snapshot: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, failed.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusFailed || stored.ErrorCode != "PROVIDER_ERROR" || stored.CancelledAt != nil {
		t.Fatalf("terminal run changed to a different terminal state = %+v", stored)
	}

	cancelledWithoutFinal := mustSaveRun(t, runtime, Run{
		ID: "run-terminal-final-message-enrichment", SessionID: "session-terminal-monotonic", AgentID: "agent",
		Status: RunStatusCancelled, Message: "cancelled", ErrorCode: "RUN_CANCELLED",
		CreatedAt: now, UpdatedAt: now, CompletedAt: &cancelledAt, CancelledAt: &cancelledAt,
	})
	enriched := cancelledWithoutFinal
	enriched.FinalMessageID = "message-final-after-cancel"
	if err := runtime.Store().SaveRun(ctx, enriched); err != nil {
		t.Fatalf("SaveRun cancelled final message enrichment: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, cancelledWithoutFinal.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusCancelled || stored.FinalMessageID != "message-final-after-cancel" {
		t.Fatalf("cancelled final message enrichment = %+v", stored)
	}

	completedRunningWorkflow := mustSaveRun(t, runtime, Run{
		ID: "run-terminal-completed-running-workflow", SessionID: "session-terminal-monotonic", AgentID: "agent",
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Message: "completed intermediate workflow state", CreatedAt: now, UpdatedAt: now, CompletedAt: &failedAt,
	})
	staleRunningWorkflow := completedRunningWorkflow
	staleRunningWorkflow.Status = RunStatusRunning
	staleRunningWorkflow.Message = "stale workflow running snapshot"
	staleRunningWorkflow.CompletedAt = nil
	if err := runtime.Store().SaveRun(ctx, staleRunningWorkflow); err != nil {
		t.Fatalf("SaveRun stale completed-running workflow snapshot: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, completedRunningWorkflow.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusCompleted || stored.WorkflowStatus != workflowStatusRunning || stored.CompletedAt == nil {
		t.Fatalf("completed workflow regressed to stale running snapshot = %+v", stored)
	}
	failedAfterIntermediate := completedRunningWorkflow
	failedAfterIntermediate.Status = RunStatusFailed
	failedAfterIntermediate.WorkflowStatus = workflowStatusFailed
	failedAfterIntermediate.Message = "workflow max iterations exceeded"
	failedAfterIntermediate.ErrorCode = "WORKFLOW_GOAL_MAX_ITERATIONS_EXCEEDED"
	failedAfterIntermediate.CompletedAt = &failedAt
	if err := runtime.Store().SaveRun(ctx, failedAfterIntermediate); err != nil {
		t.Fatalf("SaveRun terminal correction after completed-running workflow: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, completedRunningWorkflow.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusFailed || stored.ErrorCode != "WORKFLOW_GOAL_MAX_ITERATIONS_EXCEEDED" {
		t.Fatalf("completed-running workflow did not accept terminal correction = %+v", stored)
	}
}

func TestSaveRunReopensCompletedRunForFreshPendingApproval(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	completedAt := nowString()
	run := mustSaveRun(t, runtime, Run{
		ID: "run-reopen-fresh-approval", SessionID: "session-reopen-fresh-approval", AgentID: "agent",
		Status: RunStatusCompleted, CompletedAt: &completedAt, CreatedAt: now, UpdatedAt: now,
	})
	approval := Approval{
		ID: "approval-reopen-fresh", RunID: run.ID, AgentID: run.AgentID,
		ToolName: "strategy.research_backtest", Status: ApprovalStatusPending,
		FunctionCallID: "function-reopen-fresh", ConfirmationCallID: "confirmation-reopen-fresh",
	}
	reopened := run
	reopened.Status = RunStatusPending
	reopened.CompletedAt = nil
	reopened.ResumeState = "waiting_approval"
	reopened.PendingApprovals = []Approval{approval}
	if err := runtime.Store().SaveRun(ctx, reopened); err != nil {
		t.Fatalf("SaveRun reopened approval: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPending || stored.CompletedAt != nil || len(stored.PendingApprovals) != 1 {
		t.Fatalf("reopened run = %+v", stored)
	}
}

func TestSaveRunAllowsPausedWorkflowLifecycleUpdates(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-paused-workflow-updatable", SessionID: "session-paused-workflow-updatable", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
		Message: "waiting for child approval", CreatedAt: now, UpdatedAt: now,
	})
	updated := parent
	updated.Message = "workflow resumed after approval"
	updated.WorkflowStatus = workflowStatusRunning
	updated.Iteration = 2
	if err := runtime.Store().SaveRun(ctx, updated); err != nil {
		t.Fatalf("SaveRun paused workflow update: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusRunning || stored.WorkflowStatus != workflowStatusRunning || stored.Message != updated.Message || stored.Iteration != 2 {
		t.Fatalf("paused workflow update was blocked = %+v", stored)
	}

	pausedAgain := stored
	pausedAgain.WorkflowStatus = workflowStatusPaused
	if err := runtime.Store().SaveRun(ctx, pausedAgain); err != nil {
		t.Fatalf("SaveRun pause workflow again: %v", err)
	}
	completedAt := nowString()
	completed := pausedAgain
	completed.Status = RunStatusCompleted
	completed.WorkflowStatus = workflowStatusComplete
	completed.Message = "workflow completed after approval"
	completed.CompletedAt = &completedAt
	if err := runtime.Store().SaveRun(ctx, completed); err != nil {
		t.Fatalf("SaveRun terminal update from paused workflow: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup after terminal update ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusCompleted || stored.WorkflowStatus != workflowStatusComplete || stored.Message != completed.Message {
		t.Fatalf("terminal update from paused workflow was blocked = %+v", stored)
	}
}

func TestSaveRunPreservesUserGoalPauseLifecycle(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()

	t.Run("keeps pause request when stale running snapshot clears it", func(t *testing.T) {
		initial := Run{
			ID:               "run-goal-pause-requested",
			SessionID:        "session-1",
			AgentID:          "agent-1",
			Status:           RunStatusRunning,
			Message:          "目标将在当前轮结束后暂停。",
			WorkMode:         WorkModeLoop,
			Objective:        "推进目标",
			WorkflowStatus:   workflowStatusRunning,
			PauseRequestedAt: &now,
			ResumeState:      "user_pause_requested",
			CreatedAt:        now,
			StartedAt:        now,
			UpdatedAt:        now,
			ToolCalls:        []ToolCall{},
			PendingApprovals: []Approval{},
			Usage:            &RunUsage{},
		}
		if err := runtime.Store().SaveRun(ctx, initial); err != nil {
			t.Fatalf("SaveRun initial: %v", err)
		}

		stale := initial
		stale.Message = "goal running"
		stale.PauseRequestedAt = nil
		stale.ResumeState = ""
		if err := runtime.Store().SaveRun(ctx, stale); err != nil {
			t.Fatalf("SaveRun stale: %v", err)
		}

		stored, ok, err := runtime.Store().Run(ctx, initial.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup ok=%v err=%v", ok, err)
		}
		if stored.PauseRequestedAt == nil || stored.ResumeState != "user_pause_requested" {
			t.Fatalf("stored run = %+v, want pause request preserved", stored)
		}
	})

	t.Run("keeps user paused state when stale running snapshot overwrites it", func(t *testing.T) {
		initial := Run{
			ID:               "run-goal-user-paused",
			SessionID:        "session-1",
			AgentID:          "agent-1",
			Status:           RunStatusPaused,
			Message:          "目标已暂停。",
			WorkMode:         WorkModeLoop,
			Objective:        "推进目标",
			WorkflowStatus:   workflowStatusPaused,
			PauseRequestedAt: &now,
			PausedAt:         &now,
			PausedReason:     "user",
			ResumeState:      "user_paused",
			CreatedAt:        now,
			StartedAt:        now,
			UpdatedAt:        now,
			ToolCalls:        []ToolCall{},
			PendingApprovals: []Approval{},
			Usage:            &RunUsage{},
		}
		if err := runtime.Store().SaveRun(ctx, initial); err != nil {
			t.Fatalf("SaveRun initial paused: %v", err)
		}

		stale := initial
		stale.Status = RunStatusRunning
		stale.WorkflowStatus = workflowStatusRunning
		stale.Message = "goal running"
		stale.PauseRequestedAt = nil
		stale.PausedAt = nil
		stale.PausedReason = ""
		stale.ResumeState = ""
		if err := runtime.Store().SaveRun(ctx, stale); err != nil {
			t.Fatalf("SaveRun stale paused: %v", err)
		}

		stored, ok, err := runtime.Store().Run(ctx, initial.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup ok=%v err=%v", ok, err)
		}
		if stored.Status != RunStatusPaused || stored.WorkflowStatus != workflowStatusPaused || stored.PausedReason != "user" || stored.ResumeState != "user_paused" {
			t.Fatalf("stored run = %+v, want user pause preserved", stored)
		}
	})

	t.Run("allows an explicit resume snapshot to clear pause fields", func(t *testing.T) {
		initial := Run{
			ID:               "run-goal-user-resuming",
			SessionID:        "session-1",
			AgentID:          "agent-1",
			Status:           RunStatusPaused,
			Message:          "目标已暂停。",
			WorkMode:         WorkModeLoop,
			Objective:        "推进目标",
			WorkflowStatus:   workflowStatusPaused,
			PauseRequestedAt: &now,
			PausedAt:         &now,
			PausedReason:     "user",
			ResumeState:      "user_paused",
			CreatedAt:        now,
			StartedAt:        now,
			UpdatedAt:        now,
			ToolCalls:        []ToolCall{},
			PendingApprovals: []Approval{},
			Usage:            &RunUsage{},
		}
		if err := runtime.Store().SaveRun(ctx, initial); err != nil {
			t.Fatalf("SaveRun initial resuming: %v", err)
		}

		resuming := initial
		resuming.Status = RunStatusRunning
		resuming.WorkflowStatus = workflowStatusRunning
		resuming.Message = "goal resumed"
		resuming.PauseRequestedAt = nil
		resuming.PausedAt = nil
		resuming.PausedReason = ""
		resuming.ResumeState = "user_resuming"
		if err := runtime.Store().SaveRun(ctx, resuming); err != nil {
			t.Fatalf("SaveRun resuming: %v", err)
		}

		stored, ok, err := runtime.Store().Run(ctx, initial.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup ok=%v err=%v", ok, err)
		}
		if stored.Status != RunStatusRunning || stored.PauseRequestedAt != nil || stored.PausedAt != nil || stored.PausedReason != "" || stored.ResumeState != "user_resuming" {
			t.Fatalf("stored run = %+v, want explicit resume preserved", stored)
		}
	})
}

func TestListSessionsPageFiltersQueryAndPaginates(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	older := mustCreateSession(t, runtime, "agent-a", "Alpha Review")
	time.Sleep(10 * time.Millisecond)
	newer := mustCreateSession(t, runtime, "agent-a", "alpha Deep Dive")
	time.Sleep(10 * time.Millisecond)
	mustCreateSession(t, runtime, "agent-b", "Alpha Other Agent")
	time.Sleep(10 * time.Millisecond)
	mustCreateSession(t, runtime, "agent-a", "Gamma Notes")

	page, total, err := runtime.Store().ListSessionsPage(ctx, "agent-a", "ALPHA", 1, 0)
	if err != nil {
		t.Fatalf("ListSessionsPage: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(page) != 1 || page[0].ID != newer.ID {
		t.Fatalf("first page = %+v, want newest alpha session", page)
	}

	nextPage, total, err := runtime.Store().ListSessionsPage(ctx, "agent-a", "alpha", 1, 1)
	if err != nil {
		t.Fatalf("ListSessionsPage next: %v", err)
	}
	if total != 2 {
		t.Fatalf("next total = %d, want 2", total)
	}
	if len(nextPage) != 1 || nextPage[0].ID != older.ID {
		t.Fatalf("next page = %+v, want older alpha session", nextPage)
	}
}

func TestSessionComposerStatePersistsAndDeletesWithSession(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent-composer", "composer")

	state, ok, err := runtime.Store().SessionComposerState(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionComposerState empty: %v", err)
	}
	if ok || state.SessionID != session.ID || state.ChatDraft != "" {
		t.Fatalf("empty composer state = %+v ok=%v", state, ok)
	}

	saved, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{
		ChatDraft:              new(strings.Repeat("x", MaxMessageLength+20)),
		ProviderIDOverride:     new(" provider-session "),
		ModelOverride:          new(" model-session "),
		WorkModeOverride:       new(WorkModeLoop),
		PermissionModeOverride: new(PermissionModeLessApproval),
		GoalObjectiveDraft:     new("目标草稿"),
		GoalObjectiveTouched:   new(true),
	})
	if err != nil {
		t.Fatalf("SaveSessionComposerState: %v", err)
	}
	if len([]rune(saved.ChatDraft)) != MaxMessageLength || saved.ProviderIDOverride != "provider-session" || saved.ModelOverride != "model-session" || saved.WorkModeOverride != WorkModeLoop || saved.PermissionModeOverride != PermissionModeLessApproval || !saved.GoalObjectiveTouched {
		t.Fatalf("saved composer state = %+v", saved)
	}

	if _, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{WorkModeOverride: new("sequential")}); err == nil {
		t.Fatal("SaveSessionComposerState invalid mode err = nil")
	}
	if _, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{PermissionModeOverride: new("root")}); err == nil {
		t.Fatal("SaveSessionComposerState invalid permission mode err = nil")
	}

	if err := runtime.Store().DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	deleted, ok, err := runtime.Store().SessionComposerState(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionComposerState after delete: %v", err)
	}
	if ok || deleted.ChatDraft != "" {
		t.Fatalf("deleted composer state = %+v ok=%v, want empty", deleted, ok)
	}
}

func TestDeleteSessionMissingAndBlankAreNotFound(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if err := runtime.Store().DeleteSession(ctx, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSession blank error = %v, want os.ErrNotExist", err)
	}
	if err := runtime.Store().DeleteSession(ctx, "session-missing"); err != nil {
		t.Fatalf("DeleteSession missing = %v, want nil for idempotent delete", err)
	}
}

func TestListApprovalsPageFiltersAndSortsNewestFirst(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	approvals := []Approval{
		{
			ID: "approval-older", RunID: "run-a", AgentID: "agent-a", ToolName: "strategy.save_draft",
			Status: ApprovalStatusPending, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
		},
		{
			ID: "approval-newer", RunID: "run-b", AgentID: "agent-a", ToolName: "strategy.optimize",
			Status: ApprovalStatusPending, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-02T00:00:00Z",
		},
		{
			ID: "approval-other-agent", RunID: "run-c", AgentID: "agent-b", ToolName: "strategy.save_draft",
			Status: ApprovalStatusPending, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-03T00:00:00Z",
		},
		{
			ID: "approval-other-status", RunID: "run-d", AgentID: "agent-a", ToolName: "strategy.save_draft",
			Status: ApprovalStatusApproved, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-04T00:00:00Z",
		},
	}
	for _, approval := range approvals {
		if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval(%s): %v", approval.ID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	page, total, err := runtime.Store().ListApprovalsPage(ctx, ApprovalStatusPending, "agent-a", 10, 0)
	if err != nil {
		t.Fatalf("ListApprovalsPage: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2", len(page))
	}
	if page[0].ID != "approval-newer" || page[1].ID != "approval-older" {
		t.Fatalf("page order = [%s %s], want [approval-newer approval-older]", page[0].ID, page[1].ID)
	}
}

func TestListOptimizationTasksSortsByUpdatedAtDesc(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if _, err := runtime.Store().SaveOptimizationTask(ctx, OptimizationTask{
		ID:        "opt-older",
		Objective: "older",
		CreatedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveOptimizationTask first: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := runtime.Store().SaveOptimizationTask(ctx, OptimizationTask{
		ID:        "opt-newer",
		Objective: "newer",
		CreatedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveOptimizationTask second: %v", err)
	}

	tasks, err := runtime.Store().ListOptimizationTasks(ctx)
	if err != nil {
		t.Fatalf("ListOptimizationTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2", len(tasks))
	}
	if tasks[0].ID != "opt-newer" || tasks[1].ID != "opt-older" {
		t.Fatalf("tasks order = [%s %s], want [opt-newer opt-older]", tasks[0].ID, tasks[1].ID)
	}
}

func TestRecentOpenAIMessagesKeepsLatestConversation(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
		{Role: "user", Content: "third"},
	}
	history := recentOpenAIMessages(messages, 2, 100)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Role != "assistant" || history[0].Content != "second" || history[1].Content != "third" {
		t.Fatalf("history = %+v, want latest assistant/user pair", history)
	}
}

func TestOpenAIToolsFromDescriptorsIncludesSchemaAndRisk(t *testing.T) {
	tools := openAIToolsFromDescriptors([]ToolDescriptor{{
		Name:          "market.snapshot",
		DisplayName:   "Snapshot",
		Description:   "read snapshot",
		Permission:    "read_internal",
		OutputSummary: "snapshot output",
		RiskLevel:     "low",
	}})
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	if tools[0].Function.Name != "market-snapshot" {
		t.Fatalf("tool name = %q", tools[0].Function.Name)
	}
	if tools[0].Function.Parameters["type"] != "object" {
		t.Fatalf("tool parameters = %#v, want object schema", tools[0].Function.Parameters)
	}
	if !strings.Contains(tools[0].Function.Description, "Risk: low") {
		t.Fatalf("tool description = %q, want risk annotation", tools[0].Function.Description)
	}
}

func TestExecuteToolTagInvokesCanonicalToolWithParameters(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { jftradeErr2 := store.Close(); jftradeCheckTestError(t, jftradeErr2) })
	registry := NewToolRegistry()
	var received map[string]any
	registry.Register(ToolDescriptor{
		Name:        "portfolio.summary",
		DisplayName: "组合摘要",
		Description: "test portfolio summary",
		Category:    "portfolio",
		Permission:  "read_internal",
	}, func(_ context.Context, input map[string]any) (any, error) {
		received = input
		return map[string]any{"ok": true}, nil
	})
	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     testProviderID,
		Tools:          []string{"portfolio.summary"},
		PermissionMode: PermissionModeLessApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="jftrade portfolio summary" parameters='{"showDetails": true, "showPositions": true}' />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.Run.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(response.Run.ToolCalls))
	}
	call := response.Run.ToolCalls[0]
	if call.ToolName != "portfolio.summary" || call.Status != "SUCCEEDED" {
		t.Fatalf("tool call = %+v, want successful portfolio.summary", call)
	}
	if received["showDetails"] != true || received["showPositions"] != true {
		t.Fatalf("received input = %#v, want parsed boolean parameters", received)
	}
}

func TestRejectUnsafeHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "10.0.0.1", "169.254.169.254", "::1"} {
		t.Run(host, func(t *testing.T) {
			if err := rejectUnsafeHost(context.Background(), host); err == nil {
				t.Fatalf("rejectUnsafeHost(%q) = nil, want error", host)
			}
		})
	}
}

func TestInternalSkillCannotBeUninstalled(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skill, ok, err := runtime.Skills().Get(ctx, "jftrade-market")
	if err != nil {
		t.Fatalf("Get builtin skill: %v", err)
	}
	if !ok {
		t.Fatal("builtin skill jftrade-market not found")
	}

	err = runtime.Skills().Uninstall(ctx, skill.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot be uninstalled") {
		t.Fatalf("Uninstall internal skill error = %v, want cannot be uninstalled", err)
	}
	stored, ok, err := runtime.Skills().Get(ctx, skill.ID)
	if err != nil {
		t.Fatalf("Get builtin skill: %v", err)
	}
	if !ok || stored.Source != "builtin" {
		t.Fatalf("internal skill was changed or removed: ok=%v skill=%+v", ok, stored)
	}
}

func TestExternalSkillUninstallRemovesInstallDir(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	installDir := filepath.Join(runtime.Store().SkillsPath(), "external-skill")
	installPath := filepath.Join(installDir, "SKILL.md")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(installPath, []byte("---\nname: external-skill\ndescription: external test skill\nmetadata:\n  source: https://example.com/SKILL.md\n---\nAlways cite external sources.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	skill, ok, err := runtime.Skills().Get(ctx, "external-skill")
	if err != nil {
		t.Fatalf("Get external skill: %v", err)
	}
	if !ok {
		t.Fatal("external skill not discovered")
	}

	if err := runtime.Skills().Uninstall(ctx, skill.ID); err != nil {
		t.Fatalf("Uninstall external skill: %v", err)
	}
	if _, ok, err := runtime.Skills().Get(ctx, skill.ID); err != nil || ok {
		t.Fatalf("external skill still exists: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("install dir stat error = %v, want not exist", err)
	}
}

func TestPreparedAgentLoadsOnlyEnabledBoundSkillsAndTools(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), "research")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: research\ndescription: external research skill\nmetadata:\n  source: test\n  version: 2\nallowed-tools: [http.fetch]\n---\nAlways cite external sources.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	agent := Agent{
		ID: "agent", Name: "Agent", Instruction: "Base instruction.",
		Tools: []string{"http.fetch", "system.status"}, Skills: []string{"research"},
	}
	prepared, err := runtime.prepareAgent(ctx, agent)
	if err != nil {
		t.Fatalf("prepareAgent: %v", err)
	}
	if prepared.Instruction != agent.Instruction {
		t.Fatalf("prepared instruction = %q, want original instruction", prepared.Instruction)
	}
	if len(prepared.Tools) != len(agent.Tools) {
		t.Fatalf("prepared tools = %#v, want original tools %#v", prepared.Tools, agent.Tools)
	}
}

func TestSkillRegistryReportsMetadataAndAllowedTools(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	research, ok, err := runtime.Skills().Get(ctx, strategypinespec.ResearchBuiltinSkillName)
	if err != nil {
		t.Fatalf("Get builtin research skill: %v", err)
	}
	if !ok {
		t.Fatalf("builtin skill %s not found", strategypinespec.ResearchBuiltinSkillName)
	}
	if !research.Builtin || research.Source != "builtin" || research.ValidationStatus != "VALID" || research.ContentHash == "" {
		t.Fatalf("research skill metadata = %+v", research)
	}
	if research.Version != strategypinespec.BuiltinSkillVersion {
		t.Fatalf("research skill version = %q, want %q", research.Version, strategypinespec.BuiltinSkillVersion)
	}
	for _, toolName := range []string{
		strategypinespec.ToolName,
		"strategy.validate_pine",
		"strategy.research_backtest",
		"backtest.runs",
		"backtest.result_view",
		"workflow.wait",
		"market.snapshot",
		"market.candles",
	} {
		if !containsString(research.Tools, toolName) {
			t.Fatalf("research skill tools = %+v, want %s", research.Tools, toolName)
		}
	}
	for _, forbidden := range []string{"strategy.save_draft", "strategy.save_definition", "strategy.optimize"} {
		if containsString(research.Tools, forbidden) {
			t.Fatalf("research skill unexpectedly exposes %s: %+v", forbidden, research.Tools)
		}
	}
	publish, ok, err := runtime.Skills().Get(ctx, strategypinespec.PublishBuiltinSkillName)
	if err != nil {
		t.Fatalf("Get builtin publish skill: %v", err)
	}
	if !ok {
		t.Fatalf("builtin skill %s not found", strategypinespec.PublishBuiltinSkillName)
	}
	if !publish.Builtin || publish.Source != "builtin" || publish.ValidationStatus != "VALID" || publish.ContentHash == "" {
		t.Fatalf("publish skill metadata = %+v", publish)
	}
	for _, toolName := range []string{"strategy.validate_pine", "strategy.save_draft", "strategy.save_definition", "strategy.update_instance_mode", "strategy.optimize", "backtest.runs"} {
		if !containsString(publish.Tools, toolName) {
			t.Fatalf("publish skill tools = %+v, want %s", publish.Tools, toolName)
		}
	}
	if containsString(publish.Tools, "strategy.research_backtest") {
		t.Fatalf("publish skill unexpectedly exposes research_backtest: %+v", publish.Tools)
	}
	if _, ok, err := runtime.Skills().Get(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy strategy skill ok=%v err=%v, want absent", ok, err)
	}
	for _, item := range []struct {
		skillName string
		resource  string
	}{
		{strategypinespec.ResearchBuiltinSkillName, "references/pine-v6-spec.md"},
		{strategypinespec.ResearchBuiltinSkillName, "references/pine-v6-examples.md"},
		{strategypinespec.ResearchBuiltinSkillName, "references/pine-v6-cheatsheet.md"},
		{strategypinespec.ResearchBuiltinSkillName, "references/strategy-research-workflow.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/pine-v6-spec.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/pine-v6-examples.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/pine-v6-cheatsheet.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/strategy-publish-checklist.md"},
	} {
		if _, err := os.Stat(filepath.Join(runtime.Store().SkillsPath(), item.skillName, item.resource)); err != nil {
			t.Fatalf("resource %s/%s stat: %v", item.skillName, item.resource, err)
		}
	}
}
