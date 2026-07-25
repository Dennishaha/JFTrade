package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func countStrategyDesignDefinitionRows(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() { jftradeCheckTestError(t, db.Close()) }()

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM `+strategyDesignDefinitionTable); err != nil {
		t.Fatalf("count design definitions: %v", err)
	}
	return count
}

func countStrategyDesignDefinitionVersionRows(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() { jftradeCheckTestError(t, db.Close()) }()

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM `+strategyDesignDefinitionVersionTable); err != nil {
		t.Fatalf("count design definition versions: %v", err)
	}
	return count
}

func readStrategyDesignDefinitionRow(t *testing.T, dbPath string, id string) strategyDesignDefinitionRow {
	t.Helper()
	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() { jftradeCheckTestError(t, db.Close()) }()

	var row strategyDesignDefinitionRow
	if err := db.Get(&row,
		`SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at `+
			`FROM `+strategyDesignDefinitionTable+` WHERE id = ?`,
		id,
	); err != nil {
		t.Fatalf("read design definition row %s: %v", id, err)
	}
	return row
}

func legacyVisualModelWithBlockKind(blockKind string) *stratsrv.VisualModel {
	return &stratsrv.VisualModel{
		Engine:  "logic-flow",
		Version: 1,
		Nodes: []stratsrv.VisualNode{
			{
				ID:   "legacy-node",
				Type: "rect",
				X:    260,
				Y:    100,
				Text: "Legacy Node",
				Properties: map[string]any{
					"blockKind": blockKind,
				},
			},
		},
	}
}

func TestStrategyDesignStoreIgnoresLegacyJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-definitions.json")
	legacy := `{
	  "definitions": [
	    {
	      "id": "legacy-ma-strategy",
	      "name": "Legacy MA",
	      "version": "0.1.0",
	      "description": "legacy builder payload",
	      "runtime": "legacy-runtime",
	      "sourceFormat": "legacy-v0",
	      "symbol": "00700",
	      "interval": "1m",
	      "script": "strategy Legacy MA\non kline_close:\n  log \"close\"",
	      "createdAt": "2026-05-26T00:00:00Z",
	      "updatedAt": "2026-05-26T00:00:00Z"
	    }
	  ]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy definitions: %v", err)
	}

	store, err := NewStrategyDesignStore(path)
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	if got := store.listDefinitions(); len(got) != 0 {
		t.Fatalf("expected legacy json definitions to be ignored, got %+v", got)
	}
	if _, ok, err := store.definition("legacy-ma-strategy"); err != nil || ok {
		t.Fatal("expected legacy json definition to be ignored")
	}
	if got := countStrategyDesignDefinitionRows(t, store.dbPath); got != 0 {
		t.Fatalf("db definition row count = %d, want 0", got)
	}
	if persisted, err := os.ReadFile(path); err != nil {
		t.Fatalf("read legacy file: %v", err)
	} else if string(persisted) != legacy {
		t.Fatalf("expected legacy json file to remain untouched, got %s", string(persisted))
	}
}

func TestStrategyDesignStoreSaveDefinitionManagesVersionAndScriptMetadata(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	created, err := store.saveDefinition(stratsrv.Definition{
		ID:           "dsl-versioned",
		Name:         "Versioned Strategy",
		Version:      "9.9.9",
		Description:  "first save",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Versioned Strategy\", overlay=true)\nlog.info(\"close\")",
	})
	if err != nil {
		t.Fatalf("saveDefinition(create): %v", err)
	}
	if created.Version != defaultStrategyVersion {
		t.Fatalf("created version = %q, want %q", created.Version, defaultStrategyVersion)
	}
	if !strings.Contains(created.Script, "strategy(\"Versioned Strategy\"") {
		t.Fatalf("expected created Pine script to be preserved, got %q", created.Script)
	}

	updated, err := store.saveDefinition(stratsrv.Definition{
		ID:           created.ID,
		Name:         created.Name,
		Version:      created.Version,
		Description:  "second save",
		Runtime:      created.Runtime,
		SourceFormat: created.SourceFormat,
		Script:       created.Script,
		CreatedAt:    created.CreatedAt,
		UpdatedAt:    created.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("saveDefinition(update): %v", err)
	}
	if updated.Version != "0.1.1" {
		t.Fatalf("updated version = %q, want 0.1.1", updated.Version)
	}
	if updated.Script != created.Script {
		t.Fatalf("expected updated Pine script to stay unchanged, got %q", updated.Script)
	}

	unchanged, err := store.saveDefinition(stratsrv.Definition{
		ID:           updated.ID,
		Name:         updated.Name,
		Version:      "88.88.88",
		Description:  updated.Description,
		Runtime:      updated.Runtime,
		SourceFormat: updated.SourceFormat,
		Script:       updated.Script,
		CreatedAt:    updated.CreatedAt,
		UpdatedAt:    updated.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("saveDefinition(unchanged): %v", err)
	}
	if unchanged.Version != "0.1.1" {
		t.Fatalf("unchanged version = %q, want 0.1.1", unchanged.Version)
	}
	if unchanged.UpdatedAt != updated.UpdatedAt {
		t.Fatalf("unchanged UpdatedAt = %q, want %q", unchanged.UpdatedAt, updated.UpdatedAt)
	}
	row := readStrategyDesignDefinitionRow(t, store.dbPath, updated.ID)
	if row.Version != "0.1.1" {
		t.Fatalf("persisted version = %q, want 0.1.1", row.Version)
	}
	if row.Script != updated.Script {
		t.Fatalf("expected persisted Pine script to stay unchanged, got %q", row.Script)
	}
}

func TestStrategyDesignStorePersistsImmutableDefinitionVersionSnapshots(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	created, err := store.saveDefinition(stratsrv.Definition{
		ID:           "version-history",
		Name:         "Version History",
		Description:  "first saved description",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "5m",
		Script:       "//@version=6\nstrategy(\"Version History\", overlay=true)\nlog.info(\"first\")",
	})
	if err != nil {
		t.Fatalf("saveDefinition(create): %v", err)
	}
	if got := countStrategyDesignDefinitionVersionRows(t, store.dbPath); got != 1 {
		t.Fatalf("snapshot count after create = %d, want 1", got)
	}

	updatedInput := created
	updatedInput.Description = "second saved description"
	updatedInput.Script = "//@version=6\nstrategy(\"Version History\", overlay=true)\nlog.info(\"second\")"
	updated, err := store.saveDefinition(updatedInput)
	if err != nil {
		t.Fatalf("saveDefinition(update): %v", err)
	}
	if updated.Version != "0.1.1" {
		t.Fatalf("updated version = %q, want 0.1.1", updated.Version)
	}

	unchanged, err := store.saveDefinition(updated)
	if err != nil {
		t.Fatalf("saveDefinition(unchanged): %v", err)
	}
	if unchanged.Version != updated.Version {
		t.Fatalf("unchanged version = %q, want %q", unchanged.Version, updated.Version)
	}
	if got := countStrategyDesignDefinitionVersionRows(t, store.dbPath); got != 2 {
		t.Fatalf("snapshot count after unchanged save = %d, want 2", got)
	}

	versions, ok, err := store.listDefinitionVersions(created.ID)
	if err != nil || !ok {
		t.Fatalf("listDefinitionVersions = (%+v, %t, %v)", versions, ok, err)
	}
	if len(versions) != 2 || versions[0].Version != "0.1.1" || !versions[0].IsCurrent || versions[1].Version != "0.1.0" || versions[1].IsCurrent {
		t.Fatalf("version summaries = %+v", versions)
	}
	if versions[0].SavedAt == "" || versions[1].SavedAt == "" {
		t.Fatalf("version summaries are missing savedAt: %+v", versions)
	}

	first, ok, err := store.definitionVersion(created.ID, "0.1.0")
	if err != nil || !ok {
		t.Fatalf("definitionVersion(0.1.0) = (%+v, %t, %v)", first, ok, err)
	}
	if first.DefinitionID != created.ID || first.ID != created.ID || first.Description != created.Description || first.Script != created.Script || first.IsCurrent {
		t.Fatalf("first snapshot = %+v, want immutable creation snapshot", first)
	}

	if _, err := store.db.Exec(`UPDATE `+strategyDesignDefinitionVersionTable+` SET name = 'changed' WHERE definition_id = ? AND version = ?`, created.ID, "0.1.0"); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("update immutable snapshot error = %v", err)
	}

	if _, err := store.deleteDefinition(created.ID); err != nil {
		t.Fatalf("deleteDefinition: %v", err)
	}
	versions, ok, err = store.listDefinitionVersions(created.ID)
	if err != nil || !ok || len(versions) != 2 || versions[0].IsCurrent || versions[1].IsCurrent {
		t.Fatalf("soft-deleted version history = (%+v, %t, %v)", versions, ok, err)
	}
	if purged, err := store.purgeDeletedDefinitions(t.Context(), []string{created.ID}); err != nil || purged != 1 {
		t.Fatalf("purgeDeletedDefinitions = (%d, %v), want (1, nil)", purged, err)
	}
	if _, ok, err := store.listDefinitionVersions(created.ID); err != nil || ok {
		t.Fatalf("version history after hard purge = (%t, %v), want (false, nil)", ok, err)
	}
	if got := countStrategyDesignDefinitionVersionRows(t, store.dbPath); got != 0 {
		t.Fatalf("snapshot count after hard purge = %d, want 0", got)
	}
}

func TestStrategyDesignStoreRollsBackDefinitionWhenSnapshotInsertFails(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	created, err := store.saveDefinition(stratsrv.Definition{
		ID:           "atomic-history",
		Name:         "Atomic History",
		Description:  "before failed update",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Atomic History\", overlay=true)",
	})
	if err != nil {
		t.Fatalf("saveDefinition(create): %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_strategy_definition_snapshot_insert
		BEFORE INSERT ON ` + strategyDesignDefinitionVersionTable + `
		WHEN NEW.version = '0.1.1'
		BEGIN
			SELECT RAISE(ABORT, 'snapshot insert rejected');
		END`); err != nil {
		t.Fatalf("create rejection trigger: %v", err)
	}

	failed := created
	failed.Description = "must not be persisted"
	if _, err := store.saveDefinition(failed); err == nil || !strings.Contains(err.Error(), "snapshot insert rejected") {
		t.Fatalf("saveDefinition(failed snapshot) error = %v", err)
	}
	current, ok, err := store.definition(created.ID)
	if err != nil || !ok {
		t.Fatalf("definition after failed snapshot = (%+v, %t, %v)", current, ok, err)
	}
	if current.Version != "0.1.0" || current.Description != created.Description {
		t.Fatalf("current definition changed despite failed snapshot: %+v", current)
	}
	if got := countStrategyDesignDefinitionVersionRows(t, store.dbPath); got != 1 {
		t.Fatalf("snapshot count after failed update = %d, want 1", got)
	}
}

func TestStrategyDesignStoreRejectsV1DatabaseWithoutMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-definitions.json")
	store, err := NewStrategyDesignStore(path)
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	dbPath := store.dbPath
	if err := store.Close(); err != nil {
		t.Fatalf("close current store: %v", err)
	}

	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if _, err := db.Exec(`UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy'`); err != nil {
		_ = db.Close()
		t.Fatalf("downgrade schema metadata: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite db: %v", err)
	}

	if migrated, err := NewStrategyDesignStore(path); err == nil || migrated != nil || !strings.Contains(err.Error(), "schema version 1 does not match required version 2") || !strings.Contains(err.Error(), "rebuild database") {
		t.Fatalf("NewStrategyDesignStore(v1) = (%#v, %v), want strict rebuild error", migrated, err)
	}
}

func TestStrategyDesignStoreRejectsLegacyRuntimeSourceAndVisualBlocks(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	tests := []struct {
		name       string
		definition stratsrv.Definition
	}{
		{
			name: "explicit legacy runtime",
			definition: stratsrv.Definition{
				ID:           "legacy-runtime",
				Name:         "Legacy Runtime",
				Runtime:      "legacy-runtime",
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script:       "//@version=6\nstrategy(\"Legacy Runtime\", overlay=true)\nlog.info(\"close\")",
			},
		},
		{
			name: "explicit legacy source format",
			definition: stratsrv.Definition{
				ID:           "legacy-source",
				Name:         "Legacy Source",
				Runtime:      strategyRuntimePinePlan,
				SourceFormat: "legacy-js",
				Script:       "//@version=6\nstrategy(\"Legacy Source\", overlay=true)\nlog.info(\"close\")",
			},
		},
		{
			name: "legacy codeBlock",
			definition: stratsrv.Definition{
				ID:           "legacy-codeblock",
				Name:         "Legacy CodeBlock",
				Runtime:      strategyRuntimePinePlan,
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script:       "//@version=6\nstrategy(\"Legacy CodeBlock\", overlay=true)\nlog.info(\"close\")",
				VisualModel:  legacyVisualModelWithBlockKind("codeBlock"),
			},
		},
		{
			name: "legacy unified technicalIndicator",
			definition: stratsrv.Definition{
				ID:           "legacy-indicator",
				Name:         "Legacy Indicator",
				Runtime:      strategyRuntimePinePlan,
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script:       "//@version=6\nstrategy(\"Legacy Indicator\", overlay=true)\nlog.info(\"close\")",
				VisualModel:  legacyVisualModelWithBlockKind("technicalIndicator"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.saveDefinition(test.definition); !errors.Is(err, errUnsupportedLegacyStrategyDefinition) {
				t.Fatalf("saveDefinition error = %v, want unsupported legacy strategy definition", err)
			}
		})
	}
	if got := countStrategyDesignDefinitionRows(t, store.dbPath); got != 0 {
		t.Fatalf("db definition row count = %d, want 0", got)
	}
}

func TestStrategyDesignStoreGeneratesUUIDWhenIDMissing(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	created, err := store.saveDefinition(stratsrv.Definition{
		Name:         "UUID Strategy",
		Description:  "id generated by store",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"UUID Strategy\", overlay=true)\nlog.info(\"close\")",
	})
	if err != nil {
		t.Fatalf("saveDefinition(create without id): %v", err)
	}

	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(created.ID) {
		t.Fatalf("created id = %q, want uuid", created.ID)
	}
	if created.Name != "UUID Strategy" {
		t.Fatalf("created name = %q, want UUID Strategy", created.Name)
	}
}

func TestStrategyDesignStoreDeleteDefinitionSoftDeletes(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	created, err := store.saveDefinition(stratsrv.Definition{
		ID:           "dsl-delete-me",
		Name:         "Delete Me",
		Description:  "soft delete target",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Delete Me\", overlay=true)\nlog.info(\"close\")",
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}

	deleted, err := store.deleteDefinition(created.ID)
	if err != nil {
		t.Fatalf("deleteDefinition: %v", err)
	}
	if deleted.ID != created.ID {
		t.Fatalf("deleted id = %q, want %q", deleted.ID, created.ID)
	}
	if _, ok, err := store.definition(created.ID); err != nil || ok {
		t.Fatal("expected soft-deleted definition to be hidden from definition lookup")
	}
	if got := store.listDefinitions(); len(got) != 0 {
		t.Fatalf("expected soft-deleted definition to be hidden from list, got %+v", got)
	}
	if got := countStrategyDesignDefinitionRows(t, store.dbPath); got != 1 {
		t.Fatalf("db row count after soft delete = %d, want 1", got)
	}
	row := readStrategyDesignDefinitionRow(t, store.dbPath, created.ID)
	if !row.DeletedAt.Valid || strings.TrimSpace(row.DeletedAt.String) == "" {
		t.Fatalf("expected deleted_at to be populated, got %+v", row.DeletedAt)
	}
}
