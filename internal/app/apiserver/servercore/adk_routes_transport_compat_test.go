package servercore

import jfadk "github.com/jftrade/jftrade-main/pkg/adk"

// adkChatStreamEvent keeps legacy integration tests focused on the public SSE
// contract after the transport moved to internal/api/assistant.
type adkChatStreamEvent struct {
	Type     string                        `json:"type"`
	Timeline *jfadk.TimelineEntry          `json:"timeline,omitempty"`
	Response *jfadk.ChatResponse           `json:"response,omitempty"`
	Session  *jfadk.Session                `json:"session,omitempty"`
	Run      *jfadk.Run                    `json:"run,omitempty"`
	Context  *jfadk.SessionContextSnapshot `json:"context,omitempty"`
	Message  string                        `json:"message,omitempty"`
}
