package persistence

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

type auditEventPayloadRow struct {
	PayloadJSON string `db:"payload_json"`
}

func (s *StoreCore) AddAuditEvent(ctx context.Context, event jfadkmodel.AuditEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "audit-" + uuid.NewString()
	}
	if strings.TrimSpace(event.CreatedAt) == "" {
		event.CreatedAt = jfadkmodel.NowString()
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO `+tableAudit+` (id, kind, subject_id, payload_json, created_at) VALUES (?, ?, ?, ?, ?)`, event.ID, event.Kind, event.SubjectID, string(payload), event.CreatedAt)
	return err
}

func (s *StoreCore) ListAuditEvents(ctx context.Context) ([]jfadkmodel.AuditEvent, error) {
	return s.ListAuditEventsFiltered(ctx, "", "")
}

func (s *StoreCore) ListAuditEventsFiltered(ctx context.Context, kind string, subjectID string) ([]jfadkmodel.AuditEvent, error) {
	whereSQL, args := auditEventWhere(kind, subjectID)
	return s.selectAuditEvents(ctx, whereSQL, args, 0, 0)
}

func (s *StoreCore) ListAuditEventsPage(
	ctx context.Context,
	kind string,
	subjectID string,
	limit int,
	offset int,
) ([]jfadkmodel.AuditEvent, int, error) {
	whereSQL, args := auditEventWhere(kind, subjectID)
	var total int
	if err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+tableAudit+whereSQL, args...); err != nil {
		return nil, 0, err
	}
	offset = min(max(offset, 0), total)
	events, err := s.selectAuditEvents(ctx, whereSQL, args, limit, offset)
	return events, total, err
}

func auditEventWhere(kind string, subjectID string) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, kind)
	}
	if subjectID != "" {
		clauses = append(clauses, "subject_id = ?")
		args = append(args, subjectID)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *StoreCore) selectAuditEvents(
	ctx context.Context,
	whereSQL string,
	args []any,
	limit int,
	offset int,
) ([]jfadkmodel.AuditEvent, error) {
	query := `SELECT payload_json FROM ` + tableAudit + whereSQL + ` ORDER BY created_at DESC, id ASC`
	queryArgs := append([]any(nil), args...)
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		queryArgs = append(queryArgs, limit, offset)
	}
	rows := []auditEventPayloadRow{}
	if err := s.db.SelectContext(ctx, &rows, query, queryArgs...); err != nil {
		return nil, err
	}
	events := make([]jfadkmodel.AuditEvent, 0, len(rows))
	for _, row := range rows {
		var event jfadkmodel.AuditEvent
		if err := json.Unmarshal([]byte(row.PayloadJSON), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
