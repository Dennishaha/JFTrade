use super::*;

use jftrade_integration_futu::{
    PredictionMarketReadError, PredictionMarketReadPort, TradeAccountSnapshot,
    TradeCashFlowSnapshot, TradeComboMaxTradeQuantityRequest,
    TradeComboMaxTradeQuantitySnapshot, TradeFillSnapshot, TradeFilter, TradeFundsSnapshot,
    TradeHeader, TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest,
    TradeMaxTradeQuantitySnapshot, TradeModifyOrderRequest, TradeOrderFeeSnapshot,
    TradeOrderSnapshot, TradePlaceOrderRequest, TradePlaceOrderResult, TradePositionSnapshot,
    TradeReadPort, TradeSecurity, TradeSessionError, TradeSubscribeAccountsRequest,
    TradeUnlockRequest, TradeWritePort,
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
        _request: jftrade_integration_futu::TradePlaceComboOrderRequest,
    ) -> Result<jftrade_integration_futu::TradePlaceComboOrderResult, TradeSessionError> {
        unsupported()
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
