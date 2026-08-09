package servercore

import (
	"context"
	"errors"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func configureDataManagement(s *Server) {
	if s == nil || s.dataMigration == nil {
		return
	}
	maintenance := s.newMaintenanceRegistry()
	s.dataMigration.SetMaintenanceHooks(datamigration.MaintenanceHooks{
		BusyReason: func(databaseID string) string {
			return maintenance.BusyReason(context.Background(), databaseID)
		},
		Purge: func(ctx context.Context, databaseID string, candidates []datamigration.CleanupCandidate) (int, error) {
			deleted, err := maintenance.Purge(ctx, databaseID, maintenanceCandidates(candidates))
			if errors.Is(err, dmsrv.ErrCleanupCandidatesChanged) {
				return 0, datamigration.ErrPreviewStale
			}
			return deleted, err
		},
		Compact: maintenance.Compact,
	})
}

func (s *serverApplication) dataMigrationPath(databaseID string) string {
	for _, status := range mustDatabaseStatuses(s.dataMigration) {
		if status.ID == databaseID {
			return status.Path
		}
	}
	return ""
}

func mustDatabaseStatuses(manager *datamigration.Manager) []datamigration.DatabaseStatus {
	if manager == nil {
		return nil
	}
	statuses, _ := manager.Statuses(context.Background())
	return statuses
}
