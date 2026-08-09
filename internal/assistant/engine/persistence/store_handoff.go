package persistence

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jmoiron/sqlx"
)

func (s *StoreCore) HandoffSegments(ctx context.Context, sessionID string, activeOnly bool) ([]jfadkmodel.HandoffSegment, error) {
	clauses := []string{"session_id = ?"}
	args := []any{strings.TrimSpace(sessionID)}
	if activeOnly {
		clauses = append(clauses, "active = 1")
	}
	whereSQL := " WHERE " + strings.Join(clauses, " AND ")
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.DB().SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableHandoffSegments+whereSQL+` ORDER BY sequence_no ASC, created_at ASC`, args...); err != nil {
		return nil, err
	}
	items := make([]jfadkmodel.HandoffSegment, 0, len(rows))
	for _, row := range rows {
		var segment jfadkmodel.HandoffSegment
		if err := json.Unmarshal([]byte(row.PayloadJSON), &segment); err != nil {
			return nil, err
		}
		items = append(items, segment)
	}
	return items, nil
}

func (s *StoreCore) HandoffSegmentsForRevision(ctx context.Context, sessionID string, contextRevisionID string, activeOnly bool) ([]jfadkmodel.HandoffSegment, error) {
	sessionID = strings.TrimSpace(sessionID)
	contextRevisionID = strings.TrimSpace(contextRevisionID)
	if contextRevisionID == "" {
		return []jfadkmodel.HandoffSegment{}, nil
	}
	clauses := []string{"session_id = ?"}
	args := []any{sessionID}
	clauses = append(clauses, "json_extract(payload_json, '$.contextRevisionId') = ?")
	args = append(args, contextRevisionID)
	if activeOnly {
		clauses = append(clauses, "active = 1")
	}
	whereSQL := " WHERE " + strings.Join(clauses, " AND ")
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.DB().SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableHandoffSegments+whereSQL+` ORDER BY sequence_no ASC, created_at ASC`, args...); err != nil {
		return nil, err
	}
	items := make([]jfadkmodel.HandoffSegment, 0, len(rows))
	for _, row := range rows {
		var segment jfadkmodel.HandoffSegment
		if err := json.Unmarshal([]byte(row.PayloadJSON), &segment); err != nil {
			return nil, err
		}
		items = append(items, segment)
	}
	return items, nil
}

func (s *StoreCore) SaveHandoffSegment(ctx context.Context, segment jfadkmodel.HandoffSegment) (jfadkmodel.HandoffSegment, error) {
	segment.SessionID = strings.TrimSpace(segment.SessionID)
	if segment.SessionID == "" {
		return jfadkmodel.HandoffSegment{}, os.ErrNotExist
	}
	if strings.TrimSpace(segment.ContextRevisionID) == "" {
		segment.ContextRevisionID = jfadkmodel.NewContextRevisionID()
	}
	now := jfadkmodel.NowString()
	if strings.TrimSpace(segment.ID) == "" {
		segment.ID = "handoff-" + jfadkmodel.NormalizeID(segment.SessionID) + "-" + jfadkmodel.NormalizeID(now)
	}
	if segment.CreatedAt == "" {
		segment.CreatedAt = now
	}
	segment.UpdatedAt = now
	payload, err := json.Marshal(segment)
	if err != nil {
		return jfadkmodel.HandoffSegment{}, err
	}
	_, err = s.DB().ExecContext(
		ctx,
		`INSERT INTO `+tableHandoffSegments+` (id, session_id, active, sequence_no, created_at, updated_at, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, active = excluded.active, sequence_no = excluded.sequence_no, updated_at = excluded.updated_at, payload_json = excluded.payload_json`,
		segment.ID, segment.SessionID, boolToInt(segment.Active), segment.Sequence, segment.CreatedAt, segment.UpdatedAt, string(payload),
	)
	return segment, err
}

func (s *StoreCore) ReplaceActiveHandoffSegments(ctx context.Context, sessionID string, next jfadkmodel.HandoffSegment, superseded []jfadkmodel.HandoffSegment) (jfadkmodel.HandoffSegment, error) {
	tx, err := s.DB().BeginWrite(ctx, nil)
	if err != nil {
		return jfadkmodel.HandoffSegment{}, err
	}
	defer func() {
		if tx != nil {
			jftradeErr1 := tx.Rollback()
			besteffort.LogError(jftradeErr1)
		}
	}()
	next, err = s.saveHandoffSegmentTx(ctx, tx, next)
	if err != nil {
		return jfadkmodel.HandoffSegment{}, err
	}
	for _, segment := range superseded {
		segment.Active = false
		segment.SupersededBy = next.ID
		if _, err := s.saveHandoffSegmentTx(ctx, tx, segment); err != nil {
			return jfadkmodel.HandoffSegment{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return jfadkmodel.HandoffSegment{}, err
	}
	tx = nil
	return next, nil
}

func (s *StoreCore) saveHandoffSegmentTx(ctx context.Context, tx sqlx.ExtContext, segment jfadkmodel.HandoffSegment) (jfadkmodel.HandoffSegment, error) {
	segment.SessionID = strings.TrimSpace(segment.SessionID)
	if segment.SessionID == "" {
		return jfadkmodel.HandoffSegment{}, os.ErrNotExist
	}
	if strings.TrimSpace(segment.ContextRevisionID) == "" {
		segment.ContextRevisionID = jfadkmodel.NewContextRevisionID()
	}
	now := jfadkmodel.NowString()
	if strings.TrimSpace(segment.ID) == "" {
		segment.ID = "handoff-" + uuid.NewString()
	}
	if segment.CreatedAt == "" {
		segment.CreatedAt = now
	}
	segment.UpdatedAt = now
	payload, err := json.Marshal(segment)
	if err != nil {
		return jfadkmodel.HandoffSegment{}, err
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
