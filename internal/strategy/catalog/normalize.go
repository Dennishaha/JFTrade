package catalog

import (
	"path/filepath"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	instanceview "github.com/jftrade/jftrade-main/internal/strategy/instanceview"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func normalizeTargetDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultPluginDir
	}
	return value
}

func (s *Service) normalizeSnapshot(input Snapshot) Snapshot {
	input = cloneSnapshot(input)
	input.TargetDir = strings.TrimSpace(input.TargetDir)
	if input.TargetDir == "" {
		input.TargetDir = s.targetDir
	}
	if input.Plugins == nil {
		input.Plugins = []ManagedPlugin{}
	}
	for index := range input.Plugins {
		input.Plugins[index] = s.normalizePlugin(input.Plugins[index])
	}
	if input.Strategies == nil {
		input.Strategies = []stratsrv.ManagedInstance{}
	}
	for index := range input.Strategies {
		input.Strategies[index] = s.normalizeStrategy(input.Strategies[index])
		input.Strategies[index].Logs = nil
		input.Strategies[index].AuditEntries = nil
	}
	if input.Operations == nil {
		input.Operations = []stratsrv.PluginOperation{}
	}
	return input
}

func (s *Service) normalizePlugin(input ManagedPlugin) ManagedPlugin {
	input = clonePlugin(input)
	input.Descriptor.ID = strings.TrimSpace(input.Descriptor.ID)
	if input.Descriptor.Type == "" {
		input.Descriptor.Type = pluginType
	}
	if input.Descriptor.DisplayName == "" {
		input.Descriptor.DisplayName = input.Descriptor.ID
	}
	if input.Descriptor.Version == "" {
		input.Descriptor.Version = stratsrv.DefaultVersion
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

func (s *Service) normalizeStrategy(input stratsrv.ManagedInstance) stratsrv.ManagedInstance {
	input = cloneInstance(input)
	if input.ID == "" {
		input.ID = "strategy-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	if input.PluginID == "" {
		input.PluginID = instanceview.DefaultPluginID
	}
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	if runtime, _ := input.Params["runtime"].(string); strings.TrimSpace(runtime) == "" {
		input.Params["runtime"] = stratsrv.RuntimePinePlan
	} else {
		input.Params["runtime"] = pineworker.NormalizeRuntime(runtime)
	}
	if sourceFormat, _ := input.Params["sourceFormat"].(string); strings.TrimSpace(sourceFormat) == "" {
		input.Params["sourceFormat"] = strategydefinition.SourceFormatPineV6
	} else {
		input.Params["sourceFormat"] = strings.TrimSpace(sourceFormat)
	}
	if input.Definition.StrategyID == "" {
		input.Definition.StrategyID = input.PluginID
	}
	if input.Definition.Name == "" {
		input.Definition.Name = input.PluginID
	}
	if input.Definition.Version == "" {
		input.Definition.Version = stratsrv.DefaultVersion
	}
	if script, _ := input.Params["script"].(string); strings.TrimSpace(script) == "" {
		input.Params["script"] = stratsrv.DefaultPine(input.Definition.Name)
	}
	if input.Status == "" {
		input.Status = StatusStopped
	}
	if input.CreatedAt == "" {
		input.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return instanceview.NormalizeManagedInstance(input)
}

func (s *Service) effectiveTargetDirLocked() string {
	if strings.TrimSpace(s.data.TargetDir) != "" {
		return s.data.TargetDir
	}
	return normalizeTargetDir(s.targetDir)
}

func definitionIDFromParams(params map[string]any) string {
	value, _ := params["definitionId"].(string)
	return strings.TrimSpace(value)
}

func instanceUsesDefinition(instance stratsrv.ManagedInstance, definitionID string) bool {
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return false
	}
	return definitionIDFromParams(instance.Params) == definitionID ||
		strings.TrimSpace(instance.Definition.StrategyID) == definitionID
}
