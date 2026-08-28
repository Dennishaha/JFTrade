//! Read-only Futu trade projections for production broker and portfolio APIs.
//!
//! The adapter deliberately consumes the engine-neutral `TradeReadPort`; no
//! generated OpenD protobuf type crosses this module boundary.  Execution
//! writes and the durable execution ledger remain owned by the local store.

use std::sync::Arc;
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
            return Ok(json!({
                "brokers": [{"id": "futu", "displayName": "Futu", "status": "ready"}],
                "runtime": {"tradeLoggedIn": true}
            }));
        }
        let request = TradeRequest::parse(path, query).map_err(BrokerReadSnapshotError::Invalid)?;
        self.ensure_ready()?;
        let client = self
            .trade_read_port
            .as_ref()
            .ok_or_else(|| unavailable("Futu trade read client is unavailable"))?;
        match request.resource.as_str() {
            "runtime" => {
                let accounts = client
                    .read_accounts(0, None, None)
                    .map_err(session_error)?;
                let accounts_discovered = accounts.len();
                Ok(json!({
                    "accounts": accounts.into_iter().map(account_value).collect::<Vec<_>>(),
                    "session": {"brokerId": request.broker_id, "accountsDiscovered": accounts_discovered, "tradeLoggedIn": true, "connectivity": "connected", "checkedAt": checked_at()}
                }))
            }
            "funds" => {
                let header = request.header().map_err(map_broker_header_error)?;
                let funds = client
                    .read_funds(header, request.refresh_cache(), None, None)
                    .map_err(session_error)?;
                Ok(funds_value(&request, funds))
            }
            "positions" => {
                let positions = client
                    .read_positions(
                        request.header().map_err(map_broker_header_error)?,
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
                    let value = position_value(&request, v);
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
                let orders = client
                    .read_orders(request.header().map_err(map_broker_header_error)?, request.filter(), Vec::new(), request.refresh_cache())
                    .map_err(session_error)?;
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "orders": orders.into_iter().map(|v| order_value(&request, v)).collect::<Vec<_>>() }))
            }
            "fills" => {
                let fills = client
                    .read_fills(request.header().map_err(map_broker_header_error)?, request.filter(), request.refresh_cache())
                    .map_err(session_error)?;
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "fills": fills.into_iter().map(|v| fill_value(&request, v)).collect::<Vec<_>>() }))
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
        if state.opend_ready && self.trade_logged_in == Some(true) {
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
            || self.trade_logged_in != Some(true)
        {
            return Err(unavailable_portfolio("Futu trade session login/account not ready"));
        }
        let client = self
            .trade_read_port
            .as_ref()
            .ok_or_else(|| unavailable_portfolio("Futu trade read client is unavailable"))?;
        match request.resource.as_str() {
            "positions" => {
                let positions = client
                    .read_positions(request.header().map_err(map_portfolio_header_error)?, request.filter(), None, None, request.refresh_cache(), None, None, None)
                    .map_err(|e| unavailable_portfolio(e.to_string()))?;
                Ok(json!({"checkedAt": checked_at(), "connectivity": "connected", "positions": positions.into_iter().map(|v| position_value(&request, v)).collect::<Vec<_>>() }))
            }
            "cash-balances" => {
                let funds = client
                    .read_funds(request.header().map_err(map_portfolio_header_error)?, request.refresh_cache(), None, None)
                    .map_err(|e| unavailable_portfolio(e.to_string()))?;
                let balances = funds.funds.cash_info_list.into_iter().map(|cash| json!({
                    "currency": cash.currency,
                    "cash": cash.cash,
                    "availableBalance": cash.available_balance,
                    "netCashPower": cash.net_cash_power,
                })).collect::<Vec<_>>();
                let balances = balances.into_iter().map(|balance| json!({
                    "brokerId": request.broker_id,
                    "tradingEnvironment": request.environment_label(),
                    "accountId": request.account_id(),
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

    fn header(&self) -> Result<TradeHeader, String> {
        let account = self.account_id().ok_or_else(|| "accountId is required".to_owned())?;
        let acc_id = account.parse::<u64>().map_err(|_| "accountId must be a numeric Futu account id".to_owned())?;
        let env = match self.query.get_first("tradingEnvironment").unwrap_or("real").to_ascii_lowercase().as_str() {
            "real" | "production" => 0,
            "sim" | "simulate" | "paper" => 1,
            value => return Err(format!("invalid tradingEnvironment: {value}")),
        };
        let market_name = self.market_label();
        let market = market_code(&market_name)?;
        Ok(trade_header(env, acc_id, market))
    }

    fn account_id(&self) -> Option<&str> {
        self.query.get_first("accountId").map(str::trim).filter(|value| !value.is_empty())
    }

    fn environment_label(&self) -> String {
        match self.query.get_first("tradingEnvironment").map(str::trim).unwrap_or("real").to_ascii_lowercase().as_str() {
            "sim" | "simulate" | "paper" => "SIMULATE".to_owned(),
            "real" | "production" => "REAL".to_owned(),
            _ => String::new(),
        }
    }

    fn market_label(&self) -> String {
        self.query.get_first("market").map(str::trim).unwrap_or("HK").to_owned()
    }

    fn refresh_cache(&self) -> Option<bool> {
        self.query.get_first("refreshCache").and_then(|v| match v.to_ascii_lowercase().as_str() { "1" | "true" => Some(true), "0" | "false" => Some(false), _ => None })
    }

    fn filter(&self) -> Option<TradeFilter> {
        self.query.get_first("symbol").map(|symbol| TradeFilter { code_list: vec![symbol.rsplit('.').next().unwrap_or(symbol).to_owned()], ..TradeFilter::default() })
    }
}

fn market_code(value: &str) -> Result<i32, String> {
    match value.trim().to_ascii_uppercase().as_str() {
        "HK" => Ok(1), "US" => Ok(11), "SH" => Ok(21), "SZ" => Ok(22), "CN" => Ok(21),
        value => Err(format!("invalid market: {value}")),
    }
}

fn checked_at() -> String {
    SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_millis().to_string()).unwrap_or_else(|_| "0".to_owned())
}

fn account_value(value: jftrade_integration_futu::TradeAccountSnapshot) -> Value {
    json!({"tradingEnvironment": value.trd_env, "accountId": value.acc_id, "tradingMarketAuth": value.trd_market_auth_list, "accountType": value.acc_type, "cardNumber": value.card_num, "securityFirm": value.security_firm, "accountStatus": value.acc_status, "accountRole": value.acc_role})
}

fn funds_value(request: &TradeRequest, value: TradeFundsSnapshot) -> Value {
    let balances = value.funds.cash_info_list.iter().map(|cash| json!({"accountId": request.account_id(), "tradingEnvironment": request.environment_label(), "currency": currency_label(cash.currency), "cash": cash.cash, "availableWithdrawalCash": cash.available_balance, "netCashPower": cash.net_cash_power})).collect::<Vec<_>>();
    let assets = value.funds.market_info_list.iter().map(|item| json!({"accountId": request.account_id(), "tradingEnvironment": request.environment_label(), "market": market_label_from_code(item.trd_market), "assets": item.assets})).collect::<Vec<_>>();
    json!({"checkedAt": checked_at(), "connectivity": "connected", "currencyBalances": balances, "marketAssets": assets, "summary": {"accountId": request.account_id(), "tradingEnvironment": request.environment_label(), "market": request.market_label(), "power": value.funds.power, "totalAssets": value.funds.total_assets, "cash": value.funds.cash, "marketValue": value.funds.market_val, "frozenCash": value.funds.frozen_cash, "debtCash": value.funds.debt_cash, "availableWithdrawalCash": value.funds.avl_withdrawal_cash, "currency": value.funds.currency, "availableFunds": value.funds.available_funds, "unrealizedPnl": value.funds.unrealized_pl, "realizedPnl": value.funds.realized_pl, "securitiesAssets": value.funds.securities_assets, "fundAssets": value.funds.fund_assets, "bondAssets": value.funds.bond_assets, "longMarketValue": value.funds.long_mv, "shortMarketValue": value.funds.short_mv, "netCashPower": value.funds.net_cash_power, "maxWithdrawal": value.funds.max_withdrawal, "pendingAsset": value.funds.pending_asset, "initialMargin": value.funds.initial_margin, "maintenanceMargin": value.funds.maintenance_margin, "marginCallMargin": value.funds.margin_call_margin, "isPdt": value.funds.is_pdt, "pdtSeq": value.funds.pdt_seq, "beginningDTBP": value.funds.beginning_dtbp, "remainingDTBP": value.funds.remaining_dtbp, "dtCallAmount": value.funds.dt_call_amount, "exposureLevel": value.funds.exposure_level, "exposureLimit": value.funds.exposure_limit, "usedLimit": value.funds.used_limit, "remainingLimit": value.funds.remaining_limit}})
}

fn position_value(request: &TradeRequest, value: jftrade_integration_futu::TradePositionSnapshot) -> Value {
    json!({"accountId": request.account_id(), "tradingEnvironment": request.environment_label(), "market": request.market_label(), "positionId": value.position_id, "positionSide": value.position_side, "symbol": qualify_symbol(&request.market_label(), &value.code), "symbolName": value.name, "quantity": value.qty, "sellableQuantity": value.can_sell_qty, "lastPrice": value.price, "costPrice": value.cost_price, "averageCostPrice": value.average_cost_price, "marketValue": value.val, "unrealizedPnl": value.unrealized_pl, "realizedPnl": value.realized_pl, "pnlRatio": value.pl_ratio, "currency": currency_label(value.currency)})
}

fn order_value(request: &TradeRequest, value: jftrade_integration_futu::TradeOrderSnapshot) -> Value {
    json!({"accountId": request.account_id(), "tradingEnvironment": request.environment_label(), "market": request.market_label(), "brokerOrderId": value.order_id.to_string(), "brokerOrderIdEx": value.order_id_ex, "symbol": qualify_symbol(&request.market_label(), &value.code), "symbolName": value.name, "side": trade_side(value.trd_side), "orderType": value.order_type.to_string(), "status": value.order_status.to_string(), "quantity": value.qty, "price": value.price, "submittedAt": value.create_time, "updatedAt": value.update_time, "filledQuantity": value.fill_qty, "filledAveragePrice": value.fill_avg_price, "lastError": value.last_err_msg, "remark": value.remark, "timeInForce": value.time_in_force.map(|v| v.to_string()), "currency": currency_label(value.currency)})
}

fn fill_value(request: &TradeRequest, value: jftrade_integration_futu::TradeFillSnapshot) -> Value {
    json!({"accountId": request.account_id(), "tradingEnvironment": request.environment_label(), "market": request.market_label(), "brokerFillId": value.fill_id.to_string(), "brokerFillIdEx": value.fill_id_ex, "brokerOrderId": value.order_id.map(|v| v.to_string()), "brokerOrderIdEx": value.order_id_ex, "symbol": qualify_symbol(&request.market_label(), &value.code), "symbolName": value.name, "side": trade_side(value.trd_side), "quantity": value.qty, "price": value.price, "submittedAt": value.create_time, "counterBrokerId": value.counter_broker_id, "counterBrokerName": value.counter_broker_name, "status": value.status.map(|v| v.to_string())})
}

fn qualify_symbol(market: &str, code: &str) -> String {
    if code.contains('.') || market.is_empty() { code.to_owned() } else { format!("{market}.{code}") }
}

fn currency_label(currency: Option<i32>) -> Option<&'static str> {
    match currency { Some(1) => Some("HKD"), Some(2) => Some("USD"), Some(3) => Some("CNH"), Some(4) => Some("CNY"), Some(5) => Some("JPY"), _ => None }
}

fn market_label_from_code(market: Option<i32>) -> Option<&'static str> {
    match market { Some(1) => Some("HK"), Some(11) => Some("US"), Some(21) => Some("SH"), Some(22) => Some("SZ"), _ => None }
}

fn trade_side(side: i32) -> &'static str {
    match side { 1 => "BUY", 2 => "SELL", 3 => "SELL_SHORT", 4 => "BUY_BACK", _ => "UNKNOWN" }
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
        TradeAccountSnapshot, TradeFillSnapshot, TradeFunds, TradeFundsSnapshot,
        TradeOrderSnapshot, TradePositionSnapshot,
    };

    #[derive(Debug)]
    struct FakeTradeRead;

    impl TradeReadPort for FakeTradeRead {
        fn read_accounts(&self, _: u64, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> { Ok(Vec::new()) }
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
        let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: None, trade_logged_in: Some(true) };
        let error = port.read("/api/v1/brokers/futu/funds", "accountId=42&market=US").expect_err("missing client");
        assert!(error.to_string().contains("trade read client"));
    }

    #[test]
    fn broker_read_projects_futu_funds_from_neutral_client() {
        let port = ProductionBrokerPort { active_provider_state: ready_state(), trade_read_port: Some(Arc::new(FakeTradeRead)), trade_logged_in: Some(true) };
        let value = port.read("/api/v1/brokers/futu/funds", "accountId=42&market=US").expect("funds");
        assert_eq!(value["summary"]["totalAssets"], 2.0);
        assert_eq!(value["connectivity"], "connected");
    }
}
