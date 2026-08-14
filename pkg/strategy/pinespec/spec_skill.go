package pinespec

import (
	"maps"
	"strings"
)

func ResearchSkillDescription() string {
	return "用于 JFTrade 策略研究、历史版本比较、临时回测和结果查看；试错不保存策略定义。"
}

func ResearchSkillInstructions() string {
	return strings.Join([]string{
		"处理策略研究任务时，要明确区分策略想法、临时研究回测、已有回测结果和已保存策略定义。",
		"本 Skill 只负责研究路径：先确认是想法验证、参数试错还是结果解读；当前 invocation 只调用已经声明且当前 Agent 获授权的工具，不猜测、绕过或要求未解锁的工具。",
		"开始前优先读取 references/strategy-research-workflow.md；需要 Pine 快速格式时读取 references/pine-v6-cheatsheet.md；只有需要完整支持范围或示例时才读取完整 spec/examples。",
		"只能输出当前 JFTrade 支持的 Pine Script v6 策略脚本；必须包含 //@version=6 和 strategy(...).",
		"如果用户询问 Pine 支持范围，必须依据内置规范回答，不要杜撰未支持的 built-ins、订单选项或 TradingView broker emulator 行为。",
		"纯语法或兼容性检查使用 strategy.validate_pine；策略试错、参数迭代、验证收益/回撤时必须使用 strategy.research_backtest，不要保存策略定义。",
		"比较已保存策略版本时，先用 strategy.definition_versions.list 确认版本，再用 strategy.definition_versions.get 读取完整不可变快照；使用 definitionId 和 definitionVersion 过滤 backtest.runs，并且只比较标的、周期、时间范围、初始资金、复权、时段和费用配置一致的已完成回测。",
		"research_backtest 返回未完成状态时，先短暂调用 workflow.wait，再用 backtest.result_view 按 summary/chart/orders/logs/errors 和 limit/cursor/resolution 分片查看结果；已有回测列表继续用 backtest.runs；需要停止仍在运行的回测时使用 backtest.cancel。回测前用 market.providers 确认提供者能力；marketDataProvider 只冻结本次任务，不改变默认值。yfinance/AKShare 仅轮询，不能支撑实时策略。",
		"research_backtest 返回 syncing_data 时，使用 workflow.wait 和 backtest.kline_sync_status 等待；completed 后必须用完全相同参数重试 research_backtest，failed、cancelled 或 insufficient_after_sync 时停止自动重试并说明原因。",
		"不要把临时研究脚本自动保存为策略定义；如用户明确要求保存、发布、更新策略定义、修改实例模式或优化已保存定义，先 load_skill(jftrade-strategy-publish)，再把脚本、校验和回测结论交接给发布流程；发布 Skill 未加载前不得调用写入或优化工具。",
		"每次回测结论必须说明标的、周期、时间范围、关键参数、运行状态和数据限制；没有完成结果时只报告当前进度，不把历史模拟描述为收益承诺。",
		"不要承诺收益；回测只代表指定标的、周期、时间范围和数据条件下的历史模拟结果。",
	}, " ")
}

func ResearchSkillAllowedTools() []string {
	return []string{
		ToolName,
		"strategy.validate_pine",
		"strategy.definition_versions.list",
		"strategy.definition_versions.get",
		"strategy.research_backtest",
		"backtest.runs",
		"backtest.result_view",
		"backtest.kline_sync_status",
		"backtest.cancel",
		"workflow.wait",
		"market.snapshot",
		"market.candles",
		"market.providers",
		"research.screen_catalog",
		"strategy.instance_activity",
	}
}

func PublishSkillDescription() string {
	return "用于 JFTrade 策略保存、发布、历史版本恢复、实例模式调整和已保存策略定义优化。"
}

func PublishSkillInstructions() string {
	return strings.Join([]string{
		"处理策略发布任务时，要明确区分临时研究脚本、策略草稿、已保存定义和正在运行的策略实例。",
		"本 Skill 只负责明确的持久化、实例模式调整和已保存定义优化；当前 invocation 只调用已经声明且当前 Agent 获授权的工具，不猜测、绕过或要求未解锁的工具。",
		"开始前优先读取 references/strategy-publish-checklist.md；需要 Pine 快速格式时读取 references/pine-v6-cheatsheet.md；只有需要完整支持范围或示例时才读取完整 spec/examples。",
		"只有用户明确要求保存、发布、更新策略定义、修改实例模式或优化已保存定义时，才使用本 skill 的写入和优化工具。",
		"如果用户仍在验证策略想法、比较参数或需要临时回测，先 load_skill(jftrade-strategy-research) 并完成研究；不要在研究 Skill 未加载时调用研究专属工具，也不要用保存或优化动作代替临时验证。",
		"保存前必须先用 strategy.validate_pine 校验脚本；校验失败时不要调用 strategy.save_draft 或 strategy.save_definition。",
		"strategy.save_definition 用于明确的新建或更新定义；strategy.save_draft 只用于用户明确要求保存草稿的场景。",
		"只有用户明确要求恢复历史版本时，才用 strategy.definition_versions.list 和 strategy.definition_versions.get 读取旧快照；校验该快照后，把旧快照的 name、description、symbol、interval、script 和 visualModel 映射到相同 definitionId 的 strategy.save_definition 调用。恢复会创建新的当前版本，绝不能修改、覆盖或伪造历史快照。",
		"只有在用户明确要求修改某个具体实例执行模式时才用 strategy.update_instance_mode；优化已保存候选定义时用 strategy.optimize，并用 backtest.runs 查看队列状态。",
		"strategy.optimize 返回 syncing_data 时，使用 backtest.kline_sync_status 等待；completed 后用完全相同参数重试 optimize，failed、cancelled 或 insufficient_after_sync 时停止自动重试并说明原因。",
		"完成写入或优化后，明确报告实际动作、目标定义或实例、审批状态以及后续查询方式；不要承诺收益，写入、优化和实例模式变更必须遵守当前审批模式。实例化、启动、暂停/停止、刷新定义和风控更新应使用对应 strategy.instance_* 工具，并在启动前确认实时流和明确账户绑定。",
	}, " ")
}

func PublishSkillAllowedTools() []string {
	return []string{
		"strategy.validate_pine",
		"strategy.definition_versions.list",
		"strategy.definition_versions.get",
		"strategy.save_draft",
		"strategy.save_definition",
		"strategy.update_instance_mode",
		"strategy.optimize",
		"backtest.runs",
		"backtest.kline_sync_status",
		"market.providers",
		"strategy.instantiate",
		"strategy.instance_start",
		"strategy.instance_stop",
		"strategy.instance_refresh_definition",
		"strategy.instance_risk.update",
		"strategy.instance_activity",
		"backtest.cancel",
	}
}

func SkillResourceFiles() map[string]string {
	return map[string]string{
		"references/pine-v6-spec.md":       BuildSpecMarkdown(),
		"references/pine-v6-examples.md":   BuildExamplesMarkdown(),
		"references/pine-v6-cheatsheet.md": BuildCheatsheetMarkdown(),
	}
}

func ResearchSkillResourceFiles() map[string]string {
	files := cloneSkillResourceFiles(SkillResourceFiles())
	files["references/strategy-research-workflow.md"] = BuildResearchWorkflowMarkdown()
	return files
}

func PublishSkillResourceFiles() map[string]string {
	files := cloneSkillResourceFiles(SkillResourceFiles())
	files["references/strategy-publish-checklist.md"] = BuildPublishChecklistMarkdown()
	return files
}

func cloneSkillResourceFiles(files map[string]string) map[string]string {
	out := make(map[string]string, len(files)+1)
	maps.Copy(out, files)
	return out
}

func SaveDraftUsageHint() string {
	return "可以先查询 Pine v6 规范和示例，确认脚本格式正确。也可以从下面这个 JFTrade Pine v6 骨架开始：\n" + Skeleton()
}

func Skeleton() string {
	return examples[0].Script
}
