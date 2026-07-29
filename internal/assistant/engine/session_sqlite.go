package adk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	adksession "google.golang.org/adk/v2/session"
	adksessiondb "google.golang.org/adk/v2/session/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteSessionService struct {
	adksession.Service
	db   *sqliteconn.DB
	path string
}

const sqliteSessionSchemaVersion = 4
const sqliteSessionComponent = "adk-session"

func NewSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	path = strings.TrimSpace(path)
	if err := sqliteschema.ValidateCurrentFile(context.Background(), path, sqliteSessionComponent); err != nil {
		return nil, err
	}
	return openSQLiteSessionService(path)
}

func openSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	db, err := sqliteconn.Open(path)
	if err != nil {
		return nil, err
	}
	service, err := adksessiondb.NewSessionService(sqliteDialector{
		Conn: newSQLiteGormPool(db),
	}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, err
	}
	if err := prepareSQLiteSessionSchema(context.Background(), db, service, path); err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, err
	}
	return &SQLiteSessionService{Service: service, db: db, path: strings.TrimSpace(path)}, nil
}

func prepareSQLiteSessionSchema(ctx context.Context, db *sqliteconn.DB, service adksession.Service, path string) error {
	if db == nil {
		return fmt.Errorf("ADK session database is unavailable")
	}
	_ = service
	return sqliteschema.InitializeCurrent(ctx, db, path, sqliteSessionComponent)
}

func validateSQLiteSessionTables(ctx context.Context, db *sqliteconn.DB, path string) error {
	schemas := []struct {
		table   string
		columns []string
	}{
		{"app_states", []string{"app_name:TEXT:1", "state:TEXT:0", "update_time:TIMESTAMP:0"}},
		{"events", []string{"id:TEXT:1", "app_name:TEXT:2", "user_id:TEXT:3", "session_id:TEXT:4", "invocation_id:TEXT:0", "author:TEXT:0", "actions:BLOB:0", "long_running_tool_ids_json:TEXT:0", "routes_json:TEXT:0", "output_json:TEXT:0", "node_info_json:TEXT:0", "requested_input_json:TEXT:0", "branch:TEXT:0", "isolation_scope:TEXT:0", "timestamp:TIMESTAMP:0", "content:TEXT:0", "grounding_metadata:TEXT:0", "custom_metadata:TEXT:0", "usage_metadata:TEXT:0", "citation_metadata:TEXT:0", "partial:NUMERIC:0", "turn_complete:NUMERIC:0", "error_code:TEXT:0", "error_message:TEXT:0", "interrupted:NUMERIC:0"}},
		{"sessions", []string{"app_name:TEXT:1", "user_id:TEXT:2", "id:TEXT:3", "state:TEXT:0", "create_time:TIMESTAMP:0", "update_time:TIMESTAMP:0"}},
		{"user_states", []string{"app_name:TEXT:1", "user_id:TEXT:2", "state:TEXT:0", "update_time:TIMESTAMP:0"}},
	}
	for _, schema := range schemas {
		if err := sqliteschema.ValidateTable(ctx, db, schema.table, schema.columns); err != nil {
			return &sqliteschema.IncompatibleError{Component: sqliteSessionComponent, Path: path, Reason: err.Error()}
		}
	}
	return nil
}

func sqliteTableColumnExists(ctx context.Context, db *sqliteconn.DB, tableName, columnName string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("sqlite session database is unavailable")
	}
	rows, err := db.QueryxContext(ctx, `PRAGMA table_info(`+strings.TrimSpace(tableName)+`)`)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(columnName)) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func (s *SQLiteSessionService) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteSessionService) DatabasePath() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.path)
}

func CompactSQLiteSessionService(ctx context.Context, service adksession.Service) error {
	wrapper, ok := service.(*SQLiteSessionService)
	if !ok || wrapper == nil || wrapper.db == nil {
		return fmt.Errorf("ADK session database is unavailable")
	}
	if _, err := wrapper.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	_, err := wrapper.db.ExecContext(ctx, `VACUUM`)
	return err
}

func ValidateSQLiteSessionService(service adksession.Service) error {
	if wrapper, ok := service.(*SQLiteSessionService); ok && wrapper != nil {
		ready, err := sqliteSessionSchemaReady(wrapper.db)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
	}
	return fmt.Errorf("ADK session schema is unavailable")
}

func CloseSessionService(service adksession.Service) error {
	if closer, ok := service.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func sqliteSessionSchemaReady(db sqliteRowQuerier) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("sqlite session database is unavailable")
	}
	requiredTables := []string{"sessions", "events", "app_states", "user_states"}
	for _, tableName := range requiredTables {
		exists, err := sqliteTableExists(db, tableName)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

func sqliteTableExists(db sqliteRowQuerier, tableName string) (bool, error) {
	var name string
	err := db.QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1`,
		strings.TrimSpace(tableName),
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(name) != "", nil
}

type sqliteRowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
