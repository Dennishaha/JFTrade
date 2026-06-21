package sqliteschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const MetadataTable = "jftrade_schema_meta"

type IncompatibleError struct {
	Component string
	Path      string
	Reason    string
}

func (e *IncompatibleError) Error() string {
	return fmt.Sprintf("%s database schema is incompatible: %s; rebuild database %s", e.Component, e.Reason, e.Path)
}

func IsIncompatible(err error) bool {
	var target *IncompatibleError
	return errors.As(err, &target)
}

func InitializeOrValidate(
	ctx context.Context,
	db *sqlx.DB,
	path string,
	component string,
	version int,
	statements []string,
	validate func(context.Context, *sqlx.DB) error,
) error {
	if db == nil {
		return fmt.Errorf("%s database is unavailable", component)
	}
	newDatabase, err := databaseIsNew(path)
	if err != nil {
		return err
	}
	if newDatabase {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		rollback := true
		defer func() {
			if rollback {
				_ = tx.Rollback()
			}
		}()
		for _, statement := range statements {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("initialize %s database: %w", component, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`CREATE TABLE `+MetadataTable+` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`,
		); err != nil {
			return fmt.Errorf("initialize %s schema metadata: %w", component, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO `+MetadataTable+` (component_id, version, created_at) VALUES (?, ?, ?)`,
			component, version, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record %s schema metadata: %w", component, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		rollback = false
	}
	if err := ValidateMetadata(ctx, db, path, component, version); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(ctx, db); err != nil {
			if IsIncompatible(err) {
				return err
			}
			return &IncompatibleError{Component: component, Path: path, Reason: err.Error()}
		}
	}
	return nil
}

func ValidateMetadata(ctx context.Context, db *sqlx.DB, path string, component string, version int) error {
	var table string
	err := db.QueryRowxContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1`,
		MetadataTable,
	).Scan(&table)
	if errors.Is(err, sql.ErrNoRows) {
		return &IncompatibleError{Component: component, Path: path, Reason: "schema metadata is missing"}
	}
	if err != nil {
		return err
	}
	var storedVersion int
	err = db.QueryRowxContext(ctx,
		`SELECT version FROM `+MetadataTable+` WHERE component_id = ? LIMIT 1`,
		component,
	).Scan(&storedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return &IncompatibleError{Component: component, Path: path, Reason: "component metadata is missing"}
	}
	if err != nil {
		return err
	}
	if storedVersion != version {
		return &IncompatibleError{
			Component: component,
			Path:      path,
			Reason:    fmt.Sprintf("schema version %d does not match required version %d", storedVersion, version),
		}
	}
	return nil
}

func databaseIsNew(path string) (bool, error) {
	info, err := os.Stat(strings.TrimSpace(path))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("database path is not a regular file: %s", path)
	}
	return info.Size() == 0, nil
}

func ValidateTable(ctx context.Context, db *sqlx.DB, table string, expected []string) error {
	rows, err := db.QueryxContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	actual := make([]string, 0, len(expected))
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		actual = append(actual, fmt.Sprintf("%s:%s:%d", name, strings.ToUpper(dataType), primaryKey))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("%s columns do not match current schema", table)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return fmt.Errorf("%s columns do not match current schema", table)
		}
	}
	return nil
}
