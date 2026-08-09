package workflowexec

import (
	"context"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func (e *WorkflowExecutor) ResumeLoopWorkflow(ctx context.Context, session Session, parent Run) (Run, error) {
	if jfadkmodel.UserPausedGoalParent(parent) {
		parent = jfadkmodel.MarkUserPausedGoalParent(parent)
		if _, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
			return Run{}, err
		}
		return parent, nil
	}
	parent, blocked, err := e.ReconcileWorkflowChildren(ctx, parent)
	if err != nil {
		return Run{}, err
	}
	if blocked {
		return parent, nil
	}
	if jfadkmodel.UserPauseRequestedGoalParent(parent) {
		parent = jfadkmodel.MarkUserPausedGoalParent(parent)
		if _, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
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
	return e.CompleteResumedWorkflow(ctx, session, parent, jfadkmodel.WorkflowSummary(parent, replies))
}

func (e *WorkflowExecutor) ReconcileWorkflowChildren(ctx context.Context, parent Run) (Run, bool, error) {
	if jfadkmodel.UserPausedGoalParent(parent) {
		parent = jfadkmodel.MarkUserPausedGoalParent(parent)
		if _, saveErr := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
			return Run{}, false, saveErr
		}
		return parent, true, nil
	}
	for _, state := range parent.WorkflowPlan {
		childRunID := strings.TrimSpace(state.ChildRunID)
		if childRunID == "" {
			continue
		}
		child, ok, err := e.runtime.WorkflowStore().Run(ctx, childRunID)
		if err != nil {
			return Run{}, false, err
		}
		if !ok {
			continue
		}
		if !jfadkmodel.IsDirectWorkflowChild(parent, child) {
			continue
		}
		parent = jfadkmodel.UpdateWorkflowPlanForChild(parent, child)
		switch child.Status {
		case RunStatusCompleted:
			if strings.TrimSpace(state.TaskID) != "" {
				_, jftradeErr2 := e.runtime.WorkflowStore().UpdateTask(ctx, state.TaskID, TaskPatchRequest{
					Status:        new("DONE"),
					RunID:         new(child.ID),
					Executor:      new(workflowTaskExecutorChild),
					ResultSummary: new(strings.TrimSpace(child.Message)),
				})
				besteffort.LogError(jftradeErr2)
			}
			continue
		case RunStatusPending, RunStatusPendingInput:
			parent.Status = child.Status
			parent.WorkflowStatus = workflowStatusPaused
			parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(child.PendingApprovals)
			parent.InputRequest = jfadkmodel.NormalizeInputRequest(child.InputRequest)
			parent.Message = jfadkmodel.DefaultString(child.Message, jfadkmodel.WorkflowPendingReply(parent))
			if _, saveErr := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
				return Run{}, false, saveErr
			}
			return parent, true, nil
		case RunStatusRunning:
			parent.Status = RunStatusRunning
			parent.WorkflowStatus = workflowStatusRunning
			parent.Message = jfadkmodel.DefaultString(child.Message, "工作流正在等待子运行完成。")
			parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(parent.PendingApprovals)
			if _, saveErr := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
				return Run{}, false, saveErr
			}
			return parent, true, nil
		default:
			terminated, err := e.runtime.TerminateParentWorkflowFromChild(ctx, parent, child)
			return terminated, true, err
		}
	}
	return parent, false, nil
}

func (e *WorkflowExecutor) CompleteResumedWorkflow(ctx context.Context, session Session, parent Run, reply string) (Run, error) {
	parent.Status = RunStatusCompleted
	parent.Message = "workflow completed"
	parent.WorkflowStatus = workflowStatusComplete
	parent.PendingApprovals = nil
	parent.CompletedAt = new(jfadkmodel.NowString())
	jfadkmodel.FinalizeRunUsage(&parent)
	message, err := e.runtime.EnsureAssistantMessage(ctx, session, parent, jfadkmodel.AssistantExecutionResult{Reply: reply, SyntheticKind: "workflow_resume_summary"})
	if err == nil {
		parent.FinalMessageID = message.ID
	}
	if _, saveErr := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); saveErr != nil {
		return Run{}, saveErr
	}
	return parent, nil
}
