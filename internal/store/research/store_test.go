package research

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/jftrade/jftrade-main/internal/research"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestStoreScreenPresetCRUDRevisionAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "research.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	definition := screenDefinition(t, `{
		"brokerId":"futu",
		"market":"US",
		"catalogVersion":"futu-stock-screen-v1",
		"querySchemaVersion":2,
		"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]
	}`)
	created, err := store.CreateScreenPreset(t.Context(), "美股价值", definition, domain.QuerySchemaVersion)
	if err != nil {
		t.Fatalf("CreateScreenPreset: %v", err)
	}
	if created.ID == "" || created.Revision != 1 || created.QuerySchemaVersion != domain.QuerySchemaVersion {
		t.Fatalf("created = %#v", created)
	}
	if _, err := store.CreateScreenPreset(t.Context(), " 美股价值 ", definition, domain.QuerySchemaVersion); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate name error = %v", err)
	}

	updatedDefinition := screenDefinition(t, `{
		"brokerId":"futu",
		"market":"HK",
		"catalogVersion":"futu-stock-screen-v1",
		"querySchemaVersion":2,
		"columns":[
			{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}},
			{"columnId":"market-cap","factor":{"instanceId":"market-cap","factorKey":"simple.market_cap"}}
		]
	}`)
	updated, err := store.UpdateScreenPreset(t.Context(), created.ID, "港股价值", updatedDefinition, domain.QuerySchemaVersion, created.Revision)
	if err != nil {
		t.Fatalf("UpdateScreenPreset: %v", err)
	}
	if updated.Revision != 2 || updated.Name != "港股价值" || updated.Definition.Market != "HK" {
		t.Fatalf("updated = %#v", updated)
	}
	if _, err := store.UpdateScreenPreset(t.Context(), created.ID, "过期更新", definition, domain.QuerySchemaVersion, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	items, err := reopened.ListScreenPresets(t.Context())
	if err != nil {
		t.Fatalf("ListScreenPresets: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID || items[0].Revision != 2 || items[0].Definition.Market != "HK" {
		t.Fatalf("items after restart = %#v", items)
	}
	if err := reopened.DeleteScreenPreset(t.Context(), created.ID); err != nil {
		t.Fatalf("DeleteScreenPreset: %v", err)
	}
	if _, err := reopened.GetScreenPreset(t.Context(), created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetScreenPreset after delete error = %v", err)
	}
}

func TestStoreRejectsNewerSchemaAndUnavailableReceiver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "research.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO research_schema_migrations(version, applied_at) VALUES (?, ?)`, SchemaVersion+1, nowText()); err != nil {
		t.Fatalf("insert newer migration: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("Open accepted a newer research schema")
	}
	if _, err := (*Store)(nil).ListScreenPresets(t.Context()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("nil store error = %v", err)
	}
	if (*Store)(nil).Path() != "" {
		t.Fatal("nil store returned a path")
	}
	if err := (*Store)(nil).Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
	if _, err := (*Store)(nil).GetScreenPreset(t.Context(), "id"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("nil get error = %v", err)
	}
	if _, err := (*Store)(nil).CreateScreenPreset(t.Context(), "name", broker.ScreenDefinitionV2{}, 2); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("nil create error = %v", err)
	}
	if _, err := (*Store)(nil).UpdateScreenPreset(t.Context(), "id", "name", broker.ScreenDefinitionV2{}, 2, 1); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("nil update error = %v", err)
	}
	if err := (*Store)(nil).DeleteScreenPreset(t.Context(), "id"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("nil delete error = %v", err)
	}
}

func TestStoreRejectsV1PresetOnRead(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "research.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := nowText()
	_, err = store.db.ExecContext(t.Context(), `INSERT INTO research_screen_presets
		(preset_id, name, name_key, query_schema_version, query_json, revision, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, 1, ?, ?)`,
		"rsp_v1", "旧版", "旧版", `{"brokerId":"futu","market":"US","columns":[{"factor":"simple.price"}]}`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetScreenPreset(t.Context(), "rsp_v1"); err == nil ||
		!strings.Contains(err.Error(), "only version 2 is supported") {
		t.Fatalf("V1 preset error = %v", err)
	}
}

func TestStoreReportsPathMissingRowsAndInvalidPersistedDefinitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "research.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if store.Path() != path {
		t.Fatalf("Path = %q", store.Path())
	}
	if _, err := store.GetScreenPreset(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing get error = %v", err)
	}
	if err := store.DeleteScreenPreset(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing delete error = %v", err)
	}
	if _, err := store.UpdateScreenPreset(
		t.Context(), "missing", "name", screenDefinition(t, `{
			"market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2
		}`), 2, 1,
	); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing update error = %v", err)
	}

	now := nowText()
	for _, item := range []struct {
		id   string
		body string
	}{
		{id: "bad-json", body: `{`},
		{id: "bad-definition", body: `{"market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"x","factor":{"factorKey":"missing.factor"}}]}`},
	} {
		_, err := store.db.ExecContext(t.Context(), `INSERT INTO research_screen_presets
			(preset_id, name, name_key, query_schema_version, query_json, revision, created_at, updated_at)
			VALUES (?, ?, ?, 2, ?, 1, ?, ?)`,
			item.id, item.id, item.id, item.body, now, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetScreenPreset(t.Context(), item.id); err == nil {
			t.Fatalf("persisted %s was accepted", item.id)
		}
	}
	if _, err := store.ListScreenPresets(t.Context()); err == nil {
		t.Fatal("list accepted invalid persisted rows")
	}
}

func TestStoreOpenAndWriteErrorBoundaries(t *testing.T) {
	if _, err := Open(t.Context(), " "); err == nil {
		t.Fatal("Open accepted an empty path")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), filepath.Join(blocked, "research.db")); err == nil {
		t.Fatal("Open created a database below a file")
	}
	if _, err := Open(t.Context(), root); err == nil {
		t.Fatal("Open accepted a directory as a database")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Open(cancelled, filepath.Join(root, "cancelled.db")); err == nil {
		t.Fatal("Open accepted a cancelled context")
	}

	if mapWriteError(nil) != nil {
		t.Fatal("mapWriteError(nil) returned an error")
	}
	sentinel := errors.New("plain write error")
	if !errors.Is(mapWriteError(sentinel), sentinel) {
		t.Fatal("plain write error was replaced")
	}
	if !errors.Is(mapWriteError(errors.New("UNIQUE constraint failed")), domain.ErrConflict) {
		t.Fatal("unique constraint did not map to conflict")
	}
	if !parseTime("invalid").IsZero() {
		t.Fatal("invalid timestamp did not produce the zero time")
	}
}

func screenDefinition(t *testing.T, body string) broker.ScreenDefinitionV2 {
	t.Helper()
	var value broker.ScreenDefinitionV2
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("decode screen definition: %v", err)
	}
	return value
}
