package servercore

import (
	"context"
	"fmt"
	"slices"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

var productReadTools = []struct {
	name, displayName, category, skill string
}{
	{"market.capabilities", "市场能力目录", "market", "jftrade-market"},
	{"market.search", "搜索证券", "market", "jftrade-market"},
	{"market.instrument_profile", "产品资料", "market", "jftrade-market"},
	{"market.snapshot", "行情快照", "market", "jftrade-market"},
	{"market.snapshots", "批量快照", "market", "jftrade-market"},
	{"market.candles", "K 线查询", "market", "jftrade-market"},
	{"market.intraday", "分时数据", "market", "jftrade-market"},
	{"market.ticks", "逐笔成交", "market", "jftrade-market"},
	{"market.depth", "盘口深度", "market", "jftrade-market"},
	{"market.broker_queue", "经纪队列", "market", "jftrade-market"},
	{"market.capital_flow", "资金流向", "market", "jftrade-market"},
	{"derivatives.option_chain", "期权链", "derivatives", "jftrade-derivatives"},
	{"derivatives.option_screen", "期权筛选", "derivatives", "jftrade-derivatives"},
	{"derivatives.option_analysis", "期权分析", "derivatives", "jftrade-derivatives"},
	{"derivatives.option_events", "期权事件", "derivatives", "jftrade-derivatives"},
	{"derivatives.warrants", "港股轮证", "derivatives", "jftrade-derivatives"},
	{"derivatives.futures", "期货目录", "derivatives", "jftrade-derivatives"},
	{"research.instrument", "公司研究", "research", "jftrade-research"},
	{"research.financials", "财务报表", "research", "jftrade-research"},
	{"research.valuation", "估值研究", "research", "jftrade-research"},
	{"research.analyst", "分析师研究", "research", "jftrade-research"},
	{"research.ownership", "股东与机构持仓", "research", "jftrade-research"},
	{"research.corporate_actions", "公司行动", "research", "jftrade-research"},
	{"research.short_interest", "沽空数据", "research", "jftrade-research"},
	{"research.news", "证券新闻", "research", "jftrade-research"},
	{"research.screen", "研究筛选器", "research", "jftrade-research"},
	{"research.calendar", "市场日历", "research", "jftrade-research"},
	{"research.macro", "宏观研究", "research", "jftrade-research"},
	{"research.rankings", "市场榜单", "research", "jftrade-research"},
	{"research.institutions", "机构研究", "research", "jftrade-research"},
	{"research.industry", "产业链研究", "research", "jftrade-research"},
	{"research.technical_indicators", "技术指标目录与计算", "research", "jftrade-research"},
	{"prediction.discover", "预测市场发现", "prediction", "jftrade-prediction"},
	{"prediction.snapshot", "预测合约快照", "prediction", "jftrade-prediction"},
	{"prediction.depth", "预测合约盘口", "prediction", "jftrade-prediction"},
	{"prediction.history", "预测合约历史", "prediction", "jftrade-prediction"},
	{"prediction.combo_eligible", "Parlay 合格事件", "prediction", "jftrade-prediction"},
	{"prediction.combo_quote", "Parlay RFQ", "prediction", "jftrade-prediction"},
	{"execution.buying_power", "购买力预检", "execution", "jftrade-trading"},
	{"alerts.price.list", "价格提醒列表", "alerts", "jftrade-market"},
	{"alerts.option_event.list", "期权事件提醒列表", "alerts", "jftrade-derivatives"},
	{"watchlist.remote.list", "远程自选列表", "watchlist", "jftrade-market"},
}

var productTradeTools = []struct {
	name, displayName string
}{
	{"execution.order_preview", "订单预检"},
	{"execution.order_place", "正式下单"},
	{"execution.order_cancel", "撤销订单"},
	{"execution.combo_preview", "组合订单预检"},
	{"execution.combo_place", "组合下单"},
	{"execution.combo_cancel", "撤销组合订单"},
}

var productWriteTools = []struct {
	name, displayName, skill string
}{
	{"alerts.price.set", "设置价格提醒", "jftrade-market"},
	{"alerts.option_event.set", "设置期权事件提醒", "jftrade-derivatives"},
	{"watchlist.remote.modify", "修改远程自选", "jftrade-market"},
}

func registerJFTradeProductTools(registry *jfadk.ToolRegistry, deps ToolDeps) {
	for _, definition := range productReadTools {
		item := definition
		// These tools have legacy core-market handlers for isolated runtime
		// tests. Production supplies ProductTool and deliberately replaces them
		// with the broker-routed, attributed product service below.
		if deps.ProductTool == nil && slices.Contains(
			[]string{"market.snapshot", "market.candles", "market.depth"},
			item.name,
		) {
			continue
		}
		registry.Register(jfadk.ToolDescriptor{
			Name: item.name, DisplayName: item.displayName,
			Description: "通过券商抽象和统一产品服务读取数据；支持 brokerId、分页和权限归因。",
			Category:    item.category, Permission: "read_internal", RiskLevel: "low",
			OutputSummary: "返回实际券商、能力状态、数据时间、分页、warnings 和 partial errors。",
			RequiredSkill: item.skill, InputSchema: productToolInputSchema(item.name),
		}, func(ctx context.Context, input map[string]any) (any, error) {
			if deps.ProductTool == nil {
				return nil, fmt.Errorf("product feature service is unavailable")
			}
			return deps.ProductTool(ctx, item.name, input)
		})
	}
	for _, definition := range productTradeTools {
		item := definition
		registry.Register(jfadk.ToolDescriptor{
			Name: item.name, DisplayName: item.displayName,
			Description: "执行统一单腿、期权组合或预测市场订单操作；每次调用均要求审批。",
			Category:    "execution", Permission: "live_trading", RiskLevel: "critical",
			AllowedModes: approvalModes(), RequiresApprovalIn: approvalModes(),
			OutputSummary: "返回预检、内部订单、券商订单和生命周期状态。",
			RequiredSkill: "jftrade-trading", InputSchema: productToolInputSchema(item.name),
		}, func(ctx context.Context, input map[string]any) (any, error) {
			if deps.ExecutionTool == nil {
				return nil, fmt.Errorf("execution service is unavailable")
			}
			return deps.ExecutionTool(ctx, item.name, input)
		})
	}
	for _, definition := range productWriteTools {
		item := definition
		registry.Register(jfadk.ToolDescriptor{
			Name: item.name, DisplayName: item.displayName,
			Description: "修改券商侧提醒或自选数据；每次调用均要求审批。",
			Category:    "customization", Permission: "write_external", RiskLevel: "high",
			AllowedModes: approvalModes(), RequiresApprovalIn: approvalModes(),
			OutputSummary: "返回实际券商、变更结果和数据时间。",
			RequiredSkill: item.skill, InputSchema: productToolInputSchema(item.name),
		}, func(ctx context.Context, input map[string]any) (any, error) {
			if deps.ProductTool == nil {
				return nil, fmt.Errorf("product feature service is unavailable")
			}
			return deps.ProductTool(ctx, item.name, input)
		})
	}
}

func approvalModes() []string {
	return []string{
		jfadk.PermissionModeApproval,
		jfadk.PermissionModeLessApproval,
		jfadk.PermissionModeAll,
	}
}
