// Package backtest 提供回测业务编排层。回测启动、状态查询和 K 线同步由
// 独立 Service 负责，Handler 层仅处理参数绑定与响应写入。
//
// RunStore、SyncTaskStore 和外部行情同步能力均通过接口注入，业务层不依赖
// HTTP transport 或具体券商协议。
package backtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/market"
	"github.com/jftrade/jftrade-main/pkg/observability"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

// ──────────────────────────────────────────────────────────────────────────────
// 请求 / 响应类型
// ──────────────────────────────────────────────────────────────────────────────

// StartRequest 回测启动参数（从 HTTP JSON body 反序列化）。
type StartRequest struct {
	DefinitionID      string          `json:"definitionId"`
	DefinitionVersion string          `json:"definitionVersion,omitempty"`
	Market            string          `json:"market"`
	Code              string          `json:"code"`
	Symbol            string          `json:"symbol"`
	InstrumentType    string          `json:"instrumentType,omitempty"`
	Interval          string          `json:"interval"`
	StartDate         string          `json:"startDate,omitempty"`
	EndDate           string          `json:"endDate,omitempty"`
	StartTime         string          `json:"startTime,omitempty"`
	EndTime           string          `json:"endTime,omitempty"`
	MarketTimezone    string          `json:"marketTimezone,omitempty"`
	InitialBalance    float64         `json:"initialBalance"`
	RehabType         string          `json:"rehabType"`
	UseExtendedHours  *bool           `json:"useExtendedHours,omitempty"`
	TradingCosts      bt.TradingCosts `json:"tradingCosts"`
}

// ScriptStartRequest starts a transient research backtest from an inline Pine
// script without requiring or creating a saved strategy definition.
type ScriptStartRequest struct {
	Script           string          `json:"script"`
	Market           string          `json:"market"`
	Code             string          `json:"code"`
	Symbol           string          `json:"symbol"`
	InstrumentType   string          `json:"instrumentType,omitempty"`
	Interval         string          `json:"interval"`
	StartDate        string          `json:"startDate,omitempty"`
	EndDate          string          `json:"endDate,omitempty"`
	StartTime        string          `json:"startTime,omitempty"`
	EndTime          string          `json:"endTime,omitempty"`
	InitialBalance   float64         `json:"initialBalance"`
	RehabType        string          `json:"rehabType"`
	UseExtendedHours *bool           `json:"useExtendedHours,omitempty"`
	TradingCosts     bt.TradingCosts `json:"tradingCosts"`
}

// RunState 是回测运行状态的纯数据结构。
type RunState struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"` // "queued" | "running" | "completed" | "failed" | "cancelled"
	Request   StartRequest  `json:"request"`
	Result    *bt.RunResult `json:"result,omitempty"`
	CreatedAt string        `json:"createdAt"`
	UpdatedAt string        `json:"updatedAt"`
}

// ResultViewRequest describes a bounded result slice suitable for agent tools.
type ResultViewRequest struct {
	RunID      string   `json:"runId"`
	View       string   `json:"view"`
	Resolution string   `json:"resolution"`
	StartTime  string   `json:"startTime"`
	EndTime    string   `json:"endTime"`
	Include    []string `json:"include"`
	Limit      int      `json:"limit"`
	Cursor     string   `json:"cursor"`
}

// SyncRequest K线同步请求参数。
type SyncRequest struct {
	Market       string   `json:"market"`
	Code         string   `json:"code"`
	Symbol       string   `json:"symbol"`
	Intervals    []string `json:"intervals"`
	StartDate    string   `json:"startDate,omitempty"`
	EndDate      string   `json:"endDate,omitempty"`
	Since        string   `json:"since,omitempty"`
	Until        string   `json:"until,omitempty"`
	RehabType    string   `json:"rehabType"`
	SessionScope string   `json:"sessionScope,omitempty"`
}

// SyncStarted 同步启动响应。
type SyncStarted struct {
	TaskID       string               `json:"taskId"`
	Symbol       string               `json:"symbol"`
	Intervals    []bbgotypes.Interval `json:"intervals"`
	Since        string               `json:"since"`
	Until        string               `json:"until"`
	SessionScope string               `json:"sessionScope"`
	Message      string               `json:"message"`
}

const (
	DataStatusReady                 = "ready"
	DataStatusSyncing               = "syncing_data"
	DataStatusSyncFailed            = "sync_failed"
	DataStatusSyncCancelled         = "sync_cancelled"
	DataStatusInsufficientAfterSync = "insufficient_after_sync"
)

// DataReadiness describes whether a backtest can start or must wait for K-line sync.
type DataReadiness struct {
	Status   string           `json:"status"`
	Ready    bool             `json:"ready"`
	Sync     *SyncStarted     `json:"dataSync,omitempty"`
	Progress *bt.SyncProgress `json:"progress,omitempty"`
	Error    string           `json:"error,omitempty"`
}

type preparedBacktest struct {
	request    StartRequest
	definition StrategyDef
	startTime  time.Time
	endTime    time.Time
	queryStart time.Time
}

// RehabType is the broker-independent price adjustment mode used for K-lines.
type RehabType string

const (
	RehabTypeForward  RehabType = "forward"
	RehabTypeBackward RehabType = "backward"
	RehabTypeNone     RehabType = "none"
)

// ErrStrategyDefinitionNotFound identifies a missing requested strategy.
var ErrStrategyDefinitionNotFound = errors.New("strategy definition not found")

// ErrServiceClosed identifies attempts to start work after service shutdown.
var ErrServiceClosed = errors.New("backtest service closed")

// KLineSyncParams contains the stable business parameters passed to a K-line
// synchronization adapter.
type KLineSyncParams struct {
	Symbol       string
	Intervals    []bbgotypes.Interval
	Since        time.Time
	Until        time.Time
	RehabType    RehabType
	SessionScope string
}

// KLineSyncer hides broker clients, protobuf enums, and concrete storage from
// the backtest service.
type KLineSyncer interface {
	Sync(ctx context.Context, params KLineSyncParams, progress *bt.SyncProgress) error
	Close() error
}

// RequestError identifies invalid user input that API transports should expose
// as a client error.
type RequestError struct {
	err error
}

func (e *RequestError) Error() string { return e.err.Error() }
func (e *RequestError) Unwrap() error { return e.err }

// IsRequestError reports whether err was caused by invalid request input.
func IsRequestError(err error) bool {
	var target *RequestError
	return errors.As(err, &target)
}

func requestErrorf(format string, args ...any) error {
	return &RequestError{err: fmt.Errorf(format, args...)}
}

// ──────────────────────────────────────────────────────────────────────────────
// 依赖接口
// ──────────────────────────────────────────────────────────────────────────────

// RunStore 是回测运行记录持久化接口。
type RunStore interface {
	Add(run *RunState) error
	Get(runID string) (*RunState, bool)
	GetFull(runID string) (*RunState, bool, error)
	List() []*RunState
	ListLightweight() []*RunState
	Update(runID string, mutate func(*RunState)) (bool, error)
	UpdateMemoryOnly(runID string, mutate func(*RunState)) bool
	Delete(runID string) (*RunState, bool, error)
	SetCancel(runID string, cancel context.CancelFunc)
	Cancel(runID string) bool
	Close() error
}

// SyncTaskStore 是同步任务管理接口。
type SyncTaskStore interface {
	Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc)
	Get(taskID string) (*bt.SyncProgress, bool)
	Finish(taskID string)
	Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool)
}

// StrategyProvider 策略定义查询接口。
// 由应用装配层的策略定义存储适配器实现。
type StrategyProvider interface {
	// Definition 按 ID 查询策略定义。返回 (定义, 是否存在, 错误)。
	Definition(id string) (StrategyDef, bool, error)
}

// StrategyDef 策略定义（回测编排所需的最小字段集）。
type StrategyDef struct {
	ID           string
	Version      string
	SourceFormat string
	Script       string
}

// ──────────────────────────────────────────────────────────────────────────────
// Service 回测业务编排
// ──────────────────────────────────────────────────────────────────────────────

// Service 提供回测业务的统一入口：启动回测、查询状态/结果、删除记录、K线同步管理。
// 所有外部副作用（编译策略脚本、创建 Futu 连接、读取数据库路径）通过闭包注入，
// 与 HTTP Server 解耦，遵循依赖注入模式。
type Service struct {
	runs       RunStore
	syncTasks  SyncTaskStore
	strategies StrategyProvider

	lifecycleMu     sync.Mutex
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	lifecycleTasks  sync.WaitGroup
	closed          bool
	syncTaskSeq     uint64
	dataSyncMu      sync.Mutex
	dataSyncTasks   map[string]*SyncStarted

	// 回测数据库文件路径提供者
	dbPathFn func() string

	// 回测执行器（通常为 backtest.Run）
	runBacktestFn func(ctx context.Context, config bt.RunConfig) *bt.RunResult

	// Pine worker runner used by the default hard-cut PineTS backtest path.
	pineWorkerMu     sync.RWMutex
	pineWorkerRunner bt.PineWorkerRunner

	// 创建 broker-specific K 线同步适配器。
	newKLineSyncerFn func(dbPath string) (KLineSyncer, error)

	// 检查本地 K 线覆盖；测试可注入稳定结果。
	checkKLineCoverageFn func(dbPath, symbol, interval string, since, until time.Time, rehabType, sessionScope string) error
}

// NewService 创建回测服务。所有依赖通过 Option 注入。
func NewService(opts ...Option) *Service {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	s := &Service{
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		dataSyncTasks:   make(map[string]*SyncStarted),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option 函数式选项。
type Option func(*Service)

// WithRunStore 设置回测运行记录持久化存储。
func WithRunStore(store RunStore) Option {
	return func(s *Service) { s.runs = store }
}

// WithSyncTaskStore 设置同步任务管理存储。
func WithSyncTaskStore(store SyncTaskStore) Option {
	return func(s *Service) { s.syncTasks = store }
}

// WithStrategyProvider 设置策略定义提供者。
func WithStrategyProvider(p StrategyProvider) Option {
	return func(s *Service) { s.strategies = p }
}

// WithDBPathFn 设置回测数据库路径提供者。
func WithDBPathFn(fn func() string) Option {
	return func(s *Service) { s.dbPathFn = fn }
}

// WithRunBacktestFn 设置回测执行器（测试时可替换为 mock）。
func WithRunBacktestFn(fn func(ctx context.Context, config bt.RunConfig) *bt.RunResult) Option {
	return func(s *Service) { s.runBacktestFn = fn }
}

// WithPineWorkerRunner sets the PineTS worker runner used by default backtests.
func WithPineWorkerRunner(runner bt.PineWorkerRunner) Option {
	return func(s *Service) { s.SetPineWorkerRunner(runner) }
}

// SetPineWorkerRunner updates the PineTS worker runner used by default backtests.
func (s *Service) SetPineWorkerRunner(runner bt.PineWorkerRunner) {
	if s == nil {
		return
	}
	s.pineWorkerMu.Lock()
	s.pineWorkerRunner = runner
	s.pineWorkerMu.Unlock()
}

// WithNewKLineSyncerFn sets the broker integration adapter factory.
func WithNewKLineSyncerFn(fn func(dbPath string) (KLineSyncer, error)) Option {
	return func(s *Service) { s.newKLineSyncerFn = fn }
}

// WithKLineCoverageCheckFn overrides local K-line coverage checks.
func WithKLineCoverageCheckFn(fn func(dbPath, symbol, interval string, since, until time.Time, rehabType, sessionScope string) error) Option {
	return func(s *Service) { s.checkKLineCoverageFn = fn }
}

// ──────────────────────────────────────────────────────────────────────────────
// 回测生命周期方法
// ──────────────────────────────────────────────────────────────────────────────

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
	}, def)
}

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
		InstrumentType: req.InstrumentType, TradingCosts: req.TradingCosts,
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

func transientStrategyDefinition(script string) StrategyDef {
	hash := sha256.Sum256([]byte(script))
	scriptHash := hex.EncodeToString(hash[:])
	return StrategyDef{
		ID: "adk-research-" + scriptHash[:12], Version: "script-" + scriptHash[:12],
		SourceFormat: strategydefinition.SourceFormatPineV6, Script: script,
	}
}

func (s *Service) ensurePreparedData(ctx context.Context, prepared []preparedBacktest) (*DataReadiness, error) {
	base := prepared[0]
	queryStart := base.queryStart
	endTime := base.endTime
	for _, candidate := range prepared[1:] {
		if candidate.request.Symbol != base.request.Symbol || candidate.request.Interval != base.request.Interval {
			return nil, requestErrorf("optimization candidates must use the same symbol and interval")
		}
		if candidate.queryStart.Before(queryStart) {
			queryStart = candidate.queryStart
		}
		if candidate.endTime.After(endTime) {
			endTime = candidate.endTime
		}
	}
	rehabType := normalizeRehabTypeName(base.request.RehabType)
	readSessionScope := backtestReadSessionScope(base.request.UseExtendedHours)
	syncSessionScope := backtestSyncSessionScope(base.request.UseExtendedHours)
	covered, coverageErr := s.hasKLineCoverage(base.request.Symbol, base.request.Interval, queryStart, endTime, rehabType, readSessionScope)
	if coverageErr != nil && !isMissingKLineCoverageError(coverageErr) {
		return nil, coverageErr
	}
	if covered {
		return &DataReadiness{Status: DataStatusReady, Ready: true}, nil
	}

	key := dataSyncKey(base.request.Symbol, base.request.Interval, queryStart, endTime, rehabType, syncSessionScope)
	s.dataSyncMu.Lock()
	defer s.dataSyncMu.Unlock()
	if existing := s.dataSyncTasks[key]; existing != nil {
		if progress, ok := s.GetSyncProgress(existing.TaskID); ok {
			switch progress.Status {
			case "queued", "running":
				return readinessForSyncProgress(progress, existing), nil
			case "failed":
				return &DataReadiness{Status: DataStatusSyncFailed, Sync: existing, Progress: progress, Error: progress.Error}, nil
			case "cancelled":
				return &DataReadiness{Status: DataStatusSyncCancelled, Sync: existing, Progress: progress, Error: progress.Error}, nil
			case "completed":
				return &DataReadiness{Status: DataStatusInsufficientAfterSync, Sync: existing, Progress: progress, Error: coverageErr.Error()}, nil
			}
		}
		delete(s.dataSyncTasks, key)
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

func (s *Service) startResolvedBacktest(ctx context.Context, req StartRequest, def StrategyDef) (*RunState, error) {
	prepared, err := prepareResolvedBacktest(req, def)
	if err != nil {
		return nil, err
	}
	req = prepared.request
	startTime := prepared.startTime
	endTime := prepared.endTime

	// 创建运行记录
	runID := "bt-" + time.Now().UTC().Format("20060102T150405.000000000")
	run := &RunState{
		ID:        runID,
		Status:    "queued",
		Request:   req,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	if s.runs == nil {
		return nil, fmt.Errorf("run store not configured")
	}
	runCtx, cancel, err := s.beginTask(ctx)
	if err != nil {
		return nil, err
	}
	runCtx = observability.WithFields(runCtx, observability.Fields{
		RunID:        runID,
		InstrumentID: req.Symbol,
		Source:       "backtest",
	})
	if err := s.runs.Add(run); err != nil {
		s.finishTask(cancel)
		return nil, fmt.Errorf("persist backtest run: %w", err)
	}

	s.runs.SetCancel(runID, cancel)
	observability.InfoWithImportance(runCtx, observability.ImportanceNormal, "backtest run queued", "status", run.Status)

	// 异步执行回测
	go s.executeBacktest(runCtx, runID, req, def, startTime, endTime, cancel)

	return run, nil
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
	if strings.TrimSpace(req.Interval) == "" {
		req.Interval = "1m"
	}
	if strategydefinition.NormalizeSourceFormat(def.SourceFormat) != strategydefinition.SourceFormatPineV6 {
		return preparedBacktest{}, requestErrorf("unsupported strategy source format: %s", def.SourceFormat)
	}
	compilation, err := strategypine.Compile(def.Script)
	if err != nil {
		return preparedBacktest{}, requestErrorf("%v", err)
	}
	if req.InitialBalance <= 0 {
		req.InitialBalance = compilation.Program.Metadata.InitialCapital
	}
	if req.InitialBalance <= 0 {
		req.InitialBalance = 100000
	}
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
	interval := bbgotypes.Interval(req.Interval)
	warmup, err := indicatorruntime.WarmupBarsFromPlanForSymbolWithOptions(
		compilation.Requirements,
		interval,
		req.Symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: req.UseExtendedHours != nil && *req.UseExtendedHours},
	)
	if err != nil {
		return preparedBacktest{}, requestErrorf("derive strategy warmup: %v", err)
	}
	queryStart := startTime
	if warmup > 0 {
		queryStart = startTime.Add(-interval.Duration() * time.Duration(warmup))
	}
	return preparedBacktest{request: req, definition: def, startTime: startTime, endTime: endTime, queryStart: queryStart}, nil
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
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("backtest panic: %v", recovered)
			observability.ErrorWithImportance(ctx, observability.ImportanceCritical, "backtest run panicked", panicErr, "stack", string(debug.Stack()))
			s.finishRun(runID, "failed", failureResult(req, fmt.Sprintf("backtest panic: %v", recovered)))
		}
	}()

	if _, err := s.runs.Update(runID, func(run *RunState) {
		run.Status = "running"
		run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}); err != nil {
		observability.ErrorWithImportance(ctx, observability.ImportanceCritical, "backtest run store update failed", err, "status", "running")
	}
	observability.InfoWithImportance(ctx, observability.ImportanceNormal, "backtest run started", "status", "running")

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
	})

	if result == nil {
		result = failureResult(req, "backtest returned no result")
	}

	status := "completed"
	if errors.Is(ctx.Err(), context.Canceled) {
		status = "cancelled"
	} else if strings.TrimSpace(result.Error) != "" {
		status = "failed"
	}
	s.finishRun(runID, status, result)
	if status == "failed" {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "backtest run failed", errors.New(result.Error), "status", status)
	} else {
		observability.InfoWithImportance(ctx, observability.ImportanceNormal, "backtest run finished", "status", status)
	}
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
		// 尽力更新内存，保证查询视图一致
		_ = s.runs.UpdateMemoryOnly(runID, mutate)
	}
}

// List 列出所有回测运行记录（不含结果详情，仅元数据）。
func (s *Service) List() []*RunState {
	if s.runs == nil {
		return nil
	}
	return s.runs.ListLightweight()
}

// ListFull lists all backtest runs including persisted result details.
func (s *Service) ListFull() []*RunState {
	if s.runs == nil {
		return nil
	}
	return s.runs.List()
}

// GetStatus 查询回测运行状态（不含结果）。
func (s *Service) GetStatus(runID string) (*RunState, bool) {
	if s.runs == nil {
		return nil, false
	}
	return s.runs.Get(runID)
}

// GetResult 查询完整回测结果（含 PnL 曲线、交易记录等）。
func (s *Service) GetResult(runID string) (*RunState, bool, error) {
	if s.runs == nil {
		return nil, false, fmt.Errorf("run store not configured")
	}
	return s.runs.GetFull(runID)
}

// Delete 删除已完成/失败/取消的回测记录。
func (s *Service) Delete(runID string) (*RunState, bool, error) {
	if s.runs == nil {
		return nil, false, fmt.Errorf("run store not configured")
	}
	return s.runs.Delete(runID)
}

// Cancel 取消正在运行的回测。
func (s *Service) Cancel(runID string) bool {
	if s.runs == nil {
		return false
	}
	return s.runs.Cancel(runID)
}

// ──────────────────────────────────────────────────────────────────────────────
// K线同步方法
// ──────────────────────────────────────────────────────────────────────────────

// Sync 启动 K 线历史数据同步。打开 SQLite 存储 → 创建 Futu 连接 → 启动异步同步 goroutine。
func (s *Service) Sync(ctx context.Context, req SyncRequest) (*SyncStarted, error) {
	// 填充默认标的
	if strings.TrimSpace(req.Symbol) == "" && strings.TrimSpace(req.Code) == "" {
		req.Market = "HK"
		req.Code = "00700"
	}

	instrument, err := parseInstrument(req.Market, req.Symbol, req.Code)
	if err != nil {
		return nil, requestErrorf("%v", err)
	}
	req.Market = instrument.Market
	req.Code = instrument.Code
	req.Symbol = instrument.Symbol

	if len(req.Intervals) == 0 {
		req.Intervals = []string{"1m", "5m", "15m", "30m", "1h", "1d", "1w"}
	}

	sinceTime, untilTime, _, _, _, err := resolveSyncTimeRange(req.Symbol, req.StartDate, req.EndDate, req.Since, req.Until)
	if err != nil {
		return nil, err
	}
	if !untilTime.After(sinceTime) {
		return nil, requestErrorf("until must be after since")
	}

	// 解析 intervals
	var intervals []bbgotypes.Interval
	for _, iv := range req.Intervals {
		iv = strings.TrimSpace(iv)
		if iv != "" {
			intervals = append(intervals, bbgotypes.Interval(iv))
		}
	}
	if len(intervals) == 0 {
		intervals = []bbgotypes.Interval{"1m", "5m", "1h", "1d"}
	}

	req.SessionScope = normalizeSessionScope(req.SessionScope)
	intervals = planSyncIntervals(req.Symbol, intervals, req.SessionScope)

	// 解析复权类型，默认为前复权。
	rehabType := RehabTypeForward
	switch strings.ToLower(strings.TrimSpace(req.RehabType)) {
	case "none":
		rehabType = RehabTypeNone
	case "backward":
		rehabType = RehabTypeBackward
	case "forward", "":
		rehabType = RehabTypeForward
	default:
		rehabType = RehabTypeForward
	}

	dbPath := s.dbPath()
	if s.newKLineSyncerFn == nil {
		return nil, fmt.Errorf("kline sync adapter not configured")
	}
	syncer, err := s.newKLineSyncerFn(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open kline sync adapter: %w", err)
	}

	taskID := fmt.Sprintf("sync-%s-%d", time.Now().UTC().Format("20060102T150405.000000000"), atomic.AddUint64(&s.syncTaskSeq, 1))
	progress := bt.NewSyncProgress(taskID, req.Symbol, time.Now().UTC())

	if s.syncTasks == nil {
		jftradeErr2 := syncer.Close()
		jftradeLogError(jftradeErr2)
		return nil, fmt.Errorf("sync task store not configured")
	}

	syncCtx, syncCancel, err := s.beginTask(ctx)
	if err != nil {
		jftradeErr1 := syncer.Close()
		jftradeLogError(jftradeErr1)
		return nil, err
	}
	syncCtx = observability.WithFields(syncCtx, observability.Fields{
		TaskID:       taskID,
		InstrumentID: req.Symbol,
		Source:       "backtest",
	})
	s.syncTasks.Add(taskID, progress, syncCancel)
	observability.InfoWithImportance(syncCtx, observability.ImportanceNormal, "backtest sync task started", "interval_count", len(intervals))

	go func() {
		defer s.finishTask(syncCancel)
		defer func() { jftradeLogError(syncer.Close()) }()
		defer s.syncTasks.Finish(taskID)
		params := KLineSyncParams{
			Symbol:       req.Symbol,
			Intervals:    intervals,
			Since:        sinceTime,
			Until:        untilTime,
			RehabType:    rehabType,
			SessionScope: req.SessionScope,
		}
		syncErr := syncer.Sync(syncCtx, params, progress)
		snapshot := progress.Snapshot()
		if syncCtx.Err() != nil {
			if snapshot != nil && !isTerminalSyncStatus(snapshot.Status) {
				progress.MarkCancelled(time.Now().UTC())
			}
		} else if syncErr != nil {
			if snapshot != nil && !isTerminalSyncStatus(snapshot.Status) {
				progress.MarkFailed(syncErr, time.Now().UTC())
			}
			snapshot = progress.Snapshot()
			if snapshot != nil && snapshot.Status != "cancelled" {
				observability.ErrorWithImportance(syncCtx, observability.ImportanceHigh, "backtest sync task failed", syncErr, "status", snapshot.Status)
			}
		}
		snapshot = progress.Snapshot()
		if snapshot != nil {
			observability.InfoWithImportance(syncCtx, observability.ImportanceNormal, "backtest sync task finished", "status", snapshot.Status, "retries", snapshot.Retries)
		}
	}()

	return &SyncStarted{
		TaskID:       taskID,
		Symbol:       req.Symbol,
		Intervals:    intervals,
		Since:        sinceTime.UTC().Format(time.RFC3339Nano),
		Until:        untilTime.UTC().Format(time.RFC3339Nano),
		SessionScope: req.SessionScope,
		Message:      "sync started",
	}, nil
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

// Close cancels active backtests and syncs, then waits for their goroutines to
// stop before callers close the stores they use.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	if !s.closed {
		s.closed = true
		s.lifecycleCancel()
	}
	s.lifecycleMu.Unlock()
	s.lifecycleTasks.Wait()
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────────────────────────────────────────

func (s *Service) dbPath() string {
	if s.dbPathFn != nil {
		return s.dbPathFn()
	}
	return ""
}

func (s *Service) beginTask(parent context.Context) (context.Context, context.CancelFunc, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closed {
		return nil, nil, ErrServiceClosed
	}
	ctx, cancel := context.WithCancel(observability.Detach(s.lifecycleCtx, parent))
	s.lifecycleTasks.Add(1)
	return ctx, cancel, nil
}

func (s *Service) finishTask(cancel context.CancelFunc) {
	cancel()
	s.lifecycleTasks.Done()
}

func isTerminalSyncStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// parseInstrument 统一标的输入解析（市场 + 代码/符号 → 标准化的市场/前缀/代码/符号）。
func parseInstrument(marketInput, symbol, code string) (struct{ Market, Prefix, Code, Symbol string }, error) {
	instrument, err := market.ParseInstrument(market.InstrumentInput{
		Market: marketInput,
		Symbol: symbol,
		Code:   code,
	})
	if err != nil {
		return struct{ Market, Prefix, Code, Symbol string }{}, err
	}
	return struct{ Market, Prefix, Code, Symbol string }{
		Market: instrument.Market,
		Prefix: instrument.Prefix,
		Code:   instrument.Code,
		Symbol: instrument.Symbol,
	}, nil
}

func resolveBacktestTimeRange(symbol, startDate, endDate, startTime, endTime string) (time.Time, time.Time, string, string, string, error) {
	profile, ok := market.ProfileForSymbol(symbol)
	if !ok || profile.Location == nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("unsupported market timezone for %s", symbol)
	}
	startDate = strings.TrimSpace(startDate)
	endDate = strings.TrimSpace(endDate)
	if startDate != "" || endDate != "" {
		if startDate == "" || endDate == "" {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("startDate and endDate must be provided together")
		}
		startLocal, err := parseMarketDate(startDate, profile.Location)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid startDate, use YYYY-MM-DD format")
		}
		endLocal, err := parseMarketDate(endDate, profile.Location)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid endDate, use YYYY-MM-DD format")
		}
		if endLocal.Before(startLocal) {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("endDate must not be before startDate")
		}
		return startLocal.UTC(), endLocal.AddDate(0, 0, 1).Add(-time.Nanosecond).UTC(), startDate, endDate, profile.Location.String(), nil
	}

	start, err := parseRFC3339Time(startTime)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid startTime, use RFC3339 format")
	}
	end, err := parseRFC3339Time(endTime)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid endTime, use RFC3339 format")
	}
	return start.UTC(), end.UTC(), "", "", profile.Location.String(), nil
}

func resolveSyncTimeRange(symbol, startDate, endDate, since, until string) (time.Time, time.Time, string, string, string, error) {
	if strings.TrimSpace(startDate) != "" || strings.TrimSpace(endDate) != "" {
		return resolveBacktestTimeRange(symbol, startDate, endDate, "", "")
	}
	now := time.Now().UTC()
	sinceTime := now.AddDate(0, 0, -30)
	untilTime := now
	var err error
	if strings.TrimSpace(since) != "" {
		sinceTime, err = parseRFC3339Time(since)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid since time, use RFC3339")
		}
	}
	if strings.TrimSpace(until) != "" {
		untilTime, err = parseRFC3339Time(until)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", "", requestErrorf("invalid until time, use RFC3339")
		}
	}
	return sinceTime.UTC(), untilTime.UTC(), "", "", "", nil
}

func parseMarketDate(value string, location *time.Location) (time.Time, error) {
	if len(value) != len("2006-01-02") {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	return parsed, nil
}

func parseRFC3339Time(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

// failureResult 构造回测失败结果。
func failureResult(req StartRequest, message string) *bt.RunResult {
	return &bt.RunResult{
		Symbol:       req.Symbol,
		Interval:     req.Interval,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		FinalBalance: req.InitialBalance,
		TradingCosts: req.TradingCosts,
		Error:        message,
	}
}

func parseResultViewFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 同步辅助函数
// ──────────────────────────────────────────────────────────────────────────────

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
	// 3d/2w 不支持，降级为 1d
	if interval == bbgotypes.Interval("3d") || interval == bbgotypes.Interval("2w") {
		return bbgotypes.Interval1d
	}
	// 子日级别降级为 1h
	if duration > time.Hour && duration < 24*time.Hour {
		return bbgotypes.Interval1h
	}
	// 美股扩展时段 + 日线及以上 → 降级为 1h（需要小时数据计算扩展时段）
	if normalizeSessionScope(sessionScope) == "extended" &&
		strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") &&
		duration >= 24*time.Hour {
		return bbgotypes.Interval1h
	}
	return interval
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
