package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	workflowrules "github.com/jftrade/jftrade-main/internal/assistant/workflow"
)

func applyWorkflowResponse(
	log assistantmodel.WorkflowTriggerLog,
	workflow assistantmodel.WorkflowDefinition,
	trigger *assistantmodel.WorkflowTrigger,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response assistantmodel.ChatResponse,
	started string,
	finishedAt time.Time,
) assistantmodel.WorkflowTriggerLog {
	log.SessionID = response.Session.ID
	log.RunID = response.Run.ID
	log.Status = workflowLogStatusFromRun(response.Run)
	if log.Status != assistantmodel.WorkflowTriggerLogStatusRunning && log.Status != assistantmodel.WorkflowTriggerLogStatusPendingApproval {
		log.FinishedAt = finishedAt.Format(time.RFC3339Nano)
	}
	if response.Run.FailureReason != "" {
		log.Error = response.Run.FailureReason
	}
	log.Result = workflowResultFromResponse(response)
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, &response, log.Status, log.Error, started, log.FinishedAt)
	return log
}

func workflowResultFromResponse(response assistantmodel.ChatResponse) *assistantmodel.WorkflowResult {
	result := &assistantmodel.WorkflowResult{
		Format:      "markdown",
		Markdown:    strings.TrimSpace(response.Reply),
		RawResponse: &response,
	}
	if result.Markdown == "" {
		result.Markdown = strings.TrimSpace(response.Run.FailureReason)
	}
	return result
}

func workflowResultFromError(err error) *assistantmodel.WorkflowResult {
	if err == nil {
		return nil
	}
	return &assistantmodel.WorkflowResult{
		Format:   "markdown",
		Markdown: strings.TrimSpace(err.Error()),
		JSON: map[string]any{
			"error": strings.TrimSpace(err.Error()),
		},
	}
}

func workflowNodeRuns(
	workflow assistantmodel.WorkflowDefinition,
	trigger *assistantmodel.WorkflowTrigger,
	triggerType string,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response *assistantmodel.ChatResponse,
	status string,
	errorMessage string,
	startedAt string,
	finishedAt string,
) []assistantmodel.WorkflowNodeRun {
	if workflow.CanvasGraph != nil {
		return workflowCanvasNodeRuns(workflow, trigger, triggerType, inputs, matchedEvent, message, objective, response, status, errorMessage, startedAt, finishedAt)
	}
	context := newWorkflowNodeRunContext(workflow, trigger, triggerType, message, objective, response, status, errorMessage)
	return []assistantmodel.WorkflowNodeRun{
		context.triggerNode(inputs, matchedEvent, startedAt, finishedAt),
		context.startNode(inputs, startedAt, finishedAt),
		context.agentNode(startedAt, finishedAt),
		context.monitorNode(startedAt, finishedAt),
	}
}

func workflowCanvasNodeRuns(
	workflow assistantmodel.WorkflowDefinition,
	trigger *assistantmodel.WorkflowTrigger,
	triggerType string,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response *assistantmodel.ChatResponse,
	status string,
	errorMessage string,
	startedAt string,
	finishedAt string,
) []assistantmodel.WorkflowNodeRun {
	context := newWorkflowNodeRunContext(workflow, trigger, triggerType, message, objective, response, status, errorMessage)
	planByNodeID := map[string]assistantmodel.WorkflowStepState{}
	if response != nil {
		for _, step := range response.Run.WorkflowPlan {
			if strings.TrimSpace(step.PlannerStepID) != "" {
				planByNodeID[step.PlannerStepID] = step
			}
		}
	}
	runs := make([]assistantmodel.WorkflowNodeRun, 0, len(workflow.CanvasGraph.Nodes)+2)
	for _, node := range workflow.CanvasGraph.Nodes {
		nodeType := strings.ToLower(strings.TrimSpace(node.Type))
		switch nodeType {
		case "trigger":
			runs = append(runs, context.canvasTriggerNode(node, inputs, matchedEvent, startedAt, finishedAt))
		case "start":
			runs = append(runs, context.canvasStartNode(node, inputs, startedAt, finishedAt))
		case "agent":
			runs = append(runs, canvasAgentNodeRun(node, planByNodeID[node.ID], errorMessage, startedAt, finishedAt))
		case "monitor":
			runs = append(runs, context.canvasMonitorNode(node, startedAt, finishedAt))
		}
	}
	return runs
}

type workflowNodeRunContext struct {
	workflow       assistantmodel.WorkflowDefinition
	triggerNodeID  string
	triggerTitle   string
	triggerStatus  string
	startStatus    string
	agentStatus    string
	monitorStatus  string
	startOutputs   map[string]any
	agentInputs    map[string]any
	agentOutputs   map[string]any
	monitorOutputs map[string]any
	errorMessage   string
}

func newWorkflowNodeRunContext(
	workflow assistantmodel.WorkflowDefinition,
	trigger *assistantmodel.WorkflowTrigger,
	triggerType string,
	message string,
	objective string,
	response *assistantmodel.ChatResponse,
	status string,
	errorMessage string,
) workflowNodeRunContext {
	status = defaultString(strings.ToUpper(strings.TrimSpace(status)), assistantmodel.WorkflowTriggerLogStatusRunning)
	errorMessage = strings.TrimSpace(errorMessage)
	context := workflowNodeRunContext{
		workflow:      workflow,
		triggerNodeID: "trigger:manual",
		triggerTitle:  "Manual",
		errorMessage:  errorMessage,
	}
	if trigger != nil {
		context.triggerNodeID = "trigger:" + strings.TrimSpace(trigger.ID)
		context.triggerTitle = strings.TrimSpace(trigger.Title)
	}
	if context.triggerTitle == "" {
		context.triggerTitle = workflowrules.DefaultTriggerTitle(defaultString(triggerType, assistantmodel.WorkflowTriggerTypeManual))
	}
	context.applyStatuses(status, message)
	context.startOutputs = workflowStartOutputs(message, objective)
	context.agentInputs = workflowAgentInputs(workflow, message)
	context.agentOutputs, context.monitorOutputs = workflowResponseOutputs(response, errorMessage)
	return context
}

func (c *workflowNodeRunContext) applyStatuses(status string, message string) {
	c.triggerStatus = assistantmodel.WorkflowTriggerLogStatusSucceeded
	c.startStatus = assistantmodel.WorkflowTriggerLogStatusSucceeded
	c.agentStatus = status
	c.monitorStatus = status
	if status == assistantmodel.WorkflowTriggerLogStatusSkipped {
		c.triggerStatus = status
		c.startStatus = status
		c.agentStatus = status
		c.monitorStatus = status
	}
	if strings.TrimSpace(message) == "" && c.errorMessage != "" {
		c.startStatus = assistantmodel.WorkflowTriggerLogStatusFailed
		c.agentStatus = assistantmodel.WorkflowTriggerLogStatusSkipped
		c.monitorStatus = assistantmodel.WorkflowTriggerLogStatusSkipped
	}
}

func workflowStartOutputs(message string, objective string) map[string]any {
	startOutputs := map[string]any{}
	if strings.TrimSpace(message) != "" {
		startOutputs["message"] = message
	}
	if strings.TrimSpace(objective) != "" {
		startOutputs["objective"] = objective
	}
	return startOutputs
}

func workflowAgentInputs(workflow assistantmodel.WorkflowDefinition, message string) map[string]any {
	agentInputs := map[string]any{}
	if strings.TrimSpace(message) != "" {
		agentInputs["message"] = message
	}
	if strings.TrimSpace(workflow.AgentID) != "" {
		agentInputs["agentId"] = workflow.AgentID
	}
	if strings.TrimSpace(workflow.WorkMode) != "" {
		agentInputs["workMode"] = workflow.WorkMode
	}
	return agentInputs
}

func workflowResponseOutputs(response *assistantmodel.ChatResponse, errorMessage string) (map[string]any, map[string]any) {
	agentOutputs := map[string]any{}
	monitorOutputs := map[string]any{}
	if response != nil {
		agentOutputs["reply"] = response.Reply
		agentOutputs["sessionId"] = response.Session.ID
		agentOutputs["runId"] = response.Run.ID
		monitorOutputs["reply"] = response.Reply
		monitorOutputs["sessionId"] = response.Session.ID
		monitorOutputs["runId"] = response.Run.ID
	}
	if errorMessage != "" {
		monitorOutputs["error"] = errorMessage
	}
	return agentOutputs, monitorOutputs
}

func (c workflowNodeRunContext) triggerNode(inputs map[string]any, matchedEvent map[string]any, startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	return assistantmodel.WorkflowNodeRun{
		NodeID:     c.triggerNodeID,
		NodeType:   "trigger",
		Title:      c.triggerTitle,
		Status:     c.triggerStatus,
		StartedAt:  startedAt,
		FinishedAt: defaultString(finishedAt, startedAt),
		Inputs:     cloneMap(inputs),
		Outputs:    cloneMap(matchedEvent),
		Error:      errorForNode(c.triggerStatus, c.errorMessage),
	}
}

func (c workflowNodeRunContext) canvasTriggerNode(node assistantmodel.WorkflowCanvasNode, inputs map[string]any, matchedEvent map[string]any, startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	run := c.triggerNode(inputs, matchedEvent, startedAt, finishedAt)
	run.NodeID = node.ID
	run.Title = canvasNodeTitle(node, run.Title)
	return run
}

func (c workflowNodeRunContext) startNode(inputs map[string]any, startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	return assistantmodel.WorkflowNodeRun{
		NodeID:     "start",
		NodeType:   "start",
		Title:      "Start",
		Status:     c.startStatus,
		StartedAt:  startedAt,
		FinishedAt: defaultString(finishedAt, startedAt),
		Inputs:     cloneMap(inputs),
		Outputs:    c.startOutputs,
		Error:      errorForNode(c.startStatus, c.errorMessage),
	}
}

func (c workflowNodeRunContext) canvasStartNode(node assistantmodel.WorkflowCanvasNode, inputs map[string]any, startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	run := c.startNode(inputs, startedAt, finishedAt)
	run.NodeID = node.ID
	run.Title = canvasNodeTitle(node, run.Title)
	return run
}

func (c workflowNodeRunContext) agentNode(startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	return assistantmodel.WorkflowNodeRun{
		NodeID:     "agent",
		NodeType:   "agent",
		Title:      c.workflow.Name,
		Status:     c.agentStatus,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Inputs:     c.agentInputs,
		Outputs:    c.agentOutputs,
		Error:      errorForNode(c.agentStatus, c.errorMessage),
	}
}

func (c workflowNodeRunContext) monitorNode(startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	return assistantmodel.WorkflowNodeRun{
		NodeID:     "monitor",
		NodeType:   "monitor",
		Title:      "Monitor",
		Status:     c.monitorStatus,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Outputs:    c.monitorOutputs,
		Error:      errorForNode(c.monitorStatus, c.errorMessage),
	}
}

func (c workflowNodeRunContext) canvasMonitorNode(node assistantmodel.WorkflowCanvasNode, startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	run := c.monitorNode(startedAt, finishedAt)
	run.NodeID = node.ID
	run.Title = canvasNodeTitle(node, run.Title)
	return run
}

func canvasAgentNodeRun(node assistantmodel.WorkflowCanvasNode, step assistantmodel.WorkflowStepState, errorMessage string, startedAt string, finishedAt string) assistantmodel.WorkflowNodeRun {
	status := workflowNodeStatusFromStep(step.Status)
	if strings.TrimSpace(step.Status) == "" {
		status = assistantmodel.WorkflowTriggerLogStatusSkipped
	}
	outputs := map[string]any{}
	if strings.TrimSpace(step.ChildRunID) != "" {
		outputs["runId"] = step.ChildRunID
	}
	if strings.TrimSpace(step.OutputSummary) != "" {
		outputs["reply"] = step.OutputSummary
	}
	if strings.TrimSpace(step.ResultSummary) != "" && outputs["reply"] == nil {
		outputs["reply"] = step.ResultSummary
	}
	outputs["status"] = step.Status
	if status != assistantmodel.WorkflowTriggerLogStatusSucceeded && strings.TrimSpace(errorMessage) != "" {
		outputs["error"] = errorMessage
	}
	outputs["toolCalls"] = []any{}
	inputs := map[string]any{}
	if strings.TrimSpace(step.Message) != "" {
		inputs["message"] = step.Message
	}
	if strings.TrimSpace(step.ChildAgentID) != "" {
		inputs["agentId"] = step.ChildAgentID
	}
	if strings.TrimSpace(step.ChildProviderID) != "" {
		inputs["providerId"] = step.ChildProviderID
	}
	if strings.TrimSpace(step.ChildModel) != "" {
		inputs["model"] = step.ChildModel
	}
	return assistantmodel.WorkflowNodeRun{
		NodeID: node.ID, NodeType: "agent", Title: canvasNodeTitle(node, defaultString(step.Title, node.ID)),
		Status: status, StartedAt: startedAt, FinishedAt: finishedAt, Inputs: inputs, Outputs: outputs,
		Error: errorForNode(status, errorMessage),
	}
}

func workflowNodeStatusFromStep(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DONE":
		return assistantmodel.WorkflowTriggerLogStatusSucceeded
	case "IN_PROGRESS":
		return assistantmodel.WorkflowTriggerLogStatusRunning
	case "TODO":
		return assistantmodel.WorkflowTriggerLogStatusQueued
	case "BLOCKED", "CANCELLED":
		return assistantmodel.WorkflowTriggerLogStatusFailed
	default:
		return strings.ToUpper(strings.TrimSpace(status))
	}
}

func canvasNodeTitle(node assistantmodel.WorkflowCanvasNode, fallback string) string {
	if node.Data != nil {
		if title := strings.TrimSpace(fmt.Sprint(node.Data["title"])); title != "" && title != "<nil>" {
			return title
		}
	}
	return fallback
}

func errorForNode(status string, message string) string {
	switch status {
	case assistantmodel.WorkflowTriggerLogStatusFailed, assistantmodel.WorkflowTriggerLogStatusCancelled, assistantmodel.WorkflowTriggerLogStatusSkipped:
		return strings.TrimSpace(message)
	default:
		return ""
	}
}

func (s *Service) workflowStore() (*jfadkruntime.Store, error) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store(), nil
}

func (s *Service) validateWorkflowDefinition(ctx context.Context, workflow assistantmodel.WorkflowDefinition) error {
	if strings.TrimSpace(workflow.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if strings.TrimSpace(workflow.AgentID) == "" {
		return fmt.Errorf("workflow agentId is required")
	}
	agent, ok, err := s.runtime.Store().Agent(ctx, workflow.AgentID)
	if err != nil {
		return err
	}
	if !ok || agent.DeletedAt != nil {
		return fmt.Errorf("workflow agent not found")
	}
	if workflow.Status == assistantmodel.WorkflowStatusEnabled && agent.Status != assistantmodel.AgentStatusEnabled {
		return fmt.Errorf("enabled workflow requires an enabled agent")
	}
	switch strings.ToLower(strings.TrimSpace(workflow.WorkMode)) {
	case assistantmodel.WorkModeChat, assistantmodel.WorkModeLoop:
	default:
		return fmt.Errorf("invalid workflow work mode")
	}
	if strings.TrimSpace(workflow.PromptTemplate) == "" {
		return fmt.Errorf("workflow promptTemplate is required")
	}
	return nil
}

func (s *Service) prepareWorkflowTriggerSchedule(trigger *assistantmodel.WorkflowTrigger, now time.Time) error {
	if trigger == nil || trigger.Type != assistantmodel.WorkflowTriggerTypeSchedule {
		if trigger != nil {
			trigger.NextRunAt = ""
		}
		return nil
	}
	next, err := workflowrules.NextScheduleRun(trigger.Config, now)
	if err != nil {
		return err
	}
	if trigger.Status == assistantmodel.WorkflowTriggerStatusEnabled {
		trigger.NextRunAt = next.Format(time.RFC3339Nano)
	} else {
		trigger.NextRunAt = ""
	}
	return nil
}

func (s *Service) workflowTriggerHasActiveRun(ctx context.Context, triggerID string) (bool, error) {
	return workflowTriggerHasActiveRun(ctx, s.runtime.Store(), triggerID)
}

func workflowTriggerHasActiveRun(ctx context.Context, store workflowInvocationStore, triggerID string) (bool, error) {
	logs, err := store.ListActiveWorkflowTriggerLogs(ctx, triggerID)
	if err != nil {
		return false, err
	}
	active := false
	for _, log := range logs {
		if log.RunID == "" {
			active = true
			continue
		}
		run, ok, err := store.Run(ctx, log.RunID)
		if err != nil {
			return false, err
		}
		if !ok {
			log = finishWorkflowLog(ctx, store, log, assistantmodel.WorkflowTriggerLogStatusFailed, "run not found")
			continue
		}
		status := workflowLogStatusFromRun(run)
		if status == assistantmodel.WorkflowTriggerLogStatusRunning || status == assistantmodel.WorkflowTriggerLogStatusPendingApproval {
			active = true
			continue
		}
		log.Status = status
		if run.FailureReason != "" {
			log.Error = run.FailureReason
		}
		if log.FinishedAt == "" {
			log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		_, _ = store.SaveWorkflowTriggerLog(ctx, log)
	}
	return active, nil
}

func (s *Service) reconcileActiveWorkflowLogs(ctx context.Context) {
	store := s.runtime.Store()
	for _, status := range []string{
		assistantmodel.WorkflowTriggerLogStatusQueued,
		assistantmodel.WorkflowTriggerLogStatusRunning,
		assistantmodel.WorkflowTriggerLogStatusPendingApproval,
	} {
		logs, _, err := store.ListWorkflowTriggerLogsPage(ctx, "", "", status, 100, 0)
		if err != nil {
			continue
		}
		for _, log := range logs {
			if log.TriggerID == "" {
				continue
			}
			_, _ = s.workflowTriggerHasActiveRun(ctx, log.TriggerID)
		}
	}
}

func (s *Service) updateTriggerAfterRun(ctx context.Context, trigger *assistantmodel.WorkflowTrigger, runID string, lastError string) {
	if trigger == nil || s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return
	}
	current, ok, err := s.runtime.Store().WorkflowTrigger(ctx, trigger.ID)
	if err != nil || !ok {
		return
	}
	current.LastRunAt = time.Now().UTC().Format(time.RFC3339Nano)
	current.LastRunID = strings.TrimSpace(runID)
	current.LastError = strings.TrimSpace(lastError)
	_, _ = s.runtime.Store().SaveWorkflowTrigger(ctx, current)
}

func finishWorkflowLog(ctx context.Context, store workflowInvocationStore, log assistantmodel.WorkflowTriggerLog, status string, message string) assistantmodel.WorkflowTriggerLog {
	log.Status = status
	log.Error = strings.TrimSpace(message)
	if log.FinishedAt == "" {
		log.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	updated, err := store.SaveWorkflowTriggerLog(ctx, log)
	if err != nil {
		return log
	}
	return updated
}

func workflowLogStatusFromRun(run assistantmodel.Run) string {
	switch run.Status {
	case assistantmodel.RunStatusCompleted:
		return assistantmodel.WorkflowTriggerLogStatusSucceeded
	case assistantmodel.RunStatusPending:
		return assistantmodel.WorkflowTriggerLogStatusPendingApproval
	case assistantmodel.RunStatusCancelled, assistantmodel.RunStatusDenied:
		return assistantmodel.WorkflowTriggerLogStatusCancelled
	case assistantmodel.RunStatusFailed, assistantmodel.RunStatusTimedOut:
		return assistantmodel.WorkflowTriggerLogStatusFailed
	default:
		return assistantmodel.WorkflowTriggerLogStatusRunning
	}
}
