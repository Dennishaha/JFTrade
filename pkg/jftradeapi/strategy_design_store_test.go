package jftradeapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func countStrategyDesignDefinitionRows(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM `+strategyDesignDefinitionTable); err != nil {
		t.Fatalf("count design definitions: %v", err)
	}
	return count
}

func readStrategyDesignDefinitionRow(t *testing.T, dbPath string, id string) strategyDesignDefinitionRow {
	t.Helper()
	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

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
	t.Cleanup(func() { store.Close() })

	if got := store.listDefinitions(); len(got) != 0 {
		t.Fatalf("expected legacy json definitions to be ignored, got %+v", got)
	}
	if _, ok := store.definition("legacy-ma-strategy"); ok {
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
	t.Cleanup(func() { store.Close() })

	created, err := store.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-versioned",
		Name:         "Versioned Strategy",
		Version:      "9.9.9",
		Description:  "first save",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy Versioned Strategy\nversion 9.9.9\non init:\n  log \"init\"\non kline_close:\n  log \"close\"",
	})
	if err != nil {
		t.Fatalf("saveDefinition(create): %v", err)
	}
	if created.Version != defaultStrategyVersion {
		t.Fatalf("created version = %q, want %q", created.Version, defaultStrategyVersion)
	}
	if !strings.Contains(created.Script, "version "+defaultStrategyVersion) {
		t.Fatalf("expected created script version to sync to %s, got %q", defaultStrategyVersion, created.Script)
	}

	updated, err := store.saveDefinition(strategyDesignDefinition{
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
	if !strings.Contains(updated.Script, "version 0.1.1") {
		t.Fatalf("expected updated script version to sync to 0.1.1, got %q", updated.Script)
	}

	unchanged, err := store.saveDefinition(strategyDesignDefinition{
		ID:           updated.ID,
		Name:         updated.Name,
		Version:      "88.88.88",
		Description:  updated.Description,
		Runtime:      updated.Runtime,
		SourceFormat: updated.SourceFormat,
		Script:       strings.Replace(updated.Script, "version 0.1.1", "version 88.88.88", 1),
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
	if !strings.Contains(row.Script, "version 0.1.1") {
		t.Fatalf("expected persisted script version to sync to 0.1.1, got %q", row.Script)
	}
}

func TestStrategyDesignStoreGeneratesUUIDWhenIDMissing(t *testing.T) {
	store, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	created, err := store.saveDefinition(strategyDesignDefinition{
		Name:         "UUID Strategy",
		Description:  "id generated by store",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy UUID Strategy\nversion 0.1.0\non init:\n  log \"init\"\non kline_close:\n  log \"close\"",
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
	t.Cleanup(func() { store.Close() })

	created, err := store.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-delete-me",
		Name:         "Delete Me",
		Description:  "soft delete target",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy Delete Me\nversion 0.1.0\non init:\n  log \"init\"\non kline_close:\n  log \"close\"",
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
	if _, ok := store.definition(created.ID); ok {
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
