package adk

func BuiltinAgentTemplates() []AgentWriteRequest {
	return []AgentWriteRequest{
		{
			ID: "investment-analyst", Name: "投资分析助手",
			Instruction:    "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；输出必须说明数据来源，不承诺收益。",
			PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, MemoryEnabled: true, WorkMode: WorkModeChat, LoopMaxIterations: DefaultLoopMaxIterations,
			Tools:  []string{"system.status", "tools.search", "market.snapshot", "market.candles", "market.depth", "portfolio.summary", "broker.orders", "broker.fills", "risk.state", "strategy.pine_spec", "strategy.validate_pine", "strategy.save_definition", "strategy.update_instance_mode", "tasks.create", "tasks.update", "tasks.delete", "tasks.list", "memory.list", "memory.remember", "memory.forget"},
			Skills: []string{"jftrade-market", "jftrade-portfolio", "jftrade-strategy"},
		},
		{
			ID: "strategy-researcher", Name: "策略研究助手",
			Instruction:    "你是 JFTrade 策略研究 agent。清晰区分想法、草稿、回测结果和运行实例；不承诺收益。",
			PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, MemoryEnabled: true, WorkMode: WorkModeChat, LoopMaxIterations: DefaultLoopMaxIterations,
			Tools:  []string{"tools.search", "strategy.definitions", "strategy.pine_spec", "strategy.validate_pine", "strategy.save_draft", "strategy.save_definition", "strategy.update_instance_mode", "backtest.runs", "strategy.optimize", "market.snapshot", "market.candles", "tasks.create", "tasks.update", "tasks.delete", "tasks.list", "memory.list", "memory.remember", "memory.forget"},
			Skills: []string{"jftrade-market", "jftrade-strategy"},
		},
		{
			ID: "opend-diagnostician", Name: "OpenD 诊断助手",
			Instruction:    "你是 JFTrade OpenD 诊断 agent。优先检查系统状态、OpenD 健康、行情订阅和 broker 连接；给出可执行排查步骤。",
			PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, MemoryEnabled: true, WorkMode: WorkModeChat, LoopMaxIterations: DefaultLoopMaxIterations,
			Tools:  []string{"system.status", "system.futu_opend", "market.subscriptions", "broker.orders", "broker.fills", "tasks.create", "tasks.update", "tasks.delete", "tasks.list", "memory.list", "memory.remember", "memory.forget"},
			Skills: []string{"jftrade-market", "jftrade-portfolio"},
		},
		{
			ID: "risk-reviewer", Name: "风控审查助手",
			Instruction:    "你是 JFTrade 风控审查 agent。重点检查实盘开关、风控限制、订单事件、持仓、资金和保证金信息；涉及交易动作必须保持人工确认。",
			PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, MemoryEnabled: true, WorkMode: WorkModeChat, LoopMaxIterations: DefaultLoopMaxIterations,
			Tools:  []string{"system.status", "risk.state", "risk.events", "portfolio.summary", "broker.orders", "broker.fills", "broker.fees", "broker.margin_ratios", "execution.order_events", "strategy.pine_spec", "strategy.validate_pine", "strategy.save_definition", "strategy.update_instance_mode", "tasks.create", "tasks.update", "tasks.delete", "tasks.list", "memory.list", "memory.remember", "memory.forget"},
			Skills: []string{"jftrade-portfolio", "jftrade-strategy"},
		},
	}
}

func BuiltinAgentTemplate(id string) (AgentWriteRequest, bool) {
	for _, template := range BuiltinAgentTemplates() {
		if template.ID == id {
			return template, true
		}
	}
	return AgentWriteRequest{}, false
}
