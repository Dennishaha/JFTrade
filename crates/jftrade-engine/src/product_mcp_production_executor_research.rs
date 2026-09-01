//! Production MCP adapters for alerts and the research read surface.
//!
//! These tools deliberately call the same snapshot ports as the HTTP product
//! routes.  The MCP boundary only normalizes arguments into the route path or
//! query string; it does not own provider selection, helper lifecycle, or
//! research payload projection.

use serde_json::Value;

use crate::product::AlertKind;
use crate::product::product_research_screen_write_port::{
    RESEARCH_SCREEN_PATH, ResearchScreenWriteRequest, dispatch_research_screen_write,
};

use super::errors::{alert_error, news_search_error, research_error, screen_catalog_error};
use super::helpers::{arguments_query, bounded_integer, instrument, query_string};
use super::{McpToolFailure, ProductionMcpToolExecutor};

impl ProductionMcpToolExecutor {
    /// Read either of the production alert snapshots. The concrete alert
    /// reader remains Futu/OpenD-owned; this method only validates the public
    /// list query and forwards it to the shared port.
    pub(super) fn alerts_read(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        let kind = match name {
            "alerts.price.list" => AlertKind::Price,
            "alerts.option_event.list" => AlertKind::OptionEvents,
            _ => {
                return Err(McpToolFailure::unavailable(
                    "MCP_TOOL_UNAVAILABLE",
                    format!("unsupported alert tool {name}"),
                ));
            }
        };
        let broker_id = super::optional_string(arguments, "brokerId");
        if broker_id
            .as_deref()
            .is_some_and(|value| !value.eq_ignore_ascii_case("futu"))
        {
            return Err(McpToolFailure::failed(
                409,
                "BROKER_CAPABILITY_UNAVAILABLE",
                "alerts are currently served by the futu broker only",
            ));
        }
        let page_size = match kind {
            AlertKind::Price => None,
            AlertKind::OptionEvents => Some(bounded_integer(arguments, "pageSize", 100, 1, 100)?),
        };
        let query = query_string([
            ("brokerId", broker_id),
            ("accountId", super::optional_string(arguments, "accountId")),
            ("market", super::optional_string(arguments, "market")),
            ("cursor", super::optional_string(arguments, "cursor")),
            ("pageSize", page_size.map(|value| value.to_string())),
        ]);
        self.ports()?
            .alert_snapshot
            .snapshot(kind, &query)
            .map_err(alert_error)
    }

    /// Dispatch all research tools with a concrete Rust production port.
    /// Unsupported research sub-capabilities are intentionally not routed
    /// here; their MCP descriptors stay fail-closed until a typed port is
    /// installed.
    pub(super) fn research_read(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        match name {
            "research.screen_catalog" => self.research_screen_catalog(arguments),
            "research.screen" => self.research_screen(arguments),
            "research.news" => self.research_news(arguments),
            "research.instrument"
            | "research.financials"
            | "research.analyst"
            | "research.ownership"
            | "research.corporate_actions"
            | "research.valuation" => self.research_instrument_read(name, arguments),
            "research.institutions" => self.research_institutions(arguments),
            "research.short_interest" => self.research_short_interest(arguments),
            "research.technical_indicators" => self.research_technical_indicators(arguments),
            "research.rankings" | "research.industry" | "research.calendar" | "research.macro" => {
                self.research_market_read(name, arguments)
            }
            _ => Err(McpToolFailure::unavailable(
                "MCP_TOOL_UNAVAILABLE",
                format!("unsupported research tool {name}"),
            )),
        }
    }

    fn research_instrument_read(
        &self,
        name: &str,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        let (market, symbol) = instrument(arguments)?;
        let path = format!(
            "/api/v1/research/{}/{}.{}",
            research_instrument_route(name)?,
            market,
            symbol,
        );
        let query = arguments_query(arguments, &["instrumentId", "market", "symbol"], &[])?;
        self.ports()?
            .research_read
            .read(&path, &query)
            .map_err(research_error)
    }

    fn research_institutions(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        // `underlying` is retained in the reviewed collection-tool schema for
        // Go compatibility, but the institution endpoint has no such filter.
        let query = arguments_query(arguments, &["underlying"], &[])?;
        self.ports()?
            .research_read
            .read("/api/v1/research/institutions", &query)
            .map_err(research_error)
    }

    fn research_short_interest(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let (market, symbol) = instrument(arguments)?;
        let path = format!("/api/v1/research/short-interest/{market}.{symbol}");
        // The reviewed instrument schema carries generic series filters. The
        // short-interest endpoint never consumed them, so accept and ignore
        // them instead of forwarding an unsupported HTTP query parameter.
        let query = arguments_query(
            arguments,
            &[
                "instrumentId",
                "market",
                "symbol",
                "startTime",
                "endTime",
                "period",
            ],
            &[],
        )?;
        self.ports()?
            .research_read
            .read(&path, &query)
            .map_err(research_error)
    }

    fn research_technical_indicators(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let (market, symbol) = instrument(arguments)?;
        let path = format!("/api/v1/research/technical-indicators/{market}.{symbol}");
        let query = technical_indicator_query(arguments)?;
        self.ports()?
            .research_read
            .read(&path, &query)
            .map_err(research_error)
    }

    fn research_market_read(&self, name: &str, arguments: &Value) -> Result<Value, McpToolFailure> {
        let (path, query) = market_research_request(name, arguments)?;
        self.ports()?
            .research_read
            .read(path, &query)
            .map_err(research_error)
    }

    fn research_news(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        // The current MCP schema requires instrumentId for this tool. Keep a
        // compatibility fallback for callers that provide market/symbol, but
        // never manufacture a keyword or an empty news request.
        let (market, symbol) = instrument(arguments)?;
        let instrument_id = format!("{market}.{symbol}");
        let mut arguments = arguments.clone();
        let object = arguments
            .as_object_mut()
            .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
        object.insert("instrumentId".to_owned(), Value::String(instrument_id));
        object.remove("market");
        object.remove("symbol");
        let query = arguments_query(&arguments, &[], &[])?;
        self.ports()?
            .market_data_news_search
            .read("/api/v1/market-data/news", &query)
            .map_err(news_search_error)
    }

    fn research_screen_catalog(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        // Keep the composition-root guard even though the catalog itself is
        // immutable: MCP calls must only execute from the production bundle.
        let _ = self.ports()?;
        let broker_id = super::optional_string(arguments, "brokerId");
        let market = super::optional_string(arguments, "market");
        let query = query_string([("brokerId", broker_id), ("market", market)]);
        let (broker_id, market) = query_pairs(&query);
        jftrade_research::screen_catalog(&broker_id, &market).map_err(screen_catalog_error)
    }

    fn research_screen(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let mut body = arguments.clone();
        let object = body
            .as_object_mut()
            .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
        if let Some(operation) = object.get("operation") {
            if operation.as_str().map(str::trim) != Some("stock_v2") {
                return Err(McpToolFailure::invalid(
                    "operation must be stock_v2 when provided",
                ));
            }
            // The Rust V2 request parser is route-specific and does not carry
            // the compatibility-only operation field.
            object.remove("operation");
        }
        let body = serde_json::to_vec(&body).map_err(|error| {
            McpToolFailure::failed(
                400,
                "BAD_REQUEST",
                format!("encode research screen request: {error}"),
            )
        })?;
        let response = dispatch_research_screen_write(
            &ResearchScreenWriteRequest {
                method: "POST".to_owned(),
                path: RESEARCH_SCREEN_PATH.to_owned(),
                body: Some(body),
            },
            Some(self.ports()?.research_screen_write.as_ref()),
            &super::super::product_production_ports::provider_now_rfc3339(),
        );
        if response.status != 200 {
            return Err(screen_response_error(
                response.status,
                &response.body,
                &response.headers,
            ));
        }
        response
            .body
            .get("data")
            .cloned()
            .filter(|value| !value.is_null())
            .ok_or_else(|| {
                McpToolFailure::failed(
                    502,
                    "MCP_PRODUCTION_PAYLOAD_INVALID",
                    "research screen response is missing data",
                )
            })
    }
}

fn market_research_request(
    name: &str,
    arguments: &Value,
) -> Result<(&'static str, String), McpToolFailure> {
    let path = match name {
        "research.rankings" => "/api/v1/research/rankings",
        "research.industry" => "/api/v1/research/industries",
        "research.calendar" => "/api/v1/research/calendars",
        "research.macro" => "/api/v1/research/macro",
        _ => {
            return Err(McpToolFailure::unavailable(
                "MCP_TOOL_UNAVAILABLE",
                format!("unsupported market research tool {name}"),
            ));
        }
    };
    require_market_research_operation(name, arguments)?;
    let query = arguments_query(arguments, &[], &[])?;
    Ok((path, query))
}

fn research_instrument_route(name: &str) -> Result<&'static str, McpToolFailure> {
    match name {
        "research.instrument" => Ok("instruments"),
        "research.financials" => Ok("financials"),
        "research.analyst" => Ok("analyst"),
        "research.ownership" => Ok("ownership"),
        "research.corporate_actions" => Ok("corporate-actions"),
        "research.valuation" => Ok("valuation"),
        _ => Err(McpToolFailure::invalid(format!(
            "research tool {name} is not instrument-scoped"
        ))),
    }
}

fn technical_indicator_query(arguments: &Value) -> Result<String, McpToolFailure> {
    let object = arguments
        .as_object()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    let mut normalized = object.clone();
    for key in ["kLine", "inputs"] {
        let Some(value) = normalized.get(key) else {
            continue;
        };
        if !value.is_array() {
            return Err(McpToolFailure::invalid(format!("{key} must be an array")));
        }
        let encoded = serde_json::to_string(value).map_err(|error| {
            McpToolFailure::invalid(format!("{key} must be valid JSON: {error}"))
        })?;
        normalized.insert(key.to_owned(), Value::String(encoded));
    }
    arguments_query(
        &Value::Object(normalized),
        &[
            "instrumentId",
            "market",
            "symbol",
            "startTime",
            "endTime",
            "period",
        ],
        &[],
    )
}

fn require_market_research_operation(name: &str, arguments: &Value) -> Result<(), McpToolFailure> {
    let arguments = arguments
        .as_object()
        .ok_or_else(|| McpToolFailure::invalid("tool arguments must be an object"))?;
    match arguments.get("operation") {
        Some(Value::String(operation)) if !operation.trim().is_empty() => Ok(()),
        None | Some(Value::Null | Value::String(_)) => Err(McpToolFailure::failed(
            409,
            "CAPABILITY_UNAVAILABLE",
            format!("{name} requires an explicit operation"),
        )),
        Some(_) => Err(McpToolFailure::invalid("operation must be a string")),
    }
}

fn query_pairs(query: &str) -> (String, String) {
    let mut broker_id = String::new();
    let mut market = String::new();
    for pair in query.split('&').filter(|pair| !pair.is_empty()) {
        let (key, value) = pair.split_once('=').unwrap_or((pair, ""));
        let key = percent_encoding::percent_decode_str(key)
            .decode_utf8()
            .map_or_else(|_| key.to_owned(), |value| value.into_owned());
        let value = percent_encoding::percent_decode_str(value)
            .decode_utf8()
            .map_or_else(|_| value.to_owned(), |value| value.into_owned());
        match key.as_str() {
            "brokerId" if broker_id.is_empty() => broker_id = value,
            "market" if market.is_empty() => market = value,
            _ => {}
        }
    }
    (broker_id, market)
}

fn screen_response_error(
    status: u16,
    body: &Value,
    headers: &std::collections::BTreeMap<String, String>,
) -> McpToolFailure {
    let code = body
        .get("error")
        .and_then(|error| error.get("code"))
        .and_then(Value::as_str)
        .unwrap_or(if status == 400 {
            "BAD_REQUEST"
        } else {
            "RESEARCH_SCREEN_UNAVAILABLE"
        });
    let message = body
        .get("error")
        .and_then(|error| error.get("message"))
        .and_then(Value::as_str)
        .unwrap_or("research screen request failed");
    let retry_after_seconds = headers
        .get("Retry-After")
        .and_then(|value| value.parse::<u64>().ok());
    McpToolFailure {
        status,
        code: code.to_owned(),
        message: message.to_owned(),
        retry_after_seconds,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn instrument_research_routes_match_product_paths() {
        assert_eq!(
            research_instrument_route("research.instrument").unwrap(),
            "instruments"
        );
        assert_eq!(
            research_instrument_route("research.corporate_actions").unwrap(),
            "corporate-actions"
        );
    }

    #[test]
    fn market_research_requires_explicit_operation() {
        for name in [
            "research.rankings",
            "research.industry",
            "research.calendar",
            "research.macro",
        ] {
            let failure = require_market_research_operation(name, &json!({})).unwrap_err();
            assert_eq!(failure.status, 409, "{name}");
            assert_eq!(failure.code, "CAPABILITY_UNAVAILABLE", "{name}");
        }
        let explicit = json!({"operation": "hot"});
        assert!(require_market_research_operation("research.rankings", &explicit).is_ok());

        let (path, query) = market_research_request(
            "research.macro",
            &json!({"operation": "indicator_history", "indicatorId": "cpi_yoy"}),
        )
        .unwrap();
        assert_eq!(path, "/api/v1/research/macro");
        let query = crate::product::product_query::QueryMap::parse(&query).unwrap();
        assert_eq!(query.get_first("operation"), Some("indicator_history"));
        assert_eq!(query.get_first("indicatorId"), Some("cpi_yoy"));
    }

    #[test]
    fn screen_errors_preserve_retry_after_and_wire_code() {
        let failure = screen_response_error(
            503,
            &json!({"error": {"code": "MARKET_DATA_PROVIDER_BUSY", "message": "busy"}}),
            &std::collections::BTreeMap::from([(String::from("Retry-After"), String::from("2"))]),
        );
        assert_eq!(failure.status, 503);
        assert_eq!(failure.code, "MARKET_DATA_PROVIDER_BUSY");
        assert_eq!(failure.retry_after_seconds, Some(2));
    }
}
