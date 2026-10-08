use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortOutput, AdkChatStreamPort,
};

struct BlockedChat {
    directory: tempfile::TempDir,
    store: Arc<AdkStore>,
    runtime: Arc<ProductionAdkChatRuntime>,
    entered: mpsc::Receiver<()>,
    release: Option<mpsc::Sender<()>>,
    provider: Option<thread::JoinHandle<()>>,
}

impl BlockedChat {
    fn new(streaming: bool) -> Self {
        let (directory, store, sessions) = initialized_stores();
        let (endpoint, entered, release, provider) = blocked_provider(streaming, true, false);
        store
            .upsert_provider(
                "provider-close",
                &json!({
                    "id":"provider-close", "baseUrl":endpoint, "model":"fixture",
                    "enabled":true, "apiKey":"sk-fixture", "requestTimeoutMs":60_000,
                })
                .to_string(),
            )
            .unwrap();
        store
            .upsert_agent(
                "agent-close",
                &json!({
                    "id":"agent-close", "providerId":"provider-close", "status":"ENABLED",
                    "toolAccessMode":"none",
                })
                .to_string(),
            )
            .unwrap();
        let runtime = runtime_for(&directory, &store, &sessions);
        Self {
            directory,
            store,
            runtime,
            entered,
            release: Some(release),
            provider: Some(provider),
        }
    }

    fn cancellation(&self) -> Arc<AtomicBool> {
        self.entered.recv_timeout(Duration::from_secs(15)).unwrap();
        self.runtime
            .cancellation_registry
            .active
            .lock()
            .unwrap()
            .get("run-close")
            .unwrap()[0]
            .clone()
    }
}

impl Drop for BlockedChat {
    fn drop(&mut self) {
        if let Some(release) = self.release.take() {
            let _ = release.send(());
        }
        self.runtime.shutdown();
        if let Some(provider) = self.provider.take() {
            let result = provider.join();
            if !thread::panicking() {
                result.unwrap();
            }
        }
    }
}

fn input(id: &str) -> AdkChatInput {
    AdkChatInput {
        client_request_id: id.to_owned(),
        body: json!({"agentId":"agent-close", "message":"hello"})
            .to_string()
            .into_bytes(),
    }
}

fn observe_cancel(token: &AtomicBool) -> bool {
    let deadline = Instant::now() + Duration::from_secs(1);
    while !token.load(Ordering::Acquire) && Instant::now() < deadline {
        thread::sleep(Duration::from_millis(1));
    }
    token.load(Ordering::Acquire)
}

fn close_while_release_is_blocked(
    fixture: &BlockedChat,
    token: &AtomicBool,
) -> (bool, bool, Result<(), AdkChatPortError>) {
    let blocker = Connection::open(fixture.directory.path().join("adk.db")).unwrap();
    blocker.execute_batch("BEGIN IMMEDIATE").unwrap();
    let runtime = fixture.runtime.clone();
    let (finished, result) = mpsc::channel();
    let closer = thread::spawn(move || {
        finished.send(runtime.shutdown_with_error()).unwrap();
    });
    let cancelled_before_deadline = observe_cancel(token);
    let returned_early = result.try_recv().ok();
    let returned_before_release = returned_early.is_some();
    blocker.execute_batch("ROLLBACK").unwrap();
    let closed = match returned_early {
        Some(result) => result,
        None => result.recv_timeout(Duration::from_secs(2)).unwrap(),
    };
    closer.join().unwrap();
    (cancelled_before_deadline, returned_before_release, closed)
}

fn assert_closed(fixture: &BlockedChat, observation: (bool, bool, Result<(), AdkChatPortError>)) {
    let (cancelled_before_deadline, returned_before_release, closed) = observation;
    assert!(
        cancelled_before_deadline,
        "close must cancel the lease owner within one second"
    );
    assert!(
        !returned_before_release,
        "shutdown returned before the durable lease release could finish"
    );
    closed.unwrap();
    assert_eq!(
        fixture
            .store
            .get_run_lease("run-close")
            .unwrap()
            .unwrap()
            .owner_id,
        ""
    );
    assert!(
        fixture
            .runtime
            .continuation_supervisor
            .tasks
            .lock()
            .unwrap()
            .is_empty()
    );
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:176 TestRuntimeCloseCancelsAndWaitsForInFlightRunLease
#[test]
fn production_shutdown_waits_for_the_sync_chat_lease_release() {
    let fixture = BlockedChat::new(false);
    let runtime = fixture.runtime.clone();
    let caller = thread::spawn(move || runtime.dispatch(AdkChatRoute::Chat, &input("close")));
    let token = fixture.cancellation();
    let observation = close_while_release_is_blocked(&fixture, &token);
    assert!(caller.join().unwrap().is_err());
    assert_closed(&fixture, observation);
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:176 TestRuntimeCloseCancelsAndWaitsForInFlightRunLease
#[test]
fn production_shutdown_waits_for_the_live_stream_lease_release() {
    let fixture = BlockedChat::new(true);
    let output = fixture
        .runtime
        .dispatch(AdkChatRoute::Stream, &input("close"))
        .unwrap();
    assert!(matches!(output, AdkChatPortOutput::LiveStream(_)));
    let token = fixture.cancellation();
    let observation = close_while_release_is_blocked(&fixture, &token);
    drop(output);
    assert_closed(&fixture, observation);
}

// Parity: go:452dea11:internal/assistant/engine/runtime_execution_lease_boundaries_test.go:234 TestRuntimeCloseRejectsRunLeaseWorkAfterClosingStarts
#[test]
fn production_lease_admission_rejects_closing_and_closed_without_persisting() {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    let (cancelled, observed) = mpsc::channel();
    let (release, cleanup) = mpsc::channel();
    runtime
        .continuation_supervisor
        .spawn("close-barrier", move |cancellation| {
            assert!(observe_cancel(&cancellation));
            cancelled.send(()).unwrap();
            cleanup.recv_timeout(Duration::from_secs(3)).unwrap();
        })
        .unwrap();
    let closing = runtime.clone();
    let closer = thread::spawn(move || closing.shutdown_with_error());
    observed.recv_timeout(Duration::from_secs(1)).unwrap();
    let admission = runtime.acquire_run_lease_with_retry(
        "run-after-closing",
        "late-owner",
        &Arc::new(AtomicBool::new(false)),
    );
    let rejected = matches!(admission, Err(AdkChatPortError::Unavailable(ref message)) if message == "assistant runtime is stopping");
    drop(admission);
    let persisted = store.get_run_lease("run-after-closing").unwrap();
    let chat_rejected = runtime
        .dispatch(AdkChatRoute::Chat, &input("after-closing"))
        .is_err();
    let stream_rejected = runtime
        .dispatch(AdkChatRoute::Stream, &input("after-closing-stream"))
        .is_err();
    release.send(()).unwrap();
    closer.join().unwrap().unwrap();
    let after = runtime.acquire_run_lease_with_retry(
        "run-after-close",
        "late-owner",
        &Arc::new(AtomicBool::new(false)),
    );
    let after_rejected = matches!(after, Err(AdkChatPortError::Unavailable(ref message)) if message == "assistant runtime is stopping");
    drop(after);
    assert!(rejected, "closing runtime must reject new run leases");
    assert!(after_rejected, "closed runtime must reject new run leases");
    assert!(persisted.is_none());
    assert!(store.get_run_lease("run-after-close").unwrap().is_none());
    assert!(chat_rejected && stream_rejected);
    assert!(store.list_runs().unwrap().is_empty());
}

// Parity: go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:314 TestGoBackgroundNilGuardsAndClosingState
// Parity: go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:347 TestRuntimeCloseWaitsForInFlightBackgroundWorkAndRejectsNewWork
// Parity: go:452dea11:internal/assistant/engine/runner_continuation_boundaries_test.go:406 TestGoBackgroundUsesBackgroundCtxOrFallback
#[test]
fn production_background_shutdown_cancels_joins_and_rejects_new_work() {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    let (started, entered) = mpsc::channel();
    let (cancelled, observed) = mpsc::channel();
    let (release, cleanup) = mpsc::channel();
    runtime
        .continuation_supervisor
        .spawn("owned-background", move |cancellation| {
            started.send(()).unwrap();
            assert!(observe_cancel(&cancellation));
            cancelled.send(()).unwrap();
            cleanup.recv_timeout(Duration::from_secs(3)).unwrap();
        })
        .unwrap();
    entered.recv_timeout(Duration::from_millis(100)).unwrap();
    let closing = runtime.clone();
    let closer = thread::spawn(move || closing.shutdown_with_error());
    observed.recv_timeout(Duration::from_secs(1)).unwrap();
    let returned_early = closer.is_finished();
    let rejected = runtime
        .continuation_supervisor
        .spawn("while-closing", |_| panic!("closed admission"))
        .is_err();
    release.send(()).unwrap();
    closer.join().unwrap().unwrap();
    assert!(!returned_early);
    assert!(rejected);
    assert!(
        runtime
            .continuation_supervisor
            .spawn("after-close", |_| panic!("closed admission"))
            .is_err()
    );
    assert!(
        runtime
            .continuation_supervisor
            .tasks
            .lock()
            .unwrap()
            .is_empty()
    );
}

#[test]
fn production_shutdown_cancels_a_provider_token_registered_after_close_started() {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    runtime.shutdown_with_error().unwrap();
    let token = runtime.cancellation_registry.register("late-provider");
    let cancelled = token.load(Ordering::Acquire);
    runtime
        .cancellation_registry
        .unregister("late-provider", &token);
    assert!(
        cancelled,
        "provider registration cannot miss shutdown cancellation"
    );
}
