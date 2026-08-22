use std::sync::Arc;

use serde_json::{Value, json};

use super::super::product_research_preset_write_port::{
    ResearchPresetWriteMutation, ResearchPresetWritePortError,
};
use super::super::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureResearchPresetWritePort;

impl ResearchPresetWritePort for FixtureResearchPresetWritePort {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        Ok(json!({
            "operation": format!("{:?}", mutation.operation()),
            "presetId": "preset-test",
            "revision": 2,
            "deleted": matches!(mutation, ResearchPresetWriteMutation::Delete { .. }),
        }))
    }
}

#[derive(Debug)]
struct FixtureStrategyDefinitionWritePort;

impl StrategyDefinitionWritePort for FixtureStrategyDefinitionWritePort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        Ok(json!({
            "operation": input.operation.name(),
            "definitionId": input.definition_id,
            "hasDefinition": input.definition.is_some(),
            "hasBinding": input.binding.is_some(),
        }))
    }
}

#[derive(Debug)]
struct FailingStrategyDefinitionWritePort;

impl StrategyDefinitionWritePort for FailingStrategyDefinitionWritePort {
    fn mutate(
        &self,
        _input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        Err(StrategyDefinitionWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: "fixture mutation failed".to_owned(),
        })
    }
}

#[tokio::test]
async fn strategy_and_research_write_routes_are_opt_in_and_counted_exactly() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/research/screens/presets",
        Some("{}"),
    )
    .await;
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let research =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_write_port(Arc::new(FixtureResearchPresetWritePort));
    let handle = start_product(research)
        .await
        .expect("start research product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    let response = request_json(
        handle.startup_record().address,
        "PATCH",
        "/api/v1/research/screens/presets/preset-test",
        Some(r#"{"name":"Updated","expectedRevision":1}"#),
    )
    .await;
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["presetId"], "preset-test");
    handle.shutdown().await.expect("shutdown research product");

    let strategy =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_definition_write_port(Arc::new(FixtureStrategyDefinitionWritePort));
    let handle = start_product(strategy)
        .await
        .expect("start strategy product");
    assert_eq!(handle.startup_record().owned_routes, 53);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategy-definitions/definition-1/instantiate",
        None,
    )
    .await;
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["operation"], "instantiate");
    handle.shutdown().await.expect("shutdown strategy product");
}

#[tokio::test]
async fn strategy_and_research_write_routes_preserve_failure_and_combined_registration() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_write_port(Arc::new(FixtureResearchPresetWritePort))
            .with_strategy_definition_write_port(Arc::new(FailingStrategyDefinitionWritePort));
    let handle = start_product(config).await.expect("start combined product");
    assert_eq!(handle.startup_record().owned_routes, 56);
    let response = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/strategy-definitions",
        Some(r#"{"name":"Draft"}"#),
    )
    .await;
    assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
    let response = request_json(
        handle.startup_record().address,
        "DELETE",
        "/api/v1/research/screens/presets/preset-test",
        Some("not-json"),
    )
    .await;
    assert_eq!(response["ok"], true);
    handle.shutdown().await.expect("shutdown combined product");
}
