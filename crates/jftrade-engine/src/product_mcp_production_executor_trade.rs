//! MCP broker/execution readers backed by the production trade ports.

use serde_json::Value;

use super::helpers::{optional_string_array, query_string};
use super::{
    McpToolFailure, ProductionMcpToolExecutor, broker_error, execution_error, execution_write_error,
};
use crate::product::product_execution_write_port::{ExecutionWriteInput, ExecutionWriteOperation};

impl ProductionMcpToolExecutor {
    pub(super) fn broker_cash_flows(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let clearing_date = super::optional_string(arguments, "clearingDate")
            .ok_or_else(|| McpToolFailure::invalid("clearingDate is required"))?;
        let scope = super::optional_string(arguments, "scope")
            .unwrap_or_else(|| "CURRENT".to_owned())
            .to_ascii_uppercase();
        if !matches!(scope.as_str(), "CURRENT" | "HISTORY") {
            return Err(McpToolFailure::invalid("scope must be CURRENT or HISTORY"));
        }
        let query = broker_query_with_fields(
            arguments,
            scope,
            [
                ("clearingDate", Some(clearing_date)),
                ("direction", super::optional_string(arguments, "direction")),
            ],
        );
        self.ports()?
            .broker
            .read("/api/v1/brokers/futu/cash-flows", &query)
            .map_err(broker_error)
    }

    pub(super) fn broker_fees(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let mut order_ids = optional_string_array(arguments, "orderIdEx")?.unwrap_or_default();
        order_ids.extend(optional_string_array(arguments, "orderIdExList")?.unwrap_or_default());
        if order_ids.is_empty() {
            return Err(McpToolFailure::invalid(
                "query parameter orderIdEx is required",
            ));
        }
        let query = broker_query_with_fields(
            arguments,
            "CURRENT".to_owned(),
            [("orderIdEx", Some(order_ids.join(",")))],
        );
        self.ports()?
            .broker
            .read("/api/v1/brokers/futu/order-fees", &query)
            .map_err(broker_error)
    }

    pub(super) fn broker_margin_ratios(&self, arguments: &Value) -> Result<Value, McpToolFailure> {
        let mut symbols = optional_string_array(arguments, "symbols")?.unwrap_or_default();
        if let Some(symbol) = super::optional_string(arguments, "symbol") {
            symbols.push(symbol);
        }
        if symbols.is_empty() {
            return Err(McpToolFailure::invalid("at least one symbol is required"));
        }
        let query = broker_query_with_fields(
            arguments,
            "CURRENT".to_owned(),
            [("symbols", Some(symbols.join(",")))],
        );
        self.ports()?
            .broker
            .read("/api/v1/brokers/futu/margin-ratios", &query)
            .map_err(broker_error)
    }

    pub(super) fn execution_order_events(
        &self,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        let Some(order_id) = super::optional_string(arguments, "internalOrderId") else {
            let query = query_string([
                ("scope", Some("CURRENT".to_owned())),
                ("brokerId", super::optional_string(arguments, "brokerId")),
                (
                    "tradingEnvironment",
                    super::optional_string(arguments, "tradingEnvironment"),
                ),
                ("accountId", super::optional_string(arguments, "accountId")),
                ("market", super::optional_string(arguments, "market")),
            ]);
            return self
                .ports()?
                .execution_read
                .read("/api/v1/execution/orders", &query)
                .map_err(execution_error);
        };
        self.ports()?
            .execution_read
            .read(
                &format!(
                    "/api/v1/execution/orders/{}/events",
                    super::helpers::path_segment(&order_id)
                ),
                "",
            )
            .map_err(execution_error)
    }

    pub(super) fn execution_buying_power(
        &self,
        arguments: &Value,
    ) -> Result<Value, McpToolFailure> {
        for key in ["accountId", "tradingEnvironment", "market", "orderKind"] {
            super::required_string(arguments, key)?;
        }
        if arguments.get("instrument").is_none() {
            return Err(McpToolFailure::invalid("instrument is required"));
        }
        let input = ExecutionWriteInput {
            operation: ExecutionWriteOperation::BuyingPower,
            internal_order_id: None,
            payload: arguments.clone(),
            context: Default::default(),
        };
        self.ports()?
            .execution_write
            .mutate(&input)
            .map_err(execution_write_error)
    }
}

fn broker_query_with_fields<const N: usize>(
    arguments: &Value,
    scope: String,
    extra: [(&str, Option<String>); N],
) -> String {
    let mut fields = vec![
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
    ];
    fields.extend(extra);
    fields
        .into_iter()
        .filter_map(|(key, value)| {
            value
                .filter(|value| !value.trim().is_empty())
                .map(|value| (key, value))
        })
        .map(|(key, value)| format!("{key}={}", percent_encode(&value)))
        .collect::<Vec<_>>()
        .join("&")
}

fn percent_encode(value: &str) -> String {
    percent_encoding::utf8_percent_encode(value, percent_encoding::NON_ALPHANUMERIC).to_string()
}
