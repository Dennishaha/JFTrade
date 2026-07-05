package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/observability"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

// Start 启动异步回测。校验请求 → 查策略定义 → 编译脚本 → 创建运行记录 → 启动 goroutine。
// 返回初始状态（status="queued"）。回测完成后通过 RunStore.Update 写入结果。
func (s *Service) Start(ctx context.Context, req StartRequest) (*RunState, error) {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return nil, requestErrorf("definitionId is required")
	}
	if _, err := parseInstrument(req.Market, req.Symbol, req.Code); err != nil {
		return nil, requestErrorf("%v", err)
	}
	if s.strategies == nil {
		return nil, fmt.Errorf("strategy provider not configured")
	}
	def, ok, err := s.strategies.Definition(req.DefinitionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrStrategyDefinitionNotFound
	}
	return s.startResolvedBacktest(ctx, req, def)
}

// StartScript starts an asynchronous research backtest from an inline Pine
// script. The script is used only for this run and is not persisted into the
// strategy definition catalog.
func (s *Service) StartScript(ctx context.Context, req ScriptStartRequest) (*RunState, error) {
	script := strings.TrimSpace(req.Script)
	if script == "" {
		return nil, requestErrorf("script is required")
	}
	def := transientStrategyDefinition(script)
	return s.startResolvedBacktest(ctx, StartRequest{
		DefinitionID:      def.ID,
		DefinitionVersion: def.Version,
		Market:            req.Market,
		Code:              req.Code,
		Symbol:            req.Symbol,
		InstrumentType:    req.InstrumentType,
		Interval:          req.Interval,
		StartDate:         req.StartDate,
		EndDate:           req.EndDate,
		StartTime:         req.StartTime,
		EndTime:           req.EndTime,
		InitialBalance:    req.InitialBalance,
		RehabType:         req.RehabType,
		UseExtendedHours:  req.UseExtendedHours,
		TradingCosts:      req.TradingCosts,
		ExecutionModel:    req.ExecutionModel,
	}, def)
}

func transientStrategyDefinition(script string) StrategyDef {
	hash := sha256.Sum256([]byte(script))
	scriptHash := hex.EncodeToString(hash[:])
	return StrategyDef{
		ID: "adk-research-" + scriptHash[:12], Version: "script-" + scriptHash[:12],
		SourceFormat: strategydefinition.SourceFormatPineV6, Script: script,
	}
}

func (s *Service) startResolvedBacktest(ctx context.Context, req StartRequest, def StrategyDef) (*RunState, error) {
	prepared, err := prepareResolvedBacktest(req, def)
	if err != nil {
		return nil, err
	}
	run := newQueuedRun(prepared.request)
	if s.runs == nil {
		return nil, fmt.Errorf("run store not configured")
	}
	runCtx, cancel, err := s.beginBacktestTask(ctx, run.ID, run.Request.Symbol)
	if err != nil {
		return nil, err
	}
	if err := s.persistQueuedRun(run, cancel); err != nil {
		s.finishTask(cancel)
		return nil, err
	}
	go s.executeBacktest(runCtx, run.ID, prepared.request, def, prepared.startTime, prepared.endTime, cancel)
	return run, nil
}

func newQueuedRun(req StartRequest) *RunState {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &RunState{
		ID:        "bt-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Status:    "queued",
		Request:   req,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *Service) beginBacktestTask(ctx context.Context, runID string, symbol string) (context.Context, context.CancelFunc, error) {
	runCtx, cancel, err := s.beginTask(ctx)
	if err != nil {
		return nil, nil, err
	}
	runCtx = observability.WithFields(runCtx, observability.Fields{
		RunID:        runID,
		InstrumentID: symbol,
		Source:       "backtest",
	})
	return runCtx, cancel, nil
}

func (s *Service) persistQueuedRun(run *RunState, cancel context.CancelFunc) error {
	if err := s.runs.Add(run); err != nil {
		return fmt.Errorf("persist backtest run: %w", err)
	}
	s.runs.SetCancel(run.ID, cancel)
	return nil
}

func prepareResolvedBacktest(req StartRequest, def StrategyDef) (preparedBacktest, error) {
	instrument, err := parseInstrument(req.Market, req.Symbol, req.Code)
	if err != nil {
		return preparedBacktest{}, requestErrorf("%v", err)
	}
	req.Market = instrument.Market
	req.Code = instrument.Code
	req.Symbol = instrument.Symbol
	req.InstrumentType = normalizeBacktestInstrumentType(req.InstrumentType)
	executionModel, err := bt.NormalizeExecutionModelName(req.ExecutionModel)
	if err != nil {
		return preparedBacktest{}, requestErrorf("%v", err)
	}
	req.ExecutionModel = executionModel
	if strings.TrimSpace(req.Interval) == "" {
		req.Interval = "1m"
	}
	compilation, err := compileBacktestDefinition(def)
	if err != nil {
		return preparedBacktest{}, err
	}
	req.InitialBalance = normalizeInitialBalance(req.InitialBalance, compilation.Program.Metadata.InitialCapital)
	req.DefinitionVersion = def.Version
	startTime, endTime, startDate, endDate, timezone, err := resolveBacktestTimeRange(req.Symbol, req.StartDate, req.EndDate, req.StartTime, req.EndTime)
	if err != nil {
		return preparedBacktest{}, err
	}
	if !endTime.After(startTime) {
		return preparedBacktest{}, requestErrorf("endTime must be after startTime")
	}
	req.StartDate = startDate
	req.EndDate = endDate
	req.StartTime = startTime.UTC().Format(time.RFC3339Nano)
	req.EndTime = endTime.UTC().Format(time.RFC3339Nano)
	req.MarketTimezone = timezone
	queryStart, err := deriveWarmupQueryStart(compilation, req, startTime)
	if err != nil {
		return preparedBacktest{}, err
	}
	return preparedBacktest{request: req, definition: def, startTime: startTime, endTime: endTime, queryStart: queryStart}, nil
}

func compileBacktestDefinition(def StrategyDef) (strategypine.Compilation, error) {
	if strategydefinition.NormalizeSourceFormat(def.SourceFormat) != strategydefinition.SourceFormatPineV6 {
		return strategypine.Compilation{}, requestErrorf("unsupported strategy source format: %s", def.SourceFormat)
	}
	compilation, err := strategypine.Compile(def.Script)
	if err != nil {
		return strategypine.Compilation{}, requestErrorf("%v", err)
	}
	return compilation, nil
}

func normalizeInitialBalance(balance float64, compiled float64) float64 {
	if balance > 0 {
		return balance
	}
	if compiled > 0 {
		return compiled
	}
	return 100000
}

func deriveWarmupQueryStart(compilation strategypine.Compilation, req StartRequest, startTime time.Time) (time.Time, error) {
	interval := bbgotypes.Interval(req.Interval)
	warmup, err := indicatorruntime.WarmupBarsFromPlanForSymbolWithOptions(
		compilation.Requirements,
		interval,
		req.Symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: req.UseExtendedHours != nil && *req.UseExtendedHours},
	)
	if err != nil {
		return time.Time{}, requestErrorf("derive strategy warmup: %v", err)
	}
	queryStart := startTime
	if warmup > 0 {
		queryStart = startTime.Add(-interval.Duration() * time.Duration(warmup))
	}
	return queryStart, nil
}

// executeBacktest 在独立 goroutine 中运行回测并持久化结果。
func (s *Service) executeBacktest(
	ctx context.Context,
	runID string,
	req StartRequest,
	def StrategyDef,
	startTime, endTime time.Time,
	cancel context.CancelFunc,
) {
	defer s.finishTask(cancel)
	defer s.runs.SetCancel(runID, nil)
	defer s.recoverBacktestPanic(ctx, runID, req)

	s.markBacktestRunning(ctx, runID)
	result := s.runBacktest(ctx, bt.RunConfig{
		DBPath:           s.dbPath(),
		Market:           req.Market,
		Symbol:           req.Symbol,
		Interval:         req.Interval,
		SourceFormat:     def.SourceFormat,
		StartTime:        startTime,
		EndTime:          endTime,
		StrategyScript:   def.Script,
		InitialBalance:   req.InitialBalance,
		RehabType:        req.RehabType,
		UseExtendedHours: req.UseExtendedHours,
		InstrumentType:   req.InstrumentType,
		TradingCosts:     req.TradingCosts,
		ExecutionModel:   req.ExecutionModel,
	})
	result = ensureBacktestResult(req, result)
	status := backtestResultStatus(ctx, result)
	s.finishRun(runID, status, result)
	logBacktestCompletion(ctx, status, result)
}

func (s *Service) recoverBacktestPanic(ctx context.Context, runID string, req StartRequest) {
	if recovered := recover(); recovered != nil {
		panicErr := fmt.Errorf("backtest panic: %v", recovered)
		observability.ErrorWithImportance(ctx, observability.ImportanceCritical, "backtest run panicked", panicErr, "stack", string(debug.Stack()))
		s.finishRun(runID, "failed", failureResult(req, fmt.Sprintf("backtest panic: %v", recovered)))
	}
}

func (s *Service) markBacktestRunning(ctx context.Context, runID string) {
	if _, err := s.runs.Update(runID, func(run *RunState) {
		run.Status = "running"
		run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}); err != nil {
		observability.ErrorWithImportance(ctx, observability.ImportanceCritical, "backtest run store update failed", err, "status", "running")
	}
	observability.InfoWithImportance(ctx, observability.ImportanceNormal, "backtest run started", "status", "running")
}

func ensureBacktestResult(req StartRequest, result *bt.RunResult) *bt.RunResult {
	if result != nil {
		return result
	}
	return failureResult(req, "backtest returned no result")
}

func backtestResultStatus(ctx context.Context, result *bt.RunResult) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "cancelled"
	}
	if strings.TrimSpace(result.Error) != "" {
		return "failed"
	}
	return "completed"
}

func logBacktestCompletion(ctx context.Context, status string, result *bt.RunResult) {
	if status == "failed" {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "backtest run failed", errors.New(result.Error), "status", status)
		return
	}
	observability.InfoWithImportance(ctx, observability.ImportanceNormal, "backtest run finished", "status", status)
}

func (s *Service) runBacktest(ctx context.Context, config bt.RunConfig) *bt.RunResult {
	if s.runBacktestFn != nil {
		return s.runBacktestFn(ctx, config)
	}
	s.pineWorkerMu.RLock()
	runner := s.pineWorkerRunner
	s.pineWorkerMu.RUnlock()
	if runner == nil {
		return failureResult(StartRequest{Symbol: config.Symbol, Interval: config.Interval}, "pine worker runner is not configured")
	}
	return bt.RunWithPineWorker(ctx, config, runner)
}

// finishRun 将回测结果写入 RunStore。失败时回退到仅内存更新。
func (s *Service) finishRun(runID string, status string, result *bt.RunResult) {
	mutate := func(run *RunState) {
		run.Result = result
		run.Status = status
		run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if _, err := s.runs.Update(runID, mutate); err != nil {
		log.Printf("backtest run store update(%s %s) failed: %v", runID, status, err)
		_ = s.runs.UpdateMemoryOnly(runID, mutate)
	}
}

// failureResult 构造回测失败结果。
func failureResult(req StartRequest, message string) *bt.RunResult {
	return &bt.RunResult{
		Symbol:         req.Symbol,
		Interval:       req.Interval,
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		FinalBalance:   req.InitialBalance,
		TradingCosts:   req.TradingCosts,
		ExecutionModel: req.ExecutionModel,
		Error:          message,
	}
}
