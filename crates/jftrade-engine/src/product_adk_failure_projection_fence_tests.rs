use super::super::{AdkChatPortError, AdkChatRoute};
use super::*;

fn stream_chat(store: &AdkStore, run_id: &str) -> ChatExecution {
    create_running_run(store, run_id, json!([]));
    let mut chat = chat_for(run_id);
    chat.route = AdkChatRoute::Stream;
    let row = store.get_run(run_id).unwrap().unwrap();
    let mut payload: Value = serde_json::from_str(&row.payload_json).unwrap();
    payload["route"] = json!("stream");
    payload["streamId"] = json!(run_id);
    payload["streamEvents"] = json!([]);
    store
        .update_run_state(run_id, "RUNNING", &payload.to_string())
        .unwrap();
    chat
}

fn setup_error() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "session context is corrupt".to_owned(),
    }
}

#[test]
fn created_failure_owner_rejects_stale_fence_before_terminal_and_message_writes() {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    let chat = stream_chat(&store, "run-created-stale");
    let old = store
        .claim_run_lease(&chat.run_id, "old", Duration::from_secs(60))
        .unwrap();
    // Retain the real old token but keep the heartbeat dormant. Expiry is an
    // explicit database transition, independent of scheduling or a short TTL.
    let guard = RunLeaseGuard::from_lease(
        store.clone(),
        old.clone(),
        Duration::from_secs(60),
        Duration::from_secs(3600),
    )
    .unwrap();
    Connection::open(directory.path().join("adk.db"))
        .unwrap()
        .execute(
            "UPDATE adk_run_leases SET expires_at_unix_ms=0 WHERE run_id=?1",
            [&chat.run_id],
        )
        .unwrap();
    let new = store
        .claim_run_lease(&chat.run_id, "new", Duration::from_secs(60))
        .unwrap();
    assert!(new.fencing_token > old.fencing_token);
    let before = store.get_run(&chat.run_id).unwrap().unwrap();
    let events = sessions.list_events(&chat.session_id).unwrap();
    let audits = store.list_audit_events().unwrap();
    assert!(
        runtime
            .persist_failure(&chat, &setup_error(), &guard)
            .is_err()
    );
    let mut payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    assert!(
        runtime
            .attach_terminal_failure_projection(&chat, &mut payload, &setup_error(), &guard)
            .is_err()
    );
    assert_eq!(store.get_run(&chat.run_id).unwrap().unwrap(), before);
    assert_eq!(sessions.list_events(&chat.session_id).unwrap(), events);
    assert_eq!(store.list_audit_events().unwrap(), audits);
    drop(guard);
    assert_eq!(store.get_run_lease(&chat.run_id).unwrap().unwrap(), new);
    runtime.shutdown_with_error().unwrap();
}

#[test]
fn failed_message_transaction_retains_error_then_attaches_final_at_same_sequence() {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    let chat = stream_chat(&store, "run-created-message-fail");
    let lease = RunLeaseGuard::acquire(store.clone(), &chat.run_id, "message-owner").unwrap();
    let connection = Connection::open(directory.path().join("adk-session.db")).unwrap();
    connection.execute_batch("CREATE TRIGGER fail_synthetic_message BEFORE INSERT ON events WHEN NEW.author='agent-terminal' BEGIN SELECT RAISE(ABORT, 'synthetic message write failed'); END;").unwrap();
    assert!(
        runtime
            .persist_failure(&chat, &setup_error(), &lease)
            .is_err()
    );
    let failed = store.get_run(&chat.run_id).unwrap().unwrap();
    assert_eq!(failed.status, "FAILED");
    let mut payload: Value = serde_json::from_str(&failed.payload_json).unwrap();
    assert!(payload.get("response").is_none());
    assert!(payload.get("finalMessageId").is_none());
    assert_eq!(payload["streamEvents"].as_array().unwrap().len(), 1);
    assert_eq!(payload["streamEvents"][0]["type"], "error");
    assert_eq!(payload["streamEvents"][0]["sequence"], 1);
    let events = sessions.list_events(&chat.session_id).unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].author, "assistant.stream");
    let audits = store.list_audit_events().unwrap();
    assert_eq!(
        audits.iter().filter(|row| row.kind == "run.failed").count(),
        1
    );
    connection
        .execute_batch("DROP TRIGGER fail_synthetic_message;")
        .unwrap();
    runtime
        .attach_terminal_failure_projection(&chat, &mut payload, &setup_error(), &lease)
        .unwrap();
    assert_eq!(payload["streamEvents"].as_array().unwrap().len(), 1);
    assert_eq!(payload["streamEvents"][0]["type"], "final");
    assert_eq!(payload["streamEvents"][0]["sequence"], 1);
    assert_eq!(payload["streamEvents"][0]["response"], payload["response"]);
    let retained = store.get_run(&chat.run_id).unwrap().unwrap();
    let events = sessions.list_events(&chat.session_id).unwrap();
    assert_eq!(
        events
            .iter()
            .filter(|row| Some(row.id.as_str()) == payload["finalMessageId"].as_str())
            .count(),
        1
    );
    runtime
        .persist_failure(&chat, &setup_error(), &lease)
        .unwrap();
    assert_eq!(store.get_run(&chat.run_id).unwrap().unwrap(), retained);
    assert_eq!(sessions.list_events(&chat.session_id).unwrap(), events);
    assert_eq!(store.list_audit_events().unwrap(), audits);
    runtime.shutdown_with_error().unwrap();
}
