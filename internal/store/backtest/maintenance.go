package backtest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

// ErrCleanupCandidatesChanged means the approved terminal-run set no longer matches storage.
var ErrCleanupCandidatesChanged = errors.New("backtest cleanup candidates changed")

// MaintenanceBusyReason reports active runs that make destructive maintenance unsafe.
func (s *Store) MaintenanceBusyReason(context.Context) string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, run := range s.runs {
		if run != nil && (run.Status == "queued" || run.Status == "running") {
			return "存在正在排队或运行的回测"
		}
	}
	return ""
}

// PurgeTerminalRuns removes the exact approved set of terminal runs atomically.
func (s *Store) PurgeTerminalRuns(ctx context.Context, ids []string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("backtest run database is unavailable")
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
		result, err := tx.ExecContext(ctx,
			`DELETE FROM `+runTable+` WHERE id = ? AND status IN ('completed', 'failed', 'cancelled')`,
			strings.TrimSpace(id),
		)
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
	for _, id := range ids {
		delete(s.runs, strings.TrimSpace(id))
	}
	return deleted, nil
}

// Compact checkpoints the WAL and vacuums the run database under the store lock.
func (s *Store) Compact(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("backtest run database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Compact(ctx)
}

// PurgeMaintenanceCandidates implements datamanagement.CandidatePurger while
// keeping the in-memory run index synchronized with the database.
func (s *Store) PurgeMaintenanceCandidates(
	ctx context.Context,
	candidates []dmsrv.CleanupCandidate,
) (int, error) {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	deleted, err := s.PurgeTerminalRuns(ctx, ids)
	if errors.Is(err, ErrCleanupCandidatesChanged) {
		return 0, fmt.Errorf("%w: %v", dmsrv.ErrCleanupCandidatesChanged, err)
	}
	return deleted, err
}

// CompactMaintenanceResource implements datamanagement.Compactor.
func (s *Store) CompactMaintenanceResource(ctx context.Context) error {
	return s.Compact(ctx)
}
