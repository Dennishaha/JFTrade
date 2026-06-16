package adk

import "strings"

func normalizeToolCalls(toolCalls []ToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return []ToolCall{}
	}
	return append([]ToolCall(nil), toolCalls...)
}

func normalizeApprovals(approvals []Approval) []Approval {
	if len(approvals) == 0 {
		return []Approval{}
	}
	return append([]Approval(nil), approvals...)
}

func normalizeWorkflowPlan(plan []WorkflowStepState) []WorkflowStepState {
	if len(plan) == 0 {
		return []WorkflowStepState{}
	}
	normalized := make([]WorkflowStepState, 0, len(plan))
	for _, step := range plan {
		if len(step.DependsOn) == 0 {
			step.DependsOn = []string{}
		} else {
			step.DependsOn = normalizeStringSlice(step.DependsOn)
		}
		normalized = append(normalized, step)
	}
	return normalized
}

func normalizeTimelineEntries(entries []TimelineEntry) []TimelineEntry {
	if len(entries) == 0 {
		return []TimelineEntry{}
	}
	normalized := make([]TimelineEntry, 0, len(entries))
	for _, entry := range entries {
		normalized = append(normalized, NormalizeTimelineEntry(entry))
	}
	return normalized
}

func NormalizeRun(run Run) Run {
	run.WorkMode = normalizeWorkMode(run.WorkMode)
	if len(run.ChildRunIDs) == 0 {
		run.ChildRunIDs = []string{}
	} else {
		run.ChildRunIDs = normalizeStringSlice(run.ChildRunIDs)
	}
	run.ToolCalls = normalizeToolCalls(run.ToolCalls)
	run.PendingApprovals = normalizeApprovals(run.PendingApprovals)
	run.WorkflowPlan = normalizeWorkflowPlan(run.WorkflowPlan)
	if len(run.ToolSummaries) == 0 {
		run.ToolSummaries = []string{}
	} else {
		run.ToolSummaries = append([]string(nil), run.ToolSummaries...)
	}
	return run
}

func NormalizeAgent(agent Agent) Agent {
	agent.Tools = normalizeStringSlice(agent.Tools)
	agent.Skills = normalizeStringSlice(agent.Skills)
	agent.PermissionMode = normalizePermissionMode(agent.PermissionMode)
	agent.RecentUserWindow = normalizeRecentUserWindow(agent.RecentUserWindow)
	agent.WorkMode = normalizeAgentDefaultWorkMode(agent.WorkMode)
	agent.LoopMaxIterations = normalizeLoopMaxIterations(agent.LoopMaxIterations)
	if strings.TrimSpace(agent.Status) == "" {
		agent.Status = AgentStatusEnabled
	}
	return agent
}

func NormalizeTimelineEntry(entry TimelineEntry) TimelineEntry {
	entry.ToolCalls = normalizeToolCalls(entry.ToolCalls)
	entry.Approvals = normalizeApprovals(entry.Approvals)
	return entry
}

func NormalizeChatResponse(response ChatResponse) ChatResponse {
	response.Run = NormalizeRun(response.Run)
	response.PendingApprovals = normalizeApprovals(response.PendingApprovals)
	response.Timeline = normalizeTimelineEntries(response.Timeline)
	return response
}

func NormalizeApprovalResolution(resolution ApprovalResolution) ApprovalResolution {
	if resolution.Run != nil {
		resolution.Run = new(NormalizeRun(*resolution.Run))
	}
	if resolution.ParentRun != nil {
		resolution.ParentRun = new(NormalizeRun(*resolution.ParentRun))
	}
	return resolution
}

func NormalizeSessionsResponse(response SessionsResponse) SessionsResponse {
	response.Timeline = normalizeTimelineEntries(response.Timeline)
	return response
}
