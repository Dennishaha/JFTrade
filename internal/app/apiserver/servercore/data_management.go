package servercore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func translateDataManagementError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, datamigration.ErrMaintenanceConflict):
		return fmt.Errorf("%w: %v", dmsrv.ErrDatabaseMaintenanceConflict, err)
	case errors.Is(err, datamigration.ErrPreviewNotFound):
		return fmt.Errorf("%w: %v", dmsrv.ErrCleanupPreviewNotFound, err)
	case errors.Is(err, datamigration.ErrPreviewStale):
		return fmt.Errorf("%w: %v", dmsrv.ErrCleanupPreviewStale, err)
	case errors.Is(err, datamigration.ErrBackupRateLimited):
		return fmt.Errorf("%w: %v", dmsrv.ErrBackupRateLimited, err)
	case errors.Is(err, datamigration.ErrBackupQuotaExceeded):
		return fmt.Errorf("%w: %v", dmsrv.ErrBackupQuotaExceeded, err)
	default:
		return err
	}
}

type dataManagementBackend struct {
	manager *datamigration.Manager
}

func (s *Server) newDataManagementService() *dmsrv.Service {
	if s == nil || s.dataMigration == nil {
		return dmsrv.NewService(nil)
	}
	return dmsrv.NewService(dataManagementBackend{manager: s.dataMigration})
}

func (b dataManagementBackend) Overview(ctx context.Context, request dmsrv.OverviewRequest) (any, error) {
	if b.manager == nil {
		return map[string]any{"databases": []any{}}, nil
	}
	return b.manager.Overview(ctx, datamigration.OverviewRequest{
		SummaryOnly: request.SummaryOnly,
		DatabaseID:  request.DatabaseID,
	})
}

func (b dataManagementBackend) PreviewCleanup(ctx context.Context, request dmsrv.CleanupPreviewRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database cleanup preview is unavailable")
	}
	result, err := b.manager.PreviewCleanup(ctx, datamigration.CleanupPreviewRequest{
		Kind:          request.Kind,
		DatabaseID:    request.DatabaseID,
		OlderThanDays: request.OlderThanDays,
		KeepLatest:    request.KeepLatest,
	})
	return result, translateDataManagementError(err)
}

func (b dataManagementBackend) ExecuteCleanup(ctx context.Context, request dmsrv.CleanupExecuteRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database cleanup is unavailable")
	}
	result, err := b.manager.ExecuteCleanup(ctx, datamigration.CleanupExecuteRequest{
		PreviewID:    request.PreviewID,
		Confirmation: request.Confirmation,
	})
	return result, translateDataManagementError(err)
}

func (b dataManagementBackend) Compact(ctx context.Context, databaseID string, request dmsrv.CompactRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database compaction is unavailable")
	}
	result, err := b.manager.Compact(ctx, databaseID, datamigration.CompactRequest{Confirmation: request.Confirmation})
	return result, translateDataManagementError(err)
}

func (b dataManagementBackend) Backup(ctx context.Context, request dmsrv.BackupRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database backup is unavailable")
	}
	result, err := b.manager.Backup(ctx, request.DatabaseID, request.Confirmation)
	if err != nil {
		return nil, translateDataManagementError(err)
	}
	return dmsrv.BackupResult{
		DatabaseID: result.DatabaseID,
		BackupPath: result.BackupPath,
		SizeBytes:  result.SizeBytes,
		CreatedAt:  result.CreatedAt,
	}, nil
}

func (b dataManagementBackend) Rebuild(ctx context.Context, request dmsrv.RebuildRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database rebuild is unavailable")
	}
	ids := append([]string{}, request.DatabaseIDs...)
	if strings.TrimSpace(request.DatabaseID) != "" {
		ids = append(ids, request.DatabaseID)
	}
	return b.manager.ScheduleRebuild(ctx, datamigration.RebuildRequest{
		DatabaseIDs:  ids,
		Mode:         request.Mode,
		Confirmation: request.Confirmation,
	})
}

func (s *Server) configureDataManagement() {
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
