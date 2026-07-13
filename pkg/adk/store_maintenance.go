package adk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrCleanupCandidatesChanged = errors.New("cleanup candidates changed")

type DeletedConfigIDs struct {
	Agents    []string
	Workflows []string
	Triggers  []string
}

func (s *Store) PurgeDeletedConfigs(ctx context.Context, ids DeletedConfigIDs) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("adk database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	expected := len(ids.Agents) + len(ids.Workflows) + len(ids.Triggers)
	deleted := 0
	workflowSet := make(map[string]struct{}, len(ids.Workflows))
	for _, id := range ids.Workflows {
		trimmedID := strings.TrimSpace(id)
		var markedDeleted bool
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' FROM `+tableWorkflows+` WHERE id = ?`, trimmedID).Scan(&markedDeleted); err != nil || !markedDeleted {
			return 0, ErrCleanupCandidatesChanged
		}
		workflowSet[trimmedID] = struct{}{}
	}
	for _, id := range ids.Agents {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+tableAgents+` WHERE id = ? AND COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> ''`, strings.TrimSpace(id))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += int(count)
	}
	for _, id := range ids.Triggers {
		var workflowID string
		var markedDeleted bool
		err := tx.QueryRowContext(ctx, `SELECT workflow_id, COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> '' FROM `+tableWorkflowTriggers+` WHERE id = ?`, strings.TrimSpace(id)).Scan(&workflowID, &markedDeleted)
		if err != nil {
			if err == sql.ErrNoRows {
				return 0, ErrCleanupCandidatesChanged
			}
			return 0, err
		}
		_, parentDeleted := workflowSet[workflowID]
		if !markedDeleted && !parentDeleted {
			return 0, ErrCleanupCandidatesChanged
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM `+tableWorkflowTriggers+` WHERE id = ?`, strings.TrimSpace(id))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += int(count)
	}
	for _, id := range ids.Workflows {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+tableWorkflows+` WHERE id = ? AND COALESCE(json_extract(payload_json, '$.deletedAt'), '') <> ''`, strings.TrimSpace(id))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += int(count)
	}
	if deleted != expected {
		return 0, ErrCleanupCandidatesChanged
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *Store) HasDatabaseActivity(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM `+tableRuns+` WHERE UPPER(status) IN ('PENDING', 'PENDING_APPROVAL', 'PENDING_INPUT', 'RUNNING', 'PAUSED')) +
		(SELECT COUNT(*) FROM `+tableOptimizations+` WHERE UPPER(COALESCE(json_extract(payload_json, '$.status'), '')) IN ('QUEUED', 'RUNNING'))`).Scan(&count)
	return count > 0, err
}

func (s *Store) CompactDatabase(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("adk database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}
