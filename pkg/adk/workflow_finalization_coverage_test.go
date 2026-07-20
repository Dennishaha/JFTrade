package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowExecutorPersistsFinalizedAndIncompletePlans(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "workflow-final-agent", Name: "Workflow Final Agent", Status: AgentStatusEnabled})
	session := mustCreateSession(t, runtime, agent.ID, "workflow finalization")
	executor := runtime.workflowExecutor()

	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-final-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	doneTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-final-task", Title: "Publish conclusion", Status: "DONE", AgentID: agent.ID, RunID: parent.ID, Order: 1,
	})
	if err != nil {
		t.Fatalf("SaveTask done: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{doneTask}, nil)
	mustSaveRun(t, runtime, parent)
	child := mustSaveRun(t, runtime, Run{
		ID: "workflow-final-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID, Iteration: 1,
		Status: RunStatusCompleted, Message: "child complete", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	completed, err := executor.finalizePlannedWorkflow(ctx, workflowRequest{Session: session}, parent, []Task{doneTask}, []ChatResponse{{Reply: "verified conclusion", Run: child}}, nil)
	if err != nil {
		t.Fatalf("finalize completed workflow: %v", err)
	}
	if completed.Run.Status != RunStatusCompleted || completed.Run.WorkflowStatus != workflowStatusComplete || completed.Run.Iteration != 1 || completed.Run.FinalMessageID == "" {
		t.Fatalf("completed workflow response = %+v", completed)
	}
	if !strings.Contains(completed.Reply, "verified conclusion") || len(completed.Run.WorkflowPlan) != 1 || completed.Run.WorkflowPlan[0].OutputSummary != "verified conclusion" {
		t.Fatalf("completed workflow summary/plan = %+v", completed)
	}

	incompleteParent := mustSaveRun(t, runtime, Run{
		ID: "workflow-incomplete-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	incompleteTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-incomplete-task", Title: "Still pending", Status: "TODO", AgentID: agent.ID, RunID: incompleteParent.ID,
	})
	if err != nil {
		t.Fatalf("SaveTask incomplete: %v", err)
	}
	incompleteParent.WorkflowPlan = workflowPlanFromTasks([]Task{incompleteTask}, nil)
	mustSaveRun(t, runtime, incompleteParent)
	incomplete, err := executor.finalizePlannedWorkflow(ctx, workflowRequest{Session: session}, incompleteParent, []Task{incompleteTask}, nil, nil)
	if err != nil {
		t.Fatalf("finalize incomplete workflow: %v", err)
	}
	if incomplete.Run.Status != RunStatusFailed || incomplete.Run.ErrorCode != workflowTaskIncompleteErr || !strings.Contains(incomplete.Run.FailureReason, "scheduler incomplete") {
		t.Fatalf("incomplete workflow response = %+v", incomplete)
	}
}

func TestWorkflowExecutorProjectsPendingInputAndRegistersLiveExecution(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "workflow-input-agent", "workflow pending input")
	executor := runtime.workflowExecutor()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-input-parent", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	taskAwaitingInput, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "workflow-input-task", Title: "Need user choice", Status: "TODO", RunID: parent.ID, Order: 1})
	if err != nil {
		t.Fatalf("SaveTask awaiting input: %v", err)
	}
	passiveTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "workflow-passive-task", Title: "Passive child", Status: "TODO", RunID: parent.ID, Order: 2})
	if err != nil {
		t.Fatalf("SaveTask passive: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{taskAwaitingInput, passiveTask}, nil)
	mustSaveRun(t, runtime, parent)
	awaiting := Run{ID: "workflow-input-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID, Iteration: 1, Status: RunStatusRunning, Usage: &RunUsage{}}
	passive := Run{ID: "workflow-passive-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID, Iteration: 2, Status: RunStatusCompleted, Usage: &RunUsage{}}
	request := &InputRequest{ID: "workflow-input-request", RunID: awaiting.ID, AgentID: session.AgentID, FunctionCallID: "ask-user", Status: InputRequestStatusPending}
	execution := newBareGoogleADKExecution(parent.ID)
	execution.onDelta = func(ChatDelta) error { return nil }

	// While waiting for an answer the same execution must remain addressable by
	// both parent and child run IDs, so a restart/reconcile pass can continue it.
	executor.registerWorkflowExecution(parent, []Run{awaiting, passive}, execution)
	if execution.onDelta != nil || runtime.adkRuns[parent.ID] != execution || runtime.adkRuns[awaiting.ID] != execution || runtime.adkRuns[passive.ID] != execution {
		t.Fatalf("registered workflow execution = root:%p awaiting:%p passive:%p", runtime.adkRuns[parent.ID], runtime.adkRuns[awaiting.ID], runtime.adkRuns[passive.ID])
	}

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

func TestWorkflowExecutorPreparesParentPlanAndEmitsAuthoritativeSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "workflow-prepare-agent", "workflow preparation")
	executor := runtime.workflowExecutor()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-prepare-parent", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "prepare-task", Title: "Prepare", Status: "TODO"}},
		CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := Run{ID: "workflow-prepare-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID, Status: RunStatusRunning}
	var snapshots []Run
	prepared, err := executor.prepareWorkflowParent(ctx, workflowRequest{
		Agent: sessionAgent(session), Session: session, Message: "prepare plan", EmitRun: true,
		OnDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				snapshots = append(snapshots, *delta.Run)
			}
			return nil
		},
	}, parent, []Run{child})
	if err != nil {
		t.Fatalf("prepareWorkflowParent: %v", err)
	}
	if prepared.WorkflowEngine != WorkflowEngineADK2Loop || len(prepared.ChildRunIDs) != 1 || prepared.ChildRunIDs[0] != child.ID || prepared.WorkflowPlan[0].NodeName != googleADKWorkflowChildName(parent.ID, 0) {
		t.Fatalf("prepared parent = %+v", prepared)
	}
	if len(snapshots) != 1 || snapshots[0].ID != parent.ID {
		t.Fatalf("emitted workflow snapshots = %+v", snapshots)
	}
}

func sessionAgent(session Session) Agent {
	return Agent{ID: session.AgentID, Name: "workflow prepare agent", WorkMode: WorkModeLoop, PermissionMode: PermissionModeApproval}
}
