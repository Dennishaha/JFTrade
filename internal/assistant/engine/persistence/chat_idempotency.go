package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jmoiron/sqlx"
)

var ErrChatRequestConflict = errors.New("chat request idempotency conflict")

// ChatRequestConflictError reports that a clientRequestId was already used
// with a different chat request payload.
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

// NormalizeChatRequestIdentity validates and canonicalizes the idempotency
// identity of a chat request.
func NormalizeChatRequestIdentity(req model.ChatRequest) (model.ChatRequest, string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(req.ClientRequestID))
	if err != nil {
		return model.ChatRequest{}, "", fmt.Errorf("clientRequestId must be a valid UUID")
	}
	req.ClientRequestID = parsed.String()
	canonical := canonicalChatRequest{
		AgentID:                 strings.TrimSpace(req.AgentID),
		SessionID:               strings.TrimSpace(req.SessionID),
		Message:                 strings.TrimSpace(req.Message),
		ProviderID:              strings.TrimSpace(req.ProviderID),
		Model:                   strings.TrimSpace(req.Model),
		ReasoningEffortOverride: string(model.NormalizeOptionalReasoningEffort(req.ReasoningEffortOverride)),
		WorkModeOverride:        canonicalWorkMode(req.WorkModeOverride),
		PermissionModeOverride:  canonicalPermissionMode(req.PermissionModeOverride),
		Objective:               strings.TrimSpace(req.Objective),
		LoopMaxIterations:       model.NormalizeLoopMaxIterations(0),
	}
	if canonical.Objective == "" {
		canonical.Objective = canonical.Message
	}
	if req.RunOptions != nil {
		canonical.LoopMaxIterations = model.NormalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return model.ChatRequest{}, "", fmt.Errorf("encode chat request fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return req, hex.EncodeToString(digest[:]), nil
}

// EnsureChatRequestIdentity assigns a fresh UUID when the caller omitted one.
func EnsureChatRequestIdentity(req model.ChatRequest) (model.ChatRequest, string, error) {
	if strings.TrimSpace(req.ClientRequestID) == "" {
		req.ClientRequestID = uuid.NewString()
	}
	return NormalizeChatRequestIdentity(req)
}

func canonicalWorkMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if model.ValidWorkMode(normalized) {
		return model.NormalizeWorkMode(normalized)
	}
	return "invalid:" + normalized
}

func canonicalPermissionMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return model.NormalizePermissionMode(normalized)
	}
	if model.ValidPermissionMode(normalized) {
		return model.NormalizePermissionMode(normalized)
	}
	return "invalid:" + normalized
}

type runRequestQueryer interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

// ChatRunByClientRequestID looks up an existing run by its idempotency key.
func (s *StoreCore) ChatRunByClientRequestID(ctx context.Context, clientRequestID string) (model.Run, string, bool, error) {
	if s == nil || s.DB() == nil {
		return model.Run{}, "", false, fmt.Errorf("adk store is unavailable")
	}
	return s.chatRunByClientRequestID(ctx, s.DB(), strings.TrimSpace(clientRequestID))
}

func (s *StoreCore) chatRunByClientRequestID(ctx context.Context, queryer runRequestQueryer, clientRequestID string) (model.Run, string, bool, error) {
	if queryer == nil || clientRequestID == "" {
		return model.Run{}, "", false, nil
	}
	var payload string
	var fingerprint string
	err := queryer.QueryRowxContext(ctx, `SELECT payload_json, request_fingerprint FROM `+tableRuns+` WHERE client_request_id = ? LIMIT 1`, clientRequestID).Scan(&payload, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Run{}, "", false, nil
	}
	if err != nil {
		return model.Run{}, "", false, err
	}
	run, err := decodeRun([]byte(payload))
	if err != nil {
		return model.Run{}, "", false, err
	}
	return s.normalizeRun(run), fingerprint, true, nil
}

// ClaimChatRun atomically inserts the run row when the idempotency key is
// unused, or returns the existing run when the key was already claimed.
func (s *StoreCore) ClaimChatRun(ctx context.Context, run model.Run, clientRequestID string, fingerprint string) (model.Run, bool, error) {
	if s == nil || s.DB() == nil {
		return model.Run{}, false, fmt.Errorf("adk store is unavailable")
	}
	run, err := s.PrepareRunForSave(ctx, run)
	if err != nil {
		return model.Run{}, false, err
	}
	payload, err := encodeRun(run)
	if err != nil {
		return model.Run{}, false, err
	}
	tx, err := s.DB().BeginWrite(ctx, nil)
	if err != nil {
		return model.Run{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		run.ID, run.SessionID, run.AgentID, run.Status, clientRequestID, fingerprint, string(payload), run.CreatedAt, run.UpdatedAt)
	if err != nil {
		return model.Run{}, false, err
	}
	created, err := result.RowsAffected()
	if err != nil {
		return model.Run{}, false, err
	}
	if created == 1 {
		if err := tx.Commit(); err != nil {
			return model.Run{}, false, err
		}
		return run, true, nil
	}
	existing, existingFingerprint, ok, err := s.chatRunByClientRequestID(ctx, tx, clientRequestID)
	if err != nil {
		return model.Run{}, false, err
	}
	if !ok {
		return model.Run{}, false, fmt.Errorf("chat request claim conflicted without an existing request")
	}
	if existingFingerprint != fingerprint {
		return model.Run{}, false, &ChatRequestConflictError{ClientRequestID: clientRequestID}
	}
	return existing, false, nil
}
