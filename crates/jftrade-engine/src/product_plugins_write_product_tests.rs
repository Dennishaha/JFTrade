use super::*;
use crate::product::product_plugins_write_port::{
    PluginWriteOperation, PluginWritePort, PluginWritePortError,
};
use serde_json::{Value, json};
use std::sync::Arc;
use tempfile::tempdir;

#[derive(Debug)]
struct FixturePluginWritePort {
    mode: &'static str,
}

impl PluginWritePort for FixturePluginWritePort {
    fn mutate(
        &self,
        operation: PluginWriteOperation,
        plugin_id: &str,
    ) -> Result<Value, PluginWritePortError> {
        if self.mode == "missing" {
            return Err(PluginWritePortError::Internal(
                "strategy resource not found".to_owned(),
            ));
        }
        if self.mode == "unavailable" {
            return Err(PluginWritePortError::Unavailable(
                "plugin write fixture is unavailable".to_owned(),
            ));
        }
        let installed = matches!(operation, PluginWriteOperation::Install);
        Ok(json!({
            "operationId": "fixture-operation",
            "pluginId": plugin_id,
            "status": "SUCCEEDED",
            "phase": if installed { "installed" } else { "uninstalled" },
            "progress": 100,
            "message": if installed {
                "plugin metadata installed"
            } else {
                "plugin metadata uninstalled"
            },
            "targetDir": "plugins",
            "installPath": "plugins/alpha.so",
            "startedAt": "2026-08-22T00:00:00Z",
            "updatedAt": "2026-08-22T00:00:00Z",
            "completedAt": "2026-08-22T00:00:00Z",
            "error": null,
        }))
    }
}

#[tokio::test]
async fn plugins_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_plugin_write_port(Arc::new(FixturePluginWritePort { mode: "success" }));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route == "POST /api/v1/plugins/{pluginId}/install")
    );

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/plugins/alpha/install",
        Some("arbitrary body is ignored"),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["operation"]["pluginId"], "alpha");
    assert_eq!(response["data"]["operation"]["phase"], "installed");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/plugins/alpha/uninstall",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["operation"]["phase"], "uninstalled");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn plugins_write_routes_preserve_go_error_and_default_route_isolation() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_plugin_write_port(Arc::new(FixturePluginWritePort { mode: "missing" }));
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/plugins/alpha/install",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 500);
    assert_eq!(response["error"]["code"], "INTERNAL_ERROR");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/plugins/%20/install",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(response["error"]["code"], "BAD_REQUEST");
    handle.shutdown().await.expect("shutdown product");

    let isolated =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("isolated config");
    let handle = start_product(isolated)
        .await
        .expect("start isolated product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/plugins/alpha/install",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown isolated product");
}
