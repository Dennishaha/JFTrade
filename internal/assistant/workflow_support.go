package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	workflowrules "github.com/jftrade/jftrade-main/internal/assistant/workflow"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func applyWorkflowResponse(
	log jfadk.WorkflowTriggerLog,
	workflow jfadk.WorkflowDefinition,
	trigger *jfadk.WorkflowTrigger,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response jfadk.ChatResponse,
	started string,
	finishedAt time.Time,
) jfadk.WorkflowTriggerLog {
	log.SessionID = response.Session.ID
	log.RunID = response.Run.ID
	log.Status = workflowLogStatusFromRun(response.Run)
	if log.Status != jfadk.WorkflowTriggerLogStatusRunning && log.Status != jfadk.WorkflowTriggerLogStatusPendingApproval {
		log.FinishedAt = finishedAt.Format(time.RFC3339Nano)
	}
	if response.Run.FailureReason != "" {
		log.Error = response.Run.FailureReason
	}
	log.Result = workflowResultFromResponse(response)
	log.NodeRuns = workflowNodeRuns(workflow, trigger, log.TriggerType, inputs, matchedEvent, message, objective, &response, log.Status, log.Error, started, log.FinishedAt)
	return log
}

func workflowResultFromResponse(response jfadk.ChatResponse) *jfadk.WorkflowResult {
	result := &jfadk.WorkflowResult{
		Format:      "markdown",
		Markdown:    strings.TrimSpace(response.Reply),
		RawResponse: &response,
	}
	if result.Markdown == "" {
		result.Markdown = strings.TrimSpace(response.Run.FailureReason)
	}
	return result
}

func workflowResultFromError(err error) *jfadk.WorkflowResult {
	if err == nil {
		return nil
	}
	return &jfadk.WorkflowResult{
		Format:   "markdown",
		Markdown: strings.TrimSpace(err.Error()),
		JSON: map[string]any{
			"error": strings.TrimSpace(err.Error()),
		},
	}
}

func workflowNodeRuns(
	workflow jfadk.WorkflowDefinition,
	trigger *jfadk.WorkflowTrigger,
	triggerType string,
	inputs map[string]any,
	matchedEvent map[string]any,
	message string,
	objective string,
	response *jfadk.ChatResponse,
	status string,
	errorMessage string,
	startedAt string,
	finishedAt string,
) []jfadk.WorkflowNodeRun {
	status = defaultString(strings.ToUpper(strings.TrimSpace(status)), jfadk.WorkflowTriggerLogStatusRunning)
	errorMessage = strings.TrimSpace(errorMessage)
	triggerNodeID := "trigger:manual"
	triggerTitle := "Manual"
	if trigger != nil {
		triggerNodeID = "trigger:" + strings.TrimSpace(trigger.ID)
		triggerTitle = strings.TrimSpace(trigger.Title)
	}
	if triggerTitle == "" {
		triggerTitle = workflowrules.DefaultTriggerTitle(defaultString(triggerType, jfadk.WorkflowTriggerTypeManual))
	}

	triggerStatus := jfadk.WorkflowTriggerLogStatusSucceeded
	startStatus := jfadk.WorkflowTriggerLogStatusSucceeded
	agentStatus := status
	monitorStatus := status
	if status == jfadk.WorkflowTriggerLogStatusSkipped {
		triggerStatus = status
		startStatus = status
		agentStatus = status
		monitorStatus = status
	}
	if strings.TrimSpace(message) == "" && errorMessage != "" {
		startStatus = jfadk.WorkflowTriggerLogStatusFailed
		agentStatus = jfadk.WorkflowTriggerLogStatusSkipped
		monitorStatus = jfadk.WorkflowTriggerLogStatusSkipped
	}

	startOutputs := map[string]any{}
	if strings.TrimSpace(message) != "" {
		startOutputs["message"] = message
	}
	if strings.TrimSpace(objective) != "" {
		startOutputs["objective"] = objective
	}

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

	return []jfadk.WorkflowNodeRun{
		{
			NodeID:     triggerNodeID,
			NodeType:   "trigger",
			Title:      triggerTitle,
			Status:     triggerStatus,
			StartedAt:  startedAt,
			FinishedAt: defaultString(finishedAt, startedAt),
			Inputs:     cloneMap(inputs),
			Outputs:    cloneMap(matchedEvent),
			Error:      errorForNode(triggerStatus, errorMessage),
		},
		{
			NodeID:     "start",
			NodeType:   "start",
			Title:      "Start",
			Status:     startStatus,
			StartedAt:  startedAt,
			FinishedAt: defaultString(finishedAt, startedAt),
			Inputs:     cloneMap(inputs),
			Outputs:    startOutputs,
			Error:      errorForNode(startStatus, errorMessage),
		},
		{
			NodeID:     "agent",
			NodeType:   "agent",
			Title:      workflow.Name,
			Status:     agentStatus,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Inputs:     agentInputs,
			Outputs:    agentOutputs,
			Error:      errorForNode(agentStatus, errorMessage),
		},
		{
			NodeID:     "monitor",
			NodeType:   "monitor",
			Title:      "Monitor",
			Status:     monitorStatus,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Outputs:    monitorOutputs,
			Error:      errorForNode(monitorStatus, errorMessage),
		},
	}
}

func errorForNode(status string, message string) string {
	switch status {
	case jfadk.WorkflowTriggerLogStatusFailed, jfadk.WorkflowTriggerLogStatusCancelled, jfadk.WorkflowTriggerLogStatusSkipped:
		return strings.TrimSpace(message)
	default:
		return ""
	}
}

func (s *Service) workflowStore() (*jfadk.Store, error) {
	if s == nil || s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store(), nil
}

func (s *Service) validateWorkflowDefinition(ctx context.Context, workflow jfadk.WorkflowDefinition) error {
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
	if workflow.Status == jfadk.WorkflowStatusEnabled && agent.Status != jfadk.AgentStatusEnabled {
		return fmt.Errorf("enabled workflow requires an enabled agent")
	}
	if strings.TrimSpace(workflow.PromptTemplate) == "" {
		return fmt.Errorf("workflow promptTemplate is required")
	}
	return nil
}

func (s *Service) prepareWorkflowTriggerSchedule(trigger *jfadk.WorkflowTrigger, now time.Time) error {
	if trigger == nil || trigger.Type != jfadk.WorkflowTriggerTypeSchedule {
		if trigger != nil {
			trigger.NextRunAt = ""
		}
		return nil
	}
	next, err := workflowrules.NextScheduleRun(trigger.Config, now)
	if err != nil {
		return err
	}
	if trigger.Status == jfadk.WorkflowTriggerStatusEnabled {
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
			log = finishWorkflowLog(ctx, store, log, jfadk.WorkflowTriggerLogStatusFailed, "run not found")
			continue
		}
		status := workflowLogStatusFromRun(run)
		if status == jfadk.WorkflowTriggerLogStatusRunning || status == jfadk.WorkflowTriggerLogStatusPendingApproval {
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
		jfadk.WorkflowTriggerLogStatusQueued,
		jfadk.WorkflowTriggerLogStatusRunning,
		jfadk.WorkflowTriggerLogStatusPendingApproval,
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

func (s *Service) updateTriggerAfterRun(ctx context.Context, trigger *jfadk.WorkflowTrigger, runID string, lastError string) {
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

func finishWorkflowLog(ctx context.Context, store workflowInvocationStore, log jfadk.WorkflowTriggerLog, status string, message string) jfadk.WorkflowTriggerLog {
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

func workflowLogStatusFromRun(run jfadk.Run) string {
	switch run.Status {
	case jfadk.RunStatusCompleted:
		return jfadk.WorkflowTriggerLogStatusSucceeded
	case jfadk.RunStatusPending:
		return jfadk.WorkflowTriggerLogStatusPendingApproval
	case jfadk.RunStatusCancelled, jfadk.RunStatusDenied:
		return jfadk.WorkflowTriggerLogStatusCancelled
	case jfadk.RunStatusFailed, jfadk.RunStatusTimedOut:
		return jfadk.WorkflowTriggerLogStatusFailed
	default:
		return jfadk.WorkflowTriggerLogStatusRunning
	}
}
