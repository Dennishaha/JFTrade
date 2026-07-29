package adk

import (
	"context"
	"strings"
)

func (r *Runtime) saveRunPreservingUserGoalPause(ctx context.Context, run Run) (Run, error) {
	if r == nil || r.store == nil || !isRootLoopGoalRun(run) {
		if r != nil && r.store != nil {
			return run, r.store.SaveRun(ctx, run)
		}
		return run, nil
	}
	prepared, err := r.store.prepareRunForSave(ctx, run)
	if err != nil {
		return Run{}, err
	}
	if err := r.store.savePreparedRun(ctx, prepared); err != nil {
		return Run{}, err
	}
	return prepared, nil
}

func preserveUserGoalPauseLifecycle(latest Run, candidate Run) Run {
	if latest.ID == "" || latest.ID != candidate.ID {
		return candidate
	}
	if !isRootLoopGoalRun(latest) || !isRootLoopGoalRun(candidate) {
		return candidate
	}
	if candidate.ResumeState == "user_resuming" {
		return candidate
	}
	if userPausedGoalParent(latest) && candidate.Status != RunStatusCancelled {
		candidate.Status = RunStatusPaused
		candidate.WorkflowStatus = workflowStatusPaused
		candidate.PauseRequestedAt = latest.PauseRequestedAt
		candidate.PausedAt = latest.PausedAt
		candidate.PausedReason = latest.PausedReason
		candidate.ResumeState = defaultString(latest.ResumeState, "user_paused")
		candidate.Message = defaultString(latest.Message, "目标已暂停。")
		candidate.FailureReason = latest.FailureReason
		candidate.ErrorCode = latest.ErrorCode
		candidate.CompletedAt = latest.CompletedAt
		candidate.CancelledAt = latest.CancelledAt
		return candidate
	}
	if latest.PauseRequestedAt != nil && candidate.PauseRequestedAt == nil && !isTerminalLifecycleRunStatus(candidate.Status) {
		candidate.PauseRequestedAt = latest.PauseRequestedAt
		if strings.TrimSpace(candidate.ResumeState) == "" {
			candidate.ResumeState = defaultString(latest.ResumeState, "user_pause_requested")
		}
	}
	return candidate
}

func isRootLoopGoalRun(run Run) bool {
	return strings.TrimSpace(run.ID) != "" &&
		strings.TrimSpace(run.ParentRunID) == "" &&
		normalizeWorkMode(run.WorkMode) == WorkModeLoop
}
