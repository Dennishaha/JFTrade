package assembly

import (
	"context"
	"fmt"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/chart"
)

func (a *ApplicationAdapter) ensureBacktestData(
	definitionIDs []string,
	input BacktestStartInput,
) (BacktestDataReadiness, error) {
	service, err := a.requireBacktest()
	if err != nil {
		return BacktestDataReadiness{}, err
	}
	readiness, err := service.EnsureDefinitionsData(
		context.Background(),
		backtestStartRequest(input),
		definitionIDs,
	)
	return backtestDataReadinessFromService(readiness), err
}

func (a *ApplicationAdapter) ensureResearchBacktestData(
	input ResearchBacktestInput,
) (BacktestDataReadiness, error) {
	service, err := a.requireBacktest()
	if err != nil {
		return BacktestDataReadiness{}, err
	}
	readiness, err := service.EnsureScriptData(context.Background(), researchBacktestRequest(input))
	return backtestDataReadinessFromService(readiness), err
}

func (a *ApplicationAdapter) enqueueBacktest(input BacktestStartInput) (BacktestRunRef, error) {
	service, err := a.requireBacktest()
	if err != nil {
		return BacktestRunRef{}, err
	}
	run, err := service.Start(context.Background(), backtestStartRequest(input))
	if err != nil {
		return BacktestRunRef{}, err
	}
	return BacktestRunRef{ID: run.ID, Status: run.Status}, nil
}

func (a *ApplicationAdapter) startResearchBacktest(
	input ResearchBacktestInput,
) (BacktestRunSummary, error) {
	service, err := a.requireBacktest()
	if err != nil {
		return BacktestRunSummary{}, err
	}
	run, err := service.StartScript(context.Background(), researchBacktestRequest(input))
	if err != nil {
		return BacktestRunSummary{}, err
	}
	return backtestRunSummaryFromService(run), nil
}

func (a *ApplicationAdapter) backtestResultView(input BacktestResultViewInput) (any, error) {
	service, err := a.requireBacktest()
	if err != nil {
		return nil, err
	}
	return service.ResultView(btsrv.ResultViewRequest{
		RunID: input.RunID, View: input.View, Resolution: input.Resolution,
		StartTime: input.StartTime, EndTime: input.EndTime,
		Include: append([]string(nil), input.Include...),
		Limit:   input.Limit, Cursor: input.Cursor,
	})
}

func (a *ApplicationAdapter) backtestRunSummaries() []BacktestRunSummary {
	service := a.backtest()
	if service == nil {
		return nil
	}
	runs := service.ListFull()
	out := make([]BacktestRunSummary, 0, len(runs))
	for _, run := range runs {
		if run != nil {
			out = append(out, backtestRunSummaryFromService(run))
		}
	}
	return out
}

func (a *ApplicationAdapter) backtestKLineSyncProgress(taskID string) (*bt.SyncProgress, bool) {
	service := a.backtest()
	if service == nil {
		return nil, false
	}
	return service.GetSyncProgress(taskID)
}

func (a *ApplicationAdapter) cancelBacktest(runID string) {
	if service := a.backtest(); service != nil {
		service.Cancel(runID)
	}
}

func (a *ApplicationAdapter) cancelBacktestResult(runID string) bool {
	if service := a.backtest(); service != nil {
		return service.Cancel(runID)
	}
	return false
}

func (a *ApplicationAdapter) optimizationRuns() assistant.OptimizationRuns {
	return applicationOptimizationRuns{adapter: a}
}

type applicationOptimizationRuns struct {
	adapter *ApplicationAdapter
}

func (r applicationOptimizationRuns) Get(runID string) (assistant.OptimizationRun, bool) {
	if r.adapter == nil {
		return assistant.OptimizationRun{}, false
	}
	service := r.adapter.backtest()
	if service == nil {
		return assistant.OptimizationRun{}, false
	}
	run, ok, err := service.GetResult(runID)
	if err != nil || !ok {
		return assistant.OptimizationRun{}, false
	}
	return assistant.OptimizationRun{Status: run.Status, Result: run.Result}, true
}

func (r applicationOptimizationRuns) Cancel(runID string) {
	if r.adapter != nil {
		r.adapter.cancelBacktest(runID)
	}
}

func (a *ApplicationAdapter) requireBacktest() (*btsrv.Service, error) {
	service := a.backtest()
	if service == nil {
		return nil, fmt.Errorf("backtest service is unavailable")
	}
	return service, nil
}

func backtestStartRequest(input BacktestStartInput) btsrv.StartRequest {
	return btsrv.StartRequest{
		DefinitionID: input.DefinitionID, Market: input.Market, Symbol: input.Symbol,
		Code: input.Code, Interval: input.Interval, StartDate: input.StartDate, EndDate: input.EndDate,
		StartTime: input.StartTime, EndTime: input.EndTime, InitialBalance: input.InitialBalance,
		RehabType: input.RehabType, ChartType: chart.ChartType(input.ChartType),
		InstrumentType: input.InstrumentType, UseExtendedHours: input.UseExtendedHours,
		TradingCosts: input.TradingCosts, ExecutionModel: input.ExecutionModel,
		MarketDataProviderOverride: input.MarketDataProvider,
	}
}

func researchBacktestRequest(input ResearchBacktestInput) btsrv.ScriptStartRequest {
	return btsrv.ScriptStartRequest{
		Script: input.Script, Market: input.Market, Symbol: input.Symbol, Code: input.Code,
		Interval: input.Interval, StartDate: input.StartDate, EndDate: input.EndDate,
		StartTime: input.StartTime, EndTime: input.EndTime, InitialBalance: input.InitialBalance,
		RehabType: input.RehabType, UseExtendedHours: input.UseExtendedHours,
		ChartType: chart.ChartType(input.ChartType), InstrumentType: input.InstrumentType,
		TradingCosts: input.TradingCosts, ExecutionModel: input.ExecutionModel,
		MarketDataProviderOverride: input.MarketDataProvider,
	}
}

func backtestDataReadinessFromService(readiness *btsrv.DataReadiness) BacktestDataReadiness {
	if readiness == nil {
		return BacktestDataReadiness{}
	}
	result := BacktestDataReadiness{
		Status:             readiness.Status,
		Ready:              readiness.Ready,
		MarketDataProvider: readiness.MarketDataProvider,
		Progress:           readiness.Progress,
		Error:              readiness.Error,
	}
	if readiness.Sync == nil {
		return result
	}
	intervals := make([]string, 0, len(readiness.Sync.Intervals))
	for _, interval := range readiness.Sync.Intervals {
		intervals = append(intervals, string(interval))
	}
	status := "queued"
	if readiness.Progress != nil && readiness.Progress.Status != "" {
		status = readiness.Progress.Status
	}
	result.DataSync = &BacktestDataSync{
		TaskID: readiness.Sync.TaskID, Symbol: readiness.Sync.Symbol, Intervals: intervals,
		Since: readiness.Sync.Since, Until: readiness.Sync.Until,
		SessionScope: readiness.Sync.SessionScope, Status: status,
		MarketDataProvider: readiness.Sync.MarketDataProvider,
	}
	if result.MarketDataProvider == "" {
		result.MarketDataProvider = result.DataSync.MarketDataProvider
	}
	return result
}

func backtestRunSummaryFromService(run *btsrv.RunState) BacktestRunSummary {
	if run == nil {
		return BacktestRunSummary{}
	}
	provider := run.MarketDataProvider
	if provider == "" {
		provider = run.Request.MarketDataProviderOverride
	}
	if provider == "" {
		provider = "futu"
	}
	return BacktestRunSummary{
		ID: run.ID, Status: run.Status, DefinitionID: run.Request.DefinitionID,
		DefinitionVersion: run.Request.DefinitionVersion, Market: run.Request.Market,
		Code: run.Request.Code, Symbol: run.Request.Symbol, Interval: run.Request.Interval,
		StartDate: run.Request.StartDate, EndDate: run.Request.EndDate,
		StartTime: run.Request.StartTime, EndTime: run.Request.EndTime,
		MarketTimezone: run.Request.MarketTimezone, InitialBalance: run.Request.InitialBalance,
		RehabType: run.Request.RehabType, ChartType: string(run.Request.ChartType),
		InstrumentType: run.Request.InstrumentType, MarketDataProvider: provider,
		ExecutionModel: run.Request.ExecutionModel, TradingCosts: run.Request.TradingCosts,
		UseExtendedHours: run.Request.UseExtendedHours, Result: run.Result,
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
}
