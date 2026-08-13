package assembly

import (
	"context"
	"fmt"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
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
	Accepted bool                              `json:"accepted"`
	Workflow assistantmodel.WorkflowDefinition `json:"workflow"`
	Trigger  *assistantmodel.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      assistantmodel.WorkflowTriggerLog `json:"log"`
}

// WorkflowToolManager is the narrow workflow surface consumed by tool wiring.
type WorkflowToolManager interface {
	ListWorkflows(context.Context, string, int, int) (WorkflowToolPage[assistantmodel.WorkflowDefinition], error)
	GetWorkflow(context.Context, string) (assistantmodel.WorkflowDefinition, error)
	SaveWorkflow(context.Context, string, assistantmodel.WorkflowDefinitionWriteRequest) (assistantmodel.WorkflowDefinition, error)
	DeleteWorkflow(context.Context, string) (assistantmodel.WorkflowDefinition, error)
	ListWorkflowTriggers(context.Context, string) ([]assistantmodel.WorkflowTrigger, error)
	GetWorkflowTrigger(context.Context, string, string) (assistantmodel.WorkflowTrigger, error)
	SaveWorkflowTrigger(context.Context, string, string, assistantmodel.WorkflowTriggerWriteRequest) (assistantmodel.WorkflowTrigger, error)
	DeleteWorkflowTrigger(context.Context, string, string) (assistantmodel.WorkflowTrigger, error)
	ListWorkflowRuns(context.Context, string, string, string, int, int) (WorkflowToolPage[assistantmodel.WorkflowTriggerLog], error)
	GetWorkflowRun(context.Context, string) (assistantmodel.WorkflowTriggerLog, error)
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

func (m workflowManager) ListWorkflows(ctx context.Context, status string, limit int, offset int) (WorkflowToolPage[assistantmodel.WorkflowDefinition], error) {
	service, err := m.service()
	if err != nil {
		return WorkflowToolPage[assistantmodel.WorkflowDefinition]{}, err
	}
	page, err := service.ListWorkflows(ctx, assistant.WorkflowQuery{Status: status, Limit: limit, Offset: offset})
	return WorkflowToolPage[assistantmodel.WorkflowDefinition]{Items: page.Items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}, err
}

func (m workflowManager) GetWorkflow(ctx context.Context, workflowID string) (assistantmodel.WorkflowDefinition, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	return service.GetWorkflow(ctx, workflowID)
}

func (m workflowManager) SaveWorkflow(ctx context.Context, workflowID string, payload assistantmodel.WorkflowDefinitionWriteRequest) (assistantmodel.WorkflowDefinition, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	return service.SaveWorkflow(ctx, workflowID, payload)
}

func (m workflowManager) DeleteWorkflow(ctx context.Context, workflowID string) (assistantmodel.WorkflowDefinition, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowDefinition{}, err
	}
	return service.DeleteWorkflow(ctx, workflowID)
}

func (m workflowManager) ListWorkflowTriggers(ctx context.Context, workflowID string) ([]assistantmodel.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return nil, err
	}
	return service.ListWorkflowTriggers(ctx, workflowID)
}

func (m workflowManager) GetWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (assistantmodel.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	return service.GetWorkflowTrigger(ctx, workflowID, triggerID)
}

func (m workflowManager) SaveWorkflowTrigger(ctx context.Context, workflowID string, triggerID string, payload assistantmodel.WorkflowTriggerWriteRequest) (assistantmodel.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	result, err := service.SaveWorkflowTrigger(ctx, workflowID, triggerID, payload)
	return result.Trigger, err
}

func (m workflowManager) DeleteWorkflowTrigger(ctx context.Context, workflowID string, triggerID string) (assistantmodel.WorkflowTrigger, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowTrigger{}, err
	}
	return service.DeleteWorkflowTrigger(ctx, workflowID, triggerID)
}

func (m workflowManager) ListWorkflowRuns(ctx context.Context, workflowID string, triggerID string, status string, limit int, offset int) (WorkflowToolPage[assistantmodel.WorkflowTriggerLog], error) {
	service, err := m.service()
	if err != nil {
		return WorkflowToolPage[assistantmodel.WorkflowTriggerLog]{}, err
	}
	page, err := service.ListWorkflowTriggerLogs(ctx, assistant.WorkflowTriggerLogQuery{WorkflowID: workflowID, TriggerID: triggerID, Status: status, Limit: limit, Offset: offset})
	return WorkflowToolPage[assistantmodel.WorkflowTriggerLog]{Items: page.Items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}, err
}

func (m workflowManager) GetWorkflowRun(ctx context.Context, logID string) (assistantmodel.WorkflowTriggerLog, error) {
	service, err := m.service()
	if err != nil {
		return assistantmodel.WorkflowTriggerLog{}, err
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
