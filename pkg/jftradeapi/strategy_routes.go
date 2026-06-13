package jftradeapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

// handleStrategyDefinitions godoc
// @Summary 读取策略定义列表
// @Tags strategy
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/strategy-definitions [get]
func (s *Server) handleStrategyDefinitions(c *gin.Context) {
	s.writeOK(c, s.designStore.listDefinitions())
}

// handleCreateStrategyDefinition godoc
// @Summary 创建策略定义
// @Tags strategy
// @Accept json
// @Produce json
// @Param request body strategyDesignDefinition true "策略定义"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/strategy-definitions [post]
func (s *Server) handleCreateStrategyDefinition(c *gin.Context) {
	var payload strategyDesignDefinition
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy definition payload")
		return
	}
	payload.ID = ""
	if err := strategydefinition.ValidateScript(payload.SourceFormat, payload.Script); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	definition, err := s.designStore.saveDefinition(payload)
	if err != nil {
		if errors.Is(err, errUnsupportedLegacyStrategyDefinition) {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save strategy definition")
		return
	}
	s.writeOK(c, definition)
}

// handleStrategyDefinition godoc
// @Summary 读取策略定义
// @Tags strategy
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Param interval query string false "预览周期"
// @Param symbol query string false "预览标的"
// @Param useExtendedHours query bool false "是否包含盘前盘后"
// @Success 200 {object} envelope{data=strategyDefinitionResponse}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/strategy-definitions/{definitionId} [get]
func (s *Server) handleStrategyDefinition(c *gin.Context) {
	definitionID, ok := s.strategyDefinitionParam(c)
	if !ok {
		return
	}
	definition, exists, err := s.designStore.definition(definitionID)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
		return
	}
	var query strategyDefinitionPreviewQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy definition query")
		return
	}
	s.writeOK(c, buildStrategyDefinitionResponse(definition, query.Interval, query.Symbol, query.UseExtendedHours))
}

// handleUpdateStrategyDefinition godoc
// @Summary 更新策略定义
// @Tags strategy
// @Accept json
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Param request body strategyDesignDefinition true "策略定义"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/strategy-definitions/{definitionId} [put]
func (s *Server) handleUpdateStrategyDefinition(c *gin.Context) {
	definitionID, ok := s.strategyDefinitionParam(c)
	if !ok {
		return
	}
	var payload strategyDesignDefinition
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy definition payload")
		return
	}
	if err := strategydefinition.ValidateScript(payload.SourceFormat, payload.Script); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	payload.ID = definitionID
	definition, err := s.designStore.saveDefinition(payload)
	if err != nil {
		if errors.Is(err, errUnsupportedLegacyStrategyDefinition) {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save strategy definition")
		return
	}
	s.writeOK(c, definition)
}

// handleDeleteStrategyDefinition godoc
// @Summary 删除策略定义
// @Tags strategy
// @Produce json
// @Param definitionId path string true "策略定义 ID"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/strategy-definitions/{definitionId} [delete]
func (s *Server) handleDeleteStrategyDefinition(c *gin.Context) {
	definitionID, ok := s.strategyDefinitionParam(c)
	if !ok {
		return
	}
	linkedInstances := s.strategyStore.linkedStrategyInstanceIDs(definitionID)
	if len(linkedInstances) > 0 {
		message := fmt.Sprintf("当前有 %d 个实例仍关联该策略，请先删除对应实例再删除。实例: %s", len(linkedInstances), strings.Join(linkedInstances, ", "))
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", message)
		return
	}
	definition, err := s.designStore.deleteDefinition(definitionID)
	if err != nil {
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete strategy definition")
		return
	}
	s.writeOK(c, definition)
}

func (s *Server) handleApplyLinkedStrategyInstances(c *gin.Context) {
	definitionID, ok := s.strategyDefinitionParam(c)
	if !ok {
		return
	}
	definition, exists, err := s.designStore.definition(definitionID)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
		return
	}
	result, applyErr := s.strategyStore.applyDefinitionToLinkedStrategies(definition)
	if applyErr != nil {
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to apply linked strategy instances")
		return
	}
	s.writeOK(c, result)
}

func (s *Server) handleInstantiateStrategyDefinition(c *gin.Context) {
	definitionID, ok := s.strategyDefinitionParam(c)
	if !ok {
		return
	}
	definition, exists, err := s.designStore.definition(definitionID)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
		return
	}
	if err := strategydefinition.ValidateScript(definition.SourceFormat, definition.Script); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if !strategydefinition.SupportsInstantiation(definition.SourceFormat) {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("strategy source format %s is not instantiable yet", strategydefinition.NormalizeSourceFormat(definition.SourceFormat)))
		return
	}
	var payload strategyInstanceBinding
	if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy instance payload")
		return
	}
	instance, err := s.strategyStore.instantiateStrategy(definition, payload)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to instantiate strategy")
		return
	}
	s.writeOK(c, s.enrichStrategyItem(instance))
}

// handleStrategies godoc
// @Summary 读取策略实例列表
// @Tags strategy
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/strategies [get]
func (s *Server) handleStrategies(c *gin.Context) {
	s.writeOK(c, s.enrichStrategyItems(s.strategyStore.strategies()))
}

func (s *Server) handleUpdateStrategy(c *gin.Context) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	var payload strategyInstanceBinding
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy instance payload")
		return
	}
	instance, err := s.strategyStore.updateStrategyBinding(instanceID, payload)
	if err != nil {
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return
		}
		if err == errStrategyInstanceBusy {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "strategy instance must be stopped before updating bindings")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update strategy instance")
		return
	}
	s.writeOK(c, s.enrichStrategyItem(instance))
}

func (s *Server) handleDeleteStrategy(c *gin.Context) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	instance, err := s.strategyStore.deleteStrategy(instanceID)
	if err != nil {
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return
		}
		if err == errStrategyInstanceBusy {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "strategy instance must be stopped before deletion")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete strategy instance")
		return
	}
	s.writeOK(c, s.enrichStrategyItem(instance))
}

func (s *Server) handleStartStrategy(c *gin.Context) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	instanceRecord, exists := s.strategyStore.strategy(instanceID)
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
		return
	}
	if !strategyInstanceStartable(instanceRecord) {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("strategy runtime %s is not startable yet", strategyRuntimeFromParams(instanceRecord.Params)))
		return
	}
	if err := s.strategyRuntimeManager.startStrategy(c.Request.Context(), instanceRecord); err != nil {
		status, code := strategyRuntimeStartError(err)
		s.writeError(c, status, code, err.Error())
		return
	}
	instance, err := s.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "strategy runtime requested start")
	if err != nil {
		s.strategyRuntimeManager.stopStrategy(instanceID)
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to start strategy")
		return
	}
	s.ensureLiveMarketStream(context.Background(), s.activeLiveStreamInstrumentIDs())
	s.writeOK(c, s.enrichStrategyItem(instance))
}

func (s *Server) handleRefreshStrategyDefinition(c *gin.Context) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	instanceRecord, exists := s.strategyStore.strategy(instanceID)
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
		return
	}
	definitionID := strings.TrimSpace(instanceRecord.Definition.StrategyID)
	if definitionID == "" {
		definitionID = strategyDefinitionIDFromParams(instanceRecord.Params)
	}
	if definitionID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "strategy instance is not linked to a saved definition")
		return
	}
	definition, exists, err := s.designStore.definition(definitionID)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
		return
	}
	instance, refreshErr := s.strategyStore.refreshStrategyDefinition(instanceID, definition)
	if refreshErr != nil {
		if errorsIsNotFound(refreshErr) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return
		}
		if refreshErr == errStrategyInstanceBusy {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "strategy instance must be stopped before refreshing definition")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to refresh strategy definition")
		return
	}
	s.writeOK(c, s.enrichStrategyItem(instance))
}

func (s *Server) handlePauseStrategy(c *gin.Context) {
	s.transitionStrategy(c, strategyStatusPaused, "paused", "manual pause", "failed to pause strategy")
}

func (s *Server) handleStopStrategy(c *gin.Context) {
	s.transitionStrategy(c, strategyStatusStopped, "stopped", "manual stop", "failed to stop strategy")
}

func (s *Server) transitionStrategy(c *gin.Context, status string, action string, message string, errorMessage string) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	instance, err := s.strategyStore.transitionStrategy(instanceID, status, action, message)
	if err != nil {
		if errorsIsNotFound(err) {
			s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return
		}
		s.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", errorMessage)
		return
	}
	s.strategyRuntimeManager.stopStrategy(instanceID)
	s.ensureLiveMarketStream(context.Background(), s.activeLiveStreamInstrumentIDs())
	s.writeOK(c, s.enrichStrategyItem(instance))
}

// handleStrategyLogs godoc
// @Summary 读取策略运行日志
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param level query string false "日志级别"
// @Param fromTime query string false "起始时间"
// @Param toTime query string false "结束时间"
// @Success 200 {object} envelope{data=strategyLogsResponse}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/strategies/{instanceId}/logs [get]
func (s *Server) handleStrategyLogs(c *gin.Context) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	var query strategyActivityPageQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy logs query")
		return
	}
	limit, offset := normalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 500, 5000)
	logs, exists := s.strategyStore.strategyLogsPage(instanceID, strategyRuntimeLogQuery{
		Limit:  limit,
		Offset: offset,
		Level:  strings.TrimSpace(query.Level),
		FromAt: query.FromTime.PtrUTC(),
		ToAt:   query.ToTime.PtrUTC(),
	})
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
		return
	}
	s.writeOK(c, logs)
}

// handleStrategyAudit godoc
// @Summary 读取策略审计记录
// @Tags strategy
// @Produce json
// @Param instanceId path string true "策略实例 ID"
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param kind query string false "审计类型"
// @Param fromTime query string false "起始时间"
// @Param toTime query string false "结束时间"
// @Success 200 {object} envelope{data=strategyAuditResponse}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/strategies/{instanceId}/audit [get]
func (s *Server) handleStrategyAudit(c *gin.Context) {
	instanceID, ok := s.strategyInstanceParam(c)
	if !ok {
		return
	}
	var query strategyActivityPageQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy audit query")
		return
	}
	limit, offset := normalizeBoundPage(query.Limit.Int(), query.Offset.Int(), 500, 5000)
	audit, exists := s.strategyStore.strategyAuditPage(instanceID, strategyRuntimeAuditQuery{
		Limit:  limit,
		Offset: offset,
		Kind:   strings.TrimSpace(query.Kind),
		FromAt: query.FromTime.PtrUTC(),
		ToAt:   query.ToTime.PtrUTC(),
	})
	if !exists {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
		return
	}
	s.writeOK(c, audit)
}

func (s *Server) strategyDefinitionParam(c *gin.Context) (string, bool) {
	var uri definitionURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.DefinitionID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "definitionId is invalid")
		return "", false
	}
	return uri.DefinitionID, true
}

func (s *Server) strategyInstanceParam(c *gin.Context) (string, bool) {
	var uri instanceURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.InstanceID) == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
		return "", false
	}
	return uri.InstanceID, true
}
