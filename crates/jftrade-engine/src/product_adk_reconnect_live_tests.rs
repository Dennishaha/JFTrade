use super::*;
use crate::product::product_production_ports::ProductionAdkPort;
use jftrade_store_sqlite::{AdkArtifactStore, CreateAdkRunParams};
use tokio_stream::StreamExt;

fn reconnect_port() -> (tempfile::TempDir, Arc<ProductionAdkPort>) {
    let (directory, store, sessions) = initialized_stores();
    let artifact = directory.path().join("adk-artifact.db");
    File::create(&artifact).unwrap();
    initialize_current(&Connection::open(&artifact).unwrap(), "adk-artifact").unwrap();
    let settings = directory.path().join("settings.json");
    fs::write(&settings, "{}").unwrap();
    let port = ProductionAdkPort::new_for_test(
        store,
        sessions,
        Arc::new(AdkArtifactStore::open(&artifact).unwrap()),
        settings,
    );
    (directory, Arc::new(port))
}

fn seed_reconnect(port: &ProductionAdkPort, status: &str, events: Vec<Value>) {
    port.store
        .create_run(CreateAdkRunParams {
            id: "run-cursor",
            session_id: "session-cursor",
            agent_id: "agent-cursor",
            status,
            client_request_id: "request-cursor",
            request_fingerprint: "cursor",
            payload_json: &json!({"streamId":"stream-cursor", "streamEvents":events}).to_string(),
        })
        .unwrap();
}

#[tokio::test]
async fn reconnect_cursor_pages_history_without_duplicates_and_returns_canonical_stream_id() {
    let (_directory, port) = reconnect_port();
    let mut events: Vec<Value> = (1..=136)
        .map(|seq| json!({"type":"timeline", "sequence":seq}))
        .collect();
    events.push(json!({"type":"final"})); // Legacy event without sequence uses its array ordinal.
    seed_reconnect(&port, "COMPLETED", events);
    let cursor = port
        .store
        .open_stream_cursor("stream-cursor")
        .unwrap()
        .unwrap();
    assert_eq!(cursor.run_id, "run-cursor");
    assert_eq!(cursor.replay_until, 137);
    let first = port
        .store
        .read_stream_page(&cursor.run_id, 7, 10_000)
        .unwrap();
    assert_eq!(
        first.events.len(),
        64,
        "store bounds decoded events even for a large limit"
    );
    assert_eq!(first.events.first().unwrap().0, 8);
    assert_eq!(first.events.last().unwrap().0, 71);
    let second = port.store.read_stream_page(&cursor.run_id, 71, 64).unwrap();
    assert_eq!(second.events.len(), 64);
    assert_eq!(second.events.first().unwrap().0, 72);
    assert_eq!(second.events.last().unwrap().0, 135);
    let third = port
        .store
        .read_stream_page(&cursor.run_id, 135, 64)
        .unwrap();
    assert_eq!(
        third
            .events
            .iter()
            .map(|(sequence, _)| *sequence)
            .collect::<Vec<_>>(),
        vec![136, 137]
    );
    for path in [
        "/api/v1/adk/runs/run-cursor/stream",
        "/api/v1/adk/streams/stream-cursor",
    ] {
        let crate::product::AdkReadOutput::LiveStream(stream) =
            crate::product::dispatch_adk_read(Some(port.as_ref()), "GET", path, "after=7").unwrap()
        else {
            panic!("production reconnect must use the live reader")
        };
        assert_eq!(
            stream.headers,
            vec![("X-ADK-Stream-ID".to_owned(), "stream-cursor".to_owned())]
        );
        let mut body = stream.body.take_body().unwrap();
        assert_eq!(body.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
        let mut ids = Vec::new();
        while let Some(frame) = tokio::time::timeout(Duration::from_secs(3), body.next())
            .await
            .unwrap()
        {
            let frame = String::from_utf8(frame.unwrap()).unwrap();
            ids.push(
                frame
                    .lines()
                    .find_map(|line| line.strip_prefix("id: "))
                    .unwrap()
                    .parse::<u64>()
                    .unwrap(),
            );
            let value: Value = serde_json::from_str(
                frame
                    .lines()
                    .find_map(|line| line.strip_prefix("data: "))
                    .unwrap(),
            )
            .unwrap();
            assert_eq!(value["replay"], true);
        }
        assert_eq!(
            ids,
            (8..=137).collect::<Vec<_>>(),
            "cross-page ordering and no duplicates or omissions"
        );
    }
}

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:110 TestChatStreamReconnectAndReplayRespectClientDisconnect
#[tokio::test]
async fn reconnect_reader_drop_releases_idle_store_reference_without_background_reads() {
    let (_directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", Vec::new());
    let baseline = Arc::strong_count(&port.store);
    let stream = crate::product::AdkReadSnapshotPort::open_stream(
        port.as_ref(),
        "/api/v1/adk/streams/stream-cursor",
        "",
    )
    .unwrap()
    .unwrap();
    let mut body = stream.body.take_body().unwrap();
    assert_eq!(Arc::strong_count(&port.store), baseline + 1);
    assert_eq!(body.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
    assert!(
        tokio::time::timeout(Duration::from_millis(20), body.next())
            .await
            .is_err(),
        "idle body waits without fabricating events"
    );
    drop(body);
    assert_eq!(
        Arc::strong_count(&port.store),
        baseline,
        "dropping an idle HTTP body releases its sole reader owner"
    );
    assert_eq!(
        port.store.get_run("run-cursor").unwrap().unwrap().status,
        "RUNNING",
        "reader never writes lifecycle state"
    );
}

#[tokio::test]
async fn reconnect_store_failure_ends_body_after_one_error() {
    let (directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", Vec::new());
    let stream = crate::product::AdkReadSnapshotPort::open_stream(
        port.as_ref(),
        "/api/v1/adk/runs/run-cursor/stream",
        "",
    )
    .unwrap()
    .unwrap();
    let mut body = stream.body.take_body().unwrap();
    assert_eq!(body.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
    Connection::open(directory.path().join("adk.db"))
        .unwrap()
        .execute("DROP TABLE adk_runs", [])
        .unwrap();
    assert!(body.next().await.unwrap().is_err());
    assert!(
        body.next().await.is_none(),
        "failed reader must not retry forever"
    );
}

#[tokio::test]
async fn reconnect_http_shutdown_closes_idle_connection_without_mutating_the_run() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", Vec::new());
    let config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        &port.settings_path,
    )
    .unwrap()
    .with_adk_read_snapshot_port(port.clone());
    let handle = crate::product::start_product(config).await.unwrap();
    let response = reqwest::Client::new()
        .get(format!(
            "http://{}/api/v1/adk/runs/run-cursor/stream",
            handle.startup_record().address
        ))
        .send()
        .await
        .unwrap();
    assert_eq!(response.headers()["x-adk-stream-id"], "stream-cursor");
    let mut body = response.bytes_stream();
    assert_eq!(
        tokio::time::timeout(Duration::from_secs(3), body.next())
            .await
            .unwrap()
            .unwrap()
            .unwrap()
            .as_ref(),
        b"retry: 3000\n\n"
    );
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    let next = tokio::time::timeout(Duration::from_secs(3), body.next())
        .await
        .unwrap();
    assert!(
        next.is_none() || next.is_some_and(|result| result.is_err()),
        "listener shutdown closes the idle SSE body"
    );
    assert_eq!(
        port.store.get_run("run-cursor").unwrap().unwrap().status,
        "RUNNING"
    );
}

// Parity: go:452dea11:internal/api/assistant/routes_test.go:136 TestChatStreamHubReplayAndCleanupBoundaries
#[tokio::test]
async fn reconnect_http_replays_history_then_tails_live_events_on_the_same_connection() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    let first = json!({"type":"run", "sequence":1, "runId":"run-live", "streamId":"run-live"});
    port.store
        .create_run(CreateAdkRunParams {
            id: "run-live",
            session_id: "session-live",
            agent_id: "agent-live",
            status: "RUNNING",
            client_request_id: "request-live",
            request_fingerprint: "live",
            payload_json: &json!({"streamId":"run-live",
            "streamEvents":[first.clone()]})
            .to_string(),
        })
        .unwrap();
    let config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        &port.settings_path,
    )
    .unwrap()
    .with_adk_read_snapshot_port(port.clone());
    let handle = crate::product::start_product(config).await.unwrap();
    let response = reqwest::Client::new()
        .get(format!(
            "http://{}/api/v1/adk/runs/run-live/stream",
            handle.startup_record().address
        ))
        .send()
        .await
        .unwrap();
    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(response.headers()["x-adk-stream-id"], "run-live");
    let mut body = response.bytes_stream();
    let mut received = String::new();
    while !received.contains("data:") {
        let chunk = tokio::time::timeout(Duration::from_secs(3), body.next())
            .await
            .unwrap()
            .unwrap()
            .unwrap();
        received.push_str(std::str::from_utf8(&chunk).unwrap());
    }
    let second = json!({"type":"timeline", "sequence":2, "runId":"run-live", "streamId":"run-live",
        "timeline":{"id":"live-after-reconnect"}});
    let final_event =
        json!({"type":"final", "sequence":3, "runId":"run-live", "streamId":"run-live"});
    port.store
        .update_run_state(
            "run-live",
            "COMPLETED",
            &json!({"streamId":"run-live",
        "streamEvents":[first, second, final_event]})
            .to_string(),
        )
        .unwrap();
    let deadline = tokio::time::Instant::now() + Duration::from_secs(3);
    while let Ok(Some(Ok(chunk))) = tokio::time::timeout_at(deadline, body.next()).await {
        received.push_str(std::str::from_utf8(&chunk).unwrap());
    }
    drop(body);
    handle.shutdown().await.unwrap();
    let events: Vec<Value> = received
        .lines()
        .filter_map(|line| line.strip_prefix("data: "))
        .map(|line| serde_json::from_str(line).unwrap())
        .collect();
    assert_eq!(
        events.len(),
        3,
        "same connection must deliver newly persisted events: {received}"
    );
    assert_eq!(
        events
            .iter()
            .map(|event| event["sequence"].as_u64().unwrap())
            .collect::<Vec<_>>(),
        vec![1, 2, 3]
    );
    assert_eq!(events[0]["replay"], true);
    assert_eq!(events[1]["replay"], false);
    assert_eq!(events[2]["replay"], false);
}
