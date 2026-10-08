use super::*;

#[path = "product_adk_terminal_projection_recovery_owner_tests.rs"]
mod projection_recovery;

#[test]
fn production_created_sync_context_failure_returns_stored_failed_projection_on_replay() {
    let (_directory, port, provider, input) = context_failure_port();
    let output = port.dispatch(AdkChatRoute::Chat, &input).unwrap();
    let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::Json(response) = output
    else {
        panic!("failed projection")
    };
    let run = port
        .store
        .get_run_by_client_request_id(&input.client_request_id)
        .unwrap()
        .unwrap();
    assert_eq!(run.status, "FAILED");
    let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(response, payload["response"]);
    assert_eq!(response["run"]["status"], "FAILED");
    assert!(response["reply"].as_str().unwrap().contains("context"));
    let events = port.session_store.list_events(&run.session_id).unwrap();
    assert_eq!(
        events.iter().filter(|event| event.author == "user").count(),
        1
    );
    assert_eq!(
        events
            .iter()
            .filter(|event| Some(event.id.as_str()) == payload["finalMessageId"].as_str())
            .count(),
        1
    );
    let audit = port.store.list_audit_events().unwrap();
    let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::Json(replay) =
        port.dispatch(AdkChatRoute::Chat, &input).unwrap()
    else {
        panic!("same projection")
    };
    assert_eq!(replay, response);
    assert_eq!(port.unavailable_streams.retained_request_count(), 0);
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run(&run.id).unwrap().unwrap(), run);
    assert_eq!(
        port.session_store.list_events(&run.session_id).unwrap(),
        events
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

#[tokio::test]
async fn production_created_stream_preparation_failure_retains_terminal_after_retry_disconnect() {
    let (_directory, port, provider, input) = context_failure_port();
    let id = fail_stream_write(
        port.clone(),
        "/api/v1/adk/chat/stream",
        Some(&input.body),
        b"retry: 3000",
        Some("run-"),
    )
    .await
    .unwrap();
    let run = port
        .store
        .get_run_by_client_request_id(&input.client_request_id)
        .unwrap()
        .unwrap();
    assert_eq!(id, run.id);
    assert_eq!(run.status, "FAILED");
    let stored: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert!(
        stored["failureReason"]
            .as_str()
            .unwrap()
            .contains("context"),
        "{stored}"
    );
    assert_eq!(stored["streamEvents"].as_array().unwrap().len(), 1);
    assert_eq!(stored["streamEvents"][0]["type"], "final");
    assert_eq!(stored["streamEvents"][0]["response"], stored["response"]);
    let events = port.session_store.list_events(&run.session_id).unwrap();
    let audit = port.store.list_audit_events().unwrap();
    for path in [
        format!("/api/v1/adk/streams/{id}"),
        format!("/api/v1/adk/runs/{id}/stream"),
    ] {
        let replay = port.open_stream(&path, "").unwrap().unwrap();
        let mut reader = replay.body.take_body().unwrap();
        assert_eq!(reader.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
        let frame = String::from_utf8(reader.next().await.unwrap().unwrap()).unwrap();
        let event: Value = serde_json::from_str(
            frame
                .lines()
                .find_map(|line| line.strip_prefix("data: "))
                .unwrap(),
        )
        .unwrap();
        assert_eq!(event["type"], "final");
        assert_eq!(event["streamId"], id);
        assert_eq!(event["sequence"], 1);
        assert_eq!(event["replay"], true);
        assert_eq!(event["response"], stored["response"]);
        assert!(reader.next().await.is_none());
    }
    assert_eq!(port.unavailable_streams.retained_request_count(), 0);
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run(&run.id).unwrap().unwrap(), run);
    assert_eq!(
        port.session_store.list_events(&run.session_id).unwrap(),
        events
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

// Parity: go:452dea11:internal/api/assistant/chat_stream_recovery_contracts_test.go:41 TestChatStreamExecutionReusesKnownContextAndRecoversTerminalRun
#[tokio::test]
async fn production_live_failure_replays_final_projection_without_provisional_error() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = ready_port();
    port.store.upsert_agent("agent-failure-final",&json!({"id":"agent-failure-final","providerId":"provider-readiness","model":"fixture-model"}).to_string()).unwrap();
    let mock = axum::Router::new().route(
        "/v1/responses",
        axum::routing::post(|| async {
            (
                axum::http::StatusCode::BAD_REQUEST,
                axum::Json(json!({"error":{"message":"provider failed turn"}})),
            )
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
    let url = format!(
        "http://{}/api/v1/adk/chat/stream",
        handle.startup_record().address
    );
    let response=reqwest::Client::new().post(&url).json(&json!({"clientRequestId":"11111111-1111-4111-8111-111111111121","agentId":"agent-failure-final","message":"hello"})).send().await.unwrap();
    assert_eq!(response.status(), 200);
    let id = response.headers()["x-adk-stream-id"]
        .to_str()
        .unwrap()
        .to_owned();
    let text = response.text().await.unwrap();
    let frames = text
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .map(|line| serde_json::from_str::<Value>(line).unwrap())
        .collect::<Vec<_>>();
    assert_eq!(frames.last().unwrap()["type"], "final");
    assert!(!frames.iter().any(|event| event["type"] == "error"));
    let run = port.store.get_run(&id).unwrap().unwrap();
    assert_eq!(run.status, "FAILED");
    let stored: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(frames.last().unwrap()["response"], stored["response"]);
    assert_eq!(stored["response"]["run"]["status"], "FAILED");
    assert_eq!(
        stored["response"]["run"]["failureReason"],
        "provider failed turn"
    );
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    stop.send(()).unwrap();
    server.await.unwrap().unwrap();
}
