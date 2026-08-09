package adk

import (
	"context"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (r *Runtime) syncParentWorkflowFromChild(ctx context.Context, child Run) (*Run, error) {
	parent, ok, err := r.workflowParentForChild(ctx, child)
	if err != nil || !ok {
		return nil, err
	}
	parentCtx, finishParent, err := r.beginOrReuseRunExecutionLease(ctx, parent.ID)
	if err != nil {
		return nil, err
	}
	defer finishParent()
	return r.syncClaimedParentWorkflowFromChild(parentCtx, parent, child)
}

func (r *Runtime) workflowParentForChild(ctx context.Context, child Run) (Run, bool, error) {
	if r == nil || r.store == nil || strings.TrimSpace(child.ParentRunID) == "" {
		return Run{}, false, nil
	}
	parent, ok, err := r.store.Run(ctx, child.ParentRunID)
	if err != nil || !ok {
		return Run{}, false, err
	}
	if jfadkmodel.NormalizeWorkMode(parent.WorkMode) == WorkModeChat || strings.TrimSpace(parent.WorkflowStatus) == "" {
		return Run{}, false, nil
	}
	return parent, true, nil
}

func (r *Runtime) syncClaimedParentWorkflowFromChild(ctx context.Context, parent Run, child Run) (*Run, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = jfadkmodel.WorkflowEngineForMode(parent.WorkMode)
	}
	parent.ChildRunIDs = jfadkmodel.AppendUniqueString(parent.ChildRunIDs, child.ID)
	parent = jfadkmodel.UpdateWorkflowPlanForChild(parent, child)
	parent.PendingApprovals = PendingApprovalsOnly(child.PendingApprovals)
	parent.InputRequest = normalizeInputRequest(child.InputRequest)
	if userPausedGoalParent(parent) {
		if _, err := r.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
			return nil, err
		}
		return &parent, nil
	}
	switch child.Status {
	case RunStatusPendingInput:
		parent.Status = RunStatusPendingInput
		parent.WorkflowStatus = workflowStatusPaused
		parent.Message = child.Message
	case RunStatusPending:
		parent.Status = RunStatusPending
		parent.WorkflowStatus = workflowStatusPaused
		parent.Message = child.Message
	case RunStatusRunning:
		parent.Status = RunStatusRunning
		parent.WorkflowStatus = workflowStatusRunning
		parent.Message = child.Message
	default:
		if userPauseRequestedGoalParent(parent) {
			parent = markUserPausedGoalParent(parent)
			if _, err := r.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
				return nil, err
			}
			return &parent, nil
		}
		if parent.Status == RunStatusPending || parent.Status == RunStatusPendingInput || parent.Status == RunStatusRunning {
			parent.Status = RunStatusRunning
			parent.WorkflowStatus = workflowStatusRunning
			parent.Message = "workflow resumed"
		}
	}
	if _, err := r.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
		return nil, err
	}
	return &parent, nil
}

func (r *Runtime) continueParentWorkflowAfterChild(ctx context.Context, child Run) (*Run, error) {
	parentRun, ok, err := r.workflowParentForChild(ctx, child)
	if err != nil || !ok {
		return nil, err
	}
	resumeCtx, finishParent, err := r.beginOrReuseRunExecutionLease(ctx, parentRun.ID)
	if err != nil {
		if isRunLeaseHeld(err) {
			return &parentRun, nil
		}
		return nil, err
	}
	defer finishParent()
	parent, err := r.syncClaimedParentWorkflowFromChild(resumeCtx, parentRun, child)
	if err != nil {
		return nil, err
	}
	if userPausedGoalParent(*parent) {
		paused := markUserPausedGoalParent(*parent)
		if _, saveErr := r.SaveRunPreservingUserGoalPause(resumeCtx, paused); saveErr != nil {
			return nil, saveErr
		}
		return &paused, nil
	}
	if child.Status == RunStatusPending || child.Status == RunStatusPendingInput || child.Status == RunStatusRunning {
		return parent, nil
	}
	if userPauseRequestedGoalParent(*parent) {
		paused := markUserPausedGoalParent(*parent)
		if _, saveErr := r.SaveRunPreservingUserGoalPause(resumeCtx, paused); saveErr != nil {
			return nil, saveErr
		}
		return &paused, nil
	}
	if child.Status != RunStatusCompleted {
		terminated, terminateErr := r.TerminateParentWorkflowFromChild(resumeCtx, *parent, child)
		if terminateErr != nil {
			return nil, terminateErr
		}
		return &terminated, nil
	}
	session, _, err := r.workflowResumeContext(resumeCtx, *parent)
	if err != nil {
		executor, executorErr := r.workflowExecutor()
		if executorErr != nil {
			return nil, executorErr
		}
		failed, persistErr := executor.FailParent(resumeCtx, *parent, err)
		if persistErr != nil {
			return nil, persistErr
		}
		return &failed, nil
	}
	executor, executorErr := r.workflowExecutor()
	if executorErr != nil {
		return nil, executorErr
	}
	var updated Run
	switch jfadkmodel.NormalizeWorkMode(parent.WorkMode) {
	case WorkModeLoop:
		updated, err = executor.ResumeLoopWorkflow(resumeCtx, session, *parent)
	default:
		updated = *parent
	}
	if err != nil {
		failed, persistErr := executor.FailParent(resumeCtx, *parent, err)
		if persistErr != nil {
			return nil, persistErr
		}
		return &failed, nil
	}
	return &updated, nil
}

func (r *Runtime) workflowResumeContext(ctx context.Context, parent Run) (Session, Agent, error) {
	session, ok, err := r.store.Session(ctx, parent.SessionID)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("session not found")
		}
		return Session{}, Agent{}, err
	}
	agent, err := r.resolveAgent(ctx, parent.AgentID)
	if err != nil {
		return Session{}, Agent{}, err
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return Session{}, Agent{}, err
	}
	if validPermissionMode(parent.PermissionMode) {
		agent.PermissionMode = normalizePermissionMode(parent.PermissionMode)
	}
	agent.WorkMode = jfadkmodel.NormalizeWorkMode(parent.WorkMode)
	return session, agent, nil
}

func (r *Runtime) TerminateParentWorkflowFromChild(ctx context.Context, parent Run, child Run) (Run, error) {
	parent = jfadkmodel.UpdateWorkflowPlanForChild(parent, child)
	parent.Status = child.Status
	parent.Message = child.Message
	parent.FailureReason = child.FailureReason
	parent.ErrorCode = child.ErrorCode
	parent.Degraded = true
	parent.WorkflowStatus = workflowStatusFailed
	parent.PendingApprovals = nil
	if parent.FailureReason == "" {
		switch child.Status {
		case RunStatusDenied:
			parent.FailureReason = "workflow stopped because a child approval was denied"
		case RunStatusCancelled:
			parent.FailureReason = "workflow stopped because a child run was cancelled"
		case RunStatusTimedOut:
			parent.FailureReason = "workflow stopped because a child run timed out"
		default:
			parent.FailureReason = "workflow stopped because a child run failed"
		}
	}
	if parent.ErrorCode == "" {
		parent.ErrorCode = runErrorCode(child.Status)
		if child.Status == RunStatusDenied {
			parent.ErrorCode = "APPROVAL_DENIED"
		}
	}
	completedAt := nowString()
	parent.CompletedAt = &completedAt
	if child.Status == RunStatusCancelled {
		parent.CancelledAt = &completedAt
	}
	finalizeRunUsage(&parent)
	if err := r.store.SaveRunAndDenyPendingApprovals(ctx, parent); err != nil {
		return parent, fmt.Errorf("persist terminal parent workflow state: %w", err)
	}
	r.CancelUnfinishedWorkflowChildren(context.Background(), parent)
	return parent, nil
}

func userPauseRequestedGoalParent(run Run) bool {
	return jfadkmodel.UserPauseRequestedGoalParent(run)
}

func userPausedGoalParent(run Run) bool {
	return jfadkmodel.UserPausedGoalParent(run)
}

func markUserPausedGoalParent(run Run) Run {
	return jfadkmodel.MarkUserPausedGoalParent(run)
}
