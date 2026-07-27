package servercore

import assistant "github.com/jftrade/jftrade-main/internal/assistant"

// adkChatStreamEvent keeps legacy integration tests focused on the public SSE
// contract after the transport moved to internal/api/assistant.
type adkChatStreamEvent struct {
	Type     string                            `json:"type"`
	StreamID string                            `json:"streamId,omitempty"`
	Sequence int64                             `json:"sequence,omitempty"`
	RunID    string                            `json:"runId,omitempty"`
	Replay   bool                              `json:"replay,omitempty"`
	Timeline *assistant.TimelineEntry          `json:"timeline,omitempty"`
	Response *assistant.ChatResponse           `json:"response,omitempty"`
	Session  *assistant.Session                `json:"session,omitempty"`
	Run      *assistant.Run                    `json:"run,omitempty"`
	Context  *assistant.SessionContextSnapshot `json:"context,omitempty"`
	Message  string                            `json:"message,omitempty"`
}
