package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

const stage9SQLiteSchemaVersion = "stage9.sqlite-schema-definitions.v1"

type stage9SQLiteSchemaDefinitions struct {
	Version     string                    `json:"version"`
	Definitions []sqliteschema.Definition `json:"definitions"`
}

func TestStage9SQLiteSchemaDefinitionsMatchCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 SQLite schema reference source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/sqlite-schema-definitions.json",
	)
	want := stage9SQLiteSchemaDefinitions{
		Version:     stage9SQLiteSchemaVersion,
		Definitions: sqliteschema.Definitions(),
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode stage 9 SQLite schema definitions: %v", err)
		}
		contents = append(contents, '\n')
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatalf("write stage 9 SQLite schema definitions: %v", err)
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage 9 SQLite schema definitions: %v", err)
	}
	var got stage9SQLiteSchemaDefinitions
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode stage 9 SQLite schema definitions: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 SQLite schema definitions drifted from the Go owner")
	}
}
