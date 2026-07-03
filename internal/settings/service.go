package settings

import (
	"context"
	"errors"
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

var (
	ErrDatabaseMaintenanceConflict = errors.New("database maintenance conflict")
	ErrCleanupPreviewNotFound      = errors.New("cleanup preview not found or expired")
	ErrCleanupPreviewStale         = errors.New("cleanup preview is stale")
)

type DataCleanupPreviewRequest struct {
	Kind          string `json:"kind"`
	DatabaseID    string `json:"databaseId"`
	OlderThanDays int    `json:"olderThanDays,omitempty"`
	KeepLatest    int    `json:"keepLatest,omitempty"`
}

type DataCleanupExecuteRequest struct {
	PreviewID    string `json:"previewId"`
	Confirmation string `json:"confirmation"`
}

type DatabaseCompactRequest struct {
	Confirmation string `json:"confirmation"`
}

type DatabaseRebuildRequest struct {
	DatabaseIDs  []string `json:"databaseIds"`
	DatabaseID   string   `json:"databaseId"`
	Mode         string   `json:"mode"`
	Confirmation string   `json:"confirmation"`
}

type DataManagementOverviewRequest struct {
	SummaryOnly bool   `json:"summaryOnly"`
	DatabaseID  string `json:"databaseId"`
}

// Store 是 settings 持久化层接口，由应用装配层注入具体实现。
type Store interface {
	// 读方法 — 返回已规范化的值（含默认值）
	Appearance() jfsettings.UIAppearanceSettings
	Onboarding() jfsettings.OnboardingSettings
	ExecutionSettings() jfsettings.ExecutionSettings
	SecuritySettings() jfsettings.SecuritySettings
	ADKSettings() jfsettings.ADKRuntimeSettings
	PineWorkerSettings() jfsettings.PineWorkerSettings
	ExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings
	Integration() jfsettings.BrokerIntegration
	SavedIntegration() *jfsettings.BrokerIntegration
	ManagedAccounts() []jfsettings.ManagedBrokerAccount
	InterfaceSettings(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings

	// 写方法 — 持久化并返回规范化结果
	SaveAppearance(jfsettings.UIAppearanceSettings) (jfsettings.UIAppearanceSettings, error)
	SaveOnboarding(jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error)
	SaveExecutionSettings(jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error)
	SaveSecuritySettings(jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error)
	SaveADKSettings(jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error)
	SavePineWorkerSettings(jfsettings.PineWorkerSettings) (jfsettings.PineWorkerSettings, error)
	SaveExchangeCalendarSettings(jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error)
	SaveIntegration(jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error)
	CreateManagedAccount(jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error)
	UpdateManagedAccount(id string, input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error)
	DeleteManagedAccount(id string) error

	// 生命周期
	EnsureBootstrapFile(defaults jfsettings.LaunchDefaults) error
	HasAppearance() bool
	Path() string
}

// SideEffects 定义 settings 写操作触发的跨模块回调。
// 由 Server 实现并注入，避免 settings 包直接依赖 Futu/execution/frontend。
type SideEffects struct {
	// OnIntegrationChanged 在 broker 集成配置变更时调用（→ 更新环境变量）。
	OnIntegrationChanged func(jfsettings.BrokerIntegration)
	// OnExecutionChanged 在执行设置变更时调用（→ 更新订单保留策略）。
	OnExecutionChanged func(jfsettings.ExecutionSettings)
	// OnSecurityChanged 在安全设置变更时调用（→ 更新 auth/frontend）。
	OnSecurityChanged func(jfsettings.SecuritySettings)
	// OnExchangeCalendarsChanged 在交易所日历设置变更时调用（→ 刷新 manager 配置）。
	OnExchangeCalendarsChanged func(jfsettings.ExchangeCalendarSettings)
	// OnPineWorkerChanged 在 PineTS worker 设置变更时调用。
	OnPineWorkerChanged func(jfsettings.PineWorkerSettings)
}

// Service 提供 settings 业务逻辑：读取、持久化、副作用编排。
type Service struct {
	store       Store
	sideEffects SideEffects

	// 来自 Server 的委托（不在 Store 中的聚合信息）
	brokerDescriptor       func() map[string]any
	brokerSettingsFn       func() map[string]any
	onboardingStateFn      func(ctx context.Context) map[string]any
	defaultTradingEnv      string
	dataMigrationStatusFn  func(context.Context, DataManagementOverviewRequest) (any, error)
	dataMigrationRebuildFn func(context.Context, any) (any, error)
	dataCleanupPreviewFn   func(context.Context, DataCleanupPreviewRequest) (any, error)
	dataCleanupExecuteFn   func(context.Context, DataCleanupExecuteRequest) (any, error)
	databaseCompactFn      func(context.Context, string, DatabaseCompactRequest) (any, error)
	databaseRebuildFn      func(context.Context, DatabaseRebuildRequest) (any, error)
}

// NewService 创建 settings 服务。
func NewService(store Store, opts ...Option) *Service {
	s := &Service{store: store}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option 函数式选项。
type Option func(*Service)

// WithSideEffects 设置副作用回调。
func WithSideEffects(se SideEffects) Option {
	return func(s *Service) { s.sideEffects = se }
}

// WithBrokerDescriptor 设置 broker 描述符提供者。
func WithBrokerDescriptor(fn func() map[string]any) Option {
	return func(s *Service) { s.brokerDescriptor = fn }
}

// WithBrokerSettings 设置 broker 设置聚合提供者。
func WithBrokerSettings(fn func() map[string]any) Option {
	return func(s *Service) { s.brokerSettingsFn = fn }
}

// WithOnboardingState 设置新手引导状态提供者。
func WithOnboardingState(fn func(ctx context.Context) map[string]any) Option {
	return func(s *Service) { s.onboardingStateFn = fn }
}

// WithDefaultTradingEnvironment 设置默认交易环境。
func WithDefaultTradingEnvironment(env string) Option {
	return func(s *Service) { s.defaultTradingEnv = env }
}

func WithDataMigration(
	status func(context.Context) (any, error),
	rebuild func(context.Context, any) (any, error),
) Option {
	return func(s *Service) {
		s.dataMigrationStatusFn = func(ctx context.Context, _ DataManagementOverviewRequest) (any, error) {
			return status(ctx)
		}
		s.dataMigrationRebuildFn = rebuild
	}
}

func WithDataManagement(
	overview func(context.Context, DataManagementOverviewRequest) (any, error),
	preview func(context.Context, DataCleanupPreviewRequest) (any, error),
	execute func(context.Context, DataCleanupExecuteRequest) (any, error),
	compact func(context.Context, string, DatabaseCompactRequest) (any, error),
	rebuild func(context.Context, DatabaseRebuildRequest) (any, error),
) Option {
	return func(s *Service) {
		s.dataMigrationStatusFn = overview
		s.dataCleanupPreviewFn = preview
		s.dataCleanupExecuteFn = execute
		s.databaseCompactFn = compact
		s.databaseRebuildFn = rebuild
	}
}

func (s *Service) DataMigrationStatus(ctx context.Context) (any, error) {
	return s.DataManagementStatus(ctx, DataManagementOverviewRequest{})
}

func (s *Service) DataManagementStatus(ctx context.Context, request DataManagementOverviewRequest) (any, error) {
	if s.dataMigrationStatusFn == nil {
		return map[string]any{"databases": []any{}}, nil
	}
	return s.dataMigrationStatusFn(ctx, request)
}

func (s *Service) ScheduleDatabaseRebuild(ctx context.Context, request any) (any, error) {
	if s.dataMigrationRebuildFn == nil {
		return nil, errors.New("database rebuild is unavailable")
	}
	return s.dataMigrationRebuildFn(ctx, request)
}

func (s *Service) PreviewDataCleanup(ctx context.Context, request DataCleanupPreviewRequest) (any, error) {
	if s.dataCleanupPreviewFn == nil {
		return nil, errors.New("database cleanup preview is unavailable")
	}
	return s.dataCleanupPreviewFn(ctx, request)
}

func (s *Service) ExecuteDataCleanup(ctx context.Context, request DataCleanupExecuteRequest) (any, error) {
	if s.dataCleanupExecuteFn == nil {
		return nil, errors.New("database cleanup is unavailable")
	}
	return s.dataCleanupExecuteFn(ctx, request)
}

func (s *Service) CompactDatabase(ctx context.Context, databaseID string, request DatabaseCompactRequest) (any, error) {
	if s.databaseCompactFn == nil {
		return nil, errors.New("database compaction is unavailable")
	}
	return s.databaseCompactFn(ctx, databaseID, request)
}

func (s *Service) RebuildDatabase(ctx context.Context, request DatabaseRebuildRequest) (any, error) {
	if s.databaseRebuildFn != nil {
		return s.databaseRebuildFn(ctx, request)
	}
	return s.ScheduleDatabaseRebuild(ctx, map[string]any{
		"databaseIds":  request.DatabaseIDs,
		"databaseId":   request.DatabaseID,
		"mode":         request.Mode,
		"confirmation": request.Confirmation,
	})
}

// ── UI Appearance ──

// GetAppearance 返回 UI 外观设置。
func (s *Service) GetAppearance() jfsettings.UIAppearanceSettings {
	return s.store.Appearance()
}

// SaveAppearance 保存 UI 外观设置。
func (s *Service) SaveAppearance(input jfsettings.UIAppearanceSettings) (jfsettings.UIAppearanceSettings, error) {
	return s.store.SaveAppearance(input)
}

// ── Onboarding ──

// GetOnboarding 返回新手引导设置。
func (s *Service) GetOnboarding() jfsettings.OnboardingSettings {
	return s.store.Onboarding()
}

// SaveOnboarding 保存新手引导设置。
func (s *Service) SaveOnboarding(input jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error) {
	return s.store.SaveOnboarding(input)
}

// OnboardingState 返回前端用的新手引导上下文。
func (s *Service) OnboardingState(ctx context.Context) map[string]any {
	if s.onboardingStateFn == nil {
		return map[string]any{}
	}
	return s.onboardingStateFn(ctx)
}

// ── Execution ──

// GetExecutionSettings 返回执行设置。
func (s *Service) GetExecutionSettings() jfsettings.ExecutionSettings {
	return s.store.ExecutionSettings()
}

// SaveExecutionSettings 保存执行设置并触发副作用。
func (s *Service) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	result, err := s.store.SaveExecutionSettings(input)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnExecutionChanged != nil {
		s.sideEffects.OnExecutionChanged(result)
	}
	return result, nil
}

// ── Security ──

// GetSecuritySettings 返回安全设置。
func (s *Service) GetSecuritySettings() jfsettings.SecuritySettings {
	return s.store.SecuritySettings()
}

// SaveSecuritySettings 保存安全设置并触发副作用。
func (s *Service) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	result, err := s.store.SaveSecuritySettings(input)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnSecurityChanged != nil {
		s.sideEffects.OnSecurityChanged(result)
	}
	return result, nil
}

// ── ADK ──

// GetADKRuntimeSettings 返回 ADK 运行时设置。
func (s *Service) GetADKRuntimeSettings() jfsettings.ADKRuntimeSettings {
	return s.store.ADKSettings()
}

// SaveADKRuntimeSettings 保存 ADK 运行时设置。
func (s *Service) SaveADKRuntimeSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	return s.store.SaveADKSettings(input)
}

// ── Pine Worker ──

// GetPineWorkerSettings 返回 PineTS worker 设置。
func (s *Service) GetPineWorkerSettings() jfsettings.PineWorkerSettings {
	return s.store.PineWorkerSettings()
}

// SavePineWorkerSettings 保存 PineTS worker 设置并触发副作用。
func (s *Service) SavePineWorkerSettings(input jfsettings.PineWorkerSettings) (jfsettings.PineWorkerSettings, error) {
	result, err := s.store.SavePineWorkerSettings(input)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnPineWorkerChanged != nil {
		s.sideEffects.OnPineWorkerChanged(result)
	}
	return result, nil
}

// ── Exchange Calendars ──

// GetExchangeCalendarSettings 返回交易所日历设置。
func (s *Service) GetExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings {
	return s.store.ExchangeCalendarSettings()
}

// SaveExchangeCalendarSettings 保存交易所日历设置并触发副作用。
func (s *Service) SaveExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error) {
	result, err := s.store.SaveExchangeCalendarSettings(input)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnExchangeCalendarsChanged != nil {
		s.sideEffects.OnExchangeCalendarsChanged(result)
	}
	return result, nil
}

// ── Broker Integration ──

// GetIntegration 返回 broker 集成配置（含默认值）。
func (s *Service) GetIntegration() jfsettings.BrokerIntegration {
	return s.store.Integration()
}

// GetSavedIntegration 返回已持久化的 broker 集成配置（nil 表示未存储）。
func (s *Service) GetSavedIntegration() *jfsettings.BrokerIntegration {
	return s.store.SavedIntegration()
}

// SaveIntegration 保存 broker 集成配置并触发副作用。
func (s *Service) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	result, err := s.store.SaveIntegration(input)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnIntegrationChanged != nil {
		s.sideEffects.OnIntegrationChanged(result)
	}
	return result, nil
}

// BrokerSettings 返回聚合的 broker 设置（前端 /settings/brokers 用）。
func (s *Service) BrokerSettings() map[string]any {
	if s.brokerSettingsFn == nil {
		return map[string]any{}
	}
	return s.brokerSettingsFn()
}

// ── Managed Accounts ──

// ListManagedAccounts 返回所有托管券商账户。
func (s *Service) ListManagedAccounts() []jfsettings.ManagedBrokerAccount {
	return s.store.ManagedAccounts()
}

// CreateManagedAccount 创建托管账户。
func (s *Service) CreateManagedAccount(input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	if strings.TrimSpace(input.AccountID) == "" {
		return jfsettings.ManagedBrokerAccount{}, BadRequestError("accountId is required")
	}
	input.ID = ""
	input.CreatedAt = ""
	input.UpdatedAt = ""
	return s.store.CreateManagedAccount(input)
}

// UpdateManagedAccount 更新托管账户。
func (s *Service) UpdateManagedAccount(id string, input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	return s.store.UpdateManagedAccount(id, input)
}

// DeleteManagedAccount 删除托管账户。
func (s *Service) DeleteManagedAccount(id string) error {
	return s.store.DeleteManagedAccount(id)
}

// ── Lifecycle ──

// EnsureBootstrap 确保 settings 文件存在（含默认值）。
func (s *Service) EnsureBootstrap(defaults jfsettings.LaunchDefaults) error {
	return s.store.EnsureBootstrapFile(defaults)
}

// HasAppearance 返回是否已设置外观。
func (s *Service) HasAppearance() bool {
	return s.store.HasAppearance()
}
