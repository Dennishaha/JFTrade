//! Authentication and maintenance failures through concrete production owners.
use super::*;

fn client() -> reqwest::Client {
    // This loopback client also constructs TLS configuration. Initialize it
    // here so the fixture does not depend on another runtime adapter doing so.
    let _ = rustls::crypto::ring::default_provider().install_default();
    reqwest::Client::builder()
        .no_proxy()
        .timeout(Duration::from_secs(5))
        .redirect(reqwest::redirect::Policy::none())
        .build()
        .expect("loopback client")
}

async fn removed(handle: &ProductHandle, method: &str, path: &str) {
    let response = client()
        .request(
            method.parse().expect("method"),
            format!("http://{}{path}", handle.startup_record().address),
        )
        .bearer_auth("a".repeat(32))
        .send()
        .await
        .expect("removed route");
    assert_eq!(response.status().as_u16(), 404, "{method} {path}");
}

// Parity: go:452dea11:internal/api/settings/routes_accounts_validation_test.go:73 TestDataManagementRebuildRejectsMalformedAndRejectedRequests
#[tokio::test]
async fn production_http_rebuild_rejects_original_malformed_and_wrong_confirmation_without_marker()
{
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory, json!({})))
        .await
        .expect("production");
    let path = "/api/v1/settings/data-management/databases/rebuild";
    let before = std::fs::read(directory.path().join("settings.json")).expect("settings");
    for (body, code) in [
        (r#"{"mode":"#, "BAD_REQUEST"),
        (
            r#"{"mode":"single","databaseId":"adk","confirmation":"wrong"}"#,
            "DATABASE_REBUILD_REJECTED",
        ),
    ] {
        let (status, response) = request(&handle, "POST", path, Some(body)).await;
        assert_eq!(status, 400, "{response}");
        assert_eq!(response["ok"], false);
        assert_eq!(response["error"]["code"], code);
        assert!(
            response["timestamp"]
                .as_str()
                .is_some_and(|time| !time.is_empty())
        );
        assert!(!directory.path().join("database-rebuild.json").exists());
        assert_eq!(
            std::fs::read(directory.path().join("settings.json")).expect("settings"),
            before
        );
    }
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:147 TestDataManagementStatusMapsCallbackFailure
#[tokio::test]
async fn production_http_database_status_maps_overview_owner_failure_and_recovers() {
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory, json!({})))
        .await
        .expect("production");
    let marker = directory.path().join("database-rebuild.json");
    std::fs::write(&marker, b"{").expect("broken overview marker");
    let (status, response) = request(
        &handle,
        "GET",
        "/api/v1/settings/data-management/databases",
        None,
    )
    .await;
    assert_eq!(status, 500, "{response}");
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "DATABASE_STATUS_FAILED");
    assert!(
        response["error"]["message"]
            .as_str()
            .expect("message")
            .contains("rebuild marker")
    );
    assert_eq!(std::fs::read(&marker).expect("marker preserved"), b"{");
    std::fs::remove_file(marker).expect("restore fixture");
    let (status, response) = request(
        &handle,
        "GET",
        "/api/v1/settings/data-management/databases",
        None,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert!(
        response["data"]["databases"]
            .as_array()
            .expect("databases")
            .iter()
            .any(|entry| entry["id"] == "adk")
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/settings/routes_test.go:247 TestSettingsRoutesPreserveLegacyResponseShapes
#[tokio::test]
async fn production_http_settings_preserve_original_integration_delete_and_removed_route_shapes() {
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory, seed()))
        .await
        .expect("production");
    removed(&handle, "GET", "/api/v1/settings/runtime-dependencies").await;
    let (status, integration) = request(
        &handle,
        "PUT",
        "/api/v1/settings/brokers/futu/integration",
        Some(r#"{"enabled":true}"#),
    )
    .await;
    assert_eq!(status, 200, "{integration}");
    assert_eq!(integration["ok"], true);
    assert_eq!(integration["data"]["brokerId"], "futu");
    assert_eq!(integration["data"]["enabled"], true);
    assert!(
        integration["data"].get("integration").is_none(),
        "direct integration object"
    );
    let (status, deleted) = request(
        &handle,
        "DELETE",
        "/api/v1/settings/broker-accounts/account-1",
        None,
    )
    .await;
    assert_eq!(status, 200, "{deleted}");
    assert_eq!(deleted["ok"], true);
    assert_eq!(deleted["data"]["deleted"], true);
    assert_eq!(deleted["data"]["id"], "account-1");
    assert_eq!(
        document(directory.path())["accounts"]
            .as_array()
            .expect("accounts")
            .len(),
        1
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/settings/routes_test.go:754 TestDataManagementRoutesUseInjectedCallbacks
#[tokio::test]
async fn production_http_rebuild_original_request_respects_live_writer_owner_and_removed_routes() {
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory, json!({})))
        .await
        .expect("production");
    let (status, overview) = request(
        &handle,
        "GET",
        "/api/v1/settings/data-management/databases",
        None,
    )
    .await;
    assert_eq!(status, 200, "{overview}");
    assert!(
        overview["data"]["databases"]
            .as_array()
            .expect("databases")
            .iter()
            .any(|entry| entry["id"] == "adk")
    );
    let (status, response) = request(
        &handle,
        "POST",
        "/api/v1/settings/data-management/databases/rebuild",
        Some(r#"{"mode":"single","databaseId":"adk","confirmation":"REBUILD adk"}"#),
    )
    .await;
    // The original callback allows scheduling; concrete production ownership
    // rejects a second writer while the ADK runtime is alive.
    assert_eq!(status, 409, "{response}");
    assert_eq!(response["error"]["code"], "DATABASE_MAINTENANCE_CONFLICT");
    assert!(!directory.path().join("database-rebuild.json").exists());
    removed(&handle, "GET", "/api/v1/settings/data-migration/databases").await;
    removed(
        &handle,
        "POST",
        "/api/v1/settings/data-migration/databases/rebuild",
    )
    .await;
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/settings/routes_test.go:304 TestMCPServerSettingsRoutesReturnTokenOnlyOnce
#[tokio::test]
async fn production_http_mcp_original_token_flow_hides_secrets_and_rotation_revokes_old_token() {
    let directory = tempdir().expect("directory");
    let handle = start_product(config(
        &directory,
        json!({"mcpServer":{"enabled":false,"port":6697,"authMode":"token","tokenHash":""}}),
    ))
    .await
    .expect("production");
    let path = "/api/v1/settings/adk/mcp";
    let (status, initial) = request(&handle, "GET", path, None).await;
    assert_eq!(status, 200, "{initial}");
    assert_eq!(initial["data"]["settings"]["port"], 6697);
    assert!(!initial.to_string().contains("tokenHash"));
    let settings_file = directory.path().join("settings.json");
    let before = std::fs::read(&settings_file).expect("settings before rejected enable");
    let body = r#"{"enabled":true,"port":6697,"authMode":"token"}"#;
    let (status, rejected) = request(&handle, "PUT", path, Some(body)).await;
    assert_eq!(status, 400, "{rejected}");
    assert_eq!(rejected["error"]["code"], "MCP_SERVER_SETTINGS_REJECTED");
    assert_eq!(
        std::fs::read(&settings_file).expect("settings after rejected enable"),
        before
    );
    let (status, reset) = request(
        &handle,
        "POST",
        "/api/v1/settings/adk/mcp/token/reset",
        None,
    )
    .await;
    assert_eq!(status, 200, "{reset}");
    assert_eq!(reset["ok"], true);
    assert_eq!(reset["data"]["settings"]["tokenConfigured"], true);
    assert!(!reset.to_string().contains("tokenHash"));
    let token = reset["data"]["token"].as_str().expect("one-time token");
    assert!(token.starts_with("jft_mcp_"));
    let (status, saved) = request(&handle, "PUT", path, Some(body)).await;
    assert_eq!(status, 200, "{saved}");
    assert_eq!(saved["data"]["settings"]["enabled"], true);
    assert!(!saved.to_string().contains(token));
    let (status, readback) = request(&handle, "GET", path, None).await;
    assert_eq!(status, 200, "{readback}");
    assert!(!readback.to_string().contains(token));
    assert!(!readback.to_string().contains("tokenHash"));
    assert!(
        !std::fs::read_to_string(directory.path().join("settings.json"))
            .expect("settings")
            .contains(token)
    );
    let probe = |secret: String| async move {
        client()
            .get("http://127.0.0.1:6697/mcp")
            .bearer_auth(secret)
            .send()
            .await
            .expect("MCP auth probe")
            .status()
            .as_u16()
    };
    assert_eq!(
        probe(token.to_owned()).await,
        405,
        "valid bearer reaches method validation"
    );
    let (status, rotated) = request(
        &handle,
        "POST",
        "/api/v1/settings/adk/mcp/token/reset",
        None,
    )
    .await;
    assert_eq!(status, 200, "{rotated}");
    let successor = rotated["data"]["token"].as_str().expect("successor token");
    assert_ne!(token, successor);
    assert_eq!(probe(token.to_owned()).await, 401, "old bearer revoked");
    assert_eq!(probe(successor.to_owned()).await, 405, "new bearer valid");
    let (status, readback) = request(&handle, "GET", path, None).await;
    assert_eq!(status, 200, "{readback}");
    assert!(!readback.to_string().contains(successor));
    assert!(!readback.to_string().contains("tokenHash"));
    let persisted = std::fs::read_to_string(settings_file).expect("rotated settings");
    assert!(!persisted.contains(token));
    assert!(!persisted.contains(successor));
    let (status, disabled) = request(
        &handle,
        "PUT",
        path,
        Some(r#"{"enabled":false,"port":6697,"authMode":"token"}"#),
    )
    .await;
    assert_eq!(status, 200, "{disabled}");
    handle.shutdown().await.expect("shutdown");
}
