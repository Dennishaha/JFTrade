package workflowexec

import (
	"context"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
)

func TestRunChildBlocksDelegatedApprovalTask(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeLoop)
	providerID := saveGoalWorkflowProvider(t, runtime, "run-child-approval-provider", func(req responsesTestRequest) responsesTestMessage {
		for _, tool := range req.Tools {
			if providers.RestoreToolNameFromOpenAI(tool.Function.Name) == "approval.required" {
				return responsesTestMessage{Role: "assistant", ToolCalls: []responsesTestToolCall{{
					ID: "call-approval-required", Type: "function",
					Function: responsesTestFunction{Name: providers.SanitizeToolNameForOpenAI("approval.required"), Arguments: "{}"},
				}}}
			}
		}
		return responsesTestMessage{Role: "assistant", Content: "完成。"}
	})
	agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
		ID:             "run-child-approval-agent",
		Name:           "Run Child Approval",
		ProviderID:     providerID,
		Status:         AgentStatusEnabled,
		WorkMode:       WorkModeLoop,
		PermissionMode: PermissionModeApproval,
		Tools:          []string{"approval.required"},
	})
	session := mustCreateSession(t, runtime, agent.ID, "run child approval")
	now := assistantmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "run-child-approval-parent",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		Objective:      "保存策略草稿",
		CreatedAt:      now,
		StartedAt:      now,
		UpdatedAt:      now,
		Usage:          &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID:           "task-run-child-approval",
		Title:        "保存策略草稿",
		Status:       "IN_PROGRESS",
		AgentID:      agent.ID,
		RunID:        parent.ID,
		Order:        1,
		WorkflowMode: WorkModeLoop,
		Objective:    parent.Objective,
		Message:      "请 @approval.required 保存策略",
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = assistantmodel.WorkflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
	if err := runtime.Store().SaveRun(ctx, parent); err != nil {
		t.Fatalf("SaveRun parent with plan: %v", err)
	}

	result := (&WorkflowExecutor{runtime: runtime}).RunChild(ctx, workflowRequest{
		Agent:     agent,
		Session:   session,
		Objective: parent.Objective,
	}, parent, workflowStep{
		Title:   task.Title,
		Message: "请 @approval.required 保存策略",
	}, task, 1)
	if result.Err != nil {
		t.Fatalf("RunChild approval path: %v", result.Err)
	}
	if result.Response.Run.Status != RunStatusPending || len(result.Response.PendingApprovals) != 1 {
		t.Fatalf("child response = %+v, want pending approval child response", result.Response)
	}
	if executions.Load() != 0 {
		t.Fatalf("tool executions = %d, want 0 before approval is granted", executions.Load())
	}
	storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("Task lookup ok=%v err=%v", ok, err)
	}
	if storedTask.Status != "BLOCKED" || storedTask.Executor != workflowTaskExecutorChild || storedTask.RunID == "" {
		t.Fatalf("stored task = %+v, want blocked child task with run id", storedTask)
	}
	if !strings.Contains(storedTask.ResultSummary, "审批队列") {
		t.Fatalf("task result summary = %q, want approval guidance", storedTask.ResultSummary)
	}
	child, ok, err := runtime.Store().Run(ctx, storedTask.RunID)
	if err != nil || !ok {
		t.Fatalf("child run lookup ok=%v err=%v", ok, err)
	}
	if child.ParentRunID != parent.ID || child.Status != RunStatusPending || len(child.PendingApprovals) != 1 {
		t.Fatalf("stored child = %+v, want pending child linked to parent", child)
	}
	storedParent, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("parent run lookup ok=%v err=%v", ok, err)
	}
	if len(storedParent.ChildRunIDs) != 1 || storedParent.ChildRunIDs[0] != child.ID {
		t.Fatalf("stored parent child runs = %+v, want child %q", storedParent.ChildRunIDs, child.ID)
	}
	if got := storedParent.WorkflowPlan[0].ChildRunID; got != child.ID {
		t.Fatalf("stored workflow plan child run id = %q, want %q", got, child.ID)
	}
}
