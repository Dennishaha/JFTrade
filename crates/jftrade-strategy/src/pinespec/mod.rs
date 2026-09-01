//! Versioned Pine specification payload shared by strategy tools.
//!
//! The payload is generated from this Rust source at runtime (rather than
//! copied from a fixture), so a caller always sees the same section list and
//! executable subset as the native compiler.

use serde::Serialize;
use serde_json::{Value, json};
use thiserror::Error;

use crate::pine::{Diagnostic, LoweredProgram, Requirements, compile};

pub const PINE_VERSION: &str = "v6";
pub const PRODUCT_VERSION: &str = "v4.0";
pub const SOURCE_FORMAT: &str = "pine-v6";
pub const RUNTIME: &str = "pine-pinets";
pub const TOOL_NAME: &str = "strategy.pine_spec";

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
        ("overview", "概览", "说明 Pine v6、策略定义与运行时边界。"),
        ("syntax", "语法", "声明、缩进块、赋值和注释。"),
        ("expressions", "表达式", "表达式运算、历史引用和条件类型。"),
        (
            "indicators",
            "指标",
            "可规划的 ta.* 指标与 request.security。",
        ),
        (
            "orders",
            "下单",
            "strategy.entry/order/close/exit/cancel 映射。",
        ),
        ("unsupported", "不支持项", "明确拒绝或仅诊断的 Pine 行为。"),
        (
            "examples",
            "示例",
            "可成功 parse、lower 并完成 requirements planning 的脚本。",
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
        "compatibilityScore": 0,
        "scoreModelVersion": "native-rust-v1",
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
    pub diagnostics: Vec<Diagnostic>,
    pub features: Vec<String>,
    pub ast: Option<crate::pine::Program>,
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
    include_ast: bool,
) -> ValidationPayload {
    let normalized_script = source.trim().to_owned();
    if normalized_script.is_empty() {
        return ValidationPayload {
            ok: false,
            source_format: SOURCE_FORMAT.to_owned(),
            runtime: RUNTIME.to_owned(),
            external_engine: external_engine_payload(),
            normalized_script,
            metadata: default_metadata(),
            hooks: Vec::new(),
            requirements: None,
            warnings: Vec::new(),
            errors: vec!["script 是必填项".to_owned()],
            save_hint: Some(save_hint_payload()),
            diagnostics: vec![Diagnostic::error(
                "PINE_EMPTY_SCRIPT",
                "script is required",
                1,
            )],
            features: crate::pine::supported_features(),
            ast: None,
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
        external_engine: external_engine_payload(),
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
        diagnostics: compilation.diagnostics,
        features: compilation.features,
        ast: include_ast
            .then(|| crate::pine::parse(source).ok())
            .flatten(),
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
            "说明 Pine v6、策略定义与运行时边界。",
            vec![
                "native parser、semantic checker、lowerer 和 requirements planner 只覆盖明确的可执行子集。",
                "Rust leaf 当前不拥有 PineTS worker、交易撮合或持久化状态。",
            ],
        ),
        "syntax" => (
            "语法",
            "声明、缩进块、赋值和注释。",
            vec![
                "脚本必须包含 //@version=6 和 strategy(...)。",
                "if/else 使用 Pine 风格缩进块，var 和 := 保持显式状态语义。",
            ],
        ),
        "expressions" => (
            "表达式",
            "表达式运算、历史引用和条件类型。",
            vec![
                "支持 OHLCV、算术、比较、布尔运算、历史引用和三元表达式。",
                "条件表达式必须是 bool；数值不能直接作为 if 条件。",
            ],
        ),
        "indicators" => (
            "指标",
            "可规划的 ta.* 指标与 request.security。",
            vec![
                "支持 source-aware ta.* 指标和同标的静态 request.security。",
                "requirements planner 只记录当前 native compiler 能解析的指标调用。",
            ],
        ),
        "orders" => (
            "下单",
            "strategy.entry/order/close/exit/cancel 映射。",
            vec![
                "订单语句只产生 strategy order intents，不直接写交易状态。",
                "当前 lowerer 将策略动作绑定到闭盘 hook；完整 broker emulator 不在本 leaf 内。",
            ],
        ),
        "unsupported" => (
            "不支持项",
            "明确拒绝或仅诊断的 Pine 行为。",
            vec![
                "library/import/type/method 声明、动态外部 symbol 和完整 intrabar broker emulator 不支持。",
                "未知 built-ins 以稳定诊断拒绝，不会被静态 allowlist 当作已实现。",
            ],
        ),
        "examples" => (
            "示例",
            "可成功 parse、lower 并完成 requirements planning 的脚本。",
            vec!["示例脚本与当前 native compiler 共享同一份可执行子集。"],
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
        {"capability":"native lexer/parser","parser":true,"planner":false,"runtime":false,"jftrade":false,"frontend":false,"status":"supported","notes":"字符串、注释、缩进、运算符和调用会进入 typed AST。"},
        {"capability":"native semantic checks","parser":true,"planner":true,"runtime":false,"jftrade":false,"frontend":false,"status":"supported","notes":"版本、strategy 声明、条件类型、调用和不支持声明返回结构化诊断。"},
        {"capability":"native requirements planner","parser":true,"planner":true,"runtime":false,"jftrade":false,"frontend":false,"status":"supported","notes":"指标、持仓和账户权益依赖只做静态规划。"},
        {"capability":"strategy order lowering","parser":true,"planner":true,"runtime":false,"jftrade":false,"frontend":false,"status":"subset","notes":"动作 lower 为 closed-bar order intents，Rust leaf 不执行成交。"},
        {"capability":"PineTS worker","parser":false,"planner":false,"runtime":false,"jftrade":false,"frontend":false,"status":"external","notes":"PineTS 仍是生产执行 runtime；本 native leaf 不启动或调用 worker。"}
    ])
}
fn broker_boundary() -> Value {
    json!([
        {"area":"Closed-bar order model","status":"supported","scoreTreatment":"included in native subset","diagnosticCodes":[],"notes":"策略动作绑定到 K 线收盘 hook；native leaf 只负责 lower。"},
        {"area":"OCA and partial fill","status":"out_of_scope","scoreTreatment":"excluded from native subset","diagnosticCodes":["PINE_ORDER_OCA_UNSUPPORTED"],"notes":"OCA、partial fill 和 OCA reduce/cancel 组合不在当前 native compiler。"},
        {"area":"Intrabar tick recalculation","status":"out_of_scope","scoreTreatment":"excluded from native subset","diagnosticCodes":["PINE_BROKER_EMULATOR_OUT_OF_SCOPE"],"notes":"tick 级重算、intrabar path 和 bar magnifier 不在当前 leaf。"},
        {"area":"Advanced strategy.exit broker semantics","status":"diagnostic_only","scoreTreatment":"supported subset only","diagnosticCodes":["PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED","PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED"],"notes":"未实现的 exit broker 组合返回诊断，不计入 native subset。"},
        {"area":"Full TradingView broker emulator","status":"out_of_scope","scoreTreatment":"tracked separately and excluded","diagnosticCodes":["PINE_BROKER_EMULATOR_OUT_OF_SCOPE"],"notes":"完整 TradingView broker emulator 由独立 trading-runtime track 负责。"}
    ])
}
fn compatibility_dimensions() -> Value {
    json!([
        {"id":"native_parser","weight":1.0,"score":1.0,"supportedWeight":1.0,"totalWeight":1.0,"unsupportedIds":[]},
        {"id":"native_semantic_subset","weight":1.0,"score":1.0,"supportedWeight":1.0,"totalWeight":1.0,"unsupportedIds":[]},
        {"id":"native_requirements_planner","weight":1.0,"score":1.0,"supportedWeight":1.0,"totalWeight":1.0,"unsupportedIds":[]},
        {"id":"strategy_runtime","weight":0.0,"score":0.0,"supportedWeight":0.0,"totalWeight":0.0,"unsupportedIds":["strategy_runtime_unwired"]},
        {"id":"full_pine_v6","weight":0.0,"score":0.0,"supportedWeight":0.0,"totalWeight":0.0,"unsupportedIds":["full_pine_v6_out_of_scope"]}
    ])
}
fn capabilities() -> Value {
    json!([
        {"id":"native_pine_parser","dimension":"language","status":"supported","weight":0.0,"layers":{"parser":true,"planner":false,"runtime":false,"backtest":false,"frontend":false,"spec":true},"testIds":["pine_mcp_contract::native_pipeline_parses_lowers_and_plans_strategy_requirements"],"notes":"只覆盖 native parser 的声明子集。"},
        {"id":"native_semantic_diagnostics","dimension":"language","status":"supported","weight":0.0,"layers":{"parser":true,"planner":true,"runtime":false,"backtest":false,"frontend":false,"spec":true},"testIds":["pine_mcp_contract::semantic_checker_rejects_non_boolean_conditions_and_unsupported_declarations"],"notes":"不支持调用返回稳定诊断。"},
        {"id":"native_requirements_planner","dimension":"tooling","status":"supported","weight":0.0,"layers":{"parser":true,"planner":true,"runtime":false,"backtest":false,"frontend":false,"spec":true},"testIds":["pine_mcp_contract::native_pipeline_parses_lowers_and_plans_strategy_requirements"],"notes":"只规划指标、持仓和账户权益依赖。"},
        {"id":"closed_bar_lowering","dimension":"orders","status":"partial","weight":0.0,"layers":{"parser":true,"planner":true,"runtime":false,"backtest":false,"frontend":false,"spec":true},"testIds":["pine_mcp_contract::native_pipeline_parses_lowers_and_plans_strategy_requirements"],"notes":"lower 为 order intents；本 leaf 不执行成交。"},
        {"id":"pinets_execution","dimension":"runtime","status":"analyzed","weight":0.0,"layers":{"parser":false,"planner":false,"runtime":false,"backtest":false,"frontend":false,"spec":true},"testIds":[],"notes":"生产执行仍由外部 pine-pinets worker 负责。"}
    ])
}
fn external_engine_payload() -> Value {
    json!({
        "engine": PINE_TS_ENGINE,
        "mode": "off",
        "enabled": false,
        "status": "disabled",
        "engineVersion": "",
        "license": PINE_TS_LICENSE,
        "package": PINE_TS_PACKAGE,
        "repository": PINE_TS_REPOSITORY,
        "worker": PINE_TS_WORKER,
        "authority": "pine-pinets production runtime remains authoritative",
        "scope": "indicator and signal shadow evaluation only",
        "strategyMetrics": ["buy_and_hold_pnl", "buy_and_hold_per_gain", "strategy_outperformance"],
        "ok": false,
        "diagnostics": [],
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
    Value::Array(example_scripts().into_iter().map(example_payload).collect())
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
            "快 EMA 上穿慢 EMA 时开多的最小策略。",
            "//@version=6\nstrategy(\"EMA Crossover\", overlay=true)\n\nfast = ta.ema(close, 8)\nslow = ta.ema(close, 21)\nif ta.crossover(fast, slow)\n    strategy.entry(\"Long\", strategy.long)",
            &["ma:EMA:8", "ma:EMA:21"],
        ),
        (
            "rsi-protect",
            "RSI 与保护",
            "RSI 超卖时入场并保持闭盘策略边界。",
            "//@version=6\nstrategy(\"RSI Reversion\", overlay=true)\n\nrsi14 = ta.rsi(close, 14)\nif rsi14 < 30\n    strategy.entry(\"Long\", strategy.long, qty=100)",
            &["rsi:14"],
        ),
    ]
}
