use std::sync::Arc;

use serde_json::{Value, json};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::MarketDataOptionsReadSnapshotError;

pub(crate) fn read(
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    query: &str,
) -> Result<Value, MarketDataOptionsReadSnapshotError> {
    let runtime = runtime.ok_or_else(|| {
        MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option screen runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.option_screens_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option screen reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(query)?;
    let page = runtime
        .option_screens(&request)
        .map_err(map_error)?;
    let entries = page
        .items
        .into_iter()
        .map(|item| {
            serde_json::to_value(item).map_err(|error| {
                MarketDataOptionsReadSnapshotError::Failed {
                    status: 502,
                    code: "BAD_GATEWAY".to_owned(),
                    message: format!("failed to serialize OpenD option screen: {error}"),
                }
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    Ok(json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_screen",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": !page.last_page,
        "total": page.all_count,
    }))
}

fn parse_request(
    query: &str,
) -> Result<jftrade_integration_futu::OptionScreenQuery, MarketDataOptionsReadSnapshotError> {
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(operation) = query_map.get_first("operation")
        && !operation.trim().is_empty()
        && operation.trim() != "screen"
    {
        return Err(bad_request("operation must be screen"));
    }
    let market = query_map
        .get_first("market")
        .map(|value| value.trim().to_ascii_uppercase())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "US".to_owned());
    let default_category = match market.as_str() {
        "US" => 0,
        "HK" => 3,
        _ => return Err(bad_request("option screen market must be HK or US")),
    };
    let market_categories = parse_list(
        &query_map,
        "marketCategoryList",
        Some(default_category),
    )?;
    let page_from = parse_optional_i32(&query_map, "pageFrom")?;
    let page_count = query_map
        .get_first("pageSize")
        .or_else(|| query_map.get_first("pageCount"))
        .map(|value| parse_i32(value, "pageSize"))
        .transpose()?;
    let option_retrieve_list = parse_list(&query_map, "optionRetrieveList", None)?;
    let underlying_retrieve_list = parse_list(&query_map, "underlyingRetrieveList", None)?;
    Ok(jftrade_integration_futu::OptionScreenQuery {
        market_categories,
        page_from,
        page_count,
        option_retrieve_list,
        underlying_retrieve_list,
    })
}

fn parse_list(
    query: &crate::product::product_query::QueryMap,
    key: &str,
    default: Option<i32>,
) -> Result<Vec<i32>, MarketDataOptionsReadSnapshotError> {
    let Some(values) = query.get_all(key) else {
        return Ok(default.into_iter().collect());
    };
    let mut result = Vec::new();
    for value in values {
        for token in value.split(',') {
            let token = token.trim();
            if token.is_empty() {
                return Err(bad_request(&format!("{key} contains an empty value")));
            }
            result.push(parse_i32(token, key)?);
        }
    }
    Ok(result)
}

fn parse_optional_i32(
    query: &crate::product::product_query::QueryMap,
    key: &str,
) -> Result<Option<i32>, MarketDataOptionsReadSnapshotError> {
    query
        .get_first(key)
        .map(|value| parse_i32(value, key))
        .transpose()
}

fn parse_i32(value: &str, key: &str) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    value.trim().parse::<i32>().map_err(|_| {
        bad_request(&format!("{key} must contain integer values"))
    })
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionScreenQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionScreenQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
