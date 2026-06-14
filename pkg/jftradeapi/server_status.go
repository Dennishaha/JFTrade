package jftradeapi

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/buildinfo"
)

func (s *Server) systemStatus() map[string]any {
	return map[string]any{
		"name":                      "JFTrade",
		"apiPort":                   s.apiPort,
		"defaultBroker":             "futu",
		"defaultTradingEnvironment": s.defaultTradingEnvironment(),
		"realTradingEnabled":        false,
		"realTradingKillSwitch": map[string]any{
			"active": false, "envConfiguredActive": false, "controlPlaneActive": false,
			"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true,
		},
		"realTradingRisk": map[string]any{
			"enabled": false, "maxOrderQuantity": nil, "maxOrderNotional": nil,
			"envConfiguredMaxOrderQuantity": nil, "envConfiguredMaxOrderNotional": nil,
			"controlPlaneActive": false, "controlPlaneMaxOrderQuantity": nil, "controlPlaneMaxOrderNotional": nil,
			"riskConfigSource": nil,
		},
		"realTradeAccess": map[string]any{"approverAllowlistEnabled": false, "approverCount": 0, "adminAllowlistEnabled": false, "adminCount": 0},
		"broker":          s.descriptor(),
		"build":           buildinfo.Snapshot(),
		"persistence": map[string]any{
			"engine": "json", "databasePath": s.store.path, "status": "ok", "migrated": true,
			"pendingMigrations": []any{}, "tables": []string{"broker_integrations", "broker_accounts"}, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"strategyRuntime": s.strategyRuntimeSummary(),
		"message":         "JFTrade API adapter is running.",
	}
}

func (s *Server) strategyRuntimeSummary() map[string]any {
	if s.strategyRuntimeManager == nil {
		return map[string]any{
			"status":                 "idle",
			"activeStrategies":       0,
			"supportsBacktestParity": true,
			"activeInstances":        []strategyRuntimeActiveInstanceSummary{},
		}
	}
	return s.strategyRuntimeManager.runtimeSummary()
}

func (s *Server) enrichStrategyItem(item strategyListItem) strategyListItem {
	if sync := s.buildStrategyDefinitionSyncStatus(item); sync != nil {
		item.DefinitionSync = sync
	}
	if s.strategyRuntimeStore != nil {
		persistedLogs, err := s.strategyRuntimeStore.ListRecentLogsTail(context.Background(), item.ID, strategyListLogsTailSize)
		if err != nil {
			log.Printf("JFTrade load persisted strategy list logs degraded: %v", err)
		} else if len(persistedLogs) > 0 {
			logs := make([]string, 0, len(persistedLogs))
			for _, event := range persistedLogs {
				logs = append(logs, event.Raw)
			}
			item.Logs = logs
		}
	}
	if s.strategyRuntimeManager != nil {
		if observation, ok := s.strategyRuntimeManager.runtimeObservation(item.ID); ok {
			item.RuntimeObservation = &observation
			return item
		}
	}
	if s.strategyRuntimeStore != nil {
		snapshot, ok, err := s.strategyRuntimeStore.GetObservation(context.Background(), item.ID)
		if err != nil {
			log.Printf("JFTrade load persisted strategy runtime observation degraded: %v", err)
			return item
		}
		if ok {
			observation := strategyRuntimeObservationFromSnapshot(snapshot, item.Status)
			item.RuntimeObservation = &observation
		}
	}
	return item
}

func (s *Server) buildStrategyDefinitionSyncStatus(item strategyListItem) *strategyDefinitionSyncStatus {
	definitionID := strings.TrimSpace(item.Definition.StrategyID)
	if definitionID == "" {
		definitionID = strategyDefinitionIDFromParams(item.Params)
	}
	if definitionID == "" {
		return nil
	}
	appliedVersion := strings.TrimSpace(item.Definition.Version)
	status := &strategyDefinitionSyncStatus{
		DefinitionID:   definitionID,
		AppliedVersion: appliedVersion,
		LatestVersion:  appliedVersion,
		IsLatest:       true,
	}
	if s == nil || s.designStore == nil {
		return status
	}
	definition, ok, err := s.designStore.definition(definitionID)
	if err != nil {
		return status
	}
	if !ok {
		return status
	}
	status.LatestVersion = strings.TrimSpace(definition.Version)
	status.IsLatest = status.AppliedVersion == status.LatestVersion
	if status.IsLatest {
		return status
	}
	status.CanApplyLatest = item.Status == strategyStatusStopped
	if !status.CanApplyLatest {
		reason := "当前实例不是 STOPPED，先停止后才能刷新到最新策略。"
		status.BlockedReason = &reason
	}
	return status
}

func (s *Server) enrichStrategyItems(items []strategyListItem) []strategyListItem {
	if len(items) == 0 {
		return items
	}
	enriched := make([]strategyListItem, len(items))
	for index := range items {
		enriched[index] = s.enrichStrategyItem(items[index])
	}
	return enriched
}

func (s *Server) emptyConnectivityList(key string, value any, extraKeys ...string) map[string]any {
	result := map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "disconnected",
		"lastError":    nil,
		key:            value,
	}
	for _, extraKey := range extraKeys {
		result[extraKey] = []any{}
	}
	return result
}

func (s *Server) realTradeApprovals() map[string]any {
	return map[string]any{
		"realTradingEnabled":       false,
		"requiredConfirmationText": "ENABLE_REAL_TRADING",
		"maxApprovalAgeMs":         5 * 60 * 1000,
		"approvalPolicy":           map[string]any{"approverAllowlistEnabled": false, "approverCount": 0},
		"entries":                  []any{},
	}
}

func (s *Server) realTradeKillSwitch() map[string]any {
	return map[string]any{
		"realTradingEnabled": false, "envConfiguredActive": false, "controlPlaneActive": false,
		"killSwitchActive": false, "killSwitchSource": nil, "blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true, "entry": nil,
	}
}

func (s *Server) realTradeRiskState() map[string]any {
	return map[string]any{
		"realTradingEnabled": false, "riskEnabled": false, "riskConfigSource": nil,
		"envConfiguredMaxOrderQuantity": nil, "envConfiguredMaxOrderNotional": nil,
		"controlPlaneActive": false, "controlPlaneMaxOrderQuantity": nil, "controlPlaneMaxOrderNotional": nil,
		"effectiveMaxOrderQuantity": nil, "effectiveMaxOrderNotional": nil, "entry": nil,
	}
}

func (s *Server) realTradeRiskEvents() map[string]any {
	result := s.realTradeRiskState()
	result["maxOrderQuantity"] = nil
	result["maxOrderNotional"] = nil
	result["entries"] = []any{}
	delete(result, "entry")
	return result
}

func stringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
