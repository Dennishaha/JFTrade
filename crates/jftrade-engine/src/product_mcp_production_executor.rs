//! Production MCP tool adapter.
//!
//! The MCP listener deliberately knows nothing about HTTP route handlers or
//! SQLite stores.  This adapter translates reviewed MCP calls into the same
//! production snapshot ports used by the Rust HTTP API.  A missing provider,
//! broker session, helper, or store is surfaced as a structured failure; no
//! tool returns a fixture value merely to make the protocol call succeed.

use std::sync::Arc;

use serde_json::{Value, json};

use super::product_mcp_protocol::{
    model_search_text, optional_bool, optional_integer, optional_string, provider_model,
};
use super::product_production_ports::{ProductionPortBundle, ProductionToolCatalog};
use crate::product::{BacktestResultViewError, BacktestResultViewRequest};
use super::strategy_pine_mcp::{PINE_SPEC_TOOL, VALIDATE_PINE_TOOL, dispatch_strategy_pine_mcp};
use jftrade_store_sqlite::AdkStore;

#[path = "product_mcp_production_executor_derivatives.rs"]
mod derivatives;
#[path = "product_mcp_production_executor_errors.rs"]
mod errors;
#[path = "product_mcp_production_executor_helpers.rs"]
mod helpers;
#[path = "product_mcp_production_executor_market_data.rs"]
mod market_data;
#[path = "product_mcp_production_executor_pine.rs"]
mod pine;
#[path = "product_mcp_production_executor_prediction.rs"]
mod prediction;
#[path = "product_mcp_production_executor_research.rs"]
mod research;
#[path = "product_mcp_production_executor_trade.rs"]
mod trade;
use errors::*;
pub(super) use errors::{provider_actions_error, quote_error};
use helpers::*;
use pine::*;

/// Error envelope carried in `tools/call` structured content.  It mirrors the
/// product HTTP envelope while retaining the status for MCP clients that do
/// not have access to an HTTP response status.
#[derive(Clone, Debug)]
pub(crate) struct McpToolFailure {
    pub(crate) status: u16,
    pub(crate) code: String,
    pub(crate) message: String,
    pub(crate) retry_after_seconds: Option<u64>,
}

impl McpToolFailure {
    fn invalid(message: impl Into<String>) -> Self {
        Self {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: message.into(),
            retry_after_seconds: None,
        }
    }

    fn unavailable(code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            status: 503,
            code: code.into(),
            message: message.into(),
            retry_after_seconds: None,
        }
    }

    fn failed(status: u16, code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            status,
            code: code.into(),
            message: message.into(),
            retry_after_seconds: None,
        }
    }

    pub(crate) fn envelope(&self) -> Value {
        let mut value = json!({
            "ok": false,
            "error": {"code": self.code, "message": self.message},
            "status": self.status,
        });
        if let Some(seconds) = self.retry_after_seconds {
            value["retryAfterSeconds"] = json!(seconds);
        }
        value
    }
}

/// Concrete production executor.  `catalog` and `store` remain on the
/// executor for the existing discovery tools; `ports` is installed only by
/// the production composition root and is the sole source for tool values.
pub(crate) struct ProductionMcpToolExecutor {
    pub(crate) catalog: Arc<ProductionToolCatalog>,
    pub(crate) store: Arc<AdkStore>,
    ports: Option<Arc<ProductionPortBundle>>,
}

impl std::fmt::Debug for ProductionMcpToolExecutor {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionMcpToolExecutor")
            .field("catalog", &true)
            .field("store", &true)
            .field("ports", &self.ports.is_some())
            .finish()
    }
}

impl ProductionMcpToolExecutor {
    #[allow(dead_code)]
    pub(crate) fn new(catalog: Arc<ProductionToolCatalog>, store: Arc<AdkStore>) -> Self {
        Self {
            catalog,
            store,
            ports: None,
        }
    }

    pub(crate) fn from_production_ports(ports: Arc<ProductionPortBundle>) -> Self {
        Self {
            catalog: Arc::clone(&ports.mcp_catalog),
            store: Arc::clone(&ports.mcp_store),
            ports: Some(ports),
        }
    }

    fn ports(&self) -> Result<&Arc<ProductionPortBundle>, McpToolFailure> {
        self.ports.as_ref().ok_or_else(|| {
            McpToolFailure::unavailable(
                "MCP_PRODUCTION_EXECUTOR_UNAVAILABLE",
                "production MCP ports are not configured",
            )
        })
    }

    fn system_read(&self, path: &str) -> Result<Value, McpToolFailure> {
        self.ports()?.system_read.read(path).map_err(system_error)
    }

    fn provider_read(&self, path: &str, query: &str) -> Result<Value, McpToolFailure> {
        self.ports()?
            .provider
            .read(path, query)
            .map_err(provider_error)
    }

    fn strategy_pine_mcp(&self, name: &str, arguments: &Value) -> Result<Value, McpToolFailure> {
        self.strategy_pine_mcp_with_mode(name, arguments, pine_external_mode())
    }

    pub(crate) fn strategy_pine_mcp_with_mode(
        &self,
        name: &str,
        arguments: &Value,
        mode: &str,
    ) -> Result<Value, McpToolFailure> {
        self.strategy_pine_mcp_with_mode_and_notice(
            name,
            arguments,
            mode,
            third_party_notice_available(),
        )
    }

    pub(crate) fn strategy_pine_mcp_with_mode_and_notice(
        &self,
        name: &str,
        arguments: &Value,
        mode: &str,
        notice_available: bool,
    ) -> Result<Value, McpToolFailure> {
        let mut payload = dispatch_strategy_pine_mcp(name, arguments)
            .map_err(|error| McpToolFailure::failed(error.status, error.code, error.message))?;
        // Keep specification discovery deterministic and local.  Validation
        // may optionally enrich its external-engine projection when the
        // caller explicitly enables a PineTS shadow mode.
        if name == VALIDATE_PINE_TOOL {
            let Some(script) = arguments
                .get("script")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|script| !script.is_empty())
            else {
                return Ok(payload);
            };
            if mode != PINE_MODE_OFF {
                payload["externalEngine"] = pine_external_engine_payload(
                    self.ports.as_ref(),
                    mode,
                    script,
                    notice_available,
                )
                .unwrap_or_else(|error| pine_shadow_error_payload(mode, error));
            }
        }
        Ok(payload)
    }

    fn market_capabilities(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let query = query_string([
            ("brokerId", optional_string(arguments, "brokerId")),
            ("accountId", optional_string(arguments, "accountId")),
            (
                "tradingEnvironment",
                optional_string(arguments, "tradingEnvironment"),
            ),
            ("market", optional_string(arguments, "market")),
            ("featureId", optional_string(arguments, "featureId")),
        ]);
        // The production catalog binds market.capabilities to the shared
        // provider projection.  It contains the active provider, readiness,
        // and subscription capability state without manufacturing a broker
        // descriptor when no broker runtime is connected.
        self.provider_read("/api/v1/market-data/provider", &query)
    }

    fn market_search(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let query_text = required_string(arguments, "query")?;
        let limit = bounded_integer(arguments, "limit", 20, 1, 100)?;
        let query = query_string([
            ("query", Some(query_text)),
            ("limit", Some(limit.to_string())),
            ("market", optional_string(arguments, "market")),
        ]);
        let port = Arc::clone(&self.ports()?.catalog);
        run_catalog_read(port, "/api/v1/market-data/instruments", query).map_err(catalog_error)
    }

    fn market_snapshot(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let (market, symbol) = instrument(arguments)?;
        let path = format!(
            "/api/v1/market-data/snapshots/{}/{}",
            path_segment(&market),
            path_segment(&symbol)
        );
        let query = query_string([(
            "refresh",
            optional_bool_strict(arguments, "refresh", false)?.then(|| "true".to_owned()),
        )]);
        let port = Arc::clone(&self.ports()?.market_data_quote);
        run_quote_read(port, path, query).map_err(quote_error)
    }

    fn watchlist_list(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let include_quotes = optional_bool_strict(arguments, "includeQuotes", false)?;
        if include_quotes {
            return Err(McpToolFailure::unavailable(
                "WATCHLIST_QUOTES_UNAVAILABLE",
                "production watchlist snapshots do not provide quote enrichment",
            ));
        }
        let group = optional_string(arguments, "group")
            .or_else(|| optional_string(arguments, "groupName"))
            .or_else(|| optional_string(arguments, "groupId"));
        let market = optional_string(arguments, "market");
        let search = optional_string(arguments, "query");
        let cursor = optional_string(arguments, "cursor");
        let limit = bounded_integer(arguments, "limit", 50, 1, 200)?;
        let has_item_filter =
            group.is_some() || market.is_some() || search.is_some() || cursor.is_some();
        let path = if has_item_filter {
            "/api/v1/watchlist/items"
        } else {
            "/api/v1/watchlist/groups"
        };
        let query = query_string([
            ("groupId", group),
            ("market", market),
            ("query", search),
            ("cursor", cursor),
            ("limit", Some(limit.to_string())),
        ]);
        self.ports()?
            .watchlist
            .read(path, &query)
            .map_err(watchlist_error)
    }

    fn plugins_catalog(&self) -> Result<Value, McpToolFailure> {
        self.ports()?.plugins.catalog().map_err(plugin_error)
    }

    fn remote_watchlist_list(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let operation = optional_string(arguments, "operation")
            .unwrap_or_else(|| "groups".to_owned())
            .to_ascii_lowercase();
        if !matches!(operation.as_str(), "groups" | "members") {
            return Err(McpToolFailure::invalid(
                "operation must be groups or members",
            ));
        }
        let remote_group_id = optional_string(arguments, "remoteGroupId")
            .or_else(|| optional_string(arguments, "groupId"));
        if operation == "members" && remote_group_id.is_none() {
            return Err(McpToolFailure::invalid(
                "remoteGroupId is required for members operation",
            ));
        }
        let query = query_string([
            ("operation", Some(operation)),
            ("remoteGroupId", remote_group_id),
        ]);
        self.ports()?
            .remote_watchlist
            .read(&query)
            .map_err(remote_watchlist_error)
    }

    fn strategy_definitions(&self) -> Result<Value, McpToolFailure> {
        let definitions = self
            .ports()?
            .strategy_definition
            .list()
            .map_err(strategy_definition_error)?;
        let definition_count = definitions.len();
        Ok(json!({
            "definitions": definitions,
            "definitionCount": definition_count,
        }))
    }

    fn strategy_definition_versions(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let definition_id = required_string(arguments, "definitionId")?;
        let versions = self
            .ports()?
            .strategy_definition
            .versions(&definition_id)
            .map_err(strategy_definition_error)?
            .ok_or_else(|| {
                McpToolFailure::failed(
                    404,
                    "STRATEGY_DEFINITION_NOT_FOUND",
                    "strategy definition was not found",
                )
            })?;
        Ok(json!({
            "definitionId": definition_id,
            "versions": versions,
            "versionCount": versions.len(),
        }))
    }

    fn strategy_definition_version(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let definition_id = required_string(arguments, "definitionId")?;
        let version = required_string(arguments, "version")?;
        self.ports()?
            .strategy_definition
            .version(&definition_id, &version)
            .map_err(strategy_definition_error)?
            .ok_or_else(|| {
                McpToolFailure::failed(
                    404,
                    "STRATEGY_DEFINITION_VERSION_NOT_FOUND",
                    "strategy definition version was not found",
                )
            })
    }

    fn strategy_instance_activity(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let instance_id = required_string(arguments, "instanceId")?;
        let kind = optional_string(arguments, "kind")
            .unwrap_or_else(|| "logs".to_owned())
            .to_ascii_lowercase();
        if !matches!(kind.as_str(), "logs" | "audit") {
            return Err(McpToolFailure::invalid("kind must be logs or audit"));
        }
        let limit = bounded_integer(arguments, "limit", 50, 1, 200)?;
        let offset = bounded_integer(arguments, "offset", 0, 0, 5_000_000)?;
        let query = query_string([
            ("limit", Some(limit.to_string())),
            ("offset", Some(offset.to_string())),
            (
                "level",
                (kind == "logs")
                    .then(|| optional_string(arguments, "level"))
                    .flatten(),
            ),
            (
                "kind",
                (kind == "audit")
                    .then(|| optional_string(arguments, "eventKind"))
                    .flatten(),
            ),
            ("fromTime", optional_string(arguments, "fromTime")),
            ("toTime", optional_string(arguments, "toTime")),
        ]);
        self.ports()?
            .strategy_read
            .read(&format!("/api/v1/strategies/{instance_id}/{kind}"), &query)
            .map_err(strategy_read_error)?
            .ok_or_else(|| {
                McpToolFailure::failed(
                    404,
                    "STRATEGY_INSTANCE_NOT_FOUND",
                    "strategy instance was not found",
                )
            })
    }

    fn portfolio_summary(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let broker_id = optional_string(arguments, "brokerId").unwrap_or_else(|| "futu".to_owned());
        if !broker_id.eq_ignore_ascii_case("futu") {
            return Err(McpToolFailure::invalid("brokerId must be futu"));
        }
        let query = query_string([
            ("accountId", optional_string(arguments, "accountId")),
            (
                "tradingEnvironment",
                optional_string(arguments, "tradingEnvironment"),
            ),
            ("market", optional_string(arguments, "market")),
        ]);
        let ports = self.ports()?;
        let positions = ports
            .portfolio
            .read("/api/v1/portfolio/futu/positions", &query)
            .map_err(portfolio_error)?;
        let balances = ports
            .portfolio
            .read("/api/v1/portfolio/futu/cash-balances", &query)
            .map_err(portfolio_error)?;
        let orders = ports
            .broker
            .read("/api/v1/brokers/futu/orders", &query)
            .map_err(broker_error)?;
        let positions_payload = positions;
        let balances_payload = balances;
        let orders_payload = orders;
        let positions = required_field(&positions_payload, "positions", "array")?;
        let balances = required_field(&balances_payload, "balances", "array")?;
        let orders = required_field(&orders_payload, "orders", "array")?;
        let observed = [&positions_payload, &balances_payload, &orders_payload];
        let connectivity = same_observed_string(&observed, "connectivity")?;
        let checked_at = first_observed_string(&observed, "checkedAt")?;
        let mut result = serde_json::Map::new();
        result.insert("brokerId".to_owned(), Value::String("futu".to_owned()));
        result.insert("positions".to_owned(), positions);
        result.insert("balances".to_owned(), balances);
        result.insert("orders".to_owned(), orders);
        if let Some(connectivity) = connectivity {
            result.insert("connectivity".to_owned(), connectivity);
        }
        if let Some(checked_at) = checked_at {
            result.insert("checkedAt".to_owned(), checked_at);
        }
        Ok(Value::Object(result))
    }

    fn backtest_runs(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let payload = self.ports()?.backtest_read.list().map_err(backtest_error)?;
        let Some(runs) = nullable_runs(&payload)? else {
            return Ok(payload);
        };
        let definition_id = optional_string(arguments, "definitionId");
        let definition_version = optional_string(arguments, "definitionVersion");
        let status = optional_string(arguments, "status");
        let provider = optional_string(arguments, "marketDataProvider");
        let limit = bounded_integer(arguments, "limit", 0, 0, 200)?;
        if definition_id.is_none()
            && definition_version.is_none()
            && status.is_none()
            && provider.is_none()
            && limit <= 0
        {
            return Ok(payload);
        }
        if limit < 0 {
            return Err(McpToolFailure::invalid("limit must be non-negative"));
        }
        let mut filtered = runs
            .iter()
            .filter(|run| {
                matches_filter(run, "definitionId", definition_id.as_deref(), false)
                    && matches_filter(
                        run,
                        "definitionVersion",
                        definition_version.as_deref(),
                        false,
                    )
                    && matches_filter(run, "status", status.as_deref(), true)
                    && matches_filter(run, "marketDataProvider", provider.as_deref(), true)
            })
            .cloned()
            .collect::<Vec<_>>();
        let total_matched = filtered.len();
        if limit > 0 {
            filtered.truncate((limit as usize).min(200));
        }
        Ok(json!({
            "runs": filtered,
            "runCount": filtered.len(),
            "totalMatched": total_matched,
            "truncated": filtered.len() < total_matched,
        }))
    }

    fn backtest_kline_sync_status(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let task_id = required_string(arguments, "taskId")?;
        let _wait_ms = bounded_integer(arguments, "waitForCompletionMs", 0, 0, 25_000)?;
        let progress = self
            .ports()?
            .backtest_sync
            .progress(&task_id)
            .map_err(backtest_sync_error)?
            .ok_or_else(|| {
                McpToolFailure::failed(
                    404,
                    "BACKTEST_SYNC_TASK_NOT_FOUND",
                    "k-line sync task was not found",
                )
            })?;
        Ok(add_retry_hint(progress))
    }

    fn backtest_result_view(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let run_id = required_string(arguments, "runId")?;
        let view_req = BacktestResultViewRequest {
            run_id,
            view: optional_string(arguments, "view"),
            include: arguments
                .get("include")
                .and_then(Value::as_array)
                .map(|arr| arr.iter().filter_map(Value::as_str).map(str::to_owned).collect()),
            start_time: optional_string(arguments, "startTime"),
            end_time: optional_string(arguments, "endTime"),
            cursor: optional_string(arguments, "cursor"),
            limit: arguments.get("limit").and_then(Value::as_u64).map(|v| v as usize),
            resolution: optional_string(arguments, "resolution"),
        };
        match self.ports()?.backtest_read.result_view(&view_req) {
            Ok(Some(snapshot)) => Ok(snapshot.data),
            Ok(None) => Err(McpToolFailure::failed(
                404,
                "BACKTEST_NOT_FOUND",
                "backtest run was not found",
            )),
            Err(BacktestResultViewError::Invalid(msg)) => Err(McpToolFailure::invalid(msg)),
            Err(BacktestResultViewError::NotFound(msg)) => {
                Err(McpToolFailure::failed(404, "BACKTEST_NOT_FOUND", msg))
            }
            Err(BacktestResultViewError::Unavailable(msg)) => Err(McpToolFailure::failed(
                503,
                "BACKTEST_RESULT_UNAVAILABLE",
                msg,
            )),
            Err(BacktestResultViewError::Failed(msg)) => {
                Err(McpToolFailure::failed(500, "BACKTEST_RESULT_FAILED", msg))
            }
        }
    }

    fn broker_read(&self, resource: &str, arguments: &Value) -> Result<Value, McpToolFailure> {
        let scope = optional_string(arguments, "scope").unwrap_or_else(|| "CURRENT".to_owned());
        let scope = scope.to_ascii_uppercase();
        if !matches!(scope.as_str(), "CURRENT" | "HISTORY") {
            return Err(McpToolFailure::invalid("scope must be CURRENT or HISTORY"));
        }
        let query = broker_query(arguments, scope);
        self.ports()?
            .broker
            .read(&format!("/api/v1/brokers/futu/{resource}"), &query)
            .map_err(broker_error)
    }

    fn account_orders(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let trading_environment = required_string(arguments, "tradingEnvironment")?;
        let active_only = optional_bool_strict(arguments, "activeOnly", false)?;
        let query = query_string([
            (
                "scope",
                Some(if active_only { "ACTIVE" } else { "CURRENT" }.to_owned()),
            ),
            ("tradingEnvironment", Some(trading_environment)),
            ("accountId", optional_string(arguments, "accountId")),
            ("market", optional_string(arguments, "market")),
        ]);
        let payload = self
            .ports()?
            .execution_read
            .read("/api/v1/execution/orders", &query)
            .map_err(execution_error)?;
        let orders = required_field(&payload, "orders", "array")?;
        let count = orders.as_array().map_or(0, Vec::len);
        let mut result = payload.as_object().cloned().ok_or_else(|| {
            McpToolFailure::failed(
                502,
                "MCP_PRODUCTION_PAYLOAD_INVALID",
                "execution adapter returned a non-object payload",
            )
        })?;
        result.insert("orders".to_owned(), orders);
        result.insert("count".to_owned(), json!(count));
        result.insert("activeOnly".to_owned(), Value::Bool(active_only));
        Ok(Value::Object(result))
    }

    fn risk_state(&self) -> Result<Value, McpToolFailure> {
        let ports = self.ports()?;
        let kill_switch = ports
            .system_read
            .read("/api/v1/system/real-trade-kill-switch")
            .map_err(system_error)?;
        let risk_limits = ports
            .system_read
            .read("/api/v1/system/real-trade-risk-limits")
            .map_err(system_error)?;
        let mut result = serde_json::Map::new();
        result.insert("killSwitch".to_owned(), kill_switch);
        result.insert("riskLimits".to_owned(), risk_limits);
        Ok(Value::Object(result))
    }

    fn risk_events(&self) -> Result<Value, McpToolFailure> {
        self.ports()?
            .system_read
            .read("/api/v1/system/real-trade-risk-events")
            .map_err(system_error)
    }

    #[allow(dead_code)]
    fn _model_list(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let query = optional_string(arguments, "query")
            .unwrap_or_default()
            .to_ascii_lowercase();
        let provider_id = optional_string(arguments, "providerId").unwrap_or_default();
        let callable_only = optional_bool(arguments, "callableOnly", true);
        let limit = optional_integer(arguments, "limit", 50).clamp(1, 100) as usize;
        let models = self
            .store
            .list_providers()
            .map_err(|error| {
                McpToolFailure::failed(500, "ADK_PROVIDER_READ_FAILED", error.to_string())
            })?
            .into_iter()
            .map(provider_model)
            .collect::<Result<Vec<_>, _>>()
            .map_err(|message| McpToolFailure::failed(500, "ADK_PROVIDER_READ_FAILED", message))?
            .into_iter()
            .filter(|model| {
                let callable = model
                    .get("callable")
                    .and_then(Value::as_bool)
                    .unwrap_or(false);
                let provider_matches = provider_id.is_empty()
                    || model.get("providerId").and_then(Value::as_str)
                        == Some(provider_id.as_str());
                provider_matches
                    && (!callable_only || callable)
                    && (query.is_empty() || model_search_text(model).contains(&query))
            })
            .take(limit)
            .collect::<Vec<_>>();
        Ok(
            json!({"query": query, "providerId": provider_id, "callableOnly": callable_only, "models": models, "totalReturned": models.len()}),
        )
    }
}

include!("product_mcp_production_dispatch.rs");

#[cfg(test)]
mod tests {
    use super::*;
    use crate::product::MarketDataProviderReadSnapshotError;

    #[test]
    fn production_failure_projects_product_error_envelope() {
        let failure = McpToolFailure::failed(502, "MARKET_SNAPSHOT_FAILED", "upstream refused");
        assert_eq!(
            failure.envelope(),
            json!({
                "ok": false,
                "error": {"code": "MARKET_SNAPSHOT_FAILED", "message": "upstream refused"},
                "status": 502,
            })
        );
    }

    #[test]
    fn production_argument_validation_rejects_missing_or_out_of_range_values() {
        assert_eq!(
            required_string(&json!({}), "query")
                .expect_err("missing query")
                .status,
            400
        );
        assert_eq!(
            bounded_integer(&json!({"limit": 0}), "limit", 20, 1, 100)
                .expect_err("zero limit")
                .code,
            "BAD_REQUEST"
        );
        assert!(instrument(&json!({"instrumentId": "US"})).is_err());
    }

    #[test]
    fn production_snapshot_failures_and_malformed_payloads_fail_closed() {
        assert_eq!(
            provider_error(MarketDataProviderReadSnapshotError::Unavailable(
                "provider is offline".to_owned(),
            ))
            .status,
            503
        );
        assert_eq!(
            provider_error(MarketDataProviderReadSnapshotError::Failed {
                code: "UPSTREAM_REFUSED".to_owned(),
                message: "provider refused request".to_owned(),
            })
            .status,
            502
        );
        assert!(nullable_runs(&json!({"runs": null})).unwrap().is_none());
        let malformed = nullable_runs(&json!({"runs": {}})).expect_err("malformed runs");
        assert_eq!(malformed.status, 502);
        assert_eq!(malformed.code, "MCP_PRODUCTION_PAYLOAD_INVALID");
        let missing = nullable_runs(&json!({})).expect_err("missing runs");
        assert_eq!(missing.status, 502);
    }

    #[test]
    fn pine_external_mode_parser_accepts_only_supported_values() {
        assert_eq!(pine_external_mode_value(None), PINE_MODE_OFF);
        assert_eq!(pine_external_mode_value(Some(" off ")), PINE_MODE_OFF);
        assert_eq!(pine_external_mode_value(Some(" SHADOW ")), PINE_MODE_SHADOW);
        assert_eq!(
            pine_external_mode_value(Some("community-agpl")),
            PINE_MODE_COMMUNITY_AGPL
        );
        assert_eq!(pine_external_mode_value(Some("unknown")), PINE_MODE_OFF);
    }
}
