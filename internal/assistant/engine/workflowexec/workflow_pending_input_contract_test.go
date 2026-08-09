package workflowexec

import (
	"context"
	"strings"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowExecutorProjectsPendingInputAndPersistsChildState(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "workflow-input-agent", "workflow pending input")
	executor := &WorkflowExecutor{runtime: runtime}
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-input-parent", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	taskAwaitingInput, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "workflow-input-task", Title: "Need user choice", Status: "TODO", RunID: parent.ID, Order: 1})
	if err != nil {
		t.Fatalf("SaveTask awaiting input: %v", err)
	}
	passiveTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "workflow-passive-task", Title: "Passive child", Status: "TODO", RunID: parent.ID, Order: 2})
	if err != nil {
		t.Fatalf("SaveTask passive: %v", err)
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{taskAwaitingInput, passiveTask}, nil)
	mustSaveRun(t, runtime, parent)
	awaiting := Run{ID: "workflow-input-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID, Iteration: 1, Status: RunStatusRunning, Usage: &RunUsage{}}
	passive := Run{ID: "workflow-passive-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID, Iteration: 2, Status: RunStatusCompleted, Usage: &RunUsage{}}
	request := &InputRequest{ID: "workflow-input-request", RunID: awaiting.ID, AgentID: session.AgentID, FunctionCallID: "ask-user", Status: InputRequestStatusPending}
	execution := &fakeWorkflowExecutionHandle{}

	response, err := executor.finishWorkflowPendingInputs(ctx, workflowRequest{Session: session}, parent, []Task{taskAwaitingInput, passiveTask}, []Run{awaiting, passive}, workflowExecutionResult{
		execution: execution,
		inputRequests: map[string]*InputRequest{
			awaiting.ID: request,
		},
	})
	if err != nil {
		t.Fatalf("finish workflow pending inputs: %v", err)
	}
	if response.Run.Status != RunStatusPendingInput || response.Run.WorkflowStatus != workflowStatusPaused || response.Run.InputRequest == nil || response.Run.InputRequest.ID != request.ID {
		t.Fatalf("pending-input workflow response = %+v", response)
	}
	if response.Run.WorkflowPlan[0].Status != "BLOCKED" || !strings.Contains(response.Reply, "等待用户回答") {
		t.Fatalf("pending-input workflow plan/reply = %+v", response)
	}
	storedChild, ok, err := runtime.Store().Run(ctx, awaiting.ID)
	if err != nil || !ok || storedChild.Status != RunStatusPendingInput || storedChild.InputRequest == nil || storedChild.InputRequest.ID != request.ID {
		t.Fatalf("stored pending child = %+v ok=%v err=%v", storedChild, ok, err)
	}
}
