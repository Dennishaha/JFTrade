use super::*;
use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderStore,
    STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StoredExecutionOrder, StrategyDefinitionStore,
    StrategyRuntimeStore,
};
use rusqlite::Connection;
use std::sync::{Arc, Mutex};

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
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.mutations.lock().unwrap().push(input.clone());
        Ok(json!({"internalOrderId": "mock-order-1"}))
    }
}

#[derive(Debug, Default)]
struct FailingCancelExecutionPort {
    mutations: Mutex<Vec<ExecutionWriteInput>>,
    failing_order_id: String,
}

#[derive(Debug)]
struct SuccessfulCancelExecutionPort<'a> {
    store: &'a ExecutionOrderStore,
    mutations: Mutex<Vec<ExecutionWriteInput>>,
}

impl<'a> SuccessfulCancelExecutionPort<'a> {
    fn new(store: &'a ExecutionOrderStore) -> Self {
        Self {
            store,
            mutations: Mutex::new(Vec::new()),
        }
    }
}

impl ExecutionWritePort for SuccessfulCancelExecutionPort<'_> {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.mutations.lock().unwrap().push(input.clone());
        let id = input
            .internal_order_id
            .as_deref()
            .ok_or_else(|| ExecutionWritePortError::Unavailable("missing order id".to_owned()))?;
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .map_err(|error| ExecutionWritePortError::Unavailable(error.to_string()))?;
        self.store
            .cancel_order(id, &timestamp)
            .map_err(|error| ExecutionWritePortError::Unavailable(error.to_string()))?;
        Ok(json!({"internalOrderId": id, "status": "cancelled"}))
    }
}

impl FailingCancelExecutionPort {
    fn for_order(order_id: &str) -> Self {
        Self {
            mutations: Mutex::new(Vec::new()),
            failing_order_id: order_id.to_owned(),
        }
    }
}

impl ExecutionWritePort for FailingCancelExecutionPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.mutations.lock().unwrap().push(input.clone());
        if input.internal_order_id.as_deref() == Some(self.failing_order_id.as_str()) {
            return Err(ExecutionWritePortError::Unavailable(format!(
                "cancel failed for {}",
                self.failing_order_id
            )));
        }
        Ok(json!({"internalOrderId": input.internal_order_id}))
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

fn active_strategy_order(instance_id: &str, order_id: &str) -> StoredExecutionOrder {
    StoredExecutionOrder {
        internal_order_id: order_id.to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: Some(format!("broker-{order_id}")),
        broker_order_id_ex: None,
        source: "strategy-runtime".to_owned(),
        source_detail: instance_id.to_owned(),
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
        client_order_id: Some(format!("client-{order_id}")),
        preview_id: None,
        normalized_request: "{}".to_owned(),
        requested_amount: None,
        payout: None,
        fees: None,
    }
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

// Parity: go:11a0f579:internal/strategy/pine_live_executor_test.go:601 TestLiveCommandExecutorGeneratedOrderIDStopsAndTrackingFallbacks
#[test]
fn strategy_client_order_id_preserves_named_and_index_fallbacks() {
    let mut named = test_intent(1.0, 0.0);
    named.kind = "entry".to_owned();
    named.id = "Long".to_owned();
    named.bar_index = 43;
    named.time = 0;
    assert_eq!(
        strategy_client_order_id("inst", "US.AAPL", &named, 0),
        "strategy-inst-US.AAPL-Long-entry-43"
    );

    let mut generated = named;
    generated.id.clear();
    generated.bar_index = 7;
    assert_eq!(
        strategy_client_order_id("inst", "US.AAPL", &generated, 2),
        "strategy-inst-US.AAPL-intent-2-entry-7"
    );
}

fn cancel_boundary_context<'a>(
    execution: &'a dyn ExecutionWritePort,
    execution_store: &'a ExecutionOrderStore,
    provider: &'a ActiveProviderState,
    store: &'a StrategyRuntimeStore,
    instance_id: &'a str,
    binding: &'a Value,
) -> StrategyExecutionContext<'a> {
    StrategyExecutionContext {
        execution: Some(execution),
        execution_store: Some(execution_store),
        provider,
        store,
        instance_id,
        market: "US",
        symbol: "US.AAPL",
        binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_test.go:16 TestStrategyRuntimeNotifyOnlyEmitsSignalNotification
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

    notify_strategy_intents(Some(&notifier), &store, "inst-notify", "US.AAPL", &intents)
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

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:18 TestStrategyRuntimeOrderUsesSharedPreTradeRiskGateway
#[test]
// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:123 TestStrategyRuntimeRiskCloseOnlyRejectsBuyOrder
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
        virtual_account: None,
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

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:49 TestStrategyRuntimeLiveModeRecordsExecutionOrder
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
        virtual_account: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(
        mutations[0].payload["clientOrderId"],
        "strategy-inst-ok-US.AAPL-entry-1-order-1700000000"
    );
    assert_eq!(mutations[0].payload["orderType"], "LIMIT");
    drop(mutations);

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
        virtual_account: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_err());
    assert!(
        res.unwrap_err()
            .contains("unknown strategy runtime risk mode")
    );
}

#[test]
fn test_execute_strategy_intents_unknown_kind_fails_before_broker_side_effects() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);
    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-unknown-kind", "RUNNING", "2026-08-30T00:00:00Z")
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
        instance_id: "inst-unknown-kind",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        virtual_account: None,
        current_position: None,
        available_cash: None,
    };
    let mut intent = test_intent(10.0, 150.0);
    intent.kind = "mystery".to_owned();
    let error =
        execute_strategy_intents(ctx, &[intent]).expect_err("unknown kind must fail closed");
    assert!(error.contains("unsupported strategy order intent kind"));
    assert!(execution.mutations.lock().unwrap().is_empty());
}

#[test]
fn test_execute_strategy_intents_preflights_later_cancel_before_earlier_order() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);
    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-preflight-cancel", "RUNNING", "2026-08-30T00:00:00Z")
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
        instance_id: "inst-preflight-cancel",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        virtual_account: None,
        current_position: None,
        available_cash: None,
    };
    let order = test_intent(10.0, 150.0);
    let mut cancel = test_intent(0.0, 0.0);
    cancel.kind = "cancel".to_owned();
    cancel.id.clear();
    cancel.from_entry.clear();
    cancel.has_quantity = false;
    cancel.has_limit_price = false;

    let error = execute_strategy_intents(ctx, &[order, cancel])
        .expect_err("blank cancellation identity must reject the whole batch");
    assert!(error.contains("cancel command id is required"));
    assert!(execution.mutations.lock().unwrap().is_empty());
}

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:181 TestStrategyRuntimeLiveSizesEntryQuantityPctFromEquity
// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:334 TestStrategyRuntimeLiveDefaultsCloseToFullPosition
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
        virtual_account: None,
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
    assert!(
        mutations[1].payload["clientOrderId"]
            .as_str()
            .unwrap()
            .starts_with("strategy-inst-pct-US.AAPL-")
    );
}

/// Shared fixture for the two intent-sizing cases below: a seeded runtime
/// store, a mock execution port, and a live-ready broker binding with enough
/// cash that a percentage entry would size differently from an explicit
/// quantity.
fn sizing_execution_fixture() -> (
    tempfile::TempDir,
    Arc<StrategyDefinitionStore>,
    StrategyRuntimeStore,
    MockExecutionPort,
    ActiveProviderState,
    serde_json::Value,
) {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-sizing", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE",
        "orderSize": 200.0,
    });
    (dir, def_store, store, execution, provider, binding)
}

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:239 TestStrategyRuntimeLiveUsesExplicitQuantityBeforeQuantityPct
#[test]
fn test_execute_strategy_intents_prefers_explicit_quantity_over_quantity_pct() {
    let (_dir, _def_store, store, execution, provider, binding) = sizing_execution_fixture();
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: Some(20.0),
        available_cash: Some(30_000.0),
        virtual_account: None,
    };

    // The explicit quantity must win over a percentage that would size
    // differently: 50% of 30_000 at 150 would be 100 shares, not 20.
    let mut explicit = test_intent(20.0, 150.0);
    explicit.has_quantity_pct = true;
    explicit.quantity_pct = 50.0;

    let res = execute_strategy_intents(ctx, &[explicit]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].payload["quantity"], 20.0);
    assert_eq!(mutations[0].payload["reduceOnly"], false);
}

// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:286 TestStrategyRuntimeLiveSizesCloseQuantityPctFromPosition
#[test]
fn test_execute_strategy_intents_sizes_close_quantity_pct_from_position() {
    let (_dir, _def_store, store, execution, provider, binding) = sizing_execution_fixture();
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: Some(20.0),
        available_cash: Some(30_000.0),
        virtual_account: None,
    };

    // A close intent with quantityPct sizes from the current position: half of
    // a 20-share long is a 10-share reduce-only SELL.
    let mut close_pct = test_intent(0.0, 0.0);
    close_pct.kind = "close".to_owned();
    close_pct.direction = "long".to_owned();
    close_pct.has_quantity = false;
    close_pct.has_limit_price = false;
    close_pct.has_quantity_pct = true;
    close_pct.quantity_pct = 50.0;

    let res = execute_strategy_intents(ctx, &[close_pct]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].payload["quantity"], 10.0);
    assert_eq!(mutations[0].payload["reduceOnly"], true);
    assert_eq!(mutations[0].payload["side"], "SELL");
}

// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:93 TestLiveCommandExecutorSizesEntryQuantityPctFromEquity
#[test]
fn live_entry_quantity_percent_sizes_from_available_equity_at_the_fallback_price() {
    let (_dir, _def_store, store, execution, provider, binding) = sizing_execution_fixture();
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: Some(1_000.0),
        virtual_account: None,
    };

    let mut intent = test_intent(0.0, 0.0);
    intent.has_quantity = false;
    intent.has_limit_price = false;
    intent.has_quantity_pct = true;
    intent.quantity_pct = 50.0;

    let res = execute_strategy_intents(ctx, &[intent]);
    assert!(res.is_ok(), "percentage entry sizing must succeed: {res:?}");

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(
        mutations[0].payload["quantity"], 5.0,
        "half of 1000 USD at 100 floors to 5 shares"
    );
    assert_eq!(mutations[0].payload["side"], "BUY");
}

// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:119 TestLiveCommandExecutorSizesCloseQuantityPctFromPosition
#[test]
fn live_close_quantity_percent_sizes_from_the_open_position() {
    let (_dir, _def_store, store, execution, provider, binding) = sizing_execution_fixture();
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(10.0),
        current_position: Some(10.0),
        available_cash: Some(1_000.0),
        virtual_account: None,
    };

    let mut intent = test_intent(0.0, 0.0);
    intent.kind = "close".to_owned();
    intent.direction = "long".to_owned();
    intent.has_quantity = false;
    intent.has_limit_price = false;
    intent.has_quantity_pct = true;
    intent.quantity_pct = 50.0;

    let res = execute_strategy_intents(ctx, &[intent]);
    assert!(res.is_ok(), "percentage close sizing must succeed: {res:?}");

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].payload["quantity"], 5.0);
    assert_eq!(mutations[0].payload["side"], "SELL");
    assert_eq!(mutations[0].payload["reduceOnly"], true);
}

// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:150 TestLiveCommandExecutorDefaultsCloseToFullPosition
#[test]
fn live_close_without_quantity_defaults_to_the_full_position() {
    let (_dir, _def_store, store, execution, provider, binding) = sizing_execution_fixture();
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(3.0),
        current_position: Some(3.0),
        available_cash: Some(1_000.0),
        virtual_account: None,
    };

    let mut intent = test_intent(0.0, 0.0);
    intent.kind = "close".to_owned();
    intent.direction = "long".to_owned();
    intent.has_quantity = false;
    intent.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[intent]);
    assert!(res.is_ok(), "default close must succeed: {res:?}");

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(
        mutations[0].payload["quantity"], 3.0,
        "a close without quantity must flatten the whole position"
    );
    assert_eq!(mutations[0].payload["reduceOnly"], true);
}

// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:80 TestLiveCommandExecutorRejectsMissingQuantity
#[test]
fn live_entry_without_quantity_is_rejected_before_any_broker_submission() {
    let (_dir, _def_store, store, execution, provider, mut binding) = sizing_execution_fixture();
    // A quantity default of one is only allowed for the offline simulate
    // binding; a real broker binding has to reject a missing quantity.
    binding["tradingEnvironment"] = json!("REAL");
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: Some(1_000.0),
        virtual_account: None,
    };

    let mut intent = test_intent(0.0, 0.0);
    intent.kind = "entry".to_owned();
    intent.direction = "long".to_owned();
    intent.has_quantity = false;
    intent.has_limit_price = false;

    let err = execute_strategy_intents(ctx, &[intent])
        .expect_err("a live entry without quantity must fail closed");
    assert!(
        err.contains("requires a positive finite quantity"),
        "unexpected error: {err}"
    );
    assert!(
        execution.mutations.lock().unwrap().is_empty(),
        "a rejected entry must not reach the broker"
    );
}

#[test]
fn live_order_below_market_lot_step_is_skipped_without_submission() {
    let (_dir, _def_store, store, execution, provider, mut binding) = sizing_execution_fixture();
    binding["lotSize"] = json!(1.0);
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(10.0),
        current_position: None,
        available_cash: Some(1_000.0),
        virtual_account: None,
    };
    let intent = test_intent(0.5, 100.0);
    execute_strategy_intents(ctx, &[intent]).expect("sub-step quantity should be ignored");
    assert!(execution.mutations.lock().unwrap().is_empty());
    let audit = store
        .list_audit_events("inst-sizing")
        .expect("audit events");
    assert!(audit.iter().any(|event| {
        event.kind == "INTENT_SKIPPED" && event.detail.contains("rounded down to 0")
    }));
}

// Parity: go:452dea11:internal/strategy/live_command_business_boundaries_test.go:490 TestLiveOrderQuantityRespectsMinimumAndPrecision
#[test]
fn live_order_quantity_applies_market_minimum_and_volume_precision() {
    let (_dir, _def_store, store, execution, provider, mut binding) = sizing_execution_fixture();
    binding["minQuantity"] = json!(10.0);
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(20.0),
        current_position: None,
        available_cash: Some(1_000.0),
        virtual_account: None,
    };
    execute_strategy_intents(ctx, &[test_intent(5.0, 100.0)])
        .expect("below-minimum quantity should be skipped");
    assert!(execution.mutations.lock().unwrap().is_empty());

    let (_dir, _def_store, store, execution, provider, mut binding) = sizing_execution_fixture();
    binding["volumePrecision"] = json!(2.0);
    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sizing",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(20.0),
        current_position: None,
        available_cash: Some(1_000.0),
        virtual_account: None,
    };
    execute_strategy_intents(ctx, &[test_intent(1.239, 100.0)])
        .expect("precision quantity should execute");
    assert_eq!(
        execution.mutations.lock().unwrap()[0].payload["quantity"],
        1.23
    );
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
        virtual_account: None,
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_err(), "must reject on revision drift");
    let err_msg = res.unwrap_err();
    assert!(err_msg.contains("runtime risk revision fence triggered"));

    let audit = store.list_audit_events("inst-fence").expect("audit events");
    assert!(
        audit
            .iter()
            .any(|ev| ev.kind == "RUNTIME_RISK_REVISION_MISMATCH")
    );
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

    let ctx = || StrategyExecutionContext {
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
        virtual_account: None,
    };

    let mut close_short = test_intent(20.0, 0.0);
    close_short.kind = "close".to_owned();
    close_short.direction = "short".to_owned();
    close_short.has_limit_price = false;

    let mut wrong_side = close_short.clone();
    wrong_side.direction = "long".to_owned();
    execute_strategy_intents(ctx(), &[wrong_side]).expect("wrong-side close should be skipped");
    assert!(execution.mutations.lock().unwrap().is_empty());

    let res = execute_strategy_intents(ctx(), &[close_short]);
    assert!(res.is_ok());

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    assert_eq!(mutations[0].payload["side"], "BUY");
    assert_eq!(mutations[0].payload["quantity"], 20.0);
    assert_eq!(mutations[0].payload["reduceOnly"], true);
}

// Parity: go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:18 TestLiveOrderPassesStopPriceToExecutionGateway
#[test]
fn strategy_intents_place_stop_market_orders_with_the_stop_price_and_reduce_only_flag() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-stop-order", "RUNNING", "2026-08-30T00:00:00Z")
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
        instance_id: "inst-stop-order",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: Some(1.0),
        current_position: Some(1.0),
        available_cash: None,
        virtual_account: None,
    };

    let mut stop_market = test_intent(1.0, 0.0);
    stop_market.kind = "close".to_owned();
    stop_market.direction = "sell".to_owned();
    stop_market.has_limit_price = false;
    stop_market.has_stop_price = true;
    stop_market.stop_price = 95.25;

    let res = execute_strategy_intents(ctx, &[stop_market]);
    assert!(res.is_ok(), "stop-market close must be accepted: {res:?}");

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 1);
    let payload = &mutations[0].payload;
    assert_eq!(payload["orderType"], "STOP");
    assert_eq!(payload["stopPrice"], 95.25);
    assert!(
        payload.get("price").is_none(),
        "a stop-market order must not carry a limit price: {payload:?}"
    );
    assert_eq!(
        payload["reduceOnly"], true,
        "the gateway must receive the reduce-only flag"
    );
}

// Parity: go:11a0f579:internal/strategy/pine_live_executor_test.go:428 TestLiveCommandExecutorCancelsTrackedOrders
// Parity: go:452dea11:internal/app/apiserver/servercore/runtime_trading_test.go:443 TestStrategyRuntimeLiveCancelsTrackedOrderFromWorkerCommand
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
        virtual_account: None,
    };

    let mut cancel_intent = test_intent(0.0, 0.0);
    cancel_intent.kind = "cancel".to_owned();
    cancel_intent.id = "target-order-42".to_owned();
    cancel_intent.has_quantity = false;
    cancel_intent.has_limit_price = false;

    let res = execute_strategy_intents(ctx, &[cancel_intent]);
    assert!(res.is_err());

    let mutations = execution.mutations.lock().unwrap();
    assert!(mutations.is_empty());

    let audit = store
        .list_audit_events("inst-cancel")
        .expect("audit events");
    assert!(!audit.iter().any(|ev| ev.kind == "ORDER_CANCELLED"));
}

// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:648 TestLiveCommandExecutorCancelBoundaries
#[test]
fn cancel_boundaries_match_live_executor_contract() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strategy_path = dir.path().join("strategy.db");
    seed_strategy_test_db(&strategy_path);
    let definition_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strategy_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open definition store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&definition_store);
    store
        .seed_instance("inst-cancel-boundaries", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed boundary instance");
    store
        .seed_instance("inst-cancel-empty", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed empty instance");

    let execution_path = dir.path().join("execution.db");
    seed_execution_test_db(&execution_path);
    let execution_store =
        ExecutionOrderStore::open_existing(&execution_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open execution store");
    for order_id in ["ord-boundary-ok", "ord-boundary-failed"] {
        execution_store
            .save_order(
                active_strategy_order("inst-cancel-boundaries", order_id),
                "2026-08-30T00:00:00Z",
            )
            .expect("save active order");
    }

    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });
    let execution = FailingCancelExecutionPort::for_order("ord-boundary-failed");
    let mut blank_cancel = test_intent(0.0, 0.0);
    blank_cancel.kind = "cancel".to_owned();
    blank_cancel.id.clear();
    blank_cancel.from_entry.clear();
    blank_cancel.has_quantity = false;
    blank_cancel.has_limit_price = false;
    let error = execute_strategy_intents(
        cancel_boundary_context(
            &execution,
            &execution_store,
            &provider,
            &store,
            "inst-cancel-boundaries",
            &binding,
        ),
        &[blank_cancel],
    )
    .expect_err("blank cancel identity must fail before store lookup");
    assert!(error.contains("cancel command id is required"));
    assert!(execution.mutations.lock().unwrap().is_empty());

    let mut missing_cancel = test_intent(0.0, 0.0);
    missing_cancel.kind = "cancel".to_owned();
    missing_cancel.id = "missing".to_owned();
    missing_cancel.has_quantity = false;
    missing_cancel.has_limit_price = false;
    assert!(
        !execute_strategy_intents(
            cancel_boundary_context(
                &execution,
                &execution_store,
                &provider,
                &store,
                "inst-cancel-boundaries",
                &binding,
            ),
            &[missing_cancel],
        )
        .expect("missing tracked cancel is idempotent")
    );

    let mut empty_cancel_all = test_intent(0.0, 0.0);
    empty_cancel_all.kind = "cancel_all".to_owned();
    empty_cancel_all.has_quantity = false;
    empty_cancel_all.has_limit_price = false;
    assert!(
        !execute_strategy_intents(
            cancel_boundary_context(
                &execution,
                &execution_store,
                &provider,
                &store,
                "inst-cancel-empty",
                &binding,
            ),
            &[empty_cancel_all],
        )
        .expect("empty cancel_all is idempotent")
    );

    let mut cancel_all = test_intent(0.0, 0.0);
    cancel_all.kind = "cancel_all".to_owned();
    cancel_all.has_quantity = false;
    cancel_all.has_limit_price = false;
    let error = execute_strategy_intents(
        cancel_boundary_context(
            &execution,
            &execution_store,
            &provider,
            &store,
            "inst-cancel-boundaries",
            &binding,
        ),
        &[cancel_all],
    )
    .expect_err("cancel_all must surface a failed tracked cancellation");
    assert!(error.contains("cancel_all partially failed"));
    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(
        mutations.len(),
        2,
        "cancel_all attempts every tracked order"
    );
    drop(mutations);
    assert_eq!(
        execution_store
            .list_active_orders_for_instance("inst-cancel-boundaries")
            .expect("list active orders")
            .len(),
        2,
        "failed cancel_all retains active tracking"
    );
}

// Parity: go:11a0f579:internal/strategy/pine_live_executor_test.go:428 TestLiveCommandExecutorCancelsTrackedOrders
// Parity: go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:135 TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders
#[test]
fn targeted_cancel_only_mutates_owned_active_orders_and_removes_successful_tracking() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strat_path = dir.path().join("strategy.db");
    seed_strategy_test_db(&strat_path);
    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strat_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open definition store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-targeted-cancel", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let exec_path = dir.path().join("execution.db");
    seed_execution_test_db(&exec_path);
    let exec_store =
        ExecutionOrderStore::open_existing(&exec_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open execution store");
    let mut owned = active_strategy_order("inst-targeted-cancel", "ord-targeted-ok");
    owned.client_order_id = Some("client-targeted-ok".to_owned());
    exec_store
        .save_order(owned, "2026-08-30T00:00:00Z")
        .expect("save owned order");
    let mut failed = active_strategy_order("inst-targeted-cancel", "ord-targeted-failed");
    failed.client_order_id = Some("client-targeted-failed".to_owned());
    exec_store
        .save_order(failed, "2026-08-30T00:00:00Z")
        .expect("save failed order");
    let mut foreign = active_strategy_order("other-instance", "ord-targeted-foreign");
    foreign.client_order_id = Some("client-targeted-foreign".to_owned());
    exec_store
        .save_order(foreign, "2026-08-30T00:00:00Z")
        .expect("save foreign order");

    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });
    let targeted = |client_id: &str| {
        let mut intent = test_intent(0.0, 0.0);
        intent.kind = "cancel".to_owned();
        intent.id = client_id.to_owned();
        intent.has_quantity = false;
        intent.has_limit_price = false;
        intent
    };

    let success_execution = SuccessfulCancelExecutionPort::new(&exec_store);
    let success_ctx = StrategyExecutionContext {
        execution: Some(&success_execution),
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id: "inst-targeted-cancel",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    };
    execute_strategy_intents(success_ctx, &[targeted("client-targeted-ok")])
        .expect("owned active order should cancel");
    assert!(
        !exec_store
            .list_active_orders_for_instance("inst-targeted-cancel")
            .expect("list active after success")
            .iter()
            .any(|order| order.internal_order_id == "ord-targeted-ok"),
        "successful targeted cancellation must remove active tracking"
    );

    let failing_execution = FailingCancelExecutionPort::for_order("ord-targeted-failed");
    let failed_ctx = StrategyExecutionContext {
        execution: Some(&failing_execution),
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id: "inst-targeted-cancel",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    };
    assert!(
        execute_strategy_intents(failed_ctx, &[targeted("client-targeted-failed")]).is_err(),
        "gateway cancellation failure must be surfaced"
    );
    assert!(
        exec_store
            .list_active_orders_for_instance("inst-targeted-cancel")
            .expect("list active after failure")
            .iter()
            .any(|order| order.internal_order_id == "ord-targeted-failed"),
        "failed targeted cancellation must preserve active tracking"
    );

    let untracked_execution = MockExecutionPort::default();
    let untracked_ctx = StrategyExecutionContext {
        execution: Some(&untracked_execution),
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id: "inst-targeted-cancel",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    };
    assert!(
        execute_strategy_intents(untracked_ctx, &[targeted("client-targeted-foreign")]).is_err(),
        "foreign tracked order must be rejected"
    );
    assert!(
        untracked_execution.mutations.lock().unwrap().is_empty(),
        "foreign order must never reach the execution gateway"
    );
}

// Parity: go:11a0f579:internal/strategy/pine_live_executor_test.go:428 TestLiveCommandExecutorCancelsTrackedOrders
// Parity: go:452dea11:internal/strategy/live_command_business_boundaries_test.go:545
// TestCancelByIntentDeduplicatesAliasesAndToleratesStaleMappings.
#[test]
fn targeted_cancel_resolves_intent_aliases_once_and_tolerates_stale_mapping() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strat_path = dir.path().join("strategy.db");
    seed_strategy_test_db(&strat_path);
    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strat_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open definition store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-alias-cancel", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let exec_path = dir.path().join("execution.db");
    seed_execution_test_db(&exec_path);
    let exec_store =
        ExecutionOrderStore::open_existing(&exec_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open execution store");
    let mut aliased = active_strategy_order("inst-alias-cancel", "ord-alias");
    aliased.client_order_id =
        Some("strategy-inst-alias-cancel-US.AAPL-protect:limit-entry-1".to_owned());
    exec_store
        .save_order(aliased, "2026-08-30T00:00:00Z")
        .expect("save aliased order");
    let mut second_aliased = active_strategy_order("inst-alias-cancel", "ord-alias-stop");
    second_aliased.client_order_id =
        Some("strategy-inst-alias-cancel-US.AAPL-protect:stop-entry-2".to_owned());
    exec_store
        .save_order(second_aliased, "2026-08-30T00:00:00Z")
        .expect("save second aliased order");

    let execution = SuccessfulCancelExecutionPort::new(&exec_store);
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });
    let targeted = |id: &str| {
        let mut intent = test_intent(0.0, 0.0);
        intent.kind = "cancel".to_owned();
        intent.id = id.to_owned();
        intent.has_quantity = false;
        intent.has_limit_price = false;
        intent
    };
    let ctx = || StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id: "inst-alias-cancel",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    };

    execute_strategy_intents(ctx(), &[targeted("protect")])
        .expect("an intent alias should cancel its tracked order");
    assert_eq!(
        execution.mutations.lock().unwrap().len(),
        2,
        "one intent alias must cancel every matching tracked order exactly once"
    );
    assert!(
        exec_store
            .list_active_orders_for_instance("inst-alias-cancel")
            .expect("list active orders")
            .is_empty(),
        "successful alias cancellation must clear tracking"
    );

    execute_strategy_intents(ctx(), &[targeted("stale-intent")])
        .expect("a stale alias must be an idempotent no-op");
    assert_eq!(
        execution.mutations.lock().unwrap().len(),
        2,
        "stale aliases must not reach the broker"
    );
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
        virtual_account: None,
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

// Parity: go:452dea11:pkg/backtest/pineworker_command_executor_test.go:421 TestPineWorkerCommandExecutorCancelAll
// Parity: go:452dea11:pkg/backtest/pineworker_command_executor_test.go:626 TestPineWorkerCommandExecutorCancelBoundaries
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
    let mut terminal = active_strategy_order("inst-cancel-all", "ord-terminal-1");
    terminal.status = "FILLED".to_owned();
    terminal.client_order_id = Some("strat-terminal-1".to_owned());
    exec_store
        .save_order(terminal, "2026-08-30T00:00:00Z")
        .expect("save terminal order");

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
        virtual_account: None,
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
    assert!(
        exec_store
            .get_order("ord-terminal-1")
            .expect("read terminal order")
            .is_some_and(|order| order.status == "FILLED"),
        "cancel_all must not mutate terminal orders"
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

// Parity: go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:135 TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders
// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:443 TestLiveCommandExecutorCancelAll
#[test]
fn cancel_all_attempts_every_active_order_and_preserves_failed_tracking() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strat_path = dir.path().join("strategy.db");
    seed_strategy_test_db(&strat_path);
    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strat_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open definition store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-cancel-partial", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let exec_path = dir.path().join("execution.db");
    seed_execution_test_db(&exec_path);
    let exec_store =
        ExecutionOrderStore::open_existing(&exec_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open execution store");
    for order_id in ["ord-cancel-ok", "ord-cancel-failed"] {
        exec_store
            .save_order(
                active_strategy_order("inst-cancel-partial", order_id),
                "2026-08-30T00:00:00Z",
            )
            .expect("save active order");
    }

    let execution = FailingCancelExecutionPort::for_order("ord-cancel-failed");
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
        instance_id: "inst-cancel-partial",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    };
    let mut cancel_all = test_intent(0.0, 0.0);
    cancel_all.kind = "cancel_all".to_owned();
    cancel_all.has_quantity = false;
    cancel_all.has_limit_price = false;

    let error = execute_strategy_intents(ctx, &[cancel_all])
        .expect_err("a partial cancel_all failure must be surfaced");
    assert!(error.contains("cancel_all partially failed"));

    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 2, "every active order must be attempted");
    assert!(
        mutations
            .iter()
            .any(|input| { input.internal_order_id.as_deref() == Some("ord-cancel-ok") })
    );
    assert!(
        mutations
            .iter()
            .any(|input| { input.internal_order_id.as_deref() == Some("ord-cancel-failed") })
    );

    let audit = store
        .list_audit_events("inst-cancel-partial")
        .expect("audit events");
    assert!(audit.iter().any(|event| {
        event.kind == "ORDER_CANCELLED" && event.detail.contains("ord-cancel-ok")
    }));
    assert!(audit.iter().any(|event| {
        event.kind == "ORDER_CANCEL_FAILED" && event.detail.contains("ord-cancel-failed")
    }));
    let active = exec_store
        .list_active_orders_for_instance("inst-cancel-partial")
        .expect("active orders");
    assert_eq!(
        active.len(),
        2,
        "failed cancellation must not erase ledger tracking"
    );
}

// Parity: go:452dea11:internal/strategy/pine_live_executor_test.go:443 TestLiveCommandExecutorCancelAll
#[test]
fn cancel_all_success_removes_each_cancelled_order_from_tracking() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strat_path = dir.path().join("strategy.db");
    seed_strategy_test_db(&strat_path);
    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strat_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open definition store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-cancel-all-success", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let exec_path = dir.path().join("execution.db");
    seed_execution_test_db(&exec_path);
    let exec_store =
        ExecutionOrderStore::open_existing(&exec_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open execution store");
    for order_id in ["ord-cancel-all-1", "ord-cancel-all-2"] {
        exec_store
            .save_order(
                active_strategy_order("inst-cancel-all-success", order_id),
                "2026-08-30T00:00:00Z",
            )
            .expect("save active order");
    }

    let execution = SuccessfulCancelExecutionPort::new(&exec_store);
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
        instance_id: "inst-cancel-all-success",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: None,
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: None,
    };
    let mut cancel_all = test_intent(0.0, 0.0);
    cancel_all.kind = "cancel_all".to_owned();
    cancel_all.has_quantity = false;
    cancel_all.has_limit_price = false;

    execute_strategy_intents(ctx, &[cancel_all]).expect("cancel_all should succeed");
    let mutations = execution.mutations.lock().unwrap();
    assert_eq!(mutations.len(), 2, "every tracked order must be cancelled");
    assert!(mutations.iter().all(|input| {
        input.operation == ExecutionWriteOperation::OrderCancel && input.internal_order_id.is_some()
    }));
    drop(mutations);
    assert!(
        exec_store
            .list_active_orders_for_instance("inst-cancel-all-success")
            .expect("list active orders")
            .is_empty(),
        "successful cancel_all must clear every active tracked order"
    );
}

#[test]
fn test_execute_strategy_intents_offline_simulate_matches_order_and_updates_virtual_account() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-sim", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let provider = ActiveProviderState::default();
    let binding = json!({
        "tradingEnvironment": "SIMULATE"
    });

    let mut virtual_account = VirtualAccountState::new("sim-inst-sim", 100_000.0, 1000);

    // 1. Percentage BUY order (20% of 100,000 = 20,000 -> 200 shares at 100.0)
    let mut pct_buy = test_intent(0.0, 100.0);
    pct_buy.has_quantity = false;
    pct_buy.has_quantity_pct = true;
    pct_buy.quantity_pct = 20.0;

    let ctx = StrategyExecutionContext {
        execution: None, // Offline simulate mode
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sim",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(100.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: Some(&mut virtual_account),
    };

    let res = execute_strategy_intents(ctx, &[pct_buy]);
    assert!(
        res.is_ok(),
        "offline simulate percentage buy should succeed"
    );
    assert_eq!(virtual_account.available_cash, 80_000.0);
    let pos = virtual_account
        .get_position("US.AAPL")
        .expect("open position");
    assert_eq!(pos.quantity, 200.0);
    assert_eq!(pos.average_cost, 100.0);

    let audits = store.list_audit_events("inst-sim").expect("audit events");
    assert!(audits.iter().any(|ev| ev.kind == "ORDER_FILLED"));
    assert!(
        audits
            .iter()
            .any(|ev| ev.kind == "SIMULATE_ACCOUNT_CHECKPOINT")
    );

    // 2. Full CLOSE order at price 110.0
    let mut close_intent = test_intent(0.0, 0.0);
    close_intent.kind = "close".to_owned();
    close_intent.has_quantity = false;
    close_intent.has_limit_price = false;

    let ctx2 = StrategyExecutionContext {
        execution: None,
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-sim",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(110.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: Some(&mut virtual_account),
    };

    let res2 = execute_strategy_intents(ctx2, &[close_intent]);
    assert!(res2.is_ok(), "offline simulate close should succeed");
    assert_eq!(virtual_account.available_cash, 80_000.0 + 200.0 * 110.0);
    assert!(virtual_account.get_position("US.AAPL").is_none());
    let restored = restore_or_init_virtual_account(&store, "inst-sim", &binding).unwrap();
    assert_eq!(restored, virtual_account);
}

#[test]
fn submitted_broker_order_never_updates_virtual_cash_or_positions() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);
    let defs = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .unwrap(),
    );
    let store = StrategyRuntimeStore::from_definition_store(&defs);
    store
        .seed_instance("submitted", "RUNNING", "2026-09-08T00:00:00Z")
        .unwrap();
    let execution = MockExecutionPort::default();
    let provider = ActiveProviderState::default();
    let binding = json!({"brokerId":"futu","accountId":"42","tradingEnvironment":"SIMULATE"});
    let mut account = VirtualAccountState::new("42", 10000.0, 0);
    let initial = account.clone();
    for _ in 0..2 {
        execute_strategy_intents(
            StrategyExecutionContext {
                execution: Some(&execution),
                execution_store: None,
                provider: &provider,
                store: &store,
                instance_id: "submitted",
                market: "US",
                symbol: "US.AAPL",
                binding: &binding,
                expected_risk_revision: None,
                fallback_price: Some(200.0),
                sellable_quantity: None,
                current_position: None,
                available_cash: Some(10000.0),
                virtual_account: Some(&mut account),
            },
            &[test_intent(1.0, 100.0)],
        )
        .unwrap();
    }
    assert_eq!(account, initial);
    assert!(
        !store
            .list_audit_events("submitted")
            .unwrap()
            .iter()
            .any(|e| e.kind == "ORDER_FILLED" || e.kind == "SIMULATE_ACCOUNT_CHECKPOINT")
    );
}

#[derive(Debug, Default)]
struct UnavailableExecutionPort;

impl ExecutionWritePort for UnavailableExecutionPort {
    fn mutate(&self, _input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        Err(ExecutionWritePortError::Unavailable(
            "Futu OpenD runtime is not ready".to_owned(),
        ))
    }
}

#[test]
fn broker_unavailability_does_not_create_a_virtual_fill() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("inst-fallback", "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let execution = UnavailableExecutionPort;
    let provider = ActiveProviderState::default();
    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE"
    });

    let mut virtual_account = VirtualAccountState::new("sim-fallback", 50_000.0, 1000);

    let ctx = StrategyExecutionContext {
        execution: Some(&execution),
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id: "inst-fallback",
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(150.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: Some(&mut virtual_account),
    };

    let intents = vec![test_intent(10.0, 150.0)];
    let res = execute_strategy_intents(ctx, &intents);
    assert!(res.is_err(), "broker unavailability must remain an error");

    assert_eq!(virtual_account.available_cash, 50_000.0);
    assert!(virtual_account.get_position("US.AAPL").is_none());

    let audits = store.list_audit_events("inst-fallback").expect("audits");
    assert!(!audits.iter().any(|ev| ev.kind == "ORDER_FILLED"));
}

// Parity: go:452dea11:internal/strategy/runtimecontrol/policy_test.go:83 TestMarketDayStartUTCUsesOrderSymbolTimezone
#[test]
fn strategy_market_day_start_follows_the_order_market_timezone() {
    let instant = time::OffsetDateTime::parse(
        "2026-01-01T02:00:00Z",
        &time::format_description::well_known::Rfc3339,
    )
    .expect("instant");
    // 2025-12-31T21:00 in New York, so the current trading day started at
    // 2025-12-31T05:00Z for a US symbol.
    assert_eq!(
        strategy_market_day_start_ms("US", instant),
        1_767_157_200_000
    );
    // 2026-01-01T10:00 in Hong Kong, so the day started at 2025-12-31T16:00Z.
    assert_eq!(
        strategy_market_day_start_ms("HK", instant),
        1_767_196_800_000
    );
    // Unsupported markets keep UTC midnight instead of failing the order path.
    assert_eq!(
        strategy_market_day_start_ms("XX", instant),
        1_767_225_600_000
    );
}

// Parity: go:452dea11:internal/strategy/runtimecontrol/policy_test.go:83 TestMarketDayStartUTCUsesOrderSymbolTimezone
#[test]
fn strategy_market_day_start_follows_dst_transition() {
    let overnight = time::OffsetDateTime::parse(
        "2026-06-15T00:30:00Z",
        &time::format_description::well_known::Rfc3339,
    )
    .expect("instant");
    // 2026-06-14T20:30 in New York (EDT), so the US extended-hours trading
    // day starts at the 20:00 carry boundary on 2026-06-14: 2026-06-15T00:00Z.
    assert_eq!(
        strategy_market_day_start_ms("US", overnight),
        1_781_481_600_000
    );

    let holiday_overnight = time::OffsetDateTime::parse(
        "2026-01-02T01:30:00Z",
        &time::format_description::well_known::Rfc3339,
    )
    .expect("holiday overnight");
    // The next local date is open, so the Go session resolver still rolls the
    // holiday evening into the Jan 1 20:00 carry boundary.
    assert_eq!(
        strategy_market_day_start_ms("US", holiday_overnight),
        1_767_315_600_000
    );
}

// Parity: go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:235 TestSubmittedOrderCountKeepsInstanceScopeWithinMarketDay
#[test]
fn submitted_order_count_keeps_instance_scope_within_the_market_day() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("strategy.db");
    seed_strategy_test_db(&path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    store
        .seed_instance("multi-market-instance", "RUNNING", "2025-12-31T00:00:00Z")
        .expect("seed instance");
    for (index, at_ms) in [1_767_160_800_000_i64, 1_767_200_400_000]
        .into_iter()
        .enumerate()
    {
        store
            .append_audit_event(
                "multi-market-instance",
                "ORDER_SUBMITTED",
                &format!("US.AAPL BUY 1 (clientOrderId: client-{index})"),
                at_ms,
            )
            .expect("append order audit");
    }

    let now = time::OffsetDateTime::parse(
        "2026-01-01T02:00:00Z",
        &time::format_description::well_known::Rfc3339,
    )
    .expect("instant");
    assert_eq!(
        store
            .count_daily_orders(
                "multi-market-instance",
                strategy_market_day_start_ms("US", now)
            )
            .expect("US count"),
        2
    );
    assert_eq!(
        store
            .count_daily_orders(
                "multi-market-instance",
                strategy_market_day_start_ms("HK", now)
            )
            .expect("HK count"),
        1
    );
    assert_eq!(
        store
            .count_daily_orders("another-instance", strategy_market_day_start_ms("US", now))
            .expect("scoped count"),
        0
    );
}
