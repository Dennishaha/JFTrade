//! Read-only Futu trade projections for production broker and portfolio APIs.
//!
//! The adapter deliberately consumes the engine-neutral `TradeReadPort`; no
//! generated OpenD protobuf type crosses this module boundary.  Execution
//! writes and the durable execution ledger remain owned by the local store.

use std::sync::Arc;
#[allow(unused_imports)]
use std::time::{SystemTime, UNIX_EPOCH};

#[allow(unused_imports)]
use jftrade_integration_futu::{
    TradeFilter, TradeFunds, TradeHeader, TradeMaxTradeQuantityRequest, TradeReadPort,
    TradeSecurity, trade_header,
};
use serde_json::{Value, json};
#[allow(unused_imports)]
use time::OffsetDateTime;
#[allow(unused_imports)]
use time::format_description::well_known::Rfc3339;
#[allow(unused_imports)]
use time::format_description::{FormatItem, parse_borrowed};

use super::ActiveProviderState;
#[allow(unused_imports)]
use crate::product::product_query::QueryMap;
use crate::product::{
    BrokerReadSnapshotError, BrokerReadSnapshotPort, PortfolioSnapshotError, PortfolioSnapshotPort,
};

#[path = "product_trade_margin_cache.rs"]
mod product_trade_margin_cache;
#[path = "product_trade_margin_route.rs"]
mod product_trade_margin_route;
#[path = "product_trade_runtime_projection.rs"]
mod product_trade_runtime_projection;
pub(crate) use product_trade_runtime_projection::SharedTradeReadRuntime;
#[path = "product_production_ports_trade_requests.rs"]
mod product_production_ports_trade_requests;
#[path = "product_trade_runtime_options.rs"]
mod product_trade_runtime_options;
#[path = "product_broker_capabilities_projection.rs"]
mod product_broker_capabilities_projection;
pub(crate) use product_production_ports_trade_requests::market_code;
pub(crate) use product_production_ports_trade_requests::{
    ResolvedTradeRequest, TradeRequest, account_identity, checked_at, environment_label_from_code,
    normalize_history_time, qot_market_label, quote_market_code,
};
#[path = "trade_projection.rs"]
pub(crate) mod trade_projection;
#[allow(unused_imports)]
use trade_projection::{
    account_value, canonical_time, cash_flow_direction_label, cash_flow_value, currency_label,
    fill_status_label, fill_value, funds_value, map_broker_header_error,
    map_portfolio_header_error, margin_ratio_value, market_label_from_code,
    max_trade_order_type_label, max_trade_quantity_value, non_empty, order_fee_value,
    order_status_label, order_type_label, order_value, position_value, qualify_symbol,
    security_firm_label, session_error, session_label, simulated_account_type_label,
    time_in_force_label, trade_market_authority, trade_side, unavailable, unavailable_portfolio,
};

#[derive(Clone)]
pub(crate) struct ProductionBrokerPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_read_port: Option<Arc<dyn TradeReadPort>>,
    pub(crate) trade_logged_in: Option<bool>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

/// Trade connectivity is owned by the broker/OpenD session, not by the
/// currently selected market-data provider.  A helper-backed market-data
/// provider (yfinance or AKShare) can therefore coexist with a ready Futu
/// trade session.  Keep the runtime snapshot authoritative when it exists;
/// the startup-only fields are retained solely for the embedding path that
/// predates `SharedTradeReadRuntime`.
fn trade_session_ready(
    trade_runtime: Option<&Arc<SharedTradeReadRuntime>>,
    trade_read_port: Option<&Arc<dyn TradeReadPort>>,
    trade_logged_in: Option<bool>,
) -> bool {
    trade_runtime.map_or_else(
        || trade_read_port.is_some() && trade_logged_in == Some(true),
        |runtime| runtime.snapshot().is_ready(),
    )
}

impl std::fmt::Debug for ProductionBrokerPort {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProductionBrokerPort")
            .field("trade_read_port", &self.trade_read_port.is_some())
            .field("trade_logged_in", &self.trade_logged_in)
            .finish()
    }
}

impl BrokerReadSnapshotPort for ProductionBrokerPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, BrokerReadSnapshotError> {
        if path == "/api/v1/brokers/capabilities" {
            self.ensure_ready()?;
            let runtime = self
                .trade_runtime
                .as_ref()
                .ok_or_else(|| unavailable("Futu market-data reader is unavailable"))?;
            if !runtime.market_data_reader_available() {
                return Err(unavailable("Futu market-data reader is unavailable"));
            }
            let provider = self.active_provider_state.snapshot();
            return product_broker_capabilities_projection::project(runtime, &provider, query)
                .map_err(unavailable);
        }
        let request = TradeRequest::parse(path, query).map_err(BrokerReadSnapshotError::Invalid)?;
        self.ensure_ready()?;
        let runtime_snapshot = self.trade_runtime.as_ref().map(|r| r.snapshot());
        let client = runtime_snapshot
            .as_ref()
            .and_then(|s| s.client.as_ref())
            .or_else(|| {
                self.trade_runtime
                    .is_none()
                    .then_some(self.trade_read_port.as_ref())
                    .flatten()
            })
            .ok_or_else(|| unavailable("Futu trade read client is unavailable"))?;
        match request.resource.as_str() {
            "runtime" => {
                let runtime = self
                    .trade_runtime
                    .as_ref()
                    .ok_or_else(|| unavailable("Futu trade runtime projection is unavailable"))?;
                let connection = runtime
                    .connection_snapshot()
                    .ok_or_else(|| unavailable("Futu OpenD connection settings are unavailable"))?;
                let live_clients = runtime
                    .live_clients_snapshot()
                    .ok_or_else(|| unavailable("live websocket client metrics are unavailable"))?;
                let accounts = client.read_accounts(0, None, None).map_err(session_error)?;
                let accounts_discovered = accounts.len();
                let descriptor =
                    serde_json::to_value(jftrade_integration_futu::broker_descriptor())
                        .map_err(|error| unavailable(error.to_string()))?;
                Ok(json!({
                    "accounts": accounts.into_iter().map(account_value).collect::<Vec<_>>(),
                    "descriptor": descriptor,
                    "session": {"brokerId": request.broker_id, "displayName": "Futu", "accountsDiscovered": accounts_discovered, "tradeLoggedIn": runtime.snapshot().trade_logged_in == Some(true), "connectivity": "connected", "checkedAt": checked_at(), "connection": {"host": connection.host, "apiPort": connection.api_port, "websocketPort": connection.websocket_port, "port": connection.api_port, "useEncryption": connection.use_encryption, "marketDataTransport": "bbgo-opend-tcp-api"}, "globalState": null, "lastError": null, "liveWebSocketClients": {"connected": live_clients.0, "limit": live_clients.1, "atLimit": live_clients.0 >= live_clients.1}}
                }))
            }
            "securities" => self.read_securities_route(&request),
            "quote" => self.read_quote_route(&request),
            "klines" => self.read_klines_route(&request),
            "funds" => {
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let funds = client
                    .read_funds(resolved.header.clone(), request.refresh_cache(), None, None)
                    .map_err(session_error)?;
                Ok(funds_value(&resolved, funds))
            }
            "cash-flows" => {
                let clearing_date = request.clearing_date().ok_or_else(|| {
                    BrokerReadSnapshotError::Invalid(
                        "query parameter clearingDate is required".to_owned(),
                    )
                })?;
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let flows = client
                    .read_cash_flows(
                        resolved.header.clone(),
                        clearing_date,
                        request.cash_flow_direction(),
                    )
                    .map_err(session_error)?;
                let cash_flows = flows
                    .into_iter()
                    .map(|flow| cash_flow_value(&resolved, flow))
                    .collect::<Vec<_>>();
                Ok(
                    json!({"checkedAt": checked_at(), "connectivity": "connected", "cashFlows": cash_flows}),
                )
            }
            "order-fees" => {
                let order_ids = request
                    .order_id_ex_list()
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let fees = client
                    .read_order_fees(resolved.header.clone(), order_ids)
                    .map_err(session_error)?;
                let fees = fees
                    .into_iter()
                    .map(|fee| order_fee_value(&resolved, fee))
                    .collect::<Vec<_>>();
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "fees": fees}))
            }
            "margin-ratios" => product_trade_margin_route::read_margin_ratios(
                &request,
                client.as_ref(),
                self.trade_runtime.as_ref(),
            ),
            "max-trade-qtys" => {
                let (market, code, sec_market) = request
                    .max_trade_symbol()
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let resolved = request
                    .resolve_account_with_environment(client.as_ref(), None, Some(&market))
                    .map_err(map_broker_header_error)?;
                let max_request = request
                    .max_trade_quantity_request(resolved.header.clone(), code, sec_market)
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let snapshot = client
                    .read_max_trade_quantity(max_request)
                    .map_err(session_error)?;
                Ok(json!({
                    "checkedAt": checked_at(),
                    "connectivity": "connected",
                    "maxTradeQuantity": max_trade_quantity_value(&resolved, snapshot, &market),
                }))
            }
            "positions" => {
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let positions = client
                    .read_positions(
                        resolved.header.clone(),
                        request.filter(),
                        None,
                        None,
                        request.refresh_cache(),
                        None,
                        None,
                        None,
                    )
                    .map_err(session_error)?;
                let positions = positions.into_iter().map(|v| {
                    let value = position_value(&resolved, v);
                    json!({
                        "brokerId": request.broker_id,
                        "tradingEnvironment": value["tradingEnvironment"],
                        "accountId": value["accountId"],
                        "market": value["market"],
                        "symbol": value["symbol"],
                        "quantity": value["quantity"],
                        "averagePrice": if value["averageCostPrice"].is_null() { value["costPrice"].clone() } else { value["averageCostPrice"].clone() },
                        "marketValue": value["marketValue"],
                        "updatedAt": checked_at(),
                        "createdAt": checked_at(),
                    })
                }).collect::<Vec<_>>();
                Ok(
                    json!({"checkedAt": checked_at(), "connectivity": "connected", "positions": positions }),
                )
            }
            "orders" => {
                let history = request
                    .history_scope()
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let filter = request
                    .trade_filter(history, &resolved.market)
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let statuses = request
                    .status_codes()
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let orders = if history {
                    client.read_history_orders(
                        resolved.header.clone(),
                        filter,
                        statuses,
                        request.refresh_cache(),
                    )
                } else {
                    client.read_orders(
                        resolved.header.clone(),
                        request.filter(),
                        Vec::new(),
                        request.refresh_cache(),
                    )
                }
                .map_err(session_error)?;
                Ok(
                    json!({"checkedAt": checked_at(), "connectivity": "connected", "orders": orders.into_iter().map(|v| order_value(&resolved, v)).collect::<Vec<_>>() }),
                )
            }
            "fills" => {
                let history = request
                    .history_scope()
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let filter = request
                    .trade_filter(history, &resolved.market)
                    .map_err(BrokerReadSnapshotError::Invalid)?;
                let fills = if history {
                    client.read_history_fills(
                        resolved.header.clone(),
                        filter,
                        request.refresh_cache(),
                    )
                } else {
                    client.read_fills(
                        resolved.header.clone(),
                        request.filter(),
                        request.refresh_cache(),
                    )
                }
                .map_err(session_error)?;
                Ok(
                    json!({"checkedAt": checked_at(), "connectivity": "connected", "fills": fills.into_iter().map(|v| fill_value(&resolved, v)).collect::<Vec<_>>() }),
                )
            }
            _ => Err(unavailable(format!(
                "Futu broker resource '{}' is unavailable",
                request.resource
            ))),
        }
    }
}

impl ProductionBrokerPort {
    fn ensure_ready(&self) -> Result<(), BrokerReadSnapshotError> {
        if self.active_provider_state.snapshot().closing {
            return Err(unavailable("Futu trade session is shutting down"));
        }
        if trade_session_ready(
            self.trade_runtime.as_ref(),
            self.trade_read_port.as_ref(),
            self.trade_logged_in,
        ) {
            return Ok(());
        }
        if self.trade_runtime.is_none()
            && self.trade_read_port.is_none()
            && self.trade_logged_in == Some(true)
        {
            return Err(unavailable("Futu trade read client is unavailable"));
        }
        Err(unavailable("Futu trade session login/account not ready"))
    }
}

#[derive(Clone)]
pub(crate) struct ProductionPortfolioPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) _execution_store: Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    pub(crate) trade_read_port: Option<Arc<dyn TradeReadPort>>,
    pub(crate) trade_logged_in: Option<bool>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
}

impl std::fmt::Debug for ProductionPortfolioPort {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProductionPortfolioPort")
            .field("trade_read_port", &self.trade_read_port.is_some())
            .field("trade_logged_in", &self.trade_logged_in)
            .finish()
    }
}

impl PortfolioSnapshotPort for ProductionPortfolioPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, PortfolioSnapshotError> {
        let request = TradeRequest::parse_with_prefix(path, query, "/api/v1/portfolio/")
            .map_err(PortfolioSnapshotError::Unavailable)?;
        if self.active_provider_state.snapshot().closing
            || !trade_session_ready(
                self.trade_runtime.as_ref(),
                self.trade_read_port.as_ref(),
                self.trade_logged_in,
            )
        {
            return Err(unavailable_portfolio(
                "Futu trade session login/account not ready",
            ));
        }
        let runtime_snapshot = self.trade_runtime.as_ref().map(|r| r.snapshot());
        let client = runtime_snapshot
            .as_ref()
            .and_then(|s| s.client.as_ref())
            .or_else(|| {
                self.trade_runtime
                    .is_none()
                    .then_some(self.trade_read_port.as_ref())
                    .flatten()
            })
            .ok_or_else(|| unavailable_portfolio("Futu trade read client is unavailable"))?;
        let resolved = request
            .resolve_account(client.as_ref())
            .map_err(map_portfolio_header_error)?;
        match request.resource.as_str() {
            "positions" => {
                let positions = client
                    .read_positions(
                        resolved.header.clone(),
                        request.filter(),
                        None,
                        None,
                        request.refresh_cache(),
                        None,
                        None,
                        None,
                    )
                    .map_err(|e| unavailable_portfolio(e.to_string()))?;
                Ok(
                    json!({"checkedAt": checked_at(), "connectivity": "connected", "positions": positions.into_iter().map(|v| position_value(&resolved, v)).collect::<Vec<_>>() }),
                )
            }
            "cash-balances" => {
                let funds = client
                    .read_funds(resolved.header.clone(), request.refresh_cache(), None, None)
                    .map_err(|e| unavailable_portfolio(e.to_string()))?;
                let balances =
                    portfolio_cash_balance_values(&request.broker_id, &resolved, &funds.funds);
                Ok(
                    json!({"checkedAt": checked_at(), "connectivity": "connected", "balances": balances }),
                )
            }
            _ => Err(unavailable_portfolio(format!(
                "Futu portfolio resource '{}' is unavailable",
                request.resource
            ))),
        }
    }
}

fn portfolio_cash_balance_values(
    broker_id: &str,
    resolved: &ResolvedTradeRequest,
    funds: &TradeFunds,
) -> Vec<Value> {
    let timestamp = checked_at();
    let mut balances = funds
        .cash_info_list
        .iter()
        .map(|cash| {
            json!({
                "brokerId": broker_id,
                "tradingEnvironment": resolved.environment,
                "accountId": resolved.account_id,
                "currency": currency_label(cash.currency),
                "cashBalance": cash.cash,
                "updatedAt": timestamp,
                "createdAt": timestamp,
            })
        })
        .collect::<Vec<_>>();
    if balances.is_empty()
        && let Some(currency) = currency_label(funds.currency)
    {
        balances.push(json!({
            "brokerId": broker_id,
            "tradingEnvironment": resolved.environment,
            "accountId": resolved.account_id,
            "currency": currency,
            "cashBalance": funds.cash,
            "updatedAt": timestamp,
            "createdAt": timestamp,
        }));
    }
    balances
}

#[cfg(test)]
#[path = "product_production_ports_trade_tests.rs"]
mod tests;
