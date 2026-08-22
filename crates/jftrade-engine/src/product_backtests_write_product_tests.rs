use std::sync::Arc;

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
