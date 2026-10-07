use super::*;
use std::io::{Read, Write};
use std::sync::mpsc;

fn held_model_provider() -> (String, mpsc::Sender<()>, std::thread::JoinHandle<()>) {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("provider listener");
    let endpoint = format!(
        "http://{}/v1",
        listener.local_addr().expect("provider address")
    );
    let (release, wait) = mpsc::channel();
    let thread = std::thread::spawn(move || {
        let (mut socket, _) = listener.accept().expect("provider connection");
        socket
            .set_read_timeout(Some(Duration::from_secs(10)))
            .expect("read timeout");
        let mut request = Vec::new();
        let mut buffer = [0; 4096];
        loop {
            let count = socket.read(&mut buffer).expect("provider request");
            assert!(count > 0);
            request.extend_from_slice(&buffer[..count]);
            if let Some(at) = request.windows(4).position(|part| part == b"\r\n\r\n") {
                let length = String::from_utf8_lossy(&request[..at])
                    .lines()
                    .find_map(|line| {
                        line.split_once(':')
                            .filter(|(key, _)| key.eq_ignore_ascii_case("content-length"))
                    })
                    .map(|(_, value)| value.trim().parse::<usize>().expect("request length"))
                    .unwrap_or(0);
                if request.len() >= at + 4 + length {
                    break;
                }
            }
        }
        socket.write_all(b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pending provider\"}\n\n")
            .expect("provider delta");
        let _ = wait.recv_timeout(Duration::from_secs(15));
    });
    (endpoint, release, thread)
}

pub(super) fn protected_product_config(
    settings_path: &std::path::Path,
) -> (ProductConfig, SocketAddr) {
    product_data_management::initialize_production_databases(settings_path).expect("databases");
    let store = Arc::new(SettingsFileStore::open(settings_path).expect("settings"));
    let security = SecuritySettingsService::new(store);
    let reservation = std::net::TcpListener::bind("127.0.0.1:0").expect("reserve Web port");
    let web_address = reservation.local_addr().expect("Web address");
    security
        .save(&SecuritySettingsUpdate {
            web_access_enabled: true,
            web_port: i32::from(web_address.port()),
            new_password: "original browser password".to_owned(),
            ..Default::default()
        })
        .expect("security settings");
    let mut config = ProductConfig::new(
        "127.0.0.1:0".parse().expect("desktop address"),
        settings_path,
        AccessPolicy::desktop(Some("a".repeat(32))),
    )
    .expect("product config");
    config.production = true;
    config.capabilities = ProductCapabilities::all();
    (config, web_address)
}

fn seed_stream_agent(directory: &std::path::Path, endpoint: &str) {
    let store = jftrade_store_sqlite::AdkStore::open(directory.join("adk.db")).expect("ADK store");
    store
        .upsert_provider(
            "provider-stream",
            &json!({
                "id": "provider-stream", "displayName": "Loopback Provider", "baseUrl": endpoint,
                "model": "fixture-model", "enabled": true,
            })
            .to_string(),
        )
        .expect("provider");
    store
        .upsert_agent(
            "agent-stream",
            &json!({
                "id": "agent-stream", "name": "Stream Agent", "providerId": "provider-stream",
                "permissionMode": "all", "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("agent");
    std::fs::create_dir_all(directory.join("secrets")).expect("secrets directory");
    std::fs::write(
        directory.join("secrets/adk-secrets.json"),
        br#"{"provider-stream":"sk-fixture"}"#,
    )
    .expect("provider secret");
}

pub(super) async fn web_login(
    client: &reqwest::Client,
    address: SocketAddr,
    password: &str,
) -> (String, String) {
    let origin = format!("http://{address}");
    let response = client
        .post(format!("{origin}/api/v1/auth/login"))
        .header("Origin", &origin)
        .json(&json!({"password": password}))
        .send()
        .await
        .expect("login");
    assert_eq!(response.status(), 200);
    let cookie = response.headers()["set-cookie"]
        .to_str()
        .expect("cookie")
        .split(';')
        .next()
        .expect("cookie pair")
        .to_owned();
    let value: Value = response.json().await.expect("login JSON");
    (
        cookie,
        value["data"]["csrfToken"]
            .as_str()
            .expect("CSRF")
            .to_owned(),
    )
}

pub(super) fn web_client() -> reqwest::Client {
    let _ = rustls::crypto::ring::default_provider().install_default();
    reqwest::Client::builder()
        .timeout(Duration::from_secs(12))
        .build()
        .expect("Web client")
}

pub(super) async fn replace_web_password(address: SocketAddr, web_address: SocketAddr) {
    let (status, response) = request_json_with_status(
        address,
        "PUT",
        "/api/v1/settings/security",
        Some(
            &json!({"webAccessEnabled": true, "webPort": web_address.port(),
            "newPassword": "replacement browser password"})
            .to_string(),
        ),
        &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")],
    )
    .await;
    assert_eq!(status, 200, "security update: {response}");
    assert_eq!(response["data"]["webAccessEnabled"], true);
    assert_eq!(response["data"]["webPort"], web_address.port());
}

// Parity: go:452dea11:internal/app/apiserver/servercore/security_test.go:43 TestSecurityChangeCancelsExistingWebStream
#[tokio::test]
async fn password_change_cancels_active_web_chat_stream_on_unchanged_listener() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    let (endpoint, release, provider) = held_model_provider();
    seed_stream_agent(directory.path(), &endpoint);
    let handle = start_product(config).await.expect("production product");
    let client = web_client();
    let (cookie, csrf) = web_login(&client, web_address, "original browser password").await;
    let origin = format!("http://{web_address}");
    let mut response = client
        .post(format!("{origin}/api/v1/adk/chat/stream"))
        .header("Origin", &origin)
        .header("Cookie", &cookie)
        .header("X-CSRF-Token", &csrf)
        .json(
            &json!({"clientRequestId": "11111111-1111-4111-8111-111111111117",
            "agentId": "agent-stream", "message": "wait for provider"}),
        )
        .send()
        .await
        .expect("open Web stream");
    assert_eq!(response.status(), 200);
    assert_eq!(response.headers()["content-type"], "text/event-stream");
    let mut body = String::new();
    tokio::time::timeout(Duration::from_secs(5), async {
        while !body.contains("pending provider") {
            let chunk = response
                .chunk()
                .await
                .expect("stream frame")
                .expect("active stream");
            body.push_str(&String::from_utf8_lossy(&chunk));
        }
    })
    .await
    .expect("live provider delta");
    assert!(body.contains("\"type\":\"session\""));
    assert!(body.contains("\"type\":\"run\""));
    replace_web_password(handle.startup_record().address, web_address).await;
    let ended = tokio::time::timeout(Duration::from_secs(2), async {
        while response
            .chunk()
            .await
            .expect("revoked stream transport")
            .is_some()
        {}
    })
    .await;
    drop(response);
    let _ = release.send(());
    provider.join().expect("provider thread");
    let (status, rejected) = request_json_with_status(
        web_address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Cookie", &cookie)],
    )
    .await;
    assert_eq!(status, 401, "old session: {rejected}");
    let _ = web_login(&client, web_address, "replacement browser password").await;
    handle.shutdown().await.expect("shutdown product");
    assert!(
        ended.is_ok(),
        "password change must end the already active SSE without changing Web bind"
    );
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:275 TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict
// Parity: go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:116 TestDisablingWebImmediatelyInvalidatesBrowserButNotDesktop
#[tokio::test]
async fn active_web_chat_does_not_block_listener_disable_or_rebind() {
    for disable in [true, false] {
        let directory = tempdir().expect("directory");
        let settings_path = directory.path().join("settings.json");
        let (config, web_address) = protected_product_config(&settings_path);
        let (endpoint, release, provider) = held_model_provider();
        seed_stream_agent(directory.path(), &endpoint);
        let handle = start_product(config).await.expect("product");
        let client = web_client();
        let (cookie, csrf) = web_login(&client, web_address, "original browser password").await;
        let origin = format!("http://{web_address}");
        let mut stream = client
            .post(format!("{origin}/api/v1/adk/chat/stream"))
            .header("Origin", &origin)
            .header("Cookie", &cookie)
            .header("X-CSRF-Token", &csrf)
            .json(
                &json!({"clientRequestId": "11111111-1111-4111-8111-111111111118",
                "agentId": "agent-stream", "message": "keep this listener busy"}),
            )
            .send()
            .await
            .expect("open stream");
        assert_eq!(stream.status(), 200);
        let mut text = String::new();
        tokio::time::timeout(Duration::from_secs(5), async {
            while !text.contains("pending provider") {
                let chunk = stream.chunk().await.expect("chunk").expect("active stream");
                text.push_str(&String::from_utf8_lossy(&chunk));
            }
        })
        .await
        .expect("provider delta");
        let reservation = std::net::TcpListener::bind("127.0.0.1:0").expect("new port");
        let new_address = reservation.local_addr().expect("new address");
        drop(reservation);
        let update = client
            .put(format!(
                "http://{}/api/v1/settings/security",
                handle.startup_record().address
            ))
            .bearer_auth("a".repeat(32))
            .json(&json!({"webAccessEnabled": !disable, "webPort": new_address.port()}))
            .send();
        let updated = tokio::time::timeout(Duration::from_secs(2), update).await;
        let stream_ended = if updated.is_ok() {
            tokio::time::timeout(Duration::from_secs(2), async {
                while stream
                    .chunk()
                    .await
                    .expect("listener stream transport")
                    .is_some()
                {}
            })
            .await
            .is_ok()
        } else {
            false
        };
        // Release all resources even on the original graceful-join deadlock,
        // preserving a failed regression without leaving an active server.
        drop(stream);
        let _ = release.send(());
        provider.join().expect("provider thread");
        let (desktop_status, _) = request_json_with_status(
            handle.startup_record().address,
            "GET",
            "/api/v1/system/status",
            None,
            &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")],
        )
        .await;
        let (browser_status, rejected) = request_json_with_status(
            handle.startup_record().address,
            "GET",
            "/api/v1/system/status",
            None,
            &[("Cookie", &cookie)],
        )
        .await;
        assert_eq!(browser_status, 401, "old cookie: {rejected}");
        assert_eq!(rejected["error"]["code"], "WEB_AUTH_REQUIRED");
        if updated.is_ok() && !disable {
            let _ = web_login(&client, new_address, "original browser password").await;
        }
        handle.shutdown().await.expect("shutdown product");
        assert_eq!(desktop_status, 200);
        assert!(
            stream_ended,
            "listener change ends the active SSE before provider completion"
        );
        let response = updated
            .expect("listener change must not wait for the active provider")
            .expect("security response");
        assert_eq!(response.status(), 200, "disable={disable}");
        let settings: Value = response.json().await.expect("security response body");
        assert_eq!(settings["data"]["webAccessEnabled"], !disable);
        assert_eq!(settings["data"]["webPort"], new_address.port());
    }
}
