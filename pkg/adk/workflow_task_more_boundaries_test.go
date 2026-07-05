package adk

import (
	"strings"
	"testing"
)

func installRunWriteFailureTriggers(t *testing.T, runtime *Runtime, prefix string) {
	t.Helper()
	installFailTrigger(t, runtime, prefix+"_insert", tableRuns, "INSERT", "run save failed")
	installFailTrigger(t, runtime, prefix+"_update", tableRuns, "UPDATE", "run save failed")
}

func TestWorkflowTaskAdditionalSaveResumeAndIterationBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("run task and goal workflows fail when the initial run save fails", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		session := mustCreateSession(t, runtime, "workflow-save-fail-agent", "workflow save fail")
		now := nowString()

		taskParent := mustSaveRun(t, runtime, Run{
			ID: "run-task-save-fail", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		})
		goalParent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-save-fail", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		})
		installRunWriteFailureTriggers(t, runtime, "workflow_run_save_fail")

		taskResponse, err := executor.runADKTaskWorkflow(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, taskParent, nil)
		if err != nil {
			t.Fatalf("runADKTaskWorkflow save failure: %v", err)
		}
		if taskResponse.Run.ID != taskParent.ID || !strings.Contains(taskResponse.Reply, "run save failed") {
			t.Fatalf("task response = %+v, want save failure surfaced in reply", taskResponse)
		}

		goalResponse, err := executor.runADKGoalWorkflow(ctx, workflowRequest{Session: session, Mode: WorkModeLoop}, goalParent, nil)
		if err != nil {
			t.Fatalf("runADKGoalWorkflow save failure: %v", err)
		}
		if goalResponse.Run.ID != goalParent.ID || !strings.Contains(goalResponse.Reply, "run save failed") {
			t.Fatalf("goal response = %+v, want save failure surfaced in reply", goalResponse)
		}
	})

	t.Run("goal workflow fails when task execution init cannot resolve a provider", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		session := mustCreateSession(t, runtime, "goal-init-missing-provider", "goal init missing provider")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-init-missing-provider", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(),
		})
		response, err := executor.runADKGoalWorkflow(ctx, workflowRequest{
			Agent:   Agent{ID: session.AgentID, ProviderID: "missing-provider", Status: AgentStatusEnabled},
			Session: session,
			Mode:    WorkModeLoop,
		}, parent, nil)
		if err != nil {
			t.Fatalf("runADKGoalWorkflow missing provider: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("goal init failure response = %+v", response.Run)
		}
		if !strings.Contains(response.Reply, "provider") {
			t.Fatalf("goal init failure reply = %q, want provider failure", response.Reply)
		}
	})

	t.Run("continue goal workflow normalizes start iteration and pauses at the iteration limit", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "goal-iteration-limit-agent", Name: "Goal Iteration Limit", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal iteration limit")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-iteration-limit", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(),
		})
		response, err := executor.continueADKGoalWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, nil, "ignored", 0, 0)
		if err != nil {
			t.Fatalf("continueADKGoalWorkflow iteration limit: %v", err)
		}
		if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "iteration_limit" || response.Run.PausedReason != "iteration_limit" {
			t.Fatalf("iteration limit run = %+v, want paused iteration-limit run", response.Run)
		}
		if response.Run.Iteration != 0 || !strings.Contains(response.Reply, "运行上限") {
			t.Fatalf("iteration limit response = %+v", response)
		}
	})

	t.Run("resume goal and task workflows short-circuit when a child is still pending", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-blocked-agent", Name: "Resume Blocked", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume blocked")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-resume-blocked-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			ChildRunIDs:  []string{"run-resume-blocked-child"},
			WorkflowPlan: []WorkflowStepState{{Title: "需要审批的子任务", ChildRunID: "run-resume-blocked-child"}},
			CreatedAt:    now, UpdatedAt: now,
		})
		mustSaveRun(t, runtime, Run{
			ID: "run-resume-blocked-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusPending, Message: "waiting approval", CreatedAt: now, UpdatedAt: now,
		})

		resumedGoal, err := executor.resumeADKGoalWorkflow(ctx, session, agent, parent)
		if err != nil {
			t.Fatalf("resumeADKGoalWorkflow blocked: %v", err)
		}
		if resumedGoal.Status != RunStatusPending || resumedGoal.WorkflowStatus != workflowStatusPaused {
			t.Fatalf("blocked goal resume = %+v, want pending paused parent", resumedGoal)
		}

		taskParent := parent
		taskParent.ID = "run-resume-blocked-task-parent"
		taskParent.WorkMode = WorkModeTask
		taskParent.WorkflowPlan = []WorkflowStepState{{Title: "需要审批的子任务", ChildRunID: "run-resume-blocked-task-child"}}
		taskParent.ChildRunIDs = []string{"run-resume-blocked-task-child"}
		taskParent.UpdatedAt = nowString()
		mustSaveRun(t, runtime, taskParent)
		mustSaveRun(t, runtime, Run{
			ID: "run-resume-blocked-task-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: taskParent.ID,
			Status: RunStatusPending, Message: "waiting approval", CreatedAt: now, UpdatedAt: now,
		})

		resumedTask, err := executor.resumeADKTaskWorkflow(ctx, session, agent, taskParent)
		if err != nil {
			t.Fatalf("resumeADKTaskWorkflow blocked: %v", err)
		}
		if resumedTask.Status != RunStatusPending || resumedTask.WorkflowStatus != workflowStatusPaused {
			t.Fatalf("blocked task resume = %+v, want pending paused parent", resumedTask)
		}
	})

	t.Run("resume goal and task workflows return save errors before re-entering execution", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		session := mustCreateSession(t, runtime, "resume-save-fail-agent", "resume save fail")
		agent := Agent{ID: session.AgentID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop}
		now := nowString()

		goalParent := mustSaveRun(t, runtime, Run{
			ID: "run-resume-goal-save-fail", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			CreatedAt: now, UpdatedAt: now,
		})
		taskParent := mustSaveRun(t, runtime, Run{
			ID: "run-resume-task-save-fail", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusPaused, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusPaused,
			CreatedAt: now, UpdatedAt: now,
		})
		installRunWriteFailureTriggers(t, runtime, "resume_workflow_save_fail")

		if _, err := executor.resumeADKGoalWorkflow(ctx, session, agent, goalParent); err == nil || !strings.Contains(err.Error(), "run save failed") {
			t.Fatalf("resumeADKGoalWorkflow save err = %v", err)
		}
		if _, err := executor.resumeADKTaskWorkflow(ctx, session, agent, taskParent); err == nil || !strings.Contains(err.Error(), "run save failed") {
			t.Fatalf("resumeADKTaskWorkflow save err = %v", err)
		}
	})

	t.Run("models list functiontool surfaces runtime unavailability through Run", func(t *testing.T) {
		toolset := &workflowTaskToolset{}
		tool, err := toolset.modelsListTool()
		if err != nil {
			t.Fatalf("modelsListTool: %v", err)
		}
		runner, ok := tool.(workflowToolRunner)
		if !ok {
			t.Fatalf("modelsListTool runner type = %T", tool)
		}
		if _, err := runner.Run(newGoogleADKToolTestContext(), map[string]any{}); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
			t.Fatalf("modelsListTool run err = %v, want runtime unavailable", err)
		}
	})
}
