package strategy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

var ErrCleanupCandidatesChanged = errors.New("strategy cleanup candidates changed")

// PurgeDeletedDefinitions permanently removes the exact soft-deleted set
// approved by the data-maintenance preview.
func (s *Store) PurgeDeletedDefinitions(ctx context.Context, ids []string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("strategy database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	deleted := 0
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+strategyDesignDefinitionTable+` WHERE id = ? AND deleted_at IS NOT NULL AND TRIM(deleted_at) <> ''`, strings.TrimSpace(id))
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += int(count)
	}
	if deleted != len(ids) {
		return 0, ErrCleanupCandidatesChanged
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// Compact checkpoints the WAL and vacuums the strategy database while holding
// the store lock so CRUD cannot race with maintenance.
func (s *Store) Compact(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("strategy database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Compact(ctx)
}

// PurgeMaintenanceCandidates implements datamanagement.CandidatePurger while
// keeping candidate-to-row mapping inside the strategy storage boundary.
func (s *Store) PurgeMaintenanceCandidates(
	ctx context.Context,
	candidates []dmsrv.CleanupCandidate,
) (int, error) {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	deleted, err := s.PurgeDeletedDefinitions(ctx, ids)
	if errors.Is(err, ErrCleanupCandidatesChanged) {
		return 0, fmt.Errorf("%w: %v", dmsrv.ErrCleanupCandidatesChanged, err)
	}
	return deleted, err
}

// CompactMaintenanceResource implements datamanagement.Compactor.
func (s *Store) CompactMaintenanceResource(ctx context.Context) error {
	return s.Compact(ctx)
}
