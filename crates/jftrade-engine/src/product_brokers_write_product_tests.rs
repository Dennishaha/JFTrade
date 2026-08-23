use std::sync::Arc;

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_brokers_write_port::{
    BrokersWriteInput, BrokersWritePort, BrokersWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureBrokersWritePort;

impl BrokersWritePort for FixtureBrokersWritePort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
            "brokerId": input.query.broker_id,
        }))
    }
}

#[tokio::test]
async fn brokers_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders",
        Some(r#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_brokers_write_port(Arc::new(FixtureBrokersWritePort));
    let handle = start_product(config)
        .await
        .expect("start brokers write product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/brokers/{brokerId}/orders" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/brokers/futu/orders",
        Some(r#"{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "place-order");
    handle
        .shutdown()
        .await
        .expect("shutdown brokers write product");
}
