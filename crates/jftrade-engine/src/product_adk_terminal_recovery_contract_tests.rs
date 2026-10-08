use super::*;
use crate::product::product_adk_model_runtime::recover_terminal_stream_event;
use jftrade_store_sqlite::RecordAdkEventParams;

fn projection_port(
    status: &str,
    final_id: &str,
) -> (
    tempfile::TempDir,
    Arc<ProductionAdkPort>,
    std::net::TcpListener,
) {
    let (directory, port, provider) = ready_port();
    port.store
        .upsert_session(
            "session-recovery",
            "agent-recovery",
            r#"{"id":"session-recovery","agentId":"agent-recovery","title":"Recovery session"}"#,
        )
        .unwrap();
    port.session_store
        .upsert_session("jftrade", "local", "session-recovery", "{}")
        .unwrap();
    port.store.create_run(CreateAdkRunParams {
        id:"run-recovery", session_id:"session-recovery", agent_id:"agent-recovery", status,
        client_request_id:"request-recovery", request_fingerprint:"recovery",
        payload_json:&json!({"id":"run-recovery","sessionId":"session-recovery","agentId":"agent-recovery",
            "status":status,"finalMessageId":final_id,"streamId":"stream-recovery","streamEvents":[]}).to_string(),
    }).unwrap();
    (directory, port, provider)
}

fn message(port: &ProductionAdkPort, id: &str, author: &str, reply: &str, reasoning: &str) {
    let mut parts = Vec::new();
    if !reasoning.is_empty() {
        parts.push(json!({"text":reasoning,"thought":true}));
    }
    if !reply.is_empty() {
        parts.push(json!({"text":reply}));
    }
    let content = json!({"role":if author=="user" {"user"}else{"model"},"parts":parts}).to_string();
    port.session_store
        .record_event(RecordAdkEventParams {
            id,
            app_name: "jftrade",
            user_id: "local",
            session_id: "session-recovery",
            invocation_id: "run-recovery",
            author,
            content: &content,
        })
        .unwrap();
}

// Parity: go:452dea11:internal/assistant/service_business_test.go:169 TestServiceRecoverTerminalChatResponseFromProjection
#[test]
fn production_terminal_recovery_selects_final_message_reply_reasoning_and_native_timeline() {
    let (_directory, port, provider) = projection_port("RUNNING", "assistant-final-recovery");
    message(&port, "user-recovery", "user", "请恢复终态回复", "");
    message(
        &port,
        "assistant-final-recovery",
        "assistant",
        "最终答复",
        "中间推理",
    );
    message(
        &port,
        "assistant-later-recovery",
        "assistant",
        "later reply",
        "later reasoning",
    );
    let running =
        recover_terminal_stream_event(&port.store, &port.session_store, "run-recovery").unwrap();
    let before = port.store.get_run("run-recovery").unwrap().unwrap();
    let mut payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    payload["status"] = json!("COMPLETED");
    port.store
        .update_run_state("run-recovery", "COMPLETED", &payload.to_string())
        .unwrap();
    let run = port.store.get_run("run-recovery").unwrap().unwrap();
    let events = port.session_store.list_events("session-recovery").unwrap();
    let audits = port.store.list_audit_events().unwrap();
    let final_event =
        recover_terminal_stream_event(&port.store, &port.session_store, "run-recovery")
            .unwrap()
            .unwrap();
    port.shutdown_with_error().unwrap();
    assert!(running.is_none());
    assert_eq!(final_event["type"], "final");
    let response = &final_event["response"];
    assert_eq!(response["reply"], "最终答复");
    assert_eq!(response["reasoningContent"], "中间推理");
    assert_eq!(response["run"]["id"], run.id);
    assert_eq!(response["session"]["id"], "session-recovery");
    assert_eq!(response["run"]["createdAt"], run.created_at);
    assert_eq!(response["run"]["updatedAt"], run.updated_at);
    let timeline = response["timeline"].as_array().unwrap();
    assert!(!timeline.is_empty());
    assert!(
        timeline
            .iter()
            .any(|entry| entry["id"] == "assistant-final-recovery" && entry["text"] == "最终答复")
    );
    assert_eq!(port.store.get_run("run-recovery").unwrap().unwrap(), run);
    assert_eq!(
        port.session_store.list_events("session-recovery").unwrap(),
        events
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audits);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

// Parity: go:452dea11:internal/assistant/service_recovery_test.go:10 TestServiceRecoverTerminalChatResponseFallsBackToLatestAssistant
#[test]
fn production_terminal_recovery_missing_final_id_uses_latest_assistant_not_user_or_bookkeeping() {
    let (_directory, port, provider) =
        projection_port("COMPLETED", "assistant-missing-after-append-failure");
    message(
        &port,
        "assistant-old",
        "assistant",
        "old reply",
        "old reasoning",
    );
    message(
        &port,
        "assistant-latest",
        "assistant",
        "最新终态答复",
        "最新推理",
    );
    message(
        &port,
        "user-later",
        "user",
        "must not become assistant reply",
        "",
    );
    message(
        &port,
        "stream-later",
        "assistant.stream",
        "must not become assistant reply",
        "",
    );
    let run = port.store.get_run("run-recovery").unwrap().unwrap();
    let events = port.session_store.list_events("session-recovery").unwrap();
    let audits = port.store.list_audit_events().unwrap();
    let final_event =
        recover_terminal_stream_event(&port.store, &port.session_store, "run-recovery")
            .unwrap()
            .unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(final_event["response"]["reply"], "最新终态答复");
    assert_eq!(final_event["response"]["reasoningContent"], "最新推理");
    assert_eq!(port.store.get_run("run-recovery").unwrap().unwrap(), run);
    assert_eq!(
        port.session_store.list_events("session-recovery").unwrap(),
        events
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audits);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

// Parity: go:452dea11:internal/assistant/service_lifecycle_boundaries_test.go:95 TestServiceRecoverTerminalChatResponseHandlesBlankRunIDAndMissingProjection
#[test]
fn production_terminal_recovery_blank_id_is_none_and_empty_transcript_keeps_identifiers_without_reply()
 {
    let (_directory, port, provider) = projection_port("COMPLETED", "");
    let blank = recover_terminal_stream_event(&port.store, &port.session_store, " \t ").unwrap();
    let missing =
        recover_terminal_stream_event(&port.store, &port.session_store, "missing-run").unwrap();
    let run = port.store.get_run("run-recovery").unwrap().unwrap();
    let audits = port.store.list_audit_events().unwrap();
    let final_event =
        recover_terminal_stream_event(&port.store, &port.session_store, "run-recovery")
            .unwrap()
            .unwrap();
    port.shutdown_with_error().unwrap();
    assert!(blank.is_none());
    assert!(missing.is_none());
    assert_eq!(final_event["response"]["reply"], "");
    assert!(final_event["response"].get("reasoningContent").is_none());
    assert_eq!(final_event["response"]["run"]["id"], "run-recovery");
    assert_eq!(final_event["response"]["session"]["id"], "session-recovery");
    assert!(
        final_event["response"]["timeline"]
            .as_array()
            .unwrap()
            .is_empty()
    );
    assert_eq!(port.store.get_run("run-recovery").unwrap().unwrap(), run);
    assert!(
        port.session_store
            .list_events("session-recovery")
            .unwrap()
            .is_empty()
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audits);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

async fn read_recovery(port: &ProductionAdkPort, after: u64) -> Vec<Value> {
    use tokio_stream::StreamExt;
    let stream = crate::product::AdkReadSnapshotPort::open_stream(
        port,
        "/api/v1/adk/runs/run-recovery/stream",
        &format!("after={after}"),
    )
    .unwrap()
    .unwrap();
    let mut body = stream.body.take_body().unwrap();
    let mut text = String::new();
    while let Some(bytes) = tokio::time::timeout(Duration::from_secs(3), body.next())
        .await
        .unwrap()
    {
        text.push_str(&String::from_utf8(bytes.unwrap()).unwrap());
    }
    frames(&text)
}

#[tokio::test]
async fn production_terminal_recovery_cursor_replaces_page_tail_error_and_does_not_repeat_final() {
    let (_directory, port, provider) = projection_port("FAILED", "");
    let before = port.store.get_run("run-recovery").unwrap().unwrap();
    let mut payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    let mut history = (1..64)
        .map(|sequence| json!({"type":"timeline","sequence":sequence}))
        .collect::<Vec<_>>();
    history.push(json!({"type":"error","sequence":64,"message":"provisional error"}));
    payload["streamEvents"] = json!(history);
    payload["providerEvents"] = json!([{"large":"provider history"}]);
    port.store
        .update_run_state("run-recovery", "FAILED", &payload.to_string())
        .unwrap();
    let run = port.store.get_run("run-recovery").unwrap().unwrap();
    let audits = port.store.list_audit_events().unwrap();
    let metadata = port
        .store
        .read_stream_projection("run-recovery")
        .unwrap()
        .unwrap();
    let all = read_recovery(&port, 0).await;
    let tail = read_recovery(&port, 63).await;
    let consumed = read_recovery(&port, 64).await;
    port.shutdown_with_error().unwrap();
    assert_eq!(all.len(), 64);
    assert_eq!(
        all.iter()
            .map(|event| event["sequence"].as_u64().unwrap())
            .collect::<Vec<_>>(),
        (1..=64).collect::<Vec<_>>()
    );
    assert_eq!(all.last().unwrap()["type"], "final");
    assert!(
        all.iter()
            .all(|event| event["replay"] == true && event["type"] != "error")
    );
    assert_eq!(tail, vec![all.last().unwrap().clone()]);
    assert!(consumed.is_empty());
    let metadata_payload: Value = serde_json::from_str(&metadata.run.payload_json).unwrap();
    assert!(metadata_payload.get("streamEvents").is_none());
    assert!(metadata_payload.get("providerEvents").is_none());
    assert_eq!(metadata.last_event.unwrap().0, 64);
    assert_eq!(port.store.get_run("run-recovery").unwrap().unwrap(), run);
    assert_eq!(port.store.list_audit_events().unwrap(), audits);
    assert!(
        port.session_store
            .list_events("session-recovery")
            .unwrap()
            .is_empty()
    );
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

#[tokio::test]
async fn production_terminal_recovery_cursor_appends_once_after_full_page_and_marks_later_terminal_live()
 {
    use tokio_stream::StreamExt;
    let (_directory, port, provider) = projection_port("RUNNING", "");
    let stream = crate::product::AdkReadSnapshotPort::open_stream(
        port.as_ref(),
        "/api/v1/adk/runs/run-recovery/stream",
        "after=64",
    )
    .unwrap()
    .unwrap();
    let mut body = stream.body.take_body().unwrap();
    assert_eq!(body.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
    let before = port.store.get_run("run-recovery").unwrap().unwrap();
    let mut payload: Value = serde_json::from_str(&before.payload_json).unwrap();
    payload["streamEvents"] = json!(
        (1..=64)
            .map(|sequence| json!({"type":"timeline","sequence":sequence}))
            .collect::<Vec<_>>()
    );
    port.store
        .update_run_state("run-recovery", "COMPLETED", &payload.to_string())
        .unwrap();
    let run = port.store.get_run("run-recovery").unwrap().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let live = String::from_utf8(body.next().await.unwrap().unwrap()).unwrap();
    assert!(body.next().await.is_none());
    drop(body);
    let historical = read_recovery(&port, 0).await;
    let tail = read_recovery(&port, 64).await;
    let consumed = read_recovery(&port, 65).await;
    port.shutdown_with_error().unwrap();
    let live = frames(&live);
    assert_eq!(live.len(), 1);
    assert_eq!(live[0]["type"], "final");
    assert_eq!(live[0]["sequence"], 65);
    assert_eq!(live[0]["replay"], false);
    assert_eq!(historical.len(), 65);
    assert_eq!(historical.last().unwrap()["sequence"], 65);
    assert_eq!(historical.last().unwrap()["type"], "final");
    assert!(historical.iter().all(|event| event["replay"] == true));
    assert_eq!(tail, vec![historical.last().unwrap().clone()]);
    assert!(consumed.is_empty());
    assert_eq!(port.store.get_run("run-recovery").unwrap().unwrap(), run);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert!(
        port.session_store
            .list_events("session-recovery")
            .unwrap()
            .is_empty()
    );
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}
