package workflowexec

import (
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
)

func TestWorkflowTaskStorageFailuresFailClosed(t *testing.T) {
	ctx := t.Context()

	t.Run("planning exposes task persistence failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := (&WorkflowExecutor{runtime: runtime})
		agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
			ID: "coverage98-workflow-storage-agent", Name: "Workflow Storage", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeLoop,
		})
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-workflow-storage-parent", SessionID: "coverage98-workflow-storage-session", AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableTasks); err != nil {
			t.Fatalf("drop workflow task table: %v", err)
		}

		_, err := executor.PersistWorkflowTasks(ctx, parent, agent, []workflowStep{{
			Title: "Persist analysis", Message: "store the workflow task", Order: 1, WorkflowMode: WorkModeLoop,
		}})
		if err == nil || !strings.Contains(err.Error(), enginepersistence.TableTasks) {
			t.Fatalf("persistWorkflowTasks error = %v, want %s failure", err, enginepersistence.TableTasks)
		}
	})

	t.Run("completion never succeeds when scheduler storage is unavailable", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := (&WorkflowExecutor{runtime: runtime})
		agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
			ID: "coverage98-workflow-scheduler-agent", Name: "Workflow Scheduler", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow scheduler recovery")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-workflow-scheduler-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		knownDone := Task{ID: "coverage98-known-done", Status: "DONE", RunID: parent.ID}
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableTasks); err != nil {
			t.Fatalf("drop workflow task table: %v", err)
		}
		if executor.WorkflowTasksFinished(ctx, parent, []Task{knownDone}) {
			t.Fatal("WorkflowTasksFinished must fail closed when task storage cannot be queried")
		}

		response, err := executor.FinalizePlannedWorkflow(ctx, workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, parent, []Task{knownDone}, nil, nil)
		if err != nil {
			t.Fatalf("finalize planned workflow: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.ErrorCode != workflowTaskIncompleteErr {
			t.Fatalf("scheduler storage failure response = %+v", response.Run)
		}
		stored, ok, err := runtime.Store().Run(ctx, parent.ID)
		if err != nil || !ok || stored.Status != RunStatusFailed {
			t.Fatalf("stored failed parent = %+v ok=%v err=%v", stored, ok, err)
		}
	})
}

func TestWorkflowResponseIndexProtectsTaskState(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage98-workflow-index-parent", SessionID: "coverage98-workflow-index-session", AgentID: "coverage98-workflow-index-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "coverage98-workflow-index-task", Title: "Keep task ordering", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 1,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	if got := assistantmodel.WorkflowResponsePlanIndex(4, Run{}); got != 4 {
		t.Fatalf("response index without persisted iteration = %d, want fallback", got)
	}
	if got := assistantmodel.WorkflowResponsePlanIndex(4, Run{Iteration: 2}); got != 1 {
		t.Fatalf("response index with persisted iteration = %d, want 1", got)
	}
	executor.UpdateWorkflowTaskResult(ctx, []Task{task}, 3, Run{ID: "coverage98-out-of-range-child", Status: RunStatusCompleted}, "must not overwrite another task")
	unchanged, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok || unchanged.Status != "TODO" || unchanged.ResultSummary != "" {
		t.Fatalf("out-of-range task update changed task = %+v ok=%v err=%v", unchanged, ok, err)
	}

	executor.UpdateWorkflowTaskResult(ctx, []Task{task}, 0, Run{ID: "coverage98-blocked-child", Status: RunStatusFailed}, "  upstream execution failed  ")
	updated, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok || updated.Status != "BLOCKED" || updated.RunID != "coverage98-blocked-child" || updated.ResultSummary != "upstream execution failed" {
		t.Fatalf("failed child task projection = %+v ok=%v err=%v", updated, ok, err)
	}
}

func TestWorkflowExecutionSetupSurfacesRecoverableFailures(t *testing.T) {
	ctx := t.Context()

	t.Run("native graph start and execution setup failures become failed parent responses", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
			ID: "coverage98-workflow-native-agent", Name: "Native Workflow", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "native workflow failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-workflow-native-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		req := workflowRequest{Agent: agent, Session: session, Message: "start child", Mode: WorkModeLoop}
		response, err := (&WorkflowExecutor{runtime: runtime}).RunPlannedGoogleADKWorkflow(ctx, req, parent, []workflowStep{{
			Title: "Missing child", Message: "do not start", ChildAgentID: "coverage98-child-agent-missing",
		}}, nil)
		if err != nil || response.Run.Status != RunStatusFailed || !strings.Contains(response.Run.FailureReason, "agent not found") {
			t.Fatalf("native graph start failure = %+v err=%v", response, err)
		}

		badAgent := Agent{ID: "coverage98-workflow-missing-provider", ProviderID: "coverage98-provider-missing"}
		child := Run{ID: "coverage98-workflow-missing-provider-child", SessionID: session.ID, AgentID: badAgent.ID, ParentRunID: parent.ID, Status: RunStatusRunning, Usage: &RunUsage{}}
		if _, _, err := (&WorkflowExecutor{runtime: runtime}).RunWorkflowExecution(ctx, workflowRequest{Agent: badAgent, Session: session, Mode: WorkModeLoop}, parent, []Run{child}, []workflowStep{{Title: "Build child", Message: "must resolve provider"}}); err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("workflow execution setup error = %v, want provider failure", err)
		}
	})

	t.Run("snapshot and runner errors stop workflow preparation", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
			ID: "coverage98-workflow-snapshot-agent", Name: "Workflow Snapshot", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow snapshot failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-workflow-snapshot-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		_, err := (&WorkflowExecutor{runtime: runtime}).PrepareWorkflowParent(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop, EmitRun: true,
			OnDelta: func(ChatDelta) error { return errors.New("workflow snapshot delivery failed") },
		}, parent, nil)
		if err == nil || !strings.Contains(err.Error(), "snapshot delivery failed") {
			t.Fatalf("prepare workflow snapshot failure = %v", err)
		}

		execution := &fakeWorkflowExecutionHandle{runErr: errors.New("workflow runner interrupted")}
		if _, _, err := (&WorkflowExecutor{runtime: runtime}).ExecuteStartedWorkflowGraph(ctx, workflowRequest{Session: session}, parent, nil, nil, execution); err == nil || !strings.Contains(err.Error(), "runner interrupted") {
			t.Fatalf("started workflow runner error = %v", err)
		}
	})
}
