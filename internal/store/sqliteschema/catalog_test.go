package sqliteschema

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jmoiron/sqlx"
)

func TestCatalogLookupsReturnDefensiveCopies(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 9 {
		t.Fatalf("Definitions() count = %d", len(definitions))
	}
	definitions[0].Statements = append(definitions[0].Statements, "changed")
	if definitions[0].DynamicTable != nil {
		definitions[0].DynamicTable.Pattern = "changed"
	}

	definition, ok := DefinitionFor(DatabaseBacktest)
	if !ok || definition.ID != DatabaseBacktest || definition.DynamicTable == nil {
		t.Fatalf("DefinitionFor(backtest) = (%+v, %t)", definition, ok)
	}
	if definition.DynamicTable.Pattern == "changed" {
		t.Fatal("DefinitionFor returned aliased dynamic table metadata")
	}
	if _, ok := DefinitionFor("  missing  "); ok {
		t.Fatal("DefinitionFor(missing) = found")
	}
	if version, ok := Version(DatabaseExecution); !ok || version != ExecutionVersion {
		t.Fatalf("Version(execution) = (%d, %t)", version, ok)
	}
	if _, ok := Version("missing"); ok {
		t.Fatal("Version(missing) = found")
	}
	statements := Statements(DatabaseWatchlist)
	if len(statements) == 0 {
		t.Fatal("Statements(watchlist) is empty")
	}
	statements[0] = "changed"
	if Statements(DatabaseWatchlist)[0] == "changed" {
		t.Fatal("Statements returned an aliased slice")
	}
	if Statements("missing") != nil {
		t.Fatal("Statements(missing) != nil")
	}

	defer func() {
		if recovered := recover(); recovered == nil || !strings.Contains(recovered.(string), "unknown SQLite schema definition") {
			t.Fatalf("MustDefinition(missing) panic = %v", recovered)
		}
	}()
	MustDefinition("missing")
}

func TestCurrentCatalogInitializesAndValidatesEveryDatabase(t *testing.T) {
	for _, definition := range Definitions() {
		t.Run(definition.ID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), definition.ID+".db")
			db := openTestDB(t, path)
			defer closeTestDB(t, db)
			if err := InitializeCurrent(t.Context(), db, path, definition.ID); err != nil {
				t.Fatalf("InitializeCurrent() error = %v", err)
			}
			if err := ValidateCurrent(t.Context(), db, path, definition.ID); err != nil {
				t.Fatalf("ValidateCurrent() error = %v", err)
			}
			if err := ValidateCurrentFile(t.Context(), path, definition.ID); err != nil {
				t.Fatalf("ValidateCurrentFile() error = %v", err)
			}
		})
	}
}

func TestCatalogRejectsUnknownIDsAndInvalidPreflightPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if err := InitializeCurrent(t.Context(), db, path, "missing"); err == nil || !strings.Contains(err.Error(), "unknown SQLite database id") {
		t.Fatalf("InitializeCurrent(missing) error = %v", err)
	}
	if err := ValidateCurrent(t.Context(), db, path, "missing"); err == nil || !strings.Contains(err.Error(), "unknown SQLite database id") {
		t.Fatalf("ValidateCurrent(missing) error = %v", err)
	}
	if err := ValidateCurrentFile(t.Context(), filepath.Join(t.TempDir(), "absent.db"), DatabaseResearch); err != nil {
		t.Fatalf("ValidateCurrentFile(absent) error = %v", err)
	}
	directory := t.TempDir()
	if err := ValidateCurrentFile(t.Context(), directory, DatabaseResearch); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ValidateCurrentFile(directory) error = %v", err)
	}
}

func TestValidateCurrentDetectsManifestDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate string
		want   string
	}{
		{name: "missing table", mutate: `DROP TABLE backtest_runs`, want: "required table is missing"},
		{name: "unknown table", mutate: `CREATE TABLE unexpected (id INTEGER)`, want: "unknown application table"},
		{name: "column drift", mutate: `ALTER TABLE backtest_runs ADD COLUMN unexpected TEXT`, want: "columns do not match"},
		{name: "index drift", mutate: `DROP INDEX idx_backtest_runs_status`, want: "indexes do not match"},
		{name: "view drift", mutate: `CREATE VIEW unexpected_view AS SELECT id FROM backtest_runs`, want: "views do not match"},
		{name: "trigger drift", mutate: `CREATE TRIGGER unexpected_trigger AFTER INSERT ON backtest_runs BEGIN UPDATE backtest_runs SET status = status WHERE id = NEW.id; END`, want: "triggers do not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runs.db")
			db := openTestDB(t, path)
			defer closeTestDB(t, db)
			if err := InitializeCurrent(t.Context(), db, path, DatabaseBacktestRuns); err != nil {
				t.Fatalf("InitializeCurrent() error = %v", err)
			}
			if _, err := db.Exec(test.mutate); err != nil {
				t.Fatalf("mutate schema: %v", err)
			}
			err := ValidateCurrent(t.Context(), db, path, DatabaseBacktestRuns)
			if !IsIncompatible(err) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateCurrent() error = %v", err)
			}
		})
	}
}

func TestValidateDefinitionSupportsOnlyMatchingDynamicTables(t *testing.T) {
	definition := MustDefinition(DatabaseBacktest)
	validName := "local_klines__hk_00700__5m__forward__r__1234abcd"
	validStatement := strings.Replace(definition.DynamicTable.Statement, definition.DynamicTable.PrototypeName, validName, 1)

	path := filepath.Join(t.TempDir(), "backtest.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if err := InitializeCurrent(t.Context(), db, path, DatabaseBacktest); err != nil {
		t.Fatalf("InitializeCurrent() error = %v", err)
	}
	if _, err := db.Exec(validStatement); err != nil {
		t.Fatalf("create valid dynamic table: %v", err)
	}
	if err := ValidateCurrent(t.Context(), db, path, DatabaseBacktest); err != nil {
		t.Fatalf("ValidateCurrent(valid dynamic table) error = %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE ` + validName + ` ADD COLUMN unexpected TEXT`); err != nil {
		t.Fatalf("alter dynamic table: %v", err)
	}
	if err := ValidateCurrent(t.Context(), db, path, DatabaseBacktest); !IsIncompatible(err) || !strings.Contains(err.Error(), "columns do not match") {
		t.Fatalf("ValidateCurrent(drifted dynamic table) error = %v", err)
	}
}

func TestDefinitionConstructionAndComparisonFailurePaths(t *testing.T) {
	if _, err := compileDynamicPattern(Definition{}); err != nil {
		t.Fatalf("compileDynamicPattern(nil) error = %v", err)
	}
	if _, err := compileDynamicPattern(Definition{ID: "bad", DynamicTable: &DynamicTableDefinition{Pattern: "["}}); err == nil {
		t.Fatal("compileDynamicPattern(invalid) error = nil")
	}
	if _, err := expectedDatabase(Definition{Statements: []string{" ", "not sql"}}); err == nil {
		t.Fatal("expectedDatabase(invalid statement) error = nil")
	}
	if _, err := expectedDatabase(Definition{DynamicTable: &DynamicTableDefinition{Statement: "not sql"}}); err == nil {
		t.Fatal("expectedDatabase(invalid dynamic statement) error = nil")
	}
	if _, err := expectedDatabase(Definition{Statements: []string{`CREATE TABLE ` + MetadataTable + ` (id INTEGER)`}}); err == nil {
		t.Fatal("expectedDatabase(metadata conflict) error = nil")
	}

	base := tableSnapshot{columns: []columnSnapshot{{name: "id"}}, indexes: []indexSnapshot{{name: "idx"}}, foreignKeys: []foreignKeySnapshot{{table: "parent"}}}
	tests := []struct {
		name   string
		actual tableSnapshot
		want   string
	}{
		{name: "columns", actual: tableSnapshot{}, want: "columns"},
		{name: "indexes", actual: tableSnapshot{columns: base.columns}, want: "indexes"},
		{name: "foreign keys", actual: tableSnapshot{columns: base.columns, indexes: base.indexes}, want: "foreign keys"},
		{name: "options", actual: tableSnapshot{columns: base.columns, indexes: base.indexes, foreignKeys: base.foreignKeys, strict: true}, want: "table options"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := compareTable("records", test.actual, base); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compareTable() error = %v", err)
			}
		})
	}
	if err := compareTable("records", base, base); err != nil {
		t.Fatalf("compareTable(equal) error = %v", err)
	}
	if !equalStrings([]string{"a"}, []string{"a"}) || equalStrings(nil, []string{"a"}) || equalStrings([]string{"a"}, []string{"b"}) {
		t.Fatal("equalStrings returned an invalid comparison")
	}
}

func TestValidateDefinitionReportsBuildAndInspectionFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if err := InitializeOrValidate(t.Context(), db, path, DatabaseResearch, ResearchVersion, nil, nil); err != nil {
		t.Fatalf("initialize metadata: %v", err)
	}

	err := validateDefinition(t.Context(), db, path, Definition{ID: "test", Version: 1, Statements: []string{"not sql"}})
	if err == nil || !strings.Contains(err.Error(), "build test schema manifest") {
		t.Fatalf("validateDefinition(invalid manifest) error = %v", err)
	}
	err = validateDefinition(t.Context(), db, path, Definition{
		ID: "test", Version: 1,
		DynamicTable: &DynamicTableDefinition{Pattern: "[", PrototypeName: "prototype", Statement: `CREATE TABLE prototype (id INTEGER)`},
	})
	if err == nil || !strings.Contains(err.Error(), "compile test dynamic table pattern") {
		t.Fatalf("validateDefinition(invalid pattern) error = %v", err)
	}
	err = validateDefinition(t.Context(), db, path, Definition{
		ID: "test", Version: 1,
		DynamicTable: &DynamicTableDefinition{Pattern: ".*", PrototypeName: "missing", Statement: `CREATE TABLE prototype (id INTEGER)`},
	})
	if err == nil || !strings.Contains(err.Error(), "dynamic table prototype missing is missing") {
		t.Fatalf("validateDefinition(missing prototype) error = %v", err)
	}

	wrapped := queryFailingDatabase{Database: db, err: errors.New("manifest query failed")}
	err = ValidateCurrent(t.Context(), wrapped, path, DatabaseResearch)
	if !IsIncompatible(err) || !strings.Contains(err.Error(), "manifest query failed") {
		t.Fatalf("ValidateCurrent(query failure) error = %v", err)
	}
}

func TestCatalogInspectionPropagatesDatabaseFailures(t *testing.T) {
	for _, mode := range []string{"query-error", "scan-error", "rows-error", "close-error"} {
		t.Run(mode, func(t *testing.T) {
			db := openSchemaFaultDB(t, mode)
			defer closeTestDB(t, db)
			checks := []func() error{
				func() error { _, err := inspectSchema(t.Context(), db); return err },
				func() error { _, err := inspectColumns(t.Context(), db, "records"); return err },
				func() error { _, err := inspectIndexes(t.Context(), db, "records"); return err },
				func() error { _, err := inspectForeignKeys(t.Context(), db, "records"); return err },
				func() error { return validateIntegrity(t.Context(), db) },
			}
			for index, check := range checks {
				if err := check(); err == nil {
					t.Fatalf("check %d error = nil", index)
				}
			}
		})
	}
}

func TestValidateIntegrityDetectsForeignKeyViolations(t *testing.T) {
	db := openTestDB(t, filepath.Join(t.TempDir(), "foreign-key.db"))
	defer closeTestDB(t, db)
	if _, err := db.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY);
		CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id));
		INSERT INTO child (id, parent_id) VALUES (1, 99)`); err != nil {
		t.Fatalf("create invalid foreign key: %v", err)
	}
	if err := validateIntegrity(t.Context(), db); err == nil || !strings.Contains(err.Error(), "foreign_key_check failed") {
		t.Fatalf("validateIntegrity() error = %v", err)
	}
}

func TestValidateIntegrityFailureResults(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{mode: "quick-bad", want: "quick_check failed"},
		{mode: "quick-empty", want: "quick_check did not return ok"},
		{mode: "foreign-query-error", want: "foreign key query failed"},
		{mode: "foreign-scan-error", want: "converting driver.Value"},
		{mode: "foreign-rows-error", want: "foreign key rows failed"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			db := openSchemaFaultDB(t, test.mode)
			defer closeTestDB(t, db)
			err := validateIntegrity(t.Context(), db)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateIntegrity() error = %v", err)
			}
		})
	}
}

func TestValidateDefinitionWrapsIntegrityViolations(t *testing.T) {
	definition := Definition{ID: "test", Version: 1, Statements: []string{
		`CREATE TABLE parent (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id))`,
	}}
	path := filepath.Join(t.TempDir(), "integrity.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if err := InitializeOrValidate(t.Context(), db, path, definition.ID, definition.Version, definition.Statements, nil); err != nil {
		t.Fatalf("initialize definition: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO child (id, parent_id) VALUES (1, 99)`); err != nil {
		t.Fatalf("insert invalid child: %v", err)
	}
	if err := validateDefinition(t.Context(), db, path, definition); !IsIncompatible(err) || !strings.Contains(err.Error(), "foreign_key_check failed") {
		t.Fatalf("validateDefinition() error = %v", err)
	}
}

func TestInspectTablePropagatesEachManifestQueryFailure(t *testing.T) {
	db := openTestDB(t, filepath.Join(t.TempDir(), "inspect.db"))
	defer closeTestDB(t, db)
	if _, err := db.Exec(`CREATE TABLE records (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES records(id));
		CREATE INDEX idx_records_parent ON records(parent_id)`); err != nil {
		t.Fatalf("create inspection schema: %v", err)
	}
	for _, queryPart := range []string{"table_xinfo", "index_list", "foreign_key_list", "index_xinfo"} {
		t.Run(queryPart, func(t *testing.T) {
			wrapped := selectiveQueryFailDatabase{Database: db, queryPart: queryPart}
			if _, err := inspectTable(t.Context(), wrapped, "records", `CREATE TABLE records (id INTEGER PRIMARY KEY)`); err == nil || !strings.Contains(err.Error(), "selected query failed") {
				t.Fatalf("inspectTable() error = %v", err)
			}
		})
	}
	wrapped := selectiveQueryFailDatabase{Database: db, queryPart: "table_xinfo"}
	if _, err := inspectSchema(t.Context(), wrapped); err == nil || !strings.Contains(err.Error(), "selected query failed") {
		t.Fatalf("inspectSchema() error = %v", err)
	}
}

func TestValidateMetadataRejectsAdditionalComponentRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata-rows.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if err := InitializeOrValidate(t.Context(), db, path, "test", 1, nil, nil); err != nil {
		t.Fatalf("initialize metadata: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ` + MetadataTable + ` (component_id, version, created_at) VALUES ('other', 1, 'now')`); err != nil {
		t.Fatalf("insert extra metadata: %v", err)
	}
	if err := ValidateMetadata(t.Context(), db, path, "test", 1); !IsIncompatible(err) || !strings.Contains(err.Error(), "exactly one is required") {
		t.Fatalf("ValidateMetadata() error = %v", err)
	}
}

func TestValidateCurrentPropagatesMetadataVersionDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-drift.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if err := InitializeCurrent(t.Context(), db, path, DatabaseResearch); err != nil {
		t.Fatalf("InitializeCurrent() error = %v", err)
	}
	if _, err := db.Exec(`UPDATE ` + MetadataTable + ` SET version = version + 1`); err != nil {
		t.Fatalf("drift metadata version: %v", err)
	}
	if err := ValidateCurrent(t.Context(), db, path, DatabaseResearch); !IsIncompatible(err) || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("ValidateCurrent(version drift) error = %v", err)
	}
}

func TestInspectIndexesPropagatesIndexColumnScanFailure(t *testing.T) {
	db := openSchemaFaultDB(t, "index-xinfo-scan-error")
	defer closeTestDB(t, db)
	if _, err := inspectIndexes(t.Context(), db, "records"); err == nil || !strings.Contains(err.Error(), "converting driver.Value") {
		t.Fatalf("inspectIndexes() error = %v", err)
	}
}

func TestInitializeMetadataPublicBoundaries(t *testing.T) {
	if err := InitializeMetadata(t.Context(), nil, "test", 1); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("InitializeMetadata(nil) error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "metadata.db")
	db, err := sqliteconn.Open(path)
	if err != nil {
		t.Fatalf("open managed sqlite: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close managed sqlite: %v", err)
		}
	}()
	if err := InitializeMetadata(t.Context(), db, "test", 1); err != nil {
		t.Fatalf("InitializeMetadata() error = %v", err)
	}
}

type queryFailingDatabase struct {
	Database
	err error
}

func (db queryFailingDatabase) QueryxContext(context.Context, string, ...any) (*sqlx.Rows, error) {
	return nil, db.err
}

var _ Database = queryFailingDatabase{}

type selectiveQueryFailDatabase struct {
	Database
	queryPart string
}

func (db selectiveQueryFailDatabase) QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	if strings.Contains(strings.ToLower(query), db.queryPart) {
		return nil, errors.New("selected query failed")
	}
	return db.Database.QueryxContext(ctx, query, args...)
}

var _ Database = selectiveQueryFailDatabase{}
