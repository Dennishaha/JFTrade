//! Shared fixtures for the market-data provider actions tests.

use std::sync::Arc;

use jftrade_integration_futu::{
    PredictionComboQuotePort, PredictionMarketReadError, TradeAccountSnapshot, TradeReadPort,
    TradeSessionError,
};
use jftrade_settings::MarketDataProvider;
use serde_json::json;

use crate::product::product_market_data_provider_actions_port::{
    MarketDataProviderActionsRequest, PREDICTION_COMBO_QUOTES_PATH,
};
use crate::product::product_production_ports::SharedTradeReadRuntime;
use super::ProductionMarketDataProviderActionsPort;
use crate::product::product_active_provider_state::ActiveProviderState;

/// One account as OpenD reports it, so the eligibility gate can be exercised.
#[allow(clippy::too_many_arguments)]
pub(super) fn account_snapshot(id: u64, firm: Option<i32>, authorities: Vec<i32>) -> TradeAccountSnapshot {
    TradeAccountSnapshot {
            trd_env: 1,
            acc_id: id,
            trd_market_auth_list: authorities,
            acc_type: None,
            card_num: None,
            security_firm: firm,
            sim_acc_type: None,
            uni_card_num: None,
            acc_status: None,
            acc_role: None,
        jp_acc_type: Vec::new(),
        competition_acc_name: None,
    }
}

/// Trade-read double that only answers account discovery; every other read is
/// unsupported because the RFQ eligibility gate never calls it.
struct ComboQuoteAccounts {
    accounts: Vec<TradeAccountSnapshot>,
}

impl std::fmt::Debug for ComboQuoteAccounts {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ComboQuoteAccounts")
            .field("accounts", &self.accounts.len())
            .finish()
    }
}

impl TradeReadPort for ComboQuoteAccounts {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        Ok(self.accounts.clone())
    }

    fn read_funds(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<jftrade_integration_futu::TradeFundsSnapshot, TradeSessionError> {
        Err(unsupported())
    }

    fn read_cash_flows(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<jftrade_integration_futu::TradeCashFlowSnapshot>, TradeSessionError> {
        Err(unsupported())
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>, TradeSessionError> {
        Err(unsupported())
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>, TradeSessionError>
    {
        Err(unsupported())
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<jftrade_integration_futu::TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        Err(unsupported())
    }

    #[allow(clippy::too_many_arguments)]
    fn read_positions(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradePositionSnapshot>, TradeSessionError> {
        Err(unsupported())
    }

    fn read_orders(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeOrderSnapshot>, TradeSessionError> {
        Err(unsupported())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<jftrade_integration_futu::TradeFillSnapshot>, TradeSessionError> {
        Err(unsupported())
    }
}

fn unsupported() -> TradeSessionError {
    TradeSessionError::Unsupported("unused".to_owned())
}

#[derive(Clone)]
struct ComboQuoteFixture {
    metadata: serde_json::Value,
}

impl std::fmt::Debug for ComboQuoteFixture {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.debug_struct("ComboQuoteFixture").finish()
    }
}

impl PredictionComboQuotePort for ComboQuoteFixture {
    fn quote(&self, _payload: &serde_json::Value) -> Result<serde_json::Value, PredictionMarketReadError> {
        Ok(json!({"entries": [], "metadata": self.metadata}))
    }
}

/// Adapter double that always fails, so the upstream error mapping (Go turns a
/// broker failure into 502) can be asserted end to end.
#[derive(Clone, Debug)]
struct FailingComboQuoteFixture {
    message: String,
}

impl PredictionComboQuotePort for FailingComboQuoteFixture {
    fn quote(&self, _payload: &serde_json::Value) -> Result<serde_json::Value, PredictionMarketReadError> {
        Err(PredictionMarketReadError::Transport(self.message.clone()))
    }
}

/// Build the production market-data actions port with a real leased execution
/// store and an injected receive clock.
pub(super) fn combo_quote_port(
    metadata: serde_json::Value,
    received_at: &str,
    accounts: Vec<TradeAccountSnapshot>,
) -> (
    tempfile::TempDir,
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    ProductionMarketDataProviderActionsPort,
) {
    combo_quote_port_with_adapter(
        std::sync::Arc::new(ComboQuoteFixture { metadata }),
        received_at,
        accounts,
    )
}

/// Same production port, but the RFQ adapter always reports an upstream
/// failure. Go maps that failure to `502 BROKER_FEATURE_FAILED`, so the port
/// must not swallow or re-code it.
pub(super) fn failing_combo_quote_port(
    message: &str,
    received_at: &str,
    accounts: Vec<TradeAccountSnapshot>,
) -> (
    tempfile::TempDir,
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    ProductionMarketDataProviderActionsPort,
) {
    combo_quote_port_with_adapter(
        std::sync::Arc::new(FailingComboQuoteFixture {
            message: message.to_owned(),
        }),
        received_at,
        accounts,
    )
}

fn combo_quote_port_with_adapter(
    adapter: Arc<dyn PredictionComboQuotePort>,
    received_at: &str,
    accounts: Vec<TradeAccountSnapshot>,
) -> (
    tempfile::TempDir,
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    ProductionMarketDataProviderActionsPort,
) {
    let directory = tempfile::tempdir().expect("combo quote temp directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}").expect("write settings");
    crate::product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");
    let execution_path = settings_path
        .parent()
        .expect("settings parent")
        .join("execution-orders.db");
    let store = std::sync::Arc::new(
        jftrade_store_sqlite::ExecutionOrderStore::open_existing(
            &execution_path,
            jftrade_store_sqlite::EXECUTION_ORDERS_PRODUCTION_PROFILE,
        )
        .expect("open execution store"),
    );
    let runtime = std::sync::Arc::new(SharedTradeReadRuntime::default());
    runtime.set(
        Some(std::sync::Arc::new(ComboQuoteAccounts {
            accounts: accounts.into_iter().collect(),
        })),
        Some(true),
    );
    runtime.set_prediction_adapters(None, None, Some(adapter));
    let state = std::sync::Arc::new(ActiveProviderState::new(Some(
        MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let received_at = received_at.to_owned();
    let port = ProductionMarketDataProviderActionsPort::new(None)
        .with_trade_runtime(Some(runtime))
        .with_active_provider_state(Some(state))
        .with_prediction_quotes(Some(std::sync::Arc::clone(&store)));
    let port = port.with_prediction_quote_clock(std::sync::Arc::new(move || received_at.clone()));
    (directory, store, port)
}

pub(super) fn combo_quote_request(body: &[u8], query: &str) -> MarketDataProviderActionsRequest {
    MarketDataProviderActionsRequest {
        method: "POST".to_owned(),
        path: PREDICTION_COMBO_QUOTES_PATH.to_owned(),
        query: query.to_owned(),
        body: body.to_vec(),
    }
}

