package adk

import (
	"strings"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func (e *googleADKExecution) result() assistantExecutionResult {
	return e.ResultForRun(e.runID)
}

func (e *googleADKExecution) ResultForRun(runID string) assistantExecutionResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if runID == e.runID {
		return assistantExecutionResult{
			Reply: strings.TrimSpace(e.reply.String()), ReasoningContent: strings.TrimSpace(e.reasoning.String()),
			SourceEventID: strings.TrimSpace(e.finalMessageIDByRunID[runID]), SourceInvocationID: strings.TrimSpace(e.finalInvocationIDByRunID[runID]),
		}
	}
	reply := e.replyByRunID[runID]
	reasoning := e.reasoningByRunID[runID]
	var replyText, reasoningText string
	if reply != nil {
		replyText = reply.String()
	}
	if reasoning != nil {
		reasoningText = reasoning.String()
	}
	return assistantExecutionResult{
		Reply: strings.TrimSpace(replyText), ReasoningContent: strings.TrimSpace(reasoningText),
		SourceEventID: strings.TrimSpace(e.finalMessageIDByRunID[runID]), SourceInvocationID: strings.TrimSpace(e.finalInvocationIDByRunID[runID]),
	}
}

func (e *googleADKExecution) recordFinalMessageIDLocked(event *adksession.Event) {
	if event == nil || event.Content == nil || event.Partial || isUserEvent(event) || strings.TrimSpace(event.ID) == "" || !contentHasText(event.Content) {
		return
	}
	if event.Content.Role != "" && event.Content.Role != genai.RoleModel {
		return
	}
	if e.finalMessageIDByRunID == nil {
		e.finalMessageIDByRunID = map[string]string{}
	}
	if e.finalInvocationIDByRunID == nil {
		e.finalInvocationIDByRunID = map[string]string{}
	}
	runID := e.runIDForAgentName(event.Author)
	e.finalMessageIDByRunID[runID] = strings.TrimSpace(event.ID)
	e.finalInvocationIDByRunID[runID] = strings.TrimSpace(event.InvocationID)
}
