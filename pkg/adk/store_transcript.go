package adk

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

const transcriptKindMessage = "message"

func (s *Store) AddTranscriptEntry(
	ctx context.Context,
	sessionID string,
	runID string,
	role string,
	kind string,
	content string,
	reasoningContent string,
) (TranscriptEntry, error) {
	now := nowString()
	entry := TranscriptEntry{
		ID:               "tx-" + uuid.NewString(),
		SessionID:        strings.TrimSpace(sessionID),
		RunID:            strings.TrimSpace(runID),
		Role:             strings.TrimSpace(role),
		Kind:             defaultString(strings.TrimSpace(kind), transcriptKindMessage),
		Content:          content,
		ReasoningContent: reasoningContent,
		CreatedAt:        now,
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return TranscriptEntry{}, err
	}
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO `+tableTranscriptEntries+` (id, session_id, run_id, role, kind, payload_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.SessionID, entry.RunID, entry.Role, entry.Kind, string(payload), entry.CreatedAt,
	); err != nil {
		return TranscriptEntry{}, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE `+tableSessions+` SET updated_at = ? WHERE id = ?`, now, sessionID)
	return entry, nil
}

func (s *Store) AddMessage(ctx context.Context, sessionID string, role string, content string, reasoningContent string) (Message, error) {
	return s.AddTranscriptEntry(ctx, sessionID, "", role, transcriptKindMessage, content, reasoningContent)
}

func (s *Store) TranscriptEntries(ctx context.Context, sessionID string) ([]TranscriptEntry, error) {
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.SelectContext(
		ctx,
		&rows,
		`SELECT payload_json FROM `+tableTranscriptEntries+` WHERE session_id = ? ORDER BY created_at ASC`,
		strings.TrimSpace(sessionID),
	); err != nil {
		return nil, err
	}
	entries := make([]TranscriptEntry, 0, len(rows))
	for _, row := range rows {
		var entry TranscriptEntry
		if err := json.Unmarshal([]byte(row.PayloadJSON), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Store) Messages(ctx context.Context, sessionID string) ([]Message, error) {
	return s.TranscriptEntries(ctx, sessionID)
}
