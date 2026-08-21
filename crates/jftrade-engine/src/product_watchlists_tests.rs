use super::*;
use serde_json::Value;
use std::sync::Arc;

#[derive(Debug)]
struct FixtureRemoteWatchlistPort;
impl RemoteWatchlistSnapshotPort for FixtureRemoteWatchlistPort {
    fn read(&self, _query: &str) -> Result<Value, RemoteWatchlistSnapshotError> {
        Ok(serde_json::json!({"groups": [], "source": "fixture"}))
    }
}

#[derive(Debug)]
struct FailingRemoteWatchlistPort;
impl RemoteWatchlistSnapshotPort for FailingRemoteWatchlistPort {
    fn read(&self, _query: &str) -> Result<Value, RemoteWatchlistSnapshotError> {
        Err(RemoteWatchlistSnapshotError::Unavailable(
            "Go remote watchlist unavailable".to_owned(),
        ))
    }
}

#[tokio::test]
async fn remote_watchlist_read_route_matches_fixture_in_cutover_only() {
    let dir = tempdir().expect("temporary directory");
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        dir.path().join("settings.json"),
    )
    .expect("config")
    .with_remote_watchlist_snapshot_port(Arc::new(FixtureRemoteWatchlistPort));
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/watchlists/remote",
        None,
    )
    .await;
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["source"], "fixture");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn remote_watchlist_read_route_fails_closed_when_unavailable() {
    let dir = tempdir().expect("temporary directory");
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        dir.path().join("settings.json"),
    )
    .expect("config")
    .with_remote_watchlist_snapshot_port(Arc::new(FailingRemoteWatchlistPort));
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/watchlists/remote",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "WATCHLIST_UNAVAILABLE");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn remote_watchlist_read_route_is_not_registered_without_snapshot_port() {
    let dir = tempdir().expect("temporary directory");
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        dir.path().join("settings.json"),
    )
    .expect("config");
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/watchlists/remote",
        None,
    )
    .await;
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
