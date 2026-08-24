use std::fs;

use tempfile::tempdir;

use super::*;

const SYSTEM_CONTROL_READ_PATHS: &[&str] = &[
    "/api/v1/system/futu-opend/install-guide",
    "/api/v1/system/real-trade-approvals",
    "/api/v1/system/real-trade-hard-stop-events",
    "/api/v1/system/real-trade-hard-stops",
    "/api/v1/system/real-trade-kill-switch",
    "/api/v1/system/real-trade-kill-switch-events",
    "/api/v1/system/real-trade-risk-events",
    "/api/v1/system/real-trade-risk-limits",
];

#[tokio::test]
async fn system_control_reads_are_authenticated_and_do_not_create_control_state() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let before = fs::read(&settings_path).expect("read settings");
    let token = "system-control-read-token-012345678901234567890";
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        token,
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let address = handle.startup_record().address;

    let (status, response) =
        request_json_with_status(address, "GET", SYSTEM_CONTROL_READ_PATHS[0], None, &[]).await;
    assert_eq!(status, 401);
    assert_eq!(response["ok"], false);

    let authorization = format!("Bearer {token}");
    for path in SYSTEM_CONTROL_READ_PATHS {
        let (status, response) = request_json_with_status(
            address,
            "GET",
            path,
            None,
            &[("Authorization", authorization.as_str())],
        )
        .await;
        assert_eq!(status, 200, "status for {path}: {response}");
        assert_eq!(response["ok"], true, "envelope for {path}");
        assert!(
            response["data"].is_object(),
            "projection for {path}: {response}"
        );
    }
    let guide = request_json_with_status(
        address,
        "GET",
        SYSTEM_CONTROL_READ_PATHS[0],
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await
    .1;
    assert_eq!(guide["data"]["settings"]["host"], "127.0.0.1");
    assert_eq!(guide["data"]["settings"]["minimumVersion"], "10.9.6908");

    handle.shutdown().await.expect("shutdown shadow");
    assert_eq!(fs::read(&settings_path).expect("read settings"), before);
    assert!(!directory.path().join("real-trade-control.json").exists());
}

#[tokio::test]
async fn storage_overview_matches_the_go_empty_projection_behind_authentication() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let token = "storage-overview-read-token-012345678901234567890";
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        token,
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let authorization = format!("Bearer {token}");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/storage/overview",
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(
        response["data"],
        json!({
            "pendingOutbox": [],
            "recentJobs": [],
            "recentAuditLogs": [],
            "recentExecutionCommands": [],
        })
    );
    handle.shutdown().await.expect("shutdown shadow");
}

#[tokio::test]
async fn runtime_dependencies_use_the_normalized_settings_node_candidate() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(
        &settings_path,
        serde_json::to_vec(&json!({
            "pineWorker": {
                "backtestWorkerLimit": 2,
                "instanceWorkerLimit": 10,
                "nodeBinaryPath": " ' node ' ",
            }
        }))
        .expect("encode settings"),
    )
    .expect("seed settings");
    let before = fs::read(&settings_path).expect("read settings");
    let token = "runtime-dependencies-token-0123456789012345678901";
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        token,
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let address = handle.startup_record().address;

    let (status, response) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/runtime-dependencies",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 401);
    assert_eq!(response["ok"], false);

    let authorization = format!("Bearer {token}");
    let (status, response) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/runtime-dependencies",
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await;
    assert_eq!(status, 200, "runtime dependency response: {response}");
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["allRequiredSatisfied"], true);
    let node = &response["data"]["dependencies"][0];
    assert_eq!(node["id"], "node");
    assert_eq!(node["status"], "ok");
    assert_eq!(node["minimumVersion"], "22.0.0");
    assert_eq!(node["configuredPath"], "node");
    assert_eq!(node["effectivePath"], "node");
    assert_eq!(node["attemptedPaths"], json!(["node"]));
    assert_eq!(node["source"], "settings");
    assert!(
        node["detectedVersion"].as_str().is_some_and(|version| {
            version
                .split('.')
                .next()
                .and_then(|major| major.parse::<u64>().ok())
                .is_some_and(|major| major >= 22)
        }),
        "detected Node version: {node}"
    );
    assert!(
        node["resolvedPath"]
            .as_str()
            .is_some_and(|path| !path.is_empty()),
        "resolved Node path: {node}"
    );

    handle.shutdown().await.expect("shutdown shadow");
    assert_eq!(fs::read(&settings_path).expect("read settings"), before);
}

#[tokio::test]
async fn system_status_matches_go_stable_fields_without_claiming_migration_ownership() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(
        &settings_path,
        br#"{"interfaces":{"liveWebSocketConnectionLimit":2}}"#,
    )
    .expect("seed settings");
    let before = fs::read(&settings_path).expect("read settings");
    let token = "system-status-token-012345678901234567890123456";
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        token,
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let address = handle.startup_record().address;

    let (status, response) =
        request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
    assert_eq!(status, 401);
    assert_eq!(response["ok"], false);

    let authorization = format!("Bearer {token}");
    let (status, response) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await;
    assert_eq!(status, 200, "system status response: {response}");
    let data = &response["data"];
    assert!(data.get("migrationOwner").is_none());
    assert_eq!(data["name"], "JFTrade");
    assert_eq!(data["apiPort"], address.port());
    assert_eq!(data["defaultBroker"], "futu");
    assert_eq!(data["defaultTradingEnvironment"], "SIMULATE");
    assert_eq!(data["message"], "JFTrade API adapter is running.");
    assert_eq!(
        data["persistence"],
        json!({
            "engine": "json",
            "databasePath": settings_path,
            "status": "ok",
            "migrated": true,
            "pendingMigrations": [],
            "tables": ["broker_integrations", "broker_accounts"],
            "checkedAt": data["persistence"]["checkedAt"].clone(),
        })
    );
    assert_eq!(
        data["observability"]["requests"],
        json!({
            "recentErrors": [],
            "recentSlowRequests": [],
            "openD": {"totalCalls": 0, "failedCalls": 0},
            "slowThresholdMs": 750,
            "minimumImportance": "low",
        })
    );
    assert_eq!(
        data["observability"]["live"],
        json!({
            "connected": 0,
            "limit": 2,
            "atLimit": false,
            "activeInstruments": [],
        })
    );
    assert_eq!(
        data["observability"]["marketdata"],
        json!({
            "status": "unavailable",
            "connected": false,
            "closed": false,
            "generation": 0,
            "activeCount": 0,
            "lastRefreshAt": null,
            "quoteRetryAt": null,
            "quoteFailures": 0,
            "quoteLastError": null,
            "streamRetryAt": null,
            "streamFailures": 0,
            "streamLastError": null,
        })
    );
    let broker: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/broker-descriptor.json"
    ))
    .expect("broker descriptor fixture");
    assert_eq!(data["broker"], broker);
    assert_eq!(data["observability"]["broker"], broker);
    assert_eq!(
        data["strategyRuntime"],
        json!({
            "status": "idle",
            "activeStrategies": 0,
            "supportsBacktestParity": true,
            "activeInstances": [],
        })
    );
    assert_eq!(
        data["observability"]["strategyRuntime"],
        data["strategyRuntime"]
    );
    assert_eq!(data["runtimeResources"]["count"], 11);
    let resource_ids = data["runtimeResources"]["items"]
        .as_array()
        .expect("runtime resource items")
        .iter()
        .map(|item| item["id"].as_str().expect("runtime resource id"))
        .collect::<Vec<_>>();
    assert_eq!(
        resource_ids,
        vec![
            "settings-file",
            "backtest-kline-db",
            "backtest-run-db",
            "strategy-runtime-db",
            "execution-orders-db",
            "adk-db",
            "adk-session-db",
            "adk-artifact-db",
            "watchlist-db",
            "research-db",
            "real-trade-control",
        ]
    );

    handle.shutdown().await.expect("shutdown shadow");
    assert_eq!(fs::read(&settings_path).expect("read settings"), before);
}

#[derive(Debug)]
struct FixtureMarketDataRuntimeStatusPort;

impl MarketDataRuntimeStatusPort for FixtureMarketDataRuntimeStatusPort {
    fn snapshot(&self) -> MarketDataRuntimeState {
        MarketDataRuntimeState {
            generation: 8,
            active_count: 2,
            quote_failures: 3,
            quote_last_error: Some(" quote unavailable ".to_owned()),
            ..MarketDataRuntimeState::default()
        }
    }
}

#[tokio::test]
async fn system_status_uses_only_the_typed_market_data_runtime_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_runtime_status_port(Arc::new(FixtureMarketDataRuntimeStatusPort));
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "system status response: {response}");
    assert_eq!(
        response["data"]["observability"]["marketdata"],
        json!({
            "status": "degraded",
            "connected": false,
            "closed": false,
            "generation": 8,
            "activeCount": 2,
            "lastRefreshAt": null,
            "quoteRetryAt": null,
            "quoteFailures": 3,
            "quoteLastError": "quote unavailable",
            "streamRetryAt": null,
            "streamFailures": 0,
            "streamLastError": null,
        })
    );
    handle.shutdown().await.expect("shutdown product");
}

#[test]
fn system_status_live_projection_uses_shared_transport_metrics() {
    let metrics = Arc::new(LiveConnectionMetrics::new(2));
    let first = metrics.try_acquire().expect("first connection");
    first.set_active_instruments(&[
        " us.aapl ".to_owned(),
        "HK.00700".to_owned(),
        "US.AAPL".to_owned(),
    ]);
    assert_eq!(
        live_observability(&metrics),
        json!({
            "connected": 1,
            "limit": 2,
            "atLimit": false,
            "activeInstruments": ["HK.00700", "US.AAPL"],
        })
    );

    let second = metrics.try_acquire().expect("second connection");
    second.set_active_instruments(&["CN.600000".to_owned(), "us.aapl".to_owned()]);
    assert_eq!(
        live_observability(&metrics),
        json!({
            "connected": 2,
            "limit": 2,
            "atLimit": true,
            "activeInstruments": ["CN.600000", "HK.00700", "US.AAPL"],
        })
    );
    drop(first);
    assert_eq!(
        live_observability(&metrics)["activeInstruments"],
        json!(["CN.600000", "US.AAPL"])
    );
    drop(second);
}
