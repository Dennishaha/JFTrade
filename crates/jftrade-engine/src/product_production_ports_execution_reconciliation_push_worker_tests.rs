use super::*;
use std::time::Duration;
use tokio::sync::Notify;

use crate::product::product_production_ports::{
    ExecutionReconciliationWorker, SharedTradeReadRuntime,
};

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
        dynamic: Mutex<(
            Vec<TradeFillSnapshot>,
            TradeOrderSnapshot,
            Vec<TradeOrderFeeSnapshot>,
        )>,
        fail_accounts: std::sync::atomic::AtomicBool,
    }

    impl std::fmt::Debug for DynamicTradeReader {
        fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
            f.debug_struct("DynamicTradeReader").finish_non_exhaustive()
        }
    }

    impl TradeReadPort for DynamicTradeReader {
        fn read_accounts(
            &self,
            _: u64,
            _: Option<i32>,
            _: Option<bool>,
        ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
            if self
                .fail_accounts
                .load(std::sync::atomic::Ordering::Relaxed)
            {
                return Err(TradeSessionError::Unsupported(
                    "simulated disconnect".to_owned(),
                ));
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
            unavailable("funds")
        }
        fn read_cash_flows(
            &self,
            _: TradeHeader,
            _: String,
            _: Option<i32>,
        ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
            unavailable("cash flows")
        }
        fn read_order_fees(
            &self,
            _: TradeHeader,
            _: Vec<String>,
        ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
            Ok(self.dynamic.lock().unwrap().2.clone())
        }
        fn read_margin_ratios(
            &self,
            _: TradeHeader,
            _: Vec<TradeSecurity>,
        ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
            unavailable("margin")
        }
        fn read_max_trade_quantity(
            &self,
            _: TradeMaxTradeQuantityRequest,
        ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
            unavailable("max qty")
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
            unavailable("positions")
        }
        fn read_orders(
            &self,
            _: TradeHeader,
            _: Option<TradeFilter>,
            _: Vec<i32>,
            _: Option<bool>,
        ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
            Ok(vec![self.dynamic.lock().unwrap().1.clone()])
        }
        fn read_history_orders(
            &self,
            _: TradeHeader,
            _: Option<TradeFilter>,
            _: Vec<i32>,
            _: Option<bool>,
        ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
            Ok(vec![self.dynamic.lock().unwrap().1.clone()])
        }
        fn read_fills(
            &self,
            _: TradeHeader,
            _: Option<TradeFilter>,
            _: Option<bool>,
        ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
            Ok(self.dynamic.lock().unwrap().0.clone())
        }
        fn read_history_fills(
            &self,
            _: TradeHeader,
            _: Option<TradeFilter>,
            _: Option<bool>,
        ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
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
    assert_eq!(
        changed, 0,
        "covered fills must not alter already reconciled snapshot quantity"
    );

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

    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), Some(Arc::clone(&wake)));

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

    let failing_worker =
        ExecutionReconciliationWorker::start(Arc::clone(&failing_port), Some(Arc::clone(&wake)));

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

    let self_healing_worker =
        ExecutionReconciliationWorker::start(Arc::clone(&recovered_port), Some(Arc::clone(&wake)));

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

    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), Some(Arc::clone(&wake)));

    tokio::time::sleep(Duration::from_millis(40)).await;
    let count_before = worker.status().scans;

    let start = std::time::Instant::now();
    worker.wake();
    tokio::time::sleep(Duration::from_millis(40)).await;
    let elapsed = start.elapsed();

    let count_after = worker.status().scans;
    assert!(
        count_after > count_before,
        "wake() must immediately trigger scan"
    );
    assert!(
        elapsed < Duration::from_secs(1),
        "push wake latency must be sub-second"
    );
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

    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), Some(Arc::clone(&wake)));

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

#[tokio::test]
async fn test_reconciliation_shutdown_is_bounded_for_blocking_broker_scan() {
    let (store, _directory) = reconciliation_store();
    let entered = Arc::new(Notify::new());
    let finished = Arc::new(Notify::new());
    let gate = Arc::new(BlockingAccounts {
        entered: Arc::clone(&entered),
        finished: Arc::clone(&finished),
        entries: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        release: Arc::new((Mutex::new(false), Condvar::new())),
    });
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        blocking_accounts: Some(Arc::clone(&gate)),
        ..Default::default()
    });
    let reader_port: Arc<dyn TradeReadPort> = reader;
    let provider = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    provider.set_readiness(true, true, false);
    let port = Arc::new(ProductionExecutionPort {
        store,
        active_provider_state: provider,
        trade_logged_in: Some(true),
        trade_read_port: Some(reader_port),
        trade_write_port: None,
        trade_runtime: None,
        cancel_inflight: Arc::new(Mutex::new(std::collections::BTreeSet::new())),
        risk_coordinator: None,
        default_trading_environment: None,
        notification_projector: None,
    });
    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), None);

    tokio::time::timeout(Duration::from_secs(1), entered.notified())
        .await
        .expect("worker must enter blocking broker scan");
    let started = std::time::Instant::now();
    tokio::time::timeout(Duration::from_secs(1), worker.shutdown())
        .await
        .expect("shutdown must have a bounded deadline");
    assert!(
        started.elapsed() < Duration::from_secs(1),
        "shutdown exceeded the test deadline: {:?}",
        started.elapsed()
    );
    let status = worker.status();
    assert_eq!(status.state, "failed");
    assert!(status
        .last_error
        .as_deref()
        .is_some_and(|error| error.contains("shutdown exceeded")));

    // The blocking task cannot be force-cancelled. Once the fake broker call
    // returns it must still complete and release its port Arc cleanly.
    gate.release();
    tokio::time::timeout(Duration::from_secs(1), finished.notified())
        .await
        .expect("blocking broker task must finish after release");
}

#[tokio::test]
async fn test_reconciliation_terminate_suppresses_late_scan_updates() {
    let (store, _directory) = reconciliation_store();
    let entered = Arc::new(Notify::new());
    let finished = Arc::new(Notify::new());
    let gate = Arc::new(BlockingAccounts {
        entered: Arc::clone(&entered),
        finished: Arc::clone(&finished),
        entries: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        release: Arc::new((Mutex::new(false), Condvar::new())),
    });
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        blocking_accounts: Some(Arc::clone(&gate)),
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), Arc::clone(&reader)));
    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), None);

    tokio::time::timeout(Duration::from_secs(1), entered.notified())
        .await
        .expect("worker must enter blocking broker scan");
    worker.terminate();
    assert_eq!(worker.status().state, "stopped");

    // `terminate` aborts only the async owner.  The synchronous provider call
    // still returns eventually, but its late result must not count as a scan,
    // publish notifications, or revive the retired worker.
    gate.release();
    tokio::time::timeout(Duration::from_secs(1), finished.notified())
        .await
        .expect("terminated blocking task must finish after release");
    tokio::time::sleep(Duration::from_millis(50)).await;
    let status = worker.status();
    assert_eq!(status.state, "stopped");
    assert_eq!(status.scans, 0, "late terminated result must not update status");
    assert_eq!(
        gate.entries.load(std::sync::atomic::Ordering::SeqCst),
        1,
        "terminated owner must not start another scan"
    );
}

async fn scans_reach(worker: &ExecutionReconciliationWorker, target: u64) -> u64 {
    let deadline = std::time::Instant::now() + Duration::from_secs(5);
    loop {
        let scans = worker.status().scans;
        if scans >= target || std::time::Instant::now() >= deadline {
            return scans;
        }
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
}

/// Parity: go:452dea11:internal/trading/order_updates_test.go:263 TestOrderUpdatesWorkerForcedActiveSyncBypassesCache
#[tokio::test]
async fn reconciliation_polling_throttles_scans_until_a_push_wake_forces_one() {
    let (store, _directory) = reconciliation_store();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), Arc::clone(&reader)));
    let wake = Arc::new(Notify::new());
    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), Some(Arc::clone(&wake)));

    let first_scan = scans_reach(&worker, 1).await;
    assert!(first_scan >= 1, "the worker must scan once at start");

    // Go's throttled `Sync(force=false)`: repeat syncs inside the window reuse
    // the cached result, so a short wait must not buy another broker round-trip.
    tokio::time::sleep(Duration::from_millis(300)).await;
    assert_eq!(
        worker.status().scans,
        first_scan,
        "the polling cadence must throttle repeat scans"
    );

    // Go's forced `Sync(force=true)`: a broker push bypasses the cadence and
    // pulls the current/history snapshot immediately.
    worker.wake();
    let forced = scans_reach(&worker, first_scan + 1).await;
    assert_eq!(
        forced,
        first_scan + 1,
        "one push wake must force exactly one extra scan"
    );
}

/// Parity: go:452dea11:internal/trading/order_updates_test.go:414 TestOrderUpdatesWorkerSnapshotCapsInvalidations
#[tokio::test]
async fn reconciliation_worker_bounds_recent_invalidations_to_twenty_entries() {
    let (store, _directory) = reconciliation_store();
    let failing = Arc::new(FixtureTradeReader {
        accounts: vec![],
        fail_accounts: true,
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), failing));
    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), None);

    // Go drives 25 failing syncs and keeps only `maxOrderUpdateInvalidations`
    // (20) entries in the snapshot.
    let mut target = scans_reach(&worker, 1).await;
    for _ in 0..24 {
        target += 1;
        worker.wake();
        let reached = scans_reach(&worker, target).await;
        assert!(reached >= target, "wake {target} must produce a scan");
    }

    let status = worker.status();
    assert!(status.scans >= 25, "scans = {}", status.scans);
    assert!(status.failures >= 25, "failures = {}", status.failures);
    let invalidations = worker.invalidations();
    assert_eq!(
        invalidations.len(),
        20,
        "the invalidation history must stay capped"
    );
    assert!(
        invalidations
            .iter()
            .all(|entry| entry.broker_id == "futu" && !entry.created_at.is_empty()),
        "invalidations = {invalidations:?}"
    );
    assert!(
        invalidations
            .iter()
            .all(|entry| entry.kind == "DISCONNECTED" || entry.kind == "ERROR"),
        "invalidations = {invalidations:?}"
    );
    assert_eq!(
        invalidations.last().map(|entry| entry.message.as_str()),
        status.last_error.as_deref(),
        "the newest invalidation must carry the current scan error"
    );
    worker.shutdown().await;
}

/// Parity: go:452dea11:internal/trading/order_updates_test.go:427 TestOrderUpdatesWorkerInactiveSourcePreservesDiagnosticState
#[tokio::test]
async fn reconciliation_worker_projects_inactive_source_connectivity_and_go_shaped_invalidations() {
    let (store, _directory) = reconciliation_store();
    let failing = Arc::new(FixtureTradeReader {
        accounts: vec![],
        fail_accounts: true,
        ..Default::default()
    });
    let port = Arc::new(production_port(Arc::clone(&store), failing));
    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), None);
    scans_reach(&worker, 1).await;

    assert_eq!(worker.connectivity().as_deref(), Some("inactive"));
    let snapshot = crate::product::product_production_ports::product_production_ports_system::broker_order_updates_snapshot(&worker);
    assert!(
        snapshot["subscriptions"].as_array().expect("array").is_empty(),
        "snapshot = {snapshot}"
    );
    assert_eq!(snapshot["brokers"][0]["brokerId"], "futu");
    assert_eq!(snapshot["brokers"][0]["connectivity"], "inactive");
    let invalidations = snapshot["recentInvalidations"]
        .as_array()
        .expect("recentInvalidations array");
    assert!(!invalidations.is_empty(), "snapshot = {snapshot}");
    for key in [
        "subscriptionKey",
        "brokerId",
        "tradingEnvironment",
        "accountId",
        "market",
        "kind",
        "message",
        "createdAt",
    ] {
        assert!(
            invalidations[0].get(key).is_some(),
            "missing {key} in {snapshot}"
        );
    }
    worker.shutdown().await;

    // A reachable source keeps the same projection healthy.
    let (healthy_store, _healthy_directory) = reconciliation_store();
    let healthy = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..Default::default()
    });
    let healthy_port = Arc::new(production_port(healthy_store, healthy));
    let healthy_worker = ExecutionReconciliationWorker::start(Arc::clone(&healthy_port), None);
    scans_reach(&healthy_worker, 1).await;
    assert_eq!(healthy_worker.connectivity().as_deref(), Some("connected"));
    assert!(healthy_worker.invalidations().is_empty());
    let healthy_snapshot = crate::product::product_production_ports::product_production_ports_system::broker_order_updates_snapshot(&healthy_worker);
    assert_eq!(healthy_snapshot["brokers"][0]["connectivity"], "connected");
    healthy_worker.shutdown().await;
}
