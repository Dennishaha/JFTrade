package jftradeapi

import (
	"encoding/json"
	"errors"
	"fmt"
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
)

const (
	defaultStrategyCatalogFilename  = "strategy-catalog.json"
	defaultStrategyPluginDirName    = "plugins"
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

type strategyListItem struct {
	ID                 string                      `json:"id"`
	PluginID           string                      `json:"pluginId,omitempty"`
	Definition         strategyDefinitionSummary   `json:"definition"`
	Runtime            string                      `json:"runtime"`
	SourceFormat       string                      `json:"sourceFormat"`
	Startable          bool                        `json:"startable"`
	Binding            strategyInstanceBinding     `json:"binding"`
	Params             map[string]any              `json:"params"`
	Status             string                      `json:"status"`
	CreatedAt          string                      `json:"createdAt"`
	Logs               []string                    `json:"logs"`
	RuntimeObservation *strategyRuntimeObservation `json:"runtimeObservation,omitempty"`
}

type strategyLogsResponse struct {
	InstanceID string   `json:"instanceId"`
	Logs       []string `json:"logs"`
}

type strategyAuditResponse struct {
	InstanceID string               `json:"instanceId"`
	Entries    []strategyAuditEntry `json:"entries"`
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
	path      string
	targetDir string
	mu        sync.RWMutex
	data      strategyCatalogFile
}

func NewStrategyCatalogStore(path string, targetDir string) (*strategyCatalogStore, error) {
	store := &strategyCatalogStore{path: path, targetDir: strings.TrimSpace(targetDir)}
	if store.targetDir == "" {
		store.targetDir = defaultStrategyPluginDirName
	}
	if err := store.load(); err != nil {
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

func (s *strategyCatalogStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = strategyCatalogFile{TargetDir: s.targetDir}
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = strategyCatalogFile{TargetDir: s.targetDir}
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}
	migrated := false
	if strings.TrimSpace(s.data.TargetDir) == "" {
		s.data.TargetDir = s.targetDir
		migrated = true
	}
	for index := range s.data.Strategies {
		normalized := s.normalizeStrategy(s.data.Strategies[index])
		if !reflect.DeepEqual(s.data.Strategies[index], normalized) {
			migrated = true
		}
		s.data.Strategies[index] = normalized
	}
	if migrated {
		return s.persistLocked()
	}
	return nil
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
		Logs: []string{
			fmt.Sprintf("%s instantiated strategy from definition %s", now, definition.ID),
		},
		AuditEntries: []strategyAuditEntry{{
			InstanceID: "",
			Kind:       "instantiated",
			Detail:     strategyBindingAuditDetail(definition.ID, binding),
			At:         now,
		}},
	}
	instance.AuditEntries[0].InstanceID = instance.ID
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
	now := time.Now().UTC().Format(time.RFC3339Nano)

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
		strategy.Logs = append(strategy.Logs, fmt.Sprintf("%s updated strategy binding", now))
		strategy.AuditEntries = append(strategy.AuditEntries, strategyAuditEntry{
			InstanceID: strategy.ID,
			Kind:       "binding.updated",
			Detail:     strategyBindingAuditDetail(strategy.Definition.StrategyID, strategy.Binding),
			At:         now,
		})
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
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
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		strategy.Status = nextStatus
		logEntry := fmt.Sprintf("%s %s strategy %s", now, strings.ToLower(kind), strategy.Definition.StrategyID)
		strategy.Logs = append(strategy.Logs, logEntry)
		strategy.AuditEntries = append(strategy.AuditEntries, strategyAuditEntry{
			InstanceID: strategy.ID,
			Kind:       kind,
			Detail:     detail,
			At:         now,
		})
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) appendStrategyRuntimeEvent(instanceID string, logMessage string, kind string, detail string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strings.TrimSpace(logMessage) != "" {
			strategy.Logs = append(strategy.Logs, fmt.Sprintf("%s %s", now, strings.TrimSpace(logMessage)))
		}
		if strings.TrimSpace(kind) != "" {
			strategy.AuditEntries = append(strategy.AuditEntries, strategyAuditEntry{
				InstanceID: strategy.ID,
				Kind:       strings.TrimSpace(kind),
				Detail:     strings.TrimSpace(detail),
				At:         now,
			})
		}
		s.data.Strategies[index] = strategy
		return s.persistLocked()
	}

	return os.ErrNotExist
}

func (s *strategyCatalogStore) reconcileStrategyRuntimeFailure(instanceID string, detail string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
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
		strategy.Logs = append(strategy.Logs, fmt.Sprintf("%s strategy runtime exited unexpectedly: %s", now, detail))
		strategy.AuditEntries = append(strategy.AuditEntries, strategyAuditEntry{
			InstanceID: strategy.ID,
			Kind:       "runtime_exited",
			Detail:     detail,
			At:         now,
		})
		s.data.Strategies[index] = strategy
		return s.persistLocked()
	}

	return os.ErrNotExist
}

func (s *strategyCatalogStore) reconcileRuntimeStatesOnStartup() (int, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)

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
		strategy.Logs = append(strategy.Logs, fmt.Sprintf("%s reconciled strategy state from %s to %s after server startup", now, previousStatus, strategyStatusStopped))
		strategy.AuditEntries = append(strategy.AuditEntries, strategyAuditEntry{
			InstanceID: strategy.ID,
			Kind:       "reconciled",
			Detail:     fmt.Sprintf("server startup reset stale %s state to %s", strings.ToLower(previousStatus), strategyStatusStopped),
			At:         now,
		})
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

func (s *strategyCatalogStore) strategyLogs(instanceID string) (strategyLogsResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, strategy := range s.data.Strategies {
		normalized := s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			return strategyLogsResponse{InstanceID: instanceID, Logs: append([]string(nil), normalized.Logs...)}, true
		}
	}
	return strategyLogsResponse{}, false
}

func (s *strategyCatalogStore) strategyAudit(instanceID string) (strategyAuditResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, strategy := range s.data.Strategies {
		normalized := s.normalizeStrategy(strategy)
		if normalized.ID == instanceID {
			entries := make([]strategyAuditEntry, len(normalized.AuditEntries))
			copy(entries, normalized.AuditEntries)
			return strategyAuditResponse{InstanceID: instanceID, Entries: entries}, true
		}
	}
	return strategyAuditResponse{}, false
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
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
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
	input.Logs = append([]string(nil), input.Logs...)
	input.AuditEntries = append([]strategyAuditEntry(nil), input.AuditEntries...)
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
		Logs:         append([]string(nil), strategy.Logs...),
	}
}

func normalizeManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	applyStrategyBindingParams(&input)
	if input.Logs == nil {
		input.Logs = []string{}
	}
	if input.AuditEntries == nil {
		input.AuditEntries = []strategyAuditEntry{}
	}
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
