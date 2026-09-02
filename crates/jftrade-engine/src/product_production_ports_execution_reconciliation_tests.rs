use super::*;

use crate::product::ActiveProviderState;
use jftrade_integration_futu::{
    TradeAccountSnapshot, TradeCashFlowSnapshot, TradeFundsSnapshot, TradeHeader,
    TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest, TradeMaxTradeQuantitySnapshot,
    TradePositionSnapshot, TradeSecurity, TradeSessionError,
};
use jftrade_settings::MarketDataProvider;
use std::sync::{Arc, Mutex};

#[derive(Debug, Default)]
struct FixtureTradeReader {
    accounts: Vec<TradeAccountSnapshot>,
    active_orders: Vec<TradeOrderSnapshot>,
    history_orders: Vec<TradeOrderSnapshot>,
    active_fills: Vec<TradeFillSnapshot>,
    history_fills: Vec<TradeFillSnapshot>,
    fees: Vec<TradeOrderFeeSnapshot>,
    funds: Option<TradeFundsSnapshot>,
    fail_accounts: bool,
    fail_active_orders: bool,
    fail_history_orders: bool,
    fee_batches: Mutex<Vec<Vec<TradeOrderFeeSnapshot>>>,
    calls: Mutex<FixtureCallCounts>,
}

#[derive(Debug, Default, Clone, Copy)]
struct FixtureCallCounts {
    accounts: usize,
    active_orders: usize,
    history_orders: usize,
    active_fills: usize,
    history_fills: usize,
    fees: usize,
}

fn unavailable<T>(message: &str) -> Result<T, TradeSessionError> {
    Err(TradeSessionError::Unsupported(message.to_owned()))
}

impl TradeReadPort for FixtureTradeReader {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        self.calls.lock().expect("fixture calls").accounts += 1;
        if self.fail_accounts {
            return unavailable("fixture accounts unavailable");
        }
        Ok(self.accounts.clone())
    }

    fn read_funds(
        &self,
        _: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        self.funds
            .clone()
            .ok_or_else(|| TradeSessionError::Unsupported("fixture funds unsupported".to_owned()))
    }

    fn read_cash_flows(
        &self,
        _: TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        unavailable("fixture cash flows unsupported")
    }

    fn read_order_fees(
        &self,
        _: TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        self.calls.lock().expect("fixture calls").fees += 1;
        if let Some(batch) = self.fee_batches.lock().expect("fixture fee batches").pop() {
            return Ok(batch);
        }
        Ok(self.fees.clone())
    }

    fn read_margin_ratios(
        &self,
        _: TradeHeader,
        _: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        unavailable("fixture margin ratios unsupported")
    }

    fn read_max_trade_quantity(
        &self,
        _: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        unavailable("fixture max quantity unsupported")
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
        unavailable("fixture positions unsupported")
    }

    fn read_orders(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        self.calls.lock().expect("fixture calls").active_orders += 1;
        if self.fail_active_orders {
            return unavailable("fixture active orders unavailable");
        }
        Ok(self.active_orders.clone())
    }

    fn read_history_orders(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        self.calls.lock().expect("fixture calls").history_orders += 1;
        if self.fail_history_orders {
            return unavailable("fixture history orders unavailable");
        }
        Ok(self.history_orders.clone())
    }

    fn read_fills(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        self.calls.lock().expect("fixture calls").active_fills += 1;
        Ok(self.active_fills.clone())
    }

    fn read_history_fills(
        &self,
        _: TradeHeader,
        _: Option<TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        self.calls.lock().expect("fixture calls").history_fills += 1;
        Ok(self.history_fills.clone())
    }
}

fn reconciliation_store() -> (
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    tempfile::TempDir,
) {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("execution-orders.db");
    let connection = rusqlite::Connection::open(&path).expect("create execution database");
    jftrade_store_sqlite::initialize_current(&connection, "execution-orders")
        .expect("initialize execution schema");
    drop(connection);
    (
        Arc::new(
            jftrade_store_sqlite::ExecutionOrderStore::open(&path).expect("open execution store"),
        ),
        directory,
    )
}

fn pending_order(status: &str) -> jftrade_store_sqlite::StoredExecutionOrder {
    jftrade_store_sqlite::StoredExecutionOrder {
        internal_order_id: "rust-order-reconcile".to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: Some("11".to_owned()),
        broker_order_id_ex: Some("order-ex".to_owned()),
        source: "api".to_owned(),
        source_detail: "test".to_owned(),
        trading_environment: "REAL".to_owned(),
        account_id: "42".to_owned(),
        market: "US".to_owned(),
        symbol: Some("US.AAPL".to_owned()),
        side: Some("BUY".to_owned()),
        order_type: Some("LIMIT".to_owned()),
        status: status.to_owned(),
        raw_broker_status: None,
        requested_quantity: Some(5.0),
        requested_price: Some(99.0),
        filled_quantity: None,
        filled_average_price: None,
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: Some("2026-08-30T00:00:00Z".to_owned()),
        updated_at: "2026-08-30T00:00:01Z".to_owned(),
        created_at: "2026-08-30T00:00:00Z".to_owned(),
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "quantity".to_owned(),
        client_order_id: Some("client-reconcile".to_owned()),
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: None,
        payout: None,
        fees: None,
    }
}

fn account() -> TradeAccountSnapshot {
    TradeAccountSnapshot {
        trd_env: 1,
        acc_id: 42,
        trd_market_auth_list: vec![1],
        acc_type: None,
        card_num: None,
        security_firm: None,
        sim_acc_type: None,
        uni_card_num: None,
        acc_status: None,
        acc_role: None,
        jp_acc_type: Vec::new(),
        competition_acc_name: None,
    }
}

fn order_snapshot(status: i32, fill_qty: Option<f64>) -> TradeOrderSnapshot {
    TradeOrderSnapshot {
        trd_side: 1,
        order_type: 1,
        order_status: status,
        order_id: 11,
        order_id_ex: "order-ex".to_owned(),
        code: "AAPL".to_owned(),
        name: "Apple".to_owned(),
        qty: 5.0,
        price: Some(99.0),
        create_time: "2026-08-30T00:00:00Z".to_owned(),
        update_time: "2026-08-31T00:00:00Z".to_owned(),
        fill_qty,
        fill_avg_price: None,
        last_err_msg: None,
        sec_market: Some(11),
        create_timestamp: None,
        update_timestamp: None,
        remark: None,
        trd_market: Some(1),
        expire_time: None,
        order_amount: None,
        time_in_force: None,
        fill_outside_rth: None,
        aux_price: None,
        trail_type: None,
        trail_value: None,
        trail_spread: None,
        currency: None,
        session: None,
        jp_acc_type: None,
        strategy_type: None,
        combo_legs: Vec::new(),
    }
}

fn fill(create_time: &str, quantity: f64, id: &str) -> TradeFillSnapshot {
    TradeFillSnapshot {
        trd_side: 1,
        fill_id: 7,
        fill_id_ex: id.to_owned(),
        order_id: Some(11),
        order_id_ex: Some("order-ex".to_owned()),
        code: "AAPL".to_owned(),
        name: "Apple".to_owned(),
        qty: quantity,
        price: 100.0,
        create_time: create_time.to_owned(),
        counter_broker_id: None,
        counter_broker_name: None,
        sec_market: Some(11),
        create_timestamp: None,
        update_timestamp: None,
        status: None,
        trd_market: Some(1),
        jp_acc_type: None,
    }
}

fn fee(amount: f64) -> TradeOrderFeeSnapshot {
    TradeOrderFeeSnapshot {
        header: jftrade_integration_futu::trade_header(1, 42, 1),
        broker_order_id_ex: "order-ex".to_owned(),
        fee_amount: Some(amount),
        fee_items: Vec::new(),
    }
}

fn production_port(
    store: Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    reader: Arc<FixtureTradeReader>,
) -> ProductionExecutionPort {
    let runtime = Arc::new(super::super::SharedTradeReadRuntime::default());
    let reader_port: Arc<dyn TradeReadPort> = reader;
    runtime.set(Some(reader_port), Some(true));
    let provider = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    provider.set_readiness(true, true, false);
    ProductionExecutionPort {
        store,
        active_provider_state: provider,
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(Mutex::new(std::collections::BTreeSet::new())),
    }
}

#[test]
fn reconciliation_fill_identity_prefers_broker_extended_id() {
    let snapshot = fill("2026-08-31T01:00:00Z", 2.0, "fill-ex");
    assert_eq!(fill_identity(&snapshot), "fill-ex");
}

#[test]
fn reconciliation_snapshot_coverage_prevents_duplicate_fill_quantity() {
    let events = vec![jftrade_store_sqlite::StoredExecutionOrderEventRecord {
        id: "event-1".to_owned(),
        internal_order_id: "order-1".to_owned(),
        event_type: "reconciled".to_owned(),
        previous_status: Some("SUBMITTED".to_owned()),
        next_status: "PARTIALLY_FILLED".to_owned(),
        payload_json: r#"{"filledQuantity":4,"updatedAt":"2026-08-31T02:00:00Z"}"#.to_owned(),
        created_at: "2026-08-31T02:00:01Z".to_owned(),
    }];
    let delayed = fill("2026-08-31T01:30:00Z", 4.0, "delayed-fill");
    assert_eq!(
        covered_by_snapshot(&events, &delayed).expect("coverage"),
        4.0
    );
}

#[test]
fn reconciliation_fee_amount_falls_back_to_fee_items() {
    let fee = TradeOrderFeeSnapshot {
        header: jftrade_integration_futu::trade_header(0, 11, 1),
        broker_order_id_ex: "order-ex".to_owned(),
        fee_amount: None,
        fee_items: vec![
            jftrade_integration_futu::TradeOrderFeeItemSnapshot {
                title: "commission".to_owned(),
                value: 1.25,
            },
            jftrade_integration_futu::TradeOrderFeeItemSnapshot {
                title: "platform".to_owned(),
                value: 0.75,
            },
        ],
    };
    assert_eq!(fee_amount(&fee), Some(2.0));
}

#[test]
fn reconciliation_uses_ready_trade_session_with_helper_market_provider() {
    let (store, _directory) = reconciliation_store();
    let runtime = Arc::new(super::super::SharedTradeReadRuntime::default());
    let reader: Arc<dyn TradeReadPort> = Arc::new(FixtureTradeReader::default());
    runtime.set(Some(Arc::clone(&reader)), Some(true));
    let provider = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    provider.set_readiness(true, true, false);
    let port = ProductionExecutionPort {
        store,
        active_provider_state: provider,
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(Mutex::new(std::collections::BTreeSet::new())),
    };
    assert!(port.reconciliation_reader().is_ok());
}

#[test]
fn reconciliation_rejects_trade_client_when_opend_physical_session_is_unready() {
    let (store, _directory) = reconciliation_store();
    let runtime = Arc::new(super::super::SharedTradeReadRuntime::default());
    let reader: Arc<dyn TradeReadPort> = Arc::new(FixtureTradeReader::default());
    runtime.set(Some(reader), Some(true));
    let provider = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    provider.set_readiness(true, false, false);
    let port = ProductionExecutionPort {
        store,
        active_provider_state: provider,
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(Mutex::new(std::collections::BTreeSet::new())),
    };

    let result = port.reconciliation_reader();
    let error = match result {
        Ok(_) => panic!("a stale OpenD client must not be treated as ready"),
        Err(error) => error,
    };
    assert!(error.contains("OpenD runtime is not ready"));
}

#[test]
fn reconciliation_replays_history_fill_and_fee_once_after_restart() {
    let (store, directory) = reconciliation_store();
    let order = pending_order("SUBMITTED");
    store
        .save_order(order, "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        history_orders: vec![order_snapshot(10, None)],
        history_fills: vec![fill("2026-08-31T00:00:00Z", 3.0, "fill-1")],
        fees: vec![fee(1.5)],
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().expect("first scan"), 1);
    let saved = store
        .get_order("rust-order-reconcile")
        .expect("reload order")
        .expect("order exists");
    assert_eq!(saved.status, "PARTIALLY_FILLED");
    assert_eq!(saved.filled_quantity, Some(3.0));
    assert_eq!(saved.filled_average_price, Some(100.0));
    assert_eq!(saved.fees, Some(1.5));
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        3
    );
    assert_eq!(reader.calls.lock().unwrap().history_orders, 1);
    drop(port);
    drop(store);

    let path = directory.path().join("execution-orders.db");
    let reopened = Arc::new(
        jftrade_store_sqlite::ExecutionOrderStore::open(&path).expect("reopen execution store"),
    );
    let restarted = production_port(Arc::clone(&reopened), Arc::clone(&reader));
    assert_eq!(
        restarted.reconcile_pending_orders().expect("restart scan"),
        0
    );
    assert_eq!(
        reopened
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        3
    );
    assert_eq!(
        reopened
            .get_order("rust-order-reconcile")
            .unwrap()
            .unwrap()
            .fees,
        Some(1.5)
    );
}

#[test]
fn reconciliation_unknown_order_is_durable_and_idempotent() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert!(port.reconcile_pending_orders().is_err());
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "UNKNOWN");
    assert_eq!(
        saved.last_error_code.as_deref(),
        Some("BROKER_ORDER_NOT_FOUND")
    );
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        1
    );
    assert!(port.reconcile_pending_orders().is_err());
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .len(),
        1
    );
}

#[test]
fn reconciliation_cas_rejects_stale_fill_projection() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader::default());
    let port = production_port(Arc::clone(&store), reader);
    let current = store.get_order("rust-order-reconcile").unwrap().unwrap();
    let first_fill = fill("2026-08-31T00:00:00Z", 1.0, "fill-cas");
    assert!(
        port.apply_fill_snapshot(&current, &first_fill, 0)
            .expect("first fill")
    );
    let error = port
        .apply_fill_snapshot(
            &current,
            &fill("2026-08-31T00:00:00Z", 1.0, "fill-cas-2"),
            0,
        )
        .expect_err("stale fill must be fenced");
    assert!(
        matches!(error, ExecutionWritePortError::Failed { status: 409, ref code, .. } if code == "EXECUTION_ORDER_CONFLICT")
    );
}

#[test]
fn reconciliation_keeps_external_unavailable_as_retryable_without_unknown_write() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader {
        fail_accounts: true,
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), reader);
    assert!(port.reconcile_pending_orders().is_err());
    assert_eq!(
        store
            .get_order("rust-order-reconcile")
            .unwrap()
            .unwrap()
            .status,
        "SUBMITTED"
    );
    assert!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .is_empty()
    );
}

#[test]
fn reconciliation_retries_fees_after_terminal_order_projection() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        history_orders: vec![order_snapshot(11, Some(5.0))],
        // `pop` makes the first scan receive an empty response and the
        // second scan receive the eventual fee projection.
        fee_batches: Mutex::new(vec![vec![fee(2.25)], Vec::new()]),
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));

    assert_eq!(port.reconcile_pending_orders().expect("first scan"), 1);
    assert_eq!(
        store
            .get_order("rust-order-reconcile")
            .unwrap()
            .unwrap()
            .status,
        "FILLED"
    );
    assert_eq!(
        store
            .get_order("rust-order-reconcile")
            .unwrap()
            .unwrap()
            .fees,
        None
    );

    assert_eq!(port.reconcile_pending_orders().expect("fee retry"), 1);
    assert_eq!(
        store
            .get_order("rust-order-reconcile")
            .unwrap()
            .unwrap()
            .fees,
        Some(2.25)
    );
    assert_eq!(reader.calls.lock().unwrap().fees, 2);
}

#[test]
fn reconciliation_keeps_terminal_order_when_broker_history_ages_out() {
    let (store, _directory) = reconciliation_store();
    let mut terminal = pending_order("FILLED");
    terminal.filled_quantity = Some(5.0);
    store
        .save_order(terminal, "2026-08-30T00:00:01Z")
        .expect("save terminal order");
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        fees: vec![fee(3.0)],
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));

    assert_eq!(port.reconcile_pending_orders().expect("fee-only scan"), 1);
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "FILLED");
    assert_eq!(saved.fees, Some(3.0));
    assert!(saved.last_error.is_none());
    assert!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .iter()
            .all(|event| event.event_type != "reconcile_order_missing")
    );
}

#[test]
fn reconciliation_rejects_unknown_account_environment_without_projection() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let mut invalid_account = account();
    invalid_account.trd_env = 99;
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![invalid_account],
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), reader);

    let error = port
        .reconcile_pending_orders()
        .expect_err("unknown account environment must fail closed");
    assert!(error.contains("unknown trading environment"));
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "SUBMITTED");
    assert!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .is_empty()
    );
}

#[test]
fn reconciliation_rejects_malformed_fill_without_durable_partial_state() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let reader = Arc::new(FixtureTradeReader::default());
    let port = production_port(Arc::clone(&store), reader);
    let current = store.get_order("rust-order-reconcile").unwrap().unwrap();
    let malformed = TradeFillSnapshot {
        price: f64::NAN,
        ..fill("2026-08-31T00:00:00Z", 1.0, "fill-invalid-price")
    };

    let error = port
        .apply_fill_snapshot(&current, &malformed, 0)
        .expect_err("non-finite fill price must fail closed");
    assert!(matches!(
        error,
        ExecutionWritePortError::Failed {
            status: 502,
            ref code,
            ..
        } if code == "BROKER_INVALID_RESPONSE"
    ));
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.filled_quantity, None);
    assert!(
        store
            .list_order_events("rust-order-reconcile")
            .unwrap()
            .is_empty()
    );
}

#[test]
fn reconciliation_fails_closed_on_corrupt_event_payload() {
    let (store, _directory) = reconciliation_store();
    store
        .save_order(pending_order("SUBMITTED"), "2026-08-30T00:00:01Z")
        .expect("save pending order");
    let corrupt = StoredExecutionOrderEvent {
        id: "corrupt-event",
        internal_order_id: "rust-order-reconcile",
        event_type: "reconciled",
        previous_status: Some("SUBMITTED"),
        next_status: "SUBMITTED",
        payload_json: "{not-json",
        created_at: "2026-08-30T00:00:02Z",
    };
    store
        .record_event(&corrupt)
        .expect("record corrupt fixture");
    let port = production_port(Arc::clone(&store), Arc::new(FixtureTradeReader::default()));
    let current = store.get_order("rust-order-reconcile").unwrap().unwrap();

    let error = port
        .apply_fill_snapshot(
            &current,
            &fill("2026-08-31T00:00:00Z", 1.0, "fill-corrupt-event"),
            1,
        )
        .expect_err("corrupt event must not be silently skipped");
    assert!(matches!(
        error,
        ExecutionWritePortError::Failed {
            status: 500,
            ref code,
            ..
        } if code == "EXECUTION_ORDER_DATA_INVALID"
    ));
}

#[path = "product_production_ports_execution_reconciliation_provider_tests.rs"]
mod provider_projection_tests;

#[path = "product_production_ports_execution_reconciliation_order_state_tests.rs"]
mod order_state_tests;
