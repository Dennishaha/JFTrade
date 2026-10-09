use super::super::super::terminal_audit_tests::{
    chat_for, create_running_run, initialized_stores, runtime_for,
};
use super::*;
use tokio_stream::StreamExt;

// Parity: go:452dea11:internal/api/assistant/chat_stream_recovery_contracts_test.go:41 TestChatStreamExecutionReusesKnownContextAndRecoversTerminalRun
#[tokio::test]
async fn production_known_context_preview_publishes_only_session_without_reloading_context() {
    let (directory, store, sessions) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &sessions);
    let scanner = runtime.recovery_supervisor.as_ref().unwrap();
    scanner.shutdown();
    assert_eq!(scanner.health_snapshot().status.as_str(), "shutdown");
    assert!(!scanner.health_snapshot().running);
    let mut chat = chat_for("run-known-context-preview");
    chat.route = super::super::super::AdkChatRoute::Stream;
    let probe = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    probe.set_nonblocking(true).unwrap();
    chat.request.endpoint = format!("http://{}/v1/responses", probe.local_addr().unwrap())
        .parse()
        .unwrap();
    store
        .upsert_session(
            &chat.session_id,
            &chat.agent_id,
            &json!({"id":chat.session_id,"agentId":chat.agent_id,"title":"Known context"})
                .to_string(),
        )
        .unwrap();
    sessions
        .upsert_session("jftrade", "local", &chat.session_id, "{}")
        .unwrap();
    let known = json!({"sessionId":chat.session_id,"contextRevisionId":"known-preview",
        "summary":"already captured context","compactedEventCount":0});
    store
        .upsert_session_context(&chat.session_id, &known.to_string())
        .unwrap();
    chat.request.durable_context =
        super::super::super::durable_context_items(&store, &sessions, &chat.session_id, None)
            .unwrap();
    chat.context_deltas = vec![super::super::super::SessionContextDelta::Context(known)];
    // The prepared execution already owns its context. A fresh durable read
    // now fails, making an accidental reload visible independently of frames.
    store.upsert_session_context(&chat.session_id, "{").unwrap();
    let context = store
        .get_session_context(&chat.session_id)
        .unwrap()
        .unwrap();
    assert!(
        matches!(super::super::super::durable_context_items(&store, &sessions, &chat.session_id, None),
        Err(super::super::super::AdkChatPortError::Failed {status:500,ref code,..}) if code=="ADK_STORAGE_CORRUPT")
    );
    create_running_run(&store, &chat.run_id, json!([]));
    let mut payload: Value =
        serde_json::from_str(&store.get_run(&chat.run_id).unwrap().unwrap().payload_json).unwrap();
    payload["streamEvents"] = json!([]);
    store
        .update_run_state(&chat.run_id, "RUNNING", &payload.to_string())
        .unwrap();
    let audit = store.list_audit_events().unwrap();
    let session = store.get_session(&chat.session_id).unwrap().unwrap();
    let lease = RunLeaseGuard::acquire(store.clone(), &chat.run_id, "known-context-owner").unwrap();
    let (stream, sender) = jftrade_api::ApiStream::channel(8);
    runtime
        .emit_preview_session(&chat, &sender, &lease)
        .unwrap();
    drop(sender);
    let mut body = stream.take_body().unwrap();
    let frame = String::from_utf8(body.next().await.unwrap().unwrap()).unwrap();
    let event: Value = serde_json::from_str(
        frame
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    assert_eq!(event["type"], "session");
    assert_eq!(event["session"]["id"], chat.session_id);
    assert_eq!(event["session"]["createdAt"], session.created_at);
    assert_eq!(event["sequence"], 1);
    assert!(
        body.next().await.is_none(),
        "one preview frame, no duplicate context"
    );
    let run = store.get_run(&chat.run_id).unwrap().unwrap();
    let persisted: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(run.status, "RUNNING");
    assert_eq!(persisted["streamEvents"], json!([event]));
    assert_eq!(
        store
            .get_session_context(&chat.session_id)
            .unwrap()
            .unwrap(),
        context
    );
    assert_eq!(
        store.get_session(&chat.session_id).unwrap().unwrap(),
        session
    );
    assert_eq!(store.list_audit_events().unwrap(), audit);
    let events = sessions.list_events(&chat.session_id).unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].author, "assistant.stream");
    assert_eq!(events[0].invocation_id, chat.run_id);
    assert_eq!(
        probe.accept().unwrap_err().kind(),
        std::io::ErrorKind::WouldBlock
    );
    drop(lease);
    runtime.shutdown_with_error().unwrap();
}
