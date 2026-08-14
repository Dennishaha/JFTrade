package adk

import jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"

const DefaultBuiltinAgentID = jfadkmodel.DefaultBuiltinAgentID

// DefaultBuiltinToolNames keeps the primary assistant useful for common
// read-only market, research, portfolio and strategy questions without
// declaring every write and trading tool on every turn. Custom agents can opt
// into the full registry explicitly.
func DefaultBuiltinToolNames() []string {
	return []string{
		"interaction.request_user", "workflow.wait", "tools.search", "models.list",
		"system.status", "system.futu_opend",
		"plugins.catalog",
		"market.capabilities", "market.search", "market.snapshot", "market.snapshots",
		"market.candles", "market.intraday", "market.subscriptions", "watchlist.list",
		"research.instrument", "research.financials", "research.valuation", "research.news", "research.screen",
		"portfolio.accounts", "portfolio.overview", "portfolio.positions", "account.orders", "risk.state",
		"strategy.definitions", "strategy.validate_pine", "strategy.research_backtest",
		"backtest.runs", "backtest.result_view", "backtest.kline_sync_status",
	}
}

func BuiltinAgentTemplates() []AgentWriteRequest {
	return []AgentWriteRequest{
		{
			ID: DefaultBuiltinAgentID, Name: "默认助手",
			Instruction:    defaultAgentInstruction(),
			PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, MemoryEnabled: true, WorkMode: WorkModeChat, LoopMaxIterations: DefaultLoopMaxIterations,
			Tools: DefaultBuiltinToolNames(), ToolAccessMode: ToolAccessModeSelected,
			Skills: BuiltinSkillIDs(),
		},
	}
}

func BuiltinAgentTemplate(id string) (AgentWriteRequest, bool) {
	id = normalizeID(id)
	for _, template := range BuiltinAgentTemplates() {
		if normalizeID(template.ID) == id {
			return template, true
		}
	}
	return AgentWriteRequest{}, false
}

func IsBuiltinAgentID(id string) bool {
	_, ok := BuiltinAgentTemplate(id)
	return ok
}

func IsPrimaryBuiltinAgentID(id string) bool {
	return normalizeID(id) == DefaultBuiltinAgentID
}
