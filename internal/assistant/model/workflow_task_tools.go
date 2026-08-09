package model

import (
	"fmt"
	"strings"
)

const (
	WorkflowTasksListTool     = "workflow.tasks.list"
	WorkflowTaskAddTool       = "workflow.task.add"
	WorkflowTaskClaimTool     = "workflow.task.claim"
	WorkflowTaskCompleteTool  = "workflow.task.complete"
	WorkflowTaskBlockTool     = "workflow.task.block"
	WorkflowTaskDelegateTool  = "workflow.task.delegate"
	WorkflowModelsListTool    = "workflow.models.list"
	WorkflowTaskIncompleteErr = "WORKFLOW_TASK_INCOMPLETE"

	WorkflowGoalCompleteTool = "workflow.goal.complete"
	WorkflowGoalContinueTool = "workflow.goal.continue"
)

// AllPermissionModes returns the permission modes allowed for workflow tools.
func AllPermissionModes() []string {
	return []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll}
}

// WorkflowTaskToolDescriptors returns the built-in workflow task tool
// descriptors shared by registration and tool runtime.
func WorkflowTaskToolDescriptors() []ToolDescriptor {
	return []ToolDescriptor{
		{Name: WorkflowTasksListTool, DisplayName: "列出工作流任务", Description: "列出当前任务 DAG 和可执行 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowTaskAddTool, DisplayName: "新增工作流任务", Description: "运行中新增一个 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowTaskClaimTool, DisplayName: "领取工作流任务", Description: "领取一个可执行 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowTaskCompleteTool, DisplayName: "完成工作流任务", Description: "完成一个 TODO 并写入结果摘要。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowTaskBlockTool, DisplayName: "阻塞工作流任务", Description: "标记一个 TODO 被阻塞。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowTaskDelegateTool, DisplayName: "委派子智能体", Description: "将一个 TODO 委派给 ADK 子智能体。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowModelsListTool, DisplayName: "查询子智能体模型", Description: "列出可供委派子智能体使用的 ADK 模型。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowGoalCompleteTool, DisplayName: "完成目标", Description: "声明目标已经完成并退出目标循环。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
		{Name: WorkflowGoalContinueTool, DisplayName: "继续目标", Description: "声明目标尚未完成并继续目标循环。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: AllPermissionModes()},
	}
}

// GoalOrchestratorInstruction builds the loop-mode goal orchestrator prompt.
func GoalOrchestratorInstruction(base string) string {
	var builder strings.Builder
	builder.WriteString("JFTRADE_GOAL_ORCHESTRATOR\n你是目标模式主控调度智能体。你必须通过 workflow.task.* 工具维护 TODO DAG，可以亲自完成任务、增加后续 TODO、阻塞无法完成的任务，或在确有必要时委派子智能体。不要直接调用业务工具；业务工具只能由被委派的子智能体使用。需要为子智能体选择不同模型时，先调用 workflow.models.list 查询可调用模型，再把 childProviderId 和可选 childModel 传给委派工具。收到“是否完成目标”追问时，必须调用 workflow.goal.complete 或 workflow.goal.continue 二选一；不要只输出文字。")
	if strings.TrimSpace(base) != "" {
		builder.WriteString("\n\n基础 Agent 指令：")
		builder.WriteString(strings.TrimSpace(base))
	}
	return builder.String()
}

// GoalOrchestratorUserMessage builds the first user turn for a goal workflow.
func GoalOrchestratorUserMessage(parent Run) string {
	return fmt.Sprintf("请推进这个目标。你可以使用 workflow.task.* 工具维护 TODO DAG，并在本轮完成可见回复后等待系统追问再裁决目标是否完成。\n总体目标：%s\n用户请求：%s", strings.TrimSpace(parent.Objective), strings.TrimSpace(parent.UserMessage))
}

// GoalDecisionPrompt builds the goal completion decision prompt.
func GoalDecisionPrompt(parent Run, lastReply string, retry bool) string {
	prefix := "请判断是否完成目标"
	if retry {
		prefix = "上一次没有调用目标裁决工具。现在必须调用 workflow.goal.complete 或 workflow.goal.continue"
	}
	return fmt.Sprintf("%s：“%s”。\n上一轮可见回复：%s\n如果目标已完成，调用 workflow.goal.complete 并给出 summary；如果尚未完成，调用 workflow.goal.continue 并给出 reason。不要只输出文字。", prefix, strings.TrimSpace(parent.Objective), strings.TrimSpace(lastReply))
}

// GoalFinalReplyPrompt asks the orchestrator for a final visible reply.
func GoalFinalReplyPrompt(parent Run) string {
	return fmt.Sprintf("所有当前工作步骤已经返回，但还没有形成最终可见答复。请总结本轮结果并直接回复用户；本轮不要再调用工具。\n当前目标：%s", strings.TrimSpace(parent.Objective))
}

// GoalOrchestratorContinueNudge asks the orchestrator to keep working.
func GoalOrchestratorContinueNudge(parent Run, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "目标尚未完成。"
	}
	return fmt.Sprintf("目标尚未完成，原因：%s\n请调用 workflow.tasks.list 检查状态，然后继续完成、委派、阻塞或新增 TODO。完成本轮可见回复后等待系统再次询问目标是否完成。\n当前目标：%s", reason, strings.TrimSpace(parent.Objective))
}
