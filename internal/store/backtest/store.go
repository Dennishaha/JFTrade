// Package backtest owns durable backtest-run state and its in-process sync-task registry.
package backtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/chart"
)

const (
	defaultRunDBFilename = "backtest-runs.db"
	// RecoveredRunErrorText explains why an unfinished persisted run is failed on startup.
	RecoveredRunErrorText = "sidecar restarted before backtest completed"
)

// Store persists backtest run metadata and results while keeping the active view in memory.
// It directly implements internal/backtest.RunStore.
type Store struct {
	mu      sync.RWMutex
	runs    map[string]*btsrv.RunState
	cancels map[string]context.CancelFunc
	db      *sqliteconn.DB
	dbPath  string

	closeOnce sync.Once
	closeErr  error
}

// New opens a durable backtest-run store and restores its persisted state.
func New(dbPath string) (Resource, error) {
	return openStore(dbPath)
}

func openStore(dbPath string) (*Store, error) {
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

	db, err := sqliteconn.OpenX(trimmedPath)
	if err != nil {
		return nil, fmt.Errorf("open backtest run sqlite store: %w", err)
	}
	store := newStore(db, trimmedPath)
	if err := store.initializeOrValidateSchema(); err != nil {
		besteffort.LogError(db.Close())
		return nil, fmt.Errorf("migrate backtest run sqlite store: %w", err)
	}
	if err := store.loadFromDB(); err != nil {
		besteffort.LogError(db.Close())
		return nil, fmt.Errorf("load backtest run sqlite store: %w", err)
	}
	return store, nil
}

// NewInMemory returns a non-persisting store for degraded startup.
func NewInMemory() Resource {
	return newInMemoryStore()
}

func newInMemoryStore() *Store {
	return newStore(nil, "")
}

func newStore(db *sqliteconn.DB, dbPath string) *Store {
	return &Store{
		runs:    make(map[string]*btsrv.RunState),
		cancels: make(map[string]context.CancelFunc),
		db:      db,
		dbPath:  dbPath,
	}
}

// DerivePath resolves the run database next to settings unless explicitly overridden.
func DerivePath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_RUN_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultRunDBFilename
	}
	return filepath.Join(directory, defaultRunDBFilename)
}

// Available reports whether this store has durable SQLite persistence.
func (s *Store) Available() bool {
	return s != nil && s.db != nil
}

// Close releases the SQLite connection. It is safe to call multiple times.
func (s *Store) Close() error {
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

// Add inserts or replaces a run and rolls the memory view back if persistence fails.
func (s *Store) Add(run *btsrv.RunState) error {
	snapshot := cloneRunState(run)
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

// Get returns an independent lightweight run snapshot.
func (s *Store) Get(runID string) (*btsrv.RunState, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false
	}
	return cloneRunState(run), true
}

// GetFull loads the persisted result payload for a run when durable storage is available.
func (s *Store) GetFull(runID string) (*btsrv.RunState, bool, error) {
	snapshot, ok := s.Get(runID)
	if !ok || s == nil || s.db == nil {
		return snapshot, ok, nil
	}
	result, found, err := s.loadResult(runID)
	if err != nil {
		return nil, true, err
	}
	if found && result != nil {
		snapshot.Result = result
		if strings.TrimSpace(snapshot.MarketDataProvider) == "" {
			snapshot.MarketDataProvider = result.MarketDataProvider
		}
		if strings.TrimSpace(result.MarketDataProvider) == "" {
			result.MarketDataProvider = snapshot.MarketDataProvider
		}
	}
	return snapshot, true, nil
}

// List returns independent snapshots of all in-memory runs.
func (s *Store) List() []*btsrv.RunState {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	runs := make([]*btsrv.RunState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, cloneRunState(run))
	}
	return runs
}

// ListLightweight omits result payloads from each returned run.
func (s *Store) ListLightweight() []*btsrv.RunState {
	runs := s.List()
	for _, run := range runs {
		if run != nil {
			run.Result = nil
		}
	}
	return runs
}

// Update mutates a run and restores its previous snapshot if persistence fails.
func (s *Store) Update(runID string, mutate func(*btsrv.RunState)) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return false, nil
	}
	previous := cloneRunState(run)
	mutate(run)
	if err := s.persistRunLocked(run); err != nil {
		s.runs[runID] = previous
		return true, err
	}
	return true, nil
}

// UpdateMemoryOnly mutates a run without touching durable state.
func (s *Store) UpdateMemoryOnly(runID string, mutate func(*btsrv.RunState)) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return false
	}
	mutate(run)
	return true
}

// Delete removes a run from durable and in-memory state.
func (s *Store) Delete(runID string) (*btsrv.RunState, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	if err := s.deleteFromDBLocked(runID); err != nil {
		return nil, true, err
	}
	snapshot := cloneRunState(run)
	delete(s.runs, runID)
	return snapshot, true, nil
}

// SetCancel registers or clears the cancellation hook for an active run.
func (s *Store) SetCancel(runID string, cancel context.CancelFunc) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel == nil {
		delete(s.cancels, runID)
		return
	}
	s.cancels[runID] = cancel
}

// Cancel invokes a registered cancellation hook.
func (s *Store) Cancel(runID string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	cancel := s.cancels[runID]
	s.mu.RUnlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func cloneRunState(run *btsrv.RunState) *btsrv.RunState {
	if run == nil {
		return nil
	}
	snapshot := *run
	snapshot.Request.ChartType = chart.NormalizeChartType(string(snapshot.Request.ChartType))
	snapshot.Result = run.Result.Snapshot()
	return &snapshot
}
