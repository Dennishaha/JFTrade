use super::*;
use jftrade_store_sqlite::AdkStoreError;

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:133 TestRefreshRunExecutionLeaseRejectsExpiredLeaseBeforeStoreWrite
#[test]
fn production_heartbeat_rejects_expired_caller_snapshot_before_attempting_a_write() {
    let (directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-expired-snapshot", json!([]));
    let durable = store
        .claim_run_lease("run-expired-snapshot", "snapshot-owner", RUN_LEASE_TTL)
        .unwrap();
    let run_before = store.get_run(&durable.run_id).unwrap().unwrap();
    let mut expired = durable.clone();
    expired.expires_at_unix_ms = 0;
    Connection::open(directory.path().join("adk.db"))
        .unwrap()
        .execute_batch(
            "CREATE TRIGGER reject_heartbeat_write BEFORE UPDATE ON adk_run_leases
             BEGIN SELECT RAISE(FAIL, 'expired snapshot reached store write'); END;",
        )
        .unwrap();
    assert!(matches!(
        store.heartbeat_run_lease(&expired, Duration::from_secs(1)),
        Err(AdkStoreError::LeaseLost(_))
    ));
    assert_eq!(
        store.get_run_lease(&durable.run_id).unwrap().unwrap(),
        durable
    );
    assert_eq!(store.get_run(&durable.run_id).unwrap().unwrap(), run_before);
    // A valid snapshot reaches this same write fault: rejection above is a
    // caller-snapshot check, not a swallowed database error or durable expiry.
    assert!(matches!(
        store.heartbeat_run_lease(&durable, Duration::from_secs(1)),
        Err(AdkStoreError::Query(_))
    ));
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:161 TestRunExecutionLeaseUsesSafeDefaults
#[test]
fn production_heartbeat_keeps_its_owner_live_beyond_the_initial_snapshot_expiry() {
    let (_directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-long-lived-owner", json!([]));
    let baseline = Arc::strong_count(&store);
    let guard =
        RunLeaseGuard::acquire(store.clone(), "run-long-lived-owner", "live-owner").unwrap();
    let initial = guard.lease.clone();
    let deadline = Instant::now() + RUN_LEASE_TTL + RUN_LEASE_HEARTBEAT * 2;
    // Observe an actual production heartbeat after the initial snapshot's
    // expiry. No short lease, fixed sleep, or test-only heartbeat drives it.
    loop {
        assert!(!guard.is_lost(), "live owner lost its refreshed lease");
        let current = store.get_run_lease(&initial.run_id).unwrap().unwrap();
        if current.heartbeat_at_unix_ms > initial.expires_at_unix_ms {
            assert_eq!(current.owner_id, initial.owner_id);
            assert_eq!(current.fencing_token, initial.fencing_token);
            assert_eq!(
                current.expires_at_unix_ms - current.heartbeat_at_unix_ms,
                30_000
            );
            break;
        }
        assert!(Instant::now() < deadline, "heartbeat did not advance");
        thread::sleep(Duration::from_millis(10));
    }
    drop(guard);
    assert_eq!(Arc::strong_count(&store), baseline, "heartbeat joined");
    assert_eq!(
        store
            .get_run_lease(&initial.run_id)
            .unwrap()
            .unwrap()
            .owner_id,
        ""
    );
}
