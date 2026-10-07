//! Strategy HTTP failure matrices and durable deletion side effects.

use super::*;
use crate::product::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWritePort, StrategyDefinitionWritePortError,
};
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWritePort, StrategyRuntimeWritePortError,
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

#[derive(Debug, Default)]
struct FailingDefinitionOwner(Mutex<Vec<StrategyDefinitionWriteInput>>);

impl StrategyDefinitionWritePort for FailingDefinitionOwner {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        self.0.lock().expect("calls").push(input.clone());
        Err(StrategyDefinitionWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: "definition store unavailable".to_owned(),
        })
    }
}

#[derive(Debug)]
struct FailingDefinitionSnapshot;

impl StrategyDefinitionSnapshotPort for FailingDefinitionSnapshot {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError> {
        Err(StrategyDefinitionSnapshotError::Unavailable(
            "definition store unavailable".to_owned(),
        ))
    }
    fn get(
        &self,
        _: &str,
        _: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        Err(StrategyDefinitionSnapshotError::Unavailable(
            "definition store unavailable".to_owned(),
        ))
    }
    fn versions(&self, _: &str) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError> {
        Err(StrategyDefinitionSnapshotError::Unavailable(
            "history unavailable".to_owned(),
        ))
    }
    fn version(&self, _: &str, _: &str) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        Err(StrategyDefinitionSnapshotError::Unavailable(
            "snapshot unavailable".to_owned(),
        ))
    }
}

// Parity: go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:71 TestDefinitionRoutesMapReadWriteAndDeleteFailures
#[tokio::test]
async fn http_strategy_definition_read_write_failures_preserve_codes_and_original_identity() {
    let directory = tempdir().expect("directory");
    let write = Arc::new(FailingDefinitionOwner::default());
    // This explicit rehearsal seam mirrors Go's failing design store. The
    // concrete production writer is verified separately by preview/delete tests.
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        directory.path().join("settings.json"),
    )
    .expect("rehearsal")
    .with_strategy_definition_snapshot_port(Arc::new(FailingDefinitionSnapshot))
    .with_strategy_definition_write_port(write.clone());
    let handle = start_product(config).await.expect("rehearsal listener");
    for path in [
        "/api/v1/strategy-definitions",
        "/api/v1/strategy-definitions/def-1",
    ] {
        let (status, response) = request(&handle, "GET", path, None).await;
        assert_eq!(status, 500, "{path}: {response}");
        assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
    }
    for (method, path, body) in [
        ("POST", "/api/v1/strategy-definitions", Some("{")),
        ("PUT", "/api/v1/strategy-definitions/def-1", Some("{")),
    ] {
        let (status, response) = request(&handle, method, path, body).await;
        assert_eq!(status, 400, "{response}");
        assert_eq!(response["error"]["code"], "BAD_REQUEST");
    }
    assert!(write.0.lock().expect("calls").is_empty());
    for (method, path, body) in [
        (
            "POST",
            "/api/v1/strategy-definitions",
            Some(r#"{"id":"client-id","name":"Draft"}"#),
        ),
        (
            "PUT",
            "/api/v1/strategy-definitions/def-1",
            Some(r#"{"name":"Updated"}"#),
        ),
        ("DELETE", "/api/v1/strategy-definitions/def-1", None),
    ] {
        let (status, response) = request(&handle, method, path, body).await;
        assert_eq!(status, 500, "{response}");
        assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
    }
    let calls = write.0.lock().expect("calls").clone();
    assert_eq!(calls.len(), 3);
    assert_eq!(calls[0].definition.as_ref().expect("create")["id"], "");
    assert_eq!(
        calls[0].definition.as_ref().expect("create")["name"],
        "Draft"
    );
    assert_eq!(calls[1].definition_id.as_deref(), Some("def-1"));
    assert_eq!(calls[1].definition.as_ref().expect("update")["id"], "def-1");
    assert_eq!(
        calls[1].definition.as_ref().expect("update")["name"],
        "Updated"
    );
    assert_eq!(calls[2].definition_id.as_deref(), Some("def-1"));
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:71 TestDefinitionRoutesMapReadWriteAndDeleteFailures
#[tokio::test]
async fn production_http_strategy_definition_preview_derives_twenty_sma_warmup_bars() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    let body = r#"{"name":"Warmup","symbol":"US.AAPL","interval":"5m","sourceFormat":"pine-v6","script":"//@version=6\nstrategy(\"Warmup\", overlay=true)\nslow = ta.sma(close, 20)"}"#;
    let (status, response) =
        request(&handle, "POST", "/api/v1/strategy-definitions", Some(body)).await;
    assert_eq!(status, 200, "{response}");
    let id = response["data"]["id"].as_str().expect("id");
    let (status, response) = request(
        &handle,
        "GET",
        &format!("/api/v1/strategy-definitions/{id}?interval=5m&symbol=US.AAPL"),
        None,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["derivedWarmupBars"], 20);
    assert_eq!(response["data"]["derivedWarmupInterval"], "5m");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:252 TestDefinitionVersionRoutesExposeHistoryAndSnapshots
#[tokio::test]
async fn http_strategy_version_history_and_snapshot_owner_failures_return_strategy_failed() {
    let directory = tempdir().expect("directory");
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        directory.path().join("settings.json"),
    )
    .expect("rehearsal")
    .with_strategy_definition_snapshot_port(Arc::new(FailingDefinitionSnapshot));
    let handle = start_product(config).await.expect("rehearsal listener");
    for (path, message) in [
        (
            "/api/v1/strategy-definitions/def-1/versions",
            "history unavailable",
        ),
        (
            "/api/v1/strategy-definitions/def-1/versions/0.0.1",
            "snapshot unavailable",
        ),
    ] {
        let (status, response) = request(&handle, "GET", path, None).await;
        assert_eq!(status, 500, "{response}");
        assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
        assert!(
            response["error"]["message"]
                .as_str()
                .expect("message")
                .contains(message)
        );
    }
    handle.shutdown().await.expect("shutdown");
}

#[derive(Debug, Default)]
struct FailingInstanceOwner(Mutex<Vec<StrategyRuntimeWriteInput>>);

impl StrategyRuntimeWritePort for FailingInstanceOwner {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        self.0.lock().expect("calls").push(input.clone());
        Err(StrategyRuntimeWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: "instance store unavailable".to_owned(),
        })
    }
}

// Parity: go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:184 TestInstanceMutationRoutesMapCatalogFailures
#[tokio::test]
async fn http_strategy_instance_mutation_owner_failures_preserve_original_route_arguments() {
    let directory = tempdir().expect("directory");
    let owner = Arc::new(FailingInstanceOwner::default());
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        directory.path().join("settings.json"),
    )
    .expect("rehearsal")
    .with_strategy_runtime_write_port(owner.clone());
    let handle = start_product(config).await.expect("rehearsal listener");
    for (method, path, body) in [
        (
            "PUT",
            "/api/v1/strategies/inst-1",
            Some(r#"{"symbols":["US.AAPL"],"interval":"1m"}"#),
        ),
        (
            "PUT",
            "/api/v1/strategies/inst-1/runtime-risk",
            Some(r#"{"mode":"close_only","closeOnly":true}"#),
        ),
        ("POST", "/api/v1/strategies/inst-1/pause", None),
        ("POST", "/api/v1/strategies/inst-1/stop", None),
    ] {
        let (status, response) = request(&handle, method, path, body).await;
        assert_eq!(status, 500, "{path}: {response}");
        assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
    }
    let calls = owner.0.lock().expect("calls").clone();
    assert_eq!(calls.len(), 4);
    assert!(calls.iter().all(|call| call.instance_id == "inst-1"));
    assert_eq!(
        calls[0].binding.as_ref().expect("binding")["symbols"],
        json!(["US.AAPL"])
    );
    assert_eq!(
        calls[0].binding.as_ref().expect("binding")["interval"],
        "1m"
    );
    assert_eq!(
        calls[1].runtime_risk.as_ref().expect("risk")["closeOnly"],
        true
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:401 TestDeleteInstanceRouteMapsBusinessOutcomes
#[tokio::test]
async fn production_http_strategy_delete_maps_missing_busy_and_durable_success() {
    let directory = tempdir().expect("directory");
    let config = production_config(&directory);
    let descriptors = product_data_management::managed_database_runtime_descriptors(
        &directory.path().join("settings.json"),
    );
    let path = &descriptors
        .iter()
        .find(|descriptor| descriptor.id == "strategy")
        .expect("strategy database")
        .path;
    let store = jftrade_store_sqlite::StrategyRuntimeStore::open_existing(
        path,
        jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
    )
    .expect("writer");
    // STARTING is busy and has no live worker to reclaim at product startup.
    store
        .seed_instance("inst-1", "STOPPED", "2026-07-25T08:00:00Z")
        .expect("stopped");
    store
        .seed_instance("busy", "STARTING", "2026-07-25T08:00:00Z")
        .expect("busy");
    drop(store);
    let handle = start_product(config).await.expect("production");
    let (status, response) = request(&handle, "DELETE", "/api/v1/strategies/missing", None).await;
    assert_eq!(status, 404, "{response}");
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    let (status, response) = request(&handle, "DELETE", "/api/v1/strategies/busy", None).await;
    assert_eq!(status, 400, "{response}");
    assert_eq!(response["error"]["code"], "BAD_REQUEST");
    let (status, response) = request(&handle, "DELETE", "/api/v1/strategies/inst-1", None).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["id"], "inst-1");
    handle.shutdown().await.expect("shutdown");
    let store = jftrade_store_sqlite::StrategyRuntimeStore::open_existing(
        path,
        jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
    )
    .expect("reopen writer");
    assert!(
        store
            .get_instance("inst-1")
            .expect("deleted snapshot")
            .expect("row")
            .deleted
    );
    let busy = store
        .get_instance("busy")
        .expect("busy snapshot")
        .expect("row");
    assert_eq!(busy.status, "STARTING");
    assert!(!busy.deleted);
}

// Parity: go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:138 TestDefinitionOrchestrationRoutesMapBusinessFailures
#[tokio::test]
async fn production_http_strategy_orchestration_missing_definition_rejects_without_instances() {
    let directory = tempdir().expect("directory");
    let handle = start_product(production_config(&directory))
        .await
        .expect("production");
    for suffix in ["instantiate", "apply-linked-instances"] {
        let (status, response) = request(
            &handle,
            "POST",
            &format!("/api/v1/strategy-definitions/missing/{suffix}"),
            None,
        )
        .await;
        assert_eq!(status, 404, "{response}");
        assert_eq!(response["error"]["code"], "NOT_FOUND");
    }
    let (status, response) = request(&handle, "GET", "/api/v1/strategies", None).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"], json!([]));
    handle.shutdown().await.expect("shutdown");
}
