package sqliteconn

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestOpenXConfiguresBusyTimeoutAndConcurrentReadsByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := OpenX(path)
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	if got := db.Stats().MaxOpenConnections; got != defaultMaxOpenConns {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, defaultMaxOpenConns)
	}
	if got := db.WriteStats().MaxOpenConnections; got != 1 {
		t.Fatalf("write MaxOpenConnections = %d, want 1", got)
	}

	var timeout int
	if err := db.Get(&timeout, `PRAGMA busy_timeout`); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeout != 10000 {
		t.Fatalf("busy_timeout = %d, want 10000", timeout)
	}
	var foreignKeys int
	if err := db.Get(&foreignKeys, `PRAGMA foreign_keys`); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestOpenXCanEnableConcurrentReadConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := OpenX(path, WithMaxOpenConns(8))
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	if got := db.Stats().MaxOpenConnections; got != 8 {
		t.Fatalf("MaxOpenConnections = %d, want 8", got)
	}
}

func TestDSNAppendsPragmasToExistingQuery(t *testing.T) {
	dsn := DSN("file:test.db?mode=ro")
	if !strings.Contains(dsn, "mode=ro&") {
		t.Fatalf("DSN(%q) did not preserve existing query separator: %q", "file:test.db?mode=ro", dsn)
	}
	for _, want := range []string{"_pragma=journal_mode(WAL)", "_pragma=synchronous(NORMAL)", "_pragma=foreign_keys(ON)", "_pragma=busy_timeout(10000)"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN missing %q: %q", want, dsn)
		}
	}
}

func TestReadDSNEnforcesQueryOnlyConnections(t *testing.T) {
	for _, path := range []string{"store.db", "file:store.db?cache=shared"} {
		dsn := ReadDSN(path)
		for _, want := range []string{"_pragma=query_only(1)", "_pragma=foreign_keys(ON)", "_pragma=busy_timeout(10000)"} {
			if !strings.Contains(dsn, want) {
				t.Fatalf("ReadDSN(%q) missing %q: %q", path, want, dsn)
			}
		}
	}
}

func TestOpenCreatesUsableSQLiteDatabaseWithConfiguredPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := Open("  "+path+"  ", WithMaxOpenConns(4), WithMaxIdleConns(2))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	if got := db.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("MaxOpenConnections = %d, want 4", got)
	}
	if _, err := db.Exec(`CREATE TABLE jobs (id INTEGER PRIMARY KEY, status TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO jobs (status) VALUES (?)`, "pending"); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM jobs WHERE id = 1`).Scan(&status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}

	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestOpenFunctionsRejectBlankPath(t *testing.T) {
	if db, err := Open(" \t\n "); err == nil || db != nil || err.Error() != "sqlite database path is required" {
		t.Fatalf("Open(blank) = (%#v, %v)", db, err)
	}
	if db, err := OpenX(" \t\n "); err == nil || db != nil || err.Error() != "sqlite database path is required" {
		t.Fatalf("OpenX(blank) = (%#v, %v)", db, err)
	}
}

func TestOpenFunctionsPropagateDriverOpenErrors(t *testing.T) {
	wantErr := errors.New("open failed")
	originalOpenSQLX := openSQLX
	t.Cleanup(func() {
		openSQLX = originalOpenSQLX
	})

	openSQLX = func(string, string) (*sqlx.DB, error) { return nil, wantErr }
	if db, err := Open("store.db"); db != nil || !errors.Is(err, wantErr) {
		t.Fatalf("Open(driver error) = (%#v, %v), want nil, %v", db, err, wantErr)
	}

	if db, err := OpenX("store.db"); db != nil || !errors.Is(err, wantErr) {
		t.Fatalf("OpenX(driver error) = (%#v, %v), want nil, %v", db, err, wantErr)
	}

	callCount := 0
	openSQLX = func(driverName, dsn string) (*sqlx.DB, error) {
		callCount++
		if callCount == 2 {
			return nil, wantErr
		}
		return originalOpenSQLX(driverName, dsn)
	}
	if db, err := OpenX(filepath.Join(t.TempDir(), "reader-failure.db")); db != nil || !errors.Is(err, wantErr) {
		t.Fatalf("OpenX(reader error) = (%#v, %v), want nil, %v", db, err, wantErr)
	}
}

func TestResolveOptionsNormalizesConnectionPoolBoundaries(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want Options
	}{
		{name: "defaults", want: Options{MaxOpenConns: 8, MaxIdleConns: 8}},
		{name: "nil option", opts: []Option{nil}, want: Options{MaxOpenConns: 8, MaxIdleConns: 8}},
		{name: "nonpositive open", opts: []Option{WithMaxOpenConns(0), WithMaxIdleConns(4)}, want: Options{MaxOpenConns: 8, MaxIdleConns: 4}},
		{name: "nonpositive idle", opts: []Option{WithMaxOpenConns(8), WithMaxIdleConns(0)}, want: Options{MaxOpenConns: 8, MaxIdleConns: 8}},
		{name: "idle exceeds open", opts: []Option{WithMaxOpenConns(4), WithMaxIdleConns(8)}, want: Options{MaxOpenConns: 4, MaxIdleConns: 4}},
		{name: "explicit pool", opts: []Option{WithMaxOpenConns(8), WithMaxIdleConns(3)}, want: Options{MaxOpenConns: 8, MaxIdleConns: 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOptions(tt.opts...); got != tt.want {
				t.Fatalf("resolveOptions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReadOnlyDSNAddsModeWithoutWritePragmas(t *testing.T) {
	dsn := ReadOnlyDSN("store.db")
	for _, want := range []string{"file:store.db?mode=ro", "_pragma=foreign_keys(ON)", "_pragma=busy_timeout(10000)"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("ReadOnlyDSN missing %q: %q", want, dsn)
		}
	}
	if strings.Contains(dsn, "journal_mode") || strings.Contains(dsn, "synchronous") {
		t.Fatalf("ReadOnlyDSN contains write pragma: %q", dsn)
	}

	existing := ReadOnlyDSN("file:store.db?mode=ro")
	if strings.Count(existing, "mode=ro") != 1 || !strings.Contains(existing, "&_pragma=") {
		t.Fatalf("ReadOnlyDSN(existing mode) = %q", existing)
	}
}

func TestForeignKeysAndCascadesAreEnforcedAcrossDatabaseConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foreign-keys.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open(first): %v", err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Fatalf("Close(first): %v", err)
		}
	})
	second, err := Open(path)
	if err != nil {
		t.Fatalf("Open(second): %v", err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Fatalf("Close(second): %v", err)
		}
	})

	if _, err := first.Exec(`CREATE TABLE parents (id TEXT PRIMARY KEY);
		CREATE TABLE children (
			id TEXT PRIMARY KEY,
			parent_id TEXT NOT NULL REFERENCES parents(id) ON DELETE CASCADE
		)`); err != nil {
		t.Fatalf("create constrained tables: %v", err)
	}
	if _, err := second.Exec(`INSERT INTO children (id, parent_id) VALUES ('orphan', 'missing')`); err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("orphan insert error = %v, want foreign key rejection", err)
	}
	if _, err := first.Exec(`INSERT INTO parents (id) VALUES ('parent')`); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	if _, err := second.Exec(`INSERT INTO children (id, parent_id) VALUES ('child', 'parent')`); err != nil {
		t.Fatalf("insert child through second connection: %v", err)
	}
	if _, err := first.Exec(`DELETE FROM parents WHERE id = 'parent'`); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	var childCount int
	if err := second.Get(&childCount, `SELECT COUNT(*) FROM children`); err != nil {
		t.Fatalf("count children through second connection: %v", err)
	}
	if childCount != 0 {
		t.Fatalf("child count after cascade = %d, want 0", childCount)
	}
}
