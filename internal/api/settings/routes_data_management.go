package settings

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func handleDataMigrationDatabases(svc *dmsrv.Service, allowIncrementalQuery bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		request := dmsrv.OverviewRequest{}
		if allowIncrementalQuery {
			request.SummaryOnly = strings.EqualFold(c.Query("summaryOnly"), "true")
			request.DatabaseID = strings.TrimSpace(c.Query("databaseId"))
		}
		result, err := svc.Overview(c.Request.Context(), request)
		if err != nil {
			if request.DatabaseID != "" {
				httpserver.WriteError(c, 400, "DATABASE_STATUS_REJECTED", err.Error())
				return
			}
			httpserver.WriteError(c, 500, "DATABASE_STATUS_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDataMigrationRebuild(svc *dmsrv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input dmsrv.RebuildRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid database rebuild payload")
			return
		}
		result, err := svc.Rebuild(c.Request.Context(), input)
		if err != nil {
			httpserver.WriteError(c, 400, "DATABASE_REBUILD_REJECTED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDataCleanupPreview(svc *dmsrv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input dmsrv.CleanupPreviewRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid cleanup preview payload")
			return
		}
		result, err := svc.PreviewCleanup(c.Request.Context(), input)
		if err != nil {
			httpserver.WriteError(c, 400, "DATABASE_CLEANUP_PREVIEW_REJECTED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDataCleanupExecute(svc *dmsrv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input dmsrv.CleanupExecuteRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid cleanup payload")
			return
		}
		result, err := svc.ExecuteCleanup(c.Request.Context(), input)
		if err != nil {
			writeDataManagementError(c, err, "DATABASE_CLEANUP_FAILED")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDatabaseCompact(svc *dmsrv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input dmsrv.CompactRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid database compact payload")
			return
		}
		result, err := svc.Compact(c.Request.Context(), c.Param("databaseId"), input)
		if err != nil {
			writeDataManagementError(c, err, "DATABASE_COMPACT_FAILED")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleDatabaseBackup godoc
// @Summary 创建本地数据库一致性备份
// @Tags settings
// @Accept json
// @Produce json
// @Param databaseId path string true "数据库 ID"
// @Param request body datamanagement.BackupRequest true "备份确认"
// @Success 200 {object} httpserver.Envelope{data=datamanagement.BackupResult}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Router /api/v1/settings/data-management/databases/{databaseId}/backup [post]
func handleDatabaseBackup(svc *dmsrv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input dmsrv.BackupRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid database backup payload")
			return
		}
		input.DatabaseID = c.Param("databaseId")
		result, err := svc.Backup(c.Request.Context(), input)
		if err != nil {
			writeDataManagementError(c, err, "DATABASE_BACKUP_FAILED")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func writeDataManagementError(c *gin.Context, err error, fallbackCode string) {
	switch {
	case errors.Is(err, dmsrv.ErrDatabaseMaintenanceConflict):
		httpserver.WriteError(c, 409, "DATABASE_MAINTENANCE_CONFLICT", err.Error())
	case errors.Is(err, dmsrv.ErrCleanupPreviewNotFound):
		httpserver.WriteError(c, 404, "CLEANUP_PREVIEW_NOT_FOUND", err.Error())
	case errors.Is(err, dmsrv.ErrCleanupPreviewStale):
		httpserver.WriteError(c, 409, "CLEANUP_PREVIEW_STALE", err.Error())
	case errors.Is(err, dmsrv.ErrBackupRateLimited):
		httpserver.WriteError(c, 429, "DATABASE_BACKUP_RATE_LIMITED", err.Error())
	case errors.Is(err, dmsrv.ErrBackupQuotaExceeded):
		httpserver.WriteError(c, 507, "DATABASE_BACKUP_QUOTA_EXCEEDED", err.Error())
	default:
		httpserver.WriteError(c, 400, fallbackCode, err.Error())
	}
}
