use super::*;

#[test]
fn reconciliation_broker_identity_claims_are_scoped_to_the_trading_account() {
    for different_scope in ["broker", "account", "environment", "market"] {
        let (store, _directory) = reconciliation_store();
        let mut existing = pending_order("FILLED");
        existing.internal_order_id = "another-scope".to_owned();
        existing.client_order_id = Some("another-client-id".to_owned());
        existing.fees = Some(0.0);
        match different_scope {
            "broker" => existing.broker_id = "other-broker".to_owned(),
            "account" => existing.account_id = "43".to_owned(),
            "environment" => existing.trading_environment = "SIMULATE".to_owned(),
            "market" => existing.market = "HK".to_owned(),
            _ => unreachable!(),
        }
        store.save_order(existing, "2026-08-30T00:00:01Z").unwrap();
        let mut order = pending_order("SUBMITTING");
        order.broker_order_id = None;
        order.broker_order_id_ex = None;
        store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
        let mut snapshot = order_snapshot(5, Some(0.0));
        snapshot.remark = Some("client-reconcile".to_owned());
        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            active_orders: vec![snapshot],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), reader);
        assert_eq!(port.reconcile_pending_orders().unwrap(), 1, "{different_scope}");
        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "SUBMITTED", "{different_scope}");
        assert_eq!(store.get_order("another-scope").unwrap().unwrap().status, "FILLED");
    }
}

#[test]
fn reconciliation_missing_snapshot_remains_retryable_without_terminal_failure() {
    let (store, _directory) = reconciliation_store();
    let mut order = pending_order("SUBMITTING");
    order.broker_order_id = None;
    order.broker_order_id_ex = None;
    store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), reader);
    for _ in 0..2 {
        assert!(port.reconcile_pending_orders().is_err());
        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "UNKNOWN");
        assert!(saved.broker_order_id.is_none());
        assert_eq!(store.list_reconciliation_candidates().unwrap().len(), 1);
        assert_eq!(store.list_order_events(&saved.internal_order_id).unwrap().len(), 1);
    }
    // A later broker response can resolve UNKNOWN; an empty snapshot was not rejection.
    let mut snapshot = order_snapshot(5, Some(0.0));
    snapshot.remark = Some("client-reconcile".to_owned());
    let reader = Arc::new(FixtureTradeReader {
        accounts: vec![account()],
        active_orders: vec![snapshot],
        ..Default::default()
    });
    let port = production_port(Arc::clone(&store), reader);
    assert_eq!(port.reconcile_pending_orders().unwrap(), 1);
    assert_eq!(store.get_order("rust-order-reconcile").unwrap().unwrap().status, "SUBMITTED");
}

#[test]
fn reconciliation_never_claims_a_single_similar_order_or_a_shared_user_remark() {
    for remark in [None, Some("manual batch")] {
        let (store, _directory) = reconciliation_store();
        let mut order = pending_order("SUBMITTING");
        order.broker_order_id = None;
        order.broker_order_id_ex = None;
        order.remark = remark.map(str::to_owned);
        store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
        let mut snapshot = order_snapshot(5, Some(0.0));
        snapshot.remark = remark.map(str::to_owned);
        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            active_orders: vec![snapshot],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), reader);
        assert!(port.reconcile_pending_orders().is_err());
        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "UNKNOWN");
        assert!(saved.broker_order_id.is_none());
        assert!(saved.broker_order_id_ex.is_none());
    }
}

#[test]
fn reconciliation_client_identity_cannot_override_conflicting_order_attributes() {
    for mismatch in ["side", "quantity", "type", "price", "unsent_identity"] {
        let (store, _directory) = reconciliation_store();
        let mut order = pending_order("SUBMITTING");
        order.broker_order_id = None;
        order.broker_order_id_ex = None;
        if mismatch == "unsent_identity" {
            order.remark = Some("user remark replaced the client id on wire".to_owned());
        }
        store.save_order(order, "2026-08-30T00:00:01Z").unwrap();
        let mut snapshot = order_snapshot(5, Some(0.0));
        snapshot.remark = Some("client-reconcile".to_owned());
        match mismatch {
            "side" => snapshot.trd_side = 2,
            "quantity" => snapshot.qty = 9.0,
            "type" => snapshot.order_type = 2,
            "price" => snapshot.price = Some(101.0),
            _ => {}
        }
        let reader = Arc::new(FixtureTradeReader {
            accounts: vec![account()],
            active_orders: vec![snapshot],
            ..Default::default()
        });
        let port = production_port(Arc::clone(&store), reader);
        assert!(port.reconcile_pending_orders().is_err(), "{mismatch}");
        let saved = store.get_order("rust-order-reconcile").unwrap().unwrap();
        assert_eq!(saved.status, "UNKNOWN", "{mismatch}");
        assert!(saved.broker_order_id.is_none(), "{mismatch}");
    }
}
