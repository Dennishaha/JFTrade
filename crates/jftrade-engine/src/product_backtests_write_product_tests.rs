use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde_json::json;
use tempfile::tempdir;

use super::super::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};
use super::*;

#[derive(Debug)]
struct FixtureBacktestsWritePort;

impl BacktestsWritePort for FixtureBacktestsWritePort {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        Ok(match input {
            BacktestsWriteInput::Start { .. } => BacktestsWritePortResult::Data(json!({
                "id": "fixture-run",
                "status": "queued",
                "message": "backtest queued",
            })),
            BacktestsWriteInput::Sync { .. } => BacktestsWritePortResult::Data(json!({
                "taskId": "fixture-task",
                "status": "running",
            })),
            BacktestsWriteInput::CancelSync { .. } => BacktestsWritePortResult::SyncCancelled(true),
            BacktestsWriteInput::Delete { .. } => {
                BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Deleted)
            }
        })
    }
}

#[derive(Debug)]
struct SequencedBacktestsWritePort {
    responses: Mutex<VecDeque<Result<BacktestsWritePortResult, BacktestsWritePortError>>>,
    calls: Mutex<Vec<BacktestsWriteInput>>,
}

impl SequencedBacktestsWritePort {
    fn new(
        responses: impl IntoIterator<Item = Result<BacktestsWritePortResult, BacktestsWritePortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }
}

impl BacktestsWritePort for SequencedBacktestsWritePort {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        self.calls
            .lock()
            .expect("backtests write product call lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("backtests write product response lock")
            .pop_front()
            .expect("backtests write product response")
    }
}

#[tokio::test]
async fn backtests_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_backtests_write_port(Arc::new(FixtureBacktestsWritePort));
    let handle = start_product(config)
        .await
        .expect("start backtests product");
    assert_eq!(handle.startup_record().owned_routes, 52);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/backtests" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["status"], "queued");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "DELETE",
        "/api/v1/backtests/sync/fixture-task",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["status"], "cancelled");
    handle.shutdown().await.expect("shutdown backtests product");
}

#[tokio::test]
async fn backtests_write_product_replays_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"backtests-write\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings before replay");
    let port = Arc::new(SequencedBacktestsWritePort::new([
        Err(BacktestsWritePortError::Unavailable(
            "fixture backtests writer unavailable".to_owned(),
        )),
        Ok(BacktestsWritePortResult::Data(json!({
            "id": "fixture-run",
            "status": "queued",
            "message": "backtest queued",
        }))),
        Err(BacktestsWritePortError::Failed(
            "fixture sync failed".to_owned(),
        )),
        Ok(BacktestsWritePortResult::SyncCancelled(false)),
        Ok(BacktestsWritePortResult::RunDeleted(
            BacktestsWriteDeleteResult::Deleted,
        )),
        Ok(BacktestsWritePortResult::RunDeleted(
            BacktestsWriteDeleteResult::Missing,
        )),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_backtests_write_port(port.clone());
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config)
        .await
        .expect("start backtests product");
    assert_eq!(handle.startup_record().owned_routes, 52);
    let address = handle.startup_record().address;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/backtests"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "backtests-write-fixture"),
    ];

    let unauthorized = request_json_with_status(
        address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &[],
    )
    .await;
    assert_eq!(unauthorized.0, 401);
    let csrf_missing = request_json_with_status(
        address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/backtests"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.0, 403);

    let unavailable = request_json_with_status(
        address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(unavailable.0, 503);
    assert_eq!(
        unavailable.1["error"]["code"],
        "BACKTESTS_WRITE_UNAVAILABLE"
    );

    let started = request_json_with_status(
        address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(started.0, 200);
    assert_eq!(started.1["data"]["status"], "queued");

    let sync_failed = request_json_with_status(
        address,
        "POST",
        "/api/v1/backtests/sync",
        Some(r#"{"market":"US","symbol":"AAPL"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(sync_failed.0, 500);
    assert_eq!(sync_failed.1["error"]["code"], "SYNC_FAILED");
    assert_eq!(sync_failed.1["error"]["message"], "fixture sync failed");

    let cancel_missing = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/backtests/sync/fixture-task",
        None,
        &browser_headers,
    )
    .await;
    assert_eq!(cancel_missing.0, 404);
    assert_eq!(cancel_missing.1["error"]["code"], "NOT_FOUND");

    let deleted = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/backtests/fixture-run",
        None,
        &browser_headers,
    )
    .await;
    assert_eq!(deleted.0, 200);
    assert_eq!(deleted.1["data"]["deleted"], true);

    let repeated_delete = request_json_with_status(
        address,
        "DELETE",
        "/api/v1/backtests/fixture-run",
        None,
        &browser_headers,
    )
    .await;
    assert_eq!(repeated_delete.0, 404);
    assert_eq!(repeated_delete.1["error"]["code"], "NOT_FOUND");

    {
        let calls = port.calls.lock().expect("backtests write product calls");
        assert_eq!(calls.len(), 6);
        assert!(matches!(calls[0], BacktestsWriteInput::Start { .. }));
        assert!(matches!(calls[1], BacktestsWriteInput::Start { .. }));
        assert!(matches!(calls[2], BacktestsWriteInput::Sync { .. }));
        assert!(matches!(calls[3], BacktestsWriteInput::CancelSync { .. }));
        assert!(matches!(calls[4], BacktestsWriteInput::Delete { .. }));
        assert!(matches!(calls[5], BacktestsWriteInput::Delete { .. }));
    }
    handle.shutdown().await.expect("shutdown backtests product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after replay"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_backtests_write_port(Arc::new(FixtureBacktestsWritePort));
    let mut restarted_config = restarted_config;
    restarted_config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let restarted = start_product(restarted_config)
        .await
        .expect("restart backtests product");
    let restarted_response = request_json_with_status(
        restarted.startup_record().address,
        "POST",
        "/api/v1/backtests",
        Some(r#"{"definitionId":"def-1"}"#),
        &browser_headers,
    )
    .await;
    assert_eq!(restarted_response.0, 200);
    assert_eq!(restarted_response.1["data"]["status"], "queued");
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted backtests product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}
