package assembly

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
)

type injectedWorkflowExecution struct {
	runErr error
}

func (s *injectedWorkflowExecution) Run(context.Context, assistantmodel.WorkflowRequest) (assistantmodel.ChatResponse, error) {
	if s != nil && s.runErr != nil {
		return assistantmodel.ChatResponse{}, s.runErr
	}
	return assistantmodel.ChatResponse{}, nil
}

func (s *injectedWorkflowExecution) FailParent(context.Context, assistantmodel.Run, error) (assistantmodel.Run, error) {
	return assistantmodel.Run{}, nil
}

func (s *injectedWorkflowExecution) ResumeLoopWorkflow(context.Context, assistantmodel.Session, assistantmodel.Run) (assistantmodel.Run, error) {
	return assistantmodel.Run{}, nil
}

func (s *injectedWorkflowExecution) ReconcileWorkflowChildren(context.Context, assistantmodel.Run) (assistantmodel.Run, bool, error) {
	return assistantmodel.Run{}, false, nil
}

func (s *injectedWorkflowExecution) CompleteResumedWorkflow(context.Context, assistantmodel.Session, assistantmodel.Run, string) (assistantmodel.Run, error) {
	return assistantmodel.Run{}, nil
}

func (s *injectedWorkflowExecution) ResumeADKGoalWorkflow(context.Context, assistantmodel.Session, assistantmodel.Agent, assistantmodel.Run) (assistantmodel.Run, error) {
	return assistantmodel.Run{}, nil
}

func (s *injectedWorkflowExecution) WorkflowTasks(context.Context, assistantmodel.Run, []assistantmodel.Task) ([]assistantmodel.Task, error) {
	return nil, nil
}

func (s *injectedWorkflowExecution) PersistWorkflowTasks(context.Context, assistantmodel.Run, assistantmodel.Agent, []assistantmodel.WorkflowStep) ([]assistantmodel.Task, error) {
	return nil, nil
}

func (s *injectedWorkflowExecution) RunPlannedGoogleADKWorkflow(context.Context, assistantmodel.WorkflowRequest, assistantmodel.Run, []assistantmodel.WorkflowStep, []assistantmodel.Task) (assistantmodel.ChatResponse, error) {
	return assistantmodel.ChatResponse{}, nil
}

func (s *injectedWorkflowExecution) WorkflowResponse(context.Context, assistantmodel.Session, assistantmodel.Run, assistantmodel.AssistantExecutionResult) assistantmodel.ChatResponse {
	return assistantmodel.ChatResponse{}
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
	if _, err := store.SaveProvider(ctx, assistantmodel.ProviderWriteRequest{
		ID: "inject-test", DisplayName: "Inject Test",
		Enabled: true, APIKey: "sk-test",
	}); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := store.SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "inject-agent", Name: "inject-agent", ProviderID: "inject-test",
		WorkMode: assistantmodel.WorkModeLoop,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	runtime := assistanttestkit.NewRuntimeWithSessionService(store, assistanttestkit.NewToolRegistry(), nil)
	sentinel := errors.New("injected workflow executor ran")
	runtime.SetWorkflowExecutor(&injectedWorkflowExecution{runErr: sentinel})
	_, err = runtime.ChatStream(ctx, assistantmodel.ChatRequest{
		AgentID: "inject-agent", Message: "run", WorkModeOverride: assistantmodel.WorkModeLoop,
	}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("ChatStream err = %v, want injected sentinel", err)
	}
}
