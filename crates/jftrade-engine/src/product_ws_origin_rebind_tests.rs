//! Current-listener Origin grants through production HTTP and WebSocket.

use super::*;

struct ReboundWeb {
    handle: ProductHandle,
    _directory: tempfile::TempDir,
    client: reqwest::Client,
    retired: std::net::SocketAddr,
    current: std::net::SocketAddr,
    cookie: String,
}

async fn reconfigure(handle: &ProductHandle, port: u16) -> (u16, serde_json::Value) {
    request_json_with_status(
        handle.startup_record().address,
        "PUT",
        "/api/v1/settings/security",
        Some(&serde_json::json!({"webAccessEnabled":true,"webPort":port}).to_string()),
        AUTH,
    )
    .await
}

async fn rebound_web() -> ReboundWeb {
    let directory = tempdir().expect("directory");
    let (config, retired) = protected_product_config(&directory.path().join("settings.json"));
    let handle = start_product(config).await.expect("production");
    let reservation = std::net::TcpListener::bind("127.0.0.1:0").expect("reserve new port");
    let current = reservation.local_addr().expect("new address");
    assert_ne!(current, retired);
    drop(reservation);
    let (status, value) = reconfigure(&handle, current.port()).await;
    assert_eq!(status, 200, "{value}");
    assert_eq!(value["data"]["webPort"], current.port());
    assert!(
        std::net::TcpListener::bind(retired).is_ok(),
        "old listener released"
    );
    let client = web_client();
    // Settings updates revoke existing sessions; use a fresh current-port cookie.
    let (cookie, _) = web_login(&client, current, "original browser password").await;
    ReboundWeb {
        handle,
        _directory: directory,
        client,
        retired,
        current,
        cookie,
    }
}

async fn assert_login_denied(web: &ReboundWeb, origin: &str) {
    let response = web
        .client
        .post(format!("http://{}/api/v1/auth/login", web.current))
        .header("Origin", origin)
        .json(&serde_json::json!({"password":"original browser password"}))
        .send()
        .await
        .expect("login response");
    assert_eq!(response.status(), 403, "retired/foreign Origin {origin}");
    assert!(!response.headers().contains_key("set-cookie"));
    let value: serde_json::Value = response.json().await.expect("error JSON");
    assert_eq!(value["ok"], false);
    assert_eq!(value["error"]["code"], "ORIGIN_FORBIDDEN");
}

// Supplemental production Origin regression for the successful rebind path.
// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275 TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict
#[tokio::test]
async fn production_web_rebind_rejects_retired_listener_origins_for_login() {
    let web = rebound_web().await;
    for host in ["127.0.0.1", "localhost"] {
        assert_login_denied(&web, &format!("http://{host}:{}", web.retired.port())).await;
    }
    let (cookie, _) = web_login(&web.client, web.current, "original browser password").await;
    assert!(!cookie.is_empty());
    web.handle.shutdown().await.expect("shutdown");
}

// Supplemental production Origin regression; frozen Go275 does not assert Origin.
// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275 TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict
#[tokio::test]
async fn production_web_rebind_rejects_retired_listener_origins_for_websocket() {
    let web = rebound_web().await;
    for host in ["127.0.0.1", "localhost"] {
        let origin = format!("http://{host}:{}", web.retired.port());
        let denied =
            websocket_handshake(web.current, &[("Cookie", &web.cookie), ("Origin", &origin)]).await;
        assert_eq!(denied.status, 403, "retired Origin {origin}");
        assert!(denied.upgraded_stream.is_none());
    }
    let origin = format!("http://{}", web.current);
    let accepted =
        websocket_handshake(web.current, &[("Cookie", &web.cookie), ("Origin", &origin)]).await;
    assert_eq!(accepted.status, 101);
    let mut socket = accepted.upgraded_stream.expect("current Origin upgrade");
    let heartbeat: serde_json::Value =
        serde_json::from_str(&read_server_text_frame(&mut socket).await).expect("heartbeat");
    assert_eq!(heartbeat["type"], "heartbeat");
    drop(socket);
    wait_for_live_projection_with_headers(web.handle.startup_record().address, 0, &[], AUTH).await;
    web.handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275 TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict
#[tokio::test]
async fn production_web_rebind_conflict_keeps_current_origin_and_persisted_settings() {
    let web = rebound_web().await;
    let path = web._directory.path().join("settings.json");
    let before = std::fs::read(&path).expect("settings before conflict");
    let occupied = std::net::TcpListener::bind("127.0.0.1:0").expect("occupied port");
    let port = occupied.local_addr().expect("conflict address").port();
    let (status, value) = reconfigure(&web.handle, port).await;
    assert_eq!(status, 409, "{value}");
    assert_eq!(value["error"]["code"], "WEB_ACCESS_LISTENER_UPDATE_FAILED");
    assert!(
        value["error"]["message"]
            .as_str()
            .expect("message")
            .contains("Web access port conflict")
    );
    assert_eq!(std::fs::read(&path).expect("rolled back settings"), before);
    let response = web
        .client
        .get(format!("http://{}/api/v1/auth/session", web.current))
        .header("Cookie", &web.cookie)
        .send()
        .await
        .expect("current session");
    assert_eq!(response.status(), 200);
    let session: serde_json::Value = response.json().await.expect("session JSON");
    assert_eq!(session["data"]["authenticated"], true);
    assert_login_denied(&web, &format!("http://127.0.0.1:{port}")).await;
    let (cookie, _) = web_login(&web.client, web.current, "original browser password").await;
    assert!(!cookie.is_empty());
    drop(occupied);
    web.handle.shutdown().await.expect("shutdown");
}
