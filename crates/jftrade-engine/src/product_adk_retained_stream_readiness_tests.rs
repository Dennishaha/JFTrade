use super::*;
use crate::product::product_adk_chat_stream_port::{AdkChatInput, AdkChatRoute, AdkChatStreamPort};

const REQUEST: &str = "11111111-1111-4111-8111-111111111111";

pub(super) fn configured_port() -> (
    tempfile::TempDir,
    Arc<ProductionAdkPort>,
    std::net::TcpListener,
) {
    let (directory, mut port) = reconnect_port();
    let provider = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    provider.set_nonblocking(true).unwrap();
    set_provider(&port, &provider, false);
    let runtime = ProductionAdkChatRuntime::new(
        port.store.clone(),
        port.session_store.clone(),
        &port.settings_path,
        Arc::new(RunCancellationRegistry::default()),
        port.tool_catalog.clone(),
    );
    assert!(!runtime.runtime_ready());
    Arc::get_mut(&mut port).unwrap().chat_runtime = Some(runtime);
    (directory, port, provider)
}

pub(super) fn set_provider(port: &ProductionAdkPort, socket: &std::net::TcpListener, enabled: bool) {
    port.store
        .upsert_provider(
            "provider-readiness",
            &json!({
                "baseUrl":format!("http://{}/v1", socket.local_addr().unwrap()),
                "model":"fixture-model", "apiKey":"fixture-key", "enabled":enabled
            })
            .to_string(),
        )
        .unwrap();
}

fn input() -> AdkChatInput {
    AdkChatInput {
        client_request_id: REQUEST.to_owned(),
        body: json!({"clientRequestId":REQUEST, "message":"hello", "agentId":"missing"})
            .to_string()
            .into_bytes(),
    }
}

fn terminal(body: &str, id: &str) -> Value {
    assert!(body.starts_with("retry: 3000\n\n"), "{body}");
    assert!(body.contains(&format!("id: {id}:1\n")), "{body}");
    let events = body
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .collect::<Vec<_>>();
    assert_eq!(events.len(), 1, "{body}");
    let event: Value = serde_json::from_str(events[0]).unwrap();
    assert_eq!(event["type"], "error");
    assert_eq!(event["streamId"], id);
    assert_eq!(event["sequence"], 1);
    assert_eq!(event["replay"], true);
    assert!(!event["message"].as_str().unwrap().is_empty());
    assert!(event.get("runId").is_none());
    event
}

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:55 TestChatStreamTransportHandlesDisconnectedClients
#[tokio::test]
async fn production_retained_error_survives_runtime_readiness_change_after_retry_disconnect() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = configured_port();
    let input = input();
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let sessions = port.store.list_sessions().unwrap();
    let native_sessions = port.session_store.list_sessions().unwrap();
    let id = fail_stream_write(
        port.clone(),
        "/api/v1/adk/chat/stream",
        Some(&input.body),
        b"retry: 3000",
        Some("stream-"),
    )
    .await
    .unwrap();
    set_provider(&port, &provider, true);
    assert!(
        port.runtime_ready(),
        "same installed production runtime must become ready"
    );
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let client = reqwest::Client::new();
    let replay = client
        .post(format!("{base}/api/v1/adk/chat/stream"))
        .header("content-type", "application/json")
        .body(input.body.clone())
        .send()
        .await
        .unwrap();
    assert_eq!(replay.status(), 200);
    assert_eq!(replay.headers()["x-adk-stream-id"], id);
    let event = terminal(&replay.text().await.unwrap(), &id);
    let replay = client
        .get(format!("{base}/api/v1/adk/streams/{id}"))
        .send()
        .await
        .unwrap();
    assert_eq!(replay.status(), 200);
    assert_eq!(terminal(&replay.text().await.unwrap(), &id), event);
    let after = client
        .get(format!("{base}/api/v1/adk/streams/{id}?after=1"))
        .send()
        .await
        .unwrap();
    assert_eq!(after.status(), 200);
    assert_eq!(after.text().await.unwrap(), "retry: 3000\n\n");
    let changed = client
        .post(format!("{base}/api/v1/adk/chat/stream"))
        .json(&json!({"clientRequestId":REQUEST,"message":"different"}))
        .send()
        .await
        .unwrap();
    assert_eq!(changed.status(), 409);
    assert_eq!(
        changed.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_CHAT_IDEMPOTENCY_CONFLICT"
    );
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_runs().unwrap(), runs);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.store.list_sessions().unwrap(), sessions);
    assert_eq!(port.session_store.list_sessions().unwrap(), native_sessions);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

#[tokio::test]
async fn production_retained_error_replays_with_valid_matching_durable_identity_without_run_changes()
 {
    use sha2::Digest;
    let (_directory, port, provider) = configured_port();
    let input = input();
    let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::LiveStream(first) =
        port.dispatch(AdkChatRoute::Stream, &input).unwrap()
    else {
        panic!("retained terminal stream")
    };
    let id = first.headers["X-ADK-Stream-ID"].clone();
    let fingerprint = sha2::Sha256::digest(&input.body)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    port.store
        .create_run(CreateAdkRunParams {
            id: "run-matching",
            session_id: "session-matching",
            agent_id: "agent-matching",
            status: "COMPLETED",
            client_request_id: REQUEST,
            request_fingerprint: &fingerprint,
            payload_json: r#"{"route":"stream"}"#,
        })
        .unwrap();
    let before = port.store.get_run("run-matching").unwrap().unwrap();
    set_provider(&port, &provider, true);
    assert!(port.runtime_ready());
    let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::LiveStream(replay) =
        port.dispatch(AdkChatRoute::Stream, &input).unwrap()
    else {
        panic!("same retained terminal stream")
    };
    assert_eq!(replay.headers["X-ADK-Stream-ID"], id);
    let mut body = replay.stream.take_body().unwrap();
    let mut wire = Vec::new();
    while let Some(frame) = body.next().await {
        wire.extend(frame.unwrap());
    }
    terminal(&String::from_utf8(wire).unwrap(), &id);
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run("run-matching").unwrap().unwrap(), before);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

#[tokio::test]
async fn production_retained_error_does_not_mask_durable_lookup_failure() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (directory, port, provider) = configured_port();
    let input = input();
    let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::LiveStream(first) =
        port.dispatch(AdkChatRoute::Stream, &input).unwrap()
    else {
        panic!("retained terminal stream")
    };
    let id = first.headers["X-ADK-Stream-ID"].clone();
    let audit = port.store.list_audit_events().unwrap();
    let connection = rusqlite::Connection::open(directory.path().join("adk.db")).unwrap();
    connection
        .execute("ALTER TABLE adk_runs RENAME TO unavailable_runs", [])
        .unwrap();
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let response = reqwest::Client::new()
        .post(format!(
            "http://{}/api/v1/adk/chat/stream",
            handle.startup_record().address
        ))
        .header("content-type", "application/json")
        .body(input.body)
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), 500);
    assert_eq!(
        response.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_CHAT_FAILED"
    );
    let replay = port
        .open_stream(&format!("/api/v1/adk/streams/{id}"), "")
        .unwrap()
        .unwrap();
    let mut body = replay.body.take_body().unwrap();
    let mut wire = Vec::new();
    while let Some(frame) = body.next().await {
        wire.extend(frame.unwrap());
    }
    terminal(&String::from_utf8(wire).unwrap(), &id);
    handle.shutdown().await.unwrap();
    connection
        .execute("ALTER TABLE unavailable_runs RENAME TO adk_runs", [])
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert!(port.store.list_runs().unwrap().is_empty());
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

#[tokio::test]
async fn production_retained_error_does_not_mask_durable_request_conflict_when_runtime_unready() {
    let (_directory, port, provider) = configured_port();
    let input = input();
    port.dispatch(AdkChatRoute::Stream, &input).unwrap();
    port.store
        .create_run(CreateAdkRunParams {
            id: "run-conflict",
            session_id: "session-conflict",
            agent_id: "agent-conflict",
            status: "COMPLETED",
            client_request_id: REQUEST,
            request_fingerprint: "different",
            payload_json: "{}",
        })
        .unwrap();
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    assert!(!port.runtime_ready());
    assert!(matches!(
        port.dispatch(AdkChatRoute::Stream, &input),
        Err(crate::product::product_adk_chat_stream_port::AdkChatPortError::Conflict(_))
    ));
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_runs().unwrap(), runs);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}
