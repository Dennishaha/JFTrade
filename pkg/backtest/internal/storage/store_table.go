package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/c9s/bbgo/pkg/types"
)

func (s *FutuKLineStore) ensureKLineTable(tableName string) error {
	_, err := s.db.ExecContext(context.Background(), strings.Join([]string{
		`CREATE TABLE IF NOT EXISTS ` + quoteIdentifier(tableName) + ` (`,
		`  end_time    INTEGER NOT NULL,`,
		`  start_time  INTEGER NOT NULL,`,
		`  open        TEXT    NOT NULL,`,
		`  high        TEXT    NOT NULL,`,
		`  low         TEXT    NOT NULL,`,
		`  close       TEXT    NOT NULL,`,
		`  volume      TEXT    NOT NULL,`,
		`  PRIMARY KEY (end_time)`,
		`) WITHOUT ROWID`,
	}, " "))
	if err != nil {
		return fmt.Errorf("create %s table: %w", tableName, err)
	}
	s.tableExistsCache.Store(tableName, true)
	return s.ensureCompactSchema(tableName)
}

func (s *FutuKLineStore) ensureCompactSchema(tableName string) error {
	rows, err := s.db.QueryContext(context.Background(), `PRAGMA table_info(`+quoteIdentifier(tableName)+`)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", tableName, err)
	}
	defer func() { jftradeLogError(rows.Close()) }()

	got := make([]string, 0, len(expectedKLineSchemaColumns()))
	for rows.Next() {
		var cid, notNull, pk int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan %s schema: %w", tableName, err)
		}
		got = append(got, fmt.Sprintf("%s:%s:%d", name, strings.ToUpper(dataType), pk))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s schema: %w", tableName, err)
	}

	want := expectedKLineSchemaColumns()
	if len(got) != len(want) {
		return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", tableName)
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", tableName)
		}
	}

	var ddl string
	if err := s.db.QueryRowContext(context.Background(), `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&ddl); err != nil {
		return fmt.Errorf("load %s ddl: %w", tableName, err)
	}
	if !strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID") {
		return fmt.Errorf("%s schema is obsolete; rebuild the backtest database", tableName)
	}

	return nil
}

func (s *FutuKLineStore) klineTableExists(tableName string) (bool, error) {
	if cached, ok := s.tableExistsCache.Load(tableName); ok {
		return jftradeCheckedTypeAssertion[bool](cached), nil
	}
	var count int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, err
	}
	exists := count > 0
	s.tableExistsCache.Store(tableName, exists)
	return exists, nil
}

func (s *FutuKLineStore) writeTableName(symbol string, interval types.Interval, rehabType string) string {
	return klineTableNameForSessionScope(symbol, interval, rehabType, s.writeSessionScope)
}

func (s *FutuKLineStore) readTableNames(symbol string, interval types.Interval, rehabType string) ([3]string, int) {
	var tableNames [3]string
	add := func(index int, scope string) {
		tableNames[index] = klineTableNameForSessionScope(symbol, interval, rehabType, scope)
	}

	switch normalizeReadSessionScopeName(s.readSessionScope) {
	case klineSessionScopeRegular:
		add(0, klineSessionScopeRegular)
		add(1, klineSessionScopeLegacy)
		add(2, klineSessionScopeExtended)
		return tableNames, 3
	case klineSessionScopeExtended:
		add(0, klineSessionScopeExtended)
		add(1, klineSessionScopeLegacy)
		add(2, klineSessionScopeRegular)
		return tableNames, 3
	case klineSessionScopeLegacy:
		add(0, klineSessionScopeLegacy)
		return tableNames, 1
	default:
		add(0, klineSessionScopeLegacy)
		add(1, klineSessionScopeExtended)
		add(2, klineSessionScopeRegular)
		return tableNames, 3
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
