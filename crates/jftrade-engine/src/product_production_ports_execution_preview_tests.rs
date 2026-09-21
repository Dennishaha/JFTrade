use super::*;

use jftrade_integration_futu::{
    PredictionMarketReadError, PredictionMarketReadPort, TradeAccountSnapshot,
    TradeCashFlowSnapshot, TradeComboMaxTradeQuantityRequest,
    TradeComboMaxTradeQuantitySnapshot, TradeFillSnapshot, TradeFilter, TradeFundsSnapshot,
    TradeHeader, TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest,
    TradeMaxTradeQuantitySnapshot, TradeModifyOrderRequest, TradeOrderFeeSnapshot,
    TradeOrderSnapshot, TradePlaceComboOrderRequest, TradePlaceComboOrderResult,
    TradePlaceOrderRequest, TradePlaceOrderResult, TradePositionSnapshot, TradeReadPort,
    TradeSecurity, TradeSessionError, TradeSubscribeAccountsRequest, TradeUnlockRequest,
    TradeWritePort,
};
use serde_json::json;
use std::sync::{Arc, Mutex};

use crate::product::product_brokers_write_port::{
    BrokersWriteContext, BrokersWriteInput, BrokersWriteOperation, BrokersWritePort,
    BrokersWriteQuery,
};

/// Prediction-market read fixture keyed by the requested event-contract code.
///
/// `OpenDPredictionMarketReader::snapshot` returns every snapshot row that
/// OpenD sends, in wire order.  The engine's event-contract validation must
/// filter on the requested code before reading `status`, so the fixture keeps
/// an unrelated ACTIVE contract ahead of the requested one exactly like the
/// Go loopback server in
/// `pkg/futu/advanced_product_adapter_contracts_test.go:216`.
#[derive(Debug)]
struct PredictionReadFixture {
    entries: Vec<Value>,
    error: Option<String>,
}

impl PredictionMarketReadPort for PredictionReadFixture {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, PredictionMarketReadError> {
        if let Some(message) = self.error.as_ref() {
            return Err(PredictionMarketReadError::Transport(message.clone()));
        }
        Ok(json!({"entries": self.entries}))
    }
}

#[test]
fn buying_power_defaults_follow_settings_and_preserve_explicit_environment() {
    let request = execution_order_helpers::parse_product_rule_request(&json!({}), Some("REAL"))
        .expect("settings default");
    assert_eq!(request.trading_environment, "REAL");
    let request = execution_order_helpers::parse_product_rule_request(
        &json!({"env": "SIMULATE"}), Some("REAL"),
    ).expect("explicit environment");
    assert_eq!(request.trading_environment, "SIMULATE");
}

#[derive(Debug)]
struct PreviewTradeReader {
    calls: Arc<Mutex<Vec<TradeMaxTradeQuantityRequest>>>,
    fail: bool,
}

/// Combo-specific read fixture used by the option-combo lifecycle tests.
///
/// It answers `Trd_GetComboMaxTrdQtys` with a fixed account impact so the
/// preview can persist and be consumed by `place_combo`.
#[derive(Debug, Default)]
struct ComboPreviewTradeReader {
    calls: Arc<Mutex<Vec<TradeComboMaxTradeQuantityRequest>>>,
}

impl TradeReadPort for ComboPreviewTradeReader {
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
        _: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        unsupported()
    }

    fn read_combo_max_trade_quantity(
        &self,
        request: TradeComboMaxTradeQuantityRequest,
    ) -> Result<TradeComboMaxTradeQuantitySnapshot, TradeSessionError> {
        self.calls
            .lock()
            .expect("combo preview calls")
            .push(request.clone());
        Ok(TradeComboMaxTradeQuantitySnapshot {
            header: request.header,
            nlv_change: Some(-100.0),
            initial_margin_change: Some(-100.0),
            maintenance_margin_change: Some(-100.0),
            option_buy_power: Some(-100.0),
            max_withdraw_change: None,
            buying_power_decrease: Some(100.0),
        })
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

#[derive(Debug)]
struct OptionSpreadFixture;

impl jftrade_integration_futu::OptionStrategySpreadReadPort for OptionSpreadFixture {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionStrategySpreadQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategySpreadSnapshot,
        jftrade_integration_futu::OptionStrategySpreadQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.option_strategy, 4);
        assert_eq!(query.expire_time, "2026-07-17");
        Ok(jftrade_integration_futu::OptionStrategySpreadSnapshot {
            items: vec![jftrade_integration_futu::OptionStrategySpreadItem { spread: 10.0 }],
        })
    }
}

#[derive(Debug)]
struct OptionAnalysisFixture;

impl jftrade_integration_futu::OptionStrategyAnalysisReadPort for OptionAnalysisFixture {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionStrategyAnalysisQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategyAnalysisSnapshot,
        jftrade_integration_futu::OptionStrategyAnalysisQueryError,
    > {
        assert_eq!(query.multi_legs.len(), 2);
        Ok(jftrade_integration_futu::OptionStrategyAnalysisSnapshot {
            code: "AAPL260717C/P200".to_owned(),
            name: "AAPL vertical".to_owned(),
            option_strategy: 4,
            bid1: Some(9.5),
            ask1: Some(10.5),
            max_profit: Some(100.0),
            max_loss: Some(-100.0),
            breakeven_points: vec![210.0],
            prob_of_profit: Some(0.5),
            delta: Some(0.1),
            theta: Some(-0.2),
        })
    }
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
    preview_port_with_writer(state, reader, logged_in, None)
}

fn preview_port_with_writer(
    state: Arc<ActiveProviderState>,
    reader: Option<Arc<PreviewTradeReader>>,
    logged_in: Option<bool>,
    writer: Option<Arc<RecordingTradeWriter>>,
) -> ProductionExecutionPort {
    let (store, directory) = execution_store();
    // The port borrows only the store handle, so keep the temporary database
    // directory alive for the duration of the test process.
    let _ = directory.keep();
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: logged_in,
        trade_read_port: reader.map(|value| value as Arc<dyn TradeReadPort>),
        trade_write_port: writer.map(|value| value as Arc<dyn TradeWritePort>),
        trade_runtime: None,
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

#[derive(Debug, Default)]
struct RecordingTradeWriter {
    placed: Mutex<Vec<TradePlaceOrderRequest>>,
    /// Combo submissions recorded for preview-consume lifecycle tests.
    placed_combo: Mutex<Vec<TradePlaceComboOrderRequest>>,
    modified: Mutex<Vec<TradeModifyOrderRequest>>,
    unlocked: Mutex<Vec<TradeUnlockRequest>>,
    /// Server-issued `orderIDEx`; the adapter-bridge fixture answers
    /// `FT-9001`, while the default keeps the older preview tests on
    /// `EXT-9001`.
    order_id_ex: Mutex<Option<String>>,
}

impl TradeWritePort for RecordingTradeWriter {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        self.placed
            .lock()
            .expect("placed orders")
            .push(request.clone());
        Ok(TradePlaceOrderResult {
            header: request.header,
            order_id: Some(9001),
            order_id_ex: Some(
                self.order_id_ex
                    .lock()
                    .expect("order id ex")
                    .clone()
                    .unwrap_or_else(|| "EXT-9001".to_owned()),
            ),
        })
    }

    fn place_combo_order(
        &self,
        request: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError> {
        self.placed_combo
            .lock()
            .expect("placed combo orders")
            .push(request.clone());
        Ok(TradePlaceComboOrderResult {
            header: request.header,
            order_id_ex: Some(
                self.order_id_ex
                    .lock()
                    .expect("order id ex")
                    .clone()
                    .unwrap_or_else(|| "COMBO-9001".to_owned()),
            ),
        })
    }

    fn modify_order(
        &self,
        request: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        self.modified
            .lock()
            .expect("modified orders")
            .push(request.clone());
        Ok(TradePlaceOrderResult {
            header: request.header,
            order_id: Some(request.order_id),
            order_id_ex: None,
        })
    }

    fn unlock_trade(&self, request: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        self.unlocked
            .lock()
            .expect("unlocked trades")
            .push(request);
        Ok(())
    }

    fn subscribe_trade_accounts(
        &self,
        _request: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        Ok(())
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

fn event_parlay_payload() -> Value {
    json!({
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
    })
}

fn event_contract_entry(code: &str, status: i64) -> Value {
    json!({
        "code": {"market": 101, "code": code},
        "status": status,
    })
}

fn event_parlay_port_with_prediction(
    runtime: &Arc<SharedTradeReadRuntime>,
    reader: Option<Arc<dyn PredictionMarketReadPort>>,
) -> ProductionExecutionPort {
    runtime.set_prediction_adapters(reader, None, Some(Arc::new(ComboQuoteFixture)));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let (store, directory) = execution_store();
    let _ = directory.keep();
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(Arc::clone(runtime)),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

#[derive(Debug)]
struct ComboQuoteFixture;

impl jftrade_integration_futu::PredictionComboQuotePort for ComboQuoteFixture {
    fn quote(&self, _payload: &Value) -> Result<Value, PredictionMarketReadError> {
        Ok(json!({"entries": []}))
    }
}

#[test]
fn event_parlay_preview_rejects_inactive_contract_filtered_from_snapshot_list() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:216
    // TestFutuEventContractStatusBranches. OpenD may return protocol-compatible
    // snapshot rows for several contracts; the requested CLOSED contract must be
    // selected by code and rejected as a business denial, even when an unrelated
    // ACTIVE row precedes it in the response array.
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let port = event_parlay_port_with_prediction(
        &runtime,
        Some(Arc::new(PredictionReadFixture {
            entries: vec![
                event_contract_entry("UNRELATED", 2),
                event_contract_entry("EC.ONE", 3),
                event_contract_entry("EC.TWO", 2),
            ],
            error: None,
        })),
    );
    let error = port
        .combo_preview(&event_parlay_payload())
        .expect_err("closed contract must not preview");
    match error {
        ExecutionWritePortError::Failed { status, message, .. } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("not active"),
                "closed contract message = {message:?}"
            );
        }
        other => panic!("closed contract error = {other:?}"),
    }
}

#[test]
fn event_parlay_preview_surfaces_snapshot_transport_failure() {
    // Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:475
    // TestFutuComboProtocolTransportErrors. A dropped Qot_GetEventContractSnapshot
    // (3445) must fail closed as an infrastructure error instead of reaching the
    // preview persistence path.
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let port = event_parlay_port_with_prediction(
        &runtime,
        Some(Arc::new(PredictionReadFixture {
            entries: Vec::new(),
            error: Some("request timed out".to_owned()),
        })),
    );
    let error = port
        .combo_preview(&event_parlay_payload())
        .expect_err("snapshot transport failure must fail closed");
    assert!(matches!(error, ExecutionWritePortError::Unavailable(_)));
}

#[test]
fn submit_order_uses_client_order_id_as_the_opend_remark() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:834
    // TestSubmitOrderPlacesViaOpenD. Go's SubmitOrder forwards
    // ClientOrderID as the OpenD remark so reconciliation can correlate the
    // broker order back to the local reservation.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = preview_port_with_writer(state, None, Some(true), Some(Arc::clone(&writer)));
    let payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "execution-test-order",
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5,
        "timeInForce": "GTC"
    });
    let result = port.place_order(&payload).expect("place order");
    assert_eq!(result["brokerOrderId"], "9001");
    let placed = writer.placed.lock().expect("placed orders");
    assert_eq!(placed.len(), 1);
    assert_eq!(placed[0].code, "00700");
    assert_eq!(placed[0].quantity, 100.0);
    assert_eq!(placed[0].price, Some(320.5));
    assert_eq!(placed[0].remark.as_deref(), Some("execution-test-order"));
}

#[test]
fn cancel_order_uses_modify_order_cancel_operation() {
    // Parity: go:452dea11:pkg/futu/exchange_test.go:885
    // TestCancelOrdersUsesModifyOrderCancel. OpenD has no dedicated cancel
    // RPC; the write adapter must use Trd_ModifyOrder with cancel op=1.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = preview_port_with_writer(state, None, Some(true), Some(Arc::clone(&writer)));
    let payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "cancel-test-order",
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5
    });
    let placed = port.place_order(&payload).expect("place order");
    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    port.cancel_order(&internal_id).expect("cancel order");
    let modified = writer.modified.lock().expect("modified orders");
    assert_eq!(modified.len(), 1);
    assert_eq!(modified[0].order_id, 9001);
    // OpenD's ModifyOrderOp_Cancel enum value is 2.
    assert_eq!(modified[0].operation, 2);
}

fn cancel_contract_port(writer: Arc<dyn TradeWritePort>) -> ProductionExecutionPort {
    write_port_with_writer(writer)
}

fn cancel_contract_payload(client_order_id: &str) -> Value {
    json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": client_order_id,
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5
    })
}

// Parity: go:452dea11:internal/app/apiserver/servercore/trading_order_cancellation_contracts_test.go:98 TestTradingOrderCancellationRejectsInvalidPersistedOrders
#[test]
fn cancel_rejects_missing_terminal_and_unidentified_persisted_orders() {
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);

    let missing = port.cancel_order("missing-order").expect_err("missing order");
    assert!(
        matches!(
            missing,
            ExecutionWritePortError::Failed { status: 404, ref code, .. }
                if code == "EXECUTION_ORDER_NOT_FOUND"
        ),
        "missing order error = {missing:?}"
    );

    let placed = port
        .place_order(&cancel_contract_payload("cancel-contract-terminal"))
        .expect("place terminal order");
    let terminal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    let mut terminal = port
        .store
        .get_order(&terminal_id)
        .expect("read order")
        .expect("stored order");
    terminal.status = "FILLED".to_owned();
    port.store
        .save_order(terminal, "2026-09-08T00:00:00Z")
        .expect("persist terminal order");
    let terminal_error = port
        .cancel_order(&terminal_id)
        .expect_err("terminal order cancel");
    assert!(
        matches!(
            terminal_error,
            ExecutionWritePortError::Failed { status: 400, ref code, .. }
                if code == "EXECUTION_ORDER_TERMINAL"
        ),
        "terminal order error = {terminal_error:?}"
    );

    let placed = port
        .place_order(&cancel_contract_payload("cancel-contract-no-broker-id"))
        .expect("place unidentified order");
    let unidentified_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    let mut unidentified = port
        .store
        .get_order(&unidentified_id)
        .expect("read order")
        .expect("stored order");
    unidentified.broker_order_id = None;
    unidentified.broker_order_id_ex = None;
    port.store
        .save_order(unidentified, "2026-09-08T00:00:01Z")
        .expect("persist unidentified order");
    let unidentified_error = port
        .cancel_order(&unidentified_id)
        .expect_err("unidentified order cancel");
    assert!(
        matches!(
            unidentified_error,
            ExecutionWritePortError::Failed { status: 400, ref code, .. }
                if code == "BROKER_ORDER_ID_MISSING"
        ),
        "unidentified order error = {unidentified_error:?}"
    );

    assert!(
        writer.modified.lock().expect("modified orders").is_empty(),
        "rejected cancellations must not reach the broker"
    );
}

/// Write fixture whose placement succeeds but whose cancel acknowledgement
/// fails at the broker boundary.
#[derive(Debug, Default)]
struct CancelFailingWriter {
    modified: Mutex<usize>,
}

impl CancelFailingWriter {
    fn modified_calls(&self) -> usize {
        *self.modified.lock().expect("modified calls")
    }
}

impl TradeWritePort for CancelFailingWriter {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        Ok(TradePlaceOrderResult {
            header: request.header,
            order_id: Some(9001),
            order_id_ex: Some("EXT-9001".to_owned()),
        })
    }

    fn place_combo_order(
        &self,
        _: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError> {
        unsupported()
    }

    fn modify_order(
        &self,
        _: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        *self.modified.lock().expect("modified calls") += 1;
        Err(disconnecting_error())
    }

    fn unlock_trade(&self, _: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        unsupported()
    }

    fn subscribe_trade_accounts(
        &self,
        _: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        unsupported()
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercore/trading_order_cancellation_contracts_test.go:134 TestTradingOrderCancellationPropagatesBrokerFailuresAndPersistsAcceptedCancel
#[test]
fn accepted_cancel_persists_and_broker_failure_never_advertises_a_cancel() {
    let accepted_writer = Arc::new(RecordingTradeWriter::default());
    let accepted_port =
        cancel_contract_port(Arc::clone(&accepted_writer) as Arc<dyn TradeWritePort>);
    let placed = accepted_port
        .place_order(&cancel_contract_payload("cancel-contract-accepted"))
        .expect("place order");
    let accepted_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    let accepted = accepted_port
        .cancel_order(&accepted_id)
        .expect("accepted cancel");
    assert_eq!(accepted["status"], "CANCEL_SUBMITTED");
    assert_eq!(
        accepted_port
            .store
            .get_order(&accepted_id)
            .expect("read order")
            .expect("stored order")
            .status,
        "CANCEL_SUBMITTED"
    );
    assert_eq!(
        accepted_writer.modified.lock().expect("modified orders").len(),
        1
    );

    let failing_writer = Arc::new(CancelFailingWriter::default());
    let failing_port =
        cancel_contract_port(Arc::clone(&failing_writer) as Arc<dyn TradeWritePort>);
    let placed = failing_port
        .place_order(&cancel_contract_payload("cancel-contract-broker-failure"))
        .expect("place order");
    let rejected_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    let failure = failing_port
        .cancel_order(&rejected_id)
        .expect_err("broker cancel failure");
    assert!(
        matches!(
            failure,
            ExecutionWritePortError::Failed { status: 502, ref code, .. }
                if code == "BROKER_UNAVAILABLE"
        ),
        "broker cancel failure = {failure:?}"
    );
    assert_eq!(failing_writer.modified_calls(), 1);
    let stored = failing_port
        .store
        .get_order(&rejected_id)
        .expect("read order")
        .expect("stored order");
    // Rust commits the CANCEL_SUBMITTED fence before the broker call and
    // fails closed to UNKNOWN when the acknowledgement is lost, so a later
    // cancel can never silently reuse the same identity.
    assert_eq!(stored.status, "UNKNOWN");
    assert!(stored.last_error.is_some(), "unknown order error = {stored:?}");
    let repeat = failing_port
        .cancel_order(&rejected_id)
        .expect_err("repeat cancel on unknown order");
    assert!(
        matches!(
            repeat,
            ExecutionWritePortError::Failed { status: 400, ref code, .. }
                if code == "EXECUTION_ORDER_TERMINAL"
        ),
        "repeat cancel error = {repeat:?}"
    );
}

#[test]
fn broker_unlock_route_forwards_password_md5_and_unlock_flag_to_opend() {
    // Parity: go:452dea11:pkg/futu/adapter_bridge_test.go:709
    // TestBrokerAdapterUnlockTradeBridge. The adapter's UnlockTrade forwards
    // unlock=true and the caller's password MD5 to Trd_UnlockTrade once.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = preview_port_with_writer(state, None, Some(true), Some(Arc::clone(&writer)));
    let result = BrokersWritePort::mutate(
        &port,
        &BrokersWriteInput {
            operation: BrokersWriteOperation::Unlock,
            query: BrokersWriteQuery {
                broker_id: "futu".to_owned(),
                account_id: "42".to_owned(),
                trading_environment: "REAL".to_owned(),
                market: "HK".to_owned(),
            },
            payload: json!({"unlock": true, "passwordMd5": "dummy-md5"}),
            context: BrokersWriteContext::Normal,
        },
    )
    .expect("unlock route");
    assert_eq!(result["unlocked"], true);
    let unlocked = writer.unlocked.lock().expect("unlocked trades");
    assert_eq!(unlocked.len(), 1);
    assert!(unlocked[0].unlock);
    assert_eq!(unlocked[0].password_md5.as_deref(), Some("dummy-md5"));
    // The bridge test's caller sends no security firm, so it must stay unset
    // rather than being defaulted by the route.
    assert_eq!(unlocked[0].security_firm, None);
}

#[test]
fn broker_adapter_place_and_cancel_keep_server_order_identity_and_submitted_status() {
    // Parity: go:452dea11:pkg/futu/adapter_bridge_test.go:19
    // TestBrokerAdapterDiscoverAccountsAndTradingBridge. Go's adapter places a
    // limit order, projects the server's numeric and extended order ids with
    // Status=SUBMITTED, forwards the client order id as the OpenD remark, then
    // cancels through the same bridge with exactly one write call each.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let writer = Arc::new(RecordingTradeWriter::default());
    *writer.order_id_ex.lock().expect("order id ex") = Some("FT-9001".to_owned());
    let port = preview_port_with_writer(state, None, Some(true), Some(Arc::clone(&writer)));
    let payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "adapter-order-9001",
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5,
        "timeInForce": "GTC"
    });
    let placed = port.place_order(&payload).expect("place order");
    assert_eq!(placed["brokerOrderId"], "9001");
    assert_eq!(placed["brokerOrderIdEx"], "FT-9001");
    assert_eq!(placed["status"], "SUBMITTED");
    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    {
        let requests = writer.placed.lock().expect("placed orders");
        assert_eq!(requests.len(), 1, "exactly one Trd_PlaceOrder call");
        assert_eq!(requests[0].code, "00700");
        assert_eq!(requests[0].remark.as_deref(), Some("adapter-order-9001"));
    }
    port.cancel_order(&internal_id).expect("cancel order");
    let modified = writer.modified.lock().expect("modified orders");
    assert_eq!(modified.len(), 1, "exactly one Trd_ModifyOrder call");
    assert_eq!(modified[0].order_id, 9001);
    assert_eq!(modified[0].order_id_ex.as_deref(), Some("FT-9001"));
}

#[test]
fn product_rule_denials_return_the_go_reason_code_matrix() {
    // Parity: go:452dea11:pkg/futu/adapter_advanced_protocol_test.go:365
    // TestFutuComboAdapterProductRulesAndValidationFailures. Go's
    // ValidateProductOrder returns an allowed=false result with a stable
    // reason code instead of an error, so the public order-preview route can
    // surface the denial. Rust owns that decision in `product_rule_rejection`
    // and only reaches the reader after the request passes it.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    let port = preview_port(
        state,
        Some(Arc::new(PreviewTradeReader {
            calls: Arc::new(Mutex::new(Vec::new())),
            fail: false,
        })),
        Some(true),
    );

    let base = |overrides: Value| {
        let mut payload = buying_power_payload();
        let object = payload.as_object_mut().expect("object");
        for (key, value) in overrides.as_object().expect("object").clone() {
            object.insert(key, value);
        }
        payload
    };

    let cases = [
        (
            "event single without an event-contract instrument",
            base(json!({
                "orderKind": "event_single",
                "productClass": "equity",
                "instrument": {"instrumentId": "US.AAPL", "productClass": "equity", "tradeMarket": "US"}
            })),
            "PRODUCT_MISMATCH",
        ),
        (
            "event contract outside the US market",
            base(json!({
                "orderKind": "event_single",
                "market": "HK",
                "productClass": "event_contract",
                "amount": 20.0,
                "price": 0.6,
                "instrument": {"instrumentId": "HK.EVENT", "productClass": "event_contract", "tradeMarket": "HK"}
            })),
            "MARKET_MISMATCH",
        ),
        (
            "non-positive event amount",
            base(json!({
                "orderKind": "event_single",
                "productClass": "event_contract",
                "amount": 0.0,
                "price": 0.6,
                "instrument": {"instrumentId": "US.EVENT", "productClass": "event_contract", "tradeMarket": "US"}
            })),
            "INVALID_AMOUNT",
        ),
        (
            "event price outside 0.01..0.99",
            base(json!({
                "orderKind": "event_single",
                "productClass": "event_contract",
                "amount": 20.0,
                "price": 1.5,
                "instrument": {"instrumentId": "US.EVENT", "productClass": "event_contract", "tradeMarket": "US"}
            })),
            "INVALID_PRICE",
        ),
        (
            "non-limit event order",
            base(json!({
                "orderKind": "event_single",
                "productClass": "event_contract",
                "amount": 20.0,
                "price": 0.6,
                "orderType": "MARKET",
                "instrument": {"instrumentId": "US.EVENT", "productClass": "event_contract", "tradeMarket": "US"}
            })),
            "INVALID_ORDER_TYPE",
        ),
        (
            "fractional derivative quantity",
            base(json!({
                "quantity": 1.5,
                "instrument": {"instrumentId": "US.AAPL260918C00100000", "productClass": "option", "tradeMarket": "US"}
            })),
            "INVALID_CONTRACT_QUANTITY",
        ),
        (
            "missing derivative quantity",
            base(json!({
                "quantity": null,
                "instrument": {"instrumentId": "US.AAPL260918C00100000", "productClass": "option", "tradeMarket": "US"}
            })),
            "INVALID_CONTRACT_QUANTITY",
        ),
        (
            "option order with an extended-hours session",
            base(json!({
                "session": "RTH",
                "instrument": {"instrumentId": "US.AAPL260918C00100000", "productClass": "option", "tradeMarket": "US"}
            })),
            "INVALID_SESSION",
        ),
    ];
    for (name, payload, want) in cases {
        let value = port
            .buying_power_preview(&payload)
            .unwrap_or_else(|error| panic!("{name}: {error:?}"));
        assert_eq!(value["allowed"], false, "{name}");
        assert_eq!(value["reasonCode"], want, "{name}: {value}");
        assert!(
            value["reason"]
                .as_str()
                .is_some_and(|reason| !reason.trim().is_empty()),
            "{name}: {value}"
        );
    }

    // An ordinary equity query is not a product-rule denial: it must reach the
    // broker-owned max-trade-quantity read instead of short-circuiting on the
    // local rule. The recorded call is the evidence that the reader answered.
    let calls = Arc::new(Mutex::new(Vec::new()));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, true);
    let port = preview_port(
        state,
        Some(Arc::new(PreviewTradeReader {
            calls: Arc::clone(&calls),
            fail: false,
        })),
        Some(true),
    );
    let allowed = port
        .buying_power_preview(&buying_power_payload())
        .expect("ordinary buying power");
    assert_eq!(allowed["allowed"], true, "{allowed}");
    assert_eq!(
        calls.lock().expect("preview calls").len(),
        1,
        "an allowed product rule must still consult the OpenD reader"
    );
}

/// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:296
/// TestFutuComboAdapterOptionAndEventLifecycle.
///
/// Go drives one loopback server through the option-combo lifecycle:
/// `PreviewComboOrder` (3258 strategy spread + combo max quantity), then
/// `PlaceComboOrder`, then `CancelComboOrder`. Rust splits the same wire calls
/// across typed ports, so this test keeps the engine-level lifecycle live:
/// preview persists the preview id, place consumes it exactly once and writes
/// `SUBMITTED` with the server-issued order id, and cancel issues exactly one
/// `Trd_ModifyOrder(operation=2)`.
#[test]
fn option_combo_preview_place_and_cancel_keep_server_identity() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_strategy_spread(Some(Arc::new(OptionSpreadFixture)));
    runtime.set_option_strategy_analysis(Some(Arc::new(OptionAnalysisFixture)));
    let combo_calls = Arc::new(Mutex::new(Vec::new()));
    let reader = Arc::new(ComboPreviewTradeReader {
        calls: Arc::clone(&combo_calls),
    });
    let writer = Arc::new(RecordingTradeWriter::default());
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    // `reader()` and `writer()` prefer the runtime snapshot whenever a
    // trade runtime is installed, so the lifecycle fixture must be installed
    // through the same owner the production composition uses.
    runtime.set(
        Some(reader as Arc<dyn TradeReadPort>),
        Some(true),
    );
    runtime.set_writer(Some(Arc::clone(&writer) as Arc<dyn TradeWritePort>));
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let port = ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(Arc::clone(&runtime)),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    };

    let payload = json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "option-combo-1",
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10.0,
        "legs": [
            {"instrumentId": "US.AAPL260717C00200000", "productClass": "option", "side": "BUY", "ratio": 1},
            {"instrumentId": "US.AAPL260717C00210000", "productClass": "option", "side": "SELL", "ratio": 1}
        ]
    });

    let preview = port.combo_preview(&payload).expect("combo preview");
    assert_eq!(preview["allowed"], true, "{preview}");
    assert_eq!(preview["legs"].as_array().expect("preview legs").len(), 2);
    assert_eq!(preview["optionAnalysis"]["strategy"], "vertical");
    let maximum_calls = combo_calls.lock().expect("combo calls");
    assert_eq!(maximum_calls.len(), 1, "preview reads combo max quantity once");
    assert_eq!(maximum_calls[0].combo_legs.len(), 2);
    assert_eq!(maximum_calls[0].quantity, 1.0);
    drop(maximum_calls);
    let preview_id = preview["previewId"]
        .as_str()
        .expect("preview id")
        .to_owned();

    let mut placed_payload = payload.clone();
    placed_payload["previewId"] = json!(preview_id);
    let placed = port.place_combo(&placed_payload).expect("combo place");
    assert_eq!(placed["status"], "SUBMITTED", "{placed}");
    assert_eq!(placed["brokerOrderIdEx"], "COMBO-9001");
    let requests = writer.placed_combo.lock().expect("placed combos");
    assert_eq!(requests.len(), 1, "exactly one Trd_PlaceComboOrder call");
    assert_eq!(requests[0].combo_legs.len(), 2);
    assert_eq!(requests[0].combo_legs[0].code, "AAPL260717C00200000");
    assert_eq!(requests[0].combo_legs[1].side, Some(2));
    assert_eq!(requests[0].quantity, 1.0);
    drop(requests);

    // Placing the same client identity again must replay the stored order
    // instead of issuing a second combo submission.
    let replayed = port
        .place_combo(&placed_payload)
        .expect("idempotent combo place");
    assert_eq!(replayed["status"], "SUBMITTED");
    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        1,
        "a consumed preview must not submit a second combo"
    );

    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal combo order id");
    port.cancel_order(internal_id).expect("combo cancel");
    let modified = writer.modified.lock().expect("modified combos");
    assert_eq!(modified.len(), 1, "exactly one Trd_ModifyOrder call");
    assert_eq!(modified[0].operation, 2, "cancel uses Trd_ModifyOrder");
    drop(modified);
}

/// Parity: go:452dea11:pkg/futu/adapter_combo_transport_test.go:149
/// TestFutuOptionComboPreviewNormalizesAllAccountImpacts.
///
/// Go's loopback fixture answers `Trd_GetComboMaxTrdQtys` with six distinct
/// account-impact fields and an analysis whose `MaxProfit=9_999_999` is the
/// OpenD sentinel for an unbounded payoff. The public contract
/// (`broker.OptionComboAnalysis`, frozen in `docs/swagger/swagger.json`) carries
/// `maxProfitUnlimited`/`maxLossUnlimited`, and the console renders them as
/// "无限". Rust therefore has to translate the sentinel into the boolean flag
/// *and* drop the placeholder number: Go's `optionComboBound` returns
/// `(nil, true)` once the value reaches 9_999_999, so a preview that reported
/// both `maxProfit: 9999999` and `maxProfitUnlimited: true` would leak the
/// sentinel and let the console show a bogus finite profit.
#[test]
fn option_combo_preview_normalizes_every_account_impact_and_unlimited_bounds() {
    /// Reader that answers the Go fixture's six-field combo buying power and an
    /// analysis with an unlimited max profit and a finite max loss.
    #[derive(Debug)]
    struct ComboImpactReader;

    impl TradeReadPort for ComboImpactReader {
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
            _: TradeMaxTradeQuantityRequest,
        ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
            unsupported()
        }

        fn read_combo_max_trade_quantity(
            &self,
            request: TradeComboMaxTradeQuantityRequest,
        ) -> Result<TradeComboMaxTradeQuantitySnapshot, TradeSessionError> {
            Ok(TradeComboMaxTradeQuantitySnapshot {
                header: request.header,
                nlv_change: Some(101.0),
                initial_margin_change: Some(12.0),
                maintenance_margin_change: Some(8.0),
                option_buy_power: Some(500.0),
                max_withdraw_change: Some(-20.0),
                buying_power_decrease: Some(30.0),
            })
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

    /// Analysis reader returning Go's sentinel profit and a real loss bound.
    #[derive(Debug)]
    struct UnlimitedAnalysisReader;

    impl jftrade_integration_futu::OptionStrategyAnalysisReadPort for UnlimitedAnalysisReader {
        fn query(
            &self,
            _: &jftrade_integration_futu::OptionStrategyAnalysisQuery,
        ) -> Result<
            jftrade_integration_futu::OptionStrategyAnalysisSnapshot,
            jftrade_integration_futu::OptionStrategyAnalysisQueryError,
        > {
            Ok(jftrade_integration_futu::OptionStrategyAnalysisSnapshot {
                code: "AAPL260717C/P200".to_owned(),
                name: "AAPL vertical".to_owned(),
                option_strategy: 4,
                bid1: Some(1.1),
                ask1: Some(1.3),
                max_profit: Some(9_999_999.0),
                max_loss: Some(250.0),
                breakeven_points: Vec::new(),
                prob_of_profit: None,
                delta: None,
                theta: None,
            })
        }
    }

    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_strategy_spread(Some(Arc::new(OptionSpreadFixture)));
    runtime.set_option_strategy_analysis(Some(Arc::new(UnlimitedAnalysisReader)));
    runtime.set(
        Some(Arc::new(ComboImpactReader) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    // `ensure_futu_runtime` gates every combo read on OpenD readiness, so the
    // fixture has to publish the same ready state the lifecycle owner would.
    state.set_readiness(false, true, false);
    let port = ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(Arc::clone(&runtime)),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    };

    let payload = json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "combo-impacts",
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10.0,
        "legs": [
            {"instrumentId": "US.AAPL260717C00200000", "productClass": "option", "side": "BUY", "ratio": 1},
            {"instrumentId": "US.AAPL260717C00210000", "productClass": "option", "side": "SELL", "ratio": 1}
        ]
    });
    let preview = port.combo_preview(&payload).expect("combo preview");

    let impact = &preview["accountImpact"];
    for (field, expected) in [
        ("nlvChange", 101.0),
        ("initialMarginChange", 12.0),
        ("maintenanceMarginChange", 8.0),
        ("optionBuyingPower", 500.0),
        ("maxWithdrawalChange", -20.0),
        ("buyingPowerDecrease", 30.0),
    ] {
        assert_eq!(
            impact[field].as_f64(),
            Some(expected),
            "accountImpact.{field} must be normalized: {preview}"
        );
    }
    assert_eq!(
        preview["buyingPowerImpact"].as_f64(),
        Some(30.0),
        "the legacy buyingPowerImpact mirrors buyingPowerDecrease: {preview}"
    );

    let analysis = &preview["optionAnalysis"];
    assert_eq!(analysis["strategy"], "vertical");
    assert_eq!(
        analysis["maxProfitUnlimited"], true,
        "the OpenD 9999999 sentinel must surface as an unlimited bound: {preview}"
    );
    assert!(
        analysis.get("maxProfit").is_none_or(Value::is_null),
        "the sentinel placeholder must not be published as a finite profit: {preview}"
    );
    assert_eq!(
        analysis["maxLoss"].as_f64(),
        Some(250.0),
        "a real loss bound stays finite: {preview}"
    );
    assert!(
        analysis["maxLossUnlimited"].as_bool() != Some(true),
        "a finite loss must not be reported as unlimited: {preview}"
    );
}

/// Builds the option-combo fixture the Go legality/transport cases drive. The
/// account is a simulated HK cash account whose OpenD market authorization is
/// US, the spread list contains exactly `10.0`, and both legs are US options.
fn option_combo_fixture_payload() -> Value {
    json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "combo-transport",
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10.0,
        "legs": [
            {"instrumentId": "US.AAPL260717C00200000", "productClass": "option", "side": "BUY", "ratio": 1, "quantity": 2},
            {"instrumentId": "US.AAPL260717C00210000", "productClass": "option", "side": "SELL", "ratio": 1, "quantity": 2}
        ]
    })
}

/// Spread reader whose legality answer is scripted per test. `None` models the
/// Go loopback server dropping the `Qot_GetOptionStrategySpread` (3258) response.
#[derive(Debug)]
struct ScriptedSpreadReader {
    items: Mutex<Option<Vec<f64>>>,
    fail: bool,
}

impl ScriptedSpreadReader {
    fn with_spreads(spreads: impl IntoIterator<Item = f64>) -> Self {
        Self {
            items: Mutex::new(Some(spreads.into_iter().collect())),
            fail: false,
        }
    }
}

impl jftrade_integration_futu::OptionStrategySpreadReadPort for ScriptedSpreadReader {
    fn query(
        &self,
        _: &jftrade_integration_futu::OptionStrategySpreadQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategySpreadSnapshot,
        jftrade_integration_futu::OptionStrategySpreadQueryError,
    > {
        if self.fail {
            // A dropped 3258 response surfaces as a session/transport failure;
            // model it with the closest typed variant the reader exposes.
            return Err(
                jftrade_integration_futu::OptionStrategySpreadQueryError::InvalidResponse(
                    "Qot_GetOptionStrategySpread response was dropped".to_owned(),
                ),
            );
        }
        Ok(jftrade_integration_futu::OptionStrategySpreadSnapshot {
            items: self
                .items
                .lock()
                .expect("scripted spreads")
                .clone()
                .expect("scripted spreads")
                .into_iter()
                .map(
                    |spread| jftrade_integration_futu::OptionStrategySpreadItem { spread },
                )
                .collect(),
        })
    }
}

/// Non-spread strategy reader modelling `Qot_GetOptionStrategy` (3256).
#[derive(Debug)]
struct ScriptedStrategyReader {
    legs: Mutex<Option<Vec<jftrade_integration_futu::OptionStrategyLeg>>>,
    fail: bool,
}

impl jftrade_integration_futu::OptionStrategyReadPort for ScriptedStrategyReader {
    fn query(
        &self,
        _: &jftrade_integration_futu::OptionStrategyQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategySnapshot,
        jftrade_integration_futu::OptionStrategyQueryError,
    > {
        if self.fail {
            return Err(jftrade_integration_futu::OptionStrategyQueryError::InvalidResponse(
                "Qot_GetOptionStrategy response was dropped".to_owned(),
            ));
        }
        // Go's fixture answers `S2C.StrategyList` with a single entry whose
        // `MultiLegs` is the whole requested combination, so the snapshot has
        // exactly one item carrying every scripted leg.
        let multi_legs = self
            .legs
            .lock()
            .expect("scripted legs")
            .clone()
            .expect("scripted legs");
        Ok(jftrade_integration_futu::OptionStrategySnapshot {
            items: vec![jftrade_integration_futu::OptionStrategyItem {
                code: "combo".to_owned(),
                name: "combo".to_owned(),
                option_strategy: 6,
                stock_owner: jftrade_integration_futu::OptionStrategySecurity {
                    market: "US".to_owned(),
                    code: "AAPL".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL".to_owned(),
                },
                multi_legs,
            }],
        })
    }
}

/// Builds a combo preview port for a scripted spread/strategy legality pair.
fn option_combo_port(
    spread: Arc<dyn jftrade_integration_futu::OptionStrategySpreadReadPort>,
    strategy: Arc<dyn jftrade_integration_futu::OptionStrategyReadPort>,
) -> ProductionExecutionPort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_strategy_spread(Some(spread));
    runtime.set_option_strategy(Some(strategy));
    runtime.set_option_strategy_analysis(Some(Arc::new(OptionAnalysisFixture)));
    runtime.set(
        Some(Arc::new(ComboPreviewTradeReader::default()) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let (store, directory) = execution_store();
    let _ = directory.keep();
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

/// Parity: go:452dea11:pkg/futu/adapter_combo_transport_test.go:16
/// TestFutuOptionComboPreviewLegalityAndTransportFailures.
///
/// Go drives one loopback server through the combo preview: malformed legs and
/// an unparsable underlying fail before any RPC, a missing/illegal spread is a
/// business rejection, an unknown account fails resolution, and a dropped
/// spread/max-quantity/analysis response must surface as an error instead of a
/// manufactured `{allowed:true}` preview.
#[test]
fn option_combo_preview_rejects_invalid_legality_and_hidden_transport_failures() {
    let port = || {
        option_combo_port(
            Arc::new(ScriptedSpreadReader::with_spreads([10.0])),
            Arc::new(ScriptedStrategyReader {
                legs: Mutex::new(Some(Vec::new())),
                fail: false,
            }),
        )
    };

    // Invalid leg symbol: `US.BAD` cannot be split into a supported market, so
    // the intent is rejected before any OpenD call.
    let mut bad_leg = option_combo_fixture_payload();
    bad_leg["legs"][0]["instrumentId"] = json!("BAD");
    let error = port()
        .combo_preview(&bad_leg)
        .expect_err("an invalid combo leg symbol must fail closed");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "invalid leg symbol error = {error:?}"
    );

    // Invalid side: Go's `normalizeExecutionSide` accepts BUY/SELL only.
    let mut bad_side = option_combo_fixture_payload();
    bad_side["legs"][0]["side"] = json!("HOLD");
    let error = port()
        .combo_preview(&bad_side)
        .expect_err("an invalid combo leg side must fail closed");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "invalid leg side error = {error:?}"
    );

    // Invalid underlying: `futuSecurityFromSymbol` fails on an unqualified id.
    let mut bad_owner = option_combo_fixture_payload();
    bad_owner["underlyingInstrumentId"] = json!("BAD");
    let error = port()
        .combo_preview(&bad_owner)
        .expect_err("an invalid underlying must fail closed");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "invalid underlying error = {error:?}"
    );

    // Missing spread: vertical requires a positive spread. Go reports the
    // business reason; the wire contract for every combo rejection is a 400
    // BAD_REQUEST request error, so the status/code pair is what must match.
    let mut no_spread = option_combo_fixture_payload();
    no_spread.as_object_mut().expect("object").remove("spread");
    let error = port()
        .combo_preview(&no_spread)
        .expect_err("a strategy that requires a spread must reject its absence");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "missing spread error = {error:?}"
    );

    // Illegal spread: 5.0 is absent from the OpenD spread list, which Go
    // rejects with ILLEGAL_OPTION_SPREAD and must never turn into a preview.
    let mut illegal = option_combo_fixture_payload();
    illegal["spread"] = json!(5.0);
    let error = port()
        .combo_preview(&illegal)
        .expect_err("a spread outside the OpenD list must fail closed");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "illegal spread error = {error:?}"
    );

    // Dropped 3258 response: the legality read fails and the transport error
    // must be surfaced instead of being swallowed into an allowed preview.
    let dropped = option_combo_port(
        Arc::new(ScriptedSpreadReader {
            items: Mutex::new(Some(vec![10.0])),
            fail: true,
        }),
        Arc::new(ScriptedStrategyReader {
            legs: Mutex::new(Some(Vec::new())),
            fail: false,
        }),
    );
    dropped
        .combo_preview(&option_combo_fixture_payload())
        .expect_err("a dropped spread-legality response must not be hidden");
}

/// Parity: go:452dea11:pkg/futu/adapter_combo_transport_test.go:80
/// TestFutuOptionComboNonSpreadOpenDLegalityBranches.
///
/// A `straddle` has no spread, so Go validates the selected contracts through
/// `Qot_GetOptionStrategy` (3256): a `strategyList` without the requested legs
/// yields ILLEGAL_OPTION_COMBINATION, a matching `multiLegs` entry is accepted,
/// and a dropped 3256 response propagates as an error.
#[test]
fn option_combo_preview_validates_non_spread_legality_against_opend_strategies() {
    let leg = |code: &str, side: i32| jftrade_integration_futu::OptionStrategyLeg {
        security: jftrade_integration_futu::OptionStrategySecurity {
            market: "US".to_owned(),
            code: code.to_owned(),
            quote_market: "US".to_owned(),
            trade_market: "US".to_owned(),
            instrument_id: format!("US.{code}"),
        },
        side: Some(side),
        qty_ratio: Some(1.0),
        position_id: None,
        pred_side: None,
    };
    // The Go fixture answers `StrategyList` with `MultiLegs: legs`, i.e. the
    // exact two-leg combination the caller requested.
    let matching_legs = || {
        vec![
            leg("AAPL260717C00200000", 1),
            leg("AAPL260717C00210000", 2),
        ]
    };

    let payload = || {
        let mut value = option_combo_fixture_payload();
        value["optionStrategy"] = json!("straddle");
        value.as_object_mut().expect("object").remove("spread");
        value
    };

    // An empty strategy list cannot contain the requested combination.
    let mismatch = option_combo_port(
        Arc::new(ScriptedSpreadReader::with_spreads([10.0])),
        Arc::new(ScriptedStrategyReader {
            legs: Mutex::new(Some(Vec::new())),
            fail: false,
        }),
    );
    let error = mismatch
        .combo_preview(&payload())
        .expect_err("an unmatched strategy list must fail closed");
    // Go reports the business reason through `deniedProductRule`, which the
    // route maps to a 400 request error, so the status is the contract that
    // must hold; the reason text has to stay actionable.
    match &error {
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(*status, 400, "unmatched combination status: {error:?}");
            assert!(
                matches!(code.as_str(), "BAD_REQUEST" | "ILLEGAL_OPTION_COMBINATION"),
                "unmatched combination code = {code:?}"
            );
            assert!(
                message.to_ascii_lowercase().contains("legal")
                    || message.to_ascii_lowercase().contains("combination"),
                "unmatched combination message = {message:?}"
            );
        }
        other => panic!("unmatched combination error = {other:?}"),
    }

    // Matching legs: Go returns a nil result, i.e. the preview may continue.
    let matching = option_combo_port(
        Arc::new(ScriptedSpreadReader::with_spreads([10.0])),
        Arc::new(ScriptedStrategyReader {
            legs: Mutex::new(Some(matching_legs())),
            fail: false,
        }),
    );
    matching
        .combo_preview(&payload())
        .expect("matching strategy legs must allow the non-spread preview");

    // Dropped 3256 response must surface as an error.
    let dropped = option_combo_port(
        Arc::new(ScriptedSpreadReader::with_spreads([10.0])),
        Arc::new(ScriptedStrategyReader {
            legs: Mutex::new(Some(Vec::new())),
            fail: true,
        }),
    );
    dropped
        .combo_preview(&payload())
        .expect_err("a dropped strategy response must not be hidden");
}

// Parity: go:452dea11:internal/app/apiserver/tradingapp/execution_gateway_lifecycle_test.go:250 TestExecutionGatewayPlaceComboBoundaries
/// Parity: go:452dea11:pkg/futu/adapter_combo_transport_test.go:199
/// TestFutuComboPlaceValidatedLegAccountAndTransportFailures.
///
/// The place route reuses the same intent validation: an invalid leg side, a
/// missing account, a dropped `Trd_PlaceComboOrder` response, a mixed product
/// class and a zero leg ratio must all fail. Go checks the mixed/zero-ratio legs
/// through `validateComboIntent`, which runs before any submission.
#[test]
fn combo_place_rejects_invalid_legs_accounts_and_hidden_transport_failures() {
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = combo_place_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);

    // Invalid leg side fails during intent parsing.
    let mut bad_side = option_combo_fixture_payload();
    bad_side["legs"][0]["side"] = json!("HOLD");
    bad_side["previewId"] = json!("preview-x");
    let error = port
        .place_combo(&bad_side)
        .expect_err("placing an invalid combo side must fail");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "invalid side place error = {error:?}"
    );

    // Missing account is rejected while resolving the trade account.
    let mut missing_account = option_combo_fixture_payload();
    missing_account["previewId"] = json!("preview-x");
    missing_account["accountId"] = json!("missing");
    let error = port
        .place_combo(&missing_account)
        .expect_err("placing a combo with a missing account must fail");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
        ),
        "missing account place error = {error:?}"
    );

    // Fully formed mixed-product and zero-ratio intents are rejected by the
    // combination validator before submission.
    for (label, mutate) in [
        (
            "mixed product class",
            Box::new(|value: &mut Value| {
                value["legs"][1]["productClass"] = json!("future");
            }) as Box<dyn Fn(&mut Value)>,
        ),
        (
            "zero ratio",
            Box::new(|value: &mut Value| {
                value["legs"][1]["ratio"] = json!(0);
            }) as Box<dyn Fn(&mut Value)>,
        ),
    ] {
        let mut payload = option_combo_fixture_payload();
        mutate(&mut payload);
        payload["previewId"] = json!("preview-x");
        let error = port
            .place_combo(&payload)
            .expect_err(&format!("{label} combo must fail validation"));
        assert!(
            matches!(
                &error,
                ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
            ),
            "{label} error = {error:?}"
        );
    }
    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        0,
        "an invalid combo intent must never reach OpenD"
    );
}

/// Write fixture that always fails at the OpenD transport boundary, counting
/// how many times the adapter attempted the mutation.
#[derive(Debug, Default)]
struct DisconnectingTradeWriter {
    placed: Mutex<usize>,
    modified: Mutex<usize>,
}

impl DisconnectingTradeWriter {
    fn placed_calls(&self) -> usize {
        *self.placed.lock().expect("placed calls")
    }
}

fn disconnecting_error() -> TradeSessionError {
    TradeSessionError::Session(
        jftrade_integration_futu::OpenDManagedSessionError::Closed(
            jftrade_integration_futu::OpenDSessionCloseReason::PeerClosed,
        ),
    )
}

impl TradeWritePort for DisconnectingTradeWriter {
    fn place_order(
        &self,
        _: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        *self.placed.lock().expect("placed calls") += 1;
        Err(disconnecting_error())
    }

    fn place_combo_order(
        &self,
        _: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError> {
        Err(disconnecting_error())
    }

    fn modify_order(
        &self,
        _: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        *self.modified.lock().expect("modified calls") += 1;
        Err(disconnecting_error())
    }

    fn unlock_trade(&self, _: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        Err(disconnecting_error())
    }

    fn subscribe_trade_accounts(
        &self,
        _: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        Err(disconnecting_error())
    }
}

/// Builds a port whose only write dependency is the supplied failing writer.
/// Port whose writer is a scripted combo writer. The combo place route only
/// needs the mutation boundary: intent validation and account resolution run
/// before any submission, so the writer must never be reached by invalid
/// intents.
fn combo_place_port(writer: Arc<dyn TradeWritePort>) -> ProductionExecutionPort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    // `writer()` and `reader()` both prefer the runtime snapshot whenever a
    // trade runtime is installed, so the logged-in session and the scripted
    // writer have to be published through that same owner.
    runtime.set(None, Some(true));
    runtime.set_writer(Some(Arc::clone(&writer)));
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let (store, directory) = execution_store();
    let _ = directory.keep();
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: Some(writer),
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

fn write_port_with_writer(writer: Arc<dyn TradeWritePort>) -> ProductionExecutionPort {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let (store, directory) = execution_store();
    let _ = directory.keep();
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: Some(writer),
        trade_runtime: None,
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

/// Write fixture whose placement succeeds but whose cancel loses its response,
/// counting how many modify attempts the adapter made.
#[derive(Debug, Default)]
struct CancelLosesResponseWriter {
    modified: Mutex<usize>,
}

impl CancelLosesResponseWriter {
    fn modified_calls(&self) -> usize {
        *self.modified.lock().expect("modified calls")
    }
}

impl TradeWritePort for CancelLosesResponseWriter {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        Ok(TradePlaceOrderResult {
            header: request.header,
            order_id: Some(9001),
            order_id_ex: Some("EXT-9001".to_owned()),
        })
    }

    fn place_combo_order(
        &self,
        _: TradePlaceComboOrderRequest,
    ) -> Result<TradePlaceComboOrderResult, TradeSessionError> {
        Err(disconnecting_error())
    }

    fn modify_order(
        &self,
        _: TradeModifyOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        *self.modified.lock().expect("modified calls") += 1;
        Err(disconnecting_error())
    }

    fn unlock_trade(&self, _: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        Err(disconnecting_error())
    }

    fn subscribe_trade_accounts(
        &self,
        _: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        Err(disconnecting_error())
    }
}

/// Go `TestTradeWriteMethodsPropagateAccountAndWriteDisconnects`: a write whose
/// OpenD transport is gone must surface an error to the caller and must not
/// fabricate a submitted order. The Rust owner is the execution write port,
/// which persists UNKNOWN and returns the upstream failure.
#[test]
fn trade_write_methods_propagate_write_disconnects() {
    let writer = Arc::new(DisconnectingTradeWriter::default());
    let port = write_port_with_writer(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    let payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "disconnect-write",
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5,
        "timeInForce": "GTC"
    });
    let error = port
        .place_order(&payload)
        .expect_err("a disconnected write must fail closed");
    let message = format!("{error:?}");
    assert!(
        message.contains("Session") || message.contains("session"),
        "unexpected write error: {message}"
    );
    assert_eq!(
        writer.placed_calls(),
        1,
        "a failed submission must be attempted exactly once"
    );
}

/// Go `TestTradeWritesAreNotReplayedWhenResponseIsLost`: when an accepted
/// request loses its response, the write must fail closed and must never be
/// replayed, because a second submission could double-fill the account.
#[test]
fn trade_writes_are_not_replayed_when_the_response_is_lost() {
    let writer = Arc::new(DisconnectingTradeWriter::default());
    let port = write_port_with_writer(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    let payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "lost-response-place",
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5,
        "timeInForce": "GTC"
    });
    port.place_order(&payload)
        .expect_err("a lost response must fail closed");
    assert_eq!(
        writer.placed_calls(),
        1,
        "place order must not be replayed after a lost response"
    );

    // Cancel path: the placement succeeds, the cancel loses its response. The
    // modify RPC must be attempted exactly once and never replayed.
    let cancel_writer = Arc::new(CancelLosesResponseWriter::default());
    let cancel_port =
        write_port_with_writer(Arc::clone(&cancel_writer) as Arc<dyn TradeWritePort>);
    let cancel_payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "HK",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "lost-response-cancel",
        "symbol": "HK.00700",
        "orderKind": "single",
        "productClass": "equity",
        "instrument": {"instrumentId": "HK.00700", "tradeMarket": "HK"},
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 100.0,
        "price": 320.5,
        "timeInForce": "GTC"
    });
    let placed = cancel_port
        .place_order(&cancel_payload)
        .expect("the placement itself succeeds");
    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();
    cancel_port
        .cancel_order(&internal_id)
        .expect_err("a lost cancel response must fail closed");
    assert_eq!(
        cancel_writer.modified_calls(),
        1,
        "modify order must not be replayed after a lost response"
    );
}

/// Parity: Go's gin handler hands the decoded `:internalOrderId` to the
/// trading service, while the Rust wire forwards the raw request path to the
/// execution read port and the MCP `execution.order_events` tool
/// percent-encodes the id it builds. An id such as `order-1` must therefore be
/// decoded before the store lookup instead of surfacing as 404.
#[test]
fn execution_read_decodes_percent_encoded_order_ids() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let port = preview_port(state, None, Some(true));
    port.store
        .save_order(
            jftrade_store_sqlite::StoredExecutionOrder {
                internal_order_id: "order-1".to_owned(),
                broker_id: "futu".to_owned(),
                broker_order_id: Some("broker-1".to_owned()),
                broker_order_id_ex: None,
                source: "api".to_owned(),
                source_detail: "execution-read-decode".to_owned(),
                trading_environment: "REAL".to_owned(),
                account_id: "8240".to_owned(),
                market: "US".to_owned(),
                symbol: Some("AAPL".to_owned()),
                side: Some("BUY".to_owned()),
                order_type: Some("LIMIT".to_owned()),
                status: "SUBMITTED".to_owned(),
                raw_broker_status: None,
                requested_quantity: Some(1.0),
                requested_price: Some(100.0),
                filled_quantity: None,
                filled_average_price: None,
                remark: None,
                last_error: None,
                last_error_code: None,
                last_error_source: None,
                submitted_at: None,
                updated_at: "2026-09-01T00:00:00Z".to_owned(),
                created_at: "2026-09-01T00:00:00Z".to_owned(),
                order_kind: "single".to_owned(),
                product_class: "equity".to_owned(),
                quantity_mode: "units".to_owned(),
                client_order_id: Some("client-1".to_owned()),
                preview_id: None,
                normalized_request: "{}".to_owned(),
                requested_amount: None,
                payout: None,
                fees: None,
            },
            "2026-09-01T00:00:00Z",
        )
        .expect("save order");

    let events = port
        .read("/api/v1/execution/orders/order%2D1/events", "")
        .expect("percent-encoded event timeline");
    assert_eq!(events["internalOrderId"], "order-1");

    let detail = port
        .read("/api/v1/execution/orders/order%2D1", "")
        .expect("percent-encoded order detail");
    assert_eq!(detail["order"]["internalOrderId"], "order-1");

    let slash = port
        .read("/api/v1/execution/orders/order%2D1%2Fevil/events", "")
        .expect_err("an id that decodes to a path separator stays not found");
    assert!(matches!(slash, ExecutionReadSnapshotError::NotFound));
}
