use crate::product::{MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort};
use jftrade_integration_pine::{PineCandle, PineOrderIntent, PineRunResult};
use jftrade_store_sqlite::StrategyRuntimeStore;
use serde_json::Value;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Duration;

#[derive(Clone, Debug, PartialEq)]
pub(super) struct StrategyCandle {
    pub(super) candle: PineCandle,
    pub(super) closed: bool,
}

impl std::ops::Deref for StrategyCandle {
    type Target = PineCandle;

    fn deref(&self) -> &Self::Target {
        &self.candle
    }
}

impl std::ops::DerefMut for StrategyCandle {
    fn deref_mut(&mut self) -> &mut Self::Target {
        &mut self.candle
    }
}

pub(super) async fn read_strategy_candles(
    quote: &dyn MarketDataQuoteReadSnapshotPort,
    market: &str,
    symbol: &str,
    timeframe: &str,
    limit: usize,
    sessions: &[String],
) -> Result<Vec<StrategyCandle>, String> {
    let path = format!("/api/v1/market-data/candles/{market}/{symbol}");
    let query = format!(
        "period={timeframe}&limit={limit}&sessions={}",
        sessions.join(",")
    );
    let value = quote
        .read(&path, &query)
        .await
        .map_err(quote_error_message)?;
    parse_strategy_candles(&value, Some(timeframe))
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

pub(super) fn period_duration_millis(period: &str) -> i64 {
    match period.trim().to_ascii_lowercase().as_str() {
        "1m" => 60 * 1_000,
        "3m" => 180 * 1_000,
        "5m" => 300 * 1_000,
        "10m" => 600 * 1_000,
        "15m" => 900 * 1_000,
        "30m" => 1_800 * 1_000,
        "60m" | "1h" => 3_600 * 1_000,
        "120m" | "2h" => 7_200 * 1_000,
        "180m" | "3h" => 10_800 * 1_000,
        "240m" | "4h" => 14_400 * 1_000,
        "1d" | "day" => 86_400 * 1_000,
        "1w" | "week" => 7 * 86_400 * 1_000,
        "1mo" | "month" => 30 * 86_400 * 1_000,
        _ => 60 * 1_000,
    }
}

fn current_time_millis() -> i64 {
    time::OffsetDateTime::now_utc().unix_timestamp_nanos() as i64 / 1_000_000
}

pub(super) fn parse_strategy_candles(
    value: &Value,
    timeframe: Option<&str>,
) -> Result<Vec<StrategyCandle>, String> {
    let entries = value
        .get("candles")
        .and_then(Value::as_array)
        .ok_or_else(|| "market-data candle response is missing candles".to_owned())?;
    let mut previous = None;
    let mut candles = Vec::with_capacity(entries.len());
    let now_ms = current_time_millis();

    for (index, entry) in entries.iter().enumerate() {
        let at = entry
            .get("at")
            .and_then(Value::as_str)
            .ok_or_else(|| format!("candle[{index}] is missing at"))?;
        let timestamp =
            time::OffsetDateTime::parse(at, &time::format_description::well_known::Rfc3339)
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

        let period = entry
            .get("period")
            .and_then(Value::as_str)
            .or(timeframe)
            .or_else(|| {
                value
                    .get("request")
                    .and_then(|r| r.get("period"))
                    .and_then(Value::as_str)
            })
            .unwrap_or("1m");
        let duration_ms = period_duration_millis(period);
        let close_time = entry
            .get("close_time")
            .or_else(|| entry.get("closeTime"))
            .and_then(Value::as_i64)
            .unwrap_or(open_time.saturating_add(duration_ms));

        let closed = if let Some(explicit_closed) = entry.get("closed").and_then(Value::as_bool) {
            explicit_closed
        } else {
            now_ms >= close_time
        };

        candles.push(StrategyCandle {
            candle: PineCandle {
                open_time,
                close_time,
                open,
                high,
                low,
                close,
                volume,
            },
            closed,
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
            if intent.time > 0 {
                intent.time == open_time
            } else {
                intent.bar_index == bar_index
            }
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

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_period_duration_millis_mappings() {
        assert_eq!(period_duration_millis("1m"), 60_000);
        assert_eq!(period_duration_millis("5m"), 300_000);
        assert_eq!(period_duration_millis("15m"), 900_000);
        assert_eq!(period_duration_millis("1h"), 3_600_000);
        assert_eq!(period_duration_millis("60m"), 3_600_000);
        assert_eq!(period_duration_millis("1d"), 86_400_000);
    }

    #[test]
    fn test_parse_strategy_candles_computes_close_time_and_honors_explicit_closed() {
        let payload = json!({
            "candles": [
                {
                    "at": "2026-09-08T09:30:00Z",
                    "open": 100.0,
                    "high": 105.0,
                    "low": 99.0,
                    "close": 104.0,
                    "volume": 1000.0,
                    "period": "1m",
                    "closed": true
                },
                {
                    "at": "2026-09-08T09:31:00Z",
                    "open": 104.0,
                    "high": 106.0,
                    "low": 103.0,
                    "close": 105.5,
                    "volume": 500.0,
                    "period": "1m",
                    "closed": false
                }
            ]
        });

        let candles = parse_strategy_candles(&payload, Some("1m")).expect("parse candles");
        assert_eq!(candles.len(), 2);

        // Bar 0 is closed
        assert_eq!(candles[0].open_time, 1788859800000);
        assert_eq!(candles[0].close_time, 1788859800000 + 60_000);
        assert!(candles[0].closed);

        // Bar 1 is unclosed (in-progress)
        assert_eq!(candles[1].open_time, 1788859860000);
        assert_eq!(candles[1].close_time, 1788859860000 + 60_000);
        assert!(!candles[1].closed);
    }

    #[test]
    fn test_parse_strategy_candles_prior_bars_are_always_closed() {
        let payload = json!({
            "candles": [
                {
                    "at": "2020-01-01T00:00:00Z",
                    "open": 10.0,
                    "high": 12.0,
                    "low": 9.0,
                    "close": 11.0,
                    "volume": 100.0
                },
                {
                    "at": "2020-01-01T00:01:00Z",
                    "open": 11.0,
                    "high": 13.0,
                    "low": 10.0,
                    "close": 12.0,
                    "volume": 150.0
                }
            ]
        });

        let candles = parse_strategy_candles(&payload, Some("1m")).expect("parse candles");
        assert_eq!(candles.len(), 2);
        // Bar 0 followed by Bar 1 must be closed
        assert!(candles[0].closed);
        // Bar 1 close_time is far in the past (2020), so it's also closed
        assert!(candles[1].closed);
    }

    #[test]
    fn test_parse_strategy_candles_detects_future_bar_as_unclosed_when_flag_omitted() {
        let future_time = time::OffsetDateTime::now_utc() + time::Duration::minutes(5);
        let future_at = future_time
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap();

        let payload = json!({
            "candles": [
                {
                    "at": future_at,
                    "open": 50.0,
                    "high": 52.0,
                    "low": 49.0,
                    "close": 51.0,
                    "volume": 200.0,
                    "period": "5m"
                }
            ]
        });

        let candles = parse_strategy_candles(&payload, Some("5m")).expect("parse candles");
        assert_eq!(candles.len(), 1);
        // Future bar has close_time in the future, so closed is false
        assert!(!candles[0].closed);
    }
}
