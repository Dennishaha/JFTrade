//! Resolve immutable chart-level numeric defaults without admitting dynamic periods.
use std::collections::{BTreeMap, BTreeSet};

use super::{Expr, ExprKind, LoweredProgram, LoweredStatement, argument_text};

pub(super) struct PeriodAliases {
    eligible: BTreeSet<String>,
    values: BTreeMap<String, String>,
}

impl PeriodAliases {
    pub(super) fn new(program: &LoweredProgram) -> Self {
        let mut writes = BTreeMap::new();
        for hook in &program.hooks {
            count_writes(&hook.statements, &mut writes);
        }
        // A UDF parameter with the same name must not consume a global default.
        for function in &program.functions {
            for parameter in &function.parameters {
                *writes.entry(parameter.clone()).or_default() += 1;
            }
        }
        let eligible = program
            .hooks
            .iter()
            .flat_map(|hook| &hook.statements)
            .filter_map(|statement| match statement {
                LoweredStatement::Let { name, mode, .. }
                    if mode != "reassign" && writes.get(name) == Some(&1) =>
                {
                    Some(name.clone())
                }
                _ => None,
            })
            .collect();
        Self {
            eligible,
            values: BTreeMap::new(),
        }
    }

    pub(super) fn record(&mut self, name: &str, expression: &Expr) {
        if !self.eligible.contains(name) {
            return;
        }
        let value = match &expression.kind {
            ExprKind::Number { value } => Some(value.clone()),
            ExprKind::Identifier { name } => self.values.get(name).cloned(),
            ExprKind::Call { callee, arguments } if callee.eq_ignore_ascii_case("input.int") => {
                match arguments.first().map(|argument| &argument.kind) {
                    Some(ExprKind::Number { value }) => Some(value.clone()),
                    _ => None,
                }
            }
            _ => None,
        };
        if let Some(value) = value {
            self.values.insert(name.to_owned(), value);
        }
    }

    pub(super) fn argument(&self, expression: Option<&Expr>) -> Option<String> {
        if let Some(Expr {
            kind: ExprKind::Identifier { name },
            ..
        }) = expression
            && let Some(value) = self.values.get(name)
        {
            return Some(value.clone());
        }
        argument_text(expression)
    }
}

fn count_writes(statements: &[LoweredStatement], writes: &mut BTreeMap<String, usize>) {
    for statement in statements {
        match statement {
            LoweredStatement::Let { name, .. } => *writes.entry(name.clone()).or_default() += 1,
            LoweredStatement::Tuple { names, .. } => {
                for name in names {
                    *writes.entry(name.clone()).or_default() += 1;
                }
            }
            LoweredStatement::If {
                then_body,
                else_body,
                ..
            } => {
                count_writes(then_body, writes);
                count_writes(else_body, writes);
            }
            LoweredStatement::For { variable, body, .. } => {
                *writes.entry(variable.clone()).or_default() += 1;
                count_writes(body, writes);
            }
            LoweredStatement::Action { .. } => {}
        }
    }
}
