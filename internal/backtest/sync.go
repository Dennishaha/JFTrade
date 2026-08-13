package backtest

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

// Sync 启动 K 线历史数据同步。打开 SQLite 存储 → 创建 broker 同步器 → 启动异步同步 goroutine。
func (s *Service) Sync(ctx context.Context, req SyncRequest) (*SyncStarted, error) {
	return s.syncWithProvider(ctx, req, s.backtestProviderID())
}

func (s *Service) syncWithProvider(
	ctx context.Context,
	req SyncRequest,
	providerID string,
) (*SyncStarted, error) {
	prepared, err := prepareSyncRequest(req)
	if err != nil {
		return nil, err
	}
	syncer, err := s.newSyncerForProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	params := syncParams(prepared, providerID)
	if validator, ok := syncer.(KLineSyncValidator); ok {
		if err := validator.Validate(params); err != nil {
			besteffort.LogError(syncer.Close())
			return nil, err
		}
	}
	taskID, progress, syncCtx, syncCancel, err := s.startSyncTask(ctx, prepared.request.Symbol, providerID, len(prepared.intervals))
	if err != nil {
		besteffort.LogError(syncer.Close())
		return nil, err
	}
	go s.runSyncTask(syncCtx, syncer, taskID, progress, syncCancel, params)
	return buildSyncStarted(taskID, prepared, providerID), nil
}

func prepareSyncRequest(req SyncRequest) (preparedSync, error) {
	req = applyDefaultSyncInstrument(req)
	instrument, err := parseInstrument(req.Market, req.Symbol, req.Code)
	if err != nil {
		return preparedSync{}, requestErrorf("%v", err)
	}
	req.Market = instrument.Market
	req.Code = instrument.Code
	req.Symbol = instrument.Symbol

	if len(req.Intervals) == 0 {
		req.Intervals = []string{"1m", "5m", "15m", "30m", "1h", "1d", "1w"}
	}
	sinceTime, untilTime, _, _, _, err := resolveSyncTimeRange(req.Symbol, req.StartDate, req.EndDate, req.Since, req.Until)
	if err != nil {
		return preparedSync{}, err
	}
	if !untilTime.After(sinceTime) {
		return preparedSync{}, requestErrorf("until must be after since")
	}

	req.SessionScope, err = parseSessionScope(req.SessionScope)
	if err != nil {
		return preparedSync{}, err
	}
	intervals := planSyncIntervals(req.Symbol, parseSyncIntervals(req.Intervals), req.SessionScope)
	return preparedSync{
		request:   req,
		sinceTime: sinceTime,
		untilTime: untilTime,
		intervals: intervals,
		rehabType: parseSyncRehabType(req.RehabType),
	}, nil
}

func applyDefaultSyncInstrument(req SyncRequest) SyncRequest {
	if strings.TrimSpace(req.Symbol) == "" && strings.TrimSpace(req.Code) == "" {
		req.Market = "HK"
		req.Code = "00700"
	}
	return req
}

func parseSyncIntervals(requested []string) []bbgotypes.Interval {
	var intervals []bbgotypes.Interval
	for _, iv := range requested {
		iv = strings.TrimSpace(iv)
		if iv != "" {
			intervals = append(intervals, bbgotypes.Interval(iv))
		}
	}
	if len(intervals) == 0 {
		return []bbgotypes.Interval{"1m", "5m", "1h", "1d"}
	}
	return intervals
}

func parseSyncRehabType(value string) RehabType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return RehabTypeNone
	case "backward":
		return RehabTypeBackward
	default:
		return RehabTypeForward
	}
}

func (s *Service) newSyncerForProvider(ctx context.Context, providerID string) (KLineSyncer, error) {
	if s.newProviderKLineSyncerFn != nil {
		syncer, err := s.newProviderKLineSyncerFn(ctx, s.dbPath(), providerID)
		if err != nil {
			return nil, fmt.Errorf("open %s kline sync adapter: %w", providerID, err)
		}
		return syncer, nil
	}
	if s.newKLineSyncerFn == nil {
		return nil, fmt.Errorf("kline sync adapter not configured")
	}
	syncer, err := s.newKLineSyncerFn(s.dbPath())
	if err != nil {
		return nil, fmt.Errorf("open kline sync adapter: %w", err)
	}
	return syncer, nil
}

func (s *Service) newSyncer() (KLineSyncer, error) {
	return s.newSyncerForProvider(context.Background(), s.backtestProviderID())
}

func (s *Service) startSyncTask(ctx context.Context, symbol, providerID string, intervalCount int) (string, *bt.SyncProgress, context.Context, context.CancelFunc, error) {
	taskID := fmt.Sprintf("sync-%s-%d", time.Now().UTC().Format("20060102T150405.000000000"), atomic.AddUint64(&s.syncTaskSeq, 1))
	progress := bt.NewSyncProgress(taskID, symbol, time.Now().UTC())
	progress.MarketDataProvider = providerID
	if s.syncTasks == nil {
		return "", nil, nil, nil, fmt.Errorf("sync task store not configured")
	}
	syncCtx, syncCancel, err := s.beginTask(ctx)
	if err != nil {
		return "", nil, nil, nil, err
	}
	syncCtx = observability.WithFields(syncCtx, observability.Fields{
		TaskID:       taskID,
		InstrumentID: symbol,
		Source:       "backtest",
	})
	s.syncTasks.Add(taskID, progress, syncCancel)
	observability.InfoWithImportance(syncCtx, observability.ImportanceNormal, "backtest sync task started", "interval_count", intervalCount)
	return taskID, progress, syncCtx, syncCancel, nil
}

func (s *Service) runSyncTask(
	syncCtx context.Context,
	syncer KLineSyncer,
	taskID string,
	progress *bt.SyncProgress,
	syncCancel context.CancelFunc,
	params KLineSyncParams,
) {
	defer s.finishTask(syncCancel)
	defer func() { besteffort.LogError(syncer.Close()) }()
	defer s.syncTasks.Finish(taskID)

	syncErr := syncer.Sync(syncCtx, params, progress)
	finalizeSyncProgress(syncCtx, progress, syncErr, time.Now().UTC())
	logSyncCompletion(syncCtx, progress)
}

func syncParams(prepared preparedSync, providerID string) KLineSyncParams {
	return KLineSyncParams{
		Market:             prepared.request.Market,
		MarketDataProvider: providerID,
		Symbol:             prepared.request.Symbol,
		Intervals:          prepared.intervals,
		Since:              prepared.sinceTime,
		Until:              prepared.untilTime,
		RehabType:          prepared.rehabType,
		SessionScope:       prepared.request.SessionScope,
	}
}

func finalizeSyncProgress(ctx context.Context, progress *bt.SyncProgress, syncErr error, now time.Time) {
	snapshot := progress.Snapshot()
	if ctx.Err() != nil {
		if snapshot != nil && !isTerminalSyncStatus(snapshot.Status) {
			progress.MarkCancelled(now)
		}
		return
	}
	if syncErr == nil {
		return
	}
	if snapshot != nil && !isTerminalSyncStatus(snapshot.Status) {
		progress.MarkFailed(syncErr, now)
	}
	snapshot = progress.Snapshot()
	if snapshot != nil && snapshot.Status != "cancelled" {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "backtest sync task failed", syncErr, "status", snapshot.Status)
	}
}

func logSyncCompletion(ctx context.Context, progress *bt.SyncProgress) {
	snapshot := progress.Snapshot()
	if snapshot != nil {
		observability.InfoWithImportance(ctx, observability.ImportanceNormal, "backtest sync task finished", "status", snapshot.Status, "retries", snapshot.Retries)
	}
}

func buildSyncStarted(taskID string, prepared preparedSync, providerID string) *SyncStarted {
	return &SyncStarted{
		TaskID:             taskID,
		Symbol:             prepared.request.Symbol,
		Intervals:          prepared.intervals,
		Since:              prepared.sinceTime.UTC().Format(time.RFC3339Nano),
		Until:              prepared.untilTime.UTC().Format(time.RFC3339Nano),
		SessionScope:       prepared.request.SessionScope,
		Message:            "sync started",
		MarketDataProvider: providerID,
	}
}

// GetSyncProgress 查询同步进度。
func (s *Service) GetSyncProgress(taskID string) (*bt.SyncProgress, bool) {
	if s.syncTasks == nil {
		return nil, false
	}
	return s.syncTasks.Get(taskID)
}

// CancelSync 取消正在进行的同步任务。
func (s *Service) CancelSync(taskID string) (*bt.SyncProgress, bool) {
	if s.syncTasks == nil {
		return nil, false
	}
	return s.syncTasks.Cancel(taskID, time.Now().UTC())
}

func isTerminalSyncStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// parseSessionScope accepts only the current public contract. An omitted scope
// selects regular-hours data; obsolete or misspelled values fail closed.
func parseSessionScope(scope string) (string, error) {
	switch scope {
	case "":
		return "regular", nil
	case "regular":
		return "regular", nil
	case "extended":
		return "extended", nil
	default:
		return "", requestErrorf(`sessionScope must be "regular" or "extended"`)
	}
}

// planSyncIntervals 去重并规划同步所需的 K 线周期。
func planSyncIntervals(symbol string, requested []bbgotypes.Interval, sessionScope string) []bbgotypes.Interval {
	planned := make([]bbgotypes.Interval, 0, len(requested))
	seen := make(map[bbgotypes.Interval]struct{}, len(requested))
	for _, interval := range requested {
		plannedInterval := planSyncInterval(symbol, interval, sessionScope)
		if _, ok := seen[plannedInterval]; ok {
			continue
		}
		seen[plannedInterval] = struct{}{}
		planned = append(planned, plannedInterval)
	}
	return planned
}

// planSyncInterval 根据标的和会话范围调整单个 K 线周期。
func planSyncInterval(symbol string, interval bbgotypes.Interval, sessionScope string) bbgotypes.Interval {
	if interval == bbgotypes.Interval("3d") || interval == bbgotypes.Interval("2w") {
		interval = bbgotypes.Interval1d
	}
	duration := interval.Duration()
	if duration > time.Hour && duration < 24*time.Hour {
		return bbgotypes.Interval1h
	}
	if sessionScope == "extended" &&
		strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") &&
		duration >= 24*time.Hour {
		return bbgotypes.Interval1h
	}
	return interval
}
