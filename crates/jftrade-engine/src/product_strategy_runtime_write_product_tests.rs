use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWritePort, StrategyRuntimeWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureStrategyRuntimeWritePort;

impl StrategyRuntimeWritePort for FixtureStrategyRuntimeWritePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
            "instanceId": input.instance_id,
        }))
    }
}

#[derive(Debug)]
struct SequencedStrategyRuntimeWritePort {
    responses: Mutex<VecDeque<Result<Value, StrategyRuntimeWritePortError>>>,
    calls: Mutex<Vec<StrategyRuntimeWriteInput>>,
}

impl SequencedStrategyRuntimeWritePort {
    fn new(
        responses: impl IntoIterator<Item = Result<Value, StrategyRuntimeWritePortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<StrategyRuntimeWriteInput> {
        self.calls
            .lock()
            .expect("strategy runtime write calls lock")
            .clone()
    }
}

impl StrategyRuntimeWritePort for SequencedStrategyRuntimeWritePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        self.calls
            .lock()
            .expect("strategy runtime write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("strategy runtime write responses lock")
            .pop_front()
            .expect("strategy runtime write rehearsal response")
    }
}

#[tokio::test]
async fn strategy_runtime_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategies/fixture-instance/start",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_runtime_write_port(Arc::new(FixtureStrategyRuntimeWritePort));
    let handle = start_product(config)
        .await
        .expect("start strategy runtime product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/strategies/{instanceId}/start" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategies/fixture-instance/start",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "start");
    handle
        .shutdown()
        .await
        .expect("shutdown strategy runtime product");
}

#[tokio::test]
async fn strategy_runtime_write_product_replays_browser_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"strategies-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let success = |operation: &str| {
        Ok(json!({
            "accepted": true,
            "source": "rust-product",
            "operation": operation,
        }))
    };
    let port = Arc::new(SequencedStrategyRuntimeWritePort::new([
        Err(StrategyRuntimeWritePortError::Unavailable(
            "fixture strategy runtime unavailable".to_owned(),
        )),
        success("start"),
        Err(StrategyRuntimeWritePortError::Failed {
            status: 502,
            code: "STRATEGY_RUNTIME_START_FAILED".to_owned(),
            message: "context deadline exceeded".to_owned(),
        }),
        success("start"),
        success("update"),
        success("update-runtime-risk"),
        success("pause"),
        success("stop"),
        success("refresh-definition"),
        success("delete"),
        success("pause"),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_runtime_write_port(port.clone());
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
        .expect("start strategy runtime write product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/strategies"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "strategies-write-product"),
    ];
    let start_path = "/api/v1/strategies/instance-1/start";

    let unauthorized = request_json_with_status(address, "POST", start_path, None, &[]).await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        start_path,
        None,
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/strategies"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);
    assert_eq!(csrf_missing.1["error"]["code"], "CSRF_FAILED");

    let unavailable =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(unavailable.1["error"]["code"], "STRATEGY_UNAVAILABLE");
    let recovered =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(recovered.0, 200);
    assert_eq!(recovered.1["data"]["operation"], "start");
    let failed =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(failed.0, 502);
    assert_eq!(failed.1["error"]["code"], "STRATEGY_RUNTIME_START_FAILED");
    let recovered =
        request_json_with_status(address, "POST", start_path, None, &browser_headers).await;
    assert_eq!(recovered.0, 200);

    for (method, path, body, operation) in [
        (
            "PUT",
            "/api/v1/strategies/instance-1",
            Some(r#"{"symbols":["AAPL"],"interval":"1m"}"#),
            "update",
        ),
        (
            "PUT",
            "/api/v1/strategies/instance-1/runtime-risk",
            Some(r#"{"mode":"paper","closeOnly":true}"#),
            "update-runtime-risk",
        ),
        (
            "POST",
            "/api/v1/strategies/instance-1/pause",
            Some("ignored-pause-body"),
            "pause",
        ),
        ("POST", "/api/v1/strategies/instance-1/stop", None, "stop"),
        (
            "POST",
            "/api/v1/strategies/instance-1/refresh-definition",
            Some("not-json"),
            "refresh-definition",
        ),
        ("DELETE", "/api/v1/strategies/instance-1", None, "delete"),
    ] {
        let (status, response) =
            request_json_with_status(address, method, path, body, &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(response["data"]["operation"], operation, "{method} {path}");
    }
    let duplicate = request_json_with_status(
        address,
        "POST",
        "/api/v1/strategies/instance-1/pause",
        Some("ignored-pause-body"),
        &browser_headers,
    )
    .await;
    assert_eq!(duplicate.0, 200);
    assert_eq!(duplicate.1["data"]["operation"], "pause");

    let calls = port.calls();
    assert_eq!(calls.len(), 11);
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation.name() == "start")
            .count(),
        4
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation.name() == "pause")
            .count(),
        2
    );
    handle
        .shutdown()
        .await
        .expect("shutdown strategy runtime write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_strategy_runtime_write_port(Arc::new(FixtureStrategyRuntimeWritePort));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart strategy runtime write product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/strategies/instance-1/start",
        None,
        &[],
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["operation"], "start");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted strategy runtime write product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}
