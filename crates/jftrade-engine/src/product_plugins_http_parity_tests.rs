//! Plugin transport parity through the concrete file owner.

use super::*;
use std::path::Path;

const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];
const ARTIFACT: &[u8] = b"temporary plugin artifact; never executed";

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

async fn request(handle: &ProductHandle, method: &str, path: &str) -> (u16, Value) {
    request_json_with_status(handle.startup_record().address, method, path, None, AUTH).await
}

fn seed_marker(directory: &Path, id: &str, marker: &Value) -> std::path::PathBuf {
    let root = directory.join("plugins");
    std::fs::create_dir_all(&root).expect("plugin directory");
    std::fs::write(root.join("source.bin"), ARTIFACT).expect("source artifact");
    let path = root.join(format!("{id}.json"));
    std::fs::write(&path, serde_json::to_vec(marker).expect("marker JSON")).expect("marker");
    path
}

fn marker(id: &str) -> Value {
    json!({"descriptor":{"id":id,"type":"strategy-go-plugin","displayName":"Demo Plugin","version":"1.0.0"},
        "installation":{"sourcePath":"source.bin"},"operations":[]})
}

// Parity: go:452dea11:internal/api/strategy/routes_failure_boundaries_test.go:230 TestPluginRoutesRejectWhitespaceIdentifiers
#[tokio::test]
async fn production_http_plugin_whitespace_identifiers_reject_before_file_mutation() {
    let directory = tempdir().expect("directory");
    let config = production_config(&directory);
    let marker_path = seed_marker(directory.path(), "plugin-a", &marker("plugin-a"));
    let before = std::fs::read(&marker_path).expect("before");
    let handle = start_product(config).await.expect("production");
    for (method, path) in [
        ("GET", "/api/v1/plugins/operations/%20"),
        ("POST", "/api/v1/plugins/%20/install"),
        ("GET", "/api/v1/plugins/%20/uninstall-guidance"),
    ] {
        let (status, response) = request(&handle, method, path).await;
        assert_eq!(status, 400, "{path}: {response}");
        assert_eq!(response["error"]["code"], "BAD_REQUEST");
    }
    assert_eq!(std::fs::read(marker_path).expect("after"), before);
    assert!(!directory.path().join("plugins/plugin-a.so").exists());
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:642 TestPluginRoutesCoverCatalogOperationMutationAndGuidance
// Parity: go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:13 TestCatalogPluginLifecyclePersistsSortedMetadataAndOperations
#[tokio::test]
async fn production_http_plugin_lifecycle_persists_operations_and_sorted_catalog_after_restart() {
    let directory = tempdir().expect("directory");
    let config = production_config(&directory);
    let seed_operation = json!({"operationId":"op-1","pluginId":"plugin-a"});
    let mut initial = marker("plugin-a");
    initial["operations"] = json!([seed_operation.clone()]);
    let marker_path = seed_marker(directory.path(), "plugin-a", &initial);
    seed_marker(directory.path(), "plugin-z", &marker("plugin-z"));
    let handle = start_product(config).await.expect("production");
    let (status, response) = request(&handle, "GET", "/api/v1/plugins").await;
    assert_eq!(status, 200, "{response}");
    let rows = response["data"]["plugins"].as_array().expect("plugins");
    assert_eq!(rows.len(), 2);
    assert_eq!(rows[0]["descriptor"]["id"], "plugin-a");
    assert_eq!(rows[1]["descriptor"]["id"], "plugin-z");
    let (status, response) = request(&handle, "GET", "/api/v1/plugins/operations/op-1").await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"], seed_operation);
    let (status, response) = request(&handle, "POST", "/api/v1/plugins/plugin-a/install").await;
    assert_eq!(status, 200, "{response}");
    let installed = response["data"]["operation"].clone();
    assert_eq!(installed["pluginId"], "plugin-a");
    assert_eq!(installed["status"], "SUCCEEDED");
    assert_eq!(installed["phase"], "installed");
    assert_eq!(installed["progress"], 100);
    assert_eq!(
        std::fs::read(directory.path().join("plugins/plugin-a.so")).expect("artifact"),
        ARTIFACT
    );
    let id = installed["operationId"].as_str().expect("operation id");
    let (status, response) =
        request(&handle, "GET", &format!("/api/v1/plugins/operations/{id}")).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"], installed);
    let (status, response) = request(
        &handle,
        "GET",
        "/api/v1/plugins/plugin-a/uninstall-guidance",
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["pluginId"], "plugin-a");
    assert_eq!(response["data"]["exists"], true);
    let (status, response) = request(&handle, "POST", "/api/v1/plugins/plugin-a/uninstall").await;
    assert_eq!(status, 200, "{response}");
    let uninstalled = response["data"]["operation"].clone();
    assert_eq!(uninstalled["pluginId"], "plugin-a");
    assert_eq!(uninstalled["status"], "SUCCEEDED");
    assert_eq!(uninstalled["phase"], "uninstalled");
    assert!(!directory.path().join("plugins/plugin-a.so").exists());
    let stored: Value =
        serde_json::from_slice(&std::fs::read(marker_path).expect("marker")).expect("JSON");
    assert_eq!(
        stored["operations"],
        json!([seed_operation, installed.clone(), uninstalled.clone()])
    );
    assert_eq!(stored["installation"]["lastOperation"], uninstalled);
    assert_eq!(stored["installation"]["installed"], false);
    assert_eq!(stored["installation"]["status"], "NOT_INSTALLED");
    handle.shutdown().await.expect("shutdown");
    let handle = start_product(production_config(&directory))
        .await
        .expect("restart");
    for operation in [installed, uninstalled] {
        let id = operation["operationId"].as_str().expect("id");
        let (status, response) =
            request(&handle, "GET", &format!("/api/v1/plugins/operations/{id}")).await;
        assert_eq!(status, 200, "{response}");
        assert_eq!(response["data"], operation);
    }
    let (status, response) = request(&handle, "GET", "/api/v1/plugins").await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(
        response["data"]["plugins"][0]["installation"]["installed"],
        false
    );
    assert_eq!(
        response["data"]["plugins"][0]["installation"]["status"],
        "NOT_INSTALLED"
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/strategy/routes_lifecycle_test.go:689 TestPluginMutationRoutesMapNotFoundAndInternalFailures
// Parity: go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:89 TestCatalogPluginLifecycleClassifiesMissingResource
#[tokio::test]
async fn production_http_plugin_missing_resources_and_unlink_failure_preserve_marker() {
    let directory = tempdir().expect("directory");
    let config = production_config(&directory);
    let marker_path = seed_marker(directory.path(), "plugin-a", &marker("plugin-a"));
    std::fs::create_dir(directory.path().join("plugins/plugin-a.so")).expect("non-file artifact");
    let before = std::fs::read(&marker_path).expect("before");
    let handle = start_product(config).await.expect("production");
    for (method, path) in [
        ("POST", "/api/v1/plugins/missing/install"),
        ("POST", "/api/v1/plugins/missing/uninstall"),
        ("GET", "/api/v1/plugins/operations/missing"),
        ("GET", "/api/v1/plugins/missing/uninstall-guidance"),
    ] {
        let (status, response) = request(&handle, method, path).await;
        assert_eq!(status, 404, "{path}: {response}");
        assert_eq!(response["error"]["code"], "NOT_FOUND");
    }
    let (status, response) = request(&handle, "POST", "/api/v1/plugins/plugin-a/uninstall").await;
    assert_eq!(status, 500, "{response}");
    assert_eq!(response["error"]["code"], "INTERNAL_ERROR");
    assert_eq!(std::fs::read(marker_path).expect("after"), before);
    assert!(directory.path().join("plugins/plugin-a.so").is_dir());
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/plugin_lifecycle_test.go:16 TestPluginCatalogLifecycleEndpoints
#[tokio::test]
async fn production_http_plugin_legacy_build_tuple_exposes_native_artifact_boundary() {
    let directory = tempdir().expect("directory");
    let config = production_config(&directory);
    let legacy = json!({"descriptor":{"id":"demo-plugin","type":"strategy-go-plugin","displayName":"Demo Plugin","version":"1.0.0","description":"demo dynamic plugin","keywords":["strategy","go-plugin"]},
        "artifact":{"build":{"jftradeVersion":"legacy-version","goVersion":"go1.26","goos":"darwin","goarch":"arm64","buildMode":"plugin"}}});
    let marker_path = seed_marker(directory.path(), "demo-plugin", &legacy);
    let before = std::fs::read(&marker_path).expect("before");
    let handle = start_product(config).await.expect("production");
    let (status, response) = request(&handle, "GET", "/api/v1/plugins").await;
    assert_eq!(status, 200, "{response}");
    let rows = response["data"]["plugins"].as_array().expect("plugins");
    assert_eq!(rows.len(), 1);
    assert_eq!(rows[0]["descriptor"], legacy["descriptor"]);
    assert_eq!(rows[0]["compatibility"]["requiresRebuild"], false);
    let (status, response) = request(&handle, "POST", "/api/v1/plugins/demo-plugin/install").await;
    assert_eq!(
        status, 503,
        "native install needs an actual artifact: {response}"
    );
    assert_eq!(response["error"]["code"], "PLUGINS_UNAVAILABLE");
    assert_eq!(std::fs::read(marker_path).expect("after"), before);
    assert!(!directory.path().join("plugins/demo-plugin.so").exists());
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/strategy/catalog/plugin_normalization_business_test.go:13 TestCatalogPluginLifecyclePersistsSortedMetadataAndOperations
#[tokio::test]
async fn production_http_plugin_malformed_marker_does_not_change_artifact_or_history() {
    let directory = tempdir().expect("directory");
    let config = production_config(&directory);
    let mut cases = Vec::new();
    for (id, field, installed) in [
        ("install-bad-installation", "installation", false),
        ("install-bad-operations", "operations", false),
        ("uninstall-bad-installation", "installation", true),
        ("uninstall-bad-operations", "operations", true),
    ] {
        let mut value = marker(id);
        value["sourcePath"] = json!("source.bin");
        value[field] = json!("invalid metadata shape");
        let path = seed_marker(directory.path(), id, &value);
        let artifact = directory.path().join(format!("plugins/{id}.so"));
        if installed {
            std::fs::write(&artifact, ARTIFACT).expect("installed artifact");
        }
        cases.push((id, path, artifact, installed));
    }
    for (id, value) in [
        ("uninstall-null-marker", Value::Null),
        ("uninstall-array-marker", json!([])),
        ("uninstall-number-marker", json!(42)),
        ("uninstall-string-marker", json!("invalid marker")),
    ] {
        let path = seed_marker(directory.path(), id, &value);
        let artifact = directory.path().join(format!("plugins/{id}.so"));
        std::fs::write(&artifact, ARTIFACT).expect("installed artifact");
        cases.push((id, path, artifact, true));
    }
    let handle = start_product(config).await.expect("production");
    for (id, path, artifact, installed) in cases {
        let before = std::fs::read(&path).expect("before");
        let verb = if installed { "uninstall" } else { "install" };
        let (status, response) =
            request(&handle, "POST", &format!("/api/v1/plugins/{id}/{verb}")).await;
        assert_eq!(status, 500, "{id}: {response}");
        assert_eq!(response["error"]["code"], "INTERNAL_ERROR");
        assert_eq!(std::fs::read(path).expect("after marker"), before);
        assert_eq!(
            artifact.exists(),
            installed,
            "{id}: failed mutation changed artifact existence"
        );
        if installed {
            assert_eq!(
                std::fs::read(artifact).expect("retained artifact"),
                ARTIFACT
            );
        }
    }
    handle.shutdown().await.expect("shutdown");
}
