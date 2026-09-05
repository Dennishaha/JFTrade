use std::fs::File;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::thread;
use std::time::Duration;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{
    AdkRunEvent, AdkSessionStore, AdkStore, AdkStoreError, AdkToolInvocationClaim,
    AdkToolResultCommit, CreateAdkRunParams, StoredAdkRun, initialize_current,
};

use super::{AdkToolExecutor, replay_safe_tool};

fn initialized_stores() -> (tempfile::TempDir, Arc<AdkStore>, Arc<AdkSessionStore>) {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    File::create(&adk_path).expect("create ADK database");
    File::create(&session_path).expect("create ADK session database");
    initialize_current(
        &Connection::open(&adk_path).expect("initialize ADK database"),
        "adk",
    )
    .expect("initialize ADK schema");
    initialize_current(
        &Connection::open(&session_path).expect("initialize ADK session database"),
        "adk-session",
    )
    .expect("initialize ADK session schema");
    (
        directory,
        Arc::new(AdkStore::open(&adk_path).expect("open ADK store")),
        Arc::new(AdkSessionStore::open(&session_path).expect("open session store")),
    )
}

fn create_test_run(store: &AdkStore, id: &str) -> StoredAdkRun {
    store
        .create_run(CreateAdkRunParams {
            id,
            session_id: &format!("session-{id}"),
            agent_id: &format!("agent-{id}"),
            status: "RUNNING",
            client_request_id: &format!("request-{id}"),
            request_fingerprint: &format!("fp-{id}"),
            payload_json: "{\"status\":\"RUNNING\"}",
        })
        .expect("create run")
}

#[allow(clippy::too_many_arguments)]
fn claim_tool(
    store: &AdkStore,
    run_id: &str,
    call_id: &str,
    tool: &str,
    input: &str,
    updated_at: &str,
    owner: &str,
    token: i64,
    ttl: Duration,
    fail_closed: bool,
) -> Result<AdkToolInvocationClaim, AdkStoreError> {
    store.claim_tool_invocation_if_status_and_revision(
        run_id,
        call_id,
        tool,
        input,
        "RUNNING",
        updated_at,
        owner,
        token,
        ttl,
        fail_closed,
    )
}

#[allow(clippy::too_many_arguments)]
fn commit_tool(
    store: &AdkStore,
    run_id: &str,
    updated_at: &str,
    call_id: &str,
    tool: &str,
    input: &str,
    output: &str,
    owner: &str,
    fencing_token: i64,
    run_lease_token: i64,
    session: &AdkSessionStore,
    event: &AdkRunEvent<'_>,
) -> Result<AdkToolResultCommit, AdkStoreError> {
    store.commit_tool_result_if_status_and_revision_with_event(
        run_id,
        "RUNNING",
        updated_at,
        "{\"status\":\"RUNNING\"}",
        call_id,
        tool,
        input,
        output,
        "SUCCEEDED",
        owner,
        fencing_token,
        run_lease_token,
        session,
        event,
    )
}

#[derive(Debug)]
struct MockSideEffectToolExecutor {
    call_count: Arc<AtomicUsize>,
    delay: Duration,
}

impl AdkToolExecutor for MockSideEffectToolExecutor {
    fn supports(&self, name: &str) -> bool {
        name == "trade.place_order"
    }

    fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
        self.call_count.fetch_add(1, Ordering::SeqCst);
        if !self.delay.is_zero() {
            thread::sleep(self.delay);
        }
        Ok(json!({"orderId": "broker-order-999", "status": "SUBMITTED"}))
    }
}

#[test]
fn fail_closed_lease_takeover_blocks_duplicate_tool_execution_and_stale_commit() {
    let (_directory, store, session_store) = initialized_stores();
    let run = create_test_run(&store, "run-fail-closed");
    let first_lease = store
        .claim_run_lease(
            "run-fail-closed",
            "owner-worker-1",
            Duration::from_millis(50),
        )
        .expect("claim first run lease");

    let call_count = Arc::new(AtomicUsize::new(0));
    let executor = Arc::new(MockSideEffectToolExecutor {
        call_count: Arc::clone(&call_count),
        delay: Duration::from_millis(150),
    });

    let first_claim = match claim_tool(
        &store,
        "run-fail-closed",
        "call-trade-1",
        "trade.place_order",
        "{\"symbol\":\"AAPL\",\"quantity\":10}",
        &run.updated_at,
        "owner-worker-1",
        first_lease.fencing_token,
        Duration::from_millis(50),
        true,
    )
    .expect("claim first tool invocation")
    {
        AdkToolInvocationClaim::Execute(invocation) => invocation,
        other => panic!("expected Execute for initial claim, got {other:?}"),
    };
    assert_eq!(first_claim.fencing_token, 1);

    let executor_w1 = Arc::clone(&executor);
    let worker1_handle = thread::spawn(move || {
        executor_w1.execute("trade.place_order", &json!({"symbol":"AAPL","quantity":10}))
    });

    thread::sleep(Duration::from_millis(75));

    let second_lease = store
        .claim_run_lease("run-fail-closed", "owner-worker-2", Duration::from_secs(10))
        .expect("claim second run lease");

    let takeover_claim = claim_tool(
        &store,
        "run-fail-closed",
        "call-trade-1",
        "trade.place_order",
        "{\"symbol\":\"AAPL\",\"quantity\":10}",
        &run.updated_at,
        "owner-worker-2",
        second_lease.fencing_token,
        Duration::from_millis(100),
        true,
    )
    .expect("takeover claim query");

    let AdkToolInvocationClaim::Unknown(takeover_inv) = takeover_claim else {
        panic!("expected Unknown claim for expired fail-closed tool, got {takeover_claim:?}");
    };
    assert_eq!(takeover_inv.status, "UNKNOWN");
    assert!(takeover_inv.fencing_token > first_claim.fencing_token);
    assert_eq!(call_count.load(Ordering::SeqCst), 1);

    let worker1_result = worker1_handle.join().expect("worker 1 finish");
    assert!(worker1_result.is_ok());
    assert_eq!(call_count.load(Ordering::SeqCst), 1);

    let event = AdkRunEvent {
        id: "run-fail-closed:tool:call-trade-1",
        session_id: "session-fail-closed",
        invocation_id: "run-fail-closed",
        author: "assistant.tool",
        content: "{}",
    };
    let commit_result = commit_tool(
        &store,
        "run-fail-closed",
        &run.updated_at,
        "call-trade-1",
        "trade.place_order",
        "{\"symbol\":\"AAPL\",\"quantity\":10}",
        "{\"orderId\":\"broker-order-999\",\"status\":\"SUBMITTED\"}",
        "owner-worker-1",
        first_claim.fencing_token,
        first_lease.fencing_token,
        &session_store,
        &event,
    );

    assert!(
        matches!(commit_result, Err(AdkStoreError::LeaseLost(_))),
        "stale commit must be blocked, got: {commit_result:?}"
    );

    let third_claim = claim_tool(
        &store,
        "run-fail-closed",
        "call-trade-1",
        "trade.place_order",
        "{\"symbol\":\"AAPL\",\"quantity\":10}",
        &run.updated_at,
        "owner-worker-2",
        second_lease.fencing_token,
        Duration::from_millis(100),
        true,
    )
    .expect("third claim query");
    assert!(matches!(third_claim, AdkToolInvocationClaim::Unknown(_)));
    assert_eq!(call_count.load(Ordering::SeqCst), 1);
}

#[test]
fn multiple_workers_simultaneous_takeover_after_lease_expiry_never_executes_fail_closed_tool() {
    let (_directory, store, _session_store) = initialized_stores();
    let run = create_test_run(&store, "run-multi-takeover");
    let initial_lease = store
        .claim_run_lease(
            "run-multi-takeover",
            "owner-worker-1",
            Duration::from_millis(40),
        )
        .expect("claim initial run lease");

    let initial_claim = match claim_tool(
        &store,
        "run-multi-takeover",
        "call-multi-trade",
        "trade.place_order",
        "{\"symbol\":\"MSFT\",\"quantity\":50}",
        &run.updated_at,
        "owner-worker-1",
        initial_lease.fencing_token,
        Duration::from_millis(40),
        true,
    )
    .expect("initial claim")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute for initial claim, got {other:?}"),
    };
    assert_eq!(initial_claim.fencing_token, 1);
    thread::sleep(Duration::from_millis(60));

    let num_workers = 10;
    let mut handles = Vec::new();
    let store_arc = Arc::clone(&store);
    let run_updated_at = run.updated_at.clone();

    for worker_idx in 2..=(num_workers + 1) {
        let store = Arc::clone(&store_arc);
        let updated_at = run_updated_at.clone();
        let owner = format!("owner-worker-{worker_idx}");
        let handle = thread::spawn(move || {
            let run_lease_res =
                store.claim_run_lease("run-multi-takeover", &owner, Duration::from_millis(500));
            let (run_lease_token, run_lease_success) = match run_lease_res {
                Ok(lease) => (lease.fencing_token, true),
                Err(_) => (1, false),
            };
            let claim_res = claim_tool(
                &store,
                "run-multi-takeover",
                "call-multi-trade",
                "trade.place_order",
                "{\"symbol\":\"MSFT\",\"quantity\":50}",
                &updated_at,
                &owner,
                run_lease_token,
                Duration::from_millis(100),
                true,
            );
            (owner, run_lease_success, claim_res)
        });
        handles.push(handle);
    }

    let mut execute_claims = 0;
    let mut unknown_claims = 0;
    for handle in handles {
        let (_owner, run_lease_success, claim_res) = handle.join().expect("thread join");
        match claim_res {
            Ok(AdkToolInvocationClaim::Execute(_)) => execute_claims += 1,
            Ok(AdkToolInvocationClaim::Unknown(inv)) => {
                assert!(run_lease_success);
                assert_eq!(inv.status, "UNKNOWN");
                assert_eq!(inv.lease_expires_at_unix_ms, 0);
                unknown_claims += 1;
            }
            Ok(_) => panic!("unexpected claim outcome"),
            Err(AdkStoreError::LeaseLost(_)) => {
                assert!(!run_lease_success);
            }
            Err(err) => panic!("unexpected error: {err:?}"),
        }
    }
    assert_eq!(execute_claims, 0, "No worker must EVER receive Execute");
    assert_eq!(unknown_claims, 1, "Exactly winning owner receives Unknown");
}

#[test]
fn takeover_worker_never_invokes_external_tool_for_expired_running_invocation() {
    let (_directory, store, _session_store) = initialized_stores();
    let run = create_test_run(&store, "run-never-invoke");
    let call_count = Arc::new(AtomicUsize::new(0));
    let _executor = Arc::new(MockSideEffectToolExecutor {
        call_count: Arc::clone(&call_count),
        delay: Duration::ZERO,
    });

    assert!(!replay_safe_tool("trade.place_order"));
    let fail_closed = !replay_safe_tool("trade.place_order");
    assert!(fail_closed);

    let w1_lease = store
        .claim_run_lease(
            "run-never-invoke",
            "owner-worker-1",
            Duration::from_millis(30),
        )
        .expect("claim w1 lease");
    let w1_claim = claim_tool(
        &store,
        "run-never-invoke",
        "call-trade-never",
        "trade.place_order",
        "{\"symbol\":\"TSLA\",\"quantity\":100}",
        &run.updated_at,
        "owner-worker-1",
        w1_lease.fencing_token,
        Duration::from_millis(30),
        fail_closed,
    )
    .expect("w1 claim");
    assert!(matches!(w1_claim, AdkToolInvocationClaim::Execute(_)));
    assert_eq!(call_count.load(Ordering::SeqCst), 0);
    thread::sleep(Duration::from_millis(50));

    let w2_lease = store
        .claim_run_lease(
            "run-never-invoke",
            "owner-worker-2",
            Duration::from_millis(500),
        )
        .expect("claim w2 lease");
    assert_eq!(w2_lease.fencing_token, 2);

    let w2_claim = claim_tool(
        &store,
        "run-never-invoke",
        "call-trade-never",
        "trade.place_order",
        "{\"symbol\":\"TSLA\",\"quantity\":100}",
        &run.updated_at,
        "owner-worker-2",
        w2_lease.fencing_token,
        Duration::from_millis(100),
        fail_closed,
    )
    .expect("w2 claim");

    match w2_claim {
        AdkToolInvocationClaim::Unknown(inv) => {
            assert_eq!(inv.status, "UNKNOWN");
            assert_eq!(inv.fencing_token, 2);
            assert_eq!(inv.owner_id, "");
            assert_eq!(inv.lease_expires_at_unix_ms, 0);
        }
        other => panic!("expected Unknown claim, got {other:?}"),
    }
    assert_eq!(call_count.load(Ordering::SeqCst), 0);

    for _ in 0..5 {
        let retry_claim = claim_tool(
            &store,
            "run-never-invoke",
            "call-trade-never",
            "trade.place_order",
            "{\"symbol\":\"TSLA\",\"quantity\":100}",
            &run.updated_at,
            "owner-worker-2",
            w2_lease.fencing_token,
            Duration::from_millis(100),
            fail_closed,
        )
        .expect("retry claim");
        assert!(matches!(retry_claim, AdkToolInvocationClaim::Unknown(_)));
        assert_eq!(call_count.load(Ordering::SeqCst), 0);
    }
}
