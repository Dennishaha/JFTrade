use std::sync::Arc;

use serde_json::json;

use super::super::product_auth_session_write_port::{
    AuthSessionWriteInput, AuthSessionWritePort, AuthSessionWritePortResult,
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
