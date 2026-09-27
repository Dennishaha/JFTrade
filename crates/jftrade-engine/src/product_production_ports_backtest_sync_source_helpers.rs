//! Provider-specific page fetching and candle conversion for backtest sync.

use std::sync::Arc;

use jftrade_integration_futu::{
    HistoricalKline, HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineResult,
};
use jftrade_integration_marketdata_helper::{HelperCandlesResponse, HelperClient};
use jftrade_store_sqlite::{BacktestSyncTaskStore, StoredBacktestCandle, StoredBacktestSyncTask};

use super::super::product_backtest_sync_request::{SyncRequest, parse_timestamp};
use super::sync_helpers::{is_cancelled, persist_task};
use crate::product::product_production_ports::SharedTradeReadRuntime;

#[allow(clippy::too_many_arguments)]
pub(super) async fn fetch_futu_page_with_retry(
    tasks: &Arc<BacktestSyncTaskStore>,
    runtime: &SharedTradeReadRuntime,
    market: i32,
    code: &str,
    interval: &str,
    request: &SyncRequest,
    cursor: &[u8],
    task_id: &str,
    task: &mut StoredBacktestSyncTask,
) -> Result<HistoricalKlineResult, String> {
    let begin_time = opend_wall_clock(&request.since, &request.market)?;
    let end_time = opend_wall_clock(&request.until, &request.market)?;
    let query = HistoricalKlineQuery {
        market,
        symbol: code.to_owned(),
        period: interval.to_owned(),
        adjustment: match request.rehab_type.as_str() {
            "none" => 0,
            "backward" => 2,
            _ => 1,
        },
        begin_time,
        end_time,
        max_ack_kl_num: Some(1000),
        next_req_key: cursor.to_vec(),
        extended_time: (request.session_scope == "extended").then_some(true),
        session: (request.session_scope == "extended").then_some(3),
    };
    let mut last_error = None;
    for attempt in 0..4 {
        if is_cancelled(tasks, task_id)? {
            return Err("sync cancelled".to_owned());
        }
        let Some(reader) = runtime.historical_klines_reader() else {
            return Err("Futu historical candle sync is unavailable".to_owned());
        };
        let call_query = query.clone();
        let joined = tokio::task::spawn_blocking(move || reader.query(&call_query));
        let outcome = tokio::time::timeout(std::time::Duration::from_secs(30), joined).await;
        match outcome {
            Ok(Ok(Ok(page))) => return Ok(page),
            Ok(Ok(Err(error))) => {
                let retryable = futu_error_retryable(&error);
                last_error = Some(error.to_string());
                if !retryable || attempt == 3 {
                    break;
                }
            }
            Ok(Err(error)) => {
                last_error = Some(format!("OpenD historical worker failed: {error}"));
                if attempt == 3 {
                    break;
                }
            }
            Err(_) => {
                last_error = Some("OpenD historical request timed out".to_owned());
                if attempt == 3 {
                    break;
                }
            }
        }
        task.retries += 1;
        persist_task(tasks, task, "running", None)?;
        tokio::time::sleep(std::time::Duration::from_millis(250 * (attempt as u64 + 1))).await;
    }
    Err(last_error.unwrap_or_else(|| "OpenD historical request failed".to_owned()))
}

fn futu_error_retryable(error: &HistoricalKlineError) -> bool {
    match error {
        HistoricalKlineError::Session(_) => true,
        HistoricalKlineError::Rejected { err_code, .. } => {
            matches!(err_code, 408 | 425 | 429 | 500..)
        }
        HistoricalKlineError::Decode(_) | HistoricalKlineError::InvalidPagination => false,
    }
}

pub(super) fn validate_futu_page(
    page: &HistoricalKlineResult,
    market: i32,
    code: &str,
    interval: &str,
) -> Result<(), String> {
    if page.security.market != market || !page.security.code.eq_ignore_ascii_case(code) {
        return Err("OpenD historical response identity is invalid".to_owned());
    }
    if page
        .klines
        .iter()
        .any(|candle| candle.time.trim().is_empty())
    {
        return Err("OpenD historical response contains an empty candle timestamp".to_owned());
    }
    if !matches!(
        interval,
        "1m" | "5m" | "15m" | "30m" | "1h" | "1d" | "1w" | "1mo"
    ) {
        return Err(format!("OpenD does not support interval {interval}"));
    }
    Ok(())
}

pub(super) fn futu_rows(
    candles: &[HistoricalKline],
    _symbol: &str,
    interval: &str,
    since: time::OffsetDateTime,
    until: time::OffsetDateTime,
    market: i32,
) -> Result<Vec<StoredBacktestCandle>, String> {
    let market_label = futu_market_label(market);
    let mut rows = Vec::with_capacity(candles.len());
    for candle in candles {
        if candle.is_blank {
            continue;
        }
        let at = parse_futu_candle_time(&candle.time, market_label)?;
        if at < since || at >= until {
            continue;
        }
        let (Some(open), Some(high), Some(low), Some(close)) = (
            candle.open_price,
            candle.high_price,
            candle.low_price,
            candle.close_price,
        ) else {
            return Err("OpenD historical candle is missing OHLC values".to_owned());
        };
        rows.push(StoredBacktestCandle {
            start_time: at.unix_timestamp_nanos() as i64 / 1_000_000,
            end_time: (at + interval_duration(interval) - time::Duration::milliseconds(1))
                .unix_timestamp_nanos() as i64
                / 1_000_000,
            open: futu_decimal(open, "open")?,
            high: futu_decimal(high, "high")?,
            low: futu_decimal(low, "low")?,
            close: futu_decimal(close, "close")?,
            volume: candle.volume.unwrap_or_default().to_string(),
        });
    }
    rows.sort_by_key(|row| row.start_time);
    Ok(rows)
}

fn futu_decimal(value: f64, field: &str) -> Result<String, String> {
    if !value.is_finite() {
        return Err(format!("OpenD historical {field} is not finite"));
    }
    Ok(format!("{value:.8}"))
}

pub(super) fn futu_market_code(symbol: &str) -> Result<i32, String> {
    match symbol.split_once('.').map(|(market, _)| market) {
        Some("HK") => Ok(1),
        Some("US") => Ok(11),
        Some("SH") => Ok(21),
        Some("SZ") => Ok(22),
        _ => Err("Futu historical sync requires a supported exchange-qualified symbol".to_owned()),
    }
}

fn futu_market_label(market: i32) -> &'static str {
    match market {
        11 => "US",
        21 => "SH",
        22 => "SZ",
        _ => "HK",
    }
}

fn futu_timezone(market: &str) -> &'static str {
    match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        _ => "UTC",
    }
}

fn opend_wall_clock(value: &str, market: &str) -> Result<String, String> {
    let timestamp: jiff::Timestamp = value
        .parse()
        .map_err(|error| format!("invalid timestamp for OpenD: {error}"))?;
    let local = timestamp
        .to_zoned(jiff::tz::TimeZone::get(futu_timezone(market)).map_err(|e| e.to_string())?);
    Ok(local.strftime("%Y-%m-%d %H:%M:%S").to_string())
}

fn parse_futu_candle_time(value: &str, market: &str) -> Result<time::OffsetDateTime, String> {
    if value.contains('T') || value.ends_with('Z') || value.contains('+') {
        return parse_timestamp(value);
    }
    let local = jiff::civil::DateTime::strptime("%Y-%m-%d %H:%M:%S", value.trim())
        .map_err(|error| format!("invalid OpenD candle timestamp: {error}"))?;
    let zoned = local
        .in_tz(futu_timezone(market))
        .map_err(|error| format!("invalid OpenD candle timestamp: {error}"))?;
    parse_timestamp(&zoned.timestamp().to_string())
}

#[allow(clippy::too_many_arguments)]
pub(super) async fn fetch_helper_page_with_retry(
    tasks: &Arc<BacktestSyncTaskStore>,
    helper: &HelperClient,
    provider: &str,
    segments: &[&str],
    query: &[(&str, &str)],
    task_id: &str,
    task: &mut StoredBacktestSyncTask,
) -> Result<HelperCandlesResponse, String> {
    let mut last_error = None;
    for attempt in 0..4 {
        if is_cancelled(tasks, task_id)? {
            return Err("sync cancelled".to_owned());
        }
        match helper
            .get_provider_json_with_query(provider, segments, query)
            .await
        {
            Ok(response) => return Ok(response),
            Err(error) => {
                let retryable = is_retryable_helper_error(&error);
                last_error = Some(error.to_string());
                if !retryable || attempt == 3 {
                    break;
                }
                task.retries += 1;
                persist_task(tasks, task, "running", None)?;
                let delay = std::time::Duration::from_millis(250 * (attempt as u64 + 1));
                tokio::time::sleep(delay).await;
            }
        }
    }
    Err(last_error.unwrap_or_else(|| "helper request failed".to_owned()))
}

fn is_retryable_helper_error(
    error: &jftrade_integration_marketdata_helper::HttpAdapterError,
) -> bool {
    use jftrade_integration_marketdata_helper::HttpAdapterError;

    match error {
        HttpAdapterError::Timeout | HttpAdapterError::Unavailable(_) => true,
        HttpAdapterError::Remote { status, .. } => {
            *status == 408 || *status == 425 || *status == 429 || *status >= 500
        }
        HttpAdapterError::InvalidUrl(_)
        | HttpAdapterError::WeakToken
        | HttpAdapterError::InvalidResponse(_) => false,
    }
}

pub(super) fn symbol_code(symbol: &str) -> &str {
    symbol.split_once('.').map_or(symbol, |(_, code)| code)
}

pub(super) fn symbol_market(symbol: &str) -> &str {
    symbol.split_once('.').map_or("", |(market, _)| market)
}

pub(super) fn interval_duration(interval: &str) -> time::Duration {
    match interval {
        "1m" => time::Duration::minutes(1),
        "5m" => time::Duration::minutes(5),
        "15m" => time::Duration::minutes(15),
        "30m" => time::Duration::minutes(30),
        "1h" => time::Duration::hours(1),
        "1w" => time::Duration::days(7),
        "1mo" => time::Duration::days(30),
        _ => time::Duration::days(1),
    }
}

pub(super) fn validate_helper_page(
    response: &HelperCandlesResponse,
    market: &str,
    symbol: &str,
    interval: &str,
) -> Result<(), String> {
    let expected_instrument = format!("{market}.{}", symbol_code(symbol));
    if !response.market.eq_ignore_ascii_case(market)
        || !response.symbol.eq_ignore_ascii_case(symbol_code(symbol))
        || !response
            .instrument_id
            .eq_ignore_ascii_case(&expected_instrument)
        || response.period != interval
        || response.total_returned != response.candles.len()
    {
        return Err("helper candle response identity is invalid".to_owned());
    }
    if response.has_more && response.candles.is_empty() {
        return Err("helper returned hasMore with an empty candle page".to_owned());
    }
    let now = time::OffsetDateTime::now_utc();
    let mut previous = None;
    for candle in &response.candles {
        let at = parse_timestamp(&candle.at)?;
        if at.unix_timestamp() < 0 || at > (now + time::Duration::days(1)) {
            return Err("helper candle timestamp is outside the supported range".to_owned());
        }
        if previous.is_some_and(|p| at <= p) {
            return Err("helper candle timestamps are not strictly increasing".to_owned());
        }
        previous = Some(at);
    }
    Ok(())
}
