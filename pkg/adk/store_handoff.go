package adk

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func (s *Store) HandoffSegments(ctx context.Context, sessionID string, activeOnly bool) ([]HandoffSegment, error) {
	clauses := []string{"session_id = ?"}
	args := []any{strings.TrimSpace(sessionID)}
	if activeOnly {
		clauses = append(clauses, "active = 1")
	}
	whereSQL := " WHERE " + strings.Join(clauses, " AND ")
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableHandoffSegments+whereSQL+` ORDER BY sequence_no ASC, created_at ASC`, args...); err != nil {
		return nil, err
	}
	items := make([]HandoffSegment, 0, len(rows))
	for _, row := range rows {
		var segment HandoffSegment
		if err := json.Unmarshal([]byte(row.PayloadJSON), &segment); err != nil {
			return nil, err
		}
		items = append(items, segment)
	}
	return items, nil
}

func (s *Store) SaveHandoffSegment(ctx context.Context, segment HandoffSegment) (HandoffSegment, error) {
	segment.SessionID = strings.TrimSpace(segment.SessionID)
	if segment.SessionID == "" {
		return HandoffSegment{}, os.ErrNotExist
	}
	now := nowString()
	if strings.TrimSpace(segment.ID) == "" {
		segment.ID = "handoff-" + normalizeID(segment.SessionID) + "-" + normalizeID(now)
	}
	if segment.CreatedAt == "" {
		segment.CreatedAt = now
	}
	segment.UpdatedAt = now
	payload, err := json.Marshal(segment)
	if err != nil {
		return HandoffSegment{}, err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO `+tableHandoffSegments+` (id, session_id, active, sequence_no, created_at, updated_at, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, active = excluded.active, sequence_no = excluded.sequence_no, updated_at = excluded.updated_at, payload_json = excluded.payload_json`,
		segment.ID, segment.SessionID, boolToInt(segment.Active), segment.Sequence, segment.CreatedAt, segment.UpdatedAt, string(payload),
	)
	return segment, err
}

func (s *Store) ReplaceActiveHandoffSegments(ctx context.Context, sessionID string, next HandoffSegment, superseded []HandoffSegment) (HandoffSegment, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return HandoffSegment{}, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	next, err = s.saveHandoffSegmentTx(ctx, tx, next)
	if err != nil {
		return HandoffSegment{}, err
	}
	for _, segment := range superseded {
		segment.Active = false
		segment.SupersededBy = next.ID
		if _, err := s.saveHandoffSegmentTx(ctx, tx, segment); err != nil {
			return HandoffSegment{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return HandoffSegment{}, err
	}
	tx = nil
	return next, nil
}

func (s *Store) saveHandoffSegmentTx(ctx context.Context, tx sqlx.ExtContext, segment HandoffSegment) (HandoffSegment, error) {
	segment.SessionID = strings.TrimSpace(segment.SessionID)
	if segment.SessionID == "" {
		return HandoffSegment{}, os.ErrNotExist
	}
	now := nowString()
	if strings.TrimSpace(segment.ID) == "" {
		segment.ID = "handoff-" + uuid.NewString()
	}
	if segment.CreatedAt == "" {
		segment.CreatedAt = now
	}
	segment.UpdatedAt = now
	payload, err := json.Marshal(segment)
	if err != nil {
		return HandoffSegment{}, err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO `+tableHandoffSegments+` (id, session_id, active, sequence_no, created_at, updated_at, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, active = excluded.active, sequence_no = excluded.sequence_no, updated_at = excluded.updated_at, payload_json = excluded.payload_json`,
		segment.ID, segment.SessionID, boolToInt(segment.Active), segment.Sequence, segment.CreatedAt, segment.UpdatedAt, string(payload),
	)
	return segment, err
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
