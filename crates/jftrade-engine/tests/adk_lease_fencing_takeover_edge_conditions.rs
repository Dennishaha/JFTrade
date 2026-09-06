#[path = "../src/product_adk_chat_stream_port.rs"]
mod product_adk_chat_stream_port;

use std::fs::File;
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use jftrade_store_sqlite::{
    AdkRunEvent, AdkSessionStore, AdkStore, AdkStoreError, AdkToolInvocationClaim,
    CreateAdkRunParams, initialize_current,
};
use product_adk_chat_stream_port::{
    ADK_CHAT_PATH, ADK_CHAT_STREAM_PATH, AdkChatPortError, AdkChatPortOutput, AdkChatRequest,
    AdkChatRoute, AdkChatStreamPort, dispatch_adk_chat,
};
use rusqlite::{Connection, params};
use serde_json::Value;
use tempfile::tempdir;
use time::OffsetDateTime;

fn initialized_stores() -> (
    tempfile::TempDir,
    Arc<AdkStore>,
    Arc<AdkSessionStore>,
    std::path::PathBuf,
) {
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
        adk_path,
    )
}

#[derive(Debug)]
struct MockAdkChatPort {
    output: Result<AdkChatPortOutput, AdkChatPortError>,
}

impl AdkChatStreamPort for MockAdkChatPort {
    fn dispatch(
        &self,
        _route: AdkChatRoute,
        _input: &product_adk_chat_stream_port::AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        self.output.clone()
    }
}

// 1. Lease expiration boundary: invocation checked 1ms before expiry vs 1ms after expiry.
#[test]
fn test_lease_expiration_boundary_before_and_after_expiry() {
    let (_dir, store, session_store, adk_path) = initialized_stores();
    let run = store
        .create_run(CreateAdkRunParams {
            id: "run-boundary",
            session_id: "session-boundary",
            agent_id: "agent-boundary",
            status: "RUNNING",
            client_request_id: "req-boundary-001",
            request_fingerprint: "fingerprint-boundary",
            payload_json: "{\"status\":\"RUNNING\"}",
        })
        .expect("create run");

    let run_lease_w1 = store
        .claim_run_lease("run-boundary", "worker-1", Duration::from_secs(60))
        .expect("claim run lease w1");

    // Initial claim by worker-1 for a fail-closed tool
    let initial_claim = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-1",
            "trade.place_order",
            "{\"symbol\":\"TSLA\",\"quantity\":5}",
            "RUNNING",
            &run.updated_at,
            "worker-1",
            run_lease_w1.fencing_token,
            Duration::from_secs(10),
            true, // fail_closed
        )
        .expect("initial claim");

    let AdkToolInvocationClaim::Execute(inv) = initial_claim else {
        panic!("expected initial claim to be Execute, got {initial_claim:?}");
    };
    assert_eq!(inv.fencing_token, 1);
    assert_eq!(inv.status, "RUNNING");

    let conn = Connection::open(&adk_path).expect("open raw sqlite");

    // Case 1A: Before expiry boundary (lease_expires > now_ms)
    // Directly set lease_expires_at_unix_ms to future time (e.g. now + 5000ms)
    let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
    conn.execute(
        "UPDATE adk_tool_invocations SET lease_expires_at_unix_ms = ?1 WHERE run_id = ?2 AND idempotency_key = ?3",
        params![now_ms + 5_000, "run-boundary", "call-boundary-1"],
    )
    .expect("set future expiry");

    // Worker 1 checks claim before expiry:
    let claim_before = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-1",
            "trade.place_order",
            "{\"symbol\":\"TSLA\",\"quantity\":5}",
            "RUNNING",
            &run.updated_at,
            "worker-1",
            run_lease_w1.fencing_token,
            Duration::from_secs(5),
            true,
        )
        .expect("claim before expiry query");

    // Should return Live claim because lease has not expired yet
    match claim_before {
        AdkToolInvocationClaim::Live(live_inv) => {
            assert_eq!(live_inv.owner_id, "worker-1");
            assert_eq!(live_inv.fencing_token, 1);
            assert_eq!(live_inv.status, "RUNNING");
        }
        other => panic!("expected Live claim before expiry, got: {other:?}"),
    }

    // Expire worker-1's run lease so worker-2 can take over the run
    let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
    conn.execute(
        "UPDATE adk_run_leases SET expires_at_unix_ms = ?1 WHERE run_id = ?2",
        params![now_ms - 1, "run-boundary"],
    )
    .expect("expire run lease w1");

    let run_lease_w2 = store
        .claim_run_lease("run-boundary", "worker-2", Duration::from_secs(60))
        .expect("claim run lease w2");

    // Case 1B: After expiry boundary (lease_expires <= now_ms)
    // Set tool invocation lease_expires_at_unix_ms to 1ms in the past (expired 1ms ago)
    let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
    conn.execute(
        "UPDATE adk_tool_invocations SET lease_expires_at_unix_ms = ?1 WHERE run_id = ?2 AND idempotency_key = ?3",
        params![now_ms - 1, "run-boundary", "call-boundary-1"],
    )
    .expect("set past expiry 1ms");

    let claim_after = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-1",
            "trade.place_order",
            "{\"symbol\":\"TSLA\",\"quantity\":5}",
            "RUNNING",
            &run.updated_at,
            "worker-2",
            run_lease_w2.fencing_token,
            Duration::from_secs(5),
            true, // fail_closed = true
        )
        .expect("claim after expiry query");

    // Should strictly transition to UNKNOWN and return Unknown claim
    match claim_after {
        AdkToolInvocationClaim::Unknown(unknown_inv) => {
            assert_eq!(unknown_inv.status, "UNKNOWN");
            assert_eq!(unknown_inv.fencing_token, 2);
            assert_eq!(unknown_inv.owner_id, "");
            assert_eq!(unknown_inv.lease_expires_at_unix_ms, 0);
        }
        other => panic!("expected Unknown claim after expiry, got: {other:?}"),
    }

    // Case 1C: Commit boundary: before expiry vs after expiry
    // Initial tool claim for commit testing by worker-2
    let claim_commit = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-commit",
            "trade.place_order",
            "{\"symbol\":\"NVDA\",\"quantity\":2}",
            "RUNNING",
            &run.updated_at,
            "worker-2",
            run_lease_w2.fencing_token,
            Duration::from_secs(10),
            true,
        )
        .expect("initial claim for commit");
    let AdkToolInvocationClaim::Execute(commit_inv) = claim_commit else {
        panic!("expected Execute for commit test");
    };

    // Subcase 1C.1: Commit before expiry succeeds
    let event = AdkRunEvent {
        id: "run-boundary:tool:call-boundary-commit",
        session_id: "session-boundary",
        invocation_id: "run-boundary",
        author: "assistant.tool",
        content: "{}",
    };
    let commit_success = store.commit_tool_result_if_status_and_revision_with_event(
        "run-boundary",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-boundary-commit",
        "trade.place_order",
        "{\"symbol\":\"NVDA\",\"quantity\":2}",
        "{\"orderId\":\"ord-nvda-1\"}",
        "SUCCEEDED",
        "worker-2",
        commit_inv.fencing_token,
        run_lease_w2.fencing_token,
        &session_store,
        &event,
    );
    assert!(
        commit_success.is_ok(),
        "commit before expiry must succeed, got: {commit_success:?}"
    );

    // Refresh run revision after successful commit
    let run_current = store
        .get_run("run-boundary")
        .expect("read run")
        .expect("run exists");

    // Subcase 1C.2: Commit 1ms after expiry strictly fails with LeaseLost
    let claim_commit_expired = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-commit-expired",
            "trade.place_order",
            "{\"symbol\":\"NVDA\",\"quantity\":3}",
            "RUNNING",
            &run_current.updated_at,
            "worker-2",
            run_lease_w2.fencing_token,
            Duration::from_secs(10),
            true,
        )
        .expect("claim for expired commit");
    let AdkToolInvocationClaim::Execute(expired_inv) = claim_commit_expired else {
        panic!("expected Execute");
    };

    let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
    conn.execute(
        "UPDATE adk_tool_invocations SET lease_expires_at_unix_ms = ?1 WHERE run_id = ?2 AND idempotency_key = ?3",
        params![now_ms - 1, "run-boundary", "call-boundary-commit-expired"],
    )
    .expect("set expiry 1ms after for commit");

    let event_expired = AdkRunEvent {
        id: "run-boundary:tool:call-boundary-commit-expired",
        session_id: "session-boundary",
        invocation_id: "run-boundary",
        author: "assistant.tool",
        content: "{}",
    };
    let commit_expired = store.commit_tool_result_if_status_and_revision_with_event(
        "run-boundary",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-boundary-commit-expired",
        "trade.place_order",
        "{\"symbol\":\"NVDA\",\"quantity\":3}",
        "{\"orderId\":\"ord-nvda-2\"}",
        "SUCCEEDED",
        "worker-2",
        expired_inv.fencing_token,
        run_lease_w2.fencing_token,
        &session_store,
        &event_expired,
    );
    match commit_expired {
        Err(AdkStoreError::LeaseLost(msg)) => {
            assert!(
                msg.contains("lease has expired"),
                "expected 'lease has expired' error, got: {msg}"
            );
        }
        other => panic!("expected LeaseLost for expired commit, got: {other:?}"),
    }

    // Case 1D: Wall-clock dynamic boundary test
    let clock_claim = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-clock",
            "trade.place_order",
            "{\"symbol\":\"MSFT\",\"quantity\":1}",
            "RUNNING",
            &run_current.updated_at,
            "worker-2",
            run_lease_w2.fencing_token,
            Duration::from_millis(20), // 20ms short TTL
            true,
        )
        .expect("clock claim");
    assert!(matches!(clock_claim, AdkToolInvocationClaim::Execute(_)));

    // Immediately check: must be Live
    let live_clock = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-clock",
            "trade.place_order",
            "{\"symbol\":\"MSFT\",\"quantity\":1}",
            "RUNNING",
            &run_current.updated_at,
            "worker-2",
            run_lease_w2.fencing_token,
            Duration::from_secs(5),
            true,
        )
        .expect("immediate clock check");
    assert!(matches!(live_clock, AdkToolInvocationClaim::Live(_)));

    // Sleep past the 20ms expiry
    thread::sleep(Duration::from_millis(30));

    // Now check: must be Unknown
    let unknown_clock = store
        .claim_tool_invocation_if_status_and_revision(
            "run-boundary",
            "call-boundary-clock",
            "trade.place_order",
            "{\"symbol\":\"MSFT\",\"quantity\":1}",
            "RUNNING",
            &run_current.updated_at,
            "worker-2",
            run_lease_w2.fencing_token,
            Duration::from_secs(5),
            true,
        )
        .expect("expired clock check");
    assert!(matches!(unknown_clock, AdkToolInvocationClaim::Unknown(_)));
}

// 2. Fencing token monotonically increasing across takeovers.
#[test]
fn test_fencing_token_monotonically_increases_across_takeovers() {
    let (_dir, store, _session_store, adk_path) = initialized_stores();
    let run = store
        .create_run(CreateAdkRunParams {
            id: "run-monotonic",
            session_id: "session-monotonic",
            agent_id: "agent-monotonic",
            status: "RUNNING",
            client_request_id: "req-monotonic-001",
            request_fingerprint: "fingerprint-monotonic",
            payload_json: "{\"status\":\"RUNNING\"}",
        })
        .expect("create run");

    let conn = Connection::open(&adk_path).expect("open raw sqlite");
    let mut observed_tokens = Vec::new();

    // Initial claim: Worker 1 claims tool with fail_closed = false (replay-safe)
    let lease_1 = store
        .claim_run_lease("run-monotonic", "worker-1", Duration::from_secs(60))
        .expect("claim run lease 1");

    let claim_1 = store
        .claim_tool_invocation_if_status_and_revision(
            "run-monotonic",
            "call-monotonic-1",
            "query.market_data",
            "{\"symbol\":\"SPY\"}",
            "RUNNING",
            &run.updated_at,
            "worker-1",
            lease_1.fencing_token,
            Duration::from_millis(10),
            false, // replay-safe allows repeated execute takeover
        )
        .expect("claim 1");

    let AdkToolInvocationClaim::Execute(inv_1) = claim_1 else {
        panic!("expected Execute for claim 1");
    };
    observed_tokens.push(inv_1.fencing_token);

    // 4 successive takeovers (takeovers 2, 3, 4 with replay-safe, takeover 5 with fail_closed)
    for i in 2..=5 {
        let worker_name = format!("worker-{i}");

        // Force run lease expiration so new worker can take over run lease
        let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;
        conn.execute(
            "UPDATE adk_run_leases SET expires_at_unix_ms = ?1 WHERE run_id = ?2",
            params![now_ms - 1, "run-monotonic"],
        )
        .expect("force expire run lease");

        let lease = store
            .claim_run_lease("run-monotonic", &worker_name, Duration::from_secs(60))
            .expect("claim run lease");

        // Force tool invocation lease expiration in DB
        conn.execute(
            "UPDATE adk_tool_invocations SET lease_expires_at_unix_ms = ?1 WHERE run_id = ?2 AND idempotency_key = ?3",
            params![now_ms - 1, "run-monotonic", "call-monotonic-1"],
        )
        .expect("force expire invocation");

        let fail_closed = i == 5; // On the 5th takeover, test fail-closed transition
        let claim = store
            .claim_tool_invocation_if_status_and_revision(
                "run-monotonic",
                "call-monotonic-1",
                "query.market_data",
                "{\"symbol\":\"SPY\"}",
                "RUNNING",
                &run.updated_at,
                &worker_name,
                lease.fencing_token,
                Duration::from_millis(10),
                fail_closed,
            )
            .expect("takeover claim");

        match claim {
            AdkToolInvocationClaim::Execute(inv) => {
                assert!(!fail_closed);
                observed_tokens.push(inv.fencing_token);
            }
            AdkToolInvocationClaim::Unknown(inv) => {
                assert!(fail_closed);
                observed_tokens.push(inv.fencing_token);
            }
            other => panic!("unexpected claim variant on takeover {i}: {other:?}"),
        }
    }

    // Verify token values: [1, 2, 3, 4, 5]
    assert_eq!(observed_tokens, vec![1, 2, 3, 4, 5]);

    // Verify strict monotonicity: token[i] > token[i-1]
    for i in 1..observed_tokens.len() {
        assert!(
            observed_tokens[i] > observed_tokens[i - 1],
            "fencing token must be strictly monotonic: {} <= {}",
            observed_tokens[i],
            observed_tokens[i - 1]
        );
        assert_eq!(observed_tokens[i], observed_tokens[i - 1] + 1);
    }

    // Verify database row holds fencing_token = 5 and status = UNKNOWN
    let (db_token, db_status): (i64, String) = conn
        .query_row(
            "SELECT fencing_token, status FROM adk_tool_invocations WHERE run_id = ?1 AND idempotency_key = ?2",
            params!["run-monotonic", "call-monotonic-1"],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .expect("query invocation");
    assert_eq!(db_token, 5);
    assert_eq!(db_status, "UNKNOWN");
}

// 3. Verify that commit_tool_result_if_status_and_revision_with_event strictly returns LeaseLost
//    if status is UNKNOWN, lease expired, or token mismatched.
#[test]
fn test_commit_tool_result_strictly_returns_lease_lost_on_unknown_expired_or_mismatched_token() {
    let (_dir, store, session_store, adk_path) = initialized_stores();
    let run = store
        .create_run(CreateAdkRunParams {
            id: "run-commit-tests",
            session_id: "session-commit-tests",
            agent_id: "agent-commit-tests",
            status: "RUNNING",
            client_request_id: "req-commit-tests-001",
            request_fingerprint: "fingerprint-commit-tests",
            payload_json: "{\"status\":\"RUNNING\"}",
        })
        .expect("create run");

    let run_lease = store
        .claim_run_lease("run-commit-tests", "worker-main", Duration::from_secs(60))
        .expect("claim run lease");

    let conn = Connection::open(&adk_path).expect("open raw sqlite");
    let now_ms = (OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000) as i64;

    // Condition 3A: Status is UNKNOWN -> strictly returns LeaseLost("fenced with unknown outcome")
    conn.execute(
        "INSERT INTO adk_tool_invocations
         (run_id, idempotency_key, tool_name, status, owner_id, fencing_token, run_lease_token,
          input_json, output_json, lease_expires_at_unix_ms, created_at, updated_at)
         VALUES (?1, ?2, ?3, 'UNKNOWN', ?4, 1, ?5, ?6, 'null', ?7, ?8, ?8)",
        params![
            "run-commit-tests",
            "call-unknown",
            "trade.place_order",
            "worker-main",
            run_lease.fencing_token,
            "{\"symbol\":\"AMD\"}",
            now_ms + 10_000,
            "2026-09-05T09:00:00Z"
        ],
    )
    .expect("insert unknown invocation");

    let event_unknown = AdkRunEvent {
        id: "run-commit-tests:tool:call-unknown",
        session_id: "session-commit-tests",
        invocation_id: "run-commit-tests",
        author: "assistant.tool",
        content: "{}",
    };
    let err_unknown = store.commit_tool_result_if_status_and_revision_with_event(
        "run-commit-tests",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-unknown",
        "trade.place_order",
        "{\"symbol\":\"AMD\"}",
        "{\"status\":\"SUBMITTED\"}",
        "SUCCEEDED",
        "worker-main",
        1,
        run_lease.fencing_token,
        &session_store,
        &event_unknown,
    );
    match err_unknown {
        Err(AdkStoreError::LeaseLost(msg)) => {
            assert!(
                msg.contains("fenced with unknown outcome"),
                "expected 'fenced with unknown outcome', got: {msg}"
            );
        }
        other => panic!("expected LeaseLost for UNKNOWN invocation, got: {other:?}"),
    }

    // Condition 3B: Lease expired -> strictly returns LeaseLost("lease has expired")
    conn.execute(
        "INSERT INTO adk_tool_invocations
         (run_id, idempotency_key, tool_name, status, owner_id, fencing_token, run_lease_token,
          input_json, output_json, lease_expires_at_unix_ms, created_at, updated_at)
         VALUES (?1, ?2, ?3, 'RUNNING', ?4, 1, ?5, ?6, 'null', ?7, ?8, ?8)",
        params![
            "run-commit-tests",
            "call-expired",
            "trade.place_order",
            "worker-main",
            run_lease.fencing_token,
            "{\"symbol\":\"INTC\"}",
            now_ms - 10, // expired 10ms ago
            "2026-09-05T09:00:00Z"
        ],
    )
    .expect("insert expired invocation");

    let event_expired = AdkRunEvent {
        id: "run-commit-tests:tool:call-expired",
        session_id: "session-commit-tests",
        invocation_id: "run-commit-tests",
        author: "assistant.tool",
        content: "{}",
    };
    let err_expired = store.commit_tool_result_if_status_and_revision_with_event(
        "run-commit-tests",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-expired",
        "trade.place_order",
        "{\"symbol\":\"INTC\"}",
        "{\"status\":\"SUBMITTED\"}",
        "SUCCEEDED",
        "worker-main",
        1,
        run_lease.fencing_token,
        &session_store,
        &event_expired,
    );
    match err_expired {
        Err(AdkStoreError::LeaseLost(msg)) => {
            assert!(
                msg.contains("lease has expired"),
                "expected 'lease has expired', got: {msg}"
            );
        }
        other => panic!("expected LeaseLost for expired invocation, got: {other:?}"),
    }

    // Condition 3C: Fencing token mismatch -> strictly returns LeaseLost("fencing token is no longer current")
    conn.execute(
        "INSERT INTO adk_tool_invocations
         (run_id, idempotency_key, tool_name, status, owner_id, fencing_token, run_lease_token,
          input_json, output_json, lease_expires_at_unix_ms, created_at, updated_at)
         VALUES (?1, ?2, ?3, 'RUNNING', ?4, 5, ?5, ?6, 'null', ?7, ?8, ?8)",
        params![
            "run-commit-tests",
            "call-token-mismatch",
            "trade.place_order",
            "worker-main",
            run_lease.fencing_token,
            "{\"symbol\":\"GOOGL\"}",
            now_ms + 10_000,
            "2026-09-05T09:00:00Z"
        ],
    )
    .expect("insert token mismatch invocation");

    let event_mismatch = AdkRunEvent {
        id: "run-commit-tests:tool:call-token-mismatch",
        session_id: "session-commit-tests",
        invocation_id: "run-commit-tests",
        author: "assistant.tool",
        content: "{}",
    };
    // Passing stale token 4 instead of current token 5
    let err_mismatch = store.commit_tool_result_if_status_and_revision_with_event(
        "run-commit-tests",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-token-mismatch",
        "trade.place_order",
        "{\"symbol\":\"GOOGL\"}",
        "{\"status\":\"SUBMITTED\"}",
        "SUCCEEDED",
        "worker-main",
        4, // mismatched token
        run_lease.fencing_token,
        &session_store,
        &event_mismatch,
    );
    match err_mismatch {
        Err(AdkStoreError::LeaseLost(msg)) => {
            assert!(
                msg.contains("fencing token is no longer current"),
                "expected 'fencing token is no longer current', got: {msg}"
            );
        }
        other => panic!("expected LeaseLost for token mismatch, got: {other:?}"),
    }

    // Condition 3D: Owner mismatch -> strictly returns LeaseLost
    let err_owner = store.commit_tool_result_if_status_and_revision_with_event(
        "run-commit-tests",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-token-mismatch",
        "trade.place_order",
        "{\"symbol\":\"GOOGL\"}",
        "{\"status\":\"SUBMITTED\"}",
        "SUCCEEDED",
        "imposter-worker", // mismatched owner
        5,
        run_lease.fencing_token,
        &session_store,
        &event_mismatch,
    );
    match err_owner {
        Err(AdkStoreError::LeaseLost(msg)) => {
            assert!(
                msg.contains("fencing token is no longer current"),
                "expected 'fencing token is no longer current', got: {msg}"
            );
        }
        other => panic!("expected LeaseLost for owner mismatch, got: {other:?}"),
    }

    // Condition 3E: Run lease token mismatch -> strictly returns LeaseLost
    let err_run_lease = store.commit_tool_result_if_status_and_revision_with_event(
        "run-commit-tests",
        "RUNNING",
        &run.updated_at,
        "{\"status\":\"RUNNING\"}",
        "call-token-mismatch",
        "trade.place_order",
        "{\"symbol\":\"GOOGL\"}",
        "{\"status\":\"SUBMITTED\"}",
        "SUCCEEDED",
        "worker-main",
        5,
        run_lease.fencing_token + 100, // mismatched run lease token
        &session_store,
        &event_mismatch,
    );
    match err_run_lease {
        Err(AdkStoreError::LeaseLost(msg)) => {
            assert!(
                msg.contains("belongs to run lease token"),
                "expected 'belongs to run lease token', got: {msg}"
            );
        }
        other => panic!("expected LeaseLost for run lease token mismatch, got: {other:?}"),
    }
}

// 4. Verify that the engine maps this condition cleanly to HTTP 500 ADK_TOOL_OUTCOME_UNKNOWN without panicking or crashing.
#[test]
fn test_engine_cleanly_maps_adk_tool_outcome_unknown_to_http_500_without_panic() {
    // 4A: Chat route wire mapping
    let error = AdkChatPortError::Failed {
        status: 500,
        code: "ADK_TOOL_OUTCOME_UNKNOWN".to_owned(),
        message: "tool invocation call-123 expired while in flight; outcome unknown".to_owned(),
    };
    let port = MockAdkChatPort { output: Err(error) };

    let chat_request = AdkChatRequest {
        method: "POST".to_owned(),
        path: ADK_CHAT_PATH.to_owned(),
        body: br#"{"clientRequestId":"12345678-1234-4123-8123-123456789abc","message":"buy AAPL"}"#
            .to_vec(),
    };

    let response = dispatch_adk_chat(&chat_request, Some(&port), "2026-09-05T09:00:00Z", 420_000);

    assert_eq!(response.status(), 500);
    assert_eq!(
        response.headers()["Content-Type"],
        "application/json; charset=utf-8"
    );

    let body_val: Value = serde_json::from_str(&response.body()).expect("valid JSON response body");
    assert_eq!(body_val["ok"], false);
    assert_eq!(body_val["error"]["code"], "ADK_TOOL_OUTCOME_UNKNOWN");
    assert_eq!(
        body_val["error"]["message"],
        "tool invocation call-123 expired while in flight; outcome unknown"
    );

    // 4B: Stream route wire mapping
    let stream_request = AdkChatRequest {
        method: "POST".to_owned(),
        path: ADK_CHAT_STREAM_PATH.to_owned(),
        body: br#"{"clientRequestId":"87654321-4321-4321-8321-cba987654321","message":"stream trade"}"#.to_vec(),
    };

    let stream_response = dispatch_adk_chat(
        &stream_request,
        Some(&port),
        "2026-09-05T09:00:00Z",
        420_000,
    );

    assert_eq!(stream_response.status(), 500);
    assert_eq!(
        stream_response.headers()["Content-Type"],
        "application/json; charset=utf-8"
    );
    assert_eq!(
        stream_response.headers()["X-ADK-Stream-Idle-Timeout-Ms"],
        "420000"
    );

    let stream_body_val: Value =
        serde_json::from_str(&stream_response.body()).expect("valid stream error JSON body");
    assert_eq!(stream_body_val["ok"], false);
    assert_eq!(stream_body_val["error"]["code"], "ADK_TOOL_OUTCOME_UNKNOWN");
    assert_eq!(
        stream_body_val["error"]["message"],
        "tool invocation call-123 expired while in flight; outcome unknown"
    );
}
