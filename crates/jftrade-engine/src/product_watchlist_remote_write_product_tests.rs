use std::sync::Arc;

use serde_json::{Value, json};

use super::super::product_watchlist_remote_write_port::{
    RemoteWatchlistWriteAction, RemoteWatchlistWritePort, RemoteWatchlistWritePortError,
    RemoteWatchlistWriteResolution,
};
use super::*;

#[derive(Debug)]
struct FixtureRemoteWatchlistWritePort;

impl RemoteWatchlistWritePort for FixtureRemoteWatchlistWritePort {
    fn resolve(
        &self,
        broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
        Ok(RemoteWatchlistWriteResolution {
            broker_id: broker_id.unwrap_or("futu").to_owned(),
            security_firm: "fixture".to_owned(),
            capability: "available".to_owned(),
            selection_reason: "fixture".to_owned(),
        })
    }

    fn apply(
        &self,
        _resolution: &RemoteWatchlistWriteResolution,
        _action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
        Ok(Some(json!({"accepted": true})))
    }
}

#[tokio::test]
async fn remote_watchlist_write_route_registers_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/watchlists/remote",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_remote_watchlist_write_port(Arc::new(FixtureRemoteWatchlistWritePort));
    let handle = start_product(config)
        .await
        .expect("start watchlist product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/watchlists/remote" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/watchlists/remote?brokerId=futu",
        Some(r#"{"groupName":"Favorites"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["provider"]["brokerId"], "futu");
    handle.shutdown().await.expect("shutdown watchlist product");
}
