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
