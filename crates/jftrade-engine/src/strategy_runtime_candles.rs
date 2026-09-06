use crate::product::{MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort};
use jftrade_integration_pine::{PineCandle, PineOrderIntent, PineRunResult};
use jftrade_store_sqlite::StrategyRuntimeStore;
use serde_json::Value;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Duration;

pub(super) async fn read_strategy_candles(
    quote: &dyn MarketDataQuoteReadSnapshotPort,
    market: &str,
    symbol: &str,
    timeframe: &str,
    limit: usize,
    sessions: &[String],
) -> Result<Vec<PineCandle>, String> {
    let path = format!("/api/v1/market-data/candles/{market}/{symbol}");
    let query = format!(
        "period={timeframe}&limit={limit}&sessions={}",
        sessions.join(",")
    );
    let value = quote.read(&path, &query).await.map_err(quote_error_message)?;
    parse_strategy_candles(&value)
}

fn quote_error_message(error: MarketDataQuoteReadSnapshotError) -> String {
    match error {
        MarketDataQuoteReadSnapshotError::Unavailable(message) => {
            format!("market-data unavailable: {message}")
        }
        MarketDataQuoteReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => match retry_after_seconds {
            Some(retry) => {
                format!("market-data failed ({status} {code}): {message}; retry after {retry}s")
            }
            None => format!("market-data failed ({status} {code}): {message}"),
        },
    }
}

pub(super) fn parse_strategy_candles(value: &Value) -> Result<Vec<PineCandle>, String> {
    let entries = value
        .get("candles")
        .and_then(Value::as_array)
        .ok_or_else(|| "market-data candle response is missing candles".to_owned())?;
    let mut previous = None;
    let mut candles = Vec::with_capacity(entries.len());
    for (index, entry) in entries.iter().enumerate() {
        let at = entry
            .get("at")
            .and_then(Value::as_str)
            .ok_or_else(|| format!("candle[{index}] is missing at"))?;
        let timestamp = time::OffsetDateTime::parse(
            at,
            &time::format_description::well_known::Rfc3339,
        )
        .map_err(|error| format!("candle[{index}] has invalid at: {error}"))?;
        let open_time = timestamp.unix_timestamp_nanos() / 1_000_000;
        let open_time = i64::try_from(open_time)
            .map_err(|_| format!("candle[{index}] timestamp is out of range"))?;
        if previous.is_some_and(|previous| open_time <= previous) {
            return Err("market-data candles are not strictly chronological".to_owned());
        }
        previous = Some(open_time);
        let open = candle_number(entry, "open", index)?;
        let high = candle_number(entry, "high", index)?;
        let low = candle_number(entry, "low", index)?;
        let close = candle_number(entry, "close", index)?;
        if high < low || high < open || high < close || low > open || low > close {
            return Err(format!("candle[{index}] has invalid OHLC bounds"));
        }
        let volume = entry
            .get("volume")
            .filter(|value| !value.is_null())
            .map(|value| candle_number_value(value, "volume", index))
            .transpose()?
            .unwrap_or(0.0);
        if volume < 0.0 {
            return Err(format!("candle[{index}] has negative volume"));
        }
        candles.push(PineCandle {
            open_time,
            close_time: open_time,
            open,
            high,
            low,
            close,
            volume,
        });
    }
    Ok(candles)
}

fn candle_number(entry: &Value, field: &str, index: usize) -> Result<f64, String> {
    let value = entry
        .get(field)
        .ok_or_else(|| format!("candle[{index}] is missing {field}"))?;
    candle_number_value(value, field, index)
}

fn candle_number_value(value: &Value, field: &str, index: usize) -> Result<f64, String> {
    let parsed = match value {
        Value::Number(number) => number
            .as_f64()
            .ok_or_else(|| format!("candle[{index}] {field} is not finite"))?,
        Value::String(text) => text
            .trim()
            .parse::<f64>()
            .map_err(|_| format!("candle[{index}] {field} is not numeric"))?,
        _ => return Err(format!("candle[{index}] {field} is not numeric")),
    };
    if !parsed.is_finite() {
        return Err(format!("candle[{index}] {field} is not finite"));
    }
    Ok(parsed)
}

pub(super) fn current_bar_intents(
    intents: &[PineOrderIntent],
    bar_index: i32,
    open_time: i64,
) -> Vec<PineOrderIntent> {
    intents
        .iter()
        .filter(|intent| {
            intent.bar_index == bar_index || (intent.time > 0 && intent.time == open_time)
        })
        .cloned()
        .collect()
}

pub(super) fn record_worker_output(
    store: &StrategyRuntimeStore,
    instance_id: &str,
    response: &PineRunResult,
    at_ms: i64,
) -> Result<(), String> {
    for message in response.logs.iter().chain(response.warnings.iter()) {
        store
            .append_log_event(instance_id, message, "info", at_ms)
            .map_err(|error| error.to_string())?;
    }
    for diagnostic in &response.diagnostics {
        let detail = if diagnostic.code.trim().is_empty() {
            diagnostic.message.clone()
        } else {
            format!("{}: {}", diagnostic.code, diagnostic.message)
        };
        store
            .append_log_event(instance_id, &detail, &diagnostic.severity, at_ms)
            .map_err(|error| error.to_string())?;
    }
    if !response.order_intents.is_empty() {
        store
            .append_audit_event(
                instance_id,
                "SIGNAL",
                &format!("{} order intent(s)", response.order_intents.len()),
                at_ms,
            )
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

pub(super) fn sleep_until_next_strategy_poll(cancel: &AtomicBool) {
    const POLL_INTERVAL: Duration = Duration::from_secs(1);
    let deadline = std::time::Instant::now() + POLL_INTERVAL;
    while !cancel.load(Ordering::Acquire) {
        let remaining = deadline.saturating_duration_since(std::time::Instant::now());
        if remaining.is_zero() {
            break;
        }
        std::thread::sleep(remaining.min(Duration::from_millis(100)));
    }
}
