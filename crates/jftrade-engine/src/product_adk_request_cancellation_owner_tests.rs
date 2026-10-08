use super::*;
use jftrade_api::RequestCancellation;

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:110 TestChatStreamReconnectAndReplayRespectClientDisconnect
#[tokio::test]
async fn production_reconnect_precancelled_request_returns_zero_body_with_listener_still_active() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", Vec::new());
    let run = port.store.get_run("run-cursor").unwrap().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let native = port.session_store.list_sessions().unwrap();
    let references = Arc::strong_count(&port.store);
    let cancellation = RequestCancellation::default();
    cancellation.cancel();
    let mut prepared = prepared_router(port.clone()).await;
    prepared.router = prepared.router.layer(axum::Extension(cancellation.clone()));
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let mut results = Vec::new();
    for path in [
        "/api/v1/adk/streams/stream-cursor",
        "/api/v1/adk/runs/run-cursor/stream",
    ] {
        let response = reqwest::get(format!("{base}{path}")).await.unwrap();
        let status = response.status();
        let id = response.headers()["x-adk-stream-id"]
            .to_str()
            .unwrap()
            .to_owned();
        let body = tokio::time::timeout(Duration::from_millis(500), response.text()).await;
        results.push((status, id, body));
    }
    let health = reqwest::get(format!("{base}/api/v1/adk/runs/run-cursor"))
        .await
        .unwrap()
        .status();
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert!(cancellation.is_cancelled());
    assert_eq!(
        health, 200,
        "per-request cancellation must leave the listener active"
    );
    for (status, id, body) in results {
        assert_eq!(status, 200);
        assert_eq!(id, "stream-cursor");
        assert_eq!(
            body.expect("pre-cancelled request must end immediately")
                .unwrap(),
            ""
        );
    }
    assert_eq!(port.store.get_run("run-cursor").unwrap().unwrap(), run);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.session_store.list_sessions().unwrap(), native);
    assert_eq!(
        Arc::strong_count(&port.store),
        references,
        "request reader releases its store owner"
    );
}

#[tokio::test]
async fn production_reconnect_request_cancel_drops_only_its_reader_and_keeps_sibling_live() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", Vec::new());
    let run = port.store.get_run("run-cursor").unwrap().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let native = port.session_store.list_sessions().unwrap();
    let references = Arc::strong_count(&port.store);
    let cancellation = RequestCancellation::default();
    let injected = cancellation.clone();
    let mut prepared = prepared_router(port.clone()).await;
    prepared.router = prepared.router.layer(axum::middleware::from_fn(
        move |mut request: axum::extract::Request, next: axum::middleware::Next| {
            let injected = injected.clone();
            async move {
                if request.uri().path().contains("/streams/") {
                    request.extensions_mut().insert(injected);
                }
                next.run(request).await
            }
        },
    ));
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let response = reqwest::get(format!("{base}/api/v1/adk/streams/stream-cursor"))
        .await
        .unwrap();
    let sibling = reqwest::get(format!("{base}/api/v1/adk/runs/run-cursor/stream"))
        .await
        .unwrap();
    assert_eq!(response.status(), 200);
    assert_eq!(sibling.status(), 200);
    let mut body = response.bytes_stream();
    let mut sibling = sibling.bytes_stream();
    assert_eq!(
        body.next().await.unwrap().unwrap(),
        b"retry: 3000\n\n".as_slice()
    );
    assert_eq!(
        sibling.next().await.unwrap().unwrap(),
        b"retry: 3000\n\n".as_slice()
    );
    cancellation.cancel();
    let ended = tokio::time::timeout(Duration::from_secs(1), body.next()).await;
    let after_cancel = port.store.get_run("run-cursor").unwrap().unwrap();
    let audit_after_cancel = port.store.list_audit_events().unwrap();
    let mut payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    payload["streamEvents"] =
        json!([{"type":"final","sequence":1,"response":{"reply":"sibling still live"}}]);
    port.store
        .update_run_state("run-cursor", "COMPLETED", &payload.to_string())
        .unwrap();
    let sibling_frame = tokio::time::timeout(Duration::from_secs(1), sibling.next()).await;
    let sibling_end = tokio::time::timeout(Duration::from_secs(1), sibling.next()).await;
    drop(body);
    drop(sibling);
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert!(
        ended
            .expect("request cancellation wakes the idle body")
            .is_none()
    );
    assert_eq!(after_cancel, run);
    assert_eq!(audit_after_cancel, audit);
    let frame = String::from_utf8(sibling_frame.unwrap().unwrap().unwrap().to_vec()).unwrap();
    let frame: Value = serde_json::from_str(
        frame
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    assert_eq!(frame["type"], "final");
    assert_eq!(frame["sequence"], 1);
    assert_eq!(frame["response"]["reply"], "sibling still live");
    assert_eq!(frame["replay"], false);
    assert!(sibling_end.unwrap().is_none());
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.session_store.list_sessions().unwrap(), native);
    assert_eq!(Arc::strong_count(&port.store), references);
}
