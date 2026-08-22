use std::sync::Arc;

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_watchlist_write_port::{
    WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
};
use super::*;

#[derive(Debug)]
struct FixtureWatchlistWritePort;

impl WatchlistWritePort for FixtureWatchlistWritePort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        Ok(json!({
            "accepted": true,
            "route": mutation.value["route"].clone(),
        }))
    }
}

#[tokio::test]
async fn watchlist_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_write_port(Arc::new(FixtureWatchlistWritePort));
    let handle = start_product(config)
        .await
        .expect("start watchlist product");
    assert_eq!(handle.startup_record().owned_routes, 56);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/watchlist/groups" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/watchlist/groups",
        Some(r#"{"name":"Growth"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["route"], "create-group");
    handle.shutdown().await.expect("shutdown watchlist product");
}
