use super::product_production_assembly_tests::{seed_password_protected_web, setup_test_env};
use crate::product::start_product;
use crate::product::tests::request_json_with_status;
use serde_json::Value;
use std::fs;

const PASSWORD: &str = "a memorable Web passphrase";
const DESKTOP_AUTH: (&str, &str) = ("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");

fn assert_no_password_material(value: &Value) {
    let text = value.to_string();
    for forbidden in [PASSWORD, "newPassword", "passwordHash", "adminAuthRequired"] {
        assert!(
            !text.contains(forbidden),
            "response leaked {forbidden}: {text}"
        );
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:37 TestDesktopCanEnablePasswordProtectedWebWithoutExposingPassword
// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:138 TestWebAuthRemainingLoginResponses
#[tokio::test]
async fn production_http_desktop_enables_password_web_and_keeps_password_material_private() {
    let (_directory, settings_path, config, security) = setup_test_env();
    let reservation = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let web_address = reservation.local_addr().unwrap();
    security
        .save(&jftrade_settings::SecuritySettingsUpdate {
            web_port: i32::from(web_address.port()),
            ..Default::default()
        })
        .unwrap();
    drop(reservation);
    let handle = start_product(config).await.unwrap();
    let desktop_address = handle.startup_record().address;
    let (status, body) = request_json_with_status(
        desktop_address,
        "POST",
        "/api/v1/auth/login",
        Some("{}"),
        &[DESKTOP_AUTH],
    )
    .await;
    assert_eq!(status, 200, "trusted desktop login: {body}");
    assert_eq!(body["data"]["authenticated"], true);
    assert_eq!(body["data"]["csrfToken"], "");
    let (status, body) = request_json_with_status(
        desktop_address,
        "POST",
        "/api/v1/auth/login",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 403, "disabled browser login: {body}");
    let (status, enabled) = request_json_with_status(
        desktop_address, "PUT", "/api/v1/settings/security",
        Some(r#"{"webAccessEnabled":true,"publicAccessEnabled":false,"newPassword":"a memorable Web passphrase"}"#),
        &[DESKTOP_AUTH],
    ).await;
    assert_eq!(status, 200, "{enabled}");
    assert_eq!(enabled["data"]["webAccessEnabled"], true);
    assert_eq!(enabled["data"]["passwordConfigured"], true);
    assert_no_password_material(&enabled);
    let persisted_bytes = fs::read(&settings_path).unwrap();
    let persisted_text = String::from_utf8(persisted_bytes.clone()).unwrap();
    assert!(!persisted_text.contains(PASSWORD));
    assert!(!persisted_text.contains("adminAuthRequired"));
    let persisted: Value = serde_json::from_slice(&persisted_bytes).unwrap();
    assert!(
        persisted["security"]["passwordHash"]
            .as_str()
            .unwrap()
            .starts_with("$argon2id$")
    );
    let (status, anonymous) =
        request_json_with_status(web_address, "GET", "/api/v1/settings/security", None, &[]).await;
    assert_eq!(status, 401, "{anonymous}");
    let origin = format!("http://{web_address}");
    for (body, expected) in [("{", 400), (r#"{"password":"wrong"}"#, 401)] {
        let (status, value) = request_json_with_status(
            web_address,
            "POST",
            "/api/v1/auth/login",
            Some(body),
            &[("Origin", &origin)],
        )
        .await;
        assert_eq!(status, expected, "{value}");
        assert_eq!(value["ok"], false);
    }
    let client = crate::product::tests::security_stream_tests::web_client();
    let (cookie, _) =
        crate::product::tests::security_stream_tests::web_login(&client, web_address, PASSWORD)
            .await;
    let (status, authenticated) = request_json_with_status(
        web_address,
        "GET",
        "/api/v1/settings/security",
        None,
        &[("Cookie", &cookie)],
    )
    .await;
    assert_eq!(status, 200, "{authenticated}");
    assert_no_password_material(&authenticated);
    handle.shutdown().await.unwrap();
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/auth_boundaries_test.go:319 TestWebAuthRemainingCanceledPasswordSlotAndStatus
#[tokio::test]
async fn production_http_session_status_rejects_foreign_origin_and_returns_cookie_bound_csrf() {
    let (_directory, _settings_path, config, security) = setup_test_env();
    let web_address = seed_password_protected_web(&security);
    let handle = start_product(config).await.unwrap();
    let (status, forbidden) = request_json_with_status(
        web_address,
        "GET",
        "/api/v1/auth/session",
        None,
        &[("Origin", "https://evil.example")],
    )
    .await;
    assert_eq!(status, 403, "{forbidden}");
    let client = crate::product::tests::security_stream_tests::web_client();
    let (cookie, csrf) =
        crate::product::tests::security_stream_tests::web_login(&client, web_address, PASSWORD)
            .await;
    let (status, session) = request_json_with_status(
        web_address,
        "GET",
        "/api/v1/auth/session",
        None,
        &[("Cookie", &cookie)],
    )
    .await;
    assert_eq!(status, 200, "{session}");
    assert_eq!(session["data"]["authenticated"], true);
    assert_eq!(session["data"]["browser"], true);
    assert_eq!(session["data"]["csrfToken"], csrf);
    assert_no_password_material(&session);
    handle.shutdown().await.unwrap();
}
