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
    if !runtime.option_quotes_available() {
        return Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Futu option quote reader is not ready".to_owned(),
        ));
    }
    let request = parse_request(path, query)?;
    let quotes = runtime
        .option_quotes(&request)
        .map_err(map_error)?;
    if quotes.is_empty() {
        return Err(MarketDataOptionsReadSnapshotError::Failed {
            status: 404,
            code: "OPTION_QUOTE_NOT_FOUND".to_owned(),
            message: "OpenD returned no option quote".to_owned(),
        });
    }
    let entries = quotes
        .into_iter()
        .map(|quote| {
            serde_json::to_value(quote).map_err(|error| {
                MarketDataOptionsReadSnapshotError::Failed {
                    status: 502,
                    code: "BAD_GATEWAY".to_owned(),
                    message: format!("failed to serialize OpenD option quote: {error}"),
                }
            })
        })
        .collect::<Result<Vec<_>, _>>()?;
    let total = entries.len();
    let as_of = super::super::provider_now_rfc3339();
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
        "total": total,
    }))
}

fn parse_request(
    path: &str,
    query: &str,
) -> Result<jftrade_integration_futu::OptionQuoteQuery, MarketDataOptionsReadSnapshotError> {
    let instrument = path
        .strip_prefix("/api/v1/market-data/options/analysis/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| bad_request("unsupported options analysis route"))?;
    let (market, code) = instrument
        .split_once('.')
        .filter(|(market, code)| {
            !market.is_empty() && !code.is_empty() && !code.contains('.')
        })
        .ok_or_else(|| bad_request("instrumentId must be MARKET.CODE"))?;
    let market = market.trim().to_ascii_uppercase();
    let market_code = match market.as_str() {
        "HK" => 1,
        "US" => 11,
        _ => return Err(bad_request("option quote market must be HK or US")),
    };
    let code = code.trim();
    if code.is_empty() || code.chars().any(char::is_whitespace) {
        return Err(bad_request("option quote code is invalid"));
    }
    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| bad_request("invalid URL escape"))?;
    if query_map
        .get_first("operation")
        .is_none_or(|operation| operation.trim() != "quote")
    {
        return Err(bad_request("operation must be quote"));
    }
    if let Some(requested_market) = query_map.get_first("market")
        && !requested_market.trim().is_empty()
        && requested_market.trim().to_ascii_uppercase() != market
    {
        return Err(bad_request("market does not match instrumentId"));
    }
    let request = jftrade_integration_futu::OptionQuoteQuery {
        market: market_code,
        code: code.to_ascii_uppercase(),
    };
    request
        .validate()
        .map_err(|error| bad_request(&error.to_string()))?;
    Ok(request)
}

fn bad_request(message: &str) -> MarketDataOptionsReadSnapshotError {
    MarketDataOptionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn map_error(
    error: jftrade_integration_futu::OptionQuoteQueryError,
) -> MarketDataOptionsReadSnapshotError {
    match error {
        jftrade_integration_futu::OptionQuoteQueryError::InvalidQuery(message) => {
            bad_request(&message)
        }
        other => MarketDataOptionsReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: other.to_string(),
        },
    }
}
