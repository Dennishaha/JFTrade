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
    let (num_str, unit) =
        if let Some(num) = val.strip_suffix("mo").or_else(|| val.strip_suffix("month")) {
            (num, "mo")
        } else if let Some(num) = val.strip_suffix("min") {
            (num, "min")
        } else if let Some(num) = val.strip_suffix('w').or_else(|| val.strip_suffix("week")) {
            (num, "w")
        } else if let Some(num) = val.strip_suffix('d').or_else(|| val.strip_suffix("day")) {
            (num, "d")
        } else if let Some(num) = val.strip_suffix('h').or_else(|| val.strip_suffix("hour")) {
            (num, "h")
        } else if let Some(num) = val.strip_suffix('m') {
            (num, "m")
        } else {
            return 1;
        };
    let amount: usize = match num_str.trim().parse() {
        Ok(n) if n > 0 => n,
        _ => return 1,
    };
    match unit {
        "min" | "m" => amount,
        "h" => amount * 60,
        "d" => amount * minutes_per_day,
        "w" => amount * minutes_per_day * 5,
        "mo" => amount * minutes_per_day * 20,
        _ => 1,
    }
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
            ExprKind::Member { object, member } => {
                note_runtime_variable(member, &mut self.result);
                self.visit_expr(object)?;
                // `ta.obv` is a bare member access rather than a call; Go's
                // `parseOBVBinding` treats it as `obv()` over `close`.
                if member.eq_ignore_ascii_case("obv")
                    && matches!(&object.kind, ExprKind::Identifier { name } if name.eq_ignore_ascii_case("ta"))
                {
                    self.indicators.insert(
                        "obv:close".to_owned(),
                        IndicatorRequirement {
                            alias: String::new(),
                            kind: "obv".to_owned(),
                            key: "obv:close".to_owned(),
                        },
                    );
                }
            }
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
                // A supported `request.security` wrapper lowers the wrapped
                // indicator into the timeframe-suffixed key, and Go then keeps
                // only that key. Collecting the inner call as well would add a
                // duplicate chart-timeframe requirement the worker never needs.
                let wrapped_by_security = callee.eq_ignore_ascii_case("request.security")
                    && arguments.len() >= 3
                    && indicator_time_unit(&argument_text(arguments.get(1)).unwrap_or_default())
                        .is_some()
                    && security_inner_binding(arguments.get(2), expression.range.start_line)?
                        .is_some();
                for (index, argument) in arguments.iter().enumerate() {
                    if wrapped_by_security && index == 2 {
                        continue;
                    }
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
            ExprKind::Identifier { name } => note_runtime_variable(name, &mut self.result),
            ExprKind::Number { .. }
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
            ensure_positive_period(line, callee, &length)?;
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
        // Go drops the source only when it equals the family's legacy default
        // (`rsi:14` for close, `cci:20` for hlc3) and keeps whatever the script
        // passed for the window family (`mom:close:5`, `rising:close:3`).
        // Applying the close-only rule to every family turned `cci:20` into
        // `cci:hlc3:20` and `mom:close:5` into `mom:5`.
        "ta.rsi" | "ta.cci" => {
            let defaults = if lower == "ta.rsi" {
                ("close", "14")
            } else {
                ("hlc3", "20")
            };
            let (source, length) = source_length_arguments(arguments, defaults);
            ensure_positive_period(line, callee, &length)?;
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            key_parts.extend([source.clone(), length]);
            if legacy_source_for(kind) == Some(source.as_str()) {
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
        "ta.stdev" => {
            kind = "stdev";
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .or_else(|| argument_text(arguments.first()))
                .ok_or_else(|| invalid(line, format!("{callee} requires a length")))?;
            if legacy_source_for(kind) != Some(source.as_str()) {
                key_parts.push(source);
            }
            key_parts.push(length);
        }
        "ta.wpr" | "ta.williams_r" | "ta.williamsr" => {
            // Go's `parseWilliamsRBinding` stores the Williams %R requirement
            // under the DSL name (`williamsr:14`), not the Pine `ta.wpr` name.
            kind = "williamsr";
            for argument in arguments {
                key_parts
                    .push(argument_text(Some(argument)).unwrap_or_else(|| argument.to_string()));
            }
        }
        "ta.atr" | "ta.variance" | "ta.vwap" | "ta.mfi" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            for argument in arguments {
                key_parts
                    .push(argument_text(Some(argument)).unwrap_or_else(|| argument.to_string()));
            }
        }
        "ta.highest" | "ta.lowest" | "ta.change" | "ta.mom" | "ta.roc" | "ta.range" | "ta.mode"
        | "ta.sum" | "ta.rising" | "ta.falling" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            // Go normalizes the window family through
            // `pkg/strategy/pine/lower_ta.go::pineWindowFunctionArgs` before
            // planning: a lone argument is the source (except for
            // highest/lowest, where it is the period) and missing argument
            // lists fall back to the family defaults.
            let (source, length) = window_arguments(kind, arguments);
            ensure_positive_period(line, callee, &length)?;
            key_parts.extend([source, length]);
        }
        // Advanced indicator bindings mirror
        // `pkg/strategy/ir/planner_indicator_adv.go` key construction so the
        // worker-side catalog can resolve the same requirement keys. The
        // security wrapper delegates to this table through
        // `security_indicator_requirement`.
        "ta.linreg" => {
            kind = "linreg";
            let mut parts = source_period_parts(callee, arguments, line, 2)?;
            let offset = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.linreg offset must be a non-negative integer"))?;
            parts.push(offset);
            key_parts.extend(parts);
        }
        "ta.obv" => {
            kind = "obv";
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            key_parts.push(source);
        }
        "ta.pivothigh" | "ta.pivotlow" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            let default_source = if kind == "pivotlow" { "low" } else { "high" };
            let (source, lengths): (String, &[Expr]) = match arguments.len() {
                2 => (default_source.to_owned(), arguments),
                3 => (
                    argument_text(arguments.first()).unwrap_or_else(|| default_source.to_owned()),
                    &arguments[1..],
                ),
                _ => {
                    return Err(invalid(
                        line,
                        format!("{callee} requires left and right bars with optional source"),
                    ));
                }
            };
            let left = argument_text(lengths.first())
                .ok_or_else(|| invalid(line, format!("{callee} left bars must be positive")))?;
            let right = argument_text(lengths.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} right bars must be positive")))?;
            key_parts.extend([source, left, right]);
        }
        "ta.kc" | "ta.kcw" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} length must be positive")))?;
            let multiplier = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, format!("{callee} multiplier must be positive")))?;
            let use_true_range =
                argument_text(arguments.get(3)).unwrap_or_else(|| "true".to_owned());
            key_parts.extend([source, length, multiplier, use_true_range]);
        }
        "ta.alma" => {
            kind = "alma";
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, "ta.alma length must be positive"))?;
            let offset = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.alma offset must be numeric"))?;
            let sigma = argument_text(arguments.get(3))
                .ok_or_else(|| invalid(line, "ta.alma sigma must be positive"))?;
            key_parts.extend([source, length, offset, sigma]);
        }
        "ta.cmo" | "ta.dev" | "ta.median" | "ta.percentrank" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            key_parts.extend(source_period_parts(callee, arguments, line, 2)?);
        }
        "ta.tsi" => {
            kind = "tsi";
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let short = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, "ta.tsi short length must be positive"))?;
            let long = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.tsi long length must be positive"))?;
            key_parts.extend([source, short, long]);
        }
        "ta.correlation" => {
            kind = "correlation";
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let second = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, "ta.correlation second source is required"))?;
            let length = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.correlation length must be positive"))?;
            key_parts.extend([source, second, length]);
        }
        "ta.percentile_linear_interpolation" | "ta.percentile_nearest_rank" => {
            kind = lower.strip_prefix("ta.").unwrap_or_default();
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} length must be positive")))?;
            let percentage = argument_text(arguments.get(2)).ok_or_else(|| {
                invalid(
                    line,
                    format!("{callee} percentage must be between 0 and 100"),
                )
            })?;
            key_parts.extend([source, length, percentage]);
        }
        "ta.swma" => {
            kind = "swma";
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            key_parts.push(source);
        }
        "request.security" => {
            if arguments.len() < 3 {
                return Err(invalid(
                    line,
                    "request.security requires symbol, timeframe, and expression",
                ));
            }
            // Go lowers a supported MTF indicator expression into the plain
            // indicator key with a time-unit suffix
            // (`linreg:close:5:0:15m`), so downstream workers resolve the same
            // catalog entry as a chart-timeframe indicator.
            let timeframe_text = argument_text(arguments.get(1)).unwrap_or_default();
            if let Some(time_unit) = indicator_time_unit(&timeframe_text)
                && let Some((inner_kind, mut parts)) =
                    security_inner_binding(arguments.get(2), line)?
            {
                if inner_kind == "ma" {
                    // Go's `BuildMovingAverageKeyWithSource` keeps the time
                    // unit before the optional source
                    // (`ma:EMA:14:hour:hlc3`), so the suffix cannot simply be
                    // appended after a non-close source.
                    parts.insert(2.min(parts.len()), time_unit);
                } else {
                    parts.push(time_unit);
                }
                return Ok(Some(IndicatorRequirement {
                    alias: alias.to_owned(),
                    kind: inner_kind.clone(),
                    key: format!("{inner_kind}:{}", parts.join(":")),
                }));
            }
            kind = "security";
            let symbol = argument_text(arguments.first()).unwrap_or_default();
            let timeframe = timeframe_text;
            let expression = argument_text(arguments.get(2)).unwrap_or_default();
            // Go rejects a static timeframe string outside `pineTimeframeUnit`
            // (`request.security(syminfo.tickerid, "2", ...)`), so the fallback
            // key may not silently accept it.
            if matches!(&arguments[1].kind, ExprKind::String { .. })
                && indicator_time_unit(&timeframe).is_none()
            {
                return Err(invalid(
                    line,
                    "request.security() supports only static timeframe strings",
                ));
            }
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

/// Go's `pineTimeframeUnit` accepts a fixed set of timeframes and represents
/// each as a suffix (`minute`, `hour`, `15m`, `day`, ...). The Rust planner
/// only needs the units that can appear in an indicator key.
fn indicator_time_unit(raw: &str) -> Option<String> {
    let clean = raw.trim().trim_matches('"').trim_matches('\'');
    match clean.to_ascii_uppercase().as_str() {
        "1" => Some("minute".to_owned()),
        "5" | "15" | "30" | "45" | "120" | "240" => Some(format!("{clean}m")),
        "60" => Some("hour".to_owned()),
        "D" => Some("day".to_owned()),
        "W" => Some("week".to_owned()),
        "M" => Some("month".to_owned()),
        _ if clean.ends_with('m') && clean[..clean.len() - 1].parse::<u32>().is_ok() => {
            Some(clean.to_ascii_lowercase())
        }
        _ => None,
    }
}

/// Projects the supported `ta.<indicator>(...)` expression inside
/// `request.security` onto its Go indicator key parts.
fn security_inner_binding(
    expression: Option<&Expr>,
    line: usize,
) -> Result<Option<(String, Vec<String>)>, PlannerError> {
    let Some(expression) = expression else {
        return Ok(None);
    };
    // `ta.obv` is a bare member access, not a call; Go's
    // `lowerSupportedRequestSecurityInner` rewrites it to `obv(close, <unit>)`.
    if let ExprKind::Member { object, member } = &expression.kind
        && member.eq_ignore_ascii_case("obv")
        && matches!(&object.kind, ExprKind::Identifier { name } if name.eq_ignore_ascii_case("ta"))
    {
        return Ok(Some(("obv".to_owned(), vec!["close".to_owned()])));
    }
    let ExprKind::Call { callee, arguments } = &expression.kind else {
        return Ok(None);
    };
    let lower = callee.to_ascii_lowercase();
    let kind = lower.strip_prefix("ta.").unwrap_or_default().to_owned();
    let mut parts = match kind.as_str() {
        "linreg" => {
            let mut parts = source_period_parts(callee, arguments, line, 2)?;
            parts.push(
                argument_text(arguments.get(2)).ok_or_else(|| {
                    invalid(line, "ta.linreg offset must be a non-negative integer")
                })?,
            );
            parts
        }
        "obv" => vec![argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned())],
        "pivothigh" | "pivotlow" => {
            let default_source = if kind == "pivotlow" { "low" } else { "high" };
            let (source, lengths): (String, &[Expr]) = match arguments.len() {
                2 => (default_source.to_owned(), arguments),
                3 => (
                    argument_text(arguments.first()).unwrap_or_else(|| default_source.to_owned()),
                    &arguments[1..],
                ),
                _ => return Ok(None),
            };
            let left = argument_text(lengths.first())
                .ok_or_else(|| invalid(line, format!("{callee} left bars must be positive")))?;
            let right = argument_text(lengths.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} right bars must be positive")))?;
            vec![source, left, right]
        }
        "kc" | "kcw" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} length must be positive")))?;
            let multiplier = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, format!("{callee} multiplier must be positive")))?;
            let use_true_range =
                argument_text(arguments.get(3)).unwrap_or_else(|| "true".to_owned());
            vec![source, length, multiplier, use_true_range]
        }
        "alma" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, "ta.alma length must be positive"))?;
            let offset = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.alma offset must be numeric"))?;
            let sigma = argument_text(arguments.get(3))
                .ok_or_else(|| invalid(line, "ta.alma sigma must be positive"))?;
            vec![source, length, offset, sigma]
        }
        "cmo" | "dev" | "median" | "percentrank" => {
            source_period_parts(callee, arguments, line, 2)?
        }
        "tsi" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let short = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, "ta.tsi short length must be positive"))?;
            let long = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.tsi long length must be positive"))?;
            vec![source, short, long]
        }
        "correlation" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let second = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, "ta.correlation second source is required"))?;
            let length = argument_text(arguments.get(2))
                .ok_or_else(|| invalid(line, "ta.correlation length must be positive"))?;
            vec![source, second, length]
        }
        "percentile_linear_interpolation" | "percentile_nearest_rank" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} length must be positive")))?;
            let percentage = argument_text(arguments.get(2)).ok_or_else(|| {
                invalid(
                    line,
                    format!("{callee} percentage must be between 0 and 100"),
                )
            })?;
            vec![source, length, percentage]
        }
        // Go's `planner_indicator.go` maps `ta.ema|sma|rma|wma|hma|vwma` onto
        // the shared `ma` key, and a security wrapper keeps the requested
        // timeframe as a suffix (`ma:EMA:5:15m`). The Rust arm below mirrors
        // that shape so a higher-timeframe moving average resolves the same
        // worker catalog entry instead of degrading to an opaque
        // `security:` requirement.
        "ema" | "sma" | "rma" | "wma" | "hma" | "vwma" => {
            let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
            let length = argument_text(arguments.get(1))
                .ok_or_else(|| invalid(line, format!("{callee} requires a length")))?;
            let mut parts = vec![kind.to_ascii_uppercase(), length];
            if source != "close" {
                parts.push(source);
            }
            return Ok(Some(("ma".to_owned(), parts)));
        }
        "swma" => vec![argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned())],
        _ => return Ok(None),
    };
    parts.retain(|part| !part.is_empty());
    Ok(Some((kind, parts)))
}

/// Shared `source:length` projection for the advanced source+period
/// indicators (`cog`, `cmo`, `dev`, `median`, `percentrank`).
fn source_period_parts(
    callee: &str,
    arguments: &[Expr],
    line: usize,
    minimum: usize,
) -> Result<Vec<String>, PlannerError> {
    if arguments.len() < minimum {
        return Err(invalid(
            line,
            format!("{callee} requires source and length"),
        ));
    }
    let source = argument_text(arguments.first()).unwrap_or_else(|| "close".to_owned());
    let length = argument_text(arguments.get(1))
        .ok_or_else(|| invalid(line, format!("{callee} length must be positive")))?;
    Ok(vec![source, length])
}

fn argument_text(expression: Option<&Expr>) -> Option<String> {
    expression.map(ToString::to_string)
}

/// Go rejects a non-positive literal period while planning (`ma() period must
/// be a positive integer`). Rust keeps non-literal arguments such as alias
/// identifiers untouched and only rejects literal values it can prove are not
/// positive integers.
fn ensure_positive_period(line: usize, callee: &str, period: &str) -> Result<(), PlannerError> {
    if let Ok(value) = period.trim().parse::<f64>()
        && value <= 0.0
    {
        return Err(invalid(
            line,
            format!("{callee} period must be a positive integer"),
        ));
    }
    Ok(())
}

/// Mirror `pkg/strategy/pine/lower_ta.go::pineSourceLengthArgs`: a single
/// argument is the period and the source keeps the family default.
fn source_length_arguments(arguments: &[Expr], defaults: (&str, &str)) -> (String, String) {
    let texts = argument_texts(arguments);
    match texts.len() {
        0 => (defaults.0.to_owned(), defaults.1.to_owned()),
        1 => (defaults.0.to_owned(), texts[0].clone()),
        _ => (texts[0].clone(), texts[1].clone()),
    }
}

/// Mirror `pkg/strategy/pine/lower_ta.go::pineWindowFunctionArgs`: the window
/// family keeps `high`/`low` defaults for the extrema calls and a 14-bar
/// default for the momentum calls, while `highest`/`lowest` read a lone
/// argument as the period instead of the source.
fn window_arguments(kind: &str, arguments: &[Expr]) -> (String, String) {
    let default_source = match kind {
        "highest" => "high",
        "lowest" => "low",
        _ => "close",
    };
    let default_period = match kind {
        "highest" | "lowest" | "mom" | "roc" | "rising" | "falling" => "14",
        _ => "1",
    };
    let texts = argument_texts(arguments);
    match texts.len() {
        0 => (default_source.to_owned(), default_period.to_owned()),
        1 if matches!(kind, "highest" | "lowest") => (default_source.to_owned(), texts[0].clone()),
        1 => (texts[0].clone(), default_period.to_owned()),
        _ => (texts[0].clone(), texts[1].clone()),
    }
}

fn argument_texts(arguments: &[Expr]) -> Vec<String> {
    arguments
        .iter()
        .map(|argument| argument_text(Some(argument)).unwrap_or_else(|| argument.to_string()))
        .collect()
}
/// Go records the runtime needs of an expression by scanning it for the
/// position and account-value variables, so an assignment, an if condition or
/// a call argument can all raise them (`position_size`,
/// `position_avg_price`, `equity`).
fn note_runtime_variable(name: &str, result: &mut Requirements) {
    let lower = name.to_ascii_lowercase();
    if lower == "position_size" || lower == "position_avg_price" {
        result.requires_position = true;
    }
    if lower == "equity" {
        result.requires_total_account_value = true;
    }
}

/// Go keeps the requested source inside an indicator key unless it equals the
/// family's legacy default; the window family has no legacy form and always
/// keeps the source the script passed.
fn legacy_source_for(kind: &str) -> Option<&'static str> {
    match kind {
        "rsi" | "stdev" => Some("close"),
        "cci" => Some("hlc3"),
        _ => None,
    }
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

#[cfg(test)]
mod tests {
    use super::*;

    /// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_test.go:235 TestParseIndicatorTimeUnitValue
    /// Parity: go:452dea11:pkg/strategy/indicatorbinding/parse_semantics_test.go:20 TestParseIndicatorTimeUnitValueSupportsQuotedAndMinuteCountInputs
    #[test]
    fn indicator_time_unit_keeps_the_pine_timeframes_that_reach_indicator_keys() {
        // The Rust planner only accepts the units that can appear in an
        // indicator key, which is the subset of Go's `pineTimeframeUnit`
        // table the Pine worker can actually resolve.
        assert_eq!(indicator_time_unit("1"), Some("minute".to_owned()));
        assert_eq!(indicator_time_unit("5"), Some("5m".to_owned()));
        assert_eq!(indicator_time_unit("15"), Some("15m".to_owned()));
        assert_eq!(indicator_time_unit("30"), Some("30m".to_owned()));
        assert_eq!(indicator_time_unit("45"), Some("45m".to_owned()));
        assert_eq!(indicator_time_unit("120"), Some("120m".to_owned()));
        assert_eq!(indicator_time_unit("240"), Some("240m".to_owned()));
        assert_eq!(indicator_time_unit("60"), Some("hour".to_owned()));
        assert_eq!(indicator_time_unit("d"), Some("day".to_owned()));
        assert_eq!(indicator_time_unit("W"), Some("week".to_owned()));
        assert_eq!(indicator_time_unit("M"), Some("month".to_owned()));
        assert_eq!(indicator_time_unit(" 15m "), Some("15m".to_owned()));
        assert_eq!(indicator_time_unit("\"15\""), Some("15m".to_owned()));
        assert_eq!(indicator_time_unit("60m"), Some("60m".to_owned()));
        assert_eq!(indicator_time_unit("001m"), Some("001m".to_owned()));
        assert_eq!(indicator_time_unit(""), None);
        assert_eq!(indicator_time_unit("badm"), None);
        assert_eq!(indicator_time_unit("1D"), None);
        assert_eq!(indicator_time_unit("year"), None);
    }

    #[test]
    fn test_resolve_interval_minutes_supports_broker_intervals_and_safe_fallbacks() {
        // Parity: go:452dea11:pkg/strategy/indicatorwarmup/warmup_internal_test.go:142 TestResolveIntervalMinutesSupportsBrokerIntervalsAndSafeFallbacks
        let day_minutes = 390;
        assert_eq!(resolve_interval_minutes("", day_minutes), 1);
        assert_eq!(resolve_interval_minutes("1min", day_minutes), 1);
        assert_eq!(resolve_interval_minutes("5m", day_minutes), 5);
        assert_eq!(resolve_interval_minutes("2h", day_minutes), 120);
        assert_eq!(resolve_interval_minutes("2d", day_minutes), 2 * day_minutes);
        assert_eq!(
            resolve_interval_minutes("2w", day_minutes),
            2 * day_minutes * 5
        );
        assert_eq!(
            resolve_interval_minutes("2mo", day_minutes),
            2 * day_minutes * 20
        );
        assert_eq!(resolve_interval_minutes("bad", day_minutes), 1);
        assert_eq!(resolve_interval_minutes("0m", day_minutes), 1);
        assert_eq!(resolve_interval_minutes("xm", day_minutes), 1);
    }
}
