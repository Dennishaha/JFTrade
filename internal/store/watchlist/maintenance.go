package watchlist

import (
	"context"
	"fmt"
)

// CompactMaintenanceResource compacts the watchlist database without exposing
// its SQLite connection outside the owning store.
func (s *Store) CompactMaintenanceResource(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("watchlist database is unavailable")
	}
	return s.db.Compact(ctx)
}
