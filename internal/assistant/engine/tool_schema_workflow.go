package adk

func workflowManagementToolInputSchema(name string) (map[string]any, bool) {
	switch name {
	case "workflow.wait":
		return workflowWaitInputSchema(), true
	case "workflows.list":
		return workflowsListInputSchema(), true
	case "workflows.get", "workflows.delete":
		return workflowIDInputSchema(), true
	case "workflows.create", "workflows.update":
		return workflowWriteInputSchema(name), true
	case "workflows.run":
		return workflowRunInputSchema(), true
	case "workflow_triggers.list":
		return workflowTriggersListInputSchema(), true
	case "workflow_triggers.get", "workflow_triggers.delete":
		return workflowTriggerIDInputSchema(), true
	case "workflow_triggers.create", "workflow_triggers.update":
		return workflowTriggerWriteInputSchema(name), true
	case "workflow_triggers.run":
		return workflowTriggerRunInputSchema(), true
	case "workflow_runs.list":
		return workflowRunsListInputSchema(), true
	case "workflow_runs.get":
		return workflowRunLogIDInputSchema(), true
	default:
		return nil, false
	}
}

func workflowsListInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"status": workflowStatusSchema(),
		"limit":  pageLimitSchema(),
		"offset": pageOffsetSchema(),
	}, nil)
}

func workflowIDInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"workflowId": map[string]any{"type": "string", "description": "工作流 ID。"},
	}, []string{"workflowId"})
}

func workflowWriteInputSchema(name string) map[string]any {
	properties := map[string]any{
		"id":                map[string]any{"type": "string", "description": "创建时可选的稳定工作流 ID。"},
		"name":              map[string]any{"type": "string"},
		"description":       map[string]any{"type": "string"},
		"status":            workflowStatusSchema(),
		"agentId":           map[string]any{"type": "string"},
		"workMode":          map[string]any{"type": "string", "enum": []string{WorkModeChat, WorkModeLoop}},
		"providerId":        map[string]any{"type": "string"},
		"model":             map[string]any{"type": "string"},
		"permissionMode":    map[string]any{"type": "string", "enum": []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll}},
		"promptTemplate":    map[string]any{"type": "string"},
		"objectiveTemplate": map[string]any{"type": "string"},
		"defaultInputs":     arbitraryObjectSchema("工作流默认输入。"),
		"canvasGraph":       workflowCanvasGraphInputSchema(),
		"tags":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}
	required := []string{"name", "agentId", "promptTemplate"}
	if name == "workflows.update" {
		delete(properties, "id")
		properties["workflowId"] = map[string]any{"type": "string"}
		properties["clearCanvasGraph"] = map[string]any{"type": "boolean", "description": "设为 true 时清除现有画布。"}
		required = []string{"workflowId"}
	}
	return strictObjectSchema(properties, required)
}

func workflowRunInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"workflowId": map[string]any{"type": "string"},
		"inputs":     arbitraryObjectSchema("本次运行输入。"),
	}, []string{"workflowId"})
}

func workflowTriggersListInputSchema() map[string]any {
	return workflowIDInputSchema()
}

func workflowTriggerIDInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"workflowId": map[string]any{"type": "string"},
		"triggerId":  map[string]any{"type": "string"},
	}, []string{"workflowId", "triggerId"})
}

func workflowTriggerWriteInputSchema(name string) map[string]any {
	properties := map[string]any{
		"workflowId": map[string]any{"type": "string"},
		"id":         map[string]any{"type": "string", "description": "创建时可选的稳定触发器 ID。"},
		"type":       workflowToolTriggerTypeSchema(),
		"title":      map[string]any{"type": "string"},
		"status":     workflowTriggerStatusSchema(),
		"config":     arbitraryObjectSchema("触发器配置。"),
	}
	required := []string{"workflowId", "type"}
	if name == "workflow_triggers.update" {
		delete(properties, "id")
		properties["triggerId"] = map[string]any{"type": "string"}
		required = []string{"workflowId", "triggerId"}
	}
	return strictObjectSchema(properties, required)
}

func workflowTriggerRunInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"triggerId": map[string]any{"type": "string"},
		"inputs":    arbitraryObjectSchema("本次运行输入。"),
	}, []string{"triggerId"})
}

func workflowRunsListInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"workflowId": map[string]any{"type": "string"},
		"triggerId":  map[string]any{"type": "string"},
		"status": map[string]any{"type": "string", "enum": []string{
			WorkflowTriggerLogStatusQueued, WorkflowTriggerLogStatusRunning, WorkflowTriggerLogStatusSucceeded,
			WorkflowTriggerLogStatusPendingApproval, WorkflowTriggerLogStatusFailed, WorkflowTriggerLogStatusCancelled, WorkflowTriggerLogStatusSkipped,
		}},
		"limit":  pageLimitSchema(),
		"offset": pageOffsetSchema(),
	}, nil)
}

func workflowRunLogIDInputSchema() map[string]any {
	return strictObjectSchema(map[string]any{
		"logId": map[string]any{"type": "string"},
	}, []string{"logId"})
}

func workflowCanvasGraphInputSchema() map[string]any {
	point := strictObjectSchema(map[string]any{
		"x": map[string]any{"type": "number"},
		"y": map[string]any{"type": "number"},
	}, []string{"x", "y"})
	node := strictObjectSchema(map[string]any{
		"id":       map[string]any{"type": "string"},
		"type":     map[string]any{"type": "string"},
		"position": point,
		"data":     arbitraryObjectSchema("节点数据。"),
	}, []string{"id", "type", "position"})
	edge := strictObjectSchema(map[string]any{
		"id":           map[string]any{"type": "string"},
		"source":       map[string]any{"type": "string"},
		"target":       map[string]any{"type": "string"},
		"sourceHandle": map[string]any{"type": "string"},
		"targetHandle": map[string]any{"type": "string"},
		"type":         map[string]any{"type": "string"},
		"data":         arbitraryObjectSchema("连线数据。"),
	}, []string{"id", "source", "target"})
	return strictObjectSchema(map[string]any{
		"version":  map[string]any{"type": "string"},
		"nodes":    map[string]any{"type": "array", "items": node},
		"edges":    map[string]any{"type": "array", "items": edge},
		"viewport": arbitraryObjectSchema("画布视口。"),
	}, nil)
}

func workflowStatusSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{WorkflowStatusEnabled, WorkflowStatusDisabled}}
}

func workflowTriggerStatusSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{WorkflowTriggerStatusEnabled, WorkflowTriggerStatusDisabled, WorkflowTriggerStatusError}}
}

func workflowToolTriggerTypeSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{WorkflowTriggerTypeManual, WorkflowTriggerTypeSchedule, WorkflowTriggerTypeEvent, WorkflowTriggerTypeMarketThreshold}, "description": "Webhook 触发器必须通过 UI/API 创建。"}
}

func arbitraryObjectSchema(description string) map[string]any {
	return map[string]any{"type": "object", "description": description, "additionalProperties": true}
}

func pageLimitSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}
}

func pageOffsetSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "default": 0}
}

func strictObjectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
