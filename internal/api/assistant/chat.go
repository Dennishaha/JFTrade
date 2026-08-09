package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

type adkChatStreamEvent struct {
	Type     string                        `json:"type"`
	StreamID string                        `json:"streamId,omitempty"`
	Sequence int64                         `json:"sequence,omitempty"`
	RunID    string                        `json:"runId,omitempty"`
	Replay   bool                          `json:"replay,omitempty"`
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

func (h *Handler) handleADKChat(c *gin.Context) {
	payload, err := decodeADKChatRequest(c.Request.Body)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid chat payload")
		return
	}
	request, _, err := jfadk.NormalizeChatRequestIdentity(jfadk.ChatRequest(payload))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	result, err := h.service.Chat(c.Request.Context(), request)
	if err != nil {
		if errors.Is(err, jfadk.ErrChatRequestConflict) {
			h.writeError(c, http.StatusConflict, "ADK_CHAT_IDEMPOTENCY_CONFLICT", err.Error())
			return
		}
		h.writeError(c, http.StatusBadRequest, "ADK_CHAT_FAILED", err.Error())
		return
	}
	h.writeOK(c, jfadk.NormalizeChatResponse(result))
}

// handleADKChatStream godoc
// @Summary 启动 ADK 对话流
// @Tags adk
// @Accept json
// @Produce event-stream
// @x-error-produces ["application/json"]
// @Param request body ADKChatRequest true "ADK chat request"
// @Success 200 {string} string "Server-Sent Events stream"
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Failure 403 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/chat/stream [post]
func (h *Handler) handleADKChatStream(c *gin.Context) {
	c.Header("X-ADK-Stream-Idle-Timeout-Ms", strconv.Itoa(h.service.StreamIdleTimeoutMillis()))
	payload, err := decodeADKChatRequest(c.Request.Body)
	if err != nil {
		writer, ok := httpserver.PrepareSSEWriter(c.Writer)
		if !ok {
			h.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
			return
		}
		if err := writer.WriteRetryDirective(); err != nil {
			return
		}
		jftradeErr1 := writer.WriteEvent(adkChatStreamEvent{Type: "error", Message: "invalid chat payload: " + err.Error()})
		besteffort.LogError(jftradeErr1)
		return
	}
	request, fingerprint, err := jfadk.NormalizeChatRequestIdentity(jfadk.ChatRequest(payload))
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if h.isClosing() {
		writer, ok := httpserver.PrepareSSEWriter(c.Writer)
		if !ok {
			h.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
			return
		}
		if err := writer.WriteRetryDirective(); err != nil {
			return
		}
		besteffort.LogError(writer.WriteEvent(adkChatStreamEvent{Type: "error", Message: "assistant transport is shutting down"}))
		return
	}
	if err := h.service.CheckChatRequestConflict(context.WithoutCancel(c.Request.Context()), request.ClientRequestID, fingerprint); err != nil {
		if errors.Is(err, jfadk.ErrChatRequestConflict) {
			h.writeError(c, http.StatusConflict, "ADK_CHAT_IDEMPOTENCY_CONFLICT", err.Error())
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_CHAT_FAILED", err.Error())
		return
	}
	writer, ok := httpserver.PrepareSSEWriter(c.Writer)
	if !ok {
		h.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
		return
	}
	record, created, err := h.startOrReuseADKChatStream(request, fingerprint)
	if err != nil {
		if errors.Is(err, jfadk.ErrChatRequestConflict) {
			h.writeError(c, http.StatusConflict, "ADK_CHAT_IDEMPOTENCY_CONFLICT", err.Error())
			return
		}
		h.writeError(c, http.StatusInternalServerError, "ADK_CHAT_FAILED", err.Error())
		return
	}
	c.Header("X-ADK-Stream-ID", record.id)
	if err := writer.WriteRetryDirective(); err != nil {
		return
	}
	h.streamADKChatRecord(c, writer, record, 0, !created)
}

// handleADKChatStreamReconnect godoc
// @Summary 重连 ADK 对话流
// @Tags adk
// @Produce event-stream
// @x-error-produces ["application/json"]
// @Param streamId path string true "Stream ID"
// @Param after query int false "Last received event sequence"
// @Success 200 {string} string "Server-Sent Events stream"
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Failure 403 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/streams/{streamId} [get]
func (h *Handler) handleADKChatStreamReconnect(c *gin.Context) {
	var uri streamURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.StreamID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "streamId is invalid")
		return
	}
	after, err := parseADKStreamAfter(c)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	h.cleanupADKChatStreams(c.Request.Context())
	record, ok := h.streams.get(uri.StreamID)
	if !ok {
		h.writeError(c, http.StatusNotFound, "NOT_FOUND", "stream not found")
		return
	}
	writer, ok := httpserver.PrepareSSEWriter(c.Writer)
	if !ok {
		h.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
		return
	}
	c.Header("X-ADK-Stream-ID", record.id)
	if err := writer.WriteRetryDirective(); err != nil {
		return
	}
	h.streamADKChatRecord(c, writer, record, after, true)
}

// handleADKRunStreamReconnect godoc
// @Summary 按 Run 重连 ADK 对话流
// @Tags adk
// @Produce event-stream
// @x-error-produces ["application/json"]
// @Param runId path string true "Run ID"
// @Param after query int false "Last received event sequence"
// @Success 200 {string} string "Server-Sent Events stream"
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Failure 403 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId}/stream [get]
func (h *Handler) handleADKRunStreamReconnect(c *gin.Context) {
	var uri runURI
	if err := httpserver.BindURI(c, &uri); err != nil || strings.TrimSpace(uri.RunID) == "" {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "runId is invalid")
		return
	}
	after, err := parseADKStreamAfter(c)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	h.cleanupADKChatStreams(c.Request.Context())
	record, ok := h.streams.getByRunID(uri.RunID)
	if !ok {
		h.writeError(c, http.StatusNotFound, "NOT_FOUND", "stream not found")
		return
	}
	writer, ok := httpserver.PrepareSSEWriter(c.Writer)
	if !ok {
		h.writeError(c, http.StatusInternalServerError, "SSE_UNSUPPORTED", "streaming is unavailable")
		return
	}
	c.Header("X-ADK-Stream-ID", record.id)
	if err := writer.WriteRetryDirective(); err != nil {
		return
	}
	h.streamADKChatRecord(c, writer, record, after, true)
}

func decodeADKChatRequest(body io.Reader) (ADKChatRequest, error) {
	var payload ADKChatRequest
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return ADKChatRequest{}, err
	}
	return payload, nil
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

func (s *adkTimelineStreamState) observeTimeline(entry jfadk.TimelineEntry) {
	if strings.TrimSpace(entry.SessionID) != "" {
		s.sessionID = entry.SessionID
	}
	if strings.TrimSpace(entry.RunID) != "" {
		s.runID = entry.RunID
	}
	if entry.Sequence > s.nextSequence {
		s.nextSequence = entry.Sequence
	}
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
	return new(jfadk.NormalizeTimelineEntry(*entry))
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

func firstTimelineToolTime(toolCalls []jfadk.ToolCall, currentTime string) string {
	best := strings.TrimSpace(currentTime)
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
