package adk

import (
	"fmt"
	"strings"
)

func workflowTaskToolDescriptors() []ToolDescriptor {
	return []ToolDescriptor{
		{Name: workflowTasksListTool, DisplayName: "列出工作流任务", Description: "列出当前任务 DAG 和可执行 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskAddTool, DisplayName: "新增工作流任务", Description: "运行中新增一个 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskClaimTool, DisplayName: "领取工作流任务", Description: "领取一个可执行 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskCompleteTool, DisplayName: "完成工作流任务", Description: "完成一个 TODO 并写入结果摘要。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskBlockTool, DisplayName: "阻塞工作流任务", Description: "标记一个 TODO 被阻塞。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskDelegateTool, DisplayName: "委派子智能体", Description: "将一个 TODO 委派给 ADK 子智能体。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowModelsListTool, DisplayName: "查询子智能体模型", Description: "列出可供委派子智能体使用的 ADK 模型。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowGoalCompleteTool, DisplayName: "完成目标", Description: "声明目标已经完成并退出目标循环。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowGoalContinueTool, DisplayName: "继续目标", Description: "声明目标尚未完成并继续目标循环。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
	}
}

func allPermissionModes() []string {
	return []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll}
}

func goalOrchestratorInstruction(base string) string {
	var builder strings.Builder
	builder.WriteString("JFTRADE_GOAL_ORCHESTRATOR\n你是目标模式主控调度智能体。你必须通过 workflow.task.* 工具维护 TODO DAG，可以亲自完成任务、增加后续 TODO、阻塞无法完成的任务，或在确有必要时委派子智能体。不要直接调用业务工具；业务工具只能由被委派的子智能体使用。需要为子智能体选择不同模型时，先调用 workflow.models.list 查询可调用模型，再把 childProviderId 和可选 childModel 传给委派工具。收到“是否完成目标”追问时，必须调用 workflow.goal.complete 或 workflow.goal.continue 二选一；不要只输出文字。")
	if strings.TrimSpace(base) != "" {
		builder.WriteString("\n\n基础 Agent 指令：")
		builder.WriteString(strings.TrimSpace(base))
	}
	return builder.String()
}

func goalOrchestratorUserMessage(parent Run) string {
	return fmt.Sprintf("请推进这个目标。你可以使用 workflow.task.* 工具维护 TODO DAG，并在本轮完成可见回复后等待系统追问再裁决目标是否完成。\n总体目标：%s\n用户请求：%s", strings.TrimSpace(parent.Objective), strings.TrimSpace(parent.UserMessage))
}

func goalDecisionPrompt(parent Run, lastReply string, retry bool) string {
	prefix := "请判断是否完成目标"
	if retry {
		prefix = "上一次没有调用目标裁决工具。现在必须调用 workflow.goal.complete 或 workflow.goal.continue"
	}
	return fmt.Sprintf("%s：“%s”。\n上一轮可见回复：%s\n如果目标已完成，调用 workflow.goal.complete 并给出 summary；如果尚未完成，调用 workflow.goal.continue 并给出 reason。不要只输出文字。", prefix, strings.TrimSpace(parent.Objective), strings.TrimSpace(lastReply))
}

func goalFinalReplyPrompt(parent Run) string {
	return fmt.Sprintf("所有当前工作步骤已经返回，但还没有形成最终可见答复。请总结本轮结果并直接回复用户；本轮不要再调用工具。\n当前目标：%s", strings.TrimSpace(parent.Objective))
}

func goalTurnHasFinalReply(execution *googleADKExecution, runID string, visibleReply string) bool {
	if execution == nil || strings.TrimSpace(visibleReply) == "" || execution.activeToolCallCountForRun(runID) > 0 {
		return false
	}
	execution.mu.Lock()
	defer execution.mu.Unlock()
	if len(execution.callsForRunLocked(runID)) == 0 {
		return true
	}
	return execution.runHasPostToolTextLocked(runID)
}

func goalOrchestratorContinueNudge(parent Run, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "目标尚未完成。"
	}
	return fmt.Sprintf("目标尚未完成，原因：%s\n请调用 workflow.tasks.list 检查状态，然后继续完成、委派、阻塞或新增 TODO。完成本轮可见回复后等待系统再次询问目标是否完成。\n当前目标：%s", reason, strings.TrimSpace(parent.Objective))
}

func workflowTaskResultSummaries(tasks []Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if summary := strings.TrimSpace(task.ResultSummary); summary != "" {
			out = append(out, summary)
		}
	}
	return out
}

func taskToolTaskSummaries(tasks []Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskToolTaskSummary(task))
	}
	return out
}

func taskToolTaskSummary(task Task) map[string]any {
	return map[string]any{
		"id": task.ID, "title": task.Title, "status": task.Status, "order": task.Order,
		"dependsOn": task.DependsOn, "executor": task.Executor, "runId": task.RunID,
		"agentRole": task.AgentRole, "planSource": task.PlanSource, "resultSummary": task.ResultSummary,
		"childProviderId": task.ChildProviderID, "childModel": task.ChildModel,
	}
}

func plannerStringSliceArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	values, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func emptyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

func workflowTaskAddSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
		"dependsOn": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "agentRole": map[string]any{"type": "string"}, "modeHint": map[string]any{"type": "string"},
		"childProviderId": map[string]any{"type": "string"}, "childModel": map[string]any{"type": "string"},
	}, "required": []string{"title"}, "additionalProperties": false}
}

func workflowTaskClaimSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "executor": map[string]any{"type": "string", "enum": []string{workflowTaskExecutorSelf, workflowTaskExecutorChild}}}, "additionalProperties": false}
}

func workflowTaskCompleteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "resultSummary": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string"}}, "additionalProperties": false}
}

func workflowTaskBlockSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}, "additionalProperties": false}
}

func workflowTaskDelegateSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"taskId": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"}, "agentRole": map[string]any{"type": "string"},
		"childProviderId": map[string]any{"type": "string"}, "childModel": map[string]any{"type": "string"},
	}, "additionalProperties": false}
}

func workflowModelsListSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query": map[string]any{"type": "string"}, "providerId": map[string]any{"type": "string"},
		"callableOnly": map[string]any{"type": "boolean"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
	}, "additionalProperties": false}
}

func workflowGoalCompleteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string"}, "resultSummary": map[string]any{"type": "string"}}, "additionalProperties": false}
}

func workflowGoalContinueSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"reason": map[string]any{"type": "string"}}, "additionalProperties": false}
}
