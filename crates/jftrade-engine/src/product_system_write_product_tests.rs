use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_system_write_port::{
    SystemWriteInput, SystemWritePort, SystemWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureSystemWritePort;

impl SystemWritePort for FixtureSystemWritePort {
    fn mutate(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
        }))
    }
}

#[derive(Debug)]
struct SequencedSystemWritePort {
    responses: Mutex<VecDeque<Result<Value, SystemWritePortError>>>,
    calls: Mutex<Vec<SystemWriteInput>>,
}

impl SequencedSystemWritePort {
    fn new(responses: impl IntoIterator<Item = Result<Value, SystemWritePortError>>) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<SystemWriteInput> {
        self.calls.lock().expect("system-write calls lock").clone()
    }
}

impl SystemWritePort for SequencedSystemWritePort {
    fn mutate(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        self.calls
            .lock()
            .expect("system-write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("system-write responses lock")
            .pop_front()
            .expect("system-write rehearsal response")
    }
}

#[tokio::test]
async fn system_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_system_write_port(Arc::new(FixtureSystemWritePort));
    let handle = start_product(config)
        .await
        .expect("start system write product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/system/real-trade-kill-switch/activate" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "activate-kill-switch");
    handle
        .shutdown()
        .await
        .expect("shutdown system write product");
}

#[tokio::test]
async fn system_write_product_replays_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"system-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let success = |operation: &str| {
        Ok(json!({
            "accepted": true,
            "source": "rust-product",
            "operation": operation,
        }))
    };
    let port = Arc::new(SequencedSystemWritePort::new([
        Err(SystemWritePortError::Unavailable(
            "fixture system-write owner unavailable".to_owned(),
        )),
        success("activate-kill-switch"),
        Err(SystemWritePortError::Failed {
            status: 409,
            code: "REAL_TRADE_CONTROL_FAILED".to_owned(),
            message: "fixture control failure".to_owned(),
        }),
        success("activate-hard-stop"),
        success("manual-retry"),
        success("release-hard-stop"),
        success("release-kill-switch"),
        success("update-risk"),
        success("disable-risk"),
        success("activate-kill-switch"),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_system_write_port(port.clone());
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config)
        .await
        .expect("start system-write product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/system"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "system-write-product"),
    ];

    let unauthorized = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &[],
    )
    .await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/system"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);

    let unavailable = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(unavailable.1["error"]["code"], "SYSTEM_WRITE_UNAVAILABLE");
    let recovered = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(recovered.0, 200);
    assert_eq!(recovered.1["data"]["operation"], "activate-kill-switch");

    let failed = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-hard-stops",
        Some(r#"{"accountId":"ACC-1"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(failed.0, 409);
    assert_eq!(failed.1["error"]["code"], "REAL_TRADE_CONTROL_FAILED");
    let recovered = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-hard-stops",
        Some(r#"{"accountId":"ACC-1"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(recovered.0, 200);
    assert_eq!(recovered.1["data"]["operation"], "activate-hard-stop");

    for (method, path, body, operation) in [
        (
            "POST",
            "/api/v1/system/futu-opend/manual-retry",
            Some("not-json"),
            "manual-retry",
        ),
        (
            "POST",
            "/api/v1/system/real-trade-hard-stops/hs-1/release",
            Some("{}"),
            "release-hard-stop",
        ),
        (
            "POST",
            "/api/v1/system/real-trade-kill-switch/release",
            None,
            "release-kill-switch",
        ),
        (
            "PUT",
            "/api/v1/system/real-trade-risk-limits",
            Some(r#"{"realTradingEnabled":true,"maxOrderQuantity":1}"#),
            "update-risk",
        ),
        (
            "DELETE",
            "/api/v1/system/real-trade-risk-limits",
            None,
            "disable-risk",
        ),
    ] {
        let (status, response) =
            request_json_with_status(address, method, path, body, &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(response["data"]["operation"], operation, "{method} {path}");
    }

    let duplicate = request_json_with_status(
        address,
        "POST",
        "/api/v1/system/real-trade-kill-switch/activate",
        Some(r#"{"operatorId":"fixture"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(duplicate.0, 200);
    let calls = port.calls();
    assert_eq!(calls.len(), 10);
    assert_eq!(calls[0].operation.name(), "activate-kill-switch");
    assert_eq!(calls[1].operation.name(), "activate-kill-switch");
    assert_eq!(calls[9].operation.name(), "activate-kill-switch");
    handle
        .shutdown()
        .await
        .expect("shutdown system-write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_system_write_port(Arc::new(FixtureSystemWritePort));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart system-write product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/system/futu-opend/manual-retry",
        Some("not-json"),
        &[],
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["operation"], "manual-retry");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted system-write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}
