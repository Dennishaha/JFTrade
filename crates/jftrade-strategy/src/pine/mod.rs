//! Native Pine Script v6 analysis used by the strategy MCP leaf.
//!
//! The module intentionally stops at the strategy language boundary.  It has
//! no HTTP, worker, broker or persistence dependency: source is lexed,
//! parsed into a typed AST, checked semantically, lowered into a small
//! executable IR, and finally inspected for runtime requirements.

mod lexer;
mod lower;
mod parser;
mod planner;
mod semantic;
mod semantic_call_scope;

pub use lexer::{LexError, LexedLine, Token, TokenKind, decode_string, lex};
pub use lower::{LowerError, LoweredProgram, lower};
pub use parser::{
    AstNode, BinaryOp, Expr, ExprKind, ParseError, Program, SourceRange, Statement,
    StrategyDeclaration, UnaryOp, parse,
};
pub use planner::{IndicatorRequirement, Requirements, plan_requirements};
pub use semantic::{Diagnostic, DiagnosticSeverity, SemanticSummary, analyze};

use serde::{Deserialize, Serialize};

/// Compiler output consumed by validation and the strategy MCP leaf.
#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Compilation {
    pub normalized_script: String,
    pub program: Option<LoweredProgram>,
    pub requirements: Requirements,
    pub warnings: Vec<String>,
    pub diagnostics: Vec<Diagnostic>,
    pub semantic: SemanticSummary,
    pub features: Vec<String>,
    pub ok: bool,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct AnalysisOptions {
    #[serde(default)]
    pub include_ast: bool,
}

/// Run the complete native pipeline.  A parse or semantic error is returned
/// as diagnostics, not as a panic or a partially successful program.
/// Leading horizontal whitespace on the first line and trailing whitespace are
/// noise, but blank lines at the start of a script are not: Go keeps the
/// original line numbers in every diagnostic, so the compiler may only trim
/// horizontal whitespace and must never drop a leading line.
pub(crate) fn normalize_source(source: &str) -> String {
    source
        .trim_start_matches([' ', '\t', '\u{feff}'])
        .trim_end()
        .to_owned()
}

pub fn compile(source: &str) -> Compilation {
    let normalized_script = normalize_source(source);
    let mut diagnostics = Vec::new();
    let mut semantic = SemanticSummary::default();
    let mut requirements = Requirements::default();
    let mut lowered = None;

    match parse(&normalized_script) {
        Ok(ast) => {
            semantic = analyze(&ast);
            diagnostics.extend(semantic.diagnostics.clone());
            if !has_errors(&diagnostics) {
                match lower(&ast) {
                    Ok(program) => {
                        match plan_requirements(&program) {
                            Ok(planned) => requirements = planned,
                            Err(error) => diagnostics.push(Diagnostic::error(
                                "PINE_REQUIREMENTS_INVALID",
                                error.to_string(),
                                error.line(),
                            )),
                        }
                        lowered = Some(program);
                    }
                    Err(error) => diagnostics.push(Diagnostic::error(
                        error.code(),
                        error.to_string(),
                        error.line(),
                    )),
                }
            }
        }
        Err(error) => diagnostics.push(Diagnostic::error(
            error.code(),
            error.to_string(),
            error.line(),
        )),
    }
    let warnings = diagnostics
        .iter()
        .filter(|diagnostic| diagnostic.severity == DiagnosticSeverity::Warning)
        .map(|diagnostic| format!("pine line {}: {}", diagnostic.line, diagnostic.message))
        .collect();
    Compilation {
        normalized_script,
        program: lowered,
        requirements,
        warnings,
        ok: !has_errors(&diagnostics),
        diagnostics,
        semantic,
        features: supported_features(),
    }
}

pub fn analyze_script(source: &str, _options: AnalysisOptions) -> Compilation {
    compile(source)
}

pub fn supported_features() -> Vec<String> {
    [
        "metadata.version6",
        "metadata.strategy",
        "syntax.if_else",
        "syntax.assignment",
        "syntax.var",
        "syntax.reassign",
        "syntax.expression_parser",
        "expression.history_ref",
        "expression.ternary",
        "expression.strict_bool",
        "indicator.ma",
        "indicator.rsi",
        "indicator.macd",
        "indicator.atr",
        "indicator.rolling_window",
        "indicator.cross",
        "request.security.mtf_sources",
        "order.entry_close_exit",
        "order.qty_percent",
        "order.cancel",
    ]
    .into_iter()
    .map(str::to_owned)
    .collect()
}

fn has_errors(diagnostics: &[Diagnostic]) -> bool {
    diagnostics
        .iter()
        .any(|diagnostic| diagnostic.severity == DiagnosticSeverity::Error)
}

#[cfg(test)]
mod public_helper_guard_tests {
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:84
    /// TestAnalyzeScriptReportsPublicInternalHelperDiagnostics
    ///
    /// Go reports an error diagnostic with a stable code and the Pine v6
    /// replacement when a script calls a JFTrade-internal helper or the
    /// `ta.adx` shortcut. Rust must fail the compile with the same codes
    /// instead of silently analysing the call.
    ///
    /// Parity: go:452dea11:pkg/strategy/pine/validation_semantics_boundaries_test.go:108 TestPublicHelperGuardReturnsActionableMigrationErrors
    #[test]
    fn analyze_script_reports_public_internal_helper_diagnostics() {
        for (line, code, wanted) in [
            (
                "fast = ma(EMA, 14)",
                "PINE_INTERNAL_HELPER_PUBLIC",
                "ta.sma/ta.ema",
            ),
            ("adx = ta.adx(14)", "PINE_PUBLIC_TA_SHORTCUT", "ta.dmi"),
        ] {
            let source =
                format!("//@version=6\nstrategy(\"helper diagnostics\", overlay=true)\n{line}");
            let analysis = analyze_script(&source, AnalysisOptions::default());
            assert!(!analysis.ok, "analysis must fail for {line}");
            let diagnostic = analysis
                .diagnostics
                .first()
                .unwrap_or_else(|| panic!("missing diagnostic for {line}"));
            assert_eq!(diagnostic.code, code, "code for {line}");
            assert_eq!(diagnostic.line, 3, "line for {line}");
            assert!(
                diagnostic.message.contains(wanted),
                "message {:?} must mention {wanted}",
                diagnostic.message
            );
        }
    }

    /// Parity: pkg/strategy/pine/parse_test.go:54
    /// TestCompileRejectsPublicInternalHelperCalls
    #[test]
    fn compile_rejects_public_internal_helper_calls() {
        for (line, wanted) in [
            ("fast = ma(EMA, 14)", "ta.sma/ta.ema"),
            ("band = bollinger(20, 2)", "ta.bb"),
            ("x = cross_over(fast, slow)", "ta.crossover"),
            ("x = cross_under(fast, slow)", "ta.crossunder"),
            ("notify(\"hello\")", "alert"),
            ("x = barssince(close > open)", "ta.barssince"),
            ("x = valuewhen(close > open, close, 0)", "ta.valuewhen"),
            (
                "x = security_source(\"AAPL\", \"1D\", close)",
                "request.security",
            ),
            ("x = highest(close, 20)", "ta.highest"),
            ("x = lowest(close, 20)", "ta.lowest"),
            ("x = history(close, 1)", "series[n]"),
            (
                "x = ifelse(close > open, close, open)",
                "condition ? valueWhenTrue : valueWhenFalse",
            ),
            ("adx = ta.adx(14)", "ta.dmi"),
        ] {
            let source =
                format!("//@version=6\nstrategy(\"reject helpers\", overlay=true)\n{line}");
            let compilation = compile(&source);
            assert!(!compilation.ok, "compile must fail for {line}");
            let messages = compilation
                .diagnostics
                .iter()
                .map(|diagnostic| diagnostic.message.clone())
                .collect::<Vec<_>>()
                .join(" | ");
            assert!(
                messages.contains(wanted),
                "diagnostics for {line} = {messages:?}, want {wanted}"
            );
        }
    }
}

#[cfg(test)]
mod parse_ir_tests {
    use super::lower::LoweredStatement;
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:10
    /// TestParseScriptLowersPineStrategyToIR
    ///
    /// Go lowers the script into a single on-kline-close hook whose third
    /// statement is the if/else, with a buy order of 1 share inside the then
    /// branch. Rust's lowered IR is typed instead of interface-typed, so the
    /// same contract is asserted structurally.
    #[test]
    fn parse_script_lowers_pine_strategy_to_ir() {
        let script = r#"//@version=6
strategy("EMA Crossover", overlay=true)

fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)
else
    alert("waiting")"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("lowered program");
        assert_eq!(program.source_format, "pine-v6");
        assert_eq!(program.metadata.name, "EMA Crossover");
        assert_eq!(program.hooks.len(), 1, "exactly one hook");
        assert_eq!(program.hooks[0].kind, "on_kline_close");
        let statements = &program.hooks[0].statements;
        assert_eq!(statements.len(), 3, "statements = {statements:?}");

        let LoweredStatement::If {
            condition,
            then_body,
            else_body,
            ..
        } = &statements[2]
        else {
            panic!("statement 2 is not an if: {:?}", statements[2]);
        };
        // Rust renders argument lists without spaces; compare the normalized
        // form so the Go `cross_over(fast, slow)` contract still holds.
        let normalized = condition.to_string().replace(", ", ",");
        assert_eq!(normalized, "ta.crossover(fast,slow)");
        let LoweredStatement::Action {
            call, arguments, ..
        } = then_body
            .first()
            .unwrap_or_else(|| panic!("missing then statement: {then_body:?}"))
        else {
            panic!("then statement is not an action: {then_body:?}");
        };
        assert_eq!(call, "strategy.entry");
        assert_eq!(arguments[0].to_string(), "\"Long\"");
        // Rust keeps the named argument as an equality expression; Go stores
        // `qty=1` as the order quantity. Assert the value that reached the IR.
        assert!(
            arguments
                .iter()
                .any(|argument| argument.to_string().ends_with("1)")),
            "qty=1 must be present in the lowered call: {arguments:?}"
        );
        assert!(
            !else_body.is_empty(),
            "else branch must keep the alert call"
        );
    }

    /// Parity: pkg/strategy/pine/parse_test.go:126
    /// TestCompileUsesStrategyDefaultQuantityForEntryWithoutQty
    #[test]
    fn compile_uses_strategy_default_quantity_for_entry_without_qty() {
        let script = r#"//@version=6
strategy("Default Qty", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)
strategy.entry("Long", strategy.long)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("lowered program");
        assert_eq!(program.metadata.default_qty_mode, "percent_of_equity");
        assert_eq!(program.metadata.default_qty_value, "10");
        assert_eq!(program.metadata.pyramiding, 2);
    }
}

#[cfg(test)]
mod parse_metadata_tests {
    use super::lower::LoweredStatement;
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:145
    /// TestCompileParsesBacktestStrategyMetadata
    #[test]
    fn compile_parses_backtest_strategy_metadata() {
        let script = r#"//@version=6
strategy("Costs", initial_capital=250000, commission_type=strategy.commission.percent, commission_value=0.15, slippage=3, process_orders_on_close=true)
strategy.entry("Long", strategy.long, qty=1)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let metadata = compilation.program.expect("program").metadata;
        assert_eq!(metadata.initial_capital.as_deref(), Some("250000"));
        assert_eq!(metadata.commission_type.as_deref(), Some("percent"));
        assert_eq!(metadata.commission_value.as_deref(), Some("0.15"));
        assert_eq!(metadata.slippage, Some(3));
        assert!(metadata.process_on_close);
    }

    /// Parity: pkg/strategy/pine/parse_test.go:186
    /// TestCompileExplicitEntryQtyOverridesStrategyDefaultQuantity
    #[test]
    fn compile_explicit_entry_qty_overrides_strategy_default_quantity() {
        let script = r#"//@version=6
strategy("Explicit Qty", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)
strategy.entry("Long", strategy.long, qty=5)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("program");
        assert_eq!(
            program.metadata.default_qty_mode, "percent_of_equity",
            "strategy default stays percent_of_equity"
        );
        let LoweredStatement::Action {
            call, arguments, ..
        } = &program.hooks[0].statements[0]
        else {
            panic!("first statement is not an action");
        };
        assert_eq!(call, "strategy.entry");
        assert!(
            arguments
                .iter()
                .any(|argument| argument.to_string().ends_with("5)")),
            "explicit qty=5 must survive lowering: {arguments:?}"
        );
    }
}

#[cfg(test)]
mod framework_language_feature_tests {
    use super::lower::LoweredStatement;
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:560
    /// TestCompileSupportsFrameworkLanguageFeatures
    ///
    /// Go asserts `var`/reassignment modes plus the normalized
    /// `ifelse(history(close, 1) == na, 0, nz(history(close, 1), close))`
    /// expression. Rust keeps typed assignment modes and the parsed ternary;
    /// `ifelse(...)` normalizes to the ternary form during lowering.
    #[test]
    fn compile_supports_framework_language_features() {
        let script = r#"//@version=6
strategy("Framework", overlay=true)
var count = 0
count := count + 1
signal = close[1] == na ? 0 : nz(close[1], close)
if close > close[1]
    log.info("up")"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("lowered program");
        let statements = &program.hooks[0].statements;
        let LoweredStatement::Let { mode, .. } = &statements[0] else {
            panic!("statement 0 is not a let: {:?}", statements[0]);
        };
        assert_eq!(mode, "var");
        let LoweredStatement::Let { mode, .. } = &statements[1] else {
            panic!("statement 1 is not a let: {:?}", statements[1]);
        };
        assert_eq!(mode, "reassign");
        let LoweredStatement::Let { expression, .. } = &statements[2] else {
            panic!("statement 2 is not a let: {:?}", statements[2]);
        };
        let rendered = expression.to_string().replace(", ", ",");
        for expected in ["nz(close[1]", "close[1]", "na"] {
            assert!(
                rendered.contains(&expected.replace(", ", ",")),
                "signal {rendered:?} must contain {expected}"
            );
        }
        let LoweredStatement::If { condition, .. } = &statements[3] else {
            panic!("statement 3 is not an if: {:?}", statements[3]);
        };
        // Rust renders comparison operators by their enum name; normalize to
        // the Go symbol form before comparing.
        let condition = condition
            .to_string()
            .replace("Greater", ">")
            .replace("Less", "<")
            .replace("Equal", "==");
        assert_eq!(condition, "(close > close[1])");
    }
}

#[cfg(test)]
mod trend_stateful_ta_tests {
    use super::*;

    /// Parity: go:452dea11:pkg/strategy/pine/parse_semantic_test.go:287 TestAnalyzeScriptSupportsTrendAndStatefulTAFunctions
    #[test]
    fn analyze_script_supports_trend_and_stateful_ta_functions() {
        let analysis = analyze_script(
            r#"//@version=6
strategy("Supertrend", overlay=true)
[line, direction] = ta.supertrend(3, 10)
[plusDI, minusDI, adx] = ta.dmi(14, 14)
v = ta.vwap(hlc3)
m = ta.mfi(hlc3, 14)
if ta.barssince(close > open) > 2 and ta.valuewhen(ta.cross(close, open), close, 0) > v and adx > 20
    strategy.entry("Long", strategy.long)"#,
            AnalysisOptions::default(),
        );
        assert!(
            analysis.ok,
            "trend/stateful TA analysis must succeed: {:?}",
            analysis.diagnostics
        );
    }
}

#[cfg(test)]
mod udf_and_loop_boundary_tests {
    use super::*;

    fn diagnostics(script: &str) -> Vec<Diagnostic> {
        compile(script).diagnostics
    }

    /// Parity: pkg/strategy/pine/parse_test.go:790
    /// TestCompileSupportsExpressionUDFAndStaticForUnroll
    ///
    /// Go unrolls `for i = 0 to 3` into four `sum + history(close, i)`
    /// statements and inlines both UDFs. Rust keeps the loop as a typed `For`
    /// with the UDF call intact and defers unrolling/inlining to the PineTS
    /// worker, so the shared contract asserted here is that the script
    /// compiles with both UDF declarations preserved and the loop body
    /// visible in the lowered IR.
    #[test]
    fn compile_accepts_expression_udf_and_static_for_unroll() {
        let script = r#"//@version=6
strategy("UDF For", overlay=true)
isBull(src) => src > src[1]
smooth(src, len) => ta.ema(src, len)
len = input.int(3, "Length")
fast = smooth(close, len)
sum = 0
for i = 0 to 3
    sum := sum + close[i]
if isBull(close) and fast > fast[1] and sum > 0
    strategy.entry("Long", strategy.long, qty=1)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("lowered program");
        assert_eq!(program.functions.len(), 2, "UDF declarations preserved");
        let names = program
            .functions
            .iter()
            .map(|function| function.name.clone())
            .collect::<Vec<_>>();
        assert_eq!(names, vec!["isBull", "smooth"]);
        use super::lower::LoweredStatement;
        let statements = &program.hooks[0].statements;
        assert_eq!(
            statements.len(),
            5,
            "Rust retains the loop instead of eight expanded statements"
        );
        let LoweredStatement::Let {
            name, expression, ..
        } = &statements[1]
        else {
            panic!("{:?}", statements[1]);
        };
        assert_eq!(name, "fast");
        assert_eq!(expression.to_string(), "smooth(close,len)");
        let LoweredStatement::For {
            variable,
            start,
            end,
            step,
            body,
            ..
        } = &statements[3]
        else {
            panic!("{:?}", statements[3]);
        };
        assert_eq!(variable, "i");
        assert_eq!(start.to_string(), "0");
        assert_eq!(end.to_string(), "3");
        assert!(step.is_none());
        assert_eq!(body.len(), 1);
        let LoweredStatement::Let {
            name,
            expression,
            mode,
            ..
        } = &body[0]
        else {
            panic!("{:?}", body[0]);
        };
        assert_eq!(name, "sum");
        assert_eq!(mode, "reassign");
        assert_eq!(expression.to_string(), "(sum Add close[i])");
        let LoweredStatement::If {
            condition,
            then_body,
            else_body,
            ..
        } = &statements[4]
        else {
            panic!("{:?}", statements[4]);
        };
        assert_eq!(
            condition.to_string(),
            "((isBull(close) And (fast Greater fast[1])) And (sum Greater 0))"
        );
        assert_eq!(then_body.len(), 1);
        assert!(else_body.is_empty());
    }

    /// Parity: pkg/strategy/pine/parse_test.go:838
    /// TestValidateScriptReportsUnsupportedUDFAndStaticForCases
    ///
    /// This narrow regression retains the two static bounds cases. The six
    /// original rejection cases are exercised in pine_call_scope_contracts.
    #[test]
    fn validate_script_reports_supported_udf_and_static_for_boundaries() {
        for (script, wanted) in [
            (
                r#"//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3 by 0
    log.info("nope")"#,
                "for loop step cannot be 0",
            ),
            (
                r#"//@version=6
strategy("Loop", overlay=true)
for i = 0 to 100
    log.info("nope")"#,
                "for loop expands to more than 100 iterations",
            ),
        ] {
            let found = diagnostics(script);
            let messages = found
                .iter()
                .map(|diagnostic| diagnostic.message.clone())
                .collect::<Vec<_>>()
                .join(" | ");
            assert!(
                messages.contains(wanted),
                "diagnostics = {messages:?}, want {wanted}"
            );
        }
    }
}

#[cfg(test)]
mod risk_declaration_metadata_tests {
    use super::lower::StrategyMetadata;
    use super::*;

    fn metadata(script: &str) -> StrategyMetadata {
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        compilation.program.expect("lowered program").metadata
    }

    /// Parity: pkg/strategy/pine/parse_test.go:718
    /// TestCompileSupportsAllowEntryInRiskDeclaration
    #[test]
    fn compile_supports_allow_entry_in_risk_declaration() {
        let metadata = metadata(
            r#"//@version=6
strategy("Allow entry", overlay=true)
strategy.risk.allow_entry_in(strategy.direction.long)
if close > open
    strategy.entry("Long", strategy.long, qty=1)"#,
        );
        assert_eq!(metadata.allowed_entry_direction.as_deref(), Some("long"));
    }

    /// Parity: pkg/strategy/pine/parse_test.go:732
    /// TestCompileSupportsRuntimeRiskDeclarations
    #[test]
    fn compile_supports_runtime_risk_declarations() {
        let metadata = metadata(
            r#"//@version=6
strategy("Risk declarations", overlay=true)
strategy.risk.max_drawdown(10, strategy.percent_of_equity, alert_message="dd")
strategy.risk.max_intraday_loss(5, strategy.cash, "day")
strategy.risk.max_intraday_filled_orders(3, alert_message="fills")
strategy.risk.max_position_size(12)
strategy.risk.max_cons_loss_days(2, "days")
if close > open
    strategy.entry("Long", strategy.long, qty=1)"#,
        );
        assert_eq!(metadata.max_drawdown_value.as_deref(), Some("10"));
        assert_eq!(
            metadata.max_drawdown_type.as_deref(),
            Some("percent_of_equity")
        );
        assert_eq!(metadata.max_drawdown_alert.as_deref(), Some("dd"));
        assert_eq!(metadata.max_intraday_loss_value.as_deref(), Some("5"));
        assert_eq!(metadata.max_intraday_loss_type.as_deref(), Some("cash"));
        assert_eq!(metadata.max_intraday_loss_alert.as_deref(), Some("day"));
        assert_eq!(metadata.max_intraday_filled_orders, Some(3));
        assert_eq!(
            metadata.max_intraday_filled_orders_alert.as_deref(),
            Some("fills")
        );
        assert_eq!(metadata.max_position_size.as_deref(), Some("12"));
        assert_eq!(metadata.max_cons_loss_days, Some(2));
        assert_eq!(metadata.max_cons_loss_days_alert.as_deref(), Some("days"));
    }
}

#[cfg(test)]
mod advanced_indicator_requirement_tests {
    use super::*;

    fn requirement_keys(script: &str) -> Vec<String> {
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        compilation
            .requirements
            .indicators
            .iter()
            .map(|requirement| requirement.key.clone())
            .collect()
    }

    /// Parity: pkg/strategy/pine/parse_test.go:587
    /// TestCompileSupportsV12AdvancedIndicators
    #[test]
    fn compile_supports_v12_advanced_indicators() {
        let keys = requirement_keys(
            r#"//@version=6
strategy("Advanced indicators", overlay=true)
lr = ta.linreg(close, 5, 0)
obvValue = ta.obv
pivotHigh = ta.pivothigh(high, 2, 2)
pivotLow = ta.pivotlow(low, 2, 2)
[basis, upper, lower] = ta.kc(close, 5, 1.5)
width = ta.kcw(close, 5, 1.5)
almaValue = ta.alma(close, 5, 0.85, 6)
if close > lr and obvValue > 0 and upper > lower and width > 0 and almaValue > 0
    strategy.entry("Long", strategy.long, qty=1)"#,
        );
        for expected in [
            "linreg:close:5:0",
            "obv:close",
            "pivothigh:high:2:2",
            "pivotlow:low:2:2",
            "kc:close:5:1.5:true",
            "kcw:close:5:1.5:true",
            "alma:close:5:0.85:6",
        ] {
            assert!(
                keys.iter().any(|key| key == expected),
                "requirements {keys:?} missing {expected}"
            );
        }
    }

    /// Parity: pkg/strategy/pine/parse_test.go:621
    /// TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity
    #[test]
    fn compile_supports_v12_advanced_indicators_in_static_intraday_security() {
        let keys = requirement_keys(
            r#"//@version=6
strategy("Advanced MTF", overlay=true)
lr = request.security(syminfo.tickerid, "15", ta.linreg(close, 5, 0))
obvValue = request.security(syminfo.tickerid, "15", ta.obv)
pivotHigh = request.security(syminfo.tickerid, "15", ta.pivothigh(2, 2))
[basis, upper, lower] = request.security(syminfo.tickerid, "15", ta.kc(close, 5, 1.5))
almaValue = request.security(syminfo.tickerid, "15", ta.alma(close, 5, 0.85, 6))
if close > lr and obvValue > 0 and pivotHigh > 0 and upper > lower and almaValue > 0
    strategy.entry("Long", strategy.long, qty=1)"#,
        );
        for expected in [
            "linreg:close:5:0:15m",
            "obv:close:15m",
            "pivothigh:high:2:2:15m",
            "kc:close:5:1.5:true:15m",
            "alma:close:5:0.85:6:15m",
        ] {
            assert!(
                keys.iter().any(|key| key == expected),
                "requirements {keys:?} missing {expected}"
            );
        }
    }

    /// Parity: pkg/strategy/pine/parse_test.go:651
    /// TestCompileSupportsV13MigrationIndicators
    #[test]
    fn compile_supports_v13_migration_indicators() {
        let keys = requirement_keys(
            r#"//@version=6
strategy("v1.3 indicators", overlay=true)
cmoValue = ta.cmo(close, 5)
tsiValue = ta.tsi(close, 2, 3)
corrValue = ta.correlation(close, high, 5)
devValue = ta.dev(close, 5)
medianValue = ta.median(close, 5)
pLinear = ta.percentile_linear_interpolation(close, 5, 50)
pNearest = ta.percentile_nearest_rank(close, 5, 80)
rankValue = ta.percentrank(close, 5)
swmaValue = ta.swma(close)
rounded = math.round_to_mintick(math.avg(close, open))
if cmoValue > 0 and tsiValue > 0 and corrValue > 0 and devValue > 0 and medianValue > 0 and pLinear > 0 and pNearest > 0 and rankValue > 0 and swmaValue > 0 and rounded > 0
    strategy.entry("Long", strategy.long, qty=1)"#,
        );
        for expected in [
            "cmo:close:5",
            "tsi:close:2:3",
            "correlation:close:high:5",
            "dev:close:5",
            "median:close:5",
            "percentile_linear_interpolation:close:5:50",
            "percentile_nearest_rank:close:5:80",
            "percentrank:close:5",
            "swma:close",
        ] {
            assert!(
                keys.iter().any(|key| key == expected),
                "requirements {keys:?} missing {expected}"
            );
        }
    }

    /// Parity: pkg/strategy/pine/parse_test.go:690
    /// TestCompileSupportsV13IndicatorsInStaticIntradaySecurity
    #[test]
    fn compile_supports_v13_indicators_in_static_intraday_security() {
        let keys = requirement_keys(
            r#"//@version=6
strategy("v1.3 MTF indicators", overlay=true)
cmoValue = request.security(syminfo.tickerid, "15", ta.cmo(close, 5))
corrValue = request.security(syminfo.tickerid, "15", ta.correlation(close, high, 5))
pctValue = request.security(syminfo.tickerid, "15", ta.percentile_nearest_rank(close, 5, 80))
swmaValue = request.security(syminfo.tickerid, "15", ta.swma(close))
if cmoValue > 0 and corrValue > 0 and pctValue > 0 and swmaValue > 0
    strategy.entry("Long", strategy.long, qty=1)"#,
        );
        for expected in [
            "cmo:close:5:15m",
            "correlation:close:high:5:15m",
            "percentile_nearest_rank:close:5:80:15m",
            "swma:close:15m",
        ] {
            assert!(
                keys.iter().any(|key| key == expected),
                "requirements {keys:?} missing {expected}"
            );
        }
    }
}

#[cfg(test)]
mod order_subset_compile_tests {
    use super::lower::LoweredStatement;
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:167
    /// TestCompilePreservesOrderNotificationMetadataAndImmediateClose
    ///
    /// Go projects the notification metadata onto typed
    /// `OrderStmt.Comment/AlertMessage/DisableAlert/Immediate` fields; Rust
    /// keeps the same values as named arguments of the lowered action, so the
    /// metadata must survive lowering verbatim.
    #[test]
    fn compile_preserves_order_notification_metadata_and_immediate_close() {
        let script = r#"//@version=6
strategy("Order Metadata")
strategy.entry("Long", strategy.long, qty=1, comment="entry", alert_message="opened", disable_alert=false)
strategy.close("Long", immediately=true, comment="close", alert_message="closed", disable_alert=true)"#;
        let calls = action_calls(script);
        assert_eq!(calls.len(), 2, "calls = {calls:?}");
        let (entry_call, entry_arguments) = &calls[0];
        assert_eq!(entry_call, "strategy.entry");
        for wanted in [
            "(comment Equal \"entry\")",
            "(alert_message Equal \"opened\")",
            "(disable_alert Equal false)",
        ] {
            assert!(
                entry_arguments.iter().any(|argument| argument == wanted),
                "entry must keep `{wanted}`: {entry_arguments:?}"
            );
        }
        let (close_call, close_arguments) = &calls[1];
        assert_eq!(close_call, "strategy.close");
        for wanted in [
            "(immediately Equal true)",
            "(comment Equal \"close\")",
            "(alert_message Equal \"closed\")",
            "(disable_alert Equal true)",
        ] {
            assert!(
                close_arguments.iter().any(|argument| argument == wanted),
                "close must keep `{wanted}`: {close_arguments:?}"
            );
        }
    }

    fn action_calls(script: &str) -> Vec<(String, Vec<String>)> {
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("lowered program");
        program.hooks[0]
            .statements
            .iter()
            .map(|statement| {
                let LoweredStatement::Action {
                    call, arguments, ..
                } = statement
                else {
                    panic!("statement is not an action: {statement:?}");
                };
                (
                    call.clone(),
                    arguments
                        .iter()
                        .map(ToString::to_string)
                        .collect::<Vec<_>>(),
                )
            })
            .collect()
    }

    /// Parity: pkg/strategy/pine/parse_test.go:286, :317, :348, :367, :390,
    /// :432, :448
    ///
    /// Go lowers these order calls into typed `OrderStmt`/`ExitStmt`/
    /// `CancelStmt` projections. Rust keeps the verified call plus its
    /// arguments as `LoweredStatement::Action` and defers the order semantics
    /// to the PineTS worker and the deterministic backtest matcher, so the
    /// shared contract asserted here is that every supported script compiles
    /// and keeps its `strategy.*` calls with named arguments intact.
    #[test]
    fn compile_accepts_strategy_order_subset_scripts() {
        for script in [
            r#"//@version=6
strategy("Exit", overlay=true)
strategy.exit("Long stop", "Long", stop=close * (1 - 2 / 100), qty_percent=50)
strategy.exit("Short profit", "Short", limit=close * (1 - 3 / 100), qty=5)
strategy.exit("Bracket", from_entry="Long", stop=close - 2, limit=close + 3)
strategy.exit("Long trail", "Long", trail_points=close * 4 / 100, trail_offset=close * 4 / 100)"#,
            r#"//@version=6
strategy("When", overlay=true)
strategy.entry("Long", strategy.long, qty=1, when=close > open)
strategy.order("Net short", strategy.short, qty=2, when=close < open)
strategy.close("Long", when=ta.crossunder(close, open))
strategy.exit("Exit", "Long", stop=close - 2, when=high > low)"#,
            r#"//@version=6
strategy("Exit points", overlay=true)
strategy.exit("Points", "Long", profit=50, loss=25, qty_percent=50)"#,
            r#"//@version=6
strategy("Pending", overlay=true)
strategy.entry("Breakout", strategy.long, stop=ta.highest(high, 20), qty=1)
strategy.order("Net short", strategy.short, stop=low - 1, qty=5)
strategy.close("Long", stop=99, limit=101, qty_percent=50)
strategy.entry("StopLimit", strategy.long, stop=101, limit=99, qty=2)
strategy.cancel("Breakout")
strategy.cancel_all()"#,
            r#"//@version=6
strategy("Close all positional", overlay=true)
strategy.close_all(true, "flat", "done", false)"#,
            r#"//@version=6
strategy("Close positional", overlay=true)
strategy.close("Long", 2)"#,
        ] {
            let calls = action_calls(script);
            assert!(!calls.is_empty(), "no actions lowered for {script}");
            assert!(
                calls.iter().all(|(call, _)| call.starts_with("strategy.")),
                "unexpected calls: {calls:?}"
            );
        }
    }

    // Parity: go:452dea11:pkg/strategy/pine/parse_test.go:390 TestCompileSupportsPendingStopAndCancelOrders
    #[test]
    fn pending_order_calls_preserve_original_stop_limit_and_cancel_arguments() {
        let calls = action_calls(
            r#"//@version=6
strategy("Pending", overlay=true)
strategy.entry("Breakout", strategy.long, stop=ta.highest(high, 20), qty=1)
strategy.order("Net short", strategy.short, stop=low - 1, qty=5)
strategy.close("Long", stop=99, limit=101, qty_percent=50)
strategy.entry("StopLimit", strategy.long, stop=101, limit=99, qty=2)
strategy.cancel("Breakout")
strategy.cancel_all()"#,
        );
        let expected: [(&str, &[&str]); 6] = [
            (
                "strategy.entry",
                &[
                    "\"Breakout\"",
                    "strategy.long",
                    "(stop Equal ta.highest(high,20))",
                    "(qty Equal 1)",
                ],
            ),
            (
                "strategy.order",
                &[
                    "\"Net short\"",
                    "strategy.short",
                    "(stop Equal (low Subtract 1))",
                    "(qty Equal 5)",
                ],
            ),
            (
                "strategy.close",
                &[
                    "\"Long\"",
                    "(stop Equal 99)",
                    "(limit Equal 101)",
                    "(qty_percent Equal 50)",
                ],
            ),
            (
                "strategy.entry",
                &[
                    "\"StopLimit\"",
                    "strategy.long",
                    "(stop Equal 101)",
                    "(limit Equal 99)",
                    "(qty Equal 2)",
                ],
            ),
            ("strategy.cancel", &["\"Breakout\""]),
            ("strategy.cancel_all", &[]),
        ];
        assert_eq!(calls.len(), expected.len());
        for ((call, arguments), (wanted_call, wanted_arguments)) in calls.iter().zip(expected) {
            assert_eq!(call, wanted_call);
            assert_eq!(arguments, wanted_arguments);
        }
    }

    /// Parity: pkg/strategy/pine/parse_test.go:367
    /// TestCompileCapturesStrategyExitSpecificMetadata
    ///
    /// Named exit metadata must survive lowering so the worker can project it.
    #[test]
    fn compile_keeps_strategy_exit_named_metadata() {
        let calls = action_calls(
            r#"//@version=6
strategy("Exit metadata", overlay=true)
strategy.exit("Bracket", "Long", stop=98, limit=105, comment="generic", comment_profit="tp", comment_loss="sl", alert_message="base", alert_profit="ap", alert_loss="al")
strategy.exit("Trail", "Long", trail_points=10, trail_offset=5, comment="generic trail", comment_trailing="trail comment", alert_message="trail base", alert_trailing="trail alert")"#,
        );
        assert_eq!(calls.len(), 2, "calls = {calls:?}");
        let normalize = |arguments: &[String]| arguments.join("|").replace(" Equal ", "=");
        let joined = normalize(&calls[0].1);
        for expected in [
            r#"comment="generic""#,
            r#"comment_profit="tp""#,
            r#"comment_loss="sl""#,
            r#"alert_message="base""#,
            r#"alert_profit="ap""#,
            r#"alert_loss="al""#,
        ] {
            assert!(
                joined.contains(expected),
                "bracket exit missing {expected}: {joined}"
            );
        }
        let joined = normalize(&calls[1].1);
        for expected in [
            r#"comment_trailing="trail comment""#,
            r#"alert_trailing="trail alert""#,
        ] {
            assert!(
                joined.contains(expected),
                "trailing exit missing {expected}: {joined}"
            );
        }
    }
}

#[cfg(test)]
mod advanced_order_diagnostic_tests {
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:464
    /// TestValidateScriptReportsUnsupportedAdvancedOrders
    #[test]
    fn validate_script_reports_unsupported_advanced_orders() {
        let source = r#"//@version=6
strategy("Trail Stop", overlay=true)
strategy.exit("Exit", "Long", stop=close - 2, trail_points=close * 4 / 100, trail_offset=close * 4 / 100)"#;
        let compilation = compile(source);
        assert!(!compilation.ok, "trail plus stop must be rejected");
        let messages = compilation
            .diagnostics
            .iter()
            .map(|diagnostic| diagnostic.message.clone())
            .collect::<Vec<_>>()
            .join(" | ");
        assert!(
            messages.contains("trail with stop/limit"),
            "diagnostics = {messages:?}"
        );
        assert!(
            compilation
                .diagnostics
                .iter()
                .any(|diagnostic| diagnostic.code == "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED"),
            "missing trail-bracket code"
        );
    }

    /// Parity: pkg/strategy/pine/parse_test.go:488
    /// TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics
    ///
    /// Go returns six broker-boundary diagnostics with stable codes on the
    /// failing line; Rust mirrors the executable subset of that matrix.
    #[test]
    fn analyze_script_reports_v40_broker_boundary_diagnostics() {
        for (body, code) in [
            (
                r#"strategy.entry("Long", strategy.long, qty=1, oca_name="group")"#,
                "PINE_ORDER_OCA_UNSUPPORTED",
            ),
            (
                r#"strategy.exit("Exit", "Long", stop=98, oca_name="group")"#,
                "PINE_ORDER_OCA_UNSUPPORTED",
            ),
            (
                r#"strategy.close("Long", qty=1, qty_percent=50)"#,
                "PINE_ORDER_QTY_CONFLICT",
            ),
            (
                r#"strategy.exit("Exit", "Long", stop=close - 2, trail_points=close * 4 / 100, trail_offset=close * 4 / 100)"#,
                "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED",
            ),
            (
                r#"strategy.exit("Exit", "Long")"#,
                "PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED",
            ),
            (
                r#"strategy.entry("Long", strategy.long, risk=1)"#,
                "PINE_COMPILE_ERROR",
            ),
            (r#"strategy.close_all(foo=1)"#, "PINE_COMPILE_ERROR"),
        ] {
            let source =
                format!("//@version=6\nstrategy(\"Broker boundary\", overlay=true)\n{body}");
            let analysis = analyze_script(&source, AnalysisOptions::default());
            assert!(!analysis.ok, "analysis must fail for {body}");
            assert!(
                analysis
                    .diagnostics
                    .iter()
                    .any(|diagnostic| diagnostic.code == code && diagnostic.line == 3),
                "diagnostics for {body} = {:?}, want {code} on line 3",
                analysis.diagnostics
            );
        }
    }
}

#[cfg(test)]
mod history_reference_boundary_tests {
    use super::lower::LoweredStatement;
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:253
    /// TestValidateScriptReportsUnsupportedHistoryReferences
    ///
    /// Go rejects a call-result history reference with "assign the function
    /// result first" and a lookback above 500 with "exceeds JFTrade maximum
    /// 500"; Rust mirrors both through the semantic diagnostics.
    #[test]
    fn validate_script_reports_unsupported_history_references() {
        for (body, wanted) in [
            (
                "if ta.sma(close, 20)[2] > close\n    strategy.entry(\"Long\", strategy.long, qty=1)",
                "assign the function result first",
            ),
            (
                "if close[501] > close\n    strategy.entry(\"Long\", strategy.long, qty=1)",
                "exceeds JFTrade maximum 500",
            ),
        ] {
            let source = format!("//@version=6\nstrategy(\"History\", overlay=true)\n{body}");
            let compilation = compile(&source);
            assert!(!compilation.ok, "compile must fail for {body}");
            let messages = compilation
                .diagnostics
                .iter()
                .map(|diagnostic| diagnostic.message.clone())
                .collect::<Vec<_>>()
                .join(" | ");
            assert!(
                messages.contains(wanted),
                "diagnostics for {body} = {messages:?}, want {wanted}"
            );
            assert!(
                compilation
                    .diagnostics
                    .iter()
                    .any(|diagnostic| diagnostic.code == "PINE_HISTORY_REF_UNSUPPORTED"),
                "missing PINE_HISTORY_REF_UNSUPPORTED for {body}"
            );
        }
    }

    /// Parity: pkg/strategy/pine/parse_test.go:1044
    /// TestHistoryReferencesIgnoreStringLiterals
    ///
    /// A `close[1]` sequence inside a string literal is data, not a history
    /// reference: Go lowers both assignments unchanged and so must Rust.
    #[test]
    fn history_references_ignore_string_literals() {
        let script = r#"//@version=6
strategy("Strings", overlay=true)
label = "close[1]"
deeper = "close[2]""#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("lowered program");
        let statements = &program.hooks[0].statements;
        assert_eq!(statements.len(), 2, "statements = {statements:?}");
        for (index, expected) in [(0usize, "\"close[1]\""), (1usize, "\"close[2]\"")] {
            let LoweredStatement::Let { expression, .. } = &statements[index] else {
                panic!("statement {index} is not a let: {:?}", statements[index]);
            };
            assert_eq!(
                expression.to_string(),
                expected,
                "string literal {index} must be untouched"
            );
        }
    }
}

#[cfg(test)]
mod request_security_tests {
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:113
    /// TestCompileAcceptsNativePineIndicatorPublicEntry
    ///
    /// Go's public entry accepts a native indicator script that mixes
    /// `ta.ema`/`ta.bb` with a `request.security(syminfo.tickerid, "D", ...)`
    /// pure expression. Rust must not emit a request.security diagnostic for
    /// that supported subset.
    #[test]
    fn compile_accepts_native_pine_indicator_public_entry() {
        let script = r#"//@version=6
strategy("native indicators", overlay=true)
fast = ta.ema(close, 14)
band = ta.bb(close, 20, 2)
daily = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
if close > fast and close < band.upper and daily > 0
    strategy.entry("Long", strategy.long, qty=1)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
    }

    /// Parity: pkg/strategy/pine/compiler_and_security_diagnostics_test.go:44
    /// TestRequestSecurityPurityCoversOptionalAndBuiltinRecovery.
    #[test]
    fn compile_accepts_pure_min_request_security_expression() {
        let script = r#"//@version=6
strategy("pure helpers", overlay=true)
daily = request.security(syminfo.tickerid, "D", min(close, open))"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
    }

    /// Parity: pkg/strategy/pine/runtime_and_parser_boundaries_test.go:46
    /// TestDynamicForBoundsUseRuntimeFallback.
    #[test]
    fn compile_accepts_dynamic_integer_for_step() {
        let script = r#"//@version=6
strategy("dynamic loop", overlay=true)
for i = close to 5 by int(close)
    strategy.entry("Long", strategy.long, qty=1)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
    }

    /// Parity: pkg/strategy/pine/parse_test.go:202
    /// TestValidateScriptRejectsUnsupportedPineRuntimeFeature
    ///
    /// Go rejects four unsupported request.security forms with stable codes
    /// and messages; Rust mirrors each rejection through the same pipeline.
    #[test]
    fn validate_script_rejects_unsupported_pine_runtime_feature() {
        for (body, code, wanted) in [
            (
                r#"x = request.security("NASDAQ:AAPL", "D", close)"#,
                "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL",
                "request.security",
            ),
            (
                r#"x = request.security(syminfo.tickerid, "D", alert("no side effects"))"#,
                "PINE_REQUEST_SECURITY_SIDE_EFFECT",
                "request.security",
            ),
            (
                r#"x = request.security(syminfo.tickerid, "D", close, lookahead=barmerge.lookahead_on)"#,
                "PINE_REQUEST_SECURITY_LOOKAHEAD",
                "lookahead_on",
            ),
            (
                r#"x = request.security(syminfo.tickerid, "D", close, gaps=barmerge.gaps_on)"#,
                "PINE_REQUEST_SECURITY_GAPS",
                "gaps_on",
            ),
            (
                r#"x = request.security(syminfo.tickerid, "D", ta.sum(close, 5))"#,
                "PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED",
                "ta.sum",
            ),
        ] {
            let source = format!("//@version=6\nstrategy(\"MTF\", overlay=true)\n{body}");
            let compilation = compile(&source);
            assert!(!compilation.ok, "compile must fail for {body}");
            let diagnostic = compilation
                .diagnostics
                .first()
                .unwrap_or_else(|| panic!("missing diagnostic for {body}"));
            assert_eq!(diagnostic.code, code, "code for {body}");
            assert_eq!(diagnostic.line, 3, "line for {body}");
            assert!(
                diagnostic.message.contains(wanted),
                "message {:?} for {body} must mention {wanted}",
                diagnostic.message
            );
        }
    }
}

#[cfg(test)]
mod parse_history_reference_tests {
    use super::lower::LoweredStatement;
    use super::*;

    /// Parity: pkg/strategy/pine/parse_test.go:232
    /// TestCompileSupportsMultiBarHistoryReferences
    ///
    /// Go rewrites `close[2]` into `history(close, 2)` inside the lowered
    /// condition. Rust keeps the same information as a typed `Index`
    /// expression, so the contract is asserted structurally: every multi-bar
    /// reference survives lowering with its lookback and receiver intact.
    #[test]
    fn compile_supports_multi_bar_history_references() {
        let script = r#"//@version=6
strategy("History", overlay=true)
[basis, upper, lower] = ta.bb(close, 20, 2)
emaFast = ta.ema(close, 3)
if close > close[2] and hlc3 > hlc3[3] and emaFast > emaFast[5] and close > upper[2]
    strategy.entry("Long", strategy.long, qty=1)"#;
        let compilation = compile(script);
        assert!(
            compilation.ok,
            "diagnostics = {:?}",
            compilation.diagnostics
        );
        let program = compilation.program.expect("program");
        let LoweredStatement::If { condition, .. } = &program.hooks[0].statements[2] else {
            panic!("statement 2 is not an if");
        };
        let rendered = condition.to_string();
        for expected in ["close[2]", "hlc3[3]", "emaFast[5]", "upper[2]"] {
            assert!(
                rendered.contains(expected),
                "condition {rendered:?} must keep {expected}"
            );
        }
    }
}
