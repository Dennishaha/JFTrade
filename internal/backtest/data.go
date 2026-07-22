package backtest

import (
	"context"
	"fmt"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

// EnsureScriptData checks local K-line coverage for a transient research script
// and starts one deduplicated sync task when coverage is missing.
func (s *Service) EnsureScriptData(ctx context.Context, req ScriptStartRequest) (*DataReadiness, error) {
	script := strings.TrimSpace(req.Script)
	if script == "" {
		return nil, requestErrorf("script is required")
	}
	prepared, err := prepareResolvedBacktest(StartRequest{
		Market: req.Market, Code: req.Code, Symbol: req.Symbol, Interval: req.Interval,
		StartDate: req.StartDate, EndDate: req.EndDate, StartTime: req.StartTime, EndTime: req.EndTime,
		InitialBalance: req.InitialBalance, RehabType: req.RehabType, UseExtendedHours: req.UseExtendedHours,
		InstrumentType: req.InstrumentType, TradingCosts: req.TradingCosts, ExecutionModel: req.ExecutionModel,
	}, transientStrategyDefinition(script))
	if err != nil {
		return nil, err
	}
	return s.ensurePreparedData(ctx, []preparedBacktest{prepared})
}

// EnsureDefinitionsData checks the union of K-line requirements for optimization candidates.
func (s *Service) EnsureDefinitionsData(ctx context.Context, req StartRequest, definitionIDs []string) (*DataReadiness, error) {
	if s.strategies == nil {
		return nil, fmt.Errorf("strategy provider not configured")
	}
	prepared := make([]preparedBacktest, 0, len(definitionIDs))
	for _, definitionID := range definitionIDs {
		definitionID = strings.TrimSpace(definitionID)
		if definitionID == "" {
			continue
		}
		def, ok, err := s.strategies.Definition(definitionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrStrategyDefinitionNotFound, definitionID)
		}
		candidateReq := req
		candidateReq.DefinitionID = definitionID
		candidate, err := prepareResolvedBacktest(candidateReq, def)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, candidate)
	}
	if len(prepared) == 0 {
		return nil, requestErrorf("definitionIds is required")
	}
	return s.ensurePreparedData(ctx, prepared)
}

func (s *Service) ensurePreparedData(ctx context.Context, prepared []preparedBacktest) (*DataReadiness, error) {
	base, queryStart, endTime, err := combinePreparedBacktests(prepared)
	if err != nil {
		return nil, err
	}
	rehabType := normalizeRehabTypeName(base.request.RehabType)
	readSessionScope := backtestReadSessionScope(base.request.UseExtendedHours)
	syncSessionScope := backtestSyncSessionScope(base.request.UseExtendedHours)
	covered, coverageErr := s.hasKLineCoverage(base.request.Symbol, base.request.Interval, queryStart, endTime, rehabType, readSessionScope)
	if coverageErr != nil && !isMissingKLineCoverageError(coverageErr) {
		return nil, coverageErr
	}
	if covered {
		key := dataSyncKey(base.request.Symbol, base.request.Interval, queryStart, endTime, rehabType, syncSessionScope)
		s.dataSyncMu.Lock()
		delete(s.dataSyncTasks, key)
		s.pruneDataSyncTasksLocked("")
		s.dataSyncMu.Unlock()
		return &DataReadiness{Status: DataStatusReady, Ready: true}, nil
	}
	return s.ensureMissingCoverage(ctx, base, queryStart, endTime, rehabType, syncSessionScope, coverageErr)
}

func combinePreparedBacktests(prepared []preparedBacktest) (preparedBacktest, time.Time, time.Time, error) {
	base := prepared[0]
	queryStart := base.queryStart
	endTime := base.endTime
	for _, candidate := range prepared[1:] {
		if candidate.request.Symbol != base.request.Symbol || candidate.request.Interval != base.request.Interval {
			return preparedBacktest{}, time.Time{}, time.Time{}, requestErrorf("optimization candidates must use the same symbol and interval")
		}
		if candidate.queryStart.Before(queryStart) {
			queryStart = candidate.queryStart
		}
		if candidate.endTime.After(endTime) {
			endTime = candidate.endTime
		}
	}
	return base, queryStart, endTime, nil
}

func (s *Service) ensureMissingCoverage(
	ctx context.Context,
	base preparedBacktest,
	queryStart time.Time,
	endTime time.Time,
	rehabType string,
	syncSessionScope string,
	coverageErr error,
) (*DataReadiness, error) {
	key := dataSyncKey(base.request.Symbol, base.request.Interval, queryStart, endTime, rehabType, syncSessionScope)
	s.dataSyncMu.Lock()
	defer s.dataSyncMu.Unlock()
	s.pruneDataSyncTasksLocked(key)
	if existing := s.dataSyncTasks[key]; existing != nil {
		ready, handled := s.readinessForExistingSync(key, existing, coverageErr)
		if handled {
			return ready, nil
		}
	}
	started, err := s.Sync(ctx, SyncRequest{
		Symbol: base.request.Symbol, Intervals: []string{base.request.Interval},
		Since: queryStart.UTC().Format(time.RFC3339Nano), Until: endTime.UTC().Format(time.RFC3339Nano),
		RehabType: rehabType, SessionScope: syncSessionScope,
	})
	if err != nil {
		return nil, err
	}
	s.dataSyncTasks[key] = started
	progress, _ := s.GetSyncProgress(started.TaskID)
	return readinessForSyncProgress(progress, started), nil
}

func (s *Service) readinessForExistingSync(key string, existing *SyncStarted, coverageErr error) (*DataReadiness, bool) {
	if progress, ok := s.GetSyncProgress(existing.TaskID); ok {
		switch progress.Status {
		case "queued", "running":
			return readinessForSyncProgress(progress, existing), true
		case "failed":
			delete(s.dataSyncTasks, key)
			return &DataReadiness{Status: DataStatusSyncFailed, Sync: existing, Progress: progress, Error: progress.Error}, true
		case "cancelled":
			delete(s.dataSyncTasks, key)
			return &DataReadiness{Status: DataStatusSyncCancelled, Sync: existing, Progress: progress, Error: progress.Error}, true
		case "completed":
			delete(s.dataSyncTasks, key)
			return &DataReadiness{Status: DataStatusInsufficientAfterSync, Sync: existing, Progress: progress, Error: coverageErr.Error()}, true
		}
	}
	delete(s.dataSyncTasks, key)
	return nil, false
}

// Keep the current terminal result long enough for the caller to observe it,
// while removing completed entries for other requests so this deduplication map
// cannot grow with every distinct historical range.
func (s *Service) pruneDataSyncTasksLocked(currentKey string) {
	for key, started := range s.dataSyncTasks {
		if key == currentKey {
			continue
		}
		progress, ok := s.GetSyncProgress(started.TaskID)
		if !ok || isTerminalSyncStatus(progress.Status) {
			delete(s.dataSyncTasks, key)
		}
	}
}

func (s *Service) hasKLineCoverage(symbol, interval string, since, until time.Time, rehabType, sessionScope string) (bool, error) {
	if s.checkKLineCoverageFn != nil {
		err := s.checkKLineCoverageFn(s.dbPath(), symbol, interval, since, until, rehabType, sessionScope)
		return err == nil, err
	}
	store, err := bt.NewFutuKLineStore(s.dbPath())
	if err != nil {
		return false, fmt.Errorf("open backtest store for coverage check: %w", err)
	}
	defer func() { _ = store.Close() }()
	store.SetRehabType(rehabType)
	store.SetReadSessionScope(sessionScope)
	err = store.EnsureCoverage(symbol, bbgotypes.Interval(interval), since, until)
	return err == nil, err
}

func readinessForSyncProgress(progress *bt.SyncProgress, started *SyncStarted) *DataReadiness {
	return &DataReadiness{Status: DataStatusSyncing, Ready: false, Sync: started, Progress: progress}
}

func normalizeRehabTypeName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return "none"
	case "backward":
		return "backward"
	default:
		return "forward"
	}
}

func normalizeBacktestInstrumentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "etf", "fund":
		return "etf"
	default:
		return "stock"
	}
}

func backtestReadSessionScope(useExtendedHours *bool) string {
	if useExtendedHours == nil {
		return "auto"
	}
	if *useExtendedHours {
		return "extended"
	}
	return "regular"
}

func backtestSyncSessionScope(useExtendedHours *bool) string {
	if useExtendedHours == nil {
		return "legacy"
	}
	return backtestReadSessionScope(useExtendedHours)
}

func dataSyncKey(symbol, interval string, since, until time.Time, rehabType, sessionScope string) string {
	return strings.Join([]string{symbol, interval, since.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano), rehabType, sessionScope}, "|")
}

func isMissingKLineCoverageError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "missing k-line coverage")
}
