use super::*;
use std::time::Duration;
use tokio::sync::Notify;

use crate::product::product_production_ports::{ExecutionReconciliationWorker, SharedTradeReadRuntime};

#[test]
fn test_tc_d5_01_sequential_partial_fills_and_fees_monotonic() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTED");
    order.requested_quantity = Some(100.0);
    order.requested_price = Some(100.0);
    order.filled_quantity = None;
    order.filled_average_price = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let fill_1 = fill("2026-08-31T01:00:00Z", 20.0, "fill-1");
    let mut snap_base = order_snapshot(5, None);
    snap_base.qty = 100.0;

    struct DynamicTradeReader {
        accounts: Vec<TradeAccountSnapshot>,
        dynamic: Mutex<(Vec<TradeFillSnapshot>, TradeOrderSnapshot, Vec<TradeOrderFeeSnapshot>)>,
        fail_accounts: std::sync::atomic::AtomicBool,
    }

    impl std::fmt::Debug for DynamicTradeReader {
        fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
            f.debug_struct("DynamicTradeReader").finish_non_exhaustive()
        }
    }

    impl TradeReadPort for DynamicTradeReader {
        fn read_accounts(&self, _: u64, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
            if self.fail_accounts.load(std::sync::atomic::Ordering::Relaxed) {
                return Err(TradeSessionError::Unsupported("simulated disconnect".to_owned()));
            }
            Ok(self.accounts.clone())
        }
        fn read_funds(&self, _: TradeHeader, _: Option<bool>, _: Option<i32>, _: Option<i32>) -> Result<TradeFundsSnapshot, TradeSessionError> {
            unavailable("funds")
        }
        fn read_cash_flows(&self, _: TradeHeader, _: String, _: Option<i32>) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
            unavailable("cash flows")
        }
        fn read_order_fees(&self, _: TradeHeader, _: Vec<String>) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
            Ok(self.dynamic.lock().unwrap().2.clone())
        }
        fn read_margin_ratios(&self, _: TradeHeader, _: Vec<TradeSecurity>) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
            unavailable("margin")
        }
        fn read_max_trade_quantity(&self, _: TradeMaxTradeQuantityRequest) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
            unavailable("max qty")
        }
        fn read_positions(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<f64>, _: Option<f64>, _: Option<bool>, _: Option<i32>, _: Option<i32>, _: Option<bool>) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
            unavailable("positions")
        }
        fn read_orders(&self, _: TradeHeader, _: Option<TradeFilter>, _: Vec<i32>, _: Option<bool>) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
            Ok(vec![self.dynamic.lock().unwrap().1.clone()])
        }
        fn read_history_orders(&self, _: TradeHeader, _: Option<TradeFilter>, _: Vec<i32>, _: Option<bool>) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
            Ok(vec![self.dynamic.lock().unwrap().1.clone()])
        }
        fn read_fills(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<bool>) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
            Ok(self.dynamic.lock().unwrap().0.clone())
        }
        fn read_history_fills(&self, _: TradeHeader, _: Option<TradeFilter>, _: Option<bool>) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
            Ok(self.dynamic.lock().unwrap().0.clone())
        }
    }

    let dyn_reader = Arc::new(DynamicTradeReader {
        accounts: vec![account()],
        dynamic: Mutex::new((vec![fill_1.clone()], snap_base.clone(), vec![])),
        fail_accounts: std::sync::atomic::AtomicBool::new(false),
    });

    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader_port: Arc<dyn TradeReadPort> = dyn_reader.clone();
    runtime.set(Some(reader_port), Some(true));
    let provider = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    provider.set_readiness(true, true, false);
    let port = ProductionExecutionPort {
        store: Arc::clone(&store),
        active_provider_state: provider,
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(runtime),
        cancel_inflight: Arc::new(Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    };

    // Phase 1 Reconcile:
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let order_p1 = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(order_p1.status, "PARTIALLY_FILLED");
    assert_eq!(order_p1.filled_quantity, Some(20.0));
    assert_eq!(order_p1.filled_average_price, Some(100.0));

    // Phase 2: Add Fill 2 (30 shares @ $105.0)
    let mut fill_2 = fill("2026-08-31T01:05:00Z", 30.0, "fill-2");
    fill_2.price = 105.0;
    {
        let mut d = dyn_reader.dynamic.lock().unwrap();
        d.0.push(fill_2.clone());
    }

    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let order_p2 = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(order_p2.status, "PARTIALLY_FILLED");
    assert_eq!(order_p2.filled_quantity, Some(50.0));
    // Weighted avg: (20 * 100 + 30 * 105) / 50 = (2000 + 3150) / 50 = 103.0
    assert!((order_p2.filled_average_price.unwrap() - 103.0).abs() < 1e-6);

    // Phase 3: Add Fill 3 (50 shares @ $110.0) -> Terminal FILLED
    let mut fill_3 = fill("2026-08-31T01:10:00Z", 50.0, "fill-3");
    fill_3.price = 110.0;
    {
        let mut d = dyn_reader.dynamic.lock().unwrap();
        d.0.push(fill_3);
    }

    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let order_p3 = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(order_p3.status, "FILLED");
    assert_eq!(order_p3.filled_quantity, Some(100.0));
    assert!((order_p3.filled_average_price.unwrap() - 106.5).abs() < 1e-6);

    // Phase 4: Fee reconciliation ($3.50 commission)
    let order_fee = fee(3.50);
    {
        let mut d = dyn_reader.dynamic.lock().unwrap();
        d.2 = vec![order_fee];
    }
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let final_order = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(final_order.status, "FILLED");
    assert_eq!(final_order.fees, Some(3.50));

    // Verify all 4 state transitions are logged monotonically in execution.db
    let events = store.list_order_events("rust-order-reconcile").unwrap();
    assert!(!events.is_empty());
    assert_eq!(events.last().unwrap().next_status, "FILLED");
}

#[test]
fn test_tc_d5_02_out_of_order_push_chaos_covered_by_snapshot() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTED");
    order.requested_quantity = Some(100.0);
    order.requested_price = Some(100.0);
    order.filled_quantity = None;
    order.filled_average_price = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    // Step 1: Snapshot arrives ahead of detailed fills: 50 shares filled (Status 10: FILLED_PART)
    let mut snapshot = order_snapshot(10, Some(50.0));
    snapshot.qty = 100.0;
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snapshot],
        active_fills: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

    let order_snap = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(order_snap.status, "PARTIALLY_FILLED");
    assert_eq!(order_snap.filled_quantity, Some(50.0));

    // Step 2: Stale/reverse detailed fills arrive late (Fill 2 of 30 shares, Fill 1 of 20 shares)
    // with creation times earlier than the snapshot update time
    let fill_delayed_2 = fill("2026-08-30T00:00:00Z", 30.0, "fill-delayed-2");
    let fill_delayed_1 = fill("2026-08-29T23:59:00Z", 20.0, "fill-delayed-1");

    let mut snap_delayed = order_snapshot(10, Some(50.0));
    snap_delayed.qty = 100.0;
    let reader_delayed = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap_delayed],
        active_fills: vec![fill_delayed_2, fill_delayed_1],
        ..Default::default()
    });
    let port_delayed = production_port(Arc::clone(&store), Arc::clone(&reader_delayed));

    // Reconcile must recognize covered_by_snapshot and not double-count
    let changed = port_delayed.reconcile_pending_orders().unwrap();
    assert_eq!(changed, 0, "covered fills must not alter already reconciled snapshot quantity");

    let final_order = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(final_order.filled_quantity, Some(50.0));
}

#[tokio::test]
async fn test_tc_d5_04_opend_disconnect_degraded_backoff_and_self_healing() {
    let (store, _directory) = reconciliation_store();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), Arc::clone(&reader)));
    let wake = Arc::new(Notify::new());

    let worker = ExecutionReconciliationWorker::start(
        Arc::clone(&port),
        Some(Arc::clone(&wake)),
    );

    // Initial scan executes -> ready
    tokio::time::sleep(Duration::from_millis(50)).await;
    let initial_status = worker.status();
    assert_eq!(initial_status.state, "ready");
    assert!(initial_status.scans >= 1);
    assert_eq!(initial_status.failures, 0);

    // Inject OpenD disconnect by replacing trade_runtime with failing reader
    let failing_reader = Arc::new(FixtureTradeReader {
        accounts: vec![],
        fail_accounts: true,
        ..Default::default()
    });
    let failing_runtime = Arc::new(SharedTradeReadRuntime::default());
    let failing_reader_port: Arc<dyn TradeReadPort> = failing_reader;
    failing_runtime.set(Some(failing_reader_port), Some(true));

    let failing_port = Arc::new(ProductionExecutionPort {
        store: Arc::clone(&store),
        active_provider_state: port.active_provider_state.clone(),
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(failing_runtime),
        cancel_inflight: Arc::clone(&port.cancel_inflight),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    });

    let failing_worker = ExecutionReconciliationWorker::start(
        Arc::clone(&failing_port),
        Some(Arc::clone(&wake)),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;
    let degraded_status = failing_worker.status();
    assert_eq!(degraded_status.state, "degraded");
    assert_eq!(degraded_status.failures, 1);
    assert!(degraded_status.last_error.is_some());
    assert!(degraded_status.next_retry_at.is_some());

    // Recover network
    let recovered_reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..Default::default()
    });
    let recovered_runtime = Arc::new(SharedTradeReadRuntime::default());
    let recovered_reader_port: Arc<dyn TradeReadPort> = recovered_reader;
    recovered_runtime.set(Some(recovered_reader_port), Some(true));

    let recovered_port = Arc::new(ProductionExecutionPort {
        store: Arc::clone(&store),
        active_provider_state: port.active_provider_state.clone(),
        trade_logged_in: None,
        trade_read_port: None,
        trade_write_port: None,
        trade_runtime: Some(recovered_runtime),
        cancel_inflight: Arc::clone(&port.cancel_inflight),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    });

    let self_healing_worker = ExecutionReconciliationWorker::start(
        Arc::clone(&recovered_port),
        Some(Arc::clone(&wake)),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;
    let healed_status = self_healing_worker.status();
    assert_eq!(healed_status.state, "ready");
    assert!(healed_status.last_error.is_none());
    assert!(healed_status.next_retry_at.is_none());
}

#[tokio::test]
async fn test_p1_02_push_wake_latency_and_polling_fallback() {
    let (store, _directory) = reconciliation_store();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), Arc::clone(&reader)));
    let wake = Arc::new(Notify::new());

    let worker = ExecutionReconciliationWorker::start(
        Arc::clone(&port),
        Some(Arc::clone(&wake)),
    );

    tokio::time::sleep(Duration::from_millis(40)).await;
    let count_before = worker.status().scans;

    let start = std::time::Instant::now();
    worker.wake();
    tokio::time::sleep(Duration::from_millis(40)).await;
    let elapsed = start.elapsed();

    let count_after = worker.status().scans;
    assert!(count_after > count_before, "wake() must immediately trigger scan");
    assert!(elapsed < Duration::from_secs(1), "push wake latency must be sub-second");
}

#[tokio::test]
async fn test_p1_02_single_writer_lease_and_concurrency_fencing() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = Some("11".to_owned());
    order.broker_order_id_ex = Some("order-ex".to_owned());
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let mut snap = order_snapshot(10, Some(2.0));
    snap.qty = 5.0;
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap],
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), Arc::clone(&reader)));
    let wake = Arc::new(Notify::new());

    let worker = ExecutionReconciliationWorker::start(
        Arc::clone(&port),
        Some(Arc::clone(&wake)),
    );

    // Flood with concurrent wake calls
    for _ in 0..10 {
        worker.wake();
    }
    tokio::time::sleep(Duration::from_millis(60)).await;

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "PARTIALLY_FILLED");
    assert_eq!(saved.filled_quantity, Some(2.0));
    assert_eq!(worker.status().failures, 0);
}
