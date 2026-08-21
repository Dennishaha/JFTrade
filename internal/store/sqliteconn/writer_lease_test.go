package sqliteconn

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/ownerlock"
)

func TestSQLiteWritesRequireOwnerLeaseWhileReadsRemainAvailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "execution.db")
	seed, err := Open(path)
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	if _, err := seed.Exec(`CREATE TABLE records (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed: %v", err)
	}

	held, err := ownerlock.Acquire(path, ownerlock.CurrentDiagnostic("rust-test", "lock-conflict"))
	if err != nil {
		t.Fatalf("hold external writer lease: %v", err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open database while writer lease is held: %v", err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM records`); err != nil || count != 0 {
		t.Fatalf("read while lease held = %d, %v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO records(id) VALUES ('blocked')`); !errors.Is(err, ownerlock.ErrHeld) {
		t.Fatalf("write conflict error = %v", err)
	}
	readOnly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only shadow while lease held: %v", err)
	}
	if err := readOnly.Get(&count, `SELECT COUNT(*) FROM records`); err != nil {
		t.Fatalf("read-only shadow query: %v", err)
	}
	_ = readOnly.Close()
	if err := held.Close(); err != nil {
		t.Fatalf("release external writer lease: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO records(id) VALUES ('accepted')`); err != nil {
		t.Fatalf("write after lease release: %v", err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM records`); err != nil || count != 1 {
		t.Fatalf("count after accepted write = %d, %v", count, err)
	}
}

func TestSQLiteTransactionHoldsOwnerLeaseUntilRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE records (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	tx, err := db.BeginWrite(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	if lease, err := ownerlock.Acquire(path, ownerlock.CurrentDiagnostic("rust-test", "conflict")); lease != nil || !errors.Is(err, ownerlock.ErrHeld) {
		t.Fatalf("concurrent lease during transaction = (%v, %v)", lease, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
	lease, err := ownerlock.Acquire(path, ownerlock.CurrentDiagnostic("rust-test", "released"))
	if err != nil {
		t.Fatalf("lease after rollback: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("release lease after rollback: %v", err)
	}
}
