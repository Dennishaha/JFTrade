package sqliteconn

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseReadAndWriteAPI(t *testing.T) {
	db := openTestDatabase(t)
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext: %v", err)
	}

	if _, err := db.NamedExec(`INSERT INTO state (value) VALUES (:value)`, map[string]any{"value": "named"}); err != nil {
		t.Fatalf("NamedExec: %v", err)
	}
	if _, err := db.NamedExecContext(context.Background(), `INSERT INTO state (value) VALUES (:value)`, struct {
		Value string `db:"value"`
	}{Value: "named-context"}); err != nil {
		t.Fatalf("NamedExecContext: %v", err)
	}
	if _, err := db.NamedExec(`INSERT INTO state (value) VALUES (:missing)`, struct{}{}); err == nil {
		t.Fatal("NamedExec accepted missing named argument")
	}

	var values []string
	if err := db.Select(&values, `SELECT value FROM state ORDER BY rowid`); err != nil {
		t.Fatalf("Select: %v", err)
	}
	values = nil
	if err := db.SelectContext(context.Background(), &values, `SELECT value FROM state ORDER BY rowid`); err != nil {
		t.Fatalf("SelectContext: %v", err)
	}

	rows, err := db.Query(`SELECT value FROM state ORDER BY rowid`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = rows.Close()
	if rows, err := db.QueryContext(context.Background(), `UPDATE state SET value = 'bad' RETURNING value`); !errors.Is(err, ErrWriteQueryRequiresTransaction) || rows != nil {
		t.Fatalf("write QueryContext = (%v, %v)", rows, err)
	}

	rowsX, err := db.Queryx(`SELECT value FROM state ORDER BY rowid`)
	if err != nil {
		t.Fatalf("Queryx: %v", err)
	}
	_ = rowsX.Close()
	rowsX, err = db.QueryxContext(context.Background(), `SELECT value FROM state ORDER BY rowid`)
	if err != nil {
		t.Fatalf("QueryxContext: %v", err)
	}
	_ = rowsX.Close()
	if rowsX, err := db.QueryxContext(context.Background(), `DELETE FROM state RETURNING value`); !errors.Is(err, ErrWriteQueryRequiresTransaction) || rowsX != nil {
		t.Fatalf("write QueryxContext = (%v, %v)", rowsX, err)
	}

	var value string
	if err := db.QueryRowx(`SELECT value FROM state ORDER BY rowid LIMIT 1`).Scan(&value); err != nil {
		t.Fatalf("QueryRowx: %v", err)
	}
	if err := db.QueryRowxContext(context.Background(), `SELECT value FROM state ORDER BY rowid LIMIT 1`).Scan(&value); err != nil {
		t.Fatalf("QueryRowxContext: %v", err)
	}
	if err := db.QueryRowContext(context.Background(), `UPDATE state SET value = 'bad' RETURNING value`).Scan(&value); err == nil {
		t.Fatal("write QueryRowContext unexpectedly succeeded")
	}
	if err := db.QueryRowxContext(context.Background(), `UPDATE state SET value = 'bad' RETURNING value`).Scan(&value); err == nil {
		t.Fatal("write QueryRowxContext unexpectedly succeeded")
	}

	stmt, err := db.PrepareContext(context.Background(), `SELECT value FROM state WHERE value = ?`)
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
	if err := stmt.QueryRow("initial").Scan(&value); err != nil {
		t.Fatalf("prepared read: %v", err)
	}
	_ = stmt.Close()
	if stmt, err := db.PrepareContext(context.Background(), `UPDATE state SET value = ?`); !errors.Is(err, ErrWriteQueryRequiresTransaction) || stmt != nil {
		t.Fatalf("write PrepareContext = (%v, %v)", stmt, err)
	}

	if got := db.DriverName(); got != DriverName {
		t.Fatalf("DriverName = %q", got)
	}
	if got := db.Rebind(`SELECT ?`); got != `SELECT ?` {
		t.Fatalf("Rebind = %q", got)
	}
	bound, args, err := db.BindNamed(`SELECT :value`, map[string]any{"value": 7})
	if err != nil || bound != `SELECT ?` || len(args) != 1 || args[0] != 7 {
		t.Fatalf("BindNamed = (%q, %#v, %v)", bound, args, err)
	}
	if _, _, err := db.BindNamed(`SELECT :missing`, struct{}{}); err == nil {
		t.Fatal("BindNamed accepted missing field")
	}
	if got := db.String(); !strings.Contains(got, "read_pool=8 write_pool=1") {
		t.Fatalf("String = %q", got)
	}
}

func TestDatabaseTransactionsAndCancellationBoundaries(t *testing.T) {
	db := openTestDatabase(t)
	if err := db.WriteTx(context.Background(), nil, nil); err != nil {
		t.Fatalf("WriteTx(nil): %v", err)
	}
	wantErr := errors.New("callback failed")
	if err := db.WriteTx(context.Background(), nil, func(tx *Tx) error {
		if _, execErr := tx.ExecContext(context.Background(), `UPDATE state SET value = 'rolled-back'`); execErr != nil {
			return execErr
		}
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("WriteTx(callback error) = %v", err)
	}
	if err := db.WriteTx(context.Background(), nil, func(tx *Tx) error {
		_, execErr := tx.ExecContext(context.Background(), `UPDATE state SET value = 'write-tx'`)
		return execErr
	}); err != nil {
		t.Fatalf("WriteTx(success): %v", err)
	}

	var nilTx *Tx
	if err := nilTx.Commit(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("nil Commit = %v", err)
	}
	if err := nilTx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("nil Rollback = %v", err)
	}

	blocking, err := db.BeginWrite(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginWrite: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.BeginWrite(cancelled, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled BeginWrite = %v", err)
	}
	if _, err := db.ExecContext(cancelled, `UPDATE state SET value = 'cancelled'`); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled queued ExecContext = %v", err)
	}
	if err := db.PingContext(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled PingContext = %v", err)
	}
	if rows, err := db.QueryContext(cancelled, `SELECT value FROM state`); !errors.Is(err, context.Canceled) || rows != nil {
		t.Fatalf("cancelled QueryContext = (%v, %v)", rows, err)
	}
	if rows, err := db.QueryxContext(cancelled, `SELECT value FROM state`); !errors.Is(err, context.Canceled) || rows != nil {
		t.Fatalf("cancelled QueryxContext = (%v, %v)", rows, err)
	}
	var values []string
	if err := db.SelectContext(cancelled, &values, `SELECT value FROM state`); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled SelectContext = %v", err)
	}
	var value string
	if err := db.QueryRowContext(cancelled, `SELECT value FROM state`).Scan(&value); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled QueryRowContext = %v", err)
	}
	if err := db.QueryRowxContext(cancelled, `SELECT value FROM state`).Scan(&value); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled QueryRowxContext = %v", err)
	}
	if stmt, err := db.PrepareContext(cancelled, `SELECT value FROM state`); !errors.Is(err, context.Canceled) || stmt != nil {
		t.Fatalf("cancelled PrepareContext = (%v, %v)", stmt, err)
	}
	if err := blocking.Rollback(); err != nil {
		t.Fatalf("blocking Rollback: %v", err)
	}
}

func TestDatabaseOpenReadOnlyAndClosedPoolErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	writable, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := writable.Exec(`CREATE TABLE state (value TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	_ = writable.Close()

	readonly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	var count int
	if err := readonly.Get(&count, `SELECT COUNT(*) FROM state`); err != nil {
		t.Fatalf("read-only query: %v", err)
	}
	if _, err := readonly.Exec(`INSERT INTO state (value) VALUES ('forbidden')`); err == nil {
		t.Fatal("read-only write unexpectedly succeeded")
	}
	_ = readonly.Close()
	if _, err := readonly.BeginWrite(context.Background(), nil); err == nil {
		t.Fatal("BeginWrite on closed database unexpectedly succeeded")
	}
	if err := readonly.WriteTx(context.Background(), nil, func(*Tx) error { return nil }); err == nil {
		t.Fatal("WriteTx on closed database unexpectedly succeeded")
	}
	if _, err := readonly.Exec(`invalid sql`); err == nil {
		t.Fatal("Exec on closed database unexpectedly succeeded")
	}
	if err := readonly.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := (*DB)(nil).Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}

func TestSQLStatementClassification(t *testing.T) {
	if isReadPragma("SELECT 1") {
		t.Fatal("isReadPragma accepted non-PRAGMA statement")
	}
	reads := []string{
		"SELECT 1", "pragma table_info(x)", "PRAGMA journal_mode", "EXPLAIN SELECT 1",
		"-- comment\nSELECT 1", "/* comment */ SELECT 1", "SELECT",
	}
	for _, query := range reads {
		if !isReadStatement(query) {
			t.Fatalf("isReadStatement(%q) = false", query)
		}
	}
	for _, query := range []string{
		"UPDATE x SET y=1", "WITH x AS (SELECT 1) SELECT * FROM x", "PRAGMA wal_checkpoint(TRUNCATE)",
		"PRAGMA journal_mode=WAL", "PRAGMA optimize", "not pragma", "", "-- only comment", "/* unterminated", "(SELECT 1",
	} {
		if isReadStatement(query) {
			t.Fatalf("isReadStatement(%q) = true", query)
		}
	}
}
