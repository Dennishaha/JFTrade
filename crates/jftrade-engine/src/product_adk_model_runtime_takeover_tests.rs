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
struct MockReplayToolExecutor {
    call_count: Arc<AtomicUsize>,
    delay: Duration,
}

impl AdkToolExecutor for MockReplayToolExecutor {
    fn supports(&self, name: &str) -> bool {
        name == "portfolio.positions"
    }

    fn execute(&self, name: &str, _arguments: &Value) -> Result<Value, String> {
        self.call_count.fetch_add(1, Ordering::SeqCst);
        if !self.delay.is_zero() {
            thread::sleep(self.delay);
        }
        if name == "portfolio.positions" {
            Ok(json!({"positions": [{"symbol": "AAPL", "quantity": 10}]}))
        } else {
            Ok(json!({"orderId": "broker-order-999", "status": "SUBMITTED"}))
        }
    }
}

#[test]
fn stale_worker_late_result_commit_never_succeeds_under_any_takeover_or_expiry_condition() {
    let (_directory, store, session_store) = initialized_stores();
    let run = create_test_run(&store, "run-stale-commit");
    let event = AdkRunEvent {
        id: "run-stale-commit:tool:call-1",
        session_id: "session-run-stale-commit",
        invocation_id: "run-stale-commit",
        author: "assistant.tool",
        content: "{}",
    };

    // Scenario 1: Tool lease expired before takeover, worker tries to commit
    let lease_1 = store
        .claim_run_lease("run-stale-commit", "worker-1", Duration::from_millis(30))
        .expect("claim lease 1");
    let claim_1 = match claim_tool(
        &store,
        "run-stale-commit",
        "call-expired-before-takeover",
        "trade.place_order",
        "{\"symbol\":\"NVDA\",\"quantity\":10}",
        &run.updated_at,
        "worker-1",
        lease_1.fencing_token,
        Duration::from_millis(30),
        true,
    )
    .expect("claim tool 1")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute, got {other:?}"),
    };
    thread::sleep(Duration::from_millis(50));
    let commit_err_1 = commit_tool(
        &store,
        "run-stale-commit",
        &run.updated_at,
        "call-expired-before-takeover",
        "trade.place_order",
        "{\"symbol\":\"NVDA\",\"quantity\":10}",
        "{\"orderId\":\"ord-1\"}",
        "worker-1",
        claim_1.fencing_token,
        lease_1.fencing_token,
        &session_store,
        &event,
    );
    assert!(
        matches!(commit_err_1, Err(AdkStoreError::LeaseLost(ref msg)) if msg.contains("expired")),
        "commit after tool lease expiry must fail with LeaseLost expired, got {commit_err_1:?}"
    );

    // Scenario 2: Tool invocation transitioned to UNKNOWN by takeover worker
    let lease_2 = store
        .claim_run_lease("run-stale-commit", "worker-2", Duration::from_millis(500))
        .expect("claim lease 2");
    let claim_unknown = claim_tool(
        &store,
        "run-stale-commit",
        "call-expired-before-takeover",
        "trade.place_order",
        "{\"symbol\":\"NVDA\",\"quantity\":10}",
        &run.updated_at,
        "worker-2",
        lease_2.fencing_token,
        Duration::from_millis(100),
        true,
    )
    .expect("takeover claim");
    assert!(matches!(claim_unknown, AdkToolInvocationClaim::Unknown(_)));

    let commit_err_2 = commit_tool(
        &store,
        "run-stale-commit",
        &run.updated_at,
        "call-expired-before-takeover",
        "trade.place_order",
        "{\"symbol\":\"NVDA\",\"quantity\":10}",
        "{\"orderId\":\"ord-1\"}",
        "worker-1",
        claim_1.fencing_token,
        lease_1.fencing_token,
        &session_store,
        &event,
    );
    assert!(
        matches!(commit_err_2, Err(AdkStoreError::LeaseLost(ref msg)) if msg.contains("unknown outcome")),
        "commit after UNKNOWN transition must fail, got {commit_err_2:?}"
    );

    // Scenario 3: Tool lease not expired locally, but RUN LEASE was stolen by worker-3
    let claim_3 = match claim_tool(
        &store,
        "run-stale-commit",
        "call-long-ttl",
        "trade.place_order",
        "{\"symbol\":\"GOOG\",\"quantity\":20}",
        &run.updated_at,
        "worker-2",
        lease_2.fencing_token,
        Duration::from_secs(10),
        true,
    )
    .expect("claim long ttl")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute, got {other:?}"),
    };
    thread::sleep(Duration::from_millis(550));
    let _lease_3 = store
        .claim_run_lease("run-stale-commit", "worker-3", Duration::from_millis(40))
        .expect("claim lease 3");

    let commit_err_3 = commit_tool(
        &store,
        "run-stale-commit",
        &run.updated_at,
        "call-long-ttl",
        "trade.place_order",
        "{\"symbol\":\"GOOG\",\"quantity\":20}",
        "{\"orderId\":\"ord-goog\"}",
        "worker-2",
        claim_3.fencing_token,
        lease_2.fencing_token,
        &session_store,
        &event,
    );
    assert!(
        matches!(commit_err_3, Err(AdkStoreError::LeaseLost(ref msg)) if msg.contains("run") && msg.contains("no longer current")),
        "commit with stolen run lease must fail with LeaseLost, got {commit_err_3:?}"
    );

    // Scenario 4: Worker attempts commit with forged fencing token
    let commit_err_4 = commit_tool(
        &store,
        "run-stale-commit",
        &run.updated_at,
        "call-long-ttl",
        "trade.place_order",
        "{\"symbol\":\"GOOG\",\"quantity\":20}",
        "{\"orderId\":\"ord-goog\"}",
        "worker-2",
        999,
        lease_2.fencing_token,
        &session_store,
        &event,
    );
    assert!(matches!(commit_err_4, Err(AdkStoreError::LeaseLost(_))));

    // Scenario 5: Replay-safe tool: Worker 2 takes over and commits SUCCEEDED.
    // Worker 1 returns late and attempts to commit its stale result.
    let run_replay = create_test_run(&store, "run-stale-replay");
    let event_replay = AdkRunEvent {
        id: "run-stale-replay:tool:call-replay-commit",
        session_id: "session-run-stale-replay",
        invocation_id: "run-stale-replay",
        author: "assistant.tool",
        content: "{}",
    };
    let replay_lease_1 = store
        .claim_run_lease("run-stale-replay", "worker-1", Duration::from_millis(30))
        .expect("claim replay lease 1");
    let replay_claim_1 = match claim_tool(
        &store,
        "run-stale-replay",
        "call-replay-commit",
        "portfolio.positions",
        "{}",
        &run_replay.updated_at,
        "worker-1",
        replay_lease_1.fencing_token,
        Duration::from_millis(30),
        false,
    )
    .expect("claim replay-safe")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute, got {other:?}"),
    };
    thread::sleep(Duration::from_millis(50));
    let replay_lease_2 = store
        .claim_run_lease("run-stale-replay", "worker-2", Duration::from_secs(5))
        .expect("claim replay lease 2");
    let takeover_replay_claim = match claim_tool(
        &store,
        "run-stale-replay",
        "call-replay-commit",
        "portfolio.positions",
        "{}",
        &run_replay.updated_at,
        "worker-2",
        replay_lease_2.fencing_token,
        Duration::from_secs(5),
        false,
    )
    .expect("takeover replay claim")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute for replay-safe takeover, got {other:?}"),
    };
    let commit_w2 = commit_tool(
        &store,
        "run-stale-replay",
        &run_replay.updated_at,
        "call-replay-commit",
        "portfolio.positions",
        "{}",
        "{\"winner\":\"worker-2\"}",
        "worker-2",
        takeover_replay_claim.fencing_token,
        replay_lease_2.fencing_token,
        &session_store,
        &event_replay,
    )
    .expect("w2 commit");
    assert!(commit_w2.changed);
    assert_eq!(
        commit_w2.invocation.output_json,
        "{\"winner\":\"worker-2\"}"
    );

    let commit_w1_late = commit_tool(
        &store,
        "run-stale-replay",
        &run_replay.updated_at,
        "call-replay-commit",
        "portfolio.positions",
        "{}",
        "{\"loser\":\"worker-1\"}",
        "worker-1",
        replay_claim_1.fencing_token,
        replay_lease_1.fencing_token,
        &session_store,
        &event_replay,
    )
    .expect("terminal replay commit returns existing");
    assert!(
        !commit_w1_late.changed,
        "Late commit must NOT overwrite terminal result"
    );
    assert_eq!(
        commit_w1_late.invocation.output_json,
        "{\"winner\":\"worker-2\"}"
    );
}

#[test]
fn replay_safe_tools_re_execute_on_takeover_and_deduplicate_subsequent_claims() {
    let (_directory, store, session_store) = initialized_stores();
    let run = create_test_run(&store, "run-replay-safe");
    assert!(replay_safe_tool("portfolio.positions"));
    assert!(!replay_safe_tool("trade.place_order"));

    let call_count = Arc::new(AtomicUsize::new(0));
    let executor = Arc::new(MockReplayToolExecutor {
        call_count: Arc::clone(&call_count),
        delay: Duration::ZERO,
    });

    let lease_1 = store
        .claim_run_lease("run-replay-safe", "worker-1", Duration::from_millis(40))
        .expect("claim lease 1");
    let claim_1 = match claim_tool(
        &store,
        "run-replay-safe",
        "call-pos-1",
        "portfolio.positions",
        "{}",
        &run.updated_at,
        "worker-1",
        lease_1.fencing_token,
        Duration::from_millis(40),
        false,
    )
    .expect("claim 1")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute, got {other:?}"),
    };
    assert_eq!(claim_1.fencing_token, 1);
    let _ = executor
        .execute("portfolio.positions", &json!({}))
        .expect("exec 1");
    assert_eq!(call_count.load(Ordering::SeqCst), 1);
    thread::sleep(Duration::from_millis(60));

    let lease_2 = store
        .claim_run_lease("run-replay-safe", "worker-2", Duration::from_secs(5))
        .expect("claim lease 2");
    assert_eq!(lease_2.fencing_token, 2);

    let claim_2 = match claim_tool(
        &store,
        "run-replay-safe",
        "call-pos-1",
        "portfolio.positions",
        "{}",
        &run.updated_at,
        "worker-2",
        lease_2.fencing_token,
        Duration::from_secs(5),
        false,
    )
    .expect("claim 2")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("replay-safe tool MUST re-execute on takeover, got {other:?}"),
    };
    assert_eq!(claim_2.owner_id, "worker-2");
    assert!(claim_2.fencing_token > claim_1.fencing_token);

    let out_2 = executor
        .execute("portfolio.positions", &json!({}))
        .expect("exec 2");
    assert_eq!(call_count.load(Ordering::SeqCst), 2);

    let event = AdkRunEvent {
        id: "run-replay-safe:tool:call-pos-1",
        session_id: "session-run-replay-safe",
        invocation_id: "run-replay-safe",
        author: "assistant.tool",
        content: "{}",
    };
    let commit_2 = commit_tool(
        &store,
        "run-replay-safe",
        &run.updated_at,
        "call-pos-1",
        "portfolio.positions",
        "{}",
        &out_2.to_string(),
        "worker-2",
        claim_2.fencing_token,
        lease_2.fencing_token,
        &session_store,
        &event,
    )
    .expect("commit 2");
    assert!(commit_2.changed);

    // Read latest run revision after commit
    let current_run = store
        .get_run("run-replay-safe")
        .expect("get run")
        .expect("run exists");
    let claim_3 = claim_tool(
        &store,
        "run-replay-safe",
        "call-pos-1",
        "portfolio.positions",
        "{}",
        &current_run.updated_at,
        "worker-2",
        lease_2.fencing_token,
        Duration::from_secs(5),
        false,
    )
    .expect("claim 3");
    match claim_3 {
        AdkToolInvocationClaim::Replay(inv) => {
            assert_eq!(inv.status, "SUCCEEDED");
            assert_eq!(inv.output_json, out_2.to_string());
        }
        other => panic!("expected Replay for succeeded invocation, got {other:?}"),
    }
    assert_eq!(call_count.load(Ordering::SeqCst), 2);

    // Race on second expired replay-safe tool
    let _claim_race = match claim_tool(
        &store,
        "run-replay-safe",
        "call-pos-race",
        "portfolio.positions",
        "{}",
        &current_run.updated_at,
        "worker-2",
        lease_2.fencing_token,
        Duration::from_millis(30),
        false,
    )
    .expect("claim race init")
    {
        AdkToolInvocationClaim::Execute(inv) => inv,
        other => panic!("expected Execute, got {other:?}"),
    };
    thread::sleep(Duration::from_millis(50));

    let mut race_handles = Vec::new();
    for _ in 0..5 {
        let store = Arc::clone(&store);
        let updated_at = current_run.updated_at.clone();
        let token = lease_2.fencing_token;
        race_handles.push(thread::spawn(move || {
            claim_tool(
                &store,
                "run-replay-safe",
                "call-pos-race",
                "portfolio.positions",
                "{}",
                &updated_at,
                "worker-2",
                token,
                Duration::from_secs(5),
                false,
            )
        }));
    }

    let mut race_executes = 0;
    let mut race_lives = 0;
    for h in race_handles {
        let res = h.join().expect("race thread").expect("race claim");
        match res {
            AdkToolInvocationClaim::Execute(_) => race_executes += 1,
            AdkToolInvocationClaim::Live(_) => race_lives += 1,
            other => panic!("unexpected race claim: {other:?}"),
        }
    }
    assert_eq!(
        race_executes, 1,
        "Exactly one thread must win the takeover Execute"
    );
    assert_eq!(race_lives, 4, "Other threads must receive Live");
}
