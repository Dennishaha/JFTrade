package adk

import (
	"encoding/json"
	"strings"

	adksession "google.golang.org/adk/v2/session"
)

func (e *googleADKExecution) observeWorkflowEvent(event *adksession.Event) {
	if e == nil || event == nil || (event.NodeInfo == nil && len(event.Routes) == 0 && event.Output == nil) {
		return
	}
	nodeName := workflowEventNodeName(event)
	runID := e.runIDForAgentName(event.Author)
	if runID == e.runID && nodeName != "" {
		if mapped := e.runIDForAgentName(nodeName); mapped != e.runID {
			runID = mapped
		}
	}
	e.mu.Lock()
	parent := e.runSnapshotBaseByID[e.runID]
	if len(parent.WorkflowPlan) == 0 {
		e.mu.Unlock()
		return
	}
	changed := false
	for index := range parent.WorkflowPlan {
		step := &parent.WorkflowPlan[index]
		if !workflowObservationMatchesStep(*step, runID, nodeName) {
			continue
		}
		if nodeName != "" {
			step.NodeName = nodeName
		}
		if len(event.Routes) > 0 {
			step.Routes = normalizeStringSlice(event.Routes)
		}
		if summary := summarizeWorkflowOutput(event.Output); summary != "" {
			step.OutputSummary = summary
		}
		if status := workflowNodeStatus(event); status != "" {
			step.NodeStatus = status
		}
		changed = true
		break
	}
	if !changed {
		e.mu.Unlock()
		return
	}
	e.runSnapshotBaseByID[e.runID] = parent
	if current, ok := e.runSnapshotBaseByID[runID]; ok && runID != e.runID {
		current.WorkflowPlan = parent.WorkflowPlan
		e.runSnapshotBaseByID[runID] = current
	}
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
}

func (e *googleADKExecution) workflowRunObserved(runID string) bool {
	if e == nil {
		return false
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	parent := e.runSnapshotBaseByID[e.runID]
	for _, step := range parent.WorkflowPlan {
		if strings.TrimSpace(step.ChildRunID) != runID {
			continue
		}
		return strings.TrimSpace(step.NodeStatus) != "" ||
			strings.TrimSpace(step.OutputSummary) != "" ||
			len(step.Routes) > 0
	}
	return false
}

func workflowEventNodeName(event *adksession.Event) string {
	if event == nil {
		return ""
	}
	if event.NodeInfo != nil {
		if path := strings.TrimSpace(event.NodeInfo.Path); path != "" {
			if slash := strings.IndexByte(path, '/'); slash >= 0 {
				path = path[:slash]
			}
			if at := strings.IndexByte(path, '@'); at >= 0 {
				path = path[:at]
			}
			if strings.TrimSpace(path) != "" {
				return strings.TrimSpace(path)
			}
		}
	}
	return strings.TrimSpace(event.Author)
}

func workflowObservationMatchesStep(step WorkflowStepState, runID string, nodeName string) bool {
	if strings.TrimSpace(step.ChildRunID) != "" && strings.TrimSpace(step.ChildRunID) == strings.TrimSpace(runID) {
		return true
	}
	if strings.TrimSpace(step.NodeName) != "" && strings.TrimSpace(step.NodeName) == strings.TrimSpace(nodeName) {
		return true
	}
	return strings.TrimSpace(nodeName) != "" && strings.Contains(strings.TrimSpace(step.NodeName), strings.TrimSpace(nodeName))
}

func workflowNodeStatus(event *adksession.Event) string {
	if event == nil {
		return ""
	}
	if event.RequestedInput != nil {
		return "FAILED"
	}
	if event.Output != nil {
		return "COMPLETED"
	}
	if event.Partial {
		return "RUNNING"
	}
	return "RUNNING"
}

func summarizeWorkflowOutput(output any) string {
	if output == nil {
		return ""
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return strings.TrimSpace(jsonFallbackString(output))
	}
	text := string(raw)
	if len(text) > 600 {
		text = text[:600] + "...(truncated)"
	}
	return text
}

func jsonFallbackString(value any) string {
	raw, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		return ""
	}
	return string(raw)
}
