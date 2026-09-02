//! MCP market-data adapters backed by the production quote/provider ports.
//!
//! The MCP layer is synchronous by contract, while the market-data ports are
//! async or provider-action based. Request construction stays pure and is
//! tested independently; execution always delegates to the installed
//! production port and preserves its baseline error status.

use std::sync::Arc;

use serde_json::{Value, json};

use super::helpers::{
    bounded_integer, instrument, optional_string_array, path_segment, query_string, required_field,
    run_provider_action, run_quote_read,
};
use super::{McpToolFailure, ProductionMcpToolExecutor, provider_actions_error, quote_error};
use crate::product::product_market_data_provider_actions_port::{
    BATCH_SNAPSHOTS_PATH, MarketDataProviderActionsRequest,
};

const SUBSCRIPTIONS_PATH: &str = "/api/v1/market-data/subscriptions";

impl ProductionMcpToolExecutor {
    pub(super) fn market_microstructure(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        let (path, query) = microstructure_request(name, arguments)?;
        let port = Arc::clone(&self.ports()?.market_data_quote);
        run_quote_read(port, path, query).map_err(quote_error)
    }

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
        let payload =
            run_quote_read(port, SUBSCRIPTIONS_PATH.to_owned(), query).map_err(quote_error)?;
        subscription_projection(payload)
    }
}

fn microstructure_request(
    name: &str,
    arguments: &Value,
) -> Result<(String, String), McpToolFailure> {
    let (market, symbol) = instrument(arguments)?;
    if name == "market.depth" {
        let num = bounded_integer(arguments, "num", 10, 1, 50)?;
        return Ok((
            format!("/api/v1/market-data/depth/{market}/{symbol}"),
            query_string([("num", Some(num.to_string()))]),
        ));
    }

    let instrument_id = format!("{market}.{symbol}");
    let path = match name {
        "market.instrument_profile" => {
            format!("/api/v1/market-data/instruments/{instrument_id}/profile")
        }
        "market.intraday" => format!("/api/v1/market-data/intraday/{instrument_id}"),
        "market.ticks" => format!("/api/v1/market-data/ticks/{instrument_id}"),
        "market.broker_queue" => {
            format!("/api/v1/market-data/broker-queue/{instrument_id}")
        }
        "market.capital_flow" => {
            format!("/api/v1/market-data/capital-flow/{instrument_id}")
        }
        _ => {
            return Err(McpToolFailure::unavailable(
                "MCP_TOOL_UNAVAILABLE",
                format!("production market-data executor is not implemented for {name}"),
            ));
        }
    };
    let page_size = bounded_integer(arguments, "pageSize", 50, 1, 100)?;
    let operation = if name == "market.capital_flow" {
        let operation = super::optional_string(arguments, "operation")
            .unwrap_or_else(|| "flow".to_owned())
            .to_ascii_lowercase();
        if !matches!(operation.as_str(), "flow" | "distribution") {
            return Err(McpToolFailure::invalid(
                "operation must be flow or distribution",
            ));
        }
        Some(operation)
    } else {
        None
    };
    let period_type = match optional_query_integer(arguments, "periodType")? {
        Some(value) => Some(value),
        None => optional_query_integer(arguments, "period")?,
    };
    let begin_time = super::optional_string(arguments, "beginTime")
        .or_else(|| super::optional_string(arguments, "startTime"));
    let end_time = super::optional_string(arguments, "endTime");
    Ok((
        path,
        query_string([
            ("brokerId", super::optional_string(arguments, "brokerId")),
            ("accountId", super::optional_string(arguments, "accountId")),
            ("operation", operation),
            ("pageSize", Some(page_size.to_string())),
            ("periodType", period_type),
            ("beginTime", begin_time),
            ("endTime", end_time),
        ]),
    ))
}

fn optional_query_integer(arguments: &Value, key: &str) -> Result<Option<String>, McpToolFailure> {
    if arguments.get(key).is_none() {
        return Ok(None);
    }
    bounded_integer(arguments, key, 0, i64::from(i32::MIN), i64::from(i32::MAX))
        .map(|value| Some(value.to_string()))
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

fn subscription_projection(payload: Value) -> Result<Value, McpToolFailure> {
    let entries = required_field(&payload, "entries", "array")?;
    let mut active_instruments = entries
        .as_array()
        .into_iter()
        .flatten()
        .filter_map(|entry| entry.get("instrumentId").and_then(Value::as_str))
        .map(str::to_owned)
        .collect::<Vec<_>>();
    active_instruments.sort();
    active_instruments.dedup();
    let checked_at = payload
        .pointer("/brokerState/checkedAt")
        .and_then(Value::as_str)
        .filter(|value| !value.trim().is_empty())
        .map(str::to_owned)
        .unwrap_or_else(|| {
            time::OffsetDateTime::now_utc()
                .format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
        });
    Ok(json!({
        "subscriptions": payload,
        "activeInstruments": active_instruments,
        "checkedAt": checked_at,
    }))
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
    fn market_microstructure_maps_all_route_backed_tools_without_fixture_fallbacks() {
        let cases = [
            (
                "market.instrument_profile",
                "/api/v1/market-data/instruments/US.AAPL/profile",
            ),
            ("market.intraday", "/api/v1/market-data/intraday/US.AAPL"),
            ("market.ticks", "/api/v1/market-data/ticks/US.AAPL"),
            (
                "market.broker_queue",
                "/api/v1/market-data/broker-queue/US.AAPL",
            ),
        ];
        for (name, expected_path) in cases {
            let (path, query) =
                microstructure_request(name, &json!({"instrumentId": "us.aapl", "pageSize": 25}))
                    .expect("microstructure request");
            assert_eq!(path, expected_path);
            assert_eq!(query, "pageSize=25");
        }

        let (path, query) = microstructure_request(
            "market.depth",
            &json!({"market": "us", "symbol": "aapl", "num": 50}),
        )
        .expect("depth request");
        assert_eq!(path, "/api/v1/market-data/depth/US/AAPL");
        assert_eq!(query, "num=50");

        let (path, query) = microstructure_request(
            "market.capital_flow",
            &json!({
                "instrumentId": "US.AAPL",
                "brokerId": "futu",
                "operation": "distribution",
                "pageSize": 10,
                "period": 1,
                "startTime": "2026-08-01",
                "endTime": "2026-08-31"
            }),
        )
        .expect("capital-flow request");
        assert_eq!(path, "/api/v1/market-data/capital-flow/US.AAPL");
        assert_eq!(
            query,
            "brokerId=futu&operation=distribution&pageSize=10&periodType=1&beginTime=2026%2D08%2D01&endTime=2026%2D08%2D31"
        );
    }

    #[test]
    fn market_microstructure_preserves_dotted_symbols_for_direct_quote_port_paths() {
        let (profile_path, profile_query) = microstructure_request(
            "market.instrument_profile",
            &json!({"instrumentId": "US.BRK.B"}),
        )
        .expect("profile request");
        assert_eq!(
            profile_path,
            "/api/v1/market-data/instruments/US.BRK.B/profile"
        );
        assert_eq!(profile_query, "pageSize=50");

        let (depth_path, depth_query) =
            microstructure_request("market.depth", &json!({"market": "US", "symbol": "BRK.B"}))
                .expect("depth request");
        assert_eq!(depth_path, "/api/v1/market-data/depth/US/BRK.B");
        assert_eq!(depth_query, "num=10");
    }

    #[test]
    fn market_microstructure_rejects_invalid_operation_and_bounds_before_the_port() {
        assert!(
            microstructure_request(
                "market.capital_flow",
                &json!({"instrumentId": "US.AAPL", "operation": "unknown"}),
            )
            .is_err()
        );
        assert!(
            microstructure_request(
                "market.depth",
                &json!({"instrumentId": "US.AAPL", "num": 51}),
            )
            .is_err()
        );
        assert!(
            microstructure_request(
                "market.ticks",
                &json!({"instrumentId": "US.AAPL", "pageSize": 0})
            )
            .is_err()
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
        let projected = subscription_projection(json!({
            "entries": [{"instrumentId": "US.MSFT"}, {"instrumentId": "US.AAPL"}, {"instrumentId": "US.MSFT"}],
            "brokerState": {"checkedAt": "2026-01-01T00:00:00Z"}
        }))
        .expect("subscription projection");
        assert_eq!(
            projected["activeInstruments"],
            json!(["US.AAPL", "US.MSFT"])
        );
        assert_eq!(projected["checkedAt"], "2026-01-01T00:00:00Z");
    }
}
