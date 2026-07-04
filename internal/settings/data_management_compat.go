package settings

import "github.com/jftrade/jftrade-main/internal/datamanagement"

var (
	ErrDatabaseMaintenanceConflict = datamanagement.ErrDatabaseMaintenanceConflict
	ErrCleanupPreviewNotFound      = datamanagement.ErrCleanupPreviewNotFound
	ErrCleanupPreviewStale         = datamanagement.ErrCleanupPreviewStale
)

type DataCleanupPreviewRequest = datamanagement.CleanupPreviewRequest
type DataCleanupExecuteRequest = datamanagement.CleanupExecuteRequest
type DatabaseCompactRequest = datamanagement.CompactRequest
type DatabaseRebuildRequest = datamanagement.RebuildRequest
type DataManagementOverviewRequest = datamanagement.OverviewRequest
