//! Production projections for option market and underlying historical
//! statistics returned by OpenD.

use std::sync::Arc;

use serde_json::{Value, json};
use time::{Date, Duration, OffsetDateTime};

use super::super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataOptionsReadSnapshotError;

pub(crate) fn read_market_statistics(
    runtime: &Arc<SharedTradeReadRuntime>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_market_statistic_available() {
        return Err(unavailable(
            "Futu option market statistic reader is not ready",
        ));
    }
    let request = parse_market_statistic_request(path, query)?;
    let snapshot = runtime
        .option_market_statistic(&request)
        .map_err(map_market_error)?;
    let jftrade_integration_futu::OptionMarketStatisticSnapshot {
        option_market,
        market,
        data_type,
        items,
        next_page_key,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item)
                .map_err(|error| serialization_error("market statistic", error))
        })
        .collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::super::provider_now_rfc3339();
    let mut result = base_result(as_of, entries, !next_page_key.is_empty(), total);
    result["metadata"] = json!({
        "market": market,
        "optionMarket": option_market,
        "dataType": data_type,
        "beginTime": request.begin_time,
        "endTime": request.end_time,
    });
    if let Some(cursor) =
        jftrade_integration_futu::encode_option_market_statistic_cursor(&next_page_key)
    {
        result["nextCursor"] = Value::String(cursor);
    }
    Ok(result)
}

pub(crate) fn read_historical_statistics(
    runtime: &Arc<SharedTradeReadRuntime>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    if !runtime.option_underlying_his_statistic_available() {
        return Err(unavailable(
            "Futu option underlying historical statistic reader is not ready",
        ));
    }
    let request = parse_underlying_his_statistic_request(path, query)?;
    let snapshot = runtime
        .option_underlying_his_statistic(&request)
        .map_err(map_underlying_error)?;
    let jftrade_integration_futu::OptionUnderlyingHisStatisticSnapshot {
        security: _security,
        code,
        name,
        items,
        next_page_key,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item)
                .map_err(|error| serialization_error("underlying historical statistic", error))
        })
        .collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::super::provider_now_rfc3339();
    let mut result = base_result(as_of, entries, !next_page_key.is_empty(), total);
    let mut metadata = serde_json::Map::new();
    if let Some(value) = code {
        metadata.insert("code".to_owned(), json!(value));
    }
    if let Some(value) = name {
        metadata.insert("name".to_owned(), json!(value));
    }
    metadata.insert("market".to_owned(), json!(market_label(request.market)));
    metadata.insert("beginTime".to_owned(), json!(request.begin_time));
    metadata.insert("endTime".to_owned(), json!(request.end_time));
    if let Some(value) = request.index_option_type {
        metadata.insert("indexOptionType".to_owned(), json!(value));
    }
    result["metadata"] = Value::Object(metadata);
    if let Some(cursor) =
        jftrade_integration_futu::encode_option_underlying_his_statistic_cursor(&next_page_key)
    {
        result["nextCursor"] = Value::String(cursor);
    }
    Ok(result)
}

fn base_result(as_of: String, entries: Vec<Value>, has_more: bool, total: usize) -> Value {
    json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_analysis",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": has_more,
        "total": total,
    })
}

fn parse_market_statistic_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionMarketStatisticQuery, MarketDataOptionsReadSnapshotError>
{
    let market = parse_analysis_market(path)?;
    let query_map = parse_query(query)?;
    let option_market = query_map
        .get_first("optionMarket")
        .or_else(|| query_map.get_first("option_market"))
        .map(|value| parse_i32(value, "optionMarket"))
        .transpose()?
        .unwrap_or(if market == "US" { 1 } else { 3 });
    if market_label_for_option_market(option_market) != Some(market.as_str()) {
        return Err(bad_request("optionMarket does not match instrument market"));
    }
    let data_type = query_map
        .get_first("dataType")
        .or_else(|| query_map.get_first("data_type"))
        .map(|value| parse_i32(value, "dataType"))
        .transpose()?
        .unwrap_or(0);
    let (default_begin, default_end) = default_dates(30);
    let begin_time = query_map
        .get_first("beginTime")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .unwrap_or(default_begin);
    let end_time = query_map
        .get_first("endTime")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .unwrap_or(default_end);
    let next_page_key = decode_cursor(
        query_map
            .get_first("cursor")
            .or_else(|| query_map.get_first("nextPageKey"))
            .or_else(|| query_map.get_first("page"))
            .unwrap_or(""),
        true,
    )?;
    let request = jftrade_integration_futu::OptionMarketStatisticQuery {
        option_market,
        data_type,
        begin_time,
        end_time,
        next_page_key,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_underlying_his_statistic_request(
    path: &str,
    query: &str,
) -> Result<
    jftrade_integration_futu::OptionUnderlyingHisStatisticQuery,
    MarketDataOptionsReadSnapshotError,
> {
    let (market, code) = parse_analysis_instrument(path)?;
    let query_map = parse_query(query)?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let index_option_type = query_map
        .get_first("indexOptionType")
        .map(|value| parse_i32(value, "indexOptionType"))
        .transpose()?;
    let (default_begin, default_end) = default_dates(365);
    let begin_time = query_map
        .get_first("beginTime")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .unwrap_or(default_begin);
    let end_time = query_map
        .get_first("endTime")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .unwrap_or(default_end);
    let next_page_key = decode_cursor(
        query_map
            .get_first("cursor")
            .or_else(|| query_map.get_first("nextPageKey"))
            .or_else(|| query_map.get_first("page"))
            .unwrap_or(""),
        false,
    )?;
    let request = jftrade_integration_futu::OptionUnderlyingHisStatisticQuery {
        market: if market == "US" { 11 } else { 1 },
        code,
        index_option_type,
        begin_time,
        end_time,
        next_page_key,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_analysis_instrument(
    path: &str,
) -> Result<(String, String), MarketDataOptionsReadSnapshotError> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    if !matches!(market.as_str(), "HK" | "US") {
        return Err(bad_request("option statistic market must be HK or US"));
    }
    let code = code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(bad_request("option statistic code is invalid"));
    }
    Ok((market, code.to_ascii_uppercase()))
}

fn parse_analysis_market(path: &str) -> Result<String, MarketDataOptionsReadSnapshotError> {
    parse_analysis_instrument(path).map(|(market, _)| market)
}

fn parse_query(
    query: &str,
) -> Result<crate::product::product_query::QueryMap, MarketDataOptionsReadSnapshotError> {
    crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))
}

fn parse_i32(value: &str, name: &str) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    value
        .trim()
        .parse::<i32>()
        .map_err(|_| bad_request(&format!("{name} must be an integer")))
}

fn decode_cursor(value: &str, market: bool) -> Result<Vec<u8>, MarketDataOptionsReadSnapshotError> {
    if market {
        jftrade_integration_futu::decode_option_market_statistic_cursor(value)
            .map_err(|error| bad_request(&error.to_string()))
    } else {
        jftrade_integration_futu::decode_option_underlying_his_statistic_cursor(value)
            .map_err(|error| bad_request(&error.to_string()))
    }
}

fn market_label_for_option_market(value: i32) -> Option<&'static str> {
    match value {
        1 | 2 => Some("US"),
        3 | 4 => Some("HK"),
        _ => None,
    }
}

fn market_label(market: i32) -> &'static str {
    if market == 11 { "US" } else { "HK" }
}

fn default_dates(days: i64) -> (String, String) {
    let end = OffsetDateTime::now_utc().date();
    (format_date(end - Duration::days(days)), format_date(end))
}

fn format_date(value: Date) -> String {
    format!(
        "{:04}-{:02}-{:02}",
        value.year(),
        u8::from(value.month()),
        value.day()
    )
}

fn unavailable(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Unavailable(message.to_owned())
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn serialization_error(
    operation: &str,
    error: serde_json::Error,
) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 502,
        code: "BAD_GATEWAY".to_owned(),
        message: format!("failed to serialize OpenD option {operation}: {error}"),
    }
}

fn map_market_error(
    error: jftrade_integration_futu::OptionMarketStatisticQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionMarketStatisticQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}

fn map_underlying_error(
    error: jftrade_integration_futu::OptionUnderlyingHisStatisticQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionUnderlyingHisStatisticQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
