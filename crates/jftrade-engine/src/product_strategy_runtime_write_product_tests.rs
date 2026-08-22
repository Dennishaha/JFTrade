use std::sync::Arc;

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
