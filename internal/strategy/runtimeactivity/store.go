package runtimeactivity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

const (
	DefaultDBFilename = "strategy-runtime.db"

	LogTable         = "strategy_log_events"
	AuditTable       = "strategy_audit_events"
	ObservationTable = "strategy_runtime_observations"

	DefaultPageSize = 50
	MaxPageSize     = 5000

	CatalogMetaTable        = "strategy_catalog_meta"
	CatalogPluginTable      = "strategy_catalog_plugins"
	CatalogStrategyTable    = "strategy_catalog_strategies"
	CatalogOperationTable   = "strategy_catalog_operations"
	DesignDefinitionTable   = "strategy_design_definitions"
	StrategySchemaComponent = "strategy"
	StrategySchemaVersion   = 1
)

type Store struct {
	mu   sync.RWMutex
	db   *sqliteconn.DB
	path string
}

type LogEvent struct {
	ID         int64
	InstanceID string
	At         time.Time
	Raw        string
	Level      string
	Source     string
}

type LogQuery struct {
	InstanceID string
	Limit      int
	Offset     int
	Level      string
	FromAt     *time.Time
	ToAt       *time.Time
}

type AuditEvent struct {
	ID         int64
	InstanceID string
	Kind       string
	Detail     string
	At         time.Time
}

type AuditQuery struct {
	InstanceID string
	Limit      int
	Offset     int
	Kind       string
	FromAt     *time.Time
	ToAt       *time.Time
}

type ObservationSnapshot struct {
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

func NewStore(dbPath string) (*Store, error) {
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

	db, err := sqliteconn.OpenX(trimmedPath)
	if err != nil {
		return nil, fmt.Errorf("open strategy runtime sqlite store: %w", err)
	}
	store := &Store{db: db, path: trimmedPath}
	if err := store.initializeOrValidateSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate strategy runtime sqlite store: %w", err)
	}
	return store, nil
}

func DeriveDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return DefaultDBFilename
	}
	return filepath.Join(directory, DefaultDBFilename)
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DB() *sqliteconn.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) initializeOrValidateSchema() error {
	return InitializeDatabase(s.db, s.path)
}

func DatabaseStatements() []string {
	return []string{
		strings.Join([]string{`CREATE TABLE ` + LogTable + ` (`,
			`id INTEGER PRIMARY KEY AUTOINCREMENT, instance_id TEXT NOT NULL, at_ms INTEGER NOT NULL, raw TEXT NOT NULL,`,
			`level TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')`}, " "),
		strings.Join([]string{`CREATE TABLE ` + AuditTable + ` (`,
			`id INTEGER PRIMARY KEY AUTOINCREMENT, instance_id TEXT NOT NULL, kind TEXT NOT NULL,`,
			`detail TEXT NOT NULL DEFAULT '', at_ms INTEGER NOT NULL)`}, " "),
		strings.Join([]string{`CREATE TABLE ` + ObservationTable + ` (`,
			`instance_id TEXT PRIMARY KEY, actual_status_snapshot TEXT NOT NULL DEFAULT '',`,
			`active_symbols_json TEXT NOT NULL DEFAULT '[]', last_closed_kline_at_ms INTEGER,`,
			`last_signal_at_ms INTEGER, last_order_at_ms INTEGER, last_error_at_ms INTEGER,`,
			`last_error TEXT NOT NULL DEFAULT '', updated_at_ms INTEGER)`}, " "),
		`CREATE TABLE ` + CatalogMetaTable + ` (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + CatalogPluginTable + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + CatalogStrategyTable + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + CatalogOperationTable + ` (operation_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '')`,
		strings.Join([]string{`CREATE TABLE ` + DesignDefinitionTable + ` (`,
			`id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '',`,
			`description TEXT NOT NULL DEFAULT '', runtime TEXT NOT NULL DEFAULT '', source_format TEXT NOT NULL DEFAULT '',`,
			`symbol TEXT NOT NULL DEFAULT '', interval TEXT NOT NULL DEFAULT '', script TEXT NOT NULL DEFAULT '',`,
			`visual_model_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '',`,
			`updated_at TEXT NOT NULL DEFAULT '', deleted_at TEXT)`}, " "),
		`CREATE INDEX idx_strategy_log_events_instance_at ON ` + LogTable + ` (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX idx_strategy_log_events_level ON ` + LogTable + ` (level)`,
		`CREATE INDEX idx_strategy_audit_events_instance_at ON ` + AuditTable + ` (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX idx_strategy_audit_events_kind ON ` + AuditTable + ` (kind)`,
		`CREATE INDEX idx_strategy_catalog_strategies_created_at ON ` + CatalogStrategyTable + ` (created_at ASC, id ASC)`,
		`CREATE INDEX idx_strategy_catalog_operations_updated_at ON ` + CatalogOperationTable + ` (updated_at DESC, operation_id ASC)`,
		`CREATE INDEX idx_strategy_design_definitions_updated_at ON ` + DesignDefinitionTable + ` (updated_at DESC, id ASC)`,
		`CREATE INDEX idx_strategy_design_definitions_deleted_at ON ` + DesignDefinitionTable + ` (deleted_at)`,
	}
}

func ValidateDatabase(ctx context.Context, db sqliteschema.Database) error {
	for _, schema := range []struct {
		table   string
		columns []string
	}{
		{LogTable, ExpectedLogSchemaColumns()},
		{AuditTable, ExpectedAuditSchemaColumns()},
		{ObservationTable, ExpectedObservationSchemaColumns()},
		{CatalogMetaTable, []string{"key:TEXT:1", "value:TEXT:0"}},
		{CatalogPluginTable, []string{"id:TEXT:1", "payload_json:TEXT:0", "updated_at:TEXT:0"}},
		{CatalogStrategyTable, []string{"id:TEXT:1", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
		{CatalogOperationTable, []string{"operation_id:TEXT:1", "plugin_id:TEXT:0", "status:TEXT:0", "updated_at:TEXT:0", "payload_json:TEXT:0"}},
		{DesignDefinitionTable, []string{
			"id:TEXT:1", "name:TEXT:0", "version:TEXT:0", "description:TEXT:0", "runtime:TEXT:0",
			"source_format:TEXT:0", "symbol:TEXT:0", "interval:TEXT:0", "script:TEXT:0",
			"visual_model_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0", "deleted_at:TEXT:0",
		}},
	} {
		if err := sqliteschema.ValidateTable(ctx, db, schema.table, schema.columns); err != nil {
			return err
		}
	}
	return nil
}

func InitializeDatabase(db sqliteschema.Database, path string) error {
	return sqliteschema.InitializeOrValidate(
		context.Background(),
		db,
		path,
		StrategySchemaComponent,
		StrategySchemaVersion,
		DatabaseStatements(),
		ValidateDatabase,
	)
}

func ExpectedLogSchemaColumns() []string {
	return []string{
		"id:INTEGER:1",
		"instance_id:TEXT:0",
		"at_ms:INTEGER:0",
		"raw:TEXT:0",
		"level:TEXT:0",
		"source:TEXT:0",
	}
}

func ExpectedAuditSchemaColumns() []string {
	return []string{
		"id:INTEGER:1",
		"instance_id:TEXT:0",
		"kind:TEXT:0",
		"detail:TEXT:0",
		"at_ms:INTEGER:0",
	}
}

func ExpectedObservationSchemaColumns() []string {
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

func (s *Store) AppendLog(ctx context.Context, event LogEvent) error {
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
		`INSERT INTO `+LogTable+` (instance_id, at_ms, raw, level, source) VALUES (?, ?, ?, ?, ?)`,
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

func (s *Store) AppendAudit(ctx context.Context, event AuditEvent) error {
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
		`INSERT INTO `+AuditTable+` (instance_id, kind, detail, at_ms) VALUES (?, ?, ?, ?)`,
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

func (s *Store) UpsertObservation(ctx context.Context, snapshot ObservationSnapshot) error {
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
			`INSERT INTO ` + ObservationTable + ` (`,
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

func (s *Store) ListLogs(ctx context.Context, query LogQuery) ([]LogEvent, error) {
	limit := NormalizePageSize(query.Limit)
	clauses, args := buildStrategyRuntimeLogClauses(query)
	args = append(args, limit, NormalizeOffset(query.Offset))

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
		`SELECT id, instance_id, at_ms, raw, level, source FROM `+LogTable+
			` WHERE `+strings.Join(clauses, ` AND `)+
			` ORDER BY at_ms DESC, id DESC LIMIT ? OFFSET ?`,
		args...,
	)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("list strategy runtime logs: %w", err)
	}

	result := make([]LogEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, LogEvent{
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

func (s *Store) CountLogs(ctx context.Context, query LogQuery) (int, error) {
	clauses, args := buildStrategyRuntimeLogClauses(query)
	var total int
	s.mu.RLock()
	err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+LogTable+` WHERE `+strings.Join(clauses, ` AND `), args...)
	s.mu.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("count strategy runtime logs: %w", err)
	}
	return total, nil
}

func (s *Store) ListRecentLogsTail(ctx context.Context, instanceID string, limit int) ([]LogEvent, error) {
	return s.ListLogs(ctx, LogQuery{InstanceID: instanceID, Limit: limit})
}

func (s *Store) ListAudit(ctx context.Context, query AuditQuery) ([]AuditEvent, error) {
	limit := NormalizePageSize(query.Limit)
	clauses, args := buildStrategyRuntimeAuditClauses(query)
	args = append(args, limit, NormalizeOffset(query.Offset))

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
		`SELECT id, instance_id, kind, detail, at_ms FROM `+AuditTable+
			` WHERE `+strings.Join(clauses, ` AND `)+
			` ORDER BY at_ms DESC, id DESC LIMIT ? OFFSET ?`,
		args...,
	)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("list strategy runtime audit: %w", err)
	}

	result := make([]AuditEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, AuditEvent{
			ID:         row.ID,
			InstanceID: row.InstanceID,
			Kind:       row.Kind,
			Detail:     row.Detail,
			At:         time.UnixMilli(row.AtMs).UTC(),
		})
	}
	return result, nil
}

func (s *Store) CountAudit(ctx context.Context, query AuditQuery) (int, error) {
	clauses, args := buildStrategyRuntimeAuditClauses(query)
	var total int
	s.mu.RLock()
	err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+AuditTable+` WHERE `+strings.Join(clauses, ` AND `), args...)
	s.mu.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("count strategy runtime audit: %w", err)
	}
	return total, nil
}

func (s *Store) GetObservation(ctx context.Context, instanceID string) (ObservationSnapshot, bool, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return ObservationSnapshot{}, false, nil
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
		`SELECT instance_id, actual_status_snapshot, active_symbols_json, last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms, last_error_at_ms, last_error, updated_at_ms FROM `+ObservationTable+` WHERE instance_id = ?`,
		instanceID,
	)
	s.mu.RUnlock()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ObservationSnapshot{}, false, nil
		}
		return ObservationSnapshot{}, false, fmt.Errorf("get strategy runtime observation: %w", err)
	}

	activeSymbols := []string{}
	if strings.TrimSpace(row.ActiveSymbolsJSON) != "" {
		if unmarshalErr := json.Unmarshal([]byte(row.ActiveSymbolsJSON), &activeSymbols); unmarshalErr != nil {
			return ObservationSnapshot{}, false, fmt.Errorf("decode strategy runtime active symbols: %w", unmarshalErr)
		}
	}

	return ObservationSnapshot{
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

func NormalizePageSize(limit int) int {
	if limit <= 0 {
		return DefaultPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}

func NormalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func buildStrategyRuntimeLogClauses(query LogQuery) ([]string, []any) {
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

func buildStrategyRuntimeAuditClauses(query AuditQuery) ([]string, []any) {
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
