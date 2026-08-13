package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrChatRequestConflict = errors.New("chat request idempotency conflict")

// ChatRequestConflictError reports a reused clientRequestId with a different
// canonical request payload.
type ChatRequestConflictError struct {
	ClientRequestID string
}

func (e *ChatRequestConflictError) Error() string {
	return fmt.Sprintf("clientRequestId %s was already used with a different chat request", e.ClientRequestID)
}

func (e *ChatRequestConflictError) Unwrap() error {
	return ErrChatRequestConflict
}

type canonicalChatRequest struct {
	AgentID                 string `json:"agentId"`
	SessionID               string `json:"sessionId"`
	Message                 string `json:"message"`
	ProviderID              string `json:"providerId"`
	Model                   string `json:"model"`
	ReasoningEffortOverride string `json:"reasoningEffortOverride"`
	WorkModeOverride        string `json:"workModeOverride"`
	PermissionModeOverride  string `json:"permissionModeOverride"`
	Objective               string `json:"objective"`
	LoopMaxIterations       int    `json:"loopMaxIterations"`
}

// NormalizeChatRequestIdentity validates and canonicalizes a chat request's
// idempotency identity without depending on runtime persistence.
func NormalizeChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(req.ClientRequestID))
	if err != nil {
		return ChatRequest{}, "", fmt.Errorf("clientRequestId must be a valid UUID")
	}
	req.ClientRequestID = parsed.String()
	canonical := canonicalChatRequest{
		AgentID:                 strings.TrimSpace(req.AgentID),
		SessionID:               strings.TrimSpace(req.SessionID),
		Message:                 strings.TrimSpace(req.Message),
		ProviderID:              strings.TrimSpace(req.ProviderID),
		Model:                   strings.TrimSpace(req.Model),
		ReasoningEffortOverride: string(NormalizeOptionalReasoningEffort(req.ReasoningEffortOverride)),
		WorkModeOverride:        canonicalWorkMode(req.WorkModeOverride),
		PermissionModeOverride:  canonicalPermissionMode(req.PermissionModeOverride),
		Objective:               strings.TrimSpace(req.Objective),
		LoopMaxIterations:       NormalizeLoopMaxIterations(0),
	}
	if canonical.Objective == "" {
		canonical.Objective = canonical.Message
	}
	if req.RunOptions != nil {
		canonical.LoopMaxIterations = NormalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return ChatRequest{}, "", fmt.Errorf("encode chat request fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return req, hex.EncodeToString(digest[:]), nil
}

// EnsureChatRequestIdentity assigns a fresh UUID when the caller omitted one.
func EnsureChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	if strings.TrimSpace(req.ClientRequestID) == "" {
		req.ClientRequestID = uuid.NewString()
	}
	return NormalizeChatRequestIdentity(req)
}

func canonicalWorkMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if ValidWorkMode(normalized) {
		return NormalizeWorkMode(normalized)
	}
	return "invalid:" + normalized
}

func canonicalPermissionMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || ValidPermissionMode(normalized) {
		return NormalizePermissionMode(normalized)
	}
	return "invalid:" + normalized
}
