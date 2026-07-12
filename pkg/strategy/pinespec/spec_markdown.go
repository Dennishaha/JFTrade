package pinespec

import (
	"fmt"
	"strings"

	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineengine"
)

func BuildCheatsheetMarkdown() string {
	return strings.Join([]string{
		"# JFTrade Pine v6 快速参考",
		"",
		"- 脚本必须包含 `//@version=6` 和 `strategy(...)`。",
		"- 纯检查使用 `strategy.validate_pine`；研究试错使用 `strategy.research_backtest`；明确保存才进入发布流程。",
		"- 常用订单：`strategy.entry`、`strategy.close`、`strategy.exit`、`strategy.order`、`strategy.cancel`。",
		"- 常用数据：`open`、`high`、`low`、`close`、`volume`、`hl2`、`hlc3`、`ohlc4`。",
		"- 常用指标：EMA/SMA/RSI/CCI/Bollinger/Keltner/Donchian/SAR/linreg/OBV 等，完整范围以 `pine-v6-spec.md` 为准。",
		"- 不要假设 TradingView broker emulator 行为完全一致；以 JFTrade parser、planner 和 runtime 支持矩阵为准。",
	}, "\n")
}

func BuildResearchWorkflowMarkdown() string {
	return strings.Join([]string{
		"# 策略研究工作流",
		"",
		"1. 明确标的、周期、时间范围和初始资金；不完整时先用行情工具补齐上下文。",
		"2. 只做语法/兼容性检查时调用 `strategy.validate_pine`。",
		"3. 需要验证收益、回撤、成交或曲线时调用 `strategy.research_backtest`，输入临时脚本和回测参数。",
		"4. 返回 `syncing_data` 时调用 `workflow.wait` 和 `backtest.kline_sync_status`；同步 completed 后以完全相同参数重试 `strategy.research_backtest`。",
		"5. 同步 failed、cancelled 或 insufficient_after_sync 时停止自动重试并说明原因；回测未完成时短等后用 `backtest.result_view` 查询。",
		"6. 查看结果先用 `view=summary`；查看图表用 `view=chart`、`resolution=auto`、`limit<=1000`，并按需 include candles/trades/pnlCurve/drawdownCurve。",
		"7. 仅使用当前已声明且已获授权的工具。不要在研究阶段调用保存或优化工具；只有用户明确要求持久化、发布、实例模式调整或优化已保存定义时，先 `load_skill(jftrade-strategy-publish)` 再交接。",
		"8. 汇报时说明回测范围、参数、状态和数据限制；未完成时只报告进度。",
	}, "\n")
}

func BuildPublishChecklistMarkdown() string {
	return strings.Join([]string{
		"# 策略发布检查清单",
		"",
		"- 用户必须明确要求保存、发布、更新定义、保存草稿、修改实例模式或优化已保存定义。",
		"- 仅使用当前已声明且已获授权的工具。若仍需验证策略想法或进行临时回测，先 `load_skill(jftrade-strategy-research)`，不要用写入或优化替代研究。",
		"- 保存前必须调用 `strategy.validate_pine` 并确认校验成功。",
		"- `strategy.save_definition` 用于明确保存为策略定义；`strategy.save_draft` 只用于明确草稿保存。",
		"- `strategy.update_instance_mode` 只用于用户点名的具体实例。",
		"- `strategy.optimize` 只针对已保存定义创建真实异步回测任务，结果用 `backtest.runs` 查看。",
		"- `strategy.optimize` 返回 `syncing_data` 时用 `backtest.kline_sync_status` 等待，completed 后以相同参数重试；失败或覆盖仍不足时停止。",
		"- 输出中必须说明实际写入/优化对象、审批状态和后续查询方式；不承诺收益。",
	}, "\n")
}

func BuildSpecMarkdown() string {
	var builder strings.Builder
	builder.WriteString("# JFTrade Pine Script v6 规范\n\n")
	builder.WriteString("本文档描述当前 Pine parser、lowerer、planner 与 runtime 真正接受的 JFTrade Pine v6 语法范围。\n\n")
	builder.WriteString("- `sourceFormat`: `")
	builder.WriteString(SourceFormat)
	builder.WriteString("`\n")
	builder.WriteString("- `runtime`: `")
	builder.WriteString(Runtime)
	builder.WriteString("`\n")
	builder.WriteString("- `externalEngine`: `")
	builder.WriteString(pineengine.PinetsShadowEngineID)
	builder.WriteString("`（默认关闭，仅用于 PineTS 影子验证）\n")
	builder.WriteString("- `pineVersion`: `")
	builder.WriteString(PineVersion)
	builder.WriteString("`\n")
	builder.WriteString("- `productVersion`: `")
	builder.WriteString(ProductVersion)
	builder.WriteString("`\n")
	assessment := strategypine.CompatibilityScore()
	builder.WriteString("- `compatibilityScore`: `")
	fmt.Fprintf(&builder, "%.2f", assessment.Score)
	builder.WriteString("`（")
	builder.WriteString(assessment.ScoreModelVersion)
	builder.WriteString("）\n\n")

	writeMarkdownSection(&builder, "概览", sectionDetails("overview"))
	writeMarkdownSection(&builder, "语法", sectionDetails("syntax"))
	writeMarkdownList(&builder, "支持的顶层语句", supportedTopLevelStatements())
	writeMarkdownList(&builder, "支持的语句", supportedStatements())
	writeMarkdownSection(&builder, "表达式", sectionDetails("expressions"))
	writeMarkdownSection(&builder, "指标", sectionDetails("indicators"))
	writeMarkdownList(&builder, "保留变量", flattenNamedItems(reservedVariables()))
	writeMarkdownSection(&builder, "下单", sectionDetails("orders"))
	writeMarkdownList(&builder, "数量与下单模式", flattenNamedItems(orderModes()))
	writeMarkdownSection(&builder, "支持矩阵", sectionDetails("support-matrix"))
	writeMarkdownList(&builder, "v4.0 主路径、collection/map/matrix、tuple、动态循环、纯 UDT/method、MTF stoch、array stats、字符串/timeframe helper、object history method receiver、稳定 semantic metadata、public surface 诊断、MTF preflight、高级语言边界诊断、生成式支持快照与 broker 边界决策能力覆盖", flattenMatrixItems(supportMatrix()))
	writeMarkdownSection(&builder, "不支持项", sectionDetails("unsupported"))
	writeMarkdownList(&builder, "明确不支持的写法", unsupportedPatterns())
	builder.WriteString("## 最小骨架\n\n```text\n")
	builder.WriteString(Skeleton())
	builder.WriteString("\n```\n")

	return builder.String()
}

func BuildSupportSnapshotMarkdown() string {
	assessment := strategypine.CompatibilityScore()
	var builder strings.Builder
	builder.WriteString("# JFTrade Pine v6 Support Snapshot\n\n")
	builder.WriteString("> 自动生成，请勿手改。来源：`pkg/strategy/pinespec` 与 `pkg/strategy/pine` capability registry。\n\n")
	writeSupportSnapshotBaseline(&builder, assessment)
	writeSupportSnapshotDimensions(&builder, assessment)
	writeSupportSnapshotCapabilities(&builder)
	writeSupportSnapshotMatrix(&builder)
	writeSupportSnapshotBrokerBoundary(&builder)
	writeSupportSnapshotUnsupportedPatterns(&builder)
	return builder.String()
}

func writeSupportSnapshotBaseline(builder *strings.Builder, assessment strategypine.CompatibilityAssessment) {
	builder.WriteString("## Baseline\n\n")
	builder.WriteString("| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(builder, "| Pine version | `%s` |\n", PineVersion)
	fmt.Fprintf(builder, "| Product version | `%s` |\n", ProductVersion)
	fmt.Fprintf(builder, "| Source format | `%s` |\n", SourceFormat)
	fmt.Fprintf(builder, "| Runtime | `%s` |\n", Runtime)
	fmt.Fprintf(builder, "| External engine | `%s` (`%s`) |\n", pineengine.PinetsShadowEngineID, "off by default")
	fmt.Fprintf(builder, "| Score model | `%s` |\n", assessment.ScoreModelVersion)
	fmt.Fprintf(builder, "| Compatibility score | `%.2f` |\n\n", assessment.Score)
}

func writeSupportSnapshotDimensions(builder *strings.Builder, assessment strategypine.CompatibilityAssessment) {
	builder.WriteString("## Score Dimensions\n\n")
	builder.WriteString("| Dimension | Weight | Score | Supported Weight | Total Weight | Unsupported IDs |\n")
	builder.WriteString("| --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, dimension := range assessment.Dimensions {
		fmt.Fprintf(
			builder,
			"| `%s` | %.2f | %.2f | %.2f | %.2f | %s |\n",
			escapeMarkdownTableCell(dimension.ID),
			dimension.Weight,
			dimension.Score,
			dimension.SupportedWeight,
			dimension.TotalWeight,
			codeList(dimension.UnsupportedIDs),
		)
	}
}

func writeSupportSnapshotCapabilities(builder *strings.Builder) {
	builder.WriteString("\n## Capability Registry\n\n")
	builder.WriteString("| ID | Dimension | Status | Weight | Layers | Tests | Notes |\n")
	builder.WriteString("| --- | --- | --- | ---: | --- | --- | --- |\n")
	for _, capability := range strategypine.CapabilityRegistry() {
		fmt.Fprintf(
			builder,
			"| `%s` | `%s` | `%s` | %.2f | %s | %s | %s |\n",
			escapeMarkdownTableCell(capability.ID),
			escapeMarkdownTableCell(capability.Dimension),
			escapeMarkdownTableCell(string(capability.Status)),
			capability.Weight,
			escapeMarkdownTableCell(capabilityLayerSummary(capability.Layers)),
			codeList(capability.TestIDs),
			escapeMarkdownTableCell(capability.Notes),
		)
	}
}

func writeSupportSnapshotMatrix(builder *strings.Builder) {
	builder.WriteString("\n## Support Matrix\n\n")
	builder.WriteString("| Capability | Parser | Planner | Runtime | JFTrade | Frontend | Notes |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, item := range supportMatrix() {
		fmt.Fprintf(
			builder,
			"| %s | %s | %s | %s | %s | %s | %s |\n",
			escapeMarkdownTableCell(jftradeCheckedTypeAssertion[string](item["capability"])),
			boolMark(jftradeCheckedTypeAssertion[bool](item["parser"])),
			boolMark(jftradeCheckedTypeAssertion[bool](item["planner"])),
			boolMark(jftradeCheckedTypeAssertion[bool](item["runtime"])),
			boolMark(jftradeCheckedTypeAssertion[bool](item["jftrade"])),
			boolMark(jftradeCheckedTypeAssertion[bool](item["frontend"])),
			escapeMarkdownTableCell(jftradeCheckedTypeAssertion[string](item["notes"])),
		)
	}
}

func writeSupportSnapshotBrokerBoundary(builder *strings.Builder) {
	builder.WriteString("\n## Broker Boundary\n\n")
	builder.WriteString("| Area | Status | Score Treatment | Diagnostics | Notes |\n")
	builder.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, item := range brokerBoundary() {
		fmt.Fprintf(
			builder,
			"| %s | `%s` | %s | %s | %s |\n",
			escapeMarkdownTableCell(jftradeCheckedTypeAssertion[string](item["area"])),
			escapeMarkdownTableCell(jftradeCheckedTypeAssertion[string](item["status"])),
			escapeMarkdownTableCell(jftradeCheckedTypeAssertion[string](item["scoreTreatment"])),
			codeList(jftradeCheckedTypeAssertion[[]string](item["diagnosticCodes"])),
			escapeMarkdownTableCell(jftradeCheckedTypeAssertion[string](item["notes"])),
		)
	}
}

func writeSupportSnapshotUnsupportedPatterns(builder *strings.Builder) {
	builder.WriteString("\n## Unsupported Patterns\n\n")
	for _, pattern := range unsupportedPatterns() {
		builder.WriteString("- ")
		builder.WriteString(pattern)
		builder.WriteString("\n")
	}
}

func capabilityLayerSummary(layers strategypine.CapabilityLayers) string {
	enabled := make([]string, 0, 6)
	if layers.Parser {
		enabled = append(enabled, "parser")
	}
	if layers.Planner {
		enabled = append(enabled, "planner")
	}
	if layers.Runtime {
		enabled = append(enabled, "runtime")
	}
	if layers.Backtest {
		enabled = append(enabled, "backtest")
	}
	if layers.Frontend {
		enabled = append(enabled, "frontend")
	}
	if layers.Spec {
		enabled = append(enabled, "spec")
	}
	return strings.Join(enabled, ", ")
}

func codeList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(values))
	for _, value := range values {
		escaped = append(escaped, "`"+escapeMarkdownTableCell(value)+"`")
	}
	return strings.Join(escaped, "<br>")
}

func boolMark(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func escapeMarkdownTableCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\n", "<br>"), "|", "\\|")
}

func BuildExamplesMarkdown() string {
	var builder strings.Builder
	builder.WriteString("# JFTrade Pine Script v6 示例\n\n")
	builder.WriteString("这些示例脚本与 `strategy.pine_spec` 使用同一份规范源生成，预期都能在当前实现下成功 parse、lower 并完成 requirements planning。\n\n")
	builder.WriteString("## 基础示例\n\n")
	for _, example := range examples {
		builder.WriteString("### ")
		builder.WriteString(example.Title)
		builder.WriteString("\n\n")
		builder.WriteString(example.Description)
		builder.WriteString("\n\n```text\n")
		builder.WriteString(example.Script)
		builder.WriteString("\n```\n\n")
	}
	builder.WriteString("## v1.7 黄金脚本\n\n")
	for _, example := range goldenExamples {
		builder.WriteString("### ")
		builder.WriteString(example.Title)
		builder.WriteString("\n\n")
		builder.WriteString(example.Description)
		if len(example.RequirementKeys) > 0 {
			builder.WriteString("\n\nRequirements: `")
			builder.WriteString(strings.Join(example.RequirementKeys, "`, `"))
			builder.WriteString("`")
		}
		builder.WriteString("\n\n```text\n")
		builder.WriteString(example.Script)
		builder.WriteString("\n```\n\n")
	}
	return builder.String()
}

func writeMarkdownSection(builder *strings.Builder, title string, details []string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	for _, detail := range details {
		builder.WriteString("- ")
		builder.WriteString(detail)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func writeMarkdownList(builder *strings.Builder, title string, values []string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	for _, value := range values {
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}
