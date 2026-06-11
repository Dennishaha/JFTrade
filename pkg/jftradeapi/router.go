package jftradeapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) buildRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(s.corsMiddleware())
	router.Use(s.authMiddleware())

	router.GET("/swagger", s.handleSwaggerRoot)
	router.GET("/swagger/*any", s.handleSwaggerUI)

	api := router.Group("/api/v1")
	s.registerAuthRoutes(api)
	s.registerMarketRoutes(api)
	s.registerSettingsRoutes(api)
	s.registerSystemRoutes(api)
	s.registerADKRoutes(api)
	s.registerPluginRoutes(api)
	s.registerStrategyRoutes(api)
	s.registerBacktestRoutes(api)
	s.registerBrokerRoutes(api)
	s.registerPortfolioRoutes(api)
	s.registerExecutionRoutes(api)

	router.NoRoute(s.handleNoRoute)
	return router
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := requestOrigin(c.Request)
		if origin != "" && s.auth != nil && s.auth.originAllowed(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-CSRF-Token")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		if c.Request.Method == http.MethodOptions {
			if origin != "" && (s.auth == nil || !s.auth.originAllowed(origin)) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.requiresAuthentication(c.Request) && !s.authorizeRequest(c) {
			return
		}
		c.Next()
	}
}

func (s *Server) adkAvailabilityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.adkRuntime == nil || s.adkRuntime.Store() == nil {
			s.writeError(c, http.StatusServiceUnavailable, "ADK_UNAVAILABLE", "ADK runtime is unavailable")
			return
		}
		c.Next()
	}
}

func (s *Server) registerAuthRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	auth.POST("/login", s.auth.login)
	auth.POST("/logout", s.handleAuthLogout)
	auth.GET("/session", s.auth.status)
	auth.Any("/token", s.handleAuthTokenDeprecated)
}

func (s *Server) registerMarketRoutes(api *gin.RouterGroup) {
	api.GET("/ws/live", s.handleLiveWebSocket)

	market := api.Group("/market-data")
	market.GET("/instruments", s.handleMarketInstrumentSearch)
	market.GET("/subscriptions", s.handleMarketSubscriptions)
	market.POST("/subscriptions", s.handleAcquireMarketSubscription)
	market.DELETE("/subscriptions", s.handleClearMarketSubscriptions)
	market.POST("/subscriptions/release", s.handleReleaseMarketSubscription)
	market.POST("/subscriptions/heartbeat", s.handleHeartbeatMarketSubscription)
	market.GET("/securities/:market/:symbol", s.handleMarketSecurityDetails)
	market.GET("/snapshots/:market/:symbol", s.handleMarketSnapshot)
	market.GET("/candles/:market/:symbol", s.handleMarketCandles)
	market.GET("/depth/:market/:symbol", s.handleMarketDepth)
}

func (s *Server) registerSettingsRoutes(api *gin.RouterGroup) {
	settings := api.Group("/settings")
	settings.GET("/ui", s.handleUIAppearance)
	settings.PUT("/ui", s.handleSaveUIAppearance)
	settings.GET("/onboarding", s.handleOnboardingState)
	settings.PUT("/onboarding", s.handleSaveOnboarding)
	settings.GET("/execution", s.handleExecutionSettings)
	settings.PUT("/execution", s.handleSaveExecutionSettings)
	settings.GET("/security", s.handleSecuritySettings)
	settings.PUT("/security", s.handleSaveSecuritySettings)
	settings.GET("/adk", s.handleADKRuntimeSettings)
	settings.PUT("/adk", s.handleSaveADKRuntimeSettings)
	settings.GET("/brokers", s.handleBrokerSettings)
	settings.PUT("/brokers/:brokerId/integration", s.handleSaveBrokerIntegration)
	settings.POST("/broker-accounts", s.handleCreateManagedBrokerAccount)
	settings.PUT("/broker-accounts/:accountRecordId", s.handleUpdateManagedBrokerAccount)
	settings.DELETE("/broker-accounts/:accountRecordId", s.handleDeleteManagedBrokerAccount)
}

func (s *Server) registerSystemRoutes(api *gin.RouterGroup) {
	system := api.Group("/system")
	system.GET("/futu-opend", s.handleFutuOpenDHealth)
	system.POST("/futu-opend/manual-retry", s.handleFutuOpenDManualRetry)
	system.GET("/futu-opend/install-guide", s.handleFutuOpenDInstallGuide)
	system.GET("/status", s.handleSystemStatus)
	system.GET("/storage/overview", s.handleStorageOverview)
	system.GET("/real-trade-approvals", s.handleRealTradeApprovals)
	system.GET("/real-trade-hard-stops", s.handleRealTradeHardStops)
	system.GET("/real-trade-hard-stop-events", s.handleRealTradeHardStopEvents)
	system.GET("/real-trade-kill-switch", s.handleRealTradeKillSwitch)
	system.GET("/real-trade-kill-switch-events", s.handleRealTradeKillSwitchEvents)
	system.GET("/real-trade-risk-limits", s.handleRealTradeRiskLimits)
	system.GET("/real-trade-risk-events", s.handleRealTradeRiskEvents)
	system.GET("/worker/broker-order-updates", s.handleBrokerOrderUpdatesWorker)
}

func (s *Server) registerADKRoutes(api *gin.RouterGroup) {
	adk := api.Group("/adk", s.adkAvailabilityMiddleware())
	adk.GET("", s.handleADKSnapshot)
	adk.GET("/tools", s.handleADKTools)
	adk.GET("/agent-templates", s.handleADKAgentTemplates)
	adk.GET("/audit", s.handleADKAudit)
	adk.GET("/metrics", s.handleADKMetrics)
	adk.GET("/tasks", s.handleADKTasks)
	adk.POST("/tasks", s.handleADKSaveTask)
	adk.GET("/tasks/:taskId", s.handleADKTask)
	adk.PUT("/tasks/:taskId", s.handleADKSaveTask)
	adk.DELETE("/tasks/:taskId", s.handleADKDeleteTask)
	adk.GET("/memory", s.handleADKMemory)
	adk.POST("/memory", s.handleADKSaveMemory)
	adk.DELETE("/memory/:memoryId", s.handleADKDeleteMemory)
	adk.GET("/optimization-tasks", s.handleADKOptimizationTasks)
	adk.GET("/optimization-tasks/:taskId", s.handleADKOptimizationTask)
	adk.POST("/optimization-tasks/:taskId/cancel", s.handleADKOptimizationTaskCancel)
	adk.GET("/providers", s.handleADKProviders)
	adk.POST("/providers", s.handleADKSaveProvider)
	adk.PUT("/providers/:providerId", s.handleADKSaveProvider)
	adk.DELETE("/providers/:providerId", s.handleADKDeleteProvider)
	adk.POST("/providers/:providerId/test", s.handleADKTestProvider)
	adk.GET("/agents", s.handleADKAgents)
	adk.POST("/agents", s.handleADKSaveAgent)
	adk.PUT("/agents/:agentId", s.handleADKSaveAgent)
	adk.DELETE("/agents/:agentId", s.handleADKDeleteAgent)
	adk.GET("/sessions", s.handleADKSessions)
	adk.POST("/sessions", s.handleADKCreateSession)
	adk.GET("/sessions/:sessionId", s.handleADKSession)
	adk.GET("/sessions/:sessionId/context", s.handleADKSessionContext)
	adk.POST("/sessions/:sessionId/context/compact", s.handleADKCompactSessionContext)
	adk.PUT("/sessions/:sessionId", s.handleADKRenameSession)
	adk.DELETE("/sessions/:sessionId", s.handleADKDeleteSession)
	adk.POST("/chat", s.handleADKChat)
	adk.POST("/chat/stream", s.handleADKChatStream)
	adk.GET("/runs", s.handleADKRuns)
	adk.GET("/runs/:runId", s.handleADKRun)
	adk.POST("/runs/:runId/cancel", s.handleADKCancelRun)
	adk.GET("/approvals", s.handleADKApprovals)
	adk.POST("/approvals/:approvalId/approve", func(c *gin.Context) { s.handleADKApproval(c, true) })
	adk.POST("/approvals/:approvalId/deny", func(c *gin.Context) { s.handleADKApproval(c, false) })
	adk.GET("/skills", s.handleADKSkills)
	adk.POST("/skills", s.handleADKInstallSkill)
	adk.PUT("/skills/:skillId", s.handleADKSkillUpdateRemoved)
	adk.DELETE("/skills/:skillId", s.handleADKDeleteSkill)
}

func (s *Server) registerPluginRoutes(api *gin.RouterGroup) {
	api.GET("/plugins", s.handlePlugins)
	api.GET("/plugins/operations/:operationId", s.handlePluginOperation)
	api.POST("/plugins/:pluginId/install", s.handlePluginInstall)
	api.POST("/plugins/:pluginId/uninstall", s.handlePluginUninstall)
	api.GET("/plugins/:pluginId/uninstall-guidance", s.handlePluginUninstallGuidance)
}

func (s *Server) registerStrategyRoutes(api *gin.RouterGroup) {
	api.POST("/strategy-pine/analyze", s.handleAnalyzeStrategyPine)
	api.GET("/strategy-definitions", s.handleStrategyDefinitions)
	api.POST("/strategy-definitions", s.handleCreateStrategyDefinition)
	api.GET("/strategy-definitions/:definitionId", s.handleStrategyDefinition)
	api.PUT("/strategy-definitions/:definitionId", s.handleUpdateStrategyDefinition)
	api.DELETE("/strategy-definitions/:definitionId", s.handleDeleteStrategyDefinition)
	api.POST("/strategy-definitions/:definitionId/apply-linked-instances", s.handleApplyLinkedStrategyInstances)
	api.POST("/strategy-definitions/:definitionId/instantiate", s.handleInstantiateStrategyDefinition)

	api.GET("/strategies", s.handleStrategies)
	api.PUT("/strategies/:instanceId", s.handleUpdateStrategy)
	api.DELETE("/strategies/:instanceId", s.handleDeleteStrategy)
	api.POST("/strategies/:instanceId/start", s.handleStartStrategy)
	api.POST("/strategies/:instanceId/refresh-definition", s.handleRefreshStrategyDefinition)
	api.POST("/strategies/:instanceId/pause", s.handlePauseStrategy)
	api.POST("/strategies/:instanceId/stop", s.handleStopStrategy)
	api.GET("/strategies/:instanceId/logs", s.handleStrategyLogs)
	api.GET("/strategies/:instanceId/audit", s.handleStrategyAudit)
}

func (s *Server) registerBacktestRoutes(api *gin.RouterGroup) {
	api.GET("/backtests", s.handleBacktestList)
	api.POST("/backtests", s.handleBacktestStart)
	api.POST("/backtests/sync", s.handleBacktestSync)
	api.GET("/backtests/sync/:taskId", s.handleBacktestSyncProgress)
	api.DELETE("/backtests/sync/:taskId", s.handleBacktestSyncCancel)
	api.GET("/backtests/:runId/status", s.handleBacktestStatus)
	api.GET("/backtests/:runId", s.handleBacktestResult)
	api.DELETE("/backtests/:runId", s.handleBacktestDelete)
}

func (s *Server) registerBrokerRoutes(api *gin.RouterGroup) {
	api.GET("/brokers/:brokerId/:resource", s.handleBrokerRead)
	api.POST("/brokers/:brokerId/:resource", s.handleBrokerWrite)
	api.DELETE("/brokers/:brokerId/:resource", s.handleBrokerWrite)
}

func (s *Server) registerPortfolioRoutes(api *gin.RouterGroup) {
	api.GET("/portfolio/:brokerId/cash-balances", s.handlePortfolioCashBalances)
	api.GET("/portfolio/:brokerId/positions", s.handlePortfolioPositions)
	api.GET("/portfolio/:brokerId/cash-reconciliation", s.handlePortfolioCashReconciliation)
	api.GET("/portfolio/:brokerId/reconciliation", s.handlePortfolioReconciliation)
}

func (s *Server) registerExecutionRoutes(api *gin.RouterGroup) {
	api.GET("/execution/orders", s.handleExecutionOrders)
	api.POST("/execution/orders", s.handleExecutionPlaceOrder)
	api.POST("/execution/orders/preview", s.handleExecutionPlaceOrder)
	api.GET("/execution/orders/:internalOrderId/events", s.handleExecutionOrderEvents)
	api.POST("/execution/orders/:internalOrderId/cancel", s.handleExecutionCancelOrder)
}

func (s *Server) handleAuthLogout(c *gin.Context) {
	if !s.authorizeRequest(c) {
		return
	}
	s.auth.logout(c)
}

// handleAuthTokenDeprecated godoc
// @Summary 旧令牌入口（已退役）
// @Description 始终返回 410 Gone；CLI 请直接使用管理员密钥作为 Bearer token。
// @Tags auth
// @Produce json
// @Failure 410 {object} envelope
// @Router /api/v1/auth/token [get]
func (s *Server) handleAuthTokenDeprecated(c *gin.Context) {
	s.writeError(c, http.StatusGone, "AUTH_TOKEN_REMOVED", "use administrator login or a Bearer administrator key")
}

// handleMarketInstrumentSearch godoc
// @Summary 检索行情标的
// @Description 按关键字查询可用标的。当前实现返回空结果占位。
// @Tags market-data
// @Produce json
// @Param query query string false "关键字"
// @Success 200 {object} envelope
// @Router /api/v1/market-data/instruments [get]
func (s *Server) handleMarketInstrumentSearch(c *gin.Context) {
	var query instrumentSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instrument query")
		return
	}
	s.writeOK(c, map[string]any{"query": query.Query, "totalReturned": 0, "entries": []any{}})
}

func (s *Server) handleMarketSubscriptions(c *gin.Context) {
	s.writeOK(c, s.marketSubscriptionsResponse())
}

// handleUIAppearance godoc
// @Summary 读取 UI 颜色配置
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/ui [get]
func (s *Server) handleUIAppearance(c *gin.Context) {
	s.writeOK(c, map[string]any{"appearance": s.store.appearance()})
}

// handleOnboardingState godoc
// @Summary 读取新手引导状态
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/onboarding [get]
func (s *Server) handleOnboardingState(c *gin.Context) {
	s.writeOK(c, s.onboardingState(c.Request.Context()))
}

// handleExecutionSettings godoc
// @Summary 读取执行设置
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/execution [get]
func (s *Server) handleExecutionSettings(c *gin.Context) {
	s.writeOK(c, s.store.executionSettings())
}

// handleSecuritySettings godoc
// @Summary 读取安全设置
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/security [get]
func (s *Server) handleSecuritySettings(c *gin.Context) {
	s.writeOK(c, s.store.securitySettings())
}

// handleADKRuntimeSettings godoc
// @Summary 读取 ADK 运行时设置
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/adk [get]
func (s *Server) handleADKRuntimeSettings(c *gin.Context) {
	s.writeOK(c, s.store.adkSettings())
}

// handleBrokerSettings godoc
// @Summary 读取 broker 设置
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/brokers [get]
func (s *Server) handleBrokerSettings(c *gin.Context) {
	s.writeOK(c, s.brokerSettings())
}

// handleFutuOpenDHealth godoc
// @Summary OpenD 健康检查
// @Tags system
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/system/futu-opend [get]
func (s *Server) handleFutuOpenDHealth(c *gin.Context) {
	s.writeOK(c, s.futuOpenDHealth(c.Request.Context()))
}

// handleFutuOpenDManualRetry godoc
// @Summary 手动重置 OpenD 运行时
// @Tags system
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/system/futu-opend/manual-retry [post]
func (s *Server) handleFutuOpenDManualRetry(c *gin.Context) {
	s.resetFutuRuntime()
	s.writeOK(c, map[string]any{"accepted": true})
}

// handleFutuOpenDInstallGuide godoc
// @Summary 读取 OpenD 安装指南
// @Tags system
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/system/futu-opend/install-guide [get]
func (s *Server) handleFutuOpenDInstallGuide(c *gin.Context) {
	s.writeOK(c, s.futuOpenDInstallGuide())
}

// handleSystemStatus godoc
// @Summary 读取系统状态
// @Description 返回 API、broker 与实时流状态摘要。
// @Tags system
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/system/status [get]
func (s *Server) handleSystemStatus(c *gin.Context) {
	s.writeOK(c, s.systemStatus())
}

func (s *Server) handleStorageOverview(c *gin.Context) {
	s.writeOK(c, map[string]any{"pendingOutbox": []any{}, "recentJobs": []any{}, "recentAuditLogs": []any{}, "recentExecutionCommands": []any{}})
}

func (s *Server) handleRealTradeApprovals(c *gin.Context) {
	s.writeOK(c, s.realTradeApprovals())
}

func (s *Server) handleRealTradeHardStops(c *gin.Context) {
	s.writeOK(c, map[string]any{"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
}

func (s *Server) handleRealTradeHardStopEvents(c *gin.Context) {
	s.writeOK(c, map[string]any{"realTradingEnabled": false, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
}

func (s *Server) handleRealTradeKillSwitch(c *gin.Context) {
	s.writeOK(c, s.realTradeKillSwitch())
}

func (s *Server) handleRealTradeKillSwitchEvents(c *gin.Context) {
	s.writeOK(c, map[string]any{"realTradingEnabled": false, "killSwitchActive": false, "envConfiguredActive": false, "controlPlaneActive": false, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entries": []any{}})
}

func (s *Server) handleRealTradeRiskLimits(c *gin.Context) {
	s.writeOK(c, s.realTradeRiskState())
}

func (s *Server) handleRealTradeRiskEvents(c *gin.Context) {
	s.writeOK(c, s.realTradeRiskEvents())
}

func (s *Server) handleBrokerOrderUpdatesWorker(c *gin.Context) {
	s.writeOK(c, s.brokerOrderUpdates.snapshotResponse())
}

func (s *Server) handleADKSkillUpdateRemoved(c *gin.Context) {
	s.writeError(c, http.StatusGone, "ADK_SKILL_UPDATE_REMOVED", "skill enable/disable has been removed; bind skills directly on the agent")
}

func (s *Server) handlePlugins(c *gin.Context) {
	s.writeOK(c, s.strategyStore.pluginCatalog())
}

func (s *Server) handlePortfolioCashBalances(c *gin.Context) {
	s.writeOK(c, map[string]any{"balances": []any{}})
}

func (s *Server) handlePortfolioPositions(c *gin.Context) {
	s.writeOK(c, map[string]any{"positions": []any{}})
}

func (s *Server) handlePortfolioCashReconciliation(c *gin.Context) {
	s.writeOK(c, s.emptyConnectivityList("balances", []any{}))
}

func (s *Server) handlePortfolioReconciliation(c *gin.Context) {
	s.writeOK(c, s.emptyConnectivityList("positions", []any{}))
}

func (s *Server) handleNoRoute(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		s.notFound(c)
		return
	}
	if s.frontend != nil && s.frontend.serveRequest(c.Writer, c.Request) {
		return
	}
	s.notFound(c)
}

func (s *Server) notFound(c *gin.Context) {
	s.writeError(c, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("unknown endpoint %s", c.Request.URL.Path))
}

func hasInvalidPercentEscape(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			continue
		}
		if i+2 >= len(value) || !isHex(value[i+1]) || !isHex(value[i+2]) {
			return true
		}
		i += 2
	}
	return false
}

func isHex(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')
}
