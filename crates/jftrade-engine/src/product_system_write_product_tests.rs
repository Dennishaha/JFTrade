use std::sync::Arc;

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
