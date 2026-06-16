package adk

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
	run.ToolCalls = normalizeToolCalls(run.ToolCalls)
	run.PendingApprovals = normalizeApprovals(run.PendingApprovals)
	if len(run.ToolSummaries) == 0 {
		run.ToolSummaries = []string{}
	} else {
		run.ToolSummaries = append([]string(nil), run.ToolSummaries...)
	}
	return run
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
	return resolution
}

func NormalizeSessionsResponse(response SessionsResponse) SessionsResponse {
	response.Timeline = normalizeTimelineEntries(response.Timeline)
	return response
}
