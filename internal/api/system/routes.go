package system

import (
	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	sys "github.com/jftrade/jftrade-main/internal/system"
)

// RegisterRoutes 注册所有 /api/v1/system 路由。
// svc 是系统业务逻辑服务，由调用方（Server）装配并注入。
func RegisterRoutes(api *gin.RouterGroup, svc *sys.Service) {
	system := api.Group("/system")
	system.GET("/futu-opend", handleFutuOpenDHealth(svc))
	system.POST("/futu-opend/manual-retry", handleFutuOpenDManualRetry(svc))
	system.GET("/futu-opend/install-guide", handleFutuOpenDInstallGuide(svc))
	system.GET("/status", handleSystemStatus(svc))
	system.GET("/storage/overview", handleStorageOverview(svc))
	system.GET("/real-trade-approvals", handleRealTradeApprovals(svc))
	system.GET("/real-trade-hard-stops", handleRealTradeHardStops(svc))
	system.GET("/real-trade-hard-stop-events", handleRealTradeHardStopEvents(svc))
	system.GET("/real-trade-kill-switch", handleRealTradeKillSwitch(svc))
	system.GET("/real-trade-kill-switch-events", handleRealTradeKillSwitchEvents(svc))
	system.GET("/real-trade-risk-limits", handleRealTradeRiskLimits(svc))
	system.GET("/real-trade-risk-events", handleRealTradeRiskEvents(svc))
	system.GET("/worker/broker-order-updates", handleBrokerOrderUpdatesWorker(svc))
}

// handleFutuOpenDHealth godoc
// @Summary OpenD 健康检查
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/futu-opend [get]
func handleFutuOpenDHealth(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.FutuOpenDHealth(c.Request.Context()))
	}
}

// handleFutuOpenDManualRetry godoc
// @Summary 手动重置 OpenD 运行时
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/futu-opend/manual-retry [post]
func handleFutuOpenDManualRetry(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc.ResetFutuRuntime()
		httpserver.WriteOK(c, map[string]any{"accepted": true})
	}
}

// handleFutuOpenDInstallGuide godoc
// @Summary 读取 OpenD 安装指南
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/futu-opend/install-guide [get]
func handleFutuOpenDInstallGuide(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.FutuOpenDInstallGuide())
	}
}

// handleSystemStatus godoc
// @Summary 读取系统状态
// @Description 返回 API、broker 与实时流状态摘要。
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/status [get]
func handleSystemStatus(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.Status())
	}
}

func handleStorageOverview(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.StorageOverview())
	}
}

func handleRealTradeApprovals(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeApprovals())
	}
}

func handleRealTradeHardStops(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeHardStops())
	}
}

func handleRealTradeHardStopEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeHardStopEvents())
	}
}

func handleRealTradeKillSwitch(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeKillSwitch())
	}
}

func handleRealTradeKillSwitchEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeKillSwitchEvents())
	}
}

func handleRealTradeRiskLimits(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeRiskLimits())
	}
}

func handleRealTradeRiskEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeRiskEvents())
	}
}

func handleBrokerOrderUpdatesWorker(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.BrokerOrderUpdatesSnapshot())
	}
}
