package jftradeapi

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStrategyCatalogNormalizePluginAppliesDefaults(t *testing.T) {
	store := &strategyCatalogStore{
		targetDir: "/tmp/jftrade-plugins",
	}

	plugin := managedStrategyPlugin{
		Descriptor: strategyPluginDescriptor{ID: "  demo.plugin  "},
	}

	normalized := store.normalizePlugin(plugin)

	if normalized.Descriptor.ID != "demo.plugin" {
		t.Fatalf("expected trimmed plugin id, got %q", normalized.Descriptor.ID)
	}
	if normalized.Descriptor.Type != pluginTypeGoStrategy {
		t.Fatalf("expected default plugin type %q, got %q", pluginTypeGoStrategy, normalized.Descriptor.Type)
	}
	if normalized.Installation.TargetDir != store.targetDir {
		t.Fatalf("expected target dir %q, got %q", store.targetDir, normalized.Installation.TargetDir)
	}
	expectedInstallPath := filepath.Join(store.targetDir, "demo.plugin.so")
	if normalized.Installation.InstallPath != expectedInstallPath {
		t.Fatalf("expected install path %q, got %q", expectedInstallPath, normalized.Installation.InstallPath)
	}
	if normalized.Installation.Status != "NOT_INSTALLED" {
		t.Fatalf("expected default status NOT_INSTALLED, got %q", normalized.Installation.Status)
	}
}

func TestStrategyCatalogNormalizeStrategyAppliesDefaults(t *testing.T) {
	store := &strategyCatalogStore{}

	normalized := store.normalizeStrategy(managedStrategyInstance{})

	if normalized.PluginID != IDPinePlanPlugin() {
		t.Fatalf("expected plugin id %q, got %q", IDPinePlanPlugin(), normalized.PluginID)
	}
	if normalized.Status != strategyStatusStopped {
		t.Fatalf("expected status %q, got %q", strategyStatusStopped, normalized.Status)
	}
	if normalized.Definition.StrategyID != IDPinePlanPlugin() {
		t.Fatalf("expected definition strategy id %q, got %q", IDPinePlanPlugin(), normalized.Definition.StrategyID)
	}
	if normalized.Definition.Name != IDPinePlanPlugin() {
		t.Fatalf("expected default definition name %q, got %q", IDPinePlanPlugin(), normalized.Definition.Name)
	}
	if normalized.Definition.Version != "0.1.0" {
		t.Fatalf("expected default definition version 0.1.0, got %q", normalized.Definition.Version)
	}
	if runtime, _ := normalized.Params["runtime"].(string); runtime != strategyRuntimePinePlan {
		t.Fatalf("expected runtime %q, got %q", strategyRuntimePinePlan, runtime)
	}
	if sourceFormat, _ := normalized.Params["sourceFormat"].(string); sourceFormat == "" {
		t.Fatalf("expected non-empty source format")
	}
	if normalized.CreatedAt == "" {
		t.Fatal("expected createdAt to be populated")
	}
	if _, err := time.Parse(time.RFC3339Nano, normalized.CreatedAt); err != nil {
		t.Fatalf("expected RFC3339Nano createdAt, got %q: %v", normalized.CreatedAt, err)
	}
	if normalized.Binding.Interval != "5m" {
		t.Fatalf("expected default interval 5m, got %q", normalized.Binding.Interval)
	}
	if normalized.Binding.ExecutionMode != strategyExecutionModeLive {
		t.Fatalf("expected default execution mode %q, got %q", strategyExecutionModeLive, normalized.Binding.ExecutionMode)
	}
}
