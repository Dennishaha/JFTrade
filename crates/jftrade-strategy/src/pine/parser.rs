use std::fmt;

use serde::Serialize;
use thiserror::Error;

use super::lexer::{LexedLine, Token, TokenKind, decode_string, lex};

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum BinaryOp {
    Or,
    And,
    Equal,
    NotEqual,
    Less,
    LessEqual,
    Greater,
    GreaterEqual,
    Add,
    Subtract,
    Multiply,
    Divide,
    Remainder,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum UnaryOp {
    Negate,
    Not,
    Positive,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum ExprKind {
    Number {
        value: String,
    },
    String {
        value: String,
    },
    Boolean {
        value: bool,
    },
    Null,
    Identifier {
        name: String,
    },
    Unary {
        op: UnaryOp,
        expression: Box<Expr>,
    },
    Binary {
        left: Box<Expr>,
        op: BinaryOp,
        right: Box<Expr>,
    },
    Ternary {
        condition: Box<Expr>,
        when_true: Box<Expr>,
        when_false: Box<Expr>,
    },
    Member {
        object: Box<Expr>,
        member: String,
    },
    Index {
        object: Box<Expr>,
        index: Box<Expr>,
    },
    Call {
        callee: String,
        arguments: Vec<Expr>,
    },
    Tuple {
        items: Vec<Expr>,
    },
}

#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Expr {
    pub range: SourceRange,
    #[serde(flatten)]
    pub kind: ExprKind,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceRange {
    pub start_line: usize,
    pub start_column: usize,
    pub end_line: usize,
    pub end_column: usize,
}

impl SourceRange {
    const fn line(line: usize, start: usize, end: usize) -> Self {
        Self {
            start_line: line,
            start_column: start,
            end_line: line,
            end_column: end,
        }
    }
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Argument {
    pub name: Option<String>,
    pub value: Expr,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StrategyDeclaration {
    pub name: String,
    pub arguments: Vec<Argument>,
    pub range: SourceRange,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum Statement {
    Assignment {
        range: SourceRange,
        name: String,
        expression: Expr,
        mode: AssignmentMode,
    },
    TupleAssignment {
        range: SourceRange,
        names: Vec<String>,
        expression: Expr,
        mode: AssignmentMode,
    },
    If {
        range: SourceRange,
        condition: Expr,
        then_body: Vec<Statement>,
        else_body: Vec<Statement>,
    },
    For {
        range: SourceRange,
        variable: String,
        start: Expr,
        end: Expr,
        step: Option<Expr>,
        body: Vec<Statement>,
    },
    Call {
        range: SourceRange,
        expression: Expr,
    },
    Function {
        range: SourceRange,
        name: String,
        parameters: Vec<String>,
        body: Expr,
    },
    Unsupported {
        range: SourceRange,
        text: String,
    },
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum AssignmentMode {
    Let,
    Var,
    Reassign,
}

#[derive(Clone, Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AstNode {
    pub source_format: String,
    pub version: u8,
    pub strategy: Option<StrategyDeclaration>,
    pub statements: Vec<Statement>,
}

pub type Program = AstNode;

#[derive(Clone, Debug, Eq, PartialEq, Error)]
#[error("pine line {line}, column {column}: {message}")]
pub struct ParseError {
    pub code_name: &'static str,
    pub message: String,
    pub line: usize,
    pub column: usize,
}

impl ParseError {
    pub fn code(&self) -> &'static str {
        self.code_name
    }
    pub const fn line(&self) -> usize {
        self.line
    }
}

pub fn parse(source: &str) -> Result<Program, ParseError> {
    let lines = lex(source).map_err(|error| ParseError {
        code_name: "PINE_LEX_ERROR",
        message: error.to_string(),
        line: match error {
            super::lexer::LexError::UnterminatedString { line, .. }
            | super::lexer::LexError::InvalidCharacter { line, .. }
            | super::lexer::LexError::InvalidNumber { line, .. } => line,
        },
        column: 1,
    })?;
    let mut parser = Parser::new(lines);
    parser.parse_program()
}

struct Parser {
    lines: Vec<LexedLine>,
    cursor: usize,
}

impl Parser {
    fn new(lines: Vec<LexedLine>) -> Self {
        Self { lines, cursor: 0 }
    }

    fn parse_program(&mut self) -> Result<Program, ParseError> {
        let mut version = None;
        let mut strategy = None;
        let mut statements = Vec::new();
        while self.cursor < self.lines.len() {
            if let Some(value) = parse_version_directive(&self.lines[self.cursor].text) {
                version = Some(value);
                self.cursor += 1;
                continue;
            }
            if self.is_ignorable() {
                self.cursor += 1;
                continue;
            }
            let line = self.lines[self.cursor].clone();
            if line.indent != 0 {
                return Err(self.error(
                    "PINE_INDENT_UNEXPECTED",
                    "top-level statement must not be indented",
                    &line,
                ));
            }
            if line.text.starts_with("strategy(") {
                if strategy.is_some() {
                    return Err(self.error(
                        "PINE_STRATEGY_DUPLICATE",
                        "strategy declaration may appear only once",
                        &line,
                    ));
                }
                strategy = Some(parse_strategy(&line)?);
                self.cursor += 1;
                continue;
            }
            let (statement, next) = self.parse_statement(0)?;
            self.cursor = next;
            statements.push(statement);
        }
        let version = version.ok_or_else(|| ParseError {
            code_name: "PINE_VERSION_REQUIRED",
            message: "//@version=6 is required".to_owned(),
            line: 1,
            column: 1,
        })?;
        Ok(Program {
            source_format: "pine-v6".to_owned(),
            version,
            strategy,
            statements,
        })
    }

    fn parse_statement(&mut self, parent_indent: usize) -> Result<(Statement, usize), ParseError> {
        let line = self.lines[self.cursor].clone();
        if line.indent < parent_indent {
            return Err(self.error(
                "PINE_BLOCK_MISSING",
                "expected an indented statement",
                &line,
            ));
        }
        if line.indent > parent_indent {
            return Err(self.error("PINE_INDENT_UNEXPECTED", "unexpected indentation", &line));
        }
        let number = line.number;
        let text = line.text.trim();
        if text.starts_with("if ") {
            let condition =
                expression_from_text(&line, text.strip_prefix("if ").unwrap_or_default())?;
            self.cursor += 1;
            let then_body = self.parse_children(parent_indent, number)?;
            let mut else_body = Vec::new();
            if self.peek_text() == Some("else") || self.peek_text() == Some("else:") {
                self.cursor += 1;
                else_body = self.parse_children(parent_indent, number)?;
            }
            return Ok((
                Statement::If {
                    range: range_for_line(&line),
                    condition,
                    then_body,
                    else_body,
                },
                self.cursor,
            ));
        }
        if text.starts_with("for ") {
            let (variable, start, end, step) = parse_for_header(&line, text)?;
            self.cursor += 1;
            let body = self.parse_children(parent_indent, number)?;
            return Ok((
                Statement::For {
                    range: range_for_line(&line),
                    variable,
                    start,
                    end,
                    step,
                    body,
                },
                self.cursor,
            ));
        }
        if let Some((names, operator, rhs)) = split_assignment(&line) {
            let mode = match operator {
                "var" => AssignmentMode::Var,
                ":=" => AssignmentMode::Reassign,
                _ => AssignmentMode::Let,
            };
            let expression = expression_from_text(&line, rhs)?;
            self.cursor += 1;
            if names.len() > 1 {
                return Ok((
                    Statement::TupleAssignment {
                        range: range_for_line(&line),
                        names,
                        expression,
                        mode,
                    },
                    self.cursor,
                ));
            }
            return Ok((
                Statement::Assignment {
                    range: range_for_line(&line),
                    name: names.into_iter().next().unwrap_or_default(),
                    expression,
                    mode,
                },
                self.cursor,
            ));
        }
        if let Some((name, parameters, body)) = parse_function_header(&line) {
            let body = expression_from_text(&line, body)?;
            self.cursor += 1;
            return Ok((
                Statement::Function {
                    range: range_for_line(&line),
                    name,
                    parameters,
                    body,
                },
                self.cursor,
            ));
        }
        if let Ok(expression) = expression_from_text(&line, text) {
            self.cursor += 1;
            return Ok((
                Statement::Call {
                    range: range_for_line(&line),
                    expression,
                },
                self.cursor,
            ));
        }
        self.cursor += 1;
        Ok((
            Statement::Unsupported {
                range: range_for_line(&line),
                text: text.to_owned(),
            },
            self.cursor,
        ))
    }

    fn parse_children(
        &mut self,
        parent_indent: usize,
        line: usize,
    ) -> Result<Vec<Statement>, ParseError> {
        let child_indent = self.next_code_indent();
        let Some(child_indent) = child_indent else {
            return Err(ParseError {
                code_name: "PINE_BLOCK_MISSING",
                message: "if/for requires an indented body".to_owned(),
                line,
                column: 1,
            });
        };
        if child_indent <= parent_indent {
            return Err(ParseError {
                code_name: "PINE_BLOCK_MISSING",
                message: "if/for requires an indented body".to_owned(),
                line,
                column: 1,
            });
        }
        let mut statements = Vec::new();
        while self.cursor < self.lines.len() {
            if self.is_ignorable() {
                self.cursor += 1;
                continue;
            }
            let indent = self.lines[self.cursor].indent;
            if indent < child_indent {
                break;
            }
            if indent > child_indent {
                return Err(self.error(
                    "PINE_INDENT_UNEXPECTED",
                    "indentation is deeper than the enclosing block",
                    &self.lines[self.cursor].clone(),
                ));
            }
            let (statement, next) = self.parse_statement(child_indent)?;
            self.cursor = next;
            statements.push(statement);
        }
        Ok(statements)
    }

    fn next_code_indent(&self) -> Option<usize> {
        self.lines[self.cursor..]
            .iter()
            .find(|line| !line.text.is_empty() && !line.text.starts_with("//"))
            .map(|line| line.indent)
    }

    fn peek_text(&self) -> Option<&str> {
        self.lines[self.cursor..]
            .iter()
            .find(|line| !line.text.is_empty() && !line.text.starts_with("//"))
            .map(|line| line.text.as_str())
    }

    fn is_ignorable(&self) -> bool {
        self.lines[self.cursor].text.is_empty() || self.lines[self.cursor].text.starts_with("//")
    }

    fn error(
        &self,
        code: &'static str,
        message: impl Into<String>,
        line: &LexedLine,
    ) -> ParseError {
        ParseError {
            code_name: code,
            message: message.into(),
            line: line.number,
            column: line.indent + 1,
        }
    }
}

fn parse_version_directive(text: &str) -> Option<u8> {
    let value = text.strip_prefix("//@version=")?.trim();
    value.parse().ok()
}

fn parse_strategy(line: &LexedLine) -> Result<StrategyDeclaration, ParseError> {
    let expression = expression_from_text(line, &line.text)?;
    let ExprKind::Call { callee, arguments } = expression.kind else {
        return Err(ParseError {
            code_name: "PINE_STRATEGY_REQUIRED",
            message: "strategy declaration must be a call".to_owned(),
            line: line.number,
            column: 1,
        });
    };
    if callee != "strategy" {
        return Err(ParseError {
            code_name: "PINE_STRATEGY_REQUIRED",
            message: "strategy declaration must call strategy(...)".to_owned(),
            line: line.number,
            column: 1,
        });
    }
    let name = arguments
        .first()
        .and_then(|argument| match &argument.kind {
            ExprKind::String { value } => Some(value.clone()),
            _ => None,
        })
        .ok_or_else(|| ParseError {
            code_name: "PINE_STRATEGY_NAME_REQUIRED",
            message: "strategy() requires a string title".to_owned(),
            line: line.number,
            column: 1,
        })?;
    let arguments = arguments
        .into_iter()
        .map(|value| {
            let (name, value) = match value {
                Expr {
                    range,
                    kind:
                        ExprKind::Binary {
                            left,
                            op: BinaryOp::Equal,
                            right,
                        },
                } => match left.kind {
                    ExprKind::Identifier { name } => (Some(name), *right),
                    _ => (
                        None,
                        Expr {
                            range,
                            kind: ExprKind::Binary {
                                left,
                                op: BinaryOp::Equal,
                                right,
                            },
                        },
                    ),
                },
                other => (None, other),
            };
            Argument { name, value }
        })
        .collect();
    Ok(StrategyDeclaration {
        name,
        arguments,
        range: range_for_line(line),
    })
}

fn parse_for_header(
    line: &LexedLine,
    text: &str,
) -> Result<(String, Expr, Expr, Option<Expr>), ParseError> {
    let tokens = &line.tokens;
    let mut index = 0;
    if tokens.get(index).map(|token| token.lexeme.as_str()) != Some("for") {
        return Err(ParseError {
            code_name: "PINE_FOR_INVALID",
            message: "invalid for loop".to_owned(),
            line: line.number,
            column: 1,
        });
    }
    index += 1;
    let variable = tokens
        .get(index)
        .filter(|token| token.kind == TokenKind::Identifier)
        .map(|token| token.lexeme.clone())
        .ok_or_else(|| ParseError {
            code_name: "PINE_FOR_INVALID",
            message: "for loop variable is required".to_owned(),
            line: line.number,
            column: 1,
        })?;
    index += 1;
    if tokens.get(index).map(|token| token.lexeme.as_str()) != Some("=") {
        return Err(ParseError {
            code_name: "PINE_FOR_INVALID",
            message: "for loop requires '='".to_owned(),
            line: line.number,
            column: 1,
        });
    }
    index += 1;
    let to_index = tokens[index..]
        .iter()
        .position(|token| token.lexeme == "to")
        .map(|offset| index + offset)
        .ok_or_else(|| ParseError {
            code_name: "PINE_FOR_INVALID",
            message: "for loop requires 'to'".to_owned(),
            line: line.number,
            column: 1,
        })?;
    let by_index = tokens[to_index + 1..]
        .iter()
        .position(|token| token.lexeme == "by")
        .map(|offset| to_index + 1 + offset);
    let start = expression_from_tokens(line, &tokens[index..to_index])?;
    let end = expression_from_tokens(
        line,
        &tokens[to_index + 1..by_index.unwrap_or(tokens.len())],
    )?;
    let step = by_index
        .map(|position| expression_from_tokens(line, &tokens[position + 1..]))
        .transpose()?;
    let _ = text;
    Ok((variable, start, end, step))
}

fn parse_function_header(line: &LexedLine) -> Option<(String, Vec<String>, &str)> {
    let arrow = line.text.find("=>")?;
    let header = line.text[..arrow].trim();
    let open = header.find('(')?;
    let close = header.rfind(')')?;
    let name = header[..open].trim();
    if name.is_empty()
        || !name
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || character == '_')
    {
        return None;
    }
    let parameters = split_raw_arguments(&header[open + 1..close])
        .into_iter()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .collect();
    Some((name.to_owned(), parameters, line.text[arrow + 2..].trim()))
}

fn split_assignment(line: &LexedLine) -> Option<(Vec<String>, &str, &str)> {
    let tokens = &line.tokens;
    let mut depth = 0usize;
    let mut operator = None;
    for (index, token) in tokens.iter().enumerate() {
        match token.lexeme.as_str() {
            "(" | "[" | "{" => depth += 1,
            ")" | "]" | "}" => depth = depth.saturating_sub(1),
            "=" | ":=" if depth == 0 => {
                operator = Some((index, token.lexeme.as_str()));
                break;
            }
            _ => {}
        }
    }
    let (index, operator) = operator?;
    let lhs = &tokens[..index];
    let rhs_start = tokens[index].end_column.saturating_sub(1);
    let rhs = line.text.get(rhs_start..)?.trim();
    if lhs.first().is_some_and(|token| token.lexeme == "var") {
        let name = lhs.get(1)?.lexeme.clone();
        return Some((vec![name], "var", rhs));
    }
    if lhs
        .first()
        .is_some_and(|token| token.lexeme == "const" || token.lexeme == "varip")
    {
        let name = lhs.get(1)?.lexeme.clone();
        return Some((vec![name], operator, rhs));
    }
    if lhs.first().is_some_and(|token| token.lexeme == "[")
        && lhs.last().is_some_and(|token| token.lexeme == "]")
    {
        let names = lhs[1..lhs.len() - 1]
            .iter()
            .filter(|token| token.kind == TokenKind::Identifier)
            .map(|token| token.lexeme.clone())
            .collect::<Vec<_>>();
        if names.len() > 1 {
            return Some((names, operator, rhs));
        }
    }
    let name = lhs
        .first()
        .filter(|token| token.kind == TokenKind::Identifier)?
        .lexeme
        .clone();
    if lhs.len() == 1 {
        return Some((vec![name], operator, rhs));
    }
    None
}

fn expression_from_text(line: &LexedLine, text: &str) -> Result<Expr, ParseError> {
    let token_start = line.text.find(text).unwrap_or(0);
    let tokens = line
        .tokens
        .iter()
        .filter(|token| token.column > line.indent + token_start)
        .cloned()
        .collect::<Vec<_>>();
    expression_from_tokens(line, &tokens)
}

fn expression_from_tokens(line: &LexedLine, tokens: &[Token]) -> Result<Expr, ParseError> {
    if tokens.is_empty() {
        return Err(ParseError {
            code_name: "PINE_EXPRESSION_REQUIRED",
            message: "expression is required".to_owned(),
            line: line.number,
            column: line.indent + 1,
        });
    }
    let mut parser = ExpressionParser {
        line: line.number,
        tokens,
        cursor: 0,
    };
    let expression = parser.parse_expression(0)?;
    if parser.cursor != tokens.len() {
        let token = &tokens[parser.cursor];
        return Err(ParseError {
            code_name: "PINE_EXPRESSION_INVALID",
            message: format!("unexpected token {:?}", token.lexeme),
            line: line.number,
            column: token.column,
        });
    }
    Ok(expression)
}

struct ExpressionParser<'a> {
    line: usize,
    tokens: &'a [Token],
    cursor: usize,
}

impl ExpressionParser<'_> {
    fn parse_expression(&mut self, minimum_precedence: u8) -> Result<Expr, ParseError> {
        let mut left = self.parse_prefix()?;
        while let Some((operator, precedence)) = self.binary_operator() {
            if precedence < minimum_precedence {
                break;
            }
            self.cursor += 1;
            let right = self.parse_expression(precedence + 1)?;
            let range = merge_ranges(left.range, right.range);
            left = Expr {
                range,
                kind: ExprKind::Binary {
                    left: Box::new(left),
                    op: operator,
                    right: Box::new(right),
                },
            };
        }
        if minimum_precedence == 0 && self.consume("?") {
            let when_true = self.parse_expression(0)?;
            self.expect(":")?;
            let when_false = self.parse_expression(0)?;
            let range = merge_ranges(left.range, when_false.range);
            left = Expr {
                range,
                kind: ExprKind::Ternary {
                    condition: Box::new(left),
                    when_true: Box::new(when_true),
                    when_false: Box::new(when_false),
                },
            };
        }
        Ok(left)
    }

    fn parse_prefix(&mut self) -> Result<Expr, ParseError> {
        let token = self
            .tokens
            .get(self.cursor)
            .ok_or_else(|| self.error("PINE_EXPRESSION_REQUIRED", "expression is required"))?
            .clone();
        if token.lexeme == "-" || token.lexeme == "+" || token.lexeme == "!" {
            self.cursor += 1;
            let expression = self.parse_prefix()?;
            let op = match token.lexeme.as_str() {
                "-" => UnaryOp::Negate,
                "!" => UnaryOp::Not,
                _ => UnaryOp::Positive,
            };
            let range = merge_ranges(
                SourceRange::line(self.line, token.column, token.end_column),
                expression.range,
            );
            return Ok(Expr {
                range,
                kind: ExprKind::Unary {
                    op,
                    expression: Box::new(expression),
                },
            });
        }
        let mut expression = match token.kind {
            TokenKind::Number => {
                self.cursor += 1;
                Expr {
                    range: token_range(&token),
                    kind: ExprKind::Number {
                        value: token.lexeme,
                    },
                }
            }
            TokenKind::String => {
                self.cursor += 1;
                Expr {
                    range: token_range(&token),
                    kind: ExprKind::String {
                        value: decode_string(&token.lexeme),
                    },
                }
            }
            TokenKind::Identifier => {
                self.cursor += 1;
                match token.lexeme.to_ascii_lowercase().as_str() {
                    "true" => Expr {
                        range: token_range(&token),
                        kind: ExprKind::Boolean { value: true },
                    },
                    "false" => Expr {
                        range: token_range(&token),
                        kind: ExprKind::Boolean { value: false },
                    },
                    "na" => Expr {
                        range: token_range(&token),
                        kind: ExprKind::Null,
                    },
                    _ => Expr {
                        range: token_range(&token),
                        kind: ExprKind::Identifier { name: token.lexeme },
                    },
                }
            }
            TokenKind::Punctuation if token.lexeme == "(" => {
                self.cursor += 1;
                let inner = self.parse_expression(0)?;
                self.expect(")")?;
                inner
            }
            TokenKind::Punctuation if token.lexeme == "[" => {
                self.cursor += 1;
                let mut items = Vec::new();
                if !self.consume("]") {
                    loop {
                        items.push(self.parse_expression(0)?);
                        if self.consume("]") {
                            break;
                        }
                        self.expect(",")?;
                    }
                }
                let range = items
                    .first()
                    .map(|item| item.range)
                    .unwrap_or_else(|| token_range(&token));
                Expr {
                    range,
                    kind: ExprKind::Tuple { items },
                }
            }
            _ => {
                return Err(self.error(
                    "PINE_EXPRESSION_INVALID",
                    format!("unexpected token {:?}", token.lexeme),
                ));
            }
        };
        loop {
            if self.consume(".") {
                let member = self
                    .tokens
                    .get(self.cursor)
                    .filter(|token| token.kind == TokenKind::Identifier)
                    .ok_or_else(|| self.error("PINE_MEMBER_INVALID", "member name is required"))?
                    .clone();
                self.cursor += 1;
                let range = merge_ranges(expression.range, token_range(&member));
                expression = Expr {
                    range,
                    kind: ExprKind::Member {
                        object: Box::new(expression),
                        member: member.lexeme,
                    },
                };
            } else if self.consume("[") {
                let index = self.parse_expression(0)?;
                self.expect("]")?;
                let range = merge_ranges(expression.range, index.range);
                expression = Expr {
                    range,
                    kind: ExprKind::Index {
                        object: Box::new(expression),
                        index: Box::new(index),
                    },
                };
            } else if self.consume("(") {
                let callee = expression_name(&expression).ok_or_else(|| {
                    self.error("PINE_CALL_INVALID", "call target must be a named function")
                })?;
                let mut arguments = Vec::new();
                if !self.consume(")") {
                    loop {
                        let value = self.parse_expression(0)?;
                        arguments.push(value);
                        if self.consume(")") {
                            break;
                        }
                        self.expect(",")?;
                    }
                }
                let range = merge_ranges(
                    expression.range,
                    arguments
                        .last()
                        .map(|item| item.range)
                        .unwrap_or(expression.range),
                );
                expression = Expr {
                    range,
                    kind: ExprKind::Call { callee, arguments },
                };
            } else {
                break;
            }
        }
        Ok(expression)
    }

    fn binary_operator(&self) -> Option<(BinaryOp, u8)> {
        let token = self.tokens.get(self.cursor)?;
        Some(match token.lexeme.to_ascii_lowercase().as_str() {
            "or" | "||" => (BinaryOp::Or, 1),
            "and" | "&&" => (BinaryOp::And, 2),
            "==" | "=" => (BinaryOp::Equal, 3),
            "!=" => (BinaryOp::NotEqual, 3),
            "<" => (BinaryOp::Less, 4),
            "<=" => (BinaryOp::LessEqual, 4),
            ">" => (BinaryOp::Greater, 4),
            ">=" => (BinaryOp::GreaterEqual, 4),
            "+" => (BinaryOp::Add, 5),
            "-" => (BinaryOp::Subtract, 5),
            "*" => (BinaryOp::Multiply, 6),
            "/" => (BinaryOp::Divide, 6),
            "%" => (BinaryOp::Remainder, 6),
            _ => return None,
        })
    }

    fn consume(&mut self, value: &str) -> bool {
        if self
            .tokens
            .get(self.cursor)
            .is_some_and(|token| token.lexeme.eq_ignore_ascii_case(value))
        {
            self.cursor += 1;
            true
        } else {
            false
        }
    }

    fn expect(&mut self, value: &str) -> Result<(), ParseError> {
        if self.consume(value) {
            Ok(())
        } else {
            Err(self.error("PINE_EXPRESSION_INVALID", format!("expected {value:?}")))
        }
    }

    fn error(&self, code: &'static str, message: impl Into<String>) -> ParseError {
        ParseError {
            code_name: code,
            message: message.into(),
            line: self.line,
            column: self.tokens.get(self.cursor).map_or(1, |token| token.column),
        }
    }
}

fn expression_name(expression: &Expr) -> Option<String> {
    match &expression.kind {
        ExprKind::Identifier { name } => Some(name.clone()),
        ExprKind::Member { object, member } => {
            Some(format!("{}.{}", expression_name(object)?, member))
        }
        _ => None,
    }
}

fn split_raw_arguments(value: &str) -> Vec<String> {
    let mut result = Vec::new();
    let mut start = 0usize;
    let mut depth = 0usize;
    let mut quoted = false;
    for (index, character) in value.char_indices() {
        match character {
            '"' => quoted = !quoted,
            '(' | '[' | '{' if !quoted => depth += 1,
            ')' | ']' | '}' if !quoted => depth = depth.saturating_sub(1),
            ',' if !quoted && depth == 0 => {
                result.push(value[start..index].trim().to_owned());
                start = index + 1;
            }
            _ => {}
        }
    }
    if start < value.len() {
        result.push(value[start..].trim().to_owned());
    }
    result
}

fn range_for_line(line: &LexedLine) -> SourceRange {
    SourceRange::line(
        line.number,
        line.indent + 1,
        line.indent + line.text.len() + 1,
    )
}
fn token_range(token: &Token) -> SourceRange {
    SourceRange::line(token.line, token.column, token.end_column)
}
fn merge_ranges(first: SourceRange, second: SourceRange) -> SourceRange {
    SourceRange {
        start_line: first.start_line,
        start_column: first.start_column,
        end_line: second.end_line,
        end_column: second.end_column,
    }
}

impl fmt::Display for Expr {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match &self.kind {
            ExprKind::Number { value } => formatter.write_str(value),
            ExprKind::String { value } => write!(formatter, "\"{value}\""),
            ExprKind::Boolean { value } => {
                formatter.write_str(if *value { "true" } else { "false" })
            }
            ExprKind::Null => formatter.write_str("na"),
            ExprKind::Identifier { name } => formatter.write_str(name),
            ExprKind::Unary { op, expression } => write!(formatter, "{:?}{expression}", op),
            ExprKind::Binary { left, op, right } => write!(formatter, "({left} {:?} {right})", op),
            ExprKind::Ternary {
                condition,
                when_true,
                when_false,
            } => write!(formatter, "({condition} ? {when_true} : {when_false})"),
            ExprKind::Member { object, member } => write!(formatter, "{object}.{member}"),
            ExprKind::Index { object, index } => write!(formatter, "{object}[{index}]"),
            ExprKind::Call { callee, arguments } => {
                write!(formatter, "{callee}(")?;
                for (index, argument) in arguments.iter().enumerate() {
                    if index > 0 {
                        formatter.write_str(",")?;
                    }
                    write!(formatter, "{argument}")?;
                }
                formatter.write_str(")")
            }
            ExprKind::Tuple { items } => {
                formatter.write_str("[")?;
                for (index, item) in items.iter().enumerate() {
                    if index > 0 {
                        formatter.write_str(",")?;
                    }
                    write!(formatter, "{item}")?;
                }
                formatter.write_str("]")
            }
        }
    }
}
