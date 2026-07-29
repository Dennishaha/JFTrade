package adk

import (
	"context"
	"encoding/json"
	"os"
	"strings"
)

func (s *Store) SaveSessionNotice(ctx context.Context, notice TimelineEntry) (TimelineEntry, error) {
	if s == nil {
		return TimelineEntry{}, os.ErrNotExist
	}
	notice.SessionID = strings.TrimSpace(notice.SessionID)
	if notice.SessionID == "" {
		return TimelineEntry{}, os.ErrNotExist
	}
	notice.RunID = strings.TrimSpace(notice.RunID)
	notice.Kind = strings.TrimSpace(defaultString(notice.Kind, TimelineKindContextNotice))
	if notice.Kind == "" {
		notice.Kind = TimelineKindContextNotice
	}
	notice.Status = strings.TrimSpace(defaultString(notice.Status, TimelineStatusFinal))
	notice.Text = strings.TrimSpace(notice.Text)
	now := nowString()
	if strings.TrimSpace(notice.ID) == "" {
		notice.ID = "notice-" + normalizeID(notice.SessionID) + "-" + normalizeID(now)
	}
	if strings.TrimSpace(notice.CreatedAt) == "" {
		notice.CreatedAt = now
	}
	notice.UpdatedAt = now
	notice.ToolCalls = nil
	notice.Approvals = nil
	payload, err := json.Marshal(notice)
	if err != nil {
		return TimelineEntry{}, err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO `+tableSessionNotices+` (id, session_id, run_id, kind, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, run_id = excluded.run_id, kind = excluded.kind, status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		notice.ID, notice.SessionID, notice.RunID, notice.Kind, notice.Status, string(payload), notice.CreatedAt, notice.UpdatedAt,
	)
	return NormalizeTimelineEntry(notice), err
}

func (s *Store) SessionNotices(ctx context.Context, sessionID string) ([]TimelineEntry, error) {
	sessionID = strings.TrimSpace(sessionID)
	if s == nil || sessionID == "" {
		return []TimelineEntry{}, nil
	}
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableSessionNotices+` WHERE session_id = ? ORDER BY created_at ASC, id ASC`, sessionID); err != nil {
		return nil, err
	}
	items := make([]TimelineEntry, 0, len(rows))
	for _, row := range rows {
		var notice TimelineEntry
		if err := json.Unmarshal([]byte(row.PayloadJSON), &notice); err != nil {
			return nil, err
		}
		items = append(items, NormalizeTimelineEntry(notice))
	}
	return items, nil
}
