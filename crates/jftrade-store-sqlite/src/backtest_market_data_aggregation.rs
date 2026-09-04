//! Aggregation helpers for backtest market data.

use std::collections::BTreeMap;

use jftrade_kernel::Fixed8;
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

pub(crate) fn aggregate_range(
    connection: &Connection,
    base_table: &str,
    source_minutes: i64,
    symbol: &str,
    interval: &str,
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
    let first_bucket = floor_div(start_time_ms, target_ms).saturating_mul(target_ms);
    let last_bucket = floor_div(end_time_ms.saturating_sub(1), target_ms).saturating_mul(target_ms);
    if first_bucket > last_bucket {
        return Ok(Vec::new());
    }
    let source_end = last_bucket.saturating_add(target_ms);
    let source = read_direct_range(connection, base_table, first_bucket, source_end)?;
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
        let bucket = floor_div(candle.start_time, target_ms).saturating_mul(target_ms);
        by_bucket.entry(bucket).or_default().push(candle);
    }

    let mut aggregated = Vec::new();
    let mut bucket = first_bucket;
    while bucket <= last_bucket {
        let rows = by_bucket.get(&bucket).ok_or_else(|| {
            missing_coverage(symbol, interval, bucket, bucket.saturating_add(target_ms))
        })?;
        aggregated.push(aggregate_bucket(
            symbol, interval, bucket, target_ms, source_ms, rows,
        )?);
        bucket = bucket.saturating_add(target_ms);
    }
    Ok(aggregated)
}

pub(crate) fn aggregate_bucket(
    symbol: &str,
    interval: &str,
    bucket_start: i64,
    bucket_ms: i64,
    source_ms: i64,
    rows: &[StoredBacktestCandle],
) -> Result<StoredBacktestCandle, BacktestMarketDataStoreError> {
    let factor = usize::try_from(bucket_ms / source_ms).unwrap_or(0);
    if rows.len() != factor {
        return Err(missing_coverage(
            symbol,
            interval,
            bucket_start,
            bucket_start.saturating_add(bucket_ms),
        ));
    }
    let mut ordered = rows.to_vec();
    ordered.sort_by_key(|candle| candle.start_time);
    let mut values = Vec::with_capacity(factor);
    for (index, candle) in ordered.iter().enumerate() {
        let expected_start = bucket_start.saturating_add((index as i64).saturating_mul(source_ms));
        let expected_end = expected_start.saturating_add(source_ms).saturating_sub(1);
        if candle.start_time != expected_start || candle.end_time != expected_end {
            return Err(missing_coverage(
                symbol,
                interval,
                bucket_start,
                bucket_start.saturating_add(bucket_ms),
            ));
        }
        values.push((
            parse_fixed("open", &candle.open)?,
            parse_fixed("high", &candle.high)?,
            parse_fixed("low", &candle.low)?,
            parse_fixed("close", &candle.close)?,
            parse_fixed("volume", &candle.volume)?,
        ));
    }
    let first = values.first().ok_or_else(|| {
        missing_coverage(
            symbol,
            interval,
            bucket_start,
            bucket_start.saturating_add(bucket_ms),
        )
    })?;
    let last = values.last().ok_or_else(|| {
        missing_coverage(
            symbol,
            interval,
            bucket_start,
            bucket_start.saturating_add(bucket_ms),
        )
    })?;
    let high = values.iter().map(|value| value.1).max().unwrap_or(first.1);
    let low = values.iter().map(|value| value.2).min().unwrap_or(first.2);
    let volume = values.iter().try_fold(Fixed8::ZERO, |sum, value| {
        sum.checked_add(value.4)
            .map_err(|error| BacktestMarketDataStoreError::Validation(format!("volume: {error}")))
    })?;
    Ok(StoredBacktestCandle {
        start_time: bucket_start,
        end_time: bucket_start.saturating_add(bucket_ms).saturating_sub(1),
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
