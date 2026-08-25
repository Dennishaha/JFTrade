use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

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

#[derive(Debug)]
struct UnavailableAdkMutationPort {
    calls: AtomicUsize,
}

impl UnavailableAdkMutationPort {
    fn new() -> Self {
        Self {
            calls: AtomicUsize::new(0),
        }
    }
}

impl AdkMutationPort for UnavailableAdkMutationPort {
    fn mutate(&self, _input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        Err(AdkMutationPortError::Unavailable(
            "fixture Rust mutation port crashed".to_owned(),
        ))
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

#[tokio::test]
async fn adk_mutation_product_fails_closed_and_recovers_after_restart_without_settings_write() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read initial settings");
    let unavailable_port = Arc::new(UnavailableAdkMutationPort::new());
    let failing_config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(unavailable_port.clone());
    let handle = start_product(failing_config)
        .await
        .expect("start failing product");

    let (status, malformed) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some("{"),
        &[],
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(malformed["error"]["code"], "BAD_REQUEST");
    assert_eq!(unavailable_port.calls.load(Ordering::SeqCst), 0);

    let (status, unavailable) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 503);
    assert_eq!(unavailable["error"]["code"], "ADK_MUTATIONS_UNAVAILABLE");
    assert_eq!(unavailable_port.calls.load(Ordering::SeqCst), 1);
    handle.shutdown().await.expect("shutdown failing product");

    let recovered_config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("recovered config")
            .with_adk_mutation_port(Arc::new(FixtureAdkMutationPort));
    let recovered = start_product(recovered_config)
        .await
        .expect("start recovered product");
    let (status, response) = request_json_with_status(
        recovered.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    recovered
        .shutdown()
        .await
        .expect("shutdown recovered product");

    let settings_after = std::fs::read(&settings_path).expect("read final settings");
    assert_eq!(settings_after, settings_before);
}
