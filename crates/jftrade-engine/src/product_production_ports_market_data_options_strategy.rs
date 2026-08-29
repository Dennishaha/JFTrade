//! Production projection for OpenD option-strategy combinations.

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
    if !runtime.option_strategy_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option strategy reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(path, query)?;
    let snapshot = runtime.option_strategy(&request).map_err(map_error)?;
    let entries = snapshot
        .items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item).map_err(|error| {
                MarketDataOptionsReadSnapshotError::Failed {
                    status: 502,
                    code: "BAD_GATEWAY".to_owned(),
                    message: format!("failed to serialize OpenD option strategy: {error}"),
                }
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    let mut metadata = serde_json::Map::new();
    metadata.insert(
        "market".to_owned(),
        json!(if request.market == 1 { "HK" } else { "US" }),
    );
    metadata.insert("code".to_owned(), json!(request.code));
    metadata.insert("optionStrategy".to_owned(), json!(request.option_strategy));
    if let Some(value) = request.expire_time {
        metadata.insert("expireTime".to_owned(), json!(value));
    }
    if let Some(value) = request.far_expire_time {
        metadata.insert("farExpireTime".to_owned(), json!(value));
    }
    if let Some(value) = request.spread {
        metadata.insert("spread".to_owned(), json!(value));
    }
    if let Some(value) = request.option_type {
        metadata.insert("optionType".to_owned(), json!(value));
    }
    if let Some(value) = request.strike_price {
        metadata.insert("strikePrice".to_owned(), json!(value));
    }
    if let Some(value) = request.index_option_type {
        metadata.insert("indexOptionType".to_owned(), json!(value));
    }
    Ok(json!({
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
        "hasMore": false,
        "total": entries.len(),
        "metadata": Value::Object(metadata),
    }))
}

fn parse_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionStrategyQuery, MarketDataOptionsReadSnapshotError> {
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
        _ => return Err(bad_request("option strategy market must be HK or US")),
    };
    let code = code.trim();
    if code.is_empty()
        || code.chars().any(|value| {
            value.is_whitespace() || (!value.is_ascii_alphanumeric() && value != '-')
        })
    {
        return Err(bad_request("option strategy code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let option_strategy = query_map
        .get_first("optionStrategy")
        .or_else(|| query_map.get_first("option_strategy"))
        .or_else(|| query_map.get_first("strategy"))
        .ok_or_else(|| bad_request("optionStrategy is required"))
        .and_then(parse_option_strategy)?;
    let expire_time = optional_text(&query_map, &["expireTime", "expire_time", "nearExpiry"]);
    let far_expire_time = optional_text(&query_map, &["farExpireTime", "far_expire_time"]);
    let spread = optional_float(&query_map, &["spread"])?;
    let option_type = optional_int(&query_map, &["optionType", "option_type"])?;
    let strike_price = optional_float(&query_map, &["strikePrice", "strike_price"])?;
    let index_option_type = optional_int(&query_map, &["indexOptionType", "index_option_type"])?;
    let request = jftrade_integration_futu::OptionStrategyQuery {
        market: market_code,
        code: code.to_ascii_uppercase(),
        option_strategy,
        expire_time,
        far_expire_time,
        spread,
        option_type,
        strike_price,
        index_option_type,
    };
    request.validate().map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn optional_text(
    query_map: &crate::product::product_query::QueryMap,
    keys: &[&str],
) -> Option<String> {
    keys.iter()
        .find_map(|key| query_map.get_first(key))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn optional_float(
    query_map: &crate::product::product_query::QueryMap,
    keys: &[&str],
) -> Result<Option<f64>, MarketDataOptionsReadSnapshotError> {
    let Some(value) = keys.iter().find_map(|key| query_map.get_first(key)) else {
        return Ok(None);
    };
    value
        .trim()
        .parse::<f64>()
        .map(Some)
        .map_err(|_| bad_request(&format!("{} must be a number", keys[0])))
}

fn optional_int(
    query_map: &crate::product::product_query::QueryMap,
    keys: &[&str],
) -> Result<Option<i32>, MarketDataOptionsReadSnapshotError> {
    let Some(value) = keys.iter().find_map(|key| query_map.get_first(key)) else {
        return Ok(None);
    };
    value
        .trim()
        .parse::<i32>()
        .map(Some)
        .map_err(|_| bad_request(&format!("{} must be an integer", keys[0])))
}

fn parse_option_strategy(
    value: &str,
) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    if let Ok(value) = value.trim().parse::<i32>() {
        return Ok(value);
    }
    let normalized = value.trim().to_ascii_lowercase().replace(['-', ' '], "_");
    let value = match normalized.as_str() {
        "single" | "single_option" => 1,
        "covered" => 2,
        "spread" | "vertical" => 4,
        "straddle" => 6,
        "strangle" => 7,
        "collar" => 8,
        "butterfly" => 9,
        "condor" => 11,
        "iron_butterfly" => 13,
        "iron_condor" => 14,
        "calendar" | "calendar_spread" => 15,
        "diagonal" | "diagonal_spread" => 16,
        "custom" | "customize" => 100,
        _ => return Err(bad_request("optionStrategy must be an integer or supported strategy name")),
    };
    Ok(value)
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionStrategyQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionStrategyQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
