package jftradeapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

func NewStrategyCatalogStore(path string, targetDir string) (*strategyCatalogStore, error) {
	runtimeStore, err := NewStrategyRuntimeStore(deriveStrategyCatalogDBPath(path))
	if err != nil {
		return nil, err
	}
	store := &strategyCatalogStore{path: path, dbPath: deriveStrategyCatalogDBPath(path), db: runtimeStore.DB(), targetDir: strings.TrimSpace(targetDir), runtimeStore: runtimeStore}
	if store.targetDir == "" {
		store.targetDir = defaultStrategyPluginDirName
	}
	if err := store.load(); err != nil {
		_ = runtimeStore.Close()
		return nil, err
	}
	return store, nil
}

func deriveStrategyCatalogPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyCatalogFilename
	}
	return filepath.Join(directory, defaultStrategyCatalogFilename)
}

func deriveStrategyPluginTargetDir(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyPluginDirName
	}
	return filepath.Join(directory, defaultStrategyPluginDirName)
}

func deriveStrategyCatalogDBPath(catalogPath string) string {
	return deriveStrategyRuntimeDBPath(catalogPath)
}

func (s *strategyCatalogStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.migrateLocked(); err != nil {
		return err
	}
	s.data = strategyCatalogFile{TargetDir: s.targetDir, Plugins: []managedStrategyPlugin{}, Strategies: []managedStrategyInstance{}, Operations: []strategyPluginOperation{}}
	targetDir, err := s.loadCatalogMetaLocked("target_dir")
	if err != nil {
		return err
	}
	if strings.TrimSpace(targetDir) != "" {
		s.data.TargetDir = strings.TrimSpace(targetDir)
	}

	pluginRows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.Select(&pluginRows, `SELECT payload_json FROM `+strategyCatalogPluginTable+` ORDER BY id ASC`); err != nil {
		return err
	}
	strategyRows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.Select(&strategyRows, `SELECT payload_json FROM `+strategyCatalogStrategyTable+` ORDER BY created_at ASC, id ASC`); err != nil {
		return err
	}
	operationRows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.db.Select(&operationRows, `SELECT payload_json FROM `+strategyCatalogOperationTable+` ORDER BY updated_at DESC, operation_id ASC`); err != nil {
		return err
	}

	migrated := false
	if strings.TrimSpace(s.data.TargetDir) == "" {
		s.data.TargetDir = s.targetDir
		migrated = true
	}
	for _, row := range pluginRows {
		var plugin managedStrategyPlugin
		if err := json.Unmarshal([]byte(row.PayloadJSON), &plugin); err != nil {
			return err
		}
		normalized := s.normalizePlugin(plugin)
		if !reflect.DeepEqual(plugin, normalized) {
			migrated = true
		}
		s.data.Plugins = append(s.data.Plugins, normalized)
	}
	for _, row := range strategyRows {
		var strategy managedStrategyInstance
		if err := json.Unmarshal([]byte(row.PayloadJSON), &strategy); err != nil {
			return err
		}
		normalized := s.normalizeStrategy(strategy)
		if !reflect.DeepEqual(strategy, normalized) {
			migrated = true
		}
		normalized.Logs = nil
		normalized.AuditEntries = nil
		s.data.Strategies = append(s.data.Strategies, normalized)
	}
	for _, row := range operationRows {
		var operation strategyPluginOperation
		if err := json.Unmarshal([]byte(row.PayloadJSON), &operation); err != nil {
			return err
		}
		s.data.Operations = append(s.data.Operations, operation)
	}
	if migrated {
		return s.persistLocked()
	}
	return nil
}

func (s *strategyCatalogStore) migrateLocked() error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS ` + strategyCatalogMetaTable + ` (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS ` + strategyCatalogPluginTable + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS ` + strategyCatalogStrategyTable + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS ` + strategyCatalogOperationTable + ` (operation_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_strategy_catalog_strategies_created_at ON ` + strategyCatalogStrategyTable + ` (created_at ASC, id ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_strategy_catalog_operations_updated_at ON ` + strategyCatalogOperationTable + ` (updated_at DESC, operation_id ASC)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *strategyCatalogStore) loadCatalogMetaLocked(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	var value string
	if err := s.db.Get(&value, `SELECT value FROM `+strategyCatalogMetaTable+` WHERE key = ?`, key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (s *strategyCatalogStore) savePlugin(input managedStrategyPlugin) error {
	input = s.normalizePlugin(input)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Plugins {
		if s.data.Plugins[index].Descriptor.ID != input.Descriptor.ID {
			continue
		}
		s.data.Plugins[index] = input
		return s.persistLocked()
	}
	s.data.Plugins = append(s.data.Plugins, input)
	return s.persistLocked()
}

func (s *strategyCatalogStore) saveStrategy(input managedStrategyInstance) error {
	input = s.normalizeStrategy(input)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		if s.data.Strategies[index].ID != input.ID {
			continue
		}
		s.data.Strategies[index] = input
		return s.persistLocked()
	}
	s.data.Strategies = append(s.data.Strategies, input)
	return s.persistLocked()
}

func (s *strategyCatalogStore) pluginCatalog() strategyPluginCatalogResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	plugins := make([]strategyPluginCatalogItem, 0, len(s.data.Plugins))
	for _, plugin := range s.data.Plugins {
		normalized := s.normalizePlugin(plugin)
		plugins = append(plugins, strategyPluginCatalogItem{
			Descriptor:    normalized.Descriptor,
			Installation:  normalized.Installation,
			Compatibility: buildPluginCompatibility(normalized.Artifact),
		})
	}
	sort.Slice(plugins, func(i int, j int) bool {
		return plugins[i].Descriptor.ID < plugins[j].Descriptor.ID
	})

	return strategyPluginCatalogResponse{
		TargetDir: s.effectiveTargetDirLocked(),
		Plugins:   plugins,
	}

}

func (s *strategyCatalogStore) installPlugin(pluginID string) (strategyPluginOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for index := range s.data.Plugins {
		plugin := s.normalizePlugin(s.data.Plugins[index])
		if plugin.Descriptor.ID != pluginID {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		completedAt := now
		operation := strategyPluginOperation{
			OperationID: buildPluginOperationID(pluginID),
			PluginID:    pluginID,
			Status:      "SUCCEEDED",
			Phase:       "installed",
			Progress:    100,
			Message:     "plugin metadata installed",
			TargetDir:   plugin.Installation.TargetDir,
			InstallPath: plugin.Installation.InstallPath,
			StartedAt:   now,
			UpdatedAt:   now,
			CompletedAt: &completedAt,
		}
		plugin.Installation.Status = "INSTALLED"
		plugin.Installation.Installed = true
		plugin.Installation.CurrentOperation = nil
		plugin.Installation.LastOperation = &operation
		plugin.Installation.UninstallGuidance = buildPluginUninstallGuidance(plugin.Descriptor.ID, plugin.Installation.InstallPath)
		s.data.Plugins[index] = plugin
		s.data.Operations = append(s.data.Operations, operation)
		return operation, s.persistLocked()
	}

	return strategyPluginOperation{}, os.ErrNotExist
}

func (s *strategyCatalogStore) uninstallPlugin(pluginID string) (strategyPluginOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for index := range s.data.Plugins {
		plugin := s.normalizePlugin(s.data.Plugins[index])
		if plugin.Descriptor.ID != pluginID {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		completedAt := now
		operation := strategyPluginOperation{
			OperationID: buildPluginOperationID(pluginID),
			PluginID:    pluginID,
			Status:      "SUCCEEDED",
			Phase:       "uninstalled",
			Progress:    100,
			Message:     "plugin metadata uninstalled",
			TargetDir:   plugin.Installation.TargetDir,
			InstallPath: plugin.Installation.InstallPath,
			StartedAt:   now,
			UpdatedAt:   now,
			CompletedAt: &completedAt,
		}
		plugin.Installation.Status = "NOT_INSTALLED"
		plugin.Installation.Installed = false
		plugin.Installation.CurrentOperation = nil
		plugin.Installation.LastOperation = &operation
		plugin.Installation.UninstallGuidance = buildPluginUninstallGuidance(plugin.Descriptor.ID, plugin.Installation.InstallPath)
		s.data.Plugins[index] = plugin
		s.data.Operations = append(s.data.Operations, operation)
		return operation, s.persistLocked()
	}

	return strategyPluginOperation{}, os.ErrNotExist
}

func (s *strategyCatalogStore) pluginOperation(operationID string) (strategyPluginOperation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, operation := range s.data.Operations {
		if operation.OperationID == operationID {
			return operation, true
		}
	}
	return strategyPluginOperation{}, false
}

func (s *strategyCatalogStore) pluginUninstallGuidance(pluginID string) (strategyPluginUninstallGuidance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, plugin := range s.data.Plugins {
		normalized := s.normalizePlugin(plugin)
		if normalized.Descriptor.ID == pluginID {
			return buildPluginUninstallGuidance(pluginID, normalized.Installation.InstallPath), true
		}
	}
	return strategyPluginUninstallGuidance{}, false
}

func (s *strategyCatalogStore) strategies() []strategyListItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]strategyListItem, 0, len(s.data.Strategies))
	for _, strategy := range s.data.Strategies {
		normalized := s.normalizeStrategy(strategy)
		items = append(items, strategyToListItem(normalized))
	}
	sort.Slice(items, func(i int, j int) bool {
		return items[i].CreatedAt < items[j].CreatedAt
	})
	return items
}

func (s *strategyCatalogStore) linkedStrategyInstanceIDs(definitionID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return []string{}
	}
	linked := make([]string, 0)
	for _, strategy := range s.data.Strategies {
		normalized := s.normalizeStrategy(strategy)
		if !strategyInstanceUsesDefinition(normalized, definitionID) {
			continue
		}
		linked = append(linked, normalized.ID)
	}
	sort.Strings(linked)
	return linked
}

func (s *strategyCatalogStore) instantiateStrategy(definition strategyDesignDefinition, binding strategyInstanceBinding) (strategyListItem, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	params, err := buildStrategyInstanceParams(definition, now)
	if err != nil {
		return strategyListItem{}, err
	}
	binding = normalizeStrategyInstanceBinding(binding, params)
	instance := managedStrategyInstance{
		ID:       buildStrategyInstanceID(definition.ID),
		PluginID: strategyPluginIDForDefinition(definition),
		Definition: strategyDefinitionSummary{
			StrategyID: definition.ID,
			Name:       definition.Name,
			Version:    definition.Version,
		},
		Binding:   binding,
		Params:    params,
		Status:    strategyStatusStopped,
		CreatedAt: now,
	}
	s.recordStrategyEventsLocked(&instance, time.Now().UTC(), fmt.Sprintf("instantiated strategy from definition %s", definition.ID), "info", "control", "instantiated", strategyBindingAuditDetail(definition.ID, binding))
	if err := s.saveStrategy(instance); err != nil {
		return strategyListItem{}, err
	}
	stored, ok := s.strategy(instance.ID)
	if !ok {
		return strategyListItem{}, os.ErrNotExist
	}
	return strategyToListItem(stored), nil
}

func (s *strategyCatalogStore) updateStrategyBinding(instanceID string, binding strategyInstanceBinding) (strategyListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strategy.Status != strategyStatusStopped {
			return strategyListItem{}, errStrategyInstanceBusy
		}
		strategy.Binding = normalizeStrategyInstanceBinding(binding, strategy.Params)
		applyStrategyBindingParams(&strategy)
		s.recordStrategyEventsLocked(&strategy, time.Now().UTC(), "updated strategy binding", "info", "control", "binding.updated", strategyBindingAuditDetail(strategy.Definition.StrategyID, strategy.Binding))
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) refreshStrategyDefinition(instanceID string, definition strategyDesignDefinition) (strategyListItem, error) {
	now := time.Now().UTC()
	params, err := buildStrategyInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return strategyListItem{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		changed, refreshErr := s.refreshStrategyDefinitionLocked(&strategy, definition, params, now)
		if refreshErr != nil {
			return strategyListItem{}, refreshErr
		}
		if changed {
			s.data.Strategies[index] = strategy
			if err := s.persistLocked(); err != nil {
				return strategyListItem{}, err
			}
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) applyDefinitionToLinkedStrategies(definition strategyDesignDefinition) (strategyApplyLinkedInstancesResponse, error) {
	now := time.Now().UTC()
	params, err := buildStrategyInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return strategyApplyLinkedInstancesResponse{}, err
	}

	result := strategyApplyLinkedInstancesResponse{
		DefinitionID:  strings.TrimSpace(definition.ID),
		LatestVersion: strings.TrimSpace(definition.Version),
		Applied:       []string{},
		AlreadyLatest: []string{},
		SkippedBusy:   []string{},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	persistChanged := false
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if !strategyInstanceUsesDefinition(strategy, definition.ID) {
			continue
		}
		result.TotalLinked++
		if strategy.Status != strategyStatusStopped {
			result.SkippedBusy = append(result.SkippedBusy, strategy.ID)
			continue
		}
		if strings.TrimSpace(strategy.Definition.Version) == strings.TrimSpace(definition.Version) {
			result.AlreadyLatest = append(result.AlreadyLatest, strategy.ID)
			continue
		}
		changed, refreshErr := s.refreshStrategyDefinitionLocked(&strategy, definition, params, now)
		if refreshErr != nil {
			return strategyApplyLinkedInstancesResponse{}, refreshErr
		}
		if !changed {
			result.AlreadyLatest = append(result.AlreadyLatest, strategy.ID)
			continue
		}
		s.data.Strategies[index] = strategy
		persistChanged = true
		result.Applied = append(result.Applied, strategy.ID)
	}
	if persistChanged {
		if err := s.persistLocked(); err != nil {
			return strategyApplyLinkedInstancesResponse{}, err
		}
	}
	return result, nil
}

func (s *strategyCatalogStore) deleteStrategy(instanceID string) (strategyListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strategy.Status != strategyStatusStopped {
			return strategyListItem{}, errStrategyInstanceBusy
		}
		removed := strategyToListItem(strategy)
		s.data.Strategies = append(s.data.Strategies[:index], s.data.Strategies[index+1:]...)
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return removed, nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) transitionStrategy(instanceID string, nextStatus string, kind string, detail string) (strategyListItem, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		strategy.Status = nextStatus
		s.recordStrategyEventsLocked(&strategy, now, fmt.Sprintf("%s strategy %s", strings.ToLower(kind), strategy.Definition.StrategyID), strategyLogLevelForKind(kind, detail), "control", kind, detail)
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) appendStrategyRuntimeEvent(instanceID string, logMessage string, kind string, detail string) error {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		legacyWritten := s.recordStrategyEventsLocked(&strategy, now, logMessage, strategyLogLevelForKind(kind, logMessage), "runtime", kind, detail)
		if legacyWritten {
			s.data.Strategies[index] = strategy
			return s.persistLocked()
		}
		return nil
	}

	return os.ErrNotExist
}

func (s *strategyCatalogStore) reconcileStrategyRuntimeFailure(instanceID string, detail string) error {
	now := time.Now().UTC()
	detail = strings.TrimSpace(detail)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strategy.Status != strategyStatusRunning {
			return nil
		}
		strategy.Status = strategyStatusStopped
		s.recordStrategyEventsLocked(&strategy, now, fmt.Sprintf("strategy runtime exited unexpectedly: %s", detail), "error", "runtime", "runtime_exited", detail)
		s.data.Strategies[index] = strategy
		return s.persistLocked()
	}

	return os.ErrNotExist
}

func (s *strategyCatalogStore) reconcileRuntimeStatesOnStartup() (int, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	changed := 0
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.Status != strategyStatusRunning && strategy.Status != strategyStatusPaused {
			continue
		}
		previousStatus := strategy.Status
		strategy.Status = strategyStatusStopped
		s.recordStrategyEventsLocked(&strategy, now, fmt.Sprintf("reconciled strategy state from %s to %s after server startup", previousStatus, strategyStatusStopped), "warning", "startup", "reconciled", fmt.Sprintf("server startup reset stale %s state to %s", strings.ToLower(previousStatus), strategyStatusStopped))
		s.data.Strategies[index] = strategy
		changed++
	}

	if changed == 0 {
		return 0, nil
	}
	if err := s.persistLocked(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (s *strategyCatalogStore) strategy(instanceID string) (managedStrategyInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, strategy := range s.data.Strategies {
		normalized := s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			return normalized, true
		}
	}
	return managedStrategyInstance{}, false
}

func (s *strategyCatalogStore) refreshStrategyDefinitionLocked(strategy *managedStrategyInstance, definition strategyDesignDefinition, params map[string]any, at time.Time) (bool, error) {
	if strategy == nil {
		return false, nil
	}
	if strategy.Status != strategyStatusStopped {
		return false, errStrategyInstanceBusy
	}
	if strings.TrimSpace(strategy.Definition.Version) == strings.TrimSpace(definition.Version) {
		return false, nil
	}
	previousVersion := strings.TrimSpace(strategy.Definition.Version)
	strategy.PluginID = strategyPluginIDForDefinition(definition)
	strategy.Definition = strategyDefinitionSummary{
		StrategyID: strings.TrimSpace(definition.ID),
		Name:       strings.TrimSpace(definition.Name),
		Version:    strings.TrimSpace(definition.Version),
	}
	strategy.Params = copyMap(params)
	strategy.Binding = normalizeStrategyInstanceBinding(strategy.Binding, strategy.Params)
	applyStrategyBindingParams(strategy)
	s.recordStrategyEventsLocked(
		strategy,
		at,
		fmt.Sprintf("refreshed strategy definition %s to v%s", strings.TrimSpace(definition.ID), strings.TrimSpace(definition.Version)),
		"info",
		"control",
		"definition.refreshed",
		fmt.Sprintf("%s | %s -> %s", strings.TrimSpace(definition.ID), previousVersion, strings.TrimSpace(definition.Version)),
	)
	return true, nil
}

func (s *strategyCatalogStore) persistLocked() error {
	if strings.TrimSpace(s.data.TargetDir) == "" {
		s.data.TargetDir = s.targetDir
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(`DELETE FROM ` + strategyCatalogMetaTable); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM ` + strategyCatalogPluginTable); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM ` + strategyCatalogStrategyTable); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM ` + strategyCatalogOperationTable); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO `+strategyCatalogMetaTable+` (key, value) VALUES (?, ?)`, "target_dir", s.data.TargetDir); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, plugin := range s.data.Plugins {
		payload, marshalErr := json.Marshal(s.normalizePlugin(plugin))
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec(`INSERT INTO `+strategyCatalogPluginTable+` (id, payload_json, updated_at) VALUES (?, ?, ?)`, strings.TrimSpace(plugin.Descriptor.ID), string(payload), now); err != nil {
			return err
		}
	}
	for _, strategy := range s.data.Strategies {
		stored := s.normalizeStrategy(strategy)
		stored.Logs = nil
		stored.AuditEntries = nil
		payload, marshalErr := json.Marshal(stored)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec(`INSERT INTO `+strategyCatalogStrategyTable+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`, strings.TrimSpace(stored.ID), string(payload), strings.TrimSpace(stored.CreatedAt), now); err != nil {
			return err
		}
	}
	for _, operation := range s.data.Operations {
		payload, marshalErr := json.Marshal(operation)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.Exec(`INSERT INTO `+strategyCatalogOperationTable+` (operation_id, plugin_id, status, updated_at, payload_json) VALUES (?, ?, ?, ?, ?)`, strings.TrimSpace(operation.OperationID), strings.TrimSpace(operation.PluginID), strings.TrimSpace(operation.Status), strings.TrimSpace(operation.UpdatedAt), string(payload)); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}
