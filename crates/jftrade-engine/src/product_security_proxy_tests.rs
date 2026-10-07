use super::security_stream_tests::{
    protected_product_config, replace_web_password, web_client, web_login,
};
use super::*;

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:422 TestUntrustedOriginIsRejectedButSameOriginLANHostWorks
#[tokio::test]
async fn production_web_login_rejects_foreign_origin_and_accepts_listener_origin() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    let handle = start_product(config).await.expect("production product");
    let client = web_client();
    let endpoint = format!("http://{web_address}/api/v1/auth/login");
    let denied = client
        .post(&endpoint)
        .header("Origin", "https://evil.example.com")
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("foreign Origin login");
    assert_eq!(denied.status(), 403);
    assert!(!denied.headers().contains_key("set-cookie"));
    assert_eq!(
        denied.json::<Value>().await.expect("error JSON")["error"]["code"],
        "ORIGIN_FORBIDDEN"
    );
    let (cookie, _) = web_login(&client, web_address, "original browser password").await;
    assert_eq!(
        web_session_snapshot(&client, web_address, &cookie).await["authenticated"],
        true
    );
    handle.shutdown().await.expect("shutdown product");
}

fn router_with_test_peer(router: axum::Router, peer: SocketAddr) -> axum::Router {
    router.layer(axum::middleware::from_fn(
        move |mut request: axum::extract::Request, next: axum::middleware::Next| async move {
            request
                .extensions_mut()
                .insert(axum::extract::ConnectInfo(peer));
            next.run(request).await
        },
    ))
}

async fn install_web_test_peer(
    handle: &ProductHandle,
    address: SocketAddr,
    public: bool,
    peer: SocketAddr,
) {
    let runtime = handle.web_runtime.as_ref().expect("Web runtime");
    let (router, server) = {
        let mut state = runtime.inner.lock().expect("Web state");
        (
            state.router.clone().expect("production Web router"),
            state.server.take().expect("Web server"),
        )
    };
    server.shutdown().await.expect("retire fixture listener");
    runtime.install_router(router_with_test_peer(router, peer));
    runtime
        .apply(&SecuritySettingsRecord::new(
            true,
            public,
            address.port(),
            "fixture-verifier",
        ))
        .expect("serve production router with fixture peer");
}

async fn web_session_snapshot(
    client: &reqwest::Client,
    address: SocketAddr,
    cookie: &str,
) -> Value {
    let response = client
        .get(format!("http://{address}/api/v1/auth/session"))
        .header("Cookie", cookie)
        .send()
        .await
        .expect("session snapshot");
    assert_eq!(response.status(), 200);
    let value: Value = response.json().await.expect("session JSON");
    assert_eq!(value["ok"], true);
    value["data"].clone()
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:362 TestProductionWebDoesNotTrustDevelopmentOrigin
#[tokio::test]
async fn production_web_login_preserves_configured_development_origin_grant() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    let handle = start_product(config).await.expect("production product");
    let response = web_client()
        .post(format!("http://{web_address}/api/v1/auth/login"))
        .header("Origin", "http://localhost:3003")
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("login response");
    let status = response.status();
    let issued_cookie = response.headers().contains_key("set-cookie");
    let value: Value = response.json().await.expect("login JSON");
    handle.shutdown().await.expect("shutdown product");
    // The red receipt proves the frozen Go 403 expectation fails here.
    // Keep the configured policy and this mapping partial.
    assert_eq!(status, 200);
    assert!(issued_cookie);
    assert_eq!(value["data"]["authenticated"], true);
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:352 TestPasswordChangesInvalidateWebSessions
#[tokio::test]
async fn production_password_change_invalidates_browser_session_snapshot() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    let handle = start_product(config).await.expect("production product");
    let client = web_client();
    let (cookie, csrf) = web_login(&client, web_address, "original browser password").await;
    let before = web_session_snapshot(&client, web_address, &cookie).await;
    assert_eq!(before["authenticated"], true);
    assert_eq!(before["browser"], true);
    assert_eq!(before["csrfToken"], csrf);
    replace_web_password(handle.startup_record().address, web_address).await;
    let after = web_session_snapshot(&client, web_address, &cookie).await;
    assert_eq!(after["authenticated"], false);
    assert_eq!(after["browser"], false);
    assert!(after["csrfToken"].is_null());
    assert!(after["expiresAt"].is_null());
    let origin = format!("http://{web_address}");
    let old_password = client
        .post(format!("{origin}/api/v1/auth/login"))
        .header("Origin", &origin)
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("old password response");
    assert_eq!(old_password.status(), 401);
    assert!(!old_password.headers().contains_key("set-cookie"));
    let (fresh_cookie, _) = web_login(&client, web_address, "replacement browser password").await;
    assert_ne!(fresh_cookie, cookie);
    assert_eq!(
        web_session_snapshot(&client, web_address, &fresh_cookie).await["authenticated"],
        true
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:237 TestNetworkClientCannotSpoofHTTPSProxyScheme
#[tokio::test]
async fn public_web_login_rejects_remote_https_scheme_spoofing() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    SecuritySettingsService::new(Arc::new(
        SettingsFileStore::open(&settings_path).expect("settings"),
    ))
    .save(&SecuritySettingsUpdate {
        web_access_enabled: true,
        public_access_enabled: true,
        web_port: i32::from(web_address.port()),
        ..Default::default()
    })
    .expect("public Web settings");
    let handle = start_product(config).await.expect("production product");
    install_web_test_peer(
        &handle,
        web_address,
        true,
        "192.0.2.20:42000".parse().expect("remote peer"),
    )
    .await;
    let client = web_client();
    let endpoint = format!("http://{web_address}/api/v1/auth/login");
    let response = client
        .post(&endpoint)
        .header("Host", "trade.example")
        .header("Origin", "https://trade.example")
        .header("X-Forwarded-Proto", "https")
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("spoofed login");
    assert_eq!(response.status(), 403);
    assert!(!response.headers().contains_key("set-cookie"));
    let value: Value = response.json().await.expect("rejected login JSON");
    assert_eq!(value["error"]["code"], "ORIGIN_FORBIDDEN");
    // The same production owner accepts a legitimate HTTP Origin, but the
    // remote peer cannot turn X-Forwarded-Proto into a Secure-cookie grant.
    let accepted = client
        .post(&endpoint)
        .header("Origin", format!("http://{web_address}"))
        .header("X-Forwarded-Proto", "https")
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("legitimate remote login");
    assert_eq!(accepted.status(), 200);
    let cookie = accepted.headers()["set-cookie"]
        .to_str()
        .expect("cookie")
        .to_owned();
    assert!(cookie.contains("HttpOnly"));
    assert!(cookie.contains("SameSite=Strict"));
    assert!(!cookie.contains("; Secure"));
    assert_eq!(
        accepted.json::<Value>().await.expect("login JSON")["data"]["authenticated"],
        true
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:256 TestSameHostProxyCannotBypassPublicAccessSetting
#[tokio::test]
async fn private_web_proxy_login_preserves_origin_and_forwarded_client_boundaries() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    let handle = start_product(config).await.expect("production product");
    install_web_test_peer(
        &handle,
        web_address,
        false,
        "127.0.0.1:42000".parse().expect("proxy peer"),
    )
    .await;
    let client = web_client();
    let endpoint = format!("http://{web_address}/api/v1/auth/login");
    let rejected = client
        .post(&endpoint)
        .header("Host", "trade.example")
        .header("Origin", "https://trade.example")
        .header("X-Forwarded-Proto", "https")
        .header("X-Forwarded-For", "192.0.2.20")
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("proxy login");
    assert_eq!(rejected.status(), 403);
    assert!(!rejected.headers().contains_key("set-cookie"));
    let value: Value = rejected.json().await.expect("error JSON");
    assert_eq!(value["error"]["code"], "ORIGIN_FORBIDDEN");
    // A loopback proxy with an allowed Origin is admitted even when its
    // forwarded client is remote. This counterexample keeps the mapping partial.
    let admitted = client
        .post(&endpoint)
        .header("Origin", format!("http://{web_address}"))
        .header("X-Forwarded-Proto", "https")
        .header("X-Forwarded-For", "192.0.2.20")
        .json(&json!({"password": "original browser password"}))
        .send()
        .await
        .expect("allowed proxy Origin");
    assert_eq!(admitted.status(), 200);
    assert!(
        admitted.headers()["set-cookie"]
            .to_str()
            .expect("cookie")
            .contains("; Secure")
    );
    assert_eq!(
        admitted.json::<Value>().await.expect("login JSON")["data"]["authenticated"],
        true
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:437 TestLoopbackPolicyBlocksRemoteBrowserUntilExplicitlyEnabled
#[tokio::test]
async fn production_web_network_admission_is_owned_by_the_listener_bind() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    let handle = start_product(config).await.expect("production product");
    install_web_test_peer(
        &handle,
        web_address,
        false,
        "192.0.2.20:12345".parse().expect("remote peer"),
    )
    .await;
    let runtime = handle.web_runtime.as_ref().expect("Web runtime");
    assert_eq!(
        runtime.inner.lock().expect("Web state").bind.as_deref(),
        Some(format!("127.0.0.1:{}", web_address.port()).as_str())
    );
    let client = web_client();
    // Injecting peer metadata bypasses the TCP bind, exposing the seam
    // difference: Rust has no per-request remote/private 403.
    let private = client
        .get(format!("http://{web_address}/"))
        .send()
        .await
        .expect("private navigation");
    assert_ne!(private.status(), 403);
    drop(private);
    let (status, value) = request_json_with_status(handle.startup_record().address, "PUT", "/api/v1/settings/security",
        Some(&json!({"webAccessEnabled": true, "publicAccessEnabled": true, "webPort": web_address.port()}).to_string()),
        &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")]).await;
    assert_eq!(status, 200, "public configuration: {value}");
    assert_eq!(
        runtime.inner.lock().expect("Web state").bind.as_deref(),
        Some(format!("0.0.0.0:{}", web_address.port()).as_str())
    );
    let public = client
        .get(format!("http://{web_address}/"))
        .send()
        .await
        .expect("public navigation");
    assert_ne!(public.status(), 403);
    drop(public);
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:461 TestDesktopCapabilityStaysPasswordlessWhenWebAccessIsDisabled
#[tokio::test]
async fn disabled_production_web_keeps_desktop_capability_and_rejects_browser_access() {
    let directory = tempdir().expect("directory");
    let settings_path = directory.path().join("settings.json");
    let (config, web_address) = protected_product_config(&settings_path);
    SecuritySettingsService::new(Arc::new(
        SettingsFileStore::open(&settings_path).expect("settings"),
    ))
    .save(&SecuritySettingsUpdate {
        web_access_enabled: false,
        web_port: i32::from(web_address.port()),
        ..Default::default()
    })
    .expect("disable Web");
    let runtime = ProductRuntimeState::product_only(&config);
    let mut prepared = prepare_product_with_runtime_state(config, runtime, None)
        .await
        .expect("prepared product");
    prepared.router = router_with_test_peer(
        prepared.router,
        "192.0.2.20:12345".parse().expect("desktop peer"),
    );
    let handle = expose_prepared_product(prepared).expect("expose product");
    let address = handle.startup_record().address;
    let (desktop, value) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")],
    )
    .await;
    assert_eq!(desktop, 200);
    assert_eq!(value["ok"], true);
    let (browser, rejected) =
        request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
    assert_eq!(browser, 401);
    assert_eq!(rejected["error"]["code"], "WEB_AUTH_REQUIRED");
    assert!(
        std::net::TcpListener::bind(web_address).is_ok(),
        "disabled Web port is free"
    );
    handle.shutdown().await.expect("shutdown product");
}
