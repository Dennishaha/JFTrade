package jftradeapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	"github.com/jmoiron/sqlx"
)

const (
	defaultStrategyCatalogFilename  = "strategy-catalog.json"
	defaultStrategyPluginDirName    = "plugins"
	strategyCatalogMetaTable        = "strategy_catalog_meta"
	strategyCatalogPluginTable      = "strategy_catalog_plugins"
	strategyCatalogStrategyTable    = "strategy_catalog_strategies"
	strategyCatalogOperationTable   = "strategy_catalog_operations"
	pluginTypeGoStrategy            = "strategy-go-plugin"
	pluginBuildMode                 = "plugin"
	strategyStatusRunning           = "RUNNING"
	strategyStatusPaused            = "PAUSED"
	strategyStatusStopped           = "STOPPED"
	strategyExecutionModeLive       = "live"
	strategyExecutionModeNotifyOnly = "notify_only"
)

var errStrategyInstanceBusy = errors.New("strategy instance must be stopped before modification")

type strategyPluginBuildTuple struct {
	JFTradeVersion string   `json:"jftradeVersion"`
	GoVersion      string   `json:"goVersion"`
	GOOS           string   `json:"goos"`
	GOARCH         string   `json:"goarch"`
	BuildMode      string   `json:"buildMode"`
	BuildTags      []string `json:"buildTags,omitempty"`
}

type strategyPluginArtifact struct {
	Path  string                   `json:"path"`
	Build strategyPluginBuildTuple `json:"build"`
}

type strategyPluginCompatibility struct {
	Mode            string                    `json:"mode"`
	Supported       bool                      `json:"supported"`
	RequiresRebuild bool                      `json:"requiresRebuild"`
	Reason          *string                   `json:"reason,omitempty"`
	Host            strategyPluginBuildTuple  `json:"host"`
	Artifact        *strategyPluginBuildTuple `json:"artifact,omitempty"`
}

type strategyPluginDescriptor struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	DisplayName string   `json:"displayName"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
}

type strategyPluginOperation struct {
	OperationID string  `json:"operationId"`
	PluginID    string  `json:"pluginId"`
	Status      string  `json:"status"`
	Phase       string  `json:"phase"`
	Progress    int     `json:"progress"`
	Message     string  `json:"message"`
	TargetDir   string  `json:"targetDir"`
	InstallPath string  `json:"installPath"`
	StartedAt   string  `json:"startedAt"`
	UpdatedAt   string  `json:"updatedAt"`
	CompletedAt *string `json:"completedAt"`
	Error       *string `json:"error"`
}

type strategyPluginUninstallGuidance struct {
	PluginID string `json:"pluginId"`
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Commands struct {
		Posix      string `json:"posix"`
		PowerShell string `json:"powershell"`
	} `json:"commands"`
}

type strategyPluginInstallation struct {
	Status            string                          `json:"status"`
	Installed         bool                            `json:"installed"`
	InstallPath       string                          `json:"installPath"`
	TargetDir         string                          `json:"targetDir"`
	MarkerPath        string                          `json:"markerPath"`
	CurrentOperation  *strategyPluginOperation        `json:"currentOperation"`
	LastOperation     *strategyPluginOperation        `json:"lastOperation"`
	UninstallGuidance strategyPluginUninstallGuidance `json:"uninstallGuidance"`
}

type strategyPluginCatalogItem struct {
	Descriptor    strategyPluginDescriptor    `json:"descriptor"`
	Installation  strategyPluginInstallation  `json:"installation"`
	Compatibility strategyPluginCompatibility `json:"compatibility"`
}

type strategyPluginCatalogResponse struct {
	TargetDir string                      `json:"targetDir"`
	Plugins   []strategyPluginCatalogItem `json:"plugins"`
}

type strategyDefinitionSummary struct {
	StrategyID string `json:"strategyId"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type strategyAuditEntry struct {
	InstanceID string `json:"instanceId"`
	Kind       string `json:"kind"`
	Detail     string `json:"detail,omitempty"`
	At         string `json:"at"`
}

type strategyBrokerAccountBinding struct {
	BrokerID           string `json:"brokerId"`
	AccountID          string `json:"accountId"`
	TradingEnvironment string `json:"tradingEnvironment"`
	Market             string `json:"market"`
}

type strategyInstanceBinding struct {
	Symbols       []string                      `json:"symbols"`
	Interval      string                        `json:"interval"`
	ExecutionMode string                        `json:"executionMode"`
	BrokerAccount *strategyBrokerAccountBinding `json:"brokerAccount,omitempty"`
}

type strategyRuntimeObservation struct {
	ActualStatus      string   `json:"actualStatus"`
	ActiveSymbols     []string `json:"activeSymbols"`
	LastClosedKLineAt *string  `json:"lastClosedKlineAt,omitempty"`
	LastSignalAt      *string  `json:"lastSignalAt,omitempty"`
	LastOrderAt       *string  `json:"lastOrderAt,omitempty"`
	LastErrorAt       *string  `json:"lastErrorAt,omitempty"`
	LastError         *string  `json:"lastError,omitempty"`
	UpdatedAt         *string  `json:"updatedAt,omitempty"`
}

type strategyRuntimeActiveInstanceSummary struct {
	InstanceID        string   `json:"instanceId"`
	DefinitionName    string   `json:"definitionName"`
	ActualStatus      string   `json:"actualStatus"`
	ActiveSymbols     []string `json:"activeSymbols"`
	LastClosedKLineAt *string  `json:"lastClosedKlineAt,omitempty"`
	LastSignalAt      *string  `json:"lastSignalAt,omitempty"`
	LastOrderAt       *string  `json:"lastOrderAt,omitempty"`
	LastErrorAt       *string  `json:"lastErrorAt,omitempty"`
	LastError         *string  `json:"lastError,omitempty"`
	UpdatedAt         *string  `json:"updatedAt,omitempty"`
}

type strategyDefinitionSyncStatus struct {
	DefinitionID   string  `json:"definitionId"`
	AppliedVersion string  `json:"appliedVersion"`
	LatestVersion  string  `json:"latestVersion"`
	IsLatest       bool    `json:"isLatest"`
	CanApplyLatest bool    `json:"canApplyLatest"`
	BlockedReason  *string `json:"blockedReason,omitempty"`
}

type strategyApplyLinkedInstancesResponse struct {
	DefinitionID  string   `json:"definitionId"`
	LatestVersion string   `json:"latestVersion"`
	TotalLinked   int      `json:"totalLinked"`
	Applied       []string `json:"applied"`
	AlreadyLatest []string `json:"alreadyLatest"`
	SkippedBusy   []string `json:"skippedBusy"`
}

type strategyListItem struct {
	ID                 string                        `json:"id"`
	PluginID           string                        `json:"pluginId,omitempty"`
	Definition         strategyDefinitionSummary     `json:"definition"`
	Runtime            string                        `json:"runtime"`
	SourceFormat       string                        `json:"sourceFormat"`
	Startable          bool                          `json:"startable"`
	Binding            strategyInstanceBinding       `json:"binding"`
	Params             map[string]any                `json:"params"`
	Status             string                        `json:"status"`
	CreatedAt          string                        `json:"createdAt"`
	Logs               []string                      `json:"logs"`
	DefinitionSync     *strategyDefinitionSyncStatus `json:"definitionSync,omitempty"`
	RuntimeObservation *strategyRuntimeObservation   `json:"runtimeObservation,omitempty"`
}

type strategyLogsResponse struct {
	InstanceID string               `json:"instanceId"`
	Logs       []string             `json:"logs"`
	Page       strategyActivityPage `json:"page"`
}

type strategyAuditResponse struct {
	InstanceID string               `json:"instanceId"`
	Entries    []strategyAuditEntry `json:"entries"`
	Page       strategyActivityPage `json:"page"`
}

type strategyActivityPage struct {
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
	Total    int  `json:"total"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"hasMore"`
}

type managedStrategyPlugin struct {
	Descriptor   strategyPluginDescriptor   `json:"descriptor"`
	Artifact     *strategyPluginArtifact    `json:"artifact,omitempty"`
	Installation strategyPluginInstallation `json:"installation"`
}

type managedStrategyInstance struct {
	ID           string                    `json:"id"`
	PluginID     string                    `json:"pluginId,omitempty"`
	Definition   strategyDefinitionSummary `json:"definition"`
	Binding      strategyInstanceBinding   `json:"binding"`
	Params       map[string]any            `json:"params"`
	Status       string                    `json:"status"`
	CreatedAt    string                    `json:"createdAt"`
	Logs         []string                  `json:"logs,omitempty"`
	AuditEntries []strategyAuditEntry      `json:"auditEntries,omitempty"`
}

type strategyCatalogFile struct {
	TargetDir  string                    `json:"targetDir,omitempty"`
	Plugins    []managedStrategyPlugin   `json:"plugins,omitempty"`
	Strategies []managedStrategyInstance `json:"strategies,omitempty"`
	Operations []strategyPluginOperation `json:"operations,omitempty"`
}

type strategyCatalogStore struct {
	path         string
	dbPath       string
	db           *sqlx.DB
	targetDir    string
	runtimeStore *strategyRuntimeStore
	mu           sync.RWMutex
	data         strategyCatalogFile
}

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

func (s *strategyCatalogStore) strategyLogs(instanceID string) (strategyLogsResponse, bool) {
	return s.strategyLogsPage(instanceID, strategyRuntimeLogQuery{InstanceID: instanceID, Limit: maxStrategyRuntimePageSize})
}

func (s *strategyCatalogStore) strategyLogsPage(instanceID string, query strategyRuntimeLogQuery) (strategyLogsResponse, bool) {
	s.mu.RLock()
	var normalized managedStrategyInstance
	var found bool
	for _, strategy := range s.data.Strategies {
		normalized = s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			found = true
			break
		}
	}
	s.mu.RUnlock()
	if !found {
		return strategyLogsResponse{}, false
	}
	if s.runtimeStore == nil {
		return strategyLogsResponse{InstanceID: instanceID, Logs: []string{}, Page: strategyActivityPage{Limit: normalizeStrategyRuntimePageSize(query.Limit), Offset: normalizeStrategyRuntimeOffset(query.Offset), Total: 0, Returned: 0, HasMore: false}}, true
	}
	query.InstanceID = instanceID
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	offset := normalizeStrategyRuntimeOffset(query.Offset)
	total, countErr := s.runtimeStore.CountLogs(context.Background(), query)
	persisted, listErr := s.runtimeStore.ListLogs(context.Background(), query)
	if countErr != nil {
		log.Printf("JFTrade strategy log count degraded: %v", countErr)
		return strategyLogsResponse{InstanceID: instanceID, Logs: []string{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: 0, Returned: 0, HasMore: false}}, true
	}
	if listErr != nil {
		log.Printf("JFTrade strategy log query degraded: %v", listErr)
		return strategyLogsResponse{InstanceID: instanceID, Logs: []string{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: 0, HasMore: false}}, true
	}
	logs := make([]string, 0, len(persisted))
	for _, event := range persisted {
		logs = append(logs, event.Raw)
	}
	return strategyLogsResponse{InstanceID: instanceID, Logs: logs, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: len(logs), HasMore: offset+len(logs) < total}}, true
}

func (s *strategyCatalogStore) strategyAudit(instanceID string) (strategyAuditResponse, bool) {
	return s.strategyAuditPage(instanceID, strategyRuntimeAuditQuery{InstanceID: instanceID, Limit: maxStrategyRuntimePageSize})
}

func (s *strategyCatalogStore) strategyAuditPage(instanceID string, query strategyRuntimeAuditQuery) (strategyAuditResponse, bool) {
	s.mu.RLock()
	var normalized managedStrategyInstance
	var found bool
	for _, strategy := range s.data.Strategies {
		normalized = s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			found = true
			break
		}
	}
	s.mu.RUnlock()
	if !found {
		return strategyAuditResponse{}, false
	}
	if s.runtimeStore == nil {
		return strategyAuditResponse{InstanceID: instanceID, Entries: []strategyAuditEntry{}, Page: strategyActivityPage{Limit: normalizeStrategyRuntimePageSize(query.Limit), Offset: normalizeStrategyRuntimeOffset(query.Offset), Total: 0, Returned: 0, HasMore: false}}, true
	}
	query.InstanceID = instanceID
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	offset := normalizeStrategyRuntimeOffset(query.Offset)
	total, countErr := s.runtimeStore.CountAudit(context.Background(), query)
	persisted, listErr := s.runtimeStore.ListAudit(context.Background(), query)
	if countErr != nil {
		log.Printf("JFTrade strategy audit count degraded: %v", countErr)
		return strategyAuditResponse{InstanceID: instanceID, Entries: []strategyAuditEntry{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: 0, Returned: 0, HasMore: false}}, true
	}
	if listErr != nil {
		log.Printf("JFTrade strategy audit query degraded: %v", listErr)
		return strategyAuditResponse{InstanceID: instanceID, Entries: []strategyAuditEntry{}, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: 0, HasMore: false}}, true
	}
	entries := make([]strategyAuditEntry, 0, len(persisted))
	for _, event := range persisted {
		entries = append(entries, strategyAuditEntry{InstanceID: event.InstanceID, Kind: event.Kind, Detail: event.Detail, At: event.At.UTC().Format(time.RFC3339Nano)})
	}
	return strategyAuditResponse{InstanceID: instanceID, Entries: entries, Page: strategyActivityPage{Limit: limit, Offset: offset, Total: total, Returned: len(entries), HasMore: offset+len(entries) < total}}, true
}

func (s *strategyCatalogStore) recordStrategyEventsLocked(strategy *managedStrategyInstance, at time.Time, logMessage string, logLevel string, logSource string, kind string, detail string) bool {
	rawLog := buildStrategyRuntimeLogEntry(at, logMessage)
	if rawLog != "" {
		if s.runtimeStore != nil {
			if err := s.runtimeStore.AppendLog(context.Background(), strategyRuntimeLogEvent{
				InstanceID: strategy.ID,
				At:         at,
				Raw:        rawLog,
				Level:      strings.ToLower(strings.TrimSpace(logLevel)),
				Source:     strings.ToLower(strings.TrimSpace(logSource)),
			}); err != nil {
				log.Printf("JFTrade persist strategy runtime log degraded: %v", err)
			}
		}
	}

	kind = strings.TrimSpace(kind)
	if kind != "" {
		auditEntry := strategyAuditEntry{
			InstanceID: strategy.ID,
			Kind:       kind,
			Detail:     strings.TrimSpace(detail),
			At:         at.UTC().Format(time.RFC3339Nano),
		}
		if s.runtimeStore != nil {
			if err := s.runtimeStore.AppendAudit(context.Background(), strategyRuntimeAuditEvent{
				InstanceID: strategy.ID,
				Kind:       auditEntry.Kind,
				Detail:     auditEntry.Detail,
				At:         at,
			}); err != nil {
				log.Printf("JFTrade persist strategy runtime audit degraded: %v", err)
			}
		}
	}

	return false
}

func buildStrategyRuntimeLogEntry(at time.Time, logMessage string) string {
	logMessage = strings.TrimSpace(logMessage)
	if logMessage == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", at.UTC().Format(time.RFC3339Nano), logMessage)
}

func strategyLogLevelForKind(kind string, logMessage string) string {
	switch strings.TrimSpace(kind) {
	case "runtime_error", "order_submit_failed", "runtime_exited":
		return "error"
	case "reconciled":
		return "warning"
	}
	message := strings.ToLower(strings.TrimSpace(logMessage))
	if strings.Contains(message, "error") || strings.Contains(message, "failed") || strings.Contains(message, "panic") {
		return "error"
	}
	return "info"
}

func sliceStrategyActivityPage[T any](items []T, limit int, offset int) ([]T, int, bool) {
	total := len(items)
	if offset >= total {
		return []T{}, total, false
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := append([]T(nil), items[offset:end]...)
	return page, total, end < total
}

func (s *strategyCatalogStore) runtimeSummary() map[string]any {
	strategies := s.strategies()
	activeCount := 0
	for _, strategy := range strategies {
		if strategy.Status == strategyStatusRunning || strategy.Status == strategyStatusPaused {
			activeCount++
		}
	}
	status := "idle"
	if activeCount > 0 {
		status = "active"
	}
	return map[string]any{
		"status":                 status,
		"activeStrategies":       activeCount,
		"supportsBacktestParity": true,
	}
}

func (s *strategyCatalogStore) normalizePlugin(input managedStrategyPlugin) managedStrategyPlugin {
	input = cloneManagedStrategyPlugin(input)
	input.Descriptor.ID = strings.TrimSpace(input.Descriptor.ID)
	if input.Descriptor.Type == "" {
		input.Descriptor.Type = pluginTypeGoStrategy
	}
	if input.Descriptor.DisplayName == "" {
		input.Descriptor.DisplayName = input.Descriptor.ID
	}
	if input.Descriptor.Version == "" {
		input.Descriptor.Version = "0.1.0"
	}
	if input.Descriptor.Keywords == nil {
		input.Descriptor.Keywords = []string{}
	}

	targetDir := s.effectiveTargetDirLocked()
	if input.Installation.TargetDir == "" {
		input.Installation.TargetDir = targetDir
	}
	if input.Installation.InstallPath == "" {
		input.Installation.InstallPath = filepath.Join(input.Installation.TargetDir, input.Descriptor.ID+".so")
	}
	if input.Installation.MarkerPath == "" {
		input.Installation.MarkerPath = filepath.Join(input.Installation.TargetDir, input.Descriptor.ID+".json")
	}
	if input.Installation.Status == "" {
		if input.Installation.Installed {
			input.Installation.Status = "INSTALLED"
		} else {
			input.Installation.Status = "NOT_INSTALLED"
		}
	}
	input.Installation.UninstallGuidance = buildPluginUninstallGuidance(input.Descriptor.ID, input.Installation.InstallPath)
	if input.Artifact != nil {
		if input.Artifact.Path == "" {
			input.Artifact.Path = input.Installation.InstallPath
		}
		if input.Artifact.Build.BuildMode == "" {
			input.Artifact.Build.BuildMode = pluginBuildMode
		}
	}
	return input
}

func (s *strategyCatalogStore) normalizeStrategy(input managedStrategyInstance) managedStrategyInstance {
	input = cloneManagedStrategyInstance(input)
	if input.ID == "" {
		input.ID = "strategy-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	input.PluginID = IDDSLPlanPlugin()
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	rawRuntime, _ := input.Params["runtime"].(string)
	rawSourceFormat, _ := input.Params["sourceFormat"].(string)
	if runtime, ok := input.Params["runtime"].(string); ok {
		input.Params["runtime"] = normalizeStrategyRuntime(runtime)
	} else {
		input.Params["runtime"] = strategyRuntimeDSLPlan
	}
	input.Params["sourceFormat"] = strategydefinition.SourceFormatDSLV1
	if input.Definition.StrategyID == "" {
		input.Definition.StrategyID = input.PluginID
	}
	if input.Definition.Name == "" {
		input.Definition.Name = input.PluginID
	}
	if input.Definition.Version == "" {
		input.Definition.Version = "0.1.0"
	}
	if script, _ := input.Params["script"].(string); shouldReplaceWithDefaultDSLScript(rawSourceFormat, rawRuntime, script) {
		input.Params["script"] = defaultStrategyDesignDSL(input.Definition.Name)
	}
	if input.Status == "" {
		input.Status = strategyStatusStopped
	}
	if input.CreatedAt == "" {
		input.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return normalizeManagedStrategyInstance(input)
}

func (s *strategyCatalogStore) effectiveTargetDirLocked() string {
	if strings.TrimSpace(s.data.TargetDir) != "" {
		return s.data.TargetDir
	}
	if strings.TrimSpace(s.targetDir) != "" {
		return s.targetDir
	}
	return defaultStrategyPluginDirName
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

func buildPluginCompatibility(artifact *strategyPluginArtifact) strategyPluginCompatibility {
	host := currentPluginBuildTuple()
	compatibility := strategyPluginCompatibility{
		Mode:      pluginBuildMode,
		Supported: runtime.GOOS != "windows",
		Host:      host,
	}
	if !compatibility.Supported {
		reason := "go plugin is unsupported on windows hosts"
		compatibility.Reason = &reason
	}
	if artifact == nil {
		return compatibility
	}
	artifactBuild := artifact.Build
	compatibility.Artifact = &artifactBuild
	compatibility.RequiresRebuild = !samePluginBuildTuple(host, artifactBuild)
	if compatibility.RequiresRebuild {
		reason := "artifact build tuple does not match the current jftrade host"
		compatibility.Reason = &reason
	}
	return compatibility
}

func currentPluginBuildTuple() strategyPluginBuildTuple {
	return strategyPluginBuildTuple{
		JFTradeVersion: currentJFTradeVersion(),
		GoVersion:      runtime.Version(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		BuildMode:      pluginBuildMode,
	}
}

func currentJFTradeVersion() string {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		version := strings.TrimSpace(buildInfo.Main.Version)
		if version != "" {
			return version
		}
	}
	return "devel"
}

func samePluginBuildTuple(left strategyPluginBuildTuple, right strategyPluginBuildTuple) bool {
	if left.JFTradeVersion != right.JFTradeVersion || left.GoVersion != right.GoVersion || left.GOOS != right.GOOS || left.GOARCH != right.GOARCH || left.BuildMode != right.BuildMode {
		return false
	}
	if len(left.BuildTags) != len(right.BuildTags) {
		return false
	}
	for index := range left.BuildTags {
		if left.BuildTags[index] != right.BuildTags[index] {
			return false
		}
	}
	return true
}

func buildPluginOperationID(pluginID string) string {
	return strings.ToLower(strings.ReplaceAll(pluginID, " ", "-")) + "-" + time.Now().UTC().Format("20060102150405.000000000")
}

func buildStrategyInstanceParams(definition strategyDesignDefinition, compiledAt string) (map[string]any, error) {
	sourceFormat := strategydefinition.NormalizeSourceFormat(definition.SourceFormat)
	if sourceFormat != strategydefinition.SourceFormatDSLV1 {
		return nil, fmt.Errorf("unsupported strategy source format: %s", sourceFormat)
	}
	symbol := strings.ToUpper(strings.TrimSpace(definition.Symbol))
	interval := strings.TrimSpace(definition.Interval)
	if interval == "" {
		interval = "5m"
	}
	params := map[string]any{
		"definitionId": definition.ID,
		"sourceFormat": sourceFormat,
		"symbol":       symbol,
		"interval":     interval,
		"script":       definition.Script,
	}
	program, err := strategydsl.ParseScript(definition.Script)
	if err != nil {
		return nil, err
	}
	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		return nil, err
	}
	params["runtime"] = strategyRuntimeDSLPlan
	params["compiledAt"] = compiledAt
	params["compiledHooks"] = buildCompiledHookKinds(program)
	params["compiledRequirements"] = buildCompiledRequirementsPayload(requirements)

	return params, nil
}

func strategyDefinitionIDFromParams(params map[string]any) string {
	definitionID, _ := params["definitionId"].(string)
	return strings.TrimSpace(definitionID)
}

func strategyInstanceUsesDefinition(strategy managedStrategyInstance, definitionID string) bool {
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return false
	}
	if strategyDefinitionIDFromParams(strategy.Params) == definitionID {
		return true
	}
	return strings.TrimSpace(strategy.Definition.StrategyID) == definitionID
}

func normalizeStrategyInstrumentID(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	if strings.Contains(normalized, ":") {
		parts := strings.SplitN(normalized, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			return strings.TrimSpace(parts[0]) + "." + strings.TrimSpace(parts[1])
		}
	}
	return normalized
}

func normalizeStrategyExecutionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case strategyExecutionModeNotifyOnly:
		return strategyExecutionModeNotifyOnly
	default:
		return strategyExecutionModeLive
	}
}

func normalizeStrategyBrokerAccountBinding(input *strategyBrokerAccountBinding) *strategyBrokerAccountBinding {
	if input == nil {
		return nil
	}
	copyValue := *input
	copyValue.BrokerID = strings.ToLower(strings.TrimSpace(copyValue.BrokerID))
	copyValue.AccountID = strings.TrimSpace(copyValue.AccountID)
	copyValue.TradingEnvironment = strings.ToUpper(strings.TrimSpace(copyValue.TradingEnvironment))
	copyValue.Market = strings.ToUpper(strings.TrimSpace(copyValue.Market))
	if copyValue.BrokerID == "" && copyValue.AccountID == "" && copyValue.TradingEnvironment == "" && copyValue.Market == "" {
		return nil
	}
	return &copyValue
}

func readStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, entry := range typed {
			if text, ok := entry.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func strategyBrokerAccountBindingFromAny(value any) *strategyBrokerAccountBinding {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	brokerID, _ := raw["brokerId"].(string)
	accountID, _ := raw["accountId"].(string)
	tradingEnvironment, _ := raw["tradingEnvironment"].(string)
	market, _ := raw["market"].(string)
	return normalizeStrategyBrokerAccountBinding(&strategyBrokerAccountBinding{
		BrokerID:           brokerID,
		AccountID:          accountID,
		TradingEnvironment: tradingEnvironment,
		Market:             market,
	})
}

func normalizeStrategyInstanceBinding(input strategyInstanceBinding, params map[string]any) strategyInstanceBinding {
	if len(input.Symbols) == 0 {
		input.Symbols = readStringSlice(params["symbols"])
		if len(input.Symbols) == 0 {
			if symbol, ok := params["symbol"].(string); ok && strings.TrimSpace(symbol) != "" {
				input.Symbols = []string{symbol}
			}
		}
	}
	seen := map[string]struct{}{}
	normalizedSymbols := make([]string, 0, len(input.Symbols))
	for _, symbol := range input.Symbols {
		normalized := normalizeStrategyInstrumentID(symbol)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		normalizedSymbols = append(normalizedSymbols, normalized)
	}
	input.Symbols = normalizedSymbols

	input.Interval = strings.TrimSpace(input.Interval)
	if input.Interval == "" {
		if interval, ok := params["interval"].(string); ok {
			input.Interval = strings.TrimSpace(interval)
		}
	}
	if input.Interval == "" {
		input.Interval = "5m"
	}

	if input.BrokerAccount == nil {
		input.BrokerAccount = strategyBrokerAccountBindingFromAny(params["brokerAccount"])
	}
	input.BrokerAccount = normalizeStrategyBrokerAccountBinding(input.BrokerAccount)

	if strings.TrimSpace(input.ExecutionMode) == "" {
		if executionMode, ok := params["executionMode"].(string); ok {
			input.ExecutionMode = executionMode
		}
	}
	input.ExecutionMode = normalizeStrategyExecutionMode(input.ExecutionMode)

	return input
}

func applyStrategyBindingParams(input *managedStrategyInstance) {
	if input == nil {
		return
	}
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	input.Binding = normalizeStrategyInstanceBinding(input.Binding, input.Params)
	input.Params["symbols"] = append([]string(nil), input.Binding.Symbols...)
	if len(input.Binding.Symbols) > 0 {
		input.Params["symbol"] = input.Binding.Symbols[0]
	} else {
		delete(input.Params, "symbol")
	}
	input.Params["interval"] = input.Binding.Interval
	input.Params["executionMode"] = input.Binding.ExecutionMode
	if input.Binding.BrokerAccount != nil {
		input.Params["brokerAccount"] = map[string]any{
			"brokerId":           input.Binding.BrokerAccount.BrokerID,
			"accountId":          input.Binding.BrokerAccount.AccountID,
			"tradingEnvironment": input.Binding.BrokerAccount.TradingEnvironment,
			"market":             input.Binding.BrokerAccount.Market,
		}
	} else {
		delete(input.Params, "brokerAccount")
	}
}

func strategyBindingAuditDetail(definitionID string, binding strategyInstanceBinding) string {
	parts := []string{strings.TrimSpace(definitionID)}
	if len(binding.Symbols) > 0 {
		parts = append(parts, "symbols="+strings.Join(binding.Symbols, ","))
	}
	parts = append(parts, "interval="+binding.Interval)
	parts = append(parts, "mode="+binding.ExecutionMode)
	if binding.BrokerAccount != nil {
		parts = append(parts, fmt.Sprintf("account=%s/%s/%s/%s", binding.BrokerAccount.BrokerID, binding.BrokerAccount.TradingEnvironment, binding.BrokerAccount.AccountID, binding.BrokerAccount.Market))
	}
	return strings.Join(parts, " | ")
}

func buildCompiledHookKinds(program *strategyir.Program) []string {
	if program == nil {
		return []string{}
	}
	result := make([]string, 0, len(program.Hooks))
	for _, hook := range program.Hooks {
		result = append(result, string(hook.Kind))
	}
	return result
}

func buildCompiledRequirementsPayload(requirements strategyir.Requirements) map[string]any {
	indicators := make([]map[string]any, 0, len(requirements.Indicators))
	for _, indicator := range requirements.Indicators {
		indicators = append(indicators, map[string]any{
			"alias": indicator.Alias,
			"kind":  indicator.Kind,
			"key":   indicator.Key,
		})
	}
	return map[string]any{
		"indicators":                indicators,
		"requiresPosition":          requirements.RequiresPosition,
		"requiresAvailableCash":     requirements.RequiresAvailableCash,
		"requiresMarginBuyingPower": requirements.RequiresMarginBuyingPower,
		"requiresShortSellingPower": requirements.RequiresShortSellingPower,
		"requiresTotalAccountValue": requirements.RequiresTotalAccountValue,
	}
}

func strategyPluginIDForDefinition(definition strategyDesignDefinition) string {
	_ = definition
	return IDDSLPlanPlugin()
}

func strategyRuntimeFromParams(params map[string]any) string {
	if runtime, ok := params["runtime"].(string); ok {
		return normalizeStrategyRuntime(runtime)
	}
	return strategyRuntimeDSLPlan
}

func strategySourceFormatFromParams(params map[string]any) string {
	if sourceFormat, ok := params["sourceFormat"].(string); ok {
		return strategydefinition.NormalizeSourceFormat(sourceFormat)
	}
	return strategydefinition.SourceFormatDSLV1
}

func strategyInstanceStartable(instance managedStrategyInstance) bool {
	sourceFormat := strategySourceFormatFromParams(instance.Params)
	runtime := strategyRuntimeFromParams(instance.Params)
	return sourceFormat == strategydefinition.SourceFormatDSLV1 && runtime == strategyRuntimeDSLPlan
}

func buildPluginUninstallGuidance(pluginID string, installPath string) strategyPluginUninstallGuidance {
	guidance := strategyPluginUninstallGuidance{
		PluginID: pluginID,
		Path:     installPath,
		Exists:   false,
	}
	guidance.Commands.Posix = "rm -f " + shellQuote(installPath)
	guidance.Commands.PowerShell = "Remove-Item -LiteralPath '" + strings.ReplaceAll(installPath, "'", "''") + "' -Force"
	return guidance
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = copyDynamicValue(value)
	}
	return output
}

func copyDynamicValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []any:
		output := make([]any, len(typed))
		for index, entry := range typed {
			output[index] = copyDynamicValue(entry)
		}
		return output
	case []map[string]any:
		output := make([]map[string]any, len(typed))
		for index, entry := range typed {
			output[index] = copyMap(entry)
		}
		return output
	default:
		return value
	}
}

func cloneManagedStrategyPlugin(input managedStrategyPlugin) managedStrategyPlugin {
	if input.Descriptor.Keywords != nil {
		input.Descriptor.Keywords = append([]string(nil), input.Descriptor.Keywords...)
	}
	if input.Artifact != nil {
		artifactCopy := *input.Artifact
		artifactCopy.Build.BuildTags = append([]string(nil), artifactCopy.Build.BuildTags...)
		input.Artifact = &artifactCopy
	}
	if input.Installation.CurrentOperation != nil {
		operationCopy := *input.Installation.CurrentOperation
		input.Installation.CurrentOperation = &operationCopy
	}
	if input.Installation.LastOperation != nil {
		operationCopy := *input.Installation.LastOperation
		input.Installation.LastOperation = &operationCopy
	}
	return input
}

func cloneManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	input.Params = copyMap(input.Params)
	input.Binding.Symbols = append([]string(nil), input.Binding.Symbols...)
	if input.Binding.BrokerAccount != nil {
		bindingCopy := *input.Binding.BrokerAccount
		input.Binding.BrokerAccount = &bindingCopy
	}
	return input
}

func strategyToListItem(strategy managedStrategyInstance) strategyListItem {
	strategy = normalizeManagedStrategyInstance(strategy)
	return strategyListItem{
		ID:           strategy.ID,
		PluginID:     strategy.PluginID,
		Definition:   strategy.Definition,
		Runtime:      strategyRuntimeFromParams(strategy.Params),
		SourceFormat: strategySourceFormatFromParams(strategy.Params),
		Startable:    strategyInstanceStartable(strategy),
		Binding:      strategy.Binding,
		Params:       copyMap(strategy.Params),
		Status:       strategy.Status,
		CreatedAt:    strategy.CreatedAt,
		Logs:         []string{},
	}
}

func normalizeManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	applyStrategyBindingParams(&input)
	return input
}

func buildStrategyInstanceID(definitionID string) string {
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		definitionID = IDDSLPlanPlugin()
	}
	return definitionID + "-" + time.Now().UTC().Format("20060102150405.000000000")
}

func IDDSLPlanPlugin() string {
	return "dsl-go-plan"
}
