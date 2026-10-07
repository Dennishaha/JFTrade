use super::*;

fn runtime_fixture() -> (tempfile::TempDir, Arc<AdkStore>, Arc<ProductionAdkChatRuntime>) {
    let (directory, store, sessions) = initialized_stores();
    let settings = directory.path().join("settings.json");
    std::fs::write(&settings, b"{}").unwrap();
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store), sessions, &settings,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    (directory, store, runtime)
}

fn wait_for_cancel(cancellation: &std::sync::atomic::AtomicBool) {
    let deadline = std::time::Instant::now() + Duration::from_secs(2);
    while !cancellation.load(Ordering::Acquire) {
        assert!(std::time::Instant::now() < deadline, "background cancellation deadline");
        thread::sleep(Duration::from_millis(1));
    }
}

// Parity: go:452dea11:internal/assistant/workflow_lifecycle_test.go:13 TestServiceCloseCancelsAndJoinsAdmittedWorkflowBackground
#[test]
fn concurrent_runtime_shutdowns_cancel_and_join_admitted_background_before_returning() {
    let (_directory, _store, runtime) = runtime_fixture();
    let (started, entered) = std::sync::mpsc::channel();
    let (cancelled, observed) = std::sync::mpsc::channel();
    let (release, cleanup) = std::sync::mpsc::channel();
    runtime.continuation_supervisor.spawn("workflow-background", move |cancellation| {
        started.send(()).unwrap();
        wait_for_cancel(&cancellation);
        cancelled.send(()).unwrap();
        cleanup.recv_timeout(Duration::from_secs(2)).unwrap();
    }).expect("admit workflow background");
    entered.recv_timeout(Duration::from_secs(1)).unwrap();
    let barrier = Arc::new(Barrier::new(9));
    let (finished, results) = std::sync::mpsc::channel();
    let mut callers = Vec::new();
    for _ in 0..8 {
        let runtime = Arc::clone(&runtime);
        let barrier = Arc::clone(&barrier);
        let finished = finished.clone();
        callers.push(thread::spawn(move || {
            barrier.wait();
            runtime.shutdown();
            finished.send(()).unwrap();
        }));
    }
    barrier.wait();
    observed.recv_timeout(Duration::from_secs(1)).unwrap();
    let returned_early = results.recv_timeout(Duration::from_millis(30)).is_ok();
    release.send(()).unwrap();
    for caller in callers { caller.join().unwrap(); }
    assert!(!returned_early, "shutdown returned before admitted work exited");
    assert_eq!(results.try_iter().count(), 8);
    assert!(runtime.continuation_supervisor.spawn("after-close", |_| panic!("closed admission")).is_err());
}

// Parity: go:452dea11:internal/assistant/workflow_lifecycle_test.go:58 TestServiceCloseKeepsStoreOpenUntilWorkflowCleanupFinishes
#[test]
fn runtime_shutdown_allows_cancelled_background_to_persist_cleanup_before_joining() {
    let (_directory, store, runtime) = runtime_fixture();
    let (written, write_result) = std::sync::mpsc::channel();
    let (release, cleanup) = std::sync::mpsc::channel();
    let task_store = Arc::clone(&store);
    runtime.continuation_supervisor.spawn("workflow-lifecycle-test", move |cancellation| {
        wait_for_cancel(&cancellation);
        written.send(task_store.record_audit_event(
            "cleanup", "workflow.shutdown.cleanup", "workflow-lifecycle-test", "{}",
        )).unwrap();
        cleanup.recv_timeout(Duration::from_secs(2)).unwrap();
    }).unwrap();
    let closing = Arc::clone(&runtime);
    let closer = thread::spawn(move || closing.shutdown());
    write_result.recv_timeout(Duration::from_secs(1)).unwrap().expect("cleanup store remains writable");
    let returned_early = closer.is_finished();
    release.send(()).unwrap();
    closer.join().unwrap();
    assert!(!returned_early);
    let events = store.list_audit_events().unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].kind, "workflow.shutdown.cleanup");
    // Rust retains explicitly owned Arc stores; shutdown does not close borrowed stores.
    store.record_audit_event("after", "workflow.shutdown.after-close", "", "{}").unwrap();
    assert!(runtime.continuation_supervisor.spawn("after-close", |_| panic!("closed admission")).is_err());
}

// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_test.go:86 TestWorkflowRunWaitHonorsDeadlineAndCancellation
#[test]
fn workflow_wait_cancellation_is_immediate_before_a_long_requested_wait() {
    let start = std::time::Instant::now();
    let error = crate::product::product_mcp_production_executor::workflow_wait_cancellable(
        &json!({"durationMs": 25_000}), &|| true,
    ).unwrap_err();
    assert_eq!(error.status, 499);
    assert_eq!(error.code, "MCP_TOOL_CANCELLED");
    assert_eq!(error.message, "workflow.wait was cancelled");
    assert!(start.elapsed() < Duration::from_millis(500));
}
