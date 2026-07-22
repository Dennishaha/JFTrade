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
	"sync"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/observability"
)

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
	ExecutionModel    string          `json:"executionModel,omitempty"`
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
	ExecutionModel   string          `json:"executionModel,omitempty"`
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

type preparedSync struct {
	request   SyncRequest
	sinceTime time.Time
	untilTime time.Time
	intervals []bbgotypes.Interval
	rehabType RehabType
}

type normalizedResultView struct {
	view      string
	limit     int
	offset    int
	startTime *time.Time
	endTime   *time.Time
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
	Definition(id string) (StrategyDef, bool, error)
}

// StrategyDef 策略定义（回测编排所需的最小字段集）。
type StrategyDef struct {
	ID           string
	Version      string
	SourceFormat string
	Script       string
}

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

	dataSyncMu    sync.Mutex
	dataSyncTasks map[string]*SyncStarted

	dbPathFn func() string

	runBacktestFn func(ctx context.Context, config bt.RunConfig) *bt.RunResult

	pineWorkerMu     sync.RWMutex
	pineWorkerRunner bt.PineWorkerRunner

	newKLineSyncerFn func(dbPath string) (KLineSyncer, error)

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
	s.dataSyncMu.Lock()
	clear(s.dataSyncTasks)
	s.dataSyncMu.Unlock()
	return nil
}

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
