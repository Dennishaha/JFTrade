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

#[test]
fn execution_trade_errors_preserve_go_request_connectivity_and_command_codes() {
    let account_missing = execution_order_helpers::map_trade_error(
        jftrade_integration_futu::TradeSessionError::Response(
            jftrade_integration_futu::ResponseError::ReturnCode {
                ret_type: 1,
                err_code: 1001,
                message: "account not found".to_owned(),
            },
        ),
    );
    assert!(matches!(
        account_missing,
        ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
    ));

    let disconnected = execution_order_helpers::map_trade_error(
        jftrade_integration_futu::TradeSessionError::Coordinator(
            jftrade_integration_futu::OpenDSessionCoordinatorError::Closed,
        ),
    );
    assert!(matches!(
        disconnected,
        ExecutionWritePortError::Failed { status: 502, code, .. } if code == "BROKER_NOT_CONNECTED"
    ));

    let command_failed = execution_order_helpers::map_trade_error(
        jftrade_integration_futu::TradeSessionError::Response(
            jftrade_integration_futu::ResponseError::ReturnCode {
                ret_type: 1,
                err_code: 9001,
                message: "order rejected".to_owned(),
            },
        ),
    );
    assert!(matches!(
        command_failed,
        ExecutionWritePortError::Failed { status: 502, code, .. } if code == "BROKER_COMMAND_FAILED"
    ));

    let timeout = execution_order_helpers::map_trade_error(
        jftrade_integration_futu::TradeSessionError::Session(
            jftrade_integration_futu::OpenDManagedSessionError::RequestTimeout {
                protocol: 2001,
                serial: 7,
            },
        ),
    );
    assert!(matches!(
        timeout,
        ExecutionWritePortError::Failed { status: 504, code, .. } if code == "BROKER_TIMEOUT"
    ));

    let rate_limited = execution_order_helpers::map_trade_error(
        jftrade_integration_futu::TradeSessionError::RateLimited,
    );
    assert!(matches!(
        rate_limited,
        ExecutionWritePortError::Failed { status: 429, code, .. } if code == "BROKER_RATE_LIMITED"
    ));

    let unsupported_market = execution_order_helpers::map_trade_error(
        jftrade_integration_futu::TradeSessionError::Response(
            jftrade_integration_futu::ResponseError::ReturnCode {
                ret_type: 1,
                err_code: 9002,
                message: "market not supported".to_owned(),
            },
        ),
    );
    assert!(matches!(
        unsupported_market,
        ExecutionWritePortError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
    ));
}

use crate::product::product_brokers_write_port::{
    BrokersWriteContext, BrokersWriteInput, BrokersWriteOperation, BrokersWritePort,
    BrokersWritePortError, BrokersWriteQuery,
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
/// preview can persist and be consumed by `place_combo`, and answers the
/// ordinary `Trd_GetMaxTrdQtys` probe that the buying-power route forwards for
/// option products.
#[derive(Debug, Default)]
struct ComboPreviewTradeReader {
    calls: Arc<Mutex<Vec<TradeComboMaxTradeQuantityRequest>>>,
    max_quantity_calls: Arc<Mutex<Vec<TradeMaxTradeQuantityRequest>>>,
    /// Answer only `buyingPowerDecrease`, like Go's broker preview result in
    /// `TestExecutionComboPreviewKeepsLegacyBuyingPowerCompatible` (42.0) where
    /// the service has to backfill the legacy `buyingPowerImpact` field.
    buying_power_decrease_only: bool,
    /// Account discovery rows; the futures-authority guard (Go's
    /// `validateFuturesTradingAuthority`) reads them.
    accounts: Vec<TradeAccountSnapshot>,
}

impl TradeReadPort for ComboPreviewTradeReader {
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
        self.max_quantity_calls
            .lock()
            .expect("max quantity calls")
            .push(request.clone());
        Ok(TradeMaxTradeQuantitySnapshot {
            header: request.header,
            code: request.code,
            order_type: request.order_type,
            price: request.price,
            max_cash_buy: 250.0,
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
        request: TradeComboMaxTradeQuantityRequest,
    ) -> Result<TradeComboMaxTradeQuantitySnapshot, TradeSessionError> {
        self.calls
            .lock()
            .expect("combo preview calls")
            .push(request.clone());
        let decrease_only = self.buying_power_decrease_only;
        Ok(TradeComboMaxTradeQuantitySnapshot {
            header: request.header,
            nlv_change: (!decrease_only).then_some(-100.0),
            initial_margin_change: (!decrease_only).then_some(-100.0),
            maintenance_margin_change: (!decrease_only).then_some(-100.0),
            option_buy_power: (!decrease_only).then_some(-100.0),
            max_withdraw_change: None,
            buying_power_decrease: Some(if decrease_only { 42.0 } else { 100.0 }),
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
// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:150 TestFutuComboAdapterErrorPropagationBranches
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

/// Parity: go:452dea11:internal/trading/execution_test.go:173
/// `TestExecutionOrderServiceFacadeUsesInjectedStoresAndBrokerCommands`. Go
/// normalizes the requested scope (`" futu "`, `" acc-1 "`, `" us "` and the
/// default REAL environment) and asserts the normalized filter reaches the
/// injected store, then checks the cancel and event calls carry the returned
/// order id. Rust owns that normalization inside the production read port, so
/// the equivalent evidence is the filtered result set plus the identifier
/// round-trip through the events route.
#[test]
fn execution_read_port_normalizes_filters_and_routes_order_identifiers() {
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);

    let simulate = port
        .place_order(&cancel_contract_payload("filter-simulate-order"))
        .expect("place simulate order");
    let simulate_id = simulate["internalOrderId"]
        .as_str()
        .expect("simulate order id")
        .to_owned();

    let real = port
        .place_order(&cancel_contract_payload("filter-real-order"))
        .expect("place real order");
    let real_id = real["internalOrderId"]
        .as_str()
        .expect("real order id")
        .to_owned();
    let mut stored = port
        .store
        .get_order(&real_id)
        .expect("read stored order")
        .expect("stored order");
    stored.trading_environment = "REAL".to_owned();
    stored.account_id = "acc-1".to_owned();
    stored.market = "US".to_owned();
    port.store
        .save_order(stored, "2026-09-22T00:00:00Z")
        .expect("persist real order fixture");

    let filtered = port
        .read(
            "/api/v1/execution/orders",
            "scope=active&brokerId=%20futu%20&tradingEnvironment=%20real%20&accountId=%20acc-1%20&market=%20us%20",
        )
        .expect("filtered execution orders");
    let orders = filtered["orders"].as_array().expect("orders array");
    assert_eq!(orders.len(), 1, "filtered orders = {filtered}");
    assert_eq!(orders[0]["internalOrderId"], json!(real_id.clone()));
    assert_ne!(orders[0]["internalOrderId"], json!(simulate_id));

    let events = port
        .read(&format!("/api/v1/execution/orders/{real_id}/events"), "")
        .expect("order events");
    assert_eq!(events["internalOrderId"], json!(real_id));
    assert!(
        events["events"]
            .as_array()
            .is_some_and(|events| !events.is_empty()),
        "events = {events}"
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:342
/// `TestCreateExecutionOrderRunsPreTradeRiskBeforeBrokerCall`. Go configures a
/// risk gateway with REAL trading disabled and a `placeOrder` stub that fails
/// the test when invoked, then requires the "real trading is disabled"
/// rejection. Rust evaluates the same decision in
/// `ExecutionRiskCoordinator::execute_with_risk_guard` before the broker
/// closure runs, so the broker-facing writer must stay untouched.
#[test]
fn pre_trade_risk_rejection_stops_the_real_order_before_the_broker_is_called() {
    let directory = tempfile::tempdir().expect("risk control directory");
    let writer = Arc::new(RecordingTradeWriter::default());
    let mut port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    port.risk_coordinator = Some(Arc::new(crate::product::ExecutionRiskCoordinator::new(
        directory.path().join("real-trade-control.json"),
    )));

    let mut payload = cancel_contract_payload("risk-rejects-real-order");
    payload["tradingEnvironment"] = json!("REAL");
    let error = port
        .place_order(&payload)
        .expect_err("risk rejection must stop the order");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ref message,
            } if code == "REAL_TRADING_DISABLED" && message.contains("real trading is disabled")
        ),
        "risk rejection error = {error:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "broker must not receive risk-rejected orders"
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:470
/// Parity: go:452dea11:internal/trading/broker_test.go:711 TestPlaceBrokerOrderFailsClosedWhenRealRiskGatewayIsUnavailable
/// `TestPlaceExecutionOrderFailsClosedWithoutRealRiskGateway`. Go defaults the
/// environment to REAL, injects no risk gateway, and requires the fail-closed
/// `PRE_TRADE_RISK_UNAVAILABLE` rejection with no order-gateway call.
#[test]
fn real_order_without_a_risk_gateway_fails_closed_before_the_broker_is_called() {
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);

    let mut payload = cancel_contract_payload("missing-risk-gateway");
    payload["tradingEnvironment"] = json!("REAL");
    let error = port
        .place_order(&payload)
        .expect_err("missing gateway must fail closed");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "PRE_TRADE_RISK_UNAVAILABLE"
        ),
        "fail-closed error = {error:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "broker must not receive REAL orders without a risk gateway"
    );
}

/// Seed a REAL-enabled runtime risk entry so the coordinator approves orders.
fn write_enabled_risk_config(path: &std::path::Path) {
    std::fs::write(
        path,
        r#"{
  "riskConfig": {
    "id": "runtime-risk-fixture",
    "tradingEnvironment": "REAL",
    "realTradingEnabled": true,
    "maxOrderNotional": 500,
    "operatorId": "fixture-operator",
    "reason": "batch 117 shard 3 fixture",
    "activatedAt": "2026-09-22T00:00:00Z",
    "updatedAt": "2026-09-22T00:00:00Z"
  }
}
"#,
    )
    .expect("seed runtime risk config");
}

/// Parity: go:452dea11:internal/trading/execution_test.go:367
/// `TestCreateExecutionOrderAllowsRuntimeEnabledRealTradeBeforeBrokerCall`. Go
/// enables REAL trading with a runtime max notional and requires the order to
/// be accepted *and* to reach the injected broker (`placed=true`). Rust must
/// therefore pass the risk gate and hand the order to the write port.
#[test]
fn runtime_enabled_real_trade_reaches_the_broker_after_risk_approval() {
    let directory = tempfile::tempdir().expect("risk control directory");
    let control_path = directory.path().join("real-trade-control.json");
    write_enabled_risk_config(&control_path);
    let writer = Arc::new(RecordingTradeWriter::default());
    let mut port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    port.risk_coordinator = Some(Arc::new(crate::product::ExecutionRiskCoordinator::new(
        control_path,
    )));

    let mut payload = cancel_contract_payload("runtime-enabled-real-order");
    payload["tradingEnvironment"] = json!("REAL");
    payload["price"] = json!(100.0);
    payload["quantity"] = json!(1.0);
    let placed = port
        .place_order(&payload)
        .expect("runtime-enabled REAL order");
    assert!(
        placed["internalOrderId"].is_string(),
        "placed response = {placed}"
    );
    assert_eq!(
        writer.placed.lock().expect("placed orders").len(),
        1,
        "risk-approved REAL order must reach the broker"
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:397
/// `TestCreateExecutionOrderAllowsSimulateWhenRealTradingIsDisabled`. Go keeps
/// REAL disabled but still requires a SIMULATE order to be created and reach
/// the broker (`placed=true`).
#[test]
fn simulate_order_reaches_the_broker_while_real_trading_is_disabled() {
    let directory = tempfile::tempdir().expect("risk control directory");
    let writer = Arc::new(RecordingTradeWriter::default());
    let mut port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    port.risk_coordinator = Some(Arc::new(crate::product::ExecutionRiskCoordinator::new(
        directory.path().join("real-trade-control.json"),
    )));

    let placed = port
        .place_order(&cancel_contract_payload("simulate-with-real-disabled"))
        .expect("simulate order");
    assert!(
        placed["internalOrderId"].is_string(),
        "placed response = {placed}"
    );
    assert_eq!(
        writer.placed.lock().expect("placed orders").len(),
        1,
        "SIMULATE order must reach the broker even when REAL trading is disabled"
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:426
/// `TestPlaceExecutionOrderResolvesImplicitRealEnvironmentBeforeRisk`. Go's
/// default environment is REAL; a request that omits `tradingEnvironment` must
/// still resolve to REAL, hit the risk gate, and never reach the order gateway.
#[test]
fn implicit_real_environment_is_risk_rejected_before_the_broker_is_called() {
    let directory = tempfile::tempdir().expect("risk control directory");
    let writer = Arc::new(RecordingTradeWriter::default());
    let mut port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    port.risk_coordinator = Some(Arc::new(crate::product::ExecutionRiskCoordinator::new(
        directory.path().join("real-trade-control.json"),
    )));
    port.default_trading_environment = Some(Arc::new(|| "REAL".to_owned()));

    let mut payload = cancel_contract_payload("implicit-real-order");
    payload
        .as_object_mut()
        .expect("payload object")
        .remove("tradingEnvironment");
    let error = port
        .place_order(&payload)
        .expect_err("implicit REAL must be risk rejected");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "REAL_TRADING_DISABLED"
        ),
        "implicit REAL error = {error:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "order gateway must not run after implicit REAL risk rejection"
    );
}

/// Parity: go:452dea11:internal/trading/broker_test.go:680
/// `TestPlaceBrokerOrderCannotBypassRiskWithImplicitRealEnvironment`.
/// Parity: go:452dea11:internal/trading/broker_test.go:648 TestPlaceBrokerOrderRunsPreTradeRiskBeforeBrokerSubmission Go's
/// service resolves an omitted `tradingEnvironment` through the configured
/// default (REAL) before the pre-trade risk gate, so an active kill switch
/// rejects the placement with `REAL_TRADE_KILL_SWITCH_ACTIVE` and the broker
/// gateway is never called. Rust resolves the same default inside
/// `ProductionExecutionPort::place_order`; this pins the implicit-REAL and
/// kill-switch combination end to end instead of relying on an explicit REAL
/// fixture.
#[test]
fn implicit_real_environment_hits_the_kill_switch_before_the_broker_is_called() {
    let directory = tempfile::tempdir().expect("risk control directory");
    let writer = Arc::new(RecordingTradeWriter::default());
    let mut port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    std::fs::write(
        directory.path().join("real-trade-control.json"),
        r#"{"riskConfig":{"realTradingEnabled":true},"killSwitch":{"id":"ks-1"}}"#,
    )
    .expect("write real-trade control plane");
    port.risk_coordinator = Some(Arc::new(crate::product::ExecutionRiskCoordinator::new(
        directory.path().join("real-trade-control.json"),
    )));
    port.default_trading_environment = Some(Arc::new(|| "REAL".to_owned()));

    let mut payload = cancel_contract_payload("implicit-real-kill-switch");
    payload
        .as_object_mut()
        .expect("payload object")
        .remove("tradingEnvironment");
    let error = port
        .place_order(&payload)
        .expect_err("implicit REAL must be blocked by the kill switch");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed {
                status: 403,
                ref code,
                ..
            } if code == "REAL_TRADE_KILL_SWITCH_ACTIVE"
        ),
        "implicit REAL kill-switch error = {error:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "order gateway must not run after an implicit REAL kill-switch rejection"
    );
}

/// Parity: go:452dea11:internal/trading/execution_test.go:327
/// `TestCreateExecutionOrderRejectsInvalidPayloadBeforeBrokerCall`. Go builds
/// the order service with a fake `placeOrder` that fails the test when it is
/// invoked, then asserts a zero quantity is rejected as a request error. In
/// Rust the payload is parsed inside `ProductionExecutionPort::place_order`
/// before `self.writer()` is resolved, so the equivalent evidence is: the
/// broker-facing `TradeWritePort` records no placement while the caller gets
/// the 400 BAD_REQUEST validation envelope.
#[test]
fn invalid_order_payload_is_rejected_before_the_broker_is_called() {
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);

    let mut zero_quantity = cancel_contract_payload("invalid-quantity-order");
    zero_quantity["quantity"] = json!(0);
    let error = port.place_order(&zero_quantity).expect_err("zero quantity");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed {
                status: 400,
                ref code,
                ref message,
            } if code == "BAD_REQUEST" && message.contains("quantity must be positive")
        ),
        "zero quantity error = {error:?}"
    );

    let mut invalid_side = cancel_contract_payload("invalid-side-order");
    invalid_side["side"] = json!("HOLD");
    let error = port.place_order(&invalid_side).expect_err("invalid side");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed {
                status: 400,
                ref code,
                ..
            } if code == "BAD_REQUEST"
        ),
        "invalid side error = {error:?}"
    );

    assert!(
        writer
            .placed
            .lock()
            .expect("placed orders")
            .is_empty(),
        "broker must not receive invalid payloads"
    );
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
// Parity: go:452dea11:internal/trading/broker_conformance_test.go:58 TestFakeBrokerConformanceCancelAcceptedAndCancelRejected
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

/// Write fixture whose every submission fails with the same upstream
/// broker error, so write-path tests can prove the failure surfaces instead
/// of being masked by the port.
#[derive(Debug, Default)]
struct UpstreamFailingTradeWriter {
    placed: Mutex<usize>,
    modified: Mutex<usize>,
    unlocked: Mutex<usize>,
}

impl TradeWritePort for UpstreamFailingTradeWriter {
    fn place_order(
        &self,
        _: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        *self.placed.lock().expect("placed calls") += 1;
        Err(TradeSessionError::Unsupported("broker write failed".to_owned()))
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
        Err(TradeSessionError::Unsupported("broker write failed".to_owned()))
    }

    fn unlock_trade(&self, _: TradeUnlockRequest) -> Result<(), TradeSessionError> {
        *self.unlocked.lock().expect("unlocked calls") += 1;
        Err(TradeSessionError::Unsupported("broker write failed".to_owned()))
    }

    fn subscribe_trade_accounts(
        &self,
        _: TradeSubscribeAccountsRequest,
    ) -> Result<(), TradeSessionError> {
        unsupported()
    }
}

/// Builds a write port over an explicit store so one test can seed ledger
/// rows through a recording writer and then exercise them through another
/// writer configuration.
fn write_port_on_store(
    store: Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    writer: Option<Arc<dyn TradeWritePort>>,
) -> ProductionExecutionPort {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: writer,
        trade_runtime: None,
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

/// Parity: go:452dea11:internal/trading/broker_boundaries_test.go:116 TestServiceBrokerWriteOperationsPropagateUpstreamFailures
#[test]
fn broker_write_operations_propagate_upstream_broker_failures() {
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let seed_writer = Arc::new(RecordingTradeWriter::default());
    let seed_port = write_port_on_store(
        Arc::clone(&store),
        Some(Arc::clone(&seed_writer) as Arc<dyn TradeWritePort>),
    );
    let seeded = seed_port
        .place_order(&cancel_contract_payload("upstream-fail-seed"))
        .expect("seed order");
    let seeded_id = seeded["internalOrderId"]
        .as_str()
        .expect("seeded order id")
        .to_owned();

    let failing_writer = Arc::new(UpstreamFailingTradeWriter::default());
    let port = write_port_on_store(
        Arc::clone(&store),
        Some(Arc::clone(&failing_writer) as Arc<dyn TradeWritePort>),
    );
    let query = BrokersWriteQuery {
        broker_id: "futu".to_owned(),
        account_id: "42".to_owned(),
        trading_environment: "SIMULATE".to_owned(),
        market: "HK".to_owned(),
    };

    let mut place_payload = cancel_contract_payload("upstream-fail-place");
    place_payload["tradingEnvironment"] = serde_json::json!("SIMULATE");
    let place_error = BrokersWritePort::mutate(
        &port,
        &BrokersWriteInput {
            operation: BrokersWriteOperation::PlaceOrder,
            query: query.clone(),
            payload: place_payload,
            context: BrokersWriteContext::Normal,
        },
    )
    .expect_err("place must propagate the broker failure");
    assert!(
        format!("{place_error:?}").contains("broker write failed"),
        "place error = {place_error:?}",
    );

    let unlock_error = BrokersWritePort::mutate(
        &port,
        &BrokersWriteInput {
            operation: BrokersWriteOperation::Unlock,
            query: query.clone(),
            payload: serde_json::json!({"unlock": true, "passwordMd5": "dummy-md5"}),
            context: BrokersWriteContext::Normal,
        },
    )
    .expect_err("unlock must propagate the broker failure");
    assert!(
        format!("{unlock_error:?}").contains("broker write failed"),
        "unlock error = {unlock_error:?}",
    );

    let cancel_error = BrokersWritePort::mutate(
        &port,
        &BrokersWriteInput {
            operation: BrokersWriteOperation::CancelOrders,
            query,
            payload: serde_json::json!({"orders": [{"internalOrderId": seeded_id}]}),
            context: BrokersWriteContext::Normal,
        },
    )
    .expect_err("cancel must propagate the broker failure");
    assert!(
        format!("{cancel_error:?}").contains("broker write failed"),
        "cancel error = {cancel_error:?}",
    );

    assert_eq!(*failing_writer.placed.lock().expect("placed calls"), 1);
    assert_eq!(*failing_writer.modified.lock().expect("modified calls"), 1);
    assert_eq!(*failing_writer.unlocked.lock().expect("unlocked calls"), 1);
}

/// Parity: go:452dea11:internal/trading/broker_account_read_failures_test.go:87 TestFundsMapsMarketAssetsAlongsideCashBalances
#[test]
fn cancel_without_a_trade_writer_fails_closed_before_broker_call() {
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let seed_writer = Arc::new(RecordingTradeWriter::default());
    let seed_port = write_port_on_store(
        Arc::clone(&store),
        Some(Arc::clone(&seed_writer) as Arc<dyn TradeWritePort>),
    );
    let seeded = seed_port
        .place_order(&cancel_contract_payload("no-writer-seed"))
        .expect("seed order");
    let seeded_id = seeded["internalOrderId"]
        .as_str()
        .expect("seeded order id")
        .to_owned();

    let port = write_port_on_store(Arc::clone(&store), None);
    let error = port
        .cancel_order(&seeded_id)
        .expect_err("cancel without a writer must fail closed");
    assert!(
        format!("{error:?}").contains("OpenD trade runtime is unavailable"),
        "cancel error = {error:?}",
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

#[derive(Debug, Default)]
struct CancelRejectingWriter;

impl TradeWritePort for CancelRejectingWriter {
    fn place_order(
        &self,
        request: TradePlaceOrderRequest,
    ) -> Result<TradePlaceOrderResult, TradeSessionError> {
        Ok(TradePlaceOrderResult {
            header: request.header,
            order_id: Some(9002),
            order_id_ex: Some("EXT-9002".to_owned()),
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
        Err(TradeSessionError::Response(
            jftrade_integration_futu::ResponseError::ReturnCode {
                ret_type: 1,
                err_code: 3001,
                message: "broker refused cancel because order is locked".to_owned(),
            },
        ))
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
                if code == "BROKER_NOT_CONNECTED"
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
fn broker_cancel_rejection_keeps_cancel_requested_and_records_rejection_event() {
    // Parity: internal/trading/broker_conformance_test.go:58 TestFakeBrokerConformanceCancelAcceptedAndCancelRejected
    let writer = Arc::new(CancelRejectingWriter);
    let port = cancel_contract_port(Arc::clone(&writer) as Arc<dyn TradeWritePort>);
    let placed = port
        .place_order(&cancel_contract_payload("cancel-contract-rejected"))
        .expect("place order");
    let order_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();

    let error = port
        .cancel_order(&order_id)
        .expect_err("broker cancellation rejection");
    assert!(
        matches!(
            error,
            ExecutionWritePortError::Failed { status: 502, ref code, .. }
                if code == "BROKER_COMMAND_FAILED"
        ),
        "cancel rejection = {error:?}"
    );
    let stored = port
        .store
        .get_order(&order_id)
        .expect("read order")
        .expect("stored order");
    assert_eq!(stored.status, "CANCEL_SUBMITTED");
    assert_eq!(stored.last_error_source.as_deref(), Some("broker.cancel"));
    assert!(stored
        .last_error
        .as_deref()
        .is_some_and(|message| message.contains("order is locked")));
    let events = port
        .store
        .list_order_events(&order_id)
        .expect("list order events");
    assert!(events
        .iter()
        .any(|event| event.event_type == "BROKER_CANCEL_REJECTED"));
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

/// Parity: go:452dea11:internal/trading/broker_test.go:533
/// `TestServiceBrokerWriteAndTimeoutBehaviors`. Go rejects a write when the
/// requested broker is not the active one (`ErrBrokerNotFound`). Rust owns that
/// guard in `BrokersWritePort::mutate`; this pins the 404 envelope and proves
/// no place/unlock request reaches the trade writer for an inactive broker.
#[test]
fn broker_write_routes_reject_an_inactive_broker_before_the_trade_writer() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = preview_port_with_writer(state, None, Some(true), Some(Arc::clone(&writer)));

    for operation in [
        BrokersWriteOperation::PlaceOrder,
        BrokersWriteOperation::CancelOrders,
        BrokersWriteOperation::Unlock,
    ] {
        let error = BrokersWritePort::mutate(
            &port,
            &BrokersWriteInput {
                operation,
                query: BrokersWriteQuery {
                    broker_id: "ib".to_owned(),
                    account_id: "42".to_owned(),
                    trading_environment: "REAL".to_owned(),
                    market: "US".to_owned(),
                },
                payload: json!({}),
                context: BrokersWriteContext::Normal,
            },
        )
        .expect_err("inactive broker must be rejected");
        assert!(
            matches!(
                error,
                BrokersWritePortError::Failed {
                    status: 404,
                    ref code,
                    ..
                } if code == "BROKER_NOT_FOUND"
            ),
            "{operation:?} error = {error:?}"
        );
    }
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "an inactive broker must never reach Trd_PlaceOrder"
    );
    assert!(
        writer.unlocked.lock().expect("unlocked trades").is_empty(),
        "an inactive broker must never reach Trd_UnlockTrade"
    );
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
// Parity: go:452dea11:pkg/futu/advanced_product_adapter_contracts_test.go:150 TestFutuComboAdapterErrorPropagationBranches
fn option_combo_preview_place_and_cancel_keep_server_identity() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_strategy_spread(Some(Arc::new(OptionSpreadFixture)));
    runtime.set_option_strategy_analysis(Some(Arc::new(OptionAnalysisFixture)));
    let combo_calls = Arc::new(Mutex::new(Vec::new()));
    let reader = Arc::new(ComboPreviewTradeReader {
        calls: Arc::clone(&combo_calls),
        ..Default::default()
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
// Parity: go:452dea11:pkg/futu/transport_error_propagation_test.go:125 TestTradeWriteMethodsPropagateAccountAndWriteDisconnects
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
// Parity: go:452dea11:pkg/futu/transport_error_propagation_test.go:166 TestTradeWritesAreNotReplayedWhenResponseIsLost
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
// Parity: go:452dea11:internal/assistant/assembly/tool_catalog_test.go:707 TestExecutionReadToolsPropagateProjectionFailures
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

/// Parity: go:452dea11:internal/trading/execution_test.go:1014 TestExecutionOrderDetailsReturnsOrderAndBoundedRecentEvents
#[test]
fn execution_order_detail_returns_order_with_bounded_recent_events() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let port = preview_port(state, None, Some(false));
    port.store
        .save_order(
            jftrade_store_sqlite::StoredExecutionOrder {
                internal_order_id: "exec-1".to_owned(),
                broker_id: "futu".to_owned(),
                broker_order_id: Some("broker-exec-1".to_owned()),
                broker_order_id_ex: None,
                source: "api".to_owned(),
                source_detail: "execution-detail-bound".to_owned(),
                trading_environment: "SIMULATE".to_owned(),
                account_id: "42".to_owned(),
                market: "US".to_owned(),
                symbol: Some("AAPL".to_owned()),
                side: Some("BUY".to_owned()),
                order_type: Some("LIMIT".to_owned()),
                status: "BROKER_ACCEPTED".to_owned(),
                raw_broker_status: None,
                requested_quantity: Some(10.0),
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
                client_order_id: Some("client-exec-1".to_owned()),
                preview_id: None,
                normalized_request: "{}".to_owned(),
                requested_amount: None,
                payout: None,
                fees: None,
            },
            "2026-09-01T00:00:00Z",
        )
        .expect("save order");

    for index in 1..=12 {
        let id = format!("evt-{index:02}");
        let created_at = format!("2026-09-01T00:00:{index:02}Z");
        port.store
            .record_event(&jftrade_store_sqlite::StoredExecutionOrderEvent {
                id: &id,
                internal_order_id: "exec-1",
                event_type: "STATUS",
                previous_status: None,
                next_status: "BROKER_ACCEPTED",
                payload_json: "{}",
                created_at: &created_at,
            })
            .expect("record order event");
    }

    let detail = port
        .read("/api/v1/execution/orders/exec-1", "")
        .expect("order detail");
    assert_eq!(detail["order"]["internalOrderId"], "exec-1");
    assert_eq!(detail["order"]["status"], "BROKER_ACCEPTED");
    let events = detail["recentEvents"]
        .as_array()
        .expect("recentEvents array")
        .iter()
        .map(|event| event["id"].as_str().unwrap_or_default().to_owned())
        .collect::<Vec<_>>();
    assert_eq!(
        events,
        vec![
            "evt-03", "evt-04", "evt-05", "evt-06", "evt-07", "evt-08", "evt-09", "evt-10",
            "evt-11", "evt-12"
        ],
        "the newest ten events must be returned oldest-first"
    );
    assert!(
        detail["checkedAt"]
            .as_str()
            .is_some_and(|value| !value.is_empty()),
        "checkedAt must be populated"
    );

    let missing = port
        .read("/api/v1/execution/orders/exec-missing", "")
        .expect_err("missing order must not resolve to an empty detail");
    assert!(matches!(missing, ExecutionReadSnapshotError::NotFound));
}

/// Real-trade control file for the combo lifecycle parity tests: REAL trading
/// enabled with a two-contract quantity limit and an optional kill switch.
fn combo_lifecycle_control_file(
    directory: &std::path::Path,
    kill_switch: bool,
) -> std::path::PathBuf {
    let path = directory.join("real-trade-control.json");
    let mut state = json!({
        "riskConfig": {
            "realTradingEnabled": true,
            "maxOrderQuantity": 2.0,
            "maxOrderNotional": 100000.0
        }
    });
    if kill_switch {
        state["killSwitch"] = json!({
            "id": "ks-combo-lifecycle",
            "tradingEnvironment": "REAL",
            "operatorId": "ops",
            "reason": "combo staging halt",
            "activatedAt": "2026-09-22T00:00:00Z",
            "updatedAt": "2026-09-22T00:00:00Z"
        });
    }
    std::fs::write(&path, state.to_string()).expect("write real-trade control file");
    path
}

/// Production port for the combo lifecycle parity tests: one trade runtime owns
/// the option-strategy readers, the combo/max-quantity reader and the scripted
/// writer, and the risk coordinator reads the supplied real-trade control file.
fn combo_lifecycle_port(
    runtime: &Arc<SharedTradeReadRuntime>,
    reader: &Arc<ComboPreviewTradeReader>,
    writer: &Arc<RecordingTradeWriter>,
    control_path: std::path::PathBuf,
) -> ProductionExecutionPort {
    combo_lifecycle_port_with_risk(
        runtime,
        reader,
        writer,
        Arc::new(crate::product::ExecutionRiskCoordinator::new(control_path)),
    )
}

/// Same port, but the caller keeps the risk coordinator so a test can drive the
/// production control-plane mutations (activate or release a kill switch) that
/// the engine's system write port uses.
fn combo_lifecycle_port_with_risk(
    runtime: &Arc<SharedTradeReadRuntime>,
    reader: &Arc<ComboPreviewTradeReader>,
    writer: &Arc<RecordingTradeWriter>,
    risk_coordinator: Arc<crate::product::ExecutionRiskCoordinator>,
) -> ProductionExecutionPort {
    runtime.set_option_strategy_spread(Some(Arc::new(OptionSpreadFixture)));
    runtime.set_option_strategy_analysis(Some(Arc::new(OptionAnalysisFixture)));
    runtime.set(
        Some(Arc::clone(reader) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    runtime.set_writer(Some(Arc::clone(writer) as Arc<dyn TradeWritePort>));
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
        trade_runtime: Some(Arc::clone(runtime)),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: Some(risk_coordinator),
        default_trading_environment: None,
        notification_projector: None,
    }
}

/// Option-combo payload whose legs keep Go's padded, lower-case spelling.
fn padded_real_option_combo_payload(client_order_id: &str) -> Value {
    json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "REAL",
        "clientOrderId": client_order_id,
        "orderKind": "option_combo",
        "productClass": "option",
        "underlyingInstrumentId": "US.AAPL",
        "optionStrategy": "vertical",
        "nearExpiry": "2026-07-17",
        "spread": 10.0,
        "price": 1.25,
        "legs": [
            {"instrumentId": " us.aapl260717c00200000 ", "productClass": "option", "side": " buy ", "ratio": 1, "quantity": 2},
            {"instrumentId": "us.aapl260717c00210000", "productClass": "option", "side": "sell", "ratio": 1, "quantity": 2}
        ]
    })
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:15
/// `TestExecutionComboCompletePreviewPlaceCancelAndBuyingPower`.
///
/// Go drives one option-combo order through the trading service: the
/// buying-power query carries the execution-buying-power feature and reaches
/// the broker rule provider, the combo preview persists a credential bound to
/// the canonical `option_combo` intent, placement spends that credential while
/// handing the pre-trade gateway a two-contract command, and cancel trims the
/// internal order id before reaching the combo gateway. Rust splits the same
/// lifecycle across the execution write port, the durable preview/order fence
/// and the risk coordinator, so this test keeps the whole chain on the
/// production port instead of the compatibility placeholder.
#[test]
fn option_combo_lifecycle_preview_place_cancel_and_buying_power_keep_go_contract() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(ComboPreviewTradeReader::default());
    let writer = Arc::new(RecordingTradeWriter::default());
    let control_directory = tempfile::tempdir().expect("control directory");
    let control_path = combo_lifecycle_control_file(control_directory.path(), false);
    let port = combo_lifecycle_port(&runtime, &reader, &writer, control_path);

    // Go's first step asks the buying-power feature for an option product. The
    // Rust endpoint owns that feature id (the route *is*
    // `execution.buying_power`), so the equivalent evidence is the
    // broker-owned max-trade-quantity probe carrying the option contract.
    let buying_power = port
        .buying_power_preview(&json!({
            "accountId": "42",
            "brokerId": "futu",
            "market": "US",
            "tradingEnvironment": "SIMULATE",
            "orderKind": "single",
            "orderType": "LIMIT",
            "quantity": 2.0,
            "price": 1.25,
            "instrument": {
                "instrumentId": "US.AAPL260717C00200000",
                "productClass": "option",
                "tradeMarket": "US"
            }
        }))
        .expect("option buying power preview");
    assert_eq!(buying_power["allowed"], true, "{buying_power}");
    let probes = reader
        .max_quantity_calls
        .lock()
        .expect("max quantity probes");
    assert_eq!(probes.len(), 1, "one buying-power probe");
    assert_eq!(probes[0].header.acc_id, 42);
    // Futu's trade market for US is 2 (`TrdMarket_US`); 11 is the quote
    // market the option-strategy readers use.
    assert_eq!(probes[0].header.trd_market, 2);
    assert_eq!(probes[0].code, "AAPL260717C00200000");
    assert_eq!(probes[0].price, 1.25);
    assert_eq!(probes[0].order_type, 1, "LIMIT reaches OpenD as 1");
    drop(probes);

    let payload = padded_real_option_combo_payload("client-combo-1");
    let preview = port.combo_preview(&payload).expect("combo preview");
    assert_eq!(preview["allowed"], true, "{preview}");
    assert_eq!(preview["productClass"], "option");
    assert_eq!(preview["orderKind"], "option_combo");
    let legs = preview["legs"].as_array().expect("preview legs");
    assert_eq!(legs.len(), 2);
    assert_eq!(
        legs[0]["instrumentId"], "US.AAPL260717C00200000",
        "the padded lower-case leg must be canonicalized: {preview}"
    );
    assert_eq!(legs[0]["side"], "BUY");
    let preview_id = preview["previewId"]
        .as_str()
        .expect("preview id")
        .to_owned();
    assert!(!preview_id.trim().is_empty());

    // Go asserts the exact command handed to the pre-trade gateway
    // (`Query.Quantity == 2`, `QuantityMode == contracts`). Rust builds that
    // command in `build_pre_trade_risk_combo_order`; the policy below fences
    // the same two-contract quantity, and the REAL placement has to pass it.
    let parsed = super::execution_order_parse::parse_combo(&payload).expect("parsed combo");
    let risk_order = super::execution_order_helpers::build_pre_trade_risk_combo_order(&parsed);
    assert_eq!(risk_order.order_kind, "option_combo");
    assert_eq!(risk_order.quantity_mode, "contracts");
    assert_eq!(risk_order.quantity.to_string(), "2");
    assert_eq!(risk_order.amount, None);
    assert_eq!(risk_order.legs.len(), 2);
    assert_eq!(risk_order.legs[0].quantity.to_string(), "2");
    assert_eq!(risk_order.legs[1].quantity.to_string(), "2");

    let mut place_payload = payload.clone();
    place_payload["previewId"] = json!(preview_id);
    let placed = port
        .place_combo(&place_payload)
        .expect("combo place under the two-contract limit");
    assert_eq!(placed["status"], "SUBMITTED", "{placed}");
    assert_eq!(placed["quantityMode"], "contracts");
    assert_eq!(placed["requestedQuantity"], 2.0);
    assert_eq!(placed["brokerOrderIdEx"], "COMBO-9001");
    let submitted = writer.placed_combo.lock().expect("placed combos");
    assert_eq!(submitted.len(), 1, "exactly one combo submission");
    assert_eq!(submitted[0].quantity, 2.0);
    assert_eq!(submitted[0].combo_legs.len(), 2);
    assert_eq!(submitted[0].combo_legs[0].code, "AAPL260717C00200000");
    assert_eq!(submitted[0].combo_legs[1].side, Some(2));
    drop(submitted);

    // The consumed credential replays the stored order instead of issuing a
    // second combo submission, matching Go's `previewStore.consumed == 1`.
    let replayed = port
        .place_combo(&place_payload)
        .expect("idempotent combo replay");
    assert_eq!(replayed["status"], "SUBMITTED");
    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        1,
        "the consumed preview must not submit a second combo"
    );

    // Go's `CancelExecutionCombo(" internal-combo ")` trims the id before the
    // combo gateway sees it. Rust trims while parsing the cancel route, so the
    // padded path has to reach the same order with exactly one modify call.
    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal combo order id");
    // Go stores the canonical combo intent (`NormalizedRequest` contains
    // `option_combo`) so the durable identity fence can replay it byte for
    // byte. Rust persists the same canonical intent on the order record; the
    // preview row keeps the raw payload that produced the request hash.
    let stored = port
        .store
        .get_order(internal_id)
        .expect("read stored combo order")
        .expect("stored combo order row");
    assert!(
        stored.normalized_request.contains("option_combo"),
        "canonical normalized request = {}",
        stored.normalized_request
    );
    assert_eq!(stored.quantity_mode, "contracts");
    assert_eq!(stored.requested_quantity, Some(2.0));
    let cancel_response = crate::product::product_execution_write_port::dispatch_execution_write(
        &crate::product::product_execution_write_port::ExecutionWriteRequest {
            method: "POST".to_owned(),
            path: format!("/api/v1/execution/combos/%20{internal_id}%20/cancel"),
            body: None,
            context: crate::product::product_execution_write_port::ExecutionWriteContext::Normal,
        },
        Some(&port),
        "2026-09-22T00:00:00Z",
    );
    assert_eq!(cancel_response.status, 200, "{:?}", cancel_response.body);
    let modified = writer.modified.lock().expect("modified combos");
    assert_eq!(modified.len(), 1, "exactly one combo cancel attempt");
    assert_eq!(modified[0].operation, 2, "cancel uses Trd_ModifyOrder");
    assert_eq!(modified[0].order_id_ex.as_deref(), Some("COMBO-9001"));
    drop(modified);
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:83
/// `TestExecutionOrderRechecksRiskImmediatelyBeforeBrokerSubmission`.
///
/// Go evaluates pre-trade risk, consumes the preview credential and then
/// re-evaluates risk immediately before submission; a kill switch that
/// appeared in between rejects the placement (`placed == false`) while the
/// credential stays spent (`store.consumed == 1`). Rust keeps the same order:
/// the pre-consume evaluation reads the state the coordinator already owns, the
/// reservation then consumes the preview, and the submission gate re-reads the
/// control file so the kill switch activated in between is what rejects the
/// placement. The rejection is persisted as REJECTED without touching the
/// broker.
#[test]
fn real_option_combo_rechecks_risk_after_consuming_preview_before_submission() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(ComboPreviewTradeReader::default());
    let writer = Arc::new(RecordingTradeWriter::default());
    let control_directory = tempfile::tempdir().expect("control directory");
    let control_path = combo_lifecycle_control_file(control_directory.path(), false);
    let coordinator = Arc::new(crate::product::ExecutionRiskCoordinator::new(
        control_path,
    ));
    let port = combo_lifecycle_port_with_risk(
        &runtime,
        &reader,
        &writer,
        Arc::clone(&coordinator),
    );

    let payload = padded_real_option_combo_payload("client-risk-recheck");
    let preview = port.combo_preview(&payload).expect("combo preview");
    assert_eq!(preview["allowed"], true, "{preview}");
    let mut place_payload = payload.clone();
    place_payload["previewId"] = json!(preview["previewId"].as_str().expect("preview id"));

    // The operator activates the kill switch while the preview credential is
    // still in flight; only the submission-time re-read can see it.
    combo_lifecycle_control_file(control_directory.path(), true);

    let rejected = port
        .place_combo(&place_payload)
        .expect_err("kill switch activated after the preview must block submission");
    match &rejected {
        ExecutionWritePortError::Failed { status, code, .. } => {
            assert_eq!(*status, 403, "{rejected:?}");
            assert_eq!(code, "REAL_TRADE_KILL_SWITCH_ACTIVE", "{rejected:?}");
        }
        other => panic!("expected a kill-switch rejection, got {other:?}"),
    }
    assert!(
        writer.placed_combo.lock().expect("placed combos").is_empty(),
        "a rejected REAL combo must never reach the broker"
    );

    // The reservation already happened before the recheck, so the retry
    // replays the durable REJECTED projection instead of submitting again.
    let replayed = port
        .place_combo(&place_payload)
        .expect("identical replay returns the stored rejection");
    assert_eq!(replayed["status"], "REJECTED", "{replayed}");
    assert_eq!(replayed["lastErrorCode"], "REAL_TRADE_KILL_SWITCH_ACTIVE");
    assert_eq!(replayed["lastErrorSource"], "risk");
    assert!(
        writer.placed_combo.lock().expect("placed combos").is_empty(),
        "the replay must not issue a broker call"
    );

    // The preview credential is spent exactly once: a different client
    // identity reusing the same preview id is refused as PREVIEW_INVALID. The
    // kill switch has to be released through the production control-plane
    // mutation first, because Go also evaluates the pre-trade gateway before
    // it inspects the preview record.
    coordinator
        .mutate_with(|state| {
            state.kill_switch = None;
            Ok(())
        })
        .expect("release the kill switch through the control plane");
    let mut other_identity = padded_real_option_combo_payload("client-risk-recheck-2");
    other_identity["previewId"] = place_payload["previewId"].clone();
    let reused = port
        .place_combo(&other_identity)
        .expect_err("a spent preview must not bind a second client identity");
    match &reused {
        ExecutionWritePortError::Failed { status, code, .. } => {
            assert_eq!(*status, 400, "{reused:?}");
            assert_eq!(code, "PREVIEW_INVALID", "{reused:?}");
        }
        other => panic!("expected PREVIEW_INVALID, got {other:?}"),
    }
    assert!(
        writer.placed_combo.lock().expect("placed combos").is_empty(),
        "no combo submission may follow a consumed preview"
    );
}

/// Port whose owners are injected one by one so a failure matrix can remove the
/// store, the trade runtime, the writer or the risk coordinator.
fn combo_failure_port(
    store: Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    runtime: Option<Arc<SharedTradeReadRuntime>>,
    writer: Option<Arc<RecordingTradeWriter>>,
    risk_coordinator: Option<Arc<crate::product::ExecutionRiskCoordinator>>,
) -> ProductionExecutionPort {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    ProductionExecutionPort {
        store,
        active_provider_state: state,
        trade_logged_in: Some(true),
        trade_read_port: None,
        trade_write_port: writer.map(|value| value as Arc<dyn TradeWritePort>),
        trade_runtime: runtime,
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator,
        default_trading_environment: None,
        notification_projector: None,
    }
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:205
/// `TestExecutionComboPreviewKeepsLegacyBuyingPowerCompatible`.
///
/// Go's broker rule result carried only `AccountImpact.BuyingPowerDecrease`
/// (42.0) and no top-level `BuyingPowerImpact`; the service backfilled the
/// legacy field from the account impact (`preview.BuyingPowerImpact =
/// preview.AccountImpact.BuyingPowerDecrease`) while `accountImpact` stayed
/// populated. Rust derives both fields from the single OpenD
/// `Trd_GetComboMaxTrdQtys` read, so the equivalent contract is: a reader that
/// reports only `buyingPowerDecrease` must publish the legacy
/// `buyingPowerImpact` with that value, keep
/// `accountImpact.buyingPowerDecrease`, and never invent null placeholders for
/// the five impact fields OpenD did not send.
#[test]
fn combo_preview_backfills_legacy_buying_power_impact_from_account_impact_decrease() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(ComboPreviewTradeReader {
        buying_power_decrease_only: true,
        ..Default::default()
    });
    let writer = Arc::new(RecordingTradeWriter::default());
    let control_directory = tempfile::tempdir().expect("control directory");
    let control_path = combo_lifecycle_control_file(control_directory.path(), false);
    let port = combo_lifecycle_port(&runtime, &reader, &writer, control_path);

    let mut payload = option_combo_fixture_payload();
    payload["clientOrderId"] = json!("combo-legacy-impact");
    let preview = port.combo_preview(&payload).expect("combo preview");

    assert_eq!(preview["allowed"], true, "{preview}");
    assert_eq!(
        preview["buyingPowerImpact"].as_f64(),
        Some(42.0),
        "the legacy buyingPowerImpact mirrors AccountImpact.BuyingPowerDecrease: {preview}"
    );
    let impact = preview["accountImpact"]
        .as_object()
        .expect("accountImpact stays populated");
    assert_eq!(impact["buyingPowerDecrease"].as_f64(), Some(42.0));
    assert_eq!(
        impact.len(),
        1,
        "only the reported impact field may be published: {preview}"
    );
    assert!(
        preview["previewId"]
            .as_str()
            .is_some_and(|id| id.starts_with("preview-")),
        "an allowed combo preview still persists its credential: {preview}"
    );
    assert_eq!(
        reader.calls.lock().expect("combo preview calls").len(),
        1,
        "the combo preview reads Trd_GetComboMaxTrdQtys exactly once"
    );
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:237
/// `TestExecutionComboRejectsEveryUnsafeBoundary`.
///
/// Go walks ten unsafe intents (five option-combo, five event-parlay) plus one
/// placement without a preview id, and requires every one of them to fail as a
/// request error naming the violated rule while the broker stays untouched.
#[test]
fn combo_lifecycle_rejects_every_unsafe_boundary_before_the_broker() {
    type Case = (&'static str, fn(&mut Value), &'static str);

    fn assert_request_error(error: ExecutionWritePortError, part: &str, label: &str) {
        match &error {
            ExecutionWritePortError::Failed {
                status,
                code,
                message,
            } => {
                assert_eq!(*status, 400, "{label}: {error:?}");
                assert_eq!(code, "BAD_REQUEST", "{label}: {error:?}");
                assert!(message.contains(part), "{label}: {error:?}");
            }
            other => panic!("{label}: expected a request error, got {other:?}"),
        }
    }

    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(ComboPreviewTradeReader::default());
    let writer = Arc::new(RecordingTradeWriter::default());
    let control_directory = tempfile::tempdir().expect("control directory");
    let control_path = combo_lifecycle_control_file(control_directory.path(), false);
    let port = combo_lifecycle_port(&runtime, &reader, &writer, control_path);

    let combo_cases: [Case; 5] = [
        (
            "unknown kind",
            |value: &mut Value| value["orderKind"] = json!("single"),
            "orderKind",
        ),
        (
            "too few legs",
            |value: &mut Value| {
                value["legs"]
                    .as_array_mut()
                    .expect("legs")
                    .truncate(1);
            },
            "at least two",
        ),
        (
            "missing client",
            |value: &mut Value| {
                value
                    .as_object_mut()
                    .expect("payload")
                    .remove("clientOrderId");
            },
            "clientOrderId",
        ),
        (
            "mixed product",
            |value: &mut Value| value["legs"][1]["productClass"] = json!("event_contract"),
            "cannot mix",
        ),
        (
            "bad leg",
            |value: &mut Value| value["legs"][0]["ratio"] = json!(0),
            "each combo leg",
        ),
    ];
    for (label, mutate, part) in combo_cases {
        let mut payload = option_combo_fixture_payload();
        mutate(&mut payload);
        let error = port
            .combo_preview(&payload)
            .expect_err(&format!("{label} combo preview must be rejected"));
        assert_request_error(error, part, label);
    }

    let parlay_runtime = Arc::new(SharedTradeReadRuntime::default());
    let parlay_writer = Arc::new(RecordingTradeWriter::default());
    let parlay_port = event_parlay_place_port(&parlay_runtime, &parlay_writer);
    let parlay_cases: [Case; 4] = [
        (
            "missing expiry",
            |value: &mut Value| {
                value
                    .as_object_mut()
                    .expect("payload")
                    .remove("quoteExpiresAt");
            },
            "quote expired",
        ),
        (
            "missing rfq",
            |value: &mut Value| {
                value.as_object_mut().expect("payload").remove("rfqId");
            },
            "rfqId",
        ),
        (
            "zero amount",
            |value: &mut Value| value["amount"] = json!(0),
            "positive amount",
        ),
        (
            "missing prediction side",
            |value: &mut Value| {
                value["legs"][1]
                    .as_object_mut()
                    .expect("leg")
                    .remove("predictionSide");
            },
            "predictionSide",
        ),
    ];
    for (label, mutate, part) in parlay_cases {
        let mut payload = event_parlay_payload();
        mutate(&mut payload);
        let error = parlay_port
            .combo_preview(&payload)
            .expect_err(&format!("{label} parlay preview must be rejected"));
        assert_request_error(error, part, label);
    }

    // Go mutates only the market to HK while the legs keep their US prefixes.
    // Rust normalizes every leg against the requested market first, so that
    // doubly invalid intent fails one step earlier; both shapes stay request
    // errors and neither reaches the broker.
    let mut mismatched = event_parlay_payload();
    mismatched["market"] = json!("HK");
    let error = parlay_port
        .combo_preview(&mismatched)
        .expect_err("a HK market with US legs must fail");
    assert_request_error(error, "does not match symbol", "parlay market vs leg prefix");

    // A parlay that is consistently HK-quoted hits Go's own rule verbatim.
    let mut hk_parlay = event_parlay_payload();
    hk_parlay["market"] = json!("HK");
    hk_parlay["legs"][0]["instrumentId"] = json!("HK.EC.ONE");
    hk_parlay["legs"][1]["instrumentId"] = json!("HK.EC.TWO");
    let error = parlay_port
        .combo_preview(&hk_parlay)
        .expect_err("an event parlay must use the US market");
    assert_request_error(error, "market US", "parlay market");

    let mut no_preview = option_combo_fixture_payload();
    no_preview["clientOrderId"] = json!("combo-no-preview");
    let error = port
        .place_combo(&no_preview)
        .expect_err("placing a combo without a preview id must fail");
    assert_request_error(error, "previewId", "place without preview");

    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        0,
        "no unsafe option combo may reach the broker"
    );
    assert_eq!(
        parlay_writer.placed_combo.lock().expect("placed combos").len(),
        0,
        "no unsafe event parlay may reach the broker"
    );
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:319
/// `TestExecutionComboProviderStoreRiskAndGatewayFailures`.
///
/// Go walks the combo failure matrix: buying power without an active broker, a
/// broker without the product-rule/combo services, an unusable preview result,
/// a preview-store save failure, a consumed-preview error, a risk rejection
/// that must not spend the credential, and a missing combo gateway for both
/// placement and cancellation. Rust's engine is Futu-only, so "no broker" maps
/// onto the fail-closed OpenD/trade-owner absence (`Unavailable`) documented in
/// the provider rows; every other branch keeps Go's status codes.
#[test]
fn combo_lifecycle_surfaces_provider_store_risk_and_gateway_failures() {
    // Go: PreviewExecutionBuyingPower without an active broker.
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, false, false);
    let providerless = preview_port(Arc::clone(&state), None, None);
    let error = providerless
        .buying_power_preview(&buying_power_payload())
        .expect_err("buying power without the OpenD owner must fail closed");
    assert!(
        matches!(error, ExecutionWritePortError::Unavailable(_)),
        "buying power error = {error:?}"
    );

    let reader = Arc::new(ComboPreviewTradeReader::default());
    let writer = Arc::new(RecordingTradeWriter::default());
    let control_directory = tempfile::tempdir().expect("control directory");
    let control_path = combo_lifecycle_control_file(control_directory.path(), false);
    let (store, directory) = execution_store();
    let _ = directory.keep();

    // Go: a broker without ComboTradingService. Rust fails closed when the
    // option-strategy readers are missing instead of publishing allowed=true.
    let bare_runtime = Arc::new(SharedTradeReadRuntime::default());
    bare_runtime.set(
        Some(Arc::clone(&reader) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    bare_runtime.set_writer(Some(Arc::clone(&writer) as Arc<dyn TradeWritePort>));
    let unsupported = combo_failure_port(
        Arc::clone(&store),
        Some(Arc::clone(&bare_runtime)),
        Some(Arc::clone(&writer)),
        None,
    );
    let error = unsupported
        .combo_preview(&option_combo_fixture_payload())
        .expect_err("a combo preview without the strategy readers must fail closed");
    assert!(
        matches!(error, ExecutionWritePortError::Unavailable(_)),
        "unsupported combo error = {error:?}"
    );

    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_strategy_spread(Some(Arc::new(OptionSpreadFixture)));
    runtime.set_option_strategy_analysis(Some(Arc::new(OptionAnalysisFixture)));
    runtime.set(
        Some(Arc::clone(&reader) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    runtime.set_writer(Some(Arc::clone(&writer) as Arc<dyn TradeWritePort>));
    let coordinator = Arc::new(crate::product::ExecutionRiskCoordinator::new(
        control_path,
    ));
    let port = combo_failure_port(
        Arc::clone(&store),
        Some(Arc::clone(&runtime)),
        Some(Arc::clone(&writer)),
        Some(Arc::clone(&coordinator)),
    );

    // Go: the preview store reports "already consumed", so placement fails as a
    // request error instead of silently reusing the credential.
    let mut unknown_preview = option_combo_fixture_payload();
    unknown_preview["clientOrderId"] = json!("combo-consumed");
    unknown_preview["previewId"] = json!("preview-missing");
    let error = port
        .place_combo(&unknown_preview)
        .expect_err("an unusable preview credential must fail");
    match &error {
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(*status, 400, "{error:?}");
            assert_eq!(code, "PREVIEW_INVALID", "{error:?}");
            assert!(message.contains("preview-missing"), "{error:?}");
        }
        other => panic!("expected PREVIEW_INVALID, got {other:?}"),
    }

    // Go: a preview-store save failure must surface instead of an allowed
    // preview. Dropping the preview table models the same store fault.
    let (broken_store, broken_directory) = execution_store();
    let broken_database = broken_directory.path().join("execution-preview.db");
    let connection =
        rusqlite::Connection::open(&broken_database).expect("reopen execution database");
    connection
        .execute("DROP TABLE execution_order_previews", [])
        .expect("drop preview table");
    drop(connection);
    let broken_runtime = Arc::new(SharedTradeReadRuntime::default());
    broken_runtime.set_option_strategy_spread(Some(Arc::new(OptionSpreadFixture)));
    broken_runtime.set_option_strategy_analysis(Some(Arc::new(OptionAnalysisFixture)));
    broken_runtime.set(
        Some(Arc::new(ComboPreviewTradeReader::default()) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    broken_runtime.set_writer(Some(Arc::new(RecordingTradeWriter::default()) as Arc<dyn TradeWritePort>));
    let broken = combo_failure_port(broken_store, Some(broken_runtime), None, None);
    let error = broken
        .combo_preview(&option_combo_fixture_payload())
        .expect_err("a preview-store save failure must not publish an allowed preview");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 500, code, .. } if code == "EXECUTION_STORE_ERROR"
        ),
        "preview save error = {error:?}"
    );

    // Go: a risk rejection must not spend the preview credential. Rust
    // evaluates the gateway before reserving the credential and again inside
    // the submission gate, so releasing the kill switch lets the same
    // credential place the order.
    let real_payload = padded_real_option_combo_payload("client-hard-reject");
    let preview = port.combo_preview(&real_payload).expect("combo preview");
    let mut place_real = real_payload.clone();
    place_real["previewId"] = json!(preview["previewId"].as_str().expect("preview id"));
    coordinator
        .mutate_with(|state| {
            state.kill_switch = Some(jftrade_trading::RealTradeKillSwitchEntry {
                id: "ks-combo-failures".to_owned(),
                trading_environment: "REAL".to_owned(),
                operator_id: "ops".to_owned(),
                reason: "combo staging halt".to_owned(),
                activated_at: "2026-09-22T00:00:00Z".to_owned(),
                updated_at: "2026-09-22T00:00:00Z".to_owned(),
            });
            Ok(())
        })
        .expect("activate kill switch");
    let rejected = port
        .place_combo(&place_real)
        .expect_err("an active kill switch must reject the REAL combo");
    match &rejected {
        ExecutionWritePortError::Failed { status, code, .. } => {
            assert_eq!(*status, 403, "{rejected:?}");
            assert_eq!(code, "REAL_TRADE_KILL_SWITCH_ACTIVE", "{rejected:?}");
        }
        other => panic!("expected a kill-switch rejection, got {other:?}"),
    }
    assert!(
        writer.placed_combo.lock().expect("placed combos").is_empty(),
        "a risk-rejected combo must never reach the broker"
    );
    coordinator
        .mutate_with(|state| {
            state.kill_switch = None;
            Ok(())
        })
        .expect("release kill switch");
    let placed = port
        .place_combo(&place_real)
        .expect("a hard risk rejection must not spend the preview credential");
    assert_eq!(placed["status"], "SUBMITTED", "{placed}");
    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        1,
        "the retried credential submits exactly once"
    );

    // Go: ErrOrderGatewayUnavailable for a missing combo gateway, on both the
    // placement and the cancellation route.
    let unwired_runtime = Arc::new(SharedTradeReadRuntime::default());
    unwired_runtime.set(
        Some(Arc::clone(&reader) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    let gatewayless = combo_failure_port(
        Arc::clone(&port.store),
        Some(unwired_runtime),
        None,
        Some(Arc::clone(&coordinator)),
    );
    let mut next = padded_real_option_combo_payload("client-gatewayless");
    let next_preview = port.combo_preview(&next).expect("second combo preview");
    next["previewId"] = json!(next_preview["previewId"].as_str().expect("preview id"));
    let error = gatewayless
        .place_combo(&next)
        .expect_err("a missing combo gateway must fail the placement");
    assert!(
        matches!(error, ExecutionWritePortError::Unavailable(_)),
        "missing gateway place error = {error:?}"
    );
    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal combo order id");
    let error = gatewayless
        .cancel_order(internal_id)
        .expect_err("a missing combo gateway must fail the cancel");
    assert!(
        matches!(error, ExecutionWritePortError::Unavailable(_)),
        "missing gateway cancel error = {error:?}"
    );
}

/// Port that can preview and place an event parlay: the prediction adapters
/// answer the RFQ/contract reads and the scripted writer records submissions.
fn event_parlay_place_port(
    runtime: &Arc<SharedTradeReadRuntime>,
    writer: &Arc<RecordingTradeWriter>,
) -> ProductionExecutionPort {
    runtime.set_prediction_adapters(
        Some(Arc::new(PredictionReadFixture {
            entries: vec![
                event_contract_entry("EC.ONE", 2),
                event_contract_entry("EC.TWO", 2),
            ],
            error: None,
        })),
        None,
        Some(Arc::new(ComboQuoteFixture)),
    );
    // `writer()` reads the logged-in flag from the same runtime snapshot as the
    // trade reader, so the parlay fixture installs an inert reader alongside it.
    runtime.set(
        Some(Arc::new(ComboPreviewTradeReader::default()) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    runtime.set_writer(Some(Arc::clone(writer) as Arc<dyn TradeWritePort>));
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
        trade_runtime: Some(Arc::clone(runtime)),
        cancel_inflight: Arc::new(std::sync::Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    }
}

/// Persist the RFQ that the market-data route would have stored for `payload`.
fn stored_parlay_rfq(quote_id: &str, payload: &Value) -> jftrade_store_sqlite::StoredPredictionQuote {
    let (mvc, legs_hash) = crate::product::product_production_ports::product_production_ports_market_data::product_production_ports_market_data_actions::product_prediction_combo_quote::prediction_quote_binding(payload)
        .expect("prediction quote binding");
    jftrade_store_sqlite::StoredPredictionQuote {
        quote_id: quote_id.to_owned(),
        broker_id: "futu".to_owned(),
        account_id: "42".to_owned(),
        trading_environment: "SIMULATE".to_owned(),
        mvc,
        legs_hash,
        bid_price: Some(0.4),
        ask_price: Some(0.5),
        should_retry: false,
        received_at: "2026-09-22T00:00:00Z".to_owned(),
        expires_at: "2999-01-01T00:00:00Z".to_owned(),
        expiry_source: "jftrade_policy".to_owned(),
        status: "active".to_owned(),
        consumed_at: None,
        consumed_preview_id: None,
        consumed_client_order_id: None,
    }
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:110
/// `TestExecutionEventParlayCompletePreviewAndAmountRisk`.
///
/// Go previews an event parlay, places it, and then asserts that the intent
/// handed to the combo gateway keeps `RFQID` while the pre-trade gateway sees
/// `Query.Quantity == amount` with `QuantityMode == amount`. Rust keeps the RFQ
/// binding on the combo request (`quoteId`) and the size in amount mode on the
/// risk command, and the stored prediction RFQ is the durable credential Go
/// consumes for exactly one preview/client-order pair.
#[test]
fn event_parlay_preview_place_and_amount_risk_keep_go_contract() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = event_parlay_place_port(&runtime, &writer);

    let payload = event_parlay_payload();
    port.store
        .save_prediction_quote(&stored_parlay_rfq("rfq-1", &payload))
        .expect("save prediction RFQ");

    let preview = port.combo_preview(&payload).expect("parlay preview");
    assert_eq!(preview["allowed"], true, "{preview}");
    assert_eq!(preview["productClass"], "event_contract");
    assert_eq!(preview["orderKind"], "event_parlay");
    let preview_id = preview["previewId"]
        .as_str()
        .expect("parlay preview id")
        .to_owned();

    // Go asserts the gateway command shape: the parlay is sized by `amount`,
    // not by a contract count, and carries no caller price.
    let parsed = super::execution_order_parse::parse_combo(&payload).expect("parsed parlay");
    let risk_order = super::execution_order_helpers::build_pre_trade_risk_combo_order(&parsed);
    assert_eq!(risk_order.quantity_mode, "amount");
    assert_eq!(risk_order.quantity.to_string(), "10");
    assert_eq!(
        risk_order.amount.map(|value| value.to_string()),
        Some("10".to_owned())
    );
    assert_eq!(risk_order.price, None);

    let mut place_payload = payload.clone();
    place_payload["previewId"] = json!(preview_id);
    let placed = port.place_combo(&place_payload).expect("parlay place");
    assert_eq!(placed["status"], "SUBMITTED", "{placed}");
    let submitted = writer.placed_combo.lock().expect("placed combos");
    assert_eq!(submitted.len(), 1, "exactly one parlay submission");
    assert_eq!(
        submitted[0].quote_id.as_deref(),
        Some("rfq-1"),
        "Go keeps intent.RFQID on the submitted combo"
    );
    drop(submitted);

    // The RFQ funds exactly one preview/client-order pair; a second parlay that
    // reuses it must fail closed without another broker call.
    let mut second = event_parlay_payload();
    second["clientOrderId"] = json!("parlay-2");
    let second_preview = port.combo_preview(&second).expect("second parlay preview");
    second["previewId"] = json!(second_preview["previewId"].as_str().expect("preview id"));
    let reused = port
        .place_combo(&second)
        .expect_err("a consumed RFQ must not price a second parlay");
    match &reused {
        ExecutionWritePortError::Failed { status, message, .. } => {
            assert_eq!(*status, 400, "{reused:?}");
            assert!(message.contains("already consumed"), "{reused:?}");
        }
        other => panic!("expected a consumed-RFQ rejection, got {other:?}"),
    }
    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        1,
        "a rejected parlay must never reach the broker"
    );

    // An RFQ id the server never issued cannot price a parlay either.
    let mut unknown = event_parlay_payload();
    unknown["clientOrderId"] = json!("parlay-3");
    unknown["rfqId"] = json!("rfq-missing");
    let unknown_preview = port.combo_preview(&unknown).expect("third parlay preview");
    unknown["previewId"] = json!(unknown_preview["previewId"].as_str().expect("preview id"));
    let missing = port
        .place_combo(&unknown)
        .expect_err("an unknown RFQ must fail closed");
    match &missing {
        ExecutionWritePortError::Failed { status, message, .. } => {
            assert_eq!(*status, 400, "{missing:?}");
            assert!(message.contains("prediction RFQ"), "{missing:?}");
        }
        other => panic!("expected an invalid-RFQ rejection, got {other:?}"),
    }
    assert_eq!(
        writer.placed_combo.lock().expect("placed combos").len(),
        1,
        "no submission may follow an invalid RFQ"
    );
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:412
/// `TestExecutionComboHelperBranches`.
///
/// Go's helper matrix: `comboRiskQuantity` prefers the parlay amount, then the
/// first leg quantity, then 1; `comboQuantityMode` maps an event parlay to
/// amount mode and an option combo to contracts; `normalizedComboIntent` keeps
/// the client order id; and an empty combo gateway reports
/// `ErrOrderGatewayUnavailable` for place and cancel.
#[test]
fn combo_helper_branches_keep_go_risk_quantity_quantity_mode_and_gateway_contract() {
    let parlay_payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "clientOrderId": "helper-parlay",
        "orderKind": "event_parlay",
        "productClass": "event_contract",
        "rfqId": "rfq-helper",
        "mvc": "US.MVC",
        "quoteExpiresAt": "2999-01-01T00:00:00Z",
        "amount": 15.0,
        "legs": [
            {"instrumentId": "US.EC.ONE", "side": "BUY", "ratio": 1, "quantity": 3, "predictionSide": "YES"},
            {"instrumentId": "US.EC.TWO", "side": "SELL", "ratio": 1, "quantity": 3, "predictionSide": "NO"}
        ]
    });
    let parlay = super::execution_order_parse::parse_combo(&parlay_payload).expect("parlay intent");
    let parlay_risk = super::execution_order_helpers::build_pre_trade_risk_combo_order(&parlay);
    assert_eq!(
        parlay_risk.quantity.to_string(),
        "15",
        "the parlay amount is the risk quantity"
    );
    assert_eq!(parlay_risk.quantity_mode, "amount");

    let combo_payload = option_combo_fixture_payload();
    let combo = super::execution_order_parse::parse_combo(&combo_payload).expect("option combo intent");
    let combo_risk = super::execution_order_helpers::build_pre_trade_risk_combo_order(&combo);
    assert_eq!(
        combo_risk.quantity.to_string(),
        "2",
        "the first leg quantity is the combo risk quantity"
    );
    assert_eq!(combo_risk.quantity_mode, "contracts");

    let mut bare_payload = option_combo_fixture_payload();
    for leg in bare_payload["legs"].as_array_mut().expect("legs") {
        leg.as_object_mut().expect("leg").remove("quantity");
    }
    let bare = super::execution_order_parse::parse_combo(&bare_payload).expect("combo without leg quantities");
    assert_eq!(
        super::execution_order_helpers::build_pre_trade_risk_combo_order(&bare)
            .quantity
            .to_string(),
        "1",
        "a combo without sizes falls back to one"
    );

    let canonical = super::canonical_execution_request(
        &combo_payload,
        &combo.order,
        Some(super::execution_order_previews::canonical_combo_legs(&combo)),
    )
    .expect("canonical combo intent");
    assert!(
        canonical.contains("combo-transport"),
        "the normalized combo intent keeps the client order id: {canonical}"
    );

    let (store, directory) = execution_store();
    let _ = directory.keep();
    let gatewayless = combo_failure_port(store, None, None, None);
    let mut place_payload = combo_payload.clone();
    place_payload["previewId"] = json!("preview-helper");
    let error = gatewayless
        .place_combo(&place_payload)
        .expect_err("an empty combo gateway must fail closed");
    assert!(
        matches!(error, ExecutionWritePortError::Unavailable(_)),
        "empty combo gateway error = {error:?}"
    );
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:441
/// `TestExecutionProductPreviewAndSubmissionFailureContractsComplete`.
///
/// Go walks the single-order preview/submission failure contracts: a preview id
/// without a client order id, the prediction request validation matrix, and the
/// requirement that a risk rejection never spends the preview credential. The
/// broker-registry branches (`resolveExecutionBroker` without a default broker
/// or with a mismatched one) are Go's multi-broker registry; Rust is Futu-only
/// and defaults `brokerId`, and the product-rule denials plus the prediction
/// account eligibility are covered by `product_rule_denials_return_the_go_reason_code_matrix`
/// and `prediction_account_eligibility` respectively.
#[test]
fn single_and_prediction_submission_failure_contracts_keep_go_boundaries() {
    let writer = Arc::new(RecordingTradeWriter::default());
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    state.set_readiness(false, true, false);
    let port = preview_port_with_writer(state, None, Some(true), Some(Arc::clone(&writer)));

    let mut equity_preview = cancel_contract_payload("equity-preview-client");
    equity_preview["previewId"] = json!("preview-equity");
    equity_preview
        .as_object_mut()
        .expect("payload")
        .remove("clientOrderId");
    let error = port
        .place_order(&equity_preview)
        .expect_err("an equity preview without a client order id must fail");
    match &error {
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(*status, 400, "{error:?}");
            assert_eq!(code, "BAD_REQUEST", "{error:?}");
            assert!(message.contains("clientOrderId"), "{error:?}");
        }
        other => panic!("expected a request error, got {other:?}"),
    }

    let option_preview = json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "symbol": "US.AAPL260717C00200000",
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 1,
        "price": 2.5,
        "productClass": "option",
        "previewId": "preview-locked",
    });
    let error = port
        .place_order(&option_preview)
        .expect_err("a locked derivative preview without a client order id must fail");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, message }
                if code == "BAD_REQUEST" && message.contains("clientOrderId")
        ),
        "locked preview error = {error:?}"
    );

    // Go's prediction validation matrix: amount, predictionSide and price are
    // all required by the event-contract endpoint.
    // Go's fixtures leave `productClass` empty, so both implementations reject
    // them in the shared single-order gates (Go: "quantity must be greater than
    // 0", Rust: "quantity is required"); the Go assertion only requires a
    // request error for each case.
    let prediction = |product_class: Value, amount: Value, side: &str, price: Value| {
        json!({
            "accountId": "1001",
            "brokerId": "futu",
            "market": "US",
            "tradingEnvironment": "SIMULATE",
            "symbol": "US.EVENT",
            "side": "BUY",
            "orderType": "LIMIT",
            "productClass": product_class,
            "amount": amount,
            "predictionSide": side,
            "price": price,
            "clientOrderId": "prediction-client",
        })
    };
    for (label, payload) in [
        (
            "missing amount",
            prediction(Value::Null, Value::Null, "YES", json!(0.5)),
        ),
        (
            "invalid side",
            prediction(Value::Null, json!(10.0), "MAYBE", json!(0.5)),
        ),
        (
            "missing price",
            prediction(Value::Null, json!(10.0), "YES", Value::Null),
        ),
    ] {
        let error = super::execution_order_parse::parse_order(&payload).expect_err(label);
        assert!(error.contains("quantity"), "{label} error = {error:?}");
    }
    let mut non_us = prediction(Value::Null, json!(10.0), "YES", json!(0.5));
    non_us["market"] = json!("HK");
    non_us["symbol"] = json!("HK.EVENT");
    let error = super::execution_order_parse::parse_order(&non_us)
        .expect_err("a non-US prediction contract must fail");
    assert!(
        error.contains("quantity"),
        "non-US prediction error = {error:?}"
    );

    // The explicit event-contract shape reaches Rust's prediction validator,
    // which owns the same three rules plus the US-market gate.
    for (label, payload, part) in [
        (
            "missing amount",
            prediction(json!("event_contract"), Value::Null, "YES", json!(0.5)),
            "amount",
        ),
        (
            "invalid side",
            prediction(json!("event_contract"), json!(10.0), "MAYBE", json!(0.5)),
            "predictionSide",
        ),
        (
            "missing price",
            prediction(json!("event_contract"), json!(10.0), "YES", Value::Null),
            "price",
        ),
    ] {
        let error = super::execution_order_parse::parse_order(&payload).expect_err(label);
        assert!(error.contains(part), "{label} error = {error:?}");
    }
    let mut explicit_non_us = prediction(
        json!("event_contract"),
        json!(10.0),
        "YES",
        json!(0.5),
    );
    explicit_non_us["market"] = json!("HK");
    explicit_non_us["symbol"] = json!("HK.EVENT");
    let error = super::execution_order_parse::parse_order(&explicit_non_us)
        .expect_err("a non-US prediction contract must fail");
    assert!(
        error.contains("market US"),
        "non-US prediction error = {error:?}"
    );

    // Go: a risk rejection must not spend the single-order preview credential.
    let control_directory = tempfile::tempdir().expect("control directory");
    let control_path = control_directory.path().join("real-trade-control.json");
    std::fs::write(
        &control_path,
        json!({"riskConfig": {"realTradingEnabled": true, "maxOrderQuantity": 100.0}}).to_string(),
    )
    .expect("write real-trade control file");
    let coordinator = Arc::new(crate::product::ExecutionRiskCoordinator::new(
        control_path,
    ));
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let risk_port = combo_failure_port(
        store,
        None,
        Some(Arc::clone(&writer)),
        Some(Arc::clone(&coordinator)),
    );
    let mut real_payload = cancel_contract_payload("single-hard-reject");
    real_payload["tradingEnvironment"] = json!("REAL");
    let preview = risk_port
        .order_preview(&real_payload)
        .expect("single-order preview");
    real_payload["previewId"] = json!(preview["previewId"].as_str().expect("preview id"));
    coordinator
        .mutate_with(|state| {
            state.kill_switch = Some(jftrade_trading::RealTradeKillSwitchEntry {
                id: "ks-single-failures".to_owned(),
                trading_environment: "REAL".to_owned(),
                operator_id: "ops".to_owned(),
                reason: "single-order staging halt".to_owned(),
                activated_at: "2026-09-22T00:00:00Z".to_owned(),
                updated_at: "2026-09-22T00:00:00Z".to_owned(),
            });
            Ok(())
        })
        .expect("activate kill switch");
    let rejected = risk_port
        .place_order(&real_payload)
        .expect_err("an active kill switch must reject the REAL single order");
    assert!(
        matches!(
            &rejected,
            ExecutionWritePortError::Failed { status: 403, code, .. }
                if code == "REAL_TRADE_KILL_SWITCH_ACTIVE"
        ),
        "single risk rejection = {rejected:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "a risk-rejected single order must never reach the broker"
    );
    coordinator
        .mutate_with(|state| {
            state.kill_switch = None;
            Ok(())
        })
        .expect("release kill switch");
    let placed = risk_port
        .place_order(&real_payload)
        .expect("a hard risk rejection must not spend the single-order preview");
    assert_eq!(placed["status"], "SUBMITTED", "{placed}");
    assert_eq!(
        writer.placed.lock().expect("placed orders").len(),
        1,
        "the retried single-order credential submits exactly once"
    );
}

/// Parity: go:452dea11:internal/trading/execution_products_test.go:128
/// `TestRealFuturesPreviewRequiresFuturesAuthority`.
///
/// Go's `validateFuturesTradingAuthority`: a REAL futures preview needs an
/// account whose market authorities include FUTURES; discovery failures and
/// accounts without the authority stay request errors.
#[test]
fn real_futures_preview_requires_futures_authority() {
    // Go's `validateFuturesTradingAuthority`: a REAL futures preview needs an
    // account whose market authorities include FUTURES; discovery failures and
    // accounts without the authority stay request errors.
    fn futures_account(id: u64, authorities: Vec<i32>) -> TradeAccountSnapshot {
        TradeAccountSnapshot {
            trd_env: 1,
            acc_id: id,
            trd_market_auth_list: authorities,
            acc_type: None,
            card_num: None,
            security_firm: Some(2),
            sim_acc_type: None,
            uni_card_num: None,
            acc_status: None,
            acc_role: None,
            jp_acc_type: Vec::new(),
            competition_acc_name: None,
        }
    }
    let futures_payload = json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "REAL",
        "clientOrderId": "future-client",
        "symbol": "US.ESZ6",
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 1,
        "price": 5500.0,
        "productClass": "future",
    });
    for (label, accounts, expected) in [
        (
            "missing FUTURES authority",
            vec![futures_account(1001, vec![2])],
            Some("FUTURES account authority"),
        ),
        (
            "other account carries FUTURES authority",
            vec![futures_account(2002, vec![5])],
            Some("FUTURES account authority"),
        ),
    ] {
        let runtime = Arc::new(SharedTradeReadRuntime::default());
        runtime.set(
            Some(Arc::new(ComboPreviewTradeReader {
                accounts,
                ..Default::default()
            }) as Arc<dyn TradeReadPort>),
            Some(true),
        );
        let (store, directory) = execution_store();
        let _ = directory.keep();
        let futures_port = combo_failure_port(store, Some(runtime), None, None);
        let error = futures_port
            .order_preview(&futures_payload)
            .expect_err(label);
        match (&error, expected) {
            (ExecutionWritePortError::Failed { status, message, .. }, Some(part)) => {
                assert_eq!(*status, 400, "{label}: {error:?}");
                assert!(message.contains(part), "{label}: {error:?}");
            }
            _ => panic!("{label}: expected a request error, got {error:?}"),
        }
    }
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(
        Some(Arc::new(ComboPreviewTradeReader {
            accounts: vec![futures_account(1001, vec![5])],
            ..Default::default()
        }) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let futures_port = combo_failure_port(store, Some(runtime), None, None);
    let preview = futures_port
        .order_preview(&futures_payload)
        .expect("a FUTURES-authority account is eligible");
    assert!(
        preview["previewId"]
            .as_str()
            .is_some_and(|id| id.starts_with("preview-")),
        "eligible futures preview = {preview}"
    );
}

/// Parity: go:452dea11:internal/trading/execution_products_test.go:12
/// `TestDerivativeSingleLegRequiresBrokerPreviewAndStableClientID`.
///
/// Go's derivative single-leg contract: the preview itself must carry a
/// clientOrderId, an allowed preview keeps the derivative product class, and a
/// placement without the locked preview id is rejected before the broker runs.
#[test]
fn derivative_single_leg_preview_requires_client_order_id_and_locks_the_place() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(
        Some(Arc::new(ComboPreviewTradeReader::default()) as Arc<dyn TradeReadPort>),
        Some(true),
    );
    let writer = Arc::new(RecordingTradeWriter::default());
    let (store, directory) = execution_store();
    let _ = directory.keep();
    let port = combo_failure_port(
        Arc::clone(&store),
        Some(runtime),
        Some(Arc::clone(&writer)),
        None,
    );

    let option_payload = json!({
        "accountId": "42",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "symbol": "US.AAPL260717C00200000",
        "productClass": "option",
        "orderKind": "single",
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 1,
        "price": 2.25
    });
    let error = port
        .order_preview(&option_payload)
        .expect_err("a derivative preview without a clientOrderId must fail");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, message }
                if code == "BAD_REQUEST" && message.contains("clientOrderId")
        ),
        "derivative preview error = {error:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "a rejected preview must not reach the broker"
    );

    let mut locked = option_payload.clone();
    locked["clientOrderId"] = json!("option-client-1");
    let preview = port.order_preview(&locked).expect("option preview");
    assert_eq!(preview["productClass"], "option", "{preview}");
    assert!(
        preview["previewId"]
            .as_str()
            .is_some_and(|id| id.starts_with("preview-")),
        "{preview}"
    );

    let mut place = locked.clone();
    place.as_object_mut().expect("payload").remove("previewId");
    let error = port
        .place_order(&place)
        .expect_err("placing a derivative without the locked preview must fail");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, message }
                if code == "BAD_REQUEST" && message.contains("previewId")
        ),
        "derivative place error = {error:?}"
    );
    assert!(
        writer.placed.lock().expect("placed orders").is_empty(),
        "no derivative order may reach the broker without a locked preview"
    );
}

/// Parity: go:452dea11:internal/trading/execution_products_test.go:149
/// `TestComboPreviewRejectsMixedProductsAndExpiredParlayRFQ`.
///
/// The mixed-product half is pinned by
/// `combo_lifecycle_rejects_every_unsafe_boundary_before_the_broker`; this test
/// pins the elapsed-deadline half: a parlay whose `quoteExpiresAt` already
/// passed fails as a request error and never reaches the combo gateway.
#[test]
fn expired_parlay_rfq_is_rejected_before_the_combo_gateway() {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = event_parlay_place_port(&runtime, &writer);
    let mut payload = event_parlay_payload();
    payload["quoteExpiresAt"] = json!("2020-01-01T00:00:00Z");
    let error = port
        .combo_preview(&payload)
        .expect_err("an expired parlay RFQ must be rejected");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, message }
                if code == "BAD_REQUEST" && message.contains("request a new RFQ")
        ),
        "expired parlay error = {error:?}"
    );
    assert!(
        writer.placed_combo.lock().expect("placed combos").is_empty(),
        "an expired RFQ must never reach the combo gateway"
    );
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:571
/// `TestExecutionProductRemainingLifecycleAndUpdateHelpers`.
///
/// Go covers the remaining lifecycle helpers: a preview whose store save fails
/// must surface the store error, a combo intent must be refused by the
/// single-order endpoint, fractional contract quantities and option
/// extended-hours sessions are request errors, and the canonical
/// stored/reconciled status helpers keep their mapping.
#[test]
fn execution_preview_and_order_state_helpers_keep_go_boundaries() {
    let (store, directory) = execution_store();
    let database = directory.path().join("execution-preview.db");
    let connection = rusqlite::Connection::open(&database).expect("reopen execution database");
    connection
        .execute("DROP TABLE execution_order_previews", [])
        .expect("drop preview table");
    drop(connection);
    let port = combo_failure_port(store, None, None, None);
    let error = port
        .order_preview(&cancel_contract_payload("preview-storage-failure"))
        .expect_err("a preview storage failure must not be hidden");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 500, code, .. }
                if code == "EXECUTION_STORE_ERROR"
        ),
        "preview storage error = {error:?}"
    );

    let mut combo_on_single = option_combo_fixture_payload();
    combo_on_single["previewId"] = json!("preview-single-endpoint");
    let error = port
        .order_preview(&combo_on_single)
        .expect_err("a combo intent must not use the single-order endpoint");
    assert!(
        matches!(
            &error,
            ExecutionWritePortError::Failed { status: 400, code, message }
                if code == "BAD_REQUEST"
                    && message.contains("must use the combo execution endpoint")
        ),
        "single-endpoint combo error = {error:?}"
    );

    let mut fractional = json!({
        "accountId": "1001",
        "brokerId": "futu",
        "market": "US",
        "tradingEnvironment": "SIMULATE",
        "symbol": "US.AAPL260717C00200000",
        "side": "BUY",
        "orderType": "LIMIT",
        "quantity": 1.5,
        "price": 2.5,
        "productClass": "option",
    });
    let error =
        super::execution_order_parse::parse_order(&fractional).expect_err("fractional contracts must fail");
    assert!(
        error.contains("integer number of contracts"),
        "fractional option quantity error = {error:?}"
    );
    fractional["quantity"] = json!(1);
    fractional["session"] = json!("ETH");
    let error = super::execution_order_parse::parse_order(&fractional)
        .expect_err("options must not use extended hours");
    assert!(
        error.contains("extended-hours"),
        "option session error = {error:?}"
    );

    assert_eq!(
        jftrade_trading::canonical_stored_status("SUBMISSION_UNKNOWN"),
        OrderStatus::SubmissionUnknown
    );
    assert_eq!(
        jftrade_trading::canonical_stored_status("not-a-broker-status"),
        OrderStatus::Unknown
    );
    assert_eq!(
        jftrade_trading::reconcile_status(OrderStatus::Submitted, OrderStatus::Submitted),
        (OrderStatus::Submitted, true)
    );
}

/// Parity: go:452dea11:internal/trading/execution_combo_lifecycle_test.go:635
/// `TestExecutionDetailsResolverAndOrderUpdateCacheFailureBranches`.
///
/// Go covers the order-details resolver boundaries and the legacy order-update
/// worker cache. Rust owns the details read through the production read port:
/// an unusable ledger surface must propagate a failure instead of an empty
/// detail payload. Go's `resolveExecutionBroker`/`OrderUpdatesWorker` cache
/// branches belong to the Go multi-broker registry and the Wails-era worker;
/// Rust's owner is the Futu-only port plus the reconciliation worker
/// (`reconciliation_discovery_deduplicates_repeated_pages_and_keeps_newest_snapshot`),
/// so those stay documented boundaries.
#[test]
fn execution_details_read_boundaries_keep_go_failure_propagation() {
    let (store, directory) = execution_store();
    let database = directory.path().join("execution-preview.db");
    let _ = directory.keep();
    let writer = Arc::new(RecordingTradeWriter::default());
    let port = combo_failure_port(store, None, Some(Arc::clone(&writer)), None);
    let placed = port
        .place_order(&cancel_contract_payload("details-boundary"))
        .expect("place order");
    let internal_id = placed["internalOrderId"]
        .as_str()
        .expect("internal order id")
        .to_owned();

    // Go: a blank identifier is a request error. Rust decodes it away and
    // reports the order as missing; both refuse to read the ledger.
    let error = port
        .read("/api/v1/execution/orders/%20", "")
        .expect_err("a blank order id must not resolve");
    assert!(matches!(error, ExecutionReadSnapshotError::NotFound), "{error:?}");

    let details = port
        .read(&format!("/api/v1/execution/orders/{internal_id}"), "")
        .expect("order details");
    assert_eq!(details["order"]["internalOrderId"], json!(internal_id));
    assert!(
        details["recentEvents"].as_array().is_some(),
        "details keep the recent event projection: {details}"
    );

    // Go: a list/get failure must propagate instead of an empty detail body.
    let connection = rusqlite::Connection::open(&database).expect("reopen execution database");
    connection
        .execute("DROP TABLE execution_order_events", [])
        .expect("drop order events table");
    drop(connection);
    let error = port
        .read(&format!("/api/v1/execution/orders/{internal_id}"), "")
        .expect_err("a broken event ledger must surface");
    assert!(
        matches!(
            &error,
            ExecutionReadSnapshotError::Failed { code, .. } if code == "GET_ORDER_FAILED"
        ),
        "broken events error = {error:?}"
    );
}
