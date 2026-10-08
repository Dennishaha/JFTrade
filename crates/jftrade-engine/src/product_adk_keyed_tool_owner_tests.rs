use super::super::super::{AdkChatPortError, execute_tool_with_timeout};
use super::*;

#[derive(Debug)]
struct KeyedTool {
    calls: Arc<AtomicUsize>,
    consume_key: bool,
}

impl AdkToolExecutor for KeyedTool {
    fn supports(&self, name: &str) -> bool {
        name == "test.key_ignored"
    }

    fn execute(&self, _: &str, _: &Value) -> Result<Value, String> {
        panic!("durable handler must receive its invocation context")
    }

    fn execute_with_context(
        &self,
        _: &str,
        _: &Value,
        context: &AdkToolInvocationContext,
        _: &dyn Fn() -> bool,
    ) -> Result<Value, String> {
        self.calls.fetch_add(1, Ordering::AcqRel);
        if self.consume_key {
            Ok(json!({"key":context.idempotency_key()}))
        } else {
            Ok(json!({"ok":true}))
        }
    }
}

fn keyed_fixture(consume_key: bool) -> (ToolFixture, ChatExecution) {
    let calls = Arc::new(AtomicUsize::new(0));
    let mut fixture =
        ToolFixture::with_executor(calls.clone(), Arc::new(KeyedTool { calls, consume_key }));
    fixture.runtime.tool_catalog = Arc::new(
        crate::product::product_production_ports::ProductionToolCatalog::from_tool_rows(vec![
            json!({"id":"test.key_ignored", "permission":"write_internal",
                "idempotencyMode":"keyed", "allowedModes":["all"]}),
        ]),
    );
    create_running_run(
        &fixture.store,
        "run-key-ignored",
        json!([{
            "id":"function-call-test", "name":"test.key_ignored", "toolName":"test.key_ignored",
            "arguments":{"value":"once"}, "status":"RUNNING", "requiresUser":false,
        }]),
    );
    fixture
        .sessions
        .upsert_session("jftrade", "local", "session-run-key-ignored", "{}")
        .unwrap();
    (fixture, chat_for("run-key-ignored"))
}

fn restore_keyed_checkpoint(fixture: &ToolFixture, chat: &ChatExecution, lease: &RunLeaseGuard) {
    let run = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let mut payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    payload["status"] = json!("RUNNING");
    payload["toolCalls"][0]["status"] = json!("RUNNING");
    payload["toolResults"] = json!([]);
    assert!(
        fixture
            .store
            .update_run_state_if_status_and_revision_with_lease(
                &chat.run_id,
                &run.status,
                &run.updated_at,
                "RUNNING",
                &payload.to_string(),
                lease.owner_id(),
                lease.token(),
            )
            .unwrap()
    );
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:214 TestGoogleADKKeyedToolFailsClosedWhenHandlerIgnoresKey
#[test]
fn production_worker_returns_unknown_and_its_validation_does_not_count_as_handler_consumption() {
    let (fixture, chat) = keyed_fixture(false);
    let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "key-owner").unwrap();
    assert!(
        fixture
            .runtime
            .tool_catalog
            .requires_idempotency_key("test.key_ignored")
    );
    let claim = fixture
        .runtime
        .claim_tool_invocation_with_retry(
            &chat,
            "function-call-test",
            "test.key_ignored",
            r#"{"value":"once"}"#,
            lease.owner_id(),
            &lease,
            &Arc::new(AtomicBool::new(false)),
            false,
        )
        .unwrap()
        .unwrap();
    let AdkToolInvocationClaim::Execute(invocation) = claim else {
        panic!("fresh invocation must execute");
    };
    let context = AdkToolInvocationContext::from_invocation(&invocation, true);
    let error = execute_tool_with_timeout(
        &fixture.runtime.tool_executor,
        "test.key_ignored",
        &json!({"value":"once"}),
        Some(&context),
        Arc::new(|| false),
        Duration::from_secs(5),
    )
    .unwrap_err();
    assert!(
        matches!(error, AdkChatPortError::Failed { code, .. } if code == "ADK_TOOL_OUTCOME_UNKNOWN")
    );
    assert!(
        context.key_consumption_missing(),
        "framework check must not record a handler read"
    );
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

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:214 TestGoogleADKKeyedToolFailsClosedWhenHandlerIgnoresKey
#[test]
fn production_keyed_handler_ignoring_key_fails_unknown_once_and_blocks_the_second_attempt() {
    let (fixture, mut chat) = keyed_fixture(false);
    let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "key-owner").unwrap();
    let mut first_invocation = None;
    for attempt in 0..2 {
        let provider = Provider::new();
        chat.request.endpoint = provider.endpoint.parse().unwrap();
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
        assert_eq!(invocation.status, "UNKNOWN");
        let output: Value = serde_json::from_str(&invocation.output_json).unwrap();
        assert_eq!(output["error"]["code"], "SUBMISSION_UNKNOWN");
        assert_eq!(payload["toolCalls"][0]["errorCode"], "SUBMISSION_UNKNOWN");
        assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
        if attempt == 0 {
            assert_eq!(run.status, "COMPLETED", "{payload}");
            assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
            assert_eq!(payload["degraded"], true);
            assert!(
                payload["toolCalls"][0]["error"]
                    .as_str()
                    .unwrap()
                    .contains("did not consume")
            );
            first_invocation = Some(invocation);
            restore_keyed_checkpoint(&fixture, &chat, &lease);
        } else {
            assert_eq!(run.status, "COMPLETED", "{payload}");
            assert_eq!(payload["errorCode"], "");
            assert_eq!(payload["degraded"], true);
            assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
            assert_eq!(payload["failureReason"], "");
            assert!(
                payload["toolCalls"][0]["error"]
                    .as_str()
                    .unwrap()
                    .contains("did not consume")
            );
            assert_eq!(
                Some(invocation),
                first_invocation,
                "retry must keep the unknown row unchanged"
            );
        }
    }
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:54 TestGoogleADKToolUsesDurableInvocationKeyAndReplay
#[test]
fn production_keyed_handler_consuming_the_key_completes_and_replays_without_execution() {
    let (fixture, mut chat) = keyed_fixture(true);
    let provider = Provider::new();
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "key-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let output = fixture.restore_tool_checkpoint(&chat, &lease);
    assert_eq!(output["key"], "run-key-ignored:function-call-test");
    let replay_provider = Provider::new();
    chat.request.endpoint = replay_provider.endpoint.parse().unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let run = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(run.status, "COMPLETED");
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
}

fn unknown_checkpoint() -> (
    ToolFixture,
    ChatExecution,
    RunLeaseGuard,
    Value,
    jftrade_store_sqlite::StoredAdkToolInvocation,
) {
    let (fixture, mut chat) = keyed_fixture(false);
    let provider = Provider::new();
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "unknown-owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let output = fixture.restore_tool_checkpoint(&chat, &lease);
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    assert_eq!(invocation.status, "UNKNOWN");
    (fixture, chat, lease, output, invocation)
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:214 TestGoogleADKKeyedToolFailsClosedWhenHandlerIgnoresKey
// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:167 TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover
// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:343 TestRunSaveIsFencedWithExecutionLeaseContext
#[test]
fn production_unknown_replay_rejects_stale_owner_and_late_results_before_takeover_restoration() {
    let (fixture, mut chat, stale, output, invocation) = unknown_checkpoint();
    fixture.expire(&chat.run_id);
    let current =
        RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "current-owner").unwrap();
    assert!(current.token() > stale.token());
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    let events = fixture.sessions.list_events(&chat.session_id).unwrap();
    for (owner, token, proposed, status) in [
        (stale.owner_id(), stale.token(), output.clone(), "UNKNOWN"),
        (
            current.owner_id(),
            current.token(),
            output.clone(),
            "SUCCEEDED",
        ),
        (
            current.owner_id(),
            current.token(),
            json!({"success":true}),
            "UNKNOWN",
        ),
    ] {
        assert!(matches!(
            fixture.runtime.persist_tool_result(
                &chat,
                &payload["toolCalls"][0],
                "function-call-test",
                proposed,
                status,
                None,
                owner,
                invocation.fencing_token,
                token,
            ),
            Err(AdkChatPortError::Conflict(_))
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
    }
    let provider = Provider::new();
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &current);
    let restored = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&restored.payload_json).unwrap();
    assert_eq!(restored.status, "COMPLETED");
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert_eq!(payload["toolCalls"][0]["errorCode"], "SUBMISSION_UNKNOWN");
    assert_eq!(payload["degraded"], true);
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
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
}

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:214 TestGoogleADKKeyedToolFailsClosedWhenHandlerIgnoresKey
#[test]
fn production_unknown_replay_journal_failure_rolls_back_run_events_and_ledger() {
    let (fixture, chat, lease, output, invocation) = unknown_checkpoint();
    let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    let events = fixture.sessions.list_events(&chat.session_id).unwrap();
    let connection = Connection::open(fixture.directory.path().join("adk-session.db")).unwrap();
    connection.execute_batch("CREATE TRIGGER reject_tool_replay BEFORE INSERT ON events
        WHEN NEW.author='assistant.tool' BEGIN SELECT RAISE(FAIL, 'replay journal write failed'); END;").unwrap();
    let persist = || {
        fixture.runtime.persist_tool_result(
            &chat,
            &payload["toolCalls"][0],
            "function-call-test",
            output.clone(),
            "UNKNOWN",
            None,
            lease.owner_id(),
            invocation.fencing_token,
            lease.token(),
        )
    };
    assert!(matches!(persist(), Err(AdkChatPortError::Unavailable(_))));
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
    persist().unwrap();
    let restored = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&restored.payload_json).unwrap();
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert_eq!(fixture.calls.load(Ordering::Acquire), 1);
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
}
