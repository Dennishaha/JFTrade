package model

import (
	"encoding/json"
	"strings"
)

// WorkflowChildInstruction composes the child agent instruction from the
// parent base instruction and the delegated task marker.
func WorkflowChildInstruction(base string, task string) string {
	task = strings.TrimSpace(task)
	instruction := strings.TrimSpace(base)
	marker := "JFTRADE_WORKFLOW_TASK: " + task
	if instruction == "" {
		return marker
	}
	if task == "" {
		return instruction
	}
	return instruction + "\n\n" + marker + "\n请只完成上述 JFTRADE_WORKFLOW_TASK 指定的子任务。"
}

// WorkflowChildInstructionTask projects one planner step into the child agent
// task instruction.
func WorkflowChildInstructionTask(step WorkflowStep) string {
	var builder strings.Builder
	if objective := strings.TrimSpace(step.Objective); objective != "" {
		builder.WriteString("总体目标：")
		builder.WriteString(objective)
	}
	if task := strings.TrimSpace(step.Message); task != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("当前子任务：")
		builder.WriteString(task)
	}
	if description := strings.TrimSpace(step.Description); description != "" && description != strings.TrimSpace(step.Message) {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("子任务说明：")
		builder.WriteString(description)
	}
	if role := strings.TrimSpace(step.AgentRole); role != "" {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("子 Agent 角色：")
		builder.WriteString(role)
	}
	if builder.Len() == 0 {
		return strings.TrimSpace(step.Message)
	}
	builder.WriteString("\n\n请只基于以上明确给出的目标和子任务工作；不要假设自己能看到父对话的其他上下文。")
	return builder.String()
}

// WorkflowFinalSynthesisInstruction appends the final-reply-only requirement
// to a child agent instruction.
func WorkflowFinalSynthesisInstruction(base string, task string) string {
	instruction := WorkflowChildInstruction(base, task)
	return instruction + "\n\n工具调用已经完成。现在必须基于已有工具结果输出最终回复。不要再调用工具，不要请求审批，不要只说明准备继续。"
}

// WorkflowObservationMatchesStep reports whether an observed workflow event
// belongs to the given plan step.
func WorkflowObservationMatchesStep(step WorkflowStepState, runID string, nodeName string) bool {
	if strings.TrimSpace(step.ChildRunID) != "" && strings.TrimSpace(step.ChildRunID) == strings.TrimSpace(runID) {
		return true
	}
	if strings.TrimSpace(step.NodeName) != "" && strings.TrimSpace(step.NodeName) == strings.TrimSpace(nodeName) {
		return true
	}
	return strings.TrimSpace(nodeName) != "" && strings.Contains(strings.TrimSpace(step.NodeName), strings.TrimSpace(nodeName))
}

// SummarizeWorkflowOutput renders a workflow event output as a compact
// summary line.
func SummarizeWorkflowOutput(output any) string {
	if output == nil {
		return ""
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return strings.TrimSpace(JSONFallbackString(output))
	}
	text := string(raw)
	if len(text) > 600 {
		text = text[:600] + "...(truncated)"
	}
	return text
}

// JSONFallbackString encodes a non-marshalable value inside a JSON payload so
// summaries degrade to a stable text form.
func JSONFallbackString(value any) string {
	raw, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		return ""
	}
	return string(raw)
}
