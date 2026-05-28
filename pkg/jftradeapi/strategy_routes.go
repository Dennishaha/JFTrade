package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		s.writeOK(w, buildStrategyDefinitionResponse(definition, r.URL.Query().Get("interval")))
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
		logs, ok := s.strategyStore.strategyLogs(instanceID)
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
		audit, ok := s.strategyStore.strategyAudit(instanceID)
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
