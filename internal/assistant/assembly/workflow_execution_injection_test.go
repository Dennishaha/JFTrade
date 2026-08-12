package assembly

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
)

type injectedWorkflowExecution struct {
	runErr error
}

func (s *injectedWorkflowExecution) Run(context.Context, jfadk.WorkflowRequest) (jfadk.ChatResponse, error) {
	if s != nil && s.runErr != nil {
		return jfadk.ChatResponse{}, s.runErr
	}
	return jfadk.ChatResponse{}, nil
}

func (s *injectedWorkflowExecution) FailParent(context.Context, jfadk.Run, error) (jfadk.Run, error) {
	return jfadk.Run{}, nil
}

func (s *injectedWorkflowExecution) ResumeLoopWorkflow(context.Context, jfadk.Session, jfadk.Run) (jfadk.Run, error) {
	return jfadk.Run{}, nil
}

func (s *injectedWorkflowExecution) ReconcileWorkflowChildren(context.Context, jfadk.Run) (jfadk.Run, bool, error) {
	return jfadk.Run{}, false, nil
}

func (s *injectedWorkflowExecution) CompleteResumedWorkflow(context.Context, jfadk.Session, jfadk.Run, string) (jfadk.Run, error) {
	return jfadk.Run{}, nil
}

func (s *injectedWorkflowExecution) ResumeADKGoalWorkflow(context.Context, jfadk.Session, jfadk.Agent, jfadk.Run) (jfadk.Run, error) {
	return jfadk.Run{}, nil
}

func (s *injectedWorkflowExecution) WorkflowTasks(context.Context, jfadk.Run, []jfadk.Task) ([]jfadk.Task, error) {
	return nil, nil
}

func (s *injectedWorkflowExecution) PersistWorkflowTasks(context.Context, jfadk.Run, jfadk.Agent, []jfadk.WorkflowStep) ([]jfadk.Task, error) {
	return nil, nil
}

func (s *injectedWorkflowExecution) RunPlannedGoogleADKWorkflow(context.Context, jfadk.WorkflowRequest, jfadk.Run, []jfadk.WorkflowStep, []jfadk.Task) (jfadk.ChatResponse, error) {
	return jfadk.ChatResponse{}, nil
}

func (s *injectedWorkflowExecution) WorkflowResponse(context.Context, jfadk.Session, jfadk.Run, jfadk.AssistantExecutionResult) jfadk.ChatResponse {
	return jfadk.ChatResponse{}
}

func TestRuntimeUsesInjectedWorkflowExecutionForLoopChat(t *testing.T) {
	paths := t.TempDir()
	store, err := assistanttestkit.NewStore(
		filepath.Join(paths, "runtime.sqlite"),
		filepath.Join(paths, "secrets"),
		filepath.Join(paths, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ctx := t.Context()
	if _, err := store.SaveProvider(ctx, jfadkmodel.ProviderWriteRequest{
		ID: "inject-test", DisplayName: "Inject Test",
		Enabled: true, APIKey: "sk-test",
	}); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := store.SaveAgent(ctx, jfadkmodel.AgentWriteRequest{
		ID: "inject-agent", Name: "inject-agent", ProviderID: "inject-test",
		WorkMode: jfadkmodel.WorkModeLoop,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	runtime := assistanttestkit.NewRuntimeWithSessionService(store, assistanttestkit.NewToolRegistry(), nil)
	sentinel := errors.New("injected workflow executor ran")
	runtime.SetWorkflowExecutor(&injectedWorkflowExecution{runErr: sentinel})
	_, err = runtime.ChatStream(ctx, jfadk.ChatRequest{
		AgentID: "inject-agent", Message: "run", WorkModeOverride: jfadkmodel.WorkModeLoop,
	}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("ChatStream err = %v, want injected sentinel", err)
	}
}
