package adk

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	adksession "google.golang.org/adk/session"
	adksessiondb "google.golang.org/adk/session/database"
)

type SQLiteSessionService struct {
	adksession.Service
	db *sql.DB
}

func NewSQLiteSessionService(path string) (*SQLiteSessionService, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)"
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	service, err := adksessiondb.NewSessionService(sqliteDialector{
		DriverName: sqliteDriverName,
		DSN:        dsn,
		Conn:       db,
	})
	if err != nil {
		_ = db.Close()
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

func MigrateSQLiteSessionService(service adksession.Service) error {
	if wrapper, ok := service.(*SQLiteSessionService); ok && wrapper != nil {
		if ready, err := sqliteSessionSchemaReady(wrapper.db); err == nil && ready {
			return nil
		} else if err != nil {
			return err
		}
		service = wrapper.Service
	}
	err := adksessiondb.AutoMigrate(service)
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "already exists") {
		return nil
	}
	return err
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
	err := db.QueryRow(
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
