use std::collections::BTreeMap;

use serde::Serialize;
use thiserror::Error;

use super::lower::{LoweredProgram, LoweredStatement};
use super::parser::{Expr, ExprKind};

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct IndicatorRequirement {
    pub alias: String,
    pub kind: String,
    pub key: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Requirements {
    pub indicators: Vec<IndicatorRequirement>,
    pub requires_position: bool,
    pub requires_total_account_value: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum PlannerError {
    #[error("pine line {line}: {message}")]
    Invalid { line: usize, message: String },
}

impl PlannerError {
    pub const fn line(&self) -> usize {
        match self {
            Self::Invalid { line, .. } => *line,
        }
    }
}

pub fn plan_requirements(program: &LoweredProgram) -> Result<Requirements, PlannerError> {
    let mut context = PlannerContext {
        indicators: BTreeMap::new(),
        result: Requirements::default(),
    };
    for hook in &program.hooks {
        for statement in &hook.statements {
            context.visit_statement(statement)?;
        }
    }
    context.result.indicators = context.indicators.into_values().collect();
    Ok(context.result)
}

struct PlannerContext {
    indicators: BTreeMap<String, IndicatorRequirement>,
    result: Requirements,
}

impl PlannerContext {
    fn visit_statement(&mut self, statement: &LoweredStatement) -> Result<(), PlannerError> {
        match statement {
            LoweredStatement::Let {
                name, expression, ..
            } => {
                self.visit_expr(expression)?;
                if let ExprKind::Call { callee, arguments } = &expression.kind
                    && let Some(requirement) =
                        requirement_for_call(callee, arguments, name, expression.range.start_line)?
                {
                    self.indicators.insert(requirement.key.clone(), requirement);
                }
            }
            LoweredStatement::Tuple {
                expression, names, ..
            } => {
                self.visit_expr(expression)?;
                if let ExprKind::Call { callee, arguments } = &expression.kind
                    && let Some(requirement) = requirement_for_call(
                        callee,
                        arguments,
                        names.first().map(String::as_str).unwrap_or_default(),
                        expression.range.start_line,
                    )?
                {
                    self.indicators.insert(requirement.key.clone(), requirement);
                }
            }
            LoweredStatement::Action {
                call,
                arguments,
                range,
            } => {
                self.visit_action(call, arguments, range.start_line)?;
            }
            LoweredStatement::If {
                condition,
                then_body,
                else_body,
                ..
            } => {
                self.visit_expr(condition)?;
                for item in then_body {
                    self.visit_statement(item)?;
                }
                for item in else_body {
                    self.visit_statement(item)?;
                }
            }
            LoweredStatement::For {
                start,
                end,
                step,
                body,
                ..
            } => {
                self.visit_expr(start)?;
                self.visit_expr(end)?;
                if let Some(step) = step {
                    self.visit_expr(step)?;
                }
                for item in body {
                    self.visit_statement(item)?;
                }
            }
        }
        Ok(())
    }

    fn visit_action(
        &mut self,
        call: &str,
        arguments: &[Expr],
        line: usize,
    ) -> Result<(), PlannerError> {
        for argument in arguments {
            self.visit_expr(argument)?;
        }
        match call {
            "strategy.entry" | "strategy.order" | "strategy.close" | "strategy.close_all"
            | "strategy.exit" => {
                self.result.requires_position = true;
                if arguments.iter().any(expr_contains_equity) {
                    self.result.requires_total_account_value = true;
                }
            }
            _ => {}
        }
        if let Some(requirement) = requirement_for_call(call, arguments, "", line)? {
            self.indicators.insert(requirement.key.clone(), requirement);
        }
        Ok(())
    }

    fn visit_expr(&mut self, expression: &Expr) -> Result<(), PlannerError> {
        match &expression.kind {
            ExprKind::Unary { expression, .. } => self.visit_expr(expression)?,
            ExprKind::Binary { left, right, .. } => {
                self.visit_expr(left)?;
                self.visit_expr(right)?;
            }
            ExprKind::Ternary {
                condition,
                when_true,
                when_false,
            } => {
                self.visit_expr(condition)?;
                self.visit_expr(when_true)?;
                self.visit_expr(when_false)?;
            }
            ExprKind::Member { object, .. } => self.visit_expr(object)?,
            ExprKind::Index { object, index } => {
                self.visit_expr(object)?;
                self.visit_expr(index)?;
            }
            ExprKind::Tuple { items } => {
                for item in items {
                    self.visit_expr(item)?;
                }
            }
            ExprKind::Call { callee, arguments } => {
                for argument in arguments {
                    self.visit_expr(argument)?;
                }
                if expr_contains_equity(expression) {
                    self.result.requires_total_account_value = true;
                }
                if let Some(requirement) =
                    requirement_for_call(callee, arguments, "", expression.range.start_line)?
                {
                    self.indicators.insert(requirement.key.clone(), requirement);
                }
            }
            ExprKind::Identifier { .. }
            | ExprKind::Number { .. }
            | ExprKind::String { .. }
            | ExprKind::Boolean { .. }
            | ExprKind::Null => {}
        }
        Ok(())
    }
}

fn requirement_for_call(
    callee: &str,
    arguments: &[Expr],
    alias: &str,
    line: usize,
) -> Result<Option<IndicatorRequirement>, PlannerError> {
    let lower = callee.to_ascii_lowercase();
    if lower == "ta.crossover" || lower == "ta.crossunder" || lower == "ta.cross" {
        return Ok(None);
    }
    let mut key_parts = Vec::new();
    let kind;
    match lower.as_str() {
        "ta.ema" | "ta.sma" | "ta.rma" | "ta.wma" | "ta.hma" | "ta.vwma" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} requires a length")))?;
            let label = lower
                .strip_prefix("ta.")
                .unwrap_or_default()
                .to_ascii_uppercase();
            kind = "ma";
            key_parts.extend([label, length]);
            if source != "close" {
                key_parts.push(source);
            }
        }
        "ta.rsi" | "ta.cci" | "ta.mom" | "ta.roc" | "ta.range" | "ta.mode" | "ta.sum"
        | "ta.rising" | "ta.falling" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .or_else(|| argument_text(arguments.first()))
                .ok_or_else(|| invalid(line, format!("{callee} requires a length")))?;
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            key_parts.extend([source.clone(), length]);
            if source == "close" {
                key_parts.remove(0);
            }
        }
        "ta.macd" => {
            kind = "macd";
            for argument in arguments {
                key_parts
                    .push(argument_text(Some(argument)).unwrap_or_else(|| argument.to_string()));
            }
        }
        "ta.atr" | "ta.stdev" | "ta.variance" | "ta.wpr" | "ta.vwap" | "ta.mfi" | "ta.obv" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            for argument in arguments {
                key_parts
                    .push(argument_text(Some(argument)).unwrap_or_else(|| argument.to_string()));
            }
        }
        "ta.highest" | "ta.lowest" | "ta.change" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            for argument in arguments {
                key_parts
                    .push(argument_text(Some(argument)).unwrap_or_else(|| argument.to_string()));
            }
        }
        "request.security" => {
            if arguments.len() < 3 {
                return Err(invalid(
                    line,
                    "request.security requires symbol, timeframe, and expression",
                ));
            }
            kind = "security";
            let symbol = argument_text(arguments.first()).unwrap_or_default();
            let timeframe = argument_text(arguments.get(1)).unwrap_or_default();
            let expression = argument_text(arguments.get(2)).unwrap_or_default();
            key_parts.extend([symbol, timeframe, expression]);
        }
        _ => return Ok(None),
    }
    Ok(Some(IndicatorRequirement {
        alias: alias.to_owned(),
        kind: kind.to_owned(),
        key: format!("{}:{}", kind, key_parts.join(":")),
    }))
}

fn argument_text(expression: Option<&Expr>) -> Option<String> {
    expression.map(ToString::to_string)
}
fn expr_contains_equity(expression: &Expr) -> bool {
    match &expression.kind {
        ExprKind::Identifier { name } => name == "strategy.equity",
        ExprKind::Member { object, member } => member == "equity" || expr_contains_equity(object),
        ExprKind::Unary { expression, .. } => expr_contains_equity(expression),
        ExprKind::Binary { left, right, .. } => {
            expr_contains_equity(left) || expr_contains_equity(right)
        }
        ExprKind::Ternary {
            condition,
            when_true,
            when_false,
        } => {
            expr_contains_equity(condition)
                || expr_contains_equity(when_true)
                || expr_contains_equity(when_false)
        }
        ExprKind::Index { object, index } => {
            expr_contains_equity(object) || expr_contains_equity(index)
        }
        ExprKind::Call { arguments, .. } | ExprKind::Tuple { items: arguments } => {
            arguments.iter().any(expr_contains_equity)
        }
        ExprKind::Number { .. }
        | ExprKind::String { .. }
        | ExprKind::Boolean { .. }
        | ExprKind::Null => false,
    }
}
fn invalid(line: usize, message: impl Into<String>) -> PlannerError {
    PlannerError::Invalid {
        line,
        message: message.into(),
    }
}
