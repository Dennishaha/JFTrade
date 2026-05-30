package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func (s *Server) serveStrategyRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/strategy-definitions" && r.Method == http.MethodGet:
		s.writeOK(w, s.designStore.listDefinitions())
	case r.URL.Path == "/api/v1/strategy-definitions" && r.Method == http.MethodPost:
		var payload strategyDesignDefinition
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy definition payload")
			return true
		}
		payload.ID = ""
		if err := strategydefinition.ValidateScript(payload.SourceFormat, payload.Script); err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return true
		}
		definition, err := s.designStore.saveDefinition(payload)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save strategy definition")
			return true
		}
		s.writeOK(w, definition)
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategy-definitions/") && r.Method == http.MethodGet:
		definitionID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/strategy-definitions/"))
		if err != nil || strings.TrimSpace(definitionID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "definitionId is invalid")
			return true
		}
		definition, ok := s.designStore.definition(definitionID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
			return true
		}
		query := r.URL.Query()
		useExtendedHours := false
		if raw := strings.TrimSpace(query.Get("useExtendedHours")); raw != "" {
			parsed, parseErr := strconv.ParseBool(raw)
			if parseErr != nil {
				s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "useExtendedHours must be a boolean")
				return true
			}
			useExtendedHours = parsed
		}
		s.writeOK(w, buildStrategyDefinitionResponse(
			definition,
			query.Get("interval"),
			query.Get("symbol"),
			useExtendedHours,
		))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategy-definitions/") && r.Method == http.MethodPut:
		definitionID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/strategy-definitions/"))
		if err != nil || strings.TrimSpace(definitionID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "definitionId is invalid")
			return true
		}
		var payload strategyDesignDefinition
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy definition payload")
			return true
		}
		if err := strategydefinition.ValidateScript(payload.SourceFormat, payload.Script); err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return true
		}
		payload.ID = definitionID
		definition, err := s.designStore.saveDefinition(payload)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save strategy definition")
			return true
		}
		s.writeOK(w, definition)
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategy-definitions/") && r.Method == http.MethodDelete:
		definitionID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/strategy-definitions/"))
		if err != nil || strings.TrimSpace(definitionID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "definitionId is invalid")
			return true
		}
		linkedInstances := s.strategyStore.linkedStrategyInstanceIDs(definitionID)
		if len(linkedInstances) > 0 {
			message := fmt.Sprintf("当前有 %d 个实例仍关联该策略，请先删除对应实例再删除。", len(linkedInstances))
			if len(linkedInstances) > 0 {
				message = fmt.Sprintf("当前有 %d 个实例仍关联该策略，请先删除对应实例再删除。实例: %s", len(linkedInstances), strings.Join(linkedInstances, ", "))
			}
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", message)
			return true
		}
		definition, err := s.designStore.deleteDefinition(definitionID)
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete strategy definition")
			return true
		}
		s.writeOK(w, definition)
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategy-definitions/") && strings.HasSuffix(r.URL.Path, "/apply-linked-instances") && r.Method == http.MethodPost:
		definitionID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategy-definitions/", "/apply-linked-instances"))
		if err != nil || strings.TrimSpace(definitionID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "definitionId is invalid")
			return true
		}
		definition, ok := s.designStore.definition(definitionID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
			return true
		}
		result, applyErr := s.strategyStore.applyDefinitionToLinkedStrategies(definition)
		if applyErr != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to apply linked strategy instances")
			return true
		}
		s.writeOK(w, result)
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategy-definitions/") && strings.HasSuffix(r.URL.Path, "/instantiate") && r.Method == http.MethodPost:
		definitionID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategy-definitions/", "/instantiate"))
		if err != nil || strings.TrimSpace(definitionID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "definitionId is invalid")
			return true
		}
		definition, ok := s.designStore.definition(definitionID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
			return true
		}
		if err := strategydefinition.ValidateScript(definition.SourceFormat, definition.Script); err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return true
		}
		if !strategydefinition.SupportsInstantiation(definition.SourceFormat) {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("strategy source format %s is not instantiable yet", strategydefinition.NormalizeSourceFormat(definition.SourceFormat)))
			return true
		}
		var payload strategyInstanceBinding
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy instance payload")
			return true
		}
		instance, err := s.strategyStore.instantiateStrategy(definition, payload)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to instantiate strategy")
			return true
		}
		s.writeOK(w, s.enrichStrategyItem(instance))
	case r.URL.Path == "/api/v1/strategies" && r.Method == http.MethodGet:
		s.writeOK(w, s.enrichStrategyItems(s.strategyStore.strategies()))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && r.Method == http.MethodPut:
		instanceID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/strategies/"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		var payload strategyInstanceBinding
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy instance payload")
			return true
		}
		instance, err := s.strategyStore.updateStrategyBinding(instanceID, payload)
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
				return true
			}
			if err == errStrategyInstanceBusy {
				s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "strategy instance must be stopped before updating bindings")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update strategy instance")
			return true
		}
		s.writeOK(w, s.enrichStrategyItem(instance))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && r.Method == http.MethodDelete:
		instanceID, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/strategies/"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		instance, err := s.strategyStore.deleteStrategy(instanceID)
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
				return true
			}
			if err == errStrategyInstanceBusy {
				s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "strategy instance must be stopped before deletion")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete strategy instance")
			return true
		}
		s.writeOK(w, s.enrichStrategyItem(instance))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/start") && r.Method == http.MethodPost:
		instanceID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategies/", "/start"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		instanceRecord, ok := s.strategyStore.strategy(instanceID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return true
		}
		if !strategyInstanceStartable(instanceRecord) {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("strategy runtime %s is not startable yet", strategyRuntimeFromParams(instanceRecord.Params)))
			return true
		}
		if err := s.strategyRuntimeManager.startStrategy(r.Context(), instanceRecord); err != nil {
			status, code := strategyRuntimeStartError(err)
			s.writeError(w, status, code, err.Error())
			return true
		}
		instance, err := s.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "strategy runtime requested start")
		if err != nil {
			s.strategyRuntimeManager.stopStrategy(instanceID)
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to start strategy")
			return true
		}
		s.ensureLiveMarketStream(context.Background(), s.activeLiveStreamInstrumentIDs())
		s.writeOK(w, s.enrichStrategyItem(instance))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/refresh-definition") && r.Method == http.MethodPost:
		instanceID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategies/", "/refresh-definition"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		instanceRecord, ok := s.strategyStore.strategy(instanceID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return true
		}
		definitionID := strings.TrimSpace(instanceRecord.Definition.StrategyID)
		if definitionID == "" {
			definitionID = strategyDefinitionIDFromParams(instanceRecord.Params)
		}
		if definitionID == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "strategy instance is not linked to a saved definition")
			return true
		}
		definition, ok := s.designStore.definition(definitionID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
			return true
		}
		instance, refreshErr := s.strategyStore.refreshStrategyDefinition(instanceID, definition)
		if refreshErr != nil {
			if errorsIsNotFound(refreshErr) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
				return true
			}
			if refreshErr == errStrategyInstanceBusy {
				s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "strategy instance must be stopped before refreshing definition")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to refresh strategy definition")
			return true
		}
		s.writeOK(w, s.enrichStrategyItem(instance))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/pause") && r.Method == http.MethodPost:
		instanceID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategies/", "/pause"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		instance, err := s.strategyStore.transitionStrategy(instanceID, strategyStatusPaused, "paused", "manual pause")
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to pause strategy")
			return true
		}
		s.strategyRuntimeManager.stopStrategy(instanceID)
		s.ensureLiveMarketStream(context.Background(), s.activeLiveStreamInstrumentIDs())
		s.writeOK(w, s.enrichStrategyItem(instance))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/stop") && r.Method == http.MethodPost:
		instanceID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategies/", "/stop"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		instance, err := s.strategyStore.transitionStrategy(instanceID, strategyStatusStopped, "stopped", "manual stop")
		if err != nil {
			if errorsIsNotFound(err) {
				s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
				return true
			}
			s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to stop strategy")
			return true
		}
		s.strategyRuntimeManager.stopStrategy(instanceID)
		s.ensureLiveMarketStream(context.Background(), s.activeLiveStreamInstrumentIDs())
		s.writeOK(w, s.enrichStrategyItem(instance))
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/logs") && r.Method == http.MethodGet:
		instanceID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategies/", "/logs"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		logs, ok := s.strategyStore.strategyLogsPage(instanceID, strategyRuntimeLogQuery{
			Limit:  intQuery(r.URL.Query(), "limit", 500),
			Offset: intQuery(r.URL.Query(), "offset", 0),
			Level:  firstQuery(r.URL.Query(), "level", ""),
			FromAt: strategyActivityQueryTime(r.URL.Query(), "fromTime"),
			ToAt:   strategyActivityQueryTime(r.URL.Query(), "toTime"),
		})
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return true
		}
		s.writeOK(w, logs)
	case strings.HasPrefix(r.URL.Path, "/api/v1/strategies/") && strings.HasSuffix(r.URL.Path, "/audit") && r.Method == http.MethodGet:
		instanceID, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/strategies/", "/audit"))
		if err != nil || strings.TrimSpace(instanceID) == "" {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "instanceId is invalid")
			return true
		}
		audit, ok := s.strategyStore.strategyAuditPage(instanceID, strategyRuntimeAuditQuery{
			Limit:  intQuery(r.URL.Query(), "limit", 500),
			Offset: intQuery(r.URL.Query(), "offset", 0),
			Kind:   firstQuery(r.URL.Query(), "kind", ""),
			FromAt: strategyActivityQueryTime(r.URL.Query(), "fromTime"),
			ToAt:   strategyActivityQueryTime(r.URL.Query(), "toTime"),
		})
		if !ok {
			s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy instance not found")
			return true
		}
		s.writeOK(w, audit)
	default:
		return false
	}
	return true
}

func strategyActivityQueryTime(query map[string][]string, key string) *time.Time {
	raw := strings.TrimSpace(firstQuery(query, key, ""))
	if raw == "" {
		return nil
	}
	parsed := parseQueryTime(raw, time.Time{})
	if parsed.IsZero() {
		return nil
	}
	result := parsed.UTC()
	return &result
}
