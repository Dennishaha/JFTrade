package persistence

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) SaveSessionNotice(ctx context.Context, notice jfadkmodel.TimelineEntry) (jfadkmodel.TimelineEntry, error) {
	if s == nil {
		return jfadkmodel.TimelineEntry{}, os.ErrNotExist
	}
	notice.SessionID = strings.TrimSpace(notice.SessionID)
	if notice.SessionID == "" {
		return jfadkmodel.TimelineEntry{}, os.ErrNotExist
	}
	notice.RunID = strings.TrimSpace(notice.RunID)
	notice.Kind = strings.TrimSpace(jfadkmodel.DefaultString(notice.Kind, jfadkmodel.TimelineKindContextNotice))
	if notice.Kind == "" {
		notice.Kind = jfadkmodel.TimelineKindContextNotice
	}
	notice.Status = strings.TrimSpace(jfadkmodel.DefaultString(notice.Status, jfadkmodel.TimelineStatusFinal))
	notice.Text = strings.TrimSpace(notice.Text)
	now := jfadkmodel.NowString()
	if strings.TrimSpace(notice.ID) == "" {
		notice.ID = "notice-" + jfadkmodel.NormalizeID(notice.SessionID) + "-" + jfadkmodel.NormalizeID(now)
	}
	if strings.TrimSpace(notice.CreatedAt) == "" {
		notice.CreatedAt = now
	}
	notice.UpdatedAt = now
	notice.ToolCalls = nil
	notice.Approvals = nil
	payload, err := json.Marshal(notice)
	if err != nil {
		return jfadkmodel.TimelineEntry{}, err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO `+tableSessionNotices+` (id, session_id, run_id, kind, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, run_id = excluded.run_id, kind = excluded.kind, status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		notice.ID, notice.SessionID, notice.RunID, notice.Kind, notice.Status, string(payload), notice.CreatedAt, notice.UpdatedAt,
	)
	return s.normalizeTimelineEntry(notice), err
}

func (s *StoreCore) SessionNotices(ctx context.Context, sessionID string) ([]jfadkmodel.TimelineEntry, error) {
	sessionID = strings.TrimSpace(sessionID)
	if s == nil || sessionID == "" {
		return []jfadkmodel.TimelineEntry{}, nil
	}
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableSessionNotices+` WHERE session_id = ? ORDER BY created_at ASC, id ASC`, sessionID); err != nil {
		return nil, err
	}
	items := make([]jfadkmodel.TimelineEntry, 0, len(rows))
	for _, row := range rows {
		var notice jfadkmodel.TimelineEntry
		if err := json.Unmarshal([]byte(row.PayloadJSON), &notice); err != nil {
			return nil, err
		}
		items = append(items, s.normalizeTimelineEntry(notice))
	}
	return items, nil
}
