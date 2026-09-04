//! Filtering, validation, and resolution downsampling for ResultView projection.

use std::time::Duration;

use serde_json::{Value, json};

use super::NormalizedBacktestData;
use crate::product::{BacktestResultViewError, BacktestResultViewRequest};

pub(crate) struct ValidatedResultViewParams {
    pub view: String,
    pub offset: usize,
    pub limit: usize,
    pub resolution: Option<String>,
}

pub(crate) struct ChartWindowArgs<'a> {
    pub start_time: Option<&'a str>,
    pub end_time: Option<&'a str>,
    pub offset: usize,
    pub limit: usize,
}

pub(crate) fn validate_result_view_request(
    request: &BacktestResultViewRequest,
) -> Result<ValidatedResultViewParams, BacktestResultViewError> {
    let view = request.view.as_deref().unwrap_or("summary");
    if !matches!(
        view,
        "summary" | "chart" | "orders" | "logs" | "warnings" | "errors"
    ) {
        return Err(BacktestResultViewError::Invalid(format!(
            "view must be summary, chart, orders, logs, warnings, or errors, got {view}"
        )));
    }

    if let Some(includes) = &request.include {
        if view != "chart" && !includes.is_empty() {
            return Err(BacktestResultViewError::Invalid(
                "include parameter is only supported for chart view".to_owned(),
            ));
        }
        for inc in includes {
            if !matches!(
                inc.as_str(),
                "candles" | "trades" | "pnlCurve" | "drawdownCurve"
            ) {
                return Err(BacktestResultViewError::Invalid(format!(
                    "unsupported include series: {inc}"
                )));
            }
        }
    }

    let start_nanos = request
        .start_time
        .as_deref()
        .map(validate_rfc3339_timestamp)
        .transpose()?;
    let end_nanos = request
        .end_time
        .as_deref()
        .map(validate_rfc3339_timestamp)
        .transpose()?;

    if let (Some(sn), Some(en)) = (start_nanos, end_nanos)
        && en < sn
    {
        return Err(BacktestResultViewError::Invalid(
            "endTime must not be earlier than startTime".to_owned(),
        ));
    }

    let offset = match &request.cursor {
        Some(c) => c.parse::<usize>().map_err(|_| {
            BacktestResultViewError::Invalid(format!(
                "cursor must be a non-negative integer, got {c}"
            ))
        })?,
        None => 0,
    };

    let limit = match request.limit {
        Some(lim) => {
            if lim == 0 || lim > 2000 {
                return Err(BacktestResultViewError::Invalid(format!(
                    "limit must be between 1 and 2000, got {lim}"
                )));
            }
            lim
        }
        None => 500,
    };

    let resolution = match &request.resolution {
        Some(res) => {
            let s = res.trim();
            if s.is_empty() || s.eq_ignore_ascii_case("auto") {
                Some("auto".to_owned())
            } else if parse_interval_duration(s).is_some() {
                Some(s.to_owned())
            } else {
                return Err(BacktestResultViewError::Invalid(format!(
                    "unsupported resolution: {res}"
                )));
            }
        }
        None => None,
    };

    Ok(ValidatedResultViewParams {
        view: view.to_owned(),
        offset,
        limit,
        resolution,
    })
}

pub(crate) fn validate_rfc3339_timestamp(s: &str) -> Result<i128, BacktestResultViewError> {
    parse_rfc3339_nanos(s)
        .ok_or_else(|| BacktestResultViewError::Invalid(format!("invalid RFC3339 timestamp '{s}'")))
}

pub(crate) fn parse_interval_duration(value: &str) -> Option<Duration> {
    let s = value.trim().to_ascii_lowercase();
    if s.is_empty() {
        return None;
    }
    let (num_str, unit) = if let Some(stripped) = s.strip_suffix("min") {
        (stripped, 'm')
    } else if let Some(stripped) = s.strip_suffix("sec") {
        (stripped, 's')
    } else {
        let last = s.chars().last()?;
        if last.is_ascii_digit() {
            (s.as_str(), 'm')
        } else {
            (&s[..s.len() - last.len_utf8()], last)
        }
    };
    let num: u64 = num_str.parse().ok()?;
    if num == 0 {
        return None;
    }
    match unit {
        's' => Some(Duration::from_secs(num)),
        'm' => Some(Duration::from_secs(num * 60)),
        'h' => Some(Duration::from_secs(num * 3600)),
        'd' => Some(Duration::from_secs(num * 86400)),
        'w' => Some(Duration::from_secs(num * 7 * 86400)),
        _ => None,
    }
}

pub(crate) fn choose_result_view_auto_resolution(
    native: Duration,
    count: usize,
    limit: usize,
) -> Duration {
    if count <= limit || limit == 0 {
        return native;
    }
    let mult = (count as f64 / limit as f64).ceil() as u64;
    let required = native.saturating_mul(mult as u32);
    let candidates = [
        Duration::from_secs(60),
        Duration::from_secs(300),
        Duration::from_secs(900),
        Duration::from_secs(1800),
        Duration::from_secs(3600),
        Duration::from_secs(7200),
        Duration::from_secs(14400),
        Duration::from_secs(86400),
        Duration::from_secs(7 * 86400),
    ];
    for candidate in candidates {
        if candidate >= native && candidate >= required {
            return candidate;
        }
    }
    required
}

pub(crate) fn result_view_resolution_label(duration: Duration) -> String {
    let secs = duration.as_secs();
    const WEEK: u64 = 7 * 86400;
    const DAY: u64 = 86400;
    const HOUR: u64 = 3600;
    const MINUTE: u64 = 60;
    if secs.is_multiple_of(WEEK) {
        format!("{}w", secs / WEEK)
    } else if secs.is_multiple_of(DAY) {
        format!("{}d", secs / DAY)
    } else if secs.is_multiple_of(HOUR) {
        format!("{}h", secs / HOUR)
    } else if secs.is_multiple_of(MINUTE) {
        format!("{}m", secs / MINUTE)
    } else {
        format!("{secs}s")
    }
}

pub(crate) fn result_view_candles(
    candles: &[Value],
    native_interval: &str,
    requested_resolution: Option<&str>,
    start_time: Option<&str>,
    end_time: Option<&str>,
    limit: usize,
) -> Result<(String, Vec<Value>), BacktestResultViewError> {
    let filtered = filter_timed_items(candles, "time", start_time, end_time);
    let native_duration =
        parse_interval_duration(native_interval).unwrap_or(Duration::from_secs(60));
    let norm_res = requested_resolution.map(|s| s.trim().to_ascii_lowercase());
    let target_duration = match norm_res.as_deref() {
        None | Some("") | Some("auto") => {
            choose_result_view_auto_resolution(native_duration, filtered.len(), limit)
        }
        Some(res) => {
            let d = parse_interval_duration(res).ok_or_else(|| {
                BacktestResultViewError::Invalid(format!("unsupported resolution: {res}"))
            })?;
            if d < native_duration {
                return Err(BacktestResultViewError::Invalid(format!(
                    "resolution {res} is finer than native interval {native_interval}"
                )));
            }
            d
        }
    };
    let label = result_view_resolution_label(target_duration);
    if target_duration <= native_duration || filtered.is_empty() {
        return Ok((label, filtered));
    }
    Ok((label, aggregate_result_view_candles(&filtered, target_duration)))
}

pub(crate) fn aggregate_result_view_candles(candles: &[Value], resolution: Duration) -> Vec<Value> {
    if candles.is_empty() || resolution.is_zero() {
        return candles.to_vec();
    }
    let res_secs = resolution.as_secs() as i64;
    let mut out = Vec::with_capacity(candles.len());
    let mut current: Option<Value> = None;
    let mut current_bucket: i64 = -1;
    let mut high_val = f64::NEG_INFINITY;
    let mut low_val = f64::INFINITY;
    let mut volume_sum = 0.0f64;

    for candle in candles {
        let time_str = candle.get("time").and_then(Value::as_str).unwrap_or_default();
        let Some(nanos) = parse_rfc3339_nanos(time_str) else {
            continue;
        };
        let unix_sec = (nanos / 1_000_000_000) as i64;
        let bucket = unix_sec / res_secs;

        let c_open = parse_f64_helper(candle.get("open"));
        let c_high = parse_f64_helper(candle.get("high"));
        let c_low = parse_f64_helper(candle.get("low"));
        let c_close = parse_f64_helper(candle.get("close"));
        let c_vol = parse_f64_helper(candle.get("volume"));

        if current.is_none() || bucket != current_bucket {
            if let Some(mut prev) = current.take() {
                prev["high"] = json!(high_val);
                prev["low"] = json!(low_val);
                prev["volume"] = json!(volume_sum);
                out.push(prev);
            }
            let mut clone = candle.clone();
            let bucket_dt = time::OffsetDateTime::from_unix_timestamp(bucket * res_secs)
                .unwrap_or(time::OffsetDateTime::UNIX_EPOCH);
            let formatted_time = bucket_dt
                .format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_else(|_| time_str.to_owned());
            clone["time"] = Value::String(formatted_time);
            clone["open"] = json!(c_open);
            clone["close"] = json!(c_close);
            high_val = c_high;
            low_val = c_low;
            volume_sum = c_vol;
            current = Some(clone);
            current_bucket = bucket;
            continue;
        }

        if let Some(cur) = current.as_mut() {
            if c_high > high_val {
                high_val = c_high;
            }
            if c_low < low_val {
                low_val = c_low;
            }
            volume_sum += c_vol;
            cur["close"] = json!(c_close);
        }
    }
    if let Some(mut prev) = current {
        prev["high"] = json!(high_val);
        prev["low"] = json!(low_val);
        prev["volume"] = json!(volume_sum);
        out.push(prev);
    }
    out
}

fn parse_f64_helper(v: Option<&Value>) -> f64 {
    v.and_then(|val| {
        val.as_f64()
            .or_else(|| val.as_str().and_then(|s| s.parse::<f64>().ok()))
    })
    .unwrap_or(0.0)
}

pub(crate) fn slice_items(
    items: &[Value],
    offset: usize,
    limit: usize,
) -> (Vec<Value>, Option<String>) {
    if offset >= items.len() {
        return (Vec::new(), None);
    }
    let end = (offset + limit).min(items.len());
    let next = if end < items.len() {
        Some(end.to_string())
    } else {
        None
    };
    (items[offset..end].to_vec(), next)
}

pub(crate) fn parse_timestamp_nanos(val: Option<&Value>) -> Option<i128> {
    match val {
        Some(Value::Number(n)) => {
            let ms = n.as_i64()?;
            Some((ms as i128) * 1_000_000)
        }
        Some(Value::String(s)) => parse_rfc3339_nanos(s),
        _ => None,
    }
}

pub(crate) fn parse_rfc3339_nanos(s: &str) -> Option<i128> {
    if let Ok(dt) = time::OffsetDateTime::parse(s, &time::format_description::well_known::Rfc3339) {
        Some(dt.unix_timestamp_nanos())
    } else if let Ok(ms) = s.parse::<i64>() {
        Some((ms as i128) * 1_000_000)
    } else {
        None
    }
}

pub(crate) fn item_in_time_window(
    time_val: Option<&Value>,
    start_time: Option<&str>,
    end_time: Option<&str>,
) -> bool {
    if start_time.is_none() && end_time.is_none() {
        return true;
    }
    let Some(t_nanos) = parse_timestamp_nanos(time_val) else {
        return false;
    };
    if let Some(start) = start_time
        && let Some(start_nanos) = parse_rfc3339_nanos(start)
        && t_nanos < start_nanos
    {
        return false;
    }
    if let Some(end) = end_time
        && let Some(end_nanos) = parse_rfc3339_nanos(end)
        && t_nanos > end_nanos
    {
        return false;
    }
    true
}

pub(crate) fn filter_timed_items(
    items: &[Value],
    time_field: &str,
    start_time: Option<&str>,
    end_time: Option<&str>,
) -> Vec<Value> {
    if start_time.is_none() && end_time.is_none() {
        return items.to_vec();
    }
    items
        .iter()
        .filter(|item| {
            let t = item.get(time_field);
            item_in_time_window(t, start_time, end_time)
        })
        .cloned()
        .collect()
}

pub(crate) fn project_chart_series(
    data: &NormalizedBacktestData,
    include_set: &[&str],
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    let chart_keys: [(&str, &str, &[Value]); 4] = [
        ("candles", "time", &data.candles),
        ("trades", "time", &data.trades),
        ("pnlCurve", "time", &data.pnl_curve),
        ("drawdownCurve", "time", &data.drawdown_curve),
    ];
    for (key, time_field, items) in chart_keys {
        if !include_set.contains(&key) {
            continue;
        }
        let filtered = if key == "candles" {
            items.to_vec()
        } else {
            filter_timed_items(items, time_field, args.start_time, args.end_time)
        };
        let (sliced, next) = slice_items(&filtered, args.offset, args.limit);
        returned.insert(key.to_owned(), json!(sliced.len()));
        series.insert(key.to_owned(), Value::Array(sliced));
        if let Some(next_cursor) = next {
            window.insert("truncated".to_owned(), json!(true));
            if window
                .get("nextCursor")
                .and_then(Value::as_str)
                .unwrap_or("")
                .is_empty()
            {
                window.insert("nextCursor".to_owned(), Value::String(next_cursor));
            }
        }
    }
}

pub(crate) fn project_orders_view(
    orders: &[Value],
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    let filtered: Vec<Value> = orders
        .iter()
        .filter(|item| {
            let sub = item.get("submittedAt");
            let fil = item.get("filledAt");
            item_in_time_window(sub, args.start_time, args.end_time)
                || item_in_time_window(fil, args.start_time, args.end_time)
        })
        .cloned()
        .collect();
    let (sliced, next) = slice_items(&filtered, args.offset, args.limit);
    returned.insert("orderBook".to_owned(), json!(sliced.len()));
    series.insert("orderBook".to_owned(), Value::Array(sliced));
    if let Some(next_cursor) = next {
        window.insert("truncated".to_owned(), json!(true));
        window.insert("nextCursor".to_owned(), Value::String(next_cursor));
    }
}

pub(crate) fn project_text_series_view(
    items: &[Value],
    key: &str,
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    let filtered: Vec<Value> =
        if key == "logs" && (args.start_time.is_some() || args.end_time.is_some()) {
            items
                .iter()
                .filter(|item| {
                    let t = item
                        .get("timestamp")
                        .or_else(|| item.get("time"))
                        .or_else(|| item.get("at"));
                    item_in_time_window(t, args.start_time, args.end_time)
                })
                .cloned()
                .collect()
        } else {
            items.to_vec()
        };
    let (sliced, next) = slice_items(&filtered, args.offset, args.limit);
    returned.insert(key.to_owned(), json!(sliced.len()));
    series.insert(key.to_owned(), Value::Array(sliced));
    if let Some(next_cursor) = next {
        window.insert("truncated".to_owned(), json!(true));
        window.insert("nextCursor".to_owned(), Value::String(next_cursor));
    }
}

pub(crate) fn dispatch_view_projection(
    view: &str,
    data: &NormalizedBacktestData,
    options: Option<&Value>,
    args: &ChartWindowArgs<'_>,
    series: &mut serde_json::Map<String, Value>,
    returned: &mut serde_json::Map<String, Value>,
    window: &mut serde_json::Map<String, Value>,
) {
    match view {
        "chart" => {
            let default_includes = ["candles", "trades", "pnlCurve", "drawdownCurve"];
            let requested_includes = options
                .and_then(|o| o.get("include"))
                .and_then(Value::as_array)
                .map(|arr| arr.iter().filter_map(Value::as_str).collect::<Vec<&str>>());
            let include_refs = requested_includes
                .as_deref()
                .filter(|v| !v.is_empty())
                .unwrap_or(&default_includes);
            project_chart_series(data, include_refs, args, series, returned, window);
        }
        "orders" => project_orders_view(&data.orders, args, series, returned, window),
        "logs" => project_text_series_view(&data.logs, "logs", args, series, returned, window),
        "warnings" => {
            project_text_series_view(&data.warnings, "warnings", args, series, returned, window)
        }
        "errors" => project_text_series_view(
            &data.runtime_errors,
            "runtimeErrors",
            args,
            series,
            returned,
            window,
        ),
        _ => {}
    }
}
