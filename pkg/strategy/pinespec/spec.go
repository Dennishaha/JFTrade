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
	ID              string
	Title           string
	Description     string
	Script          string
	RequirementKeys []string
}

var sections = []Section{
	{ID: "overview", Title: "概览", Summary: "说明 JFTrade Pine Script v6 前端、pine-go-plan runtime，以及草稿、回测、运行实例之间的边界。"},
	{ID: "syntax", Title: "语法", Summary: "Pine v6 声明、缩进块、注释和当前可执行子集。"},
	{ID: "expressions", Title: "表达式", Summary: "支持的 Pine 表达式、OHLCV 序列和函数映射。"},
	{ID: "indicators", Title: "指标", Summary: "当前 compiler、planner 与 runtime 能识别的 ta.* 指标。"},
	{ID: "orders", Title: "下单", Summary: "strategy.entry/strategy.close 到 JFTrade 订单 IR 的映射。"},
	{ID: "support-matrix", Title: "支持矩阵", Summary: "按 parser、planner、runtime、JFTrade 集成和前端覆盖当前 v0.8 能力。"},
	{ID: "compatibility", Title: "兼容迁移", Summary: "列出长期保留、v1.0 前迁移和可删除的兼容层。"},
	{ID: "unsupported", Title: "不支持项", Summary: "已解析但不能在 JFTrade 中执行的 Pine v6 行为。"},
	{ID: "examples", Title: "示例", Summary: "当前实现下可以成功 parse、lower 并完成 requirements planning 的 Pine v6 脚本。"},
}

var examples = []Example{
	{
		ID:          "minimal-log",
		Title:       "最小可保存草稿",
		Description: "可保存为 JFTrade Pine Script v6 策略定义的最小完整脚本。",
		Script: `//@version=6
strategy("Minimal Draft", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)

log.info("ready")`,
	},
	{
		ID:          "ema-crossover",
		Title:       "EMA 均线交叉",
		Description: "一个基础均线交叉脚本：快 EMA 上穿慢 EMA 时开多。",
		Script: `//@version=6
strategy("EMA Crossover", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)

fast = ta.ema(close, 8)
slow = ta.ema(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long)
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
	{
		ID:          "v08-golden-capability-set",
		Title:       "v0.8 能力黄金脚本",
		Description: "覆盖当前 v0.8 锁定能力的可执行 smoke：source-aware 指标、MTF、SAR、UDF、静态 for、qty_percent、net order、bracket exit 和 cancel。",
		Script: `//@version=6
strategy("v0.8 Golden", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)

len = input.int(3, "Length")
tf = input.timeframe("15", "MTF")
isBull(src) => src > src[1]

fast = ta.ema(close, len)
avgVol = ta.sma(volume, 2)
sar = ta.sar(0.02, 0.02, 0.2)
mtfClose = request.security(syminfo.tickerid, tf, close)
mtfEma = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 3))
sum = 0
for i = 0 to 2
    sum := sum + nz(close[i], close)

if barstate.isconfirmed and session.ismarket and isBull(close) and close > fast and volume > avgVol and close > sar and mtfClose > mtfEma and sum > 0
    strategy.entry("Long", strategy.long, qty_percent=10)
    strategy.order("Net", strategy.long, qty=1)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04, qty_percent=50)
else
    strategy.cancel("Long")`,
	},
}

var goldenExamples = []Example{
	{
		ID:          "golden-ma-cross",
		Title:       "均线交叉",
		Description: "覆盖 close-source EMA/SMA、crossover 和基础 entry。",
		Script: `//@version=6
strategy("Golden MA Cross", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)

fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long)`,
		RequirementKeys: []string{"ma:EMA:8", "ma:SMA:21"},
	},
	{
		ID:          "golden-oscillators-bands",
		Title:       "RSI/CCI/Williams/Bollinger",
		Description: "覆盖常见震荡指标、Bollinger 三元组和 close/hlc3 legacy key。",
		Script: `//@version=6
strategy("Golden Oscillators", overlay=true)

rsi14 = ta.rsi(close, 14)
cci20 = ta.cci(hlc3, 20)
williams = ta.wpr(14)
[basis, upper, lower] = ta.bb(close, 20, 2)
if rsi14 < 35 and cci20 < -100 and williams < -80 and close < lower
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"rsi:14", "cci:20", "williamsr:14", "bollinger:20:2"},
	},
	{
		ID:          "golden-donchian-volume-sar",
		Title:       "Donchian、volume MA 与 SAR",
		Description: "覆盖 rolling highest/lowest、source-aware volume SMA 和 Parabolic SAR。",
		Script: `//@version=6
strategy("Golden Donchian Volume SAR", overlay=true)

upper = ta.highest(high, 20)
lower = ta.lowest(low, 20)
avgVol = ta.sma(volume, 10)
sar = ta.sar(0.02, 0.02, 0.2)
if close > upper and volume > avgVol and close > sar
    strategy.entry("Long", strategy.long, qty=1)
if close < lower
    strategy.close("Long")`,
		RequirementKeys: []string{"highest:high:20", "lowest:low:20", "ma:SMA:10:volume", "sar:0.02:0.02:0.2"},
	},
	{
		ID:          "golden-mtf-source-ma",
		Title:       "MTF source 与 MTF MA",
		Description: "覆盖 input.timeframe、request.security source/source[n] 与 source-aware MTF EMA。",
		Script: `//@version=6
strategy("Golden MTF", overlay=true)

tf = input.timeframe("15", "Signal TF")
mtfClose = request.security(syminfo.tickerid, tf, close)
mtfPrevClose = request.security(syminfo.tickerid, tf, close[1])
mtfEma = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 3))
if mtfClose > mtfPrevClose and close > mtfEma
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"security_source:15m:close", "security_source:15m:close:1", "ma:EMA:3:15m:hlc3"},
	},
	{
		ID:          "golden-orders-exits",
		Title:       "qty_percent、pending、bracket、cancel",
		Description: "覆盖 percent sizing、strategy.order、pending stop、bracket exit 和 cancel。",
		Script: `//@version=6
strategy("Golden Orders", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)

if close > open
    strategy.entry("Long", strategy.long, qty_percent=10)
    strategy.order("NetLong", strategy.long, qty=1)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04, qty_percent=50)
else
    strategy.entry("Breakout", strategy.long, stop=high + 1, qty=1)
    strategy.cancel("Breakout")`,
		RequirementKeys: []string{},
	},
	{
		ID:          "golden-udf-static-for",
		Title:       "UDF 与静态 for",
		Description: "覆盖单表达式 UDF、历史引用、input.int 默认值和静态 for 展开。",
		Script: `//@version=6
strategy("Golden UDF Static For", overlay=true)

isBull(src) => src > src[1]
len = input.int(3, "Length")
fast = ta.ema(close, len)
sum = 0
for i = 0 to 2
    sum := sum + nz(close[i], close)
if isBull(close) and fast > fast[1] and sum > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		RequirementKeys: []string{"ma:EMA:3"},
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

func GoldenExamples() []Example {
	out := make([]Example, len(goldenExamples))
	copy(out, goldenExamples)
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
		"supportMatrix":               supportMatrix(),
		"compatibilityLayers":         compatibilityLayers(),
		"unsupportedPatterns":         unsupportedPatterns(),
		"goldenScripts":               goldenExamplePayloads(),
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
	writeMarkdownSection(&builder, "支持矩阵", sectionDetails("support-matrix"))
	writeMarkdownList(&builder, "v0.8 能力覆盖", flattenMatrixItems(supportMatrix()))
	writeMarkdownSection(&builder, "兼容迁移", sectionDetails("compatibility"))
	writeMarkdownList(&builder, "兼容层清单", flattenCompatibilityItems(compatibilityLayers()))
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
	builder.WriteString("## v0.8 黄金脚本\n\n")
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

func supportedTopLevelStatements() []string {
	return []string{
		"//@version=6",
		"strategy(\"<name>\", overlay=true[, default_qty_type=strategy.fixed|strategy.cash|strategy.percent_of_equity, default_qty_value=<number>, pyramiding=<integer>])",
		"<name> = <expression>",
		"if <condition>",
		"strategy.entry(\"<id>\", strategy.long|strategy.short[, qty=<expression>|qty_percent=<number>][, limit=<expression>])",
		"strategy.order(\"<id>\", strategy.long|strategy.short[, qty=<expression>|qty_percent=<number>][, limit=<expression>])",
		"strategy.close(\"<id>\"[, qty=<expression>|qty_percent=<number>][, limit=<expression>]) / strategy.close_all()",
	}
}

func supportedHooks() []string {
	return []string{
		"JFTrade 将可执行 Pine 语句统一映射到 K 线收盘执行。",
	}
}

func supportedStatements() []string {
	return []string{
		"<name> = input.int/float/bool/string/source/time/timeframe/color(default, title?) 会取默认值，不提供运行时 UI 调参",
		"<name> = ta.ema(source, period) / ta.sma(source, period) / ta.rsi(source, period)",
		"var <name> = <expression> / <name> := <expression>",
		"<series>[n] 多 bar 历史引用；可配合 nz(<series>[n], fallback)",
		"<condition> ? <trueExpr> : <falseExpr>",
		"[macdLine, signalLine, histLine] = ta.macd(close, fast, slow, signal)",
		"[plusDI, minusDI, adx] = ta.dmi(diLength, adxSmoothing)",
		"[supertrendLine, direction] = ta.supertrend(factor, atrPeriod)",
		"if ta.crossover(left, right) / if ta.crossunder(left, right) / if ta.cross(left, right)",
		"else",
		"strategy.entry(\"Long\", strategy.long) 会继承 strategy(...) 默认仓位",
		"strategy.entry(\"Long\", strategy.long, qty=1)",
		"strategy.entry(\"Long\", strategy.long, qty_percent=10)",
		"strategy.order(\"Net\", strategy.short, qty=5) 提交净额卖出，不受 pyramiding gate 限制",
		"strategy.entry(\"Long\", strategy.long, qty=(strategy.equity * 25 / 100) / close)",
		"strategy.entry(\"Short\", strategy.short, qty=1)",
		"strategy.close(\"Long\") / strategy.close(\"Short\", qty=50) / strategy.close_all()",
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
		{"name": "hl2/hlc3/ohlc4", "description": "TradingView 常见派生价格源，可作为 source-aware 指标输入。"},
		{"name": "kline", "description": "当前 K 线载荷视图。"},
		{"name": "strategy.equity", "description": "当前账户总权益；可用于普通表达式和仓位计算。"},
		{"name": "strategy.position_size", "description": "当前策略持仓数量；多头为正，空头为负，空仓为 0。"},
		{"name": "strategy.position_avg_price", "description": "当前策略持仓均价；空仓时为 na。"},
		{"name": "bar_index", "description": "当前策略收到的 K 线序号，从 0 开始。"},
		{"name": "time/hour/minute/dayofweek/dayofmonth/month/year", "description": "当前 K 线时间派生值；time 为 Unix milliseconds。"},
		{"name": "barstate.isfirst/isnew/isconfirmed/ishistory/isrealtime/islast", "description": "closed-bar runtime 状态；isnew/isconfirmed/islast 在已知 K 线执行时为 true。"},
		{"name": "session.ismarket/ispremarket/ispostmarket", "description": "当前 K 线所属 regular/pre/after session。"},
		{"name": "dayofweek.* / month.* / color.*", "description": "TradingView 常见常量；dayofweek 与 month lower 为数值，color 常量主要用于兼容默认参数和视觉模板。"},
		{"name": "syminfo.tickerid/syminfo.prefix", "description": "当前策略标的与前缀信息。"},
		{"name": "timeframe.period/timeframe.is*", "description": "当前策略周期及 intraday/minutes/daily/weekly/monthly 布尔状态。"},
	}
}

func indicatorFunctions() []map[string]any {
	return []map[string]any{
		{"name": "input.*", "signature": "input(defval) / input.int/float/bool/string/source/time/timeframe/color(defval, title?)", "notes": "只取默认值；input.source 第一版应使用 open/high/low/close/volume/hl2/hlc3/ohlc4；input.timeframe 可用于受支持的 request.security timeframe。"},
		{"name": "math.*", "signature": "math.abs/min/max/round/floor/ceil/sqrt/pow/log/sign", "notes": "lower 到同名表达式函数。"},
		{"name": "timestamp", "signature": "timestamp(year, month, day[, hour, minute])", "notes": "返回 Unix milliseconds；第一版不支持 timezone 参数。"},
		{"name": "ta.ema", "signature": "ta.ema(source, period)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4；close 保持 legacy key。"},
		{"name": "ta.sma", "signature": "ta.sma(source, period)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4；volume SMA 不会再误当 close SMA。"},
		{"name": "ta.rma/ta.wma/ta.hma/ta.vwma", "signature": "ta.<ma>(source, period)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。"},
		{"name": "ta.rsi", "signature": "ta.rsi(source, period)", "notes": "source-aware RSI；close 保持 legacy key。"},
		{"name": "ta.macd", "signature": "ta.macd(close, fast, slow, signal)", "notes": "支持三元组赋值，signal/hist 变量会映射到 MACD 字段。"},
		{"name": "ta.atr", "signature": "ta.atr(period)", "notes": "lower 到 JFTrade ATR 指标。"},
		{"name": "ta.tr", "signature": "ta.tr / ta.tr(true)", "notes": "返回当前 True Range。"},
		{"name": "ta.stdev/ta.variance", "signature": "ta.stdev(source, period) / ta.variance(source, period)", "notes": "source-aware rolling variance/standard deviation。"},
		{"name": "ta.cci", "signature": "ta.cci(source, period)", "notes": "source-aware CCI；默认 source 为 hlc3。"},
		{"name": "ta.highest", "signature": "ta.highest(source, length) / ta.highest(length)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4；单参数默认 high。"},
		{"name": "ta.lowest", "signature": "ta.lowest(source, length) / ta.lowest(length)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4；单参数默认 low。"},
		{"name": "ta.change", "signature": "ta.change(source[, length])", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4；未传 length 默认 1。"},
		{"name": "ta.mom", "signature": "ta.mom(source, length)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。"},
		{"name": "ta.roc", "signature": "ta.roc(source, length)", "notes": "source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。"},
		{"name": "ta.sum", "signature": "ta.sum(source, length)", "notes": "滚动求和；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。"},
		{"name": "ta.rising", "signature": "ta.rising(source, length)", "notes": "返回 bool；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。"},
		{"name": "ta.falling", "signature": "ta.falling(source, length)", "notes": "返回 bool；source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。"},
		{"name": "ta.bb", "signature": "[basis, upper, lower] = ta.bb(close, length, mult)", "notes": "lower 到 JFTrade Bollinger 指标。"},
		{"name": "ta.wpr", "signature": "ta.wpr(length)", "notes": "lower 到 JFTrade Williams %R 指标。"},
		{"name": "ta.vwap", "signature": "ta.vwap(source?)", "notes": "按交易日累计 VWAP；无参数默认 hlc3。"},
		{"name": "ta.mfi", "signature": "ta.mfi(source, length)", "notes": "基于 source 与 volume 的 Money Flow Index。"},
		{"name": "ta.dmi/ta.adx", "signature": "[plusDI, minusDI, adx] = ta.dmi(diLength, adxSmoothing) / ta.adx(length)", "notes": "支持 DMI 三元组和常见 ta.adx(length) 写法。"},
		{"name": "ta.supertrend", "signature": "[line, direction] = ta.supertrend(factor, atrPeriod)", "notes": "支持三元组式绑定中的 line/direction。"},
		{"name": "ta.sar", "signature": "ta.sar(start, increment, max)", "notes": "Parabolic SAR；生成 sar:start:increment:max requirement，snapshot 提供 value/previous。"},
		{"name": "ta.barssince", "signature": "ta.barssince(condition)", "notes": "首次触发前返回 na，触发 bar 返回 0。"},
		{"name": "ta.valuewhen", "signature": "ta.valuewhen(condition, sourceExpression, occurrence)", "notes": "occurrence 必须为非负整数；历史不足返回 na。"},
		{"name": "ta.crossover", "signature": "ta.crossover(left, right)", "notes": "lower 到 cross_over。"},
		{"name": "ta.crossunder", "signature": "ta.crossunder(left, right)", "notes": "lower 到 cross_under。"},
		{"name": "ta.cross", "signature": "ta.cross(left, right)", "notes": "lower 到 cross_over(left,right) or cross_under(left,right)。"},
	}
}

func orderModes() []map[string]any {
	return []map[string]any{
		{"name": "strategy.entry qty", "description": "按股数表达式开多或开空。"},
		{"name": "strategy.entry/order qty_percent", "description": "entry/order 中 qty_percent 表示账户权益百分比。"},
		{"name": "strategy.order", "description": "净额买入或卖出；不受 strategy.entry pyramiding gate 限制。"},
		{"name": "strategy.entry amount", "description": "固定金额可写为 qty=amount/close。"},
		{"name": "strategy.entry equity percent", "description": "账户权益百分比可写为 qty=(strategy.equity*pct/100)/close。"},
		{"name": "strategy.close/close_all", "description": "平仓；不指定数量默认平 100% 持仓，close 的 qty_percent 表示当前 symbol 持仓百分比。"},
		{"name": "strategy.exit bracket", "description": "支持 stop、limit、stop+limit bracket；stop/limit 可使用普通数值表达式，qty/qty_percent 可部分退出。"},
		{"name": "pending entry/order", "description": "strategy.entry/order 支持基础 stop 或 limit pending；stop+limit 组合仍明确诊断。"},
		{"name": "strategy.cancel/cancel_all", "description": "取消当前策略 symbol 尚未触发的内部 pending orders。"},
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

func supportMatrix() []map[string]any {
	return []map[string]any{
		{"capability": "Pine metadata and diagnostics", "parser": true, "planner": true, "runtime": true, "jftrade": true, "frontend": true, "notes": "统一通过 AnalyzeScript、strategy.pine_spec、编辑器提示和结构化 diagnostics 暴露。"},
		{"capability": "Source-aware indicators", "parser": true, "planner": true, "runtime": true, "jftrade": true, "frontend": true, "notes": "MA/RSI/stdev/variance/CCI/rolling/source-aware MTF 使用稳定 key；close 保留 legacy key。"},
		{"capability": "Rolling and stateful indicators", "parser": true, "planner": true, "runtime": true, "jftrade": true, "frontend": false, "notes": "highest/lowest/change/mom/roc/rising/falling/sum、barssince、valuewhen 已可执行；前端只覆盖常用子集。"},
		{"capability": "MTF request.security subset", "parser": true, "planner": true, "runtime": true, "jftrade": true, "frontend": true, "notes": "仅同标的 source/source[n]/source-aware MA，禁止 lookahead_on/gaps_on 和任意表达式。"},
		{"capability": "Orders and exits", "parser": true, "planner": true, "runtime": true, "jftrade": true, "frontend": true, "notes": "entry/order/close/close_all/exit/cancel 的可执行子集已贯通，完整 broker emulator 不属于当前目标。"},
		{"capability": "UDF and static for", "parser": true, "planner": true, "runtime": true, "jftrade": true, "frontend": false, "notes": "单表达式 UDF 与静态整数 for 编译期展开；动态循环、多语句/递归 UDF 诊断失败。"},
	}
}

func compatibilityLayers() []map[string]any {
	return []map[string]any{
		{"name": "close legacy indicator keys", "category": "long_term", "status": "keep", "notes": "close-source MA/RSI/stdev/CCI 继续保留历史 key，避免已有回测和运行实例断裂；新增 source 使用 source-aware key。"},
		{"name": "visual-only Pine calls", "category": "long_term", "status": "keep", "notes": "plot/hline/bgcolor/label/table 等 TradingView 视觉调用作为 warning no-op 保留，降低模板迁移成本。"},
		{"name": "legacy visual codeBlock", "category": "migrate_before_v1", "status": "read_only", "notes": "旧 codeBlock 只作为历史模型兼容；Pine 反解兜底应生成 pineSnippet，保存时不再产生 JS 代码块。"},
		{"name": "unified technicalIndicator block", "category": "migrate_before_v1", "status": "read_only", "notes": "旧合并式指标块保留读取和迁移；新建路径使用 getTechnicalIndicator + technicalIndicatorCondition。"},
		{"name": "non-Pine source/runtime defaults", "category": "remove_after_migration", "status": "replace_with_default_pine", "notes": "历史非 Pine 定义只做默认 Pine 脚本替换或只读展示，不扩展旧 runtime。"},
	}
}

func unsupportedPatterns() []string {
	return []string{
		"indicator()、study()、library() 脚本不能作为 JFTrade 可执行策略。",
		"request.security() 仅支持 syminfo.tickerid + 受支持 timeframe + OHLCV/hl2/hlc3/ohlc4 source、source[n] 或 ta.sma/ema/rma/wma/hma/vwma(source, n) 的低风险多周期子集。",
		"array.*、matrix.*、map.* 集合命名空间暂不支持。",
		"静态 for 循环会在编译期展开；动态边界、break/continue、超过 100 次展开和超过 2 层嵌套会返回明确诊断。",
		"表达式 UDF 支持编译期内联；多语句函数、递归函数、method/type 会返回明确诊断。",
		"历史引用支持简单 identifier/member 的 `[n]`，最大 lookback 500；函数调用结果需先赋值再引用历史。",
		"strategy.exit() 支持基础 stop、limit、stop+limit bracket 与 trail_points/trail_offset；trail 与 stop/limit 同用、OCA、partial fill、intrabar broker emulator 等高级语义暂不支持。",
		"strategy.entry/order 的 stop+limit 组合、OCA、strategy.cancel 已成交订单等完整 broker emulator 语义暂不支持，会返回明确诊断或内部跳过。",
		"plot/hline/bgcolor/barcolor/fill/alertcondition/label.new/line.new/box.new/table.* 等非交易调用会被解析为 warning 并忽略。",
		"除文档列出的 ta.*、input.*、math.*、strategy.entry、strategy.close、alert/log 外的 built-ins 不应假定可执行。",
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
	return payloadsForExamples(examples)
}

func goldenExamplePayloads() []map[string]any {
	return payloadsForExamples(goldenExamples)
}

func payloadsForExamples(items []Example) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, example := range items {
		out = append(out, map[string]any{
			"id":              example.ID,
			"title":           example.Title,
			"description":     example.Description,
			"script":          example.Script,
			"requirementKeys": example.RequirementKeys,
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
			"支持 var 持久变量、:= 重赋值、基础三元表达式、多 bar 历史引用、表达式 UDF 和静态 for 编译期展开。",
			"表达式 UDF 形如 name(arg) => expression；支持单表达式缩进体，多语句函数暂不支持。",
			"静态 for 支持 for i = start to end [by step]，边界必须是整数常量或 input.int 默认值，按 Pine inclusive to 语义展开。",
			"JFTrade 会把顶层可执行语句作为 K 线收盘逻辑执行。",
		}
	case "expressions":
		return []string{
			"支持 close/open/high/low/volume/hl2/hlc3/ohlc4、算术、比较和布尔表达式。",
			"close[1]/open[1]/high[1]/low[1]/volume[1] 会 lower 为上一根 K 线值。",
			"条件表达式要求严格 bool；数值不能直接作为 if 条件。",
			"支持 na 常量、nz(value, fallback?) 和基础三元表达式。",
			"input()/input.int/float/bool/string/source/time/timeframe/color 会取默认值；不实现 TradingView 设置面板运行时覆盖。",
			"strategy.equity、bar_index、time/hour/minute/dayofweek/dayofmonth/month/year 可在普通表达式中读取。",
			"barstate.isfirst/isnew/isconfirmed/ishistory/isrealtime/islast 和 session.ismarket/ispremarket/ispostmarket 会 lower 为 closed-bar runtime 状态。",
			"dayofweek.sunday...saturday、month.january...december、color.*、color.new(...)、color.rgb(...) 支持常见默认值兼容。",
			"syminfo.tickerid、syminfo.prefix、timeframe.period 和 timeframe.isintraday/isminutes/isdaily/isweekly/ismonthly 可在普通表达式中读取。",
			"timestamp(year, month, day[, hour, minute]) 返回 Unix milliseconds；不支持 timezone 参数。",
			"ta.crossover/ta.crossunder/ta.cross 会映射到 JFTrade cross_over/cross_under。",
			"math.abs/min/max/round/floor/ceil/sqrt/pow/log/sign 会映射到 JFTrade 表达式函数。",
			"未知 built-ins 可能无法 lower，应先调用 strategy.validate_pine。",
		}
	case "indicators":
		return []string{
			"指标绑定通过 <alias> = ta.<function>(...) 声明。",
			"compiler 当前识别 ta.sma、ta.ema、ta.rma、ta.wma、ta.hma、ta.vwma、ta.rsi、ta.macd、ta.atr、ta.tr、ta.stdev、ta.variance、ta.cci、ta.highest、ta.lowest、ta.change、ta.mom、ta.roc、ta.sum、ta.rising、ta.falling、ta.bb、ta.wpr、ta.vwap、ta.mfi、ta.dmi、ta.adx、ta.supertrend、ta.sar、ta.barssince、ta.valuewhen、ta.crossover、ta.crossunder、ta.cross。",
			"request.security 支持同标的 timeframe：\"1\"/\"5\"/\"15\"/\"30\"/\"45\"/\"60\"/\"120\"/\"240\"、\"D\"/\"1D\"、\"W\"/\"1W\"、\"M\"/\"1M\"。",
			"request.security(syminfo.tickerid, timeframe, source) 支持 OHLCV/hl2/hlc3/ohlc4 和 source[n]；request.security(..., ta.sma/ema/rma/wma/hma/vwma(source, n)) 支持 source-aware MTF 均线。",
			"ta.macd 支持 [macdLine, signalLine, histLine] 三元组赋值。",
			"source-aware 指标第一版 source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
			"历史引用支持 close[2]、hlc3[3]、emaFast[5]、bands.upper[2] 等简单 identifier/member；超过 500 bar 会返回诊断。",
		}
	case "orders":
		return []string{
			"strategy.entry(id, strategy.long, qty=n) 映射为买入开多。",
			"strategy.entry(id, strategy.short, qty=n) 映射为卖出开空。",
			"strategy.entry 未显式传 qty 时，会继承 strategy(...) 的 default_qty_type/default_qty_value；默认等价 strategy.fixed + 1。",
			"strategy.entry/order 支持 qty_percent；entry/order 中表示账户权益百分比，close/exit 中表示当前 symbol 持仓百分比。",
			"pyramiding 默认按 1 处理；显式 pyramiding>1 时允许有限同向追加。",
			"strategy.order 提交净额买入或卖出，不套用 strategy.entry 的 pyramiding gate。",
			"strategy.close_all() 只 flatten 当前策略 symbol。",
			"固定金额可写 qty=amount/close，账户权益百分比可写 qty=(strategy.equity*pct/100)/close。",
			"strategy.entry/order(..., stop=price) 映射为基础 stop pending；limit=price 映射为基础 limit pending。",
			"strategy.close(id, qty=n, limit=price) 根据已知 entry id 映射为平多或平空，支持部分平仓与限价。",
			"strategy.exit(id, from_entry, stop=..., limit=..., qty/qty_percent=...) 映射为 closed-bar bracket；同 bar 两侧触发时采用保守 stop-first。",
			"strategy.cancel(id)/cancel_all() 取消当前策略 symbol 尚未触发的 pending orders。",
		}
	case "support-matrix":
		return []string{
			"v0.8 将当前 Pine v6 子集按 parser、planner、runtime、JFTrade API/ADK 和前端五个面向锁定。",
			"新增 Pine 能力必须同步更新 parser lowering、IR requirements、indicator/runtime lookup、规范输出和至少一层可执行测试。",
			"前端不是完整 Pine IDE；流程图覆盖常用策略 authoring，无法标准化的 Pine 行保留为 pineSnippet。",
		}
	case "compatibility":
		return []string{
			"兼容层分为长期保留、v1.0 前迁移和迁移后删除三类，避免历史策略被一次性破坏。",
			"生成路径优先切到 Pine v6 标准图块；旧模型保留只读迁移和导入解析。",
			"close legacy key 是运行时稳定性兼容，不代表新增指标继续使用旧 key 形态。",
		}
	case "unsupported":
		return []string{
			"plot/hline/bgcolor/barcolor/fill/alertcondition/label.new/line.new/box.new/table.* 等非交易调用会返回 warning 并忽略。",
			"while/switch、动态 for、break/continue、多语句/递归 UDF 和 Pine 类型系统会返回结构化诊断。",
			"除同标的 source/source[n]/source-aware MA 子集以外的 request.security、lookahead_on/gaps_on、import/library、array/matrix/map 会返回错误。",
			"strategy.entry/order 的 stop-limit、OCA 和完整 pending order broker emulator 会返回明确诊断。",
			"strategy.exit 的 OCA、partial fill、trail 与 bracket 混用、intrabar broker emulator 等高级语义会给出明确诊断。",
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

func flattenMatrixItems(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		capability, _ := item["capability"].(string)
		notes, _ := item["notes"].(string)
		out = append(out, capability+" — "+notes)
	}
	return out
}

func flattenCompatibilityItems(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		category, _ := item["category"].(string)
		status, _ := item["status"].(string)
		notes, _ := item["notes"].(string)
		out = append(out, name+" ["+category+"/"+status+"] — "+notes)
	}
	return out
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
