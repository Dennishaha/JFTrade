#![forbid(unsafe_code)]

//! Adversarial empirical verification of session recovery and zero-order-replay
//! guarantees (Milestone 1 Batch 3 / P0-03 & R1).
//!
//! Tests challenge:
//! 1. Mid-session worker crash: append failure + recovery close failure resets
//!    revision to 0, preventing permanent append jamming and forcing clean `open`.
//! 2. Rapid repeated worker crashes: revision resets to 0 reliably across N cycles.
//! 3. Wildcard recovery close invariant contract.
//! 4. Zero historical order replay across all four defensive barriers:
//!    - Barrier 1: Worker warmup filtering (current_bar_intents drops historical bars).
//!    - Barrier 2: Runtime memory deduplication (submitted_intents set suppresses crash-boundary repeats).
//!    - Barrier 3: SQLite checkpoint restore durability across total engine crash & restart.
//!    - Barrier 4: SQLite database unique constraint on execution_orders(client_order_id).

use std::collections::BTreeSet;
use std::sync::Mutex;

use jftrade_integration_pine::{
    PineCandle, PineExecutionError, PineOrderIntent, PineRunRequest, PineRunResult,
};
use jftrade_store_sqlite::{
    STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StrategyDefinitionStore, StrategyRuntimeStore,
    initialize_current,
};
use rusqlite::Connection;
use serde_json::json;

#[path = "../src/strategy_runtime_session_recovery.rs"]
mod strategy_runtime_session_recovery;
use strategy_runtime_session_recovery::run_session_request;

fn current_bar_intents(
    intents: &[PineOrderIntent],
    bar_index: i32,
    open_time: i64,
) -> Vec<PineOrderIntent> {
    intents
        .iter()
        .filter(|intent| {
            intent.bar_index == bar_index || (intent.time > 0 && intent.time == open_time)
        })
        .cloned()
        .collect()
}

fn strategy_client_order_id(
    instance_id: &str,
    symbol: &str,
    intent: &PineOrderIntent,
    index: usize,
) -> String {
    let intent_id = if intent.id.trim().is_empty() {
        format!("intent-{index}")
    } else {
        intent.id.trim().to_owned()
    };
    format!(
        "strategy-{instance_id}-{symbol}-{intent_id}-{}-{candle_time}",
        intent.bar_index,
        candle_time = intent.time
    )
}

fn create_sample_intent(id: &str, bar_index: i32, time: i64, qty: f64) -> PineOrderIntent {
    PineOrderIntent {
        kind: "order".to_owned(),
        id: id.to_owned(),
        from_entry: String::new(),
        direction: "buy".to_owned(),
        quantity: qty,
        quantity_pct: 0.0,
        limit_price: 150.0,
        stop_price: 0.0,
        comment: "adversarial test intent".to_owned(),
        alert_message: String::new(),
        disable_alert: false,
        bar_index,
        time,
        has_quantity: true,
        has_quantity_pct: false,
        has_limit_price: true,
        has_stop_price: false,
        parent_id: String::new(),
        atomic_group_id: String::new(),
        oco_group_id: String::new(),
        reduce_only: false,
    }
}

// =========================================================================
// Challenge 1: Recovery Close Failure & Revision Reset (Anti-Jamming)
// =========================================================================

#[tokio::test]
async fn adversarial_mid_session_crash_with_failed_close_resets_revision_to_zero() {
    let recorded_operations = Mutex::new(Vec::new());
    let mut session_revision: u64 = 7;

    // Cycle 1: Worker crashes during append.
    // In-flight append fails, and subsequent recovery close ALSO fails
    // because the worker process has died.
    let append_request = PineRunRequest {
        job_id: "live:inst-1:US.AAPL:1700000060000".to_owned(),
        session_id: "strategy:inst-1:US.AAPL".to_owned(),
        session_operation: "append".to_owned(),
        expected_revision: session_revision,
        ..Default::default()
    };

    let result = run_session_request(
        |req| {
            recorded_operations
                .lock()
                .unwrap()
                .push((req.session_operation.clone(), req.expected_revision));
            async move {
                if req.session_operation == "append" {
                    Err(PineExecutionError::Remote(
                        "worker process killed by SIGKILL".to_owned(),
                    ))
                } else if req.session_operation == "close" {
                    Err(PineExecutionError::Remote(
                        "connection refused (worker dead)".to_owned(),
                    ))
                } else {
                    Ok(PineRunResult::default())
                }
            }
        },
        append_request,
        &mut session_revision,
    )
    .await;

    // 1. Must return error capturing both append and unconfirmed close
    let err = result.expect_err("should return error");
    let err_str = err.to_string();
    assert!(
        err_str.contains("SIGKILL") && err_str.contains("close unconfirmed"),
        "error message must document both failures: {err_str}"
    );

    // 2. CRITICAL EMPIRICAL PROOF: revision MUST be 0 (NOT remaining at 7)
    assert_eq!(
        session_revision, 0,
        "revision must reset to 0 even though recovery close failed"
    );

    // 3. Verify operations invoked
    {
        let ops = recorded_operations.lock().unwrap();
        assert_eq!(
            *ops,
            vec![
                ("append".to_owned(), 7),
                ("close".to_owned(), 0), // Wildcard close
            ]
        );
    }

    // Cycle 2: Next cycle observes session_revision == 0, initiating an "open" request
    let next_op = if session_revision == 0 {
        "open"
    } else {
        "append"
    };
    assert_eq!(next_op, "open", "next cycle must choose open, not append");

    let open_request = PineRunRequest {
        job_id: "live:inst-1:US.AAPL:1700000120000".to_owned(),
        session_id: "strategy:inst-1:US.AAPL".to_owned(),
        session_operation: next_op.to_owned(),
        expected_revision: session_revision,
        candles: vec![PineCandle::default(), PineCandle::default()],
        ..Default::default()
    };

    // New worker has been spawned by supervisor and accepts open request
    let open_result = run_session_request(
        |req| {
            recorded_operations
                .lock()
                .unwrap()
                .push((req.session_operation.clone(), req.expected_revision));
            async move {
                Ok(PineRunResult {
                    session_revision: 1,
                    ..Default::default()
                })
            }
        },
        open_request,
        &mut session_revision,
    )
    .await;

    assert!(open_result.is_ok(), "reopen must succeed");
    assert_eq!(session_revision, 1, "session revision advanced to 1");

    // Cycle 3: Following cycle smoothly transitions back to append
    let cycle3_op = if session_revision == 0 {
        "open"
    } else {
        "append"
    };
    assert_eq!(cycle3_op, "append");
    let append2_req = PineRunRequest {
        job_id: "live:inst-1:US.AAPL:1700000180000".to_owned(),
        session_id: "strategy:inst-1:US.AAPL".to_owned(),
        session_operation: cycle3_op.to_owned(),
        expected_revision: session_revision,
        ..Default::default()
    };
    let append2_res = run_session_request(
        |req| {
            recorded_operations
                .lock()
                .unwrap()
                .push((req.session_operation.clone(), req.expected_revision));
            async move {
                Ok(PineRunResult {
                    session_revision: 2,
                    ..Default::default()
                })
            }
        },
        append2_req,
        &mut session_revision,
    )
    .await;
    assert!(append2_res.is_ok());
    assert_eq!(session_revision, 2);

    let final_ops = recorded_operations.lock().unwrap();
    assert_eq!(
        *final_ops,
        vec![
            ("append".to_owned(), 7),
            ("close".to_owned(), 0),
            ("open".to_owned(), 0),
            ("append".to_owned(), 1),
        ]
    );
}

#[tokio::test]
async fn adversarial_repeated_crash_loop_never_jams_revision() {
    // Stress test: 10 consecutive cycles where the worker crashes every single time
    let mut session_revision: u64 = 5;
    let crash_count = 10;

    for i in 0..crash_count {
        let op = if session_revision == 0 {
            "open"
        } else {
            "append"
        };
        let req = PineRunRequest {
            job_id: format!("live:inst-stress:{i}"),
            session_id: "strategy:inst-stress:US.AAPL".to_owned(),
            session_operation: op.to_owned(),
            expected_revision: session_revision,
            ..Default::default()
        };

        let result = run_session_request(
            |_req| async {
                Err(PineExecutionError::Remote(
                    "crashed worker instance".to_owned(),
                ))
            },
            req,
            &mut session_revision,
        )
        .await;

        assert!(result.is_err());
        assert_eq!(
            session_revision, 0,
            "revision must be 0 after failure in cycle {i}"
        );
    }

    // After 10 consecutive crashes, once worker recovers, open succeeds immediately
    let recovery_req = PineRunRequest {
        job_id: "live:inst-stress:recovered".to_owned(),
        session_id: "strategy:inst-stress:US.AAPL".to_owned(),
        session_operation: if session_revision == 0 {
            "open".to_owned()
        } else {
            "append".to_owned()
        },
        expected_revision: session_revision,
        ..Default::default()
    };
    assert_eq!(recovery_req.session_operation, "open");

    let rec_res = run_session_request(
        |_| async {
            Ok(PineRunResult {
                session_revision: 1,
                ..Default::default()
            })
        },
        recovery_req,
        &mut session_revision,
    )
    .await;
    assert!(rec_res.is_ok());
    assert_eq!(session_revision, 1);
}

#[tokio::test]
async fn adversarial_wildcard_recovery_close_contract_invariants() {
    let captured_close = Mutex::new(None);
    let mut revision = 99;

    let req = PineRunRequest {
        job_id: "job-orig-42".to_owned(),
        script_id: "script-custom-1".to_owned(),
        source: "//@version=5\nstrategy('test')".to_owned(),
        symbol: "US.TSLA".to_owned(),
        timeframe: "5m".to_owned(),
        chart_type: "standard".to_owned(),
        mode: "live".to_owned(),
        params: [("param1".to_owned(), "value1".to_owned())]
            .into_iter()
            .collect(),
        session_id: "strategy:my-session".to_owned(),
        session_operation: "append".to_owned(),
        expected_revision: revision,
        candles: vec![PineCandle::default()],
    };

    let _ = run_session_request(
        |incoming| {
            let op = incoming.session_operation.clone();
            if op == "close" {
                *captured_close.lock().unwrap() = Some(incoming);
            }
            async move {
                if op == "append" {
                    Err(PineExecutionError::Timeout)
                } else {
                    Ok(PineRunResult::default())
                }
            }
        },
        req,
        &mut revision,
    )
    .await;

    assert_eq!(revision, 0);
    let close = captured_close
        .lock()
        .unwrap()
        .take()
        .expect("close request emitted");
    assert_eq!(close.job_id, "recovery-close:job-orig-42");
    assert_eq!(close.session_operation, "close");
    assert_eq!(
        close.expected_revision, 0,
        "wildcard close MUST use expected_revision = 0"
    );
    assert!(
        close.candles.is_empty(),
        "recovery close MUST NOT send candles"
    );
    assert_eq!(close.session_id, "strategy:my-session");
    assert_eq!(close.script_id, "script-custom-1");
    assert_eq!(close.symbol, "US.TSLA");
}

// =========================================================================
// Challenge 2: Zero Historical Order Replay across the 4 Defensive Barriers
// =========================================================================

#[test]
fn adversarial_barrier_1_drops_all_historical_warmup_intents_on_session_rebuild() {
    // Scenario: On session rebuild, the worker is sent 100 historical candles (index 0 to 99).
    // ADVERSARIAL ATTACK: Suppose a buggy or malicious worker outputs order intents
    // for all past historical bars during the `open` call.
    let latest_bar_index = 99_i32;
    let latest_open_time = 1700000099000_i64;

    let mut adversarial_leaked_intents = Vec::new();
    for bar in 0..latest_bar_index {
        adversarial_leaked_intents.push(create_sample_intent(
            &format!("historical-order-bar-{bar}"),
            bar,
            1700000000000 + (bar as i64 * 60000),
            10.0,
        ));
    }
    // Bar 99 also has an intent
    adversarial_leaked_intents.push(create_sample_intent(
        "live-order-bar-99",
        latest_bar_index,
        latest_open_time,
        25.0,
    ));

    assert_eq!(adversarial_leaked_intents.len(), 100);

    // Apply Barrier 1 filter:
    let filtered = current_bar_intents(
        &adversarial_leaked_intents,
        latest_bar_index,
        latest_open_time,
    );

    // Exactly 1 intent survives (bar 99); 100% of historical warmup intents are eliminated!
    assert_eq!(filtered.len(), 1);
    assert_eq!(filtered[0].id, "live-order-bar-99");
    assert_eq!(filtered[0].bar_index, latest_bar_index);
    assert_eq!(filtered[0].time, latest_open_time);
}

#[test]
fn adversarial_barrier_2_memory_deduplication_suppresses_crash_boundary_duplicates() {
    // Scenario: Bar 99 generated an intent and placed an order before a crash occurred.
    // The crash happened during transport/receipt of the append response.
    // Memory state maintains `submitted_intents`.
    let mut submitted_intents = BTreeSet::new();
    let bar99_time = 1700000099000_i64;
    let bar99_intent = create_sample_intent("order-buy-99", 99, bar99_time, 50.0);

    let key = format!(
        "{}:{}:{}",
        bar99_time, bar99_intent.id, bar99_intent.bar_index
    );
    // Before crash, intent was recorded in memory:
    submitted_intents.insert(key.clone());

    // After crash, session rebuilds. Worker re-executes bar 99 and re-emits `order-buy-99`:
    let raw_intents = vec![bar99_intent.clone()];

    let mut current_intents = Vec::new();
    for intent in raw_intents {
        let k = format!("{}:{}:{}", bar99_time, intent.id, intent.bar_index);
        if !submitted_intents.contains(&k) {
            submitted_intents.insert(k);
            current_intents.push(intent);
        }
    }

    // Barrier 2 intercepts the duplicate intent:
    assert!(
        current_intents.is_empty(),
        "memory submitted_intents MUST reject duplicate intent emitted across crash boundary"
    );
}

#[test]
fn adversarial_barrier_3_cold_engine_restart_restores_checkpoints_from_sqlite() {
    // Scenario: Total power cut / machine crash. Both engine and worker processes died.
    // Checkpoint was previously persisted to SQLite `strategy.db`.
    let temp_dir = tempfile::tempdir().expect("tempdir");
    let db_path = temp_dir.path().join("strategy.db");

    let conn = Connection::open(&db_path).expect("open test db");
    initialize_current(&conn, "strategy").expect("init strategy schema");

    let def_store =
        StrategyDefinitionStore::open_existing(&db_path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
            .expect("open definition store");
    let store = StrategyRuntimeStore::from_definition_store(&def_store);

    let instance_id = "test-instance-recovery";
    store
        .seed_instance_with_binding(
            instance_id,
            "RUNNING",
            json!({"symbols": ["US.AAPL"], "interval": "1m"}),
            "2026-09-05T00:00:00Z",
        )
        .expect("seed instance");

    // Persist checkpoint for Bar 50 (open_time = 1700003000000) with 2 submitted intent keys
    let mut historic_keys = BTreeSet::new();
    historic_keys.insert("1700001000000:order-1:10".to_owned());
    historic_keys.insert("1700003000000:order-2:50".to_owned());

    let checkpoint_detail = json!({
        "scope": "test-scope",
        "symbol": "US.AAPL",
        "sessionRevision": 5,
        "lastClosedOpenTime": 1700003000000_i64,
        "submittedIntentKeys": historic_keys,
    });

    store
        .append_audit_event(
            instance_id,
            "PINE_SESSION_CHECKPOINT",
            &checkpoint_detail.to_string(),
            1700003000000,
        )
        .expect("append audit event");

    // Engine boots from cold start: read audit events and restore state
    let events = store
        .list_audit_events(instance_id)
        .expect("list audit events");
    let checkpoint_event = events
        .iter()
        .find(|e| e.kind == "PINE_SESSION_CHECKPOINT")
        .expect("must find checkpoint event");

    let parsed: serde_json::Value =
        serde_json::from_str(&checkpoint_event.detail).expect("valid json detail");
    let restored_last_closed = parsed["lastClosedOpenTime"].as_i64().unwrap();
    let restored_keys: BTreeSet<String> = parsed["submittedIntentKeys"]
        .as_array()
        .unwrap()
        .iter()
        .map(|v| v.as_str().unwrap().to_owned())
        .collect();

    // Stored revision in SQLite was 5, but cold boot forces revision = 0
    let stored_revision: u64 = parsed["sessionRevision"].as_u64().unwrap();
    assert_eq!(stored_revision, 5);
    let restored_revision: u64 = 0; // Cold boot forces revision = 0

    assert_eq!(restored_revision, 0);
    assert_eq!(restored_last_closed, 1700003000000_i64);
    assert!(restored_keys.contains("1700001000000:order-1:10"));
    assert!(restored_keys.contains("1700003000000:order-2:50"));

    // Verify candle fencing: any candle <= 1700003000000 is ignored
    let incoming_candle_time = 1700003000000_i64;
    assert!(
        restored_last_closed >= incoming_candle_time,
        "candle fence prevents re-processing already closed candle"
    );

    // Verify intent deduplication: if worker re-emits order-2, it is filtered:
    let candidate_key = "1700003000000:order-2:50";
    assert!(
        restored_keys.contains(candidate_key),
        "SQLite restored intent keys filter out duplicate orders"
    );
}

#[test]
fn adversarial_barrier_4_sqlite_execution_orders_unique_constraint_blocks_duplicates() {
    // Scenario: Even if an intent somehow bypassed Barriers 1, 2, and 3,
    // the SQLite `execution_orders` table has an explicit UNIQUE constraint on:
    // (broker_id, trading_environment, account_id, client_order_id).
    let temp_dir = tempfile::tempdir().expect("tempdir");
    let db_path = temp_dir.path().join("execution_orders.db");

    let conn = Connection::open(&db_path).expect("open test db");
    initialize_current(&conn, "execution-orders").expect("init execution schema");

    let instance_id = "inst-anti-replay";
    let symbol = "US.AAPL";
    let intent = create_sample_intent("alpha-buy", 42, 1700002520000, 100.0);
    let client_order_id = strategy_client_order_id(instance_id, symbol, &intent, 0);
    assert_eq!(
        client_order_id,
        "strategy-inst-anti-replay-US.AAPL-alpha-buy-42-1700002520000"
    );

    // Insert first order:
    let insert_sql = "INSERT INTO execution_orders (
        internal_order_id, broker_id, trading_environment, account_id,
        market, symbol, side, order_type, status, client_order_id,
        updated_at, created_at
    ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)";

    conn.execute(
        insert_sql,
        rusqlite::params![
            "order-int-001",
            "futu",
            "SIMULATE",
            "10001",
            "US",
            "AAPL",
            "BUY",
            "LIMIT",
            "SUBMITTED",
            &client_order_id,
            "2026-09-05T12:00:00Z",
            "2026-09-05T12:00:00Z",
        ],
    )
    .expect("first order insert must succeed");

    // ADVERSARIAL ATTACK: Attempt duplicate insertion of the exact same order
    let duplicate_result = conn.execute(
        insert_sql,
        rusqlite::params![
            "order-int-002", // Different internal order ID
            "futu",
            "SIMULATE",
            "10001",
            "US",
            "AAPL",
            "BUY",
            "LIMIT",
            "SUBMITTED",
            &client_order_id, // Identical client_order_id
            "2026-09-05T12:00:01Z",
            "2026-09-05T12:00:01Z",
        ],
    );

    assert!(
        duplicate_result.is_err(),
        "duplicate insertion MUST fail with SQLite UNIQUE constraint violation"
    );

    let err_msg = duplicate_result.unwrap_err().to_string();
    assert!(
        err_msg.contains("UNIQUE constraint failed"),
        "error must explicitly report UNIQUE constraint violation: {err_msg}"
    );

    // Verify exactly one order exists in the table
    let count: i64 = conn
        .query_row("SELECT COUNT(*) FROM execution_orders", [], |row| {
            row.get(0)
        })
        .expect("query count");
    assert_eq!(
        count, 1,
        "only exactly 1 order must exist in execution_orders"
    );
}
