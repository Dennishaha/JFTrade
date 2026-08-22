use std::sync::Arc;

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationPort, AdkMutationPortError,
};
use super::*;

#[derive(Debug)]
struct FixtureAdkMutationPort;

impl AdkMutationPort for FixtureAdkMutationPort {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
        }))
    }
}

#[tokio::test]
async fn adk_mutation_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"Fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(Arc::new(FixtureAdkMutationPort));
    let handle = start_product(config).await.expect("start ADK product");
    assert_eq!(handle.startup_record().owned_routes, 85);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/adk/agents" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"Fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "create-agent");
    handle.shutdown().await.expect("shutdown ADK product");
}
