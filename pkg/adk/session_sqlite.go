package adk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
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

const sqliteSessionSchemaVersion = 3
const sqliteSessionComponent = "adk-session"

func NewSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	service, err := openSQLiteSessionService(path)
	if err == nil {
		return service, nil
	}
	if !sqliteschema.IsIncompatible(err) {
		return nil, err
	}
	rebuilt, rebuildErr := rebuildSQLiteSessionService(path)
	if rebuildErr != nil {
		return nil, errors.Join(err, rebuildErr)
	}
	return rebuilt, nil
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
		jftradeLogError(jftradeErr1)
		return nil, err
	}
	if err := prepareSQLiteSessionSchema(context.Background(), db, service, path); err != nil {
		jftradeErr1 := db.Close()
		jftradeLogError(jftradeErr1)
		return nil, err
	}
	return &SQLiteSessionService{Service: service, db: db, path: strings.TrimSpace(path)}, nil
}

func prepareSQLiteSessionSchema(ctx context.Context, db *sqliteconn.DB, service adksession.Service, path string) error {
	if db == nil {
		return fmt.Errorf("ADK session database is unavailable")
	}
	metadataCurrent, err := validateSQLiteSessionMetadata(ctx, db, path)
	if err != nil {
		return err
	}
	ready, err := sqliteSessionSchemaReady(db)
	if err != nil {
		return err
	}
	if !metadataCurrent || !ready {
		if err := adksessiondb.AutoMigrate(service); err != nil {
			return &sqliteschema.IncompatibleError{
				Component: sqliteSessionComponent,
				Path:      path,
				Reason:    fmt.Sprintf("auto migrate GO-ADK session schema: %v", err),
			}
		}
	}
	if err := ensureSQLiteSessionMetadata(ctx, db); err != nil {
		return err
	}
	return validateSQLiteSessionTables(ctx, db, path)
}

func validateSQLiteSessionMetadata(ctx context.Context, db sqliteschema.Database, path string) (bool, error) {
	var table string
	err := db.QueryRowxContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1`,
		sqliteschema.MetadataTable,
	).Scan(&table)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var storedVersion int
	err = db.QueryRowxContext(ctx,
		`SELECT version FROM `+sqliteschema.MetadataTable+` WHERE component_id = ? LIMIT 1`,
		sqliteSessionComponent,
	).Scan(&storedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedVersion != sqliteSessionSchemaVersion {
		return false, &sqliteschema.IncompatibleError{
			Component: sqliteSessionComponent,
			Path:      path,
			Reason:    fmt.Sprintf("schema version %d does not match required version %d", storedVersion, sqliteSessionSchemaVersion),
		}
	}
	return true, nil
}

func ensureSQLiteSessionMetadata(ctx context.Context, db *sqliteconn.DB) error {
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS `+sqliteschema.MetadataTable+` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`,
	); err != nil {
		return fmt.Errorf("initialize ADK session schema metadata: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO `+sqliteschema.MetadataTable+` (component_id, version, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(component_id) DO UPDATE SET version = excluded.version`,
		sqliteSessionComponent,
		sqliteSessionSchemaVersion,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record ADK session schema metadata: %w", err)
	}
	return nil
}

func validateSQLiteSessionTables(ctx context.Context, db *sqliteconn.DB, path string) error {
	for _, tableName := range []string{"sessions", "events", "app_states", "user_states"} {
		exists, err := sqliteTableExists(db, tableName)
		if err != nil {
			return err
		}
		if !exists {
			return &sqliteschema.IncompatibleError{
				Component: sqliteSessionComponent,
				Path:      path,
				Reason:    "required GO-ADK session table is missing: " + tableName,
			}
		}
	}
	for tableName, columns := range map[string][]string{
		"sessions":    {"app_name", "user_id", "id"},
		"events":      {"id", "app_name", "user_id", "session_id"},
		"app_states":  {"app_name"},
		"user_states": {"app_name", "user_id"},
	} {
		for _, column := range columns {
			exists, err := sqliteTableColumnExists(ctx, db, tableName, column)
			if err != nil {
				return err
			}
			if !exists {
				return &sqliteschema.IncompatibleError{
					Component: sqliteSessionComponent,
					Path:      path,
					Reason:    fmt.Sprintf("required GO-ADK session column is missing: %s.%s", tableName, column),
				}
			}
		}
	}
	return nil
}

func rebuildSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	if err := removeSQLiteDatabaseFiles(path); err != nil {
		return nil, err
	}
	return openSQLiteSessionService(path)
}

func removeSQLiteDatabaseFiles(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("ADK session database path is empty")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove ADK session database%s: %w", suffix, err)
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
