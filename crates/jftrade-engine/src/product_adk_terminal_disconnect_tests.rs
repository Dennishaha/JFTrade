use super::*;

// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:1124 TestCancelRunOnTerminalStateIsNoop
#[test]
fn late_disconnect_and_failure_preserve_terminal_run_history_and_audit() {
    for status in ["COMPLETED", "FAILED", "CANCELLED", "TIMED_OUT", "DENIED"] {
        let (directory, store, session_store) = initialized_stores();
        let runtime = runtime_for(&directory, &store, &session_store);
        let run_id = "run-terminal-disconnect";
        create_running_run(&store, run_id, json!([]));
        let lease = RunLeaseGuard::acquire(store.clone(), run_id, "owner-terminal").unwrap();
        let mut chat = chat_for(run_id);
        chat.route = super::super::AdkChatRoute::Stream;
        let terminal = json!({
            "id": run_id, "status": status, "message": "completed",
            "completedAt": "2026-10-08T00:00:00Z",
            "cancelledAt": "2026-10-08T00:00:00Z",
            "streamEvents": [{"type":"final", "sequence":1, "response":{"reply":"completed"}}]
        });
        store
            .update_run_state(run_id, status, &terminal.to_string())
            .unwrap();
        session_store
            .upsert_session("jftrade", "local", &chat.session_id, "{}")
            .unwrap();
        session_store
            .record_event(jftrade_store_sqlite::RecordAdkEventParams {
                id: "terminal-event",
                app_name: "jftrade",
                user_id: "local",
                session_id: &chat.session_id,
                invocation_id: run_id,
                author: "assistant",
                content: "completed",
            })
            .unwrap();
        let before = store.get_run(run_id).unwrap().unwrap();
        let events = session_store.list_events(&chat.session_id).unwrap();
        let audits = store.list_audit_events().unwrap();
        let disconnect = super::super::AdkChatPortError::Failed {
            status: 499,
            code: "CLIENT_DISCONNECTED".to_owned(),
            message: "assistant chat client disconnected".to_owned(),
        };
        for _ in 0..2 {
            runtime
                .persist_cancelled(&chat, &disconnect, &lease)
                .unwrap();
            let failure = runtime.persist_failure(&chat, &disconnect, &lease);
            match status {
                "CANCELLED" => assert!(
                    matches!(failure, Err(super::super::AdkChatPortError::Failed { code, .. }) if code == "RUN_CANCELLED")
                ),
                "DENIED" => assert!(matches!(
                    failure,
                    Err(super::super::AdkChatPortError::Unavailable(_))
                )),
                _ => failure.unwrap(),
            }
            assert_eq!(store.get_run(run_id).unwrap().unwrap(), before, "{status}");
            assert_eq!(
                session_store.list_events(&chat.session_id).unwrap(),
                events,
                "{status}"
            );
            assert_eq!(store.list_audit_events().unwrap(), audits, "{status}");
        }
    }
}
