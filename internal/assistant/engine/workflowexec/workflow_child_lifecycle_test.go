package workflowexec

import (
	"context"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
)

func TestWorkflowChildLifecycleBranches(t *testing.T) {
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
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		_, finishes, err := (&WorkflowExecutor{runtime: runtime}).StartWorkflowChildRuns(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, []workflowStep{
			{Title: "valid child", Message: "run valid child"},
			{Title: "invalid child", Message: "run invalid child", ChildProviderID: "missing-provider"},
		}, nil)
		if err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("StartWorkflowChildRuns err = %v, want provider failure", err)
		}
		if finishes != nil {
			t.Fatalf("finishes = %#v, want nil after cleanup on later child failure", finishes)
		}
		runs, listErr := runtime.Store().ListRuns(ctx)
		if listErr != nil {
			t.Fatalf("ListRuns: %v", listErr)
		}
		for _, run := range runs {
			if run.ParentRunID == parent.ID && runtime.RunExecutionInFlight(run.ID) {
				t.Fatalf("active child execution %s still in flight after later child startup failure", run.ID)
			}
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
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		installFailTrigger(t, runtime, "fail_workflow_child_start_insert", enginepersistence.TableRuns, "INSERT", "child start save failed")

		childRuns, finishes, err := (&WorkflowExecutor{runtime: runtime}).StartWorkflowChildRuns(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, []workflowStep{{Title: "child", Message: "run child"}}, nil)
		if err == nil || !strings.Contains(err.Error(), "child start save failed") {
			t.Fatalf("StartWorkflowChildRuns err = %v, want child start save failure", err)
		}
		if childRuns != nil || finishes != nil {
			t.Fatalf("childRuns=%+v finishes=%+v, want nil after start failure", childRuns, finishes)
		}
	})

	t.Run("RunChild returns child start persistence failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-run-save-agent", Name: "Workflow Child Run Save", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child run save")
		parent := mustSaveRun(t, runtime, Run{
			ID: "workflow-child-run-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		installFailTrigger(t, runtime, "fail_workflow_child_run_insert", enginepersistence.TableRuns, "INSERT", "run child start save failed")

		result := (&WorkflowExecutor{runtime: runtime}).RunChild(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, workflowStep{Title: "child", Message: "run child"}, Task{ID: "task-run-child-save"}, 1)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "run child start save failed") {
			t.Fatalf("RunChild err = %v, want child start save failure", result.Err)
		}
		if result.Index != 0 || result.TaskID != "task-run-child-save" {
			t.Fatalf("RunChild result = %+v", result)
		}
	})

	t.Run("workflow child activity detects reply reasoning without tools", func(t *testing.T) {
		if !workflowChildHasExecutionActivity(nil, Run{ID: "child"}, nil, jfadk.AssistantExecutionResult{ReasoningContent: "thinking"}) {
			t.Fatal("reply reasoning should count as child execution activity")
		}
		if workflowChildHasExecutionActivity(nil, Run{ID: "child"}, nil, jfadk.AssistantExecutionResult{}) {
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
			CreatedAt:   assistantmodel.NowString(),
			UpdatedAt:   assistantmodel.NowString(),
			Usage:       &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableRuns); err != nil {
			t.Fatalf("drop runs: %v", err)
		}
		_, err := (&WorkflowExecutor{runtime: runtime}).CompleteWorkflowChildrenFromADK(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, &fakeWorkflowExecutionHandle{
			calls: []ToolCall{{ID: "child-call", RunID: child.ID, ToolName: "tool", Status: "SUCCEEDED"}},
		}, []Run{child}, nil)
		if err == nil || !strings.Contains(err.Error(), enginepersistence.TableRuns) {
			t.Fatalf("CompleteWorkflowChildrenFromADK err = %v, want %s failure", err, enginepersistence.TableRuns)
		}
	})

	t.Run("RunChild surfaces parent save and completion failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-child-parent-save-agent", Name: "Workflow Child Parent Save", Status: AgentStatusEnabled,
			ProviderID: testProviderID, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow child parent save")
		now := assistantmodel.NowString()
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
		installFailTrigger(t, runtime, "fail_workflow_child_parent_save_update", enginepersistence.TableRuns, "UPDATE", "child parent save failed")
		result := (&WorkflowExecutor{runtime: runtime}).RunChild(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, workflowStep{Title: "child", Message: "run child"}, Task{ID: "workflow-child-parent-save-task"}, 1)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "child parent save failed") {
			t.Fatalf("RunChild parent save err = %v", result.Err)
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
		if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER fail_workflow_child_complete_update
BEFORE UPDATE ON `+enginepersistence.TableRuns+`
WHEN NEW.id != '`+parent.ID+`' AND NEW.status IN ('`+RunStatusCompleted+`', '`+RunStatusFailed+`')
BEGIN
  SELECT RAISE(FAIL, 'child completion save failed');
END;`); err != nil {
			t.Fatalf("create completion trigger: %v", err)
		}
		result = (&WorkflowExecutor{runtime: runtime}).RunChild(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, workflowStep{Title: "child", Message: "run child"}, Task{ID: "workflow-child-complete-error-task"}, 1)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "child completion save failed") {
			t.Fatalf("RunChild completion err = %v", result.Err)
		}
	})
}
