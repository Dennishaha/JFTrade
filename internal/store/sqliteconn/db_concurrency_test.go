package sqliteconn

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDatabaseUsesSeparateConcurrentReadPoolAndSerialWritePool(t *testing.T) {
	db := openTestDatabase(t)
	if got := db.Stats().MaxOpenConnections; got != defaultMaxOpenConns {
		t.Fatalf("read MaxOpenConnections = %d, want %d", got, defaultMaxOpenConns)
	}
	if got := db.WriteStats().MaxOpenConnections; got != 1 {
		t.Fatalf("write MaxOpenConnections = %d, want 1", got)
	}
	if _, err := db.reader.ExecContext(context.Background(), `UPDATE state SET value = 'reader-write'`); err == nil {
		t.Fatal("physical read pool accepted a write")
	}

	rowsOne, err := db.QueryContext(context.Background(), `SELECT value FROM state`)
	if err != nil {
		t.Fatalf("first QueryContext: %v", err)
	}
	t.Cleanup(func() { _ = rowsOne.Close() })
	rowsTwo, err := db.QueryContext(context.Background(), `SELECT value FROM state`)
	if err != nil {
		t.Fatalf("second QueryContext: %v", err)
	}
	t.Cleanup(func() { _ = rowsTwo.Close() })
	if got := db.Stats().InUse; got != 2 {
		t.Fatalf("concurrent read connections in use = %d, want 2", got)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := db.ExecContext(context.Background(), `UPDATE state SET value = 'written-with-readers-open'`)
		writeDone <- writeErr
	}()
	if err := waitForSignal(t, writeDone, "WAL write alongside active readers"); err != nil {
		t.Fatalf("concurrent WAL write: %v", err)
	}
}

func TestDatabaseReadWaitsForPreviouslyQueuedWrite(t *testing.T) {
	db := openTestDatabase(t)
	tx, err := db.BeginWrite(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginWrite: %v", err)
	}
	if _, err := tx.ExecContext(context.Background(), `UPDATE state SET value = 'committed'`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("update: %v", err)
	}

	readDone := make(chan struct {
		value string
		err   error
	}, 1)
	go func() {
		var value string
		readErr := db.GetContext(context.Background(), &value, `SELECT value FROM state`)
		readDone <- struct {
			value string
			err   error
		}{value: value, err: readErr}
	}()
	assertNotSignalled(t, readDone, "read submitted after write")

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	result := waitForSignal(t, readDone, "read after committed write")
	if result.err != nil || result.value != "committed" {
		t.Fatalf("read result = (%q, %v), want committed", result.value, result.err)
	}
}

func TestDatabaseQueuedWritesPreserveSubmissionOrder(t *testing.T) {
	db := openTestDatabase(t)
	tx, err := db.BeginWrite(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginWrite: %v", err)
	}
	if _, err := tx.ExecContext(context.Background(), `UPDATE state SET value = 'first'`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("first update: %v", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		_, writeErr := db.ExecContext(context.Background(), `UPDATE state SET value = 'second'`)
		secondDone <- writeErr
	}()
	assertNotSignalled(t, secondDone, "second write")
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := waitForSignal(t, secondDone, "second write"); err != nil {
		t.Fatalf("second write: %v", err)
	}

	var value string
	if err := db.Get(&value, `SELECT value FROM state`); err != nil {
		t.Fatalf("read final value: %v", err)
	}
	if value != "second" {
		t.Fatalf("final value = %q, want second", value)
	}
}

func TestDatabaseReadBarrierHonorsContextCancellation(t *testing.T) {
	db := openTestDatabase(t)
	tx, err := db.BeginWrite(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginWrite: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var value string
	if err := db.GetContext(ctx, &value, `SELECT value FROM state`); err == nil {
		t.Fatal("GetContext returned nil while prior write remained open")
	}
}

func openTestDatabase(t *testing.T) *DB {
	t.Helper()
	db, err := OpenX(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})
	if _, err := db.Exec(`CREATE TABLE state (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create state: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO state (value) VALUES ('initial')`); err != nil {
		t.Fatalf("insert state: %v", err)
	}
	return db
}
