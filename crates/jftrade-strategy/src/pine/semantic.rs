use std::collections::{BTreeMap, BTreeSet};

use serde::Serialize;

use super::parser::{
    BinaryOp, Expr, ExprKind, Program, SourceRange, Statement, StrategyDeclaration, UnaryOp,
};

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
    if let Some(strategy) = program.strategy.as_ref() {
        summary
            .diagnostics
            .extend(strategy_declaration_diagnostics(strategy));
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

/// Go's maximum `series[n]` lookback (`pkg/strategy/pine/parse.go`).
const MAX_HISTORY_LOOKBACK: u64 = 500;

/// Go's static `for` unroll cap (`pkg/strategy/pine/parse.go`).
const MAX_STATIC_FOR_ITERATIONS: u64 = 100;

/// Reads a constant integer literal, including a leading unary minus.
fn constant_int(expression: &Expr) -> Option<i64> {
    match &expression.kind {
        ExprKind::Number { value } => value.trim().parse().ok(),
        ExprKind::Unary {
            op: UnaryOp::Negate,
            expression,
        } => constant_int(expression).map(|value| -value),
        ExprKind::Unary {
            op: UnaryOp::Positive,
            expression,
        } => constant_int(expression),
        _ => None,
    }
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
                range,
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
                self.report_static_for_diagnostics(start, end, step.as_ref(), *range);
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

    /// Parity: `pkg/strategy/pine/static_for_helpers.go::expandStaticForLoopValues`.
    ///
    /// Go unrolls a constant `for` loop and rejects a zero step, a step that
    /// never reaches the end value, and more than
    /// `maxStaticForIterations = 100` iterations. Rust keeps the loop typed
    /// but surfaces the same three diagnostics for constant bounds.
    fn report_static_for_diagnostics(
        &mut self,
        start: &Expr,
        end: &Expr,
        step: Option<&Expr>,
        range: SourceRange,
    ) {
        let line = range.start_line;
        let (Some(start), Some(end)) = (constant_int(start), constant_int(end)) else {
            return;
        };
        let step = match step {
            Some(step) => match constant_int(step) {
                Some(step) => step,
                None => return,
            },
            None => 1,
        };
        if step == 0 {
            self.summary.diagnostics.push(Diagnostic::error(
                "PINE_LOOP_LIMIT_UNSUPPORTED",
                "for loop step cannot be 0",
                line,
            ));
            return;
        }
        if (step > 0 && start > end) || (step < 0 && start < end) {
            self.summary.diagnostics.push(Diagnostic::error(
                "PINE_LOOP_LIMIT_UNSUPPORTED",
                "for loop step does not reach the end value",
                line,
            ));
            return;
        }
        let mut iterations = 0u64;
        let mut value = start;
        loop {
            // Go stops once the value passes the end bound, so a range such as
            // `0 to 3 by 2` reports "does not reach" instead of running to the
            // iteration cap.
            if (step > 0 && value > end) || (step < 0 && value < end) {
                break;
            }
            iterations += 1;
            if iterations > MAX_STATIC_FOR_ITERATIONS {
                self.summary.diagnostics.push(Diagnostic::error(
                    "PINE_LOOP_LIMIT_UNSUPPORTED",
                    format!("for loop expands to more than {MAX_STATIC_FOR_ITERATIONS} iterations"),
                    line,
                ));
                return;
            }
            if value == end {
                return;
            }
            value = value.saturating_add(step);
        }
        self.summary.diagnostics.push(Diagnostic::error(
            "PINE_LOOP_LIMIT_UNSUPPORTED",
            "for loop step does not reach the end value",
            line,
        ));
    }

    /// Parity: `pkg/strategy/pine/validate.go::historyDiagnosticMessage`.
    ///
    /// Go rejects a history reference whose receiver is a call result
    /// (`ta.sma(close, 20)[2]`) and one whose lookback exceeds 500 bars. The
    /// Rust parser keeps `series[n]` as a typed index expression, so the same
    /// contract is detected structurally instead of by regex.
    fn report_history_reference_diagnostics(
        &mut self,
        object: &Expr,
        index: &Expr,
        expression: &Expr,
    ) {
        let line = expression.range.start_line;
        if matches!(object.kind, ExprKind::Call { .. }) {
            self.summary.diagnostics.push(Diagnostic::error(
                "PINE_HISTORY_REF_UNSUPPORTED",
                "history references are supported only on identifiers or object fields; assign the function result first",
                line,
            ));
            return;
        }
        if let ExprKind::Number { value } = &index.kind {
            match value.trim().parse::<u64>() {
                Ok(lookback) if lookback > MAX_HISTORY_LOOKBACK => {
                    self.summary.diagnostics.push(Diagnostic::error(
                        "PINE_HISTORY_REF_UNSUPPORTED",
                        format!(
                            "history reference lookback {lookback} exceeds JFTrade maximum {MAX_HISTORY_LOOKBACK}"
                        ),
                        line,
                    ));
                }
                Ok(_) => {}
                // Go parses the lookback with `strconv.Atoi`; a value that
                // overflows the integer type is reported as a non-negative
                // integer violation instead of being accepted.
                Err(_) => {
                    self.summary.diagnostics.push(Diagnostic::error(
                        "PINE_HISTORY_REF_UNSUPPORTED",
                        "history reference lookback must be a non-negative integer",
                        line,
                    ));
                }
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
                self.report_history_reference_diagnostics(object, index, expression);
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
        // Go classifies `request.security` side effects from the whole line
        // before it walks the inner calls, so the purity diagnostic must lead
        // the diagnostics of the expression it wraps.
        if lower == "request.security"
            && let Some(diagnostic) = request_security_diagnostic(callee, arguments, range)
        {
            self.summary.diagnostics.push(diagnostic);
            for argument in arguments {
                self.visit_expr(argument);
            }
            return ValueType::Unknown;
        }
        for argument in arguments {
            self.visit_expr(argument);
        }
        // Go's public helper guard runs before the generic unsupported-call
        // diagnostic so a script that calls an internal JFTrade helper gets
        // the actionable "use Pine v6 ..." replacement instead of a bare
        // `function not supported` error.
        if let Some(guard) = public_helper_guard(callee) {
            self.summary.diagnostics.push(Diagnostic::error(
                guard.code,
                guard.message,
                range.start_line,
            ));
            return ValueType::Unknown;
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
        if lower.starts_with("strategy.risk.")
            && let Some(diagnostic) = strategy_risk_diagnostic(&lower, arguments, range)
        {
            self.summary.diagnostics.push(diagnostic);
            return ValueType::Unknown;
        }
        if lower.starts_with("strategy.")
            && let Some(diagnostic) = strategy_order_diagnostic(&lower, arguments, range)
        {
            self.summary.diagnostics.push(diagnostic);
            return ValueType::Unknown;
        }
        if lower == "request.security" {
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
            | "strategy.risk.max_drawdown"
            | "strategy.risk.max_intraday_loss"
            | "strategy.risk.max_intraday_filled_orders"
            | "strategy.risk.max_position_size"
            | "strategy.risk.max_cons_loss_days"
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
            | "ta.linreg"
            | "ta.obv"
            | "ta.pivothigh"
            | "ta.pivotlow"
            | "ta.kc"
            | "ta.kcw"
            | "ta.alma"
            | "ta.cmo"
            | "ta.tsi"
            | "ta.correlation"
            | "ta.dev"
            | "ta.median"
            | "ta.percentile_linear_interpolation"
            | "ta.percentile_nearest_rank"
            | "ta.percentrank"
            | "ta.swma"
            | "ta.crossover"
            | "ta.crossunder"
            | "ta.cross"
            | "ta.cum"
            | "ta.highestbars"
            | "ta.lowestbars"
            | "ta.stoch"
            | "ta.barssince"
            | "ta.valuewhen"
            | "request.security"
            | "min"
            | "max"
            | "int"
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

/// Stable guard metadata for a JFTrade-only helper that must not appear in a
/// public Pine v6 script.
pub(crate) struct PublicHelperGuard {
    pub code: &'static str,
    pub message: String,
}

/// Maps a public helper call to the Go guard diagnostic, mirroring
/// `pkg/strategy/pine/public_helper_guard.go`.
pub(crate) fn public_helper_guard(callee: &str) -> Option<PublicHelperGuard> {
    let lower = callee.trim().to_ascii_lowercase();
    if let Some(name) = lower.strip_prefix("ta.")
        && let Some(replacement) = ta_shortcut_replacement(name)
    {
        return Some(PublicHelperGuard {
            code: "PINE_PUBLIC_TA_SHORTCUT",
            message: format!(
                "ta.{name}() is a JFTrade-only shortcut; use Pine v6 {replacement} instead"
            ),
        });
    }
    let replacement = internal_helper_replacement(&lower)?;
    Some(PublicHelperGuard {
        code: "PINE_INTERNAL_HELPER_PUBLIC",
        message: format!(
            "{lower}() is an internal JFTrade helper; use Pine v6 {replacement} instead"
        ),
    })
}

fn ta_shortcut_replacement(name: &str) -> Option<&'static str> {
    match name {
        "adx" => Some("ta.dmi"),
        _ => None,
    }
}

/// The substring of Go's `publicDisabledHelperNames` that the Rust parser can
/// currently reach; every entry keeps the documented Pine v6 replacement.
fn internal_helper_replacement(name: &str) -> Option<&'static str> {
    match name {
        "alma" => Some("ta.alma"),
        "anchored_vwap" => Some("ta.vwap(source, timeframe.change(...))"),
        "atr" => Some("ta.atr"),
        "bbw" => Some("ta.bbw"),
        "bollinger" => Some("ta.bb"),
        "barssince" => Some("ta.barssince"),
        "cci" => Some("ta.cci"),
        "change" => Some("ta.change"),
        "cmo" => Some("ta.cmo"),
        "cog" => Some("ta.cog"),
        "correlation" => Some("ta.correlation"),
        "cum" => Some("ta.cum"),
        "dev" => Some("ta.dev"),
        "dmi" => Some("ta.dmi"),
        "falling" => Some("ta.falling"),
        "highest" => Some("ta.highest"),
        "highestbars" => Some("ta.highestbars"),
        "history" => Some("series[n]"),
        "ifelse" => Some("condition ? valueWhenTrue : valueWhenFalse"),
        "kc" => Some("ta.kc"),
        "kcw" => Some("ta.kcw"),
        "kdj" => Some("ta.stoch plus Pine smoothing"),
        "linreg" => Some("ta.linreg"),
        "lowest" => Some("ta.lowest"),
        "lowestbars" => Some("ta.lowestbars"),
        "ma" => Some("ta.sma/ta.ema/ta.rma/ta.wma/ta.hma/ta.vwma"),
        "macd" => Some("ta.macd"),
        "median" => Some("ta.median"),
        "mfi" => Some("ta.mfi"),
        "mode" => Some("ta.mode"),
        "mom" => Some("ta.mom"),
        "notify" => Some("alert"),
        "obv" => Some("ta.obv"),
        "percentile_linear_interpolation" => Some("ta.percentile_linear_interpolation"),
        "percentile_nearest_rank" => Some("ta.percentile_nearest_rank"),
        "percentrank" => Some("ta.percentrank"),
        "pivothigh" => Some("ta.pivothigh"),
        "pivotlow" => Some("ta.pivotlow"),
        "previous" => Some("series[1]"),
        "range" => Some("ta.range"),
        "rising" => Some("ta.rising"),
        "roc" => Some("ta.roc"),
        "rsi" => Some("ta.rsi"),
        "sar" => Some("ta.sar"),
        "security_source" => Some("request.security"),
        "stdev" => Some("ta.stdev"),
        "stoch" => Some("ta.stoch"),
        "sum" => Some("ta.sum"),
        "supertrend" => Some("ta.supertrend"),
        "swma" => Some("ta.swma"),
        "tr" => Some("ta.tr"),
        "tsi" => Some("ta.tsi"),
        "variance" => Some("ta.variance"),
        "valuewhen" => Some("ta.valuewhen"),
        "vwap" => Some("ta.vwap"),
        "williams_r" => Some("ta.wpr"),
        "williamsr" => Some("ta.wpr"),
        "cross_over" => Some("ta.crossover"),
        "cross_under" => Some("ta.crossunder"),
        _ => None,
    }
}

/// Validates one `request.security(...)` call against Go's supported subset.
///
/// Parity: `pkg/strategy/pine/validate.go::requestSecurityUnsupportedDiagnostic`.
/// Go only allows a static current-symbol ticker, a static timeframe string,
/// and a pure expression; lookahead/gaps merge flags, side effects, nested
/// calls and dynamic symbols are rejected with stable diagnostic codes.
fn request_security_diagnostic(
    callee: &str,
    arguments: &[Expr],
    range: SourceRange,
) -> Option<Diagnostic> {
    let error = |code: &str, message: &str| {
        Some(Diagnostic::error(
            code,
            message.to_owned(),
            range.start_line,
        ))
    };
    if arguments.len() < 3 {
        return error(
            "PINE_REQUEST_SECURITY_UNSUPPORTED",
            "request.security() requires symbol, timeframe, and expression arguments",
        );
    }
    // Go inspects every argument after the expression textually, so the merge
    // flags are rejected in both the named (`gaps=barmerge.gaps_on`) and the
    // positional (`request.security(..., barmerge.gaps_on)`) call forms.
    for argument in arguments.iter().skip(3) {
        let lower = argument.to_string().to_ascii_lowercase();
        if lower.contains("barmerge.lookahead_on") {
            return error(
                "PINE_REQUEST_SECURITY_LOOKAHEAD",
                "request.security() lookahead_on is not supported by JFTrade; use default lookahead_off",
            );
        }
        if lower.contains("barmerge.gaps_on") {
            return error(
                "PINE_REQUEST_SECURITY_GAPS",
                "request.security() gaps_on is not supported by JFTrade; use default gaps_off",
            );
        }
        let (name, _) = request_security_named_argument(argument);
        if lower.starts_with("calc_bars_count=") || name.eq_ignore_ascii_case("calc_bars_count") {
            return error(
                "PINE_REQUEST_SECURITY_CALC_BARS_COUNT",
                "request.security() calc_bars_count is not supported by JFTrade",
            );
        }
    }
    let symbol = arguments[0].to_string();
    if !is_supported_request_security_ticker(&symbol) {
        return error(
            "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL",
            "request.security() currently supports only syminfo.tickerid and static ticker.heikinashi/standard/inherit expressions rooted at it; dynamic or external symbols are not supported",
        );
    }
    let timeframe = arguments[1].to_string();
    if !timeframe.trim().starts_with('"') && !timeframe.trim().starts_with('\'') {
        // Go accepts a string literal or an input.timeframe alias (an
        // identifier); arbitrary expressions are dynamic timeframes.
        let is_alias = matches!(&arguments[1].kind, ExprKind::Identifier { .. });
        if !is_alias {
            return error(
                "PINE_REQUEST_SECURITY_DYNAMIC_TIMEFRAME",
                "request.security() currently supports only static timeframe strings",
            );
        }
    }
    let expression = arguments[2].to_string().to_ascii_lowercase();
    if expression.contains("request.security(") {
        return error(
            "PINE_REQUEST_SECURITY_NESTED",
            "nested request.security() calls are not supported by JFTrade",
        );
    }
    if request_security_expression_has_side_effect(&arguments[2]) {
        return error(
            "PINE_REQUEST_SECURITY_SIDE_EFFECT",
            "request.security() expression must be pure; strategy, alert, visual, collection mutation, and reassignment side effects are not supported",
        );
    }
    if let Some(message) = request_security_inner_ta_contract(&arguments[2], &arguments[1]) {
        return error("PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED", &message);
    }
    let _ = callee;
    None
}

/// Go's request.security lowering accepts a narrower TA subset than the
/// chart-timeframe planner.  Keep unsupported inner calls from silently
/// falling back to an opaque `security:` requirement, which would make an
/// invalid MTF expression look executable.
fn request_security_inner_ta_contract(expression: &Expr, timeframe: &Expr) -> Option<String> {
    match &expression.kind {
        ExprKind::Call { callee, arguments } => {
            let lower = callee.to_ascii_lowercase();
            match lower.as_str() {
                "ta.sum" => {
                    return Some("request.security() expression ta.sum is unsupported".to_owned());
                }
                "ta.bb" if arguments.len() != 3 => {
                    return Some(
                        "request.security() ta.bb requires source, length, and multiplier"
                            .to_owned(),
                    );
                }
                "ta.correlation"
                    if arguments
                        .get(1)
                        .is_some_and(|value| !request_security_source_is_allowed(value)) =>
                {
                    return Some(
                        "request.security() ta.correlation second source is unsupported".to_owned(),
                    );
                }
                "ta.obv" if !request_security_timeframe_is_intraday(timeframe) => {
                    return Some(
                        "request.security() ta.obv is supported only for intraday timeframes"
                            .to_owned(),
                    );
                }
                _ => {}
            }
            arguments
                .iter()
                .find_map(|argument| request_security_inner_ta_contract(argument, timeframe))
        }
        ExprKind::Member { object, member }
            if member.eq_ignore_ascii_case("obv")
                && matches!(&object.kind, ExprKind::Identifier { name } if name.eq_ignore_ascii_case("ta"))
                && !request_security_timeframe_is_intraday(timeframe) =>
        {
            Some("request.security() ta.obv is supported only for intraday timeframes".to_owned())
        }
        ExprKind::Binary { left, right, .. } => request_security_inner_ta_contract(left, timeframe)
            .or_else(|| request_security_inner_ta_contract(right, timeframe)),
        ExprKind::Unary { expression, .. }
        | ExprKind::Index {
            object: expression, ..
        } => request_security_inner_ta_contract(expression, timeframe),
        ExprKind::Ternary {
            condition,
            when_true,
            when_false,
        } => request_security_inner_ta_contract(condition, timeframe)
            .or_else(|| request_security_inner_ta_contract(when_true, timeframe))
            .or_else(|| request_security_inner_ta_contract(when_false, timeframe)),
        ExprKind::Tuple { items } => items
            .iter()
            .find_map(|item| request_security_inner_ta_contract(item, timeframe)),
        ExprKind::Member { object, .. } => request_security_inner_ta_contract(object, timeframe),
        _ => None,
    }
}

fn request_security_source_is_allowed(expression: &Expr) -> bool {
    matches!(&expression.kind, ExprKind::Identifier { name }
        if matches!(name.to_ascii_lowercase().as_str(), "open" | "high" | "low" | "close" | "volume" | "hl2" | "hlc3" | "ohlc4"))
}

fn request_security_timeframe_is_intraday(expression: &Expr) -> bool {
    let ExprKind::String { value } = &expression.kind else {
        return false;
    };
    matches!(
        value.trim().to_ascii_uppercase().as_str(),
        "1" | "5" | "15" | "30" | "45" | "60" | "120" | "240"
    )
}

/// Argument lists Go accepts for each executable order call
/// (`strategy_call_helpers.go`, `strategy_call_helpers.go::parseStrategyExit`).
const ORDER_ENTRY_ALLOWED_ARGUMENTS: &[&str] = &[
    "qty",
    "qty_percent",
    "limit",
    "stop",
    "oca_name",
    "oca_type",
    "comment",
    "alert_message",
    "disable_alert",
    "when",
];

const ORDER_CLOSE_ALLOWED_ARGUMENTS: &[&str] = &[
    "qty",
    "qty_percent",
    "limit",
    "stop",
    "comment",
    "alert_message",
    "immediately",
    "disable_alert",
    "when",
];

const ORDER_EXIT_ALLOWED_ARGUMENTS: &[&str] = &[
    "from_entry",
    "qty",
    "qty_percent",
    "profit",
    "limit",
    "loss",
    "stop",
    "trail_price",
    "trail_points",
    "trail_offset",
    "oca_name",
    "oca_type",
    "comment",
    "comment_profit",
    "comment_loss",
    "comment_trailing",
    "alert_message",
    "alert_profit",
    "alert_loss",
    "alert_trailing",
    "disable_alert",
    "when",
];

const ORDER_CLOSE_ALL_ALLOWED_ARGUMENTS: &[&str] =
    &["immediately", "comment", "alert_message", "disable_alert"];

/// Parity: `pkg/strategy/pine/analysis.go::diagnosticCodeForCompileMessage`
/// plus `strategy_call_helpers.go` arity, argument and trigger validation.
///
/// Go rejects order calls that are missing their id/direction, use OCA
/// arguments, mix `qty` with `qty_percent`, combine a trail with a stop/limit
/// bracket, or ask for an exit without any trigger. It also rejects unknown
/// named arguments, non-boolean `disable_alert`/`immediately` metadata and
/// excess `strategy.close_all` positionals. Rust surfaces the same stable codes
/// and keeps the messages that the Go analysis layer matches on.
fn strategy_order_diagnostic(
    callee: &str,
    arguments: &[Expr],
    range: SourceRange,
) -> Option<Diagnostic> {
    let names = order_named_arguments(arguments);
    let has = |target: &str| names.iter().any(|(name, _)| name == target);
    if let Some(diagnostic) = order_arity_diagnostic(callee, arguments, range) {
        return Some(diagnostic);
    }
    if has("oca_name") || has("oca_type") {
        return Some(Diagnostic::error(
            "PINE_ORDER_OCA_UNSUPPORTED",
            format!("{callee} OCA arguments are not supported by JFTrade"),
            range.start_line,
        ));
    }
    let allowed = match callee {
        "strategy.entry" | "strategy.order" => Some(ORDER_ENTRY_ALLOWED_ARGUMENTS),
        "strategy.close" => Some(ORDER_CLOSE_ALLOWED_ARGUMENTS),
        "strategy.exit" => Some(ORDER_EXIT_ALLOWED_ARGUMENTS),
        "strategy.close_all" => Some(ORDER_CLOSE_ALL_ALLOWED_ARGUMENTS),
        _ => None,
    };
    if let Some(allowed) = allowed {
        for (name, _) in &names {
            if !allowed.iter().any(|candidate| candidate == name) {
                return Some(Diagnostic::error(
                    "PINE_COMPILE_ERROR",
                    format!("{callee} argument {name} is not supported by JFTrade"),
                    range.start_line,
                ));
            }
        }
    }
    if callee == "strategy.close_all"
        && let Some(diagnostic) = order_close_all_positional_diagnostic(arguments, &has, range)
    {
        return Some(diagnostic);
    }
    if callee == "strategy.cancel_all" && !arguments.is_empty() {
        return Some(Diagnostic::error(
            "PINE_COMPILE_ERROR",
            "strategy.cancel_all arguments are not supported by JFTrade yet",
            range.start_line,
        ));
    }
    if let Some(diagnostic) = order_boolean_metadata_diagnostic(callee, arguments, &has, range) {
        return Some(diagnostic);
    }
    if has("qty") && has("qty_percent") {
        return Some(Diagnostic::error(
            "PINE_ORDER_QTY_CONFLICT",
            format!("{callee} supports qty or qty_percent, not both"),
            range.start_line,
        ));
    }
    if callee == "strategy.exit" {
        let trail_points = has("trail_points");
        let trail_price = has("trail_price");
        let bracket = has("stop") || has("limit") || has("profit") || has("loss");
        if trail_points && trail_price {
            return Some(Diagnostic::error(
                "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED",
                "strategy.exit accepts trail_points or trail_price, not both",
                range.start_line,
            ));
        }
        if (trail_points || trail_price) && bracket {
            return Some(Diagnostic::error(
                "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED",
                "strategy.exit trail with stop/limit is not supported by JFTrade yet",
                range.start_line,
            ));
        }
        if (trail_points || trail_price) && !has("trail_offset") {
            return Some(Diagnostic::error(
                "PINE_COMPILE_ERROR",
                "strategy.exit trailing stop requires trail_offset",
                range.start_line,
            ));
        }
        let has_trigger = bracket || trail_points || trail_price;
        if !has_trigger {
            return Some(Diagnostic::error(
                "PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED",
                "strategy.exit advanced exit semantics are not supported by JFTrade yet",
                range.start_line,
            ));
        }
    }
    None
}

/// Parity: `pkg/strategy/pine/validate.go::applyStrategyNamedArg`.
///
/// Go keeps the documented default for every strategy declaration constant and
/// appends a warning that names the offending argument. Rust silently ignored
/// unsupported constants, so `validate_script` reported ok without the warnings
/// the Go payload carried.
fn strategy_declaration_diagnostics(strategy: &StrategyDeclaration) -> Vec<Diagnostic> {
    let line = strategy.range.start_line;
    let mut diagnostics = Vec::new();
    let mut warn = |message: String| {
        diagnostics.push(Diagnostic::warning(
            "PINE_STRATEGY_DECLARATION_FALLBACK",
            message,
            line,
        ));
    };
    for argument in strategy.arguments.iter().skip(1) {
        let Some(name) = argument.name.as_deref() else {
            continue;
        };
        let raw = constant_text(&argument.value);
        let value = raw.trim();
        match name.to_ascii_lowercase().as_str() {
            "default_qty_type" if normalize_strategy_default_qty_mode(value).is_none() => {
                warn(format!(
                    "pine strategy default_qty_type {value:?} is not supported by JFTrade; using strategy.fixed"
                ))
            }
            "pyramiding" if !is_non_negative_int_text(value) => warn(format!(
                "pine strategy pyramiding {value:?} is not a supported constant integer; using 1"
            )),
            "initial_capital" if !is_positive_float_text(value) => warn(format!(
                "pine strategy initial_capital {value:?} must be a positive constant number"
            )),
            "commission_type" if normalize_strategy_commission_type(value).is_none() => warn(
                format!("pine strategy commission_type {value:?} is not supported by JFTrade"),
            ),
            "commission_value" if !is_non_negative_float_text(value) => warn(format!(
                "pine strategy commission_value {value:?} must be a non-negative constant number"
            )),
            "slippage" if !is_non_negative_int_text(value) => warn(format!(
                "pine strategy slippage {value:?} must be a non-negative constant integer"
            )),
            "process_orders_on_close" if !is_bool_text(value) => warn(format!(
                "pine strategy process_orders_on_close {value:?} must be true or false"
            )),
            _ => {}
        }
    }
    diagnostics
}

fn normalize_strategy_default_qty_mode(raw: &str) -> Option<&'static str> {
    let normalized = raw.trim().to_ascii_lowercase();
    let normalized = normalized
        .strip_prefix("strategy.")
        .unwrap_or(normalized.as_str());
    match normalized {
        "" | "fixed" => Some("fixed"),
        "cash" => Some("cash"),
        "percent_of_equity" => Some("percent_of_equity"),
        _ => None,
    }
}

fn normalize_strategy_commission_type(raw: &str) -> Option<&'static str> {
    let normalized = raw.trim().to_ascii_lowercase();
    let normalized = normalized
        .strip_prefix("strategy.commission.")
        .unwrap_or(normalized.as_str());
    match normalized {
        "percent" => Some("percent"),
        "cash_per_order" => Some("cash_per_order"),
        "cash_per_contract" => Some("cash_per_contract"),
        _ => None,
    }
}

fn is_positive_float_text(raw: &str) -> bool {
    raw.trim().parse::<f64>().is_ok_and(|value| value > 0.0)
}

fn is_non_negative_float_text(raw: &str) -> bool {
    raw.trim().parse::<f64>().is_ok_and(|value| value >= 0.0)
}

fn is_non_negative_int_text(raw: &str) -> bool {
    raw.trim().parse::<i64>().is_ok_and(|value| value >= 0)
}

fn is_bool_text(raw: &str) -> bool {
    matches!(raw.trim().to_ascii_lowercase().as_str(), "true" | "false")
}

/// Parity: `pkg/strategy/pine/parse_strategy.go` risk entry points and
/// `pkg/strategy/pine/strategy_call_helpers.go` risk argument parsers.
///
/// Go validates every `strategy.risk.*` declaration while parsing: the
/// direction must normalise to all/long/short, drawdown-style limits need a
/// positive constant plus a `cash`/`percent_of_equity` type, count limits need
/// a positive integer, and `max_position_size` takes exactly one positive
/// constant. Rust only projected the metadata into the lowered program and
/// silently accepted invalid declarations, so `validate_script` reported ok
/// for scripts Go rejects.
fn strategy_risk_diagnostic(
    callee: &str,
    arguments: &[Expr],
    range: SourceRange,
) -> Option<Diagnostic> {
    let positional: Vec<&Expr> = arguments
        .iter()
        .filter(|argument| !is_named_argument(argument))
        .collect();
    let named = order_named_arguments(arguments);
    let extra_named = |allowed: &[&str]| {
        named
            .iter()
            .any(|(name, _)| !allowed.iter().any(|candidate| name == candidate))
    };
    let error = |message: String| {
        Some(Diagnostic::error(
            "PINE_COMPILE_ERROR",
            message,
            range.start_line,
        ))
    };
    match callee {
        "strategy.risk.allow_entry_in" => {
            if positional.len() + named.len() != 1 {
                return error(
                    "strategy.risk.allow_entry_in(direction) requires one argument".to_owned(),
                );
            }
            let Some(direction) = positional.first() else {
                let (name, value) = &named[0];
                return error(format!(
                    "strategy.risk.allow_entry_in direction {name}={value} is not supported"
                ));
            };
            let raw = direction.to_string();
            if normalize_allowed_entry_direction(&raw).is_none() {
                return error(format!(
                    "strategy.risk.allow_entry_in direction {raw:?} is not supported"
                ));
            }
        }
        "strategy.risk.max_drawdown" | "strategy.risk.max_intraday_loss" => {
            if positional.len() < 2 {
                return error(format!(
                    "{callee}(value, type[, alert_message]) requires at least two arguments"
                ));
            }
            if !is_positive_float_constant(positional[0]) {
                return error(format!(
                    "{callee} value {:?} must be a positive constant number",
                    constant_text(positional[0])
                ));
            }
            let amount_type = positional[1].to_string();
            if normalize_risk_amount_type(&amount_type).is_none() {
                return error(format!("{callee} type {amount_type:?} is not supported"));
            }
            if positional.len() > 3 || extra_named(&["alert_message"]) {
                return error(format!(
                    "{callee} supports only value, type, and optional alert_message"
                ));
            }
        }
        "strategy.risk.max_intraday_filled_orders" | "strategy.risk.max_cons_loss_days" => {
            if positional.is_empty() {
                return error(format!(
                    "{callee}(count[, alert_message]) requires at least one argument"
                ));
            }
            if !is_positive_int_constant(positional[0]) {
                return error(format!(
                    "{callee} count {:?} must be a positive constant integer",
                    constant_text(positional[0])
                ));
            }
            if positional.len() > 2 || extra_named(&["alert_message"]) {
                return error(format!(
                    "{callee} supports only count and optional alert_message"
                ));
            }
        }
        "strategy.risk.max_position_size" => {
            if positional.len() + named.len() != 1 {
                return error(
                    "strategy.risk.max_position_size(contracts) requires one argument".to_owned(),
                );
            }
            if !positional
                .first()
                .is_some_and(|expr| is_positive_float_constant(expr))
            {
                let text = positional
                    .first()
                    .map(|expr| constant_text(expr))
                    .or_else(|| named.first().map(|(name, value)| format!("{name}={value}")))
                    .unwrap_or_default();
                return error(format!(
                    "strategy.risk.max_position_size contracts {text:?} must be a positive constant number"
                ));
            }
        }
        _ => {}
    }
    None
}

/// Go trims and strips the `strategy.direction.`/`strategy.` prefixes before it
/// accepts an `allow_entry_in` direction.
fn normalize_allowed_entry_direction(raw: &str) -> Option<&'static str> {
    let normalized = raw.trim().to_ascii_lowercase();
    let normalized = normalized
        .strip_prefix("strategy.direction.")
        .or_else(|| normalized.strip_prefix("strategy."))
        .unwrap_or(normalized.as_str());
    match normalized {
        "" | "all" => Some("all"),
        "long" => Some("long"),
        "short" => Some("short"),
        _ => None,
    }
}

fn normalize_risk_amount_type(raw: &str) -> Option<&'static str> {
    let normalized = raw.trim().to_ascii_lowercase();
    let normalized = normalized
        .strip_prefix("strategy.")
        .unwrap_or(normalized.as_str());
    match normalized {
        "percent_of_equity" => Some("percent_of_equity"),
        "cash" => Some("cash"),
        _ => None,
    }
}

fn is_positive_float_constant(expression: &Expr) -> bool {
    match &expression.kind {
        ExprKind::Number { value } => value.parse::<f64>().is_ok_and(|parsed| parsed > 0.0),
        _ => false,
    }
}

/// Go reports the raw constant text, so a negative literal keeps its sign
/// instead of the parser's `Negate` debug form.
fn constant_text(expression: &Expr) -> String {
    match &expression.kind {
        ExprKind::Unary {
            op: UnaryOp::Negate,
            expression: inner,
        } => match &inner.kind {
            ExprKind::Number { value } => format!("-{value}"),
            _ => expression.to_string(),
        },
        _ => expression.to_string(),
    }
}

fn is_positive_int_constant(expression: &Expr) -> bool {
    match &expression.kind {
        ExprKind::Number { value } => value.parse::<i64>().is_ok_and(|parsed| parsed > 0),
        _ => false,
    }
}

fn is_named_argument(argument: &Expr) -> bool {
    matches!(
        &argument.kind,
        ExprKind::Binary {
            left,
            op: BinaryOp::Equal,
            ..
        } if matches!(&left.kind, ExprKind::Identifier { .. })
    )
}

/// Go's parser entry points require ids and directions before any metadata is
/// inspected, so the arity contract runs first.
fn order_arity_diagnostic(
    callee: &str,
    arguments: &[Expr],
    range: SourceRange,
) -> Option<Diagnostic> {
    let (message, violated) = match callee {
        "strategy.entry" => (
            "strategy.entry(id, direction, ...) requires at least two arguments",
            arguments.len() < 2,
        ),
        "strategy.order" => (
            "strategy.order(id, direction, ...) requires at least two arguments",
            arguments.len() < 2,
        ),
        "strategy.close" => (
            "strategy.close(id) requires an entry id",
            arguments.is_empty(),
        ),
        "strategy.exit" => (
            "strategy.exit(id, ...) requires an exit id",
            arguments.is_empty(),
        ),
        "strategy.cancel" => (
            "strategy.cancel(id) requires one order id",
            arguments.len() != 1,
        ),
        _ => return None,
    };
    violated.then(|| Diagnostic::error("PINE_COMPILE_ERROR", message, range.start_line))
}

/// `strategy.close_all` reads its first four entries positionally and rejects
/// every later positional argument, exactly like
/// `pkg/strategy/pine/parse_order_metadata.go::pineCloseAllMetadata`.
fn order_close_all_positional_diagnostic(
    arguments: &[Expr],
    has: &impl Fn(&str) -> bool,
    range: SourceRange,
) -> Option<Diagnostic> {
    for (index, argument) in arguments.iter().enumerate() {
        let named = !request_security_named_argument(argument).0.is_empty();
        if index >= 4 {
            if !named {
                return Some(Diagnostic::error(
                    "PINE_COMPILE_ERROR",
                    "strategy.close_all supports positional immediately, comment, alert_message, and disable_alert only",
                    range.start_line,
                ));
            }
            continue;
        }
        if named {
            continue;
        }
        let (keyword, field) = match index {
            0 => ("immediately", "immediately"),
            3 => ("disable_alert", "disable_alert"),
            _ => continue,
        };
        if has(keyword) {
            continue;
        }
        if !is_boolean_literal(argument) {
            return Some(Diagnostic::error(
                "PINE_COMPILE_ERROR",
                format!("strategy.close_all {field} must be true or false"),
                range.start_line,
            ));
        }
    }
    None
}

/// `disable_alert` must be a boolean literal everywhere, `immediately` is only
/// executable for close calls, and its value must still be a boolean literal.
fn order_boolean_metadata_diagnostic(
    callee: &str,
    arguments: &[Expr],
    has: &impl Fn(&str) -> bool,
    range: SourceRange,
) -> Option<Diagnostic> {
    if let Some(value) = named_argument_value(arguments, "disable_alert")
        && !is_boolean_literal(value)
    {
        return Some(Diagnostic::error(
            "PINE_COMPILE_ERROR",
            format!("{callee} disable_alert must be true or false"),
            range.start_line,
        ));
    }
    if has("immediately") && !matches!(callee, "strategy.close" | "strategy.close_all") {
        return Some(Diagnostic::error(
            "PINE_COMPILE_ERROR",
            format!("{callee} does not support immediately"),
            range.start_line,
        ));
    }
    if let Some(value) = named_argument_value(arguments, "immediately")
        && !is_boolean_literal(value)
    {
        return Some(Diagnostic::error(
            "PINE_COMPILE_ERROR",
            format!("{callee} immediately must be true or false"),
            range.start_line,
        ));
    }
    None
}

fn order_named_arguments(arguments: &[Expr]) -> Vec<(String, String)> {
    arguments
        .iter()
        .filter_map(|argument| {
            let (name, value) = request_security_named_argument(argument);
            (!name.is_empty()).then(|| (name.to_ascii_lowercase(), value))
        })
        .collect()
}

fn named_argument_value<'a>(arguments: &'a [Expr], name: &str) -> Option<&'a Expr> {
    arguments.iter().find_map(|argument| match &argument.kind {
        ExprKind::Binary {
            left,
            op: BinaryOp::Equal,
            right,
        } => match &left.kind {
            ExprKind::Identifier { name: candidate } if candidate.eq_ignore_ascii_case(name) => {
                Some(right.as_ref())
            }
            _ => None,
        },
        _ => None,
    })
}

fn is_boolean_literal(expression: &Expr) -> bool {
    matches!(expression.kind, ExprKind::Boolean { .. })
}

/// Named call arguments arrive from the expression parser as
/// `identifier = value` binary expressions, matching Go's textual
/// `name=value` merge-argument inspection.
fn request_security_named_argument(argument: &Expr) -> (&str, String) {
    match &argument.kind {
        ExprKind::Binary {
            left,
            op: BinaryOp::Equal,
            right,
        } => match &left.kind {
            ExprKind::Identifier { name } => (name.as_str(), right.to_string()),
            _ => ("", argument.to_string()),
        },
        _ => ("", argument.to_string()),
    }
}

/// Go accepts `syminfo.tickerid` plus the static
/// `ticker.heikinashi(...)`, `ticker.standard(...)` and `ticker.inherit(...)`
/// wrappers rooted at it. Anything else is a dynamic/external symbol.
fn is_supported_request_security_ticker(symbol: &str) -> bool {
    let trimmed = symbol.trim();
    if trimmed == "syminfo.tickerid" {
        return true;
    }
    let Some(open) = trimmed.find('(') else {
        return false;
    };
    if !trimmed.ends_with(')') {
        return false;
    }
    let name = trimmed[..open].trim().to_ascii_lowercase();
    let arguments = split_top_level_arguments(&trimmed[open + 1..trimmed.len() - 1]);
    match name.as_str() {
        "ticker.heikinashi" => arguments.len() == 1 && arguments[0].trim() == "syminfo.tickerid",
        "ticker.standard" => {
            arguments.len() <= 1
                && arguments
                    .iter()
                    .all(|item| item.trim() == "syminfo.tickerid")
        }
        "ticker.inherit" => {
            arguments.len() == 2
                && is_supported_request_security_ticker(&arguments[0])
                && arguments[1].trim() == "syminfo.tickerid"
        }
        _ => false,
    }
}

/// Split a call argument list on top-level commas so a nested call such as
/// `ticker.inherit(ticker.heikinashi(syminfo.tickerid), syminfo.tickerid)`
/// keeps its two arguments.
fn split_top_level_arguments(inner: &str) -> Vec<String> {
    if inner.trim().is_empty() {
        return Vec::new();
    }
    let mut arguments = Vec::new();
    let mut depth = 0usize;
    let mut current = String::new();
    for character in inner.chars() {
        match character {
            '(' | '[' | '{' => {
                depth += 1;
                current.push(character);
            }
            ')' | ']' | '}' => {
                depth = depth.saturating_sub(1);
                current.push(character);
            }
            ',' if depth == 0 => {
                arguments.push(current.trim().to_owned());
                current.clear();
            }
            _ => current.push(character),
        }
    }
    arguments.push(current.trim().to_owned());
    arguments
}

/// Parity: `pkg/strategy/pine/validate.go::requestSecurityExpressionHasSideEffect`.
///
/// Go rejects a `request.security` expression that reads trading state
/// (`strategy.position_size`), logs, draws objects, or mutates a collection.
/// The call denylist and the mutation suffixes below mirror that textual
/// contract, and member reads are rejected by their root namespace.
fn request_security_expression_has_side_effect(expression: &Expr) -> bool {
    match &expression.kind {
        ExprKind::Call { callee, arguments } => {
            let lower = callee.to_ascii_lowercase();
            if lower.starts_with("strategy.")
                || lower.starts_with("log.")
                || lower.starts_with("table.")
                || lower.starts_with("array.")
                || lower.starts_with("matrix.")
                || lower.starts_with("map.")
                || REQUEST_SECURITY_DENIED_CALLS.contains(&lower.as_str())
                || REQUEST_SECURITY_MUTATOR_SUFFIXES
                    .iter()
                    .any(|suffix| lower.ends_with(suffix))
            {
                return true;
            }
            arguments
                .iter()
                .any(request_security_expression_has_side_effect)
        }
        ExprKind::Binary { left, right, .. } => {
            request_security_expression_has_side_effect(left)
                || request_security_expression_has_side_effect(right)
        }
        ExprKind::Unary { expression, .. } => {
            request_security_expression_has_side_effect(expression)
        }
        ExprKind::Ternary {
            condition,
            when_true,
            when_false,
        } => {
            request_security_expression_has_side_effect(condition)
                || request_security_expression_has_side_effect(when_true)
                || request_security_expression_has_side_effect(when_false)
        }
        ExprKind::Index { object, index } => {
            request_security_expression_has_side_effect(object)
                || request_security_expression_has_side_effect(index)
        }
        ExprKind::Tuple { items } => items
            .iter()
            .any(request_security_expression_has_side_effect),
        ExprKind::Member { object, .. } => {
            request_security_expression_has_side_effect(object)
                || matches!(
                    &object.kind,
                    ExprKind::Identifier { name }
                        if REQUEST_SECURITY_DENIED_MEMBER_ROOTS
                            .iter()
                            .any(|root| name.eq_ignore_ascii_case(root))
                )
        }
        _ => false,
    }
}

/// Calls Go refuses inside a `request.security` expression.
const REQUEST_SECURITY_DENIED_CALLS: &[&str] = &[
    "alert",
    "alertcondition",
    "runtime.error",
    "line.new",
    "label.new",
    "box.new",
    "plot",
    "plotshape",
    "plotchar",
    "hline",
    "fill",
    "bgcolor",
    "barcolor",
];

/// Collection and drawing mutators Go refuses inside a `request.security`
/// expression, matched as member-call suffixes (`.push(`, `.set(`, ...).
const REQUEST_SECURITY_MUTATOR_SUFFIXES: &[&str] = &[
    ".push", ".pop", ".shift", ".unshift", ".insert", ".remove", ".clear", ".set", ".fill", ".put",
];

/// Namespaces whose member reads carry trading, logging or rendering state.
const REQUEST_SECURITY_DENIED_MEMBER_ROOTS: &[&str] = &["strategy", "log", "table"];
