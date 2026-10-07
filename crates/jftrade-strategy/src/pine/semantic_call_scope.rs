//! Validate calls expanded from UDFs and the lifetime of loop bindings.
use std::collections::{BTreeMap, BTreeSet};

use super::parser::{Expr, ExprKind, Program, Statement};
use super::semantic::Diagnostic;

pub(super) fn diagnostics(program: &Program) -> Vec<Diagnostic> {
    let functions = program
        .statements
        .iter()
        .filter_map(|statement| match statement {
            Statement::Function {
                name,
                parameters,
                body,
                ..
            } => Some((name.as_str(), (parameters.len(), body))),
            _ => None,
        })
        .collect();
    let mut scope = CallScope {
        functions,
        loops: Vec::new(),
        calls: Vec::new(),
        checked: BTreeSet::new(),
        errors: Vec::new(),
    };
    scope.statements(&program.statements);
    scope.errors
}

struct CallScope<'a> {
    functions: BTreeMap<&'a str, (usize, &'a Expr)>,
    loops: Vec<&'a str>,
    calls: Vec<&'a str>,
    checked: BTreeSet<(usize, &'a str)>,
    errors: Vec<Diagnostic>,
}

impl<'a> CallScope<'a> {
    fn statements(&mut self, statements: &'a [Statement]) {
        for statement in statements {
            match statement {
                Statement::Assignment {
                    name,
                    expression,
                    range,
                    ..
                } => {
                    self.assignment(name, range.start_line);
                    self.expression(expression, expression.range.start_line);
                }
                Statement::TupleAssignment {
                    names,
                    expression,
                    range,
                    ..
                } => {
                    for name in names {
                        self.assignment(name, range.start_line);
                    }
                    self.expression(expression, expression.range.start_line);
                }
                Statement::Call { expression, .. } => {
                    self.expression(expression, expression.range.start_line);
                }
                Statement::If {
                    condition,
                    then_body,
                    else_body,
                    ..
                } => {
                    self.expression(condition, condition.range.start_line);
                    self.statements(then_body);
                    self.statements(else_body);
                }
                Statement::For {
                    variable,
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
                        self.expression(expression, expression.range.start_line);
                    }
                    self.loops.push(variable);
                    self.statements(body);
                    self.loops.pop();
                }
                Statement::Function { .. } | Statement::Unsupported { .. } => {}
            }
        }
    }

    fn assignment(&mut self, name: &str, line: usize) {
        if self.loops.contains(&name) {
            self.errors.push(Diagnostic::error(
                "PINE_LOOP_VARIABLE_READONLY",
                format!("loop variable {name:?} is read-only"),
                line,
            ));
        }
    }

    fn expression(&mut self, expression: &'a Expr, line: usize) {
        match &expression.kind {
            ExprKind::Call { callee, arguments } => {
                for argument in arguments {
                    self.expression(argument, line);
                }
                self.call(callee, arguments.len(), line);
            }
            ExprKind::Unary { expression, .. }
            | ExprKind::Member {
                object: expression, ..
            } => {
                self.expression(expression, line);
            }
            ExprKind::Binary { left, right, .. }
            | ExprKind::Index {
                object: left,
                index: right,
            } => {
                self.expression(left, line);
                self.expression(right, line);
            }
            ExprKind::Ternary {
                condition,
                when_true,
                when_false,
            } => {
                for expression in [condition, when_true, when_false] {
                    self.expression(expression, line);
                }
            }
            ExprKind::Tuple { items } => {
                for item in items {
                    self.expression(item, line);
                }
            }
            _ => {}
        }
    }

    fn call(&mut self, callee: &'a str, count: usize, line: usize) {
        let Some(&(expected, body)) = self.functions.get(callee) else {
            return;
        };
        if expected != count {
            self.errors.push(Diagnostic::error(
                "PINE_UDF_SIGNATURE_UNSUPPORTED",
                format!("{callee} expects {expected} arguments, got {count}"),
                line,
            ));
            return;
        }
        if self.calls.contains(&callee) {
            self.errors.push(Diagnostic::error(
                "PINE_UDF_RECURSIVE_UNSUPPORTED",
                format!("recursive user-defined function {callee:?} is not supported"),
                line,
            ));
            return;
        }
        // A branching UDF graph can revisit the same body exponentially.
        // Validation depends on the signature and invocation line, not values.
        if !self.checked.insert((line, callee)) {
            return;
        }
        self.calls.push(callee);
        self.expression(body, line);
        self.calls.pop();
    }
}
