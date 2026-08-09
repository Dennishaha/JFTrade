package adk

import (
	"context"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

type WorkflowCanvasRunRequest struct {
	Workflow  WorkflowDefinition
	SessionID string
	Message   string
	Objective string
}

func (r *Runtime) RunCanvasWorkflow(ctx context.Context, req WorkflowCanvasRunRequest) (ChatResponse, error) {
	text, err := r.prepareChatRequest(ctx, ChatRequest{Message: req.Message})
	if err != nil {
		return ChatResponse{}, err
	}
	defer func() { <-r.runSem }()

	workflow := NormalizeWorkflowDefinition(req.Workflow)
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = text
	}
	steps, err := compileWorkflowCanvasSteps(workflow, text, objective)
	if err != nil {
		return ChatResponse{}, err
	}

	agent, err := r.resolveCanvasAgent(ctx, workflow)
	if err != nil {
		return ChatResponse{}, err
	}
	session, err := r.resolveSession(ctx, req.SessionID, agent, text)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := r.maybeAutoCompactSession(ctx, session, agent, text, nil); err != nil {
		return ChatResponse{}, err
	}

	return r.runCanvasWorkflowWithExecutor(ctx, session, agent, text, objective, steps)
}

func (r *Runtime) resolveCanvasAgent(ctx context.Context, workflow WorkflowDefinition) (Agent, error) {
	agent, err := r.resolveAgentDefinition(ctx, workflow.AgentID)
	if err != nil {
		return Agent{}, err
	}
	if providerID := strings.TrimSpace(workflow.ProviderID); providerID != "" {
		agent.ProviderID = providerID
	}
	if model := strings.TrimSpace(workflow.Model); model != "" {
		agent.Model = model
	}
	if permissionMode := strings.TrimSpace(workflow.PermissionMode); permissionMode != "" {
		if !validPermissionMode(permissionMode) {
			return Agent{}, fmt.Errorf("invalid permission mode %q", permissionMode)
		}
		agent.PermissionMode = normalizePermissionMode(permissionMode)
	}
	agent.WorkMode = WorkModeLoop
	agent, err = r.resolveAgentProvider(ctx, agent)
	if err != nil {
		return Agent{}, err
	}
	return r.prepareAgent(ctx, agent)
}

func (r *Runtime) runCanvasWorkflowWithExecutor(
	ctx context.Context,
	session Session,
	agent Agent,
	text string,
	objective string,
	steps []workflowStep,
) (ChatResponse, error) {
	executor, err := r.workflowExecutor()
	if err != nil {
		return ChatResponse{}, err
	}
	parent, parentCtx, finishParent, err := r.StartRunWithOptions(ctx, session.ID, agent, text, RunStartOptions{
		WorkMode:       WorkModeLoop,
		Objective:      objective,
		WorkflowStatus: workflowStatusRunning,
		WorkflowEngine: WorkflowEngineADK2Canvas,
	})
	if err != nil {
		return ChatResponse{}, err
	}
	defer finishParent()
	tasks, err := executor.PersistWorkflowTasks(parentCtx, parent, agent, steps)
	if err != nil {
		parent, persistErr := executor.FailParent(parentCtx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return executor.WorkflowResponse(parentCtx, session, parent, assistantExecutionResult{Reply: parent.FailureReason}), nil
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if err := r.store.SaveRun(parentCtx, parent); err != nil {
		parent, persistErr := executor.FailParent(parentCtx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return executor.WorkflowResponse(parentCtx, session, parent, assistantExecutionResult{Reply: parent.FailureReason}), nil
	}
	return executor.RunPlannedGoogleADKWorkflow(parentCtx, workflowRequest{
		Agent: agent, Session: session, Message: text, Mode: WorkModeLoop, Objective: objective,
	}, parent, steps, tasks)
}

func compileWorkflowCanvasSteps(workflow WorkflowDefinition, message string, objective string) ([]workflowStep, error) {
	return jfadkmodel.CompileWorkflowCanvasSteps(workflow, message, objective)
}
