package system

import (
	"context"
	"time"

	"github.com/jftrade/jftrade-main/internal/buildinfo"
)

// Service 提供系统级状态查询能力。所有外部依赖通过接口注入。
type Service struct {
	startedAt                   time.Time
	apiPort                     int
	apiPortFn                   func() int
	settingsPath                string
	defaultTradingEnvironment   string
	defaultTradingEnvironmentFn func() string
	brokerDescriptor            func() map[string]any
	strategyRuntimeSummary      func() map[string]any
	liveStats                   func() map[string]any
	marketdataRuntimeSummary    func() map[string]any
	brokerOrderSnapshot         func() map[string]any
	exchangeCalendarStatusFn    func() map[string]any
	exchangeCalendarSourcesFn   func() []map[string]any
	refreshExchangeCalendarsFn  func(ctx context.Context, market string) map[string]any
	probeExchangeCalendarsFn    func(ctx context.Context, market string) map[string]any
	futuOpenDHealthFn           func(ctx context.Context) map[string]any
	futuOpenDInstallGuideFn     func() map[string]any
	resetFutuRuntimeFn          func()
	runtimeDependenciesFn       func(ctx context.Context) map[string]any
}

// NewService 创建一个系统服务。
func NewService(opts ...Option) *Service {
	s := &Service{apiPort: 3000, startedAt: time.Now().UTC()}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option 函数式选项。
type Option func(*Service)

// WithAPIPort 设置 API 端口。
func WithAPIPort(port int) Option {
	return func(s *Service) { s.apiPort = port }
}

// WithAPIPortFunc 设置动态 API 端口提供者。
func WithAPIPortFunc(fn func() int) Option {
	return func(s *Service) { s.apiPortFn = fn }
}

// WithSettingsPath 设置持久化文件路径。
func WithSettingsPath(path string) Option {
	return func(s *Service) { s.settingsPath = path }
}

// WithDefaultTradingEnvironment 设置默认交易环境。
func WithDefaultTradingEnvironment(env string) Option {
	return func(s *Service) { s.defaultTradingEnvironment = env }
}

// WithDefaultTradingEnvironmentFunc 设置动态默认交易环境提供者。
func WithDefaultTradingEnvironmentFunc(fn func() string) Option {
	return func(s *Service) { s.defaultTradingEnvironmentFn = fn }
}

// WithBrokerDescriptor 设置 broker 描述符提供者。
func WithBrokerDescriptor(fn func() map[string]any) Option {
	return func(s *Service) { s.brokerDescriptor = fn }
}

// WithStrategyRuntimeSummary 设置策略运行时摘要提供者。
func WithStrategyRuntimeSummary(fn func() map[string]any) Option {
	return func(s *Service) { s.strategyRuntimeSummary = fn }
}

// WithLiveStats 设置实时连接统计提供者。
func WithLiveStats(fn func() map[string]any) Option {
	return func(s *Service) { s.liveStats = fn }
}

// WithMarketdataRuntimeSummary 设置行情采集运行时摘要提供者。
func WithMarketdataRuntimeSummary(fn func() map[string]any) Option {
	return func(s *Service) { s.marketdataRuntimeSummary = fn }
}

// WithBrokerOrderSnapshot 设置 broker 订单更新 Worker 快照提供者。
func WithBrokerOrderSnapshot(fn func() map[string]any) Option {
	return func(s *Service) { s.brokerOrderSnapshot = fn }
}

// WithExchangeCalendarStatus 设置交易所日历状态提供者。
func WithExchangeCalendarStatus(fn func() map[string]any) Option {
	return func(s *Service) { s.exchangeCalendarStatusFn = fn }
}

// WithExchangeCalendarSources 设置交易所日历数据源提供者。
func WithExchangeCalendarSources(fn func() []map[string]any) Option {
	return func(s *Service) { s.exchangeCalendarSourcesFn = fn }
}

// WithRefreshExchangeCalendars 设置交易所日历刷新回调。
func WithRefreshExchangeCalendars(fn func(ctx context.Context, market string) map[string]any) Option {
	return func(s *Service) { s.refreshExchangeCalendarsFn = fn }
}

// WithProbeExchangeCalendars 设置交易所日历在线探针回调。
func WithProbeExchangeCalendars(fn func(ctx context.Context, market string) map[string]any) Option {
	return func(s *Service) { s.probeExchangeCalendarsFn = fn }
}

// WithFutuOpenDHealth 设置 Futu/OpenD 健康检查提供者。
func WithFutuOpenDHealth(fn func(ctx context.Context) map[string]any) Option {
	return func(s *Service) { s.futuOpenDHealthFn = fn }
}

// WithFutuOpenDInstallGuide 设置 OpenD 安装指南提供者。
func WithFutuOpenDInstallGuide(fn func() map[string]any) Option {
	return func(s *Service) { s.futuOpenDInstallGuideFn = fn }
}

// WithResetFutuRuntime 设置 Futu 运行时重置回调。
func WithResetFutuRuntime(fn func()) Option {
	return func(s *Service) { s.resetFutuRuntimeFn = fn }
}

// WithRuntimeDependencies 设置运行时依赖检查提供者。
func WithRuntimeDependencies(fn func(ctx context.Context) map[string]any) Option {
	return func(s *Service) { s.runtimeDependenciesFn = fn }
}

// ── 系统状态 ──

// Status 返回系统整体状态摘要。
func (s *Service) Status() map[string]any {
	now := time.Now().UTC()
	apiPort := s.apiPort
	if s.apiPortFn != nil {
		apiPort = s.apiPortFn()
	}
	defaultTradingEnvironment := s.defaultTradingEnvironment
	if s.defaultTradingEnvironmentFn != nil {
		defaultTradingEnvironment = s.defaultTradingEnvironmentFn()
	}
	broker := map[string]any(nil)
	if s.brokerDescriptor != nil {
		broker = s.brokerDescriptor()
	}
	strategyRuntime := map[string]any(nil)
	if s.strategyRuntimeSummary != nil {
		strategyRuntime = s.strategyRuntimeSummary()
	}
	live := map[string]any(nil)
	if s.liveStats != nil {
		live = s.liveStats()
	}
	marketdata := map[string]any(nil)
	if s.marketdataRuntimeSummary != nil {
		marketdata = s.marketdataRuntimeSummary()
	}
	exchangeCalendars := map[string]any(nil)
	if s.exchangeCalendarStatusFn != nil {
		exchangeCalendars = s.exchangeCalendarStatusFn()
	}
	status := map[string]any{
		"name":                      "JFTrade",
		"apiPort":                   apiPort,
		"defaultBroker":             "futu",
		"defaultTradingEnvironment": defaultTradingEnvironment,
		"realTradingEnabled":        false,
		"realTradingKillSwitch": map[string]any{
			"active": false, "envConfiguredActive": false, "controlPlaneActive": false,
			"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true,
		},
		"realTradingRisk": map[string]any{
			"enabled": false, "maxOrderQuantity": nil, "maxOrderNotional": nil,
			"envConfiguredMaxOrderQuantity": nil, "envConfiguredMaxOrderNotional": nil,
			"controlPlaneActive": false, "controlPlaneMaxOrderQuantity": nil, "controlPlaneMaxOrderNotional": nil,
			"riskConfigSource": nil,
		},
		"realTradeAccess": map[string]any{
			"approverAllowlistEnabled": false, "approverCount": 0,
			"adminAllowlistEnabled": false, "adminCount": 0,
		},
		"build": buildinfo.Snapshot(),
		"persistence": map[string]any{
			"engine":            "json",
			"databasePath":      s.settingsPath,
			"status":            "ok",
			"migrated":          true,
			"pendingMigrations": []any{},
			"tables":            []string{"broker_integrations", "broker_accounts"},
			"checkedAt":         time.Now().UTC().Format(time.RFC3339Nano),
		},
		"observability": map[string]any{
			"api": map[string]any{
				"startedAt": s.startedAt.Format(time.RFC3339Nano),
				"uptimeMs":  now.Sub(s.startedAt).Milliseconds(),
			},
			"live":              live,
			"marketdata":        marketdata,
			"exchangeCalendars": exchangeCalendars,
			"broker":            broker,
			"strategyRuntime":   strategyRuntime,
		},
		"message": "JFTrade API adapter is running.",
	}

	if broker != nil {
		status["broker"] = broker
	}
	if strategyRuntime != nil {
		status["strategyRuntime"] = strategyRuntime
	}

	return status
}

// ExchangeCalendarStatus 返回交易所日历状态。
func (s *Service) ExchangeCalendarStatus() map[string]any {
	if s.exchangeCalendarStatusFn == nil {
		return map[string]any{}
	}
	return s.exchangeCalendarStatusFn()
}

// ExchangeCalendarSources 返回交易所日历数据源状态。
func (s *Service) ExchangeCalendarSources() []map[string]any {
	if s.exchangeCalendarSourcesFn == nil {
		return nil
	}
	return s.exchangeCalendarSourcesFn()
}

// RefreshExchangeCalendars 手动刷新交易所日历。
func (s *Service) RefreshExchangeCalendars(ctx context.Context, market string) map[string]any {
	if s.refreshExchangeCalendarsFn == nil {
		return map[string]any{"accepted": false, "reason": "exchange calendar manager not configured"}
	}
	return s.refreshExchangeCalendarsFn(ctx, market)
}

// ProbeExchangeCalendars 执行交易所日历官方源在线探针。
func (s *Service) ProbeExchangeCalendars(ctx context.Context, market string) map[string]any {
	if s.probeExchangeCalendarsFn == nil {
		return map[string]any{"accepted": false, "reason": "exchange calendar probe not configured"}
	}
	return s.probeExchangeCalendarsFn(ctx, market)
}

// ── 存储概览 ──

// StorageOverview 返回当前未启用持久化任务队列时的空存储概览。
func (s *Service) StorageOverview() map[string]any {
	return map[string]any{
		"pendingOutbox":           []any{},
		"recentJobs":              []any{},
		"recentAuditLogs":         []any{},
		"recentExecutionCommands": []any{},
	}
}

// ── 实盘风控 ──

// RealTradeApprovals 返回实盘审批状态。
func (s *Service) RealTradeApprovals() map[string]any {
	return map[string]any{
		"realTradingEnabled":       false,
		"requiredConfirmationText": "ENABLE_REAL_TRADING",
		"maxApprovalAgeMs":         5 * 60 * 1000,
		"approvalPolicy":           map[string]any{"approverAllowlistEnabled": false, "approverCount": 0},
		"entries":                  []any{},
	}
}

// RealTradeHardStops 返回硬止损状态。
func (s *Service) RealTradeHardStops() map[string]any {
	return map[string]any{
		"blockedOperations": []string{"PLACE", "MODIFY"},
		"allowsCancel":      true,
		"entries":           []any{},
	}
}

// RealTradeHardStopEvents 返回硬止损事件。
func (s *Service) RealTradeHardStopEvents() map[string]any {
	return map[string]any{
		"realTradingEnabled": false,
		"blockedOperations":  []string{"PLACE", "MODIFY"},
		"allowsCancel":       true,
		"entries":            []any{},
	}
}

// RealTradeKillSwitch 返回熔断状态。
func (s *Service) RealTradeKillSwitch() map[string]any {
	return map[string]any{
		"realTradingEnabled":  false,
		"killSwitchActive":    false,
		"killSwitchSource":    nil,
		"envConfiguredActive": false,
		"controlPlaneActive":  false,
		"blockedOperations":   []string{"PLACE", "MODIFY"},
		"allowsCancel":        true,
		"entry":               nil,
	}
}

// RealTradeKillSwitchEvents 返回熔断事件。
func (s *Service) RealTradeKillSwitchEvents() map[string]any {
	return map[string]any{
		"realTradingEnabled":  false,
		"killSwitchActive":    false,
		"envConfiguredActive": false,
		"controlPlaneActive":  false,
		"blockedOperations":   []string{"PLACE", "MODIFY"},
		"allowsCancel":        true,
		"entries":             []any{},
	}
}

// RealTradeRiskLimits 返回风控限额。
func (s *Service) RealTradeRiskLimits() map[string]any {
	return map[string]any{
		"realTradingEnabled":            false,
		"riskEnabled":                   false,
		"riskConfigSource":              nil,
		"envConfiguredMaxOrderQuantity": nil,
		"envConfiguredMaxOrderNotional": nil,
		"controlPlaneActive":            false,
		"controlPlaneMaxOrderQuantity":  nil,
		"controlPlaneMaxOrderNotional":  nil,
		"effectiveMaxOrderQuantity":     nil,
		"effectiveMaxOrderNotional":     nil,
		"entry":                         nil,
	}
}

// RealTradeRiskEvents 返回风控事件。
func (s *Service) RealTradeRiskEvents() map[string]any {
	return map[string]any{
		"realTradingEnabled":            false,
		"riskEnabled":                   false,
		"riskConfigSource":              nil,
		"envConfiguredMaxOrderQuantity": nil,
		"envConfiguredMaxOrderNotional": nil,
		"controlPlaneActive":            false,
		"controlPlaneMaxOrderQuantity":  nil,
		"controlPlaneMaxOrderNotional":  nil,
		"effectiveMaxOrderQuantity":     nil,
		"effectiveMaxOrderNotional":     nil,
		"maxOrderQuantity":              nil,
		"maxOrderNotional":              nil,
		"entries":                       []any{},
	}
}

// ── Futu/OpenD ──

// FutuOpenDHealth 返回 Futu/OpenD 健康信息（委托给注入的提供者）。
func (s *Service) FutuOpenDHealth(ctx context.Context) map[string]any {
	if s.futuOpenDHealthFn == nil {
		return map[string]any{"status": "unavailable", "reason": "futu integration not enabled"}
	}
	return s.futuOpenDHealthFn(ctx)
}

// FutuOpenDInstallGuide 返回 OpenD 安装指南。
func (s *Service) FutuOpenDInstallGuide() map[string]any {
	if s.futuOpenDInstallGuideFn == nil {
		return map[string]any{}
	}
	return s.futuOpenDInstallGuideFn()
}

// ResetFutuRuntime 重置 Futu 运行时状态。
func (s *Service) ResetFutuRuntime() {
	if s.resetFutuRuntimeFn != nil {
		s.resetFutuRuntimeFn()
	}
}

// RuntimeDependencies 返回运行时依赖检查结果。
func (s *Service) RuntimeDependencies(ctx context.Context) map[string]any {
	if s.runtimeDependenciesFn == nil {
		return map[string]any{
			"checkedAt":            time.Now().UTC().Format(time.RFC3339Nano),
			"allRequiredSatisfied": true,
			"dependencies":         []any{},
		}
	}
	return s.runtimeDependenciesFn(ctx)
}

// ── Broker 订单更新 Worker ──

// BrokerOrderUpdatesSnapshot 返回订单更新 Worker 快照。
func (s *Service) BrokerOrderUpdatesSnapshot() map[string]any {
	if s.brokerOrderSnapshot == nil {
		return map[string]any{}
	}
	return s.brokerOrderSnapshot()
}
