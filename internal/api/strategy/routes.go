// Package strategy 提供策略 HTTP 路由注册与瘦 handler 工厂函数。
// 每个 handler 是纯工厂函数 func(svc *srv.Service) gin.HandlerFunc，
// 只负责参数绑定与响应写入，不持有任何内部状态。
//
// 覆盖策略定义、实例生命周期、活动查询和 Pine 分析路由。
package strategy

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
)

// logPageQuery 是日志/审计分页查询参数。
type logPageQuery struct {
	Limit    httpserver.OptionalIntValue  `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset   httpserver.OptionalIntValue  `form:"offset,parser=encoding.TextUnmarshaler"`
	Level    string                       `form:"level"`
	Kind     string                       `form:"kind"`
	FromTime httpserver.OptionalTimeValue `form:"fromTime,parser=encoding.TextUnmarshaler"`
	ToTime   httpserver.OptionalTimeValue `form:"toTime,parser=encoding.TextUnmarshaler"`
}

type definitionPreviewQuery struct {
	Interval         string `form:"interval"`
	Symbol           string `form:"symbol"`
	UseExtendedHours bool   `form:"useExtendedHours"`
}

// RegisterRoutes 注册所有 /api/v1/strategy* 路由。
func RegisterRoutes(api *gin.RouterGroup, svc *srv.Service) {
	// Pine analyze
	api.POST("/strategy-pine/analyze", handleAnalyzePine(svc))

	// Strategy Definitions
	api.GET("/strategy-definitions", handleListDefinitions(svc))
	api.POST("/strategy-definitions", handleCreateDefinition(svc))
	api.GET("/strategy-definitions/:definitionId", handleGetDefinition(svc))
	api.PUT("/strategy-definitions/:definitionId", handleUpdateDefinition(svc))
	api.DELETE("/strategy-definitions/:definitionId", handleDeleteDefinition(svc))
	api.POST("/strategy-definitions/:definitionId/apply-linked-instances", handleApplyLinked(svc))
	api.POST("/strategy-definitions/:definitionId/instantiate", handleInstantiate(svc))

	// Strategy Instances
	api.GET("/strategies", handleListInstances(svc))
	api.PUT("/strategies/:instanceId", handleUpdateInstance(svc))
	api.PUT("/strategies/:instanceId/runtime-risk", handleUpdateInstanceRuntimeRisk(svc))
	api.DELETE("/strategies/:instanceId", handleDeleteInstance(svc))
	api.POST("/strategies/:instanceId/start", handleStartInstance(svc))
	api.POST("/strategies/:instanceId/refresh-definition", handleRefreshDefinition(svc))
	api.POST("/strategies/:instanceId/pause", handlePauseInstance(svc))
	api.POST("/strategies/:instanceId/stop", handleStopInstance(svc))
	api.GET("/strategies/:instanceId/logs", handleGetLogs(svc))
	api.GET("/strategies/:instanceId/audit", handleGetAudit(svc))
}

// ──────────────────────────────────────────────────────────────────────────────
// Pine Analyze
// ──────────────────────────────────────────────────────────────────────────────

// handleAnalyzePine godoc
// @Summary 分析 Pine 脚本
// @Tags strategy
// @Accept json
// @Produce json
// @Param request body AnalyzePineRequest true "Pine 脚本"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/strategy-pine/analyze [post]
func handleAnalyzePine(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			Script       string `json:"script"`
			SourceFormat string `json:"sourceFormat"`
			IncludeAST   bool   `json:"includeAst"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy pine analyze payload")
			return
		}
		result, err := svc.AnalyzePine(srv.PineAnalyzeInput{
			Script:       input.Script,
			SourceFormat: input.SourceFormat,
			IncludeAST:   input.IncludeAST,
		})
		if err != nil {
			if errors.Is(err, srv.ErrBadRequest) {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			httpserver.WriteError(c, http.StatusBadRequest, "PINE_ANALYSIS_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Strategy Definitions — CRUD
// ──────────────────────────────────────────────────────────────────────────────

// handleListDefinitions godoc
// @Summary 读取策略定义列表
// @Tags strategy
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions [get]
func handleListDefinitions(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.ListDefinitions())
	}
}

// handleGetDefinition godoc
// @Summary 读取策略定义
// @Tags strategy
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Param interval query string false "预览周期"
// @Param symbol query string false "预览标的"
// @Param useExtendedHours query bool false "是否包含盘前盘后"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions/{definitionId} [get]
func handleGetDefinition(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			DefinitionID string `uri:"definitionId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition id")
			return
		}
		result, ok, err := svc.GetDefinition(uri.DefinitionID)
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "STRATEGY_FAILED", err.Error())
			return
		}
		if !ok {
			httpserver.WriteNotFound(c)
			return
		}
		var query definitionPreviewQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy definition query")
			return
		}
		// Enrich with preview data (mirrors old strategyDefinitionResponse)
		enriched := enrichDefinitionResponse(result, query)
		httpserver.WriteOK(c, enriched)
	}
}

// enrichDefinitionResponse 为策略定义附加预热 K 线推导等预览字段。
func enrichDefinitionResponse(definition srv.Definition, query definitionPreviewQuery) srv.DefinitionView {
	interval := strings.TrimSpace(query.Interval)
	if interval == "" {
		interval = "5m"
	}

	symbol := definition.Symbol
	if qs := strings.TrimSpace(query.Symbol); qs != "" {
		symbol = qs
	}

	derivedWarmupBars := 0
	if warmupBars, err := indicatorruntime.WarmupBarsFromScriptForSymbolWithOptions(
		definition.Script,
		types.Interval(interval),
		symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: query.UseExtendedHours},
	); err == nil {
		derivedWarmupBars = warmupBars
	}

	return srv.DefinitionView{
		Definition:            definition,
		DerivedWarmupBars:     derivedWarmupBars,
		DerivedWarmupInterval: interval,
	}
}

func writeStrategyError(c *gin.Context, err error, fallbackStatus int, fallbackCode string, fallbackMessage string) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, srv.ErrNotFound):
		httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", fallbackIfEmpty(err.Error(), "resource not found"))
	case errors.Is(err, srv.ErrBusy), errors.Is(err, srv.ErrBadRequest):
		httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", fallbackIfEmpty(err.Error(), fallbackMessage))
	case errors.Is(err, srv.ErrUpstream):
		httpserver.WriteError(c, http.StatusBadGateway, "STRATEGY_RUNTIME_START_FAILED", fallbackIfEmpty(err.Error(), fallbackMessage))
	default:
		httpserver.WriteError(c, fallbackStatus, fallbackCode, fallbackIfEmpty(fallbackMessage, err.Error()))
	}
}

func fallbackIfEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// handleCreateDefinition godoc
// @Summary 创建策略定义
// @Tags strategy
// @Accept json
// @Produce json
// @Param request body StrategyDesignDefinition true "策略定义"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions [post]
func handleCreateDefinition(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input srv.Definition
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition payload")
			return
		}
		// 创建时忽略客户端传入的 ID，由 store 自动生成 UUID。
		input.ID = ""
		result, err := svc.SaveDefinition(input)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to save strategy definition")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleUpdateDefinition godoc
// @Summary 更新策略定义
// @Tags strategy
// @Accept json
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Param request body StrategyDesignDefinition true "策略定义"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions/{definitionId} [put]
func handleUpdateDefinition(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			DefinitionID string `uri:"definitionId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition id")
			return
		}
		var input srv.Definition
		if err := c.ShouldBindJSON(&input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition payload")
			return
		}
		input.ID = uri.DefinitionID
		result, err := svc.SaveDefinition(input)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to save strategy definition")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleDeleteDefinition godoc
// @Summary 删除策略定义
// @Tags strategy
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions/{definitionId} [delete]
func handleDeleteDefinition(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			DefinitionID string `uri:"definitionId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition id")
			return
		}
		linked := svc.GetLinkedInstanceIDs(uri.DefinitionID)
		if len(linked) > 0 {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST",
				"当前有 "+strings.Join(linked, ", ")+" 个实例仍关联该策略，请先删除对应实例再删除。实例: "+strings.Join(linked, ", "))
			return
		}
		result, err := svc.DeleteDefinition(uri.DefinitionID)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to delete strategy definition")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Strategy Definitions — Advanced
// ──────────────────────────────────────────────────────────────────────────────

// handleApplyLinked godoc
// @Summary 应用策略定义到关联实例
// @Tags strategy
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions/{definitionId}/apply-linked-instances [post]
func handleApplyLinked(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			DefinitionID string `uri:"definitionId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition id")
			return
		}
		def, ok, err := svc.GetDefinition(uri.DefinitionID)
		if err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		if !ok {
			httpserver.WriteNotFound(c)
			return
		}
		result, applyErr := svc.ApplyDefinitionToLinked(def)
		if applyErr != nil {
			writeStrategyError(c, applyErr, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to apply linked instances")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleInstantiate godoc
// @Summary 从策略定义创建实例
// @Tags strategy
// @Accept json
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Param request body StrategyBindingRequest false "实例绑定参数"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategy-definitions/{definitionId}/instantiate [post]
func handleInstantiate(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			DefinitionID string `uri:"definitionId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid definition id")
			return
		}
		def, ok, err := svc.GetDefinition(uri.DefinitionID)
		if err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		if !ok {
			httpserver.WriteNotFound(c)
			return
		}
		var binding srv.InstanceBinding
		if bindErr := c.ShouldBindJSON(&binding); bindErr != nil {
			if !errors.Is(bindErr, io.EOF) {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy instance payload")
				return
			}
			// 允许空 body
			binding = srv.InstanceBinding{}
		}
		instance, createErr := svc.CreateInstance(def, binding)
		if createErr != nil {
			writeStrategyError(c, createErr, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to instantiate strategy")
			return
		}
		httpserver.WriteOK(c, instance)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Strategy Instances — CRUD
// ──────────────────────────────────────────────────────────────────────────────

// handleListInstances godoc
// @Summary 读取策略实例列表
// @Tags strategy
// @Produce json
// @Success 200 {object} httpserver.Envelope
// @Router /api/v1/strategies [get]
func handleListInstances(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		httpserver.WriteOK(c, svc.ListInstances())
	}
}

// handleUpdateInstance godoc
// @Summary 更新策略实例绑定参数
// @Tags strategy
// @Accept json
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Param request body StrategyBindingRequest true "实例绑定参数"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId} [put]
func handleUpdateInstance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		var binding srv.InstanceBinding
		if err := c.ShouldBindJSON(&binding); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance payload")
			return
		}
		instance, err := svc.UpdateInstance(uri.InstanceID, binding)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to update strategy instance")
			return
		}
		httpserver.WriteOK(c, instance)
	}
}

// handleUpdateInstanceRuntimeRisk godoc
// @Summary 更新策略实例动态风控
// @Tags strategy
// @Accept json
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Param request body srv.RuntimeRiskSettings true "动态风控设置"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/runtime-risk [put]
func handleUpdateInstanceRuntimeRisk(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		var risk srv.RuntimeRiskSettings
		if err := c.ShouldBindJSON(&risk); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid runtime risk payload")
			return
		}
		instance, err := svc.UpdateInstanceRuntimeRisk(uri.InstanceID, risk)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to update strategy runtime risk")
			return
		}
		httpserver.WriteOK(c, instance)
	}
}

// handleDeleteInstance godoc
// @Summary 删除策略实例
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId} [delete]
func handleDeleteInstance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		instance, err := svc.DeleteInstance(uri.InstanceID)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to delete strategy instance")
			return
		}
		httpserver.WriteOK(c, instance)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Strategy Instances — Lifecycle
// ──────────────────────────────────────────────────────────────────────────────

// handleStartInstance godoc
// @Summary 启动策略实例
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Failure 502 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/start [post]
func handleStartInstance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		result, err := svc.StartInstance(c.Request.Context(), uri.InstanceID)
		if err != nil {
			writeStrategyError(c, err, http.StatusBadGateway, "STRATEGY_RUNTIME_START_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleRefreshDefinition godoc
// @Summary 刷新实例关联的策略定义
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/refresh-definition [post]
func handleRefreshDefinition(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		result, err := svc.RefreshInstanceDefinition(uri.InstanceID)
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to refresh strategy definition")
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handlePauseInstance godoc
// @Summary 暂停策略实例
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/pause [post]
func handlePauseInstance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		instance, err := svc.TransitionInstance(uri.InstanceID, "PAUSED")
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to pause strategy")
			return
		}
		svc.Stop(uri.InstanceID)
		httpserver.WriteOK(c, instance)
	}
}

// handleStopInstance godoc
// @Summary 停止策略实例
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/stop [post]
func handleStopInstance(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		instance, err := svc.TransitionInstance(uri.InstanceID, "STOPPED")
		if err != nil {
			writeStrategyError(c, err, http.StatusInternalServerError, "STRATEGY_FAILED", "failed to stop strategy")
			return
		}
		svc.Stop(uri.InstanceID)
		httpserver.WriteOK(c, instance)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Strategy Instances — Activity
// ──────────────────────────────────────────────────────────────────────────────

// handleGetLogs godoc
// @Summary 读取策略运行日志
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param level query string false "日志级别"
// @Param fromTime query string false "起始时间"
// @Param toTime query string false "结束时间"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/logs [get]
func handleGetLogs(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		var query logPageQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid logs query")
			return
		}
		limit, offset := httpserver.NormalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 500, 5000)
		logs, exists := svc.GetLogs(uri.InstanceID, srv.LogQuery{
			Limit:  limit,
			Offset: offset,
			Level:  strings.TrimSpace(query.Level),
			FromAt: query.FromTime.PtrUTC(),
			ToAt:   query.ToTime.PtrUTC(),
		})
		if !exists {
			httpserver.WriteNotFound(c)
			return
		}
		httpserver.WriteOK(c, logs)
	}
}

// handleGetAudit godoc
// @Summary 读取策略审计记录
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param kind query string false "审计类型"
// @Param fromTime query string false "起始时间"
// @Param toTime query string false "结束时间"
// @Success 200 {object} httpserver.Envelope
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Router /api/v1/strategies/{instanceId}/audit [get]
func handleGetAudit(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			InstanceID string `uri:"instanceId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid instance id")
			return
		}
		var query logPageQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid audit query")
			return
		}
		limit, offset := httpserver.NormalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 500, 5000)
		audit, exists := svc.GetAudit(uri.InstanceID, srv.AuditQuery{
			Limit:  limit,
			Offset: offset,
			Kind:   strings.TrimSpace(query.Kind),
			FromAt: query.FromTime.PtrUTC(),
			ToAt:   query.ToTime.PtrUTC(),
		})
		if !exists {
			httpserver.WriteNotFound(c)
			return
		}
		httpserver.WriteOK(c, audit)
	}
}
