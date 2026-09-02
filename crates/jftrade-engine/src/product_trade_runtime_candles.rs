use jftrade_integration_futu::HistoricalKlineResult;
use serde_json::{Value, json};

use super::super::{TradeRequest, qot_market_label};
use crate::product::product_query::QueryMap;

pub(super) fn historical_snapshot(
    request: &TradeRequest,
    result: &HistoricalKlineResult,
    period: &str,
    extended_hours: bool,
    sessions: &[&str],
    requested_limit: Option<i32>,
) -> Value {
    let mut rows = Vec::with_capacity(result.klines.len());
    for candle in &result.klines {
        if candle.is_blank {
            continue;
        }
        let mut row = serde_json::Map::new();
        let market = qot_market_label(result.security.market).unwrap_or("UTC");
        row.insert(
            "time".to_owned(),
            json!(canonical_candle_time(&candle.time, market)),
        );
        if let Some(value) = candle.open_price {
            row.insert("open".to_owned(), json!(value));
        }
        if let Some(value) = candle.close_price {
            row.insert("close".to_owned(), json!(value));
        }
        if let Some(value) = candle.high_price {
            row.insert("high".to_owned(), json!(value));
        }
        if let Some(value) = candle.low_price {
            row.insert("low".to_owned(), json!(value));
        }
        if let Some(value) = candle.volume {
            row.insert("volume".to_owned(), json!(value as f64));
        }
        if let Some(value) = candle.turnover {
            row.insert("turnover".to_owned(), json!(value));
        }
        if let Some(value) = candle.change_rate {
            row.insert("changeRate".to_owned(), json!(value));
        }
        rows.push(Value::Object(row));
    }
    if let Some(limit) = requested_limit {
        let limit = usize::try_from(limit).unwrap_or(0);
        if rows.len() > limit {
            rows = rows.split_off(rows.len() - limit);
        }
    }
    let next_before = rows
        .first()
        .and_then(|row| row["time"].as_str())
        .map(str::to_owned);
    let bounded = request
        .query
        .get_first("fromTime")
        .is_some_and(|v| !v.trim().is_empty())
        || request
            .query
            .get_first("toTime")
            .is_some_and(|v| !v.trim().is_empty());
    let pagination = if !bounded && !result.next_req_key.is_empty() {
        json!({"hasMore": true, "nextBefore": next_before})
    } else {
        json!({"hasMore": false})
    };
    json!({
        "accountId": request.account_id().unwrap_or_default(),
        "symbol": format!("{}.{}", qot_market_label(result.security.market).unwrap_or("UNKNOWN"), result.security.code),
        "period": period,
        "klines": rows,
        "pagination": pagination,
        "extendedHours": extended_hours,
        "session": if sessions.len() == 1 { sessions[0] } else if extended_hours { "all" } else { "regular" },
        "sessions": sessions,
    })
}

pub(super) fn canonical_candle_time(value: &str, market: &str) -> String {
    if value.contains('T') || value.ends_with('Z') {
        return value.to_owned();
    }
    let timezone = match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        "JP" => "Asia/Tokyo",
        _ => "UTC",
    };
    let Ok(local) = jiff::civil::DateTime::strptime("%Y-%m-%d %H:%M:%S", value) else {
        return value.to_owned();
    };
    let Ok(zoned) = local.in_tz(timezone) else {
        return value.to_owned();
    };
    zoned.timestamp().to_string()
}

pub(super) fn parse_requested_sessions(
    query: &QueryMap,
    extended_hours: bool,
) -> Result<Vec<&'static str>, String> {
    let mut values = Vec::new();
    for key in ["sessions", "session"] {
        if let Some(items) = query.get_all(key) {
            values.extend(items.iter().flat_map(|item| item.split(',')));
        }
    }
    if values.is_empty() {
        return Ok(if extended_hours {
            vec!["regular", "extended", "overnight"]
        } else {
            vec!["regular"]
        });
    }
    let mut result = Vec::new();
    for value in values {
        match value.trim().to_ascii_lowercase().as_str() {
            "regular" if !result.contains(&"regular") => result.push("regular"),
            "extended" if extended_hours && !result.contains(&"extended") => {
                result.push("extended")
            }
            "overnight" if extended_hours && !result.contains(&"overnight") => {
                result.push("overnight")
            }
            "regular" | "extended" | "overnight" => {
                return Err("requested session is unsupported for this period or market".to_owned());
            }
            other => return Err(format!("invalid candle session {other:?}")),
        }
    }
    Ok(result)
}
