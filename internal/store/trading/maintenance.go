package trading

import (
	"context"
	"fmt"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

// BusyReason reports why destructive database maintenance must be rejected.
func (s *Store) BusyReason(context.Context) string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, order := range s.orders {
		if !trdsrv.IsTerminalOrderStatus(order.Status) {
			return "存在非终态执行订单"
		}
	}
	return ""
}

// MaintenanceBusyReason implements datamanagement.BusyChecker without making
// the storage package depend on the orchestration package.
func (s *Store) MaintenanceBusyReason(ctx context.Context) string {
	return s.BusyReason(ctx)
}

// Compact checkpoints the WAL and vacuums the execution database.
func (s *Store) Compact(ctx context.Context) error {
	if !s.Available() {
		return fmt.Errorf("execution database is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.databaseMu.Lock()
	defer s.databaseMu.Unlock()
	return s.persistence.db.Compact(ctx)
}

// CompactMaintenanceResource implements datamanagement.Compactor.
func (s *Store) CompactMaintenanceResource(ctx context.Context) error {
	return s.Compact(ctx)
}
