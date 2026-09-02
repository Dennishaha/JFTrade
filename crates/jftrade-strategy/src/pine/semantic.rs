use std::collections::{BTreeMap, BTreeSet};

use serde::Serialize;

use super::parser::{BinaryOp, Expr, ExprKind, Program, SourceRange, Statement, UnaryOp};

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum DiagnosticSeverity {
    Error,
    Warning,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Diagnostic {
    pub severity: DiagnosticSeverity,
    pub code: String,
    pub message: String,
    pub line: usize,
    pub column: usize,
    pub end_line: usize,
    pub end_column: usize,
}

impl Diagnostic {
    pub fn error(code: impl Into<String>, message: impl Into<String>, line: usize) -> Self {
        Self::new(DiagnosticSeverity::Error, code, message, line)
    }
    pub fn warning(code: impl Into<String>, message: impl Into<String>, line: usize) -> Self {
        Self::new(DiagnosticSeverity::Warning, code, message, line)
    }
    fn new(
        severity: DiagnosticSeverity,
        code: impl Into<String>,
        message: impl Into<String>,
        line: usize,
    ) -> Self {
        Self {
            severity,
            code: code.into(),
            message: message.into(),
            line,
            column: 1,
            end_line: line,
            end_column: 1,
        }
    }
}

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SemanticSummary {
    pub diagnostics: Vec<Diagnostic>,
    pub declarations: Vec<SemanticDeclaration>,
    pub visuals: Vec<VisualMetadata>,
    pub symbols: BTreeMap<String, String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SemanticDeclaration {
    pub kind: String,
    pub name: String,
    pub line: usize,
    pub executable: bool,
    pub unsupported_reason: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct VisualMetadata {
    pub line: usize,
    pub kind: String,
    pub call: String,
    pub target: Option<String>,
    pub arguments: Vec<String>,
    pub text: String,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ValueType {
    Bool,
    Number,
    String,
    Null,
    Unknown,
}

impl ValueType {
    fn as_str(self) -> &'static str {
        match self {
            Self::Bool => "bool",
            Self::Number => "number",
            Self::String => "string",
            Self::Null => "na",
            Self::Unknown => "unknown",
        }
    }
}

pub fn analyze(program: &Program) -> SemanticSummary {
    let mut summary = SemanticSummary::default();
    if program.version != 6 {
        summary.diagnostics.push(Diagnostic::error(
            "PINE_VERSION_UNSUPPORTED",
            format!(
                "Pine version {} is not supported; use //@version=6",
                program.version
            ),
            1,
        ));
    }
    if program.strategy.is_none() {
        summary.diagnostics.push(Diagnostic::error(
            "PINE_STRATEGY_REQUIRED",
            "a strategy(...) declaration is required",
            1,
        ));
    }
    let mut context = SemanticContext {
        summary: &mut summary,
        symbols: BTreeMap::new(),
        functions: BTreeSet::new(),
    };
    for statement in &program.statements {
        context.visit_statement(statement);
    }
    summary.symbols = context
        .symbols
        .into_iter()
        .map(|(name, ty)| (name, ty.as_str().to_owned()))
        .collect();
    summary
}

struct SemanticContext<'a> {
    summary: &'a mut SemanticSummary,
    symbols: BTreeMap<String, ValueType>,
    functions: BTreeSet<String>,
}

impl SemanticContext<'_> {
    fn visit_statement(&mut self, statement: &Statement) {
        match statement {
            Statement::Assignment {
                name, expression, ..
            } => {
                let ty = self.visit_expr(expression);
                self.symbols.insert(name.clone(), ty);
                self.summary.declarations.push(SemanticDeclaration {
                    kind: "variable".to_owned(),
                    name: name.clone(),
                    line: expression.range.start_line,
                    executable: true,
                    unsupported_reason: None,
                });
            }
            Statement::TupleAssignment {
                names, expression, ..
            } => {
                self.visit_expr(expression);
                for name in names {
                    self.symbols.insert(name.clone(), ValueType::Unknown);
                    self.summary.declarations.push(SemanticDeclaration {
                        kind: "variable".to_owned(),
                        name: name.clone(),
                        line: expression.range.start_line,
                        executable: true,
                        unsupported_reason: None,
                    });
                }
            }
            Statement::If {
                condition,
                then_body,
                else_body,
                ..
            } => {
                if self.visit_expr(condition) != ValueType::Bool
                    && self.visit_expr(condition) != ValueType::Unknown
                {
                    self.summary.diagnostics.push(Diagnostic::error(
                        "PINE_CONDITION_NOT_BOOL",
                        "if condition must evaluate to bool",
                        condition.range.start_line,
                    ));
                }
                for item in then_body {
                    self.visit_statement(item);
                }
                for item in else_body {
                    self.visit_statement(item);
                }
            }
            Statement::For {
                start,
                end,
                step,
                body,
                ..
            } => {
                for expression in [Some(start), Some(end), step.as_ref()]
                    .into_iter()
                    .flatten()
                {
                    if self.visit_expr(expression) != ValueType::Number
                        && self.visit_expr(expression) != ValueType::Unknown
                    {
                        self.summary.diagnostics.push(Diagnostic::error(
                            "PINE_FOR_BOUND_NOT_NUMBER",
                            "for loop bounds must be numeric",
                            expression.range.start_line,
                        ));
                    }
                }
                for item in body {
                    self.visit_statement(item);
                }
            }
            Statement::Call { expression, .. } => {
                self.visit_expr(expression);
            }
            Statement::Function {
                name,
                parameters,
                body,
                range,
            } => {
                if self.functions.contains(name) {
                    self.summary.diagnostics.push(Diagnostic::error(
                        "PINE_FUNCTION_DUPLICATE",
                        format!("function {name:?} is declared more than once"),
                        range.start_line,
                    ));
                }
                self.functions.insert(name.clone());
                self.summary.declarations.push(SemanticDeclaration {
                    kind: "function".to_owned(),
                    name: name.clone(),
                    line: range.start_line,
                    executable: true,
                    unsupported_reason: None,
                });
                for parameter in parameters {
                    self.symbols.insert(parameter.clone(), ValueType::Unknown);
                }
                self.visit_expr(body);
            }
            Statement::Unsupported { range, text } => {
                let (code, message) = if text.starts_with("import ")
                    || text.starts_with("library(")
                    || text.starts_with("type ")
                    || text.starts_with("method ")
                {
                    (
                        "PINE_DECLARATION_UNSUPPORTED",
                        "Pine libraries, types, and methods are not executable in this strategy runtime",
                    )
                } else {
                    (
                        "PINE_STATEMENT_UNSUPPORTED",
                        "statement is outside the executable Pine v6 subset",
                    )
                };
                self.summary
                    .diagnostics
                    .push(Diagnostic::error(code, message, range.start_line));
            }
        }
    }

    fn visit_expr(&mut self, expression: &Expr) -> ValueType {
        match &expression.kind {
            ExprKind::Number { .. } => ValueType::Number,
            ExprKind::String { .. } => ValueType::String,
            ExprKind::Boolean { .. } => ValueType::Bool,
            ExprKind::Null => ValueType::Null,
            ExprKind::Identifier { name } => self.identifier_type(name),
            ExprKind::Member { object, member } => {
                let _ = self.visit_expr(object);
                if member == "long" || member == "short" || member.starts_with("is") {
                    ValueType::Bool
                } else {
                    ValueType::Unknown
                }
            }
            ExprKind::Index { object, index } => {
                self.visit_expr(index);
                self.visit_expr(object)
            }
            ExprKind::Unary { op, expression } => {
                let ty = self.visit_expr(expression);
                match op {
                    UnaryOp::Not => ValueType::Bool,
                    UnaryOp::Negate | UnaryOp::Positive => ty,
                }
            }
            ExprKind::Binary { left, op, right } => {
                let left_type = self.visit_expr(left);
                let right_type = self.visit_expr(right);
                match op {
                    BinaryOp::Or
                    | BinaryOp::And
                    | BinaryOp::Equal
                    | BinaryOp::NotEqual
                    | BinaryOp::Less
                    | BinaryOp::LessEqual
                    | BinaryOp::Greater
                    | BinaryOp::GreaterEqual => ValueType::Bool,
                    BinaryOp::Add
                        if left_type == ValueType::String || right_type == ValueType::String =>
                    {
                        ValueType::String
                    }
                    BinaryOp::Add
                    | BinaryOp::Subtract
                    | BinaryOp::Multiply
                    | BinaryOp::Divide
                    | BinaryOp::Remainder => ValueType::Number,
                }
            }
            ExprKind::Ternary {
                condition,
                when_true,
                when_false,
            } => {
                if self.visit_expr(condition) != ValueType::Bool
                    && self.visit_expr(condition) != ValueType::Unknown
                {
                    self.summary.diagnostics.push(Diagnostic::error(
                        "PINE_CONDITION_NOT_BOOL",
                        "ternary condition must evaluate to bool",
                        condition.range.start_line,
                    ));
                }
                let true_type = self.visit_expr(when_true);
                let false_type = self.visit_expr(when_false);
                if true_type == false_type {
                    true_type
                } else {
                    ValueType::Unknown
                }
            }
            ExprKind::Tuple { items } => {
                for item in items {
                    self.visit_expr(item);
                }
                ValueType::Unknown
            }
            ExprKind::Call { callee, arguments } => {
                self.visit_call(callee, arguments, expression.range)
            }
        }
    }

    fn visit_call(&mut self, callee: &str, arguments: &[Expr], range: SourceRange) -> ValueType {
        let lower = callee.to_ascii_lowercase();
        for argument in arguments {
            self.visit_expr(argument);
        }
        if is_visual_call(&lower) {
            self.summary.diagnostics.push(Diagnostic::warning(
                "PINE_VISUAL_IGNORED",
                format!("visual-only call \"{callee}\" is ignored by JFTrade"),
                range.start_line,
            ));
            self.summary.visuals.push(VisualMetadata {
                line: range.start_line,
                kind: lower.trim_start_matches("ta.").to_owned(),
                call: callee.to_owned(),
                target: arguments.first().and_then(identifier_name),
                arguments: arguments.iter().map(ToString::to_string).collect(),
                text: format_expr_call(callee, arguments),
            });
            return ValueType::Unknown;
        }
        if is_supported_call(&lower) {
            return call_result_type(&lower);
        }
        if self.functions.contains(callee) {
            return ValueType::Unknown;
        }
        self.summary.diagnostics.push(Diagnostic::error(
            "PINE_CALL_UNSUPPORTED",
            format!("function {callee:?} is not supported by the Pine v6 runtime"),
            range.start_line,
        ));
        ValueType::Unknown
    }

    fn identifier_type(&self, name: &str) -> ValueType {
        if let Some(value) = self.symbols.get(name) {
            return *value;
        }
        match name.to_ascii_lowercase().as_str() {
            "close" | "open" | "high" | "low" | "volume" | "hl2" | "hlc3" | "ohlc4"
            | "bar_index" | "time" | "hour" | "minute" | "dayofweek" | "dayofmonth" | "month"
            | "year" | "na" => ValueType::Number,
            "true" | "false" => ValueType::Bool,
            _ => ValueType::Unknown,
        }
    }
}

fn is_supported_call(callee: &str) -> bool {
    matches!(
        callee,
        "strategy.entry"
            | "strategy.order"
            | "strategy.close"
            | "strategy.close_all"
            | "strategy.exit"
            | "strategy.cancel"
            | "strategy.cancel_all"
            | "strategy.risk.allow_entry_in"
            | "alert"
            | "alertcondition"
            | "log.info"
            | "log.warning"
            | "log.error"
            | "ta.ema"
            | "ta.sma"
            | "ta.rma"
            | "ta.wma"
            | "ta.hma"
            | "ta.vwma"
            | "ta.rsi"
            | "ta.macd"
            | "ta.atr"
            | "ta.tr"
            | "ta.stdev"
            | "ta.variance"
            | "ta.cci"
            | "ta.highest"
            | "ta.lowest"
            | "ta.change"
            | "ta.mom"
            | "ta.roc"
            | "ta.range"
            | "ta.mode"
            | "ta.sum"
            | "ta.rising"
            | "ta.falling"
            | "ta.bb"
            | "ta.bbw"
            | "ta.cog"
            | "ta.wpr"
            | "ta.vwap"
            | "ta.mfi"
            | "ta.dmi"
            | "ta.supertrend"
            | "ta.sar"
            | "ta.crossover"
            | "ta.crossunder"
            | "ta.cross"
            | "request.security"
            | "nz"
            | "timestamp"
            | "math.abs"
            | "math.min"
            | "math.max"
            | "math.avg"
            | "math.round"
            | "math.round_to_mintick"
            | "math.floor"
            | "math.ceil"
            | "math.sqrt"
            | "math.pow"
            | "math.log"
            | "math.sign"
            | "input"
            | "input.int"
            | "input.float"
            | "input.bool"
            | "input.string"
            | "input.source"
            | "input.time"
            | "input.timeframe"
            | "input.color"
            | "color.new"
            | "color.rgb"
            | "ticker.heikinashi"
            | "ticker.standard"
            | "ticker.inherit"
    )
}

fn call_result_type(callee: &str) -> ValueType {
    if matches!(
        callee,
        "ta.crossover"
            | "ta.crossunder"
            | "ta.cross"
            | "ta.rising"
            | "ta.falling"
            | "strategy.risk.allow_entry_in"
            | "input.bool"
            | "barstate.isfirst"
    ) {
        ValueType::Bool
    } else if matches!(
        callee,
        "alert"
            | "alertcondition"
            | "log.info"
            | "log.warning"
            | "log.error"
            | "strategy.entry"
            | "strategy.order"
            | "strategy.close"
            | "strategy.close_all"
            | "strategy.exit"
            | "strategy.cancel"
            | "strategy.cancel_all"
    ) {
        ValueType::Unknown
    } else {
        ValueType::Number
    }
}

fn is_visual_call(callee: &str) -> bool {
    matches!(
        callee,
        "plot"
            | "plotchar"
            | "plotshape"
            | "hline"
            | "bgcolor"
            | "barcolor"
            | "fill"
            | "label.new"
            | "line.new"
            | "box.new"
            | "table.new"
            | "table.cell"
            | "alertcondition"
    )
}

fn identifier_name(expression: &Expr) -> Option<String> {
    match &expression.kind {
        ExprKind::Identifier { name } => Some(name.clone()),
        ExprKind::Member { member, .. } => Some(member.clone()),
        _ => None,
    }
}
fn format_expr_call(callee: &str, arguments: &[Expr]) -> String {
    format!(
        "{}({})",
        callee,
        arguments
            .iter()
            .map(ToString::to_string)
            .collect::<Vec<_>>()
            .join(", ")
    )
}
