package servercore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

type WorkflowToolPage[T any] struct {
	Items  []T
	Total  int
	Limit  int
	Offset int
}

type WorkflowToolStartResult struct {
	Accepted bool                     `json:"accepted"`
	Workflow jfadk.WorkflowDefinition `json:"workflow"`
	Trigger  *jfadk.WorkflowTrigger   `json:"trigger,omitempty"`
	Log      jfadk.WorkflowTriggerLog `json:"log"`
}

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

func registerJFTradeADKWorkflowManagementTools(store *jfadk.Store, registry *jfadk.ToolRegistry, manager WorkflowToolManager) {
	registerWorkflowDefinitionTools(registry, manager)
	registerWorkflowTriggerTools(registry, manager)
	registerWorkflowRunTools(store, registry, manager)
}

func registerWorkflowDefinitionTools(registry *jfadk.ToolRegistry, manager WorkflowToolManager) {
	registry.Register(workflowReadToolDescriptor("workflows.list", "工作流列表", "分页列出 ADK 产品工作流摘要。", "工作流摘要和分页信息。"), func(ctx context.Context, input map[string]any) (any, error) {
		limit, offset := workflowToolPageBounds(input)
		page, err := workflowToolManagerRequired(manager).ListWorkflows(ctx, stringValue(input, "status"), limit, offset)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, workflow := range page.Items {
			items = append(items, workflowToolSummary(workflow))
		}
		return map[string]any{"workflows": items, "page": pageEnvelope(page.Limit, page.Offset, page.Total, len(items))}, nil
	})
	registry.Register(workflowReadToolDescriptor("workflows.get", "读取工作流", "按 workflowId 读取完整工作流定义。", "完整工作流定义。"), func(ctx context.Context, input map[string]any) (any, error) {
		return workflowToolManagerRequired(manager).GetWorkflow(ctx, stringValue(input, "workflowId"))
	})
	registry.Register(workflowWriteToolDescriptor("workflows.create", "创建工作流", "创建 ADK 产品工作流；默认沿用现有工作流校验和审计。", "创建后的完整工作流定义。"), func(ctx context.Context, input map[string]any) (any, error) {
		payload, err := workflowCreateRequest(input)
		if err != nil {
			return nil, err
		}
		return workflowToolManagerRequired(manager).SaveWorkflow(ctx, "", payload)
	})
	registry.Register(workflowWriteToolDescriptor("workflows.update", "更新工作流", "按 workflowId 补丁更新工作流；未提供字段保持不变。", "更新后的完整工作流定义。"), func(ctx context.Context, input map[string]any) (any, error) {
		workflowID := stringValue(input, "workflowId")
		current, err := workflowToolManagerRequired(manager).GetWorkflow(ctx, workflowID)
		if err != nil {
			return nil, err
		}
		payload, err := workflowUpdateRequest(current, input)
		if err != nil {
			return nil, err
		}
		return workflowToolManagerRequired(manager).SaveWorkflow(ctx, workflowID, payload)
	})
	registry.Register(workflowWriteToolDescriptor("workflows.delete", "删除工作流", "按 workflowId 禁用并软删除工作流。", "删除标记和被删除的工作流。"), func(ctx context.Context, input map[string]any) (any, error) {
		workflow, err := workflowToolManagerRequired(manager).DeleteWorkflow(ctx, stringValue(input, "workflowId"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "workflow": workflow}, nil
	})
}

func registerWorkflowTriggerTools(registry *jfadk.ToolRegistry, manager WorkflowToolManager) {
	registry.Register(workflowReadToolDescriptor("workflow_triggers.list", "触发器列表", "列出指定工作流的触发器；不会返回 Webhook secret hash。", "脱敏后的工作流触发器。"), func(ctx context.Context, input map[string]any) (any, error) {
		triggers, err := workflowToolManagerRequired(manager).ListWorkflowTriggers(ctx, stringValue(input, "workflowId"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"triggers": triggers}, nil
	})
	registry.Register(workflowReadToolDescriptor("workflow_triggers.get", "读取触发器", "按 workflowId 和 triggerId 读取脱敏后的触发器。", "脱敏后的完整触发器。"), func(ctx context.Context, input map[string]any) (any, error) {
		return workflowToolManagerRequired(manager).GetWorkflowTrigger(ctx, stringValue(input, "workflowId"), stringValue(input, "triggerId"))
	})
	registry.Register(workflowWriteToolDescriptor("workflow_triggers.create", "创建触发器", "为工作流创建非 Webhook 触发器；Webhook 密钥操作仅允许通过 UI/API。", "创建后的脱敏触发器。"), func(ctx context.Context, input map[string]any) (any, error) {
		payload := workflowTriggerCreateRequest(input)
		if payload.Type == jfadk.WorkflowTriggerTypeWebhook {
			return nil, fmt.Errorf("webhook triggers must be created through the UI/API")
		}
		return workflowToolManagerRequired(manager).SaveWorkflowTrigger(ctx, stringValue(input, "workflowId"), "", payload)
	})
	registry.Register(workflowWriteToolDescriptor("workflow_triggers.update", "更新触发器", "补丁更新触发器元数据；禁止通过 tool 创建或重置 Webhook secret。", "更新后的脱敏触发器。"), func(ctx context.Context, input map[string]any) (any, error) {
		workflowID, triggerID := stringValue(input, "workflowId"), stringValue(input, "triggerId")
		current, err := workflowToolManagerRequired(manager).GetWorkflowTrigger(ctx, workflowID, triggerID)
		if err != nil {
			return nil, err
		}
		payload, err := workflowTriggerUpdateRequest(current, input)
		if err != nil {
			return nil, err
		}
		return workflowToolManagerRequired(manager).SaveWorkflowTrigger(ctx, workflowID, triggerID, payload)
	})
	registry.Register(workflowWriteToolDescriptor("workflow_triggers.delete", "删除触发器", "按 workflowId 和 triggerId 禁用并软删除触发器。", "删除标记和脱敏后的触发器。"), func(ctx context.Context, input map[string]any) (any, error) {
		trigger, err := workflowToolManagerRequired(manager).DeleteWorkflowTrigger(ctx, stringValue(input, "workflowId"), stringValue(input, "triggerId"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "trigger": trigger}, nil
	})
}

func registerWorkflowRunTools(store *jfadk.Store, registry *jfadk.ToolRegistry, manager WorkflowToolManager) {
	registry.Register(workflowRunToolDescriptor("workflows.run", "运行工作流", "从普通交互会话异步启动工作流，立即返回 QUEUED 日志。", "是否已接受以及工作流运行日志。"), func(ctx context.Context, input map[string]any) (any, error) {
		if err := requireInteractiveWorkflowToolSession(ctx, store); err != nil {
			return nil, err
		}
		return workflowToolManagerRequired(manager).StartWorkflow(ctx, stringValue(input, "workflowId"), toolObjectValue(input, "inputs"))
	})
	registry.Register(workflowRunToolDescriptor("workflow_triggers.run", "运行触发器", "从普通交互会话异步运行已启用触发器，立即返回 QUEUED 或 SKIPPED 日志。", "是否已接受以及触发器运行日志。"), func(ctx context.Context, input map[string]any) (any, error) {
		if err := requireInteractiveWorkflowToolSession(ctx, store); err != nil {
			return nil, err
		}
		return workflowToolManagerRequired(manager).StartWorkflowTrigger(ctx, stringValue(input, "triggerId"), toolObjectValue(input, "inputs"))
	})
	registry.Register(workflowReadToolDescriptor("workflow_runs.list", "工作流运行列表", "分页查询工作流运行日志摘要。", "运行摘要和分页信息。"), func(ctx context.Context, input map[string]any) (any, error) {
		limit, offset := workflowToolPageBounds(input)
		page, err := workflowToolManagerRequired(manager).ListWorkflowRuns(ctx, stringValue(input, "workflowId"), stringValue(input, "triggerId"), stringValue(input, "status"), limit, offset)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, log := range page.Items {
			items = append(items, workflowRunToolSummary(log))
		}
		return map[string]any{"runs": items, "page": pageEnvelope(page.Limit, page.Offset, page.Total, len(items))}, nil
	})
	registry.Register(workflowReadToolDescriptor("workflow_runs.get", "读取工作流运行", "按 logId 读取完整工作流运行日志、节点和结果。", "完整工作流运行日志。"), func(ctx context.Context, input map[string]any) (any, error) {
		return workflowToolManagerRequired(manager).GetWorkflowRun(ctx, stringValue(input, "logId"))
	})
}

func workflowReadToolDescriptor(name string, displayName string, description string, output string) jfadk.ToolDescriptor {
	return jfadk.ToolDescriptor{Name: name, DisplayName: displayName, Description: description, Category: "workflow", Permission: "read_internal", RiskLevel: "low", OutputSummary: output, RequiredSkill: jfadk.WorkflowManagementSkillName}
}

func workflowWriteToolDescriptor(name string, displayName string, description string, output string) jfadk.ToolDescriptor {
	return jfadk.ToolDescriptor{Name: name, DisplayName: displayName, Description: description, Category: "workflow", Permission: "write_workflow", RiskLevel: "high", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: output, RequiredSkill: jfadk.WorkflowManagementSkillName}
}

func workflowRunToolDescriptor(name string, displayName string, description string, output string) jfadk.ToolDescriptor {
	return jfadk.ToolDescriptor{Name: name, DisplayName: displayName, Description: description, Category: "workflow", Permission: "execute_workflow", RiskLevel: "high", RequiresApprovalIn: []string{jfadk.PermissionModeApproval}, OutputSummary: output, RequiredSkill: jfadk.WorkflowManagementSkillName}
}

func workflowToolManagerRequired(manager WorkflowToolManager) WorkflowToolManager {
	if manager == nil {
		return unavailableWorkflowToolManager{}
	}
	return manager
}

type unavailableWorkflowToolManager struct{}

func (unavailableWorkflowToolManager) unavailable() error {
	return fmt.Errorf("workflow management is unavailable")
}
func (m unavailableWorkflowToolManager) ListWorkflows(context.Context, string, int, int) (WorkflowToolPage[jfadk.WorkflowDefinition], error) {
	return WorkflowToolPage[jfadk.WorkflowDefinition]{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) GetWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error) {
	return jfadk.WorkflowDefinition{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) SaveWorkflow(context.Context, string, jfadk.WorkflowDefinitionWriteRequest) (jfadk.WorkflowDefinition, error) {
	return jfadk.WorkflowDefinition{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) DeleteWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error) {
	return jfadk.WorkflowDefinition{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) ListWorkflowTriggers(context.Context, string) ([]jfadk.WorkflowTrigger, error) {
	return nil, m.unavailable()
}
func (m unavailableWorkflowToolManager) GetWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error) {
	return jfadk.WorkflowTrigger{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) SaveWorkflowTrigger(context.Context, string, string, jfadk.WorkflowTriggerWriteRequest) (jfadk.WorkflowTrigger, error) {
	return jfadk.WorkflowTrigger{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) DeleteWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error) {
	return jfadk.WorkflowTrigger{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) ListWorkflowRuns(context.Context, string, string, string, int, int) (WorkflowToolPage[jfadk.WorkflowTriggerLog], error) {
	return WorkflowToolPage[jfadk.WorkflowTriggerLog]{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) GetWorkflowRun(context.Context, string) (jfadk.WorkflowTriggerLog, error) {
	return jfadk.WorkflowTriggerLog{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) StartWorkflow(context.Context, string, map[string]any) (WorkflowToolStartResult, error) {
	return WorkflowToolStartResult{}, m.unavailable()
}
func (m unavailableWorkflowToolManager) StartWorkflowTrigger(context.Context, string, map[string]any) (WorkflowToolStartResult, error) {
	return WorkflowToolStartResult{}, m.unavailable()
}

func workflowToolPageBounds(input map[string]any) (int, int) {
	return httpserver.NormalizeBoundPage(intValue(input, "limit", 20), intValue(input, "offset", 0), 20, 100)
}

func workflowToolSummary(workflow jfadk.WorkflowDefinition) map[string]any {
	return map[string]any{"id": workflow.ID, "name": workflow.Name, "description": workflow.Description, "status": workflow.Status, "agentId": workflow.AgentID, "workMode": workflow.WorkMode, "permissionMode": workflow.PermissionMode, "tags": workflow.Tags, "builtinTemplate": workflow.BuiltinTemplate, "updatedAt": workflow.UpdatedAt}
}

func workflowRunToolSummary(log jfadk.WorkflowTriggerLog) map[string]any {
	return map[string]any{"id": log.ID, "workflowId": log.WorkflowID, "triggerId": log.TriggerID, "triggerType": log.TriggerType, "status": log.Status, "runId": log.RunID, "sessionId": log.SessionID, "error": log.Error, "startedAt": log.StartedAt, "finishedAt": log.FinishedAt, "createdAt": log.CreatedAt, "updatedAt": log.UpdatedAt}
}

func requireInteractiveWorkflowToolSession(ctx context.Context, store *jfadk.Store) error {
	if store == nil {
		return fmt.Errorf("ADK store is unavailable")
	}
	sessionID, ok := jfadk.ToolInvocationSessionID(ctx)
	if !ok {
		return fmt.Errorf("workflow runs require a resolvable interactive ADK session")
	}
	session, found, err := store.Session(ctx, sessionID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("workflow runs require an existing interactive ADK session")
	}
	if strings.TrimSpace(session.WorkflowID) != "" {
		return fmt.Errorf("workflow-origin sessions cannot start another workflow")
	}
	return nil
}

func workflowCreateRequest(input map[string]any) (jfadk.WorkflowDefinitionWriteRequest, error) {
	payload := jfadk.WorkflowDefinitionWriteRequest{ID: stringValue(input, "id")}
	applyWorkflowWriteFields(&payload, input)
	if err := decodeWorkflowCanvasGraph(input, &payload); err != nil {
		return jfadk.WorkflowDefinitionWriteRequest{}, err
	}
	return payload, nil
}

func workflowUpdateRequest(current jfadk.WorkflowDefinition, input map[string]any) (jfadk.WorkflowDefinitionWriteRequest, error) {
	payload := jfadk.WorkflowDefinitionWriteRequest{ID: current.ID, Name: current.Name, Description: current.Description, Status: current.Status, AgentID: current.AgentID, WorkMode: current.WorkMode, ProviderID: current.ProviderID, Model: current.Model, PermissionMode: current.PermissionMode, PromptTemplate: current.PromptTemplate, ObjectiveTemplate: current.ObjectiveTemplate, DefaultInputs: current.DefaultInputs, CanvasGraph: current.CanvasGraph, Tags: append([]string(nil), current.Tags...)}
	applyWorkflowWriteFields(&payload, input)
	if toolBoolValue(input, "clearCanvasGraph") {
		payload.CanvasGraph = nil
	} else if err := decodeWorkflowCanvasGraph(input, &payload); err != nil {
		return jfadk.WorkflowDefinitionWriteRequest{}, err
	}
	return payload, nil
}

func applyWorkflowWriteFields(payload *jfadk.WorkflowDefinitionWriteRequest, input map[string]any) {
	applyPresentString(input, "name", &payload.Name)
	applyPresentString(input, "description", &payload.Description)
	applyPresentString(input, "status", &payload.Status)
	applyPresentString(input, "agentId", &payload.AgentID)
	applyPresentString(input, "workMode", &payload.WorkMode)
	applyPresentString(input, "providerId", &payload.ProviderID)
	applyPresentString(input, "model", &payload.Model)
	applyPresentString(input, "permissionMode", &payload.PermissionMode)
	applyPresentString(input, "promptTemplate", &payload.PromptTemplate)
	applyPresentString(input, "objectiveTemplate", &payload.ObjectiveTemplate)
	if _, ok := input["defaultInputs"]; ok {
		payload.DefaultInputs = toolObjectValue(input, "defaultInputs")
	}
	if _, ok := input["tags"]; ok {
		payload.Tags = stringSliceValue(input, "tags")
	}
}

func decodeWorkflowCanvasGraph(input map[string]any, payload *jfadk.WorkflowDefinitionWriteRequest) error {
	value, ok := input["canvasGraph"]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode canvasGraph: %w", err)
	}
	var graph jfadk.WorkflowCanvasGraph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return fmt.Errorf("invalid canvasGraph: %w", err)
	}
	payload.CanvasGraph = &graph
	return nil
}

func workflowTriggerCreateRequest(input map[string]any) jfadk.WorkflowTriggerWriteRequest {
	return jfadk.WorkflowTriggerWriteRequest{ID: stringValue(input, "id"), Type: strings.ToLower(strings.TrimSpace(stringValue(input, "type"))), Title: stringValue(input, "title"), Status: stringValue(input, "status"), Config: toolObjectValue(input, "config")}
}

func workflowTriggerUpdateRequest(current jfadk.WorkflowTrigger, input map[string]any) (jfadk.WorkflowTriggerWriteRequest, error) {
	payload := jfadk.WorkflowTriggerWriteRequest{ID: current.ID, Type: current.Type, Title: current.Title, Status: current.Status, Config: current.Config}
	if rawType, ok := input["type"]; ok {
		requested := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawType)))
		if requested != current.Type && (requested == jfadk.WorkflowTriggerTypeWebhook || current.Type == jfadk.WorkflowTriggerTypeWebhook) {
			return jfadk.WorkflowTriggerWriteRequest{}, fmt.Errorf("webhook trigger type changes must use the UI/API")
		}
		payload.Type = requested
	}
	applyPresentString(input, "title", &payload.Title)
	applyPresentString(input, "status", &payload.Status)
	if _, ok := input["config"]; ok {
		payload.Config = toolObjectValue(input, "config")
	}
	return payload, nil
}

func toolObjectValue(input map[string]any, key string) map[string]any {
	if typed, ok := input[key].(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func toolBoolValue(input map[string]any, key string) bool {
	value, _ := input[key].(bool)
	return value
}

func applyPresentString(input map[string]any, key string, target *string) {
	if _, ok := input[key]; ok {
		*target = stringValue(input, key)
	}
}
