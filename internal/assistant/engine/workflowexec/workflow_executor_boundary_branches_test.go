package workflowexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowExecutorAdditionalBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := ((*WorkflowExecutor)(nil)).Run(ctx, workflowRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil workflow executor err = %v, want unavailable", err)
	}
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	if _, err := executor.Run(ctx, workflowRequest{Mode: WorkModeChat}); err == nil || !strings.Contains(err.Error(), "workflow mode") {
		t.Fatalf("chat workflow err = %v, want workflow mode required", err)
	}
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-executor-agent", Name: "Workflow Executor", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "workflow executor")
	if _, err := executor.Run(ctx, workflowRequest{
		Agent: agent, Session: session, Mode: WorkModeLoop, Message: "loop objective", Objective: "loop objective", EmitRun: true,
		OnDelta: func(ChatDelta) error { return errors.New("emit failed") },
	}); err == nil || !strings.Contains(err.Error(), "emit failed") {
		t.Fatalf("emit workflow err = %v, want emit failed", err)
	}
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-executor-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{
		{Title: "First", Description: "Desc", Message: "First message", DependencyID: "first", Order: 1, AgentRole: "researcher", ModeHint: WorkModeChat, PlanSource: jfadkmodel.WorkflowPlanSourcePlanner, WorkflowMode: WorkModeLoop},
		{Title: "Second", Message: "Second message", DependsOn: []string{"first", "__previous_step_1"}, DependencyID: "second", Order: 2, PlanSource: jfadkmodel.WorkflowPlanSourcePlanner, WorkflowMode: WorkModeLoop},
	}
	tasks, err := executor.PersistWorkflowTasks(ctx, parent, agent, steps)
	if err != nil {
		t.Fatalf("persistWorkflowTasks: %v", err)
	}
	if len(tasks) != 2 || tasks[1].DependsOn[0] != tasks[0].ID || !strings.Contains(tasks[0].Description, "Agent role: researcher") {
		t.Fatalf("persisted tasks = %+v", tasks)
	}
	failingParent := parent
	failingParent.ID = "workflow-executor-failing-parent"
	mustSaveRun(t, runtime, failingParent)
	response, err := executor.RunPlannedGoogleADKWorkflow(ctx, workflowRequest{
		Agent:   Agent{ID: "bad-child-agent", Name: "Bad Child", ProviderID: "missing-provider"},
		Session: session, Message: "run children", Mode: WorkModeLoop,
	}, failingParent, []workflowStep{{Title: "Bad", Message: "bad child"}}, nil)
	if err != nil || response.Run.Status != RunStatusFailed || response.Run.FailureReason == "" {
		t.Fatalf("runPlannedGoogleADKWorkflow response=%+v err=%v, want failed response", response, err)
	}
	if _, _, err := executor.StartWorkflowChildRuns(ctx, workflowRequest{
		Agent:   Agent{ID: "bad-child-agent", Name: "Bad Child", ProviderID: "missing-provider"},
		Session: session, Message: "run child", Mode: WorkModeLoop,
	}, parent, []workflowStep{{Title: "Bad", Message: "bad child"}}, nil); err == nil {
		t.Fatal("StartWorkflowChildRuns bad provider err = nil, want error")
	}
	ordered := []Task{
		{ID: "zero", Order: 0, CreatedAt: "b"},
		{ID: "two", Order: 2, CreatedAt: "a"},
		{ID: "one", Order: 1, CreatedAt: "c"},
		{ID: "zero-a", Order: 0, CreatedAt: "a"},
	}
	jfadkmodel.SortWorkflowTasks(ordered)
	if got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID}; strings.Join(got, ",") != "one,two,zero-a,zero" {
		t.Fatalf("sortWorkflowTasks order = %v", got)
	}
	if got := jfadkmodel.WorkflowDescriptionWithoutAgentRole("Agent role: only role"); got != "" {
		t.Fatalf("workflowDescriptionWithoutAgentRole prefix = %q, want empty", got)
	}
	if got := jfadkmodel.WorkflowDescriptionWithoutAgentRole("body\n\nAgent role: worker"); got != "body" {
		t.Fatalf("workflowDescriptionWithoutAgentRole suffix = %q, want body", got)
	}
	if got := jfadkmodel.WorkflowDescriptionWithoutAgentRole("body"); got != "body" {
		t.Fatalf("workflowDescriptionWithoutAgentRole plain = %q, want body", got)
	}
}

func TestWorkflowTaskModelsListUsesRuntimeProviderCatalog(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID: "disabled-provider", DisplayName: "Disabled Provider", BaseURL: "https://disabled.example/v1", Model: "cold-model", Enabled: false,
	}); err != nil {
		t.Fatalf("SaveProvider disabled: %v", err)
	}
	toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, "", "")
	result, err := toolset.ModelsList(map[string]any{"query": "test-model", "limit": 5})
	if err != nil {
		t.Fatalf("modelsList: %v", err)
	}
	models, ok := result["models"].([]map[string]any)
	if !ok || len(models) != 1 || models[0]["providerId"] != testProviderID || models[0]["callable"] != true {
		t.Fatalf("modelsList result = %#v", result)
	}
	if _, leaked := models[0]["apiKey"]; leaked {
		t.Fatalf("modelsList leaked api key: %#v", models[0])
	}
	result, err = toolset.ModelsList(map[string]any{"providerId": "disabled-provider", "callableOnly": "false"})
	if err != nil {
		t.Fatalf("modelsList disabled: %v", err)
	}
	models, ok = result["models"].([]map[string]any)
	if !ok || len(models) != 1 || models[0]["providerId"] != "disabled-provider" || models[0]["callable"] != false {
		t.Fatalf("disabled modelsList result = %#v", result)
	}
	if _, err := (*WorkflowTaskToolset)(nil).ModelsList(map[string]any{}); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
		t.Fatalf("nil modelsList err = %v", err)
	}
	if _, err := NewWorkflowTaskToolset(nil, "", "").ModelsList(map[string]any{}); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
		t.Fatalf("empty modelsList err = %v", err)
	}
}

func TestWorkflowModelsListToolNameAndClosedRuntime(t *testing.T) {
	runtime := newTestRuntime(t)
	toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, "", "")
	modelTool, err := toolset.ModelsListTool()
	if err != nil {
		t.Fatalf("modelsListTool: %v", err)
	}
	if modelTool.Name() != workflowModelsListTool {
		t.Fatalf("models tool name = %q, want %q", modelTool.Name(), workflowModelsListTool)
	}
	if err := runtime.Store().Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := toolset.ModelsList(map[string]any{"query": "test"}); err == nil {
		t.Fatal("modelsList closed runtime err = nil, want error")
	}
}
