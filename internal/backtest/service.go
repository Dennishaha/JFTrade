// Package backtest 提供回测业务编排层。回测启动、状态查询和 K 线同步由
// 独立 Service 负责，Handler 层仅处理参数绑定与响应写入。
//
// RunStore、SyncTaskStore 和外部行情同步能力均通过接口注入，业务层不依赖
// HTTP transport 或具体券商协议。
package backtest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
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

// RunState 是回测运行状态的纯数据结构。
type RunState struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"` // "queued" | "running" | "completed" | "failed" | "cancelled"
	Request   StartRequest  `json:"request"`
	Result    *bt.RunResult `json:"result,omitempty"`
	CreatedAt string        `json:"createdAt"`
	UpdatedAt string        `json:"updatedAt"`
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

	// 查策略定义
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
		_ = syncer.Close()
		return nil, fmt.Errorf("sync task store not configured")
	}

	syncCtx, syncCancel, err := s.beginTask()
	if err != nil {
		_ = syncer.Close()
		return nil, err
	}
	s.syncTasks.Add(taskID, progress, syncCancel)

	go func() {
		defer s.finishTask(syncCancel)
		defer syncer.Close()
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
