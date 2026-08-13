package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jmoiron/sqlx"
)

var ErrChatRequestConflict = model.ErrChatRequestConflict

type ChatRequestConflictError = model.ChatRequestConflictError

// NormalizeChatRequestIdentity validates and canonicalizes the idempotency
// identity of a chat request.
func NormalizeChatRequestIdentity(req model.ChatRequest) (model.ChatRequest, string, error) {
	return model.NormalizeChatRequestIdentity(req)
}

// EnsureChatRequestIdentity assigns a fresh UUID when the caller omitted one.
func EnsureChatRequestIdentity(req model.ChatRequest) (model.ChatRequest, string, error) {
	return model.EnsureChatRequestIdentity(req)
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
