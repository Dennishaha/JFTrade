package settings

import (
	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	srv "github.com/jftrade/jftrade-main/internal/settings"
)

// RegisterRoutes 注册所有 /api/v1/settings 路由。
func RegisterRoutes(api *gin.RouterGroup, svc *srv.Service, dataManagementServices ...*dmsrv.Service) {
	settings := api.Group("/settings")
	dataManagementSvc := dmsrv.NewService(nil)
	if len(dataManagementServices) > 0 && dataManagementServices[0] != nil {
		dataManagementSvc = dataManagementServices[0]
	}

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

	// System Notifications
	settings.GET("/system-notifications", handleSystemNotificationSettings(svc))
	settings.PUT("/system-notifications", handleSaveSystemNotificationSettings(svc))

	// ADK
	settings.GET("/adk", handleADKRuntimeSettings(svc))
	settings.PUT("/adk", handleSaveADKRuntimeSettings(svc))
	settings.GET("/adk/mcp", handleMCPServerSettings(svc))
	settings.PUT("/adk/mcp", handleSaveMCPServerSettings(svc))
	settings.POST("/adk/mcp/token/reset", handleResetMCPServerToken(svc))

	// Pine Worker
	settings.GET("/pine-worker", handlePineWorkerSettings(svc))
	settings.PUT("/pine-worker", handleSavePineWorkerSettings(svc))

	settings.GET("/data-migration/databases", httpserver.Deprecated("/api/v1/settings/data-management/databases", handleDataMigrationDatabases(dataManagementSvc, false)))
	settings.POST("/data-migration/databases/rebuild", httpserver.Deprecated("/api/v1/settings/data-management/databases/rebuild", handleDataMigrationRebuild(dataManagementSvc)))
	settings.GET("/data-management/databases", handleDataMigrationDatabases(dataManagementSvc, true))
	settings.POST("/data-management/cleanup/preview", handleDataCleanupPreview(dataManagementSvc))
	settings.POST("/data-management/cleanup/execute", handleDataCleanupExecute(dataManagementSvc))
	settings.POST("/data-management/databases/:databaseId/compact", handleDatabaseCompact(dataManagementSvc))
	settings.POST("/data-management/databases/:databaseId/backup", handleDatabaseBackup(dataManagementSvc))
	settings.POST("/data-management/databases/rebuild", handleDataMigrationRebuild(dataManagementSvc))

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
