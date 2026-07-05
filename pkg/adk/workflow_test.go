package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

func TestWorkflowTaskStateMachineRejectsPrematureChildCompletion(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	child := mustSaveRun(t, runtime, Run{
		ID: "run-child-awaiting-approval", SessionID: "session-workflow-guard", AgentID: "agent",
		Status: RunStatusPending, Message: "waiting", CreatedAt: now, UpdatedAt: now,
	})
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-parent-workflow-guard", SessionID: child.SessionID, AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "task-child-guard", Title: "child task", Status: "BLOCKED", ChildRunID: child.ID}},
		ChildRunIDs:  []string{child.ID}, CreatedAt: now, UpdatedAt: now,
	})
	_, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-child-guard", Title: "child task", Status: "BLOCKED", AgentID: "agent",
		RunID: child.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	decision := &workflowGoalDecision{}
	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime}, parentID: parent.ID, currentTaskID: "task-child-guard",
		req: workflowRequest{Mode: WorkModeLoop, GoalDecision: decision},
	}
	result, err := toolset.complete(map[string]any{"taskId": "task-child-guard", "resultSummary": "pretend done"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if success, _ := result["success"].(bool); success {
		t.Fatalf("premature complete result = %+v", result)
	}
	task, ok, err := runtime.Store().Task(ctx, "task-child-guard")
	if err != nil || !ok {
		t.Fatalf("Task lookup ok=%v err=%v", ok, err)
	}
	if task.Status == "DONE" {
		t.Fatalf("pending child task was marked DONE")
	}
	goalResult, err := toolset.goalComplete(map[string]any{"summary": "pretend goal complete"})
	if err != nil {
		t.Fatalf("goalComplete: %v", err)
	}
	if success, _ := goalResult["success"].(bool); success || decision.snapshot().status == "complete" {
		t.Fatalf("premature goalComplete result=%+v decision=%+v", goalResult, decision.snapshot())
	}
}

func TestRunChildUsesStepProviderAndModel(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	childProviderID := saveGoalWorkflowProvider(t, runtime, "child-model-provider", func(openAIChatRequest) openAIChatMessage {
		return openAIChatMessage{Role: "assistant", Content: "子模型完成。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "child-provider-parent-agent", Name: "Child Provider Parent", Status: AgentStatusEnabled, WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "child provider override")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-child-provider-override", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "验证子模型选择", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-child-provider-override", Title: "子模型任务", Status: "IN_PROGRESS", AgentID: agent.ID,
		RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeTask, Objective: parent.Objective,
		ChildProviderID: childProviderID, ChildModel: "child-special-model",
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	result := (&WorkflowExecutor{runtime: runtime}).runChild(ctx, workflowRequest{
		Agent: agent, Session: session, Mode: WorkModeTask, Objective: parent.Objective,
	}, parent, workflowStep{
		Title: "子模型任务", Message: "直接回答", ChildProviderID: childProviderID, ChildModel: "child-special-model",
	}, task, 1)
	if result.Err != nil {
		t.Fatalf("runChild: %v", result.Err)
	}
	if result.Response.Run.Status != RunStatusCompleted {
		t.Fatalf("child run status = %q, want completed", result.Response.Run.Status)
	}
	if result.Response.Run.ProviderID != childProviderID || result.Response.Run.ProviderName != childProviderID || result.Response.Run.Model != "child-special-model" {
		t.Fatalf("child run model snapshot = %+v, want provider/model override", result.Response.Run)
	}
	stored, ok, err := runtime.Store().Run(ctx, result.Response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("stored child lookup ok=%v err=%v", ok, err)
	}
	if stored.ProviderID != childProviderID || stored.ProviderName != childProviderID || stored.Model != "child-special-model" {
		t.Fatalf("stored child model snapshot = %+v, want provider/model override", stored)
	}
}

func TestWorkflowTaskToolsExposeModelSelection(t *testing.T) {
	toolset := &workflowTaskToolset{req: workflowRequest{Mode: WorkModeTask}}
	tools, err := toolset.Tools(nil)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	if !names[workflowModelsListTool] {
		t.Fatalf("workflow task tools = %+v, want %s", names, workflowModelsListTool)
	}
	modelToolDeclaration := workflowToolDeclaration(t, tools, workflowModelsListTool)
	modelToolSchema := schemaMap(t, modelToolDeclaration.ParametersJsonSchema)
	modelToolProperties, ok := modelToolSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("workflow.models.list declaration properties missing: %+v", modelToolSchema)
	}
	for name, schema := range map[string]map[string]any{
		workflowTaskAddTool:      workflowTaskAddSchema(),
		workflowTaskDelegateTool: workflowTaskDelegateSchema(),
	} {
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s schema properties = %#v", name, schema["properties"])
		}
		for _, field := range []string{"childProviderId", "childModel"} {
			if _, ok := properties[field]; !ok {
				t.Fatalf("%s schema missing %s in %+v", name, field, properties)
			}
		}
	}
	modelProperties, ok := workflowModelsListSchema()["properties"].(map[string]any)
	if !ok {
		t.Fatalf("workflow.models.list schema properties missing")
	}
	for _, field := range []string{"query", "providerId", "callableOnly", "limit"} {
		if _, ok := modelProperties[field]; !ok {
			t.Fatalf("workflow.models.list schema missing %s in %+v", field, modelProperties)
		}
		if _, ok := modelToolProperties[field]; !ok {
			t.Fatalf("workflow.models.list declaration missing %s in %+v", field, modelToolProperties)
		}
	}
}

type workflowDeclaredTool interface {
	Declaration() *genai.FunctionDeclaration
}

func workflowToolDeclaration(t *testing.T, tools []adktool.Tool, name string) *genai.FunctionDeclaration {
	t.Helper()
	for _, item := range tools {
		if item.Name() != name {
			continue
		}
		declared, ok := item.(workflowDeclaredTool)
		if !ok {
			t.Fatalf("%s does not expose a declaration", name)
		}
		declaration := declared.Declaration()
		if declaration == nil {
			t.Fatalf("%s declaration is nil", name)
		}
		return declaration
	}
	t.Fatalf("%s not found", name)
	return nil
}

func schemaMap(t *testing.T, schema any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return out
}

func TestGoalWorkflowToolsAreIsolatedByPhase(t *testing.T) {
	decision := &workflowGoalDecision{}
	decision.reset()
	toolset := &workflowTaskToolset{req: workflowRequest{Mode: WorkModeLoop, GoalDecision: decision}}
	workTools, err := toolset.Tools(nil)
	if err != nil {
		t.Fatalf("work tools: %v", err)
	}
	workNames := map[string]bool{}
	for _, tool := range workTools {
		workNames[tool.Name()] = true
	}
	if !workNames[workflowTasksListTool] || workNames[workflowGoalCompleteTool] || workNames[workflowGoalContinueTool] {
		t.Fatalf("work phase tools = %+v, want only workflow task tools", workNames)
	}

	decision.beginDecision()
	decisionTools, err := toolset.Tools(nil)
	if err != nil {
		t.Fatalf("decision tools: %v", err)
	}
	decisionNames := map[string]bool{}
	for _, tool := range decisionTools {
		decisionNames[tool.Name()] = true
	}
	if len(decisionNames) != 2 || !decisionNames[workflowGoalCompleteTool] || !decisionNames[workflowGoalContinueTool] {
		t.Fatalf("decision phase tools = %+v, want only goal decision tools", decisionNames)
	}
}

func TestGoalCompletionWaitsForChildApprovalContinuation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	child := mustSaveRun(t, runtime, Run{
		ID: "run-child-still-resuming", SessionID: "session-goal-active-child", AgentID: "agent",
		Status: RunStatusCompleted, ParentRunID: "run-parent-waits-child", CreatedAt: now, UpdatedAt: now,
	})
	parent := mustSaveRun(t, runtime, Run{
		ID: child.ParentRunID, SessionID: child.SessionID, AgentID: "agent", Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, ChildRunIDs: []string{child.ID},
		WorkflowPlan: []WorkflowStepState{{TaskID: "task-child-still-resuming", ChildRunID: child.ID, Status: "DONE"}},
		CreatedAt:    now, UpdatedAt: now,
	})
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-child-still-resuming", Title: "child", Status: "DONE", AgentID: "agent",
		RunID: child.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeLoop,
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	runtime.approvalRuns[child.ID] = struct{}{}
	decision := &workflowGoalDecision{}
	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime}, parentID: parent.ID,
		req: workflowRequest{Mode: WorkModeLoop, GoalDecision: decision},
	}
	result, err := toolset.goalComplete(map[string]any{"summary": "too early"})
	if err != nil {
		t.Fatalf("goalComplete: %v", err)
	}
	if success, _ := result["success"].(bool); success || decision.snapshot().status == "complete" {
		t.Fatalf("goal completed during child continuation: result=%+v decision=%+v", result, decision.snapshot())
	}
}

func TestFailParentRefreshesAuthoritativeTaskPlan(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-parent-fail-refresh", SessionID: "session-fail-refresh", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "task-fail-refresh", Title: "finished task", Status: "BLOCKED"}},
		PendingApprovals: []Approval{
			{ID: "approval-resolved", Status: ApprovalStatusApproved},
			{ID: "approval-pending", Status: ApprovalStatusPending},
		},
		CreatedAt: now, UpdatedAt: now,
	})
	_, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-fail-refresh", Title: "finished task", Status: "DONE", AgentID: "agent",
		RunID: parent.ID, ResultSummary: "authoritative result", WorkflowMode: WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	failed := (&WorkflowExecutor{runtime: runtime}).failParent(ctx, parent, errors.New("provider failed"))
	if len(failed.WorkflowPlan) != 1 || failed.WorkflowPlan[0].Status != "DONE" || failed.WorkflowPlan[0].ResultSummary != "authoritative result" {
		t.Fatalf("failed workflow plan = %+v", failed.WorkflowPlan)
	}
	if len(failed.PendingApprovals) != 0 {
		t.Fatalf("failed parent pending approvals = %+v, want none on terminal parent", failed.PendingApprovals)
	}
}

func TestFailParentCancelsPendingChildrenAndDeniesApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	approval := Approval{ID: "approval-child-terminal", RunID: "run-child-terminal", AgentID: "agent", ToolName: "dangerous", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	child := mustSaveRun(t, runtime, Run{
		ID: approval.RunID, SessionID: "session-terminal-cascade", AgentID: "agent", Status: RunStatusPending,
		ParentRunID: "run-parent-terminal", PendingApprovals: []Approval{approval}, CreatedAt: now, UpdatedAt: now,
	})
	parent := mustSaveRun(t, runtime, Run{
		ID: child.ParentRunID, SessionID: child.SessionID, AgentID: "agent", Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: now, UpdatedAt: now,
	})

	failed := (&WorkflowExecutor{runtime: runtime}).failParent(ctx, parent, errors.New("provider failed"))
	if failed.Status != RunStatusFailed {
		t.Fatalf("parent status = %s", failed.Status)
	}
	storedChild, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok {
		t.Fatalf("child lookup ok=%v err=%v", ok, err)
	}
	if storedChild.Status != RunStatusCancelled || storedChild.ErrorCode != "PARENT_RUN_TERMINATED" || len(storedChild.PendingApprovals) != 0 {
		t.Fatalf("child after parent failure = %+v", storedChild)
	}
	storedApproval, ok, err := runtime.Store().Approval(ctx, approval.ID)
	if err != nil || !ok || storedApproval.Status != ApprovalStatusDenied {
		t.Fatalf("approval after parent failure = %+v ok=%v err=%v", storedApproval, ok, err)
	}
}

func TestCancelWorkflowParentCancelsChildrenDiscoveredByParentRunID(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-parent-cancel-by-parent-id", SessionID: "session-parent-id-cascade", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		// Historical data can miss ChildRunIDs while child.ParentRunID is still
		// present. User cancellation must still cascade to that child.
		ChildRunIDs: nil, CreatedAt: now, UpdatedAt: now,
	})
	approval := Approval{
		ID: "approval-child-parent-id-cascade", RunID: "run-child-parent-id-cascade", AgentID: "agent",
		ToolName: "dangerous", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	child := mustSaveRun(t, runtime, Run{
		ID: approval.RunID, SessionID: parent.SessionID, AgentID: "agent",
		ParentRunID: parent.ID, Status: RunStatusPending,
		PendingApprovals: []Approval{approval}, CreatedAt: now, UpdatedAt: now,
	})

	cancelled, err := runtime.CancelRun(ctx, parent.ID)
	if err != nil {
		t.Fatalf("CancelRun parent: %v", err)
	}
	if cancelled.Status != RunStatusCancelled {
		t.Fatalf("parent after cancel = %+v", cancelled)
	}
	storedChild, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok {
		t.Fatalf("child lookup ok=%v err=%v", ok, err)
	}
	if storedChild.Status != RunStatusCancelled || storedChild.ErrorCode != "RUN_CANCELLED" || len(storedChild.PendingApprovals) != 0 {
		t.Fatalf("child after parent cancel = %+v", storedChild)
	}
	storedApproval, ok, err := runtime.Store().Approval(ctx, approval.ID)
	if err != nil || !ok || storedApproval.Status != ApprovalStatusDenied {
		t.Fatalf("approval after parent cancel = %+v ok=%v err=%v", storedApproval, ok, err)
	}
}

func TestReconcileStaleRunsCancelsChildOfTerminalParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-parent-already-terminal", SessionID: "session-reconcile-terminal", AgentID: "agent",
		Status: RunStatusFailed, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusFailed, CreatedAt: now, UpdatedAt: now,
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "run-child-stale-approval", SessionID: parent.SessionID, AgentID: "agent", ParentRunID: parent.ID,
		Status: RunStatusPending, ResumeState: "waiting_approval", PendingApprovals: []Approval{{ID: "approval-stale-child", Status: ApprovalStatusPending}}, CreatedAt: now, UpdatedAt: now,
	})
	pausedChild := mustSaveRun(t, runtime, Run{
		ID: "run-child-stale-paused", SessionID: parent.SessionID, AgentID: "agent", ParentRunID: parent.ID,
		Status: RunStatusPaused, PausedReason: "user", CreatedAt: now, UpdatedAt: now,
	})

	runtime.reconcileStaleRuns(ctx)
	stored, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok {
		t.Fatalf("child lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusCancelled || stored.ErrorCode != "PARENT_RUN_TERMINATED" {
		t.Fatalf("reconciled child = %+v", stored)
	}
	storedPaused, ok, err := runtime.Store().Run(ctx, pausedChild.ID)
	if err != nil || !ok || storedPaused.Status != RunStatusCancelled || storedPaused.ErrorCode != "PARENT_RUN_TERMINATED" {
		t.Fatalf("reconciled paused child = %+v ok=%v err=%v", storedPaused, ok, err)
	}
}

func TestReconcileStaleRunsReopensCompletedRunningParentWithPendingChild(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	approval := Approval{
		ID: "approval-reconcile-reopen-child", RunID: "run-reconcile-reopen-child", AgentID: "agent",
		ToolName: "strategy.research_backtest", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now,
		FunctionCallID: "call-reconcile-reopen", ConfirmationCallID: "confirm-reconcile-reopen",
	}
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-reconcile-reopen-parent", SessionID: "session-reconcile-reopen", AgentID: "agent",
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Message: "running", Objective: "等待子审批", ChildRunIDs: []string{"run-reconcile-reopen-child"},
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "task-reconcile-reopen", Title: "需要审批的步骤", Status: "DONE", ChildRunID: "run-reconcile-reopen-child",
		}},
		CreatedAt: now, UpdatedAt: now,
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "run-reconcile-reopen-child", SessionID: parent.SessionID, AgentID: "agent", ParentRunID: parent.ID,
		Status: RunStatusPending, ResumeState: "waiting_approval", PendingApprovals: []Approval{approval},
		CreatedAt: now, UpdatedAt: now,
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	runtime.reconcileStaleRuns(ctx)
	storedParent, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("parent lookup ok=%v err=%v", ok, err)
	}
	if storedParent.Status != RunStatusPending || storedParent.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("reconciled parent = %+v, want reopened pending workflow", storedParent)
	}
	if len(storedParent.PendingApprovals) != 1 || storedParent.PendingApprovals[0].ID != approval.ID {
		t.Fatalf("parent pending approvals = %+v, want child approval", storedParent.PendingApprovals)
	}
	if got := storedParent.WorkflowPlan[0].Status; got != "BLOCKED" {
		t.Fatalf("workflow step status = %q, want BLOCKED", got)
	}
	storedChild, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok {
		t.Fatalf("child lookup ok=%v err=%v", ok, err)
	}
	if storedChild.Status != RunStatusPending || storedChild.ErrorCode == "PARENT_RUN_TERMINATED" {
		t.Fatalf("reconciled child = %+v, want pending child preserved", storedChild)
	}
}

func TestReconcileTerminalWorkflowRefreshesPlanAndClearsApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-terminal-refresh", SessionID: "session-terminal-refresh", AgentID: "agent",
		Status: RunStatusFailed, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusFailed,
		WorkflowPlan:     []WorkflowStepState{{TaskID: "task-terminal-refresh", Title: "task", Status: "BLOCKED"}},
		PendingApprovals: []Approval{{ID: "approval-terminal-resolved", Status: ApprovalStatusApproved}},
		CreatedAt:        now, UpdatedAt: now,
	})
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-terminal-refresh", Title: "task", Status: "DONE", AgentID: "agent",
		RunID: parent.ID, ResultSummary: "authoritative", WorkflowMode: WorkModeLoop,
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	runtime.reconcileStaleRuns(ctx)
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("parent lookup ok=%v err=%v", ok, err)
	}
	if len(stored.PendingApprovals) != 0 {
		t.Fatalf("terminal parent approvals = %+v", stored.PendingApprovals)
	}
	if len(stored.WorkflowPlan) != 1 || stored.WorkflowPlan[0].Status != "DONE" || stored.WorkflowPlan[0].ResultSummary != "authoritative" {
		t.Fatalf("terminal parent workflow plan = %+v", stored.WorkflowPlan)
	}
}

func TestSaveAgentNormalizesWorkflowDefaults(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-defaults", Name: "Workflow Defaults", Status: AgentStatusEnabled,
		LoopMaxIterations: 99,
	})
	if agent.WorkMode != WorkModeChat {
		t.Fatalf("work mode = %q, want %q", agent.WorkMode, WorkModeChat)
	}
	if agent.LoopMaxIterations != MaxLoopIterations {
		t.Fatalf("loop max iterations = %d, want %d", agent.LoopMaxIterations, MaxLoopIterations)
	}
	sequential := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-hidden-sequential", Name: "Workflow Hidden Sequential", Status: AgentStatusEnabled,
		WorkMode: "sequential",
	})
	if sequential.WorkMode != WorkModeChat {
		t.Fatalf("sequential default work mode = %q, want %q", sequential.WorkMode, WorkModeChat)
	}
	parallel := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-hidden-parallel", Name: "Workflow Hidden Parallel", Status: AgentStatusEnabled,
		WorkMode: "parallel",
	})
	if parallel.WorkMode != WorkModeChat {
		t.Fatalf("parallel default work mode = %q, want %q", parallel.WorkMode, WorkModeChat)
	}
	taskAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-task-agent", Name: "Workflow Task", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	if taskAgent.WorkMode != WorkModeTask {
		t.Fatalf("task default work mode = %q, want %q", taskAgent.WorkMode, WorkModeTask)
	}
}

func TestSequentialParallelWorkModesAreRejectedForRuns(t *testing.T) {
	if got := normalizeWorkMode("sequential"); got != WorkModeChat {
		t.Fatalf("normalizeWorkMode(sequential) = %q, want %q", got, WorkModeChat)
	}
	if got := normalizeWorkMode("parallel"); got != WorkModeChat {
		t.Fatalf("normalizeWorkMode(parallel) = %q, want %q", got, WorkModeChat)
	}
	for _, mode := range []string{"sequential", "parallel"} {
		if validWorkMode(mode) {
			t.Fatalf("validWorkMode(%q) = true, want false", mode)
		}
	}
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "invalid-work-mode-agent", Name: "Invalid Work Mode", Status: AgentStatusEnabled,
	})
	if _, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID: agent.ID, Message: "hello", WorkModeOverride: "sequential",
	}); err == nil || !strings.Contains(err.Error(), "invalid work mode") {
		t.Fatalf("Chat invalid sequential err = %v, want invalid work mode", err)
	}
}

func TestTaskWorkflowCanCompleteTODOWithoutChildRun(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "task-agent-self", Name: "Task Agent", Status: AgentStatusEnabled, WorkMode: WorkModeTask,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "整理一个执行清单",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task workflow: %v", err)
	}
	if response.Run.WorkMode != WorkModeTask || response.Run.WorkflowStatus != workflowStatusComplete {
		t.Fatalf("parent run = %+v, want completed task workflow", response.Run)
	}
	if len(response.Run.ChildRunIDs) != 0 {
		t.Fatalf("child run ids = %+v, want none for self-completed task", response.Run.ChildRunIDs)
	}
	if len(response.Run.WorkflowPlan) == 0 {
		t.Fatalf("workflow plan is empty")
	}
	step := response.Run.WorkflowPlan[0]
	if step.Status != "DONE" || step.Executor != workflowTaskExecutorSelf || step.ChildRunID != "" {
		t.Fatalf("workflow step = %+v, want self DONE without child run", step)
	}
}

func TestTaskWorkflowDelegatesChildRunOnlyWhenToolCalled(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "task-agent-child", Name: "Task Agent", Status: AgentStatusEnabled, WorkMode: WorkModeTask,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体完成行情分析",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat delegated task workflow: %v", err)
	}
	if response.Run.WorkMode != WorkModeTask || response.Run.WorkflowStatus != workflowStatusComplete {
		t.Fatalf("parent run = %+v, want completed task workflow", response.Run)
	}
	if len(response.Run.ChildRunIDs) != 1 {
		t.Fatalf("child run ids = %+v, want delegated child run", response.Run.ChildRunIDs)
	}
	if len(response.Run.WorkflowPlan) == 0 || response.Run.WorkflowPlan[0].ChildRunID != response.Run.ChildRunIDs[0] {
		t.Fatalf("workflow plan = %+v child ids=%+v, want child mapped to task", response.Run.WorkflowPlan, response.Run.ChildRunIDs)
	}
	if response.Run.WorkflowPlan[0].Executor != workflowTaskExecutorChild || response.Run.WorkflowPlan[0].Status != "DONE" {
		t.Fatalf("workflow step = %+v, want child DONE", response.Run.WorkflowPlan[0])
	}
	hasDelegateCall := false
	for _, call := range response.Run.ToolCalls {
		if call.ToolName == workflowTaskDelegateTool {
			hasDelegateCall = true
		}
		if call.RunID != response.Run.ID {
			t.Fatalf("parent tool call = %+v, want only parent-owned task tool calls", call)
		}
	}
	if !hasDelegateCall {
		t.Fatalf("parent tool calls = %+v, want %s", response.Run.ToolCalls, workflowTaskDelegateTool)
	}
	child, ok, err := runtime.Store().Run(context.Background(), response.Run.ChildRunIDs[0])
	if err != nil || !ok {
		t.Fatalf("child run lookup err=%v ok=%v", err, ok)
	}
	if child.ParentRunID != response.Run.ID || child.Status != RunStatusCompleted {
		t.Fatalf("child run = %+v, want completed child owned by parent", child)
	}
}

func TestTaskWorkflowStreamEmitsParentAfterChildRunCreated(t *testing.T) {
	ctx := context.Background()
	runtime, _ := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "task-agent-child-stream", Name: "Task Agent Child Stream", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})

	var runDeltas []Run
	response, err := runtime.ChatStream(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		WorkModeOverride: WorkModeTask,
	}, func(delta ChatDelta) error {
		if delta.Run != nil {
			runDeltas = append(runDeltas, *delta.Run)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream delegated task workflow: %v", err)
	}
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %+v, want child approval", response.PendingApprovals)
	}

	parentIndex := -1
	for index, delta := range runDeltas {
		if delta.ID != response.Run.ID {
			continue
		}
		if len(delta.ChildRunIDs) == 1 && len(delta.WorkflowPlan) > 0 && delta.WorkflowPlan[0].ChildRunID == delta.ChildRunIDs[0] {
			parentIndex = index
			break
		}
	}
	if parentIndex < 0 {
		t.Fatalf("run deltas = %+v, want parent delta with child run id before approval resolution", runDeltas)
	}
	if childID := response.PendingApprovals[0].RunID; childID == "" || runDeltas[parentIndex].ChildRunIDs[0] != childID {
		t.Fatalf("parent delta child ids = %+v approval=%+v, want pending child run", runDeltas[parentIndex].ChildRunIDs, response.PendingApprovals[0])
	}
}

func TestRuntimeWorkflowTaskAddRules(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "runtime-task-agent", Name: "Runtime Task", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "runtime tasks")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-runtime-tasks", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "动态任务", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	base, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "runtime-base", Title: "基础任务", Message: "先做基础任务", Status: "IN_PROGRESS",
		AgentID: agent.ID, RunID: parent.ID, Order: 1, WorkflowMode: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("SaveTask base: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{base}, nil)
	executor := &WorkflowExecutor{runtime: runtime}

	added, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{
		Title: "追加验证", Message: "验证新增条件",
	})
	if err != nil {
		t.Fatalf("addRuntimeWorkflowTask: %v", err)
	}
	if added.PlanSource != workflowPlanSourceRuntime || added.PlannerStepID != "runtime-1" || len(added.DependsOn) != 0 {
		t.Fatalf("runtime task = %+v, want runtime task without implicit dependency", added)
	}

	if _, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{Title: "坏依赖", DependsOn: []string{"missing-task"}}); err == nil {
		t.Fatal("addRuntimeWorkflowTask unknown dependency err = nil, want error")
	}
	for index := 2; index <= maxRuntimeWorkflowTasks; index++ {
		parent.WorkflowPlan = workflowPlanFromTasks(mustWorkflowTasks(t, runtime, parent), parent.WorkflowPlan)
		if _, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{Title: fmt.Sprintf("追加 %d", index)}); err != nil {
			t.Fatalf("addRuntimeWorkflowTask %d: %v", index, err)
		}
	}
	parent.WorkflowPlan = workflowPlanFromTasks(mustWorkflowTasks(t, runtime, parent), parent.WorkflowPlan)
	if _, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{Title: "超过上限"}); err == nil {
		t.Fatal("addRuntimeWorkflowTask over limit err = nil, want error")
	}
}

func TestClaimedRuntimeChildTaskDoesNotReuseParentRun(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "runtime-child-self-ref-agent", Name: "Runtime Child Self Ref", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "runtime child self ref")
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-runtime-child-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "delegate runtime child", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-runtime-child", Title: "child task", Message: "analyze child task", Status: "IN_PROGRESS",
		AgentID: agent.ID, RunID: parent.ID, Executor: workflowTaskExecutorChild,
		Order: 1, PlanSource: workflowPlanSourceRuntime, WorkflowMode: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, nil)
	if parent.WorkflowPlan[0].ChildRunID != "" {
		t.Fatalf("claimed task projected parent as child: %+v", parent.WorkflowPlan[0])
	}
	if err := runtime.Store().SaveRun(ctx, parent); err != nil {
		t.Fatalf("SaveRun parent: %v", err)
	}
	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime}, parentID: parent.ID,
		req: workflowRequest{Agent: agent, Session: session, Mode: WorkModeTask},
	}
	result, err := toolset.delegate(map[string]any{"taskId": task.ID, "prompt": "analyze child task"})
	if err != nil {
		t.Fatalf("delegate: %v", err)
	}
	childRunID := strings.TrimSpace(fmt.Sprint(result["childRunId"]))
	if childRunID == "" || childRunID == parent.ID {
		t.Fatalf("delegate result = %+v, want distinct child run", result)
	}
	if reused, _ := result["reused"].(bool); reused {
		t.Fatalf("delegate result = %+v, parent run must not be reused", result)
	}
	child, ok, err := runtime.Store().Run(ctx, childRunID)
	if err != nil || !ok {
		t.Fatalf("child lookup ok=%v err=%v", ok, err)
	}
	if child.ParentRunID != parent.ID {
		t.Fatalf("child parent = %q, want %q", child.ParentRunID, parent.ID)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok || storedTask.RunID != child.ID {
		t.Fatalf("stored task = %+v ok=%v err=%v", storedTask, ok, err)
	}
}

func TestDelegatePersistsChildModelSelectionWhenProviderInvalid(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "runtime-child-invalid-provider-agent", Name: "Runtime Child Invalid Provider", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "runtime child invalid provider")
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-runtime-child-invalid-provider", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "delegate runtime child with invalid provider", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-runtime-child-invalid-provider", Title: "child invalid provider", Message: "analyze child task", Status: "TODO",
		AgentID: agent.ID, RunID: parent.ID, Order: 1, PlanSource: workflowPlanSourceRuntime, WorkflowMode: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, nil)
	if err := runtime.Store().SaveRun(ctx, parent); err != nil {
		t.Fatalf("SaveRun parent: %v", err)
	}
	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime}, parentID: parent.ID,
		req: workflowRequest{Agent: agent, Session: session, Mode: WorkModeTask},
	}
	result, err := toolset.delegate(map[string]any{
		"taskId": task.ID, "prompt": "analyze child task",
		"childProviderId": "missing-child-provider", "childModel": "expensive-child-model",
	})
	if err != nil {
		t.Fatalf("delegate: %v", err)
	}
	if result["status"] != RunStatusFailed || result["result"] != "agent provider is unavailable" {
		t.Fatalf("delegate result = %+v, want failed child response with provider error", result)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
	}
	if storedTask.Status != "BLOCKED" || storedTask.ResultSummary != "agent provider is unavailable" {
		t.Fatalf("stored task = %+v, want blocked provider failure", storedTask)
	}
	if storedTask.ChildProviderID != "missing-child-provider" || storedTask.ChildModel != "expensive-child-model" {
		t.Fatalf("stored child model fields = %+v, want delegate arguments persisted", storedTask)
	}
}

func TestRepairWorkflowSelfReferenceMakesGoalResumable(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-self-reference-recovery", SessionID: "session-self-reference-recovery", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
		ChildRunIDs: []string{"run-self-reference-recovery"},
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "task-self-reference-recovery", Title: "child task", Status: "IN_PROGRESS",
			ChildRunID: "run-self-reference-recovery", Executor: workflowTaskExecutorChild,
		}},
		CreatedAt: now, UpdatedAt: now,
	})
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-self-reference-recovery", Title: "child task", Status: "IN_PROGRESS", AgentID: "agent",
		RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeLoop,
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if child, _, blocked := (&WorkflowExecutor{runtime: runtime}).firstBlockingTaskChild(ctx, parent); blocked {
		t.Fatalf("self reference treated as blocking child: %+v", child)
	}
	runtime.reconcileStaleRuns(ctx)
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPaused || stored.ResumeState != "self_reference_recovered" || len(stored.ChildRunIDs) != 0 || stored.WorkflowPlan[0].ChildRunID != "" {
		t.Fatalf("repaired parent = %+v", stored)
	}
	if err := validateUserGoalResumeRun(stored); err != nil {
		t.Fatalf("repaired goal is not resumable: %v", err)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, "task-self-reference-recovery")
	if err != nil || !ok || storedTask.Status != "TODO" || storedTask.Executor != "" {
		t.Fatalf("repaired task = %+v ok=%v err=%v", storedTask, ok, err)
	}
}

func mustWorkflowTasks(t *testing.T, runtime *Runtime, parent Run) []Task {
	t.Helper()
	tasks, err := (&WorkflowExecutor{runtime: runtime}).workflowTasks(context.Background(), parent, nil)
	if err != nil {
		t.Fatalf("workflowTasks: %v", err)
	}
	return tasks
}

func TestWorkflowChildInstructionTaskIsSelfContained(t *testing.T) {
	task := workflowChildInstructionTask(workflowStep{
		Objective:    "完成 TME 策略",
		Message:      "验证收益与频次",
		Description:  "检查年化收益、交易频次和过拟合风险。",
		AgentRole:    "验证与风控子 Agent",
		DependencyID: "__planner_step_3",
	})
	for _, want := range []string{"总体目标：完成 TME 策略", "当前子任务：验证收益与频次", "子任务说明：检查年化收益、交易频次和过拟合风险。", "子 Agent 角色：验证与风控子 Agent", "不要假设自己能看到父对话"} {
		if !strings.Contains(task, want) {
			t.Fatalf("workflow child task = %q, want to contain %q", task, want)
		}
	}
	restored := workflowStepFromState(WorkflowStepState{
		Title:       "验证",
		Description: "检查年化收益。\n\nAgent role: 验证与风控子 Agent",
		Message:     "验证收益",
		AgentRole:   "验证与风控子 Agent",
	})
	restoredTask := workflowChildInstructionTask(restored)
	if strings.Count(restoredTask, "验证与风控子 Agent") != 1 {
		t.Fatalf("restored workflow child task = %q, want role once", restoredTask)
	}
}

func TestWorkflowFinalSynthesisCompletesToolOnlyChildRun(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-final-synth-agent", Name: "Workflow Final Synth", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "final synth")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-final-synth", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask,
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "step-final-synth", Title: "读取数据后总结", Status: "IN_PROGRESS",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-final-synth", SessionID: session.ID, AgentID: agent.ID,
		ParentRunID: parent.ID, Status: RunStatusRunning, UserMessage: "读取数据后总结",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{{Title: "读取数据后总结", Message: "读取数据后总结"}}
	execution, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, []Run{child}, steps, WorkModeTask, RunOptions{}, nil)
	if err != nil {
		t.Fatalf("newGoogleADKWorkflowExecution: %v", err)
	}
	call := execution.ensureCallForRun("call-final-synth", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, child.ID)
	execution.finishCall(call.ID, map[string]any{"symbol": "TME", "close": 10.2}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-final-synth",
		Name:     "market.candles",
		Response: map[string]any{"symbol": "TME", "close": 10.2},
	})

	if !execution.runNeedsFinalSynthesis(child.ID) {
		t.Fatal("child run does not need final synthesis before post-tool text")
	}
	executor := WorkflowExecutor{runtime: runtime}
	err = executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{
		Agent: agent, Session: session, Message: child.UserMessage,
	}, execution, []Run{child}, steps, nil)
	if err != nil {
		t.Fatalf("ensureWorkflowChildrenFinalReplies: %v", err)
	}
	if execution.runNeedsFinalSynthesis(child.ID) || !execution.runHasPostToolText(child.ID) {
		t.Fatal("child run still lacks post-tool final text after synthesis")
	}
	if reply := execution.resultForRun(child.ID).Reply; strings.TrimSpace(reply) == "" || !strings.Contains(reply, "读取数据后总结") {
		t.Fatalf("synthesized reply = %q, want local final reply with child task", reply)
	}
}

func TestWorkflowFinalSynthesisSkipsChildrenWithPendingApproval(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-final-synth-approval-agent", Name: "Workflow Final Synth Approval", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "final synth approval")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-final-synth-approval", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask,
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "step-final-synth-approval", Title: "读取数据后审批", Status: "IN_PROGRESS",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-final-synth-approval", SessionID: session.ID, AgentID: agent.ID,
		ParentRunID: parent.ID, Status: RunStatusRunning, UserMessage: "读取数据后审批",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{{Title: "读取数据后审批", Message: "读取数据后审批"}}
	execution, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, []Run{child}, steps, WorkModeTask, RunOptions{}, nil)
	if err != nil {
		t.Fatalf("newGoogleADKWorkflowExecution: %v", err)
	}
	call := execution.ensureCallForRun("call-final-synth-approval", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, child.ID)
	execution.finishCall(call.ID, map[string]any{"symbol": "TME", "close": 10.2}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-final-synth-approval",
		Name:     "market.candles",
		Response: map[string]any{"symbol": "TME", "close": 10.2},
	})
	approval := Approval{
		ID: "approval-final-synth-child", RunID: child.ID, AgentID: agent.ID,
		ToolName: "strategy.save_draft", Status: ApprovalStatusPending,
		CreatedAt: nowString(), UpdatedAt: nowString(),
	}

	if !execution.runNeedsFinalSynthesis(child.ID) {
		t.Fatal("child run does not need final synthesis before approval")
	}
	executor := WorkflowExecutor{runtime: runtime}
	if err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{
		Agent: agent, Session: session, Message: child.UserMessage,
	}, execution, []Run{child}, steps, []Approval{approval}); err != nil {
		t.Fatalf("ensureWorkflowChildrenFinalReplies: %v", err)
	}
	if !execution.runNeedsFinalSynthesis(child.ID) {
		t.Fatal("pending approval child unexpectedly synthesized a final reply")
	}
	responses, err := executor.completeWorkflowChildrenFromADK(ctx, workflowRequest{
		Agent: agent, Session: session, Message: child.UserMessage,
	}, execution, []Run{child}, []Approval{approval})
	if err != nil {
		t.Fatalf("completeWorkflowChildrenFromADK: %v", err)
	}
	if len(responses) != 1 || responses[0].Run.Status != RunStatusPending {
		t.Fatalf("child response = %+v, want pending approval", responses)
	}
}

func newWorkflowApprovalRuntime(t *testing.T, mode string) (*Runtime, *atomic.Int32) {
	t.Helper()
	base := newTestRuntime(t)
	executions := &atomic.Int32{}
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions.Add(1)
		return map[string]any{"saved": true, "mode": mode}, nil
	})
	return newRuntimeWithRegistry(t, base.Store(), registry), executions
}

func waitForRunStatus(t *testing.T, runtime *Runtime, runID string, status string) Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		run, ok, err := runtime.Store().Run(context.Background(), runID)
		if err != nil || !ok {
			t.Fatalf("Run lookup err=%v ok=%v", err, ok)
		}
		if run.Status == status {
			return run
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s status = %q, want %q; run=%+v", runID, run.Status, status, run)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func runHasToolCall(run Run, toolName string) bool {
	for _, call := range run.ToolCalls {
		if call.ToolName == toolName {
			return true
		}
	}
	return false
}

func saveGoalWorkflowProvider(t *testing.T, runtime *Runtime, id string, message func(openAIChatRequest) openAIChatMessage) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		defer func() { jftradeCheckTestError(t, r.Body.Close()) }()
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		jftradeErr6 := json.NewEncoder(w).Encode(openAIChatResponse{
			Choices: []struct {
				Message openAIChatMessage `json:"message"`
			}{{Message: message(req)}},
		})
		jftradeCheckTestError(t, jftradeErr6)
	}))
	t.Cleanup(server.Close)
	provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: id, DisplayName: id, BaseURL: server.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	})
	return provider.ID
}

func testGoalWorkflowTaskProgressCalls(req openAIChatRequest) []openAIToolCall {
	seen := testProviderToolResponseNames(req.Messages)
	tools := testProviderToolNames(req)
	if containsTool(tools, workflowTasksListTool) && !seen[workflowTasksListTool] {
		return []openAIToolCall{testProviderToolCall("call-task-list", workflowTasksListTool, map[string]any{})}
	}
	if containsTool(tools, workflowTaskCompleteTool) && !seen[workflowTaskCompleteTool] {
		text := strings.ToLower(testProviderConversationText(req.Messages))
		return []openAIToolCall{testProviderToolCall("call-task-complete", workflowTaskCompleteTool, map[string]any{
			"taskId": testProviderTaskIDFromText(text), "resultSummary": "已完成一次目标推进。",
		})}
	}
	return nil
}

func testGoalWorkflowLastUserMessage(req openAIChatRequest) string {
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == "user" {
			return req.Messages[index].Content
		}
	}
	return ""
}

func testGoalWorkflowToolResponsesSinceLastUser(messages []openAIChatMessage) map[string]bool {
	names := map[string]bool{}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "user" {
			break
		}
		if message.Role != "tool" {
			continue
		}
		name := restoreToolNameFromOpenAI(message.Name)
		if name == "" {
			name = message.Name
		}
		names[name] = true
	}
	return names
}
