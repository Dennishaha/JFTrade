use super::*;
use crate::initialize_current;
use tempfile::tempdir;

fn fixture() -> (
    tempfile::TempDir,
    AdkStore,
    StoredAdkRunLease,
    OffsetDateTime,
) {
    let directory = tempdir().unwrap();
    let path = directory.path().join("adk.db");
    initialize_current(&Connection::open(&path).unwrap(), "adk").unwrap();
    let store = AdkStore::open(&path).unwrap();
    store
        .create_run(CreateAdkRunParams {
            id: "run-snapshot",
            session_id: "session-snapshot",
            agent_id: "agent-snapshot",
            status: "RUNNING",
            client_request_id: "request-snapshot",
            request_fingerprint: "snapshot",
            payload_json: r#"{"id":"run-snapshot","status":"RUNNING"}"#,
        })
        .unwrap();
    let lease = store
        .claim_run_lease("run-snapshot", "snapshot-owner", Duration::from_secs(5))
        .unwrap();
    let now = OffsetDateTime::from_unix_timestamp_nanos(
        i128::from(lease.heartbeat_at_unix_ms) * 1_000_000,
    )
    .unwrap();
    (directory, store, lease, now)
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:144 TestRefreshRunExecutionLeaseUsesRemainingTTLForNearExpiryLease
#[test]
fn heartbeat_near_expiry_caller_snapshot_uses_the_full_requested_ttl() {
    let (_directory, store, mut lease, now) = fixture();
    lease.expires_at_unix_ms = lease.heartbeat_at_unix_ms + 100;
    // The public heartbeat captures one now and calls this same production
    // implementation. Keep the exact 100ms input without a scheduler window.
    let refreshed = store
        .heartbeat_run_lease_at(&lease, Duration::from_secs(1), now)
        .unwrap();
    assert_eq!(
        refreshed.expires_at_unix_ms - refreshed.heartbeat_at_unix_ms,
        1_000
    );
    assert_eq!(refreshed.owner_id, lease.owner_id);
    assert_eq!(refreshed.fencing_token, lease.fencing_token);
    assert_eq!(
        store.get_run_lease(&lease.run_id).unwrap().unwrap(),
        refreshed
    );
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:133 TestRefreshRunExecutionLeaseRejectsExpiredLeaseBeforeStoreWrite
#[test]
fn heartbeat_expired_sparse_snapshot_is_rejected_before_store_access() {
    let (_directory, store, mut expired, now) = fixture();
    expired.owner_id.clear();
    expired.fencing_token = 0;
    expired.expires_at_unix_ms = expired.heartbeat_at_unix_ms - 1;
    // An inaccessible connection stands in for any attempted store access.
    // Rejection must happen even before locking it or validating owner fields.
    let poisoned = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        let _connection = store.connection.lock().unwrap();
        panic!("fixture: unavailable store lock");
    }));
    assert!(poisoned.is_err());
    assert!(matches!(
        store.heartbeat_run_lease_at(&expired, Duration::from_secs(1), now),
        Err(AdkStoreError::LeaseLost(_))
    ));
    assert!(matches!(
        store.get_run_lease(&expired.run_id),
        Err(AdkStoreError::LockUnavailable)
    ));
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:133 TestRefreshRunExecutionLeaseRejectsExpiredLeaseBeforeStoreWrite
#[test]
fn heartbeat_snapshot_at_the_expiry_boundary_preserves_the_live_durable_row() {
    let (_directory, store, lease, now) = fixture();
    let mut expired = lease.clone();
    expired.expires_at_unix_ms = expired.heartbeat_at_unix_ms;
    assert!(matches!(
        store.heartbeat_run_lease_at(&expired, Duration::from_secs(1), now),
        Err(AdkStoreError::LeaseLost(_))
    ));
    assert_eq!(store.get_run_lease(&lease.run_id).unwrap().unwrap(), lease);
}
