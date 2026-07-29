package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowChildAdditionalCoverageBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("start child runs finishes already started children when a later child model is invalid", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-start-agent", Name: "Workflow Child Start", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child start")
		parent := mustSaveRun(t, runtime, Run{
			ID: "workflow-child-start-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		_, finishes, err := runtime.workflowExecutor().startWorkflowChildRuns(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, []workflowStep{
			{Title: "valid child", Message: "run valid child"},
			{Title: "invalid child", Message: "run invalid child", ChildProviderID: "missing-provider"},
		}, nil)
		if err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("startWorkflowChildRuns err = %v, want provider failure", err)
		}
		if finishes != nil {
			t.Fatalf("finishes = %#v, want nil after cleanup on later child failure", finishes)
		}
		runtime.activeMu.Lock()
		activeCount := len(runtime.activeRuns)
		runtime.activeMu.Unlock()
		if activeCount != 0 {
			t.Fatalf("active child executions = %d, want cleanup after later child startup failure", activeCount)
		}
	})

	t.Run("start child runs returns child start persistence failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-start-save-agent", Name: "Workflow Child Start Save", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child start save")
		parent := mustSaveRun(t, runtime, Run{
			ID: "workflow-child-start-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installFailTrigger(t, runtime, "fail_workflow_child_start_insert", tableRuns, "INSERT", "child start save failed")

		childRuns, finishes, err := runtime.workflowExecutor().startWorkflowChildRuns(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, []workflowStep{{Title: "child", Message: "run child"}}, nil)
		if err == nil || !strings.Contains(err.Error(), "child start save failed") {
			t.Fatalf("startWorkflowChildRuns err = %v, want child start save failure", err)
		}
		if childRuns != nil || finishes != nil {
			t.Fatalf("childRuns=%+v finishes=%+v, want nil after start failure", childRuns, finishes)
		}
	})

	t.Run("runChild returns child start persistence failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-run-save-agent", Name: "Workflow Child Run Save", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child run save")
		parent := mustSaveRun(t, runtime, Run{
			ID: "workflow-child-run-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installFailTrigger(t, runtime, "fail_workflow_child_run_insert", tableRuns, "INSERT", "run child start save failed")

		result := runtime.workflowExecutor().runChild(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, workflowStep{Title: "child", Message: "run child"}, Task{ID: "task-run-child-save"}, 1)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "run child start save failed") {
			t.Fatalf("runChild err = %v, want child start save failure", result.Err)
		}
		if result.Index != 0 || result.TaskID != "task-run-child-save" {
			t.Fatalf("runChild result = %+v", result)
		}
	})

	t.Run("workflow child activity detects reply reasoning without tools", func(t *testing.T) {
		if !workflowChildHasExecutionActivity(nil, Run{ID: "child"}, toolExecutionContext{}, nil, openAIChatResult{ReasoningContent: "thinking"}) {
			t.Fatal("reply reasoning should count as child execution activity")
		}
		if workflowChildHasExecutionActivity(nil, Run{ID: "child"}, toolExecutionContext{}, nil, openAIChatResult{}) {
			t.Fatal("empty child execution should not count as activity without observation")
		}
	})

	t.Run("complete child responses surface completion errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-complete-agent", Name: "Workflow Child Complete", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child complete")
		child := mustSaveRun(t, runtime, Run{
			ID:          "workflow-child-complete-run",
			SessionID:   session.ID,
			AgentID:     agent.ID,
			Status:      RunStatusRunning,
			UserMessage: "finish child",
			CreatedAt:   nowString(),
			UpdatedAt:   nowString(),
			Usage:       &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs: %v", err)
		}
		_, err := runtime.workflowExecutor().completeWorkflowChildrenFromADK(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, &googleADKExecution{
			calls: []ToolCall{{ID: "child-call", RunID: child.ID, ToolName: "tool", Status: "SUCCEEDED"}},
		}, []Run{child}, nil)
		if err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("completeWorkflowChildrenFromADK err = %v, want %s failure", err, tableRuns)
		}
	})

	t.Run("runChild surfaces parent save and completion failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-parent-save-agent", Name: "Workflow Child Parent Save", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child parent save")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "workflow-child-parent-save-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			WorkflowPlan:   []WorkflowStepState{{TaskID: "workflow-child-parent-save-task", Title: "child"}},
			CreatedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		installFailTrigger(t, runtime, "fail_workflow_child_parent_save_update", tableRuns, "UPDATE", "child parent save failed")
		result := runtime.workflowExecutor().runChild(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, workflowStep{Title: "child", Message: "run child"}, Task{ID: "workflow-child-parent-save-task"}, 1)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "child parent save failed") {
			t.Fatalf("runChild parent save err = %v", result.Err)
		}

		runtime = newTestRuntime(t)
		agent = mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-complete-error-agent", Name: "Workflow Child Complete Error", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session = mustCreateSession(t, runtime, agent.ID, "workflow child complete error")
		parent = mustSaveRun(t, runtime, Run{
			ID:             "workflow-child-complete-error-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			WorkflowPlan:   []WorkflowStepState{{TaskID: "workflow-child-complete-error-task", Title: "child"}},
			CreatedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER fail_workflow_child_complete_update
BEFORE UPDATE ON `+tableRuns+`
WHEN NEW.id != '`+parent.ID+`' AND NEW.status IN ('`+RunStatusCompleted+`', '`+RunStatusFailed+`')
BEGIN
  SELECT RAISE(FAIL, 'child completion save failed');
END;`); err != nil {
			t.Fatalf("create completion trigger: %v", err)
		}
		result = runtime.workflowExecutor().runChild(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, workflowStep{Title: "child", Message: "run child"}, Task{ID: "workflow-child-complete-error-task"}, 1)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "child completion save failed") {
			t.Fatalf("runChild completion err = %v", result.Err)
		}
	})

}
