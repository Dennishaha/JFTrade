package system

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jftrade/jftrade-main/internal/buildinfo"
)

var errRealTradeControlUnavailable = errors.New("real-trade control plane is not configured")

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
	runtimeResourcesFn          func() map[string]any
	brokerOrderSnapshot         func() map[string]any
	exchangeCalendarStatusFn    func() map[string]any
	exchangeCalendarSourcesFn   func() []map[string]any
	refreshExchangeCalendarsFn  func(ctx context.Context, market string) map[string]any
	probeExchangeCalendarsFn    func(ctx context.Context, market string) map[string]any
	futuOpenDHealthFn           func(ctx context.Context) map[string]any
	futuOpenDInstallGuideFn     func() map[string]any
	resetFutuRuntimeFn          func()
	runtimeDependenciesFn       func(ctx context.Context) map[string]any
	requestObservabilityFn      func() any
	realTradeRiskStateFn        func() map[string]any
	activateKillSwitchFn        func(context.Context, RealTradeKillSwitchCommand) (map[string]any, error)
	releaseKillSwitchFn         func(context.Context, RealTradeKillSwitchCommand) (map[string]any, error)
	activateHardStopFn          func(context.Context, RealTradeHardStopCommand) (map[string]any, error)
	releaseHardStopFn           func(context.Context, string, RealTradeHardStopCommand) (map[string]any, error)
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

// WithRuntimeResources 设置运行时资源 owner 清单提供者。
func WithRuntimeResources(fn func() map[string]any) Option {
	return func(s *Service) { s.runtimeResourcesFn = fn }
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

// WithRequestObservability sets the bounded request and dependency summary provider.
func WithRequestObservability(fn func() any) Option {
	return func(s *Service) { s.requestObservabilityFn = fn }
}

// WithRealTradeRiskState sets the shared real-trade risk/kill-switch state provider.
func WithRealTradeRiskState(fn func() map[string]any) Option {
	return func(s *Service) { s.realTradeRiskStateFn = fn }
}

func WithRealTradeKillSwitchControls(
	activate func(context.Context, RealTradeKillSwitchCommand) (map[string]any, error),
	release func(context.Context, RealTradeKillSwitchCommand) (map[string]any, error),
) Option {
	return func(s *Service) {
		s.activateKillSwitchFn = activate
		s.releaseKillSwitchFn = release
	}
}

func WithRealTradeHardStopControls(
	activate func(context.Context, RealTradeHardStopCommand) (map[string]any, error),
	release func(context.Context, string, RealTradeHardStopCommand) (map[string]any, error),
) Option {
	return func(s *Service) {
		s.activateHardStopFn = activate
		s.releaseHardStopFn = release
	}
}

type RealTradeKillSwitchCommand struct {
	TradingEnvironment string `json:"tradingEnvironment"`
	OperatorID         string `json:"operatorId"`
	Reason             string `json:"reason"`
}

type RealTradeHardStopCommand struct {
	BrokerID           string `json:"brokerId"`
	TradingEnvironment string `json:"tradingEnvironment"`
	AccountID          string `json:"accountId"`
	Market             string `json:"market"`
	Symbol             string `json:"symbol"`
	HardStopScope      string `json:"hardStopScope"`
	OperatorID         string `json:"operatorId"`
	Reason             string `json:"reason"`
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
	runtimeResources := map[string]any{"checkedAt": now.Format(time.RFC3339Nano), "count": 0, "items": []any{}}
	if s.runtimeResourcesFn != nil {
		runtimeResources = s.runtimeResourcesFn()
	}
	exchangeCalendars := map[string]any(nil)
	if s.exchangeCalendarStatusFn != nil {
		exchangeCalendars = s.exchangeCalendarStatusFn()
	}
	requestObservability := any(nil)
	if s.requestObservabilityFn != nil {
		requestObservability = s.requestObservabilityFn()
	}
	realTrade := s.realTradeRiskState()
	status := map[string]any{
		"name":                      "JFTrade",
		"apiPort":                   apiPort,
		"defaultBroker":             "futu",
		"defaultTradingEnvironment": defaultTradingEnvironment,
		"realTradingEnabled":        boolValue(realTrade, "realTradingEnabled"),
		"realTradingKillSwitch": map[string]any{
			"active": boolValue(realTrade, "killSwitchActive"), "envConfiguredActive": boolValue(realTrade, "envConfiguredActive"), "controlPlaneActive": boolValue(realTrade, "killSwitchControlPlaneActive"),
			"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true,
		},
		"realTradingRisk": map[string]any{
			"enabled": boolValue(realTrade, "riskEnabled"), "maxOrderQuantity": realTrade["effectiveMaxOrderQuantity"], "maxOrderNotional": realTrade["effectiveMaxOrderNotional"],
			"envConfiguredMaxOrderQuantity": realTrade["envConfiguredMaxOrderQuantity"], "envConfiguredMaxOrderNotional": realTrade["envConfiguredMaxOrderNotional"],
			"controlPlaneActive": boolValue(realTrade, "riskControlPlaneActive"), "controlPlaneMaxOrderQuantity": realTrade["controlPlaneMaxOrderQuantity"], "controlPlaneMaxOrderNotional": realTrade["controlPlaneMaxOrderNotional"],
			"riskConfigSource": realTrade["riskConfigSource"],
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
			"requests":          requestObservability,
		},
		"runtimeResources": runtimeResources,
		"message":          "JFTrade API adapter is running.",
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
	realTrade := s.realTradeRiskState()
	approvalThreshold := realTrade["approvalRequiredNotional"]
	approvalThresholdConfigured := approvalThreshold != nil
	if threshold, ok := approvalThreshold.(*float64); ok {
		approvalThresholdConfigured = threshold != nil
	}
	workflowStatus := "not_configured"
	workflowMessage := "real-trade approval workflow is not configured; no approval entries are recorded."
	approvalMode := "none"
	if approvalThresholdConfigured {
		workflowStatus = "not_implemented"
		workflowMessage = "large-order real-trade approval threshold is configured, but approval creation/approval consumption is not implemented yet; matching orders are blocked before broker submission."
		approvalMode = "blocking_threshold_without_workflow"
	}
	return map[string]any{
		"realTradingEnabled":        boolValue(realTrade, "realTradingEnabled"),
		"requiredConfirmationText":  "ENABLE_REAL_TRADING",
		"maxApprovalAgeMs":          5 * 60 * 1000,
		"approvalWorkflowAvailable": false,
		"approvalWorkflowStatus":    workflowStatus,
		"approvalWorkflowMessage":   workflowMessage,
		"approvalPolicy": map[string]any{
			"approverAllowlistEnabled":  false,
			"approverCount":             0,
			"largeOrderNotional":        approvalThreshold,
			"approvalWorkflowAvailable": false,
			"approvalMode":              approvalMode,
		},
		"entries": []any{},
	}
}

// RealTradeHardStops 返回硬止损状态。
func (s *Service) RealTradeHardStops() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"blockedOperations": []string{"PLACE", "MODIFY"},
		"allowsCancel":      true,
		"entries":           anySliceValue(realTrade, "hardStopEntries"),
	}
}

// RealTradeHardStopEvents 返回硬止损事件。
func (s *Service) RealTradeHardStopEvents() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled": boolValue(realTrade, "realTradingEnabled"),
		"blockedOperations":  []string{"PLACE", "MODIFY"},
		"allowsCancel":       true,
		"entries":            anySliceValue(realTrade, "hardStopEvents"),
	}
}

// RealTradeKillSwitch 返回熔断状态。
func (s *Service) RealTradeKillSwitch() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled":  boolValue(realTrade, "realTradingEnabled"),
		"killSwitchActive":    boolValue(realTrade, "killSwitchActive"),
		"killSwitchSource":    realTrade["killSwitchSource"],
		"envConfiguredActive": boolValue(realTrade, "envConfiguredActive"),
		"controlPlaneActive":  boolValue(realTrade, "killSwitchControlPlaneActive"),
		"blockedOperations":   []string{"PLACE", "MODIFY"},
		"allowsCancel":        true,
		"entry":               realTrade["killSwitchEntry"],
	}
}

// RealTradeKillSwitchEvents 返回熔断事件。
func (s *Service) RealTradeKillSwitchEvents() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled":  boolValue(realTrade, "realTradingEnabled"),
		"killSwitchActive":    boolValue(realTrade, "killSwitchActive"),
		"envConfiguredActive": boolValue(realTrade, "envConfiguredActive"),
		"controlPlaneActive":  boolValue(realTrade, "killSwitchControlPlaneActive"),
		"blockedOperations":   []string{"PLACE", "MODIFY"},
		"allowsCancel":        true,
		"entries":             anySliceValue(realTrade, "killSwitchEvents"),
	}
}

// RealTradeRiskLimits 返回风控限额。
func (s *Service) RealTradeRiskLimits() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled":            boolValue(realTrade, "realTradingEnabled"),
		"riskEnabled":                   boolValue(realTrade, "riskEnabled"),
		"riskConfigSource":              realTrade["riskConfigSource"],
		"envConfiguredMaxOrderQuantity": realTrade["envConfiguredMaxOrderQuantity"],
		"envConfiguredMaxOrderNotional": realTrade["envConfiguredMaxOrderNotional"],
		"controlPlaneActive":            boolValue(realTrade, "riskControlPlaneActive"),
		"controlPlaneMaxOrderQuantity":  realTrade["controlPlaneMaxOrderQuantity"],
		"controlPlaneMaxOrderNotional":  realTrade["controlPlaneMaxOrderNotional"],
		"effectiveMaxOrderQuantity":     realTrade["effectiveMaxOrderQuantity"],
		"effectiveMaxOrderNotional":     realTrade["effectiveMaxOrderNotional"],
		"entry":                         nil,
	}
}

func (s *Service) ActivateRealTradeKillSwitch(ctx context.Context, command RealTradeKillSwitchCommand) (map[string]any, error) {
	if s.activateKillSwitchFn == nil {
		return nil, errRealTradeControlUnavailable
	}
	return s.activateKillSwitchFn(ctx, command)
}

func (s *Service) ReleaseRealTradeKillSwitch(ctx context.Context, command RealTradeKillSwitchCommand) (map[string]any, error) {
	if s.releaseKillSwitchFn == nil {
		return nil, errRealTradeControlUnavailable
	}
	return s.releaseKillSwitchFn(ctx, command)
}

func (s *Service) ActivateRealTradeHardStop(ctx context.Context, command RealTradeHardStopCommand) (map[string]any, error) {
	if s.activateHardStopFn == nil {
		return nil, errRealTradeControlUnavailable
	}
	return s.activateHardStopFn(ctx, command)
}

func (s *Service) ReleaseRealTradeHardStop(ctx context.Context, id string, command RealTradeHardStopCommand) (map[string]any, error) {
	if s.releaseHardStopFn == nil {
		return nil, errRealTradeControlUnavailable
	}
	return s.releaseHardStopFn(ctx, id, command)
}

// RealTradeRiskEvents 返回风控事件。
func (s *Service) RealTradeRiskEvents() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled":            boolValue(realTrade, "realTradingEnabled"),
		"riskEnabled":                   boolValue(realTrade, "riskEnabled"),
		"riskConfigSource":              realTrade["riskConfigSource"],
		"envConfiguredMaxOrderQuantity": realTrade["envConfiguredMaxOrderQuantity"],
		"envConfiguredMaxOrderNotional": realTrade["envConfiguredMaxOrderNotional"],
		"controlPlaneActive":            boolValue(realTrade, "riskControlPlaneActive"),
		"controlPlaneMaxOrderQuantity":  realTrade["controlPlaneMaxOrderQuantity"],
		"controlPlaneMaxOrderNotional":  realTrade["controlPlaneMaxOrderNotional"],
		"effectiveMaxOrderQuantity":     realTrade["effectiveMaxOrderQuantity"],
		"effectiveMaxOrderNotional":     realTrade["effectiveMaxOrderNotional"],
		"maxOrderQuantity":              realTrade["effectiveMaxOrderQuantity"],
		"maxOrderNotional":              realTrade["effectiveMaxOrderNotional"],
		"entries":                       []any{},
	}
}

func (s *Service) realTradeRiskState() map[string]any {
	if s.realTradeRiskStateFn == nil {
		return map[string]any{
			"realTradingEnabled":            false,
			"killSwitchActive":              false,
			"killSwitchSource":              nil,
			"envConfiguredActive":           false,
			"controlPlaneActive":            false,
			"killSwitchControlPlaneActive":  false,
			"riskEnabled":                   false,
			"riskConfigSource":              nil,
			"envConfiguredMaxOrderQuantity": nil,
			"envConfiguredMaxOrderNotional": nil,
			"approvalRequiredNotional":      nil,
			"riskControlPlaneActive":        false,
			"controlPlaneMaxOrderQuantity":  nil,
			"controlPlaneMaxOrderNotional":  nil,
			"effectiveMaxOrderQuantity":     nil,
			"effectiveMaxOrderNotional":     nil,
		}
	}
	state := s.realTradeRiskStateFn()
	if state == nil {
		return map[string]any{}
	}
	return state
}

func boolValue(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok {
		return false
	}
	result, ok := value.(bool)
	return ok && result
}

func anySliceValue(values map[string]any, key string) any {
	value, ok := values[key]
	if !ok || value == nil {
		return []any{}
	}
	if reflected := reflect.ValueOf(value); reflected.Kind() == reflect.Slice && reflected.IsNil() {
		return []any{}
	}
	return value
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
