package research

import (
	"context"
	"fmt"
)

// CompactMaintenanceResource compacts the research database without exposing
// its SQLite connection outside the owning store.
func (s *Store) CompactMaintenanceResource(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("research database is unavailable")
	}
	return s.db.Compact(ctx)
}
