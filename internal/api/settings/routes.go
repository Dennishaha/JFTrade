package settings

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/settings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// RegisterRoutes 注册所有 /api/v1/settings 路由。
func RegisterRoutes(api *gin.RouterGroup, svc *srv.Service) {
	settings := api.Group("/settings")

	// UI Appearance
	settings.GET("/ui", handleUIAppearance(svc))
	settings.PUT("/ui", handleSaveUIAppearance(svc))

	// Onboarding
	settings.GET("/onboarding", handleOnboardingState(svc))
	settings.PUT("/onboarding", handleSaveOnboarding(svc))

	// Execution
	settings.GET("/execution", handleExecutionSettings(svc))
	settings.PUT("/execution", handleSaveExecutionSettings(svc))

	// Security
	settings.GET("/security", handleSecuritySettings(svc))
	settings.PUT("/security", handleSaveSecuritySettings(svc))

	// ADK
	settings.GET("/adk", handleADKRuntimeSettings(svc))
	settings.PUT("/adk", handleSaveADKRuntimeSettings(svc))

	// Pine Worker
	settings.GET("/pine-worker", handlePineWorkerSettings(svc))
	settings.PUT("/pine-worker", handleSavePineWorkerSettings(svc))

	settings.GET("/data-migration/databases", handleDataMigrationDatabases(svc, false))
	settings.POST("/data-migration/databases/rebuild", handleDataMigrationRebuild(svc))
	settings.GET("/data-management/databases", handleDataMigrationDatabases(svc, true))
	settings.POST("/data-management/cleanup/preview", handleDataCleanupPreview(svc))
	settings.POST("/data-management/cleanup/execute", handleDataCleanupExecute(svc))
	settings.POST("/data-management/databases/:databaseId/compact", handleDatabaseCompact(svc))
	settings.POST("/data-management/databases/rebuild", handleDataMigrationRebuild(svc))

	// Exchange Calendars
	settings.GET("/exchange-calendars", handleExchangeCalendarSettings(svc))
	settings.PUT("/exchange-calendars", handleSaveExchangeCalendarSettings(svc))

	// Broker
	settings.GET("/brokers", handleBrokerSettings(svc))
	settings.PUT("/brokers/:brokerId/integration", handleSaveBrokerIntegration(svc))

	// Managed Accounts
	settings.POST("/broker-accounts", handleCreateManagedBrokerAccount(svc))
	settings.PUT("/broker-accounts/:accountRecordId", handleUpdateManagedBrokerAccount(svc))
	settings.DELETE("/broker-accounts/:accountRecordId", handleDeleteManagedBrokerAccount(svc))
}

func handleDataMigrationDatabases(svc *srv.Service, allowIncrementalQuery bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		request := srv.DataManagementOverviewRequest{}
		if allowIncrementalQuery {
			request.SummaryOnly = strings.EqualFold(c.Query("summaryOnly"), "true")
			request.DatabaseID = strings.TrimSpace(c.Query("databaseId"))
		}
		result, err := svc.DataManagementStatus(c.Request.Context(), request)
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

func handleDataMigrationRebuild(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input srv.DatabaseRebuildRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid database rebuild payload")
			return
		}
		result, err := svc.RebuildDatabase(c.Request.Context(), input)
		if err != nil {
			httpserver.WriteError(c, 400, "DATABASE_REBUILD_REJECTED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDataCleanupPreview(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input srv.DataCleanupPreviewRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid cleanup preview payload")
			return
		}
		result, err := svc.PreviewDataCleanup(c.Request.Context(), input)
		if err != nil {
			httpserver.WriteError(c, 400, "DATABASE_CLEANUP_PREVIEW_REJECTED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDataCleanupExecute(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input srv.DataCleanupExecuteRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid cleanup payload")
			return
		}
		result, err := svc.ExecuteDataCleanup(c.Request.Context(), input)
		if err != nil {
			writeDataManagementError(c, err, "DATABASE_CLEANUP_FAILED")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDatabaseCompact(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input srv.DatabaseCompactRequest
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid database compact payload")
			return
		}
		result, err := svc.CompactDatabase(c.Request.Context(), c.Param("databaseId"), input)
		if err != nil {
			writeDataManagementError(c, err, "DATABASE_COMPACT_FAILED")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func writeDataManagementError(c *gin.Context, err error, fallbackCode string) {
	switch {
	case errors.Is(err, srv.ErrDatabaseMaintenanceConflict):
		httpserver.WriteError(c, 409, "DATABASE_MAINTENANCE_CONFLICT", err.Error())
	case errors.Is(err, srv.ErrCleanupPreviewNotFound):
		httpserver.WriteError(c, 404, "CLEANUP_PREVIEW_NOT_FOUND", err.Error())
	case errors.Is(err, srv.ErrCleanupPreviewStale):
		httpserver.WriteError(c, 409, "CLEANUP_PREVIEW_STALE", err.Error())
	default:
		httpserver.WriteError(c, 400, fallbackCode, err.Error())
	}
}

// ── UI Appearance ──

// handleUIAppearance godoc
// @Summary 读取 UI 颜色配置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/ui [get]
func handleUIAppearance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, map[string]any{"appearance": svc.GetAppearance()})
	}
}

// handleSaveUIAppearance godoc
// @Summary 保存 UI 颜色配置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body UIAppearanceSettingsWriteRequest true "UI 配置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/ui [put]
func handleSaveUIAppearance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Appearance jfsettings.UIAppearanceSettings `json:"appearance"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid appearance payload")
			return
		}
		result, err := svc.SaveAppearance(payload.Appearance)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{"appearance": result})
	}
}

// ── Onboarding ──

// handleOnboardingState godoc
// @Summary 读取新手引导状态
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/onboarding [get]
func handleOnboardingState(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.OnboardingState(c.Request.Context()))
	}
}

// handleSaveOnboarding godoc
// @Summary 保存新手引导状态
// @Tags settings
// @Accept json
// @Produce json
// @Param request body OnboardingWriteRequest true "引导状态"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/onboarding [put]
func handleSaveOnboarding(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			Completed    bool   `json:"completed"`
			Dismissed    bool   `json:"dismissed"`
			LastBrokerID string `json:"lastBrokerId"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid onboarding payload")
			return
		}

		existing := svc.GetOnboarding()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		next := existing
		next.LastBrokerID = payload.LastBrokerID
		if strings.TrimSpace(next.LastBrokerID) == "" {
			next.LastBrokerID = existing.LastBrokerID
		}
		if payload.Completed || payload.Dismissed {
			next.Completed = true
			if payload.Dismissed {
				next.DismissedAt = now
			}
			if next.CompletedAt == "" {
				next.CompletedAt = now
			}
		} else {
			next.Completed = false
			next.CompletedAt = ""
			next.DismissedAt = ""
		}

		_, err := svc.SaveOnboarding(next)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, svc.OnboardingState(c.Request.Context()))
	}
}

// ── Execution ──

// handleExecutionSettings godoc
// @Summary 读取执行设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/execution [get]
func handleExecutionSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetExecutionSettings())
	}
}

// handleSaveExecutionSettings godoc
// @Summary 保存执行设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.ExecutionSettings true "执行设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/execution [put]
func handleSaveExecutionSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.ExecutionSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid execution payload")
			return
		}
		result, err := svc.SaveExecutionSettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Security ──

// handleSecuritySettings godoc
// @Summary 读取安全设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/security [get]
func handleSecuritySettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetSecuritySettings())
	}
}

// handleSaveSecuritySettings godoc
// @Summary 保存安全设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.SecuritySettings true "安全设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/security [put]
func handleSaveSecuritySettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.SecuritySettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid security payload")
			return
		}
		result, err := svc.SaveSecuritySettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── ADK ──

// handleADKRuntimeSettings godoc
// @Summary 读取 ADK 运行时设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/adk [get]
func handleADKRuntimeSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetADKRuntimeSettings())
	}
}

// handleSaveADKRuntimeSettings godoc
// @Summary 保存 ADK 运行时设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.ADKRuntimeSettings true "ADK 运行时设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/adk [put]
func handleSaveADKRuntimeSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.ADKRuntimeSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid adk payload")
			return
		}
		result, err := svc.SaveADKRuntimeSettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Pine Worker ──

// handlePineWorkerSettings godoc
// @Summary 读取 PineTS worker 设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/pine-worker [get]
func handlePineWorkerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.GetPineWorkerSettings())
	}
}

// handleSavePineWorkerSettings godoc
// @Summary 保存 PineTS worker 设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body jfsettings.PineWorkerSettings true "PineTS worker 设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/pine-worker [put]
func handleSavePineWorkerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.PineWorkerSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid pine worker payload")
			return
		}
		result, err := svc.SavePineWorkerSettings(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Exchange Calendars ──

// handleExchangeCalendarSettings godoc
// @Summary 读取交易日历设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/exchange-calendars [get]
func handleExchangeCalendarSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, map[string]any{"exchangeCalendars": svc.GetExchangeCalendarSettings()})
	}
}

// handleSaveExchangeCalendarSettings godoc
// @Summary 保存交易日历设置
// @Tags settings
// @Accept json
// @Produce json
// @Param request body ExchangeCalendarSettingsWriteRequest true "交易日历设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/exchange-calendars [put]
func handleSaveExchangeCalendarSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			ExchangeCalendars jfsettings.ExchangeCalendarSettings `json:"exchangeCalendars"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid exchange calendar payload")
			return
		}
		result, err := svc.SaveExchangeCalendarSettings(payload.ExchangeCalendars)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{"exchangeCalendars": result})
	}
}

// ── Broker ──

// handleBrokerSettings godoc
// @Summary 读取 broker 设置
// @Tags settings
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/settings/brokers [get]
func handleBrokerSettings(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.BrokerSettings())
	}
}

// handleSaveBrokerIntegration godoc
// @Summary 保存 broker 集成
// @Tags settings
// @Accept json
// @Produce json
// @Param brokerId path string true "Broker 标识"
// @Param request body BrokerIntegrationSaveRequest true "Broker 集成配置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/brokers/{brokerId}/integration [put]
func handleSaveBrokerIntegration(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			BrokerID string `uri:"brokerId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid broker id")
			return
		}
		var input jfsettings.BrokerIntegration
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid integration payload")
			return
		}
		input.BrokerID = uri.BrokerID
		result, err := svc.SaveIntegration(input)
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ── Managed Accounts ──

// handleCreateManagedBrokerAccount godoc
// @Summary 创建托管账户
// @Tags settings
// @Accept json
// @Produce json
// @Param request body ManagedBrokerAccountWriteRequest true "托管账户"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/settings/broker-accounts [post]
func handleCreateManagedBrokerAccount(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jfsettings.ManagedBrokerAccount
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account payload")
			return
		}
		result, err := svc.CreateManagedAccount(input)
		if err != nil {
			if errors.Is(err, srv.ErrBadRequest) {
				httpserver.WriteError(c, 400, "BAD_REQUEST", err.Error())
				return
			}
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleUpdateManagedBrokerAccount godoc
// @Summary 更新托管账户
// @Tags settings
// @Accept json
// @Produce json
// @Param accountRecordId path string true "托管账户记录 ID"
// @Param request body ManagedBrokerAccountWriteRequest true "托管账户"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/settings/broker-accounts/{accountRecordId} [put]
func handleUpdateManagedBrokerAccount(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			AccountRecordID string `uri:"accountRecordId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account id")
			return
		}
		var input jfsettings.ManagedBrokerAccount
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account payload")
			return
		}
		result, err := svc.UpdateManagedAccount(uri.AccountRecordID, input)
		if errors.Is(err, os.ErrNotExist) {
			httpserver.WriteError(c, 404, "NOT_FOUND", "managed broker account not found")
			return
		}
		if err != nil {
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleDeleteManagedBrokerAccount godoc
// @Summary 删除托管账户
// @Tags settings
// @Produce json
// @Param accountRecordId path string true "托管账户记录 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/settings/broker-accounts/{accountRecordId} [delete]
func handleDeleteManagedBrokerAccount(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			AccountRecordID string `uri:"accountRecordId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, 400, "BAD_REQUEST", "invalid account id")
			return
		}
		if err := svc.DeleteManagedAccount(uri.AccountRecordID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				httpserver.WriteError(c, 404, "NOT_FOUND", "managed broker account not found")
				return
			}
			httpserver.WriteError(c, 500, "SETTINGS_SAVE_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, map[string]any{"deleted": true, "id": uri.AccountRecordID})
	}
}
