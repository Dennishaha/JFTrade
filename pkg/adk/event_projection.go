package adk

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	adksession "google.golang.org/adk/session"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

type SessionProjection struct {
	SessionID        string
	Messages         []TranscriptEntry
	LatestAssistant  *TranscriptEntry
	Reply            string
	ReasoningContent string
	ToolCalls        []ToolCall
	PendingApprovals []Approval
	PreToolContent   string
	PreToolReasoning string
	FinalMessageID   string
}

type projectedRunState struct {
	runID            string
	entryIndex       int
	entryID          string
	createdAt        string
	reply            strings.Builder
	reasoning        strings.Builder
	preToolContent   string
	preToolReasoning string
	preToolCaptured  bool
	toolCalls        map[string]*ToolCall
	toolCallOrder    []string
}

func (s *Store) TranscriptEntries(ctx context.Context, sessionID string) ([]TranscriptEntry, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return []TranscriptEntry{}, nil
	}
	projected, ok, err := s.SessionProjection(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if ok {
		return projected.Messages, nil
	}
	return []TranscriptEntry{}, nil
}

func (s *Store) Messages(ctx context.Context, sessionID string) ([]Message, error) {
	return s.TranscriptEntries(ctx, sessionID)
}

func (s *Store) SessionProjection(ctx context.Context, sessionID string) (SessionProjection, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || s == nil {
		return SessionProjection{}, false, nil
	}
	s.mu.RLock()
	service := s.sessions
	s.mu.RUnlock()
	if service == nil {
		return SessionProjection{SessionID: sessionID}, false, nil
	}
	session, ok, err := s.Session(ctx, sessionID)
	if err != nil || !ok {
		return SessionProjection{}, false, err
	}
	response, err := service.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(session.AgentID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil || response == nil || response.Session == nil {
		return SessionProjection{SessionID: session.ID}, false, nil
	}
	projection := sessionProjectionFromADKEvents(eventSlice(response.Session.Events()))
	projection.SessionID = session.ID
	for index := range projection.Messages {
		projection.Messages[index].SessionID = session.ID
	}
	latestRun, hasRun, err := s.latestRunBySession(ctx, session.ID)
	if err != nil {
		return SessionProjection{}, false, err
	}
	if hasRun {
		latestPendingApprovals := pendingApprovalsOnly(latestRun.PendingApprovals)
		if len(projection.PendingApprovals) == 0 && len(latestPendingApprovals) > 0 {
			projection.PendingApprovals = latestPendingApprovals
		}
		if len(projection.ToolCalls) == 0 && len(latestRun.ToolCalls) > 0 {
			projection.ToolCalls = append([]ToolCall(nil), latestRun.ToolCalls...)
		}
		if strings.TrimSpace(projection.PreToolContent) == "" {
			projection.PreToolContent = strings.TrimSpace(latestRun.PreToolContent)
		}
		if strings.TrimSpace(projection.PreToolReasoning) == "" {
			projection.PreToolReasoning = strings.TrimSpace(latestRun.PreToolReasoning)
		}
		if strings.TrimSpace(projection.FinalMessageID) == "" {
			projection.FinalMessageID = strings.TrimSpace(latestRun.FinalMessageID)
		}
	}
	if projection.LatestAssistant != nil && strings.TrimSpace(projection.FinalMessageID) == "" {
		projection.FinalMessageID = projection.LatestAssistant.ID
	}
	hasProjection := len(projection.Messages) > 0 ||
		len(projection.ToolCalls) > 0 ||
		len(projection.PendingApprovals) > 0 ||
		strings.TrimSpace(projection.PreToolContent) != "" ||
		strings.TrimSpace(projection.PreToolReasoning) != ""
	return projection, hasProjection, nil
}

func (s *Store) latestRunBySession(ctx context.Context, sessionID string) (Run, bool, error) {
	runs, _, err := s.ListRunsPage(ctx, "", "", sessionID, 1, 0)
	if err != nil || len(runs) == 0 {
		return Run{}, false, err
	}
	return runs[0], true, nil
}

func sessionProjectionFromADKEvents(events []*adksession.Event) SessionProjection {
	sort.SliceStable(events, func(i, j int) bool {
		left := events[i]
		right := events[j]
		switch {
		case left == nil && right == nil:
			return false
		case left == nil:
			return false
		case right == nil:
			return true
		case left.Timestamp.Equal(right.Timestamp):
			return strings.TrimSpace(left.ID) < strings.TrimSpace(right.ID)
		default:
			return left.Timestamp.Before(right.Timestamp)
		}
	})
	entries := make([]TranscriptEntry, 0, len(events))
	runStates := map[string]*projectedRunState{}
	runOrder := make([]string, 0, len(events))
	for _, event := range events {
		if event == nil || event.Content == nil || len(event.Content.Parts) == 0 {
			continue
		}
		if isUserEvent(event) {
			if event.Partial {
				continue
			}
			entry, ok := transcriptEntryFromADKEvent(event)
			if ok {
				entries = append(entries, entry)
			}
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil {
				continue
			}
			state := ensureProjectedRunState(runStates, &runOrder, &entries, event)
			switch {
			case part.FunctionCall != nil:
				if part.FunctionCall.Name == toolconfirmation.FunctionCallName {
					continue
				}
				ensureProjectedToolCall(state, part.FunctionCall, eventTimeString(event))
			case part.FunctionResponse != nil:
				if part.FunctionResponse.Name == toolconfirmation.FunctionCallName {
					continue
				}
				projectedToolResponse(state, part.FunctionResponse, eventTimeString(event))
			case part.Text != "":
				reply, reasoning := visibleTextFromParts([]*genai.Part{part})
				mergeProjectedText(&state.reply, reply, event.Partial)
				mergeProjectedText(&state.reasoning, reasoning, event.Partial)
				if textID := strings.TrimSpace(event.ID); textID != "" {
					state.entryID = textID
				}
			}
		}
	}

	projection := SessionProjection{}
	for _, runID := range runOrder {
		state := runStates[runID]
		if state == nil {
			continue
		}
		if state.entryIndex >= 0 && state.entryIndex < len(entries) {
			entry := &entries[state.entryIndex]
			entry.ID = defaultString(strings.TrimSpace(state.entryID), entry.ID)
			entry.Content = strings.TrimSpace(state.reply.String())
			entry.ReasoningContent = strings.TrimSpace(state.reasoning.String())
		}
		if strings.TrimSpace(state.preToolContent) != "" || strings.TrimSpace(state.preToolReasoning) != "" {
			projection.PreToolContent = strings.TrimSpace(state.preToolContent)
			projection.PreToolReasoning = strings.TrimSpace(state.preToolReasoning)
		}
		for _, toolCallID := range state.toolCallOrder {
			call := state.toolCalls[toolCallID]
			if call == nil {
				continue
			}
			projection.ToolCalls = append(projection.ToolCalls, *call)
		}
	}

	filtered := make([]TranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Role == "assistant" &&
			strings.TrimSpace(entry.Content) == "" &&
			strings.TrimSpace(entry.ReasoningContent) == "" {
			continue
		}
		filtered = append(filtered, entry)
		if entry.Role == "assistant" {
			projection.LatestAssistant = new(entry)
		}
	}
	projection.Messages = filtered
	if projection.LatestAssistant != nil {
		projection.Reply = projection.LatestAssistant.Content
		projection.ReasoningContent = projection.LatestAssistant.ReasoningContent
		projection.FinalMessageID = projection.LatestAssistant.ID
	}
	return projection
}

func ensureProjectedRunState(
	runStates map[string]*projectedRunState,
	runOrder *[]string,
	entries *[]TranscriptEntry,
	event *adksession.Event,
) *projectedRunState {
	runID := projectionRunID(event)
	if state, ok := runStates[runID]; ok {
		return state
	}
	createdAt := eventTimeString(event)
	entryID := strings.TrimSpace(event.ID)
	if entryID == "" {
		entryID = "event-message-" + runID
	}
	*entries = append(*entries, TranscriptEntry{
		ID:        entryID,
		RunID:     runID,
		Role:      "assistant",
		Kind:      transcriptKindMessage,
		CreatedAt: createdAt,
	})
	state := &projectedRunState{
		runID:         runID,
		entryIndex:    len(*entries) - 1,
		entryID:       entryID,
		createdAt:     createdAt,
		toolCalls:     map[string]*ToolCall{},
		toolCallOrder: []string{},
	}
	runStates[runID] = state
	*runOrder = append(*runOrder, runID)
	return state
}

func ensureProjectedToolCall(state *projectedRunState, call *genai.FunctionCall, timestamp string) *ToolCall {
	if state == nil || call == nil {
		return nil
	}
	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		callID = call.Name + ":" + timestamp
	}
	if existing, ok := state.toolCalls[callID]; ok {
		return existing
	}
	if !state.preToolCaptured {
		state.preToolCaptured = true
		state.preToolContent = strings.TrimSpace(state.reply.String())
		state.preToolReasoning = strings.TrimSpace(state.reasoning.String())
	}
	projected := &ToolCall{
		ID:             "event-tool-" + callID,
		RunID:          state.runID,
		ToolName:       strings.TrimSpace(call.Name),
		Status:         "RUNNING",
		Input:          call.Args,
		IdempotencyKey: callID,
		CreatedAt:      timestamp,
		StartedAt:      timestamp,
		UpdatedAt:      timestamp,
	}
	state.toolCalls[callID] = projected
	state.toolCallOrder = append(state.toolCallOrder, callID)
	return projected
}

func projectedToolResponse(state *projectedRunState, response *genai.FunctionResponse, timestamp string) {
	if state == nil || response == nil {
		return
	}
	call := ensureProjectedToolCall(state, &genai.FunctionCall{
		ID:   response.ID,
		Name: response.Name,
	}, timestamp)
	if call == nil {
		return
	}
	call.UpdatedAt = timestamp
	if errorValue, ok := response.Response["error"]; ok {
		errText := fmt.Sprint(errorValue)
		if strings.Contains(errText, adktool.ErrConfirmationRequired.Error()) {
			call.Status = "PENDING_APPROVAL"
			call.RequiresUser = true
			return
		}
		call.Status = "FAILED"
		call.Error = &errText
		finishToolCall(call)
		return
	}
	call.Status = "SUCCEEDED"
	call.Output = limitToolOutput(response.Response)
	finishToolCall(call)
}

func mergeProjectedText(builder *strings.Builder, text string, partial bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	current := builder.String()
	switch {
	case partial || current == "":
		builder.WriteString(text)
	case strings.HasPrefix(text, current):
		builder.Reset()
		builder.WriteString(text)
	case strings.HasSuffix(current, text):
		return
	default:
		builder.WriteString(text)
	}
}

func projectedToolProgress(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "unknown"
	}
	return fmt.Sprintf("🔧 执行工具 %s...", toolName)
}

func projectionRunID(event *adksession.Event) string {
	if event == nil {
		return ""
	}
	if runID := strings.TrimSpace(event.InvocationID); runID != "" {
		return runID
	}
	if eventID := strings.TrimSpace(event.ID); eventID != "" {
		return eventID
	}
	return eventTimeString(event)
}

func eventTimeString(event *adksession.Event) string {
	if event != nil && !event.Timestamp.IsZero() {
		return event.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return nowString()
}

func transcriptEntryFromADKEvent(event *adksession.Event) (TranscriptEntry, bool) {
	if event == nil || event.Content == nil || len(event.Content.Parts) == 0 {
		return TranscriptEntry{}, false
	}
	role := "assistant"
	if strings.EqualFold(strings.TrimSpace(event.Author), "user") || event.Content.Role == genai.RoleUser {
		role = "user"
	}
	content, reasoning := visibleTextFromParts(event.Content.Parts)
	if content == "" && reasoning == "" {
		return TranscriptEntry{}, false
	}
	createdAt := eventTimeString(event)
	id := strings.TrimSpace(event.ID)
	if id == "" {
		id = "event-message-" + strings.TrimSpace(event.InvocationID)
		if id == "event-message-" {
			id = "event-message-" + createdAt
		}
	}
	return TranscriptEntry{
		ID:               id,
		SessionID:        "",
		RunID:            strings.TrimSpace(event.InvocationID),
		Role:             role,
		Kind:             transcriptKindMessage,
		Content:          content,
		ReasoningContent: reasoning,
		CreatedAt:        createdAt,
	}, true
}

func visibleTextFromParts(parts []*genai.Part) (string, string) {
	var reply strings.Builder
	var reasoning strings.Builder
	for _, part := range parts {
		if part == nil || part.Text == "" {
			continue
		}
		if part.Thought {
			reasoning.WriteString(part.Text)
			continue
		}
		reply.WriteString(part.Text)
	}
	return strings.TrimSpace(reply.String()), strings.TrimSpace(reasoning.String())
}

func partsFromReplyAndReasoning(reply string, reasoning string) []*genai.Part {
	parts := make([]*genai.Part, 0, 2)
	if trimmedReasoning := strings.TrimSpace(reasoning); trimmedReasoning != "" {
		parts = append(parts, &genai.Part{Text: trimmedReasoning, Thought: true})
	}
	if trimmedReply := strings.TrimSpace(reply); trimmedReply != "" {
		parts = append(parts, &genai.Part{Text: trimmedReply})
	}
	return parts
}
