package adk

import (
	"strings"
	"testing"
)

func TestCoverage98WorkflowChildrenFailClosedForMissingAgentsAndFinalReplies(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-child-parent-agent", Name: "Coverage Child Parent", ProviderID: testProviderID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "coverage child recovery")
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage98-child-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	executor := runtime.workflowExecutor()

	if children, finishes, err := executor.startWorkflowChildRuns(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, parent, []workflowStep{{
		Title: "missing agent", Message: "must not start", ChildAgentID: "coverage98-agent-does-not-exist",
	}}, nil); err == nil || !strings.Contains(err.Error(), "agent not found") || children != nil || finishes != nil {
		t.Fatalf("startWorkflowChildRuns missing agent = children:%#v finishes:%#v err:%v", children, finishes, err)
	}

	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "coverage98-child-missing-agent-task", Title: "Missing Agent", Status: "TODO", AgentID: agent.ID, RunID: parent.ID, Order: 1,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	blocked := executor.runChild(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, parent, workflowStep{
		Title: "missing agent", Message: "must not start", ChildAgentID: "coverage98-agent-does-not-exist",
	}, task, 1)
	if blocked.Err != nil || blocked.Response.Run.Status != RunStatusFailed || !strings.Contains(blocked.Response.Reply, "agent not found") {
		t.Fatalf("runChild missing agent = %+v", blocked)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok || storedTask.Status != "BLOCKED" {
		t.Fatalf("missing agent task = %+v ok=%v err=%v", storedTask, ok, err)
	}

	child := Run{
		ID: "coverage98-child-without-final", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusRunning, UserMessage: "publish an audited conclusion", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	}
	execution := newBareGoogleADKExecution(parent.ID)
	execution.calls = []ToolCall{{ID: "coverage98-finished-tool", RunID: child.ID, ToolName: "market.snapshot", Status: "SUCCEEDED"}}
	if err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, execution, []Run{child}, []workflowStep{{
		Title: "synthesis", Message: child.UserMessage,
	}}, nil); err == nil || !strings.Contains(err.Error(), "agent mapping missing") {
		t.Fatalf("ensureWorkflowChildrenFinalReplies missing mapping = %v", err)
	}
	failedChild, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok || failedChild.Status != RunStatusFailed || failedChild.ErrorCode == "" {
		t.Fatalf("failed child terminal state = %+v ok=%v err=%v", failedChild, ok, err)
	}
}

func TestCoverage98WorkflowChildrenSkipIdleOrApprovalBlockedFinalization(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-child-idle-agent", Name: "Coverage Child Idle", ProviderID: testProviderID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "coverage child idle")
	executor := runtime.workflowExecutor()
	child := Run{ID: "coverage98-idle-child", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning, Usage: &RunUsage{}}

	responses, err := executor.completeWorkflowChildrenFromADK(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, newBareGoogleADKExecution("coverage98-idle-parent"), []Run{child}, nil)
	if err != nil || len(responses) != 0 {
		t.Fatalf("idle child completion = %#v, %v", responses, err)
	}

	execution := newBareGoogleADKExecution("coverage98-approval-parent")
	execution.calls = []ToolCall{{ID: "coverage98-approval-call", RunID: child.ID, ToolName: "trade.submit", Status: "PENDING_APPROVAL"}}
	if err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, execution, []Run{child}, []workflowStep{{
		Title: "approval child", Message: "wait for approval",
	}}, []Approval{{ID: "coverage98-approval", RunID: child.ID, Status: ApprovalStatusPending}}); err != nil {
		t.Fatalf("approval-blocked child finalization: %v", err)
	}

	missingAgentExecution := newBareGoogleADKExecution("coverage98-missing-agent-parent")
	missingAgentExecution.calls = []ToolCall{{ID: "coverage98-missing-agent-call", RunID: child.ID, ToolName: "market.snapshot", Status: "SUCCEEDED"}}
	if err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, missingAgentExecution, []Run{child}, []workflowStep{{
		Title: "unavailable child", Message: "must surface lookup failure", ChildAgentID: "coverage98-missing-child-agent",
	}}, nil); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("missing child-agent finalization = %v", err)
	}
}
