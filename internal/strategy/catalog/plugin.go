package catalog

import (
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func (s *Service) RegisterPlugin(input ManagedPlugin) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	input = s.normalizePlugin(input)
	for index := range s.data.Plugins {
		if s.data.Plugins[index].Descriptor.ID == input.Descriptor.ID {
			s.data.Plugins[index] = input
			return s.persistLocked()
		}
	}
	s.data.Plugins = append(s.data.Plugins, input)
	return s.persistLocked()
}

func (s *Service) PluginCatalog() stratsrv.PluginCatalog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plugins := make([]stratsrv.PluginCatalogItem, 0, len(s.data.Plugins))
	for _, plugin := range s.data.Plugins {
		normalized := s.normalizePlugin(plugin)
		plugins = append(plugins, stratsrv.PluginCatalogItem{
			Descriptor:    normalized.Descriptor,
			Installation:  normalized.Installation,
			Compatibility: buildPluginCompatibility(normalized.Artifact),
		})
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Descriptor.ID < plugins[j].Descriptor.ID
	})
	return stratsrv.PluginCatalog{TargetDir: s.effectiveTargetDirLocked(), Plugins: plugins}
}

func (s *Service) InstallPlugin(pluginID string) (stratsrv.PluginOperation, error) {
	return s.changePluginInstallation(pluginID, true)
}

func (s *Service) UninstallPlugin(pluginID string) (stratsrv.PluginOperation, error) {
	return s.changePluginInstallation(pluginID, false)
}

func (s *Service) changePluginInstallation(pluginID string, installed bool) (stratsrv.PluginOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Plugins {
		plugin := s.normalizePlugin(s.data.Plugins[index])
		if plugin.Descriptor.ID != pluginID {
			continue
		}
		phase := "uninstalled"
		message := "plugin metadata uninstalled"
		status := "NOT_INSTALLED"
		if installed {
			phase = "installed"
			message = "plugin metadata installed"
			status = "INSTALLED"
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		operation := stratsrv.PluginOperation{
			OperationID: buildPluginOperationID(pluginID),
			PluginID:    pluginID,
			Status:      "SUCCEEDED",
			Phase:       phase,
			Progress:    100,
			Message:     message,
			TargetDir:   plugin.Installation.TargetDir,
			InstallPath: plugin.Installation.InstallPath,
			StartedAt:   now,
			UpdatedAt:   now,
			CompletedAt: new(now),
		}
		plugin.Installation.Status = status
		plugin.Installation.Installed = installed
		plugin.Installation.CurrentOperation = nil
		plugin.Installation.LastOperation = &operation
		plugin.Installation.UninstallGuidance = buildPluginUninstallGuidance(plugin.Descriptor.ID, plugin.Installation.InstallPath)
		s.data.Plugins[index] = plugin
		s.data.Operations = append(s.data.Operations, operation)
		return operation, s.persistLocked()
	}
	return stratsrv.PluginOperation{}, stratsrv.NotFoundError("strategy resource not found")
}

func (s *Service) PluginOperation(operationID string) (stratsrv.PluginOperation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, operation := range s.data.Operations {
		if operation.OperationID == operationID {
			return operation, true
		}
	}
	return stratsrv.PluginOperation{}, false
}

func (s *Service) PluginUninstallGuidance(pluginID string) (stratsrv.PluginUninstallGuidance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, plugin := range s.data.Plugins {
		normalized := s.normalizePlugin(plugin)
		if normalized.Descriptor.ID == pluginID {
			return buildPluginUninstallGuidance(pluginID, normalized.Installation.InstallPath), true
		}
	}
	return stratsrv.PluginUninstallGuidance{}, false
}

func buildPluginCompatibility(artifact *PluginArtifact) stratsrv.PluginCompatibility {
	host := currentPluginBuildTuple()
	compatibility := stratsrv.PluginCompatibility{
		Mode:      pluginBuildMode,
		Supported: runtime.GOOS != "windows",
		Host:      host,
	}
	if !compatibility.Supported {
		compatibility.Reason = new("go plugin is unsupported on windows hosts")
	}
	if artifact == nil {
		return compatibility
	}
	artifactBuild := artifact.Build
	compatibility.Artifact = &artifactBuild
	compatibility.RequiresRebuild = !samePluginBuildTuple(host, artifactBuild)
	if compatibility.RequiresRebuild {
		compatibility.Reason = new("artifact build tuple does not match the current jftrade host")
	}
	return compatibility
}

func currentPluginBuildTuple() stratsrv.PluginBuildTuple {
	return stratsrv.PluginBuildTuple{
		JFTradeVersion: currentJFTradeVersion(),
		GoVersion:      runtime.Version(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		BuildMode:      pluginBuildMode,
	}
}

func currentJFTradeVersion() string {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		if version := strings.TrimSpace(buildInfo.Main.Version); version != "" {
			return version
		}
	}
	return "devel"
}

func samePluginBuildTuple(left, right stratsrv.PluginBuildTuple) bool {
	if left.JFTradeVersion != right.JFTradeVersion || left.GoVersion != right.GoVersion ||
		left.GOOS != right.GOOS || left.GOARCH != right.GOARCH || left.BuildMode != right.BuildMode {
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
	return strings.ToLower(strings.ReplaceAll(pluginID, " ", "-")) + "-" +
		time.Now().UTC().Format("20060102150405.000000000")
}

func buildPluginUninstallGuidance(pluginID, installPath string) stratsrv.PluginUninstallGuidance {
	guidance := stratsrv.PluginUninstallGuidance{PluginID: pluginID, Path: installPath}
	guidance.Commands.Posix = "rm -f " + shellQuote(installPath)
	guidance.Commands.PowerShell = "Remove-Item -LiteralPath '" +
		strings.ReplaceAll(installPath, "'", "''") + "' -Force"
	return guidance
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
