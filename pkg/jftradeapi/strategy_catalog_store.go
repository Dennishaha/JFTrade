package jftradeapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultStrategyCatalogFilename = "strategy-catalog.json"
	defaultStrategyPluginDirName   = "plugins"
	pluginTypeGoStrategy           = "strategy-go-plugin"
	pluginBuildMode                = "plugin"
	strategyStatusRunning          = "RUNNING"
	strategyStatusPaused           = "PAUSED"
	strategyStatusStopped          = "STOPPED"
)

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

type strategyListItem struct {
	ID         string                    `json:"id"`
	PluginID   string                    `json:"pluginId,omitempty"`
	Definition strategyDefinitionSummary `json:"definition"`
	Params     map[string]any            `json:"params"`
	Status     string                    `json:"status"`
	CreatedAt  string                    `json:"createdAt"`
	Logs       []string                  `json:"logs"`
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
	if strings.TrimSpace(s.data.TargetDir) == "" {
		s.data.TargetDir = s.targetDir
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
		items = append(items, strategyListItem{
			ID:         normalized.ID,
			PluginID:   normalized.PluginID,
			Definition: normalized.Definition,
			Params:     copyMap(normalized.Params),
			Status:     normalized.Status,
			CreatedAt:  normalized.CreatedAt,
			Logs:       append([]string(nil), normalized.Logs...),
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		return items[i].CreatedAt < items[j].CreatedAt
	})
	return items
}

func (s *strategyCatalogStore) instantiateStrategy(definition strategyDesignDefinition) (strategyListItem, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	instance := managedStrategyInstance{
		ID:       buildStrategyInstanceID(definition.ID),
		PluginID: IDQuickJSPlugin(),
		Definition: strategyDefinitionSummary{
			StrategyID: definition.ID,
			Name:       definition.Name,
			Version:    definition.Version,
		},
		Params: map[string]any{
			"runtime":      normalizeStrategyRuntime(definition.Runtime),
			"definitionId": definition.ID,
			"symbol":       definition.Symbol,
			"interval":     definition.Interval,
			"script":       definition.Script,
		},
		Status:    strategyStatusStopped,
		CreatedAt: now,
		Logs: []string{
			fmt.Sprintf("%s instantiated strategy from definition %s", now, definition.ID),
		},
		AuditEntries: []strategyAuditEntry{{
			InstanceID: "",
			Kind:       "instantiated",
			Detail:     definition.ID,
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
	if strings.TrimSpace(input.PluginID) == "" {
		input.PluginID = IDQuickJSPlugin()
	}
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	if runtime, ok := input.Params["runtime"].(string); ok {
		input.Params["runtime"] = normalizeStrategyRuntime(runtime)
	} else {
		input.Params["runtime"] = strategyRuntimeQuickJS
	}
	if input.Definition.StrategyID == "" {
		input.Definition.StrategyID = input.PluginID
	}
	if input.Definition.Name == "" {
		input.Definition.Name = input.PluginID
	}
	if input.Definition.Version == "" {
		input.Definition.Version = "0.1.0"
	}
	if input.Status == "" {
		input.Status = strategyStatusStopped
	}
	if input.CreatedAt == "" {
		input.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if input.Logs == nil {
		input.Logs = []string{}
	}
	if input.AuditEntries == nil {
		input.AuditEntries = []strategyAuditEntry{}
	}
	return input
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
		output[key] = value
	}
	return output
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
	input.Logs = append([]string(nil), input.Logs...)
	input.AuditEntries = append([]strategyAuditEntry(nil), input.AuditEntries...)
	return input
}

func strategyToListItem(strategy managedStrategyInstance) strategyListItem {
	strategy = normalizeManagedStrategyInstance(strategy)
	return strategyListItem{
		ID:         strategy.ID,
		PluginID:   strategy.PluginID,
		Definition: strategy.Definition,
		Params:     copyMap(strategy.Params),
		Status:     strategy.Status,
		CreatedAt:  strategy.CreatedAt,
		Logs:       append([]string(nil), strategy.Logs...),
	}
}

func normalizeManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	if input.Params == nil {
		input.Params = map[string]any{}
	}
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
		definitionID = IDQuickJSPlugin()
	}
	return definitionID + "-" + time.Now().UTC().Format("20060102150405.000000000")
}

func IDQuickJSPlugin() string {
	return "quickjs"
}
