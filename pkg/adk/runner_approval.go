package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r *Runtime) ResolveApproval(ctx context.Context, approvalID string, approved bool) (ApprovalResolution, error) {
	status := ApprovalStatusDenied
	if approved {
		status = ApprovalStatusApproved
	}
	approval, changed, err := r.store.ResolvePendingApproval(ctx, approvalID, status)
	if err != nil {
		return ApprovalResolution{}, err
	}
	if !changed {
		if approval.ID != "" && approval.Status == status {
			return r.continueResolvedApproval(ctx, approval, approved)
		}
		return ApprovalResolution{Approval: approval}, nil
	}
	r.audit(ctx, "approval.resolved", approval.ID, "Agent approval resolved.", map[string]any{
		"runId": approval.RunID, "toolName": approval.ToolName, "approved": approved,
	})
	return r.continueResolvedApproval(ctx, approval, approved)
}

func (r *Runtime) ResolveApprovalAsync(ctx context.Context, approvalID string, approved bool) (ApprovalResolution, error) {
	status := ApprovalStatusDenied
	if approved {
		status = ApprovalStatusApproved
	}
	approval, changed, err := r.store.ResolvePendingApproval(ctx, approvalID, status)
	if err != nil {
		return ApprovalResolution{}, err
	}
	if !changed {
		if approval.ID == "" || approval.Status != status {
			return ApprovalResolution{Approval: approval}, nil
		}
	} else {
		r.audit(ctx, "approval.resolved", approval.ID, "Agent approval resolved.", map[string]any{
			"runId": approval.RunID, "toolName": approval.ToolName, "approved": approved,
		})
	}
	resolution, shouldContinue, err := r.stageResolvedApproval(ctx, approval, approved)
	if err != nil {
		return ApprovalResolution{}, err
	}
	if shouldContinue {
		r.enqueueResolvedApprovalContinuation(approval.RunID)
	}
	return resolution, nil
}

func (r *Runtime) stageResolvedApproval(ctx context.Context, approval Approval, approved bool) (ApprovalResolution, bool, error) {
	run, ok, err := r.store.Run(ctx, approval.RunID)
	if err != nil || !ok {
		return ApprovalResolution{Approval: approval}, false, err
	}
	if run.Status != RunStatusPending {
		return ApprovalResolution{Approval: approval}, false, nil
	}
	replacedApproval := false
	for index := range run.PendingApprovals {
		if run.PendingApprovals[index].ID == approval.ID {
			run.PendingApprovals[index] = approval
			replacedApproval = true
		}
	}
	if !replacedApproval {
		return ApprovalResolution{Approval: approval, Run: &run}, false, nil
	}
	if !approved {
		for index := range run.PendingApprovals {
			item := &run.PendingApprovals[index]
			if item.Status != ApprovalStatusPending {
				continue
			}
			resolved, changed, resolveErr := r.store.ResolvePendingApproval(ctx, item.ID, ApprovalStatusDenied)
			if resolveErr == nil && changed {
				*item = resolved
			}
		}
		for index := range run.ToolCalls {
			call := &run.ToolCalls[index]
			if call.Status == "PENDING_APPROVAL" {
				call.Status = "DENIED"
				call.RequiresUser = false
				finishToolCall(call)
			}
		}
	}
	if runHasPendingApproval(run.PendingApprovals) {
		if err := r.store.SaveRun(ctx, run); err != nil {
			return ApprovalResolution{}, false, err
		}
		return ApprovalResolution{Approval: approval, Run: &run}, false, nil
	}
	run.ResumeState = "approval_resuming"
	if approved {
		run.Status = RunStatusRunning
		for index := range run.ToolCalls {
			call := &run.ToolCalls[index]
			if call.Status != "PENDING_APPROVAL" {
				continue
			}
			call.Status = "RUNNING"
			call.RequiresUser = false
			call.UpdatedAt = nowString()
		}
		run.Message = "审批已通过，正在后台继续执行。"
	} else {
		run.Message = "审批已拒绝，正在后台结束运行。"
	}
	if err := r.store.SaveRun(ctx, run); err != nil {
		return ApprovalResolution{}, false, err
	}
	return ApprovalResolution{Approval: approval, Run: &run}, true, nil
}

func (r *Runtime) enqueueResolvedApprovalContinuation(runID string) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || runID == "" {
		return
	}
	r.approvalMu.Lock()
	if _, ok := r.approvalRuns[runID]; ok {
		r.approvalMu.Unlock()
		return
	}
	r.approvalRuns[runID] = struct{}{}
	r.approvalMu.Unlock()
	go func() {
		defer func() {
			r.approvalMu.Lock()
			delete(r.approvalRuns, runID)
			r.approvalMu.Unlock()
		}()
		if err := r.continueResolvedApprovalRun(context.Background(), runID); err != nil {
			r.markApprovalContinuationFailed(context.Background(), runID, err)
		}
	}()
}

func (r *Runtime) continueResolvedApprovalRun(ctx context.Context, runID string) error {
	run, ok, err := r.store.Run(ctx, runID)
	if err != nil || !ok {
		return err
	}
	var approval Approval
	for _, item := range run.PendingApprovals {
		if item.Status != ApprovalStatusPending {
			approval = item
			break
		}
	}
	if approval.ID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, runTimeoutForRun(run))
	defer cancel()
	_, err = r.continueResolvedApproval(ctx, approval, approval.Status == ApprovalStatusApproved)
	return err
}

func (r *Runtime) markApprovalContinuationFailed(ctx context.Context, runID string, cause error) {
	run, ok, err := r.store.Run(ctx, runID)
	if err != nil || !ok || !runCanContinueResolvedApproval(run) || runHasPendingApproval(run.PendingApprovals) {
		return
	}
	run.Status = RunStatusFailed
	run.ResumeState = "approval_continuation_failed"
	run.Message = "审批已提交，但后台执行失败。"
	run.FailureReason = userFacingADKError(cause)
	run.ErrorCode = "APPROVAL_CONTINUATION_FAILED"
	run.Degraded = true
	run.CompletedAt = new(nowString())
	finalizeRunUsage(&run)
	_ = r.store.SaveRun(ctx, run)
	replyResult := openAIChatResult{Reply: localReply(run.UserMessage, toolSummariesForRun(run), cause)}
	if saved, msgErr := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, replyResult); msgErr == nil {
		run.FinalMessageID = saved.ID
		_ = r.store.SaveRun(ctx, run)
	}
	r.audit(ctx, "run.failed", run.ID, "Agent approval continuation failed.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState, "failureReason": run.FailureReason,
	})
}

func runHasPendingApproval(approvals []Approval) bool {
	for _, approval := range approvals {
		if approval.Status == ApprovalStatusPending {
			return true
		}
	}
	return false
}

func (r *Runtime) continueResolvedApproval(ctx context.Context, approval Approval, approved bool) (ApprovalResolution, error) {
	var updatedRun *Run
	var createdMessage *Message
	run, ok, err := r.store.Run(ctx, approval.RunID)
	if err == nil && ok {
		if !runCanContinueResolvedApproval(run) {
			return ApprovalResolution{Approval: approval}, nil
		}
		replacedApproval := false
		for index := range run.PendingApprovals {
			if run.PendingApprovals[index].ID == approval.ID {
				run.PendingApprovals[index] = approval
				replacedApproval = true
			}
		}
		if !replacedApproval {
			updatedRun = &run
			return ApprovalResolution{Approval: approval, Run: updatedRun}, nil
		}
		if err := r.store.SaveRun(ctx, run); err != nil {
			return ApprovalResolution{}, err
		}
		if !approved {
			for index := range run.PendingApprovals {
				item := &run.PendingApprovals[index]
				if item.Status != ApprovalStatusPending {
					continue
				}
				resolved, changed, resolveErr := r.store.ResolvePendingApproval(ctx, item.ID, ApprovalStatusDenied)
				if resolveErr == nil && changed {
					*item = resolved
				}
			}
			for index := range run.ToolCalls {
				call := &run.ToolCalls[index]
				if call.Status == "PENDING_APPROVAL" {
					call.Status = "DENIED"
					call.RequiresUser = false
					finishToolCall(call)
				}
			}
		}
		hasPending := false
		for _, item := range run.PendingApprovals {
			if item.Status == ApprovalStatusPending {
				hasPending = true
				break
			}
		}
		if hasPending {
			_ = r.store.SaveRun(ctx, run)
			updatedRun = &run
			return ApprovalResolution{Approval: approval, Run: updatedRun}, nil
		}
		resumedRun, message, handled, resumeErr := r.resumeGoogleADKWithBusyRetry(ctx, run)
		if resumeErr != nil {
			return ApprovalResolution{}, resumeErr
		}
		if !handled {
			return ApprovalResolution{}, fmt.Errorf("approval context is unavailable for run %s", run.ID)
		}
		updatedRun = &resumedRun
		createdMessage = message
		return ApprovalResolution{Approval: approval, Run: updatedRun, Message: createdMessage}, nil
	}
	return ApprovalResolution{Approval: approval, Run: updatedRun, Message: createdMessage}, nil
}

func (r *Runtime) resumeGoogleADKWithBusyRetry(ctx context.Context, run Run) (Run, *Message, bool, error) {
	delays := []time.Duration{120 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, time.Second}
	var lastErr error
	for attempt := 0; attempt <= len(delays); attempt++ {
		resumedRun, message, handled, err := r.resumeGoogleADK(ctx, run)
		if err == nil || !isRetryableADKSessionBusy(err) || attempt == len(delays) {
			return resumedRun, message, handled, err
		}
		lastErr = err
		timer := time.NewTimer(delays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return resumedRun, message, handled, errors.Join(ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
	return run, nil, true, lastErr
}

func isRetryableADKSessionBusy(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "append event to sessionservice") &&
		(strings.Contains(lower, "database is locked") || strings.Contains(lower, "sqlite_busy"))
}

func (r *Runtime) ReconcileResolvedApprovals(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	runs, err := r.store.ListRuns(ctx)
	if err != nil {
		return
	}
	for _, run := range runs {
		if !runCanContinueResolvedApproval(run) {
			continue
		}
		hasPending := false
		for _, embedded := range run.PendingApprovals {
			if embedded.Status != ApprovalStatusPending {
				continue
			}
			hasPending = true
			approval, ok, err := r.store.Approval(ctx, embedded.ID)
			if err != nil || !ok || approval.Status == ApprovalStatusPending {
				continue
			}
			_, _ = r.ResolveApprovalAsync(ctx, approval.ID, approval.Status == ApprovalStatusApproved)
			break
		}
		if !hasPending && len(run.PendingApprovals) > 0 {
			r.enqueueResolvedApprovalContinuation(run.ID)
		}
	}
}

func runHasRecoverableApprovalContext(run Run) bool {
	for _, approval := range run.PendingApprovals {
		if approval.Status != ApprovalStatusPending {
			continue
		}
		if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
			return true
		}
	}
	return false
}

func runHasRecoverableResolvedApprovalContext(run Run) bool {
	if strings.TrimSpace(run.ResumeState) != "approval_resuming" {
		return false
	}
	for _, approval := range run.PendingApprovals {
		if approval.Status == ApprovalStatusPending {
			continue
		}
		if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
			return true
		}
	}
	return false
}

func runCanContinueResolvedApproval(run Run) bool {
	if run.Status == RunStatusPending {
		return true
	}
	return run.Status == RunStatusRunning && runHasRecoverableResolvedApprovalContext(run)
}
