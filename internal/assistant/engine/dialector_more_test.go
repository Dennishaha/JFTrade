package adk

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestSQLiteDialectorAdditionalBoundaryBranches(t *testing.T) {
	t.Run("initialize rejects nil and closed managed connections", func(t *testing.T) {
		if _, err := gorm.Open(sqliteDialector{}, &gorm.Config{}); err == nil || !strings.Contains(err.Error(), "managed SQLite connection is required") {
			t.Fatalf("gorm.Open(nil conn) err = %v", err)
		}

		managed, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		if err := managed.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err := gorm.Open(sqliteDialector{Conn: newSQLiteGormPool(managed)}, &gorm.Config{}); err == nil {
			t.Fatal("gorm.Open accepted closed managed connection")
		}
	})

	t.Run("clause builders and quoting cover fallback branches", func(t *testing.T) {
		db := openTestSQLiteGORM(t)
		dialector := sqliteDialector{}
		builders := dialector.ClauseBuilders()

		var quoted strings.Builder
		dialector.QuoteTo(&quoted, "`weird``name`")
		if got := quoted.String(); !strings.Contains(got, "``") {
			t.Fatalf("QuoteTo backtick escaping = %q, want escaped backticks", got)
		}

		insertStmt := &gorm.Statement{DB: db}
		builders["INSERT"](clause.Clause{Expression: clause.Expr{SQL: "RAW INSERT"}}, insertStmt)
		if got := insertStmt.SQL.String(); !strings.Contains(got, "RAW INSERT") {
			t.Fatalf("INSERT fallback builder = %q, want raw expression", got)
		}

		limitStmt := &gorm.Statement{DB: db}
		builders["LIMIT"](clause.Clause{Expression: clause.Limit{}}, limitStmt)
		if got := limitStmt.SQL.String(); got != "" {
			t.Fatalf("LIMIT empty builder = %q, want empty SQL", got)
		}

		forStmt := &gorm.Statement{DB: db}
		builders["FOR"](clause.Clause{Expression: clause.Expr{SQL: "FOR SHARE"}}, forStmt)
		if got := forStmt.SQL.String(); !strings.Contains(got, "FOR SHARE") {
			t.Fatalf("FOR fallback builder = %q, want raw expression", got)
		}

		if got := compareSQLiteVersion("3.35", "3.35.0.1"); got != -1 {
			t.Fatalf("compareSQLiteVersion short-vs-long = %d, want -1", got)
		}
	})
}
