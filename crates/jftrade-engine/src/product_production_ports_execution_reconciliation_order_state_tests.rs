use super::*;

fn persist_reconciliation_order(
    status: &str,
) -> (
    Arc<jftrade_store_sqlite::ExecutionOrderStore>,
    ProductionExecutionPort,
    tempfile::TempDir,
) {
    let (store, directory) = reconciliation_store();
    store
        .save_order(pending_order(status), "2026-08-30T00:00:01Z")
        .expect("save reconciliation order");
    let reader = Arc::new(FixtureTradeReader::default());
    let port = production_port(Arc::clone(&store), reader);
    (store, port, directory)
}

fn apply_order_snapshot(
    port: &ProductionExecutionPort,
    store: &jftrade_store_sqlite::ExecutionOrderStore,
    status: i32,
    filled: Option<f64>,
) -> bool {
    let current = store
        .get_order("rust-order-reconcile")
        .expect("load reconciliation order")
        .expect("reconciliation order exists");
    let revision = store
        .order_revision(&current.internal_order_id)
        .expect("read reconciliation revision");
    port.apply_broker_snapshot(&current, &order_snapshot(status, filled), revision)
        .expect("apply broker order snapshot")
}

#[test]
fn reconciliation_rejects_terminal_and_partial_status_regressions() {
    let cases = [
        ("filled ignores submitted", 11, Some(5.0), 5, Some(0.0), "FILLED", 5.0),
        (
            "filled ignores cancelled",
            11,
            Some(5.0),
            15,
            Some(5.0),
            "FILLED",
            5.0,
        ),
        (
            "partial ignores submitted",
            10,
            Some(2.0),
            5,
            Some(1.0),
            "PARTIALLY_FILLED",
            2.0,
        ),
    ];

    for (name, advance_status, advance_fill, regression_status, regression_fill, expected_status, expected_fill) in cases {
        let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
        assert!(
            apply_order_snapshot(&port, &store, advance_status, advance_fill),
            "{name}: forward snapshot must be accepted"
        );
        let events_before = store
            .list_order_events("rust-order-reconcile")
            .expect("list forward events")
            .len();
        assert!(!apply_order_snapshot(
            &port,
            &store,
            regression_status,
            regression_fill
        ));
        let saved = store
            .get_order("rust-order-reconcile")
            .expect("reload regressed order")
            .expect("regressed order exists");
        assert_eq!(saved.status, expected_status, "{name}: status");
        assert_eq!(saved.filled_quantity, Some(expected_fill), "{name}: fill");
        assert_eq!(
            store
                .list_order_events("rust-order-reconcile")
                .expect("list regression events")
                .len(),
            events_before,
            "{name}: regression must not append an event"
        );
    }
}

#[test]
fn reconciliation_cancel_submitted_resolves_fill_and_cancel_confirmation() {
    let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
    assert!(apply_order_snapshot(&port, &store, 13, Some(0.0)));
    let cancel_requested = store
        .get_order("rust-order-reconcile")
        .expect("load cancel-requested order")
        .expect("cancel-requested order exists");
    assert_eq!(cancel_requested.status, "CANCEL_SUBMITTED");
    assert!(apply_order_snapshot(&port, &store, 11, Some(5.0)));
    let filled = store
        .get_order("rust-order-reconcile")
        .expect("load filled order")
        .expect("filled order exists");
    assert_eq!(filled.status, "FILLED");
    assert_eq!(filled.filled_quantity, Some(5.0));
    let events = store
        .list_order_events("rust-order-reconcile")
        .expect("list fill-wins events");
    assert_eq!(events.len(), 2);
    assert_eq!(events[1].previous_status.as_deref(), Some("CANCEL_SUBMITTED"));
    assert_eq!(events[1].next_status, "FILLED");

    let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
    assert!(apply_order_snapshot(&port, &store, 13, Some(0.0)));
    assert!(apply_order_snapshot(&port, &store, 15, Some(0.0)));
    let cancelled = store
        .get_order("rust-order-reconcile")
        .expect("load cancelled order")
        .expect("cancelled order exists");
    assert_eq!(cancelled.status, "CANCELLED");
    let events = store
        .list_order_events("rust-order-reconcile")
        .expect("list cancel-confirm events");
    assert_eq!(events.len(), 2);
    assert_eq!(events[1].previous_status.as_deref(), Some("CANCEL_SUBMITTED"));
    assert_eq!(events[1].next_status, "CANCELLED");
}

#[test]
fn reconciliation_duplicate_terminal_snapshot_is_idempotent() {
    let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
    assert!(apply_order_snapshot(&port, &store, 11, Some(5.0)));
    let first = store
        .get_order("rust-order-reconcile")
        .expect("load first terminal order")
        .expect("first terminal order exists");
    let events_before = store
        .list_order_events("rust-order-reconcile")
        .expect("list first terminal events")
        .len();

    assert!(!apply_order_snapshot(&port, &store, 11, Some(5.0)));
    let duplicate = store
        .get_order("rust-order-reconcile")
        .expect("load duplicate terminal order")
        .expect("duplicate terminal order exists");
    assert_eq!(duplicate.status, "FILLED");
    assert_eq!(duplicate.filled_quantity, Some(5.0));
    assert_eq!(duplicate.updated_at, first.updated_at);
    assert_eq!(
        store
            .list_order_events("rust-order-reconcile")
            .expect("list duplicate terminal events")
            .len(),
        events_before
    );
}
