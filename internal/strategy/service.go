package strategy

// Package strategy 提供策略业务逻辑门面，将策略定义、目录和运行时能力
// 收敛为统一的 Service。
//
// DesignStore、CatalogStore 和 RuntimeManager 由应用装配层通过适配器实现。
// Service 负责稳定业务入口，具体存储与运行时实现保留在 servercore。
//
import (
	"context"
	"errors"
	"fmt"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

// ──────────────────────────────────────────────────────────────────────────────
// 依赖接口
// ──────────────────────────────────────────────────────────────────────────────

// DesignStore 策略定义持久化接口。
// 由应用装配层的策略定义存储适配器实现。
type DesignStore interface {
	// ListDefinitions 返回所有未删除的策略定义（按 updated_at 倒序）。
	ListDefinitions() []Definition

	// GetDefinition 按 ID 查询策略定义。
	// 返回 (定义, 是否存在, 错误)。
	GetDefinition(id string) (Definition, bool, error)

	// SaveDefinition 创建或更新策略定义（upsert）。
	// 新定义自动分配版本号 0.1.0；内容变更时自增版本号。
	SaveDefinition(input Definition) (Definition, error)

	// DeleteDefinition 软删除策略定义（设置 deleted_at 时间戳）。
	DeleteDefinition(id string) (Definition, error)
}

// CatalogStore 策略目录/实例管理接口。
// 由应用装配层的策略目录存储适配器实现。
type CatalogStore interface {
	// ── 实例 CRUD ──

	// ListInstances 返回所有策略实例列表（含运行时观测增强）。
	ListInstances() []InstanceView

	// GetInstance 按 ID 查询策略实例内部记录。
	// 返回 (实例, 是否存在)。
	GetInstance(id string) (ManagedInstance, bool)

	// ValidateStartable 检查实例是否可以进入实时运行。
	ValidateStartable(instance ManagedInstance) error

	// CreateInstance 从策略定义创建新实例（状态为 STOPPED）。
	CreateInstance(def Definition, binding InstanceBinding) (InstanceView, error)

	// UpdateInstance 更新实例绑定参数（仅 STOPPED 状态可更新）。
	UpdateInstance(id string, binding InstanceBinding) (InstanceView, error)

	// UpdateInstanceRuntimeRisk 更新实例运行风控（允许运行中快速切换）。
	UpdateInstanceRuntimeRisk(id string, risk RuntimeRiskSettings) (InstanceView, error)

	// DeleteInstance 删除实例（仅 STOPPED 状态可删）。
	DeleteInstance(id string) (InstanceView, error)

	// TransitionInstance 变更实例状态。
	// status: "RUNNING" | "PAUSED" | "STOPPED"。
	TransitionInstance(id string, status string) (InstanceView, error)

	// RefreshDefinition 将实例关联的策略定义刷新到最新版本。
	RefreshDefinition(id string, def Definition) (InstanceView, error)

	// RefreshInstanceDefinition 查找实例关联的策略定义并刷新到最新版本。
	// 适配器层负责：获取实例记录 → 提取 definitionID → 获取最新定义 → 执行刷新。
	RefreshInstanceDefinition(instanceID string) (InstanceView, error)

	// ApplyDefinitionToLinked 将定义的最新版本批量应用到所有关联实例。
	ApplyDefinitionToLinked(def Definition) (ApplyLinkedInstancesResult, error)

	// GetLinkedInstanceIDs 返回关联指定定义 ID 的所有实例 ID 列表。
	GetLinkedInstanceIDs(definitionID string) []string

	// ── 活动日志/审计 ──

	// GetLogs 查询实例运行日志（支持分页、级别、时间范围过滤）。
	GetLogs(id string, query LogQuery) (LogsResult, bool)

	// GetAudit 查询实例审计记录（支持分页、类型、时间范围过滤）。
	GetAudit(id string, query AuditQuery) (AuditResult, bool)

	// ── 生命周期 ──

	// ReconcileOnStartup 启动时将遗留的 RUNNING/PAUSED 实例重置为 STOPPED。
	// 返回被重置的实例数量。
	ReconcileOnStartup() (int, error)

	// PluginCatalog 返回插件目录（含兼容性检测）。
	PluginCatalog() PluginCatalog

	// PluginOperation 返回插件操作状态。
	PluginOperation(id string) (PluginOperation, bool)

	// PluginUninstallGuidance 返回插件卸载指引。
	PluginUninstallGuidance(id string) (PluginUninstallGuidance, bool)

	// InstallPlugin 安装插件元数据。
	InstallPlugin(id string) (PluginOperation, error)

	// UninstallPlugin 卸载插件元数据。
	UninstallPlugin(id string) (PluginOperation, error)

	// Close 关闭 catalog 存储（含 runtime store）。
	Close() error
}

// RuntimeManager 策略运行时控制接口。
// 由应用装配层的策略运行时适配器实现。
type RuntimeManager interface {
	// Start 启动策略实例的实时运行（创建 goroutine + 订阅行情）。
	Start(ctx context.Context, instance ManagedInstance) error

	// Stop 停止策略实例的实时运行（取消 context + 清理订阅）。
	Stop(instanceID string)

	// GetObservation 获取实例的运行时观测状态。
	GetObservation(id string) (RuntimeObservation, bool)

	// RuntimeSummary 返回所有活跃实例的运行时摘要（供 system status 使用）。
	RuntimeSummary() RuntimeSummary

	// ActiveInstrumentIDs 返回所有活跃实例订阅的标的 ID（供行情流管理）。
	ActiveInstrumentIDs() []string
}

// ──────────────────────────────────────────────────────────────────────────────
// Service 策略业务门面
// ──────────────────────────────────────────────────────────────────────────────

// Service 提供策略业务的统一入口：定义管理、实例生命周期、运行时控制、活动查询。
// 持有 DesignStore、CatalogStore、RuntimeManager 的引用，方法直接委托。
type Service struct {
	design  DesignStore
	catalog CatalogStore
	runtime RuntimeManager

	analyzePine             func(PineAnalyzeInput) (PineAnalysisResult, error)
	refreshLiveMarketStream func(context.Context)
}

// Option 函数选项模式，用于注入可选依赖。
type Option func(*Service)

// PineAnalyzeInput 描述 /strategy-pine/analyze 的业务输入。
type PineAnalyzeInput struct {
	Script       string
	SourceFormat string
	IncludeAST   bool
}

// WithPineAnalyzer 注入 Pine 脚本分析器。
func WithPineAnalyzer(fn func(PineAnalyzeInput) (PineAnalysisResult, error)) Option {
	return func(s *Service) { s.analyzePine = fn }
}

// WithLiveMarketStreamRefresher 注入策略运行状态变化后的行情流刷新动作。
func WithLiveMarketStreamRefresher(fn func(context.Context)) Option {
	return func(s *Service) { s.refreshLiveMarketStream = fn }
}

// NewService 创建策略服务。所有依赖通过构造函数注入。
func NewService(design DesignStore, catalog CatalogStore, runtime RuntimeManager, opts ...Option) *Service {
	s := &Service{
		design:  design,
		catalog: catalog,
		runtime: runtime,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ──────────────────────────────────────────────────────────────────────────────
// Design 委托 — 策略定义 CRUD
// ──────────────────────────────────────────────────────────────────────────────

// ListDefinitions 返回所有策略定义。
func (s *Service) ListDefinitions() []Definition {
	return s.design.ListDefinitions()
}

// GetDefinition 按 ID 查询策略定义。
func (s *Service) GetDefinition(id string) (Definition, bool, error) {
	return s.design.GetDefinition(id)
}

// SaveDefinition 创建或更新策略定义。
func (s *Service) SaveDefinition(input Definition) (Definition, error) {
	return s.design.SaveDefinition(input)
}

// DeleteDefinition 删除策略定义。
func (s *Service) DeleteDefinition(id string) (Definition, error) {
	return s.design.DeleteDefinition(id)
}

// ──────────────────────────────────────────────────────────────────────────────
// Instance 委托 — 策略实例生命周期
// ──────────────────────────────────────────────────────────────────────────────

// ListInstances 返回所有策略实例。
func (s *Service) ListInstances() []InstanceView {
	return s.catalog.ListInstances()
}

// GetInstance 按 ID 查询策略实例。
func (s *Service) GetInstance(id string) (ManagedInstance, bool) {
	return s.catalog.GetInstance(id)
}

// ValidateStartable 检查实例是否可以启动。
func (s *Service) ValidateStartable(instance ManagedInstance) error {
	return s.catalog.ValidateStartable(instance)
}

// CreateInstance 从策略定义创建新实例。
func (s *Service) CreateInstance(def Definition, binding InstanceBinding) (InstanceView, error) {
	return s.catalog.CreateInstance(def, binding)
}

// UpdateInstance 更新实例绑定参数。
func (s *Service) UpdateInstance(id string, binding InstanceBinding) (InstanceView, error) {
	return s.catalog.UpdateInstance(id, binding)
}

// UpdateInstanceRuntimeRisk 更新实例运行风控。
func (s *Service) UpdateInstanceRuntimeRisk(id string, risk RuntimeRiskSettings) (InstanceView, error) {
	return s.catalog.UpdateInstanceRuntimeRisk(id, risk)
}

// DeleteInstance 删除实例。
func (s *Service) DeleteInstance(id string) (InstanceView, error) {
	return s.catalog.DeleteInstance(id)
}

// TransitionInstance 变更实例状态。
func (s *Service) TransitionInstance(id string, status string) (InstanceView, error) {
	return s.catalog.TransitionInstance(id, status)
}

// RefreshDefinition 刷新实例关联的策略定义到最新版本。
func (s *Service) RefreshDefinition(id string, def Definition) (InstanceView, error) {
	return s.catalog.RefreshDefinition(id, def)
}

// ApplyDefinitionToLinked 将定义的最新版本应用到所有关联实例。
func (s *Service) ApplyDefinitionToLinked(def Definition) (ApplyLinkedInstancesResult, error) {
	return s.catalog.ApplyDefinitionToLinked(def)
}

// GetLinkedInstanceIDs 返回关联指定定义的所有实例 ID。
func (s *Service) GetLinkedInstanceIDs(definitionID string) []string {
	return s.catalog.GetLinkedInstanceIDs(definitionID)
}

// ──────────────────────────────────────────────────────────────────────────────
// Runtime 委托 — 策略运行时控制
// ──────────────────────────────────────────────────────────────────────────────

// Start 启动策略实例的实时运行。
func (s *Service) Start(ctx context.Context, instance ManagedInstance) error {
	return s.runtime.Start(ctx, instance)
}

// StartInstance 启动策略实例，并在启动成功后完成状态切换与行情流刷新。
func (s *Service) StartInstance(ctx context.Context, instanceID string) (InstanceView, error) {
	instance, ok := s.catalog.GetInstance(instanceID)
	if !ok {
		return InstanceView{}, NotFoundError("strategy instance not found")
	}
	if err := s.catalog.ValidateStartable(instance); err != nil {
		return InstanceView{}, err
	}
	if err := s.runtime.Start(ctx, instance); err != nil {
		if errors.Is(err, pineworker.ErrCapacityExceeded) {
			return InstanceView{}, BusyError("运行实例 PineTS Worker 已达到上限。请停止其他运行实例，或到设置的 PineTS Worker 中调高“运行实例 Worker 最大值”后再启动。")
		}
		return InstanceView{}, err
	}
	result, err := s.catalog.TransitionInstance(instanceID, "RUNNING")
	if err != nil {
		s.runtime.Stop(instanceID)
		return InstanceView{}, err
	}
	if s.refreshLiveMarketStream != nil {
		s.refreshLiveMarketStream(context.Background())
	}
	return result, nil
}

// Stop 停止策略实例的实时运行。
func (s *Service) Stop(instanceID string) {
	s.runtime.Stop(instanceID)
}

// GetObservation 获取实例的运行时观测状态。
func (s *Service) GetObservation(id string) (RuntimeObservation, bool) {
	return s.runtime.GetObservation(id)
}

// RuntimeSummary 返回所有活跃实例的运行时摘要。
func (s *Service) RuntimeSummary() RuntimeSummary {
	return s.runtime.RuntimeSummary()
}

// ActiveInstrumentIDs 返回所有活跃实例订阅的标的 ID。
func (s *Service) ActiveInstrumentIDs() []string {
	return s.runtime.ActiveInstrumentIDs()
}

// ──────────────────────────────────────────────────────────────────────────────
// Activity 委托 — 日志与审计
// ──────────────────────────────────────────────────────────────────────────────

// GetLogs 查询实例运行日志（支持分页与过滤）。
func (s *Service) GetLogs(id string, query LogQuery) (LogsResult, bool) {
	return s.catalog.GetLogs(id, query)
}

// GetAudit 查询实例审计记录（支持分页与过滤）。
func (s *Service) GetAudit(id string, query AuditQuery) (AuditResult, bool) {
	return s.catalog.GetAudit(id, query)
}

// ──────────────────────────────────────────────────────────────────────────────
// Lifecycle 委托 — 启动调和、插件管理、资源释放
// ──────────────────────────────────────────────────────────────────────────────

// ReconcileOnStartup 启动时重置遗留的运行中状态。
func (s *Service) ReconcileOnStartup() (int, error) {
	return s.catalog.ReconcileOnStartup()
}

// PluginCatalog 返回插件目录。
func (s *Service) PluginCatalog() PluginCatalog {
	return s.catalog.PluginCatalog()
}

// PluginOperation 返回插件操作状态。
func (s *Service) PluginOperation(id string) (PluginOperation, bool) {
	return s.catalog.PluginOperation(id)
}

// PluginUninstallGuidance 返回插件卸载指引。
func (s *Service) PluginUninstallGuidance(id string) (PluginUninstallGuidance, bool) {
	return s.catalog.PluginUninstallGuidance(id)
}

// InstallPlugin 安装插件。
func (s *Service) InstallPlugin(id string) (PluginOperation, error) {
	return s.catalog.InstallPlugin(id)
}

// UninstallPlugin 卸载插件。
func (s *Service) UninstallPlugin(id string) (PluginOperation, error) {
	return s.catalog.UninstallPlugin(id)
}

// Close 关闭 catalog 存储。DesignStore 的生命周期由调用方管理。
func (s *Service) Close() error {
	if s.catalog != nil {
		return s.catalog.Close()
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Pine 分析（通过闭包注入）
// ──────────────────────────────────────────────────────────────────────────────

// AnalyzePine 分析 Pine 脚本（需通过 WithPineAnalyzer 注入编译器）。
func (s *Service) AnalyzePine(input PineAnalyzeInput) (PineAnalysisResult, error) {
	if s.analyzePine == nil {
		return nil, fmt.Errorf("pine analyzer not configured")
	}
	sourceFormat := strategydefinition.NormalizeSourceFormat(input.SourceFormat)
	if sourceFormat != strategydefinition.SourceFormatPineV6 {
		return nil, BadRequestError("strategy-pine analyze supports pine-v6 only")
	}
	input.SourceFormat = sourceFormat
	return s.analyzePine(input)
}

// ──────────────────────────────────────────────────────────────────────────────
// 编排方法 — 跨 store 的复合操作
// ──────────────────────────────────────────────────────────────────────────────

// RefreshInstanceDefinition 查找实例关联的策略定义并刷新到最新版本。
// 委托给 catalog 适配器层完成跨 store 编排（获取实例记录 → 提取 definitionID → 获取最新定义 → 执行刷新）。
func (s *Service) RefreshInstanceDefinition(instanceID string) (InstanceView, error) {
	return s.catalog.RefreshInstanceDefinition(instanceID)
}
