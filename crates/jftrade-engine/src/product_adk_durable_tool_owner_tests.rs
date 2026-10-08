use super::*;
use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::thread;

use super::super::{AdkToolExecutor, AdkToolInvocationContext, RUN_LEASE_TTL};
use jftrade_store_sqlite::AdkToolInvocationClaim;

#[path = "product_adk_keyed_tool_owner_tests.rs"]
mod keyed_tool;

#[path = "product_adk_durable_completion_owner_tests.rs"]
mod durable_completion;

#[path = "product_adk_uncertain_write_owner_tests.rs"]
mod uncertain_write;

#[derive(Debug)]
struct ReadTool {
    calls: Arc<AtomicUsize>,
    fail: bool,
}

impl AdkToolExecutor for ReadTool {
    fn supports(&self, name: &str) -> bool {
        name == "market.snapshot"
    }
    fn execute(&self, _: &str, arguments: &Value) -> Result<Value, String> {
        self.calls.fetch_add(1, Ordering::AcqRel);
        if self.fail {
            Err("provider rejected the request".to_owned())
        } else {
            Ok(json!({"value":arguments["value"]}))
        }
    }
}

struct ToolFixture {
    directory: tempfile::TempDir,
    store: Arc<AdkStore>,
    sessions: Arc<AdkSessionStore>,
    runtime: ProductionAdkChatRuntime,
    calls: Arc<AtomicUsize>,
}

impl ToolFixture {
    fn new(fail: bool) -> Self {
        let calls = Arc::new(AtomicUsize::new(0));
        Self::with_executor(calls.clone(), Arc::new(ReadTool { calls, fail }))
    }

    fn with_executor(calls: Arc<AtomicUsize>, executor: Arc<dyn AdkToolExecutor>) -> Self {
        let (directory, store, sessions) = initialized_stores();
        let runtime = ProductionAdkChatRuntime::with_tool_executor_for_test(
            store.clone(),
            sessions.clone(),
            &directory.path().join("settings.json"),
            Arc::new(RunCancellationRegistry::default()),
            Arc::new(
                crate::product::product_production_ports::ProductionToolCatalog::empty_for_test(),
            ),
            executor,
        );
        Self {
            directory,
            store,
            sessions,
            runtime,
            calls,
        }
    }

    fn seed(&self, run_id: &str) -> ChatExecution {
        self.seed_with_value(run_id, "once")
    }

    fn seed_with_value(&self, run_id: &str, value: &str) -> ChatExecution {
        self.seed_with_tool(run_id, value, "market.snapshot")
    }

    fn seed_with_tool(&self, run_id: &str, value: &str, tool: &str) -> ChatExecution {
        create_running_run(
            &self.store,
            run_id,
            json!([{
                "id":"function-call-test", "name":tool, "toolName":tool,
                "arguments":{"value":value}, "status":"RUNNING", "requiresUser":false,
            }]),
        );
        self.sessions
            .upsert_session("jftrade", "local", &format!("session-{run_id}"), "{}")
            .unwrap();
        chat_for(run_id)
    }

    fn expire(&self, run_id: &str) {
        assert_eq!(
            Connection::open(self.directory.path().join("adk.db"))
                .unwrap()
                .execute(
                    "UPDATE adk_run_leases SET expires_at_unix_ms=0 WHERE run_id=?1",
                    [run_id]
                )
                .unwrap(),
            1
        );
    }

    fn restore_tool_checkpoint(&self, chat: &ChatExecution, lease: &RunLeaseGuard) -> Value {
        let completed = self.store.get_run(&chat.run_id).unwrap().unwrap();
        assert_eq!(completed.status, "COMPLETED");
        let mut payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
        let output = payload["toolResults"][0]["output"].clone();
        payload["status"] = json!("RUNNING");
        payload["toolCalls"][0]["status"] = json!("RUNNING");
        payload["toolResults"] = json!([]);
        assert!(
            self.store
                .update_run_state_if_status_and_revision_with_lease(
                    &chat.run_id,
                    "COMPLETED",
                    &completed.updated_at,
                    "RUNNING",
                    &payload.to_string(),
                    lease.owner_id(),
                    lease.token(),
                )
                .unwrap()
        );
        output
    }
}

impl Drop for ToolFixture {
    fn drop(&mut self) {
        self.runtime.shutdown();
    }
}

struct Provider {
    endpoint: String,
    stop: Arc<AtomicBool>,
    worker: Option<thread::JoinHandle<()>>,
}

impl Provider {
    fn new() -> Self {
        let (endpoint, stop, worker) = super::approval_concurrency::response_provider();
        Self {
            endpoint,
            stop,
            worker: Some(worker),
        }
    }
}

impl Drop for Provider {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        if let Some(worker) = self.worker.take() {
            let result = worker.join();
            if !thread::panicking() {
                result.unwrap();
            }
        }
    }
}

#[derive(Debug)]
struct ContextReadTool {
    calls: Arc<AtomicUsize>,
    observed_key: Arc<Mutex<Option<String>>>,
}

impl AdkToolExecutor for ContextReadTool {
    fn supports(&self, name: &str) -> bool {
        name == "market.snapshot"
    }
    fn execute(&self, _: &str, _: &Value) -> Result<Value, String> {
        self.calls.fetch_add(1, Ordering::AcqRel);
        Err("missing idempotency key".to_owned())
    }
    fn execute_with_context(
        &self,
        _: &str,
        arguments: &Value,
        invocation: &AdkToolInvocationContext,
        _: &dyn Fn() -> bool,
    ) -> Result<Value, String> {
        self.calls.fetch_add(1, Ordering::AcqRel);
        let key = invocation.idempotency_key();
        if key.is_empty() {
            return Err("missing idempotency key".to_owned());
        }
        *self.observed_key.lock().unwrap() = Some(key.to_owned());
        Ok(json!({"value":arguments["value"], "key":key}))
    }
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:54 TestGoogleADKToolUsesDurableInvocationKeyAndReplay
#[test]
fn production_handler_observes_a_stable_run_key_and_replay_keeps_the_same_output_key() {
    let calls = Arc::new(AtomicUsize::new(0));
    let observed_key = Arc::new(Mutex::new(None));
    let fixture = ToolFixture::with_executor(
        calls.clone(),
        Arc::new(ContextReadTool {
            calls: calls.clone(),
            observed_key: observed_key.clone(),
        }),
    );
    let provider = Provider::new();
    let mut chat = fixture.seed("run-wrapper");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "key-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let first = fixture.restore_tool_checkpoint(&chat, &lease);
    assert_eq!(first["value"], "once", "{first}");
    let key = observed_key
        .lock()
        .unwrap()
        .clone()
        .expect("handler observed key");
    assert!(!key.is_empty());
    assert!(key.contains("run-wrapper"));
    assert_eq!(key, "run-wrapper:function-call-test");
    assert_eq!(first["key"], key);
    let replay_provider = Provider::new();
    chat.request.endpoint = replay_provider.endpoint.parse().unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let replayed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&replayed.payload_json).unwrap();
    assert_eq!(replayed.status, "COMPLETED");
    assert_eq!(payload["toolResults"][0]["output"], first);
    assert_eq!(payload["toolResults"][0]["output"]["key"], key);
    assert_eq!(calls.load(Ordering::Acquire), 1);
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:54 TestGoogleADKToolUsesDurableInvocationKeyAndReplay
#[test]
fn production_read_checkpoint_replay_restores_the_original_result_without_execution() {
    replay_restored_checkpoint(false);
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:108 TestFailedReadIsDurablyCompletedButProjectedAsFailedToolCall
#[test]
fn production_failed_read_checkpoint_replay_preserves_the_original_error_without_execution() {
    replay_restored_checkpoint(true);
}

fn replay_restored_checkpoint(fail: bool) {
    let fixture = ToolFixture::new(fail);
    let provider = Provider::new();
    let mut chat = fixture.seed("run-checkpoint-replay");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "replay-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let first = fixture.restore_tool_checkpoint(&chat, &lease);
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    assert_eq!(invocation.status, "COMPLETED");
    let replay_provider = Provider::new();
    chat.request.endpoint = replay_provider.endpoint.parse().unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let replayed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&replayed.payload_json).unwrap();
    assert_eq!(replayed.status, "COMPLETED", "{payload}");
    assert_eq!(payload["toolResults"][0]["output"], first);
    assert_eq!(payload["toolCalls"][0]["output"], first);
    assert_eq!(
        payload["toolCalls"][0]["status"],
        if fail { "FAILED" } else { "SUCCEEDED" }
    );
    if fail {
        assert_eq!(
            payload["toolResults"][0]["output"]["error"]["message"],
            "provider rejected the request"
        );
        assert_eq!(
            payload["toolCalls"][0]["error"],
            "provider rejected the request"
        );
    }
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
    assert_eq!(
        fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .unwrap(),
        invocation,
        "terminal invocation remains unchanged"
    );
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:167 TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover
// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:343 TestRunSaveIsFencedWithExecutionLeaseContext
#[test]
fn production_checkpoint_replay_rejects_the_stale_owner_and_restores_under_takeover() {
    let fixture = ToolFixture::new(false);
    let provider = Provider::new();
    let mut chat = fixture.seed("run-replay-takeover");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let stale = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "stale-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &stale);
    let output = fixture.restore_tool_checkpoint(&chat, &stale);
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    fixture.expire(&chat.run_id);
    let current =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "current-owner").unwrap();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    let events = fixture.sessions.list_events(&chat.session_id).unwrap();
    assert!(matches!(
        fixture.runtime.persist_tool_result(
            &chat,
            &payload["toolCalls"][0],
            "function-call-test",
            output.clone(),
            "SUCCEEDED",
            None,
            stale.owner_id(),
            invocation.fencing_token,
            stale.token(),
        ),
        Err(super::super::AdkChatPortError::Conflict(_))
    ));
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert_eq!(
        fixture.sessions.list_events(&chat.session_id).unwrap(),
        events
    );
    let replay_provider = Provider::new();
    chat.request.endpoint = replay_provider.endpoint.parse().unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &current);
    let restored = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&restored.payload_json).unwrap();
    assert_eq!(restored.status, "COMPLETED");
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
    assert_eq!(
        fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .unwrap(),
        invocation
    );
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:108 TestFailedReadIsDurablyCompletedButProjectedAsFailedToolCall
#[test]
fn production_checkpoint_replay_journal_failure_rolls_back_the_projection() {
    let fixture = ToolFixture::new(true);
    let provider = Provider::new();
    let mut chat = fixture.seed("run-replay-journal-fault");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "replay-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let output = fixture.restore_tool_checkpoint(&chat, &lease);
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    let events = fixture.sessions.list_events(&chat.session_id).unwrap();
    let connection = Connection::open(fixture.directory.path().join("adk-session.db")).unwrap();
    connection.execute_batch("CREATE TRIGGER reject_tool_replay BEFORE INSERT ON events
        WHEN NEW.author='assistant.tool' BEGIN SELECT RAISE(FAIL, 'replay journal write failed'); END;").unwrap();
    let result = fixture.runtime.persist_tool_result(
        &chat,
        &payload["toolCalls"][0],
        "function-call-test",
        output.clone(),
        "FAILED",
        None,
        lease.owner_id(),
        invocation.fencing_token,
        lease.token(),
    );
    assert!(matches!(
        result,
        Err(super::super::AdkChatPortError::Unavailable(_))
    ));
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert_eq!(
        fixture.sessions.list_events(&chat.session_id).unwrap(),
        events
    );
    assert_eq!(
        fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .unwrap(),
        invocation
    );
    connection
        .execute_batch("DROP TRIGGER reject_tool_replay")
        .unwrap();
    fixture
        .runtime
        .persist_tool_result(
            &chat,
            &payload["toolCalls"][0],
            "function-call-test",
            output.clone(),
            "FAILED",
            None,
            lease.owner_id(),
            invocation.fencing_token,
            lease.token(),
        )
        .unwrap();
    let restored = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&restored.payload_json).unwrap();
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(
        fixture.sessions.list_events(&chat.session_id).unwrap(),
        events
    );
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:54 TestGoogleADKToolUsesDurableInvocationKeyAndReplay
#[test]
fn production_read_tool_result_replays_from_the_same_durable_invocation_without_execution() {
    let fixture = ToolFixture::new(false);
    let provider = Provider::new();
    let mut chat = fixture.seed("run-wrapper");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "replay-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let completed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    assert_eq!(completed.status, "COMPLETED");
    let mut payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
    let first = payload["toolResults"][0]["output"].clone();
    assert_eq!(first, json!({"value":"once"}));
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
    // Restore an in-progress checkpoint while retaining its durable result.
    // This exercises the production claim/replay owner, not another executor.
    payload["status"] = json!("RUNNING");
    assert!(
        fixture
            .store
            .update_run_state_if_status_and_revision_with_lease(
                &chat.run_id,
                "COMPLETED",
                &completed.updated_at,
                "RUNNING",
                &payload.to_string(),
                lease.owner_id(),
                lease.token(),
            )
            .unwrap()
    );
    let replay = fixture
        .runtime
        .claim_tool_invocation_with_retry(
            &chat,
            "function-call-test",
            "market.snapshot",
            r#"{"value":"once"}"#,
            lease.owner_id(),
            &lease,
            &Arc::new(AtomicBool::new(false)),
            false,
        )
        .unwrap()
        .unwrap();
    let AdkToolInvocationClaim::Replay(replay) = replay else {
        panic!("completed result must replay");
    };
    assert_eq!(replay.run_id, chat.run_id);
    assert_eq!(replay.idempotency_key, "function-call-test");
    assert_eq!(
        serde_json::from_str::<Value>(&replay.output_json).unwrap(),
        first
    );
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:108 TestFailedReadIsDurablyCompletedButProjectedAsFailedToolCall
#[test]
fn production_failed_read_persists_its_false_output_and_failed_call() {
    let fixture = ToolFixture::new(true);
    let provider = Provider::new();
    let mut chat = fixture.seed_with_value("run-failed-read", "invalid");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "failed-read-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let run = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    let output: Value = serde_json::from_str(&invocation.output_json).unwrap();
    assert_eq!(output["success"], false);
    assert_eq!(invocation.status, "COMPLETED");
    assert_eq!(output["error"]["message"], "provider rejected the request");
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert!(
        payload["toolCalls"][0]["error"]
            .as_str()
            .unwrap()
            .contains("provider rejected")
    );
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
    assert_eq!(run.status, "COMPLETED");
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:167 TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover
#[test]
fn production_stale_run_lease_executes_no_read_tool_and_preserves_the_replacement() {
    let fixture = ToolFixture::new(false);
    let chat = fixture.seed("run-stale-context");
    let stale = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "stale-owner").unwrap();
    fixture.expire(&chat.run_id);
    let current =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "current-owner").unwrap();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let replacement = fixture.store.get_run_lease(&chat.run_id).unwrap().unwrap();
    let claim = fixture
        .runtime
        .claim_tool_invocation_with_retry(
            &chat,
            "function-call-test",
            "market.snapshot",
            r#"{"value":"once"}"#,
            stale.owner_id(),
            &stale,
            &Arc::new(AtomicBool::new(false)),
            false,
        )
        .unwrap_err();
    assert!(matches!(
        claim,
        super::super::AdkChatPortError::Failed { status: 409, code, .. }
            if code == "ADK_RUN_LEASE_LOST"
    ));
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &stale);
    assert_eq!(fixture.calls.load(Ordering::Acquire), 0);
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert_eq!(
        fixture.store.get_run_lease(&chat.run_id).unwrap().unwrap(),
        replacement
    );
    assert!(
        fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .is_none()
    );
    drop(stale);
    assert_eq!(
        fixture.store.get_run_lease(&chat.run_id).unwrap().unwrap(),
        replacement
    );
    assert!(!current.is_lost());
    let provider = Provider::new();
    let mut current_chat = chat;
    current_chat.request.endpoint = provider.endpoint.parse().unwrap();
    fixture.runtime.run_tool_loop(
        current_chat.clone(),
        Arc::new(AtomicBool::new(false)),
        &current,
    );
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
    assert_eq!(
        fixture
            .store
            .get_run(&current_chat.run_id)
            .unwrap()
            .unwrap()
            .status,
        "COMPLETED"
    );
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:167 TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover
#[test]
fn production_tool_claim_preserves_precancellation_and_allows_its_current_owner() {
    let fixture = ToolFixture::new(false);
    let chat = fixture.seed("run-claim-cancelled");
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "current-owner").unwrap();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let cancelled = Arc::new(AtomicBool::new(true));
    let claim = || {
        fixture.runtime.claim_tool_invocation_with_retry(
            &chat,
            "function-call-test",
            "market.snapshot",
            r#"{"value":"once"}"#,
            lease.owner_id(),
            &lease,
            &cancelled,
            false,
        )
    };
    assert!(claim().unwrap().is_none());
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert!(
        fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .is_none()
    );
    cancelled.store(false, Ordering::Release);
    let Some(AdkToolInvocationClaim::Execute(invocation)) = claim().unwrap() else {
        panic!("current owner must obtain an execution claim");
    };
    assert_eq!(invocation.owner_id, lease.owner_id());
    assert_eq!(invocation.run_lease_token, lease.token());
    assert_eq!(fixture.calls.load(Ordering::Acquire), 0);
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:167 TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover
#[test]
fn production_tool_claim_classifies_sql_owner_and_cross_run_fence_rejections_as_lease_lost() {
    let fixture = ToolFixture::new(false);
    let chat = fixture.seed("run-claim-bound");
    let other_chat = fixture.seed("run-claim-other");
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "current-owner").unwrap();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let other_before = fixture.store.get_run(&other_chat.run_id).unwrap().unwrap();
    let durable = fixture.store.get_run_lease(&chat.run_id).unwrap().unwrap();
    for (attempt, owner) in [(&chat, "wrong-owner"), (&other_chat, lease.owner_id())] {
        assert!(
            !lease.is_lost(),
            "SQL fence rejects a valid lease bound to another identity"
        );
        let error = fixture
            .runtime
            .claim_tool_invocation_with_retry(
                attempt,
                "function-call-test",
                "market.snapshot",
                r#"{"value":"once"}"#,
                owner,
                &lease,
                &Arc::new(AtomicBool::new(false)),
                false,
            )
            .unwrap_err();
        assert!(super::super::is_nonfatal_durable_error(&error));
        assert!(
            matches!(error, super::super::AdkChatPortError::Failed { status:409, code, .. }
            if code == "ADK_RUN_LEASE_LOST")
        );
        assert!(
            fixture
                .store
                .get_tool_invocation(&attempt.run_id, "function-call-test")
                .unwrap()
                .is_none()
        );
    }
    assert_eq!(fixture.calls.load(Ordering::Acquire), 0);
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert_eq!(
        fixture.store.get_run(&other_chat.run_id).unwrap().unwrap(),
        other_before
    );
    assert_eq!(
        fixture.store.get_run_lease(&chat.run_id).unwrap().unwrap(),
        durable
    );
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:265 TestRuntimeReconciliationDoesNotStealFreshForeignLease
#[test]
fn production_expiry_reconciliation_preserves_a_fresh_foreign_lease_and_run() {
    let fixture = ToolFixture::new(false);
    let chat = fixture.seed("run-owned-elsewhere");
    let original = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload =
        json!({"status":"RUNNING", "startedAt":"2020-01-01T00:00:00Z", "maxDurationMs":1});
    assert!(
        fixture
            .store
            .update_run_state_if_status_and_revision(
                &chat.run_id,
                "RUNNING",
                &original.updated_at,
                "RUNNING",
                &payload.to_string()
            )
            .unwrap()
    );
    let foreign = fixture
        .store
        .claim_run_lease(
            &chat.run_id,
            "executor-other-process",
            Duration::from_secs(60),
        )
        .unwrap();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    fixture.runtime.reconcile_expired_runs().unwrap();
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert_eq!(
        fixture.store.get_run_lease(&chat.run_id).unwrap().unwrap(),
        foreign
    );
    assert_eq!(before.status, "RUNNING");
    assert!(fixture.store.list_audit_events().unwrap().is_empty());
    fixture.expire(&chat.run_id);
    fixture.runtime.reconcile_expired_runs().unwrap();
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap().status,
        "TIMED_OUT"
    );
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:343 TestRunSaveIsFencedWithExecutionLeaseContext
#[test]
fn production_run_writes_reject_stale_and_mismatched_fences_before_the_current_write() {
    let fixture = ToolFixture::new(false);
    let chat = fixture.seed("run-save-fenced");
    fixture.seed("run-save-fenced-other");
    let row = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let before_payload = json!({"status":"RUNNING", "message":"before takeover"});
    assert!(
        fixture
            .store
            .update_run_state_if_status_and_revision(
                &chat.run_id,
                "RUNNING",
                &row.updated_at,
                "RUNNING",
                &before_payload.to_string()
            )
            .unwrap()
    );
    let stale = fixture
        .store
        .claim_run_lease(&chat.run_id, "executor-stale", RUN_LEASE_TTL)
        .unwrap();
    fixture.expire(&chat.run_id);
    let current = fixture
        .store
        .claim_run_lease(&chat.run_id, "executor-current", RUN_LEASE_TTL)
        .unwrap();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let other = fixture
        .store
        .get_run("run-save-fenced-other")
        .unwrap()
        .unwrap();
    let stale_payload = json!({"status":"RUNNING", "message":"stale write"});
    assert!(matches!(
        fixture
            .store
            .update_run_state_if_status_and_revision_with_lease(
                &chat.run_id,
                "RUNNING",
                &before.updated_at,
                "RUNNING",
                &stale_payload.to_string(),
                &stale.owner_id,
                stale.fencing_token,
            ),
        Ok(false)
    ));
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        before
    );
    assert!(matches!(
        fixture
            .store
            .update_run_state_if_status_and_revision_with_lease(
                &other.id,
                "RUNNING",
                &other.updated_at,
                "RUNNING",
                &stale_payload.to_string(),
                &current.owner_id,
                current.fencing_token,
            ),
        Ok(false)
    ));
    assert_eq!(fixture.store.get_run(&other.id).unwrap().unwrap(), other);
    let payload = json!({"status":"RUNNING", "message":"current write"});
    assert!(
        fixture
            .store
            .update_run_state_if_status_and_revision_with_lease(
                &chat.run_id,
                "RUNNING",
                &before.updated_at,
                "RUNNING",
                &payload.to_string(),
                &current.owner_id,
                current.fencing_token,
            )
            .unwrap()
    );
    assert_eq!(
        serde_json::from_str::<Value>(
            &fixture
                .store
                .get_run(&chat.run_id)
                .unwrap()
                .unwrap()
                .payload_json
        )
        .unwrap()["message"],
        "current write"
    );
}
