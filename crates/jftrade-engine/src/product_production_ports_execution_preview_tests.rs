use super::*;

use jftrade_integration_futu::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeComboMaxTradeQuantityRequest,
    TradeComboMaxTradeQuantitySnapshot, TradeFillSnapshot, TradeFilter, TradeFundsSnapshot,
    TradeHeader, TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest,
    TradeMaxTradeQuantitySnapshot, TradeOrderFeeSnapshot, TradeOrderSnapshot,
    TradePositionSnapshot, TradeReadPort, TradeSecurity, TradeSessionError,
};
use serde_json::json;
use std::sync::{Arc, Mutex};

#[derive(Debug)]
struct PreviewTradeReader {
    calls: Arc<Mutex<Vec<TradeMaxTradeQuantityRequest>>>,
    fail: bool,
}

fn unsupported<T>() -> Result<T, TradeSessionError> {
    Err(TradeSessionError::Unsupported(
        "fixture unsupported".to_owned(),
    ))
}

impl TradeReadPort for PreviewTradeReader {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        unsupported()
    }

    fn read_funds(
        &self,
        _: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        unsupported()
    }

    fn read_cash_flows(
        &self,
        _: TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        unsupported()
    }

    fn read_order_fees(
        &self,
        _: TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        unsupported()
    }

    fn read_margin_ratios(
        &self,
        _: TradeHeader,
        _: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        unsupported()
    }

    fn read_max_trade_quantity(
        &self,
        request: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        self.calls
            .lock()
            .expect("preview calls")
            .push(request.clone());
        if self.fail {
            return unsupported();
        }
        Ok(TradeMaxTradeQuantitySnapshot {
            header: request.header,
            code: request.code,
            order_type: request.order_type,
            price: request.price,
            max_cash_buy: 10.0,
            max_cash_and_margin_buy: None,
            max_position_sell: 0.0,
            max_sell_short: None,
            max_buy_back: None,
            long_required_im: None,
            short_required_im: None,
            session: request.session,
        })
    }

    fn read_combo_max_trade_quantity(
        &self,
        _: TradeComboMaxTradeQuantityRequest,
    ) -> Result<TradeComboMaxTradeQuantitySnapshot, TradeSessionError> {
        unsupported()
    }

    fn read_positions(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        unsupported()
    }

    fn read_orders(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        unsupported()
    }

    fn read_fills(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        unsupported()
    }
}

fn execution_store() -> (
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    tempfile::TempDir,
) {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("execution-preview.db");
    let connection = rusqlite::Connection::open(&path).expect("create execution database");
    jftrade_store_sqlite::initialize_current(&connection, "execution-orders")
        .expect("initialize execution schema");
    drop(connection);
    (
        Arc::new(jftrade_store_sqlite::ExecutionOrderStore::open(&path).expect("open store")),
        directory,
    )
}

fn preview_port(
    state: Arc<ActiveProviderState>,
    reader: Option<Arc<PreviewTradeReader>>,
    logged_in: Option<bool>,
) -> ProductionExecutionPort {
    let (store, _directory) = execution_store();
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: logged_in,
        trade_read_port: reader.map(|value| value as Arc<dyn TradeReadPort>),
        trade_write_port: None,
        trade_runtime: None,
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
    }
}

fn buying_power_payload() -> Value {
    json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "orderKind": "single",
        "orderType": "LIMIT",
        "quantity": 2.0,
        "price": 100.0,
        "instrument": {
            "instrumentId": "US.AAPL",
            "productClass": "equity",
            "tradeMarket": "US"
        }
    })
}

#[test]
fn buying_power_requires_opend_trade_reader_and_forwards_max_quantity_query() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let calls = Arc::new(Mutex::new(Vec::new()));
    let reader = Arc::new(PreviewTradeReader {
        calls: Arc::clone(&calls),
        fail: false,
    });
    let port = preview_port(state, Some(reader), Some(true));

    let result = port
        .buying_power_preview(&buying_power_payload())
        .expect("preview");
    assert_eq!(result["allowed"], true);
    let calls = calls.lock().expect("preview calls");
    assert_eq!(calls.len(), 1);
    assert_eq!(calls[0].header.acc_id, 42);
    assert_eq!(calls[0].code, "AAPL");
    assert_eq!(calls[0].price, 100.0);
}

#[test]
fn buying_power_without_opend_cannot_project_allowed_true() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, false, false);
    let port = preview_port(state, None, None);
    let error = port
        .buying_power_preview(&buying_power_payload())
        .expect_err("OpenD absence must fail closed");
    assert!(matches!(error, ExecutionWritePortError::Unavailable(_)));
}

#[test]
fn buying_power_reader_failure_cannot_project_allowed_true() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let reader = Arc::new(PreviewTradeReader {
        calls: Arc::new(Mutex::new(Vec::new())),
        fail: true,
    });
    let port = preview_port(state, Some(reader), Some(true));
    let error = port
        .buying_power_preview(&buying_power_payload())
        .expect_err("reader failure must fail closed");
    assert!(matches!(error, ExecutionWritePortError::Unavailable(_)));
}

#[test]
fn event_parlay_preview_without_real_rfq_adapter_is_unavailable() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let port = preview_port(state, None, None);
    let payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "parlay-1",
        "orderKind": "event_parlay",
        "productClass": "event_contract",
        "rfqId": "rfq-1",
        "mvc": "US.MVC",
        "quoteExpiresAt": "2999-01-01T00:00:00Z",
        "amount": 10.0,
        "legs": [
            {"instrumentId": "US.EC.ONE", "side": "BUY", "ratio": 1, "predictionSide": "YES"},
            {"instrumentId": "US.EC.TWO", "side": "SELL", "ratio": 1, "predictionSide": "NO"}
        ]
    });
    let error = port
        .combo_preview(&payload)
        .expect_err("RFQ adapter is required");
    assert!(matches!(error, ExecutionWritePortError::Unavailable(_)));
}
