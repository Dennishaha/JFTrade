package servercore

import (
	"context"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

// ──────────────────────────────────────────────────────────────────────────────
// 类型转换辅助
// ──────────────────────────────────────────────────────────────────────────────
// backtestRunState（jftradeapi）与 btsrv.RunState（internal/backtest）结构字段
// 完全一致，仅 Request 内嵌类型不同（backtestStartRequest vs btsrv.StartRequest）。
// 此处通过字段逐项拷贝完成零依赖转换。

func toSrvRunState(r *backtestRunState) *btsrv.RunState {
	if r == nil {
		return nil
	}
	return &btsrv.RunState{
		ID:     r.ID,
		Status: r.Status,
		Request: btsrv.StartRequest{
			DefinitionID:      r.Request.DefinitionID,
			DefinitionVersion: r.Request.DefinitionVersion,
			Market:            r.Request.Market,
			Code:              r.Request.Code,
			Symbol:            r.Request.Symbol,
			Interval:          r.Request.Interval,
			StartTime:         r.Request.StartTime,
			EndTime:           r.Request.EndTime,
			InitialBalance:    r.Request.InitialBalance,
			RehabType:         r.Request.RehabType,
			UseExtendedHours:  r.Request.UseExtendedHours,
		},
		Result:    r.Result, // 相同类型 *bt.RunResult
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func toBacktestRunState(r *btsrv.RunState) *backtestRunState {
	if r == nil {
		return nil
	}
	return &backtestRunState{
		ID:     r.ID,
		Status: r.Status,
		Request: backtestStartRequest{
			DefinitionID:      r.Request.DefinitionID,
			DefinitionVersion: r.Request.DefinitionVersion,
			Market:            r.Request.Market,
			Code:              r.Request.Code,
			Symbol:            r.Request.Symbol,
			Interval:          r.Request.Interval,
			StartTime:         r.Request.StartTime,
			EndTime:           r.Request.EndTime,
			InitialBalance:    r.Request.InitialBalance,
			RehabType:         r.Request.RehabType,
			UseExtendedHours:  r.Request.UseExtendedHours,
		},
		Result:    r.Result, // 相同类型 *bt.RunResult
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func toSrvStrategyDef(d strategyDesignDefinition) btsrv.StrategyDef {
	return btsrv.StrategyDef{
		ID:           d.ID,
		Version:      d.Version,
		SourceFormat: d.SourceFormat,
		Script:       d.Script,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// RunStore 适配器：backtestRunStore → btsrv.RunStore
// ──────────────────────────────────────────────────────────────────────────────

type backtestRunStoreAdapter struct {
	store *backtestRunStore
}

func (a *backtestRunStoreAdapter) Add(run *btsrv.RunState) error {
	return a.store.add(toBacktestRunState(run))
}

func (a *backtestRunStoreAdapter) Get(runID string) (*btsrv.RunState, bool) {
	run, ok := a.store.get(runID)
	if !ok {
		return nil, false
	}
	return toSrvRunState(run), true
}

func (a *backtestRunStoreAdapter) GetFull(runID string) (*btsrv.RunState, bool, error) {
	run, ok, err := a.store.getFull(runID)
	if err != nil || !ok {
		return nil, ok, err
	}
	return toSrvRunState(run), true, nil
}

func (a *backtestRunStoreAdapter) List() []*btsrv.RunState {
	runs := a.store.list()
	out := make([]*btsrv.RunState, 0, len(runs))
	for _, r := range runs {
		out = append(out, toSrvRunState(r))
	}
	return out
}

func (a *backtestRunStoreAdapter) ListLightweight() []*btsrv.RunState {
	runs := a.store.listLightweight()
	out := make([]*btsrv.RunState, 0, len(runs))
	for _, r := range runs {
		out = append(out, toSrvRunState(r))
	}
	return out
}

func (a *backtestRunStoreAdapter) Update(runID string, mutate func(*btsrv.RunState)) (bool, error) {
	return a.store.update(runID, func(rs *backtestRunState) {
		run := toSrvRunState(rs)
		mutate(run)
		updated := toBacktestRunState(run)
		*rs = *updated
	})
}

func (a *backtestRunStoreAdapter) UpdateMemoryOnly(runID string, mutate func(*btsrv.RunState)) bool {
	return a.store.updateMemoryOnly(runID, func(rs *backtestRunState) {
		run := toSrvRunState(rs)
		mutate(run)
		updated := toBacktestRunState(run)
		*rs = *updated
	})
}

func (a *backtestRunStoreAdapter) Delete(runID string) (*btsrv.RunState, bool, error) {
	run, ok, err := a.store.delete(runID)
	if err != nil || !ok {
		return nil, ok, err
	}
	return toSrvRunState(run), true, nil
}

func (a *backtestRunStoreAdapter) SetCancel(runID string, cancel context.CancelFunc) {
	a.store.setCancel(runID, cancel)
}

func (a *backtestRunStoreAdapter) Cancel(runID string) bool {
	return a.store.cancel(runID)
}

func (a *backtestRunStoreAdapter) Close() error {
	return a.store.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// SyncTaskStore 适配器：backtestSyncTaskStore → btsrv.SyncTaskStore
// ──────────────────────────────────────────────────────────────────────────────
// 双方均使用 *bt.SyncProgress，仅方法签名包装。

type backtestSyncTaskStoreAdapter struct {
	store *backtestSyncTaskStore
}

func (a *backtestSyncTaskStoreAdapter) Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc) {
	a.store.add(taskID, progress, cancel)
}

func (a *backtestSyncTaskStoreAdapter) Get(taskID string) (*bt.SyncProgress, bool) {
	return a.store.get(taskID)
}

func (a *backtestSyncTaskStoreAdapter) Finish(taskID string) {
	a.store.finish(taskID)
}

func (a *backtestSyncTaskStoreAdapter) Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool) {
	return a.store.cancel(taskID, cancelledAt)
}

// ──────────────────────────────────────────────────────────────────────────────
// StrategyProvider 适配器：strategyDesignStore → btsrv.StrategyProvider
// ──────────────────────────────────────────────────────────────────────────────

type strategyProviderAdapter struct {
	store *strategyDesignStore
}

func (a *strategyProviderAdapter) Definition(id string) (btsrv.StrategyDef, bool, error) {
	d, ok, err := a.store.definition(id)
	if err != nil || !ok {
		return btsrv.StrategyDef{}, ok, err
	}
	return toSrvStrategyDef(d), true, nil
}
