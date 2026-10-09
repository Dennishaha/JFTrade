use super::*;

async fn reconnect_statuses(port: Arc<ProductionAdkPort>) -> Vec<u16> {
    let run = port.store.get_run("run-cursor").unwrap().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let sessions = port.store.list_sessions().unwrap();
    let native = port.session_store.list_sessions().unwrap();
    let metadata = port
        .store
        .read_stream_run_metadata("run-cursor")
        .unwrap()
        .unwrap();
    let metadata: Value = serde_json::from_str(&metadata.payload_json).unwrap();
    assert!(metadata.get("streamEvents").is_none());
    assert!(metadata.get("providerEvents").is_none());
    let prepared = prepared_router(port.clone()).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let client = reqwest::Client::new();
    let mut statuses = Vec::new();
    for path in [
        "/api/v1/adk/streams/stream-cursor",
        "/api/v1/adk/runs/run-cursor/stream",
    ] {
        let response = tokio::time::timeout(
            Duration::from_secs(3),
            client.get(format!("{base}{path}")).send(),
        )
        .await
        .unwrap()
        .unwrap();
        statuses.push(response.status().as_u16());
        drop(response);
    }
    tokio::time::timeout(Duration::from_secs(3), handle.shutdown())
        .await
        .unwrap()
        .unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run("run-cursor").unwrap().unwrap(), run);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.store.list_sessions().unwrap(), sessions);
    assert_eq!(port.session_store.list_sessions().unwrap(), native);
    statuses
}

// Parity: go:452dea11:internal/api/assistant/routes_test.go:136 TestChatStreamHubReplayAndCleanupBoundaries
#[tokio::test]
async fn production_reconnect_hides_expired_running_stream_without_mutating_the_run() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", vec![json!({"type":"run", "sequence":1})]);
    let mut payload: Value = serde_json::from_str(
        &port
            .store
            .get_run("run-cursor")
            .unwrap()
            .unwrap()
            .payload_json,
    )
    .unwrap();
    payload["startedAt"] = json!("2000-01-01T00:00:00Z");
    payload["maxDurationMs"] = json!(1_800_000);
    port.store
        .update_run_state("run-cursor", "RUNNING", &payload.to_string())
        .unwrap();
    let statuses = reconnect_statuses(port).await;
    assert_eq!(
        statuses,
        vec![404, 404],
        "expired retention is a read visibility decision"
    );
}

#[tokio::test]
async fn production_reconnect_retention_uses_saved_duration_and_preserves_failed_lookup_semantics()
{
    let _ = rustls::crypto::ring::default_provider().install_default();
    let recent = time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap();
    let hour_ago = (time::OffsetDateTime::now_utc() - time::Duration::minutes(90))
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap();
    let cases = [
        (
            "RUNNING",
            json!({"startedAt":recent, "maxDurationMs":1}),
            200,
        ),
        (
            "RUNNING",
            json!({"startedAt":hour_ago, "maxDurationMs":7_200_000}),
            200,
        ),
        (
            "RUNNING",
            json!({"startedAt":hour_ago, "maxDurationMs":0}),
            404,
        ),
        (
            "RUNNING",
            json!({"startedAt":hour_ago, "maxDurationMs":-1}),
            404,
        ),
        (
            "PENDING_INPUT",
            json!({"startedAt":"bad", "createdAt":hour_ago}),
            404,
        ),
        (
            "RUNNING",
            json!({"startedAt":"bad", "createdAt":"bad"}),
            200,
        ),
        (
            "RUNNING",
            json!({"startedAt":hour_ago, "maxDurationMs":"invalid"}),
            200,
        ),
        (
            "COMPLETED",
            json!({"startedAt":"2000-01-01T00:00:00Z"}),
            200,
        ),
    ];
    for (status, mut payload, expected) in cases {
        let (_directory, port) = reconnect_port();
        seed_reconnect(&port, status, vec![json!({"type":"run", "sequence":1})]);
        payload["streamId"] = json!("stream-cursor");
        payload["streamEvents"] = json!([{"type":"run", "sequence":1}]);
        port.store
            .update_run_state("run-cursor", status, &payload.to_string())
            .unwrap();
        assert_eq!(
            reconnect_statuses(port).await,
            vec![expected, expected],
            "{status}: {payload}"
        );
    }
}
