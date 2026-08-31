//! MCP adapters for the prediction-market read and quote operations.
//!
//! Prediction tools are thin adapters over the production prediction reader
//! and provider-action ports.  They build the same route path/query/body that
//! the HTTP API consumes; no fixture or synthetic successful payload is
//! created when OpenD or the provider is unavailable.

use std::sync::Arc;

use serde_json::{Map, Value};

use super::helpers::{arguments_query, bounded_integer, required_string, run_provider_action};
use super::{McpToolFailure, ProductionMcpToolExecutor, prediction_error, provider_actions_error};
use crate::product::product_market_data_provider_actions_port::{
    MarketDataProviderActionsRequest, PREDICTION_COMBO_QUOTES_PATH,
};

const PREDICTION_CATEGORIES_PATH: &str = "/api/v1/market-data/prediction/categories";
const PREDICTION_COMPETITIONS_PATH: &str = "/api/v1/market-data/prediction/competitions";
const PREDICTION_SERIES_PATH: &str = "/api/v1/market-data/prediction/series";
const PREDICTION_EVENTS_PATH: &str = "/api/v1/market-data/prediction/events";
const PREDICTION_ELIGIBLE_EVENTS_PATH: &str =
    "/api/v1/market-data/prediction/combos/eligible-events";

impl ProductionMcpToolExecutor {
    pub(super) fn prediction(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        if name == "prediction.combo_quote" {
            return self.prediction_combo_quote(arguments);
        }
        let (path, query) = prediction_read_request(name, arguments)?;
        self.ports()?
            .market_data_prediction
            .read(&path, &query)
            .map_err(prediction_error)
    }

    fn prediction_combo_quote(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        validate_combo_quote(arguments)?;
        let object = arguments
            .as_object()
            .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
        let query = query_context(object)?;
        let body = serde_json::to_vec(arguments).map_err(|error| {
            McpToolFailure::failed(500, "MCP_REQUEST_ENCODE_FAILED", error.to_string())
        })?;
        let request = MarketDataProviderActionsRequest {
            method: "POST".to_owned(),
            path: PREDICTION_COMBO_QUOTES_PATH.to_owned(),
            query,
            body,
        };
        let port = Arc::clone(&self.ports()?.market_data_provider_actions);
        run_provider_action(port, request).map_err(provider_actions_error)
    }
}

fn prediction_read_request(
    name: &str,
    arguments: &Value,
) -> Result<(String, String), McpToolFailure> {
    match name {
        "prediction.discover" => prediction_discover_request(arguments),
        "prediction.snapshot" => prediction_contract_request(arguments, "snapshot"),
        "prediction.depth" => prediction_depth_request(arguments),
        "prediction.history" => prediction_history_request(arguments),
        "prediction.combo_eligible" => prediction_combo_eligible_request(arguments),
        _ => Err(McpToolFailure::unavailable(
            "MCP_TOOL_UNAVAILABLE",
            format!("production prediction executor is not implemented for {name}"),
        )),
    }
}

fn prediction_discover_request(arguments: &Value) -> Result<(String, String), McpToolFailure> {
    const OPERATIONS: &[&str] = &[
        "categories",
        "competitions",
        "series",
        "events",
        "contracts",
        "milestones",
    ];
    let operation = normalized_operation(arguments, "categories", OPERATIONS)?;
    let (path, excluded, aliases) = match operation.as_str() {
        "categories" => (PREDICTION_CATEGORIES_PATH, &[][..], &[][..]),
        "competitions" => (PREDICTION_COMPETITIONS_PATH, &[][..], &[][..]),
        "series" => (PREDICTION_SERIES_PATH, &[][..], &[][..]),
        "events" => {
            require_alias(arguments, &["seriesId", "series"], "seriesId")?;
            let mut arguments = with_operation(arguments, &operation)?;
            let object = arguments
                .as_object_mut()
                .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
            normalize_alias(object, "seriesId", &["seriesId", "series"])?;
            let query = arguments_query(&arguments, &[], &[])?;
            return Ok((PREDICTION_EVENTS_PATH.to_owned(), query));
        }
        "contracts" => {
            let event_id = required_prediction_value(arguments, "eventId")?;
            let path = format!("/api/v1/market-data/prediction/events/{event_id}/contracts");
            let arguments = with_operation(arguments, &operation)?;
            let query = arguments_query(&arguments, &["eventId"], &[])?;
            return Ok((path, query));
        }
        "milestones" => {
            let code = prediction_code(arguments, &["code", "instrumentId"])?;
            let path = format!("/api/v1/market-data/prediction/contracts/{code}/milestones");
            let arguments = with_operation(arguments, &operation)?;
            let query = arguments_query(&arguments, &["code", "instrumentId"], &[])?;
            return Ok((path, query));
        }
        _ => unreachable!("operation was validated above"),
    };
    let arguments = with_operation(arguments, &operation)?;
    let query = arguments_query(&arguments, excluded, aliases)?;
    Ok((path.to_owned(), query))
}

fn prediction_contract_request(
    arguments: &Value,
    operation: &str,
) -> Result<(String, String), McpToolFailure> {
    let code = prediction_code(arguments, &["code", "instrumentId"])?;
    let arguments = with_operation(arguments, operation)?;
    let query = arguments_query(&arguments, &["code", "instrumentId"], &[])?;
    Ok((
        format!(
            "/api/v1/market-data/prediction/contracts/{code}/{operation_path}",
            operation_path = if operation == "snapshot" {
                "snapshot"
            } else {
                operation
            }
        ),
        query,
    ))
}

fn prediction_depth_request(arguments: &Value) -> Result<(String, String), McpToolFailure> {
    let code = prediction_code(arguments, &["code", "instrumentId"])?;
    let depth = integer_alias(arguments, &["depth", "num"], "depth", 10, 1, 100)?;
    let mut arguments = with_operation(arguments, "order_book")?;
    let object = arguments
        .as_object_mut()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    object.remove("code");
    object.remove("instrumentId");
    object.remove("depth");
    object.remove("num");
    object.insert("depth".to_owned(), Value::from(depth));
    let query = arguments_query(&arguments, &[], &[])?;
    Ok((
        format!("/api/v1/market-data/prediction/contracts/{code}/order-book"),
        query,
    ))
}

fn prediction_history_request(arguments: &Value) -> Result<(String, String), McpToolFailure> {
    const OPERATIONS: &[&str] = &["candles", "historical", "ticks"];
    let operation = normalized_operation(arguments, "candles", OPERATIONS)?;
    let code = prediction_code(arguments, &["code", "instrumentId"])?;
    let mut arguments = with_operation(arguments, &operation)?;
    normalize_history_arguments(&mut arguments, operation.as_str())?;
    let query = arguments_query(
        &arguments,
        &["code", "instrumentId"],
        &[("interval", "period")],
    )?;
    let route = match operation.as_str() {
        "candles" => "candles",
        "historical" => "candles/history",
        "ticks" => "ticks",
        _ => unreachable!("operation was validated above"),
    };
    Ok((
        format!("/api/v1/market-data/prediction/contracts/{code}/{route}"),
        query,
    ))
}

fn prediction_combo_eligible_request(
    arguments: &Value,
) -> Result<(String, String), McpToolFailure> {
    let mut arguments = with_operation(arguments, "eligible_events")?;
    let object = arguments
        .as_object_mut()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    normalize_alias(object, "seriesId", &["seriesId", "series"])?;
    let query = arguments_query(&arguments, &[], &[])?;
    Ok((PREDICTION_ELIGIBLE_EVENTS_PATH.to_owned(), query))
}

fn normalized_operation(
    arguments: &Value,
    default: &str,
    allowed: &[&str],
) -> Result<String, McpToolFailure> {
    let operation = match arguments.get("operation") {
        None => default.to_owned(),
        Some(Value::String(value)) => {
            let value = value.trim().to_ascii_lowercase();
            if value.is_empty() {
                default.to_owned()
            } else {
                value
            }
        }
        Some(_) => return Err(McpToolFailure::invalid("operation must be a string")),
    };
    if allowed.contains(&operation.as_str()) {
        Ok(operation)
    } else {
        Err(McpToolFailure::invalid(format!(
            "operation must be one of {}",
            allowed.join(", ")
        )))
    }
}

fn with_operation(arguments: &Value, operation: &str) -> Result<Value, McpToolFailure> {
    let mut object: Map<String, Value> = arguments
        .as_object()
        .cloned()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    object.insert("operation".to_owned(), Value::String(operation.to_owned()));
    Ok(Value::Object(object))
}

fn required_prediction_value(arguments: &Value, key: &str) -> Result<String, McpToolFailure> {
    let value = required_string(arguments, key)?;
    validate_path_value(&value, key)
}

fn prediction_code(arguments: &Value, keys: &[&str]) -> Result<String, McpToolFailure> {
    let raw = require_alias(arguments, keys, "code")?;
    let raw = raw
        .strip_prefix("US.")
        .or_else(|| raw.strip_prefix("us."))
        .unwrap_or(&raw);
    let value = validate_path_value(raw, "code")?;
    Ok(value.to_ascii_uppercase())
}

fn validate_path_value(value: &str, label: &str) -> Result<String, McpToolFailure> {
    let value = value.trim();
    if value.is_empty()
        || value.len() > 512
        || value.chars().any(|character| {
            character.is_control()
                || character.is_whitespace()
                || matches!(character, '/' | '\\' | '?' | '#' | '%')
        })
    {
        return Err(McpToolFailure::invalid(format!("{label} is invalid")));
    }
    Ok(value.to_owned())
}

fn require_alias(arguments: &Value, keys: &[&str], label: &str) -> Result<String, McpToolFailure> {
    let mut found = None;
    for key in keys {
        if let Some(value) = arguments.get(*key) {
            let value = value
                .as_str()
                .ok_or_else(|| McpToolFailure::invalid(format!("{key} must be a string")))?;
            let value = value.trim();
            if value.is_empty() {
                return Err(McpToolFailure::invalid(format!("{label} is required")));
            }
            if found.is_some_and(|previous: String| previous != value) {
                return Err(McpToolFailure::invalid(format!("{label} aliases disagree")));
            }
            found = Some(value.to_owned());
        }
    }
    found.ok_or_else(|| McpToolFailure::invalid(format!("{label} is required")))
}

fn integer_alias(
    arguments: &Value,
    keys: &[&str],
    label: &str,
    default: i64,
    min: i64,
    max: i64,
) -> Result<i64, McpToolFailure> {
    let mut parsed = None;
    for key in keys {
        if arguments.get(*key).is_some() {
            let value = bounded_integer(arguments, key, default, min, max)?;
            if parsed.is_some_and(|previous| previous != value) {
                return Err(McpToolFailure::invalid(format!("{label} aliases disagree")));
            }
            parsed = Some(value);
        }
    }
    Ok(parsed.unwrap_or(default))
}

fn normalize_history_arguments(
    arguments: &mut Value,
    operation: &str,
) -> Result<(), McpToolFailure> {
    let object = arguments
        .as_object_mut()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    normalize_alias(object, "period", &["period", "interval"])?;
    normalize_alias(object, "from", &["from", "startTime"])?;
    normalize_alias(object, "to", &["to", "endTime"])?;
    if operation == "historical" {
        if !object
            .get("from")
            .and_then(Value::as_str)
            .is_some_and(|value| !value.trim().is_empty())
        {
            return Err(McpToolFailure::invalid("from is required"));
        }
        if !object
            .get("to")
            .and_then(Value::as_str)
            .is_some_and(|value| !value.trim().is_empty())
        {
            return Err(McpToolFailure::invalid("to is required"));
        }
    }
    Ok(())
}

fn normalize_alias(
    object: &mut Map<String, Value>,
    canonical: &str,
    aliases: &[&str],
) -> Result<(), McpToolFailure> {
    let mut value = None;
    for key in aliases {
        if let Some(item) = object.get(*key) {
            let item = item
                .as_str()
                .ok_or_else(|| McpToolFailure::invalid(format!("{key} must be a string")))?;
            let item = item.trim();
            if item.is_empty() {
                return Err(McpToolFailure::invalid(format!(
                    "{canonical} must not be empty"
                )));
            }
            if value.is_some_and(|previous: String| previous != item) {
                return Err(McpToolFailure::invalid(format!(
                    "{canonical} aliases disagree"
                )));
            }
            value = Some(item.to_owned());
        }
    }
    for key in aliases {
        if *key != canonical {
            object.remove(*key);
        }
    }
    if let Some(value) = value {
        object.insert(canonical.to_owned(), Value::String(value));
    }
    Ok(())
}

fn validate_combo_quote(arguments: &Value) -> Result<(), McpToolFailure> {
    let object = arguments
        .as_object()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    let mvc = required_string(arguments, "mvc")?;
    if mvc.len() > 256 || mvc.chars().any(char::is_control) {
        return Err(McpToolFailure::invalid("mvc is invalid"));
    }
    let legs = object
        .get("legs")
        .or_else(|| object.get("comboLegList"))
        .and_then(Value::as_array)
        .ok_or_else(|| McpToolFailure::invalid("legs are required"))?;
    if !(2..=20).contains(&legs.len()) {
        return Err(McpToolFailure::invalid(
            "legs must contain between 2 and 20 items",
        ));
    }
    for (index, leg) in legs.iter().enumerate() {
        let Some(leg) = leg.as_object() else {
            return Err(McpToolFailure::invalid(format!(
                "legs[{index}] must be an object"
            )));
        };
        let instrument = leg
            .get("instrumentId")
            .and_then(Value::as_str)
            .ok_or_else(|| {
                McpToolFailure::invalid(format!("legs[{index}].instrumentId is required"))
            })?;
        validate_path_value(instrument, &format!("legs[{index}].instrumentId"))?;
        let side = leg
            .get("predictionSide")
            .and_then(Value::as_str)
            .ok_or_else(|| {
                McpToolFailure::invalid(format!("legs[{index}].predictionSide is required"))
            })?;
        if !matches!(side.trim().to_ascii_uppercase().as_str(), "YES" | "NO") {
            return Err(McpToolFailure::invalid(format!(
                "legs[{index}].predictionSide must be YES or NO"
            )));
        }
        if let Some(ratio) = leg.get("ratio") {
            let ratio = ratio.as_i64().ok_or_else(|| {
                McpToolFailure::invalid(format!("legs[{index}].ratio must be an integer"))
            })?;
            if !(1..=100).contains(&ratio) {
                return Err(McpToolFailure::invalid(format!(
                    "legs[{index}].ratio must be between 1 and 100"
                )));
            }
        }
        if let Some(side) = leg.get("side") {
            let side = side.as_str().ok_or_else(|| {
                McpToolFailure::invalid(format!("legs[{index}].side must be a string"))
            })?;
            if !matches!(side.trim().to_ascii_uppercase().as_str(), "BUY" | "SELL") {
                return Err(McpToolFailure::invalid(format!(
                    "legs[{index}].side must be BUY or SELL"
                )));
            }
        }
    }
    Ok(())
}

fn query_context(object: &Map<String, Value>) -> Result<String, McpToolFailure> {
    let mut values = Map::new();
    for key in ["brokerId", "accountId", "tradingEnvironment", "market"] {
        if let Some(value) = object.get(key) {
            if !value.is_string() {
                return Err(McpToolFailure::invalid(format!("{key} must be a string")));
            }
            values.insert(key.to_owned(), value.clone());
        }
    }
    arguments_query(&Value::Object(values), &[], &[])
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn prediction_discovery_requests_select_real_routes_and_required_context() {
        let (path, query) = prediction_read_request(
            "prediction.discover",
            &json!({"operation": "events", "seriesId": "SERIES.1", "pageSize": 20}),
        )
        .expect("prediction events request");
        assert_eq!(path, PREDICTION_EVENTS_PATH);
        assert!(query.contains("operation=events"));
        assert!(query.contains("seriesId=SERIES%2E1"));
        assert!(
            prediction_read_request(
                "prediction.discover",
                &json!({
                    "operation": "events"
                })
            )
            .is_err()
        );
    }

    #[test]
    fn prediction_contract_requests_normalize_code_and_alias_history_times() {
        let (path, query) = prediction_read_request(
            "prediction.history",
            &json!({
                "operation": "historical",
                "instrumentId": "US.EC-42",
                "startTime": "2026-08-01T00:00:00Z",
                "endTime": "2026-08-02T00:00:00Z",
                "interval": "5m",
                "pageSize": 50
            }),
        )
        .expect("historical request");
        assert_eq!(
            path,
            "/api/v1/market-data/prediction/contracts/EC-42/candles/history"
        );
        assert!(query.contains("from=2026%2D08%2D01T00%3A00%3A00Z"));
        assert!(query.contains("to=2026%2D08%2D02T00%3A00%3A00Z"));
        assert!(query.contains("period=5m"));
        assert!(!query.contains("startTime"));
        assert!(!query.contains("interval"));
    }

    #[test]
    fn prediction_depth_and_combo_quote_reject_malformed_arguments() {
        let (_, query) = prediction_read_request(
            "prediction.depth",
            &json!({"instrumentId": "US.EC-42", "num": 20}),
        )
        .expect("depth request");
        assert!(query.contains("depth=20"));
        assert!(
            prediction_read_request(
                "prediction.depth",
                &json!({"instrumentId": "US.EC-42", "depth": 101}),
            )
            .is_err()
        );
        assert!(
            validate_combo_quote(&json!({
                "mvc": "US.MVC",
                "legs": [{"instrumentId": "US.A", "predictionSide": "YES"}]
            }))
            .is_err()
        );
    }
}
