package strategy

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
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

// RuntimeActivityResource is the persisted runtime event/observation boundary.
// It deliberately does not expose the underlying SQLite connection.
type RuntimeActivityResource interface {
	runtimeactivity.Store
	Available() bool
	Close() error
}

type runtimeActivityStore struct {
	mu        sync.RWMutex
	db        *sqliteconn.DB
	path      string
	closeOnce sync.Once
	closeErr  error
}

var (
	_ runtimeactivity.Store   = (*runtimeActivityStore)(nil)
	_ RuntimeActivityResource = (*runtimeActivityStore)(nil)
)

// OpenRuntimeActivity opens the strategy runtime database behind narrow
// activity ports. Catalog assembly normally uses NewCatalog instead.
func OpenRuntimeActivity(dbPath string) (RuntimeActivityResource, error) {
	return openRuntimeActivity(dbPath)
}

func openRuntimeActivity(dbPath string) (*runtimeActivityStore, error) {
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
	store := &runtimeActivityStore{db: db, path: trimmedPath}
	if err := initializeStrategyDatabase(db, trimmedPath); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate strategy runtime sqlite store: %w", err)
	}
	return store, nil
}

func (s *runtimeActivityStore) Available() bool {
	return s != nil && s.db != nil
}

func (s *runtimeActivityStore) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.db != nil {
			s.closeErr = s.db.Close()
		}
	})
	return s.closeErr
}

func (s *runtimeActivityStore) AppendLog(ctx context.Context, event runtimeactivity.LogEvent) error {
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

func (s *runtimeActivityStore) AppendAudit(ctx context.Context, event runtimeactivity.AuditEvent) error {
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

func (s *runtimeActivityStore) UpsertObservation(ctx context.Context, snapshot runtimeactivity.ObservationSnapshot) error {
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
		runtimeTimeToNullMillis(snapshot.LastClosedKLineAt),
		runtimeTimeToNullMillis(snapshot.LastSignalAt),
		runtimeTimeToNullMillis(snapshot.LastOrderAt),
		runtimeTimeToNullMillis(snapshot.LastErrorAt),
		snapshot.LastError,
		runtimeTimeToNullMillis(snapshot.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert strategy runtime observation: %w", err)
	}
	return nil
}

func (s *runtimeActivityStore) ListLogs(ctx context.Context, query runtimeactivity.LogQuery) ([]runtimeactivity.LogEvent, error) {
	query = runtimeactivity.NormalizeLogQuery(query)
	clauses, args := runtimeLogClauses(query)
	args = append(args, query.Limit, query.Offset)

	var rows []runtimeLogRow
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

	result := make([]runtimeactivity.LogEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, runtimeactivity.LogEvent{
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

func (s *runtimeActivityStore) CountLogs(ctx context.Context, query runtimeactivity.LogQuery) (int, error) {
	query = runtimeactivity.NormalizeLogQuery(query)
	clauses, args := runtimeLogClauses(query)
	var total int
	s.mu.RLock()
	err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+strategyRuntimeLogTable+` WHERE `+strings.Join(clauses, ` AND `), args...)
	s.mu.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("count strategy runtime logs: %w", err)
	}
	return total, nil
}

func (s *runtimeActivityStore) ListRecentLogsTail(ctx context.Context, instanceID string, limit int) ([]runtimeactivity.LogEvent, error) {
	return s.ListLogs(ctx, runtimeactivity.LogQuery{InstanceID: instanceID, Limit: limit})
}

func (s *runtimeActivityStore) ListAudit(ctx context.Context, query runtimeactivity.AuditQuery) ([]runtimeactivity.AuditEvent, error) {
	query = runtimeactivity.NormalizeAuditQuery(query)
	clauses, args := runtimeAuditClauses(query)
	args = append(args, query.Limit, query.Offset)

	var rows []runtimeAuditRow
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

	result := make([]runtimeactivity.AuditEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, runtimeactivity.AuditEvent{
			ID:         row.ID,
			InstanceID: row.InstanceID,
			Kind:       row.Kind,
			Detail:     row.Detail,
			At:         time.UnixMilli(row.AtMs).UTC(),
		})
	}
	return result, nil
}

func (s *runtimeActivityStore) CountAudit(ctx context.Context, query runtimeactivity.AuditQuery) (int, error) {
	query = runtimeactivity.NormalizeAuditQuery(query)
	clauses, args := runtimeAuditClauses(query)
	var total int
	s.mu.RLock()
	err := s.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM `+strategyRuntimeAuditTable+` WHERE `+strings.Join(clauses, ` AND `), args...)
	s.mu.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("count strategy runtime audit: %w", err)
	}
	return total, nil
}

func (s *runtimeActivityStore) GetObservation(ctx context.Context, instanceID string) (runtimeactivity.ObservationSnapshot, bool, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return runtimeactivity.ObservationSnapshot{}, false, nil
	}

	var row runtimeObservationRow
	s.mu.RLock()
	err := s.db.GetContext(
		ctx,
		&row,
		`SELECT instance_id, actual_status_snapshot, active_symbols_json, last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms, last_error_at_ms, last_error, updated_at_ms FROM `+strategyRuntimeObservationTable+` WHERE instance_id = ?`,
		instanceID,
	)
	s.mu.RUnlock()
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeactivity.ObservationSnapshot{}, false, nil
	}
	if err != nil {
		return runtimeactivity.ObservationSnapshot{}, false, fmt.Errorf("get strategy runtime observation: %w", err)
	}

	activeSymbols := []string{}
	if strings.TrimSpace(row.ActiveSymbolsJSON) != "" {
		if err := json.Unmarshal([]byte(row.ActiveSymbolsJSON), &activeSymbols); err != nil {
			return runtimeactivity.ObservationSnapshot{}, false, fmt.Errorf("decode strategy runtime active symbols: %w", err)
		}
	}
	return runtimeactivity.ObservationSnapshot{
		InstanceID:        row.InstanceID,
		ActualStatus:      row.ActualStatusSnapshot,
		ActiveSymbols:     activeSymbols,
		LastClosedKLineAt: runtimeNullMillisToTime(row.LastClosedKLineAtMs),
		LastSignalAt:      runtimeNullMillisToTime(row.LastSignalAtMs),
		LastOrderAt:       runtimeNullMillisToTime(row.LastOrderAtMs),
		LastErrorAt:       runtimeNullMillisToTime(row.LastErrorAtMs),
		LastError:         row.LastError,
		UpdatedAt:         runtimeNullMillisToTime(row.UpdatedAtMs),
	}, true, nil
}

type runtimeLogRow struct {
	ID         int64  `db:"id"`
	InstanceID string `db:"instance_id"`
	AtMs       int64  `db:"at_ms"`
	Raw        string `db:"raw"`
	Level      string `db:"level"`
	Source     string `db:"source"`
}

type runtimeAuditRow struct {
	ID         int64  `db:"id"`
	InstanceID string `db:"instance_id"`
	Kind       string `db:"kind"`
	Detail     string `db:"detail"`
	AtMs       int64  `db:"at_ms"`
}

type runtimeObservationRow struct {
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

func runtimeLogClauses(query runtimeactivity.LogQuery) ([]string, []any) {
	clauses := []string{"instance_id = ?"}
	args := []any{query.InstanceID}
	if query.Level != "" {
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

func runtimeAuditClauses(query runtimeactivity.AuditQuery) ([]string, []any) {
	clauses := []string{"instance_id = ?"}
	args := []any{query.InstanceID}
	if query.Kind != "" {
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

func runtimeTimeToNullMillis(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().UnixMilli()
}

func runtimeNullMillisToTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	return new(time.UnixMilli(value.Int64).UTC())
}
