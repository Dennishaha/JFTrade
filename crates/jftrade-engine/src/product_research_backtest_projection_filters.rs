//! Filtering, validation, and resolution downsampling for ResultView projection.

use std::collections::BTreeMap;

use serde_json::{Value, json};

use crate::product::{BacktestResultViewError, BacktestResultViewRequest};

pub(crate) struct ValidatedResultViewParams {
    pub view: String,
    pub offset: usize,
    pub limit: usize,
    pub resolution_ms: Option<i64>,
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

    let resolution_ms = match &request.resolution {
        Some(res) => Some(parse_resolution_ms(res).ok_or_else(|| {
            BacktestResultViewError::Invalid(format!("unsupported resolution: {res}"))
        })?),
        None => None,
    };

    Ok(ValidatedResultViewParams {
        view: view.to_owned(),
        offset,
        limit,
        resolution_ms,
    })
}

pub(crate) fn validate_rfc3339_timestamp(s: &str) -> Result<i128, BacktestResultViewError> {
    parse_rfc3339_nanos(s).ok_or_else(|| {
        BacktestResultViewError::Invalid(format!("invalid RFC3339 timestamp '{s}'"))
    })
}

pub(crate) fn parse_resolution_ms(res: &str) -> Option<i64> {
    let s = res.trim().to_ascii_lowercase();
    match s.as_str() {
        "1m" | "1min" => Some(60_000),
        "5m" | "5min" => Some(300_000),
        "15m" | "15min" => Some(900_000),
        "30m" | "30min" => Some(1_800_000),
        "60m" | "60min" | "1h" => Some(3_600_000),
        "2h" => Some(7_200_000),
        "4h" => Some(14_400_000),
        "1d" | "d" => Some(86_400_000),
        "1w" | "w" => Some(7 * 86_400_000),
        other => {
            let stripped = other.strip_suffix('s').unwrap_or(other);
            if let Ok(sec) = stripped.parse::<i64>()
                && sec > 0
            {
                return Some(sec * 1000);
            }
            None
        }
    }
}

pub(crate) fn slice_items(items: &[Value], offset: usize, limit: usize) -> (Vec<Value>, Option<String>) {
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
        return true;
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

pub(crate) fn downsample_candles(candles: &[Value], resolution_ms: i64) -> Vec<Value> {
    if candles.is_empty() || resolution_ms <= 0 {
        return candles.to_vec();
    }
    let mut buckets: BTreeMap<i64, Vec<&Value>> = BTreeMap::new();
    for c in candles {
        if let Some(nanos) = parse_timestamp_nanos(c.get("start").or_else(|| c.get("time"))) {
            let ms = (nanos / 1_000_000) as i64;
            let bucket = (ms / resolution_ms) * resolution_ms;
            buckets.entry(bucket).or_default().push(c);
        }
    }
    buckets
        .into_iter()
        .map(|(b_ms, group)| {
            let first = group[0];
            let last = group[group.len() - 1];
            let open = parse_f64_helper(first.get("open"));
            let close = parse_f64_helper(last.get("close"));
            let mut high = open;
            let mut low = open;
            let mut volume = 0.0;
            for c in &group {
                let h = parse_f64_helper(c.get("high"));
                let l = parse_f64_helper(c.get("low"));
                if h > high {
                    high = h;
                }
                if l < low {
                    low = l;
                }
                volume += parse_f64_helper(c.get("volume"));
            }
            let rfc = time::OffsetDateTime::from_unix_timestamp_nanos((b_ms as i128) * 1_000_000)
                .ok()
                .and_then(|dt| dt.format(&time::format_description::well_known::Rfc3339).ok());
            json!({
                "time": rfc.unwrap_or_else(|| b_ms.to_string()),
                "start": b_ms,
                "open": open,
                "high": high,
                "low": low,
                "close": close,
                "volume": volume,
            })
        })
        .collect()
}

pub(crate) fn downsample_curve(points: &[Value], val_key: &str, resolution_ms: i64) -> Vec<Value> {
    if points.is_empty() || resolution_ms <= 0 {
        return points.to_vec();
    }
    let mut buckets: BTreeMap<i64, &Value> = BTreeMap::new();
    for p in points {
        if let Some(nanos) = parse_timestamp_nanos(p.get("time")) {
            let ms = (nanos / 1_000_000) as i64;
            let bucket = (ms / resolution_ms) * resolution_ms;
            buckets.insert(bucket, p);
        }
    }
    buckets
        .into_iter()
        .map(|(b_ms, p)| {
            let rfc = time::OffsetDateTime::from_unix_timestamp_nanos((b_ms as i128) * 1_000_000)
                .ok()
                .and_then(|dt| dt.format(&time::format_description::well_known::Rfc3339).ok());
            let val = parse_f64_helper(p.get(val_key));
            json!({
                "time": rfc.unwrap_or_else(|| b_ms.to_string()),
                val_key: val,
            })
        })
        .collect()
}

fn parse_f64_helper(v: Option<&Value>) -> f64 {
    v.and_then(|val| {
        val.as_f64()
            .or_else(|| val.as_str().and_then(|s| s.parse::<f64>().ok()))
    })
    .unwrap_or(0.0)
}
