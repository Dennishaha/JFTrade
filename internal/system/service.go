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
	updateRiskConfigFn          func(context.Context, RealTradeRuntimeRiskCommand) (map[string]any, error)
	disableRiskConfigFn         func(context.Context, RealTradeRuntimeRiskCommand) (map[string]any, error)
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

func WithRealTradeRuntimeRiskControls(
	update func(context.Context, RealTradeRuntimeRiskCommand) (map[string]any, error),
	disable func(context.Context, RealTradeRuntimeRiskCommand) (map[string]any, error),
) Option {
	return func(s *Service) {
		s.updateRiskConfigFn = update
		s.disableRiskConfigFn = disable
	}
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

type RealTradeRuntimeRiskCommand struct {
	TradingEnvironment string   `json:"tradingEnvironment"`
	RealTradingEnabled bool     `json:"realTradingEnabled"`
	MaxOrderQuantity   *float64 `json:"maxOrderQuantity"`
	MaxOrderNotional   *float64 `json:"maxOrderNotional"`
	OperatorID         string   `json:"operatorId"`
	Reason             string   `json:"reason"`
}

// ── 系统状态 ──

// Status 返回系统整体状态摘要。
func (s *Service) Status() map[string]any {
	now := time.Now().UTC()
	apiPort := s.currentAPIPort()
	defaultTradingEnvironment := s.currentDefaultTradingEnvironment()
	broker := s.optionalBrokerDescriptor()
	strategyRuntime := s.optionalStrategyRuntimeSummary()
	live := s.optionalLiveStats()
	marketdata := s.optionalMarketdataRuntimeSummary()
	runtimeResources := s.currentRuntimeResources(now)
	exchangeCalendars := s.optionalExchangeCalendarStatus()
	requestObservability := s.optionalRequestObservability()
	realTrade := s.realTradeRiskState()
	status := map[string]any{
		"name":                      "JFTrade",
		"apiPort":                   apiPort,
		"defaultBroker":             "futu",
		"defaultTradingEnvironment": defaultTradingEnvironment,
		"realTradingEnabled":        boolValue(realTrade, "realTradingEnabled"),
		"realTradingKillSwitch": map[string]any{
			"active": boolValue(realTrade, "killSwitchActive"), "runtimeActive": boolValue(realTrade, "runtimeKillSwitchActive"),
			"blockedOperations": []string{"PLACE", "MODIFY"}, "allowsCancel": true,
		},
		"realTradingRisk": map[string]any{
			"enabled": boolValue(realTrade, "riskEnabled"), "maxOrderQuantity": realTrade["effectiveMaxOrderQuantity"], "maxOrderNotional": realTrade["effectiveMaxOrderNotional"],
			"runtimeConfiguredMaxOrderQuantity": realTrade["runtimeConfiguredMaxOrderQuantity"], "runtimeConfiguredMaxOrderNotional": realTrade["runtimeConfiguredMaxOrderNotional"],
			"runtimeRiskConfigured": boolValue(realTrade, "runtimeRiskConfigured"),
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

	attachOptionalSystemStatus(status, broker, strategyRuntime)
	return status
}

func (s *Service) currentAPIPort() int {
	apiPort := s.apiPort
	if s.apiPortFn != nil {
		apiPort = s.apiPortFn()
	}
	return apiPort
}

func (s *Service) currentDefaultTradingEnvironment() string {
	environment := s.defaultTradingEnvironment
	if s.defaultTradingEnvironmentFn != nil {
		environment = s.defaultTradingEnvironmentFn()
	}
	return environment
}

func (s *Service) optionalBrokerDescriptor() map[string]any {
	if s.brokerDescriptor == nil {
		return nil
	}
	return s.brokerDescriptor()
}

func (s *Service) optionalStrategyRuntimeSummary() map[string]any {
	if s.strategyRuntimeSummary == nil {
		return nil
	}
	return s.strategyRuntimeSummary()
}

func (s *Service) optionalLiveStats() map[string]any {
	if s.liveStats == nil {
		return nil
	}
	return s.liveStats()
}

func (s *Service) optionalMarketdataRuntimeSummary() map[string]any {
	if s.marketdataRuntimeSummary == nil {
		return nil
	}
	return s.marketdataRuntimeSummary()
}

func (s *Service) currentRuntimeResources(now time.Time) map[string]any {
	if s.runtimeResourcesFn == nil {
		return map[string]any{"checkedAt": now.Format(time.RFC3339Nano), "count": 0, "items": []any{}}
	}
	return s.runtimeResourcesFn()
}

func (s *Service) optionalExchangeCalendarStatus() map[string]any {
	if s.exchangeCalendarStatusFn == nil {
		return nil
	}
	return s.exchangeCalendarStatusFn()
}

func (s *Service) optionalRequestObservability() any {
	if s.requestObservabilityFn == nil {
		return nil
	}
	return s.requestObservabilityFn()
}

func attachOptionalSystemStatus(status map[string]any, broker map[string]any, strategyRuntime map[string]any) {
	if broker != nil {
		status["broker"] = broker
	}
	if strategyRuntime != nil {
		status["strategyRuntime"] = strategyRuntime
	}
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
	return map[string]any{
		"realTradingEnabled":        boolValue(realTrade, "realTradingEnabled"),
		"requiredConfirmationText":  "ENABLE_REAL_TRADING",
		"maxApprovalAgeMs":          5 * 60 * 1000,
		"approvalWorkflowAvailable": false,
		"approvalWorkflowStatus":    "not_configured",
		"approvalWorkflowMessage":   "real-trade approval workflow is not configured; runtime risk limits are enforced before broker submission.",
		"approvalPolicy": map[string]any{
			"approverAllowlistEnabled":  false,
			"approverCount":             0,
			"largeOrderNotional":        nil,
			"approvalWorkflowAvailable": false,
			"approvalMode":              "none",
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
		"realTradingEnabled": boolValue(realTrade, "realTradingEnabled"),
		"killSwitchActive":   boolValue(realTrade, "killSwitchActive"),
		"killSwitchSource":   realTrade["killSwitchSource"],
		"runtimeActive":      boolValue(realTrade, "runtimeKillSwitchActive"),
		"blockedOperations":  []string{"PLACE", "MODIFY"},
		"allowsCancel":       true,
		"entry":              realTrade["killSwitchEntry"],
	}
}

// RealTradeKillSwitchEvents 返回熔断事件。
func (s *Service) RealTradeKillSwitchEvents() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled": boolValue(realTrade, "realTradingEnabled"),
		"killSwitchActive":   boolValue(realTrade, "killSwitchActive"),
		"runtimeActive":      boolValue(realTrade, "runtimeKillSwitchActive"),
		"blockedOperations":  []string{"PLACE", "MODIFY"},
		"allowsCancel":       true,
		"entries":            anySliceValue(realTrade, "killSwitchEvents"),
	}
}

// RealTradeRiskLimits 返回风控限额。
func (s *Service) RealTradeRiskLimits() map[string]any {
	realTrade := s.realTradeRiskState()
	return map[string]any{
		"realTradingEnabled":                boolValue(realTrade, "realTradingEnabled"),
		"riskEnabled":                       boolValue(realTrade, "riskEnabled"),
		"runtimeRiskConfigured":             boolValue(realTrade, "runtimeRiskConfigured"),
		"runtimeConfiguredMaxOrderQuantity": realTrade["runtimeConfiguredMaxOrderQuantity"],
		"runtimeConfiguredMaxOrderNotional": realTrade["runtimeConfiguredMaxOrderNotional"],
		"effectiveMaxOrderQuantity":         realTrade["effectiveMaxOrderQuantity"],
		"effectiveMaxOrderNotional":         realTrade["effectiveMaxOrderNotional"],
		"entry":                             realTrade["riskEntry"],
	}
}

func (s *Service) UpdateRealTradeRuntimeRisk(ctx context.Context, command RealTradeRuntimeRiskCommand) (map[string]any, error) {
	if s.updateRiskConfigFn == nil {
		return nil, errRealTradeControlUnavailable
	}
	return s.updateRiskConfigFn(ctx, command)
}

func (s *Service) DisableRealTradeRuntimeRisk(ctx context.Context, command RealTradeRuntimeRiskCommand) (map[string]any, error) {
	if s.disableRiskConfigFn == nil {
		return nil, errRealTradeControlUnavailable
	}
	return s.disableRiskConfigFn(ctx, command)
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
		"realTradingEnabled":                boolValue(realTrade, "realTradingEnabled"),
		"riskEnabled":                       boolValue(realTrade, "riskEnabled"),
		"runtimeRiskConfigured":             boolValue(realTrade, "runtimeRiskConfigured"),
		"runtimeConfiguredMaxOrderQuantity": realTrade["runtimeConfiguredMaxOrderQuantity"],
		"runtimeConfiguredMaxOrderNotional": realTrade["runtimeConfiguredMaxOrderNotional"],
		"effectiveMaxOrderQuantity":         realTrade["effectiveMaxOrderQuantity"],
		"effectiveMaxOrderNotional":         realTrade["effectiveMaxOrderNotional"],
		"maxOrderQuantity":                  realTrade["effectiveMaxOrderQuantity"],
		"maxOrderNotional":                  realTrade["effectiveMaxOrderNotional"],
		"entries":                           anySliceValue(realTrade, "riskEvents"),
	}
}

func (s *Service) realTradeRiskState() map[string]any {
	if s.realTradeRiskStateFn == nil {
		return map[string]any{
			"realTradingEnabled":                false,
			"killSwitchActive":                  false,
			"killSwitchSource":                  nil,
			"runtimeKillSwitchActive":           false,
			"riskEnabled":                       false,
			"runtimeRiskConfigured":             false,
			"runtimeConfiguredMaxOrderQuantity": nil,
			"runtimeConfiguredMaxOrderNotional": nil,
			"effectiveMaxOrderQuantity":         nil,
			"effectiveMaxOrderNotional":         nil,
			"riskEntry":                         nil,
			"riskEvents":                        []any{},
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
	switch value.(type) {
	case []any, []string:
		return value
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice {
		return value
	}
	if reflected.IsNil() {
		return []any{}
	}
	items := make([]any, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		items[index] = reflected.Index(index).Interface()
	}
	return items
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
