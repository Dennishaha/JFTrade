use super::*;

#[path = "product_adk_terminal_recovery_contract_tests.rs"]
mod contracts;

fn frames(text: &str) -> Vec<Value> {
    text.lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .map(|line| serde_json::from_str(line).unwrap())
        .collect()
}

// Frozen Go RecoverTerminalChatResponse reads the stored run and native
// transcript even when AttachFinalAssistantMessage failed; no embedded
// response or new message append is required to publish the final frame.
// Parity: go:452dea11:internal/api/assistant/chat_stream_recovery_contracts_test.go:41 TestChatStreamExecutionReusesKnownContextAndRecoversTerminalRun
#[tokio::test]
async fn production_failed_message_append_recovers_final_without_saved_response() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (directory, port, provider) = ready_port();
    port.store.upsert_agent("agent-recover-append", &json!({
        "id":"agent-recover-append", "providerId":"provider-readiness", "model":"fixture-model"
    }).to_string()).unwrap();
    rusqlite::Connection::open(directory.path().join("adk-session.db")).unwrap()
        .execute_batch("CREATE TRIGGER break_final_append BEFORE INSERT ON events WHEN NEW.author='agent-recover-append' BEGIN SELECT RAISE(ABORT, 'final append failed'); END;").unwrap();
    let calls = Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let observed = calls.clone();
    let mock = axum::Router::new().route(
        "/v1/responses",
        axum::routing::post(move || {
            observed.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            async {
                (
                    axum::http::StatusCode::BAD_REQUEST,
                    axum::Json(json!({"error":{"message":"failed before final append"}})),
                )
            }
        }),
    );
    let socket = TcpListener::from_std(provider).unwrap();
    let (stop, stopped) = oneshot::channel();
    let server = tokio::spawn(
        axum::serve(socket, mock)
            .with_graceful_shutdown(async {
                let _ = stopped.await;
            })
            .into_future(),
    );
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let response = reqwest::Client::new().post(format!("{base}/api/v1/adk/chat/stream"))
        .json(&json!({"clientRequestId":"11111111-1111-4111-8111-111111111122", "agentId":"agent-recover-append", "message":"hello"}))
        .send().await.unwrap();
    assert_eq!(response.status(), 200);
    let id = response.headers()["x-adk-stream-id"]
        .to_str()
        .unwrap()
        .to_owned();
    let text = response.text().await.unwrap();
    let run = port.store.get_run(&id).unwrap().unwrap();
    assert_eq!(run.status, "FAILED");
    let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert!(payload.get("response").is_none());
    assert!(payload.get("finalMessageId").is_none());
    let events = port.session_store.list_events(&run.session_id).unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let replay = reqwest::Client::new().post(format!("{base}/api/v1/adk/chat/stream"))
        .json(&json!({"clientRequestId":"11111111-1111-4111-8111-111111111122", "agentId":"agent-recover-append", "message":"hello"}))
        .send().await.unwrap();
    let replay_status = replay.status();
    let replay_text = replay.text().await.unwrap();
    let reconnect = reqwest::get(format!("{base}/api/v1/adk/runs/{id}/stream"))
        .await
        .unwrap();
    let reconnect_text = reconnect.text().await.unwrap();
    // Close owners before assertions so a production red cannot leak tasks.
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    stop.send(()).unwrap();
    server.await.unwrap().unwrap();
    let events_on_wire = frames(&text);
    assert_eq!(events_on_wire.last().unwrap()["type"], "final", "{text}");
    assert!(!events_on_wire.iter().any(|event| event["type"] == "error"));
    let final_response = &events_on_wire.last().unwrap()["response"];
    assert_eq!(replay_status, 200);
    for text in [replay_text, reconnect_text] {
        let replay = frames(&text);
        assert_eq!(replay.last().unwrap()["type"], "final", "{text}");
        assert_eq!(replay.last().unwrap()["response"], *final_response);
        assert_eq!(
            replay.last().unwrap()["sequence"],
            events_on_wire.last().unwrap()["sequence"]
        );
        assert_eq!(replay.last().unwrap()["replay"], true);
        assert!(!replay.iter().any(|event| event["type"] == "error"));
    }
    assert_eq!(calls.load(std::sync::atomic::Ordering::SeqCst), 1);
    assert_eq!(final_response["run"]["id"], id);
    assert_eq!(final_response["run"]["status"], "FAILED");
    assert_eq!(final_response["session"]["id"], run.session_id);
    assert_eq!(final_response["reply"], "");
    assert_eq!(port.store.get_run(&id).unwrap().unwrap(), run);
    assert_eq!(
        port.session_store.list_events(&run.session_id).unwrap(),
        events
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

// Parity: go:452dea11:internal/api/assistant/chat_stream_recovery_contracts_test.go:41 TestChatStreamExecutionReusesKnownContextAndRecoversTerminalRun
#[tokio::test]
async fn production_reconnect_recovers_go_terminal_run_without_embedded_response() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = ready_port();
    port.store.upsert_session("session-go-recover", "agent-go-recover", &json!({
        "id":"session-go-recover", "agentId":"agent-go-recover", "title":"Go terminal recovery"
    }).to_string()).unwrap();
    port.session_store
        .upsert_session("jftrade", "local", "session-go-recover", "{}")
        .unwrap();
    let run_value = json!({"id":"run-go-recover", "sessionId":"session-go-recover",
        "agentId":"agent-go-recover", "status":"COMPLETED"});
    let payload = json!({"id":"run-go-recover", "sessionId":"session-go-recover",
        "agentId":"agent-go-recover", "status":"COMPLETED", "streamId":"stream-go-recover",
        "streamEvents":[{"type":"run", "run":run_value, "sequence":1}]});
    port.store
        .create_run(CreateAdkRunParams {
            id: "run-go-recover",
            session_id: "session-go-recover",
            agent_id: "agent-go-recover",
            status: "COMPLETED",
            client_request_id: "request-go-recover",
            request_fingerprint: "go-recover",
            payload_json: &payload.to_string(),
        })
        .unwrap();
    let run = port.store.get_run("run-go-recover").unwrap().unwrap();
    let events = port
        .session_store
        .list_events("session-go-recover")
        .unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let prepared = prepared_router(port.clone()).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let mut delivered = Vec::new();
    for path in [
        "/api/v1/adk/streams/stream-go-recover",
        "/api/v1/adk/runs/run-go-recover/stream",
    ] {
        let response = reqwest::get(format!("{base}{path}")).await.unwrap();
        assert_eq!(response.status(), 200);
        assert_eq!(response.headers()["x-adk-stream-id"], "stream-go-recover");
        delivered.push(response.text().await.unwrap());
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    for text in delivered {
        let events_on_wire = frames(&text);
        assert_eq!(events_on_wire.len(), 2, "{text}");
        assert_eq!(events_on_wire[0]["type"], "run");
        assert_eq!(events_on_wire[1]["type"], "final");
        assert_eq!(events_on_wire[1]["response"]["run"]["id"], "run-go-recover");
        assert_eq!(
            events_on_wire[1]["response"]["session"]["id"],
            "session-go-recover"
        );
        assert_eq!(events_on_wire[1]["response"]["reply"], "");
        assert!(!events_on_wire.iter().any(|event| event["type"] == "error"));
    }
    assert_eq!(port.store.get_run("run-go-recover").unwrap().unwrap(), run);
    assert_eq!(
        port.session_store
            .list_events("session-go-recover")
            .unwrap(),
        events
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}
