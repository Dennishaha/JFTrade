package servercore

import (
	"path/filepath"
	"strings"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

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
	if input.PluginID == "" {
		input.PluginID = IDPinePlanPlugin()
	}
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	if runtime := jftradeOptionalTypeAssertion[string](input.Params["runtime"]); strings.TrimSpace(runtime) == "" {
		input.Params["runtime"] = strategyRuntimePinePlan
	} else {
		input.Params["runtime"] = pineworker.NormalizeRuntime(runtime)
	}
	if sourceFormat := jftradeOptionalTypeAssertion[string](input.Params["sourceFormat"]); strings.TrimSpace(sourceFormat) == "" {
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
		input.Definition.Version = "0.1.0"
	}
	if script := jftradeOptionalTypeAssertion[string](input.Params["script"]); strings.TrimSpace(script) == "" {
		input.Params["script"] = defaultStrategyDesignPine(input.Definition.Name)
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
