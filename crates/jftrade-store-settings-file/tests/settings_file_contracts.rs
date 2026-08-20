use std::fs;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use jftrade_settings::{
    AppearanceService, AssistantRuntimeService, AssistantRuntimeSettings,
    ExchangeCalendarSettingsService, ExecutionService, ExecutionSettings, MarketDataProvider,
    MarketDataProviderSettingsService, McpServerRuntimePort, McpServerSettings,
    McpServerSettingsError, McpServerSettingsRecord, McpServerSettingsService,
    McpServerSettingsStorePort, McpServerSettingsUpdate, OnboardingSettingsService,
    PineWorkerSettings, PineWorkerSettingsService, SecurityRuntimePort, SecuritySettings,
    SecuritySettingsError, SecuritySettingsRecord, SecuritySettingsService,
    SecuritySettingsStorePort, SecuritySettingsUpdate, SettingsStorePort, SystemMcpServerSecrets,
    SystemNotificationService, SystemNotificationSettings, SystemSecurityPasswords,
    UiAppearanceSettings, verify_mcp_server_token, verify_web_access_password,
};
use jftrade_store_settings_file::SettingsFileStore;
use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ProductSliceCorpus {
    version: String,
    appearance_cases: Vec<AppearanceCase>,
    onboarding_cases: Vec<OnboardingCase>,
    futu_install_cases: Vec<FutuInstallCase>,
    execution_cases: Vec<ExecutionCase>,
    security_cases: Vec<SecurityCase>,
    market_data_provider_cases: Vec<MarketDataProviderCase>,
    backtest_market_data_provider_cases: Vec<BacktestMarketDataProviderCase>,
    exchange_calendar_cases: Vec<ExchangeCalendarCase>,
    assistant_runtime_cases: Vec<AssistantRuntimeCase>,
    mcp_server_cases: Vec<McpServerCase>,
    system_notification_cases: Vec<SystemNotificationCase>,
    pine_worker_cases: Vec<PineWorkerCase>,
    notification_forward_cases: Vec<NotificationForwardCase>,
    seed_document: Value,
    write_appearance: UiAppearanceSettings,
    expected_stored_appearance: UiAppearanceSettings,
    write_execution: ExecutionSettings,
    expected_stored_execution: ExecutionSettings,
    write_assistant_runtime: AssistantRuntimeSettings,
    expected_stored_assistant_runtime: AssistantRuntimeSettings,
    write_system_notifications: SystemNotificationSettings,
    expected_stored_system_notifications: SystemNotificationSettings,
    write_pine_worker: PineWorkerSettings,
    expected_stored_pine_worker: PineWorkerSettings,
}

#[derive(Debug, Deserialize)]
struct AppearanceCase {
    name: String,
    input: UiAppearanceSettings,
    expected: UiAppearanceSettings,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct OnboardingCase {
    name: String,
    input: Value,
    dependencies_satisfied: bool,
    expected: Value,
}

#[derive(Debug, Deserialize)]
struct FutuInstallCase {
    name: String,
    input: Value,
    expected: Value,
}

#[derive(Debug, Deserialize)]
struct ExecutionCase {
    name: String,
    input: ExecutionSettings,
    expected: ExecutionSettings,
}

#[derive(Debug, Deserialize)]
struct SecurityCase {
    name: String,
    input: Value,
    expected: SecuritySettings,
}

#[derive(Debug, Deserialize)]
struct MarketDataProviderCase {
    name: String,
    input: String,
    expected: MarketDataProvider,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BacktestMarketDataProviderCase {
    name: String,
    active_provider: Option<String>,
    backtest_provider: Option<String>,
    expected: MarketDataProvider,
}

#[derive(Debug, Deserialize)]
struct ExchangeCalendarCase {
    name: String,
    input: Value,
    expected: Value,
}

#[derive(Debug, Deserialize)]
struct AssistantRuntimeCase {
    name: String,
    input: AssistantRuntimeSettings,
    expected: AssistantRuntimeSettings,
}

#[derive(Debug, Deserialize)]
struct McpServerCase {
    name: String,
    input: Value,
    expected: McpServerSettings,
}

#[derive(Debug, Deserialize)]
struct SystemNotificationCase {
    name: String,
    input: SystemNotificationSettings,
    expected: Value,
}

#[derive(Debug, Deserialize)]
struct NotificationForwardCase {
    name: String,
    settings: SystemNotificationSettings,
    level: String,
    category: String,
    expected: bool,
}

#[derive(Debug, Deserialize)]
struct PineWorkerCase {
    name: String,
    input: PineWorkerSettings,
    expected: PineWorkerSettings,
}

#[test]
fn stage9_product_corpus_matches_go_and_preserves_unowned_fields() {
    let fixture = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../tests/fixtures/rust-migration/stage9/product-slice-corpus.json");
    let corpus: ProductSliceCorpus = serde_json::from_slice(
        &fs::read(&fixture).unwrap_or_else(|error| panic!("read {}: {error}", fixture.display())),
    )
    .expect("decode Stage 9 product corpus");
    assert_eq!(corpus.version, "stage9.product-slice.v10");
    assert!(corpus.appearance_cases.len() >= 4);
    for test_case in corpus.appearance_cases {
        let actual = jftrade_settings::normalize_appearance(&test_case.input);
        assert_eq!(actual, test_case.expected, "case {}", test_case.name);
    }
    assert!(corpus.onboarding_cases.len() >= 4);
    for test_case in corpus.onboarding_cases {
        let directory = tempdir().expect("temporary onboarding directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(&test_case.input).expect("encode onboarding document"),
        )
        .expect("seed onboarding document");
        let service = OnboardingSettingsService::new(std::sync::Arc::new(
            SettingsFileStore::open_read_only(&path).expect("open onboarding document"),
        ));
        let readiness = service
            .readiness(test_case.dependencies_satisfied)
            .expect("onboarding readiness");
        let actual = serde_json::json!({
            "state": readiness.state,
            "shouldShowOobe": readiness.should_show_oobe,
            "reasons": readiness.reasons,
            "brokerEnabled": readiness.broker_enabled,
            "brokerConfigured": readiness.broker_configured,
        });
        assert_eq!(actual, test_case.expected, "case {}", test_case.name);
    }
    assert!(corpus.futu_install_cases.len() >= 4);
    for test_case in corpus.futu_install_cases {
        let directory = tempdir().expect("temporary Futu install directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(&test_case.input).expect("encode Futu install document"),
        )
        .expect("seed Futu install document");
        let service = jftrade_settings::FutuOpenDInstallSettingsService::new(std::sync::Arc::new(
            SettingsFileStore::open_read_only(&path).expect("open Futu install document"),
        ));
        let settings = service.settings().expect("Futu install settings");
        let actual = serde_json::json!({
            "host": settings.host,
            "apiPort": settings.api_port,
            "websocketPort": settings.websocket_port,
            "maxWebSocketConnections": settings.max_websocket_connections,
            "useEncryption": settings.use_encryption,
            "websocketKeyRequired": settings.websocket_key_required,
            "marketDataTransport": "bbgo-opend-tcp-api",
            "minimumVersion": "10.9.6908",
        });
        assert_eq!(actual, test_case.expected, "case {}", test_case.name);
        assert!(actual.get("websocketKey").is_none());
    }
    assert!(corpus.execution_cases.len() >= 4);
    for test_case in corpus.execution_cases {
        let actual = jftrade_settings::normalize_execution_settings(&test_case.input);
        assert_eq!(actual, test_case.expected, "case {}", test_case.name);
    }
    assert!(corpus.security_cases.len() >= 4);
    for test_case in corpus.security_cases {
        let directory = tempdir().expect("temporary security directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(&serde_json::json!({ "security": test_case.input }))
                .expect("encode security case"),
        )
        .expect("seed security case");
        let service = SecuritySettingsService::new(std::sync::Arc::new(
            SettingsFileStore::open_read_only(&path).expect("open security case"),
        ));
        assert_eq!(
            service.settings().expect("security settings"),
            test_case.expected,
            "case {}",
            test_case.name
        );
    }
    assert!(corpus.market_data_provider_cases.len() >= 5);
    for test_case in corpus.market_data_provider_cases {
        let directory = tempdir().expect("temporary provider directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(&serde_json::json!({ "activeMarketDataProvider": test_case.input }))
                .expect("encode provider case"),
        )
        .expect("seed provider case");
        let service = MarketDataProviderSettingsService::new(std::sync::Arc::new(
            SettingsFileStore::open_read_only(&path).expect("open provider case"),
        ));
        assert_eq!(
            service.active_provider().expect("active provider"),
            test_case.expected,
            "case {}",
            test_case.name
        );
    }
    assert!(corpus.backtest_market_data_provider_cases.len() >= 4);
    for test_case in corpus.backtest_market_data_provider_cases {
        let directory = tempdir().expect("temporary backtest provider directory");
        let path = directory.path().join("settings.json");
        let mut document = serde_json::Map::new();
        if let Some(provider) = test_case.active_provider {
            document.insert(
                "activeMarketDataProvider".to_owned(),
                Value::String(provider),
            );
        }
        if let Some(provider) = test_case.backtest_provider {
            document.insert(
                "backtestMarketDataProvider".to_owned(),
                Value::String(provider),
            );
        }
        fs::write(
            &path,
            serde_json::to_vec(&document).expect("encode provider document"),
        )
        .expect("seed provider document");
        let service =
            jftrade_settings::BacktestMarketDataProviderSettingsService::new(std::sync::Arc::new(
                SettingsFileStore::open_read_only(&path).expect("open provider document"),
            ));
        assert_eq!(
            service.active_provider().expect("backtest provider"),
            test_case.expected,
            "case {}",
            test_case.name
        );
    }
    assert!(corpus.exchange_calendar_cases.len() >= 4);
    for test_case in corpus.exchange_calendar_cases {
        let directory = tempdir().expect("temporary calendar directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(
                &serde_json::json!({ "exchangeCalendars": test_case.input.clone() }),
            )
            .expect("encode calendar case"),
        )
        .expect("seed calendar case");
        let service = ExchangeCalendarSettingsService::new(std::sync::Arc::new(
            SettingsFileStore::open(&path).expect("open calendar case"),
        ));
        let current = service.settings().expect("calendar settings");
        assert_eq!(
            serde_json::to_value(&current).expect("encode calendar settings"),
            test_case.expected,
            "case {}",
            test_case.name
        );
        let saved = service.save(current).expect("save calendar settings");
        assert_eq!(
            serde_json::to_value(saved).expect("encode saved calendar settings"),
            test_case.expected,
            "saved case {}",
            test_case.name
        );
        let persisted: Value = serde_json::from_slice(&fs::read(&path).expect("read calendar"))
            .expect("decode calendar document");
        assert_eq!(
            persisted["exchangeCalendars"], test_case.expected,
            "persisted case {}",
            test_case.name
        );
    }
    assert!(corpus.assistant_runtime_cases.len() >= 4);
    for test_case in corpus.assistant_runtime_cases {
        let actual = jftrade_settings::normalize_assistant_runtime_settings(&test_case.input);
        assert_eq!(actual, test_case.expected, "case {}", test_case.name);
    }
    assert!(corpus.mcp_server_cases.len() >= 4);
    for test_case in corpus.mcp_server_cases {
        let directory = tempdir().expect("temporary MCP directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(&serde_json::json!({ "mcpServer": test_case.input }))
                .expect("encode MCP case"),
        )
        .expect("seed MCP case");
        let service = McpServerSettingsService::new(std::sync::Arc::new(
            SettingsFileStore::open_read_only(&path).expect("open MCP case"),
        ));
        let snapshot = service.stopped_snapshot().expect("MCP snapshot");
        assert_eq!(
            snapshot.settings, test_case.expected,
            "case {}",
            test_case.name
        );
        assert!(!snapshot.status.running, "case {}", test_case.name);
        assert_eq!(
            snapshot.status.endpoint,
            format!("http://127.0.0.1:{}/mcp", snapshot.settings.port),
            "case {}",
            test_case.name
        );
        let public = serde_json::to_value(snapshot).expect("encode MCP snapshot");
        assert!(public["settings"].get("tokenHash").is_none());
    }
    assert!(corpus.system_notification_cases.len() >= 4);
    for test_case in corpus.system_notification_cases {
        let actual = jftrade_settings::normalize_system_notification_settings(&test_case.input);
        assert_eq!(
            serde_json::to_value(actual).expect("encode system notifications"),
            test_case.expected,
            "case {}",
            test_case.name
        );
    }
    assert!(corpus.notification_forward_cases.len() >= 5);
    for test_case in corpus.notification_forward_cases {
        assert_eq!(
            jftrade_settings::should_forward_system_notification(
                &test_case.settings,
                &test_case.level,
                &test_case.category,
            ),
            test_case.expected,
            "case {}",
            test_case.name
        );
    }
    assert!(corpus.pine_worker_cases.len() >= 4);
    for test_case in corpus.pine_worker_cases {
        assert_eq!(
            jftrade_settings::normalize_pine_worker_settings(&test_case.input),
            test_case.expected,
            "case {}",
            test_case.name
        );
    }

    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    fs::write(
        &path,
        serde_json::to_vec(&corpus.seed_document).expect("encode seed document"),
    )
    .expect("seed settings");
    let store = SettingsFileStore::open(&path).expect("open settings");
    let service = AppearanceService::new(std::sync::Arc::new(store));
    assert_eq!(
        service
            .save_appearance(&corpus.write_appearance)
            .expect("save appearance"),
        corpus.expected_stored_appearance
    );
    let persisted: Value = serde_json::from_slice(&fs::read(&path).expect("read settings"))
        .expect("decode persisted settings");
    assert_eq!(persisted["interfaces"], corpus.seed_document["interfaces"]);
    assert_eq!(persisted["security"], corpus.seed_document["security"]);
    assert_eq!(
        persisted["futureOwner"],
        corpus.seed_document["futureOwner"]
    );

    let execution = ExecutionService::new(std::sync::Arc::new(
        SettingsFileStore::open(&path).expect("reopen settings for execution"),
    ));
    assert_eq!(
        execution
            .save(&corpus.write_execution)
            .expect("save execution"),
        corpus.expected_stored_execution
    );
    let persisted: Value = serde_json::from_slice(&fs::read(&path).expect("read settings"))
        .expect("decode persisted settings");
    assert_eq!(persisted["interfaces"], corpus.seed_document["interfaces"]);
    assert_eq!(persisted["security"], corpus.seed_document["security"]);
    assert_eq!(
        persisted["execution"],
        serde_json::to_value(&corpus.expected_stored_execution).expect("encode execution")
    );

    let assistant_runtime = AssistantRuntimeService::new(std::sync::Arc::new(
        SettingsFileStore::open(&path).expect("reopen settings for assistant runtime"),
    ));
    assert_eq!(
        assistant_runtime
            .save(&corpus.write_assistant_runtime)
            .expect("save assistant runtime"),
        corpus.expected_stored_assistant_runtime
    );
    let notifications = SystemNotificationService::new(std::sync::Arc::new(
        SettingsFileStore::open(&path).expect("reopen settings for notifications"),
    ));
    assert_eq!(
        notifications
            .save(&corpus.write_system_notifications)
            .expect("save system notifications"),
        corpus.expected_stored_system_notifications
    );
    let pine_worker = PineWorkerSettingsService::new(std::sync::Arc::new(
        SettingsFileStore::open(&path).expect("reopen settings for Pine worker"),
    ));
    assert_eq!(
        pine_worker
            .save(&corpus.write_pine_worker)
            .expect("save Pine worker settings"),
        corpus.expected_stored_pine_worker
    );
    let persisted: Value = serde_json::from_slice(&fs::read(&path).expect("read settings"))
        .expect("decode persisted settings");
    assert_eq!(persisted["interfaces"], corpus.seed_document["interfaces"]);
    assert_eq!(
        persisted["adk"],
        serde_json::to_value(&corpus.expected_stored_assistant_runtime)
            .expect("encode assistant runtime")
    );
    assert_eq!(
        persisted["systemNotifications"],
        serde_json::to_value(&corpus.expected_stored_system_notifications)
            .expect("encode notifications")
    );
    assert_eq!(
        persisted["pineWorker"],
        serde_json::to_value(&corpus.expected_stored_pine_worker)
            .expect("encode Pine worker settings")
    );
}

#[test]
fn appearance_round_trip_preserves_unknown_go_owned_fields() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    fs::write(
        &path,
        r#"{"interfaces":{"apiBind":"127.0.0.1:3000"},"futureOwner":{"enabled":true}}"#,
    )
    .expect("seed settings");

    let store = std::sync::Arc::new(SettingsFileStore::open(&path).expect("open settings"));
    let service = AppearanceService::new(store);
    let saved = service
        .save_appearance(&UiAppearanceSettings {
            up_color: " #ABCDEF ".into(),
            down_color: "invalid".into(),
        })
        .expect("save appearance");
    assert_eq!(saved.up_color, "#abcdef");
    assert_eq!(saved.down_color, "#ea3943");

    let document: Value = serde_json::from_slice(&fs::read(&path).expect("read settings"))
        .expect("decode persisted settings");
    assert_eq!(document["interfaces"]["apiBind"], "127.0.0.1:3000");
    assert_eq!(document["futureOwner"]["enabled"], true);
    assert_eq!(document["appearance"]["upColor"], "#abcdef");

    let reloaded = SettingsFileStore::open(&path).expect("reload settings");
    assert_eq!(reloaded.load_appearance().expect("appearance"), Some(saved));
}

#[test]
fn missing_empty_and_corrupted_documents_have_distinct_behavior() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    let store = SettingsFileStore::open(&path).expect("missing file is empty settings");
    assert_eq!(store.load_appearance().expect("missing appearance"), None);

    fs::write(&path, "  \n").expect("empty settings");
    assert!(SettingsFileStore::open(&path).is_ok());

    fs::write(&path, r##"{"appearance":"#broken"}"##).expect("invalid appearance");
    assert!(SettingsFileStore::open(&path).is_err());

    fs::write(&path, r##"{"appearance":"#broken""##).expect("corrupted settings");
    assert!(SettingsFileStore::open(&path).is_err());
}

#[test]
fn read_only_shadow_loads_without_mutating_or_persisting_settings() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    let original = br##"{"appearance":{"upColor":"#010203","downColor":"#a0b0c0"}}"##;
    fs::write(&path, original).expect("seed settings");

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;

        fs::set_permissions(directory.path(), fs::Permissions::from_mode(0o755))
            .expect("set directory mode");
        fs::set_permissions(&path, fs::Permissions::from_mode(0o644)).expect("set file mode");
    }

    let store = SettingsFileStore::open_read_only(&path).expect("open read-only settings");
    assert_eq!(
        store
            .load_appearance()
            .expect("load appearance")
            .expect("appearance")
            .up_color,
        "#010203"
    );
    fs::write(
        &path,
        br##"{"appearance":{"upColor":"#112233","downColor":"#445566"}}"##,
    )
    .expect("replace Go-owned settings");
    assert_eq!(
        store
            .load_appearance()
            .expect("reload appearance")
            .expect("reloaded appearance")
            .up_color,
        "#112233"
    );
    let error = store
        .save_appearance(&UiAppearanceSettings {
            up_color: "#ffffff".into(),
            down_color: "#000000".into(),
        })
        .expect_err("read-only settings must reject writes");
    assert!(error.to_string().contains("read-only"));
    assert_eq!(
        fs::read(&path).expect("read settings"),
        br##"{"appearance":{"upColor":"#112233","downColor":"#445566"}}"##
    );

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;

        assert_eq!(
            fs::metadata(directory.path())
                .expect("directory metadata")
                .permissions()
                .mode()
                & 0o777,
            0o755
        );
        assert_eq!(
            fs::metadata(&path)
                .expect("file metadata")
                .permissions()
                .mode()
                & 0o777,
            0o644
        );
    }
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PersistedAppearance {
    up_color: String,
    down_color: String,
}

#[test]
fn overwrite_replaces_existing_file_without_leaving_a_temporary_peer() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    fs::write(&path, "{}").expect("seed settings");
    let store = SettingsFileStore::open(&path).expect("open settings");
    store
        .save_appearance(&UiAppearanceSettings {
            up_color: "#010203".into(),
            down_color: "#a0b0c0".into(),
        })
        .expect("replace settings");

    let document: Value =
        serde_json::from_slice(&fs::read(&path).expect("read settings")).expect("decode settings");
    let appearance: PersistedAppearance =
        serde_json::from_value(document["appearance"].clone()).expect("appearance");
    assert_eq!(appearance.up_color, "#010203");
    assert_eq!(appearance.down_color, "#a0b0c0");
    assert_eq!(
        fs::read_dir(directory.path())
            .expect("list settings directory")
            .count(),
        1
    );
}

#[derive(Default)]
struct AcceptingMcpRuntime(AtomicUsize);

impl McpServerRuntimePort for AcceptingMcpRuntime {
    fn apply(&self, _record: &McpServerSettingsRecord) -> Result<(), String> {
        self.0.fetch_add(1, Ordering::SeqCst);
        Ok(())
    }
}

struct FailingMcpRuntime;

impl McpServerRuntimePort for FailingMcpRuntime {
    fn apply(&self, _record: &McpServerSettingsRecord) -> Result<(), String> {
        Err("port occupied".to_owned())
    }
}

#[test]
fn stage9_mcp_settings_writes_match_current_go_owner() {
    let directory = tempdir().expect("temporary MCP directory");
    let path = directory.path().join("settings.json");
    fs::write(
        &path,
        r##"{"appearance":{"upColor":"#010203","downColor":"#a0b0c0"}}"##,
    )
    .expect("seed MCP settings");
    let store = Arc::new(SettingsFileStore::open(&path).expect("open MCP settings"));
    let runtime = Arc::new(AcceptingMcpRuntime::default());
    let service = McpServerSettingsService::with_ports(
        store.clone(),
        Some(runtime.clone()),
        Arc::new(SystemMcpServerSecrets),
    );

    let invalid_port = service.save(&McpServerSettingsUpdate {
        enabled: true,
        port: 80,
        auth_mode: "token".to_owned(),
    });
    let invalid_mode = service.save(&McpServerSettingsUpdate {
        auth_mode: "basic".to_owned(),
        ..McpServerSettingsUpdate::default()
    });
    let token_required = service.save(&McpServerSettingsUpdate {
        enabled: true,
        port: 6697,
        auth_mode: "token".to_owned(),
    });
    let reset = service.reset_token().expect("reset MCP token");
    let stored_after_reset = store
        .load_mcp_server_record()
        .expect("load MCP record")
        .expect("stored MCP record");
    let public_json = serde_json::to_string(&reset.settings).expect("encode MCP settings");
    let saved = service
        .save(&McpServerSettingsUpdate {
            enabled: true,
            port: 6697,
            auth_mode: "token".to_owned(),
        })
        .expect("enable MCP settings");
    let persisted = fs::read_to_string(&path).expect("read persisted MCP settings");
    let persisted_document: Value = serde_json::from_str(&persisted).expect("decode MCP settings");

    let original = store
        .load_mcp_server_record()
        .expect("load original MCP record")
        .expect("original MCP record");
    let failing_service = McpServerSettingsService::with_ports(
        store.clone(),
        Some(Arc::new(FailingMcpRuntime)),
        Arc::new(SystemMcpServerSecrets),
    );
    let runtime_failure = failing_service.save(&McpServerSettingsUpdate {
        enabled: true,
        port: 7443,
        auth_mode: "token".to_owned(),
    });
    let rolled_back = store
        .load_mcp_server_record()
        .expect("load rolled-back MCP record")
        .expect("rolled-back MCP record");

    let actual = serde_json::json!({
        "version": "stage9.mcp-settings-write.v1",
        "invalidPortRejected": matches!(invalid_port, Err(McpServerSettingsError::InvalidPort)),
        "invalidModeRejected": matches!(invalid_mode, Err(McpServerSettingsError::InvalidAuthMode)),
        "tokenRequiredRejected": matches!(token_required, Err(McpServerSettingsError::TokenRequired)),
        "tokenHasPrefix": reset.token.starts_with("jft_mcp_"),
        "tokenLength": reset.token.len(),
        "tokenConfigured": reset.settings.token_configured,
        "verifierValid": verify_mcp_server_token(stored_after_reset.token_hash(), &reset.token),
        "publicLeaksTokenHash": public_json.contains("tokenHash"),
        "publicLeaksToken": public_json.contains(&reset.token),
        "persistedLeaksToken": persisted.contains(&reset.token),
        "persistedHasArgon2id": persisted.contains("$argon2id$v=19$m=65536,t=3,p=1$"),
        "unrelatedSettingsPreserved": persisted_document["appearance"]["upColor"] == "#010203",
        "savedEnabled": saved.enabled,
        "successfulRuntimeApplies": runtime.0.load(Ordering::SeqCst),
        "runtimeFailureMapped": matches!(
            runtime_failure,
            Err(McpServerSettingsError::Runtime { .. })
        ),
        "runtimeFailureRolledBack": rolled_back == original,
    });
    let expected = serde_json::json!({
        "version": "stage9.mcp-settings-write.v1",
        "invalidPortRejected": true,
        "invalidModeRejected": true,
        "tokenRequiredRejected": true,
        "tokenHasPrefix": true,
        "tokenLength": 51,
        "tokenConfigured": true,
        "verifierValid": true,
        "publicLeaksTokenHash": false,
        "publicLeaksToken": false,
        "persistedLeaksToken": false,
        "persistedHasArgon2id": true,
        "unrelatedSettingsPreserved": true,
        "savedEnabled": true,
        "successfulRuntimeApplies": 2,
        "runtimeFailureMapped": true,
        "runtimeFailureRolledBack": true,
    });
    assert_eq!(actual, expected);
    if let Ok(reference_path) = std::env::var("JFTRADE_STAGE9_MCP_SETTINGS_WRITE_REFERENCE") {
        let reference: Value = serde_json::from_slice(
            &fs::read(reference_path).expect("read Go MCP settings reference"),
        )
        .expect("decode Go MCP settings reference");
        assert_eq!(actual, reference);
    }
}

#[derive(Default)]
struct AcceptingSecurityRuntime(AtomicUsize);

impl SecurityRuntimePort for AcceptingSecurityRuntime {
    fn apply(&self, _record: &SecuritySettingsRecord) -> Result<(), String> {
        self.0.fetch_add(1, Ordering::SeqCst);
        Ok(())
    }
}

struct FailingSecurityRuntime;

impl SecurityRuntimePort for FailingSecurityRuntime {
    fn apply(&self, _record: &SecuritySettingsRecord) -> Result<(), String> {
        Err("port occupied".to_owned())
    }
}

#[test]
fn stage9_security_settings_writes_match_current_go_owner() {
    let directory = tempdir().expect("temporary security directory");
    let path = directory.path().join("settings.json");
    fs::write(
        &path,
        r##"{"appearance":{"upColor":"#010203","downColor":"#a0b0c0"}}"##,
    )
    .expect("seed security settings");
    let store = Arc::new(SettingsFileStore::open(&path).expect("open security settings"));
    let runtime = Arc::new(AcceptingSecurityRuntime::default());
    let service = SecuritySettingsService::with_ports(
        store.clone(),
        Some(runtime.clone()),
        Arc::new(SystemSecurityPasswords),
    );

    let invalid_port = service.save(&SecuritySettingsUpdate {
        web_port: 80,
        ..SecuritySettingsUpdate::default()
    });
    let password_required = service.save(&SecuritySettingsUpdate {
        web_access_enabled: true,
        web_port: 6688,
        ..SecuritySettingsUpdate::default()
    });
    let password_too_short = service.save(&SecuritySettingsUpdate {
        new_password: "short".to_owned(),
        ..SecuritySettingsUpdate::default()
    });
    let password_too_long = service.save(&SecuritySettingsUpdate {
        new_password: "a".repeat(1025),
        ..SecuritySettingsUpdate::default()
    });
    let password = "a sufficiently long password";
    let saved = service
        .save(&SecuritySettingsUpdate {
            web_access_enabled: true,
            public_access_enabled: true,
            web_port: 6688,
            new_password: password.to_owned(),
        })
        .expect("save security settings");
    let stored = store
        .load_security_record()
        .expect("load security record")
        .expect("stored security record");
    let public_json = serde_json::to_string(&saved).expect("encode security settings");
    let disabled = service
        .save(&SecuritySettingsUpdate {
            public_access_enabled: true,
            ..SecuritySettingsUpdate::default()
        })
        .expect("disable security settings");
    let persisted = fs::read_to_string(&path).expect("read security settings");
    let persisted_document: Value =
        serde_json::from_str(&persisted).expect("decode security settings");

    let original = store
        .load_security_record()
        .expect("load original security record")
        .expect("original security record");
    let failing_service = SecuritySettingsService::with_ports(
        store.clone(),
        Some(Arc::new(FailingSecurityRuntime)),
        Arc::new(SystemSecurityPasswords),
    );
    let runtime_failure = failing_service.save(&SecuritySettingsUpdate {
        web_access_enabled: true,
        web_port: 7443,
        ..SecuritySettingsUpdate::default()
    });
    let rolled_back = store
        .load_security_record()
        .expect("load rolled-back security record")
        .expect("rolled-back security record");

    let actual = serde_json::json!({
        "version": "stage9.security-settings-write.v1",
        "invalidPortRejected": matches!(invalid_port, Err(SecuritySettingsError::InvalidPort)),
        "passwordRequiredRejected": matches!(password_required, Err(SecuritySettingsError::PasswordRequired)),
        "passwordTooShortRejected": matches!(password_too_short, Err(SecuritySettingsError::PasswordTooShort)),
        "passwordTooLongRejected": matches!(password_too_long, Err(SecuritySettingsError::PasswordTooLong)),
        "savedWebAccessEnabled": saved.web_access_enabled,
        "savedPublicAccessEnabled": saved.public_access_enabled,
        "passwordConfigured": saved.password_configured,
        "verifierValid": verify_web_access_password(stored.password_hash(), password),
        "publicLeaksPasswordHash": public_json.contains("passwordHash"),
        "publicLeaksPassword": public_json.contains(password),
        "persistedLeaksPassword": persisted.contains(password),
        "persistedHasArgon2id": persisted.contains("$argon2id$v=19$m=65536,t=3,p=1$"),
        "unrelatedSettingsPreserved": persisted_document["appearance"]["upColor"] == "#010203",
        "disabledWebAccess": !disabled.web_access_enabled,
        "disabledPublicAccess": !disabled.public_access_enabled,
        "successfulRuntimeApplies": runtime.0.load(Ordering::SeqCst),
        "runtimeFailureMapped": matches!(runtime_failure, Err(SecuritySettingsError::Runtime { .. })),
        "runtimeFailureRolledBack": rolled_back == original,
    });
    let expected = serde_json::json!({
        "version": "stage9.security-settings-write.v1",
        "invalidPortRejected": true,
        "passwordRequiredRejected": true,
        "passwordTooShortRejected": true,
        "passwordTooLongRejected": true,
        "savedWebAccessEnabled": true,
        "savedPublicAccessEnabled": true,
        "passwordConfigured": true,
        "verifierValid": true,
        "publicLeaksPasswordHash": false,
        "publicLeaksPassword": false,
        "persistedLeaksPassword": false,
        "persistedHasArgon2id": true,
        "unrelatedSettingsPreserved": true,
        "disabledWebAccess": true,
        "disabledPublicAccess": true,
        "successfulRuntimeApplies": 2,
        "runtimeFailureMapped": true,
        "runtimeFailureRolledBack": true,
    });
    assert_eq!(actual, expected);
    if let Ok(reference_path) = std::env::var("JFTRADE_STAGE9_SECURITY_SETTINGS_WRITE_REFERENCE") {
        let reference: Value = serde_json::from_slice(
            &fs::read(reference_path).expect("read Go security settings reference"),
        )
        .expect("decode Go security settings reference");
        assert_eq!(actual, reference);
    }
}
