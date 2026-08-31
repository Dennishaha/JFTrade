//! MCP market-data adapters backed by the production quote/provider ports.
//!
//! The MCP layer is synchronous by contract, while the market-data ports are
//! async or provider-action based. Request construction stays pure and is
//! tested independently; execution always delegates to the installed
//! production port and preserves its baseline error status.

use std::sync::Arc;

use serde_json::{Value, json};

use super::helpers::{
    bounded_integer, instrument, optional_string_array, path_segment, query_string,
    run_provider_action, run_quote_read,
};
use super::{McpToolFailure, ProductionMcpToolExecutor, provider_actions_error, quote_error};
use crate::product::product_market_data_provider_actions_port::{
    BATCH_SNAPSHOTS_PATH, MarketDataProviderActionsRequest,
};

const SUBSCRIPTIONS_PATH: &str = "/api/v1/market-data/subscriptions";

impl ProductionMcpToolExecutor {
    pub(super) fn market_candles(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let (path, query) = candle_request(arguments)?;
        let port = Arc::clone(&self.ports()?.market_data_quote);
        run_quote_read(port, path, query).map_err(quote_error)
    }

    pub(super) fn market_snapshots(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let request = snapshot_request(arguments)?;
        let port = Arc::clone(&self.ports()?.market_data_provider_actions);
        run_provider_action(port, request).map_err(provider_actions_error)
    }

    pub(super) fn market_subscriptions(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let query = subscription_query(arguments)?;
        let port = Arc::clone(&self.ports()?.market_data_quote);
        run_quote_read(port, SUBSCRIPTIONS_PATH.to_owned(), query).map_err(quote_error)
    }
}

fn candle_request(arguments: &Value) -> Result<(String, String), McpToolFailure> {
    let (market, symbol) = instrument(arguments)?;
    let operation = super::optional_string(arguments, "operation")
        .unwrap_or_else(|| "current".to_owned())
        .to_ascii_lowercase();
    if !matches!(operation.as_str(), "current" | "historical") {
        return Err(McpToolFailure::invalid(
            "operation must be current or historical",
        ));
    }
    let period = super::optional_string(arguments, "period").unwrap_or_else(|| "1m".to_owned());
    let limit = bounded_integer(arguments, "limit", 50, 1, 500)?;
    let sessions = optional_string_array(arguments, "sessions")?
        .map(|items| {
            if items.is_empty() {
                return Err(McpToolFailure::invalid(
                    "sessions must contain at least one value",
                ));
            }
            if items.iter().any(|item| {
                !matches!(
                    item.to_ascii_lowercase().as_str(),
                    "regular" | "extended" | "overnight"
                )
            }) {
                return Err(McpToolFailure::invalid(
                    "sessions must contain regular, extended, or overnight",
                ));
            }
            Ok(items
                .into_iter()
                .map(|item| item.to_ascii_lowercase())
                .collect::<Vec<_>>()
                .join(","))
        })
        .transpose()?;
    let from = super::optional_string(arguments, "startTime")
        .or_else(|| super::optional_string(arguments, "from"));
    let to = super::optional_string(arguments, "endTime")
        .or_else(|| super::optional_string(arguments, "to"));
    let before = super::optional_string(arguments, "beforeTime")
        .or_else(|| super::optional_string(arguments, "before"));
    let query = query_string([
        ("period", Some(period)),
        ("limit", Some(limit.to_string())),
        ("from", from),
        ("to", to),
        ("before", before),
        ("sessions", sessions),
        (
            "adjustment",
            super::optional_string(arguments, "adjustment"),
        ),
    ]);
    Ok((
        format!(
            "/api/v1/market-data/candles/{}/{}",
            path_segment(&market),
            path_segment(&symbol)
        ),
        query,
    ))
}

fn snapshot_request(arguments: &Value) -> Result<MarketDataProviderActionsRequest, McpToolFailure> {
    let market =
        super::optional_string(arguments, "market").map(|value| value.to_ascii_uppercase());
    let mut values = optional_string_array(arguments, "symbols")?.unwrap_or_default();
    if let Some(instrument_id) = super::optional_string(arguments, "instrumentId") {
        values.push(instrument_id);
    }
    if values.is_empty() {
        return Err(McpToolFailure::invalid(
            "instrumentId or symbols is required",
        ));
    }
    if values.len() > 200 {
        return Err(McpToolFailure::invalid(
            "symbols must contain at most 200 items",
        ));
    }

    let default_market = market.as_deref().unwrap_or("US");
    let mut symbols = Vec::with_capacity(values.len());
    for value in values {
        let value = value.trim().to_ascii_uppercase();
        let normalized = if value.contains('.') {
            value
        } else {
            format!("{default_market}.{value}")
        };
        let Some((resolved_market, resolved_symbol)) = normalized.split_once('.') else {
            return Err(McpToolFailure::invalid("symbols must use MARKET.SYMBOL"));
        };
        if resolved_market.is_empty()
            || resolved_symbol.is_empty()
            || resolved_market.contains('/')
            || resolved_symbol.contains('/')
            || resolved_market.chars().any(char::is_control)
            || resolved_symbol.chars().any(char::is_control)
        {
            return Err(McpToolFailure::invalid("symbols must use MARKET.SYMBOL"));
        }
        if !symbols.contains(&normalized) {
            symbols.push(normalized);
        }
    }
    if symbols.is_empty() {
        return Err(McpToolFailure::invalid(
            "instrumentId or symbols is required",
        ));
    }
    let query = query_string([
        ("brokerId", super::optional_string(arguments, "brokerId")),
        ("accountId", super::optional_string(arguments, "accountId")),
        ("market", market),
    ]);
    let body = serde_json::to_vec(&json!({"symbols": symbols})).map_err(|error| {
        McpToolFailure::failed(500, "MCP_REQUEST_ENCODE_FAILED", error.to_string())
    })?;
    Ok(MarketDataProviderActionsRequest {
        method: "POST".to_owned(),
        path: BATCH_SNAPSHOTS_PATH.to_owned(),
        query,
        body,
    })
}

fn subscription_query(arguments: &Value) -> Result<String, McpToolFailure> {
    Ok(query_string([
        ("brokerId", super::optional_string(arguments, "brokerId")),
        ("accountId", super::optional_string(arguments, "accountId")),
        ("market", super::optional_string(arguments, "market")),
    ]))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn market_candles_maps_instrument_and_advanced_query() {
        let (path, query) = candle_request(&json!({
            "instrumentId": "us.aapl",
            "operation": "historical",
            "period": "1h",
            "limit": 25,
            "startTime": "2026-01-01T00:00:00Z",
            "endTime": "2026-01-02T00:00:00Z",
            "sessions": ["regular", "extended"],
            "adjustment": "forward"
        }))
        .expect("candle request");
        assert_eq!(path, "/api/v1/market-data/candles/US/AAPL");
        assert_eq!(
            query,
            "period=1h&limit=25&from=2026%2D01%2D01T00%3A00%3A00Z&to=2026%2D01%2D02T00%3A00%3A00Z&sessions=regular%2Cextended&adjustment=forward"
        );
    }

    #[test]
    fn market_snapshots_maps_symbols_to_provider_action_body() {
        let request = snapshot_request(&json!({
            "market": "hk",
            "symbols": ["00700", "US.AAPL", "00700"],
            "brokerId": "active"
        }))
        .expect("snapshot request");
        assert_eq!(request.method, "POST");
        assert_eq!(request.path, BATCH_SNAPSHOTS_PATH);
        assert_eq!(request.query, "brokerId=active&market=HK");
        assert_eq!(
            serde_json::from_slice::<Value>(&request.body).expect("snapshot body"),
            json!({"symbols": ["HK.00700", "US.AAPL"]})
        );
    }

    #[test]
    fn market_subscriptions_forwards_optional_scope_without_fixture_values() {
        assert_eq!(
            subscription_query(&json!({"brokerId": "futu", "accountId": "a1", "market": "US"}))
                .expect("subscription query"),
            "brokerId=futu&accountId=a1&market=US"
        );
    }
}
