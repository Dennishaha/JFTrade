package pinespec

import (
	"fmt"
	"strings"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategypineruntime "github.com/jftrade/jftrade-main/pkg/strategy/pineruntime"
)

const (
	PineVersion         = "v6"
	SourceFormat        = strategydefinition.SourceFormatPineV6
	Runtime             = strategypineruntime.ID
	ToolName            = "strategy.pine_spec"
	BuiltinSkillName    = "jftrade-strategy"
	BuiltinSkillVersion = "5"
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
	{ID: "overview", Title: "概览", Summary: "说明 JFTrade Pine Script v6 前端、pine-go-plan runtime，以及草稿、回测、运行实例之间的边界。"},
	{ID: "syntax", Title: "语法", Summary: "Pine v6 声明、缩进块、注释和当前可执行子集。"},
	{ID: "expressions", Title: "表达式", Summary: "支持的 Pine 表达式、OHLCV 序列和函数映射。"},
	{ID: "indicators", Title: "指标", Summary: "当前 compiler、planner 与 runtime 能识别的 ta.* 指标。"},
	{ID: "orders", Title: "下单", Summary: "strategy.entry/strategy.close 到 JFTrade 订单 IR 的映射。"},
	{ID: "unsupported", Title: "不支持项", Summary: "已解析但不能在 JFTrade 中执行的 Pine v6 行为。"},
	{ID: "examples", Title: "示例", Summary: "当前实现下可以成功 parse、lower 并完成 requirements planning 的 Pine v6 脚本。"},
}

var examples = []Example{
	{
		ID:          "minimal-log",
		Title:       "最小可保存草稿",
		Description: "可保存为 JFTrade Pine Script v6 策略定义的最小完整脚本。",
		Script: `//@version=6
strategy("Minimal Draft", overlay=true)

log.info("ready")`,
	},
	{
		ID:          "ema-crossover",
		Title:       "EMA 均线交叉",
		Description: "一个基础均线交叉脚本：快 EMA 上穿慢 EMA 时开多。",
		Script: `//@version=6
strategy("EMA Crossover", overlay=true)

fast = ta.ema(close, 8)
slow = ta.ema(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)
else
    alert("waiting for next crossover")`,
	},
	{
		ID:          "rsi-protect",
		Title:       "RSI 与 protect",
		Description: "一个均值回归草稿：在 RSI 超卖时入场。",
		Script: `//@version=6
strategy("RSI Reversion", overlay=true)

rsi14 = ta.rsi(close, 14)
if rsi14 < 30
    strategy.entry("Long", strategy.long, qty=100)
else
    log.info("RSI condition not met")`,
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
	return "谨慎使用 JFTrade 策略工具；在起草、校验或保存策略定义前，先查阅内置的 Pine Script v6 规范。"
}

func SkillInstructions() string {
	return strings.Join([]string{
		"处理策略相关任务时，要明确区分策略想法、策略草稿、已保存定义、回测结果和正在运行的策略实例。",
		"起草、校验或保存策略前，先读取 references/pine-v6-spec.md；需要完整脚本时读取 references/pine-v6-examples.md；需要结构化摘要时调用 strategy.pine_spec。",
		"只能输出当前 JFTrade 支持的 Pine Script v6 策略脚本；必须包含 //@version=6 和 strategy(...)。",
		"如果用户询问 Pine 支持范围，必须依据内置规范回答，不要杜撰未支持的 built-ins、订单选项或 TradingView broker emulator 行为。",
		"脚本还不完整时先用 strategy.validate_pine 校验；明确的新建或更新流程用 strategy.save_definition；只有在用户明确要求修改某个具体实例执行模式时才用 strategy.update_instance_mode。",
		"不要承诺收益；优化和写入类操作属于受权限约束的动作，必须遵守当前审批模式。",
	}, " ")
}

func SkillAllowedTools() []string {
	return []string{
		"strategy.definitions",
		ToolName,
		"strategy.validate_pine",
		"strategy.save_draft",
		"strategy.save_definition",
		"strategy.update_instance_mode",
		"backtest.runs",
		"strategy.optimize",
	}
}

func SkillResourceFiles() map[string]string {
	return map[string]string{
		"references/pine-v6-spec.md":     BuildSpecMarkdown(),
		"references/pine-v6-examples.md": BuildExamplesMarkdown(),
	}
}

func SaveDraftUsageHint() string {
	return "可以先查询 Pine v6 规范和示例，确认脚本格式正确。也可以从下面这个 JFTrade Pine v6 骨架开始：\n" + Skeleton()
}

func Skeleton() string {
	return examples[0].Script
}

func BuildToolPayload(section string, includeExamples bool) (map[string]any, error) {
	normalizedSection := NormalizeSection(section)
	if normalizedSection != "" && !isKnownSection(normalizedSection) {
		return nil, fmt.Errorf("strategy.pine_spec 不支持 section %q（可选值：%s）", section, strings.Join(AllowedSections(), ", "))
	}

	payload := map[string]any{
		"version":                     PineVersion,
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
	builder.WriteString("# JFTrade Pine Script v6 规范\n\n")
	builder.WriteString("本文档描述当前 Pine parser、lowerer、planner 与 runtime 真正接受的 JFTrade Pine v6 语法范围。\n\n")
	builder.WriteString("- `sourceFormat`: `")
	builder.WriteString(SourceFormat)
	builder.WriteString("`\n")
	builder.WriteString("- `runtime`: `")
	builder.WriteString(Runtime)
	builder.WriteString("`\n")
	builder.WriteString("- `pineVersion`: `")
	builder.WriteString(PineVersion)
	builder.WriteString("`\n\n")

	writeMarkdownSection(&builder, "概览", sectionDetails("overview"))
	writeMarkdownSection(&builder, "语法", sectionDetails("syntax"))
	writeMarkdownList(&builder, "支持的顶层语句", supportedTopLevelStatements())
	writeMarkdownList(&builder, "支持的语句", supportedStatements())
	writeMarkdownSection(&builder, "表达式", sectionDetails("expressions"))
	writeMarkdownSection(&builder, "指标", sectionDetails("indicators"))
	writeMarkdownList(&builder, "保留变量", flattenNamedItems(reservedVariables()))
	writeMarkdownSection(&builder, "下单", sectionDetails("orders"))
	writeMarkdownList(&builder, "数量与下单模式", flattenNamedItems(orderModes()))
	writeMarkdownSection(&builder, "不支持项", sectionDetails("unsupported"))
	writeMarkdownList(&builder, "明确不支持的写法", unsupportedPatterns())
	builder.WriteString("## 最小骨架\n\n```text\n")
	builder.WriteString(Skeleton())
	builder.WriteString("\n```\n")

	return builder.String()
}

func BuildExamplesMarkdown() string {
	var builder strings.Builder
	builder.WriteString("# JFTrade Pine Script v6 示例\n\n")
	builder.WriteString("这些示例脚本与 `strategy.pine_spec` 使用同一份规范源生成，预期都能在当前实现下成功 parse、lower 并完成 requirements planning。\n\n")
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
		"//@version=6",
		"strategy(\"<name>\", overlay=true)",
		"<name> = <expression>",
		"if <condition>",
		"strategy.entry(\"<id>\", strategy.long|strategy.short, qty=<expression>[, limit=<expression>])",
		"strategy.close(\"<id>\"[, qty=<expression>][, limit=<expression>])",
	}
}

func supportedHooks() []string {
	return []string{
		"JFTrade 将可执行 Pine 语句统一映射到 K 线收盘执行。",
	}
}

func supportedStatements() []string {
	return []string{
		"<name> = ta.ema(close, period) / ta.sma(close, period) / ta.rsi(close, period)",
		"var <name> = <expression> / <name> := <expression>",
		"<series>[1] 一阶历史引用；可配合 nz(<series>[1], fallback)",
		"<condition> ? <trueExpr> : <falseExpr>",
		"[macdLine, signalLine, histLine] = ta.macd(close, fast, slow, signal)",
		"if ta.crossover(left, right) / if ta.crossunder(left, right)",
		"else",
		"strategy.entry(\"Long\", strategy.long, qty=1)",
		"strategy.entry(\"Long\", strategy.long, qty=(strategy.equity * 25 / 100) / close)",
		"strategy.entry(\"Short\", strategy.short, qty=1)",
		"strategy.close(\"Long\") / strategy.close(\"Short\", qty=50)",
		"alert(\"message\") / log.info(\"message\")",
	}
}

func reservedVariables() []map[string]any {
	return []map[string]any{
		{"name": "close", "description": "当前及历史 close 序列值，可用于比较和 cross 类辅助函数。"},
		{"name": "open", "description": "当前及历史 open 序列值。"},
		{"name": "high", "description": "当前及历史 high 序列值。"},
		{"name": "low", "description": "当前及历史 low 序列值。"},
		{"name": "volume", "description": "当前及历史 volume 序列值。"},
		{"name": "kline", "description": "当前 K 线载荷视图。"},
		{"name": "strategy.equity", "description": "用于下单数量表达式时按账户权益百分比估算股数。"},
		{"name": "strategy.position_size", "description": "当前策略持仓数量；多头为正，空头为负，空仓为 0。"},
		{"name": "strategy.position_avg_price", "description": "当前策略持仓均价；空仓时为 na。"},
	}
}

func indicatorFunctions() []map[string]any {
	return []map[string]any{
		{"name": "ta.ema", "signature": "ta.ema(close, period)", "notes": "lower 到 JFTrade EMA 指标。"},
		{"name": "ta.sma", "signature": "ta.sma(close, period)", "notes": "lower 到 JFTrade SMA 指标。"},
		{"name": "ta.rsi", "signature": "ta.rsi(close, period)", "notes": "lower 到 JFTrade RSI 指标。"},
		{"name": "ta.macd", "signature": "ta.macd(close, fast, slow, signal)", "notes": "支持三元组赋值，signal/hist 变量会映射到 MACD 字段。"},
		{"name": "ta.atr", "signature": "ta.atr(period)", "notes": "lower 到 JFTrade ATR 指标。"},
		{"name": "ta.stdev", "signature": "ta.stdev(close, period)", "notes": "lower 到 JFTrade rolling standard deviation 指标。"},
		{"name": "ta.cci", "signature": "ta.cci(close, period)", "notes": "lower 到 JFTrade CCI 指标。"},
		{"name": "ta.crossover", "signature": "ta.crossover(left, right)", "notes": "lower 到 cross_over。"},
		{"name": "ta.crossunder", "signature": "ta.crossunder(left, right)", "notes": "lower 到 cross_under。"},
	}
}

func orderModes() []map[string]any {
	return []map[string]any{
		{"name": "strategy.entry qty", "description": "按股数表达式开多或开空。"},
		{"name": "strategy.entry amount", "description": "固定金额可写为 qty=amount/close。"},
		{"name": "strategy.entry equity percent", "description": "账户权益百分比可写为 qty=(strategy.equity*pct/100)/close。"},
		{"name": "strategy.close", "description": "平仓；不指定数量默认平 100% 持仓，可传入 qty 按股数或金额表达式部分平仓。"},
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
		"indicator()、study()、library() 脚本不能作为 JFTrade 可执行策略。",
		"request.security() 仅支持 syminfo.tickerid + ta.sma/ema/rma/wma/hma/vwma(close, n) 的受限多周期均线子集。",
		"array.*、matrix.*、map.* 集合命名空间暂不支持。",
		"for/while/switch、用户自定义函数、type/method 会被解析为明确诊断，但本版本不可执行。",
		"历史引用仅支持一阶 `[1]`；更深 lookback 暂不支持。",
		"strategy.exit() 支持基础 stop、limit、trail_points/trail_offset；高级 broker emulator 语义暂不支持。",
		"plot/hline/bgcolor/barcolor 等视觉调用会被解析为 warning 并忽略。",
		"除文档列出的 ta.*、math.abs、strategy.entry、strategy.close、alert/log 外的 built-ins 不应假定可执行。",
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
			"JFTrade Pine Script v6 前端会把支持的 Pine 策略语句 lower 到 pine-go-plan runtime。",
			"已保存草稿、回测结果和正在运行的策略实例必须视为不同工作状态，不能混为一谈。",
			"当前目标是完整解析 Pine v6 常见语法，并对 JFTrade 暂不能执行的语义给出明确诊断。",
		}
	case "syntax":
		return []string{
			"脚本必须包含 //@version=6 和 strategy(...)。",
			"空行与普通 // 注释会被忽略；// @jftradeFlow* 注释用于前端流程图双向同步。",
			"if/else 使用 Pine 风格缩进块；顶层可执行语句统一按 K 线收盘逻辑 lower。",
			"支持 var 持久变量、:= 重赋值、基础三元表达式、一阶历史引用和 na/nz。",
			"JFTrade 会把顶层可执行语句作为 K 线收盘逻辑执行。",
		}
	case "expressions":
		return []string{
			"支持 close/open/high/low/volume、算术、比较和布尔表达式。",
			"close[1]/open[1]/high[1]/low[1]/volume[1] 会 lower 为上一根 K 线值。",
			"条件表达式要求严格 bool；数值不能直接作为 if 条件。",
			"支持 na 常量、nz(value, fallback?) 和基础三元表达式。",
			"ta.crossover/ta.crossunder 会映射到 JFTrade cross_over/cross_under。",
			"math.abs 会映射到 JFTrade abs。",
			"未知 built-ins 可能无法 lower，应先调用 strategy.validate_pine。",
		}
	case "indicators":
		return []string{
			"指标绑定通过 <alias> = ta.<function>(...) 声明。",
			"compiler 当前识别 ta.sma、ta.ema、ta.rma、ta.wma、ta.hma、ta.vwma、ta.rsi、ta.macd、ta.atr、ta.cci、ta.crossover、ta.crossunder。",
			"分钟/小时/日/周/月均线使用 request.security(syminfo.tickerid, timeframe, ta.*(close, n)) 的受限子集。",
			"ta.macd 支持 [macdLine, signalLine, histLine] 三元组赋值。",
		}
	case "orders":
		return []string{
			"strategy.entry(id, strategy.long, qty=n) 映射为买入开多。",
			"strategy.entry(id, strategy.short, qty=n) 映射为卖出开空。",
			"固定金额可写 qty=amount/close，账户权益百分比可写 qty=(strategy.equity*pct/100)/close。",
			"strategy.entry(..., limit=price) 映射为基础限价开仓。",
			"strategy.close(id, qty=n, limit=price) 根据已知 entry id 映射为平多或平空，支持部分平仓与限价。",
			"strategy.exit(id, from_entry, stop/limit/trail_points/trail_offset=...) 映射为基础止损、止盈或追踪止损。",
		}
	case "unsupported":
		return []string{
			"plot/hline/bgcolor/barcolor 等视觉调用会返回 warning 并忽略。",
			"for/while/switch、用户自定义函数和 Pine 类型系统会返回结构化诊断。",
			"除受限均线子集以外的 request.security、import/library、array/matrix/map 会返回错误。",
			"strategy.exit 的 OCA、partial fill、intrabar broker emulator 等高级语义会给出明确诊断。",
			"完整 TradingView broker emulator 行为不属于当前 JFTrade runtime。",
		}
	case "examples":
		return []string{
			"这些示例脚本与内置 skill 资源和 strategy.pine_spec 输出共用同一份规范源。",
			"这些示例旨在保证当前实现下可以成功 parse、lower 并完成 requirements planning。",
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
