package system

import (
	"errors"
	"io"
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
// @Success 200 {object} httpserver.Envelope{data=FutuOpenDHealthResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=AcceptedResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=FutuOpenDInstallGuideResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=RuntimeDependenciesResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=SystemStatusResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/status [get]
func handleSystemStatus(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, toSystemStatusResponse(svc.Status()))
	}
}

// handleExchangeCalendarStatus godoc
// @Summary 读取交易日历源状态
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ExchangeCalendarStatusResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=ExchangeCalendarSourcesResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=ExchangeCalendarRefreshResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=ExchangeCalendarRefreshResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=ExchangeCalendarProbeResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=ExchangeCalendarProbeResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/exchange-calendars/probe/{market} [post]
func handleExchangeCalendarProbePath(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.ProbeExchangeCalendars(c.Request.Context(), c.Param("market")))
	}
}

// handleStorageOverview godoc
// @Summary 读取系统存储概览
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=StorageOverviewResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/storage/overview [get]
func handleStorageOverview(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.StorageOverview())
	}
}

// handleRealTradeApprovals godoc
// @Summary 读取实盘审批状态
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeApprovalsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-approvals [get]
func handleRealTradeApprovals(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeApprovals())
	}
}

// handleRealTradeHardStops godoc
// @Summary 读取实盘硬停止列表
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeHardStopsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-hard-stops [get]
func handleRealTradeHardStops(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeHardStops())
	}
}

// handleActivateRealTradeHardStop godoc
// @Summary 创建实盘硬停止
// @Tags system
// @Accept json
// @Produce json
// @Param request body RealTradeHardStopRequest true "硬停止创建请求"
// @Success 200 {object} httpserver.Envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-hard-stops [post]
func handleActivateRealTradeHardStop(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request RealTradeHardStopRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade hard stop payload")
			return
		}
		result, err := svc.ActivateRealTradeHardStop(c.Request.Context(), request.command())
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleReleaseRealTradeHardStop godoc
// @Summary 解除实盘硬停止
// @Tags system
// @Accept json
// @Produce json
// @Param hardStopId path string true "硬停止 ID"
// @Param request body RealTradeHardStopRequest false "硬停止解除请求"
// @Success 200 {object} httpserver.Envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-hard-stops/{hardStopId}/release [post]
func handleReleaseRealTradeHardStop(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		hardStopID := strings.TrimSpace(c.Param("hardStopId"))
		if hardStopID == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "hard stop id is required")
			return
		}
		var request RealTradeHardStopRequest
		if err := bindOptionalJSON(c, &request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade hard stop release payload")
			return
		}
		result, err := svc.ReleaseRealTradeHardStop(c.Request.Context(), hardStopID, request.command())
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleRealTradeHardStopEvents godoc
// @Summary 读取实盘硬停止事件
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeHardStopEventsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-hard-stop-events [get]
func handleRealTradeHardStopEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeHardStopEvents())
	}
}

// handleRealTradeKillSwitch godoc
// @Summary 读取实盘熔断状态
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeKillSwitchStateResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-kill-switch [get]
func handleRealTradeKillSwitch(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeKillSwitch())
	}
}

// handleActivateRealTradeKillSwitch godoc
// @Summary 激活实盘熔断
// @Tags system
// @Accept json
// @Produce json
// @Param request body RealTradeKillSwitchRequest true "熔断激活请求"
// @Success 200 {object} httpserver.Envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-kill-switch/activate [post]
func handleActivateRealTradeKillSwitch(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request RealTradeKillSwitchRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade kill switch payload")
			return
		}
		result, err := svc.ActivateRealTradeKillSwitch(c.Request.Context(), request.command())
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleReleaseRealTradeKillSwitch godoc
// @Summary 解除实盘熔断
// @Tags system
// @Accept json
// @Produce json
// @Param request body RealTradeKillSwitchRequest false "熔断解除请求"
// @Success 200 {object} httpserver.Envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-kill-switch/release [post]
func handleReleaseRealTradeKillSwitch(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request RealTradeKillSwitchRequest
		if err := bindOptionalJSON(c, &request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade kill switch release payload")
			return
		}
		result, err := svc.ReleaseRealTradeKillSwitch(c.Request.Context(), request.command())
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleRealTradeKillSwitchEvents godoc
// @Summary 读取实盘熔断事件
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeKillSwitchEventsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-kill-switch-events [get]
func handleRealTradeKillSwitchEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeKillSwitchEvents())
	}
}

// handleRealTradeRiskLimits godoc
// @Summary 读取实盘运行时风控限额
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeRiskLimitsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-risk-limits [get]
func handleRealTradeRiskLimits(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeRiskLimits())
	}
}

// handleUpdateRealTradeRiskLimits godoc
// @Summary 更新实盘运行时风控限额
// @Tags system
// @Accept json
// @Produce json
// @Param request body RealTradeRuntimeRiskRequest true "运行时风控配置"
// @Success 200 {object} httpserver.Envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-risk-limits [put]
func handleUpdateRealTradeRiskLimits(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request RealTradeRuntimeRiskRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade runtime risk payload")
			return
		}
		command := request.command()
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

// handleDisableRealTradeRiskLimits godoc
// @Summary 禁用实盘运行时风控限额
// @Tags system
// @Accept json
// @Produce json
// @Param request body RealTradeRuntimeRiskRequest false "禁用请求"
// @Success 200 {object} httpserver.Envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-risk-limits [delete]
func handleDisableRealTradeRiskLimits(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request RealTradeRuntimeRiskRequest
		if err := bindOptionalJSON(c, &request); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid real-trade runtime risk disable payload")
			return
		}
		result, err := svc.DisableRealTradeRuntimeRisk(c.Request.Context(), request.command())
		if err != nil {
			httpserver.WriteError(c, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleRealTradeRiskEvents godoc
// @Summary 读取实盘运行时风控事件
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=sys.RealTradeRiskEventsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/real-trade-risk-events [get]
func handleRealTradeRiskEvents(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.RealTradeRiskEvents())
	}
}

func bindOptionalJSON(c *gin.Context, target any) error {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil
	}
	if err := c.ShouldBindJSON(target); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
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

// handleBrokerOrderUpdatesWorker godoc
// @Summary 读取 broker 订单更新 Worker 状态
// @Tags system
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=BrokerOrderUpdatesResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/system/worker/broker-order-updates [get]
func handleBrokerOrderUpdatesWorker(svc *sys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.BrokerOrderUpdatesSnapshot())
	}
}
