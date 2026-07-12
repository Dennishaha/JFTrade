package settings

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	minWebAccessPasswordChars = 15
	maxWebAccessPasswordBytes = 1024
	mcpServerTokenBytes       = 32
)

var (
	ErrWebAccessPasswordRequired = errors.New("a Web access password is required before Web access can be enabled")
	ErrWebAccessPasswordTooShort = errors.New("web access password must contain at least 15 characters")
	ErrWebAccessPasswordTooLong  = errors.New("web access password must contain at most 1024 bytes")
	ErrWebAccessPortInvalid      = errors.New("web access port must be between 1024 and 65535")
	ErrWebAccessRuntimeUpdate    = errors.New("could not apply Web access listener settings")
	ErrMCPServerPortInvalid      = errors.New("MCP server port must be between 1024 and 65535")
	ErrMCPServerAuthModeInvalid  = errors.New("MCP server auth mode must be token or none")
	ErrMCPServerTokenRequired    = errors.New("an MCP server token is required before token authentication can be enabled")
	ErrMCPServerRuntimeUpdate    = errors.New("could not apply MCP server listener settings")
	ErrMCPServerStoreUnavailable = errors.New("MCP server settings store is unavailable")
)

// Store 是 settings 持久化层接口，由应用装配层注入具体实现。
type Store interface {
	// 读方法 — 返回已规范化的值（含默认值）
	Appearance() jfsettings.UIAppearanceSettings
	Onboarding() jfsettings.OnboardingSettings
	ExecutionSettings() jfsettings.ExecutionSettings
	SecuritySettings() jfsettings.SecuritySettings
	SystemNotificationSettings() jfsettings.SystemNotificationSettings
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
	SaveSystemNotificationSettings(jfsettings.SystemNotificationSettings) (jfsettings.SystemNotificationSettings, error)
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

// MCPServerStore is an optional extension to Store. Keeping MCP persistence
// outside the long-standing settings contract preserves compatibility for
// existing settings stores and services that do not manage the local MCP
// listener.
type MCPServerStore interface {
	MCPServerSettings() jfsettings.MCPServerSettings
	SaveMCPServerSettings(jfsettings.MCPServerSettings) (jfsettings.MCPServerSettings, error)
}

// SideEffects 定义 settings 写操作触发的跨模块回调。
// 由 Server 实现并注入，避免 settings 包直接依赖 Futu/execution/frontend。
type SideEffects struct {
	// OnIntegrationChanged 在 broker 集成配置变更时调用（→ 更新环境变量）。
	OnIntegrationChanged func(jfsettings.BrokerIntegration)
	// OnExecutionChanged 在执行设置变更时调用（→ 更新订单保留策略）。
	OnExecutionChanged func(jfsettings.ExecutionSettings)
	// OnSecurityChanged 在安全设置变更时调用（→ 更新 auth/frontend）。
	OnSecurityChanged func(jfsettings.SecuritySettings) error
	// OnExchangeCalendarsChanged 在交易所日历设置变更时调用（→ 刷新 manager 配置）。
	OnExchangeCalendarsChanged func(jfsettings.ExchangeCalendarSettings)
	// OnPineWorkerChanged 在 PineTS worker 设置变更时调用。
	OnPineWorkerChanged func(jfsettings.PineWorkerSettings)
	// OnMCPServerChanged 在本机 MCP listener 设置变更时调用。
	OnMCPServerChanged func(jfsettings.MCPServerSettings) error
}

// Service 提供 settings 业务逻辑：读取、持久化、副作用编排。
type Service struct {
	store        Store
	mcpStore     MCPServerStore
	sideEffects  SideEffects
	securityMu   sync.Mutex
	mcpServerMu  sync.Mutex
	hashPassword func(string) (string, error)
	newMCPToken  func() (string, error)
	mcpStatus    func() jfsettings.MCPServerStatus

	// 来自 Server 的委托（不在 Store 中的聚合信息）
	brokerDescriptor  func() map[string]any
	brokerSettingsFn  func() map[string]any
	onboardingStateFn func(ctx context.Context) map[string]any
	defaultTradingEnv string
}

// NewService 创建 settings 服务。
func NewService(store Store, opts ...Option) *Service {
	mcpStore, _ := store.(MCPServerStore)
	s := &Service{store: store, mcpStore: mcpStore, hashPassword: passwordhash.Hash, newMCPToken: newMCPServerToken}
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

// WithMCPServerStatus supplies listener state owned by application assembly.
func WithMCPServerStatus(fn func() jfsettings.MCPServerStatus) Option {
	return func(s *Service) { s.mcpStatus = fn }
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

// SaveSecuritySettings stores a one-way password hash and triggers runtime
// access-policy updates. The plaintext password never reaches the Store.
func (s *Service) SaveSecuritySettings(input jfsettings.SecuritySettingsUpdate) (jfsettings.SecuritySettings, error) {
	s.securityMu.Lock()
	defer s.securityMu.Unlock()

	current := s.store.SecuritySettings()
	webPort := input.WebPort
	if webPort == 0 {
		webPort = current.WebPort
	}
	if webPort == 0 {
		webPort = jfsettings.DefaultWebAccessPort
	}
	next := jfsettings.SecuritySettings{
		WebAccessEnabled:    input.WebAccessEnabled,
		PublicAccessEnabled: input.PublicAccessEnabled,
		WebPort:             webPort,
		PasswordHash:        current.PasswordHash,
	}
	if next.WebPort < jfsettings.MinWebAccessPort || next.WebPort > jfsettings.MaxWebAccessPort {
		return current, ErrWebAccessPortInvalid
	}
	if input.NewPassword != "" {
		if err := validateWebAccessPassword(input.NewPassword); err != nil {
			return current, err
		}
		hashPassword := s.hashPassword
		if hashPassword == nil {
			hashPassword = passwordhash.Hash
		}
		hash, err := hashPassword(input.NewPassword)
		if err != nil {
			return current, err
		}
		next.PasswordHash = hash
	}
	if next.WebAccessEnabled && next.PasswordHash == "" {
		return current, ErrWebAccessPasswordRequired
	}
	next.PasswordConfigured = next.PasswordHash != ""
	if !next.WebAccessEnabled {
		next.PublicAccessEnabled = false
	}

	result, err := s.store.SaveSecuritySettings(next)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnSecurityChanged != nil {
		if err := s.sideEffects.OnSecurityChanged(result); err != nil {
			if _, rollbackErr := s.store.SaveSecuritySettings(current); rollbackErr != nil {
				return current, fmt.Errorf("%w: %v; settings rollback failed: %v", ErrWebAccessRuntimeUpdate, err, rollbackErr)
			}
			return current, fmt.Errorf("%w: %v", ErrWebAccessRuntimeUpdate, err)
		}
	}
	return result, nil
}

func validateWebAccessPassword(password string) error {
	if strings.TrimSpace(password) == "" || utf8.RuneCountInString(password) < minWebAccessPasswordChars {
		return ErrWebAccessPasswordTooShort
	}
	if len([]byte(password)) > maxWebAccessPasswordBytes {
		return ErrWebAccessPasswordTooLong
	}
	return nil
}

// ── System Notifications ──

// GetSystemNotificationSettings 返回系统通知设置。
func (s *Service) GetSystemNotificationSettings() jfsettings.SystemNotificationSettings {
	return s.store.SystemNotificationSettings()
}

// SaveSystemNotificationSettings 保存系统通知设置。
func (s *Service) SaveSystemNotificationSettings(input jfsettings.SystemNotificationSettings) (jfsettings.SystemNotificationSettings, error) {
	return s.store.SaveSystemNotificationSettings(input)
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

// ── Local MCP Server ──

// GetMCPServerSettings returns the public local MCP server settings. The
// stored token verifier remains private through its json:"-" tag.
func (s *Service) GetMCPServerSettings() jfsettings.MCPServerSettings {
	if s == nil || s.mcpStore == nil {
		return jfsettings.MCPServerSettings{
			Port:     jfsettings.DefaultMCPServerPort,
			AuthMode: "token",
		}
	}
	return s.mcpStore.MCPServerSettings()
}

// GetMCPServerSettingsSnapshot returns persisted settings and current listener
// status for the settings UI.
func (s *Service) GetMCPServerSettingsSnapshot() jfsettings.MCPServerSettingsSnapshot {
	settings := s.GetMCPServerSettings()
	status := jfsettings.MCPServerStatus{
		Endpoint: fmt.Sprintf("http://127.0.0.1:%d/mcp", settings.Port),
	}
	if s.mcpStatus != nil {
		status = s.mcpStatus()
	}
	return jfsettings.MCPServerSettingsSnapshot{Settings: settings, Status: status}
}

// SaveMCPServerSettings persists local MCP listener configuration and applies
// it immediately. A failed listener update restores the previous file state.
func (s *Service) SaveMCPServerSettings(input jfsettings.MCPServerSettingsUpdate) (jfsettings.MCPServerSettings, error) {
	s.mcpServerMu.Lock()
	defer s.mcpServerMu.Unlock()
	if s.mcpStore == nil {
		return s.GetMCPServerSettings(), ErrMCPServerStoreUnavailable
	}

	current := s.mcpStore.MCPServerSettings()
	port := input.Port
	if port == 0 {
		port = current.Port
	}
	if port == 0 {
		port = jfsettings.DefaultMCPServerPort
	}
	if port < jfsettings.MinWebAccessPort || port > jfsettings.MaxWebAccessPort {
		return current, ErrMCPServerPortInvalid
	}
	authMode := strings.ToLower(strings.TrimSpace(input.AuthMode))
	if authMode == "" {
		authMode = current.AuthMode
	}
	if authMode != "token" && authMode != "none" {
		return current, ErrMCPServerAuthModeInvalid
	}
	next := jfsettings.MCPServerSettings{
		Enabled:   input.Enabled,
		Port:      port,
		AuthMode:  authMode,
		TokenHash: current.TokenHash,
	}
	next.TokenConfigured = next.TokenHash != ""
	if next.Enabled && next.AuthMode == "token" && !next.TokenConfigured {
		return current, ErrMCPServerTokenRequired
	}
	return s.saveMCPServerSettingsLocked(current, next)
}

// ResetMCPServerToken creates a fresh bearer secret, persists only its
// verifier, applies it to a running listener, and returns the secret once.
func (s *Service) ResetMCPServerToken() (jfsettings.MCPServerSettings, string, error) {
	s.mcpServerMu.Lock()
	defer s.mcpServerMu.Unlock()
	if s.mcpStore == nil {
		return s.GetMCPServerSettings(), "", ErrMCPServerStoreUnavailable
	}

	current := s.mcpStore.MCPServerSettings()
	newToken := s.newMCPToken
	if newToken == nil {
		newToken = newMCPServerToken
	}
	token, err := newToken()
	if err != nil {
		return current, "", err
	}
	hashPassword := s.hashPassword
	if hashPassword == nil {
		hashPassword = passwordhash.Hash
	}
	hash, err := hashPassword(token)
	if err != nil {
		return current, "", err
	}
	next := current
	next.TokenHash = hash
	next.TokenConfigured = true
	result, err := s.saveMCPServerSettingsLocked(current, next)
	if err != nil {
		return result, "", err
	}
	return result, token, nil
}

func (s *Service) saveMCPServerSettingsLocked(current jfsettings.MCPServerSettings, next jfsettings.MCPServerSettings) (jfsettings.MCPServerSettings, error) {
	if s.mcpStore == nil {
		return current, ErrMCPServerStoreUnavailable
	}
	result, err := s.mcpStore.SaveMCPServerSettings(next)
	if err != nil {
		return result, err
	}
	if s.sideEffects.OnMCPServerChanged != nil {
		if err := s.sideEffects.OnMCPServerChanged(result); err != nil {
			if _, rollbackErr := s.mcpStore.SaveMCPServerSettings(current); rollbackErr != nil {
				return current, fmt.Errorf("%w: %v; settings rollback failed: %v", ErrMCPServerRuntimeUpdate, err, rollbackErr)
			}
			return current, fmt.Errorf("%w: %v", ErrMCPServerRuntimeUpdate, err)
		}
	}
	return result, nil
}

func newMCPServerToken() (string, error) {
	bytes := make([]byte, mcpServerTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "jft_mcp_" + base64.RawURLEncoding.EncodeToString(bytes), nil
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
