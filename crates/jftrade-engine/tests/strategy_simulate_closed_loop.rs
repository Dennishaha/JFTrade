#![forbid(unsafe_code)]

//! Integration testing for Milestone 5 (R4: SIMULATE 模拟环境账户与持仓状态模型建立).
//!
//! Verifies:
//! 1. End-to-end strategy execution in SIMULATE mode without Futu OpenD.
//! 2. Accurate percentage buy order execution, position opening, and cash deduction.
//! 3. Subsequent strategy close order execution, position clearing, and cash crediting.
//! 4. Recovery of virtual cash and positions from SQLite SIMULATE_ACCOUNT_CHECKPOINT across restarts.
//! 5. Fallback from unavailable broker execution port to offline simulated fill.
//! 6. Enforcement of virtual cash and position limits.

mod product {
    pub use jftrade_engine::product::*;
}

#[allow(dead_code)]
#[path = "../src/strategy_runtime_simulate.rs"]
mod strategy_runtime_simulate;

#[allow(dead_code)]
#[path = "../src/strategy_runtime_execution.rs"]
mod strategy_runtime_execution;

use std::sync::Arc;

use product::product_active_provider_state::ActiveProviderState;
use product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWritePort, ExecutionWritePortError,
};
use rusqlite::Connection;
use serde_json::{Value, json};

use jftrade_integration_pine::PineOrderIntent;
use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderStore,
    STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
};
use jftrade_trading::{SimulateExecutionError, VirtualAccountState};
use strategy_runtime_execution::{StrategyExecutionContext, execute_strategy_intents};
use strategy_runtime_simulate::restore_or_init_virtual_account;

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

fn test_intent(
    id: &str,
    kind: &str,
    direction: &str,
    qty: f64,
    limit_price: f64,
    bar_index: i32,
) -> PineOrderIntent {
    PineOrderIntent {
        kind: kind.to_owned(),
        id: id.to_owned(),
        from_entry: String::new(),
        direction: direction.to_owned(),
        quantity: qty,
        quantity_pct: 0.0,
        limit_price,
        stop_price: 0.0,
        comment: String::new(),
        alert_message: String::new(),
        disable_alert: false,
        bar_index,
        time: 1700000000 + (bar_index as i64) * 60,
        has_quantity: qty > 0.0,
        has_quantity_pct: false,
        has_limit_price: limit_price > 0.0,
        has_stop_price: false,
        parent_id: String::new(),
        atomic_group_id: String::new(),
        oco_group_id: String::new(),
        reduce_only: false,
    }
}

// =========================================================================
// Test 1: Full Closed-Loop: Percentage Buy, Position Opening, Close, Restart
// =========================================================================

#[test]
fn test_strategy_simulate_closed_loop_buy_close_and_checkpoint_recovery() {
    let dir = tempfile::tempdir().expect("tempdir");
    let strat_path = dir.path().join("strategy.db");
    let exec_path = dir.path().join("execution.db");
    seed_strategy_test_db(&strat_path);
    seed_execution_test_db(&exec_path);

    let def_store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &strat_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
        )
        .expect("open def store"),
    );
    let store = StrategyRuntimeStore::from_definition_store(&def_store);
    let exec_store =
        ExecutionOrderStore::open_existing(&exec_path, EXECUTION_ORDERS_TEST_CUTOVER_PROFILE)
            .expect("open execution store");

    let instance_id = "inst-closed-loop-1";
    store
        .seed_instance(instance_id, "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let binding = json!({
        "tradingEnvironment": "SIMULATE",
        "accountId": "sim-user-1",
        "initialCash": 100_000.0,
        "symbols": ["US.AAPL"],
        "interval": "1m"
    });

    let provider = ActiveProviderState::default();

    // 1. Initial Cold Start: Restore or initialize VirtualAccountState
    let mut virtual_account = restore_or_init_virtual_account(&store, instance_id, &binding)
        .expect("initialize virtual account");
    assert_eq!(virtual_account.account_id, "sim-user-1");
    assert_eq!(virtual_account.available_cash, 100_000.0);
    assert_eq!(virtual_account.initial_cash, 100_000.0);
    assert!(virtual_account.positions.is_empty());

    // 2. Bar 1: Pine strategy signals a percentage BUY order (30% at $150.0)
    // 30% of 100,000 = $30,000 notional / 150 = 200 shares.
    let mut pct_buy = test_intent("entry-1", "order", "buy", 0.0, 150.0, 1);
    pct_buy.has_quantity = false;
    pct_buy.has_quantity_pct = true;
    pct_buy.quantity_pct = 30.0;

    let ctx1 = StrategyExecutionContext {
        execution: None, // OFFLINE simulation without OpenD
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id,
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

    let placed1 = execute_strategy_intents(ctx1, &[pct_buy]).expect("execute percentage buy");
    assert!(placed1, "order must be placed and filled");

    // Verify virtual cash deduction and position update
    assert_eq!(virtual_account.available_cash, 70_000.0);
    let pos = virtual_account
        .get_position("US.AAPL")
        .expect("AAPL position exists");
    assert_eq!(pos.quantity, 200.0);
    assert_eq!(pos.sellable_quantity, 200.0);
    assert_eq!(pos.average_cost, 150.0);
    assert_eq!(pos.realized_pnl, 0.0);

    // Verify SQLite audit event checkpoints
    let audits1 = store.list_audit_events(instance_id).expect("audit events");
    assert!(
        audits1
            .iter()
            .any(|ev| ev.kind == "ORDER_FILLED" && ev.detail.contains("200"))
    );
    assert!(
        audits1
            .iter()
            .any(|ev| ev.kind == "SIMULATE_ACCOUNT_CHECKPOINT")
    );

    // Verify execution order store record
    let orders1 = exec_store.list_orders().expect("list execution orders");
    assert_eq!(orders1.len(), 1);
    assert_eq!(orders1[0].status, "FILLED");
    assert_eq!(orders1[0].filled_quantity, Some(200.0));
    assert_eq!(orders1[0].filled_average_price, Some(150.0));

    // 3. Bar 2: Pine strategy signals a subsequent CLOSE order at $180.0
    // Strategy close order has no explicit quantity: it closes the open position (200 shares).
    // Gross proceeds: 200 * 180 = $36,000. Realized PnL: (180 - 150) * 200 = +$6,000.
    // Resulting cash: 70,000 + 36,000 = $106,000.
    let close_intent = test_intent("close-1", "close", "long", 0.0, 0.0, 2);

    let ctx2 = StrategyExecutionContext {
        execution: None,
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id,
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(180.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: Some(&mut virtual_account),
    };

    let placed2 = execute_strategy_intents(ctx2, &[close_intent]).expect("execute strategy close");
    assert!(placed2, "close order must be executed");

    // Verify position cleared and cash credited
    assert_eq!(virtual_account.available_cash, 106_000.0);
    assert!(virtual_account.get_position("US.AAPL").is_none());

    // 4. Bar 3: Simulate Engine Restart across crashes/reboots
    // Drop the in-memory state and restore directly from SQLite audit checkpoints
    drop(virtual_account);

    let mut restored_account = restore_or_init_virtual_account(&store, instance_id, &binding)
        .expect("restore virtual account across restart");
    assert_eq!(restored_account.available_cash, 106_000.0);
    assert!(restored_account.positions.is_empty());

    // 5. Bar 4 on Restored Instance: Buy 100 shares at $200.0, verify persistence
    let buy2 = test_intent("entry-2", "order", "buy", 100.0, 200.0, 3);
    let ctx3 = StrategyExecutionContext {
        execution: None,
        execution_store: Some(&exec_store),
        provider: &provider,
        store: &store,
        instance_id,
        market: "US",
        symbol: "US.AAPL",
        binding: &binding,
        expected_risk_revision: None,
        fallback_price: Some(200.0),
        sellable_quantity: None,
        current_position: None,
        available_cash: None,
        virtual_account: Some(&mut restored_account),
    };

    let placed3 = execute_strategy_intents(ctx3, &[buy2]).expect("execute second buy");
    assert!(placed3);
    assert_eq!(restored_account.available_cash, 86_000.0);
    assert_eq!(
        restored_account.get_position("US.AAPL").unwrap().quantity,
        100.0
    );

    // 6. Second Restart Test: Restore with active open position
    drop(restored_account);

    let restored_with_pos = restore_or_init_virtual_account(&store, instance_id, &binding)
        .expect("restore virtual account with open position");
    assert_eq!(restored_with_pos.available_cash, 86_000.0);
    let restored_pos = restored_with_pos
        .get_position("US.AAPL")
        .expect("restored open position");
    assert_eq!(restored_pos.quantity, 100.0);
    assert_eq!(restored_pos.average_cost, 200.0);
    assert_eq!(restored_pos.sellable_quantity, 100.0);
}

// =========================================================================
// Test 2: Fallback from Unavailable Live Broker to Offline Simulation
// =========================================================================

#[derive(Debug, Default)]
struct BrokerUnavailablePort;

impl ExecutionWritePort for BrokerUnavailablePort {
    fn mutate(&self, _input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        Err(ExecutionWritePortError::Unavailable(
            "Futu OpenD runtime is not ready".to_owned(),
        ))
    }
}

#[test]
fn broker_simulation_does_not_fall_back_to_local_matching() {
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
    let instance_id = "inst-fallback-test";
    store
        .seed_instance(instance_id, "RUNNING", "2026-08-30T00:00:00Z")
        .expect("seed instance");

    let binding = json!({
        "brokerId": "futu",
        "accountId": "12345",
        "tradingEnvironment": "SIMULATE",
        "initialCash": 50_000.0,
    });

    let provider = ActiveProviderState::default();
    let unavailable_exec = BrokerUnavailablePort;

    let mut virtual_account = VirtualAccountState::new("sim-fallback", 50_000.0, 1000);

    let buy_intent = test_intent("order-fallback", "order", "buy", 50.0, 100.0, 1);

    let ctx = StrategyExecutionContext {
        execution: Some(&unavailable_exec), // Port returns Unavailable ("Futu OpenD runtime is not ready")
        execution_store: None,
        provider: &provider,
        store: &store,
        instance_id,
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

    let result = execute_strategy_intents(ctx, &[buy_intent]);
    assert!(result.is_err(), "broker errors must not synthesize fills");

    assert_eq!(virtual_account.available_cash, 50_000.0);
    assert!(virtual_account.get_position("US.AAPL").is_none());

    let audits = store.list_audit_events(instance_id).expect("list audits");
    assert!(!audits.iter().any(|ev| ev.kind == "ORDER_FILLED"));
    assert!(
        !audits
            .iter()
            .any(|ev| ev.kind == "SIMULATE_ACCOUNT_CHECKPOINT")
    );
}

// =========================================================================
// Test 3: Cash & Position Guard Enforcement in SIMULATE mode
// =========================================================================

#[test]
fn test_strategy_simulate_cash_and_position_limits() {
    let mut account = VirtualAccountState::new("sim-limits", 1_000.0, 1000);

    // Attempt buy exceeding available cash ($100 * 20 = $2,000 > $1,000)
    let buy_err = account.apply_fill("US", "AAPL", "BUY", 20.0, 100.0, 0.0, 2000);
    assert!(matches!(
        buy_err,
        Err(SimulateExecutionError::InsufficientCash { available, required })
            if available == 1_000.0 && required == 2_000.0
    ));
    assert_eq!(account.available_cash, 1_000.0);
    assert!(account.positions.is_empty());

    // Successful buy within cash
    account
        .apply_fill("US", "AAPL", "BUY", 5.0, 100.0, 0.0, 2000)
        .unwrap();
    assert_eq!(account.available_cash, 500.0);

    // Attempt sell exceeding position (sell 10 shares when holding 5)
    let sell_err = account.apply_fill("US", "AAPL", "SELL", 10.0, 120.0, 0.0, 3000);
    assert!(matches!(
        sell_err,
        Err(SimulateExecutionError::InsufficientPosition { sellable, required })
            if sellable == 5.0 && required == 10.0
    ));
}
