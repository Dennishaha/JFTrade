package datamigration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

// NewService adapts the database migration manager to the business service.
func NewService(manager *Manager) *dmsrv.Service {
	if manager == nil {
		return dmsrv.NewService(nil)
	}
	return dmsrv.NewService(NewBackend(manager))
}

// Backend exposes Manager operations through the datamanagement service port.
type Backend struct {
	manager *Manager
}

func NewBackend(manager *Manager) Backend {
	return Backend{manager: manager}
}

func (b Backend) Overview(ctx context.Context, request dmsrv.OverviewRequest) (any, error) {
	if b.manager == nil {
		return map[string]any{"databases": []any{}}, nil
	}
	return b.manager.Overview(ctx, OverviewRequest{
		SummaryOnly: request.SummaryOnly, DatabaseID: request.DatabaseID,
	})
}

func (b Backend) PreviewCleanup(ctx context.Context, request dmsrv.CleanupPreviewRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database cleanup preview is unavailable")
	}
	result, err := b.manager.PreviewCleanup(ctx, CleanupPreviewRequest{
		Kind: request.Kind, DatabaseID: request.DatabaseID,
		OlderThanDays: request.OlderThanDays, KeepLatest: request.KeepLatest,
	})
	return result, TranslateServiceError(err)
}

func (b Backend) ExecuteCleanup(ctx context.Context, request dmsrv.CleanupExecuteRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database cleanup is unavailable")
	}
	result, err := b.manager.ExecuteCleanup(ctx, CleanupExecuteRequest{
		PreviewID: request.PreviewID, Confirmation: request.Confirmation,
	})
	return result, TranslateServiceError(err)
}

func (b Backend) Compact(ctx context.Context, databaseID string, request dmsrv.CompactRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database compaction is unavailable")
	}
	result, err := b.manager.Compact(ctx, databaseID, CompactRequest{Confirmation: request.Confirmation})
	return result, TranslateServiceError(err)
}

func (b Backend) Backup(ctx context.Context, request dmsrv.BackupRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database backup is unavailable")
	}
	result, err := b.manager.Backup(ctx, request.DatabaseID, request.Confirmation)
	if err != nil {
		return nil, TranslateServiceError(err)
	}
	return dmsrv.BackupResult{
		DatabaseID: result.DatabaseID, BackupPath: result.BackupPath,
		SizeBytes: result.SizeBytes, CreatedAt: result.CreatedAt,
	}, nil
}

func (b Backend) Rebuild(ctx context.Context, request dmsrv.RebuildRequest) (any, error) {
	if b.manager == nil {
		return nil, fmt.Errorf("database rebuild is unavailable")
	}
	ids := append([]string{}, request.DatabaseIDs...)
	if strings.TrimSpace(request.DatabaseID) != "" {
		ids = append(ids, request.DatabaseID)
	}
	return b.manager.ScheduleRebuild(ctx, RebuildRequest{
		DatabaseIDs: ids, Mode: request.Mode, Confirmation: request.Confirmation,
	})
}

func TranslateServiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrMaintenanceConflict):
		return fmt.Errorf("%w: %w", dmsrv.ErrDatabaseMaintenanceConflict, err)
	case errors.Is(err, ErrPreviewNotFound):
		return fmt.Errorf("%w: %w", dmsrv.ErrCleanupPreviewNotFound, err)
	case errors.Is(err, ErrPreviewStale):
		return fmt.Errorf("%w: %w", dmsrv.ErrCleanupPreviewStale, err)
	case errors.Is(err, ErrBackupRateLimited):
		return fmt.Errorf("%w: %w", dmsrv.ErrBackupRateLimited, err)
	case errors.Is(err, ErrBackupQuotaExceeded):
		return fmt.Errorf("%w: %w", dmsrv.ErrBackupQuotaExceeded, err)
	default:
		return err
	}
}
