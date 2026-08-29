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
            "Futu option event runtime is not configured".to_owned(),
        )
    })?;
    if !runtime.option_events_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option event reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(query)?;
    let page = runtime.option_events(&request).map_err(map_error)?;
    let entries = page
        .events
        .into_iter()
        .map(|event| {
            serde_json::to_value(event).map_err(|error| {
                MarketDataOptionsReadSnapshotError::Failed {
                    status: 502,
                    code: "BAD_GATEWAY".to_owned(),
                    message: format!("failed to serialize OpenD option event: {error}"),
                }
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let as_of = super::super::provider_now_rfc3339();
    let total = page.all_count.unwrap_or(entries.len() as i32);
    let has_more = page.next_page.is_some();
    let mut result = json!({
        "provider": {
            "brokerId": "futu",
            "securityFirm": "Futu/Moomoo via OpenD",
            "featureId": "derivatives.option_events",
            "capability": "available",
            "selectionReason": "adapter_request",
            "resolvedAt": as_of,
            "asOf": as_of,
        },
        "asOf": as_of,
        "entries": entries,
        "hasMore": has_more,
        "total": total,
    });
    if let Some(next_page) = page.next_page {
        result["nextCursor"] = Value::String(next_page);
    }
    Ok(result)
}

fn parse_request(
    query: &str,
) -> Result<jftrade_integration_futu::OptionEventQuery, MarketDataOptionsReadSnapshotError> {
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if let Some(operation) = query_map.get_first("operation")
        && !operation.trim().is_empty()
        && operation.trim() != "unusual"
    {
        return Err(bad_request("operation must be unusual"));
    }
    let market = query_map
        .get_first("market")
        .map(|value| value.trim().to_ascii_uppercase())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "US".to_owned());
    let (market_code, market_label) = match market.as_str() {
        "US" => (11, "US"),
        "HK" => (1, "HK"),
        _ => return Err(bad_request("option event market must be HK or US")),
    };
    let underlying_product_class = parse_product_class(&query_map)?;
    let owner_value = query_map
        .get_first("underlying")
        .or_else(|| query_map.get_first("instrumentId"))
        .or_else(|| query_map.get_first("code"));
    let owner = owner_value
        .filter(|value| !value.trim().is_empty())
        .map(|value| parse_owner(value, market_label))
        .transpose()?;
    let count = query_map
        .get_first("pageSize")
        .or_else(|| query_map.get_first("count"))
        .map(|value| parse_i32(value, "pageSize"))
        .transpose()?
        .unwrap_or(100);
    if !(1..=300).contains(&count) {
        return Err(bad_request("pageSize must be between 1 and 300"));
    }
    let page = query_map
        .get_first("cursor")
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned);
    let sort = parse_sort(&query_map)?;
    Ok(jftrade_integration_futu::OptionEventQuery {
        market: option_market(market_code, underlying_product_class),
        underlying_product_class: Some(underlying_product_class),
        owner,
        count,
        page,
        filters: Vec::new(),
        sort,
    })
}

fn parse_product_class(
    query: &crate::product::product_query::QueryMap,
) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    match query
        .get_first("underlyingProductClass")
        .map(|value| value.trim().to_ascii_lowercase())
        .as_deref()
    {
        None | Some("") | Some("equity") | Some("option") => Ok(1),
        Some("index") => Ok(2),
        Some(_) => Err(bad_request(
            "underlyingProductClass must be equity or index",
        )),
    }
}

fn parse_owner(
    value: &str,
    expected_market: &str,
) -> Result<jftrade_integration_futu::OptionEventSecurity, MarketDataOptionsReadSnapshotError> {
    let (market, code) = value
        .trim()
        .split_once('.')
        .filter(|(market, code)| !market.is_empty() && !code.is_empty() && !code.contains('.'))
        .ok_or_else(|| bad_request("underlying must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    if market != expected_market {
        return Err(bad_request("underlying market does not match market"));
    }
    let code = code.trim();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(bad_request("underlying code is invalid"));
    }
    Ok(jftrade_integration_futu::OptionEventSecurity {
        market: market.clone(),
        code: code.to_ascii_uppercase(),
        quote_market: market.clone(),
        trade_market: market.clone(),
        instrument_id: format!("{market}.{}", code.to_ascii_uppercase()),
    })
}

fn parse_sort(
    query: &crate::product::product_query::QueryMap,
) -> Result<Option<jftrade_integration_futu::EventSort>, MarketDataOptionsReadSnapshotError> {
    let Some(value) = query.get_first("sort").filter(|value| !value.trim().is_empty()) else {
        return Ok(None);
    };
    let indicator_type = match value.trim().to_ascii_lowercase().as_str() {
        "time" | "fill_time" => 305,
        "price" => 304,
        "volume" => 302,
        "turnover" => 303,
        "dte" | "expiry_days" => 204,
        "iv" => 504,
        "delta" => 601,
        "gamma" => 602,
        "vega" => 603,
        "theta" => 604,
        "rho" => 605,
        _ => return Err(bad_request("unsupported option event sort")),
    };
    let is_asc = match query.get_first("sortAsc") {
        None | Some("") => false,
        Some(value) => match value.trim().to_ascii_lowercase().as_str() {
            "true" | "1" | "asc" => true,
            "false" | "0" | "desc" => false,
            _ => return Err(bad_request("sortAsc must be true or false")),
        },
    };
    Ok(Some(jftrade_integration_futu::EventSort {
        indicator_type,
        is_asc,
    }))
}

fn option_market(market: i32, product_class: i32) -> i32 {
    match (market, product_class) {
        (11, 2) => 2,
        (1, 2) => 4,
        (11, _) => 1,
        (1, _) => 3,
        _ => 0,
    }
}

fn parse_i32(value: &str, key: &str) -> Result<i32, MarketDataOptionsReadSnapshotError> {
    value
        .trim()
        .parse::<i32>()
        .map_err(|_| bad_request(&format!("{key} must be an integer")))
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionEventQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionEventQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
