use std::sync::Arc;

use percent_encoding::{NON_ALPHANUMERIC, utf8_percent_encode};
use serde_json::Value;

use super::McpToolFailure;
use crate::product::product_market_data_provider_actions_port::{
    MarketDataProviderActionsPort, MarketDataProviderActionsPortError,
    MarketDataProviderActionsRequest,
};
use crate::product::{MarketDataCatalogReadSnapshotError, MarketDataQuoteReadSnapshotError};

/// Return a required object field without manufacturing a default.  MCP
/// projections are contract adapters, so a malformed production port payload
/// must fail closed instead of turning a missing field into `null`/`[]`.
pub(super) fn required_field(
    payload: &Value,
    key: &str,
    expected: &str,
) -> Result<Value, McpToolFailure> {
    let object = payload.as_object().ok_or_else(|| {
        McpToolFailure::failed(
            502,
            "MCP_PRODUCTION_PAYLOAD_INVALID",
            "production adapter returned a non-object payload",
        )
    })?;
    let value = object.get(key).ok_or_else(|| {
        McpToolFailure::failed(
            502,
            "MCP_PRODUCTION_PAYLOAD_INVALID",
            format!("production adapter payload is missing {key}"),
        )
    })?;
    let valid = match expected {
        "array" => value.is_array(),
        "object" => value.is_object(),
        "string" => value.as_str().is_some_and(|text| !text.trim().is_empty()),
        _ => true,
    };
    if !valid {
        return Err(McpToolFailure::failed(
            502,
            "MCP_PRODUCTION_PAYLOAD_INVALID",
            format!("production adapter payload field {key} is not a valid {expected}"),
        ));
    }
    Ok(value.clone())
}

/// Validate the nullable `runs` field emitted by the production backtest
/// snapshot. An empty production store intentionally uses `null` (matching
/// the existing HTTP shape); a missing or non-array field is an adapter
/// contract violation and must not be treated as an empty result.
pub(super) fn nullable_runs<'a>(
    payload: &'a Value,
) -> Result<Option<&'a Vec<Value>>, McpToolFailure> {
    let Some(value) = payload.get("runs") else {
        return Err(McpToolFailure::failed(
            502,
            "MCP_PRODUCTION_PAYLOAD_INVALID",
            "backtest adapter payload is missing runs",
        ));
    };
    if value.is_null() {
        return Ok(None);
    }
    value.as_array().map(Some).ok_or_else(|| {
        McpToolFailure::failed(
            502,
            "MCP_PRODUCTION_PAYLOAD_INVALID",
            "backtest adapter payload field runs is not an array or null",
        )
    })
}

pub(super) fn same_observed_string(
    payloads: &[&Value],
    key: &str,
) -> Result<Option<Value>, McpToolFailure> {
    let mut observed = None;
    for payload in payloads {
        let Some(value) = payload.as_object().and_then(|object| object.get(key)) else {
            continue;
        };
        if !value.is_string() {
            return Err(McpToolFailure::failed(
                502,
                "MCP_PRODUCTION_PAYLOAD_INVALID",
                format!("production adapter payload field {key} is not a string"),
            ));
        }
        if let Some(previous) = observed.as_ref()
            && previous != value
        {
            return Err(McpToolFailure::failed(
                502,
                "MCP_PRODUCTION_PAYLOAD_INCONSISTENT",
                format!("production adapter payload field {key} disagrees across reads"),
            ));
        }
        observed = Some(value.clone());
    }
    Ok(observed)
}

pub(super) fn first_observed_string(
    payloads: &[&Value],
    key: &str,
) -> Result<Option<Value>, McpToolFailure> {
    for payload in payloads {
        let Some(value) = payload.as_object().and_then(|object| object.get(key)) else {
            continue;
        };
        if !value.is_string() {
            return Err(McpToolFailure::failed(
                502,
                "MCP_PRODUCTION_PAYLOAD_INVALID",
                format!("production adapter payload field {key} is not a string"),
            ));
        }
        return Ok(Some(value.clone()));
    }
    Ok(None)
}

pub(super) fn required_string(arguments: &Value, key: &str) -> Result<String, McpToolFailure> {
    super::optional_string(arguments, key)
        .ok_or_else(|| McpToolFailure::invalid(format!("{key} is required")))
}

pub(super) fn bounded_integer(
    arguments: &Value,
    key: &str,
    default: i64,
    min: i64,
    max: i64,
) -> Result<i64, McpToolFailure> {
    let Some(value) = arguments.get(key) else {
        return Ok(default);
    };
    let parsed = match value {
        Value::Number(number) => number
            .as_i64()
            .ok_or_else(|| McpToolFailure::invalid(format!("{key} must be an integer")))?,
        Value::String(text) => text
            .trim()
            .parse::<i64>()
            .map_err(|_| McpToolFailure::invalid(format!("{key} must be an integer")))?,
        _ => return Err(McpToolFailure::invalid(format!("{key} must be an integer"))),
    };
    if !(min..=max).contains(&parsed) {
        return Err(McpToolFailure::invalid(format!(
            "{key} must be between {min} and {max}"
        )));
    }
    Ok(parsed)
}

pub(super) fn optional_bool_strict(
    arguments: &Value,
    key: &str,
    default: bool,
) -> Result<bool, McpToolFailure> {
    let Some(value) = arguments.get(key) else {
        return Ok(default);
    };
    match value {
        Value::Bool(value) => Ok(*value),
        Value::String(value) => match value.trim().to_ascii_lowercase().as_str() {
            "true" | "1" | "yes" | "y" => Ok(true),
            "false" | "0" | "no" | "n" => Ok(false),
            _ => Err(McpToolFailure::invalid(format!("{key} must be a boolean"))),
        },
        _ => Err(McpToolFailure::invalid(format!("{key} must be a boolean"))),
    }
}

/// Decode a MCP string-array argument without silently accepting malformed
/// values. Empty items are rejected so a provider action can never receive a
/// synthetic/empty instrument and report a misleading success.
pub(super) fn optional_string_array(
    arguments: &Value,
    key: &str,
) -> Result<Option<Vec<String>>, McpToolFailure> {
    let Some(value) = arguments.get(key) else {
        return Ok(None);
    };
    let values = value
        .as_array()
        .ok_or_else(|| McpToolFailure::invalid(format!("{key} must be an array")))?;
    let mut result = Vec::with_capacity(values.len());
    for item in values {
        let text = item
            .as_str()
            .map(str::trim)
            .filter(|text| !text.is_empty())
            .ok_or_else(|| McpToolFailure::invalid(format!("{key} must contain strings")))?;
        result.push(text.to_owned());
    }
    Ok(Some(result))
}

pub(super) fn instrument(arguments: &Value) -> Result<(String, String), McpToolFailure> {
    let instrument_id = super::optional_string(arguments, "instrumentId");
    let (market, symbol) = if let Some(instrument_id) = instrument_id {
        instrument_id
            .split_once('.')
            .map(|(market, symbol)| (market.to_owned(), symbol.to_owned()))
            .ok_or_else(|| McpToolFailure::invalid("instrumentId must be MARKET.SYMBOL"))?
    } else {
        (
            required_string(arguments, "market")?,
            required_string(arguments, "symbol")?,
        )
    };
    let market = market.trim().to_ascii_uppercase();
    let symbol = symbol.trim().to_ascii_uppercase();
    if market.is_empty() || symbol.is_empty() {
        return Err(McpToolFailure::invalid("market and symbol are required"));
    }
    for value in [&market, &symbol] {
        if value
            .chars()
            .any(|character| character.is_control() || character == '/')
        {
            return Err(McpToolFailure::invalid(
                "instrument contains an invalid path segment",
            ));
        }
    }
    Ok((market, symbol))
}

pub(super) fn path_segment(value: &str) -> String {
    utf8_percent_encode(value, NON_ALPHANUMERIC).to_string()
}

pub(super) fn query_string<const N: usize>(fields: [(&str, Option<String>); N]) -> String {
    fields
        .into_iter()
        .filter_map(|(key, value)| {
            value
                .filter(|value| !value.trim().is_empty())
                .map(|value| (key, value))
        })
        .map(|(key, value)| format!("{key}={}", utf8_percent_encode(&value, NON_ALPHANUMERIC)))
        .collect::<Vec<_>>()
        .join("&")
}

pub(super) fn broker_query(arguments: &Value, scope: String) -> String {
    query_string([
        ("scope", Some(scope)),
        (
            "tradingEnvironment",
            super::optional_string(arguments, "tradingEnvironment"),
        ),
        ("accountId", super::optional_string(arguments, "accountId")),
        ("market", super::optional_string(arguments, "market")),
        ("symbol", super::optional_string(arguments, "symbol")),
        ("startTime", super::optional_string(arguments, "startTime")),
        ("endTime", super::optional_string(arguments, "endTime")),
    ])
}

pub(super) fn validate_result_view_arguments(arguments: &Value) -> Result<(), McpToolFailure> {
    if let Some(view) = super::optional_string(arguments, "view")
        && !matches!(
            view.to_ascii_lowercase().as_str(),
            "summary" | "chart" | "orders" | "logs" | "warnings" | "errors"
        )
    {
        return Err(McpToolFailure::invalid(
            "view must be summary, chart, orders, logs, warnings, or errors",
        ));
    }
    if arguments.get("include").is_some() {
        let Some(include) = arguments.get("include").and_then(Value::as_array) else {
            return Err(McpToolFailure::invalid("include must be an array"));
        };
        if include.iter().any(|value| {
            !value.as_str().is_some_and(|item| {
                matches!(item, "candles" | "trades" | "pnlCurve" | "drawdownCurve")
            })
        }) {
            return Err(McpToolFailure::invalid(
                "include contains an unsupported series",
            ));
        }
    }
    if arguments.get("limit").is_some() {
        bounded_integer(arguments, "limit", 0, 1, 2_000)?;
    }
    Ok(())
}

pub(super) fn add_retry_hint(mut payload: Value) -> Value {
    if let Some(object) = payload.as_object_mut()
        && !object.contains_key("readyToRetry")
    {
        let ready = object
            .get("status")
            .and_then(Value::as_str)
            .is_some_and(|status| status.eq_ignore_ascii_case("completed"));
        object.insert("readyToRetry".to_owned(), Value::Bool(ready));
    }
    payload
}

pub(super) fn matches_filter(
    value: &Value,
    key: &str,
    expected: Option<&str>,
    fold_case: bool,
) -> bool {
    let Some(expected) = expected else {
        return true;
    };
    let actual = value.get(key).and_then(Value::as_str).unwrap_or_default();
    if fold_case {
        actual.eq_ignore_ascii_case(expected)
    } else {
        actual == expected
    }
}

pub(super) fn run_catalog_read(
    port: Arc<dyn crate::product::MarketDataCatalogReadSnapshotPort>,
    path: &'static str,
    query: String,
) -> Result<Value, MarketDataCatalogReadSnapshotError> {
    std::thread::spawn(move || {
        tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| MarketDataCatalogReadSnapshotError::Unavailable(error.to_string()))?
            .block_on(port.read(path, &query))
    })
    .join()
    .map_err(|_| {
        MarketDataCatalogReadSnapshotError::Unavailable("catalog worker panicked".to_owned())
    })?
}

pub(super) fn run_quote_read(
    port: Arc<dyn crate::product::MarketDataQuoteReadSnapshotPort>,
    path: String,
    query: String,
) -> Result<Value, MarketDataQuoteReadSnapshotError> {
    std::thread::spawn(move || {
        tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| MarketDataQuoteReadSnapshotError::Unavailable(error.to_string()))?
            .block_on(port.read(&path, &query))
    })
    .join()
    .map_err(|_| {
        MarketDataQuoteReadSnapshotError::Unavailable("quote worker panicked".to_owned())
    })?
}

/// Bridge the synchronous MCP executor to the provider-action port's async
/// contract. A dedicated current-thread runtime avoids blocking whichever
/// Tokio runtime is serving the HTTP/MCP request while preserving the same
/// production port and error semantics.
pub(super) fn run_provider_action(
    port: Arc<dyn MarketDataProviderActionsPort>,
    request: MarketDataProviderActionsRequest,
) -> Result<Value, MarketDataProviderActionsPortError> {
    std::thread::spawn(move || {
        tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|error| MarketDataProviderActionsPortError::Unavailable(error.to_string()))?
            .block_on(async move { port.dispatch(&request).await })
    })
    .join()
    .map_err(|_| {
        MarketDataProviderActionsPortError::Unavailable(
            "provider action worker panicked".to_owned(),
        )
    })?
}
