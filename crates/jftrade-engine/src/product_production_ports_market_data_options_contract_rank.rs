//! Production projection for the OpenD option-contract rank operation.

use std::sync::Arc;

use serde_json::{Value, json};

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
    if !runtime.option_contract_rank_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option contract rank reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(path, query)?;
    let snapshot = runtime.option_contract_rank(&request).map_err(map_error)?;
    let jftrade_integration_futu::OptionContractRankSnapshot {
        market,
        sort_type,
        trading_date,
        trading_timestamp,
        items,
        next_page,
        all_count,
    } = snapshot;
    let entries = items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item).map_err(|error| MarketDataOptionsReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message: format!("failed to serialize OpenD option contract rank: {error}"),
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    let mut metadata = serde_json::Map::new();
    metadata.insert("market".to_owned(), json!(market));
    metadata.insert("sortType".to_owned(), json!(sort_type));
    if let Some(value) = trading_date {
        metadata.insert("tradingDate".to_owned(), json!(value));
    }
    if let Some(value) = trading_timestamp {
        metadata.insert("tradingTimestamp".to_owned(), json!(value));
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
        "hasMore": next_page.is_some(),
        "total": all_count.unwrap_or(0),
    });
    if !metadata.is_empty() {
        result["metadata"] = Value::Object(metadata);
    }
    if let Some(next_page) = next_page {
        result["nextCursor"] = Value::String(next_page);
    }
    Ok(result)
}

fn parse_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionContractRankQuery, MarketDataOptionsReadSnapshotError> {
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
        _ => return Err(bad_request("option contract rank market must be HK or US")),
    };
    if code.trim().is_empty()
        || code
            .trim()
            .chars()
            .any(|value| value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-'))
    {
        return Err(bad_request("option contract rank code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let sort_type = query_map
        .get_first("sortType")
        .or_else(|| query_map.get_first("sort"))
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("sortType must be an integer"))
        })
        .transpose()?
        .unwrap_or(1);
    let count = query_map
        .get_first("count")
        .or_else(|| query_map.get_first("pageSize"))
        .map(|value| {
            value
                .trim()
                .parse::<i32>()
                .map_err(|_| bad_request("count must be an integer"))
        })
        .transpose()?
        .or(Some(200));
    let trading_date = query_map
        .get_first("tradingDate")
        .map(|value| value.trim().to_owned());
    let is_asc = query_map.get_first("isAsc").map(parse_bool).transpose()?;
    let page = query_map
        .get_first("page")
        .or_else(|| query_map.get_first("cursor"))
        .map(|value| value.trim().to_owned());
    let request = jftrade_integration_futu::OptionContractRankQuery {
        market: market_code,
        sort_type,
        count,
        trading_date,
        is_asc,
        page,
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn parse_bool(value: &str) -> Result<bool, MarketDataOptionsReadSnapshotError> {
    match value.trim().to_ascii_lowercase().as_str() {
        "true" | "1" => Ok(true),
        "false" | "0" => Ok(false),
        _ => Err(bad_request("isAsc must be true or false")),
    }
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionContractRankQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionContractRankQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
