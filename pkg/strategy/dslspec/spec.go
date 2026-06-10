package dslspec

import (
	"fmt"
	"strings"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategydslruntime "github.com/jftrade/jftrade-main/pkg/strategy/dslruntime"
)

const (
	DSLVersion          = "v1"
	SourceFormat        = strategydefinition.SourceFormatDSLV1
	Runtime             = strategydslruntime.ID
	ToolName            = "strategy.dsl_spec"
	BuiltinSkillName    = "jftrade-strategy"
	BuiltinSkillVersion = "4"
)

type Section struct {
	ID      string
	Title   string
	Summary string
}

type Example struct {
	ID          string
	Title       string
	Description string
	Script      string
}

var sections = []Section{
	{ID: "overview", Title: "概览", Summary: "说明 JFTrade DSL v1 是什么、由哪个 runtime 消费，以及草稿、回测、运行实例之间的边界。"},
	{ID: "syntax", Title: "语法", Summary: "顶层声明、hook 块、缩进规则与注释约定。"},
	{ID: "expressions", Title: "表达式", Summary: "基于 expr 的表达式子语言、已支持的辅助函数与指标字段访问方式。"},
	{ID: "indicators", Title: "指标", Summary: "当前 planner 与 runtime 能识别的指标绑定函数。"},
	{ID: "orders", Title: "下单", Summary: "下单动作、数量模式、policy 以及市价/限价行为。"},
	{ID: "protect", Title: "保护", Summary: "protect 语句语法，以及 direction、mode、unit、window 的支持范围。"},
	{ID: "examples", Title: "示例", Summary: "当前实现下可以成功 parse 并完成 requirements planning 的完整脚本。"},
}

var examples = []Example{
	{
		ID:          "minimal-log",
		Title:       "最小可保存草稿",
		Description: "可保存为 JFTrade DSL v1 策略定义的最小完整脚本。",
		Script: `strategy Minimal Draft
version 0.1.0
symbol US.TME
interval 1m

on kline_close:
  log "ready"`,
	},
	{
		ID:          "ema-crossover",
		Title:       "EMA 均线交叉",
		Description: "一个基础均线交叉脚本：在 init 时记录日志，并在快 EMA 上穿慢 EMA 时开多。",
		Script: `strategy EMA Crossover
version 0.1.0
symbol US.TME
interval 5m

on init:
  log "EMA crossover booted"

on kline_close:
  let fast = ma(EMA, 8)
  let slow = ma(EMA, 21)
  if cross_over(fast, slow):
    buy cash_percent 25 policy flat_only
  else:
    notify "waiting for next crossover"`,
	},
	{
		ID:          "rsi-protect",
		Title:       "RSI 与 protect",
		Description: "一个均值回归草稿：在 RSI 超卖时入场，并设置 session 级别止损保护。",
		Script: `strategy RSI Protect
version 0.1.0
symbol US.TME
interval 15m

on kline_close:
  let rsi14 = rsi(14)
  if rsi14 < 30:
    buy shares 100 policy same_direction
    protect auto stop_loss 1 day 5% window session
  else:
    log "RSI condition not met"`,
	},
}

func Sections() []Section {
	out := make([]Section, len(sections))
	copy(out, sections)
	return out
}

func Examples() []Example {
	out := make([]Example, len(examples))
	copy(out, examples)
	return out
}

func NormalizeSection(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func AllowedSections() []string {
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		out = append(out, section.ID)
	}
	return out
}

func SkillDescription() string {
	return "谨慎使用 JFTrade 策略工具；在起草、校验或保存策略定义前，先查阅内置的 DSL v1 规范。"
}

func SkillInstructions() string {
	return strings.Join([]string{
		"处理策略相关任务时，要明确区分策略想法、策略草稿、已保存定义、回测结果和正在运行的策略实例。",
		"起草、校验或保存策略前，先读取 references/dsl-v1-spec.md；需要完整脚本时读取 references/dsl-v1-examples.md；需要结构化摘要时调用 strategy.dsl_spec。",
		"只能输出当前真实支持的 JFTrade DSL v1，不要输出 TradingView Pine Script 语法，例如 //@version、strategy(...) 或 indicator(...)。",
		"如果用户询问 DSL v1 的定义或语法，必须依据内置规范回答，不要杜撰未支持的 hook、语句、函数或订单选项。",
		"脚本还不完整时先用 strategy.validate_dsl 校验；明确的新建或更新流程用 strategy.save_definition；只有在用户明确要求修改某个具体实例执行模式时才用 strategy.update_instance_mode。",
		"不要承诺收益；优化和写入类操作属于受权限约束的动作，必须遵守当前审批模式。",
	}, " ")
}

func SkillAllowedTools() []string {
	return []string{
		"strategy.definitions",
		ToolName,
		"strategy.validate_dsl",
		"strategy.save_draft",
		"strategy.save_definition",
		"strategy.update_instance_mode",
		"backtest.runs",
		"strategy.optimize",
	}
}

func SkillResourceFiles() map[string]string {
	return map[string]string{
		"references/dsl-v1-spec.md":     BuildSpecMarkdown(),
		"references/dsl-v1-examples.md": BuildExamplesMarkdown(),
	}
}

func SaveDraftUsageHint() string {
	return "可以先查询看 DSL 规范和示例，确认脚本格式正确。也可以从下面这个 JFTrade DSL v1 骨架开始：\n" + Skeleton()
}

func Skeleton() string {
	return examples[0].Script
}

func BuildToolPayload(section string, includeExamples bool) (map[string]any, error) {
	normalizedSection := NormalizeSection(section)
	if normalizedSection != "" && !isKnownSection(normalizedSection) {
		return nil, fmt.Errorf("strategy.dsl_spec 不支持 section %q（可选值：%s）", section, strings.Join(AllowedSections(), ", "))
	}

	payload := map[string]any{
		"version":                     DSLVersion,
		"sourceFormat":                SourceFormat,
		"runtime":                     Runtime,
		"sections":                    sectionPayloads(),
		"selectedSection":             normalizedSection,
		"supportedTopLevelStatements": supportedTopLevelStatements(),
		"supportedHooks":              supportedHooks(),
		"supportedStatements":         supportedStatements(),
		"reservedVariables":           reservedVariables(),
		"indicatorFunctions":          indicatorFunctions(),
		"orderModes":                  orderModes(),
		"protectModes":                protectModes(),
		"unsupportedPatterns":         unsupportedPatterns(),
		"skeleton":                    Skeleton(),
		"examples":                    []map[string]any{},
	}

	if normalizedSection != "" {
		payload["sectionContent"] = sectionContent(normalizedSection)
	}
	if includeExamples || normalizedSection == "examples" {
		payload["examples"] = examplePayloads()
	}

	return payload, nil
}

func BuildSpecMarkdown() string {
	var builder strings.Builder
	builder.WriteString("# JFTrade DSL v1 规范\n\n")
	builder.WriteString("本文档描述当前 parser、planner 与 runtime 真正接受的 JFTrade DSL v1 语法范围。\n\n")
	builder.WriteString("- `sourceFormat`: `")
	builder.WriteString(SourceFormat)
	builder.WriteString("`\n")
	builder.WriteString("- `runtime`: `")
	builder.WriteString(Runtime)
	builder.WriteString("`\n")
	builder.WriteString("- `dslVersion`: `")
	builder.WriteString(DSLVersion)
	builder.WriteString("`\n\n")

	writeMarkdownSection(&builder, "概览", sectionDetails("overview"))
	writeMarkdownSection(&builder, "语法", sectionDetails("syntax"))
	writeMarkdownList(&builder, "支持的顶层语句", supportedTopLevelStatements())
	writeMarkdownList(&builder, "支持的 hooks", supportedHooks())
	writeMarkdownList(&builder, "支持的语句", supportedStatements())
	writeMarkdownSection(&builder, "表达式", sectionDetails("expressions"))
	writeMarkdownSection(&builder, "指标", sectionDetails("indicators"))
	writeMarkdownList(&builder, "保留变量", flattenNamedItems(reservedVariables()))
	writeMarkdownSection(&builder, "下单", sectionDetails("orders"))
	writeMarkdownList(&builder, "数量与下单模式", flattenNamedItems(orderModes()))
	writeMarkdownSection(&builder, "保护", sectionDetails("protect"))
	writeMarkdownList(&builder, "protect 支持项", flattenNamedItems(protectModes()))
	writeMarkdownList(&builder, "明确不支持的写法", unsupportedPatterns())
	builder.WriteString("## 最小骨架\n\n```text\n")
	builder.WriteString(Skeleton())
	builder.WriteString("\n```\n")

	return builder.String()
}

func BuildExamplesMarkdown() string {
	var builder strings.Builder
	builder.WriteString("# JFTrade DSL v1 示例\n\n")
	builder.WriteString("这些示例脚本与 `strategy.dsl_spec` 使用同一份规范源生成，预期都能在当前实现下成功 parse 并完成 requirements planning。\n\n")
	for _, example := range examples {
		builder.WriteString("## ")
		builder.WriteString(example.Title)
		builder.WriteString("\n\n")
		builder.WriteString(example.Description)
		builder.WriteString("\n\n```text\n")
		builder.WriteString(example.Script)
		builder.WriteString("\n```\n\n")
	}
	return builder.String()
}

func supportedTopLevelStatements() []string {
	return []string{
		"strategy <name>",
		"version <text>",
		"symbol <market.symbol>",
		"interval <timeframe>",
	}
}

func supportedHooks() []string {
	return []string{
		"on init:",
		"on kline_close:",
	}
}

func supportedStatements() []string {
	return []string{
		"let <name> = <expression>",
		"if <condition>:",
		"else:",
		"log <string>",
		"notify <string>",
		"buy <mode> <expression> [policy <policy>] [limit <expression>] [type <MARKET|LIMIT>]",
		"sell <mode> <expression> [limit <expression>] [type <MARKET|LIMIT>]",
		"short <mode> <expression> [policy <policy>] [limit <expression>] [type <MARKET|LIMIT>]",
		"cover <mode> <expression> [limit <expression>] [type <MARKET|LIMIT>]",
		"protect <direction> <mode> <timeValue> <timeUnit> <percentage> [window <continuous|session>]",
	}
}

func reservedVariables() []map[string]any {
	return []map[string]any{
		{"name": "close", "description": "当前及历史 close 序列值，可用于比较和 cross 类辅助函数。"},
		{"name": "open", "description": "当前 bar 的开盘价。"},
		{"name": "high", "description": "当前 bar 的最高价。"},
		{"name": "low", "description": "当前 bar 的最低价。"},
		{"name": "volume", "description": "当前 bar 的成交量。"},
		{"name": "kline", "description": "当前 K 线载荷视图。"},
		{"name": "indicators", "description": "runtime 已规划指标值的快照容器。"},
	}
}

func indicatorFunctions() []map[string]any {
	return []map[string]any{
		{"name": "ma", "signature": "ma(type, period, unit?)", "notes": "支持类型：MA、EMA、SMA、SMMA、LWMA、TMA、EXPMA、HMA、VWMA、BOLL。可选 unit：bar、minute、hour、day、week、month。"},
		{"name": "rsi", "signature": "rsi(period)", "notes": "period 必须是正整数。"},
		{"name": "macd", "signature": "macd(fast, slow, signal)", "notes": "可访问 .diff、.signal、.histogram 等字段。"},
		{"name": "kdj", "signature": "kdj(periodK, periodD, smooth)", "notes": "可访问 .k、.d、.j 等字段。"},
		{"name": "atr", "signature": "atr(period)", "notes": "period 必须是正整数。"},
		{"name": "cci", "signature": "cci(period)", "notes": "period 必须是正整数。"},
		{"name": "williams_r", "signature": "williams_r(period)", "notes": "runtime 同时接受别名 williamsr(period)。"},
		{"name": "bollinger", "signature": "bollinger(period, multiplier)", "notes": "可访问 .upper、.middle、.lower 等字段。"},
	}
}

func orderModes() []map[string]any {
	return []map[string]any{
		{"name": "shares", "description": "按股数表达式下单。"},
		{"name": "amount", "description": "按现金金额表达式下单。"},
		{"name": "cash_percent", "description": "按可用现金百分比下单。"},
		{"name": "margin_buying_power_percent", "description": "按融资购买力百分比下单。"},
		{"name": "short_selling_power_percent", "description": "按融券卖空能力百分比下单。"},
		{"name": "symbol_position_percent", "description": "按当前标的持仓百分比下单；parser 同时接受别名 position_percent。"},
		{"name": "account_position_percent", "description": "按账户总持仓市值百分比下单。"},
	}
}

func protectModes() []map[string]any {
	return []map[string]any{
		{"name": "stopLoss", "description": "用于止损；parser 同时接受 stop_loss。"},
		{"name": "takeProfit", "description": "用于止盈；parser 同时接受 take_profit。"},
		{"name": "trailingStop", "description": "用于追踪止损；parser 同时接受 trailing_stop。"},
		{"name": "direction", "description": "支持值：auto、long、short。"},
		{"name": "timeUnit", "description": "支持值：bar、minute、hour、day、week、month。"},
		{"name": "window", "description": "支持值：continuous、session。"},
	}
}

func unsupportedPatterns() []string {
	return []string{
		"TradingView Pine Script 标记和语法，例如 //@version、strategy(...)、indicator(...)、plot(...) 或 ta.* 辅助函数。",
		"除 strategy、version、symbol、interval 和 hook 声明之外的顶层语句。",
		"除 on init: 与 on kline_close: 之外的 hook 名称。",
		"除 policy、limit、type 之外的订单选项。",
		"除 cross_over、cross_under、divergence_top、divergence_bottom、abs 之外的表达式辅助函数。",
		"除 ma、rsi、macd、kdj、atr、cci、williams_r/williamsr、bollinger 之外的指标绑定函数。",
	}
}

func sectionPayloads() []map[string]any {
	out := make([]map[string]any, 0, len(sections))
	for _, section := range sections {
		out = append(out, map[string]any{
			"id":      section.ID,
			"title":   section.Title,
			"summary": section.Summary,
		})
	}
	return out
}

func examplePayloads() []map[string]any {
	out := make([]map[string]any, 0, len(examples))
	for _, example := range examples {
		out = append(out, map[string]any{
			"id":          example.ID,
			"title":       example.Title,
			"description": example.Description,
			"script":      example.Script,
		})
	}
	return out
}

func sectionContent(section string) map[string]any {
	return map[string]any{
		"id":      section,
		"title":   sectionTitle(section),
		"summary": sectionSummary(section),
		"details": sectionDetails(section),
	}
}

func sectionTitle(section string) string {
	for _, item := range sections {
		if item.ID == section {
			return item.Title
		}
	}
	return section
}

func sectionSummary(section string) string {
	for _, item := range sections {
		if item.ID == section {
			return item.Summary
		}
	}
	return ""
}

func sectionDetails(section string) []string {
	switch section {
	case "overview":
		return []string{
			"JFTrade DSL v1 是面向行的策略语言，会被编译到 dsl-go-plan runtime。",
			"已保存草稿、回测结果和正在运行的策略实例必须视为不同工作状态，不能混为一谈。",
			"只有当前 parser、planner 和 runtime 的真实行为才属于规范范围，不要臆造未来 DSL 语法。",
		}
	case "syntax":
		return []string{
			"空行与以 # 开头的行会被忽略。",
			"缩进语句必须位于 hook 块或 if/else 块内部。",
			"每个 hook 块至少要有一条缩进语句。",
			"else: 必须紧跟在同一缩进层级的 if 块之后。",
		}
	case "expressions":
		return []string{
			"表达式子语言由 expr 解析，因此在 runtime 理解对应值的前提下，可以使用常见的算术、比较、布尔、字符串和标识符语法。",
			"已支持的辅助调用包括 cross_over(left, right)、cross_under(left, right)、divergence_top(alias, lookback)、divergence_bottom(alias, lookback) 和 abs(value)。",
			"指标别名可以直接作为序列值参与比较；结构化指标可访问 macdAlias.diff、macdAlias.signal、bollAlias.upper、kdjAlias.j 等字段。",
			"未知辅助函数可能能通过表达式解析，但仍会在 runtime 失败，因此只应使用文档明确列出的辅助调用。",
		}
	case "indicators":
		return []string{
			"指标绑定通过 let <alias> = <functionCall> 声明。",
			"planner 当前识别 ma、rsi、macd、kdj、atr、cci、williams_r/williamsr、bollinger。",
			"ma() 的第三个可选参数是时间单位；留空表示按 bar 维度计算。",
		}
	case "orders":
		return []string{
			"支持的动作有 buy、sell、short、cover。",
			"支持的入场 policy 有 same_direction、flat_only、allow；不支持的值会在 runtime 中归一化为 same_direction。",
			"若出现 limit <expr> 且未显式写 type，runtime 会按 LIMIT 处理；否则默认是 MARKET。",
			"parser 接受 type 选项，但 runtime 当前只区分 LIMIT 与“其他类型”。",
		}
	case "protect":
		return []string{
			"语法为 protect <direction> <mode> <timeValue> <timeUnit> <percentage> [window <continuous|session>]。",
			"timeValue 必须是正整数；percentage 必须是正数，可以带结尾百分号。",
			"direction 支持 auto、long、short；mode 支持 stopLoss、takeProfit、trailingStop 及其下划线别名。",
		}
	case "examples":
		return []string{
			"这些示例脚本与内置 skill 资源和 strategy.dsl_spec 输出共用同一份规范源。",
			"这些示例旨在保证当前实现下可以成功 parse 并完成 requirements planning。",
		}
	default:
		return nil
	}
}

func isKnownSection(section string) bool {
	for _, item := range sections {
		if item.ID == section {
			return true
		}
	}
	return false
}

func flattenNamedItems(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		description, _ := item["description"].(string)
		if description == "" {
			notes, _ := item["notes"].(string)
			description = notes
		}
		if description == "" {
			out = append(out, name)
			continue
		}
		out = append(out, name+": "+description)
	}
	return out
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
