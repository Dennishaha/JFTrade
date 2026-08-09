package adk

import (
	"strings"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func normalizeApprovals(approvals []Approval) []Approval {
	return jfadkmodel.NormalizeApprovals(approvals)
}

func normalizeTimelineEntries(entries []TimelineEntry) []TimelineEntry {
	return jfadkmodel.NormalizeTimelineEntries(entries)
}

func NormalizeRun(run Run) Run {
	return jfadkmodel.NormalizeRun(run)
}

func NormalizeAgent(agent Agent) Agent {
	agent.Tools = jfadkmodel.NormalizeStringSlice(agent.Tools)
	agent.Skills = jfadkmodel.NormalizeStringSlice(agent.Skills)
	agent.PermissionMode = normalizePermissionMode(agent.PermissionMode)
	agent.RecentUserWindow = normalizeRecentUserWindow(agent.RecentUserWindow)
	agent.WorkMode = normalizeAgentDefaultWorkMode(agent.WorkMode)
	agent.LoopMaxIterations = jfadkmodel.NormalizeLoopMaxIterations(agent.LoopMaxIterations)
	agent.Builtin = agent.Builtin || IsBuiltinAgentID(agent.ID)
	if IsPrimaryBuiltinAgentID(agent.ID) {
		agent.Name = "默认助手"
		agent.Skills = BuiltinSkillIDs()
	}
	if strings.TrimSpace(agent.Status) == "" {
		agent.Status = AgentStatusEnabled
	}
	return agent
}

func NormalizeTimelineEntry(entry TimelineEntry) TimelineEntry {
	return jfadkmodel.NormalizeTimelineEntry(entry)
}

func NormalizeChatResponse(response ChatResponse) ChatResponse {
	response.Run = NormalizeRun(response.Run)
	response.PendingApprovals = normalizeApprovals(response.PendingApprovals)
	response.InputRequest = normalizeInputRequest(response.InputRequest)
	response.Timeline = normalizeTimelineEntries(response.Timeline)
	return response
}

func normalizeInputRequest(request *InputRequest) *InputRequest {
	return jfadkmodel.NormalizeInputRequest(request)
}

func appendInputRequestIfMissing(requests []InputRequest, request InputRequest) []InputRequest {
	return jfadkmodel.AppendInputRequestIfMissing(requests, request)
}

func NormalizeWorkflowDefinition(workflow WorkflowDefinition) WorkflowDefinition {
	workflow.ID = strings.TrimSpace(workflow.ID)
	workflow.Name = strings.TrimSpace(workflow.Name)
	workflow.Description = strings.TrimSpace(workflow.Description)
	workflow.Status = strings.ToUpper(strings.TrimSpace(workflow.Status))
	if workflow.Status == "" {
		workflow.Status = WorkflowStatusEnabled
	}
	workflow.AgentID = strings.TrimSpace(workflow.AgentID)
	workflow.WorkMode = jfadkmodel.NormalizeWorkMode(workflow.WorkMode)
	workflow.ProviderID = strings.TrimSpace(workflow.ProviderID)
	workflow.Model = strings.TrimSpace(workflow.Model)
	workflow.PermissionMode = normalizeOptionalPermissionMode(workflow.PermissionMode)
	workflow.PromptTemplate = strings.TrimSpace(workflow.PromptTemplate)
	workflow.ObjectiveTemplate = strings.TrimSpace(workflow.ObjectiveTemplate)
	workflow.DefaultInputs = normalizeAnyMap(workflow.DefaultInputs)
	workflow.CanvasGraph = normalizeWorkflowCanvasGraph(workflow.CanvasGraph)
	workflow.Tags = jfadkmodel.NormalizeStringSlice(workflow.Tags)
	return workflow
}

func normalizeWorkflowCanvasGraph(graph *WorkflowCanvasGraph) *WorkflowCanvasGraph {
	if graph == nil {
		return nil
	}
	normalized := &WorkflowCanvasGraph{
		Version:  strings.TrimSpace(graph.Version),
		Nodes:    make([]WorkflowCanvasNode, 0, len(graph.Nodes)),
		Edges:    make([]WorkflowCanvasEdge, 0, len(graph.Edges)),
		Viewport: normalizeAnyMap(graph.Viewport),
	}
	for _, node := range graph.Nodes {
		node.ID = strings.TrimSpace(node.ID)
		node.Type = strings.TrimSpace(node.Type)
		node.Data = normalizeAnyMap(node.Data)
		normalized.Nodes = append(normalized.Nodes, node)
	}
	for _, edge := range graph.Edges {
		edge.ID = strings.TrimSpace(edge.ID)
		edge.Source = strings.TrimSpace(edge.Source)
		edge.Target = strings.TrimSpace(edge.Target)
		edge.SourceHandle = strings.TrimSpace(edge.SourceHandle)
		edge.TargetHandle = strings.TrimSpace(edge.TargetHandle)
		edge.Type = strings.TrimSpace(edge.Type)
		edge.Data = normalizeAnyMap(edge.Data)
		normalized.Edges = append(normalized.Edges, edge)
	}
	return normalized
}

func NormalizeWorkflowTrigger(trigger WorkflowTrigger) WorkflowTrigger {
	trigger.ID = strings.TrimSpace(trigger.ID)
	trigger.WorkflowID = strings.TrimSpace(trigger.WorkflowID)
	trigger.Type = strings.ToLower(strings.TrimSpace(trigger.Type))
	trigger.Title = strings.TrimSpace(trigger.Title)
	trigger.Status = strings.ToUpper(strings.TrimSpace(trigger.Status))
	if trigger.Status == "" {
		trigger.Status = WorkflowTriggerStatusEnabled
	}
	trigger.Config = normalizeAnyMap(trigger.Config)
	trigger.SecretHash = strings.TrimSpace(trigger.SecretHash)
	trigger.HasSecret = trigger.HasSecret || trigger.SecretHash != ""
	trigger.NextRunAt = strings.TrimSpace(trigger.NextRunAt)
	trigger.LastRunAt = strings.TrimSpace(trigger.LastRunAt)
	trigger.LastRunID = strings.TrimSpace(trigger.LastRunID)
	trigger.LastError = strings.TrimSpace(trigger.LastError)
	return trigger
}

func NormalizeWorkflowTriggerLog(log WorkflowTriggerLog) WorkflowTriggerLog {
	log.ID = strings.TrimSpace(log.ID)
	log.WorkflowID = strings.TrimSpace(log.WorkflowID)
	log.TriggerID = strings.TrimSpace(log.TriggerID)
	log.TriggerType = strings.TrimSpace(log.TriggerType)
	log.Status = strings.ToUpper(strings.TrimSpace(log.Status))
	if log.Status == "" {
		log.Status = WorkflowTriggerLogStatusQueued
	}
	log.RunID = strings.TrimSpace(log.RunID)
	log.SessionID = strings.TrimSpace(log.SessionID)
	log.Inputs = normalizeAnyMap(log.Inputs)
	log.MatchedEvent = normalizeAnyMap(log.MatchedEvent)
	log.Result = normalizeWorkflowResult(log.Result)
	log.NodeRuns = normalizeWorkflowNodeRuns(log.NodeRuns)
	log.Error = strings.TrimSpace(log.Error)
	log.StartedAt = strings.TrimSpace(log.StartedAt)
	log.FinishedAt = strings.TrimSpace(log.FinishedAt)
	return log
}

func normalizeWorkflowResult(result *WorkflowResult) *WorkflowResult {
	if result == nil {
		return nil
	}
	normalized := *result
	normalized.Format = strings.TrimSpace(normalized.Format)
	normalized.Markdown = strings.TrimSpace(normalized.Markdown)
	normalized.JSON = normalizeAnyMap(normalized.JSON)
	if normalized.RawResponse != nil {
		raw := NormalizeChatResponse(*normalized.RawResponse)
		normalized.RawResponse = &raw
	}
	return &normalized
}

func normalizeWorkflowNodeRuns(nodes []WorkflowNodeRun) []WorkflowNodeRun {
	if len(nodes) == 0 {
		return nil
	}
	normalized := make([]WorkflowNodeRun, 0, len(nodes))
	for _, node := range nodes {
		node.NodeID = strings.TrimSpace(node.NodeID)
		node.NodeType = strings.TrimSpace(node.NodeType)
		node.Title = strings.TrimSpace(node.Title)
		node.Status = strings.ToUpper(strings.TrimSpace(node.Status))
		if node.Status == "" {
			node.Status = WorkflowTriggerLogStatusQueued
		}
		node.StartedAt = strings.TrimSpace(node.StartedAt)
		node.FinishedAt = strings.TrimSpace(node.FinishedAt)
		node.Inputs = normalizeAnyMap(node.Inputs)
		node.Outputs = normalizeAnyMap(node.Outputs)
		node.Error = strings.TrimSpace(node.Error)
		if node.NodeID != "" {
			normalized = append(normalized, node)
		}
	}
	return normalized
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
	if len(response.Runs) == 0 {
		response.Runs = []Run{}
	} else {
		for index := range response.Runs {
			response.Runs[index] = NormalizeRun(response.Runs[index])
		}
	}
	response.ComposerState = enginepersistence.NormalizeSessionComposerState(response.Session.ID, response.ComposerState)
	return response
}

func normalizeOptionalPermissionMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return ""
	}
	return normalizePermissionMode(mode)
}

func normalizeAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return map[string]any{}
	}
	return out
}
