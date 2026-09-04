//! Strategy/Pine backtest execution adapter.
//!
//! The product engine owns request validation, persistence, cancellation and
//! task lifecycle.  This module owns the optional worker-side execution
//! boundary and the deterministic Rust matcher adapter.  Keeping the port and
//! its implementation here prevents the composition root from depending
//! directly on the backtest capability crate.

use std::collections::BTreeMap;
use std::sync::Arc;

use serde_json::{Map, Value, json};
use thiserror::Error;

use crate::{
    PineExecutionError, PineExecutionPort, PineOrderIntent, PineRunRequest, PineRunResult,
};

/// A validated candle handed to a strategy/Pine backtest adapter.
///
/// The shape intentionally stays independent of SQLite rows so the execution
/// port can be used by a worker or fixture without importing a store adapter.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BacktestExecutionCandle {
    pub start_time: i64,
    pub end_time: i64,
    pub open: String,
    pub high: String,
    pub low: String,
    pub close: String,
    pub volume: String,
}

/// Input handed to a strategy/Pine backtest adapter after request and history
/// validation. The raw request is retained so adapters can preserve fields
/// that are not part of the Rust domain model yet.
#[derive(Clone, Debug)]
pub struct BacktestExecutionRequest {
    pub run_id: String,
    pub payload: Value,
    pub market_data_provider: String,
    pub candles: Vec<BacktestExecutionCandle>,
}

#[derive(Debug, Error, Clone, Eq, PartialEq)]
pub enum BacktestExecutionError {
    #[error("backtest execution is unavailable: {0}")]
    Unavailable(String),
    #[error("invalid backtest execution input: {0}")]
    Invalid(String),
    #[error("backtest execution failed: {0}")]
    Failed(String),
}

/// Narrow adapter contract for strategy/PineTS and the deterministic Rust
/// matcher. Implementations may perform blocking work; the product task
/// registry runs them behind `spawn_blocking` and fences the resulting write
/// with a status CAS.
pub trait BacktestExecutionPort: Send + Sync + std::fmt::Debug {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError>;
}

/// Explicit adapter used by fixtures and local rehearsals. It invokes the
/// deterministic `jftrade-backtest` corpus boundary and returns decoded JSON.
/// A normal StartRequest is not itself a corpus; callers must provide a
/// `corpus` object (or a corpus-shaped payload) produced by the strategy/Pine
/// adapter.
#[derive(Clone, Copy, Debug, Default)]
pub struct RunJsonBacktestExecutionPort;

impl BacktestExecutionPort for RunJsonBacktestExecutionPort {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError> {
        let mut corpus = request
            .payload
            .get("corpus")
            .cloned()
            .unwrap_or_else(|| request.payload.clone());
        // A strategy adapter may provide only corpus metadata while history is
        // resolved by the production market-data store. Fill an explicitly
        // empty first case from that validated history; never overwrite
        // worker-provided candles.
        if let Some(case) = corpus
            .get_mut("cases")
            .and_then(Value::as_array_mut)
            .and_then(|cases| cases.first_mut())
            && case
                .get("candles")
                .and_then(Value::as_array)
                .is_none_or(Vec::is_empty)
        {
            let warmup_bars = request
                .payload
                .get("warmupBars")
                .and_then(Value::as_u64)
                .unwrap_or(0) as usize;
            let formal_candles = if warmup_bars < request.candles.len() {
                &request.candles[warmup_bars..]
            } else {
                &request.candles[..]
            };
            case["candles"] = Value::Array(formal_candles.iter().map(candle_wire).collect());
        }
        let bytes = serde_json::to_vec(&corpus)
            .map_err(|error| BacktestExecutionError::Invalid(error.to_string()))?;
        let output = jftrade_backtest::run_json(&bytes)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))?;
        serde_json::from_slice(&output)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))
    }
}

/// Bridges the asynchronous Pine worker port to the deterministic Rust
/// matcher.  The bridge deliberately requires an inline strategy source in
/// the request payload: a definition id alone is not executable in this
/// crate, so silently producing a no-op result would hide a missing runtime
/// dependency.
#[derive(Clone, Debug)]
pub struct PineBacktestExecutionAdapter {
    port: Arc<dyn PineExecutionPort>,
}

impl PineBacktestExecutionAdapter {
    pub fn new(port: Arc<dyn PineExecutionPort>) -> Self {
        Self { port }
    }

    pub fn port(&self) -> &Arc<dyn PineExecutionPort> {
        &self.port
    }
}

impl BacktestExecutionPort for PineBacktestExecutionAdapter {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError> {
        let pine_request = build_pine_request(&request)?;
        let pine_result = run_pine_port(Arc::clone(&self.port), pine_request)
            .map_err(map_pine_execution_error)?;
        let corpus = build_corpus(&request, &pine_result.order_intents)?;
        let bytes = serde_json::to_vec(&corpus)
            .map_err(|error| BacktestExecutionError::Invalid(error.to_string()))?;
        let output = jftrade_backtest::run_json(&bytes).map_err(map_backtest_error)?;
        serde_json::from_slice(&output)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))
    }
}

fn build_pine_request(
    request: &BacktestExecutionRequest,
) -> Result<PineRunRequest, BacktestExecutionError> {
    if request.candles.is_empty() {
        return Err(BacktestExecutionError::Unavailable(
            "backtest K-line history is unavailable".to_owned(),
        ));
    }
    let source = required_text(
        &request.payload,
        &[
            "strategyScript",
            "strategySource",
            "strategy_source",
            "source",
            "script",
        ],
        "strategy source",
    )?;
    let symbol = optional_text(&request.payload, &["symbol", "instrumentId"])?
        .or_else(|| {
            request
                .payload
                .get("market")
                .and_then(Value::as_str)
                .zip(request.payload.get("code").and_then(Value::as_str))
                .map(|(market, code)| format!("{market}.{code}"))
        })
        .or_else(|| {
            corpus_case_value(&request.payload, "symbol")
                .and_then(Value::as_str)
                .map(str::to_owned)
        })
        .ok_or_else(|| BacktestExecutionError::Invalid("symbol is required".to_owned()))?;
    let timeframe = required_text(&request.payload, &["interval", "timeframe"], "timeframe")?;
    let script_id = optional_text(
        &request.payload,
        &["scriptId", "definitionId", "strategyId"],
    )?
    .unwrap_or_else(|| request.run_id.clone());
    let chart_type =
        optional_text(&request.payload, &["chartType"])?.unwrap_or_else(|| "standard".to_owned());
    let params = parse_params(&request.payload)?;
    let candles = request
        .candles
        .iter()
        .enumerate()
        .map(|(index, candle)| pine_candle(candle, index))
        .collect::<Result<Vec<_>, _>>()?;
    Ok(PineRunRequest {
        job_id: request.run_id.clone(),
        script_id,
        source,
        symbol,
        timeframe,
        chart_type,
        mode: "backtest".to_owned(),
        candles,
        params,
        ..PineRunRequest::default()
    })
}

fn build_corpus(
    request: &BacktestExecutionRequest,
    intents: &[PineOrderIntent],
) -> Result<Value, BacktestExecutionError> {
    let case = corpus_case(&request.payload);
    let symbol = optional_text(&request.payload, &["symbol", "instrumentId"])?
        .or_else(|| {
            request
                .payload
                .get("market")
                .and_then(Value::as_str)
                .zip(request.payload.get("code").and_then(Value::as_str))
                .map(|(market, code)| format!("{market}.{code}"))
        })
        .or_else(|| {
            case.get("symbol")
                .and_then(Value::as_str)
                .map(str::to_owned)
        })
        .ok_or_else(|| BacktestExecutionError::Invalid("symbol is required".to_owned()))?;
    let initial_balance = scalar_text(
        first_value(&request.payload, &["initialBalance"]).or_else(|| case.get("initialBalance")),
        "initialBalance",
    )
    .map(|value| value.unwrap_or_else(|| "0".to_owned()))?;
    let market = market_rules(&request.payload, &case)?;
    let warmup_bars = request
        .payload
        .get("warmupBars")
        .and_then(Value::as_u64)
        .unwrap_or(0) as usize;
    let formal_candles = if warmup_bars < request.candles.len() {
        &request.candles[warmup_bars..]
    } else {
        &request.candles[..]
    };
    let candles = formal_candles
        .iter()
        .enumerate()
        .map(|(index, candle)| candle_wire_checked(candle, index))
        .collect::<Result<Vec<_>, _>>()?;
    let mut converted_intents = Vec::new();
    for (ordinal, intent) in intents.iter().enumerate() {
        let bar_index = intent.bar_index;
        if bar_index < 0 || (bar_index as usize) < warmup_bars {
            continue;
        }
        let shifted_index = (bar_index as usize) - warmup_bars;
        let mut shifted_intent = intent.clone();
        shifted_intent.bar_index = shifted_index as i32;
        converted_intents.push(corpus_intent(&shifted_intent, ordinal, candles.len())?);
    }
    let base_currency = optional_text(&request.payload, &["baseCurrency"])?
        .or_else(|| {
            case.get("baseCurrency")
                .and_then(Value::as_str)
                .map(str::to_owned)
        })
        .unwrap_or_else(|| {
            symbol
                .rsplit(['.', ':'])
                .next()
                .unwrap_or(&symbol)
                .to_owned()
        });
    let quote_currency = optional_text(&request.payload, &["quoteCurrency"])?
        .or_else(|| {
            case.get("quoteCurrency")
                .and_then(Value::as_str)
                .map(str::to_owned)
        })
        .unwrap_or_else(|| "USD".to_owned());
    let mut output_case = json!({
        "id": request.run_id,
        "symbol": symbol,
        "baseCurrency": base_currency,
        "quoteCurrency": quote_currency,
        "initialBalance": initial_balance,
        "market": market,
        "candles": candles,
        "intents": converted_intents,
    });
    if let Some(val) = first_value(
        &request.payload,
        &["processOrdersOnClose", "process_orders_on_close"],
    )
    .or_else(|| case.get("processOrdersOnClose"))
    {
        output_case["processOrdersOnClose"] = val.clone();
    }
    copy_case_field(&case, &mut output_case, "slippageTicks");
    copy_case_field(&case, &mut output_case, "feeRules");
    copy_case_field(&case, &mut output_case, "indicatorPeriods");
    copy_case_field(&case, &mut output_case, "cancelBeforeBar");
    Ok(json!({"version": 1, "cases": [output_case]}))
}

fn market_rules(
    payload: &Value,
    case: &Map<String, Value>,
) -> Result<Value, BacktestExecutionError> {
    let case_market = case.get("market").and_then(Value::as_object);
    let value = |name: &str, default: &str| {
        scalar_text(
            first_value(payload, &[name]).or_else(|| case_market.and_then(|m| m.get(name))),
            name,
        )
        .map(|value| value.unwrap_or_else(|| default.to_owned()))
    };
    Ok(json!({
        "tickSize": value("tickSize", "0.01")?,
        "quantityStep": value("quantityStep", "1")?,
        "minQuantity": value("minQuantity", "1")?,
    }))
}

fn corpus_intent(
    intent: &PineOrderIntent,
    ordinal: usize,
    candle_count: usize,
) -> Result<Value, BacktestExecutionError> {
    let kind = intent.kind.trim().to_ascii_lowercase();
    let bar_index = usize::try_from(intent.bar_index).map_err(|_| {
        BacktestExecutionError::Invalid(format!("pine intent {ordinal} has a negative bar index"))
    })?;
    if bar_index >= candle_count {
        return Err(BacktestExecutionError::Invalid(format!(
            "pine intent {} targets unavailable bar {}",
            intent.id, bar_index
        )));
    }
    if kind == "cancel" {
        let id = required_intent_id(intent, ordinal)?;
        return Ok(json!({
            "barIndex": bar_index,
            "action": "cancel",
            "targetId": id,
            "id": format!("cancel:{id}"),
        }));
    }
    if kind == "cancel_all" {
        return Err(BacktestExecutionError::Invalid(
            "pine cancel_all intent cannot be represented by the deterministic matcher".to_owned(),
        ));
    }
    if !matches!(
        kind.as_str(),
        "entry" | "order" | "exit" | "close" | "close_all"
    ) {
        return Err(BacktestExecutionError::Invalid(format!(
            "unsupported pine order intent kind: {}",
            intent.kind
        )));
    }
    let id = required_intent_id(intent, ordinal)?;
    let quantity = if intent.has_quantity || intent.quantity != 0.0 {
        positive_finite(intent.quantity, "quantity", &id)?
    } else if matches!(kind.as_str(), "entry" | "order") {
        1.0
    } else if intent.has_quantity_pct || intent.quantity_pct != 0.0 {
        return Err(BacktestExecutionError::Invalid(format!(
            "pine intent {} uses quantity percent without resolved absolute quantity, which the deterministic matcher cannot represent",
            intent.id
        )));
    } else {
        return Err(BacktestExecutionError::Invalid(format!(
            "pine intent {id} requires a quantity"
        )));
    };
    let side = intent_side(&kind, &intent.direction, &id)?;
    let has_limit = intent.has_limit_price || intent.limit_price != 0.0;
    let has_stop = intent.has_stop_price || intent.stop_price != 0.0;
    if has_limit {
        positive_finite(intent.limit_price, "limit price", &id)?;
    }
    if has_stop {
        positive_finite(intent.stop_price, "stop price", &id)?;
    }
    if kind == "exit" && has_limit && has_stop {
        return Err(BacktestExecutionError::Invalid(format!(
            "pine exit intent {id} combines limit and stop prices"
        )));
    }
    let order_type = match (has_limit, has_stop) {
        (true, true) => "stop_limit",
        (true, false) => "limit",
        (false, true) => "stop_market",
        (false, false) => "market",
    };
    Ok(json!({
        "barIndex": bar_index,
        "action": "submit",
        "id": id,
        "side": side,
        "orderType": order_type,
        "quantity": quantity.to_string(),
        "limitPrice": if has_limit { intent.limit_price.to_string() } else { "0".to_owned() },
        "stopPrice": if has_stop { intent.stop_price.to_string() } else { "0".to_owned() },
        "reduceOnly": intent.reduce_only || matches!(kind.as_str(), "exit" | "close" | "close_all"),
        "parentId": intent.parent_id,
        "ocoGroupId": intent.oco_group_id,
        "atomicGroupId": intent.atomic_group_id,
    }))
}

fn required_intent_id(
    intent: &PineOrderIntent,
    ordinal: usize,
) -> Result<String, BacktestExecutionError> {
    let id = intent.id.trim();
    if id.is_empty() {
        return Err(BacktestExecutionError::Invalid(format!(
            "pine intent {ordinal} requires an id"
        )));
    }
    Ok(id.to_owned())
}

fn intent_side(
    kind: &str,
    direction: &str,
    id: &str,
) -> Result<&'static str, BacktestExecutionError> {
    let direction = direction.trim().to_ascii_lowercase();
    let side = match kind {
        "entry" | "order" => match direction.as_str() {
            "long" | "buy" => "buy",
            "short" | "sell" => "sell",
            _ => {
                return Err(BacktestExecutionError::Invalid(format!(
                    "pine intent {id} requires long/short direction"
                )));
            }
        },
        "exit" | "close" | "close_all" => match direction.as_str() {
            "short" | "buy" | "cover" => "buy",
            "long" | "sell" | "flat" | "" => "sell",
            _ => {
                return Err(BacktestExecutionError::Invalid(format!(
                    "unsupported pine close direction for intent {id}: {direction}"
                )));
            }
        },
        _ => unreachable!("intent kind checked by caller"),
    };
    Ok(side)
}

fn positive_finite(value: f64, field: &str, id: &str) -> Result<f64, BacktestExecutionError> {
    if value.is_finite() && value > 0.0 {
        Ok(value)
    } else {
        Err(BacktestExecutionError::Invalid(format!(
            "pine intent {id} {field} must be positive and finite"
        )))
    }
}

fn run_pine_port(
    port: Arc<dyn PineExecutionPort>,
    request: PineRunRequest,
) -> Result<PineRunResult, PineExecutionError> {
    std::thread::spawn(move || {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| PineExecutionError::Unavailable(error.to_string()))?;
        runtime.block_on(port.run(request))
    })
    .join()
    .map_err(|_| PineExecutionError::Transport("pine execution thread panicked".to_owned()))?
}

fn map_pine_execution_error(error: PineExecutionError) -> BacktestExecutionError {
    match error {
        PineExecutionError::Unavailable(message) | PineExecutionError::Transport(message) => {
            BacktestExecutionError::Unavailable(message)
        }
        PineExecutionError::Timeout => {
            BacktestExecutionError::Unavailable("pine worker request timed out".to_owned())
        }
        PineExecutionError::Cancelled => {
            BacktestExecutionError::Unavailable("pine worker request cancelled".to_owned())
        }
        PineExecutionError::Remote(message) => BacktestExecutionError::Failed(message),
        PineExecutionError::InvalidEndpoint(message)
        | PineExecutionError::InvalidRequest(message) => BacktestExecutionError::Invalid(message),
        PineExecutionError::WeakToken => {
            BacktestExecutionError::Invalid("pine worker token is weak".to_owned())
        }
        PineExecutionError::InvalidResponse(message) => BacktestExecutionError::Failed(message),
    }
}

fn map_backtest_error(error: jftrade_backtest::BacktestError) -> BacktestExecutionError {
    if matches!(error, jftrade_backtest::BacktestError::Arithmetic(_)) {
        BacktestExecutionError::Failed(error.to_string())
    } else {
        BacktestExecutionError::Invalid(error.to_string())
    }
}

fn first_value<'a>(payload: &'a Value, names: &[&str]) -> Option<&'a Value> {
    let object = payload.as_object()?;
    names.iter().find_map(|name| object.get(*name)).or_else(|| {
        ["strategy", "definition", "strategyDefinition"]
            .iter()
            .filter_map(|container| object.get(*container).and_then(Value::as_object))
            .find_map(|nested| names.iter().find_map(|name| nested.get(*name)))
    })
}

fn optional_text(
    payload: &Value,
    names: &[&str],
) -> Result<Option<String>, BacktestExecutionError> {
    scalar_text(first_value(payload, names), names[0])
}

fn required_text(
    payload: &Value,
    names: &[&str],
    field: &str,
) -> Result<String, BacktestExecutionError> {
    optional_text(payload, names)?
        .filter(|value| !value.is_empty())
        .ok_or_else(|| BacktestExecutionError::Invalid(format!("{field} is required")))
}

fn scalar_text(
    value: Option<&Value>,
    field: &str,
) -> Result<Option<String>, BacktestExecutionError> {
    let Some(value) = value else {
        return Ok(None);
    };
    let text = match value {
        Value::String(value) => value.trim().to_owned(),
        Value::Number(value) => value.to_string(),
        Value::Bool(value) => value.to_string(),
        Value::Null => {
            return Err(BacktestExecutionError::Invalid(format!(
                "{field} must not be null"
            )));
        }
        _ => {
            return Err(BacktestExecutionError::Invalid(format!(
                "{field} must be a scalar"
            )));
        }
    };
    if text.is_empty() {
        return Err(BacktestExecutionError::Invalid(format!(
            "{field} must not be empty"
        )));
    }
    Ok(Some(text))
}

fn parse_params(payload: &Value) -> Result<BTreeMap<String, String>, BacktestExecutionError> {
    let Some(value) = first_value(
        payload,
        &["params", "parameters", "strategyParams", "inputs"],
    ) else {
        return Ok(BTreeMap::new());
    };
    let object = value.as_object().ok_or_else(|| {
        BacktestExecutionError::Invalid("strategy params must be an object".to_owned())
    })?;
    object
        .iter()
        .map(|(key, value)| {
            let parsed = scalar_text(Some(value), &format!("strategy param {key}"))?
                .expect("scalar_text returned Some for present value");
            Ok((key.clone(), parsed))
        })
        .collect()
}

fn corpus_case(payload: &Value) -> Map<String, Value> {
    payload
        .get("corpus")
        .and_then(|value| value.get("cases"))
        .and_then(Value::as_array)
        .and_then(|cases| cases.first())
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default()
}

fn corpus_case_value<'a>(payload: &'a Value, name: &str) -> Option<&'a Value> {
    payload
        .get("corpus")
        .and_then(|value| value.get("cases"))
        .and_then(Value::as_array)
        .and_then(|cases| cases.first())
        .and_then(|case| case.get(name))
}

fn copy_case_field(case: &Map<String, Value>, output: &mut Value, name: &str) {
    if let Some(value) = case.get(name) {
        output[name] = value.clone();
    }
}

fn pine_candle(
    candle: &BacktestExecutionCandle,
    index: usize,
) -> Result<crate::PineCandle, BacktestExecutionError> {
    if candle.start_time <= 0 || (candle.end_time != 0 && candle.end_time < candle.start_time) {
        return Err(BacktestExecutionError::Invalid(format!(
            "candle {index} has invalid timestamps"
        )));
    }
    let open = candle_number(&candle.open, index, "open")?;
    let high = candle_number(&candle.high, index, "high")?;
    let low = candle_number(&candle.low, index, "low")?;
    let close = candle_number(&candle.close, index, "close")?;
    let volume = candle_number(&candle.volume, index, "volume")?;
    if high < low || open < low || open > high || close < low || close > high || volume < 0.0 {
        return Err(BacktestExecutionError::Invalid(format!(
            "candle {index} has invalid OHLCV range"
        )));
    }
    Ok(crate::PineCandle {
        open_time: candle.start_time,
        close_time: candle.end_time,
        open,
        high,
        low,
        close,
        volume,
    })
}

fn candle_number(value: &str, index: usize, field: &str) -> Result<f64, BacktestExecutionError> {
    let number = value.trim().parse::<f64>().map_err(|_| {
        BacktestExecutionError::Invalid(format!("candle {index} {field} is not numeric"))
    })?;
    if number.is_finite() {
        Ok(number)
    } else {
        Err(BacktestExecutionError::Invalid(format!(
            "candle {index} {field} must be finite"
        )))
    }
}

fn candle_wire_checked(
    candle: &BacktestExecutionCandle,
    index: usize,
) -> Result<Value, BacktestExecutionError> {
    pine_candle(candle, index)?;
    let start = timestamp_wire(candle.start_time, index)?;
    let end = timestamp_wire(candle.end_time, index)?;
    Ok(json!({
        "start": start,
        "end": end,
        "open": candle.open,
        "high": candle.high,
        "low": candle.low,
        "close": candle.close,
        "volume": candle.volume,
    }))
}

fn timestamp_wire(millis: i64, index: usize) -> Result<String, BacktestExecutionError> {
    time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(millis) * 1_000_000)
        .ok()
        .and_then(|value| {
            value
                .format(&time::format_description::well_known::Rfc3339)
                .ok()
        })
        .ok_or_else(|| {
            BacktestExecutionError::Invalid(format!("candle {index} timestamp is out of range"))
        })
}

fn candle_wire(candle: &BacktestExecutionCandle) -> Value {
    let timestamp = |millis: i64| {
        time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(millis) * 1_000_000)
            .ok()
            .and_then(|value| {
                value
                    .format(&time::format_description::well_known::Rfc3339)
                    .ok()
            })
            .unwrap_or_else(|| "1970-01-01T00:00:00Z".to_owned())
    };
    json!({
        "start": timestamp(candle.start_time),
        "end": timestamp(candle.end_time),
        "open": candle.open,
        "high": candle.high,
        "low": candle.low,
        "close": candle.close,
        "volume": candle.volume,
    })
}

#[cfg(test)]
#[path = "backtest_tests.rs"]
mod tests;
