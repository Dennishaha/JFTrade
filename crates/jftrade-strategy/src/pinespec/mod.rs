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
#[error("strategy.pine_spec does not support section {section:?}; allowed values: {allowed}")]
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
        "compatibilityScore": 0,
        "scoreModelVersion": "native-rust-v1",
        "compatibilityDimensions": {},
        "brokerBoundary": broker_boundary(),
        "externalEngine": {"engine": "pine-pinets", "mode": "off", "enabled": false, "status": "disabled"},
        "unsupportedPatterns": ["import/library/type/method declarations", "dynamic external request.security symbols", "intrabar broker emulator"],
        "goldenScripts": [],
        "skeleton": "//@version=6\nstrategy(\"My Strategy\", overlay=true)\nif close > open\n    strategy.entry(\"Long\", strategy.long)",
        "examples": []
    });
    if !selected.is_empty() {
        payload["sectionContent"] = Value::Array(
            section_content(&selected)
                .into_iter()
                .map(Value::String)
                .collect(),
        );
    }
    if include_examples || selected == "examples" {
        payload["examples"] = Value::Array(example_scripts().into_iter().map(|(id, title, script)| json!({"id": id, "title": title, "description": "可成功 parse、lower 并完成 requirements planning 的最小示例。", "script": script, "requirementKeys": []})).collect());
    }
    Ok(payload)
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ValidationPayload {
    pub ok: bool,
    pub source_format: String,
    pub runtime: String,
    pub normalized_script: String,
    pub metadata: Value,
    pub hooks: Vec<String>,
    pub requirements: Option<Requirements>,
    pub warnings: Vec<String>,
    pub errors: Vec<String>,
    pub diagnostics: Vec<Diagnostic>,
    pub features: Vec<String>,
    pub ast: Option<crate::pine::Program>,
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
            normalized_script,
            metadata: default_metadata(),
            hooks: Vec::new(),
            requirements: None,
            warnings: Vec::new(),
            errors: vec!["script is required".to_owned()],
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
    ValidationPayload {
        ok: compilation.ok,
        source_format: SOURCE_FORMAT.to_owned(),
        runtime: RUNTIME.to_owned(),
        normalized_script,
        metadata,
        hooks,
        requirements: include_requirements.then_some(compilation.requirements.clone()),
        warnings: compilation.warnings.clone(),
        errors: compilation
            .diagnostics
            .iter()
            .filter(|diagnostic| diagnostic.severity == crate::pine::DiagnosticSeverity::Error)
            .map(|diagnostic| diagnostic.message.clone())
            .collect(),
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
fn section_content(section: &str) -> Vec<String> {
    match section { "overview" => vec!["Pine v6 策略定义由 native parser、semantic checker、lowerer 和 requirements planner 处理。".to_owned()], "syntax" => vec!["脚本必须包含 //@version=6 和 strategy(...)；if/else 使用缩进块。".to_owned()], "expressions" => vec!["支持 OHLCV、算术、比较、布尔运算、历史引用和三元表达式。".to_owned()], "indicators" => vec!["支持 source-aware ta.* 指标和同标的静态 request.security。".to_owned()], "orders" => vec!["订单语句只产生 strategy order intents，不直接写交易状态。".to_owned()], "unsupported" => vec!["library/import、动态外部 symbol 和完整 intrabar broker emulator 不支持。".to_owned()], "examples" => vec!["示例脚本与当前 native compiler 共享同一份可执行子集。".to_owned()], _ => Vec::new() }
}
fn reserved_variables() -> Value {
    json!([{"name":"close","description":"当前及历史 close 序列。"},{"name":"open","description":"当前及历史 open 序列。"},{"name":"high","description":"当前及历史 high 序列。"},{"name":"low","description":"当前及历史 low 序列。"},{"name":"volume","description":"当前及历史 volume 序列。"},{"name":"strategy.equity","description":"当前账户总权益。"},{"name":"strategy.position_size","description":"当前策略持仓数量。"}])
}
fn indicator_functions() -> Value {
    json!([{"name":"ta.ema","signature":"ta.ema(source, period)"},{"name":"ta.sma","signature":"ta.sma(source, period)"},{"name":"ta.rsi","signature":"ta.rsi(source, period)"},{"name":"ta.macd","signature":"ta.macd(close, fast, slow, signal)"},{"name":"ta.atr","signature":"ta.atr(period)"},{"name":"request.security","signature":"request.security(syminfo.tickerid, timeframe, expression)"}])
}
fn support_matrix() -> Value {
    json!([{"capability":"lexer","parser":true,"planner":true,"runtime":true},{"capability":"typed expressions","parser":true,"planner":true,"runtime":true},{"capability":"strategy orders","parser":true,"planner":true,"runtime":false},{"capability":"PineTS worker","parser":false,"planner":false,"runtime":"external"}])
}
fn broker_boundary() -> Value {
    json!([{"area":"Closed-bar order model","status":"supported","scoreTreatment":"included"},{"area":"Intrabar broker emulator","status":"out_of_scope","scoreTreatment":"excluded"}])
}
fn example_scripts() -> Vec<(&'static str, &'static str, &'static str)> {
    vec![
        (
            "minimal",
            "最小策略",
            "//@version=6\nstrategy(\"Minimal\", overlay=true)\nif close > open\n    strategy.entry(\"Long\", strategy.long)",
        ),
        (
            "ema",
            "EMA crossover",
            "//@version=6\nstrategy(\"EMA\", overlay=true)\nfast = ta.ema(close, 8)\nslow = ta.ema(close, 21)\nif ta.crossover(fast, slow)\n    strategy.entry(\"Long\", strategy.long)",
        ),
    ]
}
