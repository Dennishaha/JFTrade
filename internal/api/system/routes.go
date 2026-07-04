package system

import (
	"errors"
	"net/http"
	"strings"

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
	system.GET("/runtime-dependencies", handleRuntimeDependencies(svc))
	system.GET("/exchange-calendars/status", handleExchangeCalendarStatus(svc))
	system.GET("/exchange-calendars/sources", handleExchangeCalendarSources(svc))
	system.POST("/exchange-calendars/refresh", handleExchangeCalendarRefresh(svc, ""))
	system.POST("/exchange-calendars/refresh/:market", handleExchangeCalendarRefreshPath(svc))
	system.POST("/exchange-calendars/probe", handleExchangeCalendarProbe(svc, ""))
	system.POST("/exchange-calendars/probe/:market", handleExchangeCalendarProbePath(svc))
	system.GET("/status", handleSystemStatus(svc))
	system.GET("/storage/overview", handleStorageOverview(svc))
	system.GET("/real-trade-approvals", handleRealTradeApprovals(svc))
	system.GET("/real-trade-hard-stops", handleRealTradeHardStops(svc))
	system.POST("/real-trade-hard-stops", handleActivateRealTradeHardStop(svc))
	system.POST("/real-trade-hard-stops/:hardStopId/release", handleReleaseRealTradeHardStop(svc))
	system.GET("/real-trade-hard-stop-events", handleRealTradeHardStopEvents(svc))
	system.GET("/real-trade-kill-switch", handleRealTradeKillSwitch(svc))
	system.POST("/real-trade-kill-switch/activate", handleActivateRealTradeKillSwitch(svc))
	system.POST("/real-trade-kill-switch/release", handleReleaseRealTradeKillSwitch(svc))
	system.GET("/real-trade-kill-switch-events", handleRealTradeKillSwitchEvents(svc))
	system.GET("/real-trade-risk-limits", handleRealTradeRiskLimits(svc))
	system.PUT("/real-trade-risk-limits", handleUpdateRealTradeRiskLimits(svc))
	system.DELETE("/real-trade-risk-limits", handleDisableRealTradeRiskLimits(svc))
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

// handleRuntimeDependencies godoc
// @Summary 读取运行时依赖检查结果
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/runtime-dependencies [get]
func handleRuntimeDependencies(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RuntimeDependencies(c.Request.Context()))
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

// handleExchangeCalendarStatus godoc
// @Summary 读取交易日历源状态
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/exchange-calendars/status [get]
func handleExchangeCalendarStatus(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.ExchangeCalendarStatus())
	}
}

// handleExchangeCalendarSources godoc
// @Summary 列出交易日历数据源
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/exchange-calendars/sources [get]
func handleExchangeCalendarSources(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, map[string]any{"sources": svc.ExchangeCalendarSources()})
	}
}

// handleExchangeCalendarRefresh godoc
// @Summary 刷新所有交易日历
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/exchange-calendars/refresh [post]
func handleExchangeCalendarRefresh(svc *sys.Service, market string) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RefreshExchangeCalendars(c.Request.Context(), market))
	}
}

// handleExchangeCalendarRefreshPath godoc
// @Summary 刷新指定市场交易日历
// @Tags system
// @Produce json
// @Param market path string true "市场代码"
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/exchange-calendars/refresh/{market} [post]
func handleExchangeCalendarRefreshPath(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RefreshExchangeCalendars(c.Request.Context(), c.Param("market")))
	}
}

// handleExchangeCalendarProbe godoc
// @Summary 探测所有交易日历源
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/exchange-calendars/probe [post]
func handleExchangeCalendarProbe(svc *sys.Service, market string) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.ProbeExchangeCalendars(c.Request.Context(), market))
	}
}

// handleExchangeCalendarProbePath godoc
// @Summary 探测指定市场交易日历源
// @Tags system
// @Produce json
// @Param market path string true "市场代码"
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/system/exchange-calendars/probe/{market} [post]
func handleExchangeCalendarProbePath(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.ProbeExchangeCalendars(c.Request.Context(), c.Param("market")))
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

func handleActivateRealTradeHardStop(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var command sys.RealTradeHardStopCommand
		if err := c.ShouldBindJSON(&command); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade hard stop payload")
			return
		}
		result, err := svc.ActivateRealTradeHardStop(c.Request.Context(), command)
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleReleaseRealTradeHardStop(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		hardStopID := strings.TrimSpace(c.Param("hardStopId"))
		if hardStopID == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "hard stop id is required")
			return
		}
		var command sys.RealTradeHardStopCommand
		if c.Request.Body != nil {
			_ = c.ShouldBindJSON(&command)
		}
		result, err := svc.ReleaseRealTradeHardStop(c.Request.Context(), hardStopID, command)
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
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

func handleActivateRealTradeKillSwitch(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var command sys.RealTradeKillSwitchCommand
		if err := c.ShouldBindJSON(&command); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade kill switch payload")
			return
		}
		result, err := svc.ActivateRealTradeKillSwitch(c.Request.Context(), command)
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleReleaseRealTradeKillSwitch(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var command sys.RealTradeKillSwitchCommand
		if c.Request.Body != nil {
			_ = c.ShouldBindJSON(&command)
		}
		result, err := svc.ReleaseRealTradeKillSwitch(c.Request.Context(), command)
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
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

func handleUpdateRealTradeRiskLimits(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var command sys.RealTradeRuntimeRiskCommand
		if err := c.ShouldBindJSON(&command); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade runtime risk payload")
			return
		}
		if err := validateRealTradeRuntimeRiskCommand(command); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		result, err := svc.UpdateRealTradeRuntimeRisk(c.Request.Context(), command)
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleDisableRealTradeRiskLimits(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var command sys.RealTradeRuntimeRiskCommand
		if c.Request.Body != nil {
			_ = c.ShouldBindJSON(&command)
		}
		result, err := svc.DisableRealTradeRuntimeRisk(c.Request.Context(), command)
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

func handleRealTradeRiskEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeRiskEvents())
	}
}

func validateRealTradeRuntimeRiskCommand(command sys.RealTradeRuntimeRiskCommand) error {
	if command.MaxOrderQuantity != nil && *command.MaxOrderQuantity <= 0 {
		return errors.New("maxOrderQuantity must be positive when provided")
	}
	if command.MaxOrderNotional != nil && *command.MaxOrderNotional <= 0 {
		return errors.New("maxOrderNotional must be positive when provided")
	}
	if command.RealTradingEnabled {
		hasQuantityLimit := command.MaxOrderQuantity != nil && *command.MaxOrderQuantity > 0
		hasNotionalLimit := command.MaxOrderNotional != nil && *command.MaxOrderNotional > 0
		if !hasQuantityLimit && !hasNotionalLimit {
			return errors.New("at least one positive runtime risk limit is required before enabling real trading")
		}
	}
	return nil
}

func handleBrokerOrderUpdatesWorker(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.BrokerOrderUpdatesSnapshot())
	}
}
