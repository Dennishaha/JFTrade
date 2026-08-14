package assembly

import (
	"context"
	"fmt"
	"strings"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func registerJFTradeADKResearchScreenTools(registry *jfadkruntime.ToolRegistry, deps ToolDeps) {
	registry.Register(assistantmodel.ToolDescriptor{Name: "research.screen_catalog", DisplayName: "筛选因子目录", Description: "按市场读取版本化股票筛选因子、参数、运算符、列能力和限流目录。", Category: "research", Permission: "read_internal", RiskLevel: "low", OutputSummary: "筛选目录版本、因子、参数、运算符和限流信息。", RequiredSkills: []string{"jftrade-research"}}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.ResearchScreenCatalog == nil {
			return nil, fmt.Errorf("research screen catalog is unavailable")
		}
		return deps.ResearchScreenCatalog(strings.ToUpper(strings.TrimSpace(stringValue(input, "market"))))
	})
}

func registerADKStrategyInstanceTools(registry *jfadkruntime.ToolRegistry, deps ToolDeps) {
	registry.Register(assistantmodel.ToolDescriptor{Name: "strategy.instantiate", DisplayName: "实例化策略", Description: "从已保存的策略定义创建策略实例并保存完整 binding；实例初始为 STOPPED。", Category: "strategy", Permission: "write_strategy", RiskLevel: "high", RequiresApprovalIn: []string{assistantmodel.PermissionModeApproval}, OutputSummary: "新建策略实例及 binding。", RequiredSkills: []string{strategypinespec.PublishBuiltinSkillName}}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.InstantiateStrategy == nil {
			return nil, fmt.Errorf("strategy instance service is unavailable")
		}
		definitionID := strings.TrimSpace(stringValue(input, "definitionId"))
		if definitionID == "" {
			return nil, fmt.Errorf("definitionId is required")
		}
		var binding stratsrv.InstanceBinding
		if value, ok := input["binding"]; ok {
			if err := decodeToolInputValue(value, &binding); err != nil {
				return nil, fmt.Errorf("binding must be a valid object: %w", err)
			}
		}
		return deps.InstantiateStrategy(definitionID, binding)
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "strategy.instance_start", DisplayName: "启动策略实例", Description: "启动策略实例实时运行；启动前检查行情提供者健康、实时流能力、账户绑定、实例状态和 Pine Worker 容量。", Category: "strategy", Permission: "live_trading", RiskLevel: "critical", AllowedModes: approvalModes(), RequiresApprovalIn: approvalModes(), OutputSummary: "启动后的实例、运行状态和 runtime observation。", RequiredSkills: []string{strategypinespec.PublishBuiltinSkillName}}, func(ctx context.Context, input map[string]any) (any, error) {
		if deps.StartStrategyInstance == nil {
			return nil, fmt.Errorf("strategy instance service is unavailable")
		}
		instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
		if instanceID == "" {
			return nil, fmt.Errorf("instanceId is required")
		}
		return deps.StartStrategyInstance(ctx, instanceID)
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "strategy.instance_stop", DisplayName: "暂停或停止策略实例", Description: "执行 pause 或 stop，并由策略 Service 原子完成状态变更与 runtime 停止。", Category: "strategy", Permission: "write_strategy", RiskLevel: "high", RequiresApprovalIn: []string{assistantmodel.PermissionModeApproval}, OutputSummary: "停止后的实例状态。", RequiredSkills: []string{strategypinespec.PublishBuiltinSkillName}}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.StopStrategyInstance == nil {
			return nil, fmt.Errorf("strategy instance service is unavailable")
		}
		instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
		if instanceID == "" {
			return nil, fmt.Errorf("instanceId is required")
		}
		return deps.StopStrategyInstance(instanceID, stringOrDefault(stringValue(input, "action"), "stop"))
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "strategy.instance_refresh_definition", DisplayName: "刷新策略实例定义", Description: "将已停止的策略实例刷新到关联策略定义最新版本。", Category: "strategy", Permission: "write_strategy", RiskLevel: "high", RequiresApprovalIn: []string{assistantmodel.PermissionModeApproval}, OutputSummary: "刷新后的实例及定义同步状态。", RequiredSkills: []string{strategypinespec.PublishBuiltinSkillName}}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.RefreshStrategyInstance == nil {
			return nil, fmt.Errorf("strategy instance service is unavailable")
		}
		instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
		if instanceID == "" {
			return nil, fmt.Errorf("instanceId is required")
		}
		return deps.RefreshStrategyInstance(instanceID)
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "strategy.instance_risk.update", DisplayName: "更新实例动态风控", Description: "更新策略实例动态风控；该操作在所有权限模式下逐次审批，尤其不允许静默放宽限制。", Category: "strategy", Permission: "write_strategy", RiskLevel: "critical", AllowedModes: approvalModes(), RequiresApprovalIn: approvalModes(), OutputSummary: "更新后的风控和实例状态。", RequiredSkills: []string{strategypinespec.PublishBuiltinSkillName}}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.UpdateStrategyInstanceRisk == nil {
			return nil, fmt.Errorf("strategy instance risk service is unavailable")
		}
		instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
		if instanceID == "" {
			return nil, fmt.Errorf("instanceId is required")
		}
		value, ok := input["risk"]
		if !ok {
			return nil, fmt.Errorf("risk is required")
		}
		var risk stratsrv.RuntimeRiskSettings
		if err := decodeToolInputValue(value, &risk); err != nil {
			return nil, fmt.Errorf("risk must be a valid object: %w", err)
		}
		if err := validateStrategyRuntimeRisk(risk); err != nil {
			return nil, err
		}
		return deps.UpdateStrategyInstanceRisk(instanceID, risk)
	})
	registry.Register(assistantmodel.ToolDescriptor{Name: "strategy.instance_activity", DisplayName: "策略实例活动", Description: "分页读取策略实例运行日志或控制审计记录。", Category: "strategy", Permission: "read_internal", RiskLevel: "low", OutputSummary: "分页日志或审计时间线。", RequiredSkills: []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName}}, func(_ context.Context, input map[string]any) (any, error) {
		if deps.StrategyInstanceActivity == nil {
			return nil, fmt.Errorf("strategy instance activity is unavailable")
		}
		instanceID := strings.TrimSpace(stringValue(input, "instanceId"))
		if instanceID == "" {
			return nil, fmt.Errorf("instanceId is required")
		}
		return deps.StrategyInstanceActivity(instanceID, stringOrDefault(stringValue(input, "kind"), "logs"), intValue(input, "limit", 50), intValue(input, "offset", 0))
	})
}

func validateStrategyRuntimeRisk(risk stratsrv.RuntimeRiskSettings) error {
	switch strings.ToLower(strings.TrimSpace(risk.Mode)) {
	case "off", "monitor", "enforce":
	default:
		return fmt.Errorf("risk.mode must be one of off, monitor, enforce")
	}
	if risk.MaxOrderQuantity != nil && *risk.MaxOrderQuantity <= 0 {
		return fmt.Errorf("risk.maxOrderQuantity must be greater than zero")
	}
	if risk.MaxOrderNotional != nil && *risk.MaxOrderNotional <= 0 {
		return fmt.Errorf("risk.maxOrderNotional must be greater than zero")
	}
	if risk.DailyMaxOrders != nil && *risk.DailyMaxOrders <= 0 {
		return fmt.Errorf("risk.dailyMaxOrders must be greater than zero")
	}
	return nil
}
