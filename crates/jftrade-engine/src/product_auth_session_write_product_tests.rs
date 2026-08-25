use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::json;

use super::super::product_auth_session_write_port::{
    AuthSessionWriteInput, AuthSessionWritePort, AuthSessionWritePortError,
    AuthSessionWritePortResult,
};
use super::*;

#[derive(Debug)]
struct FixtureAuthSessionWritePort;

impl AuthSessionWritePort for FixtureAuthSessionWritePort {
    fn mutate(
        &self,
        input: &AuthSessionWriteInput,
    ) -> Result<
        AuthSessionWritePortResult,
        super::super::product_auth_session_write_port::AuthSessionWritePortError,
    > {
        let data = match input {
            AuthSessionWriteInput::Login { .. } => {
                json!({"authenticated": true, "csrfToken": "fixture-csrf"})
            }
            AuthSessionWriteInput::Logout { .. } => json!({"authenticated": false}),
        };
        Ok(AuthSessionWritePortResult {
            data,
            set_cookie: Some("jftrade_web_session=fixture; Path=/".to_owned()),
        })
    }
}

#[derive(Debug)]
struct RecordingAuthSessionWritePort {
    calls: Mutex<Vec<AuthSessionWriteInput>>,
    responses: Mutex<VecDeque<Result<AuthSessionWritePortResult, AuthSessionWritePortError>>>,
}

impl RecordingAuthSessionWritePort {
    fn new(
        responses: impl IntoIterator<
            Item = Result<AuthSessionWritePortResult, AuthSessionWritePortError>,
        >,
    ) -> Self {
        Self {
            calls: Mutex::new(Vec::new()),
            responses: Mutex::new(responses.into_iter().collect()),
        }
    }

    fn calls(&self) -> Vec<AuthSessionWriteInput> {
        self.calls
            .lock()
            .expect("auth session write calls lock")
            .clone()
    }
}

impl AuthSessionWritePort for RecordingAuthSessionWritePort {
    fn mutate(
        &self,
        input: &AuthSessionWriteInput,
    ) -> Result<AuthSessionWritePortResult, AuthSessionWritePortError> {
        self.calls
            .lock()
            .expect("auth session write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("auth session write responses lock")
            .pop_front()
            .expect("auth session write product response")
    }
}

#[tokio::test]
async fn auth_session_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/auth/login",
        Some("not-json"),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_auth_session_write_port(Arc::new(FixtureAuthSessionWritePort));
    let handle = start_product(config).await.expect("start auth product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/auth/login" })
    );
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/auth/logout" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/auth/logout",
        Some("not-json"),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["authenticated"], false);
    handle.shutdown().await.expect("shutdown auth product");
}

#[tokio::test]
async fn auth_session_write_product_preserves_browser_context_and_recovers() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let port = Arc::new(RecordingAuthSessionWritePort::new([
        Err(AuthSessionWritePortError::Canceled(
            "login request was canceled".to_owned(),
        )),
        Ok(AuthSessionWritePortResult {
            data: json!({
                "authenticated": true,
                "csrfToken": "fixture-csrf",
                "expiresAt": "fixture-time"
            }),
            set_cookie: Some("jftrade_web_session=fixture-session; Path=/".to_owned()),
        }),
        Ok(AuthSessionWritePortResult {
            data: json!({"authenticated": false}),
            set_cookie: Some("jftrade_web_session=; Max-Age=0; Path=/".to_owned()),
        }),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_auth_session_write_port(port.clone());
    config.access = AccessPolicy {
        desktop_token: Some("fixture-desktop-token".to_owned()),
        session_token: Some("fixture-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let login_headers = [("Origin", "https://fixture.jftrade.local")];
    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/auth/login",
        Some(r#"{"password":"fixture-password"}"#),
        &login_headers,
    )
    .await;
    assert_eq!(status, 408);
    assert_eq!(response["error"]["code"], "REQUEST_CANCELED");

    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/auth/login",
        Some(r#"{"password":"fixture-password"}"#),
        &login_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["authenticated"], true);

    let logout_headers = [
        ("Cookie", "jftrade_web_session=fixture-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("X-CSRF-Token", "fixture-csrf"),
    ];
    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/auth/logout",
        Some("not-json"),
        &logout_headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["authenticated"], false);

    let invalid_csrf_headers = [
        ("Cookie", "jftrade_web_session=fixture-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("X-CSRF-Token", "wrong-csrf"),
    ];
    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/auth/logout",
        Some("not-json"),
        &invalid_csrf_headers,
    )
    .await;
    assert_eq!(status, 403);
    assert_eq!(response["error"]["code"], "CSRF_FAILED");

    assert_eq!(
        port.calls(),
        vec![
            AuthSessionWriteInput::Login {
                password: "fixture-password".to_owned()
            },
            AuthSessionWriteInput::Login {
                password: "fixture-password".to_owned()
            },
            AuthSessionWriteInput::Logout {
                session_cookie: Some("fixture-session".to_owned())
            }
        ]
    );
    handle.shutdown().await.expect("shutdown product");
}
