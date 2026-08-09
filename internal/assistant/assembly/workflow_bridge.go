package assembly

import (
	"context"
	"fmt"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
)

// WorkflowToolPage is the transport-neutral page used by workflow tools.
type WorkflowToolPage[T any] struct {
	Items  []T
	Total  int
	Limit  int
	Offset int
}

// WorkflowToolStartResult is the stable result returned by workflow start tools.
type WorkflowToolStartResult struct {
	Accepted bool                     `json:"accepted"`
	Workflow jfadk.WorkflowDefinition `json:"workflow"`
	Trigger  *jfadk.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      jfadk.WorkflowTriggerLog `json:"log"`
}

// WorkflowToolManager is the narrow workflow surface consumed by tool wiring.
type WorkflowToolManager interface {
	ListWorkflows(context.Context, string, int, int) (WorkflowToolPage[jfadk.WorkflowDefinition], error)
	GetWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error)
	SaveWorkflow(context.Context, string, jfadk.WorkflowDefinitionWriteRequest) (jfadk.WorkflowDefinition, error)
	DeleteWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error)
	ListWorkflowTriggers(context.Context, string) ([]jfadk.WorkflowTrigger, error)
	GetWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error)
	SaveWorkflowTrigger(context.Context, string, string, jfadk.WorkflowTriggerWriteRequest) (jfadk.WorkflowTrigger, error)
	DeleteWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error)
	ListWorkflowRuns(context.Context, string, string, string, int, int) (WorkflowToolPage[jfadk.WorkflowTriggerLog], error)
	GetWorkflowRun(context.Context, string) (jfadk.WorkflowTriggerLog, error)
	StartWorkflow(context.Context, string, map[string]any) (WorkflowToolStartResult, error)
	StartWorkflowTrigger(context.Context, string, map[string]any) (WorkflowToolStartResult, error)
}

// WorkflowServiceProvider resolves the assistant facade after runtime assembly.
// Deferred resolution breaks the intentional cycle between tool registration
// and the service backed by the newly created runtime.
type WorkflowServiceProvider func() *assistant.Service

type workflowManager struct {
	serviceProvider WorkflowServiceProvider
}

// NewWorkflowToolManager bridges tool calls to the assistant service without
// exposing the application Server or any HTTP concerns.
func NewWorkflowToolManager(provider WorkflowServiceProvider) WorkflowToolManager {
	return workflowManager{serviceProvider: provider}
}

func (m workflowManager) service() (*assistant.Service, error) {
	if m.serviceProvider == nil {
		return nil, fmt.Errorf("workflow management is unavailable")
	}
	service := m.serviceProvider()
	if service == nil || !service.Available() {
		return nil, fmt.Errorf("workflow management is unavailable")
	}
	return service, nil
}

func (m workflowManager) ListWorkflows(ctx context.Context, status string, limit int, offset int) (WorkflowToolPage[jfadk.WorkflowDefinition], error) {
	service, err := m.service()
	if err != nil {
		return WorkflowToolPage[jfadk.WorkflowDefinition]{}, err
	}
	page, err := service.ListWorkflows(ctx, assistant.WorkflowQuery{Status: status, Limit: limit, Offset: offset})
	return WorkflowToolPage[jfadk.WorkflowDefinition]{Items: page.Items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}, err
}

func (m workflowManager) GetWorkflow(ctx context.Context, workflowID string) (jfadk.WorkflowDefinition, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	return service.GetWorkflow(ctx, workflowID)
}

func (m workflowManager) SaveWorkflow(ctx context.Context, workflowID string, payload jfadk.WorkflowDefinitionWriteRequest) (jfadk.WorkflowDefinition, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	return service.SaveWorkflow(ctx, workflowID, payload)
}

func (m workflowManager) DeleteWorkflow(ctx context.Context, workflowID string) (jfadk.WorkflowDefinition, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowDefinition{}, err
	}
	return service.DeleteWorkflow(ctx, workflowID)
}

func (m workflowManager) ListWorkflowTriggers(ctx context.Context, workflowID string) ([]jfadk.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return nil, err
	}
	return service.ListWorkflowTriggers(ctx, workflowID)
}

func (m workflowManager) GetWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (jfadk.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	return service.GetWorkflowTrigger(ctx, workflowID, triggerID)
}

func (m workflowManager) SaveWorkflowTrigger(ctx context.Context, workflowID string, triggerID string, payload jfadk.WorkflowTriggerWriteRequest) (jfadk.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	result, err := service.SaveWorkflowTrigger(ctx, workflowID, triggerID, payload)
	return result.Trigger, err
}

func (m workflowManager) DeleteWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (jfadk.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowTrigger{}, err
	}
	return service.DeleteWorkflowTrigger(ctx, workflowID, triggerID)
}

func (m workflowManager) ListWorkflowRuns(ctx context.Context, workflowID string, triggerID string, status string, limit int, offset int) (WorkflowToolPage[jfadk.WorkflowTriggerLog], error) {
	service, err := m.service()
	if err != nil {
		return WorkflowToolPage[jfadk.WorkflowTriggerLog]{}, err
	}
	page, err := service.ListWorkflowTriggerLogs(ctx, assistant.WorkflowTriggerLogQuery{WorkflowID: workflowID, TriggerID: triggerID, Status: status, Limit: limit, Offset: offset})
	return WorkflowToolPage[jfadk.WorkflowTriggerLog]{Items: page.Items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}, err
}

func (m workflowManager) GetWorkflowRun(ctx context.Context, logID string) (jfadk.WorkflowTriggerLog, error) {
	service, err := m.service()
	if err != nil {
		return jfadk.WorkflowTriggerLog{}, err
	}
	return service.GetWorkflowTriggerLog(ctx, logID)
}

func (m workflowManager) StartWorkflow(ctx context.Context, workflowID string, inputs map[string]any) (WorkflowToolStartResult, error) {
	service, err := m.service()
	if err != nil {
		return WorkflowToolStartResult{}, err
	}
	result, err := service.StartWorkflow(ctx, workflowID, inputs)
	return WorkflowToolStartResult{Accepted: result.Accepted, Workflow: result.Workflow, Trigger: result.Trigger, Log: result.Log}, err
}

func (m workflowManager) StartWorkflowTrigger(ctx context.Context, triggerID string, inputs map[string]any) (WorkflowToolStartResult, error) {
	service, err := m.service()
	if err != nil {
		return WorkflowToolStartResult{}, err
	}
	result, err := service.StartWorkflowTrigger(ctx, triggerID, inputs)
	return WorkflowToolStartResult{Accepted: result.Accepted, Workflow: result.Workflow, Trigger: result.Trigger, Log: result.Log}, err
}
