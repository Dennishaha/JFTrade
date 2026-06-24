package sqliteconn

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenXConfiguresBusyTimeoutAndSingleConnectionByDefault(t *testing.T) {
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

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}

	var timeout int
	if err := db.Get(&timeout, `PRAGMA busy_timeout`); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeout != 10000 {
		t.Fatalf("busy_timeout = %d, want 10000", timeout)
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
	for _, want := range []string{"_pragma=journal_mode(WAL)", "_pragma=synchronous(NORMAL)", "_pragma=busy_timeout(10000)"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN missing %q: %q", want, dsn)
		}
	}
}
