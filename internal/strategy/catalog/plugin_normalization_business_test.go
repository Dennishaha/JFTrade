package catalog

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestCatalogPluginLifecyclePersistsSortedMetadataAndOperations(t *testing.T) {
	service, repository, _ := newCatalogBusinessService(t, Snapshot{})
	for _, plugin := range []ManagedPlugin{
		{Descriptor: stratsrv.PluginDescriptor{ID: " plugin.zeta "}},
		{Descriptor: stratsrv.PluginDescriptor{ID: "plugin.alpha", DisplayName: "Alpha", Version: "1.0.0"}},
	} {
		if err := service.RegisterPlugin(plugin); err != nil {
			t.Fatalf("RegisterPlugin(%q): %v", plugin.Descriptor.ID, err)
		}
	}
	if err := service.RegisterPlugin(ManagedPlugin{
		Descriptor: stratsrv.PluginDescriptor{ID: "plugin.alpha", DisplayName: "Alpha Updated", Version: "1.1.0"},
	}); err != nil {
		t.Fatalf("update plugin: %v", err)
	}

	plugins := service.PluginCatalog()
	if len(plugins.Plugins) != 2 ||
		plugins.Plugins[0].Descriptor.ID != "plugin.alpha" ||
		plugins.Plugins[1].Descriptor.ID != "plugin.zeta" {
		t.Fatalf("sorted plugins = %#v", plugins.Plugins)
	}
	if plugins.Plugins[0].Descriptor.DisplayName != "Alpha Updated" ||
		plugins.Plugins[0].Descriptor.Version != "1.1.0" {
		t.Fatalf("updated plugin = %#v", plugins.Plugins[0])
	}
	if plugins.Plugins[1].Descriptor.Type != pluginType ||
		plugins.Plugins[1].Descriptor.DisplayName != "plugin.zeta" ||
		plugins.Plugins[1].Descriptor.Version != stratsrv.DefaultVersion {
		t.Fatalf("normalized plugin = %#v", plugins.Plugins[1])
	}

	installed, err := service.InstallPlugin("plugin.alpha")
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	if installed.Status != "SUCCEEDED" || installed.Phase != "installed" || installed.Progress != 100 {
		t.Fatalf("install operation = %#v", installed)
	}
	if stored, ok := service.PluginOperation(installed.OperationID); !ok || stored.PluginID != "plugin.alpha" {
		t.Fatalf("stored install operation = %#v, found=%v", stored, ok)
	}
	plugins = service.PluginCatalog()
	if !plugins.Plugins[0].Installation.Installed ||
		plugins.Plugins[0].Installation.Status != "INSTALLED" ||
		plugins.Plugins[0].Installation.LastOperation == nil {
		t.Fatalf("installed metadata = %#v", plugins.Plugins[0].Installation)
	}

	uninstalled, err := service.UninstallPlugin("plugin.alpha")
	if err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}
	if uninstalled.Phase != "uninstalled" {
		t.Fatalf("uninstall operation = %#v", uninstalled)
	}
	plugins = service.PluginCatalog()
	if plugins.Plugins[0].Installation.Installed || plugins.Plugins[0].Installation.Status != "NOT_INSTALLED" {
		t.Fatalf("uninstalled metadata = %#v", plugins.Plugins[0].Installation)
	}
	if repository.saveCount() != 5 {
		t.Fatalf("plugin lifecycle save count = %d, want 5", repository.saveCount())
	}

	guidance, ok := service.PluginUninstallGuidance("plugin.alpha")
	if !ok || guidance.PluginID != "plugin.alpha" || guidance.Path == "" {
		t.Fatalf("uninstall guidance = %#v, found=%v", guidance, ok)
	}
	if operation, ok := service.PluginOperation("missing"); ok || operation.OperationID != "" {
		t.Fatalf("missing operation = %#v, found=%v", operation, ok)
	}
	if guidance, ok := service.PluginUninstallGuidance("missing"); ok || guidance.PluginID != "" {
		t.Fatalf("missing guidance = %#v, found=%v", guidance, ok)
	}
}

func TestCatalogPluginLifecycleClassifiesMissingResource(t *testing.T) {
	service, _, _ := newCatalogBusinessService(t, Snapshot{})
	if _, err := service.InstallPlugin("missing"); err == nil {
		t.Fatal("InstallPlugin missing error = nil")
	}
	if _, err := service.UninstallPlugin("missing"); err == nil {
		t.Fatal("UninstallPlugin missing error = nil")
	}
}

func TestCatalogNormalizesLegacySnapshotAndDropsRuntimeOnlyFields(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "plugins")
	repository := &catalogMemoryRepository{snapshot: Snapshot{
		Plugins: []ManagedPlugin{{
			Descriptor: stratsrv.PluginDescriptor{ID: " raw.plugin "},
			Artifact:   &PluginArtifact{},
		}},
		Strategies: []stratsrv.ManagedInstance{{
			ID: "legacy",
			Params: map[string]any{
				"runtime":      stratsrv.RuntimePinePlan,
				"sourceFormat": "",
				"symbols":      []string{" us:aapl ", "HK.00700"},
				"interval":     "15m",
			},
			Logs: []string{"runtime-only"},
			AuditEntries: []stratsrv.AuditEntry{{
				Kind: "runtime-only",
			}},
		}},
	}}
	service, err := New(repository, nil, targetDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	plugin := service.PluginCatalog().Plugins[0]
	if plugin.Descriptor.ID != "raw.plugin" || plugin.Descriptor.Type != pluginType {
		t.Fatalf("normalized descriptor = %#v", plugin.Descriptor)
	}
	if plugin.Installation.TargetDir != targetDir ||
		plugin.Installation.InstallPath != filepath.Join(targetDir, "raw.plugin.so") ||
		plugin.Installation.MarkerPath != filepath.Join(targetDir, "raw.plugin.json") {
		t.Fatalf("normalized installation = %#v", plugin.Installation)
	}
	if plugin.Compatibility.Artifact == nil || plugin.Compatibility.Artifact.BuildMode != pluginBuildMode {
		t.Fatalf("normalized artifact compatibility = %#v", plugin.Compatibility)
	}

	instance, ok := service.GetInstance("legacy")
	if !ok {
		t.Fatal("normalized legacy instance missing")
	}
	if instance.PluginID == "" || instance.Status != StatusStopped ||
		instance.Definition.Version != stratsrv.DefaultVersion ||
		len(instance.Logs) != 0 || len(instance.AuditEntries) != 0 {
		t.Fatalf("normalized instance = %#v", instance)
	}
	if instance.Binding.Interval != "15m" ||
		len(instance.Binding.Symbols) != 2 ||
		instance.Binding.Symbols[0] != "US.AAPL" {
		t.Fatalf("normalized binding = %#v", instance.Binding)
	}
	if _, err := time.Parse(time.RFC3339Nano, instance.CreatedAt); err != nil {
		t.Fatalf("createdAt = %q: %v", instance.CreatedAt, err)
	}
}

func TestCatalogNormalizationPreservesExplicitUnsupportedRuntimeForValidation(t *testing.T) {
	service, _, _ := newCatalogBusinessService(t, Snapshot{Strategies: []stratsrv.ManagedInstance{{
		ID:       "legacy-runtime",
		PluginID: "removed-script-runtime",
		Definition: stratsrv.DefinitionSummary{
			StrategyID: "legacy-definition",
			Name:       "Legacy",
			Version:    "1.0.0",
		},
		Params: map[string]any{
			"runtime":      "removed-script-runtime",
			"sourceFormat": "removed-script-source",
			"script":       "function onInit(ctx) { return ctx.symbol }",
		},
	}}})
	instance, ok := service.GetInstance("legacy-runtime")
	if !ok {
		t.Fatal("legacy runtime missing")
	}
	if instance.PluginID != "removed-script-runtime" ||
		instance.Params["runtime"] != "removed-script-runtime" ||
		instance.Params["sourceFormat"] != "removed-script-source" {
		t.Fatalf("explicit legacy values changed = %#v", instance)
	}
	if err := service.ValidateStartable(instance); err == nil {
		t.Fatal("unsupported legacy runtime should not be startable")
	}
}

func TestCatalogPluginCompatibilityAndUninstallCommandsDescribeHostBoundary(t *testing.T) {
	host := currentPluginBuildTuple()
	compatible := buildPluginCompatibility(&PluginArtifact{Path: "plugin.so", Build: host})
	if compatible.Supported != (runtime.GOOS != "windows") ||
		compatible.RequiresRebuild ||
		compatible.Artifact == nil {
		t.Fatalf("compatible artifact = %#v", compatible)
	}

	stale := host
	stale.GoVersion = "go0.0"
	drifted := buildPluginCompatibility(&PluginArtifact{Path: "plugin.so", Build: stale})
	if !drifted.RequiresRebuild || drifted.Reason == nil {
		t.Fatalf("drifted artifact = %#v", drifted)
	}
	withTags := host
	withTags.BuildTags = []string{"netgo", "sqlite"}
	if !samePluginBuildTuple(withTags, withTags) {
		t.Fatal("identical build tuple should match")
	}
	reordered := withTags
	reordered.BuildTags = []string{"sqlite", "netgo"}
	if samePluginBuildTuple(withTags, reordered) {
		t.Fatal("reordered build tags should require rebuild")
	}

	operationID := buildPluginOperationID("My Plugin")
	if !strings.HasPrefix(operationID, "my-plugin-") || strings.Contains(operationID, " ") {
		t.Fatalf("operation ID = %q", operationID)
	}
	guidance := buildPluginUninstallGuidance("plugin", "/tmp/O'Brien/plugin.so")
	if !strings.Contains(guidance.Commands.Posix, "'\\''") ||
		!strings.Contains(guidance.Commands.PowerShell, "O''Brien") {
		t.Fatalf("shell guidance = %#v", guidance.Commands)
	}
}
