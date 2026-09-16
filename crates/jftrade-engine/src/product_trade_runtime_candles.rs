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

pub(crate) fn canonical_candle_time(value: &str, market: &str) -> String {
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

impl super::SharedTradeReadRuntime {
    pub(crate) fn set_ticker_quotes(
        &self,
        reader: Option<std::sync::Arc<dyn jftrade_integration_futu::TickerQuoteReadPort>>,
    ) {
        *self
            .ticker_quotes
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn ticker_quotes_reader(
        &self,
    ) -> Option<std::sync::Arc<dyn jftrade_integration_futu::TickerQuoteReadPort>> {
        self.ticker_quotes
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn set_historical_klines(
        &self,
        reader: Option<std::sync::Arc<dyn jftrade_integration_futu::HistoricalKlineReadPort>>,
    ) {
        *self
            .historical_klines
            .write()
            .unwrap_or_else(|error| error.into_inner()) = reader;
    }

    pub(crate) fn historical_klines_available(&self) -> bool {
        self.historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .is_some()
    }

    pub(crate) fn historical_klines_reader(
        &self,
    ) -> Option<std::sync::Arc<dyn jftrade_integration_futu::HistoricalKlineReadPort>> {
        self.historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn historical_klines(
        &self,
        query: &jftrade_integration_futu::HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, String> {
        let reader = self
            .historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| "Futu historical klines runtime is unavailable".to_owned())?;
        reader.query(query).map_err(|error| error.to_string())
    }

    #[allow(dead_code)]
    pub(crate) fn current_kline(
        &self,
        query: &jftrade_integration_futu::CurrentKlineQuery,
    ) -> Result<jftrade_integration_futu::CurrentKlineResult, String> {
        let reader = self
            .historical_klines
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
            .ok_or_else(|| "Futu historical klines runtime is unavailable".to_owned())?;
        reader.query_current(query).map_err(|error| error.to_string())
    }

    pub(crate) fn historical_klines_window(
        &self,
        query: &jftrade_integration_futu::HistoricalKlineQuery,
    ) -> Result<HistoricalKlineResult, String> {
        let reader = self
            .historical_klines_reader()
            .ok_or_else(|| "Futu historical klines runtime is unavailable".to_owned())?;
        reader.query_window(query).map_err(|error| error.to_string())
    }

    /// US-aware history read: fans the window out across OpenD session routes
    /// and merges the results, falling back to `Session_ALL` when the server
    /// rejects an individual route.
    ///
    /// Parity: `pkg/futu/exchange_kline.go::queryHistoricalKLinesAcrossPlans`.
    pub(crate) fn historical_klines_window_routed<F>(
        &self,
        query: &jftrade_integration_futu::HistoricalKlineQuery,
        plans: &[jftrade_integration_futu::HistoricalKlineRequestPlan],
        mut keep_route_candles: F,
    ) -> Result<HistoricalKlineResult, String>
    where
        F: FnMut(
            &jftrade_integration_futu::HistoricalKlineRequestPlan,
            &mut HistoricalKlineResult,
        ),
    {
        if plans.len() <= 1 && plans.first().is_none_or(|plan| plan.session.is_none()) {
            return self.historical_klines_window(query);
        }
        let mut merged: Option<HistoricalKlineResult> = None;
        for plan in plans {
            let mut routed = query.clone();
            routed.extended_time = Some(plan.extended_time);
            routed.session = plan.session;
            match self.historical_klines_window(&routed) {
                Ok(mut result) => {
                    // A routed plan keeps only the market sessions it owns so
                    // the broad `Session_ALL` route cannot overwrite the more
                    // specific RTH/ETH candles with its duplicates.
                    keep_route_candles(plan, &mut result);
                    merged = Some(match merged {
                        Some(existing) => merge_routed_history(existing, result),
                        None => result,
                    });
                }
                Err(error) => {
                    let route_error = jftrade_integration_futu::HistoricalKlineRouteError {
                        session: plan.session,
                        ret_type: 1,
                        err_code: 0,
                        ret_msg: error.clone(),
                    };
                    if !jftrade_integration_futu::should_fallback_to_all(plan, &route_error) {
                        return Err(error);
                    }
                    let all = jftrade_integration_futu::HistoricalKlineRequestPlan::all(
                        plan.keep_sessions.clone(),
                    );
                    let mut request = query.clone();
                    request.extended_time = Some(all.extended_time);
                    request.session = all.session;
                    return self.historical_klines_window(&request);
                }
            }
        }
        merged.ok_or_else(|| "Futu historical klines runtime is unavailable".to_owned())
    }
}

/// Merges two routed pages, keeping the last writer for a duplicate bucket so
/// the later (more specific) route wins, mirroring Go's map assignment order.
fn merge_routed_history(
    mut existing: HistoricalKlineResult,
    page: HistoricalKlineResult,
) -> HistoricalKlineResult {
    existing.name = existing.name.or(page.name);
    existing.klines =
        jftrade_integration_futu::merge_klines_by_time(&existing.klines, &page.klines);
    existing.next_req_key = page.next_req_key;
    existing
}
