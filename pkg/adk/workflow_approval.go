package adk

import (
	"context"
	"fmt"
	"strings"
)

func (r *Runtime) syncParentWorkflowFromChild(ctx context.Context, child Run) (*Run, error) {
	if r == nil || r.store == nil || strings.TrimSpace(child.ParentRunID) == "" {
		return nil, nil
	}
	parent, ok, err := r.store.Run(ctx, child.ParentRunID)
	if err != nil || !ok {
		return nil, err
	}
	if normalizeWorkMode(parent.WorkMode) == WorkModeChat || strings.TrimSpace(parent.WorkflowStatus) == "" {
		return nil, nil
	}
	parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
	parent = updateWorkflowPlanForChild(parent, child)
	parent.PendingApprovals = pendingApprovalsOnly(child.PendingApprovals)
	if userPausedGoalParent(parent) {
		if _, err := r.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
			return nil, err
		}
		return &parent, nil
	}
	switch child.Status {
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
			if _, err := r.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
				return nil, err
			}
			return &parent, nil
		}
		if parent.Status == RunStatusPending || parent.Status == RunStatusRunning {
			parent.Status = RunStatusRunning
			parent.WorkflowStatus = workflowStatusRunning
			parent.Message = "workflow resumed"
		}
	}
	if _, err := r.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
		return nil, err
	}
	return &parent, nil
}

func (r *Runtime) continueParentWorkflowAfterChild(ctx context.Context, child Run) (*Run, error) {
	parent, err := r.syncParentWorkflowFromChild(ctx, child)
	if err != nil || parent == nil {
		return parent, err
	}
	if userPausedGoalParent(*parent) {
		paused := markUserPausedGoalParent(*parent)
		if _, saveErr := r.saveRunPreservingUserGoalPause(ctx, paused); saveErr != nil {
			return nil, saveErr
		}
		return &paused, nil
	}
	if child.Status == RunStatusPending || child.Status == RunStatusRunning {
		return parent, nil
	}
	if userPauseRequestedGoalParent(*parent) {
		paused := markUserPausedGoalParent(*parent)
		if _, saveErr := r.saveRunPreservingUserGoalPause(ctx, paused); saveErr != nil {
			return nil, saveErr
		}
		return &paused, nil
	}
	if child.Status != RunStatusCompleted {
		return new(r.terminateParentWorkflowFromChild(ctx, *parent, child)), nil
	}
	session, agent, err := r.workflowResumeContext(ctx, *parent)
	if err != nil {
		return new((&WorkflowExecutor{runtime: r}).failParent(ctx, *parent, err)), nil
	}
	executor := &WorkflowExecutor{runtime: r}
	var updated Run
	switch normalizeWorkMode(parent.WorkMode) {
	case WorkModeTask:
		updated, err = executor.resumeADKTaskWorkflow(ctx, session, agent, *parent)
	case WorkModeLoop:
		updated, err = executor.resumeLoopWorkflow(ctx, session, *parent)
	default:
		updated = *parent
	}
	if err != nil {
		return new(executor.failParent(ctx, *parent, err)), nil
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
	agent.WorkMode = normalizeWorkMode(parent.WorkMode)
	return session, agent, nil
}

func (r *Runtime) terminateParentWorkflowFromChild(ctx context.Context, parent Run, child Run) Run {
	parent = updateWorkflowPlanForChild(parent, child)
	parent.Status = child.Status
	parent.Message = child.Message
	parent.FailureReason = child.FailureReason
	parent.ErrorCode = child.ErrorCode
	parent.Degraded = true
	parent.WorkflowStatus = workflowStatusFailed
	parent.PendingApprovals = append([]Approval(nil), child.PendingApprovals...)
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
	jftradeErr1 := r.store.SaveRun(ctx, parent)
	jftradeLogError(jftradeErr1)
	return parent
}

func (e *WorkflowExecutor) resumeLoopWorkflow(ctx context.Context, session Session, parent Run) (Run, error) {
	if userPausedGoalParent(parent) {
		parent = markUserPausedGoalParent(parent)
		if _, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
			return Run{}, err
		}
		return parent, nil
	}
	parent, blocked, err := e.reconcileWorkflowChildren(ctx, parent)
	if err != nil {
		return Run{}, err
	}
	if blocked {
		return parent, nil
	}
	if userPauseRequestedGoalParent(parent) {
		parent = markUserPausedGoalParent(parent)
		if _, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
			return Run{}, err
		}
		return parent, nil
	}
	replies := make([]string, 0, len(parent.WorkflowPlan))
	for _, state := range parent.WorkflowPlan {
		if strings.TrimSpace(state.ChildRunID) != "" {
			replies = append(replies, fmt.Sprintf("%s 已完成", state.Title))
		}
	}
	return e.completeResumedWorkflow(ctx, session, parent, workflowSummary(parent, replies))
}

func (e *WorkflowExecutor) reconcileWorkflowChildren(ctx context.Context, parent Run) (Run, bool, error) {
	if userPausedGoalParent(parent) {
		parent = markUserPausedGoalParent(parent)
		if _, saveErr := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
			return Run{}, false, saveErr
		}
		return parent, true, nil
	}
	for _, state := range parent.WorkflowPlan {
		childRunID := strings.TrimSpace(state.ChildRunID)
		if childRunID == "" {
			continue
		}
		child, ok, err := e.runtime.store.Run(ctx, childRunID)
		if err != nil {
			return Run{}, false, err
		}
		if !ok {
			continue
		}
		parent = updateWorkflowPlanForChild(parent, child)
		switch child.Status {
		case RunStatusCompleted:
			if strings.TrimSpace(state.TaskID) != "" {
				_, jftradeErr2 := e.runtime.store.UpdateTask(ctx, state.TaskID, TaskPatchRequest{
					Status:        new("DONE"),
					RunID:         new(child.ID),
					Executor:      new(workflowTaskExecutorChild),
					ResultSummary: new(strings.TrimSpace(child.Message)),
				})
				jftradeLogError(jftradeErr2)
			}
			continue
		case RunStatusPending:
			parent.Status = RunStatusPending
			parent.WorkflowStatus = workflowStatusPaused
			parent.Message = defaultString(child.Message, "工作流正在等待审批。")
			parent.PendingApprovals = pendingApprovalsOnly(child.PendingApprovals)
			if _, saveErr := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
				return Run{}, false, saveErr
			}
			return parent, true, nil
		case RunStatusRunning:
			parent.Status = RunStatusRunning
			parent.WorkflowStatus = workflowStatusRunning
			parent.Message = defaultString(child.Message, "工作流正在等待子运行完成。")
			parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
			if _, saveErr := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
				return Run{}, false, saveErr
			}
			return parent, true, nil
		default:
			return e.runtime.terminateParentWorkflowFromChild(ctx, parent, child), true, nil
		}
	}
	return parent, false, nil
}

func userPauseRequestedGoalParent(run Run) bool {
	return normalizeWorkMode(run.WorkMode) == WorkModeLoop &&
		strings.TrimSpace(run.ParentRunID) == "" &&
		run.PauseRequestedAt != nil
}

func userPausedGoalParent(run Run) bool {
	return normalizeWorkMode(run.WorkMode) == WorkModeLoop &&
		strings.TrimSpace(run.ParentRunID) == "" &&
		run.Status == RunStatusPaused &&
		run.PausedReason == "user"
}

func markUserPausedGoalParent(run Run) Run {
	pausedAt := nowString()
	run.Status = RunStatusPaused
	run.WorkflowStatus = workflowStatusPaused
	if run.PausedAt == nil {
		run.PausedAt = &pausedAt
	}
	run.PausedReason = "user"
	run.ResumeState = "user_paused"
	run.Message = "目标已暂停。"
	run.PendingApprovals = pendingApprovalsOnly(run.PendingApprovals)
	return run
}

func (e *WorkflowExecutor) completeResumedWorkflow(ctx context.Context, session Session, parent Run, reply string) (Run, error) {
	parent.Status = RunStatusCompleted
	parent.Message = "workflow completed"
	parent.WorkflowStatus = workflowStatusComplete
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent.CompletedAt = new(nowString())
	finalizeRunUsage(&parent)
	message, err := e.runtime.ensureAssistantMessage(ctx, session, parent, openAIChatResult{Reply: reply})
	if err == nil {
		parent.FinalMessageID = message.ID
	}
	if _, saveErr := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
		return Run{}, saveErr
	}
	return parent, nil
}
