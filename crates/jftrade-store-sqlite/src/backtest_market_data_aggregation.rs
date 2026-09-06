//! Aggregation helpers for backtest market data.

use std::collections::BTreeMap;

use jftrade_kernel::Fixed8;
use jiff::civil::Date;
use jiff::tz::TimeZone;
use rusqlite::Connection;

use super::{BacktestMarketDataStoreError, StoredBacktestCandle, read_direct_range, table_exists};

pub(crate) fn interval_minutes(interval: &str) -> Option<i64> {
    match interval.trim().to_ascii_lowercase().as_str() {
        "1m" | "1min" => Some(1),
        "5m" | "5min" => Some(5),
        "15m" | "15min" => Some(15),
        "30m" | "30min" => Some(30),
        "60m" | "60min" | "1h" | "1hour" => Some(60),
        _ => None,
    }
}

pub(crate) fn interval_duration_ms(interval: &str) -> Option<i64> {
    interval_minutes(interval).map(|minutes| minutes.saturating_mul(60_000))
}

pub(crate) fn is_aggregate_interval(interval: &str) -> bool {
    matches!(interval_minutes(interval), Some(5 | 15 | 30 | 60))
}

pub(crate) fn aggregation_candidate_intervals(target_interval: &str) -> Vec<(&'static str, i64)> {
    const CANDIDATES: [(&str, i64); 4] = [("30m", 30), ("15m", 15), ("5m", 5), ("1m", 1)];
    let Some(target_min) = interval_minutes(target_interval) else {
        return Vec::new();
    };
    CANDIDATES
        .iter()
        .copied()
        .filter(|&(_, min)| min < target_min && target_min % min == 0)
        .collect()
}

pub(crate) fn normalize_limit(limit: usize) -> usize {
    limit.max(1)
}

fn market_from_symbol(symbol: &str) -> Option<&'static str> {
    let upper = symbol.trim().to_ascii_uppercase();
    if upper.starts_with("US.") {
        Some("US")
    } else if upper.starts_with("HK.") {
        Some("HK")
    } else if upper.starts_with("CN.") || upper.starts_with("SH.") || upper.starts_with("SZ.") {
        Some("CN")
    } else {
        None
    }
}

fn market_timezone(market: &str) -> &'static str {
    match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "CN" => "Asia/Shanghai",
        _ => "UTC",
    }
}

fn is_black_friday(date: Date) -> bool {
    date.month() == 11
        && date.weekday() == jiff::civil::Weekday::Friday
        && (23..=29).contains(&date.day())
}

fn is_christmas_eve_early_close(date: Date) -> bool {
    date.month() == 12
        && date.day() == 24
        && !matches!(
            date.weekday(),
            jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
        )
}

fn is_independence_day_early_close(date: Date) -> bool {
    date.month() == 7
        && date.day() == 3
        && matches!(
            date.weekday(),
            jiff::civil::Weekday::Monday
                | jiff::civil::Weekday::Tuesday
                | jiff::civil::Weekday::Wednesday
                | jiff::civil::Weekday::Thursday
        )
}

fn market_session_windows(market: &str, date: Date, session_scope: &str) -> Vec<(i32, i32)> {
    match market {
        "US" => {
            let early_close = is_black_friday(date)
                || is_christmas_eve_early_close(date)
                || is_independence_day_early_close(date);
            let regular_end = if early_close { 780 } else { 960 };
            let after_end = if early_close { 1080 } else { 1200 };
            match session_scope.trim().to_ascii_lowercase().as_str() {
                "regular" => vec![(570, regular_end)],
                "extended" => vec![
                    (0, 240),
                    (240, 570),
                    (570, regular_end),
                    (regular_end, after_end),
                ],
                _ => vec![(570, regular_end)],
            }
        }
        "HK" => vec![(570, 720), (780, 960)],
        "CN" => vec![(570, 690), (780, 900)],
        _ => Vec::new(),
    }
}

pub(crate) fn resolve_aggregation_buckets(
    symbol: &str,
    session_scope: &str,
    target_ms: i64,
    start_time_ms: i64,
    end_time_ms: i64,
) -> Vec<(i64, i64)> {
    if start_time_ms >= end_time_ms || target_ms <= 0 {
        return Vec::new();
    }
    if let Some(market) = market_from_symbol(symbol) {
        let tz_name = market_timezone(market);
        if let Ok(tz) = TimeZone::get(tz_name) {
            let start_ts = jiff::Timestamp::from_millisecond(start_time_ms).ok();
            let end_ts = jiff::Timestamp::from_millisecond(end_time_ms.saturating_sub(1)).ok();
            if let (Some(s_ts), Some(e_ts)) = (start_ts, end_ts) {
                let start_date = s_ts.to_zoned(tz.clone()).date();
                let end_date = e_ts.to_zoned(tz.clone()).date();
                let mut buckets = Vec::new();
                let mut intersects_session = false;
                let mut current_date = start_date;
                while current_date <= end_date {
                    if !matches!(
                        current_date.weekday(),
                        jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
                    ) {
                        for (start_min, end_min) in
                            market_session_windows(market, current_date, session_scope)
                        {
                            let s_hour = (start_min / 60) as i8;
                            let s_min = (start_min % 60) as i8;
                            let e_hour = (end_min / 60) as i8;
                            let e_min = (end_min % 60) as i8;

                            let s_ms = current_date
                                .at(s_hour, s_min, 0, 0)
                                .in_tz(tz_name)
                                .map(|z| z.timestamp().as_millisecond());
                            let e_ms = current_date
                                .at(e_hour, e_min, 0, 0)
                                .in_tz(tz_name)
                                .map(|z| z.timestamp().as_millisecond());
                            if let (Ok(session_start), Ok(session_end)) = (s_ms, e_ms)
                                && session_end > start_time_ms
                                && session_start < end_time_ms
                            {
                                intersects_session = true;
                                let mut cursor = session_start;
                                while cursor < session_end {
                                    let b_start = cursor;
                                    let b_end = cursor.saturating_add(target_ms).min(session_end);
                                    // Only a real session boundary may shorten a bar.
                                    // A query cutoff does not close the current bucket.
                                    if b_end <= end_time_ms
                                        && b_start >= start_time_ms
                                        && b_start < end_time_ms
                                    {
                                        buckets.push((b_start, b_end));
                                    }
                                    cursor = cursor.saturating_add(target_ms);
                                }
                            }
                        }
                    }
                    let next = match current_date.tomorrow() {
                        Ok(d) => d,
                        Err(_) => break,
                    };
                    current_date = next;
                }
                if intersects_session {
                    return buckets;
                }
            }
        }
    }

    // Generic UTC fallback (e.g. mock timestamps or unknown symbols)
    let first_bucket = floor_div(start_time_ms, target_ms).saturating_mul(target_ms);
    let last_bucket = floor_div(end_time_ms.saturating_sub(1), target_ms).saturating_mul(target_ms);
    if first_bucket > last_bucket {
        return Vec::new();
    }
    let mut buckets = Vec::new();
    let mut bucket = first_bucket;
    while bucket <= last_bucket {
        let b_end = bucket.saturating_add(target_ms);
        buckets.push((bucket, b_end));
        bucket = b_end;
    }
    buckets
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn aggregate_range(
    connection: &Connection,
    base_table: &str,
    source_minutes: i64,
    symbol: &str,
    interval: &str,
    session_scope: &str,
    start_time_ms: i64,
    end_time_ms: i64,
) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
    let target_minutes = match interval_minutes(interval) {
        Some(minutes) if minutes > source_minutes && minutes % source_minutes == 0 => minutes,
        _ => {
            return Err(BacktestMarketDataStoreError::Validation(format!(
                "unsupported aggregate interval: {interval}"
            )));
        }
    };
    if !table_exists(connection, base_table)? {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }
    let target_ms = target_minutes.saturating_mul(60_000);
    let source_ms = source_minutes.saturating_mul(60_000);

    let buckets =
        resolve_aggregation_buckets(symbol, session_scope, target_ms, start_time_ms, end_time_ms);
    if buckets.is_empty() {
        return Ok(Vec::new());
    }

    let source_start = buckets.first().map(|b| b.0).unwrap_or(start_time_ms);
    let source_end = buckets.last().map(|b| b.1).unwrap_or(end_time_ms);
    let source = read_direct_range(connection, base_table, source_start, source_end)?;
    if source.is_empty() {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }

    let mut by_bucket: BTreeMap<i64, Vec<StoredBacktestCandle>> = BTreeMap::new();
    for candle in source {
        if let Ok(idx) = buckets.binary_search_by(|&(b_start, b_end)| {
            if candle.start_time < b_start {
                std::cmp::Ordering::Greater
            } else if candle.start_time >= b_end {
                std::cmp::Ordering::Less
            } else {
                std::cmp::Ordering::Equal
            }
        }) {
            by_bucket.entry(buckets[idx].0).or_default().push(candle);
        }
    }

    let mut aggregated = Vec::with_capacity(buckets.len());
    for &(bucket_start, bucket_end) in &buckets {
        let rows = by_bucket
            .get(&bucket_start)
            .ok_or_else(|| missing_coverage(symbol, interval, bucket_start, bucket_end))?;
        aggregated.push(aggregate_bucket(
            symbol,
            interval,
            bucket_start,
            bucket_end,
            source_ms,
            rows,
        )?);
    }
    Ok(aggregated)
}

pub(crate) fn aggregate_bucket(
    symbol: &str,
    interval: &str,
    bucket_start: i64,
    bucket_end: i64,
    source_ms: i64,
    rows: &[StoredBacktestCandle],
) -> Result<StoredBacktestCandle, BacktestMarketDataStoreError> {
    let bucket_ms = bucket_end.saturating_sub(bucket_start);
    let factor = usize::try_from(bucket_ms / source_ms).unwrap_or(0);
    if factor == 0 || rows.len() != factor {
        return Err(missing_coverage(symbol, interval, bucket_start, bucket_end));
    }
    let mut ordered = rows.to_vec();
    ordered.sort_by_key(|candle| candle.start_time);
    let mut values = Vec::with_capacity(factor);
    for (index, candle) in ordered.iter().enumerate() {
        let expected_start = bucket_start.saturating_add((index as i64).saturating_mul(source_ms));
        let expected_end = expected_start.saturating_add(source_ms).saturating_sub(1);
        if candle.start_time != expected_start || candle.end_time != expected_end {
            return Err(missing_coverage(symbol, interval, bucket_start, bucket_end));
        }
        values.push((
            parse_fixed("open", &candle.open)?,
            parse_fixed("high", &candle.high)?,
            parse_fixed("low", &candle.low)?,
            parse_fixed("close", &candle.close)?,
            parse_fixed("volume", &candle.volume)?,
        ));
    }
    let first = values
        .first()
        .ok_or_else(|| missing_coverage(symbol, interval, bucket_start, bucket_end))?;
    let last = values
        .last()
        .ok_or_else(|| missing_coverage(symbol, interval, bucket_start, bucket_end))?;
    let high = values.iter().map(|value| value.1).max().unwrap_or(first.1);
    let low = values.iter().map(|value| value.2).min().unwrap_or(first.2);
    let volume = values.iter().try_fold(Fixed8::ZERO, |sum, value| {
        sum.checked_add(value.4)
            .map_err(|error| BacktestMarketDataStoreError::Validation(format!("volume: {error}")))
    })?;
    Ok(StoredBacktestCandle {
        start_time: bucket_start,
        end_time: bucket_end.saturating_sub(1),
        open: first.0.storage_text(),
        high: high.storage_text(),
        low: low.storage_text(),
        close: last.3.storage_text(),
        volume: volume.storage_text(),
    })
}

fn parse_fixed(name: &str, value: &str) -> Result<Fixed8, BacktestMarketDataStoreError> {
    value
        .parse::<Fixed8>()
        .map_err(|error| BacktestMarketDataStoreError::Validation(format!("{name}: {error}")))
}

pub(crate) fn missing_coverage(
    symbol: &str,
    interval: &str,
    start_time_ms: i64,
    end_time_ms: i64,
) -> BacktestMarketDataStoreError {
    BacktestMarketDataStoreError::Coverage(format!(
        "missing {interval} coverage for {symbol} [{start_time_ms}, {end_time_ms})"
    ))
}

pub(crate) fn floor_div(value: i64, divisor: i64) -> i64 {
    value.div_euclid(divisor)
}
