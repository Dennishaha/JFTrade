package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	_ "modernc.org/sqlite"
)

type snapshot struct {
	ComponentID string  `json:"componentId"`
	Version     int64   `json:"version"`
	Pragmas     pragmas `json:"pragmas"`
	Tables      []table `json:"tables"`
	KLines      []kline `json:"klines"`
}

type pragmas struct {
	ForeignKeys int64  `json:"foreignKeys"`
	QueryOnly   int64  `json:"queryOnly"`
	BusyTimeout int64  `json:"busyTimeout"`
	JournalMode string `json:"journalMode"`
}

type table struct {
	Name         string   `json:"name"`
	WithoutRowID bool     `json:"withoutRowid"`
	Columns      []column `json:"columns"`
}

type column struct {
	CID        int64  `json:"cid"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	NotNull    int64  `json:"notNull"`
	PrimaryKey int64  `json:"primaryKey"`
	Hidden     int64  `json:"hidden"`
}

type kline struct {
	Table     string `json:"table"`
	EndTime   int64  `json:"endTime"`
	StartTime int64  `json:"startTime"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
}

func main() {
	sqlPath := flag.String("sql", "", "SQL fixture to materialize")
	dbPath := flag.String("db", "", "new SQLite database path")
	inspectOnly := flag.Bool("inspect-only", false, "inspect an existing database without materializing")
	flag.Parse()
	valid := strings.TrimSpace(*dbPath) != "" && flag.NArg() == 0
	valid = valid && ((*inspectOnly && strings.TrimSpace(*sqlPath) == "") ||
		(!*inspectOnly && strings.TrimSpace(*sqlPath) != ""))
	if !valid {
		fmt.Fprintln(os.Stderr, "usage: sqlite-oracle (--sql <fixture.sql> | --inspect-only) --db <database.db>")
		os.Exit(2)
	}
	if err := run(*sqlPath, *dbPath, *inspectOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(sqlPath, dbPath string, inspectOnly bool) error {
	if inspectOnly {
		if info, err := os.Stat(dbPath); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("inspect-only database is not a regular file: %s", dbPath)
		}
	} else {
		if err := materialize(sqlPath, dbPath); err != nil {
			return err
		}
	}
	ctx := context.Background()
	if err := sqliteschema.ValidateCurrentFile(ctx, dbPath, sqliteschema.DatabaseBacktest); err != nil {
		return fmt.Errorf("validate Go schema contract: %w", err)
	}
	value, err := inspect(ctx, dbPath)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode canonical snapshot: %w", err)
	}
	return nil
}

func materialize(sqlPath, dbPath string) error {
	if _, err := os.Stat(dbPath); err == nil {
		return fmt.Errorf("refuse to overwrite existing SQLite database: %s", dbPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect SQLite output path: %w", err)
	}
	script, err := os.ReadFile(sqlPath)
	if err != nil {
		return fmt.Errorf("read SQL fixture: %w", err)
	}
	writer, err := sql.Open(sqliteconn.DriverName, dbPath)
	if err != nil {
		return fmt.Errorf("open fixture database: %w", err)
	}
	writer.SetMaxOpenConns(1)
	if _, err := writer.Exec(string(script)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("materialize SQL fixture: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close fixture database: %w", err)
	}
	return nil
}

func inspect(ctx context.Context, path string) (result snapshot, resultErr error) {
	db, err := sql.Open(sqliteconn.DriverName, sqliteconn.ReadOnlyDSN(path))
	if err != nil {
		return result, fmt.Errorf("open read-only Go oracle: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		if closeErr := db.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close read-only Go oracle: %w", closeErr)
		}
	}()
	if _, err := db.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return result, fmt.Errorf("enable query_only: %w", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT component_id, version FROM jftrade_schema_meta",
	).Scan(&result.ComponentID, &result.Version); err != nil {
		return result, fmt.Errorf("read schema metadata: %w", err)
	}
	if err := readPragmas(ctx, db, &result.Pragmas); err != nil {
		return result, err
	}
	result.Tables, err = readTables(ctx, db)
	if err != nil {
		return result, err
	}
	result.KLines, err = readKLines(ctx, db, result.Tables)
	return result, err
}

func readPragmas(ctx context.Context, db *sql.DB, result *pragmas) error {
	checks := []struct {
		query  string
		target any
	}{
		{"PRAGMA foreign_keys", &result.ForeignKeys},
		{"PRAGMA query_only", &result.QueryOnly},
		{"PRAGMA busy_timeout", &result.BusyTimeout},
		{"PRAGMA journal_mode", &result.JournalMode},
	}
	for _, check := range checks {
		if err := db.QueryRowContext(ctx, check.query).Scan(check.target); err != nil {
			return fmt.Errorf("read %s: %w", check.query, err)
		}
	}
	return nil
}

func readTables(ctx context.Context, db *sql.DB) (result []table, resultErr error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name, sql FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("list SQLite tables: %w", err)
	}
	var entries []struct {
		name      string
		statement string
	}
	for rows.Next() {
		var entry struct {
			name      string
			statement string
		}
		if err := rows.Scan(&entry.name, &entry.statement); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan SQLite table: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate SQLite tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close SQLite table rows: %w", err)
	}
	for _, entry := range entries {
		item := table{
			Name:         entry.name,
			WithoutRowID: strings.Contains(strings.ToUpper(entry.statement), "WITHOUT ROWID"),
		}
		item.Columns, err = readColumns(ctx, db, item.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func readColumns(ctx context.Context, db *sql.DB, tableName string) (result []column, resultErr error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_xinfo("`+tableName+`")`)
	if err != nil {
		return nil, fmt.Errorf("inspect table %s: %w", tableName, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close columns for %s: %w", tableName, closeErr)
		}
	}()
	for rows.Next() {
		var item column
		var defaultValue sql.NullString
		if err := rows.Scan(
			&item.CID, &item.Name, &item.Type, &item.NotNull, &defaultValue, &item.PrimaryKey, &item.Hidden,
		); err != nil {
			return nil, fmt.Errorf("scan columns for %s: %w", tableName, err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func readKLines(ctx context.Context, db *sql.DB, tables []table) (result []kline, resultErr error) {
	for _, table := range tables {
		if !strings.HasPrefix(table.Name, "local_klines__") {
			continue
		}
		rows, err := db.QueryContext(ctx,
			`SELECT end_time, start_time, open, high, low, close, volume FROM "`+table.Name+`" ORDER BY end_time ASC`,
		)
		if err != nil {
			return nil, fmt.Errorf("read K-lines from %s: %w", table.Name, err)
		}
		for rows.Next() {
			var item kline
			var values [5]string
			item.Table = table.Name
			if err := rows.Scan(
				&item.EndTime, &item.StartTime, &values[0], &values[1], &values[2], &values[3], &values[4],
			); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan K-line from %s: %w", table.Name, err)
			}
			canonical := make([]string, len(values))
			for index, value := range values {
				fixed, err := fixedpoint.NewFromString(value)
				if err != nil {
					_ = rows.Close()
					return nil, fmt.Errorf("decode K-line fixedpoint %q: %w", value, err)
				}
				canonical[index] = fixed.String()
			}
			item.Open, item.High, item.Low, item.Close, item.Volume =
				canonical[0], canonical[1], canonical[2], canonical[3], canonical[4]
			result = append(result, item)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close K-lines from %s: %w", table.Name, err)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Table != result[j].Table {
			return result[i].Table < result[j].Table
		}
		return result[i].EndTime < result[j].EndTime
	})
	return result, nil
}
