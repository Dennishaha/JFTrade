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
