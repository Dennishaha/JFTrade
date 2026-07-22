package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func (r *Runtime) ResolveApproval(ctx context.Context, approvalID string, approved bool) (ApprovalResolution, error) {
	status := ApprovalStatusDenied
	if approved {
		status = ApprovalStatusApproved
	}
	approval, changed, run, shouldContinue, err := r.store.resolveAndStageApproval(ctx, approvalID, status)
	if err != nil {
		return ApprovalResolution{}, err
	}
	if !changed && (approval.ID == "" || approval.Status != status) {
		return ApprovalResolution{Approval: approval}, nil
	}
	if changed {
		r.audit(ctx, "approval.resolved", approval.ID, "Agent approval resolved.", map[string]any{
			"runId": approval.RunID, "toolName": approval.ToolName, "approved": approved,
		})
	}
	staged := ApprovalResolution{Approval: approval, Run: run}
	if !shouldContinue {
		return r.attachParentWorkflowResolution(ctx, staged)
	}
	if !r.claimApprovalContinuation(approval.RunID) {
		return r.attachParentWorkflowResolution(ctx, staged)
	}
	defer r.releaseApprovalContinuation(approval.RunID)
	leaseCtx, cancel, waitForLease, leaseErr := r.beginRunExecutionLease(ctx, approval.RunID)
	if leaseErr != nil {
		if isRunLeaseHeld(leaseErr) {
			return r.attachParentWorkflowResolution(ctx, staged)
		}
		return ApprovalResolution{}, leaseErr
	}
	defer func() {
		cancel()
		waitForLease()
	}()
	resolution, err := r.continueResolvedApproval(leaseCtx, approval, approved)
	if err != nil {
		return ApprovalResolution{}, err
	}
	return r.attachParentWorkflowResolution(ctx, resolution)
}

func (r *Runtime) claimApprovalContinuation(runID string) bool {
	runID = strings.TrimSpace(runID)
	if r == nil || runID == "" {
		return false
	}
	r.approvalMu.Lock()
	defer r.approvalMu.Unlock()
	if r.approvalRuns == nil {
		r.approvalRuns = make(map[string]struct{})
	}
	if _, exists := r.approvalRuns[runID]; exists {
		return false
	}
	r.approvalRuns[runID] = struct{}{}
	return true
}

func (r *Runtime) releaseApprovalContinuation(runID string) {
	if r == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	r.approvalMu.Lock()
	delete(r.approvalRuns, runID)
	r.approvalMu.Unlock()
}

func (r *Runtime) ResolveApprovalAsync(ctx context.Context, approvalID string, approved bool) (ApprovalResolution, error) {
	status := ApprovalStatusDenied
	if approved {
		status = ApprovalStatusApproved
	}
	approval, changed, run, shouldContinue, err := r.store.resolveAndStageApproval(ctx, approvalID, status)
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
	resolution := ApprovalResolution{Approval: approval, Run: run}
	resolution, err = r.attachParentWorkflowResolution(ctx, resolution)
	if err != nil {
		return ApprovalResolution{}, err
	}
	if shouldContinue {
		r.enqueueResolvedApprovalContinuation(approval.RunID)
	}
	return resolution, nil
}

func (r *Runtime) stageResolvedApproval(ctx context.Context, approval Approval, approved bool) (ApprovalResolution, bool, error) {
	status := ApprovalStatusDenied
	if approved {
		status = ApprovalStatusApproved
	}
	resolved, _, run, shouldContinue, err := r.store.resolveAndStageApproval(ctx, approval.ID, status)
	if err != nil {
		return ApprovalResolution{}, false, err
	}
	if resolved.ID == "" {
		resolved = approval
	}
	return ApprovalResolution{Approval: resolved, Run: run}, shouldContinue, nil
}

func (r *Runtime) enqueueResolvedApprovalContinuation(runID string) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || runID == "" {
		return
	}
	if !r.claimApprovalContinuation(runID) {
		return
	}
	r.approvalMu.Lock()
	if r.closing {
		r.approvalMu.Unlock()
		r.releaseApprovalContinuation(runID)
		return
	}
	r.approvalWG.Add(1)
	ctx := r.backgroundCtx
	if ctx == nil {
		ctx = context.Background()
	}
	r.approvalMu.Unlock()
	go func() {
		defer r.approvalWG.Done()
		defer r.releaseApprovalContinuation(runID)
		if err := r.continueResolvedApprovalRun(ctx, runID); err != nil {
			if ctx.Err() != nil {
				return
			}
			besteffort.LogError(r.markApprovalContinuationFailed(context.Background(), runID, err))
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
		if item.Status == ApprovalStatusDenied {
			approval = item
			break
		}
		if approval.ID == "" && item.Status != ApprovalStatusPending {
			approval = item
		}
	}
	if approval.ID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, runTimeoutForRun(run))
	defer cancel()
	leaseCtx, leaseCancel, waitForLease, leaseErr := r.beginRunExecutionLease(ctx, run.ID)
	if leaseErr != nil {
		if isRunLeaseHeld(leaseErr) {
			return nil
		}
		return leaseErr
	}
	defer func() {
		leaseCancel()
		waitForLease()
	}()
	resolution, err := r.continueResolvedApproval(leaseCtx, approval, approval.Status == ApprovalStatusApproved)
	if err != nil {
		return err
	}
	if resolution.Run != nil {
		_, err = r.continueParentWorkflowAfterChild(leaseCtx, *resolution.Run)
	}
	return err
}

func (r *Runtime) markApprovalContinuationFailed(ctx context.Context, runID string, cause error) error {
	run, ok, err := r.store.Run(ctx, runID)
	if err != nil {
		return err
	}
	if !ok || !runCanContinueResolvedApproval(run) || runHasPendingApproval(run.PendingApprovals) {
		return nil
	}
	run.Status = RunStatusFailed
	run.ResumeState = "approval_continuation_failed"
	run.Message = "审批已提交，但后台执行失败。"
	run.FailureReason = userFacingADKError(cause)
	run.ErrorCode = "APPROVAL_CONTINUATION_FAILED"
	run.Degraded = true
	run.CompletedAt = new(nowString())
	finalizeRunUsage(&run)
	if err := r.store.SaveRun(ctx, run); err != nil {
		return fmt.Errorf("persist approval continuation failure: %w", err)
	}
	if _, err := r.continueParentWorkflowAfterChild(ctx, run); err != nil {
		return err
	}
	replyResult := openAIChatResult{Reply: run.FailureReason}
	if saved, msgErr := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, replyResult); msgErr == nil {
		run.FinalMessageID = saved.ID
		if err := r.store.SaveRun(ctx, run); err != nil {
			return fmt.Errorf("persist approval failure message reference: %w", err)
		}
	}
	r.audit(ctx, "run.failed", run.ID, "Agent approval continuation failed.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState, "failureReason": run.FailureReason,
	})
	return nil
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
			if err := r.store.SaveRun(ctx, run); err != nil {
				return ApprovalResolution{}, err
			}
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
	for attempt := 0; ; attempt++ {
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
		if isWorkflowParentRun(run) {
			r.reconcileWorkflowParent(ctx, run)
			continue
		}
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
			_, jftradeErr7 := r.ResolveApprovalAsync(ctx, approval.ID, approval.Status == ApprovalStatusApproved)
			besteffort.LogError(jftradeErr7)
			break
		}
		if !hasPending && len(run.PendingApprovals) > 0 {
			r.enqueueResolvedApprovalContinuation(run.ID)
		}
	}
}

func (r *Runtime) attachParentWorkflowResolution(ctx context.Context, resolution ApprovalResolution) (ApprovalResolution, error) {
	if resolution.Run == nil {
		return resolution, nil
	}
	var parent *Run
	var err error
	if resolution.Run.Status == RunStatusPending || resolution.Run.Status == RunStatusRunning {
		parent, err = r.syncParentWorkflowFromChild(ctx, *resolution.Run)
	} else {
		parent, err = r.continueParentWorkflowAfterChild(ctx, *resolution.Run)
	}
	if err != nil {
		return ApprovalResolution{}, err
	}
	if parent != nil {
		resolution.ParentRun = parent
	}
	return resolution, nil
}

func (r *Runtime) reconcileWorkflowParent(ctx context.Context, parent Run) {
	if parent.Status != RunStatusPending && parent.Status != RunStatusRunning {
		return
	}
	for _, step := range parent.WorkflowPlan {
		if strings.TrimSpace(step.ChildRunID) == "" {
			continue
		}
		child, ok, err := r.store.Run(ctx, step.ChildRunID)
		if err != nil || !ok {
			continue
		}
		if child.Status == RunStatusPending {
			for _, embedded := range child.PendingApprovals {
				if embedded.Status != ApprovalStatusPending {
					continue
				}
				approval, ok, err := r.store.Approval(ctx, embedded.ID)
				if err == nil && ok && approval.Status != ApprovalStatusPending {
					_, jftradeErr6 := r.ResolveApprovalAsync(ctx, approval.ID, approval.Status == ApprovalStatusApproved)
					besteffort.LogError(jftradeErr6)
					return
				}
			}
			continue
		}
		if child.Status != RunStatusRunning {
			_, jftradeErr5 := r.continueParentWorkflowAfterChild(ctx, child)
			besteffort.LogError(jftradeErr5)
			return
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
	if isWorkflowParentRun(run) {
		return false
	}
	if run.Status == RunStatusPending {
		return true
	}
	return run.Status == RunStatusRunning && runHasRecoverableResolvedApprovalContext(run)
}

func isWorkflowParentRun(run Run) bool {
	return normalizeWorkMode(run.WorkMode) != WorkModeChat && strings.TrimSpace(run.WorkflowStatus) != ""
}
