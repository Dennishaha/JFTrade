//! Fee reconciliation through the production owner and its durable ledger.
use super::super::super::order_value;
use super::*;

// Parity: go:452dea11:internal/store/trading/broker_ledger_test.go:65 TestBrokerFeeNilAndMissingOrderBoundaries
#[test]
fn reconciliation_empty_and_unknown_fees_leave_the_durable_ledger_unchanged() {
    let mut empty = fee(0.0);
    empty.broker_order_id_ex.clear();
    empty.fee_amount = None;
    let mut missing_amount = empty.clone();
    missing_amount.broker_order_id_ex = "order-ex".to_owned();
    let mut unknown = fee(1.5);
    unknown.broker_order_id_ex = "missing-order".to_owned();
    for snapshot in [empty, missing_amount, unknown] {
        let (store, directory) = reconciliation_store();
        let before = store
            .save_order(pending_order("FILLED"), "2026-08-30T00:00:01Z")
            .expect("known terminal order");
        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            fees: vec![snapshot.clone()],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), Arc::clone(&reader));
        for _ in 0..2 {
            assert_eq!(port.reconcile_pending_orders().expect("fee scan"), 0);
            assert_eq!(store.list_orders().expect("ledger"), vec![before.clone()]);
            assert!(store.get_order("missing-order").expect("unknown").is_none());
            assert!(
                store
                    .list_order_events(&before.internal_order_id)
                    .expect("events")
                    .is_empty()
            );
            assert_eq!(
                store
                    .order_revision(&before.internal_order_id)
                    .expect("revision"),
                0
            );
        }
        assert_eq!(reader.calls.lock().expect("calls").fees, 2, "{snapshot:?}");
        drop(port);
        drop(store);
        let reopened = jftrade_store_sqlite::ExecutionOrderStore::open(
            directory.path().join("execution-orders.db"),
        )
        .expect("reopen ledger");
        assert_eq!(
            reopened.list_orders().expect("durable ledger"),
            vec![before]
        );
        assert!(
            reopened
                .list_order_events("rust-order-reconcile")
                .expect("durable events")
                .is_empty()
        );
    }

    // The original empty-ledger input must not invent a fee-only order either.
    let (store, _directory) = reconciliation_store();
    let mut unknown = fee(1.5);
    unknown.broker_order_id_ex = "missing-order".to_owned();
    let port = production_port(
        Arc::clone(&store),
        Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            fees: vec![unknown],
            ..Default::default()
        }),
    );
    assert_eq!(
        port.reconcile_pending_orders().expect("empty ledger scan"),
        0
    );
    assert!(store.list_orders().expect("empty ledger").is_empty());
    assert!(
        store
            .list_order_events("missing-order")
            .expect("missing events")
            .is_empty()
    );
}

// Parity: go:452dea11:internal/store/trading/ledger_lifecycle_test.go:73 TestExecutionStorePersistsParentBrokerFeesWithoutInventingLegAllocation
#[test]
fn reconciliation_parent_fee_sum_and_duplicate_preserve_unallocated_combo_legs() {
    let (store, directory) = reconciliation_store();
    let mut parent = pending_order("FILLED");
    parent.order_kind = "option_combo".to_owned();
    parent.product_class = "option".to_owned();
    parent.quantity_mode = "contracts".to_owned();
    // Rust requires two legs; the original Go single-leg input remains a gap.
    parent.normalized_request = json!({"legs":[
        {"instrumentId":"US.AAPL260717C00200000","productClass":"option","side":"BUY","ratio":1},
        {"instrumentId":"US.AAPL260717P00200000","productClass":"option","side":"BUY","ratio":1}
    ]})
    .to_string();
    let parent = store
        .save_order(parent, "2026-08-30T00:00:01Z")
        .expect("combo");
    let mut summed = fee(0.0);
    summed.fee_amount = None;
    summed.fee_items = vec![
        jftrade_integration_futu::TradeOrderFeeItemSnapshot {
            title: "commission".to_owned(),
            value: 1.25,
        },
        jftrade_integration_futu::TradeOrderFeeItemSnapshot {
            title: "platform".to_owned(),
            value: 0.75,
        },
    ];
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        fee_batches: Mutex::new(vec![vec![fee(2.0)], vec![summed]]),
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), Arc::clone(&reader));
    assert_eq!(port.reconcile_pending_orders().expect("sum fees"), 1);
    let saved = store
        .get_order(&parent.internal_order_id)
        .expect("parent")
        .expect("row");
    assert_eq!(saved.internal_order_id, parent.internal_order_id);
    assert_eq!(saved.fees, Some(2.0));
    assert_eq!(saved.normalized_request, parent.normalized_request);
    let events = store
        .list_order_events(&parent.internal_order_id)
        .expect("fee event");
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].event_type, "BROKER_ORDER_FEES_UPDATED");
    let payload: Value = serde_json::from_str(&events[0].payload_json).expect("payload");
    assert_eq!(payload["feeAmount"], 2.0);
    assert_eq!(payload["feeItems"][0]["value"], 1.25);
    assert_eq!(payload["feeItems"][1]["value"], 0.75);
    // Force a repeated fee read even though routine scans omit paid terminal orders.
    let reader_port: Arc<dyn TradeReadPort> = reader.clone();
    assert!(
        !port
            .reconcile_terminal_fees_only(
                &reader_port,
                header_from_order(&saved).expect("scope"),
                &saved
            )
            .expect("duplicate")
    );
    assert_eq!(reader.calls.lock().expect("calls").fees, 2);
    assert_eq!(
        store
            .get_order(&parent.internal_order_id)
            .expect("duplicate row"),
        Some(saved.clone())
    );
    assert_eq!(
        store
            .list_order_events(&parent.internal_order_id)
            .expect("duplicate events")
            .len(),
        1
    );
    assert_eq!(
        store
            .order_revision(&parent.internal_order_id)
            .expect("revision"),
        1
    );
    drop(port);
    drop(store);
    let reopened = jftrade_store_sqlite::ExecutionOrderStore::open(
        directory.path().join("execution-orders.db"),
    )
    .expect("reopen");
    let durable = reopened
        .get_order(&parent.internal_order_id)
        .expect("durable parent")
        .expect("row");
    assert_eq!(durable, saved);
    let projected = order_value(&durable).expect("combo read projection");
    assert_eq!(projected["fees"], 2.0);
    assert_eq!(projected["legs"].as_array().expect("legs").len(), 2);
    for leg in projected["legs"].as_array().expect("legs") {
        assert!(leg.get("fees").expect("leg fees field").is_null());
    }
    let mut one_leg = durable.clone();
    let mut request: Value = serde_json::from_str(&one_leg.normalized_request).expect("request");
    request["legs"].as_array_mut().expect("legs").pop();
    one_leg.normalized_request = request.to_string();
    assert!(matches!(
        order_value(&one_leg),
        Err(ExecutionWritePortError::Failed { code, message, .. })
            if code == "EXECUTION_ORDER_DATA_INVALID" && message.contains("at least two legs")
    ));
    assert_eq!(
        reopened
            .get_order(&parent.internal_order_id)
            .expect("unchanged parent"),
        Some(durable)
    );
}

#[test]
fn reconciliation_invalid_fee_amounts_reject_without_order_or_event_mutation() {
    for amount in [f64::NAN, f64::INFINITY, -0.5] {
        let (store, _directory) = reconciliation_store();
        let before = store
            .save_order(pending_order("FILLED"), "2026-08-30T00:00:01Z")
            .expect("order");
        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            fees: vec![fee(amount)],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), Arc::clone(&reader));
        let error = port.reconcile_pending_orders().expect_err("invalid fee");
        assert!(error.contains("BROKER_INVALID_RESPONSE"), "{error}");
        assert_eq!(reader.calls.lock().expect("calls").fees, 1);
        assert_eq!(store.list_orders().expect("ledger"), vec![before.clone()]);
        assert!(
            store
                .list_order_events(&before.internal_order_id)
                .expect("events")
                .is_empty()
        );
        assert_eq!(
            store
                .order_revision(&before.internal_order_id)
                .expect("revision"),
            0
        );
    }
}
