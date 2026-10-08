use super::*;

// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:54 TestGoogleADKToolUsesDurableInvocationKeyAndReplay
// Parity: go:452dea11:internal/assistant/engine/execution_claims_test.go:108 TestFailedReadIsDurablyCompletedButProjectedAsFailedToolCall
#[test]
fn production_checkpoint_replays_legacy_terminal_labels_without_rewriting_the_ledger() {
    for fail in [false, true] {
        let fixture = ToolFixture::new(fail);
        let provider = Provider::new();
        let mut chat = fixture.seed("run-legacy-completion");
        chat.request.endpoint = provider.endpoint.parse().unwrap();
        let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "owner").unwrap();
        fixture
            .runtime
            .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
        let output = fixture.restore_tool_checkpoint(&chat, &lease);
        let legacy_status = if fail { "FAILED" } else { "SUCCEEDED" };
        assert_eq!(
            Connection::open(fixture.directory.path().join("adk.db"))
                .unwrap()
                .execute(
                    "UPDATE adk_tool_invocations SET status=?1
                     WHERE run_id=?2 AND idempotency_key='function-call-test'",
                    [legacy_status, &chat.run_id],
                )
                .unwrap(),
            1
        );
        let legacy = fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .unwrap();
        let before = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
        let payload: Value = serde_json::from_str(&before.payload_json).unwrap();
        let events = fixture.sessions.list_events(&chat.session_id).unwrap();
        // A different output cannot use compatibility to replace the winner.
        fixture
            .runtime
            .persist_tool_result(
                &chat,
                &payload["toolCalls"][0],
                "function-call-test",
                json!({"unexpected":"replacement"}),
                legacy_status,
                None,
                lease.owner_id(),
                legacy.fencing_token,
                lease.token(),
            )
            .unwrap();
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
            .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
        let completed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
        let payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
        assert_eq!(completed.status, "COMPLETED");
        assert_eq!(payload["toolResults"][0]["output"], output);
        assert_eq!(payload["toolCalls"][0]["status"], legacy_status);
        assert_eq!(payload["toolCalls"][0]["output"], output);
        if fail {
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
            legacy,
        );
        assert_eq!(
            fixture.sessions.list_events(&chat.session_id).unwrap(),
            events
        );
    }
}
