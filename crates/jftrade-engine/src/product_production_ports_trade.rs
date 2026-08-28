//! Read-only Futu trade projections for production broker and portfolio APIs.
//!
//! The adapter deliberately consumes the engine-neutral `TradeReadPort`; no
//! generated OpenD protobuf type crosses this module boundary.  Execution
//! writes and the durable execution ledger remain owned by the local store.

use std::sync::{Arc, RwLock};
use std::time::{SystemTime, UNIX_EPOCH};

use jftrade_integration_futu::{
    TradeFilter, TradeFundsSnapshot, TradeHeader, TradeReadPort, TradeSessionError,
    trade_header,
};
use jftrade_settings::MarketDataProvider;
use serde_json::{Value, json};

use super::ActiveProviderState;
use crate::product::{
    BrokerReadSnapshotError, BrokerReadSnapshotPort, PortfolioSnapshotError,
    PortfolioSnapshotPort,
};
use crate::product::product_query::QueryMap;

#[derive(Clone)]
pub(crate) struct ProductionBrokerPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_read_port: Option<Arc<dyn TradeReadPort>>,
    pub(crate) trade_logged_in: Option<bool>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
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
            let descriptor = serde_json::to_value(jftrade_integration_futu::broker_descriptor())
                .map_err(|error| unavailable(error.to_string()))?;
            return Ok(json!({
                "brokers": [{"id": "futu", "displayName": "Futu", "status": "ready", "descriptor": descriptor}],
                "catalog": {"brokers": ["futu"]},
                "runtime": []
            }));
        }
        let request = TradeRequest::parse(path, query).map_err(BrokerReadSnapshotError::Invalid)?;
        self.ensure_ready()?;
        let runtime_snapshot = self.trade_runtime.as_ref().map(|r| r.snapshot());
        let client = runtime_snapshot
            .as_ref()
            .and_then(|s| s.client.as_ref())
            .or_else(|| self.trade_runtime.is_none().then_some(self.trade_read_port.as_ref()).flatten())
            .ok_or_else(|| unavailable("Futu trade read client is unavailable"))?;
        match request.resource.as_str() {
            "runtime" => {
                let accounts = client
                    .read_accounts(0, None, None)
                    .map_err(session_error)?;
                let accounts_discovered = accounts.len();
                let descriptor = serde_json::to_value(jftrade_integration_futu::broker_descriptor())
                    .map_err(|error| unavailable(error.to_string()))?;
                Ok(json!({
                    "accounts": accounts.into_iter().map(account_value).collect::<Vec<_>>(),
                    "descriptor": descriptor,
                    "session": {"brokerId": request.broker_id, "displayName": "Futu", "accountsDiscovered": accounts_discovered, "tradeLoggedIn": true, "connectivity": "connected", "checkedAt": checked_at(), "connection": {"host": "", "apiPort": 0, "websocketPort": 0, "port": 0, "useEncryption": false, "marketDataTransport": "opend-tcp"}, "globalState": null, "lastError": null, "liveWebSocketClients": {"connected": 0, "limit": 0, "atLimit": false}}
                }))
            }
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
                let clearing_date = request
                    .clearing_date()
                    .ok_or_else(|| BrokerReadSnapshotError::Invalid("query parameter clearingDate is required".to_owned()))?;
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
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "cashFlows": cash_flows}))
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
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "positions": positions }))
            }
            "orders" => {
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let orders = client
                    .read_orders(resolved.header.clone(), request.filter(), Vec::new(), request.refresh_cache())
                    .map_err(session_error)?;
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "orders": orders.into_iter().map(|v| order_value(&resolved, v)).collect::<Vec<_>>() }))
            }
            "fills" => {
                let resolved = request
                    .resolve_account(client.as_ref())
                    .map_err(map_broker_header_error)?;
                let fills = client
                    .read_fills(resolved.header.clone(), request.filter(), request.refresh_cache())
                    .map_err(session_error)?;
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "fills": fills.into_iter().map(|v| fill_value(&resolved, v)).collect::<Vec<_>>() }))
            }
            _ => Err(unavailable(format!("Futu broker resource '{}' is unavailable", request.resource))),
        }
    }
}

impl ProductionBrokerPort {
    fn ensure_ready(&self) -> Result<(), BrokerReadSnapshotError> {
        let state = self.active_provider_state.snapshot();
        if state.provider != Some(MarketDataProvider::Futu) {
            return Err(unavailable("Futu broker is not the active provider"));
        }
        let runtime_ready = self.trade_runtime.as_ref().is_some_and(|runtime| runtime.snapshot().is_ready());
        if state.opend_ready
            && (runtime_ready
                || (self.trade_runtime.is_none() && self.trade_logged_in == Some(true)))
        {
            return Ok(());
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
        let state = self.active_provider_state.snapshot();
        if state.provider != Some(MarketDataProvider::Futu)
            || !state.opend_ready
            || !(self
                .trade_runtime
                .as_ref()
                .is_some_and(|runtime| runtime.snapshot().is_ready())
                || (self.trade_runtime.is_none() && self.trade_logged_in == Some(true)))
        {
            return Err(unavailable_portfolio("Futu trade session login/account not ready"));
        }
        let runtime_snapshot = self.trade_runtime.as_ref().map(|r| r.snapshot());
        let client = runtime_snapshot
            .as_ref()
            .and_then(|s| s.client.as_ref())
            .or_else(|| self.trade_runtime.is_none().then_some(self.trade_read_port.as_ref()).flatten())
            .ok_or_else(|| unavailable_portfolio("Futu trade read client is unavailable"))?;
        let resolved = request
            .resolve_account(client.as_ref())
            .map_err(map_portfolio_header_error)?;
        match request.resource.as_str() {
            "positions" => {
                let positions = client
                    .read_positions(resolved.header.clone(), request.filter(), None, None, request.refresh_cache(), None, None, None)
                    .map_err(|e| unavailable_portfolio(e.to_string()))?;
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "positions": positions.into_iter().map(|v| position_value(&resolved, v)).collect::<Vec<_>>() }))
            }
            "cash-balances" => {
                let funds = client
                    .read_funds(resolved.header.clone(), request.refresh_cache(), None, None)
                    .map_err(|e| unavailable_portfolio(e.to_string()))?;
                let balances = funds.funds.cash_info_list.into_iter().map(|cash| json!({
                    "currency": cash.currency,
                    "cash": cash.cash,
                    "availableBalance": cash.available_balance,
                    "netCashPower": cash.net_cash_power,
                })).collect::<Vec<_>>();
                let balances = balances.into_iter().map(|balance| json!({
                    "brokerId": request.broker_id,
                    "tradingEnvironment": resolved.environment,
                    "accountId": resolved.account_id,
                    "currency": balance["currency"],
                    "cashBalance": balance["cash"],
                    "updatedAt": checked_at(),
                    "createdAt": checked_at(),
                })).collect::<Vec<_>>();
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "balances": balances }))
            }
            _ => Err(unavailable_portfolio(format!("Futu portfolio resource '{}' is unavailable", request.resource))),
        }
    }
}

#[derive(Clone, Default)]
pub(crate) struct SharedTradeReadRuntime(Arc<RwLock<Option<(Arc<dyn TradeReadPort>, bool)>>>);

impl std::fmt::Debug for SharedTradeReadRuntime {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result { f.debug_struct("SharedTradeReadRuntime").field("ready", &self.snapshot().is_ready()).finish() }
}

#[derive(Clone)]
pub(crate) struct TradeReadRuntimeSnapshot { pub client: Option<Arc<dyn TradeReadPort>>, pub trade_logged_in: Option<bool> }

impl TradeReadRuntimeSnapshot {
    pub(crate) fn is_ready(&self) -> bool {
        self.client.is_some() && self.trade_logged_in == Some(true)
    }
}

impl SharedTradeReadRuntime {
    pub(crate) fn set(&self, client: Option<Arc<dyn TradeReadPort>>, logged_in: Option<bool>) {
        *self.0.write().unwrap_or_else(|e| e.into_inner()) =
            client.map(|c| (c, logged_in == Some(true)));
    }
    pub(crate) fn clear(&self) { *self.0.write().unwrap_or_else(|e| e.into_inner()) = None; }
    pub(crate) fn snapshot(&self) -> TradeReadRuntimeSnapshot {
        self.0.read().unwrap_or_else(|e| e.into_inner()).as_ref().map_or(TradeReadRuntimeSnapshot { client: None, trade_logged_in: None }, |(c, logged)| TradeReadRuntimeSnapshot { client: Some(Arc::clone(c)), trade_logged_in: Some(*logged) })
    }
}

#[derive(Debug)]
struct TradeRequest {
    broker_id: String,
    resource: String,
    query: QueryMap,
}

impl TradeRequest {
    fn parse(path: &str, raw_query: &str) -> Result<Self, String> {
        Self::parse_with_prefix(path, raw_query, "/api/v1/brokers/")
    }

    fn parse_with_prefix(path: &str, raw_query: &str, prefix: &str) -> Result<Self, String> {
        let query = QueryMap::parse(raw_query).map_err(|_| "invalid query encoding".to_owned())?;
        let suffix = path.strip_prefix(prefix).unwrap_or(path);
        let (broker_id, resource) = suffix
            .split_once('/')
            .ok_or_else(|| "invalid broker path".to_owned())?;
        if broker_id.is_empty() || resource.is_empty() || resource.contains('/') {
            return Err("invalid broker path".to_owned());
        }
        Ok(Self { broker_id: broker_id.to_owned(), resource: resource.to_owned(), query })
    }

    #[allow(dead_code)]
    fn header(&self) -> Result<TradeHeader, String> {
        let account = self.account_id().ok_or_else(|| "accountId is required".to_owned())?;
        let acc_id = account.parse::<u64>().map_err(|_| "accountId must be a numeric Futu account id".to_owned())?;
        let env = match self.query.get_first("tradingEnvironment").unwrap_or("real").to_ascii_lowercase().as_str() {
            "sim" | "simulate" | "paper" => 0,
            "real" | "production" => 1,
            value => return Err(format!("invalid tradingEnvironment: {value}")),
        };
        let market_name = self.market_label();
        let market = market_code(&market_name)?;
        Ok(trade_header(env, acc_id, market))
    }

    fn resolve_account(
        &self,
        client: &dyn TradeReadPort,
    ) -> Result<ResolvedTradeRequest, String> {
        let accounts = client
            .read_accounts(0, None, None)
            .map_err(|error| error.to_string())?;
        if accounts.is_empty() {
            return Err("no Futu trading accounts discovered".to_owned());
        }
        let requested_account = self.account_id().map(str::to_ascii_lowercase);
        let requested_environment = self.environment_code()?;
        let requested_market = self
            .query
            .get_first("market")
            .map(str::trim)
            .filter(|market| !market.is_empty())
            .map(str::to_ascii_uppercase);
        let mut candidates = accounts.into_iter().filter(|account| {
            let account_id = account_identity(account);
            let account_matches = requested_account
                .as_deref()
                .is_none_or(|requested| account_id.as_deref().is_some_and(|id| id.eq_ignore_ascii_case(requested)));
            let environment_matches = requested_environment.is_none_or(|environment| account.trd_env == environment);
            let market_matches = requested_market.as_ref().is_none_or(|requested| {
                account
                    .trd_market_auth_list
                    .iter()
                    .any(|market| trade_market_authority(*market) == Some(requested.as_str()))
            });
            account_matches && environment_matches && market_matches
        }).collect::<Vec<_>>();
        if requested_environment.is_none() {
            let simulated = candidates.iter().filter(|account| account.trd_env == 0).count();
            if simulated > 0 {
                candidates.retain(|account| account.trd_env == 0);
            }
        }
        let selected = candidates.into_iter().next();
        let Some(account) = selected else {
            let account_detail = requested_account
                .as_deref()
                .map_or_else(|| "any account".to_owned(), |id| format!("account {id}"));
            return Err(format!(
                "no Futu trading account matched {account_detail} for tradingEnvironment={} market={}",
                environment_label_from_code(requested_environment.unwrap_or(0)),
                requested_market.as_deref().unwrap_or("HK")
            ));
        };
        let account_id = account_identity(&account)
            .ok_or_else(|| "selected Futu account has no usable account identity".to_owned())?;
        let acc_id = account.acc_id;
        let selected_market = requested_market.clone().or_else(|| {
            account
                .trd_market_auth_list
                .iter()
                .find_map(|market| trade_market_authority(*market).map(str::to_owned))
        }).unwrap_or_else(|| "HK".to_owned());
        let header_market = account
            .trd_market_auth_list
            .iter()
            .copied()
            .find(|market| trade_market_authority(*market) == Some(selected_market.as_str()))
            .unwrap_or(market_code(&self.market_label())?);
        Ok(ResolvedTradeRequest {
            account_id,
            environment: environment_label_from_code(account.trd_env).to_owned(),
            market: selected_market,
            header: trade_header(
                account.trd_env,
                acc_id,
                header_market,
            ),
        })
    }

    fn environment_code(&self) -> Result<Option<i32>, String> {
        let Some(raw) = self.query.get_first("tradingEnvironment") else {
            return Ok(None);
        };
        match self
            .query
            .get_first("tradingEnvironment")
            .unwrap_or(raw)
            .trim()
            .to_ascii_lowercase()
            .as_str()
        {
            "sim" | "simulate" | "paper" => Ok(Some(0)),
            "real" | "production" => Ok(Some(1)),
            value => Err(format!("invalid tradingEnvironment: {value}")),
        }
    }

    fn account_id(&self) -> Option<&str> {
        self.query.get_first("accountId").map(str::trim).filter(|value| !value.is_empty())
    }

    fn market_label(&self) -> String {
        self.query.get_first("market").map(str::trim).unwrap_or("HK").to_owned()
    }

    fn refresh_cache(&self) -> Option<bool> {
        self.query.get_first("refreshCache").and_then(|v| match v.to_ascii_lowercase().as_str() { "1" | "true" => Some(true), "0" | "false" => Some(false), _ => None })
    }

    fn clearing_date(&self) -> Option<String> {
        self.query
            .get_first("clearingDate")
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned)
    }

    fn cash_flow_direction(&self) -> Option<i32> {
        match self.query.get_first("direction").map(str::trim).map(str::to_ascii_uppercase).as_deref() {
            Some("IN") | Some("CASH_FLOW_DIRECTION_IN") => Some(1),
            Some("OUT") | Some("CASH_FLOW_DIRECTION_OUT") => Some(2),
            _ => None,
        }
    }

    fn filter(&self) -> Option<TradeFilter> {
        self.query.get_first("symbol").map(|symbol| TradeFilter { code_list: vec![symbol.rsplit('.').next().unwrap_or(symbol).to_owned()], ..TradeFilter::default() })
    }
}

struct ResolvedTradeRequest {
    account_id: String,
    environment: String,
    market: String,
    header: TradeHeader,
}

fn account_identity(account: &jftrade_integration_futu::TradeAccountSnapshot) -> Option<String> {
    if account.acc_id != 0 {
        return Some(account.acc_id.to_string());
    }
    account
        .card_num
        .as_deref()
        .or(account.uni_card_num.as_deref())
        .map(str::trim)
        .filter(|id| !id.is_empty())
        .map(ToOwned::to_owned)
}

fn environment_label_from_code(code: i32) -> &'static str {
    match code {
        0 => "SIMULATE",
        1 => "REAL",
        _ => "UNKNOWN",
    }
}

fn market_code(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Ok(1), "US" => Ok(2), "SH" | "SZ" | "CN" => Ok(3),
        value => Err(format!("invalid market: {value}")),
    }
}

fn checked_at() -> String {
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_millis().to_string()).unwrap_or_else(|_| "0".to_owned())
}

fn account_value(value: jftrade_integration_futu::TradeAccountSnapshot) -> Value {
    let account_id = account_identity(&value);
    let markets = value
        .trd_market_auth_list
        .iter()
        .filter_map(|market| trade_market_authority(*market))
        .fold(Vec::new(), |mut result, market| {
            if !result.contains(&market) {
                result.push(market);
            }
            result
        });
    json!({
        "tradingEnvironment": environment_label_from_code(value.trd_env),
        "accountId": account_id.unwrap_or_default(),
        "accountType": value.acc_type.map(account_type_label).unwrap_or("UNKNOWN"),
        "accountRole": value.acc_role.and_then(account_role_label),
        "securityFirm": value.security_firm.and_then(security_firm_label),
        "marketAuthorities": markets,
        "simulatedAccountType": value.sim_acc_type.and_then(simulated_account_type_label),
    })
}

fn funds_value(request: &ResolvedTradeRequest, value: TradeFundsSnapshot) -> Value {
    let balances = value.funds.cash_info_list.iter().map(|cash| json!({"accountId": request.account_id, "tradingEnvironment": request.environment, "currency": currency_label(cash.currency), "cash": cash.cash, "availableWithdrawalCash": cash.available_balance, "netCashPower": cash.net_cash_power})).collect::<Vec<_>>();
    let assets = value
        .funds
        .market_info_list
        .iter()
        .map(|item| {
            json!({
                "accountId": request.account_id,
                "tradingEnvironment": request.environment,
                "market": market_label_from_code(item.trd_market).unwrap_or(request.market.as_str()),
                "assets": item.assets,
            })
        })
        .collect::<Vec<_>>();
    json!({"checkedAt": checked_at(), "connectivity": "connected", "currencyBalances": balances, "marketAssets": assets, "summary": {"accountId": request.account_id, "tradingEnvironment": request.environment, "market": request.market, "power": value.funds.power, "totalAssets": value.funds.total_assets, "cash": value.funds.cash, "marketValue": value.funds.market_val, "frozenCash": value.funds.frozen_cash, "debtCash": value.funds.debt_cash, "availableWithdrawalCash": value.funds.avl_withdrawal_cash, "currency": value.funds.currency, "availableFunds": value.funds.available_funds, "unrealizedPnl": value.funds.unrealized_pl, "realizedPnl": value.funds.realized_pl, "securitiesAssets": value.funds.securities_assets, "fundAssets": value.funds.fund_assets, "bondAssets": value.funds.bond_assets, "longMarketValue": value.funds.long_mv, "shortMarketValue": value.funds.short_mv, "netCashPower": value.funds.net_cash_power, "maxWithdrawal": value.funds.max_withdrawal, "pendingAsset": value.funds.pending_asset, "initialMargin": value.funds.initial_margin, "maintenanceMargin": value.funds.maintenance_margin, "marginCallMargin": value.funds.margin_call_margin, "isPdt": value.funds.is_pdt, "pdtSeq": value.funds.pdt_seq, "beginningDTBP": value.funds.beginning_dtbp, "remainingDTBP": value.funds.remaining_dtbp, "dtCallAmount": value.funds.dt_call_amount, "exposureLevel": value.funds.exposure_level, "exposureLimit": value.funds.exposure_limit, "usedLimit": value.funds.used_limit, "remainingLimit": value.funds.remaining_limit}})
}

fn position_value(request: &ResolvedTradeRequest, value: jftrade_integration_futu::TradePositionSnapshot) -> Value {
    json!({"accountId": request.account_id, "tradingEnvironment": request.environment, "market": request.market, "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "quantity": value.qty, "sellableQuantity": value.can_sell_qty, "lastPrice": value.price, "costPrice": value.cost_price, "averageCostPrice": value.average_cost_price, "marketValue": value.val, "unrealizedPnl": value.unrealized_pl, "realizedPnl": value.realized_pl, "pnlRatio": value.pl_ratio, "currency": currency_label(value.currency)})
}

fn order_value(request: &ResolvedTradeRequest, value: jftrade_integration_futu::TradeOrderSnapshot) -> Value {
    json!({"accountId": request.account_id, "brokerOrderId": value.order_id.to_string(), "brokerOrderIdEx": non_empty(&value.order_id_ex), "currency": currency_label(value.currency), "filledAveragePrice": value.fill_avg_price, "filledQuantity": value.fill_qty, "lastError": value.last_err_msg, "market": request.market, "orderType": order_type_label(value.order_type), "price": value.price, "quantity": value.qty, "remark": value.remark, "side": trade_side(value.trd_side), "status": order_status_label(value.order_status), "submittedAt": canonical_time(&value.create_time), "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "timeInForce": value.time_in_force.map(time_in_force_label), "tradingEnvironment": request.environment, "updatedAt": canonical_time(&value.update_time)})
}

fn fill_value(request: &ResolvedTradeRequest, value: jftrade_integration_futu::TradeFillSnapshot) -> Value {
    json!({"accountId": request.account_id, "brokerFillId": value.fill_id.to_string(), "brokerFillIdEx": non_empty(&value.fill_id_ex), "brokerOrderId": value.order_id.map(|v| v.to_string()).unwrap_or_default(), "brokerOrderIdEx": value.order_id_ex, "fillPrice": value.price, "filledAt": canonical_time(&value.create_time), "filledQuantity": value.qty, "market": request.market, "side": trade_side(value.trd_side), "status": value.status.map(fill_status_label), "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "tradingEnvironment": request.environment})
}

fn cash_flow_value(
    request: &ResolvedTradeRequest,
    value: jftrade_integration_futu::TradeCashFlowSnapshot,
) -> Value {
    json!({
        "accountId": request.account_id,
        "tradingEnvironment": request.environment,
        "market": request.market,
        "cashFlowId": value.cash_flow_id.map(|id| id.to_string()),
        "clearingDate": value.clearing_date,
        "settlementDate": value.settlement_date,
        "currency": currency_label(value.currency),
        "cashFlowType": value.cash_flow_type,
        "cashFlowDirection": value.cash_flow_direction.map(cash_flow_direction_label),
        "cashFlowAmount": value.cash_flow_amount,
        "cashFlowRemark": value.cash_flow_remark,
    })
}

fn cash_flow_direction_label(value: i32) -> &'static str {
    match value {
        1 => "IN",
        2 => "OUT",
        _ => "UNKNOWN",
    }
}

fn qualify_symbol(market: &str, code: &str) -> String {
    if code.contains('.') || market.is_empty() { code.to_owned() } else { format!("{market}.{code}") }
}

fn non_empty(value: &str) -> Option<&str> {
    (!value.trim().is_empty()).then_some(value)
}

fn currency_label(currency: Option<i32>) -> Option<&'static str> {
    match currency {
        Some(1) => Some("HKD"), Some(2) => Some("USD"), Some(3) => Some("CNH"),
        Some(4) => Some("JPY"), Some(5) => Some("SGD"), Some(6) => Some("AUD"),
        Some(7) => Some("CAD"), Some(8) => Some("MYR"), Some(9) => Some("NZD"),
        _ => None,
    }
}

fn market_label_from_code(market: Option<i32>) -> Option<&'static str> {
    match market {
        Some(1 | 4 | 10 | 113) => Some("HK"),
        Some(2 | 11 | 123 | 17) => Some("US"),
        Some(3) => Some("CN"),
        Some(5) => Some("FUTURES"),
        Some(6 | 12 | 124) => Some("SG"),
        Some(7) => Some("CRYPTO"),
        Some(8) => Some("AU"),
        Some(13 | 15 | 126) => Some("JP"),
        Some(111 | 125) => Some("MY"),
        Some(112) => Some("CA"),
        _ => None,
    }
}

fn trade_market_authority(value: i32) -> Option<&'static str> {
    market_label_from_code(Some(value))
}

fn trade_side(side: i32) -> &'static str {
    match side {
        1 => "BUY",
        2 => "SELL",
        3 => "SELLSHORT",
        4 => "BUYBACK",
        _ => "UNKNOWN",
    }
}

fn account_type_label(value: i32) -> &'static str {
    match value { 1 => "CASH", 2 => "MARGIN", 3 => "TFSA", 4 => "RRSP", 5 => "SRRSP", 6 => "DERIVATIVES", _ => "UNKNOWN" }
}

fn account_role_label(value: i32) -> Option<&'static str> {
    match value { 1 => Some("NORMAL"), 2 => Some("MASTER"), 3 => Some("IPO"), _ => None }
}

fn security_firm_label(value: i32) -> Option<&'static str> {
    match value { 1 => Some("FUTUSECURITIES"), 2 => Some("FUTUINC"), 3 => Some("FUTUSG"), 4 => Some("FUTUAU"), 5 => Some("FUTUCA"), 6 => Some("FUTUMY"), 7 => Some("FUTUJP"), _ => None }
}

fn simulated_account_type_label(value: i32) -> Option<&'static str> {
    match value { 1 => Some("STOCK"), 2 => Some("OPTION"), 3 => Some("FUTURES"), 4 => Some("STOCKANDOPTION"), 5 => Some("COMPETITION"), _ => None }
}

fn order_type_label(value: i32) -> &'static str {
    match value {
        1 => "NORMAL", 2 => "MARKET", 5 => "ABSOLUTELIMIT", 6 => "AUCTION",
        7 => "AUCTIONLIMIT", 8 => "SPECIALLIMIT", 9 => "SPECIALLIMIT_ALL",
        10 => "STOP", 11 => "STOPLIMIT", 12 => "MARKETIFTOUCHED",
        13 => "LIMITIFTOUCHED", 14 => "TRAILINGSTOP", 15 => "TRAILINGSTOPLIMIT",
        16 => "TWAP_MARKET", 17 => "TWAP_LIMIT", 18 => "VWAP_MARKET", 19 => "VWAP_LIMIT",
        _ => "UNKNOWN",
    }
}

fn order_status_label(value: i32) -> &'static str {
    match value {
        -1 => "UNKNOWN", 0 => "UNSUBMITTED", 1 => "WAITINGSUBMIT", 2 => "SUBMITTING",
        3 => "SUBMITFAILED", 4 => "TIMEOUT", 5 => "SUBMITTED", 10 => "FILLED_PART",
        11 => "FILLED_ALL", 12 => "CANCELLING_PART", 13 => "CANCELLING_ALL",
        14 => "CANCELLED_PART", 15 => "CANCELLED_ALL", 21 => "FAILED", 22 => "DISABLED",
        23 => "DELETED", 24 => "FILLCANCELLED", _ => "UNKNOWN",
    }
}

fn fill_status_label(value: i32) -> &'static str {
    match value { 0 => "OK", 1 => "CANCELLED", 2 => "CHANGED", 3 => "PAYOUT", _ => "UNKNOWN" }
}

fn time_in_force_label(value: i32) -> &'static str {
    match value { 0 => "DAY", 1 => "GTC", 2 => "IOC", 3 => "GTD", _ => "UNKNOWN" }
}

fn canonical_time(value: &str) -> &str {
    value.trim()
}

fn session_error(error: TradeSessionError) -> BrokerReadSnapshotError {
    unavailable(error.to_string())
}

fn unavailable(message: impl Into<String>) -> BrokerReadSnapshotError {
    BrokerReadSnapshotError::Unavailable(message.into())
}

fn unavailable_portfolio(message: impl Into<String>) -> PortfolioSnapshotError {
    PortfolioSnapshotError::Unavailable(message.into())
}

fn map_broker_header_error(message: String) -> BrokerReadSnapshotError {
    if message == "accountId is required" {
        unavailable(message)
    } else {
        BrokerReadSnapshotError::Invalid(message)
    }
}

fn map_portfolio_header_error(message: String) -> PortfolioSnapshotError {
    unavailable_portfolio(message)
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_integration_futu::{
        TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFillSnapshot, TradeFunds, TradeFundsSnapshot,
        TradeOrderSnapshot, TradePositionSnapshot,
    };

    #[derive(Debug)]
    struct FakeTradeRead;

    impl TradeReadPort for FakeTradeRead {
        fn read_accounts(&self, _: u64, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
            Ok(vec![TradeAccountSnapshot {
                trd_env: 1,
                acc_id: 42,
                trd_market_auth_list: vec![1, 2],
                acc_type: Some(2),
                card_num: None,
                security_firm: Some(1),
                sim_acc_type: None,
                uni_card_num: None,
                acc_status: Some(0),
                acc_role: Some(1),
                jp_acc_type: Vec::new(),
                competition_acc_name: None,
            }])
        }
        fn read_funds(&self, header: TradeHeader, _: Option<bool>, _: Option<i32>, _: Option<i32>) -> Result<TradeFundsSnapshot, TradeSessionError> {
            Ok(TradeFundsSnapshot { header, funds: TradeFunds {
                power: 1.0, total_assets: 2.0, cash: 3.0, market_val: 4.0, frozen_cash: 0.0,
                debt_cash: 0.0, avl_withdrawal_cash: 3.0, currency: Some(1), available_funds: None,
                unrealized_pl: None, realized_pl: None, risk_level: None, initial_margin: None,
                maintenance_margin: None, cash_info_list: Vec::new(), max_power_short: None,
                net_cash_power: None, long_mv: None, short_mv: None, pending_asset: None,
                max_withdrawal: None, risk_status: None, margin_call_margin: None, is_pdt: None,
                pdt_seq: None, beginning_dtbp: None, remaining_dtbp: None, dt_call_amount: None,
                dt_status: None, securities_assets: None, fund_assets: None, bond_assets: None,
                market_info_list: Vec::new(), crypto_mv: None, exposure_level: None,
                exposure_limit: None, used_limit: None, remaining_limit: None,
            } })
        }
        fn read_cash_flows(&self, header: TradeHeader, _: String, _: Option<i32>) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
            Ok(vec![TradeCashFlowSnapshot {
                header,
                clearing_date: Some("2026-08-21".to_owned()),
                settlement_date: Some("2026-08-22".to_owned()),
                currency: Some(2),
                cash_flow_type: Some("DIVIDEND".to_owned()),
                cash_flow_direction: Some(1),
                cash_flow_amount: Some(12.5),
                cash_flow_remark: Some("fixture".to_owned()),
                cash_flow_id: Some(9),
                create_time: None,
            }])
        }
        fn read_positions(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<f64>, _: Option<f64>, _: Option<bool>, _: Option<i32>, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> { Ok(Vec::new()) }
        fn read_orders(&self, _: TradeHeader, _: Option<TradeFilter>, _: Vec<i32>, _: Option<bool>) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> { Ok(Vec::new()) }
        fn read_fills(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<bool>) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> { Ok(Vec::new()) }
    }

    fn ready_state() -> Arc<ActiveProviderState> {
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        state.set_readiness(false, true, true);
        state
    }

    #[test]
    fn broker_read_fails_closed_without_trade_client() {
        let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: None, trade_logged_in: Some(true), trade_runtime: None };
        let error = port.read("/api/v1/brokers/futu/funds", "accountId=42&market=US").expect_err("missing client");
        assert!(error.to_string().contains("trade read client"));
    }

    #[test]
    fn broker_read_projects_futu_funds_from_neutral_client() {
        let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
        let value = port.read("/api/v1/brokers/futu/funds", "accountId=42&market=US").expect("funds");
        assert_eq!(value["summary"]["totalAssets"], 2.0);
        assert_eq!(value["connectivity"], "connected");
    }

    #[test]
    fn broker_read_projects_cash_flows_with_baseline_fields_and_sorting() {
        let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
        let value = port
            .read("/api/v1/brokers/futu/cash-flows", "accountId=42&market=US&clearingDate=2026-08-21&direction=IN")
            .expect("cash flows");
        assert_eq!(value["connectivity"], "connected");
        assert_eq!(value["cashFlows"][0]["cashFlowId"], "9");
        assert_eq!(value["cashFlows"][0]["cashFlowDirection"], "IN");
        assert_eq!(value["cashFlows"][0]["cashFlowAmount"], 12.5);
    }

    #[test]
    fn cash_flows_require_clearing_date() {
        let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true), trade_runtime: None };
        let error = port
            .read("/api/v1/brokers/futu/cash-flows", "accountId=42&market=US")
            .expect_err("missing clearing date");
        assert!(matches!(error, BrokerReadSnapshotError::Invalid(message) if message.contains("clearingDate")));
    }

    #[test]
    fn trade_header_uses_futu_trade_enums_not_quote_codes() {
        let request = TradeRequest::parse(
            "/api/v1/brokers/futu/funds",
            "accountId=42&tradingEnvironment=REAL&market=US",
        )
        .expect("request");
        let header = request.header().expect("header");
        assert_eq!(header.trd_env, 1);
        assert_eq!(header.trd_market, 2);
    }

    #[test]
    fn account_projection_matches_broker_runtime_contract() {
        let value = account_value(TradeAccountSnapshot {
            trd_env: 0,
            acc_id: 42,
            trd_market_auth_list: vec![1, 2, 10, 17, 31],
            acc_type: Some(2),
            card_num: Some("ignored-card".to_owned()),
            security_firm: Some(1),
            sim_acc_type: Some(4),
            uni_card_num: None,
            acc_status: Some(0),
            acc_role: Some(1),
            jp_acc_type: Vec::new(),
            competition_acc_name: None,
        });
        assert_eq!(value["accountId"], "42");
        assert_eq!(value["tradingEnvironment"], "SIMULATE");
        assert_eq!(value["accountType"], "MARGIN");
        assert_eq!(value["securityFirm"], "FUTUSECURITIES");
        assert_eq!(value["simulatedAccountType"], "STOCKANDOPTION");
        assert_eq!(value["marketAuthorities"], json!(["HK", "US"]));
        assert!(value.get("tradingMarketAuth").is_none());
        assert!(value.get("cardNumber").is_none());
        assert_eq!(trade_market_authority(12), Some("SG"));
        assert_eq!(trade_market_authority(13), Some("JP"));
        assert_eq!(trade_market_authority(31), None);
    }

    #[test]
    fn generated_trade_enum_values_are_preserved() {
        assert_eq!(order_type_label(5), "ABSOLUTELIMIT");
        assert_eq!(order_type_label(6), "AUCTION");
        assert_eq!(order_type_label(7), "AUCTIONLIMIT");
        assert_eq!(order_type_label(9), "SPECIALLIMIT_ALL");
        assert_eq!(order_status_label(5), "SUBMITTED");
        assert_eq!(order_status_label(10), "FILLED_PART");
        assert_eq!(order_status_label(11), "FILLED_ALL");
        assert_eq!(time_in_force_label(2), "IOC");
        assert_eq!(time_in_force_label(3), "GTD");
        assert_eq!(fill_status_label(0), "OK");
        assert_eq!(currency_label(Some(4)), Some("JPY"));
        assert_eq!(currency_label(Some(5)), Some("SGD"));
        assert_eq!(trade_side(3), "SELLSHORT");
        assert_eq!(trade_side(4), "BUYBACK");
    }

    #[test]
    fn cleared_trade_runtime_cannot_fall_back_to_static_client() {
        let runtime = Arc::new(SharedTradeReadRuntime::default());
        runtime.set(Some(Arc::new(FakeTradeRead)), Some(false));
        let port = ProductionBrokerPort {
            active_provider_state: ready_state(),
            trade_read_port: Some(Arc::new(FakeTradeRead)),
            trade_logged_in: Some(true),
            trade_runtime: Some(runtime),
        };
        let error = port
            .read("/api/v1/brokers/futu/funds", "accountId=42&market=US")
            .expect_err("runtime login false must fail closed");
        assert!(error.to_string().contains("trade session"));
    }
}
