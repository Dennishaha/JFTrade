package adk

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestStoreGeneratedIdentityAndDefaultBoundaries(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	generatedProvider, err := store.SaveProvider(ctx, ProviderWriteRequest{Enabled: true})
	if err != nil {
		t.Fatalf("SaveProvider generated: %v", err)
	}
	if !strings.HasPrefix(generatedProvider.ID, "provider-") || generatedProvider.DisplayName != generatedProvider.ID || generatedProvider.BaseURL != "https://api.openai.com/v1" || generatedProvider.Model != "gpt-4o-mini" || !generatedProvider.Default {
		t.Fatalf("generated provider = %+v", generatedProvider)
	}
	if _, err := store.SetDefaultProvider(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SetDefaultProvider blank err = %v", err)
	}
	if err := store.DeleteProvider(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteProvider blank err = %v", err)
	}

	generatedSkill, err := store.SaveSkill(ctx, Skill{})
	if err != nil {
		t.Fatalf("SaveSkill generated: %v", err)
	}
	if !strings.HasPrefix(generatedSkill.ID, "skill-") || generatedSkill.CreatedAt == "" || generatedSkill.UpdatedAt == "" {
		t.Fatalf("generated skill = %+v", generatedSkill)
	}
	renamedSkill := generatedSkill
	renamedSkill.DisplayName = "Generated Skill"
	renamedSkill.CreatedAt = ""
	updatedSkill, err := store.SaveSkill(ctx, renamedSkill)
	if err != nil {
		t.Fatalf("SaveSkill update: %v", err)
	}
	if updatedSkill.CreatedAt != generatedSkill.CreatedAt || updatedSkill.DisplayName != "Generated Skill" {
		t.Fatalf("updated skill = %+v, original createdAt=%s", updatedSkill, generatedSkill.CreatedAt)
	}
}

func TestStoreTaskAndOptimizationUpdatesPreserveCreationTime(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	if _, err := store.UpdateTask(ctx, " ", TaskPatchRequest{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UpdateTask blank err = %v", err)
	}
	task, err := store.SaveTask(ctx, TaskWriteRequest{Title: "Initial task", Status: "TODO"})
	if err != nil {
		t.Fatalf("SaveTask generated: %v", err)
	}
	if !strings.HasPrefix(task.ID, "task-") || task.Status != "TODO" {
		t.Fatalf("generated task = %+v", task)
	}
	updatedTask, err := store.SaveTask(ctx, TaskWriteRequest{ID: task.ID, Title: "Updated task", Status: "IN_PROGRESS"})
	if err != nil {
		t.Fatalf("SaveTask update: %v", err)
	}
	if updatedTask.CreatedAt != task.CreatedAt || updatedTask.Title != "Updated task" || updatedTask.Status != "IN_PROGRESS" {
		t.Fatalf("updated task = %+v, original createdAt=%s", updatedTask, task.CreatedAt)
	}

	generatedOpt, err := store.SaveOptimizationTask(ctx, OptimizationTask{Status: "queued", Objective: "find better params"})
	if err != nil {
		t.Fatalf("SaveOptimizationTask generated: %v", err)
	}
	if !strings.HasPrefix(generatedOpt.ID, "opt-") {
		t.Fatalf("generated optimization = %+v", generatedOpt)
	}
	updatedOpt, err := store.SaveOptimizationTask(ctx, OptimizationTask{ID: generatedOpt.ID, Status: "running", Objective: "find better params"})
	if err != nil {
		t.Fatalf("SaveOptimizationTask update: %v", err)
	}
	if updatedOpt.CreatedAt != generatedOpt.CreatedAt || updatedOpt.Status != "running" {
		t.Fatalf("updated optimization = %+v, original createdAt=%s", updatedOpt, generatedOpt.CreatedAt)
	}
}

func TestStoreDeleteSessionCascadesRuntimeState(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	if err := store.DeleteSession(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSession blank err = %v", err)
	}
	session, err := store.CreateSession(ctx, "agent-default", "cleanup")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := Run{ID: "run-cleanup", SessionID: session.ID, AgentID: "agent-default", Status: RunStatusRunning}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if err := store.SaveApproval(ctx, Approval{ID: "approval-cleanup", RunID: run.ID, AgentID: "agent-default", Status: ApprovalStatusPending}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	if _, err := store.SaveTask(ctx, TaskWriteRequest{ID: "task-cleanup", Title: "Cleanup task", RunID: run.ID}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if _, err := store.SaveSessionContext(ctx, SessionContextState{SessionID: session.ID, CurrentInputTokens: 1}); err != nil {
		t.Fatalf("SaveSessionContext: %v", err)
	}
	if _, err := store.SaveHandoffSegment(ctx, HandoffSegment{SessionID: session.ID, Sequence: 1, Summary: "handoff"}); err != nil {
		t.Fatalf("SaveHandoffSegment: %v", err)
	}
	if _, err := store.SaveSessionNotice(ctx, TimelineEntry{SessionID: session.ID, RunID: run.ID, Kind: TimelineKindContextNotice, Status: TimelineStatusFinal, Text: "notice"}); err != nil {
		t.Fatalf("SaveSessionNotice: %v", err)
	}
	if _, err := store.SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{ChatDraft: new("draft")}); err != nil {
		t.Fatalf("SaveSessionComposerState: %v", err)
	}

	if err := store.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, err := store.Session(ctx, session.ID); err != nil || ok {
		t.Fatalf("Session after delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Run(ctx, run.ID); err != nil || ok {
		t.Fatalf("Run after session delete ok=%v err=%v", ok, err)
	}
	if approvals, err := store.ListApprovals(ctx); err != nil || len(approvals) != 0 {
		t.Fatalf("approvals after session delete = %+v err=%v", approvals, err)
	}
	if tasks, _, err := store.ListTasksPage(ctx, "", "", run.ID, 20, 0); err != nil || len(tasks) != 0 {
		t.Fatalf("tasks after session delete = %+v err=%v", tasks, err)
	}
	if _, ok, err := store.SessionContext(ctx, session.ID); err != nil || ok {
		t.Fatalf("SessionContext after session delete ok=%v err=%v", ok, err)
	}
}
