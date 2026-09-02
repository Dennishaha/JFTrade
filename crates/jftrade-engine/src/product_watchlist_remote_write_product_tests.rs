use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

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

#[tokio::test]
async fn remote_watchlist_write_product_replays_failure_recovery_restart_without_local_state() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"remote-watchlist\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read seeded settings");
    let port = Arc::new(SequencedRemoteWatchlistWritePort::default());
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_remote_watchlist_write_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let path = "/api/v1/watchlists/remote?brokerId=futu&accountId=acct-1";
    let body = r#"{"groupName":"Favorites","op":1}"#;

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        path,
        Some(body),
        &[],
    )
    .await;
    assert_eq!(status, 502);
    assert_eq!(response["error"]["code"], "BROKER_FEATURE_FAILED");

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        path,
        Some(body),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["provider"]["brokerId"], "futu");
    assert_eq!(port.apply_calls.load(Ordering::SeqCst), 2);
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings"),
        settings_before
    );

    let restarted =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("restarted config")
            .with_remote_watchlist_write_port(Arc::new(FixtureRemoteWatchlistWritePort));
    let handle = start_product(restarted).await.expect("restart product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        path,
        Some(body),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["accepted"], true);
    handle.shutdown().await.expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read restarted settings"),
        settings_before
    );
}

#[derive(Debug, Default)]
struct SequencedRemoteWatchlistWritePort {
    apply_calls: AtomicUsize,
}

impl RemoteWatchlistWritePort for SequencedRemoteWatchlistWritePort {
    fn resolve(
        &self,
        broker_id: Option<&str>,
        account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
        Ok(RemoteWatchlistWriteResolution {
            broker_id: broker_id.unwrap_or("futu").to_owned(),
            security_firm: "fixture".to_owned(),
            capability: "available".to_owned(),
            selection_reason: if account_id.is_some() {
                "explicit_broker_account".to_owned()
            } else {
                "fixture".to_owned()
            },
        })
    }

    fn apply(
        &self,
        _resolution: &RemoteWatchlistWriteResolution,
        _action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
        if self.apply_calls.fetch_add(1, Ordering::SeqCst) == 0 {
            return Err(RemoteWatchlistWritePortError::Internal(
                "remote broker unavailable".to_owned(),
            ));
        }
        Ok(Some(json!({"accepted": true})))
    }
}
