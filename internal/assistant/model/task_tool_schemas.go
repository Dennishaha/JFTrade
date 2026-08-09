package model

// EmptyObjectSchema is the JSON schema for tools that accept no arguments.
func EmptyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

// WorkflowTaskAddSchema is the JSON schema for the runtime TODO add tool.
func WorkflowTaskAddSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
		"dependsOn": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "agentRole": map[string]any{"type": "string"}, "modeHint": map[string]any{"type": "string"},
		"childProviderId": map[string]any{"type": "string"}, "childModel": map[string]any{"type": "string"},
	}, "required": []string{"title"}, "additionalProperties": false}
}

// WorkflowTaskClaimSchema is the JSON schema for the TODO claim tool.
func WorkflowTaskClaimSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "executor": map[string]any{"type": "string", "enum": []string{WorkflowTaskExecutorSelf, WorkflowTaskExecutorChild}}}, "additionalProperties": false}
}

// WorkflowTaskCompleteSchema is the JSON schema for the TODO complete tool.
func WorkflowTaskCompleteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "resultSummary": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string"}}, "additionalProperties": false}
}

// WorkflowTaskBlockSchema is the JSON schema for the TODO block tool.
func WorkflowTaskBlockSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}, "additionalProperties": false}
}

// WorkflowTaskDelegateSchema is the JSON schema for the TODO delegate tool.
func WorkflowTaskDelegateSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"taskId": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"}, "agentRole": map[string]any{"type": "string"},
		"childProviderId": map[string]any{"type": "string"}, "childModel": map[string]any{"type": "string"},
	}, "additionalProperties": false}
}

// WorkflowModelsListSchema is the JSON schema for the model list tool.
func WorkflowModelsListSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query": map[string]any{"type": "string"}, "providerId": map[string]any{"type": "string"},
		"callableOnly": map[string]any{"type": "boolean"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
	}, "additionalProperties": false}
}

// WorkflowGoalCompleteSchema is the JSON schema for the goal complete tool.
func WorkflowGoalCompleteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string"}, "resultSummary": map[string]any{"type": "string"}}, "additionalProperties": false}
}

// WorkflowGoalContinueSchema is the JSON schema for the goal continue tool.
func WorkflowGoalContinueSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"reason": map[string]any{"type": "string"}}, "additionalProperties": false}
}
