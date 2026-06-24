package adk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jmoiron/sqlx"
	adksession "google.golang.org/adk/session"
	adksessiondb "google.golang.org/adk/session/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteSessionService struct {
	adksession.Service
	db *sql.DB
}

func NewSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	dsn := sqliteconn.DSN(path)
	db, err := sqliteconn.Open(path)
	if err != nil {
		return nil, err
	}
	if err := sqliteschema.InitializeOrValidate(
		context.Background(),
		sqlx.NewDb(db, sqliteDriverName),
		path,
		"adk-session",
		1,
		[]string{
			"CREATE TABLE sessions (app_name TEXT, user_id TEXT, id TEXT, state TEXT, create_time TIMESTAMP, update_time TIMESTAMP, PRIMARY KEY (app_name,user_id,id))",
			"CREATE TABLE events (id TEXT, app_name TEXT, user_id TEXT, session_id TEXT, invocation_id TEXT, author TEXT, actions BLOB, long_running_tool_ids_json TEXT, branch TEXT, timestamp TIMESTAMP, content TEXT, grounding_metadata TEXT, custom_metadata TEXT, usage_metadata TEXT, citation_metadata TEXT, partial NUMERIC, turn_complete NUMERIC, error_code TEXT, error_message TEXT, interrupted NUMERIC, PRIMARY KEY (id,app_name,user_id,session_id), FOREIGN KEY (app_name,user_id,session_id) REFERENCES sessions(app_name,user_id,id) ON DELETE CASCADE)",
			"CREATE TABLE app_states (app_name TEXT PRIMARY KEY, state TEXT, update_time TIMESTAMP)",
			"CREATE TABLE user_states (app_name TEXT, user_id TEXT, state TEXT, update_time TIMESTAMP, PRIMARY KEY (app_name,user_id))",
		},
		func(ctx context.Context, db *sqlx.DB) error {
			for _, schema := range []struct {
				table   string
				columns []string
			}{
				{"sessions", []string{"app_name:TEXT:1", "user_id:TEXT:2", "id:TEXT:3", "state:TEXT:0", "create_time:TIMESTAMP:0", "update_time:TIMESTAMP:0"}},
				{"app_states", []string{"app_name:TEXT:1", "state:TEXT:0", "update_time:TIMESTAMP:0"}},
				{"user_states", []string{"app_name:TEXT:1", "user_id:TEXT:2", "state:TEXT:0", "update_time:TIMESTAMP:0"}},
			} {
				if err := sqliteschema.ValidateTable(ctx, db, schema.table, schema.columns); err != nil {
					return err
				}
			}
			return nil
		},
	); err != nil {
		_ = db.Close()
		return nil, err
	}
	service, err := adksessiondb.NewSessionService(sqliteDialector{
		DriverName: sqliteDriverName,
		DSN:        dsn,
		Conn:       db,
	}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		jftradeErr1 := db.Close()
		jftradeLogError(jftradeErr1)
		return nil, err
	}
	return &SQLiteSessionService{Service: service, db: db}, nil
}

func (s *SQLiteSessionService) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
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

func sqliteSessionSchemaReady(db *sql.DB) (bool, error) {
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

func sqliteTableExists(db *sql.DB, tableName string) (bool, error) {
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
