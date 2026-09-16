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
pub fn compile(source: &str) -> Compilation {
    let normalized_script = source.trim().to_owned();
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
