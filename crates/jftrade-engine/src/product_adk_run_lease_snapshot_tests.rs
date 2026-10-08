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
// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:294 TestRuntimeRunLeaseHeartbeatPreventsPrematureTakeover
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

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:294 TestRuntimeRunLeaseHeartbeatPreventsPrematureTakeover
#[test]
fn production_configured_run_lease_heartbeat_rejects_takeover_at_initial_expiry() {
    let (_directory, store, _sessions) = initialized_stores();
    create_running_run(&store, "run-configured-heartbeat", json!([]));
    let ttl = Duration::from_secs(3);
    let initial = store
        .claim_run_lease("run-configured-heartbeat", "live-owner", ttl)
        .unwrap();
    let baseline = Arc::strong_count(&store);
    let guard = RunLeaseGuard::from_lease(
        store.clone(),
        initial.clone(),
        ttl,
        Duration::from_millis(100),
    )
    .unwrap();
    let deadline = Instant::now() + Duration::from_secs(2);
    let renewed = loop {
        let current = store.get_run_lease(&initial.run_id).unwrap().unwrap();
        if current.expires_at_unix_ms > initial.expires_at_unix_ms {
            break current;
        }
        assert!(
            Instant::now() < deadline,
            "lease was not renewed within the original two-second window"
        );
        thread::sleep(Duration::from_millis(10));
    };
    assert_eq!(renewed.owner_id, initial.owner_id);
    assert_eq!(renewed.fencing_token, initial.fencing_token);
    assert_eq!(
        renewed.expires_at_unix_ms - renewed.heartbeat_at_unix_ms,
        3_000
    );
    let takeover_at = initial.expires_at_unix_ms + 1;
    assert!(renewed.expires_at_unix_ms > takeover_at);
    let initial_at = time::OffsetDateTime::from_unix_timestamp_nanos(
        i128::from(initial.heartbeat_at_unix_ms) * 1_000_000,
    )
    .unwrap();
    let takeover_time =
        time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(takeover_at) * 1_000_000)
            .unwrap();
    // At this same synthetic instant an unrenewed lease must be reclaimable;
    // a wall-clock fallback would still see its original three-second lease.
    create_running_run(&store, "run-without-heartbeat", json!([]));
    let unrenewed = store
        .claim_run_lease_at("run-without-heartbeat", "idle-owner", ttl, initial_at)
        .unwrap();
    assert_eq!(unrenewed.expires_at_unix_ms, initial.expires_at_unix_ms);
    let reclaimed = store
        .claim_run_lease_at(
            &unrenewed.run_id,
            "other-owner",
            Duration::from_secs(1),
            takeover_time,
        )
        .unwrap();
    assert!(reclaimed.fencing_token > unrenewed.fencing_token);
    assert_eq!(reclaimed.heartbeat_at_unix_ms, takeover_at);
    assert!(matches!(
        store.claim_run_lease_at(
            &initial.run_id,
            "other-owner",
            Duration::from_secs(1),
            takeover_time,
        ),
        Err(AdkStoreError::RunLeaseHeld { .. })
    ));
    assert!(!guard.is_lost());
    drop(guard);
    assert_eq!(Arc::strong_count(&store), baseline, "heartbeat joined");
    let takeover = store
        .claim_run_lease(&initial.run_id, "other-owner", Duration::from_secs(1))
        .unwrap();
    assert!(takeover.fencing_token > initial.fencing_token);
    assert!(takeover.fencing_token > 1);
}
