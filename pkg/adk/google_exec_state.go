package adk

import (
	"fmt"
	"strings"
)

func (e *googleADKExecution) markToolResponseSeenLocked(runID string) {
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	e.toolResponseSeenByRunID[runID] = true
	e.toolResponseSeqByRunID[runID]++
}

func (e *googleADKExecution) markToolResponseSeenForRun(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.markToolResponseSeenLocked(runID)
}

func (e *googleADKExecution) toolResponseSeenForRunLocked(runID string) bool {
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	return e.toolResponseSeenByRunID[runID] || e.toolResponseSeqByRunID[runID] > 0
}

func (e *googleADKExecution) markPostToolTextForRun(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.markPostToolTextForRunLocked(runID)
}

func (e *googleADKExecution) markPostToolTextForRunLocked(runID string) {
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	e.postToolTextByRunID[runID] = true
	e.postToolTextSeqByRunID[runID] = e.toolResponseSeqByRunID[runID]
}

func (e *googleADKExecution) collectRunSnapshotDeltasLocked() []ChatDelta {
	deltas := make([]ChatDelta, 0)
	for _, runID := range e.snapshotRunIDsLocked() {
		snapshot := e.runSnapshotLocked(runID, false)
		if e.persistRunSnapshot != nil {
			persisted := e.runSnapshotLocked(runID, true)
			sanitized := persisted
			if sanitized.Status == RunStatusRunning || sanitized.Status == RunStatusPending {
				sanitized.CompletedAt = nil
				sanitized.CancelledAt = nil
				sanitized.Degraded = false
				if sanitized.Status != RunStatusFailed {
					sanitized.Message = ""
					sanitized.FailureReason = ""
					sanitized.ErrorCode = ""
				}
			}
			if saved, err := e.persistRunSnapshot(sanitized); err == nil {
				snapshot = saved
			} else {
				snapshot = sanitized
			}
		}
		if e.onDelta != nil {
			snapshot = NormalizeRun(snapshot)
			deltas = append(deltas, ChatDelta{Run: &snapshot})
		}
	}
	return deltas
}

func (e *googleADKExecution) emitRunSnapshot() {
	e.mu.Lock()
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
}

func (e *googleADKExecution) derivedRunStatusForRunLocked(runID string) string {
	calls := e.callsForRunLocked(runID)
	if len(calls) == 0 {
		if e.runHasTextLocked(runID) {
			return RunStatusCompleted
		}
		return RunStatusRunning
	}
	allCancelled := true
	allCompleted := true
	for _, call := range calls {
		switch call.Status {
		case "PENDING_APPROVAL":
			return RunStatusPending
		case "RUNNING", "PENDING":
			return RunStatusRunning
		case "FAILED", "TIMED_OUT", "DENIED":
			allCompleted = false
			allCancelled = false
		case "SUCCEEDED", "COMPLETED":
			allCancelled = false
		case "CANCELLED":
			allCompleted = false
		default:
			allCompleted = false
			allCancelled = false
		}
	}
	if allCancelled {
		return RunStatusCancelled
	}
	if allCompleted {
		if !e.runHasPostToolTextLocked(runID) {
			return RunStatusRunning
		}
		return RunStatusCompleted
	}
	return RunStatusRunning
}

func (e *googleADKExecution) persistedRunStatusForRunLocked(runID string) string {
	calls := e.callsForRunLocked(runID)
	for _, call := range calls {
		switch strings.ToUpper(strings.TrimSpace(call.Status)) {
		case "PENDING_APPROVAL":
			return RunStatusPending
		}
	}
	return RunStatusRunning
}

func (e *googleADKExecution) runHasPostToolTextLocked(runID string) bool {
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	toolSeq := e.toolResponseSeqByRunID[runID]
	if toolSeq == 0 {
		return false
	}
	return e.postToolTextByRunID[runID] && e.postToolTextSeqByRunID[runID] >= toolSeq
}

func (e *googleADKExecution) runHasPostToolText(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runHasPostToolTextLocked(runID)
}

func (e *googleADKExecution) runNeedsFinalSynthesis(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	calls := e.callsForRunLocked(runID)
	if len(calls) == 0 || e.runHasPostToolTextLocked(runID) {
		return false
	}
	hasFinishedCall := false
	for _, call := range calls {
		switch strings.ToUpper(strings.TrimSpace(call.Status)) {
		case "RUNNING", "PENDING", "PENDING_APPROVAL":
			return false
		case "SUCCEEDED", "COMPLETED", "FAILED", "TIMED_OUT", "DENIED", "CANCELLED":
			hasFinishedCall = true
		}
	}
	return hasFinishedCall
}

func (e *googleADKExecution) runHasTextLocked(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" || runID == e.runID {
		return strings.TrimSpace(e.reply.String()) != "" || strings.TrimSpace(e.reasoning.String()) != ""
	}
	if builder := e.replyByRunID[runID]; builder != nil && strings.TrimSpace(builder.String()) != "" {
		return true
	}
	if builder := e.reasoningByRunID[runID]; builder != nil && strings.TrimSpace(builder.String()) != "" {
		return true
	}
	return false
}

func (e *googleADKExecution) snapshotRunIDsLocked() []string {
	ids := []string{}
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(e.runSnapshotBaseByID) > 0 {
		for id := range e.runSnapshotBaseByID {
			add(id)
		}
	}
	add(e.runID)
	for _, call := range e.calls {
		add(call.RunID)
	}
	return ids
}

func (e *googleADKExecution) runSnapshotLocked(runID string, persisted bool) Run {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	base := e.runBaseLocked(runID)
	calls := e.callsForRunLocked(runID)
	base.ToolCalls = calls
	base.ToolSummaries = toolSummariesForRun(Run{ToolCalls: calls})
	base.Status = e.derivedRunStatusForRunLocked(runID)
	if persisted {
		base.Status = e.persistedRunStatusForRunLocked(runID)
	}
	base.UpdatedAt = nowString()
	if base.CreatedAt == "" {
		base.CreatedAt = ""
	}
	return base
}

func (e *googleADKExecution) runBaseLocked(runID string) Run {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	base := Run{
		ID: runID, SessionID: e.sessionID, AgentID: e.agent.ID,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{},
	}
	if e.runSnapshotBaseByID != nil {
		if candidate, ok := e.runSnapshotBaseByID[runID]; ok {
			base = candidate
		}
	}
	return base
}

func (e *googleADKExecution) callsForRunLocked(runID string) []ToolCall {
	runID = strings.TrimSpace(runID)
	calls := make([]ToolCall, 0, len(e.calls))
	for _, call := range e.calls {
		if runID != "" && call.RunID != runID {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

func summarizeToolCall(call ToolCall) string {
	if call.Output != nil {
		return summarizeToolOutput(call.ToolName, call.Output)
	}
	if call.Error != nil && strings.TrimSpace(*call.Error) != "" {
		return call.ToolName + ": " + strings.TrimSpace(*call.Error)
	}
	return ""
}

func googleADKAgentName(id string) string {
	name := strings.ReplaceAll(normalizeID(id), "-", "_")
	if name == "" {
		return "jftrade_agent"
	}
	if name == "user" {
		return "jftrade_user_agent"
	}
	return name
}

func googleADKWorkflowRootName(parentRunID string) string {
	name := "workflow_" + strings.ReplaceAll(normalizeID(parentRunID), "-", "_")
	if name == "workflow_" {
		return "workflow_root"
	}
	return name
}

func googleADKWorkflowChildName(parentRunID string, index int) string {
	return fmt.Sprintf("%s_child_%d", googleADKWorkflowRootName(parentRunID), index+1)
}

func workflowChildInstruction(base string, task string) string {
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

func workflowChildInstructionTask(step workflowStep) string {
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

func workflowFinalSynthesisInstruction(base string, task string) string {
	instruction := workflowChildInstruction(base, task)
	return instruction + "\n\n工具调用已经完成。现在必须基于已有工具结果输出最终回复。不要再调用工具，不要请求审批，不要只说明准备继续。"
}
