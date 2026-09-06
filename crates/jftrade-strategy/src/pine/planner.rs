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

impl IndicatorRequirement {
    pub fn estimated_lookback_bars(&self) -> usize {
        self.estimated_lookback_bars_with_session("", "5m", false)
    }

    pub fn validate_timeframe_alignment(
        &self,
        symbol: &str,
        interval: &str,
        use_extended_hours: bool,
    ) -> Result<(), String> {
        let minutes_per_day = trading_minutes_per_day(symbol, use_extended_hours);
        let interval_minutes = resolve_interval_minutes(interval, minutes_per_day);
        let parts: Vec<&str> = self.key.split(':').collect();

        let tf_opt = match self.kind.as_str() {
            "security" => parts.get(2).copied(),
            "ma" if parts.len() >= 4 => parts.last().copied(),
            _ => None,
        };

        if let Some(tf_str) = tf_opt {
            let tf_str = tf_str.trim().trim_matches('"').trim_matches('\'');
            if !tf_str.is_empty()
                && let Some(target_minutes) = resolve_timeframe_minutes(tf_str, minutes_per_day)
            {
                if target_minutes < interval_minutes {
                    return Err(format!(
                        "indicator {} fixed timeframe {} is lower than strategy interval {}; JFTrade supports request.security() only at the current or a higher timeframe",
                        self.kind, tf_str, interval
                    ));
                }
                if target_minutes < minutes_per_day && target_minutes % interval_minutes != 0 {
                    return Err(format!(
                        "indicator {} fixed timeframe {} is not aligned with strategy interval {}; JFTrade aggregates MTF data from a single native interval",
                        self.kind, tf_str, interval
                    ));
                }
            }
        }
        Ok(())
    }

    pub fn estimated_lookback_bars_with_session(
        &self,
        symbol: &str,
        interval: &str,
        use_extended_hours: bool,
    ) -> usize {
        if self
            .validate_timeframe_alignment(symbol, interval, use_extended_hours)
            .is_err()
        {
            return 0;
        }
        let minutes_per_day = trading_minutes_per_day(symbol, use_extended_hours);
        let interval_minutes = resolve_interval_minutes(interval, minutes_per_day);
        let parts: Vec<&str> = self.key.split(':').collect();

        match self.kind.as_str() {
            "security" => {
                let timeframe = parts.get(2).copied().unwrap_or_default();
                let expression = parts.get(3).copied().unwrap_or_default();
                let period = parse_expression_period(expression);
                let tf_minutes = resolve_timeframe_minutes(timeframe, minutes_per_day)
                    .unwrap_or(minutes_per_day);
                (period * tf_minutes).div_ceil(interval_minutes)
            }
            "ma" => {
                let period = parts
                    .iter()
                    .filter_map(|p| p.parse::<usize>().ok())
                    .max()
                    .unwrap_or(0);
                if parts.len() >= 4 {
                    let tf_part = parts.last().copied().unwrap_or_default();
                    if let Some(tf_minutes) = resolve_timeframe_minutes(tf_part, minutes_per_day) {
                        return (period * tf_minutes).div_ceil(interval_minutes);
                    }
                }
                period
            }
            "macd" => {
                let nums: Vec<usize> = parts
                    .iter()
                    .filter_map(|p| p.parse::<usize>().ok())
                    .collect();
                if nums.len() >= 3 {
                    nums[1].saturating_add(nums[2])
                } else if !nums.is_empty() {
                    nums.iter().sum()
                } else {
                    35
                }
            }
            "atr" | "change" | "rising" | "falling" => parts
                .iter()
                .filter_map(|p| p.parse::<usize>().ok())
                .max()
                .unwrap_or(14)
                .saturating_add(1),
            _ => parts
                .iter()
                .filter_map(|p| p.parse::<usize>().ok())
                .max()
                .unwrap_or(0),
        }
    }
}

impl Requirements {
    pub fn derived_warmup_bars(&self) -> usize {
        self.derived_warmup_bars_with_session("", "5m", false)
    }

    pub fn validate_timeframe_alignments(
        &self,
        symbol: &str,
        interval: &str,
        use_extended_hours: bool,
    ) -> Result<(), String> {
        for indicator in &self.indicators {
            indicator.validate_timeframe_alignment(symbol, interval, use_extended_hours)?;
        }
        Ok(())
    }

    pub fn try_derived_warmup_bars_with_session(
        &self,
        symbol: &str,
        interval: &str,
        use_extended_hours: bool,
    ) -> Result<usize, String> {
        self.validate_timeframe_alignments(symbol, interval, use_extended_hours)?;
        Ok(self.derived_warmup_bars_with_session(symbol, interval, use_extended_hours))
    }

    pub fn derived_warmup_bars_with_session(
        &self,
        symbol: &str,
        interval: &str,
        use_extended_hours: bool,
    ) -> usize {
        self.indicators
            .iter()
            .map(|i| i.estimated_lookback_bars_with_session(symbol, interval, use_extended_hours))
            .max()
            .unwrap_or(0)
    }
}

fn trading_minutes_per_day(symbol: &str, use_extended_hours: bool) -> usize {
    let sym = symbol.trim().to_ascii_uppercase();
    if sym.starts_with("US.") || sym.starts_with("US:") {
        if use_extended_hours { 1440 } else { 390 }
    } else if sym.starts_with("HK.") || sym.starts_with("HK:") {
        330
    } else if sym.starts_with("SH.")
        || sym.starts_with("SZ.")
        || sym.starts_with("CN.")
        || sym.starts_with("SH:")
        || sym.starts_with("SZ:")
        || sym.starts_with("CN:")
    {
        240
    } else {
        390
    }
}

fn resolve_interval_minutes(interval: &str, minutes_per_day: usize) -> usize {
    let val = interval.trim().to_ascii_lowercase();
    if val.is_empty() {
        return 1;
    }
    if let Some(num) = val.strip_suffix("mo").or_else(|| val.strip_suffix("month")) {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return n * minutes_per_day * 20;
    }
    if let Some(num) = val.strip_suffix('w').or_else(|| val.strip_suffix("week")) {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return n * minutes_per_day * 5;
    }
    if let Some(num) = val.strip_suffix('d').or_else(|| val.strip_suffix("day")) {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return n * minutes_per_day;
    }
    if let Some(num) = val.strip_suffix('h').or_else(|| val.strip_suffix("hour")) {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return n * 60;
    }
    if let Some(num) = val.strip_suffix("min").or_else(|| val.strip_suffix('m')) {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return n;
    }
    val.parse::<usize>().unwrap_or(1).max(1)
}

fn resolve_timeframe_minutes(timeframe: &str, minutes_per_day: usize) -> Option<usize> {
    let clean = timeframe.trim().trim_matches('"').trim_matches('\'');
    if clean.is_empty() {
        return None;
    }
    if let Some(num) = clean.strip_suffix('m') {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return Some(n);
    }
    if let Some(num) = clean
        .strip_suffix("min")
        .or_else(|| clean.strip_suffix("MIN"))
    {
        let n: usize = num.trim().parse().unwrap_or(1).max(1);
        return Some(n);
    }
    let tf = clean.to_ascii_uppercase();
    if tf == "D" || tf == "1D" || tf == "DAY" {
        return Some(minutes_per_day);
    }
    if tf == "W" || tf == "1W" || tf == "WEEK" {
        return Some(minutes_per_day * 5);
    }
    if tf == "M" || tf == "1M" || tf == "1MO" || tf == "MONTH" {
        return Some(minutes_per_day * 20);
    }
    if let Some(num) = tf.strip_suffix('D') {
        return num
            .trim()
            .parse::<usize>()
            .ok()
            .map(|n| n * minutes_per_day);
    }
    if let Some(num) = tf.strip_suffix('W') {
        return num
            .trim()
            .parse::<usize>()
            .ok()
            .map(|n| n * minutes_per_day * 5);
    }
    if let Some(num) = tf.strip_suffix("MO") {
        return num
            .trim()
            .parse::<usize>()
            .ok()
            .map(|n| n * minutes_per_day * 20);
    }
    if let Some(num) = tf.strip_suffix('H') {
        return num.trim().parse::<usize>().ok().map(|n| n * 60);
    }
    if let Some(num) = tf.strip_suffix('M').or_else(|| tf.strip_suffix("MIN")) {
        return num.trim().parse::<usize>().ok();
    }
    tf.parse::<usize>().ok()
}

fn parse_expression_period(expression: &str) -> usize {
    let lower = expression.to_ascii_lowercase();
    let nums: Vec<usize> = lower
        .split(|c: char| !c.is_ascii_digit())
        .filter_map(|s| s.parse::<usize>().ok())
        .collect();
    if lower.contains("macd") && nums.len() >= 3 {
        nums[1].saturating_add(nums[2])
    } else if lower.contains("atr") {
        nums.into_iter().max().unwrap_or(14).saturating_add(1)
    } else {
        nums.into_iter().max().unwrap_or(1).max(1)
    }
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
