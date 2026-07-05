package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStrategyCatalogStorePluginUpdateAndMissingOperations(t *testing.T) {
	var nilStore *strategyCatalogStore
	if err := nilStore.Close(); err != nil {
		t.Fatalf("nil strategy catalog close: %v", err)
	}

	path := filepath.Join(t.TempDir(), "strategy-catalog.json")
	store, err := NewStrategyCatalogStore(path, "")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if store.pluginCatalog().TargetDir != defaultStrategyPluginDirName {
		t.Fatalf("default target dir = %q", store.pluginCatalog().TargetDir)
	}

	plugin := managedStrategyPlugin{Descriptor: strategyPluginDescriptor{ID: "plugin.alpha", DisplayName: "Alpha", Version: "1.0.0"}}
	if err := store.savePlugin(plugin); err != nil {
		t.Fatalf("save initial plugin: %v", err)
	}
	plugin.Descriptor.DisplayName = "Alpha Updated"
	plugin.Descriptor.Version = "1.1.0"
	if err := store.savePlugin(plugin); err != nil {
		t.Fatalf("update plugin: %v", err)
	}
	catalog := store.pluginCatalog()
	if len(catalog.Plugins) != 1 || catalog.Plugins[0].Descriptor.DisplayName != "Alpha Updated" || catalog.Plugins[0].Descriptor.Version != "1.1.0" {
		t.Fatalf("updated plugin catalog = %#v", catalog)
	}

	if _, err := store.installPlugin("missing-plugin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install missing plugin err = %v", err)
	}
	if _, err := store.uninstallPlugin("missing-plugin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uninstall missing plugin err = %v", err)
	}
	if operation, ok := store.pluginOperation("missing-operation"); ok || operation.OperationID != "" {
		t.Fatalf("missing operation = %#v/%v", operation, ok)
	}
	if guidance, ok := store.pluginUninstallGuidance("missing-plugin"); ok || guidance.PluginID != "" {
		t.Fatalf("missing guidance = %#v/%v", guidance, ok)
	}
}

func TestStrategyCatalogStoreFindsStrategiesLinkedToDefinition(t *testing.T) {
	store, err := NewStrategyCatalogStore(filepath.Join(t.TempDir(), "strategy-catalog.json"), "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	strategies := []managedStrategyInstance{
		{ID: "linked-by-param", Params: map[string]any{"definitionId": "definition-a"}},
		{ID: "linked-by-summary", Definition: strategyDefinitionSummary{StrategyID: "definition-a"}},
		{ID: "unrelated", Definition: strategyDefinitionSummary{StrategyID: "definition-b"}},
	}
	for _, strategy := range strategies {
		if err := store.saveStrategy(strategy); err != nil {
			t.Fatalf("saveStrategy(%s): %v", strategy.ID, err)
		}
	}
	linked := store.linkedStrategyInstanceIDs(" definition-a ")
	if len(linked) != 2 || linked[0] != "linked-by-param" || linked[1] != "linked-by-summary" {
		t.Fatalf("linked strategies = %#v", linked)
	}
	if linked := store.linkedStrategyInstanceIDs(" "); len(linked) != 0 {
		t.Fatalf("blank definition linked strategies = %#v", linked)
	}
}

func TestStrategyCatalogDerivedPathsForBareAndNestedSettings(t *testing.T) {
	if got := deriveStrategyCatalogPath("settings.json"); got != defaultStrategyCatalogFilename {
		t.Fatalf("bare catalog path = %q", got)
	}
	if got := deriveStrategyPluginTargetDir("settings.json"); got != defaultStrategyPluginDirName {
		t.Fatalf("bare plugin dir = %q", got)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if got := deriveStrategyCatalogPath(settingsPath); got != filepath.Join(filepath.Dir(settingsPath), defaultStrategyCatalogFilename) {
		t.Fatalf("nested catalog path = %q", got)
	}
	if got := deriveStrategyPluginTargetDir(settingsPath); got != filepath.Join(filepath.Dir(settingsPath), defaultStrategyPluginDirName) {
		t.Fatalf("nested plugin dir = %q", got)
	}
}
