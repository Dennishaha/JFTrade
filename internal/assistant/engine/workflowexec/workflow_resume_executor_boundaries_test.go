package workflowexec

import (
	"context"
	"fmt"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"testing"
)

func TestResumeLoopWorkflowKeepsUserPausedParentPaused(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "resume-loop-paused-agent", Name: "Resume Loop Paused", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "resume loop paused")
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-resume-loop-paused", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
		Objective: "暂停中的目标", PausedReason: "user", ResumeState: "user_paused",
		WorkflowPlan: []WorkflowStepState{{TaskID: "task-resume-loop-paused", Title: "暂停任务", Status: "DONE"}},
		CreatedAt:    assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})

	updated, err := (&WorkflowExecutor{runtime: runtime}).ResumeLoopWorkflow(ctx, session, parent)
	if err != nil {
		t.Fatalf("resumeLoopWorkflow paused: %v", err)
	}
	if updated.Status != RunStatusPaused || updated.WorkflowStatus != workflowStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
		t.Fatalf("updated parent = %+v, want still user-paused", updated)
	}
}

func TestRunChildSurfacesDeltaSinkErrorsAfterChildLaunchSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "run-child-delta-error-agent", Name: "Run Child Delta Error", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "run child delta error")
	now := assistantmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "run-child-delta-error-parent",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		Objective:      "整理一个子任务",
		CreatedAt:      now,
		StartedAt:      now,
		UpdatedAt:      now,
		Usage:          &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID:           "task-run-child-delta-error",
		Title:        "整理一个子任务",
		Status:       "IN_PROGRESS",
		AgentID:      agent.ID,
		RunID:        parent.ID,
		Order:        1,
		WorkflowMode: WorkModeLoop,
		Objective:    parent.Objective,
		Message:      "整理这个子任务",
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = assistantmodel.WorkflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
	if err := runtime.Store().SaveRun(ctx, parent); err != nil {
		t.Fatalf("SaveRun parent with plan: %v", err)
	}
	wantErr := fmt.Errorf("delta sink closed")

	result := (&WorkflowExecutor{runtime: runtime}).RunChild(ctx, workflowRequest{
		Agent:   agent,
		Session: session,
		Mode:    WorkModeLoop,
		EmitRun: true,
		OnDelta: func(ChatDelta) error { return wantErr },
	}, parent, workflowStep{
		Title:   task.Title,
		Message: "整理这个子任务",
	}, task, 1)
	if result.Err == nil || result.Err.Error() != wantErr.Error() {
		t.Fatalf("RunChild delta error = %v, want %v", result.Err, wantErr)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("Task lookup ok=%v err=%v", ok, err)
	}
	if storedTask.Status != "IN_PROGRESS" || storedTask.RunID == "" {
		t.Fatalf("stored task = %+v, want child claimed with run id before delta error", storedTask)
	}
	child, ok, err := runtime.Store().Run(ctx, storedTask.RunID)
	if err != nil || !ok {
		t.Fatalf("child run lookup ok=%v err=%v", ok, err)
	}
	if child.ParentRunID != parent.ID || child.Status != RunStatusRunning {
		t.Fatalf("stored child = %+v, want still-running child after snapshot failure", child)
	}
}

func TestRunChildBlocksTaskWhenChildExecutionFailsImmediately(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "run-child-immediate-fail-agent", Name: "Run Child Immediate Fail", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "run child immediate fail")
	now := assistantmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "run-child-immediate-fail-parent",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		Objective:      "触发子运行立即失败",
		CreatedAt:      now,
		StartedAt:      now,
		UpdatedAt:      now,
		Usage:          &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID:           "task-run-child-immediate-fail",
		Title:        "失败子任务",
		Status:       "IN_PROGRESS",
		AgentID:      agent.ID,
		RunID:        parent.ID,
		Order:        1,
		WorkflowMode: WorkModeLoop,
		Objective:    parent.Objective,
		Message:      "执行会失败的子任务",
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = assistantmodel.WorkflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
	if err := runtime.Store().SaveRun(ctx, parent); err != nil {
		t.Fatalf("SaveRun parent with plan: %v", err)
	}
	badAgent := agent
	badAgent.ProviderID = "missing-run-child-provider"

	result := (&WorkflowExecutor{runtime: runtime}).RunChild(ctx, workflowRequest{
		Agent:     badAgent,
		Session:   session,
		Mode:      WorkModeLoop,
		Objective: parent.Objective,
	}, parent, workflowStep{
		Title:   task.Title,
		Message: "执行会失败的子任务",
	}, task, 1)
	if result.Err != nil {
		t.Fatalf("RunChild immediate fail err = %v, want nil with failed child response", result.Err)
	}
	if result.Response.Run.Status != RunStatusFailed || result.Response.Reply != "agent provider is unavailable" {
		t.Fatalf("child response = %+v, want failed child run with provider error", result.Response)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
	}
	if storedTask.Status != "BLOCKED" || storedTask.ResultSummary != "agent provider is unavailable" {
		t.Fatalf("stored task = %+v, want blocked task with child failure summary", storedTask)
	}
}

func TestResumeLoopWorkflowHonorsPauseRequestAfterChildCompletion(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "resume-loop-pause-request-agent", Name: "Resume Loop Pause Request", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "resume loop pause request")
	now := assistantmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:               "run-resume-loop-pause-request",
		SessionID:        session.ID,
		AgentID:          agent.ID,
		Status:           RunStatusRunning,
		Message:          "等待子运行",
		UserMessage:      "推进目标",
		WorkMode:         WorkModeLoop,
		Objective:        "推进目标",
		Iteration:        1,
		WorkflowStatus:   workflowStatusRunning,
		PauseRequestedAt: &now,
		ChildRunIDs:      []string{"run-resume-loop-pause-request-child"},
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "task-resume-loop-pause-request", Title: "子步骤", Message: "执行子步骤", Status: "IN_PROGRESS",
			ChildRunID: "run-resume-loop-pause-request-child",
		}},
		CreatedAt: now, StartedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-resume-loop-pause-request", Title: "子步骤", Status: "IN_PROGRESS", AgentID: agent.ID,
		RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeLoop,
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	completedAt := assistantmodel.NowString()
	mustSaveRun(t, runtime, Run{
		ID:          "run-resume-loop-pause-request-child",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		ParentRunID: parent.ID,
		Status:      RunStatusCompleted,
		Message:     "子运行完成",
		UserMessage: "执行子步骤",
		CompletedAt: &completedAt,
		CreatedAt:   now,
		StartedAt:   now,
		UpdatedAt:   completedAt,
		Usage:       &RunUsage{},
	})

	updated, err := (&WorkflowExecutor{runtime: runtime}).ResumeLoopWorkflow(ctx, session, parent)
	if err != nil {
		t.Fatalf("resumeLoopWorkflow pause request: %v", err)
	}
	if updated.Status != RunStatusPaused || updated.WorkflowStatus != workflowStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
		t.Fatalf("updated parent = %+v, want user-paused parent", updated)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("updated completed at = %v, want nil while pause request wins", *updated.CompletedAt)
	}
	if got := updated.WorkflowPlan[0].Status; got != "DONE" {
		t.Fatalf("workflow step status = %q, want DONE before parent pauses", got)
	}
	storedTask, ok, err := runtime.Store().Task(ctx, "task-resume-loop-pause-request")
	if err != nil || !ok {
		t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
	}
	if storedTask.Status != "DONE" || storedTask.ResultSummary != "子运行完成" {
		t.Fatalf("stored task = %+v, want completed child summary before pause", storedTask)
	}
}
