use std::collections::BTreeMap;

use serde::Serialize;
use thiserror::Error;

use super::parser::{Expr, ExprKind, Program, SourceRange, Statement, StrategyDeclaration};

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StrategyMetadata {
    pub name: String,
    pub version: String,
    pub symbol: String,
    pub interval: String,
    pub default_qty_mode: String,
    pub default_qty_value: String,
    pub pyramiding: i64,
    pub initial_capital: Option<String>,
    pub commission_type: Option<String>,
    pub commission_value: Option<String>,
    pub slippage: Option<i64>,
    pub process_on_close: bool,
    pub allowed_entry_direction: Option<String>,
}

impl Default for StrategyMetadata {
    fn default() -> Self {
        Self {
            name: String::new(),
            version: String::new(),
            symbol: String::new(),
            interval: String::new(),
            default_qty_mode: "fixed".to_owned(),
            default_qty_value: "1".to_owned(),
            pyramiding: 1,
            initial_capital: None,
            commission_type: None,
            commission_value: None,
            slippage: None,
            process_on_close: false,
            allowed_entry_direction: None,
        }
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LoweredProgram {
    pub source_format: String,
    pub metadata: StrategyMetadata,
    pub hooks: Vec<LoweredHook>,
    pub functions: Vec<LoweredFunction>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LoweredHook {
    pub kind: String,
    pub range: SourceRange,
    pub statements: Vec<LoweredStatement>,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum LoweredStatement {
    Let {
        range: SourceRange,
        name: String,
        expression: Expr,
        mode: String,
    },
    If {
        range: SourceRange,
        condition: Expr,
        then_body: Vec<LoweredStatement>,
        else_body: Vec<LoweredStatement>,
    },
    For {
        range: SourceRange,
        variable: String,
        start: Expr,
        end: Expr,
        step: Option<Expr>,
        body: Vec<LoweredStatement>,
    },
    Action {
        range: SourceRange,
        call: String,
        arguments: Vec<Expr>,
    },
    Tuple {
        range: SourceRange,
        names: Vec<String>,
        expression: Expr,
        mode: String,
    },
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LoweredFunction {
    pub range: SourceRange,
    pub name: String,
    pub parameters: Vec<String>,
    pub body: Expr,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum LowerError {
    #[error("pine line {line}: {message}")]
    Invalid {
        code_name: &'static str,
        line: usize,
        message: String,
    },
}

impl LowerError {
    pub const fn line(&self) -> usize {
        match self {
            Self::Invalid { line, .. } => *line,
        }
    }
    pub const fn code(&self) -> &'static str {
        match self {
            Self::Invalid { code_name, .. } => code_name,
        }
    }
}

pub fn lower(program: &Program) -> Result<LoweredProgram, LowerError> {
    let strategy = program
        .strategy
        .as_ref()
        .ok_or_else(|| LowerError::Invalid {
            code_name: "PINE_STRATEGY_REQUIRED",
            line: 1,
            message: "a strategy(...) declaration is required".to_owned(),
        })?;
    let metadata = lower_metadata(strategy);
    let mut statements = Vec::new();
    let mut functions = Vec::new();
    for statement in &program.statements {
        match statement {
            Statement::Function {
                range,
                name,
                parameters,
                body,
            } => functions.push(LoweredFunction {
                range: *range,
                name: name.clone(),
                parameters: parameters.clone(),
                body: body.clone(),
            }),
            _ => statements.push(lower_statement(statement)?),
        }
    }
    Ok(LoweredProgram {
        source_format: program.source_format.clone(),
        metadata,
        hooks: vec![LoweredHook {
            kind: "on_kline_close".to_owned(),
            range: SourceRange {
                start_line: 1,
                start_column: 1,
                end_line: statements.last().map(statement_line).unwrap_or(1),
                end_column: 1,
            },
            statements,
        }],
        functions,
    })
}

fn lower_statement(statement: &Statement) -> Result<LoweredStatement, LowerError> {
    match statement {
        Statement::Assignment {
            range,
            name,
            expression,
            mode,
        } => Ok(LoweredStatement::Let {
            range: *range,
            name: name.clone(),
            expression: expression.clone(),
            mode: format!("{mode:?}").to_ascii_lowercase(),
        }),
        Statement::TupleAssignment {
            range,
            names,
            expression,
            mode,
        } => Ok(LoweredStatement::Tuple {
            range: *range,
            names: names.clone(),
            expression: expression.clone(),
            mode: format!("{mode:?}").to_ascii_lowercase(),
        }),
        Statement::If {
            range,
            condition,
            then_body,
            else_body,
        } => Ok(LoweredStatement::If {
            range: *range,
            condition: condition.clone(),
            then_body: lower_block(then_body)?,
            else_body: lower_block(else_body)?,
        }),
        Statement::For {
            range,
            variable,
            start,
            end,
            step,
            body,
        } => Ok(LoweredStatement::For {
            range: *range,
            variable: variable.clone(),
            start: start.clone(),
            end: end.clone(),
            step: step.clone(),
            body: lower_block(body)?,
        }),
        Statement::Call { range, expression } => {
            let ExprKind::Call { callee, arguments } = &expression.kind else {
                return Err(LowerError::Invalid {
                    code_name: "PINE_ACTION_REQUIRED",
                    line: range.start_line,
                    message: "top-level executable statement must be a call or assignment"
                        .to_owned(),
                });
            };
            Ok(LoweredStatement::Action {
                range: *range,
                call: callee.clone(),
                arguments: arguments.clone(),
            })
        }
        Statement::Function { range, .. } => Err(LowerError::Invalid {
            code_name: "PINE_FUNCTION_INVALID",
            line: range.start_line,
            message: "function declarations are lowered separately".to_owned(),
        }),
        Statement::Unsupported { range, .. } => Err(LowerError::Invalid {
            code_name: "PINE_STATEMENT_UNSUPPORTED",
            line: range.start_line,
            message: "statement is outside the executable Pine v6 subset".to_owned(),
        }),
    }
}

fn lower_block(statements: &[Statement]) -> Result<Vec<LoweredStatement>, LowerError> {
    statements.iter().map(lower_statement).collect()
}

fn lower_metadata(strategy: &StrategyDeclaration) -> StrategyMetadata {
    let mut metadata = StrategyMetadata {
        name: strategy.name.clone(),
        ..StrategyMetadata::default()
    };
    let mut named = BTreeMap::new();
    for argument in &strategy.arguments[1..] {
        if let Some(name) = &argument.name {
            named.insert(name.to_ascii_lowercase(), argument.value.clone());
        }
    }
    if let Some(value) = named.get("default_qty_type").and_then(simple_string) {
        metadata.default_qty_mode = match value.as_str() {
            "strategy.percent_of_equity" => "percent_of_equity",
            "strategy.cash" => "cash",
            _ => "fixed",
        }
        .to_owned();
    }
    if let Some(value) = named.get("default_qty_value").and_then(simple_string) {
        metadata.default_qty_value = value;
    }
    if let Some(value) = named.get("pyramiding").and_then(simple_i64) {
        metadata.pyramiding = value.max(1);
    }
    if let Some(value) = named.get("initial_capital").and_then(simple_string) {
        metadata.initial_capital = Some(value);
    }
    if let Some(value) = named.get("commission_type").and_then(simple_string) {
        metadata.commission_type = Some(value);
    }
    if let Some(value) = named.get("commission_value").and_then(simple_string) {
        metadata.commission_value = Some(value);
    }
    if let Some(value) = named.get("slippage").and_then(simple_i64) {
        metadata.slippage = Some(value);
    }
    if let Some(value) = named.get("process_orders_on_close").and_then(simple_bool) {
        metadata.process_on_close = value;
    }
    metadata
}

fn simple_string(expression: &Expr) -> Option<String> {
    match &expression.kind {
        ExprKind::String { value } => Some(value.clone()),
        ExprKind::Number { value } => Some(value.clone()),
        ExprKind::Identifier { name } => Some(name.clone()),
        ExprKind::Member { object, member } => Some(format!("{object}.{member}")),
        ExprKind::Boolean { value } => Some(value.to_string()),
        _ => None,
    }
}
fn simple_i64(expression: &Expr) -> Option<i64> {
    simple_string(expression)?.parse().ok()
}
fn simple_bool(expression: &Expr) -> Option<bool> {
    simple_string(expression)?.parse().ok()
}
fn statement_line(statement: &LoweredStatement) -> usize {
    match statement {
        LoweredStatement::Let { range, .. }
        | LoweredStatement::If { range, .. }
        | LoweredStatement::For { range, .. }
        | LoweredStatement::Action { range, .. }
        | LoweredStatement::Tuple { range, .. } => range.start_line,
    }
}
