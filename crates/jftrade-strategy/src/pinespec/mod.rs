//! Versioned Pine specification payload shared by strategy tools.
//!
//! The payload is generated from this Rust source at runtime (rather than
//! copied from a fixture), so a caller always sees the same section list and
//! executable subset as the native compiler.

use serde::Serialize;
use serde_json::{json, Value};
use thiserror::Error;

use crate::pine::{compile, LoweredProgram, Requirements};

pub const PINE_VERSION: &str = "v6";
pub const PRODUCT_VERSION: &str = "v4.0";
pub const SOURCE_FORMAT: &str = "pine-v6";
pub const RUNTIME: &str = "pine-pinets";
pub const TOOL_NAME: &str = "strategy.pine_spec";
pub const COMPATIBILITY_SCORE: f64 = 98.30;
pub const SCORE_MODEL_VERSION: &str = "closed-bar-strategy-v4.0";

const PINE_TS_ENGINE: &str = "pinets-shadow";
const PINE_TS_LICENSE: &str = "AGPL-3.0-only";
const PINE_TS_PACKAGE: &str = "pinets@0.9.31";
const PINE_TS_REPOSITORY: &str = "https://github.com/LuxAlgo/PineTS";
const PINE_TS_WORKER: &str = "scripts/pinets-worker.mjs";
const SKELETON: &str = r#"//@version=6
strategy("Minimal Draft", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)

log.info("ready")"#;

pub const SECTIONS: &[&str] = &[
    "overview",
    "syntax",
    "expressions",
    "indicators",
    "orders",
    "support-matrix",
    "unsupported",
    "examples",
];

#[derive(Clone, Debug, Eq, PartialEq, Error)]
#[error("strategy.pine_spec 不支持 section {section:?}（可选值：{allowed}）")]
pub struct SpecError {
    pub section: String,
    pub allowed: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Section {
    pub id: String,
    pub title: String,
    pub summary: String,
}

pub fn normalize_section(section: &str) -> String {
    section.trim().to_ascii_lowercase()
}
pub fn allowed_sections() -> Vec<String> {
    SECTIONS
        .iter()
        .map(|section| (*section).to_owned())
        .collect()
}
pub fn sections() -> Vec<Section> {
    [
        (
            "overview",
            "概览",
            "说明 JFTrade Pine Script v6 前端、pine-pinets runtime，以及草稿、回测、运行实例之间的边界。",
        ),
        (
            "syntax",
            "语法",
            "Pine v6 声明、缩进块、注释和当前可执行子集。",
        ),
        (
            "expressions",
            "表达式",
            "支持的 Pine 表达式、OHLCV 序列和函数映射。",
        ),
        (
            "indicators",
            "指标",
            "当前 compiler、planner 与 runtime 能识别的 ta.* 指标。",
        ),
        (
            "orders",
            "下单",
            "strategy.entry/strategy.close 到 JFTrade 订单 IR 的映射。",
        ),
        (
            "support-matrix",
            "支持矩阵",
            "按 parser、semantic、planner、runtime、JFTrade 集成和前端锁定 v4.0 Pine v6 主路径、collection/map/matrix、tuple、动态循环、纯 UDT/method、MTF stoch、array stats、字符串/timeframe helper、object history/method receiver、稳定 semantic metadata、public surface 诊断、MTF preflight、高级语言边界诊断、生成式支持快照与 broker 边界决策。",
        ),
        (
            "unsupported",
            "不支持项",
            "已解析但不能在 JFTrade 中执行的 Pine v6 行为。",
        ),
        (
            "examples",
            "示例",
            "当前实现下可以成功 parse、lower 并完成 requirements planning 的 Pine v6 脚本。",
        ),
    ]
    .into_iter()
    .map(|(id, title, summary)| Section {
        id: id.to_owned(),
        title: title.to_owned(),
        summary: summary.to_owned(),
    })
    .collect()
}

pub fn build_tool_payload(section: &str, include_examples: bool) -> Result<Value, SpecError> {
    let selected = normalize_section(section);
    if !selected.is_empty() && !SECTIONS.contains(&selected.as_str()) {
        return Err(SpecError {
            section: section.to_owned(),
            allowed: SECTIONS.join(", "),
        });
    }
    let mut payload = json!({
        "version": PINE_VERSION,
        "productVersion": PRODUCT_VERSION,
        "sourceFormat": SOURCE_FORMAT,
        "runtime": RUNTIME,
        "sections": sections(),
        "selectedSection": selected,
        "supportedTopLevelStatements": ["//@version=6", "strategy(\"<name>\", overlay=true[, ...])", "<name> = <expression>", "if <condition>", "strategy.entry/order/close/exit/cancel"],
        "supportedHooks": ["JFTrade 在 K 线收盘 hook 执行可执行策略语句。"],
        "supportedStatements": ["var <name> = <expression>", "<name> := <expression>", "ta.ema/sma/rsi/macd/atr", "request.security(syminfo.tickerid, timeframe, expression)", "alert(\"message\") / log.info(\"message\")"],
        "reservedVariables": reserved_variables(),
        "indicatorFunctions": indicator_functions(),
        "orderModes": ["strategy.entry", "strategy.order", "strategy.close", "strategy.close_all", "strategy.exit", "strategy.cancel", "strategy.cancel_all"],
        "protectModes": [],
        "supportMatrix": support_matrix(),
        "capabilities": capabilities(),
        "compatibilityScore": COMPATIBILITY_SCORE,
        "scoreModelVersion": SCORE_MODEL_VERSION,
        "compatibilityDimensions": compatibility_dimensions(),
        "brokerBoundary": broker_boundary(),
        "externalEngine": external_engine_payload(),
        "unsupportedPatterns": ["import/library/type/method declarations", "dynamic external request.security symbols", "intrabar broker emulator"],
        "goldenScripts": golden_scripts(),
        "skeleton": SKELETON,
        "examples": []
    });
    if !selected.is_empty() {
        payload["sectionContent"] = section_content(&selected);
    }
    if include_examples || selected == "examples" {
        payload["examples"] =
            Value::Array(example_scripts().into_iter().map(example_payload).collect());
    }
    Ok(payload)
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValidationPayload {
    pub ok: bool,
    pub source_format: String,
    pub runtime: String,
    pub external_engine: Value,
    pub normalized_script: String,
    pub metadata: Value,
    pub hooks: Vec<String>,
    pub requirements: Option<Requirements>,
    pub warnings: Vec<String>,
    pub errors: Vec<String>,
    pub save_hint: Option<SaveHintPayload>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SaveHintPayload {
    pub message: String,
    pub spec_tool: String,
    pub resource_files: Vec<String>,
    pub skeleton: String,
}

pub fn validate_script(
    source: &str,
    include_requirements: bool,
    _include_ast: bool,
) -> ValidationPayload {
    let normalized_script = source.trim().to_owned();
    if normalized_script.is_empty() {
        return ValidationPayload {
            ok: false,
            source_format: SOURCE_FORMAT.to_owned(),
            runtime: RUNTIME.to_owned(),
            external_engine: validation_external_engine_payload(),
            normalized_script,
            metadata: default_metadata(),
            hooks: Vec::new(),
            requirements: None,
            warnings: Vec::new(),
            errors: vec!["script 是必填项".to_owned()],
            save_hint: Some(save_hint_payload()),
        };
    }
    let compilation = compile(&normalized_script);
    let (metadata, hooks) = compilation
        .program
        .as_ref()
        .map(program_metadata)
        .unwrap_or_else(|| (default_metadata(), Vec::new()));
    let requirements =
        (compilation.ok && include_requirements).then_some(compilation.requirements.clone());
    ValidationPayload {
        ok: compilation.ok,
        source_format: SOURCE_FORMAT.to_owned(),
        runtime: RUNTIME.to_owned(),
        external_engine: validation_external_engine_payload(),
        normalized_script,
        metadata,
        hooks,
        requirements,
        warnings: compilation.warnings.clone(),
        errors: compilation
            .diagnostics
            .iter()
            .filter(|diagnostic| diagnostic.severity == crate::pine::DiagnosticSeverity::Error)
            .map(|diagnostic| diagnostic.message.clone())
            .collect(),
        save_hint: (!compilation.ok).then(save_hint_payload),
    }
}

fn program_metadata(program: &LoweredProgram) -> (Value, Vec<String>) {
    (
        json!({"name": program.metadata.name, "version": program.metadata.version, "symbol": program.metadata.symbol, "interval": program.metadata.interval, "defaultQtyMode": program.metadata.default_qty_mode, "defaultQtyValue": program.metadata.default_qty_value, "pyramiding": program.metadata.pyramiding, "risk": {}}),
        program.hooks.iter().map(|hook| hook.kind.clone()).collect(),
    )
}
fn default_metadata() -> Value {
    json!({"name":"", "version":"", "symbol":"", "interval":"", "defaultQtyMode":"fixed", "defaultQtyValue":"1", "pyramiding":1, "risk":{}})
}
fn section_content(section: &str) -> Value {
    let (title, summary, details) = match section {
        "overview" => (
            "概览",
            "说明 JFTrade Pine Script v6 前端、pine-pinets runtime，以及草稿、回测、运行实例之间的边界。",
            vec![
                "JFTrade Pine Script v6 前端会把支持的 Pine 策略语句交给 PineTS worker runtime 执行。",
                "已保存草稿、回测结果和正在运行的策略实例必须视为不同工作状态，不能混为一谈。",
                "当前目标是可执行、同标的、closed-bar 策略迁移兼容；不宣称完整 TradingView Pine v6 或 broker emulator 兼容。",
            ],
        ),
        "syntax" => (
            "语法",
            "Pine v6 声明、缩进块、注释和当前可执行子集。",
            vec![
                "脚本必须包含 //@version=6 和 strategy(...)。",
                "空行与普通 // 注释会被忽略；// @jftradeFlow* 注释用于前端流程图双向同步。",
                "if/else 使用 Pine 风格缩进块；顶层可执行语句统一按 K 线收盘逻辑 lower。",
                "支持 var 持久变量、:= 重赋值、基础三元表达式、多 bar 历史引用、表达式/受控多语句 UDF 和静态 for 编译期展开。",
                "UDF 支持 name(arg) => expression、单表达式缩进体，以及包含局部赋值、if/else 和最终返回表达式的受控多语句函数。",
                "静态 for 支持 for i = start to end [by step]，边界必须是整数常量或 input.int 默认值，按 Pine inclusive to 语义展开。",
                "JFTrade 会把顶层可执行语句作为 K 线收盘逻辑执行。",
            ],
        ),
        "expressions" => (
            "表达式",
            "支持的 Pine 表达式、OHLCV 序列和函数映射。",
            vec![
                "支持 close/open/high/low/volume/hl2/hlc3/ohlc4、算术、比较和布尔表达式。",
                "close[1]/open[1]/high[1]/low[1]/volume[1] 会 lower 为上一根 K 线值。",
                "条件表达式要求严格 bool；数值不能直接作为 if 条件。",
                "支持 na 常量、nz(value, fallback?) 和基础三元表达式。",
                "input()/input.int/float/bool/string/source/time/timeframe/color 会取默认值；不实现 TradingView 设置面板运行时覆盖。",
                "strategy.equity、bar_index、time/hour/minute/dayofweek/dayofmonth/month/year 可在普通表达式中读取。",
                "barstate.isfirst/isnew/isconfirmed/ishistory/isrealtime/islast 和 session.ismarket/ispremarket/ispostmarket 由 PineTS worker 按 K 线状态执行。",
                "dayofweek.sunday...saturday、month.january...december、color.*、color.new(...)、color.rgb(...) 支持常见默认值兼容。",
                "syminfo.tickerid、syminfo.prefix、timeframe.period 和 timeframe.isintraday/isminutes/isdaily/isweekly/ismonthly 可在普通表达式中读取。",
                "timestamp(year, month, day[, hour, minute]) 按当前标的交易所时区解释并返回 Unix milliseconds；不支持显式 timezone 参数。",
                "ta.crossover/ta.crossunder/ta.cross 会映射到 JFTrade cross_over/cross_under。",
                "math.abs/min/max/avg/round/round_to_mintick/floor/ceil/sqrt/pow/log/sign 会映射到 JFTrade 表达式函数。",
                "未知 built-ins 可能无法 lower，应先调用 strategy.validate_pine。",
            ],
        ),
        "indicators" => (
            "指标",
            "当前 compiler、planner 与 runtime 能识别的 ta.* 指标。",
            vec![
                "指标绑定通过 <alias> = ta.<function>(...) 声明。",
                "compiler 当前识别常用 MA、RSI/MACD/ATR、rolling/window、Bollinger、DMI/Supertrend/SAR，v1.2 的 linreg/OBV/pivot/Keltner/ALMA，v1.3 的 CMO/TSI/correlation/dev/median/percentile/percentrank/SWMA，v1.4 的窗口/动量、状态事件和 TR，v1.5 的 MTF common TA，v1.6 的 MTF tuple 白名单，以及 v2.1 的 BBW/COG/锚定 VWAP。",
                "request.security 支持同标的 timeframe：\"1\"/\"5\"/\"15\"/\"30\"/\"45\"/\"60\"/\"120\"/\"240\"、\"D\"/\"1D\"、\"W\"/\"1W\"、\"M\"/\"1M\"。",
                "request.security(syminfo.tickerid, timeframe, source) 支持 OHLCV/hl2/hlc3/ohlc4 和 source[n]；支持 source-aware MTF 均线、静态 intraday 受支持高级指标、v1.4 纯表达式 source/history/MA/math/bool/nz 组合、v1.5 RSI/MACD/ATR/Bollinger/Supertrend common TA 组合、v1.6 source/TA/纯表达式 tuple 白名单、v2.2 2-8 元纯表达式 tuple、v2.3 纯 collection/object 表达式，以及 v2.4 MTF stoch。",
                "request.security 的 ticker 参数也可使用 ticker.heikinashi(syminfo.tickerid)、ticker.standard()、ticker.standard(syminfo.tickerid) 或 ticker.inherit(..., syminfo.tickerid)；只允许当前标的，外部和动态标的返回诊断。",
                "ta.macd 支持 [macdLine, signalLine, histLine] 三元组赋值。",
                "source-aware 指标第一版 source 支持 open/high/low/close/volume/hl2/hlc3/ohlc4。",
                "历史引用支持 close[2]、hlc3[3]、emaFast[5]、bands.upper[2] 等简单 identifier/member；超过 500 bar 会返回诊断。",
            ],
        ),
        "orders" => (
            "下单",
            "strategy.entry/strategy.close 到 JFTrade 订单 IR 的映射。",
            vec![
                "strategy.entry(id, strategy.long, qty=n) 映射为买入开多。",
                "strategy.entry(id, strategy.short, qty=n) 映射为卖出开空。",
                "strategy.entry 未显式传 qty 时，会继承 strategy(...) 的 default_qty_type/default_qty_value；默认等价 strategy.fixed + 1。",
                "strategy.entry/order 支持 qty_percent；entry/order 中表示账户权益百分比，close/exit 中表示当前 symbol 持仓百分比。",
                "strategy.entry 反向开仓会按 Pine 语义自动反手；strategy.risk.allow_entry_in 可限制方向，被禁止方向在已有反向仓位时只平仓不反手。",
                "pyramiding 默认按 1 处理；显式 pyramiding>1 时允许有限同向追加。",
                "strategy.order 提交净额买入或卖出，不套用 strategy.entry 的 pyramiding gate。",
                "strategy.close_all() 只 flatten 当前策略 symbol。",
                "固定金额可写 qty=amount/close，账户权益百分比可写 qty=(strategy.equity*pct/100)/close。",
                "strategy.entry/order(..., stop=price) 映射为基础 stop pending；limit=price 映射为基础 limit pending。",
                "strategy.close(id, qty=n, limit=price) 根据已知 entry id 映射为平多或平空，支持部分平仓与限价。",
                "strategy.exit(id, from_entry, stop=..., limit=..., qty/qty_percent=...) 映射为 closed-bar bracket；同 bar 两侧触发时采用保守 stop-first。",
                "strategy.cancel(id)/cancel_all() 取消当前策略 symbol 尚未触发的 pending orders。",
                "strategy() 支持 initial_capital、commission_type/value、slippage 与 process_orders_on_close；API initialBalance 优先于脚本资金。",
                "strategy.close/close_all 支持 immediately=true；comment、alert_message、disable_alert 会进入日志/通知元数据。",
            ],
        ),
        "support-matrix" => (
            "支持矩阵",
            "按 parser、semantic、planner、runtime、JFTrade 集成和前端锁定 v4.0 Pine v6 主路径、collection/map/matrix、tuple、动态循环、纯 UDT/method、MTF stoch、array stats、字符串/timeframe helper、object history/method receiver、稳定 semantic metadata、public surface 诊断、MTF preflight、高级语言边界诊断、生成式支持快照与 broker 边界决策。",
            vec![
                "v4.0 保持闭盘可执行 Pine v6 子集作为策略定义、预览、回测、实例化、运行和 ADK 工具主路径。",
                "v4.0 让 collection/map/matrix 扩展、array stats、字符串/timeframe helper、结构化 AST、通用 tuple、动态循环、纯 UDT constructor/method、持久 object 字段更新、object collection fields、collection history aggregate、object history read/method receiver、method chain、MTF stoch、稳定 semantic declaration metadata、visual metadata、native public surface diagnostics、MTF diagnostic matrix、lower-timeframe MTF preflight、高级语言边界诊断、生成式支持快照和 broker emulator 边界决策可分析、可解释、可分层执行；library/import 和完整 TradingView method/type 系统仍只进入 metadata/diagnostics。",
                "新增 Pine 能力必须同步更新 parser lowering、semantic summary、IR requirements、indicator/runtime lookup、规范输出和至少一层可执行测试。",
                "前端不是完整 Pine IDE；流程图覆盖常用策略 authoring，无法标准化的 Pine 行会返回行号诊断，请继续在 Pine 工作台编辑。",
            ],
        ),
        "unsupported" => (
            "不支持项",
            "已解析但不能在 JFTrade 中执行的 Pine v6 行为。",
            vec![
                "plot/hline/bgcolor/barcolor/fill/alertcondition/label.new/line.new/box.new/table.* 等非交易调用由 PineTS worker 归入 visual output 或 alerts；Go 交易链路不消费这些输出。",
                "动态 for/while/break/continue 已在闭盘 runtime 执行，但递归/嵌套 UDF、library/import、method 副作用和完整 Pine method/type 系统仍会返回结构化诊断。",
                "除同标的静态 source/source[n]/MA/受支持高级指标/v1.4 纯表达式、v1.5 common TA pure-expression、v1.6 tuple 白名单、v2.2 2-8 元纯表达式 tuple、v2.3 纯 collection/object 表达式、v2.4 MTF stoch、v2.7 helper 表达式、v2.8 object method 表达式与 v2.9 object history 表达式以外的 request.security、lookahead_on/gaps_on 和 side effect 会返回错误。",
                "strategy.entry/order 支持基础 stop-limit 和 entry 反手；OCA、partial fill、保证金裸空账户模拟和完整 pending order broker emulator 不支持。",
                "strategy.exit 的 OCA、partial fill、trail 与 bracket 混用、intrabar broker emulator 等高级语义会给出明确诊断。",
                "完整 TradingView broker emulator 行为不属于当前 JFTrade runtime。",
            ],
        ),
        "examples" => (
            "示例",
            "当前实现下可以成功 parse、lower 并完成 requirements planning 的 Pine v6 脚本。",
            vec![
                "这些示例脚本与内置 skill 资源和 strategy.pine_spec 输出共用同一份规范源。",
                "这些示例旨在保证当前实现下可以成功 parse、lower 并完成 requirements planning。",
            ],
        ),
        _ => ("", "", Vec::new()),
    };
    json!({
        "id": section,
        "title": title,
        "summary": summary,
        "details": details,
    })
}
fn reserved_variables() -> Value {
    json!([
        {"name":"close","description":"当前及历史 close 序列。"},
        {"name":"open","description":"当前及历史 open 序列。"},
        {"name":"high","description":"当前及历史 high 序列。"},
        {"name":"low","description":"当前及历史 low 序列。"},
        {"name":"volume","description":"当前及历史 volume 序列。"},
        {"name":"hl2/hlc3/ohlc4","description":"常见派生价格源，可作为 source-aware 指标输入。"},
        {"name":"strategy.equity","description":"当前账户总权益。"},
        {"name":"strategy.position_size","description":"当前策略持仓数量。"},
        {"name":"bar_index","description":"当前策略收到的 K 线序号，从 0 开始。"},
        {"name":"time/hour/minute/dayofweek/dayofmonth/month/year","description":"当前 K 线时间派生值。"}
    ])
}
fn indicator_functions() -> Value {
    json!([
        {"name":"ta.ema","signature":"ta.ema(source, period)"},
        {"name":"ta.sma","signature":"ta.sma(source, period)"},
        {"name":"ta.rsi","signature":"ta.rsi(source, period)"},
        {"name":"ta.macd","signature":"ta.macd(close, fast, slow, signal)"},
        {"name":"ta.atr","signature":"ta.atr(period)"},
        {"name":"ta.crossover","signature":"ta.crossover(left, right)"},
        {"name":"ta.crossunder","signature":"ta.crossunder(left, right)"},
        {"name":"request.security","signature":"request.security(syminfo.tickerid, timeframe, expression)"}
    ])
}
fn support_matrix() -> Value {
    json!([
        {"capability":"JFTrade Pine v6 main path","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"新建、保存、预览、回测、实例化和启动统一使用 sourceFormat=pine-v6 + runtime=pine-pinets；旧 source/runtime 与旧 visual model 明确拒绝。"},
        {"capability":"Backtest capital and trading costs","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"API initialBalance > Pine initial_capital > 系统默认；支持 percent/cash commission 与按最小价格单位计算的 slippage ticks，仅作用于回测。"},
        {"capability":"Pine metadata and diagnostics","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"统一通过 AnalyzeScript、strategy.pine_spec、编辑器提示、结构化 diagnostics、visuals/declarations/collectionOperations/objectOperations metadata 和 semantic summary 暴露。"},
        {"capability":"Source-aware indicators","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"MA/RSI/stdev/variance/CCI/rolling/source-aware MTF 使用稳定 key；close 保留 legacy key。"},
        {"capability":"Rolling and stateful indicators","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":false,"notes":"highest/lowest/change/mom/roc/rising/falling/sum、barssince、valuewhen 已可执行；前端只覆盖常用子集。"},
        {"capability":"MTF request.security subset","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"同标的 source/source[n]/source-aware MA、静态 intraday 高级指标、v1.4 纯表达式、v1.5 common TA pure-expression、v1.6 tuple 白名单、v2.2 2-8 元纯表达式 tuple、v2.3 纯 collection/object 表达式，以及 v2.4 MTF stoch；禁止 lookahead_on/gaps_on、动态 symbol/timeframe、side effect 和 nested request。"},
        {"capability":"Orders and exits","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"entry/order/close/close_all/exit/cancel 的可执行子集已贯通；entry 反手与 allow_entry_in 已支持，完整 broker emulator 不属于当前目标。"},
        {"capability":"UDF, switch and static for","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":false,"notes":"表达式/受控多语句 UDF、switch 与静态整数 for 编译期展开；静态 for 内条件 break/continue 会回退到 bounded runtime loop；递归 UDF 诊断失败。"},
        {"capability":"v2.0 language foundation","parser":true,"planner":false,"runtime":false,"jftrade":true,"frontend":true,"notes":"array/map/matrix typed declaration、constructor、namespace/method-style operation、type/method/import alias/library、UDT object operation 和视觉 API 已进入 parse/semantic/top-level metadata 模型；collection namespace/type argument compatibility、visual kind/variable/target/title、type fields、method receiver/parameters/defaults、duplicate declaration/receiver/overload diagnostics、object constructor/method signatures、object arity diagnostics 与 import version/alias 可分析，非执行表面返回明确诊断。"},
        {"capability":"v2.4 collection/map, MTF stoch and persistent object expansion","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"array.from/concat/join/sort/sort_indices/binary_search/median/mode/range、map.copy/keys/values、order.ascending/descending、MTF ta.stoch、静态 for 条件 break/continue runtime fallback、持久 object 字段重赋值已进入 1250+ 语料门禁。"},
        {"capability":"v3.3 advanced language boundary diagnostics","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"AnalyzeScript 对递归 UDF、嵌套 UDF、UDF 签名问题、循环嵌套/迭代上限和循环变量只读返回稳定分码诊断；动态 for/while、collection for、break/continue 和 loop runtime 上限继续作为闭盘可执行子集的受控边界。"},
        {"capability":"v3.4 generated support snapshot","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"pnpm run generate:reference 生成 docs/reference/generated/pine-v6-support.md，将 ProductVersion、score model、compatibility dimensions、capability registry、support matrix 和 unsupported patterns 固化为可 diff 快照；pinespec 测试会拒绝过期快照。"},
        {"capability":"v4.0 broker emulator boundary decision","parser":true,"planner":true,"runtime":true,"jftrade":true,"frontend":true,"notes":"完整 TradingView broker emulator、OCA、partial fill、intrabar tick recalculation 和多标的组合撮合正式作为单独 trading-runtime parity track，排除在 JFTrade executable Pine v6 completion score 之外；brokerBoundary payload 与生成快照列出 scoreTreatment 和稳定诊断码。"}
    ])
}
fn broker_boundary() -> Value {
    json!([
        {"area":"Closed-bar order model","status":"supported","scoreTreatment":"included in executable Pine v6 score","diagnosticCodes":[],"notes":"strategy.entry/order/close/close_all/exit/cancel 在 K 线收盘执行；stop-limit、bracket、trailing、reversal、allow_entry_in、commission、slippage 和 process_orders_on_close 有专门可执行测试。"},
        {"area":"OCA and partial fill","status":"out_of_scope","scoreTreatment":"excluded from executable Pine v6 score and listed as unsupported order capability","diagnosticCodes":["PINE_ORDER_OCA_UNSUPPORTED"],"notes":"oca_name/oca_type、partial fill 和 OCA reduce/cancel 组合属于 TradingView broker-emulator parity track，不计入 JFTrade closed-bar Pine completion。"},
        {"area":"Intrabar tick recalculation","status":"out_of_scope","scoreTreatment":"excluded from executable Pine v6 score and listed as unsupported order capability","diagnosticCodes":["PINE_BROKER_EMULATOR_OUT_OF_SCOPE"],"notes":"tick 级重算、intrabar path 推断、bar magnifier 和同一根 K 线内部成交路径不属于当前 runtime；当前策略只在闭盘 hook 执行。"},
        {"area":"Advanced strategy.exit broker semantics","status":"diagnostic_only","scoreTreatment":"supported subset counted; unsupported combinations stay outside score","diagnosticCodes":["PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED","PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED"],"notes":"基础 stop、limit、stop+limit bracket、trail_points/trail_price + trail_offset 可执行；trail 与 bracket 混用、无触发器 exit 和高级 broker emulator 语义返回稳定诊断。"},
        {"area":"Full TradingView broker emulator","status":"out_of_scope","scoreTreatment":"tracked separately as order.full_tv_broker_emulator, not used to inflate Pine language completion","diagnosticCodes":["PINE_BROKER_EMULATOR_OUT_OF_SCOPE"],"notes":"完整 TradingView broker emulator、保证金清算、多标的组合撮合和 partial fill parity 需要单独 trading-runtime track；v4.0 正式将其排除在 JFTrade executable Pine v6 completion 之外。"}
    ])
}
fn compatibility_dimensions() -> Value {
    json!([
        {"id":"language","weight":0.12,"score":98.86,"supportedWeight":424.60,"totalWeight":429.50,"unsupportedIds":["syntax.recursive_nested_udf"]},
        {"id":"indicators","weight":0.30,"score":96.44,"supportedWeight":132.70,"totalWeight":137.60,"unsupportedIds":["indicator.full_ta_surface","indicator.visual_only_plot_stack"]},
        {"id":"mtf","weight":0.48,"score":99.03,"supportedWeight":244.40,"totalWeight":246.80,"unsupportedIds":["request.security.dynamic_symbol_timeframe","request.security.lookahead_gaps_on"]},
        {"id":"orders","weight":0.00,"score":77.94,"supportedWeight":21.90,"totalWeight":28.10,"unsupportedIds":["order.oca_partial_fill","order.intrabar_tick_recalc","order.full_tv_broker_emulator"]},
        {"id":"tooling","weight":0.10,"score":99.71,"supportedWeight":558.00,"totalWeight":559.60,"unsupportedIds":[]}
    ])
}
fn capabilities() -> Value {
    json!([
        {"id":"metadata.version6","dimension":"tooling","status":"supported","weight":1.0,"layers":{"parser":true,"planner":true,"runtime":true,"backtest":true,"frontend":true,"spec":true},"testIds":["TestGoldenExamplesAnalyzeAndPlan"]},
        {"id":"indicator.ma","dimension":"indicators","status":"supported","weight":1.0,"layers":{"parser":true,"planner":true,"runtime":true,"backtest":true,"frontend":true,"spec":true},"testIds":["TestGoldenExamplesAnalyzeAndPlan"]},
        {"id":"request.security.v32_lower_timeframe_preflight","dimension":"mtf","status":"supported","weight":6.0,"layers":{"parser":true,"planner":true,"runtime":true,"backtest":true,"frontend":true,"spec":true},"testIds":["TestRequestSecurityTimeframeRequirementsValidateAgainstStrategyInterval"]},
        {"id":"order.oca_partial_fill","dimension":"orders","status":"unsupported","weight":2.2,"layers":{"parser":false,"planner":false,"runtime":false,"backtest":false,"frontend":false,"spec":true},"testIds":[],"notes":"OCA、partial fill 和完整 broker emulator 暂不支持。"},
        {"id":"strategy.v40_broker_boundary_decision","dimension":"orders","status":"supported","weight":6.0,"layers":{"parser":true,"planner":true,"runtime":true,"backtest":true,"frontend":false,"spec":true},"testIds":["TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics","TestBuildToolPayloadIncludesBrokerBoundary","TestGeneratedPineSupportSnapshotIsCurrent"]},
        {"id":"tooling.v34_generated_support_snapshot","dimension":"tooling","status":"supported","weight":6.0,"layers":{"parser":true,"planner":true,"runtime":true,"backtest":true,"frontend":true,"spec":true},"testIds":["TestGeneratedPineSupportSnapshotIsCurrent","TestBuildToolPayloadIncludesSupportMatrix"]},
        {"id":"tooling.v40_broker_boundary_snapshot","dimension":"tooling","status":"supported","weight":6.0,"layers":{"parser":true,"planner":true,"runtime":true,"backtest":true,"frontend":true,"spec":true},"testIds":["TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics","TestBuildToolPayloadIncludesBrokerBoundary","TestGeneratedPineSupportSnapshotIsCurrent"]}
    ])
}
fn external_engine_payload() -> Value {
    json!({
        "engine": PINE_TS_ENGINE,
        "mode": "off",
        "enabled": false,
        "status": "disabled",
        "license": PINE_TS_LICENSE,
        "package": PINE_TS_PACKAGE,
        "repository": PINE_TS_REPOSITORY,
        "worker": PINE_TS_WORKER,
        "authority": "pine-pinets production runtime remains authoritative",
        "scope": "indicator and signal shadow evaluation only",
        "strategyMetrics": ["buy_and_hold_pnl", "buy_and_hold_per_gain", "strategy_outperformance"],
        "compliance": {
            "license": PINE_TS_LICENSE,
            "commercialLicense": false,
            "sourceOffer": "docs/legal/third-party-notices.md",
            "networkUseNotice": "If PineTS functionality is exposed over a network, provide corresponding source and license notices for the AGPL-covered integration."
        },
        "differenceSummary": {
            "evaluated": false,
            "reason": "external PineTS shadow engine is disabled by default"
        }
    })
}
fn validation_external_engine_payload() -> Value {
    json!({
        "enabled": false,
        "mode": "off",
        "engine": PINE_TS_ENGINE,
        "engineVersion": "",
        "license": "",
        "repository": "",
        "ok": false,
        "status": "disabled",
        "diagnostics": [],
        "differenceSummary": {
            "evaluated": false,
            "reason": "external PineTS shadow engine is disabled by default"
        },
        "compliance": {
            "license": PINE_TS_LICENSE,
            "commercialLicense": false,
            "sourceOffer": "docs/legal/third-party-notices.md",
            "networkUseNotice": "If PineTS functionality is exposed over a network, provide corresponding source and license notices for the AGPL-covered integration."
        }
    })
}
fn save_hint_payload() -> SaveHintPayload {
    SaveHintPayload {
        message: format!(
            "可以先查询 Pine v6 规范和示例，确认脚本格式正确。也可以从下面这个 JFTrade Pine v6 骨架开始：\n{SKELETON}"
        ),
        spec_tool: TOOL_NAME.to_owned(),
        resource_files: vec![
            "references/pine-v6-spec.md".to_owned(),
            "references/pine-v6-examples.md".to_owned(),
        ],
        skeleton: SKELETON.to_owned(),
    }
}
fn example_payload(
    (id, title, description, script, requirement_keys): (&str, &str, &str, &str, &[&str]),
) -> Value {
    json!({
        "id": id,
        "title": title,
        "description": description,
        "script": script,
        "requirementKeys": requirement_keys,
    })
}
fn golden_scripts() -> Value {
    Value::Array(
        golden_example_scripts()
            .into_iter()
            .map(example_payload)
            .collect(),
    )
}
fn example_scripts() -> Vec<(
    &'static str,
    &'static str,
    &'static str,
    &'static str,
    &'static [&'static str],
)> {
    vec![
        (
            "minimal-log",
            "最小可保存草稿",
            "可保存为 native Pine Script v6 策略定义的最小完整脚本。",
            SKELETON,
            &[],
        ),
        (
            "ema-crossover",
            "EMA 均线交叉",
            "一个基础均线交叉脚本：快 EMA 上穿慢 EMA 时开多。",
            "//@version=6\nstrategy(\"EMA Crossover\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\n\nfast = ta.ema(close, 8)\nslow = ta.ema(close, 21)\nif ta.crossover(fast, slow)\n    strategy.entry(\"Long\", strategy.long)\nelse\n    alert(\"waiting for next crossover\")",
            &[],
        ),
        (
            "rsi-protect",
            "RSI 与 protect",
            "一个均值回归草稿：在 RSI 超卖时入场。",
            "//@version=6\nstrategy(\"RSI Reversion\", overlay=true)\n\nrsi14 = ta.rsi(close, 14)\nif rsi14 < 30\n    strategy.entry(\"Long\", strategy.long, qty=100)\nelse\n    log.info(\"RSI condition not met\")",
            &[],
        ),
        (
            "v10-golden-capability-set",
            "v1.0 主路径黄金脚本",
            "覆盖当前 v1.0 Pine v6 主路径的可执行 smoke：source-aware 指标、MTF、SAR、UDF、静态 for、qty_percent、net order、bracket exit 和 cancel。",
            "//@version=6\nstrategy(\"v1.0 Golden\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)\n\nlen = input.int(3, \"Length\")\ntf = input.timeframe(\"15\", \"MTF\")\nisBull(src) => src > src[1]\n\nfast = ta.ema(close, len)\navgVol = ta.sma(volume, 2)\nsar = ta.sar(0.02, 0.02, 0.2)\nmtfClose = request.security(syminfo.tickerid, tf, close)\nmtfEma = request.security(syminfo.tickerid, \"15\", ta.ema(hlc3, 3))\nsum = 0\nfor i = 0 to 2\n    sum := sum + nz(close[i], close)\n\nif barstate.isconfirmed and session.ismarket and isBull(close) and close > fast and volume > avgVol and close > sar and mtfClose > mtfEma and sum > 0\n    strategy.entry(\"Long\", strategy.long, qty_percent=10)\n    strategy.order(\"Net\", strategy.long, qty=1)\n    strategy.exit(\"Bracket\", \"Long\", stop=close * 0.98, limit=close * 1.04, qty_percent=50)\nelse\n    strategy.cancel(\"Long\")",
            &[],
        ),
    ]
}

fn golden_example_scripts() -> Vec<(
    &'static str,
    &'static str,
    &'static str,
    &'static str,
    &'static [&'static str],
)> {
    vec![
        (
            "golden-ma-cross",
            "均线交叉",
            "覆盖 close-source EMA/SMA、crossover 和基础 entry。",
            "//@version=6\nstrategy(\"Golden MA Cross\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\n\nfast = ta.ema(close, 8)\nslow = ta.sma(close, 21)\nif ta.crossover(fast, slow)\n    strategy.entry(\"Long\", strategy.long)",
            &["ma:EMA:8", "ma:SMA:21"],
        ),
        (
            "golden-oscillators-bands",
            "RSI/CCI/Williams/Bollinger",
            "覆盖常见震荡指标、Bollinger 三元组和 close/hlc3 legacy key。",
            "//@version=6\nstrategy(\"Golden Oscillators\", overlay=true)\n\nrsi14 = ta.rsi(close, 14)\ncci20 = ta.cci(hlc3, 20)\nwilliams = ta.wpr(14)\n[basis, upper, lower] = ta.bb(close, 20, 2)\nif rsi14 < 35 and cci20 < -100 and williams < -80 and close < lower\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &["rsi:14", "cci:20", "williamsr:14", "bollinger:20:2"],
        ),
        (
            "golden-donchian-volume-sar",
            "Donchian、volume MA 与 SAR",
            "覆盖 rolling highest/lowest、source-aware volume SMA 和 Parabolic SAR。",
            "//@version=6\nstrategy(\"Golden Donchian Volume SAR\", overlay=true)\n\nupper = ta.highest(high, 20)\nlower = ta.lowest(low, 20)\navgVol = ta.sma(volume, 10)\nsar = ta.sar(0.02, 0.02, 0.2)\nif close > upper and volume > avgVol and close > sar\n    strategy.entry(\"Long\", strategy.long, qty=1)\nif close < lower\n    strategy.close(\"Long\")",
            &[
                "highest:high:20",
                "lowest:low:20",
                "ma:SMA:10:volume",
                "sar:0.02:0.02:0.2",
            ],
        ),
        (
            "golden-mtf-source-ma",
            "MTF source、MA 与高级指标",
            "覆盖 input.timeframe、request.security source/source[n]、source-aware MTF EMA 与静态 intraday MTF linreg。",
            "//@version=6\nstrategy(\"Golden MTF\", overlay=true)\n\ntf = input.timeframe(\"15\", \"Signal TF\")\nmtfClose = request.security(syminfo.tickerid, tf, close)\nmtfPrevClose = request.security(syminfo.tickerid, tf, close[1])\nmtfEma = request.security(syminfo.tickerid, \"15\", ta.ema(hlc3, 3))\nmtfLinreg = request.security(syminfo.tickerid, \"15\", ta.linreg(close, 5, 0))\nif mtfClose > mtfPrevClose and close > mtfEma and close > mtfLinreg\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "security_source:15m:close",
                "security_source:15m:close:1",
                "ma:EMA:3:15m:hlc3",
                "linreg:close:5:0:15m",
            ],
        ),
        (
            "golden-orders-exits",
            "qty_percent、pending、bracket、cancel",
            "覆盖 percent sizing、strategy.order、pending stop、bracket exit 和 cancel。",
            "//@version=6\nstrategy(\"Golden Orders\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)\n\nif close > open\n    strategy.entry(\"Long\", strategy.long, qty_percent=10)\n    strategy.order(\"NetLong\", strategy.long, qty=1)\n    strategy.exit(\"Bracket\", \"Long\", stop=close * 0.98, limit=close * 1.04, qty_percent=50)\nelse\n    strategy.entry(\"Breakout\", strategy.long, stop=high + 1, qty=1)\n    strategy.cancel(\"Breakout\")",
            &[],
        ),
        (
            "golden-udf-static-for",
            "UDF 与静态 for",
            "覆盖单表达式 UDF、历史引用、input.int 默认值和静态 for 展开。",
            "//@version=6\nstrategy(\"Golden UDF Static For\", overlay=true)\n\nisBull(src) => src > src[1]\nlen = input.int(3, \"Length\")\nfast = ta.ema(close, len)\nsum = 0\nfor i = 0 to 2\n    sum := sum + nz(close[i], close)\nif isBull(close) and fast > fast[1] and sum > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &["ma:EMA:3"],
        ),
        (
            "golden-v12-advanced-indicators",
            "v1.2 高频迁移指标",
            "覆盖 linreg、OBV、pivot、Keltner Channel/KCW 与 ALMA。",
            "//@version=6\nstrategy(\"Golden v1.2 Indicators\", overlay=true)\n\nlr = ta.linreg(close, 5, 0)\nobvValue = ta.obv\npivotHigh = ta.pivothigh(high, 2, 2)\npivotLow = ta.pivotlow(low, 2, 2)\n[basis, upper, lower] = ta.kc(close, 5, 1.5)\nwidth = ta.kcw(close, 5, 1.5)\nalmaValue = ta.alma(close, 5, 0.85, 6)\nif close > lr and obvValue > 0 and upper > lower and width > 0 and almaValue > 0 and nz(pivotHigh, close) >= nz(pivotLow, close)\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "linreg:close:5:0",
                "obv:close",
                "pivothigh:high:2:2",
                "pivotlow:low:2:2",
                "kc:close:5:1.5:true",
                "kcw:close:5:1.5:true",
                "alma:close:5:0.85:6",
            ],
        ),
        (
            "golden-v13-migration-indicators",
            "v1.3 高频迁移指标",
            "覆盖 CMO、TSI、correlation、dev、median、percentile、percentrank、SWMA、math.avg/round_to_mintick 和 v1.3 intraday MTF 指标。",
            "//@version=6\nstrategy(\"Golden v1.3 Indicators\", overlay=true)\n\ncmoValue = ta.cmo(close, 5)\ntsiValue = ta.tsi(close, 2, 3)\ncorrValue = ta.correlation(close, high, 5)\ndevValue = ta.dev(close, 5)\nmedianValue = ta.median(close, 5)\npLinear = ta.percentile_linear_interpolation(close, 5, 50)\npNearest = ta.percentile_nearest_rank(close, 5, 80)\nrankValue = ta.percentrank(close, 5)\nswmaValue = ta.swma(close)\nmtfCmo = request.security(syminfo.tickerid, \"15\", ta.cmo(close, 5))\nrounded = math.round_to_mintick(math.avg(close, open))\nif cmoValue > 0 and tsiValue > 0 and corrValue > 0 and devValue > 0 and medianValue > 0 and pLinear > 0 and pNearest > 0 and rankValue > 0 and swmaValue > 0 and mtfCmo > 0 and rounded > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "cmo:close:5",
                "tsi:close:2:3",
                "correlation:close:high:5",
                "dev:close:5",
                "median:close:5",
                "percentile_linear_interpolation:close:5:50",
                "percentile_nearest_rank:close:5:80",
                "percentrank:close:5",
                "swma:close",
                "cmo:close:5:15m",
            ],
        ),
        (
            "golden-v14-window-momentum",
            "v1.4 窗口与动量指标",
            "覆盖 highestbars、lowestbars、change、mom、roc、rising、falling、stdev 与 variance。",
            "//@version=6\nstrategy(\"Golden v1.4 Window Momentum\", overlay=true)\n\ndev = ta.stdev(close, 5)\nvariance = ta.variance(close, 5)\nhb = ta.highestbars(high, 5)\nlb = ta.lowestbars(low, 5)\ndelta = ta.change(close)\nmomentum = ta.mom(close, 3)\nrate = ta.roc(close, 3)\nup = ta.rising(close, 3)\ndown = ta.falling(close, 3)\nif up and not down and nz(dev, 0) >= 0 and nz(variance, 0) >= 0 and hb >= 0 and lb >= 0 and nz(delta, 0) + nz(momentum, 0) + nz(rate, 0) > -100\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "stdev:5",
                "variance:close:5",
                "highestbars:high:5",
                "lowestbars:low:5",
                "change:close:1",
                "mom:close:3",
                "roc:close:3",
                "rising:close:3",
                "falling:close:3",
            ],
        ),
        (
            "golden-v14-state-events",
            "v1.4 状态事件函数",
            "覆盖 barssince 与 valuewhen 的 closed-bar 状态语义。",
            "//@version=6\nstrategy(\"Golden v1.4 State Events\", overlay=true)\n\nbars = ta.barssince(close > open)\nvalue = ta.valuewhen(close > open, close, 0)\nif nz(bars, 999) < 4 and nz(value, close) >= close\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[],
        ),
        (
            "golden-v14-tr-atr",
            "v1.4 TR/ATR 组合",
            "覆盖 ta.tr(true|false) 与 ta.atr 的边界组合。",
            "//@version=6\nstrategy(\"Golden v1.4 TR ATR\", overlay=true)\n\ntrTrue = ta.tr(true)\ntrFalse = ta.tr(false)\nrange = ta.atr(5)\nif trTrue >= trFalse and trTrue > 0 and nz(range, trTrue) > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &["atr:5"],
        ),
        (
            "golden-v14-mtf-pure-expression",
            "v1.4 MTF 纯表达式",
            "覆盖同标的静态 timeframe 的 request.security 纯表达式、source history、MA、math 与 nz 组合。",
            "//@version=6\nstrategy(\"Golden v1.4 MTF Pure\", overlay=true)\n\nsignal = request.security(syminfo.tickerid, \"15\", close > ta.sma(close, 3) and nz(close[1], close) > open and math.avg(close, open) > 0)\nif signal\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "security_source:15m:close",
                "security_source:15m:close:1",
                "security_source:15m:open",
                "ma:SMA:3:15m",
            ],
        ),
        (
            "golden-v15-mtf-common-ta",
            "v1.5 MTF common TA",
            "覆盖 request.security 纯表达式中的 RSI、MACD、ATR、Bollinger 与 Supertrend 成员读取。",
            "//@version=6\nstrategy(\"Golden v1.5 MTF Common TA\", overlay=true)\n\nsignal = request.security(syminfo.tickerid, \"15\", nz(ta.rsi(close, 14), 50) > 50 and nz(ta.macd(close, 12, 26, 9).diff, 0) > 0 and nz(ta.atr(14), 0) > 0 and nz(ta.bb(close, 20, 2).upper, close) > close and nz(ta.supertrend(3, 10).direction, 0) > 0)\nif signal\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "security_source:15m:close",
                "rsi:close:14:15m",
                "macd:close:12:26:9:15m",
                "atr:14:15m",
                "bollinger:close:20:2:15m",
                "supertrend:3:10:15m",
            ],
        ),
        (
            "golden-v15-cross-state",
            "v1.5 交叉与状态事件",
            "覆盖 crossover/crossunder/cross 与 barssince/valuewhen 的常见迁移组合。",
            "//@version=6\nstrategy(\"Golden v1.5 Cross State\", overlay=true)\n\nfast = ta.ema(close, 8)\nslow = ta.sma(close, 21)\nrecentCross = ta.barssince(ta.cross(fast, slow))\nlastCrossClose = ta.valuewhen(ta.crossover(fast, slow), close, 0)\nif ta.crossover(fast, slow) or (nz(recentCross, 999) < 5 and close > nz(lastCrossClose, close))\n    strategy.entry(\"Long\", strategy.long, qty=1)\nif ta.crossunder(fast, slow)\n    strategy.close(\"Long\")",
            &["ma:EMA:8", "ma:SMA:21"],
        ),
        (
            "golden-v15-static-loop-control",
            "v1.5 静态 for 控制",
            "覆盖静态 for 展开中的无条件 continue 与 break 子集。",
            "//@version=6\nstrategy(\"Golden v1.5 Static Loop Control\", overlay=true)\n\nscore = 0\nfor i = 1 to 4\n    score := score + i\n    continue\n    score := score + 100\nfor j = 1 to 4\n    score := score + j\n    break\n    score := score + 100\nif score > 0\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[],
        ),
        (
            "golden-v16-mtf-tuple-whitelist",
            "v1.6 MTF tuple 白名单",
            "覆盖 request.security 的 source、纯表达式与常见多返回 TA tuple 白名单。",
            "//@version=6\nstrategy(\"Golden v1.6 MTF Tuple\", overlay=true)\n\n[mtfClose, mtfFast, mtfUp] = request.security(syminfo.tickerid, \"15\", [close, ta.ema(hlc3, 5), close > ta.sma(close, 3)])\n[macdLine, signalLine, histLine] = request.security(syminfo.tickerid, \"15\", ta.macd(close, 12, 26, 9))\n[basis, upper, lower] = request.security(syminfo.tickerid, \"15\", ta.bb(close, 20, 2))\nif mtfClose > mtfFast and mtfUp and histLine > signalLine and close < lower\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "security_source:15m:close",
                "ma:EMA:5:15m:hlc3",
                "ma:SMA:3:15m",
                "macd:close:12:26:9:15m",
                "bollinger:close:20:2:15m",
            ],
        ),
        (
            "golden-v17-semantic-transition",
            "v1.7 Semantic 过渡",
            "覆盖 semantic summary 可识别的 input、series symbol、MTF tuple、UDF 与函数签名路径。",
            "//@version=6\nstrategy(\"Golden v1.7 Semantic\", overlay=true)\n\nlen = input.int(8, \"Length\")\nscore(src) =>\n    base = ta.sma(src, 8)\n    if base > 0\n        src / base\n    else\n        1\nfast = ta.ema(close, len)\n[mtfClose, mtfFast] = request.security(syminfo.tickerid, \"15\", [close, ta.ema(close, 5)])\nif score(close) > 0 and mtfClose > mtfFast and fast > fast[1]\n    strategy.entry(\"Long\", strategy.long, qty=1)",
            &[
                "ma:SMA:8",
                "ma:EMA:8",
                "security_source:15m:close",
                "ma:EMA:5:15m",
            ],
        ),
    ]
}
