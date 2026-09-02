//! MCP adapters for the derivative catalogue and option-analysis routes.
//!
//! Every operation below delegates to the same production snapshot port used
//! by the HTTP API.  The MCP layer only translates its typed tool arguments to
//! the public route path/query; it never manufactures a catalogue or turns an
//! unavailable OpenD reader into an empty successful response.

use serde_json::{Map, Value};

use super::helpers::{arguments_query, instrument};
use super::{McpToolFailure, ProductionMcpToolExecutor, derivative_error, options_error};

const FUTURES_PATH: &str = "/api/v1/market-data/futures";
const WARRANTS_PATH: &str = "/api/v1/market-data/warrants";
const OPTIONS_SCREEN_PATH: &str = "/api/v1/market-data/options/screens";
const OPTIONS_EVENTS_PATH: &str = "/api/v1/market-data/options/events";

impl ProductionMcpToolExecutor {
    pub(super) fn derivative_read(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        let (path, query) = derivative_request(name, arguments)?;
        match name {
            "derivatives.futures" | "derivatives.warrants" => self
                .ports()?
                .market_data_derivative
                .read(&path, &query)
                .map_err(derivative_error),
            "derivatives.option_chain"
            | "derivatives.option_analysis"
            | "derivatives.option_events"
            | "derivatives.option_screen" => self
                .ports()?
                .market_data_options
                .read(&path, &query)
                .map_err(options_error),
            _ => Err(McpToolFailure::unavailable(
                "MCP_TOOL_UNAVAILABLE",
                format!("production derivative executor is not implemented for {name}"),
            )),
        }
    }
}

fn derivative_request(name: &str, arguments: &Value) -> Result<(String, String), McpToolFailure> {
    match name {
        "derivatives.futures" => {
            collection_request(arguments, FUTURES_PATH, "contracts", &["contracts"], &[])
        }
        "derivatives.warrants" => collection_request(
            arguments,
            WARRANTS_PATH,
            "list",
            &["related", "list", "screen"],
            &[],
        ),
        "derivatives.option_screen" => {
            collection_request(arguments, OPTIONS_SCREEN_PATH, "screen", &["screen"], &[])
        }
        "derivatives.option_events" => collection_request(
            arguments,
            OPTIONS_EVENTS_PATH,
            "unusual",
            &[
                "unusual",
                "zero_dte",
                "zero_dte_contract",
                "earnings",
                "seller",
            ],
            &[],
        ),
        "derivatives.option_chain" => option_instrument_request(
            arguments,
            &["chain", "expirations"],
            "/api/v1/market-data/options/{route}/{instrumentId}",
            &["instrumentId", "symbol"],
            &[
                ("startTime", "beginTime"),
                ("from", "beginTime"),
                ("to", "endTime"),
            ],
        ),
        "derivatives.option_analysis" => option_instrument_request(
            arguments,
            super::super::product_production_ports::OPTION_ANALYSIS_OPERATIONS,
            "/api/v1/market-data/options/analysis/{instrumentId}",
            &["instrumentId", "symbol"],
            &[],
        ),
        _ => Err(McpToolFailure::unavailable(
            "MCP_TOOL_UNAVAILABLE",
            format!("production derivative executor is not implemented for {name}"),
        )),
    }
}

fn collection_request(
    arguments: &Value,
    path: &str,
    default_operation: &str,
    operations: &[&str],
    excluded: &[&str],
) -> Result<(String, String), McpToolFailure> {
    let operation = normalized_operation(arguments, default_operation, operations)?;
    let arguments = with_operation(arguments, &operation)?;
    let query = arguments_query(&arguments, excluded, &[])?;
    Ok((path.to_owned(), query))
}

fn option_instrument_request(
    arguments: &Value,
    operations: &[&str],
    path_template: &str,
    excluded: &[&str],
    aliases: &[(&str, &str)],
) -> Result<(String, String), McpToolFailure> {
    let operation = normalized_operation(arguments, operations[0], operations)?;
    let (market, symbol) = instrument(arguments)?;
    validate_instrument_path(&market, &symbol)?;
    let instrument_id = format!("{market}.{symbol}");
    let route = match operation.as_str() {
        "chain" => "chains",
        "expirations" => "expirations",
        _ => operation.as_str(),
    };
    let path = path_template
        .replace("{operation}", &operation)
        .replace("{route}", route)
        .replace("{instrumentId}", &instrument_id);
    let arguments = with_operation(arguments, &operation)?;
    let query = arguments_query(&arguments, excluded, aliases)?;
    Ok((path, query))
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

fn validate_instrument_path(market: &str, symbol: &str) -> Result<(), McpToolFailure> {
    if !matches!(market, "HK" | "US") {
        return Err(McpToolFailure::invalid(
            "derivative instrument market must be HK or US",
        ));
    }
    if symbol
        .chars()
        .any(|value| matches!(value, '\\' | '?' | '#'))
    {
        return Err(McpToolFailure::invalid(
            "instrument contains an invalid path segment",
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn derivative_requests_select_real_http_routes_and_operations() {
        let (path, query) = derivative_request(
            "derivatives.futures",
            &json!({"market": "HK", "operation": "contracts", "pageSize": 25}),
        )
        .expect("future request");
        assert_eq!(path, FUTURES_PATH);
        assert!(query.contains("operation=contracts"));
        assert!(query.contains("pageSize=25"));

        let (path, query) = derivative_request(
            "derivatives.option_chain",
            &json!({
                "instrumentId": "us.aapl",
                "operation": "chain",
                "startTime": "2026-08-01",
                "endTime": "2026-08-31",
                "impliedVolatilityMin": 0.2
            }),
        )
        .expect("option chain request");
        assert_eq!(path, "/api/v1/market-data/options/chains/US.AAPL");
        assert!(query.contains("operation=chain"));
        assert!(query.contains("beginTime=2026%2D08%2D01"));
        assert!(query.contains("endTime=2026%2D08%2D31"));
    }

    #[test]
    fn derivative_requests_reject_invalid_operations_and_path_values() {
        assert!(
            derivative_request(
                "derivatives.option_events",
                &json!({
                    "operation": "unknown"
                })
            )
            .is_err()
        );
        assert!(
            derivative_request(
                "derivatives.option_analysis",
                &json!({
                    "instrumentId": "US/AAPL",
                    "operation": "quote"
                })
            )
            .is_err()
        );
        assert!(
            derivative_request(
                "derivatives.option_chain",
                &json!({
                    "instrumentId": "CN.00001"
                })
            )
            .is_err()
        );
    }

    #[test]
    fn unavailable_warrant_route_is_not_replaced_with_fixture_data() {
        let (path, query) = derivative_request("derivatives.warrants", &json!({})).unwrap();
        assert_eq!(path, WARRANTS_PATH);
        assert!(query.contains("operation=list"));
    }
}
