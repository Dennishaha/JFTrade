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
	"math"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/market"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

// ──────────────────────────────────────────────────────────────────────────────
// 请求 / 响应类型
// ──────────────────────────────────────────────────────────────────────────────

// StartRequest 回测启动参数（从 HTTP JSON body 反序列化）。
type StartRequest struct {
	DefinitionID      string  `json:"definitionId"`
	DefinitionVersion string  `json:"definitionVersion,omitempty"`
	Market            string  `json:"market"`
	Code              string  `json:"code"`
	Symbol            string  `json:"symbol"`
	Interval          string  `json:"interval"`
	StartTime         string  `json:"startTime"`
	EndTime           string  `json:"endTime"`
	InitialBalance    float64 `json:"initialBalance"`
	RehabType         string  `json:"rehabType"`
	UseExtendedHours  *bool   `json:"useExtendedHours,omitempty"`
}

// ScriptStartRequest starts a transient research backtest from an inline Pine
// script without requiring or creating a saved strategy definition.
type ScriptStartRequest struct {
	Script           string  `json:"script"`
	Market           string  `json:"market"`
	Code             string  `json:"code"`
	Symbol           string  `json:"symbol"`
	Interval         string  `json:"interval"`
	StartTime        string  `json:"startTime"`
	EndTime          string  `json:"endTime"`
	InitialBalance   float64 `json:"initialBalance"`
	RehabType        string  `json:"rehabType"`
	UseExtendedHours *bool   `json:"useExtendedHours,omitempty"`
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
	Since        string   `json:"since"`
	Until        string   `json:"until"`
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

	// 回测数据库文件路径提供者
	dbPathFn func() string

	// 回测执行器（通常为 backtest.Run）
	runBacktestFn func(ctx context.Context, config bt.RunConfig) *bt.RunResult

	// 创建 broker-specific K 线同步适配器。
	newKLineSyncerFn func(dbPath string) (KLineSyncer, error)
}

// NewService 创建回测服务。所有依赖通过 Option 注入。
func NewService(opts ...Option) *Service {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	s := &Service{
		runBacktestFn:   bt.Run,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
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

// WithNewKLineSyncerFn sets the broker integration adapter factory.
func WithNewKLineSyncerFn(fn func(dbPath string) (KLineSyncer, error)) Option {
	return func(s *Service) { s.newKLineSyncerFn = fn }
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
	hash := sha256.Sum256([]byte(script))
	scriptHash := hex.EncodeToString(hash[:])
	def := StrategyDef{
		ID:           "adk-research-" + scriptHash[:12],
		Version:      "script-" + scriptHash[:12],
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       script,
	}
	return s.startResolvedBacktest(ctx, StartRequest{
		DefinitionID:      def.ID,
		DefinitionVersion: def.Version,
		Market:            req.Market,
		Code:              req.Code,
		Symbol:            req.Symbol,
		Interval:          req.Interval,
		StartTime:         req.StartTime,
		EndTime:           req.EndTime,
		InitialBalance:    req.InitialBalance,
		RehabType:         req.RehabType,
		UseExtendedHours:  req.UseExtendedHours,
	}, def)
}

func (s *Service) startResolvedBacktest(ctx context.Context, req StartRequest, def StrategyDef) (*RunState, error) {
	// 统一标的解析
	instrument, err := parseInstrument(req.Market, req.Symbol, req.Code)
	if err != nil {
		return nil, requestErrorf("%v", err)
	}
	req.Market = instrument.Market
	req.Code = instrument.Code
	req.Symbol = instrument.Symbol

	if strings.TrimSpace(req.Interval) == "" {
		req.Interval = "1m"
	}

	if strategydefinition.NormalizeSourceFormat(def.SourceFormat) != strategydefinition.SourceFormatPineV6 {
		return nil, requestErrorf("unsupported strategy source format: %s", def.SourceFormat)
	}

	// 编译策略脚本
	compilation, err := strategypine.Compile(def.Script)
	if err != nil {
		return nil, requestErrorf("%v", err)
	}
	if req.InitialBalance <= 0 {
		req.InitialBalance = compilation.Program.Metadata.InitialCapital
	}
	if req.InitialBalance <= 0 {
		req.InitialBalance = 100000
	}
	req.DefinitionVersion = def.Version

	// 校验时间
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, requestErrorf("invalid startTime, use RFC3339 format")
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, requestErrorf("invalid endTime, use RFC3339 format")
	}
	if !endTime.After(startTime) {
		return nil, requestErrorf("endTime must be after startTime")
	}

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
	runCtx, cancel, err := s.beginTask()
	if err != nil {
		return nil, err
	}
	if err := s.runs.Add(run); err != nil {
		s.finishTask(cancel)
		return nil, fmt.Errorf("persist backtest run: %w", err)
	}

	s.runs.SetCancel(runID, cancel)

	// 异步执行回测
	go s.executeBacktest(runCtx, runID, req, def, startTime, endTime, cancel)

	return run, nil
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
			log.Printf("backtest run %s panicked: %v\n%s", runID, recovered, string(debug.Stack()))
			s.finishRun(runID, "failed", failureResult(req, fmt.Sprintf("backtest panic: %v", recovered)))
		}
	}()

	if _, err := s.runs.Update(runID, func(run *RunState) {
		run.Status = "running"
		run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}); err != nil {
		log.Printf("backtest run store update(%s running) failed: %v", runID, err)
	}

	result := s.runBacktestFn(ctx, bt.RunConfig{
		DBPath:           s.dbPath(),
		Symbol:           req.Symbol,
		Interval:         req.Interval,
		SourceFormat:     def.SourceFormat,
		StartTime:        startTime,
		EndTime:          endTime,
		StrategyScript:   def.Script,
		InitialBalance:   req.InitialBalance,
		RehabType:        req.RehabType,
		UseExtendedHours: req.UseExtendedHours,
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

// ResultView returns a bounded, tool-friendly view of a backtest result. Large
// series are windowed and optionally aggregated so agents can inspect charts in
// several smaller calls instead of loading the full result into context.
func (s *Service) ResultView(req ResultViewRequest) (map[string]any, error) {
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		return nil, requestErrorf("runId is required")
	}
	run, ok, err := s.GetResult(runID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, requestErrorf("backtest run not found: %s", runID)
	}

	view := strings.ToLower(strings.TrimSpace(req.View))
	if view == "" {
		view = "summary"
	}
	switch view {
	case "summary", "chart", "orders", "logs", "errors":
	default:
		return nil, requestErrorf("view must be one of summary, chart, orders, logs, errors")
	}

	limit := normalizeResultViewLimit(req.Limit)
	offset, err := parseResultViewCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	startTime, err := parseOptionalResultViewTime(req.StartTime, "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := parseOptionalResultViewTime(req.EndTime, "endTime")
	if err != nil {
		return nil, err
	}
	if startTime != nil && endTime != nil && endTime.Before(*startTime) {
		return nil, requestErrorf("endTime must be after or equal to startTime")
	}

	result := run.Result
	payload := map[string]any{
		"view":    view,
		"run":     resultViewRunPayload(run),
		"summary": resultViewSummaryPayload(run),
		"window": map[string]any{
			"startTime":      req.StartTime,
			"endTime":        req.EndTime,
			"nativeInterval": run.Request.Interval,
			"limit":          limit,
			"cursor":         req.Cursor,
			"offset":         offset,
			"returned":       map[string]int{},
			"truncated":      false,
			"nextCursor":     "",
		},
		"series": map[string]any{},
	}
	if result == nil {
		return payload, nil
	}

	window := jftradeCheckedTypeAssertion[map[string]any](payload["window"])
	series := jftradeCheckedTypeAssertion[map[string]any](payload["series"])
	returned := jftradeCheckedTypeAssertion[map[string]int](window["returned"])

	switch view {
	case "summary":
		return payload, nil
	case "chart":
		include := resultViewIncludeSet(req.Include, []string{"candles", "trades", "pnlCurve", "drawdownCurve"})
		resolution, candles, err := resultViewCandles(result.Candles, run.Request.Interval, req.Resolution, startTime, endTime, limit)
		if err != nil {
			return nil, err
		}
		window["resolution"] = resolution
		if include["candles"] {
			items, next := sliceResultViewItems(candles, offset, limit)
			series["candles"] = items
			returned["candles"] = len(items)
			applyResultViewNextCursor(window, next)
		}
		if include["trades"] {
			filtered := filterResultViewTimedItems(result.Trades, startTime, endTime, func(item bt.TradeEvent) string { return item.Time })
			items, next := sliceResultViewItems(filtered, offset, limit)
			series["trades"] = items
			returned["trades"] = len(items)
			applyResultViewNextCursor(window, next)
		}
		if include["pnlCurve"] {
			filtered := filterResultViewTimedItems(result.PnLCurve, startTime, endTime, func(item bt.PnLPoint) string { return item.Time })
			items, next := sliceResultViewItems(filtered, offset, limit)
			series["pnlCurve"] = items
			returned["pnlCurve"] = len(items)
			applyResultViewNextCursor(window, next)
		}
		if include["drawdownCurve"] {
			filtered := filterResultViewTimedItems(result.DrawdownCurve, startTime, endTime, func(item bt.DrawdownPoint) string { return item.Time })
			items, next := sliceResultViewItems(filtered, offset, limit)
			series["drawdownCurve"] = items
			returned["drawdownCurve"] = len(items)
			applyResultViewNextCursor(window, next)
		}
	case "orders":
		items, next := sliceResultViewItems(filterResultViewOrders(result.OrderBook, startTime, endTime), offset, limit)
		series["orderBook"] = items
		returned["orderBook"] = len(items)
		applyResultViewNextCursor(window, next)
	case "logs":
		items, next := sliceResultViewItems(result.Logs, offset, limit)
		series["logs"] = items
		returned["logs"] = len(items)
		applyResultViewNextCursor(window, next)
	case "errors":
		items, next := sliceResultViewItems(result.RuntimeErrors, offset, limit)
		series["runtimeErrors"] = items
		returned["runtimeErrors"] = len(items)
		applyResultViewNextCursor(window, next)
	}
	return payload, nil
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

	// 默认同步最近 30 天
	sinceTime := time.Now().AddDate(0, 0, -30)
	if req.Since != "" {
		sinceTime, err = time.Parse(time.RFC3339, req.Since)
		if err != nil {
			return nil, requestErrorf("invalid since time, use RFC3339")
		}
	}
	untilTime := time.Now()
	if req.Until != "" {
		untilTime, err = time.Parse(time.RFC3339, req.Until)
		if err != nil {
			return nil, requestErrorf("invalid until time, use RFC3339")
		}
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

	syncCtx, syncCancel, err := s.beginTask()
	if err != nil {
		jftradeErr1 := syncer.Close()
		jftradeLogError(jftradeErr1)
		return nil, err
	}
	s.syncTasks.Add(taskID, progress, syncCancel)

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
				log.Printf("backtest sync failed %s: %v", req.Symbol, syncErr)
			}
		}
		snapshot = progress.Snapshot()
		if snapshot != nil {
			log.Printf("backtest sync %s: status=%s retries=%d", req.Symbol, snapshot.Status, snapshot.Retries)
		}
	}()

	return &SyncStarted{
		TaskID:       taskID,
		Symbol:       req.Symbol,
		Intervals:    intervals,
		Since:        sinceTime.Format(time.RFC3339),
		Until:        untilTime.Format(time.RFC3339),
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

func (s *Service) beginTask() (context.Context, context.CancelFunc, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closed {
		return nil, nil, ErrServiceClosed
	}
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
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

// failureResult 构造回测失败结果。
func failureResult(req StartRequest, message string) *bt.RunResult {
	return &bt.RunResult{
		Symbol:       req.Symbol,
		Interval:     req.Interval,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		FinalBalance: req.InitialBalance,
		Error:        message,
	}
}

func normalizeResultViewLimit(limit int) int {
	if limit <= 0 {
		return 500
	}
	if limit > 2000 {
		return 2000
	}
	return limit
}

func parseResultViewCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, requestErrorf("cursor must be a non-negative integer offset")
	}
	return offset, nil
}

func parseOptionalResultViewTime(value string, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseResultViewTime(value)
	if err != nil {
		return nil, requestErrorf("invalid %s, use RFC3339 format", field)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func parseResultViewTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

func resultViewRunPayload(run *RunState) map[string]any {
	if run == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":                run.ID,
		"status":            run.Status,
		"definitionId":      run.Request.DefinitionID,
		"definitionVersion": run.Request.DefinitionVersion,
		"market":            run.Request.Market,
		"code":              run.Request.Code,
		"symbol":            run.Request.Symbol,
		"interval":          run.Request.Interval,
		"startTime":         run.Request.StartTime,
		"endTime":           run.Request.EndTime,
		"initialBalance":    run.Request.InitialBalance,
		"rehabType":         run.Request.RehabType,
		"useExtendedHours":  run.Request.UseExtendedHours,
		"createdAt":         run.CreatedAt,
		"updatedAt":         run.UpdatedAt,
	}
}

func resultViewSummaryPayload(run *RunState) map[string]any {
	summary := map[string]any{}
	if run == nil || run.Result == nil {
		return summary
	}
	result := run.Result
	summary["quoteCurrency"] = result.QuoteCurrency
	summary["finalBalance"] = result.FinalBalance
	summary["pnl"] = result.PnL
	if run.Request.InitialBalance > 0 {
		summary["totalReturn"] = result.PnL / run.Request.InitialBalance
	}
	summary["maxDrawdown"] = result.MaxDrawdown
	summary["currentDrawdown"] = result.CurrentDrawdown
	summary["totalTrades"] = result.TotalTrades
	summary["winRate"] = result.WinRate
	summary["candlesCount"] = len(result.Candles)
	summary["tradesCount"] = len(result.Trades)
	summary["orderBookCount"] = len(result.OrderBook)
	summary["pnlCurveCount"] = len(result.PnLCurve)
	summary["drawdownCurveCount"] = len(result.DrawdownCurve)
	summary["logsCount"] = len(result.Logs)
	summary["runtimeErrorCount"] = len(result.RuntimeErrors)
	summary["runtimeErrorTotal"] = result.RuntimeErrorTotal
	summary["error"] = result.Error
	if len(result.Logs) > 0 {
		summary["latestLog"] = result.Logs[len(result.Logs)-1]
	}
	if len(result.RuntimeErrors) > 0 {
		summary["latestRuntimeError"] = result.RuntimeErrors[len(result.RuntimeErrors)-1]
	}
	return summary
}

func resultViewIncludeSet(include []string, defaults []string) map[string]bool {
	if len(include) == 0 {
		include = defaults
	}
	set := make(map[string]bool, len(include))
	for _, item := range include {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = true
	}
	return set
}

func sliceResultViewItems[T any](items []T, offset int, limit int) ([]T, string) {
	if offset >= len(items) {
		return []T{}, ""
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return append([]T(nil), items[offset:end]...), next
}

func applyResultViewNextCursor(window map[string]any, next string) {
	if strings.TrimSpace(next) == "" {
		return
	}
	window["truncated"] = true
	if strings.TrimSpace(fmt.Sprint(window["nextCursor"])) == "" {
		window["nextCursor"] = next
	}
}

func filterResultViewTimedItems[T any](items []T, startTime *time.Time, endTime *time.Time, timeFn func(T) string) []T {
	if startTime == nil && endTime == nil {
		return append([]T(nil), items...)
	}
	out := make([]T, 0, len(items))
	for _, item := range items {
		parsed, err := parseResultViewTime(timeFn(item))
		if err != nil {
			continue
		}
		parsed = parsed.UTC()
		if startTime != nil && parsed.Before(*startTime) {
			continue
		}
		if endTime != nil && parsed.After(*endTime) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterResultViewOrders(items []bt.OrderBookEntry, startTime *time.Time, endTime *time.Time) []bt.OrderBookEntry {
	if startTime == nil && endTime == nil {
		return append([]bt.OrderBookEntry(nil), items...)
	}
	out := make([]bt.OrderBookEntry, 0, len(items))
	for _, item := range items {
		if resultViewTimeInWindow(item.SubmittedAt, startTime, endTime) || resultViewTimeInWindow(item.FilledAt, startTime, endTime) {
			out = append(out, item)
		}
	}
	return out
}

func resultViewTimeInWindow(value string, startTime *time.Time, endTime *time.Time) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	parsed, err := parseResultViewTime(value)
	if err != nil {
		return false
	}
	parsed = parsed.UTC()
	if startTime != nil && parsed.Before(*startTime) {
		return false
	}
	if endTime != nil && parsed.After(*endTime) {
		return false
	}
	return true
}

func resultViewCandles(
	candles []bt.Candle,
	nativeInterval string,
	resolution string,
	startTime *time.Time,
	endTime *time.Time,
	limit int,
) (string, []bt.Candle, error) {
	filtered := filterResultViewTimedItems(candles, startTime, endTime, func(item bt.Candle) string { return item.Time })
	nativeDuration, err := resultViewIntervalDuration(nativeInterval)
	if err != nil {
		return "", nil, err
	}
	var targetDuration time.Duration
	normalizedResolution := strings.ToLower(strings.TrimSpace(resolution))
	if normalizedResolution == "" || normalizedResolution == "auto" {
		targetDuration = chooseResultViewAutoResolution(nativeDuration, len(filtered), limit)
	} else {
		targetDuration, err = resultViewIntervalDuration(normalizedResolution)
		if err != nil {
			return "", nil, err
		}
		if targetDuration < nativeDuration {
			return "", nil, requestErrorf("resolution %s is finer than native interval %s", resolution, nativeInterval)
		}
	}
	label := resultViewResolutionLabel(targetDuration)
	if targetDuration <= nativeDuration || len(filtered) == 0 {
		return label, filtered, nil
	}
	return label, aggregateResultViewCandles(filtered, targetDuration), nil
}

func resultViewIntervalDuration(value string) (time.Duration, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return 0, requestErrorf("resolution is required")
	}
	if duration := bbgotypes.Interval(value).Duration(); duration > 0 {
		return duration, nil
	}
	unit := value[len(value)-1]
	numberText := value[:len(value)-1]
	if unit >= '0' && unit <= '9' {
		numberText = value
		unit = 'm'
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number <= 0 {
		return 0, requestErrorf("unsupported interval or resolution: %s", value)
	}
	switch unit {
	case 's':
		return time.Duration(number) * time.Second, nil
	case 'm':
		return time.Duration(number) * time.Minute, nil
	case 'h':
		return time.Duration(number) * time.Hour, nil
	case 'd':
		return time.Duration(number) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(number) * 7 * 24 * time.Hour, nil
	default:
		return 0, requestErrorf("unsupported interval or resolution: %s", value)
	}
}

func chooseResultViewAutoResolution(nativeDuration time.Duration, count int, limit int) time.Duration {
	if count <= limit || limit <= 0 {
		return nativeDuration
	}
	required := nativeDuration * time.Duration(int(math.Ceil(float64(count)/float64(limit))))
	for _, candidate := range []time.Duration{
		time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute,
		time.Hour, 2 * time.Hour, 4 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour,
	} {
		if candidate >= nativeDuration && candidate >= required {
			return candidate
		}
	}
	return required
}

func resultViewResolutionLabel(duration time.Duration) string {
	switch {
	case duration%(7*24*time.Hour) == 0:
		return fmt.Sprintf("%dw", int(duration/(7*24*time.Hour)))
	case duration%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(duration/(24*time.Hour)))
	case duration%time.Hour == 0:
		return fmt.Sprintf("%dh", int(duration/time.Hour))
	case duration%time.Minute == 0:
		return fmt.Sprintf("%dm", int(duration/time.Minute))
	default:
		return fmt.Sprintf("%ds", int(duration/time.Second))
	}
}

func aggregateResultViewCandles(candles []bt.Candle, resolution time.Duration) []bt.Candle {
	if len(candles) == 0 || resolution <= 0 {
		return append([]bt.Candle(nil), candles...)
	}
	out := make([]bt.Candle, 0, len(candles))
	var current *bt.Candle
	var currentBucket int64
	var volumeSum float64
	var volumeOK bool
	for _, candle := range candles {
		parsed, err := parseResultViewTime(candle.Time)
		if err != nil {
			continue
		}
		bucket := parsed.UTC().Unix() / int64(resolution.Seconds())
		if current == nil || bucket != currentBucket {
			if current != nil {
				if volumeOK {
					current.Volume = strconv.FormatFloat(volumeSum, 'f', -1, 64)
				}
				out = append(out, *current)
			}
			clone := candle
			clone.Time = time.Unix(bucket*int64(resolution.Seconds()), 0).UTC().Format(time.RFC3339)
			current = &clone
			currentBucket = bucket
			volumeSum, volumeOK = parseResultViewFloat(candle.Volume)
			continue
		}
		current.High = resultViewMaxString(current.High, candle.High)
		current.Low = resultViewMinString(current.Low, candle.Low)
		current.Close = candle.Close
		if volume, ok := parseResultViewFloat(candle.Volume); ok && volumeOK {
			volumeSum += volume
		} else {
			volumeOK = false
		}
	}
	if current != nil {
		if volumeOK {
			current.Volume = strconv.FormatFloat(volumeSum, 'f', -1, 64)
		}
		out = append(out, *current)
	}
	return out
}

func resultViewMaxString(left string, right string) string {
	leftValue, leftOK := parseResultViewFloat(left)
	rightValue, rightOK := parseResultViewFloat(right)
	if leftOK && rightOK && rightValue > leftValue {
		return right
	}
	return left
}

func resultViewMinString(left string, right string) string {
	leftValue, leftOK := parseResultViewFloat(left)
	rightValue, rightOK := parseResultViewFloat(right)
	if leftOK && rightOK && rightValue < leftValue {
		return right
	}
	return left
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
