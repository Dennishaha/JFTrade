use super::*;

fn correlated_order_snapshot(status: i32, fill_qty: Option<f64>) -> TradeOrderSnapshot {
    let mut snapshot = super::order_snapshot(status, fill_qty);
    snapshot.remark = Some("client-reconcile".to_owned());
    snapshot
}

#[test]
fn reconciliation_uses_extended_identity_when_numeric_identity_is_missing() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![correlated_order_snapshot(5, Some(0.0))],
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
    let mut snapshot1 = correlated_order_snapshot(5, Some(0.0));
    snapshot1.order_id = 11;
    snapshot1.order_id_ex = "order-ex-1".to_owned();
    let mut snapshot2 = correlated_order_snapshot(5, Some(0.0));
    snapshot2.order_id = 12;
    snapshot2.order_id_ex = "order-ex-2".to_owned();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snapshot1, snapshot2],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    for _ in 0..2 {
        assert!(port.reconcile_pending_orders().is_err());
        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "UNKNOWN");
        assert_eq!(
            saved.last_error_code.as_deref(),
            Some("EXECUTION_STATE_AMBIGUOUS")
        );
        assert!(saved.broker_order_id.is_none());
        assert!(saved.broker_order_id_ex.is_none());
        assert_eq!(saved.client_order_id.as_deref(), Some("client-reconcile"));
        assert_eq!(
            store.list_order_events(&saved.internal_order_id).unwrap().len(),
            1
        );
    }
    assert!(reader.calls.lock().unwrap().active_orders > 0);
}

#[test]
fn reconciliation_crash_window_recovers_submitting_order_and_binds_broker_ids() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![correlated_order_snapshot(5, Some(0.0))],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "SUBMITTED");
    assert_eq!(saved.broker_order_id.as_deref(), Some("11"));
    assert_eq!(saved.broker_order_id_ex.as_deref(), Some("order-ex"));
    assert_eq!(reader.calls.lock().unwrap().active_orders, 1);
}

#[test]
fn reconciliation_crash_window_recovers_filled_order_and_reconciles_fills() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let mut snap = correlated_order_snapshot(11, Some(5.0));
    snap.fill_avg_price = Some(99.0);
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap],
        active_fills: vec![fill("2026-08-30T00:00:02Z", 5.0, "fill-1")],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "FILLED");
    assert_eq!(saved.broker_order_id.as_deref(), Some("11"));
    assert_eq!(saved.broker_order_id_ex.as_deref(), Some("order-ex"));
    assert_eq!(saved.filled_quantity, Some(5.0));
    assert_eq!(saved.filled_average_price, Some(99.0));
    assert!(reader.calls.lock().unwrap().active_fills > 0);
}

#[test]
fn reconciliation_no_candidate_preserves_unknown_submission() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert!(port.reconcile_pending_orders().is_err());
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "UNKNOWN");
    assert_eq!(
        saved.last_error_code.as_deref(),
        Some("BROKER_ORDER_NOT_FOUND")
    );
    assert!(saved.broker_order_id.is_none());
    assert!(saved.broker_order_id_ex.is_none());
    let events = store.list_order_events(&saved.internal_order_id).unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].event_type, "reconcile_order_missing");
    assert_eq!(events[0].next_status, "UNKNOWN");
}

#[test]
fn reconciliation_foreign_order_protection_ignores_unmatched_remark() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    order.client_order_id = Some("strategy-instance-intent-1".to_owned());
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let mut foreign_order = correlated_order_snapshot(5, Some(0.0));
    foreign_order.remark = Some("manual-app-trader".to_owned());
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![foreign_order],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert!(port.reconcile_pending_orders().is_err());
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "UNKNOWN");
    assert_eq!(
        saved.last_error_code.as_deref(),
        Some("BROKER_ORDER_NOT_FOUND")
    );
    assert!(saved.broker_order_id.is_none());
}

#[test]
fn reconciliation_claimed_order_exclusion_protects_against_cross_binding() {
    let (store, _directory) = reconciliation_store();
    let mut existing_order = pending_order("SUBMITTED");
    existing_order.internal_order_id = "rust-order-existing".to_owned();
    existing_order.broker_order_id = Some("11".to_owned());
    existing_order.broker_order_id_ex = Some("order-ex".to_owned());
    store.save_order(existing_order, "2026-08-30T00:00:01Z").unwrap();

    let mut pending = pending_order("SUBMITTING");
    pending.internal_order_id = "rust-order-pending".to_owned();
    pending.client_order_id = Some("client-reconcile-pending".to_owned());
    pending.broker_order_id = None;
    pending.broker_order_id_ex = None;
    store.save_order(pending, "2026-08-30T00:00:01Z").unwrap();

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![correlated_order_snapshot(5, Some(0.0))],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    let _ = port.reconcile_pending_orders();
    let pending_saved = store.get_order("rust-order-pending").unwrap().unwrap();
    assert_eq!(pending_saved.status, "UNKNOWN");
    assert!(pending_saved.broker_order_id.is_none());
}

#[test]
fn reconciliation_priority_1_matches_client_order_id_in_remark() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    order.client_order_id = Some("my-unique-client-order-id".to_owned());
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let mut snap1 = correlated_order_snapshot(5, Some(0.0));
    snap1.order_id = 101;
    snap1.order_id_ex = "ex-101".to_owned();
    snap1.remark = Some("other-order".to_owned());

    let mut snap2 = correlated_order_snapshot(5, Some(0.0));
    snap2.order_id = 102;
    snap2.order_id_ex = "ex-102".to_owned();
    snap2.remark = Some("my-unique-client-order-id".to_owned());

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap1, snap2],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "SUBMITTED");
    assert_eq!(saved.broker_order_id.as_deref(), Some("102"));
    assert_eq!(saved.broker_order_id_ex.as_deref(), Some("ex-102"));
}

#[test]
fn reconciliation_stale_fill_quantity_cannot_replace_the_average_price() {
    for stale_quantity in [Some(1.0), None, Some(-1.0), Some(f64::NAN)] {
        let (store, port, _directory) = persist_reconciliation_order("SUBMITTED");
        let mut current = store.get_order("rust-order-reconcile").unwrap().unwrap();
        let mut snapshot = correlated_order_snapshot(10, Some(3.0));
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
        let mut snapshot = correlated_order_snapshot(10, Some(quantity));
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
    port.apply_broker_snapshot(&current, &correlated_order_snapshot(status, filled), revision)
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

#[test]
fn reconciliation_extended_id_matching_and_binding_without_numeric_id() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    order.symbol = Some("US.AAPL".to_owned());
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let mut snapshot = correlated_order_snapshot(5, Some(0.0));
    snapshot.order_id = 0; // Pure extended ID with NO numeric ID
    snapshot.order_id_ex = "combo-option-ex-999".to_owned();

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snapshot],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "SUBMITTED");
    assert_eq!(saved.broker_order_id, None, "numeric ID must remain None");
    assert_eq!(
        saved.broker_order_id_ex.as_deref(),
        Some("combo-option-ex-999"),
        "extended ID must be bound"
    );
}

#[test]
fn reconciliation_partial_fill_during_crash_window_recovers_to_partially_filled() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let mut snap = correlated_order_snapshot(10, Some(2.0)); // 10 is FILLED_PART
    snap.fill_avg_price = Some(98.5);
    let mut fill_item = fill("2026-08-30T00:00:02Z", 2.0, "fill-part-1");
    fill_item.order_id = Some(11);
    fill_item.order_id_ex = Some("order-ex".to_owned());

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap],
        active_fills: vec![fill_item],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "PARTIALLY_FILLED");
    assert_eq!(saved.filled_quantity, Some(2.0));
    assert_eq!(saved.filled_average_price, Some(98.5));
    assert_eq!(saved.broker_order_id.as_deref(), Some("11"));
    assert_eq!(saved.broker_order_id_ex.as_deref(), Some("order-ex"));
}

#[test]
fn reconciliation_time_proximity_is_not_identity_evidence() {
    let (store, _directory) = reconciliation_store();

    // Order A: submitted at 2026-08-30T00:05:00Z
    let mut order_a = pending_order("SUBMITTING");
    order_a.internal_order_id = "order-299s".to_owned();
    order_a.client_order_id = Some("client-299s".to_owned());
    order_a.broker_order_id = None;
    order_a.broker_order_id_ex = None;
    order_a.submitted_at = Some("2026-08-30T00:05:00Z".to_owned());
    order_a.created_at = "2026-08-30T00:05:00Z".to_owned();
    store.save_order(order_a, "2026-08-30T00:05:00Z").unwrap();

    // Order B: submitted at 2026-08-30T00:05:00Z
    let mut order_b = pending_order("SUBMITTING");
    order_b.internal_order_id = "order-301s".to_owned();
    order_b.client_order_id = Some("client-301s".to_owned());
    order_b.broker_order_id = None;
    order_b.broker_order_id_ex = None;
    order_b.requested_quantity = Some(10.0); // distinguish from Order A (qty 5.0)
    order_b.submitted_at = Some("2026-08-30T00:05:00Z".to_owned());
    order_b.created_at = "2026-08-30T00:05:00Z".to_owned();
    store.save_order(order_b, "2026-08-30T00:05:00Z").unwrap();

    // Candidate A: created at 2026-08-30T00:09:59Z (+299s) -> still not identity evidence
    let mut snap_a = correlated_order_snapshot(5, Some(0.0));
    snap_a.order_id = 101;
    snap_a.order_id_ex = "ex-101".to_owned();
    snap_a.qty = 5.0;
    snap_a.remark = None;
    snap_a.create_time = "2026-08-30T00:09:59Z".to_owned();

    // Candidate B: created at 2026-08-30T00:10:01Z (+301s) -> still not rejection evidence
    let mut snap_b = correlated_order_snapshot(5, Some(0.0));
    snap_b.order_id = 102;
    snap_b.order_id_ex = "ex-102".to_owned();
    snap_b.qty = 10.0;
    snap_b.remark = None;
    snap_b.create_time = "2026-08-30T00:10:01Z".to_owned();

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap_a, snap_b],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    let _ = port.reconcile_pending_orders();

    let saved_a = store.get_order("order-299s").unwrap().unwrap();
    assert_eq!(
        saved_a.status, "UNKNOWN",
        "A nearby order must not be guessed as ours"
    );
    assert!(saved_a.broker_order_id.is_none());

    let saved_b = store.get_order("order-301s").unwrap().unwrap();
    assert_eq!(
        saved_b.status, "UNKNOWN",
        "A distant order cannot establish submission failure"
    );
    assert_eq!(
        saved_b.last_error_code.as_deref(),
        Some("BROKER_ORDER_NOT_FOUND")
    );
}

#[test]
fn reconciliation_priority_1_matches_even_beyond_300s_window() {
    let (store, _directory) = reconciliation_store();

    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    order.client_order_id = Some("client-order-p1-window-test".to_owned());
    order.submitted_at = Some("2026-08-30T00:00:00Z".to_owned());
    order.created_at = "2026-08-30T00:00:00Z".to_owned();
    store.save_order(order, "2026-08-30T00:00:00Z").unwrap();

    // Broker candidate created 600 seconds after order submission, but has matching remark
    let mut snap = correlated_order_snapshot(5, Some(0.0));
    snap.order_id = 999;
    snap.order_id_ex = "ex-999".to_owned();
    snap.remark = Some("client-order-p1-window-test".to_owned());
    snap.create_time = "2026-08-30T00:10:00Z".to_owned(); // +600s

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(
        saved.status, "SUBMITTED",
        "Priority 1 match on client_order_id in remark is not restricted by 300s attribute window"
    );
    assert_eq!(saved.broker_order_id.as_deref(), Some("999"));
}

#[test]
fn challenge_edge_case_1_three_identical_broker_orders_no_client_id_remains_unknown() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    order.client_order_id = None;
    order.remark = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let mut snap1 = correlated_order_snapshot(5, Some(0.0));
    snap1.order_id = 101;
    snap1.order_id_ex = "ex-101".to_owned();
    snap1.remark = None;

    let mut snap2 = correlated_order_snapshot(5, Some(0.0));
    snap2.order_id = 102;
    snap2.order_id_ex = "ex-102".to_owned();
    snap2.remark = None;

    let mut snap3 = correlated_order_snapshot(5, Some(0.0));
    snap3.order_id = 103;
    snap3.order_id_ex = "ex-103".to_owned();
    snap3.remark = None;

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap1, snap2, snap3],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));

    let res = port.reconcile_pending_orders();
    assert!(res.is_err(), "Ambiguous candidates must return Err");

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "UNKNOWN");
    assert_eq!(
        saved.last_error_code.as_deref(),
        Some("BROKER_ORDER_NOT_FOUND")
    );
    assert!(saved.broker_order_id.is_none(), "Must NOT guess numeric ID");
    assert!(saved.broker_order_id_ex.is_none(), "Must NOT guess extended ID");
}

#[test]
fn challenge_edge_case_2_different_symbol_and_conflicting_remark_not_claimed() {
    let (store, _directory) = reconciliation_store();

    // Order 1: US.AAPL
    let mut order = pending_order("SUBMITTING");
    order.symbol = Some("US.AAPL".to_owned());
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    order.client_order_id = Some("strategy-client-aapl".to_owned());
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    // Broker candidate 1: Different symbol (US.TSLA)
    let mut snap_tsla = correlated_order_snapshot(5, Some(0.0));
    snap_tsla.order_id = 201;
    snap_tsla.order_id_ex = "ex-201".to_owned();
    snap_tsla.code = "US.TSLA".to_owned();

    // Broker candidate 2: Same symbol but conflicting foreign remark
    let mut snap_foreign = correlated_order_snapshot(5, Some(0.0));
    snap_foreign.order_id = 202;
    snap_foreign.order_id_ex = "ex-202".to_owned();
    snap_foreign.code = "US.AAPL".to_owned();
    snap_foreign.remark = Some("manual-phone-order".to_owned());

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snap_tsla, snap_foreign],
        history_orders: vec![],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert!(port.reconcile_pending_orders().is_err());

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(saved.status, "UNKNOWN");
    assert_eq!(
        saved.last_error_code.as_deref(),
        Some("BROKER_ORDER_NOT_FOUND")
    );
    assert!(saved.broker_order_id.is_none());
    assert!(saved.broker_order_id_ex.is_none());
}

#[test]
fn challenge_edge_case_3_broker_network_failure_does_not_mutate_or_release_quota() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        fail_active_orders: true,
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));

    let res = port.reconcile_pending_orders();
    assert!(res.is_err(), "Network failure must return error");

    let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
    assert_eq!(
        saved.status, "SUBMITTING",
        "Order MUST remain in SUBMITTING when broker read fails"
    );
    assert!(saved.broker_order_id.is_none());
    assert!(saved.broker_order_id_ex.is_none());
    assert_eq!(
        store.list_order_events("rust-order-reconcile").unwrap().len(),
        0,
        "No state transition events allowed during connection error"
    );
}

#[test]
fn challenge_edge_case_4_order_state_transitions_filled_cancelled_rejected() {
    // 4a. FILLED
    {
        let (store, _directory) = reconciliation_store();
        let mut order = pending_order("SUBMITTING");
        order.broker_order_id = None;
        order.broker_order_id_ex = None;
        store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

        let mut snap = correlated_order_snapshot(11, Some(5.0)); // 11 is FILLED_ALL
        snap.order_id = 301;
        snap.order_id_ex = "ex-301".to_owned();
        snap.fill_avg_price = Some(100.0);

        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            active_orders: vec![snap],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), Arc::clone(&reader));
        assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "FILLED");
        assert_eq!(saved.broker_order_id.as_deref(), Some("301"));
        assert_eq!(saved.broker_order_id_ex.as_deref(), Some("ex-301"));
        assert_eq!(saved.filled_quantity, Some(5.0));
        assert_eq!(saved.filled_average_price, Some(100.0));
    }

    // 4b. CANCELLED
    {
        let (store, _directory) = reconciliation_store();
        let mut order = pending_order("SUBMITTING");
        order.broker_order_id = None;
        order.broker_order_id_ex = None;
        store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

        let mut snap = correlated_order_snapshot(14, Some(0.0)); // 14 is CANCELLED_ALL
        snap.order_id = 302;
        snap.order_id_ex = "ex-302".to_owned();

        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            active_orders: vec![snap],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), Arc::clone(&reader));
        assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "CANCELLED");
        assert_eq!(saved.broker_order_id.as_deref(), Some("302"));
        assert_eq!(saved.broker_order_id_ex.as_deref(), Some("ex-302"));
    }

    // 4c. REJECTED / FAILED at broker
    {
        let (store, _directory) = reconciliation_store();
        let mut order = pending_order("SUBMITTING");
        order.broker_order_id = None;
        order.broker_order_id_ex = None;
        store.save_order(order, "2026-08-30T00:00:01Z").unwrap();

        let mut snap = correlated_order_snapshot(21, Some(0.0)); // 21 is FAILED (rejected by broker)
        snap.order_id = 303;
        snap.order_id_ex = "ex-303".to_owned();
        snap.last_err_msg = Some("broker risk rejection: margin insufficient".to_owned());

        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            active_orders: vec![snap],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), Arc::clone(&reader));
        assert_eq!(port.reconcile_pending_orders().unwrap(), 1);

        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "FAILED");
        assert_eq!(saved.broker_order_id.as_deref(), Some("303"));
        assert_eq!(saved.broker_order_id_ex.as_deref(), Some("ex-303"));
        assert_eq!(
            saved.last_error.as_deref(),
            Some("broker risk rejection: margin insufficient")
        );
    }
}
