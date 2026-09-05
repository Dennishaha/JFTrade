use super::*;

#[test]
fn reconciliation_uses_extended_identity_when_numeric_identity_is_missing() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![order_snapshot(5, Some(0.0))],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), reader);
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "SUBMITTED");
    assert_eq!(saved.broker_order_id.as_deref(), Some("11"));
    assert_eq!(saved.broker_order_id_ex.as_deref(), Some("order-ex"));
}

#[test]
fn reconciliation_does_not_guess_an_identity_for_an_uncertain_submission() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![order_snapshot(5, Some(0.0))],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    for _ in 0..2 {
        assert!(port.reconcile_pending_orders().is_err());
        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "UNKNOWN");
        assert_eq!(saved.last_error_code.as_deref(), Some("EXECUTION_STATE_UNKNOWN"));
        assert!(saved.broker_order_id.is_none());
        assert!(saved.broker_order_id_ex.is_none());
        assert_eq!(saved.client_order_id.as_deref(), Some("client-reconcile"));
        assert_eq!(store.list_order_events(&saved.internal_order_id).unwrap().len(), 1);
    }
    assert_eq!(reader.calls.lock().unwrap().active_orders, 0);
}

#[test]
fn reconciliation_stale_fill_quantity_cannot_replace_the_average_price() {
    for stale_quantity in [Some(1.0), None, Some(-1.0), Some(f64::NAN)] {
        let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
        let mut current = store.get_order("rust-order-reconcile").unwrap().unwrap();
        let mut snapshot = order_snapshot(10, Some(3.0));
        snapshot.fill_avg_price = Some(105.0);
        assert!(port.apply_broker_snapshot(&current, &snapshot, 0).unwrap());
        current = store.get_order("rust-order-reconcile").unwrap().unwrap();
        let revision = store.order_revision(&current.internal_order_id).unwrap();
        snapshot.fill_qty = stale_quantity;
        snapshot.fill_avg_price = Some(90.0);
        assert!(!port.apply_broker_snapshot(&current, &snapshot, revision).unwrap());
        let saved = store.get_order(&current.internal_order_id).unwrap().unwrap();
        assert_eq!(saved.filled_quantity, Some(3.0));
        assert_eq!(saved.filled_average_price, Some(105.0));
        assert_eq!(store.list_order_events(&current.internal_order_id).unwrap().len(), 1);
    }
}

#[test]
fn reconciliation_advancing_fill_quantity_updates_its_average_price() {
    let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
    for (quantity, price) in [(2.0, 100.0), (3.0, 105.0)] {
        let current = store.get_order("rust-order-reconcile").unwrap().unwrap();
        let revision = store.order_revision(&current.internal_order_id).unwrap();
        let mut snapshot = order_snapshot(10, Some(quantity));
        snapshot.fill_avg_price = Some(price);
        assert!(port.apply_broker_snapshot(&current, &snapshot, revision).unwrap());
        let saved = store.get_order(&current.internal_order_id).unwrap().unwrap();
        assert_eq!(saved.filled_quantity, Some(quantity));
        assert_eq!(saved.filled_average_price, Some(price));
    }
}

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
