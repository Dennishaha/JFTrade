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

func (c *schemaFaultConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.mode == "query-error" {
		return nil, errors.New("schema query failed")
	}
	query = strings.ToLower(query)
	if c.mode == "foreign-query-error" && strings.Contains(query, "foreign_key_check") {
		return nil, errors.New("foreign key query failed")
	}
	return &schemaFaultRows{mode: c.mode, query: query}, nil
}

type schemaFaultTx struct{}

func (schemaFaultTx) Commit() error {
	return nil
}

func (schemaFaultTx) Rollback() error {
	return nil
}

type schemaFaultRows struct {
	mode  string
	query string
	sent  bool
}

func (r *schemaFaultRows) Columns() []string {
	if r.mode == "index-xinfo-scan-error" {
		if strings.Contains(r.query, "index_list") {
			return []string{"seq", "name", "unique", "origin", "partial"}
		}
		if strings.Contains(r.query, "index_xinfo") {
			return []string{"seqno", "cid", "name", "desc", "coll", "key"}
		}
	}
	if strings.Contains(r.query, "quick_check") && r.usesIntegrityRows() {
		return []string{"quick_check"}
	}
	if strings.Contains(r.query, "foreign_key_check") && r.usesIntegrityRows() {
		return []string{"table", "rowid", "parent", "fkid"}
	}
	return []string{"cid", "name", "type", "notnull", "dflt_value", "pk"}
}

func (r *schemaFaultRows) Close() error {
	if r.mode == "close-error" {
		return errors.New("schema rows close failed")
	}
	return nil
}

func (r *schemaFaultRows) Next(dest []driver.Value) error {
	if r.mode == "index-xinfo-scan-error" {
		if r.sent {
			return io.EOF
		}
		r.sent = true
		if strings.Contains(r.query, "index_list") {
			dest[0] = int64(0)
			dest[1] = "idx_records"
			dest[2] = int64(0)
			dest[3] = "c"
			dest[4] = int64(0)
			return nil
		}
		if strings.Contains(r.query, "index_xinfo") {
			dest[0] = "not-an-integer"
			dest[1] = int64(0)
			dest[2] = "id"
			dest[3] = int64(0)
			dest[4] = "BINARY"
			dest[5] = int64(1)
			return nil
		}
	}
	if strings.Contains(r.query, "quick_check") && r.usesIntegrityRows() {
		if r.sent || r.mode == "quick-empty" {
			return io.EOF
		}
		r.sent = true
		if r.mode == "quick-bad" {
			dest[0] = "corrupt"
		} else {
			dest[0] = "ok"
		}
		return nil
	}
	if strings.Contains(r.query, "foreign_key_check") && r.usesIntegrityRows() {
		switch r.mode {
		case "foreign-scan-error":
			if r.sent {
				return io.EOF
			}
			r.sent = true
			dest[0] = "child"
			dest[1] = "not-an-integer"
			dest[2] = "parent"
			dest[3] = int64(0)
			return nil
		case "foreign-rows-error":
			return errors.New("foreign key rows failed")
		default:
			return io.EOF
		}
	}
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

func (r *schemaFaultRows) usesIntegrityRows() bool {
	return strings.HasPrefix(r.mode, "quick-") || strings.HasPrefix(r.mode, "foreign-")
}
