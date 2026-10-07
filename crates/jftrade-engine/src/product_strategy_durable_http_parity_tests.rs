//! Strategy definition and lifecycle parity through the production writer.

use super::*;
use crate::product::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWritePort, StrategyDefinitionWritePortError,
    dispatch_strategy_definition_write,
};
use std::sync::Mutex;

const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];

fn production_config(directory: &tempfile::TempDir) -> ProductConfig {
    let path = directory.path().join("settings.json");
    std::fs::write(&path, "{}").expect("settings");
    product_data_management::initialize_production_databases(&path).expect("databases");
    let mut config = ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("address"),
        &path,
        "a".repeat(32),
    )
    .expect("production config");
    config.capabilities = ProductCapabilities::all();
    config
}

async fn request(
    handle: &ProductHandle,
    method: &str,
    path: &str,
    body: Option<&str>,
) -> (u16, Value) {
    request_json_with_status(handle.startup_record().address, method, path, body, AUTH).await
}

async fn create_definition(handle: &ProductHandle) -> String {
    let body = r#"{"id":"client-id","name":"Draft","version":"0.1.0","description":"first snapshot","sourceFormat":"pine-v6","script":"//@version=6\nstrategy(\"Initial\")"}"#;
    let (status, response) =
        request(handle, "POST", "/api/v1/strategy-definitions", Some(body)).await;
    assert_eq!(status, 200, "{response}");
    let id = response["data"]["id"]
        .as_str()
        .expect("definition id")
        .to_owned();
    assert_ne!(id, "client-id");
    id
}

async fn instantiate(handle: &ProductHandle, definition_id: &str) -> String {
    let (status, response) = request(
        handle,
        "POST",
        &format!("/api/v1/strategy-definitions/{definition_id}/instantiate"),
        Some(r#"{"symbols":["US.AAPL"],"interval":"1m"}"#),
    )
    .await;
    assert_eq!(status, 200, "{response}");
    response["data"]["id"]
        .as_str()
        .expect("instance id")
        .to_owned()
}

#[derive(Debug, Default)]
struct RecordingDefinitionPort(Mutex<Vec<StrategyDefinitionWriteInput>>);
impl StrategyDefinitionWritePort for RecordingDefinitionPort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        self.0.lock().expect("calls").push(input.clone());
        Ok(json!({}))
    }
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:203 TestDefinitionRoutesNormalizeCreateUpdateAndDeleteGuards
#[test]
fn definition_mutation_normalizes_original_client_identity_before_owner_call() {
    let port = RecordingDefinitionPort::default();
    for (method, path, body) in [
        (
            "POST",
            "/api/v1/strategy-definitions",
            br#"{"id":"client-id","name":"Draft"}"#.as_slice(),
        ),
        (
            "PUT",
            "/api/v1/strategy-definitions/def-9",
            br#"{"name":"Updated"}"#.as_slice(),
        ),
    ] {
        let api_request = jftrade_api::ApiRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            query: String::new(),
            body: body.to_vec(),
            request_id: "strategy-definition-identity".to_owned(),
            desktop_trusted: true,
            origin_provided: false,
            origin_allowed: true,
            browser_authenticated: true,
            csrf_valid: false,
            session_cookie: None,
        };
        let response =
            dispatch_strategy_definition_write(&api_request, Some(&port), "2026-07-25T09:00:00Z");
        assert_eq!(response.status, 200, "{:?}", response.body);
    }
    let calls = port.0.lock().expect("calls");
    assert_eq!(calls.len(), 2);
    assert_eq!(calls[0].definition.as_ref().expect("create")["id"], "");
    assert_eq!(
        calls[0].definition.as_ref().expect("create")["name"],
        "Draft"
    );
    assert_eq!(calls[1].definition_id.as_deref(), Some("def-9"));
    assert_eq!(calls[1].definition.as_ref().expect("update")["id"], "def-9");
    assert_eq!(
        calls[1].definition.as_ref().expect("update")["name"],
        "Updated"
    );
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:203 TestDefinitionRoutesNormalizeCreateUpdateAndDeleteGuards
#[tokio::test]
async fn production_http_definition_delete_guard_releases_after_linked_instance_soft_delete() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    let definition_id = create_definition(&handle).await;
    let instance_id = instantiate(&handle, &definition_id).await;
    let path = format!("/api/v1/strategy-definitions/{definition_id}");
    let (status, response) = request(&handle, "GET", "/api/v1/strategy-definitions", None).await;
    assert_eq!(status, 200, "{response}");
    let (status, response) = request(&handle, "DELETE", &path, None).await;
    assert_eq!(status, 400, "linked guard: {response}");
    let (status, response) = request(&handle, "GET", &path, None).await;
    assert_eq!(status, 200, "guard must retain definition: {response}");
    let (status, response) = request(
        &handle,
        "DELETE",
        &format!("/api/v1/strategies/{instance_id}"),
        None,
    )
    .await;
    assert_eq!(status, 200, "delete stopped instance: {response}");
    handle.shutdown().await.expect("shutdown before restart");
    let handle = start_product(production_config(&directory))
        .await
        .expect("restart");
    let (status, response) = request(&handle, "DELETE", &path, None).await;
    assert_eq!(
        status, 200,
        "deleted instance must release durable guard: {response}"
    );
    assert_eq!(response["data"]["id"], definition_id);
    let (status, response) = request(&handle, "GET", "/api/v1/strategy-definitions", None).await;
    assert_eq!(status, 200, "definition list: {response}");
    assert!(
        response["data"]
            .as_array()
            .expect("definitions")
            .iter()
            .all(|row| row["id"] != definition_id)
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:252 TestDefinitionVersionRoutesExposeHistoryAndSnapshots
#[tokio::test]
async fn production_http_definition_versions_preserve_snapshot_and_current_marker_after_restart() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    let id = create_definition(&handle).await;
    let body = r#"{"name":"Current","version":"0.1.1","description":"current snapshot","sourceFormat":"pine-v6","script":"//@version=6\nstrategy(\"Current\")"}"#;
    let (status, response) = request(
        &handle,
        "PUT",
        &format!("/api/v1/strategy-definitions/{id}"),
        Some(body),
    )
    .await;
    assert_eq!(status, 200, "update: {response}");
    handle.shutdown().await.expect("shutdown");
    let handle = start_product(production_config(&directory))
        .await
        .expect("restart");
    let (status, response) = request(
        &handle,
        "GET",
        &format!("/api/v1/strategy-definitions/{id}/versions"),
        None,
    )
    .await;
    assert_eq!(status, 200, "versions: {response}");
    let versions = response["data"].as_array().expect("versions");
    assert_eq!(versions.len(), 2);
    for version in versions {
        assert_eq!(version["definitionId"], id);
        assert!(
            version["savedAt"]
                .as_str()
                .is_some_and(|date| !date.is_empty())
        );
        assert_eq!(version["isCurrent"], version["version"] == "0.1.1");
    }
    let (status, response) = request(
        &handle,
        "GET",
        &format!("/api/v1/strategy-definitions/{id}/versions/0.1.0"),
        None,
    )
    .await;
    assert_eq!(status, 200, "snapshot: {response}");
    assert_eq!(response["data"]["description"], "first snapshot");
    assert_eq!(
        response["data"]["script"],
        "//@version=6\nstrategy(\"Initial\")"
    );
    assert_eq!(response["data"]["isCurrent"], false);
    for path in [
        "/api/v1/strategy-definitions/missing/versions".to_owned(),
        format!("/api/v1/strategy-definitions/{id}/versions/0.0.1"),
    ] {
        let (status, response) = request(&handle, "GET", &path, None).await;
        assert_eq!(status, 404, "{response}");
        assert_eq!(response["error"]["code"], "NOT_FOUND");
    }
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/pine_routes_contracts_test.go:182 TestAnalyzeStrategyPineRouteRejectsUnsupportedSourceFormat
#[tokio::test]
async fn production_http_pine_legacy_source_is_rejected_before_missing_worker() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    let body =
        r#"{"sourceFormat":"legacy","script":"//@version=6\nstrategy(\"Analyze\", overlay=true)"}"#;
    let (status, response) =
        request(&handle, "POST", "/api/v1/strategy-pine/analyze", Some(body)).await;
    assert_eq!(status, 400, "{response}");
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "BAD_REQUEST");
    assert_eq!(
        response["error"]["message"],
        "strategy-pine analyze supports pine-v6 only"
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_boundary_contracts_test.go:75 TestStrategyStartRouteMapsPreflightRuntimeAndTransitionBoundaries
// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:319 TestInstantiateApplyAndLifecycleRoutesFollowBusinessStateTransitions
#[tokio::test]
async fn production_http_missing_strategy_start_does_not_change_a_stopped_sibling() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    let definition_id = create_definition(&handle).await;
    let instance_id = instantiate(&handle, &definition_id).await;
    let (status, response) =
        request(&handle, "POST", "/api/v1/strategies/missing/start", None).await;
    assert_eq!(status, 404, "{response}");
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    let (status, response) = request(&handle, "GET", "/api/v1/strategies", None).await;
    assert_eq!(status, 200, "{response}");
    let rows = response["data"].as_array().expect("instances");
    let sibling = rows
        .iter()
        .find(|row| row["id"] == instance_id)
        .expect("stopped sibling");
    assert_eq!(sibling["status"], "STOPPED");
    handle.shutdown().await.expect("shutdown");
}
