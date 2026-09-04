use super::*;
use rusqlite::Connection;
use std::sync::{Arc, Mutex};
use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderStore,
    STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    StoredExecutionOrder,
};

#[derive(Debug, Default)]
struct MockNotificationPort {
    delivered: Mutex<Vec<ProductNotificationRequest>>,
}

impl ProductNotificationPort for MockNotificationPort {
    fn deliver(
        &self,
        request: ProductNotificationRequest,
    ) -> crate::product::ProductNotificationDelivery {
        self.delivered.lock().unwrap().push(request);
        crate::product::ProductNotificationDelivery {
            delivered: true,
            status: "delivered".to_owned(),
            message: "delivered".to_owned(),
        }
    }
}

#[derive(Debug, Default)]
struct MockExecutionPort {
    mutations: Mutex<Vec<ExecutionWriteInput>>,
}

impl ExecutionWritePort for MockExecutionPort {
    fn mutate(
        &self,
        input: &ExecutionWriteInput,
    ) -> Result<Value, ExecutionWritePortError> {
        self.mutations.lock().unwrap().push(input.clone());
        Ok(json!({"internalOrderId": "mock-order-1"}))
    }
}

fn seed_strategy_test_db(path: &std::path::Path) {
    let conn = Connection::open(path).expect("open test db");
    jftrade_store_sqlite::initialize_current(&conn, "strategy")
        .expect("initialize strategy schema");
}

fn seed_execution_test_db(path: &std::path::Path) {
    let conn = Connection::open(path).expect("open test db");
    jftrade_store_sqlite::initialize_current(&conn, "execution-orders")
        .expect("initialize execution schema");
}

fn test_intent(qty: f64, limit_price: f64) -> PineOrderIntent {
    PineOrderIntent {
        kind: "order".to_owned(),
        id: "entry-1".to_owned(),
        from_entry: String::new(),
        direction: "buy".to_owned(),
        quantity: qty,
        quantity_pct: 0.0,
        limit_price,
        stop_price: 0.0,
        comment: String::new(),
        alert_message: String::new(),
        disable_alert: false,
        bar_index: 10,
        time: 1700000000,
        has_quantity: true,
        has_quantity_pct: false,
        has_limit_price: limit_price > 0.0,
        has_stop_price: false,
        parent_id: String::new(),
        atomic_group_id: String::new(),
        oco_group_id: String::new(),
        reduce_only: false,
    }
}

#[test]
fn test_notify_strategy_intents_delivers_and_records_audit() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-notify", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let notifier = MockNotificationPort::default();
    let intents = vec![test_intent(10.0, 150.0)];

    notify_strategy_intents(
        Some(&notifier),
        &store,
        "inst-notify",
        "US.AAPL",
        &intents,
    )
    .expect("notify strategy intents");

    let delivered = notifier.delivered.lock().unwrap();
    assert_eq!(delivered.len(), 1);
    assert_eq!(delivered[0].title, "策略下单信号");
    assert!(delivered[0].body.contains("(仅通知模式)"));
    assert!(delivered[0].body.contains("US.AAPL BUY 10"));

    let audit = store.list_audit_events("inst-notify").expect("list audit");
    assert_eq!(audit.len(), 1);
    assert_eq!(audit[0].kind, "SIGNAL_NOTIFIED");
    assert!(audit[0].detail.contains("(仅通知模式)"));
}

#[test]
fn test_execute_strategy_intents_risk_rejection_blocks_broker_order() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-risk", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let risk_json = json!({
        "mode": "enforce",
        "maxOrderQuantity": 5.0,
        "pauseOnReject": true
    });
    store
        .update_risk("inst-risk", risk_json, "2026-08-30T00:00:01Z")
        .expect("update risk");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-risk",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_err(), "must reject order violating risk");
    let err_msg = res.unwrap_err();
    assert!(err_msg.contains("runtime risk rejected"));

    assert_eq!(execution.mutations.lock().unwrap().len(), 0);

    let audit = store.list_audit_events("inst-risk").expect("audit events");
    assert!(audit.iter().any(|ev| ev.kind == "RUNTIME_RISK_REJECTED"));

    let inst = store.get_instance("inst-risk").unwrap().unwrap();
    assert_eq!(inst.status, "PAUSED");
}

#[test]
fn test_execute_strategy_intents_success_calls_execution_and_audits() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-ok", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-ok",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_ok());

    assert_eq!(execution.mutations.lock().unwrap().len(), 1);

    let audit = store.list_audit_events("inst-ok").expect("audit events");
    assert!(audit.iter().any(|ev| ev.kind == "ORDER_SUBMITTED"));
}

#[test]
fn test_execute_strategy_intents_unknown_risk_mode_fails_closed() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-unknown-risk", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");
    store
        .update_risk(
            "inst-unknown-risk",
            json!({ "mode": "unsupported_mode" }),
            "2026-08-30T00:00:01Z",
        )
        .expect("update risk");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-unknown-risk",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_err());
    assert!(res.unwrap_err().contains("unknown strategy runtime risk mode"));
}

#[test]
fn test_execute_strategy_intents_resolves_quantity_pct_and_close_intent() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-pct", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE",
        "orderSize": 200.0,
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-pct",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: Some(1.0),
        available_cash: Some(30_000.0),
    };

    let mut pct_intent = test_intent(0.0, 150.0);
    pct_intent.has_quantity = false;
    pct_intent.has_quantity_pct = true;
    pct_intent.quantity_pct = 25.0;

    let mut close_intent = test_intent(0.0, 0.0);
    close_intent.kind = "close".to_owned();
    close_intent.direction = "long".to_owned();
    close_intent.has_quantity = false;
    close_intent.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[pct_intent, close_intent]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 2);
    assert_eq!(mutations[0].payload["quantity"], 50.0);
    assert_eq!(mutations[0].payload["reduceOnly"], false);
    assert_eq!(mutations[0].payload["source"], "strategy-runtime");
    assert_eq!(mutations[0].payload["sourceDetail"], "inst-pct");
    assert_eq!(mutations[1].payload["quantity"], 1.0);
    assert_eq!(mutations[1].payload["reduceOnly"], true);
    assert_eq!(mutations[1].payload["side"], "SELL");
    assert!(mutations[1].payload["clientOrderId"].as_str().unwrap().starts_with("strategy-inst-pct-US.AAPL-"));
}

#[test]
fn test_execute_strategy_intents_revision_fence_mismatch_blocks_and_audits() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-fence", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-fence",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: Some(999), // Expected 999, but DB has 1
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_err(), "must reject on revision drift");
    let err_msg = res.unwrap_err();
    assert!(err_msg.contains("runtime risk revision fence triggered"));

    let audit = store.list_audit_events("inst-fence").expect("audit events");
    assert!(audit.iter().any(|ev| ev.kind == "RUNTIME_RISK_REVISION_MISMATCH"));
}

#[test]
fn test_execute_strategy_intents_close_short_maps_to_buy() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-short-close", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-short-close",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(150.0),
        sellable_quantity: Some(20.0),
        current_position: Some(-20.0),
        available_cash: None,
    };

    let mut close_short = test_intent(20.0, 0.0);
    close_short.kind = "close".to_owned();
    close_short.direction = "short".to_owned();
    close_short.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[close_short]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].payload["side"], "BUY");
    assert_eq!(mutations[0].payload["quantity"], 20.0);
    assert_eq!(mutations[0].payload["reduceOnly"], true);
}

#[test]
fn test_execute_strategy_intents_cancel_dispatches_order_cancel() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-cancel", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-cancel",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
    };

    let mut cancel_intent = test_intent(0.0, 0.0);
    cancel_intent.kind = "cancel".to_owned();
    cancel_intent.id = "target-order-42".to_owned();
    cancel_intent.has_quantity = false;
    cancel_intent.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[cancel_intent]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].operation, ExecutionWriteOperation::OrderCancel);
    assert_eq!(
        mutations[0].internal_order_id,
        Some("target-order-42".to_owned())
    );

    let audit = store.list_audit_events("inst-cancel").expect("audit events");
    assert!(audit.iter().any(|ev| ev.kind == "ORDER_CANCELLED"));
}

#[test]
fn test_execute_strategy_intents_parameterless_close_skips_when_no_position() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-close-skip", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-close-skip",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(0.0),
        current_position: Some(0.0),
        available_cash: None,
    };

    let mut close_intent = test_intent(0.0, 0.0);
    close_intent.kind = "close".to_owned();
    close_intent.has_quantity = false;
    close_intent.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[close_intent]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(
        mutations.len(),
        0,
        "should not place any order when closing zero position"
    );

    let audit = store
        .list_audit_events("inst-close-skip")
        .expect("audit events");
    assert!(
        audit
            .iter()
            .any(|ev| ev.kind == "INTENT_SKIPPED" && ev.detail.contains("no open position"))
    );
}

#[test]
fn test_execute_strategy_intents_cancel_all_queries_and_cancels_active_orders() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strat_path = dir.path().join("strategy.db");
    seed_strategy_test_db(&strat_path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strat_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-cancel-all", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let exec_path = dir.path().join("execution.db");
    seed_execution_test_db(&exec_path);
    let exec_store =
        ExecutionOrderStore::open_existing(&exec_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open exec store");

    let order = StoredExecutionOrder {
        internal_order_id: "ord-active-1".to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: Some("futu-1".to_owned()),
        broker_order_id_ex: None,
        source: "strategy-runtime".to_owned(),
        source_detail: "inst-cancel-all".to_owned(),
        trading_environment: "SIMULATE".to_owned(),
        account_id: "12345".to_owned(),
        market: "US".to_owned(),
        symbol: Some("AAPL".to_owned()),
        side: Some("BUY".to_owned()),
        order_type: Some("LIMIT".to_owned()),
        status: "SUBMITTED".to_owned(),
        raw_broker_status: None,
        requested_quantity: Some(10.0),
        requested_price: Some(150.0),
        filled_quantity: None,
        filled_average_price: None,
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: None,
        updated_at: "2026-08-30T00:00:00Z".to_owned(),
        created_at: "2026-08-30T00:00:00Z".to_owned(),
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "units".to_owned(),
        client_order_id: Some("strat-client-1".to_owned()),
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: None,
        payout: None,
        fees: None,
    };
    exec_store
        .save_order(order, "2026-08-30T00:00:00Z")
        .expect("save order");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id: "inst-cancel-all",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
    };

    let mut cancel_all = test_intent(0.0, 0.0);
    cancel_all.kind = "cancel_all".to_owned();
    cancel_all.has_quantity = false;
    cancel_all.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[cancel_all]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].operation, ExecutionWriteOperation::OrderCancel);
    assert_eq!(
        mutations[0].internal_order_id,
        Some("ord-active-1".to_owned())
    );

    let audit = store
        .list_audit_events("inst-cancel-all")
        .expect("audit events");
    assert!(
        audit
            .iter()
            .any(|ev| ev.kind == "ORDER_CANCELLED" && ev.detail.contains("ord-active-1"))
    );
}
