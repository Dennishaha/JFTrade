package servercore

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

	"github.com/jmoiron/sqlx"
	// Register the modernc SQLite driver for database/sql.
	_ "modernc.org/sqlite"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

const (
	defaultBacktestRunDBFilename  = "backtest-runs.db"
	backtestRunTable              = "backtest_runs"
	recoveredBacktestRunErrorText = "sidecar restarted before backtest completed"
)

type backtestRunStore struct {
	mu      sync.RWMutex
	runs    map[string]*backtestRunState
	cancels map[string]context.CancelFunc
	db      *sqlx.DB
}

type backtestRunStateRow struct {
	ID          string `db:"id"`
	Status      string `db:"status"`
	RequestJSON string `db:"request_json"`
	ResultJSON  string `db:"result_json"`
	CreatedAt   string `db:"created_at"`
	UpdatedAt   string `db:"updated_at"`
}

func cloneBacktestRunState(run *backtestRunState) *backtestRunState {
	if run == nil {
		return nil
	}

	snapshot := *run
	snapshot.Result = run.Result.Snapshot()
	return &snapshot
}

func deriveBacktestRunDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_RUN_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultBacktestRunDBFilename
	}
	return filepath.Join(directory, defaultBacktestRunDBFilename)
}

func newBacktestRunStore() *backtestRunStore {
	return &backtestRunStore{runs: make(map[string]*backtestRunState), cancels: make(map[string]context.CancelFunc)}
}

func newBacktestRunStoreWithDB(dbPath string) (*backtestRunStore, error) {
	trimmedPath := strings.TrimSpace(dbPath)
	if trimmedPath == "" {
		return nil, fmt.Errorf("backtest run db path is required")
	}
	directory := filepath.Dir(trimmedPath)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create backtest run db directory: %w", err)
		}
	}

	db, err := sqlx.Open("sqlite", trimmedPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open backtest run sqlite store: %w", err)
	}

	store := &backtestRunStore{
		runs:    make(map[string]*backtestRunState),
		cancels: make(map[string]context.CancelFunc),
		db:      db,
	}
	if err := store.migrate(); err != nil {
		jftradeErr2 := db.Close()
		jftradeLogError(jftradeErr2)
		return nil, fmt.Errorf("migrate backtest run sqlite store: %w", err)
	}
	if err := store.loadFromDB(); err != nil {
		jftradeErr1 := db.Close()
		jftradeLogError(jftradeErr1)
		return nil, fmt.Errorf("load backtest run sqlite store: %w", err)
	}
	return store, nil
}

func (s *backtestRunStore) setCancel(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel == nil {
		delete(s.cancels, runID)
		return
	}
	s.cancels[runID] = cancel
}

func (s *backtestRunStore) cancel(runID string) bool {
	s.mu.RLock()
	cancel := s.cancels[runID]
	s.mu.RUnlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// Close releases the underlying SQLite database connection.
// It is safe to call Close multiple times.
func (s *backtestRunStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *backtestRunStore) migrate() error {
	if s == nil || s.db == nil {
		return nil
	}
	for _, statement := range []string{
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + backtestRunTable + ` (`,
			`  id           TEXT PRIMARY KEY,`,
			`  status       TEXT NOT NULL DEFAULT '',`,
			`  request_json TEXT NOT NULL DEFAULT '',`,
			`  result_json  TEXT NOT NULL DEFAULT '',`,
			`  created_at   TEXT NOT NULL DEFAULT '',`,
			`  updated_at   TEXT NOT NULL DEFAULT ''`,
			`)`,
		}, " "),
		`CREATE INDEX IF NOT EXISTS idx_backtest_runs_updated_at ON ` + backtestRunTable + ` (updated_at DESC, id ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_backtest_runs_status ON ` + backtestRunTable + ` (status, updated_at DESC)`,
	} {
		if _, err := s.db.ExecContext(context.Background(), statement); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
				return err
			}
		}
	}
	return nil
}

func (s *backtestRunStore) loadFromDB() error {
	if s == nil || s.db == nil {
		return nil
	}

	rows := []backtestRunStateRow{}
	if err := s.db.Select(&rows,
		`SELECT id, status, request_json, '' AS result_json, created_at, updated_at `+
			`FROM `+backtestRunTable+` `+
			`ORDER BY updated_at DESC, id ASC`); err != nil {
		return err
	}

	recoveredAt := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, row := range rows {
		run, err := backtestRunStateFromRow(row)
		if err != nil {
			return err
		}
		if markRecoveredBacktestRun(run, recoveredAt) {
			if err := s.persistRunLocked(run); err != nil {
				return err
			}
		}
		s.runs[run.ID] = run
	}
	return nil
}

func backtestRunStateFromRow(row backtestRunStateRow) (*backtestRunState, error) {
	var request backtestStartRequest
	if err := json.Unmarshal([]byte(row.RequestJSON), &request); err != nil {
		return nil, fmt.Errorf("decode backtest request %s: %w", row.ID, err)
	}

	result, err := decodeBacktestResultJSON(row.ID, row.ResultJSON)
	if err != nil {
		return nil, err
	}

	return &backtestRunState{
		ID:        row.ID,
		Status:    row.Status,
		Request:   request,
		Result:    result,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func decodeBacktestResultJSON(runID string, resultJSON string) (*backtest.RunResult, error) {
	if trimmed := strings.TrimSpace(resultJSON); trimmed != "" && trimmed != "null" {
		decoded := &backtest.RunResult{}
		if err := json.Unmarshal([]byte(trimmed), decoded); err != nil {
			return nil, fmt.Errorf("decode backtest result %s: %w", runID, err)
		}
		return decoded, nil
	}
	return nil, nil
}

func markRecoveredBacktestRun(run *backtestRunState, recoveredAt string) bool {
	if run == nil {
		return false
	}
	if run.Status != "queued" && run.Status != "running" {
		return false
	}

	run.Status = "failed"
	run.UpdatedAt = recoveredAt
	if run.Result == nil {
		run.Result = &backtest.RunResult{}
	}
	if strings.TrimSpace(run.Result.Symbol) == "" {
		run.Result.Symbol = run.Request.Symbol
	}
	if strings.TrimSpace(run.Result.Interval) == "" {
		run.Result.Interval = run.Request.Interval
	}
	if strings.TrimSpace(run.Result.StartTime) == "" {
		run.Result.StartTime = run.Request.StartTime
	}
	if strings.TrimSpace(run.Result.EndTime) == "" {
		run.Result.EndTime = run.Request.EndTime
	}
	if strings.TrimSpace(run.Result.Error) == "" {
		run.Result.Error = recoveredBacktestRunErrorText
	}
	return true
}

func (s *backtestRunStore) persistRunLocked(run *backtestRunState) error {
	if s == nil || s.db == nil {
		return nil
	}
	snapshot := cloneBacktestRunState(run)
	requestJSON, err := json.Marshal(snapshot.Request)
	if err != nil {
		return err
	}
	resultJSON := ""
	if snapshot.Result != nil {
		encodedResult, err := json.Marshal(snapshot.Result)
		if err != nil {
			return err
		}
		resultJSON = string(encodedResult)
	}
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO `+backtestRunTable+` (id, status, request_json, result_json, created_at, updated_at) `+
			`VALUES (?, ?, ?, ?, ?, ?) `+
			`ON CONFLICT(id) DO UPDATE SET `+
			`status=excluded.status, request_json=excluded.request_json, result_json=excluded.result_json, `+
			`created_at=excluded.created_at, updated_at=excluded.updated_at`,
		snapshot.ID,
		snapshot.Status,
		string(requestJSON),
		resultJSON,
		snapshot.CreatedAt,
		snapshot.UpdatedAt,
	)
	return err
}

func (s *backtestRunStore) deleteFromDBLocked(runID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM `+backtestRunTable+` WHERE id = ?`, runID)
	return err
}

func (s *backtestRunStore) add(run *backtestRunState) error {
	snapshot := cloneBacktestRunState(run)
	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.runs[snapshot.ID]
	s.runs[snapshot.ID] = snapshot
	if err := s.persistRunLocked(snapshot); err != nil {
		if existed {
			s.runs[snapshot.ID] = previous
		} else {
			delete(s.runs, snapshot.ID)
		}
		return err
	}
	return nil
}

func (s *backtestRunStore) list() []*backtestRunState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]*backtestRunState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, cloneBacktestRunState(run))
	}
	return runs
}

func (s *backtestRunStore) listLightweight() []*backtestRunState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]*backtestRunState, 0, len(s.runs))
	for _, run := range s.runs {
		snapshot := cloneBacktestRunState(run)
		snapshot.Result = nil
		runs = append(runs, snapshot)
	}
	return runs
}

func (s *backtestRunStore) get(runID string) (*backtestRunState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.runs[runID]
	if !ok {
		return nil, false
	}
	return cloneBacktestRunState(run), true
}

func (s *backtestRunStore) getFull(runID string) (*backtestRunState, bool, error) {
	snapshot, ok := s.get(runID)
	if !ok || s == nil || s.db == nil {
		return snapshot, ok, nil
	}
	var resultJSON string
	if err := s.db.QueryRowContext(context.Background(), `SELECT result_json FROM `+backtestRunTable+` WHERE id = ?`, runID).Scan(&resultJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return snapshot, true, nil
		}
		return nil, true, err
	}
	result, err := decodeBacktestResultJSON(runID, resultJSON)
	if err != nil {
		return nil, true, err
	}
	if result != nil {
		snapshot.Result = result
	}
	return snapshot, true, nil
}

func (s *backtestRunStore) update(runID string, mutate func(*backtestRunState)) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return false, nil
	}
	previous := cloneBacktestRunState(run)
	mutate(run)
	if err := s.persistRunLocked(run); err != nil {
		s.runs[runID] = previous
		return true, err
	}
	return true, nil
}

func (s *backtestRunStore) updateMemoryOnly(runID string, mutate func(*backtestRunState)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return false
	}
	mutate(run)
	return true
}

func (s *backtestRunStore) delete(runID string) (*backtestRunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	if err := s.deleteFromDBLocked(runID); err != nil {
		return nil, true, err
	}
	snapshot := cloneBacktestRunState(run)
	delete(s.runs, runID)
	return snapshot, true, nil
}

type backtestSyncTaskStore struct {
	mu      sync.RWMutex
	tasks   map[string]*backtest.SyncProgress
	cancels map[string]context.CancelFunc
}

func newBacktestSyncTaskStore() *backtestSyncTaskStore {
	return &backtestSyncTaskStore{
		tasks:   make(map[string]*backtest.SyncProgress),
		cancels: make(map[string]context.CancelFunc),
	}
}

func (s *backtestSyncTaskStore) add(taskID string, progress *backtest.SyncProgress, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = progress
	s.cancels[taskID] = cancel
}

func (s *backtestSyncTaskStore) get(taskID string) (*backtest.SyncProgress, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	progress, ok := s.tasks[taskID]
	if !ok {
		return nil, false
	}
	return progress.Snapshot(), true
}

func (s *backtestSyncTaskStore) finish(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, taskID)
}

func (s *backtestSyncTaskStore) cancel(taskID string, cancelledAt time.Time) (*backtest.SyncProgress, bool) {
	s.mu.Lock()
	cancel, ok := s.cancels[taskID]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	delete(s.cancels, taskID)
	progress := s.tasks[taskID]
	s.mu.Unlock()

	cancel()
	if progress != nil {
		progress.MarkCancelled(cancelledAt)
		return progress.Snapshot(), true
	}
	return nil, true
}
