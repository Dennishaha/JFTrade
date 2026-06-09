package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

type adkChatStreamEvent struct {
	Type     string                        `json:"type"`
	Timeline *jfadk.TimelineEntry          `json:"timeline,omitempty"`
	Response *jfadk.ChatResponse           `json:"response,omitempty"`
	Session  *jfadk.Session                `json:"session,omitempty"`
	Run      *jfadk.Run                    `json:"run,omitempty"`
	Context  *jfadk.SessionContextSnapshot `json:"context,omitempty"`
	Message  string                        `json:"message,omitempty"`
}

type adkTimelineStreamState struct {
	sessionID      string
	runID          string
	nextSequence   int
	reasoningIndex int
	messageIndex   int
	toolIndex      int
	reasoning      *jfadk.TimelineEntry
	message        *jfadk.TimelineEntry
	toolGroup      *jfadk.TimelineEntry
}

func (s *Server) handleADKChat(c *gin.Context) {
	if s.adkRuntime == nil {
		s.writeError(c, http.StatusServiceUnavailable, "ADK_UNAVAILABLE", "ADK runtime is unavailable")
		return
	}
	payload, err := decodeADKChatRequest(c.Request.Body)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid chat payload")
		return
	}
	response, err := s.adkRuntime.Chat(c.Request.Context(), payload)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "ADK_CHAT_FAILED", err.Error())
		return
	}
	s.writeOK(c, response)
}

func (s *Server) handleADKChatStream(c *gin.Context) {
	if s.adkRuntime == nil {
		s.writeError(c, http.StatusServiceUnavailable, "ADK_UNAVAILABLE", "ADK runtime is unavailable")
		return
	}
	c.Header("X-ADK-Stream-Idle-Timeout-Ms", strconv.Itoa(s.store.adkSettings().StreamIdleTimeoutMs))
	payload, err := decodeADKChatRequest(c.Request.Body)
	if err != nil {
		writer, ok := prepareSSEWriter(c.Writer)
		if !ok {
			s.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
			return
		}
		if err := writer.WriteRetryDirective(); err != nil {
			return
		}
		_ = writer.WriteEvent(adkChatStreamEvent{Type: "error", Message: "invalid chat payload: " + err.Error()})
		return
	}
	writer, ok := prepareSSEWriter(c.Writer)
	if !ok {
		s.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
		return
	}
	if err := writer.WriteRetryDirective(); err != nil {
		return
	}

	sessionSent := false
	contextSent := false
	var streamMu sync.Mutex
	timelineState := &adkTimelineStreamState{}
	response, err := s.adkRuntime.ChatStream(c.Request.Context(), payload, func(delta jfadk.ChatDelta) error {
		streamMu.Lock()
		defer streamMu.Unlock()
		if delta.Run != nil {
			timelineState.observeRun(delta.Run)
			if err := writer.WriteEvent(adkChatStreamEvent{Type: "run", Run: delta.Run}); err != nil {
				return err
			}
			if timeline := timelineState.toolGroupSnapshot(); timeline != nil {
				if err := writer.WriteEvent(adkChatStreamEvent{Type: "timeline", Timeline: timeline}); err != nil {
					return err
				}
			}
			return nil
		}
		if delta.Context != nil {
			contextSent = true
			if err := writer.WriteEvent(adkChatStreamEvent{Type: "context", Context: delta.Context}); err != nil {
				return err
			}
		}
		if !sessionSent {
			session, sessionErr := s.previewADKSession(c.Request.Context(), payload)
			if sessionErr == nil {
				if err := writer.WriteEvent(adkChatStreamEvent{Type: "session", Session: &session}); err != nil {
					return err
				}
				sessionSent = true
				timelineState.observeSession(session)
				if !contextSent && strings.TrimSpace(session.ID) != "" {
					if snapshot, snapshotErr := s.adkRuntime.SessionContext(c.Request.Context(), session.ID); snapshotErr == nil {
						if err := writer.WriteEvent(adkChatStreamEvent{Type: "context", Context: &snapshot}); err != nil {
							return err
						}
						contextSent = true
					}
				}
			}
		}
		if delta.Reply == "" && delta.ReasoningContent == "" {
			return nil
		}
		if reasoningTimeline := timelineState.appendReasoning(delta.Run, delta.ReasoningContent); reasoningTimeline != nil {
			if err := writer.WriteEvent(adkChatStreamEvent{Type: "timeline", Timeline: reasoningTimeline}); err != nil {
				return err
			}
		}
		if messageTimeline := timelineState.appendMessage(delta.Run, delta.Reply); messageTimeline != nil {
			if err := writer.WriteEvent(adkChatStreamEvent{Type: "timeline", Timeline: messageTimeline}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = writer.WriteEvent(adkChatStreamEvent{Type: "error", Message: err.Error()})
		return
	}
	if !sessionSent {
		_ = writer.WriteEvent(adkChatStreamEvent{Type: "session", Session: &response.Session})
		timelineState.observeSession(response.Session)
	}
	if !contextSent && response.Context != nil {
		_ = writer.WriteEvent(adkChatStreamEvent{Type: "context", Context: response.Context})
	}
	trimmedRun := response.Run
	for i := range trimmedRun.ToolCalls {
		if trimmedRun.ToolCalls[i].Output != nil {
			trimmedRun.ToolCalls[i].Output = nil
		}
	}
	_ = writer.WriteEvent(adkChatStreamEvent{Type: "final", Response: &jfadk.ChatResponse{
		Reply:            response.Reply,
		ReasoningContent: response.ReasoningContent,
		Session:          response.Session,
		Run:              trimmedRun,
		PendingApprovals: response.PendingApprovals,
		Timeline:         response.Timeline,
		Context:          response.Context,
	}})
}

func decodeADKChatRequest(body io.Reader) (jfadk.ChatRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return jfadk.ChatRequest{}, err
	}
	var payload jfadk.ChatRequest
	_ = json.Unmarshal(raw["agentId"], &payload.AgentID)
	_ = json.Unmarshal(raw["sessionId"], &payload.SessionID)
	if err := decodeADKMessageField(raw, &payload.Message); err != nil {
		return jfadk.ChatRequest{}, err
	}
	return payload, nil
}

func decodeADKMessageField(raw map[string]json.RawMessage, target *string) error {
	for _, key := range []string{"message", "prompt", "text"} {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(value, target); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (s *Server) previewADKSession(ctx context.Context, payload jfadk.ChatRequest) (jfadk.Session, error) {
	agent, err := s.adkRuntime.Store().DefaultAgent(ctx)
	if strings.TrimSpace(payload.AgentID) != "" {
		var ok bool
		agent, ok, err = s.adkRuntime.Store().Agent(ctx, payload.AgentID)
		if err != nil {
			return jfadk.Session{}, err
		}
		if !ok {
			return jfadk.Session{}, io.EOF
		}
	}
	if strings.TrimSpace(payload.SessionID) != "" {
		session, ok, err := s.adkRuntime.Store().Session(ctx, payload.SessionID)
		if err != nil {
			return jfadk.Session{}, err
		}
		if ok {
			return session, nil
		}
	}
	title := strings.TrimSpace(payload.Message)
	if len([]rune(title)) > 28 {
		title = string([]rune(title)[:28])
	}
	return jfadk.Session{
		ID:      "",
		AgentID: agent.ID,
		Title:   title,
	}, nil
}

func (s *adkTimelineStreamState) observeSession(session jfadk.Session) {
	if strings.TrimSpace(session.ID) != "" {
		s.sessionID = session.ID
	}
}

func (s *adkTimelineStreamState) observeRun(run *jfadk.Run) {
	if run == nil {
		return
	}
	if strings.TrimSpace(run.SessionID) != "" {
		s.sessionID = run.SessionID
	}
	if strings.TrimSpace(run.ID) != "" {
		s.runID = run.ID
	}
	if len(run.ToolCalls) == 0 {
		return
	}
	s.reasoning = nil
	s.message = nil
	if s.toolGroup == nil {
		s.toolIndex++
		s.toolGroup = &jfadk.TimelineEntry{
			ID:        fmt.Sprintf("stream-tool-group:%s:%d", defaultTimelineRunID(s.runID), s.toolIndex),
			SessionID: s.sessionID,
			RunID:     defaultTimelineRunID(s.runID),
			Kind:      jfadk.TimelineKindToolGroup,
			CreatedAt: firstTimelineToolTime(run.ToolCalls, streamTimelineNow()),
			Sequence:  s.nextTimelineSequence(),
			Status:    jfadk.TimelineStatusStreaming,
		}
	}
	s.toolGroup.SessionID = defaultTimelineSessionID(s.sessionID)
	s.toolGroup.RunID = defaultTimelineRunID(s.runID)
	s.toolGroup.CreatedAt = firstTimelineToolTime(run.ToolCalls, s.toolGroup.CreatedAt)
	s.toolGroup.ToolCalls = append([]jfadk.ToolCall(nil), run.ToolCalls...)
	s.toolGroup.Status = jfadk.TimelineStatusStreaming
}

func (s *adkTimelineStreamState) appendReasoning(run *jfadk.Run, delta string) *jfadk.TimelineEntry {
	if delta == "" {
		return nil
	}
	s.observeRun(run)
	s.toolGroup = nil
	if s.reasoning == nil {
		s.reasoningIndex++
		s.reasoning = &jfadk.TimelineEntry{
			ID:        fmt.Sprintf("stream-reasoning:%s:%d", defaultTimelineRunID(s.runID), s.reasoningIndex),
			SessionID: defaultTimelineSessionID(s.sessionID),
			RunID:     defaultTimelineRunID(s.runID),
			Kind:      jfadk.TimelineKindAssistantReasoning,
			CreatedAt: streamTimelineNow(),
			Sequence:  s.nextTimelineSequence(),
			Status:    jfadk.TimelineStatusStreaming,
		}
	}
	s.reasoning.Text += delta
	return cloneTimelineEntry(s.reasoning)
}

func (s *adkTimelineStreamState) appendMessage(run *jfadk.Run, delta string) *jfadk.TimelineEntry {
	if delta == "" {
		return nil
	}
	s.observeRun(run)
	s.toolGroup = nil
	if s.message == nil {
		s.messageIndex++
		s.message = &jfadk.TimelineEntry{
			ID:        fmt.Sprintf("stream-message:%s:%d", defaultTimelineRunID(s.runID), s.messageIndex),
			SessionID: defaultTimelineSessionID(s.sessionID),
			RunID:     defaultTimelineRunID(s.runID),
			Kind:      jfadk.TimelineKindAssistantMessage,
			CreatedAt: streamTimelineNow(),
			Sequence:  s.nextTimelineSequence(),
			Status:    jfadk.TimelineStatusStreaming,
		}
	}
	s.message.Text += delta
	return cloneTimelineEntry(s.message)
}

func (s *adkTimelineStreamState) toolGroupSnapshot() *jfadk.TimelineEntry {
	if s.toolGroup == nil {
		return nil
	}
	return cloneTimelineEntry(s.toolGroup)
}

func (s *adkTimelineStreamState) nextTimelineSequence() int {
	s.nextSequence++
	return s.nextSequence
}

func cloneTimelineEntry(entry *jfadk.TimelineEntry) *jfadk.TimelineEntry {
	if entry == nil {
		return nil
	}
	cloned := *entry
	cloned.ToolCalls = append([]jfadk.ToolCall(nil), entry.ToolCalls...)
	cloned.Approvals = append([]jfadk.Approval(nil), entry.Approvals...)
	return &cloned
}

func defaultTimelineSessionID(sessionID string) string {
	return strings.TrimSpace(sessionID)
}

func defaultTimelineRunID(runID string) string {
	if trimmed := strings.TrimSpace(runID); trimmed != "" {
		return trimmed
	}
	return "stream"
}

func firstTimelineToolTime(toolCalls []jfadk.ToolCall, fallback string) string {
	best := strings.TrimSpace(fallback)
	for _, toolCall := range toolCalls {
		candidate := strings.TrimSpace(toolCall.CreatedAt)
		if candidate == "" {
			candidate = strings.TrimSpace(toolCall.UpdatedAt)
		}
		if candidate == "" {
			continue
		}
		if best == "" || candidate < best {
			best = candidate
		}
	}
	if best == "" {
		return streamTimelineNow()
	}
	return best
}

func streamTimelineNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
