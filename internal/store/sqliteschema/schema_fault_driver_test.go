package sqliteschema

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
)

const schemaFaultDriverName = "jftrade-sqliteschema-fault"

var registerSchemaFaultDriverOnce sync.Once

func TestInitializeOrValidateReportsMetadataInsertFailure(t *testing.T) {
	db := openSchemaFaultDB(t, "insert-fails")
	defer closeTestDB(t, db)

	err := InitializeOrValidate(t.Context(), db, filepath.Join(t.TempDir(), "new.db"), "test", 1, []string{
		`CREATE TABLE records (id TEXT PRIMARY KEY)`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "record test schema metadata") {
		t.Fatalf("InitializeOrValidate(metadata insert failure) error = %v", err)
	}
}

func TestValidateTablePropagatesRowsScanIterationAndCloseFailures(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected []string
		want     string
	}{
		{name: "scan error", mode: "scan-error", expected: []string{"id:TEXT:1"}, want: "converting driver.Value type string"},
		{name: "rows error", mode: "rows-error", expected: nil, want: "schema rows failed"},
		{name: "close error", mode: "close-error", expected: nil, want: "schema rows close failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openSchemaFaultDB(t, tc.mode)
			defer closeTestDB(t, db)

			err := ValidateTable(t.Context(), db, "records", tc.expected)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateTable(%s) error = %v", tc.mode, err)
			}
		})
	}
}

func TestCloseRowsPreservesPrimaryErrorAndReportsCloseOnlyFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	resultErr := error(nil)
	closeRows(failingRowCloser{err: closeErr}, &resultErr)
	if !errors.Is(resultErr, closeErr) {
		t.Fatalf("closeRows close-only error = %v", resultErr)
	}

	primaryErr := errors.New("primary failed")
	resultErr = primaryErr
	closeRows(failingRowCloser{err: closeErr}, &resultErr)
	if !errors.Is(resultErr, primaryErr) {
		t.Fatalf("closeRows primary error = %v", resultErr)
	}
}

type failingRowCloser struct {
	err error
}

func (c failingRowCloser) Close() error {
	return c.err
}

func openSchemaFaultDB(t *testing.T, mode string) *sqlx.DB {
	t.Helper()
	registerSchemaFaultDriverOnce.Do(func() {
		sql.Register(schemaFaultDriverName, schemaFaultDriver{})
	})
	db, err := sqlx.Open(schemaFaultDriverName, mode)
	if err != nil {
		t.Fatalf("open schema fault db: %v", err)
	}
	return db
}

type schemaFaultDriver struct{}

func (schemaFaultDriver) Open(name string) (driver.Conn, error) {
	return &schemaFaultConn{mode: name}, nil
}

type schemaFaultConn struct {
	mode      string
	execCount int
}

func (c *schemaFaultConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported by schema fault driver")
}

func (c *schemaFaultConn) Close() error {
	return nil
}

func (c *schemaFaultConn) Begin() (driver.Tx, error) {
	return schemaFaultTx{}, nil
}

func (c *schemaFaultConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return schemaFaultTx{}, nil
}

func (c *schemaFaultConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.execCount++
	if c.mode == "insert-fails" && c.execCount == 3 {
		return nil, errors.New("schema metadata insert failed")
	}
	return driver.RowsAffected(1), nil
}

func (c *schemaFaultConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &schemaFaultRows{mode: c.mode}, nil
}

type schemaFaultTx struct{}

func (schemaFaultTx) Commit() error {
	return nil
}

func (schemaFaultTx) Rollback() error {
	return nil
}

type schemaFaultRows struct {
	mode string
	sent bool
}

func (r *schemaFaultRows) Columns() []string {
	return []string{"cid", "name", "type", "notnull", "dflt_value", "pk"}
}

func (r *schemaFaultRows) Close() error {
	if r.mode == "close-error" {
		return errors.New("schema rows close failed")
	}
	return nil
}

func (r *schemaFaultRows) Next(dest []driver.Value) error {
	switch r.mode {
	case "scan-error":
		if r.sent {
			return io.EOF
		}
		r.sent = true
		dest[0] = "not-an-int"
		dest[1] = "id"
		dest[2] = "TEXT"
		dest[3] = int64(0)
		dest[4] = nil
		dest[5] = int64(1)
		return nil
	case "rows-error":
		return errors.New("schema rows failed")
	default:
		return io.EOF
	}
}
