package adk

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
	"github.com/jmoiron/sqlx"
)

var ErrChatRequestConflict = errors.New("chat request idempotency conflict")

type ChatRequestConflictError struct {
	ClientRequestID string
}

func (e *ChatRequestConflictError) Error() string {
	return fmt.Sprintf("clientRequestId %s was already used with a different chat request", e.ClientRequestID)
}

func (e *ChatRequestConflictError) Unwrap() error {
	return ErrChatRequestConflict
}

type reusedChatRequestError struct {
	Run Run
}

func (e *reusedChatRequestError) Error() string {
	return "chat request already belongs to run " + e.Run.ID
}

type canonicalChatRequest struct {
	AgentID                string `json:"agentId"`
	SessionID              string `json:"sessionId"`
	Message                string `json:"message"`
	ProviderID             string `json:"providerId"`
	Model                  string `json:"model"`
	WorkModeOverride       string `json:"workModeOverride"`
	PermissionModeOverride string `json:"permissionModeOverride"`
	Objective              string `json:"objective"`
	LoopMaxIterations      int    `json:"loopMaxIterations"`
}

func normalizeChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(req.ClientRequestID))
	if err != nil {
		return ChatRequest{}, "", fmt.Errorf("clientRequestId must be a valid UUID")
	}
	req.ClientRequestID = parsed.String()
	canonical := canonicalChatRequest{
		AgentID:                strings.TrimSpace(req.AgentID),
		SessionID:              strings.TrimSpace(req.SessionID),
		Message:                strings.TrimSpace(req.Message),
		ProviderID:             strings.TrimSpace(req.ProviderID),
		Model:                  strings.TrimSpace(req.Model),
		WorkModeOverride:       canonicalWorkMode(req.WorkModeOverride),
		PermissionModeOverride: canonicalPermissionMode(req.PermissionModeOverride),
		Objective:              strings.TrimSpace(req.Objective),
		LoopMaxIterations:      normalizeLoopMaxIterations(0),
	}
	if canonical.Objective == "" {
		canonical.Objective = canonical.Message
	}
	if req.RunOptions != nil {
		canonical.LoopMaxIterations = normalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return ChatRequest{}, "", fmt.Errorf("encode chat request fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return req, hex.EncodeToString(digest[:]), nil
}

func canonicalWorkMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if validWorkMode(normalized) {
		return normalizeWorkMode(normalized)
	}
	return "invalid:" + normalized
}

func canonicalPermissionMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return normalizePermissionMode(normalized)
	}
	if validPermissionMode(normalized) {
		return normalizePermissionMode(normalized)
	}
	return "invalid:" + normalized
}

func ensureChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	if strings.TrimSpace(req.ClientRequestID) == "" {
		req.ClientRequestID = uuid.NewString()
	}
	return normalizeChatRequestIdentity(req)
}

func NormalizeChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	return normalizeChatRequestIdentity(req)
}

type runRequestQueryer interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

func (s *Store) ChatRunByClientRequestID(ctx context.Context, clientRequestID string) (Run, string, bool, error) {
	if s == nil || s.db == nil {
		return Run{}, "", false, fmt.Errorf("adk store is unavailable")
	}
	return chatRunByClientRequestID(ctx, s.db, strings.TrimSpace(clientRequestID))
}

func chatRunByClientRequestID(ctx context.Context, queryer runRequestQueryer, clientRequestID string) (Run, string, bool, error) {
	if queryer == nil || clientRequestID == "" {
		return Run{}, "", false, nil
	}
	var payload string
	var fingerprint string
	err := queryer.QueryRowxContext(ctx, `SELECT payload_json, request_fingerprint FROM `+tableRuns+` WHERE client_request_id = ? LIMIT 1`, clientRequestID).Scan(&payload, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, "", false, nil
	}
	if err != nil {
		return Run{}, "", false, err
	}
	var run Run
	if err := json.Unmarshal([]byte(payload), &run); err != nil {
		return Run{}, "", false, err
	}
	return NormalizeRun(run), fingerprint, true, nil
}

func (s *Store) ClaimChatRun(ctx context.Context, run Run, clientRequestID string, fingerprint string) (Run, bool, error) {
	run, err := s.prepareRunForSave(ctx, run)
	if err != nil {
		return Run{}, false, err
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return Run{}, false, err
	}
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return Run{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		run.ID, run.SessionID, run.AgentID, run.Status, clientRequestID, fingerprint, string(payload), run.CreatedAt, run.UpdatedAt)
	if err != nil {
		return Run{}, false, err
	}
	created, err := result.RowsAffected()
	if err != nil {
		return Run{}, false, err
	}
	if created == 1 {
		if err := tx.Commit(); err != nil {
			return Run{}, false, err
		}
		return run, true, nil
	}
	existing, existingFingerprint, ok, err := chatRunByClientRequestID(ctx, tx, clientRequestID)
	if err != nil {
		return Run{}, false, err
	}
	if !ok {
		return Run{}, false, fmt.Errorf("chat request claim conflicted without an existing request")
	}
	if existingFingerprint != fingerprint {
		return Run{}, false, &ChatRequestConflictError{ClientRequestID: clientRequestID}
	}
	return existing, false, nil
}
