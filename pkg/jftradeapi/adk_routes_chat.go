package jftradeapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

type adkChatStreamEvent struct {
	Type           string                        `json:"type"`
	Delta          string                        `json:"delta,omitempty"`
	ReasoningDelta string                        `json:"reasoningDelta,omitempty"`
	ToolProgress   string                        `json:"toolProgress,omitempty"`
	Response       *jfadk.ChatResponse           `json:"response,omitempty"`
	Session        *jfadk.Session                `json:"session,omitempty"`
	Run            *jfadk.Run                    `json:"run,omitempty"`
	Context        *jfadk.SessionContextSnapshot `json:"context,omitempty"`
	Message        string                        `json:"message,omitempty"`
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
	response, err := s.adkRuntime.ChatStream(c.Request.Context(), payload, func(delta jfadk.ChatDelta) error {
		streamMu.Lock()
		defer streamMu.Unlock()
		if delta.Run != nil {
			return writer.WriteEvent(adkChatStreamEvent{Type: "run", Run: delta.Run})
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
		if delta.ToolProgress != "" {
			return writer.WriteEvent(adkChatStreamEvent{Type: "delta", ToolProgress: delta.ToolProgress})
		}
		if delta.Reply == "" && delta.ReasoningContent == "" {
			return nil
		}
		return writer.WriteEvent(adkChatStreamEvent{
			Type:           "delta",
			Delta:          delta.Reply,
			ReasoningDelta: delta.ReasoningContent,
		})
	})
	if err != nil {
		_ = writer.WriteEvent(adkChatStreamEvent{Type: "error", Message: err.Error()})
		return
	}
	if !sessionSent {
		_ = writer.WriteEvent(adkChatStreamEvent{Type: "session", Session: &response.Session})
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
