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

/// Fixture-only stand-in for the OpenD session coordinator. It owns the same
/// generation/stream boundary that the real coordinator feeds into the
/// market-data recorder, while leaving socket ownership outside this product
/// test. Snapshot polling remains an injected, synchronous operation.
#[derive(Debug)]
struct FixtureOpenDSessionCoordinator {
    recorder: Arc<jftrade_marketdata::MarketDataRuntimeRecorder>,
    generation: u64,
}

impl FixtureOpenDSessionCoordinator {
    fn connect(
        recorder: Arc<jftrade_marketdata::MarketDataRuntimeRecorder>,
        demand: &[jftrade_marketdata::InstrumentRef],
    ) -> Self {
        let instruments = demand.iter().cloned().map(|instrument| {
            instrument
                .normalize()
                .expect("fixture instrument")
                .instrument_id()
        });
        let generation = recorder.reconcile(instruments);
        assert!(recorder.record_stream_connected(generation));
        Self {
            recorder,
            generation,
        }
    }

    fn poll_snapshot(
        &self,
        cache: &mut jftrade_marketdata::TickCache,
        demand: &[jftrade_marketdata::InstrumentRef],
        now: jftrade_kernel::WireTimestamp,
        query: impl FnOnce(&[String]) -> Result<Vec<jftrade_marketdata::Tick>, String>,
    ) -> jftrade_marketdata::SnapshotPollOutcome {
        jftrade_marketdata::SnapshotPollExecutor::default().execute(
            &self.recorder,
            cache,
            demand,
            self.generation,
            now,
            query,
        )
    }

    fn fail_stream(&self, now: jftrade_kernel::WireTimestamp, reason: &str) {
        assert!(
            self.recorder
                .record_stream_failure(self.generation, now, reason)
        );
    }

    fn reconnect(&mut self) {
        self.generation = self.recorder.reconfigure();
        assert!(self.recorder.record_stream_connected(self.generation));
    }
}

fn fixture_tick(
    instrument_id: &str,
    price: &str,
    observed_at_ms: i64,
    provider_generation: u64,
) -> jftrade_marketdata::Tick {
    jftrade_marketdata::Tick {
        instrument_id: instrument_id.to_owned(),
        price: price.parse().expect("fixture price"),
        volume: "10.5".parse().expect("fixture volume"),
        volume_delta: None,
        snapshot: None,
        observed_at_ms,
        provider_generation,
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercore/desktop_token_test.go:14 TestDesktopTokenMiddlewareProtectsHTTPAndWebSocket
// Parity: go:452dea11:internal/app/apiserver/servercore/settings_security_test.go:14 TestWebAccessSettingsDefaultToDesktopOnly
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

// Parity: go:452dea11:internal/app/apiserver/databaseguard/groups_test.go:35 TestGroupsKeepRoutesAvailableWhenDatabasesAreHealthy
#[tokio::test]
async fn production_system_status_reports_real_database_lease_and_schema_state() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    product_data_management::initialize_production_databases(&settings_path)
        .expect("initialize production databases");
    let token = "production-system-status-token-012345678901234567890";
    let mut config = ProductConfig::new(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        AccessPolicy::desktop(Some(token.to_owned())),
    )
    .expect("product config");
    config.capabilities = ProductCapabilities::all();
    config.production = true;
    let handle = start_product(config)
        .await
        .expect("start production product");
    let authorization = format!("Bearer {token}");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await;
    assert_eq!(status, 200, "system status response: {response}");
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["persistence"]["engine"], "sqlite");
    assert_eq!(response["data"]["persistence"]["status"], "ok");
    assert_eq!(response["data"]["persistence"]["migrated"], true);
    assert_eq!(
        response["data"]["persistence"]["tables"]
            .as_array()
            .map(Vec::len),
        Some(9)
    );

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/storage/overview",
        None,
        &[("Authorization", authorization.as_str())],
    )
    .await;
    assert_eq!(status, 200, "storage overview response: {response}");
    assert_eq!(response["ok"], true);
    for key in [
        "pendingOutbox",
        "recentJobs",
        "recentAuditLogs",
        "recentExecutionCommands",
    ] {
        assert!(
            response["data"][key].is_array(),
            "{key} projection: {response}"
        );
    }
    handle
        .shutdown()
        .await
        .expect("shutdown production product");
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

// Parity: go:452dea11:internal/app/apiserver/server_test.go:85 TestStartDesktopDoesNotMutatePersistedWebAccessSettings
// Parity: go:452dea11:internal/app/apiserver/runtime/resources_test.go:78 TestRuntimeResourceSummaryIncludesCountAndItems
// Parity: go:452dea11:internal/system/service_test.go:13 TestStatusDefaultsAndInjectedSummaries
// Parity: go:452dea11:internal/app/apiserver/servercoretest/system_routes_test.go:12 TestSystemStatusEndpointReturnsStatus
// Parity: go:452dea11:internal/api/system/status_mapper_test.go:11 TestSystemStatusTransportMapperPreservesDomainJSON
#[tokio::test]
// Parity: go:452dea11:internal/assistant/assembly/application_adapter_boundaries_test.go:71 TestApplicationAdapterUsesConfiguredRuntimeAndSettings
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
        "../../../tests/fixtures/compatibility/api-transport/broker-descriptor.json"
    ))
    .expect("broker descriptor fixture");
    // The Go contract only requires non-empty build identity fields; the Rust
    // projection fills them from build-time environment with dev/unknown
    // fallbacks, so the assertion is non-emptiness rather than a fixed value.
    assert!(
        data["build"]["version"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );
    assert!(
        data["build"]["commit"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );
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
    let resource_ids = data["runtimeResources"]["items"]
        .as_array()
        .expect("runtime resource items")
        .iter()
        .map(|item| item["id"].as_str().expect("runtime resource id"))
        .collect::<Vec<_>>();
    assert_eq!(data["runtimeResources"]["count"], resource_ids.len());
    assert_eq!(
        resource_ids,
        vec![
            "settings-file",
            "backtest-kline-db",
            "backtest-run-db",
            "strategy-catalog",
            "strategy-designs",
            "strategy-runtime-db",
            "execution-orders-db",
            "watchlist-db",
            "research-db",
            "real-trade-control",
            "adk-db",
            "adk-session-db",
            "adk-artifact-db",
            "adk-secrets",
            "adk-skills-dir",
            "exchange-calendar-dir",
            "strategy-plugin-dir",
        ]
    );

    handle.shutdown().await.expect("shutdown shadow");
    assert_eq!(fs::read(&settings_path).expect("read settings"), before);
}

#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/servercoretest/contract_test.go:13 TestContractSystemStatus
// The status contract keeps the ok/timestamp envelope, the required top-level
// fields, and the trading `execution-orders-db` runtime resource together in
// one response instead of spreading them across separate fixtures.
async fn system_status_contract_envelope_exposes_required_fields_and_trading_resource() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
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
    assert_eq!(response["ok"], true);
    assert!(
        response["timestamp"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );
    let data = &response["data"];
    for field in [
        "name",
        "apiPort",
        "defaultBroker",
        "build",
        "persistence",
        "runtimeResources",
    ] {
        assert!(data.get(field).is_some(), "missing required field {field}");
    }
    let resource_ids = data["runtimeResources"]["items"]
        .as_array()
        .expect("runtime resource items")
        .iter()
        .map(|item| item["id"].as_str().expect("runtime resource id"))
        .collect::<Vec<_>>();
    assert!(
        resource_ids.contains(&"execution-orders-db"),
        "trading execution-orders resource must be listed: {resource_ids:?}"
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
// Parity: go:452dea11:internal/app/apiserver/servercoretest/system_routes_test.go:12 TestSystemStatusEndpointReturnsStatus
async fn system_status_uses_only_the_typed_market_data_runtime_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
    let generation = recorder.reconcile([" US.AAPL ".to_owned(), "HK.00700".to_owned()]);
    let now = "2026-08-24T09:00:00+08:00".parse().expect("timestamp");
    assert!(recorder.record_poll_started(generation, now));
    assert!(recorder.record_quote_failure(generation, now, " quote unavailable "));
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_runtime_status_port(recorder);
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
            "generation": 1,
            "activeCount": 2,
            "lastRefreshAt": "2026-08-24T01:00:00Z",
            "quoteRetryAt": "2026-08-24T01:00:05Z",
            "quoteFailures": 1,
            "quoteLastError": "quote unavailable",
            "streamRetryAt": null,
            "streamFailures": 0,
            "streamLastError": null,
        })
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn product_runtime_uses_the_router_owned_market_data_recorder() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
    let generation = recorder.reconcile(["US.AAPL".to_owned()]);
    let now = "2026-08-24T09:00:00+08:00".parse().expect("timestamp");
    assert!(recorder.record_quote_failure(generation, now, " quote unavailable "));
    let product =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let config = crate::product_runtime::ProductRuntimeConfig::desktop(
        product,
        crate::product_runtime::DesktopRetainedRuntimeConfig::default(),
    )
    .expect("runtime config")
    .with_market_data_runtime_recorder(Arc::clone(&recorder));
    let runtime = crate::product_runtime::start_product_runtime(config)
        .await
        .expect("start product runtime");
    let response = request_json_with_status(
        runtime.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        response["data"]["observability"]["marketdata"]["activeCount"],
        1
    );
    assert_eq!(
        response["data"]["observability"]["marketdata"]["quoteLastError"],
        "quote unavailable"
    );

    let next_generation = recorder.reconcile(["HK.00700".to_owned(), "US.AAPL".to_owned()]);
    assert_eq!(next_generation, generation + 1);
    let response = request_json_with_status(
        runtime.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        response["data"]["observability"]["marketdata"]["activeCount"],
        2
    );
    assert_eq!(
        response["data"]["observability"]["marketdata"]["generation"],
        generation + 1
    );
    runtime.shutdown().await.expect("shutdown product runtime");
}

#[tokio::test]
async fn product_runtime_keeps_provider_router_and_status_recorder_shared() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let router = Arc::new(std::sync::Mutex::new(
        jftrade_marketdata::ProviderRouter::new(2),
    ));
    {
        let mut router_guard = router.lock().expect("router lock");
        router_guard
            .register(
                jftrade_integration_futu::provider_descriptor(),
                jftrade_marketdata::HealthStatus {
                    connected: true,
                    readiness: jftrade_marketdata::ProviderReadiness::Ready,
                    stream_mode: "streaming".to_owned(),
                    ..jftrade_marketdata::HealthStatus::default()
                },
            )
            .expect("register Futu fixture provider");
        router_guard
            .activate("futu", jftrade_marketdata::ActivationMode::Explicit)
            .expect("activate fixture provider");
        router_guard
            .acquire_demand(
                "system-status-test",
                [jftrade_marketdata::InstrumentRef {
                    channel: "SNAPSHOT".to_owned(),
                    market: "US".to_owned(),
                    symbol: "AAPL".to_owned(),
                    interval: None,
                }],
                false,
                0,
            )
            .expect("acquire fixture demand");
    }
    let product =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let config = crate::product_runtime::ProductRuntimeConfig::desktop(
        product,
        crate::product_runtime::DesktopRetainedRuntimeConfig::default(),
    )
    .expect("runtime config")
    .with_market_data_router(Arc::clone(&router));
    let runtime = crate::product_runtime::start_product_runtime(config)
        .await
        .expect("start product runtime");
    assert!(runtime.market_data_router().is_some());
    let response = request_json_with_status(
        runtime.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        response["data"]["observability"]["marketdata"]["activeCount"],
        1
    );

    router
        .lock()
        .expect("router lock")
        .acquire_demand(
            "system-status-test-2",
            [jftrade_marketdata::InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "HK".to_owned(),
                symbol: "00700".to_owned(),
                interval: None,
            }],
            false,
            1,
        )
        .expect("acquire second fixture demand");
    let response = request_json_with_status(
        runtime.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        response["data"]["observability"]["marketdata"]["activeCount"],
        2
    );
    runtime.shutdown().await.expect("shutdown product runtime");
}

#[tokio::test]
async fn system_status_reflects_fixture_opend_poll_and_reconnect_lifecycle() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
    let demand = vec![jftrade_marketdata::InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        interval: None,
    }];
    let mut coordinator = FixtureOpenDSessionCoordinator::connect(Arc::clone(&recorder), &demand);
    let mut cache = jftrade_marketdata::TickCache::new(2);
    let first_now: jftrade_kernel::WireTimestamp = "2026-08-25T09:00:00+08:00"
        .parse()
        .expect("first poll timestamp");
    let first_tick = fixture_tick(
        "US.AAPL",
        "189.12345678",
        first_now.unix_millis().expect("first timestamp millis"),
        coordinator.generation,
    );
    assert_eq!(
        coordinator.poll_snapshot(&mut cache, &demand, first_now, move |requested| {
            assert_eq!(requested, &["US.AAPL"]);
            Ok(vec![first_tick])
        }),
        jftrade_marketdata::SnapshotPollOutcome::Applied {
            requested: 1,
            inserted: 1,
        }
    );
    assert!(matches!(
        cache.lookup_for_generation(
            "US.AAPL",
            first_now.unix_millis().expect("first timestamp millis"),
            1_500,
            coordinator.generation,
        ),
        jftrade_marketdata::CacheLookup::Fresh(_)
    ));

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_runtime_status_port(recorder.clone());
    let handle = start_product(config).await.expect("start product");
    let status = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        status["data"]["observability"]["marketdata"],
        json!({
            "status": "connected",
            "connected": true,
            "closed": false,
            "generation": 1,
            "activeCount": 1,
            "lastRefreshAt": "2026-08-25T01:00:00Z",
            "quoteRetryAt": null,
            "quoteFailures": 0,
            "quoteLastError": null,
            "streamRetryAt": null,
            "streamFailures": 0,
            "streamLastError": null,
        })
    );

    coordinator.fail_stream(first_now, "peer EOF");
    let degraded = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        degraded["data"]["observability"]["marketdata"]["status"],
        "degraded"
    );
    assert_eq!(
        degraded["data"]["observability"]["marketdata"]["streamFailures"],
        1
    );
    assert_eq!(
        degraded["data"]["observability"]["marketdata"]["streamLastError"],
        "peer EOF"
    );

    coordinator.reconnect();
    let second_now: jftrade_kernel::WireTimestamp = "2026-08-25T09:00:02+08:00"
        .parse()
        .expect("second poll timestamp");
    let second_tick = fixture_tick(
        "US.AAPL",
        "190.00",
        second_now.unix_millis().expect("second timestamp millis"),
        coordinator.generation,
    );
    assert_eq!(
        coordinator.poll_snapshot(&mut cache, &demand, second_now, move |_| Ok(vec![
            second_tick
        ])),
        jftrade_marketdata::SnapshotPollOutcome::Applied {
            requested: 1,
            inserted: 1,
        }
    );
    assert!(matches!(
        cache.lookup_for_generation(
            "US.AAPL",
            second_now.unix_millis().expect("second timestamp millis"),
            1_500,
            coordinator.generation,
        ),
        jftrade_marketdata::CacheLookup::Fresh(_)
    ));

    let recovered = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        recovered["data"]["observability"]["marketdata"],
        json!({
            "status": "connected",
            "connected": true,
            "closed": false,
            "generation": 2,
            "activeCount": 1,
            "lastRefreshAt": "2026-08-25T01:00:02Z",
            "quoteRetryAt": null,
            "quoteFailures": 0,
            "quoteLastError": null,
            "streamRetryAt": null,
            "streamFailures": 0,
            "streamLastError": null,
        })
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn product_runtime_uses_the_lifecycle_owned_strategy_registry() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let registry = Arc::new(jftrade_strategy::StrategyRuntimeRegistry::default());
    registry
        .upsert(jftrade_strategy::RuntimeInstanceSummary {
            instance_id: " runtime-1 ".to_owned(),
            definition_name: " Momentum ".to_owned(),
            actual_state: jftrade_strategy::RuntimeState::Running,
            active_symbols: vec![" US.TSLA ".to_owned()],
            last_closed_kline_at: None,
            last_signal_at: Some("2026-08-24T09:03:00+08:00".parse().expect("timestamp")),
            last_order_at: None,
            last_error_at: None,
            last_error: None,
            updated_at: None,
        })
        .expect("runtime instance");
    let product =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let config = crate::product_runtime::ProductRuntimeConfig::desktop(
        product,
        crate::product_runtime::DesktopRetainedRuntimeConfig::default(),
    )
    .expect("runtime config")
    .with_strategy_runtime_registry(Arc::clone(&registry));
    let runtime = crate::product_runtime::start_product_runtime(config)
        .await
        .expect("start product runtime");
    let response = request_json_with_status(
        runtime.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(
        response["data"]["observability"]["strategyRuntime"]["activeStrategies"],
        1
    );
    assert_eq!(
        response["data"]["observability"]["strategyRuntime"]["activeInstances"][0]["instanceId"],
        "runtime-1"
    );

    registry
        .upsert(jftrade_strategy::RuntimeInstanceSummary {
            instance_id: " runtime-2 ".to_owned(),
            definition_name: " Mean Reversion ".to_owned(),
            actual_state: jftrade_strategy::RuntimeState::Recovering,
            active_symbols: Vec::new(),
            last_closed_kline_at: None,
            last_signal_at: None,
            last_order_at: None,
            last_error_at: Some("2026-08-24T09:04:00+08:00".parse().expect("timestamp")),
            last_error: Some(" worker exited ".to_owned()),
            updated_at: None,
        })
        .expect("recovering runtime instance");
    let response = request_json_with_status(
        runtime.startup_record().address,
        "GET",
        "/api/v1/system/status",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(response["data"]["strategyRuntime"]["activeStrategies"], 2);
    assert_eq!(
        response["data"]["strategyRuntime"]["activeInstances"][1]["instanceId"],
        "runtime-2"
    );
    runtime.shutdown().await.expect("shutdown product runtime");
}

#[tokio::test]
async fn system_status_uses_only_the_typed_strategy_runtime_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let registry = Arc::new(jftrade_strategy::StrategyRuntimeRegistry::default());
    registry
        .upsert(jftrade_strategy::RuntimeInstanceSummary {
            instance_id: " strategy-1 ".to_owned(),
            definition_name: " Momentum ".to_owned(),
            actual_state: jftrade_strategy::RuntimeState::Running,
            active_symbols: vec!["US.TSLA".to_owned(), " US.AAPL ".to_owned()],
            last_closed_kline_at: None,
            last_signal_at: Some("2026-08-24T09:03:00+08:00".parse().expect("timestamp")),
            last_order_at: None,
            last_error_at: None,
            last_error: None,
            updated_at: None,
        })
        .expect("runtime instance");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_runtime_status_port(registry);
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
    let expected = json!({
        "status": "active",
        "activeStrategies": 1,
        "supportsBacktestParity": true,
        "activeInstances": [{
            "instanceId": "strategy-1",
            "definitionName": "Momentum",
            "actualStatus": "running",
            "activeSymbols": ["US.AAPL", "US.TSLA"],
            "lastSignalAt": "2026-08-24T01:03:00Z",
        }],
    });
    assert_eq!(response["data"]["strategyRuntime"], expected);
    assert_eq!(
        response["data"]["observability"]["strategyRuntime"],
        expected
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/servercore/live_heartbeat_boundaries_test.go:27 TestLiveHeartbeatActiveInstrumentDeduplicationBoundaries
// Parity: go:452dea11:internal/app/apiserver/status/status_test.go:12 TestLiveStatsSortsActiveInstruments
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

/// OpenD health is owned by the production runtime in Rust; the failure this
/// fixture forces is the same 503 the router records for the request
/// observability window.
#[derive(Debug)]
struct UnavailableSystemReadPort;

impl SystemReadSnapshotPort for UnavailableSystemReadPort {
    fn read(&self, _path: &str) -> Result<serde_json::Value, SystemReadSnapshotError> {
        Err(SystemReadSnapshotError::Unavailable(
            "system read snapshot is unavailable".to_owned(),
        ))
    }
}

/// Status-only calendar source: the manager projection is the projection
/// under test, so no schedule fetch is ever required.
#[derive(Debug)]
struct StatusOnlyCalendarSource;

impl jftrade_calendar::CalendarSourcePort for StatusOnlyCalendarSource {
    fn descriptor(&self) -> jftrade_calendar::CalendarSourceDescriptor {
        jftrade_calendar::CalendarSourceDescriptor {
            id: "fixture_source".to_owned(),
            kind: "fixture".to_owned(),
            authority: "tests".to_owned(),
            markets: vec!["US".to_owned()],
        }
    }

    fn fetch(
        &self,
        _market: &str,
        _from: jftrade_kernel::WireTimestamp,
        _to: jftrade_kernel::WireTimestamp,
        _cancellation: &jftrade_calendar::CalendarCancellationToken,
    ) -> Result<jftrade_calendar::CalendarSnapshot, jftrade_calendar::CalendarSourceError> {
        Err(jftrade_calendar::CalendarSourceError::Failed(
            "status-only fixture has no schedule source".to_owned(),
        ))
    }
}

fn status_only_calendar_manager() -> Arc<jftrade_calendar::CalendarManager> {
    let mut registry = jftrade_calendar::CalendarSourceRegistry::default();
    registry
        .register(Arc::new(StatusOnlyCalendarSource))
        .expect("register fixture calendar source");
    Arc::new(
        jftrade_calendar::CalendarManager::new(
            registry,
            None,
            jftrade_calendar::CalendarManagerSettings {
                refresh_interval_hours: 24,
                warmup_markets: vec!["US".to_owned()],
                source_policies: vec![jftrade_calendar::CalendarSourcePolicy {
                    market: "US".to_owned(),
                    preferred_source_ids: vec!["fixture_source".to_owned()],
                    enabled_source_ids: vec!["fixture_source".to_owned()],
                    fallback_to_builtin: true,
                    ..jftrade_calendar::CalendarSourcePolicy::default()
                }],
                ..jftrade_calendar::CalendarManagerSettings::default()
            },
        )
        .expect("create fixture calendar manager"),
    )
}

// Parity: go:452dea11:internal/system/service_test.go:59 TestStatusUsesDynamicPortAndTradingEnvironmentProviders
#[tokio::test]
async fn system_status_reports_the_persisted_default_trading_environment() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(
        &settings_path,
        br#"{"execution":{"defaultTradingEnvironment":"REAL"}}"#,
    )
    .expect("seed execution settings");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, response) =
        request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
    assert_eq!(status, 200, "system status response: {response}");
    assert_eq!(response["data"]["apiPort"], address.port());
    assert_eq!(response["data"]["defaultTradingEnvironment"], "REAL");

    let (status, saved) = request_json_with_status(
        address,
        "PUT",
        "/api/v1/settings/execution",
        Some(r#"{"defaultTradingEnvironment":"SIMULATE"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200, "execution settings response: {saved}");
    assert_eq!(saved["data"]["defaultTradingEnvironment"], "SIMULATE");

    let (status, response) =
        request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
    assert_eq!(status, 200, "second system status response: {response}");
    assert_eq!(response["data"]["defaultTradingEnvironment"], "SIMULATE");
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/system/service_test.go:142 TestRealTradeStateUsesInjectedRiskGatewaySnapshot
#[tokio::test]
async fn system_status_and_control_reads_follow_the_configured_risk_snapshot() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(
        directory.path().join("real-trade-control.json"),
        br#"{
            "riskConfig": {
                "id": "runtime-risk-config",
                "tradingEnvironment": "REAL",
                "realTradingEnabled": true,
                "maxOrderQuantity": 12.5,
                "maxOrderNotional": 2500,
                "operatorId": "operator",
                "reason": "risk gateway snapshot",
                "activatedAt": "2026-09-01T01:00:00Z",
                "updatedAt": "2026-09-01T01:00:00Z"
            },
            "killSwitch": {
                "id": "kill-switch-control-plane",
                "tradingEnvironment": "REAL",
                "operatorId": "operator",
                "reason": "incident",
                "activatedAt": "2026-09-01T01:01:00Z",
                "updatedAt": "2026-09-01T01:01:00Z"
            },
            "events": [
                {"id":"risk-event-1","eventType":"updated","action":"RISK_CONFIG_UPDATED","brokerId":"*","createdAt":"2026-09-01T01:02:00Z"}
            ]
        }"#,
    )
    .expect("seed real-trade control state");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, response) =
        request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
    assert_eq!(status, 200, "system status response: {response}");
    assert_eq!(response["data"]["realTradingEnabled"], true);
    assert_eq!(
        response["data"]["realTradingKillSwitch"],
        json!({
            "active": true,
            "runtimeActive": true,
            "blockedOperations": ["PLACE", "MODIFY"],
            "allowsCancel": true,
        })
    );
    let risk = &response["data"]["realTradingRisk"];
    assert_eq!(risk["enabled"], true);
    assert_eq!(risk["maxOrderQuantity"].as_f64(), Some(12.5));
    assert_eq!(risk["maxOrderNotional"].as_f64(), Some(2500.0));
    assert_eq!(
        risk["runtimeConfiguredMaxOrderQuantity"].as_f64(),
        Some(12.5)
    );
    assert_eq!(
        risk["runtimeConfiguredMaxOrderNotional"].as_f64(),
        Some(2500.0)
    );
    assert_eq!(risk["runtimeRiskConfigured"], true);

    let (status, kill_switch) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/real-trade-kill-switch",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "kill switch response: {kill_switch}");
    assert_eq!(kill_switch["data"]["realTradingEnabled"], true);
    assert_eq!(kill_switch["data"]["killSwitchActive"], true);
    assert_eq!(kill_switch["data"]["killSwitchSource"], "RUNTIME");
    assert_eq!(kill_switch["data"]["allowsCancel"], true);
    assert_eq!(
        kill_switch["data"]["entry"]["id"],
        "kill-switch-control-plane"
    );

    let (status, risk_limits) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/real-trade-risk-limits",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "risk limits response: {risk_limits}");
    assert_eq!(risk_limits["data"]["riskEnabled"], true);
    assert_eq!(
        risk_limits["data"]["effectiveMaxOrderQuantity"].as_f64(),
        Some(12.5)
    );
    assert_eq!(
        risk_limits["data"]["effectiveMaxOrderNotional"].as_f64(),
        Some(2500.0)
    );
    assert_eq!(risk_limits["data"]["entry"]["id"], "runtime-risk-config");

    let (status, risk_events) = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/real-trade-risk-events",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "risk events response: {risk_events}");
    assert_eq!(risk_events["data"]["maxOrderQuantity"].as_f64(), Some(12.5));
    assert_eq!(
        risk_events["data"]["maxOrderNotional"].as_f64(),
        Some(2500.0)
    );
    assert_eq!(risk_events["data"]["entries"][0]["id"], "risk-event-1");
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/system/service_status_defaults_test.go:10 TestStatusIncludesInjectedObservabilitySummaries
#[tokio::test]
async fn system_status_embeds_injected_live_market_data_calendar_and_request_summaries() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
    let generation = recorder.reconcile(["US.AAPL".to_owned()]);
    let now = "2026-09-02T09:00:00+08:00".parse().expect("timestamp");
    assert!(recorder.record_stream_connected(generation));
    assert!(recorder.record_poll_started(generation, now));
    let manager = status_only_calendar_manager();
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_runtime_status_port(recorder)
            .with_calendar_manager(manager)
            .with_system_read_snapshot_port(Arc::new(UnavailableSystemReadPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, failed) =
        request_json_with_status(address, "GET", "/api/v1/system/futu-opend", None, &[]).await;
    assert_eq!(status, 503, "unavailable system read response: {failed}");

    let (status, response) =
        request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
    assert_eq!(status, 200, "system status response: {response}");
    let observability = &response["data"]["observability"];
    assert_eq!(
        observability["marketdata"],
        json!({
            "status": "connected",
            "connected": true,
            "closed": false,
            "generation": 1,
            "activeCount": 1,
            "lastRefreshAt": "2026-09-02T01:00:00Z",
            "quoteRetryAt": null,
            "quoteFailures": 0,
            "quoteLastError": null,
            "streamRetryAt": null,
            "streamFailures": 0,
            "streamLastError": null,
        })
    );
    assert_eq!(
        observability["live"]["connected"], 0,
        "live projection: {observability}"
    );
    assert!(
        observability["live"]["activeInstruments"].is_array(),
        "live projection: {observability}"
    );
    assert!(
        observability["exchangeCalendars"]["autoRefreshEnabled"].is_boolean(),
        "calendar projection: {observability}"
    );
    assert!(
        observability["exchangeCalendars"]["sources"].is_array(),
        "calendar projection: {observability}"
    );
    assert_eq!(observability["requests"]["slowThresholdMs"], 750);
    assert_eq!(observability["requests"]["minimumImportance"], "low");
    assert_eq!(
        observability["requests"]["recentErrors"][0]["status"], 503,
        "request observability: {observability}"
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/system/service_status_defaults_test.go:41 TestStatusProvidesDefaultRequestObservabilitySummary
#[tokio::test]
async fn system_status_default_request_observability_matches_the_go_baseline() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
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
        response["data"]["observability"]["requests"],
        json!({
            "recentErrors": [],
            "recentSlowRequests": [],
            "openD": {"totalCalls": 0, "failedCalls": 0},
            "slowThresholdMs": 750,
            "minimumImportance": "low",
        })
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/system/service_status_defaults_test.go:109 TestStorageAndRealTradeDefaultsExposeFrontendShape
#[tokio::test]
async fn storage_overview_and_control_defaults_expose_empty_frontend_slices() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, storage) =
        request_json_with_status(address, "GET", "/api/v1/system/storage/overview", None, &[])
            .await;
    assert_eq!(status, 200, "storage overview response: {storage}");
    for key in [
        "pendingOutbox",
        "recentJobs",
        "recentAuditLogs",
        "recentExecutionCommands",
    ] {
        assert_eq!(
            storage["data"][key],
            json!([]),
            "storage key {key}: {storage}"
        );
    }

    let hard_stops = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/real-trade-hard-stops",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(hard_stops["data"]["allowsCancel"], true);
    assert_eq!(hard_stops["data"]["entries"], json!([]));

    let hard_stop_events = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/real-trade-hard-stop-events",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(hard_stop_events["data"]["realTradingEnabled"], false);
    assert_eq!(hard_stop_events["data"]["allowsCancel"], true);
    assert_eq!(hard_stop_events["data"]["entries"], json!([]));

    let kill_switch_events = request_json_with_status(
        address,
        "GET",
        "/api/v1/system/real-trade-kill-switch-events",
        None,
        &[],
    )
    .await
    .1;
    assert_eq!(kill_switch_events["data"]["killSwitchActive"], false);
    assert_eq!(kill_switch_events["data"]["allowsCancel"], true);
    assert_eq!(kill_switch_events["data"]["entries"], json!([]));
    handle.shutdown().await.expect("shutdown product");
}
