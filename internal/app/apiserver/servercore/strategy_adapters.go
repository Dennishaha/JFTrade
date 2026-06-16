package servercore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

// ──────────────────────────────────────────────────────────────────────────────
// strategyDesignStoreAdapter 适配 strategyDesignStore → stratsrv.DesignStore
// ──────────────────────────────────────────────────────────────────────────────

type strategyDesignStoreAdapter struct {
	store *strategyDesignStore
}

func (a *strategyDesignStoreAdapter) ListDefinitions() []stratsrv.Definition {
	return a.store.listDefinitions()
}

func (a *strategyDesignStoreAdapter) GetDefinition(id string) (stratsrv.Definition, bool, error) {
	return a.store.definition(id)
}

func (a *strategyDesignStoreAdapter) SaveDefinition(input stratsrv.Definition) (stratsrv.Definition, error) {
	if err := strategydefinition.ValidateScript(input.SourceFormat, input.Script); err != nil {
		return stratsrv.Definition{}, stratsrv.BadRequestError(err.Error())
	}
	result, err := a.store.saveDefinition(input)
	if err != nil {
		if errors.Is(err, errUnsupportedLegacyStrategyDefinition) {
			return stratsrv.Definition{}, stratsrv.BadRequestError(err.Error())
		}
		return stratsrv.Definition{}, err
	}
	return result, nil
}

func (a *strategyDesignStoreAdapter) DeleteDefinition(id string) (stratsrv.Definition, error) {
	result, err := a.store.deleteDefinition(id)
	if err != nil {
		return stratsrv.Definition{}, mapStrategyStoreError(err)
	}
	return result, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// strategyCatalogStoreAdapter 适配 strategyCatalogStore → stratsrv.CatalogStore
// ──────────────────────────────────────────────────────────────────────────────

type strategyCatalogStoreAdapter struct {
	store       *strategyCatalogStore
	designStore *strategyDesignStore
	runtimeMgr  *strategyRuntimeManager
}

// ── 实例 CRUD ──

func (a *strategyCatalogStoreAdapter) ListInstances() []stratsrv.InstanceView {
	items := a.store.strategies()
	return a.enrichItems(items)
}

func (a *strategyCatalogStoreAdapter) GetInstance(id string) (stratsrv.ManagedInstance, bool) {
	return a.store.strategy(id)
}

func (a *strategyCatalogStoreAdapter) ValidateStartable(instance stratsrv.ManagedInstance) error {
	if !strategyInstanceStartable(instance) {
		return stratsrv.BadRequestError(fmt.Sprintf("strategy runtime %s is not startable yet", strategyRuntimeFromParams(instance.Params)))
	}
	return nil
}

func (a *strategyCatalogStoreAdapter) CreateInstance(def stratsrv.Definition, binding stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	item, err := a.store.instantiateStrategy(def, binding)
	if err != nil {
		return stratsrv.InstanceView{}, mapStrategyStoreError(err)
	}
	return a.enrichItem(item), nil
}

func (a *strategyCatalogStoreAdapter) UpdateInstance(id string, binding stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	item, err := a.store.updateStrategyBinding(id, binding)
	if err != nil {
		return stratsrv.InstanceView{}, mapStrategyStoreError(err)
	}
	return a.enrichItem(item), nil
}

func (a *strategyCatalogStoreAdapter) DeleteInstance(id string) (stratsrv.InstanceView, error) {
	item, err := a.store.deleteStrategy(id)
	if err != nil {
		return stratsrv.InstanceView{}, mapStrategyStoreError(err)
	}
	return a.enrichItem(item), nil
}

func (a *strategyCatalogStoreAdapter) TransitionInstance(id string, status string) (stratsrv.InstanceView, error) {
	kind := statusToKind(status)
	detail := statusToDetail(status)
	item, err := a.store.transitionStrategy(id, status, kind, detail)
	if err != nil {
		return stratsrv.InstanceView{}, mapStrategyStoreError(err)
	}
	return a.enrichItem(item), nil
}

func (a *strategyCatalogStoreAdapter) RefreshDefinition(id string, def stratsrv.Definition) (stratsrv.InstanceView, error) {
	item, err := a.store.refreshStrategyDefinition(id, def)
	if err != nil {
		return stratsrv.InstanceView{}, mapStrategyStoreError(err)
	}
	return a.enrichItem(item), nil
}

// RefreshInstanceDefinition 查找实例关联的策略定义并刷新到最新版本。
// 适配器层负责跨 store 编排。
func (a *strategyCatalogStoreAdapter) RefreshInstanceDefinition(instanceID string) (stratsrv.InstanceView, error) {
	instance, ok := a.store.strategy(instanceID)
	if !ok {
		return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy instance not found")
	}
	definitionID := strings.TrimSpace(instance.Definition.StrategyID)
	if definitionID == "" {
		definitionID = strategyDefinitionIDFromParams(instance.Params)
	}
	if definitionID == "" {
		return stratsrv.InstanceView{}, stratsrv.BadRequestError("strategy instance is not linked to a saved definition")
	}
	def, exists, err := a.designStore.definition(definitionID)
	if err != nil {
		return stratsrv.InstanceView{}, fmt.Errorf("get definition %s: %w", definitionID, err)
	}
	if !exists {
		return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy definition not found")
	}
	item, err := a.store.refreshStrategyDefinition(instanceID, def)
	if err != nil {
		return stratsrv.InstanceView{}, mapStrategyStoreError(err)
	}
	return a.enrichItem(item), nil
}

func (a *strategyCatalogStoreAdapter) ApplyDefinitionToLinked(def stratsrv.Definition) (stratsrv.ApplyLinkedInstancesResult, error) {
	return a.store.applyDefinitionToLinkedStrategies(def)
}

func (a *strategyCatalogStoreAdapter) GetLinkedInstanceIDs(definitionID string) []string {
	return a.store.linkedStrategyInstanceIDs(definitionID)
}

// ── 活动日志/审计 ──

func (a *strategyCatalogStoreAdapter) GetLogs(id string, query stratsrv.LogQuery) (stratsrv.LogsResult, bool) {
	return a.store.strategyLogsPage(id, strategyRuntimeLogQuery{
		InstanceID: id,
		Limit:      query.Limit,
		Offset:     query.Offset,
		Level:      query.Level,
		FromAt:     query.FromAt,
		ToAt:       query.ToAt,
	})
}

func (a *strategyCatalogStoreAdapter) GetAudit(id string, query stratsrv.AuditQuery) (stratsrv.AuditResult, bool) {
	return a.store.strategyAuditPage(id, strategyRuntimeAuditQuery{
		InstanceID: id,
		Limit:      query.Limit,
		Offset:     query.Offset,
		Kind:       query.Kind,
		FromAt:     query.FromAt,
		ToAt:       query.ToAt,
	})
}

// ── 生命周期 ──

func (a *strategyCatalogStoreAdapter) ReconcileOnStartup() (int, error) {
	return a.store.reconcileRuntimeStatesOnStartup()
}

func (a *strategyCatalogStoreAdapter) PluginCatalog() stratsrv.PluginCatalog {
	return a.store.pluginCatalog()
}

func (a *strategyCatalogStoreAdapter) PluginOperation(id string) (stratsrv.PluginOperation, bool) {
	return a.store.pluginOperation(id)
}

func (a *strategyCatalogStoreAdapter) PluginUninstallGuidance(id string) (stratsrv.PluginUninstallGuidance, bool) {
	return a.store.pluginUninstallGuidance(id)
}

func (a *strategyCatalogStoreAdapter) InstallPlugin(id string) (stratsrv.PluginOperation, error) {
	return a.store.installPlugin(id)
}

func (a *strategyCatalogStoreAdapter) UninstallPlugin(id string) (stratsrv.PluginOperation, error) {
	return a.store.uninstallPlugin(id)
}

func (a *strategyCatalogStoreAdapter) Close() error {
	return a.store.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// strategyRuntimeManagerAdapter 适配 strategyRuntimeManager → stratsrv.RuntimeManager
// ──────────────────────────────────────────────────────────────────────────────

type strategyRuntimeManagerAdapter struct {
	mgr *strategyRuntimeManager
}

func (a *strategyRuntimeManagerAdapter) Start(ctx context.Context, instance stratsrv.ManagedInstance) error {
	if err := a.mgr.startStrategy(ctx, instance); err != nil {
		status, _ := strategyRuntimeStartError(err)
		if status == 400 {
			return stratsrv.BadRequestError(err.Error())
		}
		return stratsrv.UpstreamError(err.Error())
	}
	return nil
}

func (a *strategyRuntimeManagerAdapter) Stop(instanceID string) {
	a.mgr.stopStrategy(instanceID)
}

func (a *strategyRuntimeManagerAdapter) GetObservation(id string) (stratsrv.RuntimeObservation, bool) {
	return a.mgr.runtimeObservation(id)
}

func (a *strategyRuntimeManagerAdapter) RuntimeSummary() stratsrv.RuntimeSummary {
	return a.mgr.typedRuntimeSummary()
}

func (a *strategyRuntimeManagerAdapter) ActiveInstrumentIDs() []string {
	return a.mgr.activeInstrumentIDs()
}

func mapStrategyStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return stratsrv.NotFoundError("strategy resource not found")
	case errors.Is(err, errStrategyInstanceBusy):
		return stratsrv.BusyError("strategy instance must be stopped before modification")
	case errors.Is(err, errUnsupportedLegacyStrategyDefinition):
		return stratsrv.BadRequestError(err.Error())
	default:
		return err
	}
}

func statusToKind(status string) string {
	switch status {
	case strategyStatusRunning:
		return "started"
	case strategyStatusPaused:
		return "paused"
	case strategyStatusStopped:
		return "stopped"
	default:
		return status
	}
}

func statusToDetail(status string) string {
	switch status {
	case strategyStatusRunning:
		return "strategy runtime requested start"
	case strategyStatusPaused:
		return "manual pause"
	case strategyStatusStopped:
		return "manual stop"
	default:
		return "status transition"
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Strategy instance enrichment.
// ──────────────────────────────────────────────────────────────────────────────

func (a *strategyCatalogStoreAdapter) enrichItems(items []strategyListItem) []strategyListItem {
	if len(items) == 0 {
		return items
	}
	enriched := make([]strategyListItem, len(items))
	for i := range items {
		enriched[i] = a.enrichItem(items[i])
	}
	return enriched
}

func (a *strategyCatalogStoreAdapter) enrichItem(item strategyListItem) strategyListItem {
	// DefinitionSync
	if sync := a.buildDefinitionSyncStatus(item); sync != nil {
		item.DefinitionSync = sync
	}
	// Runtime observation from manager (prefer live over persisted)
	if a.runtimeMgr != nil {
		if observation, ok := a.runtimeMgr.runtimeObservation(item.ID); ok {
			item.RuntimeObservation = &observation
		}
	}
	// Fallback: persisted observation from runtime store (only when no live observation)
	if item.RuntimeObservation == nil && a.store != nil && a.store.runtimeStore != nil {
		snapshot, ok, err := a.store.runtimeStore.GetObservation(context.Background(), item.ID)
		if err != nil {
			log.Printf("JFTrade load persisted strategy runtime observation degraded: %v", err)
		} else if ok {
			item.RuntimeObservation = new(strategyRuntimeObservationFromSnapshot(snapshot, item.Status))
		}
	}
	// Persisted log tail for strategy list cards.
	if a.store != nil && a.store.runtimeStore != nil {
		persistedLogs, err := a.store.runtimeStore.ListRecentLogsTail(context.Background(), item.ID, strategyListLogsTailSize)
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
	return item
}

func (a *strategyCatalogStoreAdapter) buildDefinitionSyncStatus(item strategyListItem) *strategyDefinitionSyncStatus {
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
	if a.designStore == nil {
		return status
	}
	definition, ok, err := a.designStore.definition(definitionID)
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
		status.BlockedReason = new("当前实例不是 STOPPED，先停止后才能刷新到最新策略。")
	}
	return status
}
