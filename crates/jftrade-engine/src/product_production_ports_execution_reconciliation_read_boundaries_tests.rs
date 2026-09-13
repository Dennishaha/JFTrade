use super::*;

use std::sync::{Arc, Condvar, Mutex};
use std::time::{Duration, Instant};

use tokio::sync::Notify;

use crate::product::product_production_ports::ExecutionReconciliationWorker;

/// The OpenD order-list protocol has no cursor/page field.  The production
/// reader therefore exposes one active and one history snapshot, while the
/// reconciliation boundary still has to behave as if those two sources were
/// pages: duplicates must collapse, the newest observation must win, and a
/// terminal empty page must not create another durable event.
#[test]
// Parity: internal/trading/broker_test.go:480 TestServiceBrokerReadFallbacksAndUnavailableMarketData
// Verifies broker read boundary deduplicates repeated active/history pages, preserves newest snapshot and handles fallback states cleanly
fn reconciliation_discovery_deduplicates_repeated_pages_and_keeps_newest_snapshot() {
    let (store, _directory) = reconciliation_store();
    let mut account = account();
    account.trd_market_auth_list = vec![2];

    let mut stale = order_snapshot(5, Some(0.0));
    stale.trd_market = Some(2);
    stale.update_time = "2026-08-31T00:00:00Z".to_owned();
    let mut newest = order_snapshot(11, Some(5.0));
    newest.trd_market = Some(2);
    newest.update_time = "2026-08-31T00:01:00Z".to_owned();

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account],
        // A fake provider can repeat the same rows at the active/history
        // boundary.  The identity and update timestamp, rather than source
        // ordering, determine the durable projection.
        active_orders: vec![stale.clone(), newest.clone()],
        history_orders: vec![newest, stale],
        ..FixtureTradeReader::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));

    assert_eq!(port.reconcile_pending_orders().expect("first scan"), 1);
    let orders = store.list_orders().expect("list discovered orders");
    assert_eq!(orders.len(), 1);
    assert_eq!(orders[0].status, "FILLED");
    assert_eq!(orders[0].filled_quantity, Some(5.0));
    assert_eq!(
        store
            .list_order_events(&orders[0].internal_order_id)
            .expect("list discovery events")
            .len(),
        1
    );
    assert_eq!(reader.calls.lock().expect("reader calls").active_orders, 1);
    assert_eq!(reader.calls.lock().expect("reader calls").history_orders, 1);

    // Empty/terminal repetition on the next reconciliation pass is a no-op:
    // it neither allocates a second internal order nor appends an event.
    assert_eq!(port.reconcile_pending_orders().expect("terminal page retry"), 0);
    assert_eq!(store.list_orders().expect("list after retry").len(), 1);
    assert_eq!(
        store
            .list_order_events(&orders[0].internal_order_id)
            .expect("events after retry")
            .len(),
        1
    );
    // The second pass also visits the terminal fee-reconciliation candidate,
    // so each endpoint is read once for that candidate and once for the
    // account-wide discovery pass.
    assert_eq!(reader.calls.lock().expect("reader calls").active_orders, 3);
    assert_eq!(reader.calls.lock().expect("reader calls").history_orders, 3);
}

#[test]
fn reconciliation_retries_history_page_after_failure_on_restart() {
    let (store, _directory) = reconciliation_store();
    let mut account = account();
    account.trd_market_auth_list = vec![2];
    let mut external = order_snapshot(11, Some(5.0));
    external.trd_market = Some(2);

    let failed_reader = Arc::new(FixtureTradeReader {
        accounts: vec![account.clone()],
        active_orders: vec![external.clone()],
        fail_history_orders: true,
        ..FixtureTradeReader::default()
    });
    let failed_port = production_port(Arc::clone(&store), failed_reader);
    assert!(
        failed_port.reconcile_pending_orders().is_err(),
        "a timed-out/failed history page must fail closed"
    );
    assert!(
        store.list_orders().expect("orders after failed page").is_empty(),
        "a one-sided page failure must not partially persist discovery"
    );
    drop(failed_port);

    // A fresh runtime/re-reader after restart retries the complete scope and
    // can now converge without requiring the original failed page call.
    let recovered_reader = Arc::new(FixtureTradeReader {
        accounts: vec![account],
        active_orders: vec![external.clone()],
        history_orders: vec![external],
        ..FixtureTradeReader::default()
    });
    let recovered_port = production_port(Arc::clone(&store), recovered_reader);
    assert_eq!(
        recovered_port
            .reconcile_pending_orders()
            .expect("restart retry"),
        1
    );
    assert_eq!(store.list_orders().expect("recovered orders").len(), 1);
}

#[tokio::test]
async fn reconciliation_late_response_after_bounded_shutdown_converges_once() {
    let (store, _directory) = reconciliation_store();
    let entered = Arc::new(Notify::new());
    let finished = Arc::new(Notify::new());
    let gate = Arc::new(BlockingAccounts {
        entered: Arc::clone(&entered),
        finished: Arc::clone(&finished),
        entries: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        release: Arc::new((Mutex::new(false), Condvar::new())),
    });
    let mut account = account();
    account.trd_market_auth_list = vec![2];
    // The order snapshot is only partially filled; the delayed fill below
    // must be the observation that completes it and appends the second event.
    let mut external = order_snapshot(10, None);
    external.trd_market = Some(2);
    let mut delayed_fill = fill("2026-08-31T00:00:00Z", 5.0, "late-fill");
    delayed_fill.trd_market = Some(2);

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account],
        active_orders: vec![external],
        active_fills: vec![delayed_fill],
        blocking_accounts: Some(Arc::clone(&gate)),
        ..FixtureTradeReader::default()
    });
    let live_hub = Arc::new(jftrade_api::LiveHub::new(8));
    live_hub.mark_serving();
    let mut live_connection = live_hub.connect();
    let projector = Arc::new(crate::product::ExecutionNotificationProjector::new(
        Arc::clone(&store),
        None,
        Some(Arc::clone(&live_hub)),
    ));
    let mut retired_port = production_port(Arc::clone(&store), Arc::clone(&reader));
    retired_port.notification_projector = Some(projector);
    let port = Arc::new(retired_port);
    let worker = ExecutionReconciliationWorker::start(Arc::clone(&port), None);

    tokio::time::timeout(Duration::from_secs(1), entered.notified())
        .await
        .expect("scan must enter controllable broker call");
    worker
        .shutdown()
        .await;
    let shutdown_status = worker.status();
    assert_eq!(shutdown_status.state, "failed");
    assert!(shutdown_status
        .last_error
        .as_deref()
        .is_some_and(|message| message.contains("shutdown exceeded")));

    // A replacement worker may be assembled before the detached blocking
    // call returns.  It must wait on the store-owned scan fence instead of
    // entering the provider concurrently with the retired owner.
    let replacement_port = Arc::new(production_port(Arc::clone(&store), Arc::clone(&reader)));
    let replacement = ExecutionReconciliationWorker::start(Arc::clone(&replacement_port), None);
    tokio::time::sleep(Duration::from_millis(50)).await;
    assert_eq!(
        gate.entries.load(std::sync::atomic::Ordering::SeqCst),
        1,
        "replacement scan must wait for the retired scan fence"
    );

    // The synchronous provider call cannot be force-cancelled.  Releasing the
    // fake response lets the detached blocking scan finish; it must commit a
    // complete order + late fill exactly once, not a half-page projection.
    gate.release();
    tokio::time::timeout(Duration::from_secs(1), finished.notified())
        .await
        .expect("late provider call must return after release");
    let deadline = Instant::now() + Duration::from_secs(1);
    loop {
        let orders = store.list_orders().expect("late order projection");
        if orders.len() == 1 {
            let events = store
                .list_order_events(&orders[0].internal_order_id)
                .expect("late order events");
            if orders[0].status == "FILLED"
                && orders[0].filled_quantity == Some(5.0)
                && events.len() == 2
            {
                break;
            }
        }
        assert!(Instant::now() < deadline, "late scan did not converge");
        tokio::time::sleep(Duration::from_millis(10)).await;
    }

    let replacement_deadline = Instant::now() + Duration::from_secs(1);
    loop {
        if replacement.status().scans >= 1
            && gate.entries.load(std::sync::atomic::Ordering::SeqCst) == 2
        {
            break;
        }
        assert!(
            Instant::now() < replacement_deadline,
            "replacement scan did not run after the retired owner released the fence"
        );
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
    assert_eq!(
        worker.status().state,
        "failed",
        "late completion must not revive a worker whose shutdown exceeded its deadline"
    );
    assert_eq!(
        worker.status().scans,
        0,
        "late completion must not update the retired worker scan counters"
    );
    assert!(
        tokio::time::timeout(Duration::from_millis(100), live_connection.recv())
            .await
            .is_err(),
        "late completion must not project notifications after shutdown"
    );
    replacement.shutdown().await;
    tokio::time::sleep(Duration::from_millis(50)).await;
    assert_eq!(
        gate.entries.load(std::sync::atomic::Ordering::SeqCst),
        2,
        "shutdown must prevent a replacement worker from starting another scan"
    );

    // Reopening the same execution database after the timed-out worker is
    // idempotent: the late response is not replayed as a second order/fill.
    let restarted = production_port(Arc::clone(&store), reader);
    assert_eq!(restarted.reconcile_pending_orders().expect("restart scan"), 0);
    let orders = store.list_orders().expect("orders after restart");
    assert_eq!(orders.len(), 1);
    assert_eq!(
        store
            .list_order_events(&orders[0].internal_order_id)
            .expect("events after restart")
            .len(),
        2
    );
}
