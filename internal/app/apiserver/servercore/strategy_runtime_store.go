package servercore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const (
	defaultStrategyRuntimeDBFilename = "strategy-runtime.db"

	strategyRuntimeLogTable         = "strategy_log_events"
	strategyRuntimeAuditTable       = "strategy_audit_events"
	strategyRuntimeObservationTable = "strategy_runtime_observations"

	defaultStrategyRuntimePageSize = 50
	maxStrategyRuntimePageSize     = 5000
)

type strategyRuntimeStore struct {
	mu sync.RWMutex
	db *sqlx.DB
}

type strategyRuntimeLogEvent struct {
	ID         int64
	InstanceID string
	At         time.Time
	Raw        string
	Level      string
	Source     string
}

type strategyRuntimeLogQuery struct {
	InstanceID string
	Limit      int
	Offset     int
	Level      string
	FromAt     *time.Time
	ToAt       *time.Time
}

type strategyRuntimeAuditEvent struct {
	ID         int64
	InstanceID string
	Kind       string
	Detail     string
	At         time.Time
}

type strategyRuntimeAuditQuery struct {
	InstanceID string
	Limit      int
	Offset     int
	Kind       string
	FromAt     *time.Time
	ToAt       *time.Time
}

type strategyRuntimeObservationSnapshot struct {
	InstanceID        string
	ActualStatus      string
	ActiveSymbols     []string
	LastClosedKLineAt *time.Time
	LastSignalAt      *time.Time
	LastOrderAt       *time.Time
	LastErrorAt       *time.Time
	LastError         string
	UpdatedAt         *time.Time
}

func NewStrategyRuntimeStore(dbPath string) (*strategyRuntimeStore, error) {
	trimmedPath := strings.TrimSpace(dbPath)
	if trimmedPath == "" {
		return nil, fmt.Errorf("strategy runtime db path is required")
	}
	directory := filepath.Dir(trimmedPath)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create strategy runtime db directory: %w", err)
		}
	}

	db, err := sqlx.Open("sqlite", trimmedPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open strategy runtime sqlite store: %w", err)
	}
	store := &strategyRuntimeStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate strategy runtime sqlite store: %w", err)
	}
	return store, nil
}

func deriveStrategyRuntimeDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyRuntimeDBFilename
	}
	return filepath.Join(directory, defaultStrategyRuntimeDBFilename)
}

func (s *strategyRuntimeStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *strategyRuntimeStore) DB() *sqlx.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *strategyRuntimeStore) migrate() error {
	for _, statement := range []string{
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + strategyRuntimeLogTable + ` (`,
			`  id          INTEGER PRIMARY KEY AUTOINCREMENT,`,
			`  instance_id TEXT    NOT NULL,`,
			`  at_ms       INTEGER NOT NULL,`,
			`  raw         TEXT    NOT NULL,`,
			`  level       TEXT    NOT NULL DEFAULT '',`,
			`  source      TEXT    NOT NULL DEFAULT ''`,
			`)`,
		}, " "),
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + strategyRuntimeAuditTable + ` (`,
			`  id          INTEGER PRIMARY KEY AUTOINCREMENT,`,
			`  instance_id TEXT    NOT NULL,`,
			`  kind        TEXT    NOT NULL,`,
			`  detail      TEXT    NOT NULL DEFAULT '',`,
			`  at_ms       INTEGER NOT NULL`,
			`)`,
		}, " "),
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + strategyRuntimeObservationTable + ` (`,
			`  instance_id                 TEXT    PRIMARY KEY,`,
			`  actual_status_snapshot      TEXT    NOT NULL DEFAULT '',`,
			`  active_symbols_json         TEXT    NOT NULL DEFAULT '[]',`,
			`  last_closed_kline_at_ms     INTEGER,`,
			`  last_signal_at_ms           INTEGER,`,
			`  last_order_at_ms            INTEGER,`,
			`  last_error_at_ms            INTEGER,`,
			`  last_error                  TEXT    NOT NULL DEFAULT '',`,
			`  updated_at_ms               INTEGER`,
			`)`,
		}, " "),
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}

	for _, schema := range []struct {
		table   string
		columns []string
	}{
		{table: strategyRuntimeLogTable, columns: expectedStrategyRuntimeLogSchemaColumns()},
		{table: strategyRuntimeAuditTable, columns: expectedStrategyRuntimeAuditSchemaColumns()},
		{table: strategyRuntimeObservationTable, columns: expectedStrategyRuntimeObservationSchemaColumns()},
	} {
		if err := s.ensureSchema(schema.table, schema.columns); err != nil {
			return err
		}
	}

	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_strategy_log_events_instance_at ON ` + strategyRuntimeLogTable + ` (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_strategy_log_events_level ON ` + strategyRuntimeLogTable + ` (level)`,
		`CREATE INDEX IF NOT EXISTS idx_strategy_audit_events_instance_at ON ` + strategyRuntimeAuditTable + ` (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_strategy_audit_events_kind ON ` + strategyRuntimeAuditTable + ` (kind)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}

	return nil
}

func (s *strategyRuntimeStore) ensureSchema(tableName string, want []string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", tableName, err)
	}
	defer rows.Close()

	got := make([]string, 0, len(want))
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
	if len(got) != len(want) {
		return fmt.Errorf("%s schema is obsolete; rebuild the strategy runtime database", tableName)
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("%s schema is obsolete; rebuild the strategy runtime database", tableName)
		}
	}
	return nil
}

func expectedStrategyRuntimeLogSchemaColumns() []string {
	return []string{
		"id:INTEGER:1",
		"instance_id:TEXT:0",
		"at_ms:INTEGER:0",
		"raw:TEXT:0",
		"level:TEXT:0",
		"source:TEXT:0",
	}
}

func expectedStrategyRuntimeAuditSchemaColumns() []string {
	return []string{
		"id:INTEGER:1",
		"instance_id:TEXT:0",
		"kind:TEXT:0",
		"detail:TEXT:0",
		"at_ms:INTEGER:0",
	}
}

func expectedStrategyRuntimeObservationSchemaColumns() []string {
	return []string{
		"instance_id:TEXT:1",
		"actual_status_snapshot:TEXT:0",
		"active_symbols_json:TEXT:0",
		"last_closed_kline_at_ms:INTEGER:0",
		"last_signal_at_ms:INTEGER:0",
		"last_order_at_ms:INTEGER:0",
		"last_error_at_ms:INTEGER:0",
		"last_error:TEXT:0",
		"updated_at_ms:INTEGER:0",
	}
}

func (s *strategyRuntimeStore) AppendLog(ctx context.Context, event strategyRuntimeLogEvent) error {
	event.InstanceID = strings.TrimSpace(event.InstanceID)
	event.Raw = strings.TrimSpace(event.Raw)
	if event.InstanceID == "" {
		return fmt.Errorf("strategy runtime log instance id is required")
	}
	if event.Raw == "" {
		return fmt.Errorf("strategy runtime log raw text is required")
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.Level = strings.ToLower(strings.TrimSpace(event.Level))
	event.Source = strings.ToLower(strings.TrimSpace(event.Source))

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO `+strategyRuntimeLogTable+` (instance_id, at_ms, raw, level, source) VALUES (?, ?, ?, ?, ?)`,
		event.InstanceID,
		event.At.UTC().UnixMilli(),
		event.Raw,
		event.Level,
		event.Source,
	)
	if err != nil {
		return fmt.Errorf("insert strategy runtime log: %w", err)
	}
	return nil
}

func (s *strategyRuntimeStore) AppendAudit(ctx context.Context, event strategyRuntimeAuditEvent) error {
	event.InstanceID = strings.TrimSpace(event.InstanceID)
	event.Kind = strings.TrimSpace(event.Kind)
	event.Detail = strings.TrimSpace(event.Detail)
	if event.InstanceID == "" {
		return fmt.Errorf("strategy runtime audit instance id is required")
	}
	if event.Kind == "" {
		return fmt.Errorf("strategy runtime audit kind is required")
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO `+strategyRuntimeAuditTable+` (instance_id, kind, detail, at_ms) VALUES (?, ?, ?, ?)`,
		event.InstanceID,
		event.Kind,
		event.Detail,
		event.At.UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("insert strategy runtime audit: %w", err)
	}
	return nil
}

func (s *strategyRuntimeStore) UpsertObservation(ctx context.Context, snapshot strategyRuntimeObservationSnapshot) error {
	snapshot.InstanceID = strings.TrimSpace(snapshot.InstanceID)
	snapshot.ActualStatus = strings.TrimSpace(snapshot.ActualStatus)
	snapshot.LastError = strings.TrimSpace(snapshot.LastError)
	if snapshot.InstanceID == "" {
		return fmt.Errorf("strategy runtime observation instance id is required")
	}
	if snapshot.ActiveSymbols == nil {
		snapshot.ActiveSymbols = []string{}
	}
	activeSymbolsJSON, err := json.Marshal(snapshot.ActiveSymbols)
	if err != nil {
		return fmt.Errorf("marshal strategy runtime active symbols: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(
		ctx,
		strings.Join([]string{
			`INSERT INTO ` + strategyRuntimeObservationTable + ` (`,
			`instance_id, actual_status_snapshot, active_symbols_json, last_closed_kline_at_ms,`,
			`last_signal_at_ms, last_order_at_ms, last_error_at_ms, last_error, updated_at_ms`,
			`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			`ON CONFLICT(instance_id) DO UPDATE SET`,
			`actual_status_snapshot = excluded.actual_status_snapshot,`,
			`active_symbols_json = excluded.active_symbols_json,`,
			`last_closed_kline_at_ms = excluded.last_closed_kline_at_ms,`,
			`last_signal_at_ms = excluded.last_signal_at_ms,`,
			`last_order_at_ms = excluded.last_order_at_ms,`,
			`last_error_at_ms = excluded.last_error_at_ms,`,
			`last_error = excluded.last_error,`,
			`updated_at_ms = excluded.updated_at_ms`,
		}, " "),
		snapshot.InstanceID,
		snapshot.ActualStatus,
		string(activeSymbolsJSON),
		strategyRuntimeTimeToNullMillis(snapshot.LastClosedKLineAt),
		strategyRuntimeTimeToNullMillis(snapshot.LastSignalAt),
		strategyRuntimeTimeToNullMillis(snapshot.LastOrderAt),
		strategyRuntimeTimeToNullMillis(snapshot.LastErrorAt),
		snapshot.LastError,
		strategyRuntimeTimeToNullMillis(snapshot.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert strategy runtime observation: %w", err)
	}
	return nil
}

func (s *strategyRuntimeStore) ListLogs(ctx context.Context, query strategyRuntimeLogQuery) ([]strategyRuntimeLogEvent, error) {
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	clauses, args := buildStrategyRuntimeLogClauses(query)
	args = append(args, limit, normalizeStrategyRuntimeOffset(query.Offset))

	var rows []struct {
		ID         int64  `db:"id"`
		InstanceID string `db:"instance_id"`
		AtMs       int64  `db:"at_ms"`
		Raw        string `db:"raw"`
		Level      string `db:"level"`
		Source     string `db:"source"`
	}

	s.mu.RLock()
	err := s.db.SelectContext(
		ctx,
		&rows,
		`SELECT id, instance_id, at_ms, raw, level, source FROM `+strategyRuntimeLogTable+
			` WHERE `+strings.Join(clauses, ` AND `)+
			` ORDER BY at_ms DESC, id DESC LIMIT ? OFFSET ?`,
		args...,
	)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("list strategy runtime logs: %w", err)
	}

	result := make([]strategyRuntimeLogEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, strategyRuntimeLogEvent{
			ID:         row.ID,
			InstanceID: row.InstanceID,
			At:         time.UnixMilli(row.AtMs).UTC(),
			Raw:        row.Raw,
			Level:      row.Level,
			Source:     row.Source,
		})
	}
	return result, nil
}

func (s *strategyRuntimeStore) CountLogs(ctx context.Context, query strategyRuntimeLogQuery) (int, error) {
	clauses, args := buildStrategyRuntimeLogClauses(query)
	var total int
	s.mu.RLock()
	err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+strategyRuntimeLogTable+` WHERE `+strings.Join(clauses, ` AND `), args...)
	s.mu.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("count strategy runtime logs: %w", err)
	}
	return total, nil
}

func (s *strategyRuntimeStore) ListRecentLogsTail(ctx context.Context, instanceID string, limit int) ([]strategyRuntimeLogEvent, error) {
	return s.ListLogs(ctx, strategyRuntimeLogQuery{InstanceID: instanceID, Limit: limit})
}

func (s *strategyRuntimeStore) ListAudit(ctx context.Context, query strategyRuntimeAuditQuery) ([]strategyRuntimeAuditEvent, error) {
	limit := normalizeStrategyRuntimePageSize(query.Limit)
	clauses, args := buildStrategyRuntimeAuditClauses(query)
	args = append(args, limit, normalizeStrategyRuntimeOffset(query.Offset))

	var rows []struct {
		ID         int64  `db:"id"`
		InstanceID string `db:"instance_id"`
		Kind       string `db:"kind"`
		Detail     string `db:"detail"`
		AtMs       int64  `db:"at_ms"`
	}

	s.mu.RLock()
	err := s.db.SelectContext(
		ctx,
		&rows,
		`SELECT id, instance_id, kind, detail, at_ms FROM `+strategyRuntimeAuditTable+
			` WHERE `+strings.Join(clauses, ` AND `)+
			` ORDER BY at_ms DESC, id DESC LIMIT ? OFFSET ?`,
		args...,
	)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("list strategy runtime audit: %w", err)
	}

	result := make([]strategyRuntimeAuditEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, strategyRuntimeAuditEvent{
			ID:         row.ID,
			InstanceID: row.InstanceID,
			Kind:       row.Kind,
			Detail:     row.Detail,
			At:         time.UnixMilli(row.AtMs).UTC(),
		})
	}
	return result, nil
}

func (s *strategyRuntimeStore) CountAudit(ctx context.Context, query strategyRuntimeAuditQuery) (int, error) {
	clauses, args := buildStrategyRuntimeAuditClauses(query)
	var total int
	s.mu.RLock()
	err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+strategyRuntimeAuditTable+` WHERE `+strings.Join(clauses, ` AND `), args...)
	s.mu.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("count strategy runtime audit: %w", err)
	}
	return total, nil
}

func (s *strategyRuntimeStore) GetObservation(ctx context.Context, instanceID string) (strategyRuntimeObservationSnapshot, bool, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return strategyRuntimeObservationSnapshot{}, false, nil
	}

	var row struct {
		InstanceID           string        `db:"instance_id"`
		ActualStatusSnapshot string        `db:"actual_status_snapshot"`
		ActiveSymbolsJSON    string        `db:"active_symbols_json"`
		LastClosedKLineAtMs  sql.NullInt64 `db:"last_closed_kline_at_ms"`
		LastSignalAtMs       sql.NullInt64 `db:"last_signal_at_ms"`
		LastOrderAtMs        sql.NullInt64 `db:"last_order_at_ms"`
		LastErrorAtMs        sql.NullInt64 `db:"last_error_at_ms"`
		LastError            string        `db:"last_error"`
		UpdatedAtMs          sql.NullInt64 `db:"updated_at_ms"`
	}

	s.mu.RLock()
	err := s.db.GetContext(
		ctx,
		&row,
		`SELECT instance_id, actual_status_snapshot, active_symbols_json, last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms, last_error_at_ms, last_error, updated_at_ms FROM `+strategyRuntimeObservationTable+` WHERE instance_id = ?`,
		instanceID,
	)
	s.mu.RUnlock()
	if err != nil {
		if err == sql.ErrNoRows {
			return strategyRuntimeObservationSnapshot{}, false, nil
		}
		return strategyRuntimeObservationSnapshot{}, false, fmt.Errorf("get strategy runtime observation: %w", err)
	}

	activeSymbols := []string{}
	if strings.TrimSpace(row.ActiveSymbolsJSON) != "" {
		if unmarshalErr := json.Unmarshal([]byte(row.ActiveSymbolsJSON), &activeSymbols); unmarshalErr != nil {
			return strategyRuntimeObservationSnapshot{}, false, fmt.Errorf("decode strategy runtime active symbols: %w", unmarshalErr)
		}
	}

	return strategyRuntimeObservationSnapshot{
		InstanceID:        row.InstanceID,
		ActualStatus:      row.ActualStatusSnapshot,
		ActiveSymbols:     activeSymbols,
		LastClosedKLineAt: strategyRuntimeNullMillisToTime(row.LastClosedKLineAtMs),
		LastSignalAt:      strategyRuntimeNullMillisToTime(row.LastSignalAtMs),
		LastOrderAt:       strategyRuntimeNullMillisToTime(row.LastOrderAtMs),
		LastErrorAt:       strategyRuntimeNullMillisToTime(row.LastErrorAtMs),
		LastError:         row.LastError,
		UpdatedAt:         strategyRuntimeNullMillisToTime(row.UpdatedAtMs),
	}, true, nil
}

func normalizeStrategyRuntimePageSize(limit int) int {
	if limit <= 0 {
		return defaultStrategyRuntimePageSize
	}
	if limit > maxStrategyRuntimePageSize {
		return maxStrategyRuntimePageSize
	}
	return limit
}

func normalizeStrategyRuntimeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func buildStrategyRuntimeLogClauses(query strategyRuntimeLogQuery) ([]string, []any) {
	clauses := []string{"instance_id = ?"}
	args := []any{strings.TrimSpace(query.InstanceID)}
	if query.Level = strings.ToLower(strings.TrimSpace(query.Level)); query.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, query.Level)
	}
	if query.FromAt != nil {
		clauses = append(clauses, "at_ms >= ?")
		args = append(args, query.FromAt.UTC().UnixMilli())
	}
	if query.ToAt != nil {
		clauses = append(clauses, "at_ms <= ?")
		args = append(args, query.ToAt.UTC().UnixMilli())
	}
	return clauses, args
}

func buildStrategyRuntimeAuditClauses(query strategyRuntimeAuditQuery) ([]string, []any) {
	clauses := []string{"instance_id = ?"}
	args := []any{strings.TrimSpace(query.InstanceID)}
	if query.Kind = strings.TrimSpace(query.Kind); query.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, query.Kind)
	}
	if query.FromAt != nil {
		clauses = append(clauses, "at_ms >= ?")
		args = append(args, query.FromAt.UTC().UnixMilli())
	}
	if query.ToAt != nil {
		clauses = append(clauses, "at_ms <= ?")
		args = append(args, query.ToAt.UTC().UnixMilli())
	}
	return clauses, args
}

func strategyRuntimeTimeToNullMillis(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().UnixMilli()
}

func strategyRuntimeNullMillisToTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	return new(time.UnixMilli(value.Int64).UTC())
}
