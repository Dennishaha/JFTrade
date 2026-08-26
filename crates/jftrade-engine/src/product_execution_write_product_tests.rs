use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_execution_write_port::test_cutover::ExecutionSqliteTestCutoverPort;
use super::super::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWritePort, ExecutionWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureExecutionWritePort;

impl ExecutionWritePort for FixtureExecutionWritePort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
        }))
    }
}

#[derive(Debug)]
struct SequencedExecutionWritePort {
    responses: Mutex<VecDeque<Result<Value, ExecutionWritePortError>>>,
    inputs: Mutex<Vec<ExecutionWriteInput>>,
}

impl SequencedExecutionWritePort {
    fn new(responses: impl IntoIterator<Item = Result<Value, ExecutionWritePortError>>) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            inputs: Mutex::new(Vec::new()),
        }
    }
}

impl ExecutionWritePort for SequencedExecutionWritePort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.inputs
            .lock()
            .expect("execution write product inputs lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("execution write product responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(ExecutionWritePortError::Unavailable(
                    "fixture execution writer response missing".to_owned(),
                ))
            })
    }
}

fn execution_browser_access_policy() -> AccessPolicy {
    AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()])
}

#[tokio::test]
async fn execution_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/execution/orders",
        Some(r#"{"market":"US","symbol":"AAPL","side":"BUY","quantity":1}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_execution_write_port(Arc::new(FixtureExecutionWritePort));
    let handle = start_product(config)
        .await
        .expect("start execution write product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/execution/orders" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/execution/orders",
        Some(r#"{"market":"US","symbol":"AAPL","side":"BUY","quantity":1}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "order-place");
    handle
        .shutdown()
        .await
        .expect("shutdown execution write product");
}

#[tokio::test]
async fn execution_write_product_replays_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"execution-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let port = Arc::new(SequencedExecutionWritePort::new([
        Err(ExecutionWritePortError::Unavailable(
            "fixture execution writer unavailable".to_owned(),
        )),
        Ok(json!({"accepted": true, "operation": "order-place"})),
        Err(ExecutionWritePortError::Failed {
            status: 504,
            code: "BROKER_TIMEOUT".to_owned(),
            message: "fixture preview timeout".to_owned(),
        }),
        Ok(json!({"accepted": true, "operation": "combo-preview"})),
        Ok(json!({"accepted": true, "operation": "combo-place"})),
        Ok(json!({"accepted": true, "operation": "combo-cancel"})),
        Ok(json!({"accepted": true, "operation": "order-cancel"})),
        Ok(json!({"accepted": true, "operation": "order-preview"})),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_execution_write_port(port.clone());
    config.access = execution_browser_access_policy();
    let handle = start_product(config)
        .await
        .expect("start execution product");
    assert_eq!(handle.startup_record().owned_routes, 55);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/execution"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "execution-write-fixture"),
    ];
    let order_body = r#"{"brokerId":"FUTU","accountId":"acct-1","market":"US","symbol":"US.AAPL","side":"BUY","quantity":1}"#;
    let combo_body = r#"{"brokerId":"FUTU","accountId":"acct-1","market":"US","legs":[]}"#;
    let order_path = "/api/v1/execution/orders";

    let unauthorized =
        request_json_with_status(address, "POST", order_path, Some(order_body), &[]).await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        order_path,
        Some(order_body),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/execution"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);
    assert_eq!(csrf_missing.1["error"]["code"], "CSRF_FAILED");

    let unavailable = request_json_with_status(
        address,
        "POST",
        order_path,
        Some(order_body),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(
        unavailable.1["error"]["code"],
        "EXECUTION_WRITE_UNAVAILABLE"
    );

    let started = request_json_with_status(
        address,
        "POST",
        order_path,
        Some(order_body),
        &browser_headers,
    )
    .await;
    assert_eq!(started.0, 200);
    assert_eq!(started.1["data"]["operation"], "order-place");

    let preview_failed = request_json_with_status(
        address,
        "POST",
        "/api/v1/execution/combos/previews",
        Some(combo_body),
        &browser_headers,
    )
    .await;
    assert_eq!(preview_failed.0, 504);
    assert_eq!(preview_failed.1["error"]["code"], "BROKER_TIMEOUT");
    let preview_recovered = request_json_with_status(
        address,
        "POST",
        "/api/v1/execution/combos/previews",
        Some(combo_body),
        &browser_headers,
    )
    .await;
    assert_eq!(preview_recovered.0, 200);
    assert_eq!(preview_recovered.1["data"]["operation"], "combo-preview");

    let requests = [
        (
            "POST",
            "/api/v1/execution/combos",
            Some(combo_body),
            "combo-place",
        ),
        (
            "POST",
            "/api/v1/execution/combos/combo-1/cancel",
            None,
            "combo-cancel",
        ),
        (
            "POST",
            "/api/v1/execution/orders/order-1/cancel",
            None,
            "order-cancel",
        ),
        (
            "POST",
            "/api/v1/execution/previews",
            Some(order_body),
            "order-preview",
        ),
    ];
    for (method, path, body, operation) in requests {
        let (status, response) =
            request_json_with_status(address, method, path, body, &browser_headers).await;
        assert_eq!(status, 200, "{method} {path}");
        assert_eq!(response["data"]["operation"], operation);
    }
    {
        let inputs = port.inputs.lock().expect("execution write product inputs");
        assert_eq!(inputs.len(), 8);
        assert_eq!(
            inputs
                .iter()
                .map(|input| input.operation.name())
                .collect::<Vec<_>>(),
            vec![
                "order-place",
                "order-place",
                "combo-preview",
                "combo-preview",
                "combo-place",
                "combo-cancel",
                "order-cancel",
                "order-preview",
            ]
        );
    }
    handle.shutdown().await.expect("shutdown execution product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after replay"),
        settings_before
    );

    let mut restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_execution_write_port(Arc::new(FixtureExecutionWritePort));
    restarted_config.access = execution_browser_access_policy();
    let restarted = start_product(restarted_config)
        .await
        .expect("restart execution product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        order_path,
        Some(order_body),
        &browser_headers,
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["operation"], "order-place");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted execution product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

#[tokio::test]
async fn execution_sqlite_test_cutover_replays_transport_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("execution-test-cutover.db");
    std::fs::write(&settings_path, b"{\"seed\":\"execution-durable\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("settings");
    let port = Arc::new(
        ExecutionSqliteTestCutoverPort::open(&database_path).expect("open durable adapter"),
    );
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_execution_write_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let order_body = r#"{"brokerId":"fixture","symbol":"US.AAPL","side":"BUY","quantity":1}"#;
    let combo_body = r#"{"brokerId":"fixture","market":"US","legs":[]}"#;
    let operations = [
        ("/api/v1/execution/buying-power", Some(order_body)),
        ("/api/v1/execution/previews", Some(order_body)),
        ("/api/v1/execution/combos/previews", Some(combo_body)),
        ("/api/v1/execution/orders", Some(order_body)),
        ("/api/v1/execution/combos", Some(combo_body)),
    ];
    for (path, body) in operations {
        let response = request_json_with_status(address, "POST", path, body, &[]).await;
        assert_eq!(response.0, 200, "POST {path}");
    }
    for path in [
        "/api/v1/execution/orders/order-test-1/cancel",
        "/api/v1/execution/combos/combo-test-2/cancel",
    ] {
        let response = request_json_with_status(address, "POST", path, None, &[]).await;
        assert_eq!(response.0, 200, "POST {path}");
    }
    handle.shutdown().await.expect("shutdown product");
    drop(port);

    let reopened = Arc::new(
        ExecutionSqliteTestCutoverPort::open(&database_path).expect("reopen durable adapter"),
    );
    assert_eq!(
        reopened.order_status("order-test-1").expect("order status"),
        Some("cancelled".to_owned())
    );
    assert_eq!(
        reopened.order_status("combo-test-2").expect("combo status"),
        Some("cancelled".to_owned())
    );
    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restart address"),
        &settings_path,
    )
    .expect("restart config")
    .with_execution_write_port(reopened);
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/execution/orders",
        Some(order_body),
        &[],
    )
    .await;
    assert_eq!(response.0, 200);
    assert_eq!(response.1["data"]["internalOrderId"], "order-test-3");
    restarted.shutdown().await.expect("shutdown restart");
    assert_eq!(
        std::fs::read(&settings_path).expect("settings after restart"),
        settings_before
    );
}
