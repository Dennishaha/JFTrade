package servercore

import (
	"encoding/json"
	"path/filepath"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyCatalogStoreReloadNormalizesLegacyRowsAndDropsRuntimeOnlyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-catalog.json")
	store, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}

	pluginPayload := mustMarshalCatalogRecovery(t, managedStrategyPlugin{
		Descriptor: strategyPluginDescriptor{ID: "  raw.plugin  "},
	})
	strategyPayload := mustMarshalCatalogRecovery(t, managedStrategyInstance{
		ID: "raw-instance",
		Definition: strategyDefinitionSummary{
			StrategyID: "raw-definition",
			Name:       "Raw Definition",
			Version:    "0.1.0",
		},
		Params: map[string]any{
			"definitionId": "raw-definition",
			"runtime":      strategyRuntimePinePlan,
			"sourceFormat": strategydefinition.SourceFormatPineV6,
			"script":       "//@version=6\nstrategy(\"Raw\", overlay=true)\nstrategy.entry(\"Long\", strategy.long)",
			"symbols":      []string{" us.aapl ", "HK.00700"},
			"interval":     "15m",
		},
		CreatedAt: "2026-06-01T00:00:00Z",
		Logs:      []string{"runtime-only log should not remain in catalog row"},
		AuditEntries: []strategyAuditEntry{{
			InstanceID: "raw-instance",
			Kind:       "runtime-only",
			Detail:     "should not remain in catalog row",
			At:         "2026-06-01T00:00:01Z",
		}},
	})
	operationPayload := mustMarshalCatalogRecovery(t, strategyPluginOperation{
		OperationID: "op-2", PluginID: "raw.plugin", Status: "SUCCEEDED", UpdatedAt: "2026-06-01T00:00:02Z",
	})
	if _, err := store.db.Exec(
		`INSERT INTO `+strategyCatalogPluginTable+` (id, payload_json, updated_at) VALUES (?, ?, ?)`,
		"raw.plugin", pluginPayload, "2026-06-01T00:00:00Z",
	); err != nil {
		t.Fatalf("insert raw plugin: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO `+strategyCatalogStrategyTable+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"raw-instance", strategyPayload, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z",
	); err != nil {
		t.Fatalf("insert raw strategy: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO `+strategyCatalogOperationTable+` (operation_id, plugin_id, status, updated_at, payload_json) VALUES (?, ?, ?, ?, ?)`,
		"op-2", "raw.plugin", "SUCCEEDED", "2026-06-01T00:00:02Z", operationPayload,
	); err != nil {
		t.Fatalf("insert raw operation: %v", err)
	}
	jftradeCheckTestError(t, store.Close())

	reloaded, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("reload NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, reloaded.Close()) })

	catalog := reloaded.pluginCatalog()
	if catalog.TargetDir != "plugins" || len(catalog.Plugins) != 1 {
		t.Fatalf("plugin catalog = %#v", catalog)
	}
	plugin := catalog.Plugins[0]
	if plugin.Descriptor.ID != "raw.plugin" || plugin.Descriptor.Type != pluginTypeGoStrategy || plugin.Installation.TargetDir == "" || plugin.Installation.InstallPath == "" {
		t.Fatalf("normalized plugin = %#v", plugin)
	}
	strategy, ok := reloaded.strategy("raw-instance")
	if !ok {
		t.Fatal("reloaded raw-instance not found")
	}
	if strategy.PluginID != IDPinePlanPlugin() || strategy.Status != strategyStatusStopped || len(strategy.Logs) != 0 || len(strategy.AuditEntries) != 0 {
		t.Fatalf("normalized strategy = %#v", strategy)
	}
	if strategy.Binding.Interval != "15m" || len(strategy.Binding.Symbols) != 2 || strategy.Binding.Symbols[0] != "US.AAPL" {
		t.Fatalf("strategy binding = %#v", strategy.Binding)
	}
	if operation, ok := reloaded.pluginOperation("op-2"); !ok || operation.PluginID != "raw.plugin" {
		t.Fatalf("plugin operation = %#v ok=%v", operation, ok)
	}
}

func TestStrategyCatalogStoreReloadRejectsCorruptCatalogPayloads(t *testing.T) {
	tests := []struct {
		name   string
		insert func(t *testing.T, store *strategyCatalogStore)
	}{
		{
			name: "plugin",
			insert: func(t *testing.T, store *strategyCatalogStore) {
				t.Helper()
				if _, err := store.db.Exec(`INSERT INTO `+strategyCatalogPluginTable+` (id, payload_json, updated_at) VALUES (?, ?, ?)`, "bad-plugin", "{bad-json", "2026-06-01T00:00:00Z"); err != nil {
					t.Fatalf("insert corrupt plugin: %v", err)
				}
			},
		},
		{
			name: "strategy",
			insert: func(t *testing.T, store *strategyCatalogStore) {
				t.Helper()
				if _, err := store.db.Exec(`INSERT INTO `+strategyCatalogStrategyTable+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`, "bad-strategy", "{bad-json", "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z"); err != nil {
					t.Fatalf("insert corrupt strategy: %v", err)
				}
			},
		},
		{
			name: "operation",
			insert: func(t *testing.T, store *strategyCatalogStore) {
				t.Helper()
				if _, err := store.db.Exec(`INSERT INTO `+strategyCatalogOperationTable+` (operation_id, plugin_id, status, updated_at, payload_json) VALUES (?, ?, ?, ?, ?)`, "bad-operation", "plugin", "FAILED", "2026-06-01T00:00:00Z", "{bad-json"); err != nil {
					t.Fatalf("insert corrupt operation: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "strategy-catalog.json")
			store, err := NewStrategyCatalogStore(path, "plugins")
			if err != nil {
				t.Fatalf("NewStrategyCatalogStore: %v", err)
			}
			tt.insert(t, store)
			jftradeCheckTestError(t, store.Close())

			if reloaded, err := NewStrategyCatalogStore(path, "plugins"); err == nil {
				jftradeCheckTestError(t, reloaded.Close())
				t.Fatal("reload err=nil, want corrupt payload error")
			}
		})
	}
}

func mustMarshalCatalogRecovery(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal catalog recovery payload: %v", err)
	}
	return string(payload)
}
