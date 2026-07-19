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

// Sync 启动 K 线历史数据同步。打开 SQLite 存储 → 创建 Futu 连接 → 启动异步同步 goroutine。
func (s *Service) Sync(ctx context.Context, req SyncRequest) (*SyncStarted, error) {
	prepared, err := prepareSyncRequest(req)
	if err != nil {
		return nil, err
	}
	syncer, err := s.newSyncer()
	if err != nil {
		return nil, err
	}
	taskID, progress, syncCtx, syncCancel, err := s.startSyncTask(ctx, prepared.request.Symbol, len(prepared.intervals))
	if err != nil {
		besteffort.LogError(syncer.Close())
		return nil, err
	}
	go s.runSyncTask(syncCtx, syncer, taskID, progress, syncCancel, prepared)
	return buildSyncStarted(taskID, prepared), nil
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

	req.SessionScope = normalizeSessionScope(req.SessionScope)
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

func (s *Service) newSyncer() (KLineSyncer, error) {
	if s.newKLineSyncerFn == nil {
		return nil, fmt.Errorf("kline sync adapter not configured")
	}
	syncer, err := s.newKLineSyncerFn(s.dbPath())
	if err != nil {
		return nil, fmt.Errorf("open kline sync adapter: %w", err)
	}
	return syncer, nil
}

func (s *Service) startSyncTask(ctx context.Context, symbol string, intervalCount int) (string, *bt.SyncProgress, context.Context, context.CancelFunc, error) {
	taskID := fmt.Sprintf("sync-%s-%d", time.Now().UTC().Format("20060102T150405.000000000"), atomic.AddUint64(&s.syncTaskSeq, 1))
	progress := bt.NewSyncProgress(taskID, symbol, time.Now().UTC())
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
	prepared preparedSync,
) {
	defer s.finishTask(syncCancel)
	defer func() { besteffort.LogError(syncer.Close()) }()
	defer s.syncTasks.Finish(taskID)

	params := KLineSyncParams{
		Symbol:       prepared.request.Symbol,
		Intervals:    prepared.intervals,
		Since:        prepared.sinceTime,
		Until:        prepared.untilTime,
		RehabType:    prepared.rehabType,
		SessionScope: prepared.request.SessionScope,
	}
	syncErr := syncer.Sync(syncCtx, params, progress)
	finalizeSyncProgress(syncCtx, progress, syncErr, time.Now().UTC())
	logSyncCompletion(syncCtx, progress)
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

func buildSyncStarted(taskID string, prepared preparedSync) *SyncStarted {
	return &SyncStarted{
		TaskID:       taskID,
		Symbol:       prepared.request.Symbol,
		Intervals:    prepared.intervals,
		Since:        prepared.sinceTime.UTC().Format(time.RFC3339Nano),
		Until:        prepared.untilTime.UTC().Format(time.RFC3339Nano),
		SessionScope: prepared.request.SessionScope,
		Message:      "sync started",
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

// normalizeSessionScope 规范化会话范围。
func normalizeSessionScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "regular":
		return "regular"
	case "extended":
		return "extended"
	default:
		return "legacy"
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
	duration := interval.Duration()
	if interval == bbgotypes.Interval("3d") || interval == bbgotypes.Interval("2w") {
		return bbgotypes.Interval1d
	}
	if duration > time.Hour && duration < 24*time.Hour {
		return bbgotypes.Interval1h
	}
	if normalizeSessionScope(sessionScope) == "extended" &&
		strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") &&
		duration >= 24*time.Hour {
		return bbgotypes.Interval1h
	}
	return interval
}
