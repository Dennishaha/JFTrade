//! Production projection for OpenD underlying historical-volatility data.

use std::sync::Arc;

use serde_json::{Value, json};
use time::{Date, Duration, OffsetDateTime};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataOptionsReadSnapshotError;

pub(crate) fn read(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    path: &str,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option analysis runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.option_underlying_his_volatility_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option underlying historical volatility reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(path, query)?;
    let snapshot = runtime
        .option_underlying_his_volatility(&request)
        .map_err(map_error)?;
    let jftrade_integration_futu::OptionUnderlyingHisVolatilitySnapshot {
        security: _security,
        code,
        name,
        items,
        next_page_key,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item).map_err(|error| MarketDataOptionsReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: format!("failed to serialize OpenD option historical volatility: {error}"),
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    let mut metadata = serde_json::Map::new();
    if let Some(value) = code {
        metadata.insert("code".to_owned(), json!(value));
    }
    if let Some(value) = name {
        metadata.insert("name".to_owned(), json!(value));
    }
    metadata.insert(
        "market".to_owned(),
        json!(if request.market == 1 { "HK" } else { "US" }),
    );
    metadata.insert("beginTime".to_owned(), json!(request.begin_time));
    metadata.insert("endTime".to_owned(), json!(request.end_time));
    if let Some(value) = request.index_option_type {
        metadata.insert("indexOptionType".to_owned(), json!(value));
    }
    let mut result = json!({
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
        "hasMore": !next_page_key.is_empty(),
        "total": entries.len(),
    });
    if !metadata.is_empty() {
        result["metadata"] = Value::Object(metadata);
    }
    if let Some(next_cursor) = jftrade_integration_futu::encode_next_page_key(&next_page_key) {
        result["nextCursor"] = Value::String(next_cursor);
    }
    Ok(result)
}

fn parse_request(
    path: &str,
    query: &str,
) -> Result<
    jftrade_integration_futu::OptionUnderlyingHisVolatilityQuery,
    MarketDataOptionsReadSnapshotError,
> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => {
            return Err(bad_request(
                "option historical volatility market must be HK or US",
            ));
        }
    };
    let code = code.trim();
    if code.is_empty()
        || code
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(bad_request("option historical volatility code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let (default_begin, default_end) = default_historical_dates();
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
    let index_option_type = query_map
        .get_first("indexOptionType")
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("indexOptionType must be an integer"))
        })
        .transpose()?;
    let cursor = query_map
        .get_first("cursor")
        .or_else(|| query_map.get_first("nextPageKey"))
        .or_else(|| query_map.get_first("page"))
        .unwrap_or("");
    let next_page_key = jftrade_integration_futu::decode_next_page_key(cursor)
        .map_err(|error| bad_request(&error.to_string()))?;
    let request = jftrade_integration_futu::OptionUnderlyingHisVolatilityQuery {
        market: market_code,
        code: code.to_ascii_uppercase(),
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

fn default_historical_dates() -> (String, String) {
    let end = OffsetDateTime::now_utc().date();
    (format_date(end - Duration::days(365)), format_date(end))
}

fn format_date(value: Date) -> String {
    format!(
        "{:04}-{:02}-{:02}",
        value.year(),
        u8::from(value.month()),
        value.day()
    )
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionUnderlyingHisVolatilityQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionUnderlyingHisVolatilityQueryError::InvalidQuery(
            message,
        ) => bad_request(&message),
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
