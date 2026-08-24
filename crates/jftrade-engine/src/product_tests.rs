use std::path::Path;

use jftrade_settings::SettingsStorePort;
use rusqlite::Connection;
use serde_json::Value;
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use super::*;

#[test]
fn static_provider_catalog_matches_current_go_wire_fixture() {
    let mut descriptors = vec![jftrade_integration_futu::provider_descriptor()];
    descriptors.extend(jftrade_integration_marketdata_helper::provider_descriptors());
    let actual = serde_json::Value::Array(
        descriptors
            .into_iter()
            .map(provider_descriptor_wire)
            .collect(),
    );
    let expected: serde_json::Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/provider-descriptors.json"
    ))
    .expect("provider descriptor fixture");
    assert_eq!(actual, expected);
}

#[test]
fn stage9_assistant_agent_templates_match_current_go_owner() {
    let Some(reference_path) =
        std::env::var_os("JFTRADE_STAGE9_ASSISTANT_AGENT_TEMPLATES_REFERENCE")
    else {
        return;
    };
    let expected: Value = serde_json::from_slice(
        &std::fs::read(reference_path).expect("read Go agent-template reference"),
    )
    .expect("decode Go agent-template reference");
    assert_eq!(expected["version"], "stage9.assistant-agent-templates.v1");
    assert_eq!(
        agent_templates_wire(),
        json!({"templates": expected["templates"].clone()})
    );
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct OnboardingSettingsWriteCorpus {
    version: String,
    cases: Vec<OnboardingSettingsWriteCase>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct OnboardingSettingsWriteCase {
    name: String,
    seed_document: Value,
    input: OnboardingWriteRequest,
}

#[test]
fn stage9_onboarding_settings_writes_match_current_go_owner() {
    let corpus: OnboardingSettingsWriteCorpus = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/onboarding-settings-write-corpus.json"
    ))
    .expect("onboarding settings write corpus");
    assert_eq!(corpus.version, "stage9.onboarding-settings-write.v1");
    assert!(corpus.cases.len() >= 4);
    let directory = tempdir().expect("temporary directory");
    let mut results = Vec::with_capacity(corpus.cases.len());
    for (index, test_case) in corpus.cases.iter().enumerate() {
        let path = directory.path().join(format!("onboarding-{index}.json"));
        std::fs::write(
            &path,
            serde_json::to_vec(&test_case.seed_document).expect("encode onboarding seed document"),
        )
        .expect("seed onboarding settings write document");
        let store = Arc::new(SettingsFileStore::open(&path).expect("open onboarding store"));
        let service = OnboardingSettingsService::new(store);
        let saved = service
            .save(&test_case.input, "2026-08-20T04:00:00Z")
            .expect("save onboarding settings");
        let persisted: Value = serde_json::from_slice(
            &std::fs::read(&path).expect("read persisted onboarding settings"),
        )
        .expect("decode persisted onboarding settings");
        results.push(json!({
            "name": test_case.name,
            "saved": saved,
            "persisted": persisted,
        }));
    }
    let mut actual = json!({"version": corpus.version, "results": results});
    normalize_broker_timestamps(&mut actual);
    let Some(reference_path) =
        std::env::var_os("JFTRADE_STAGE9_ONBOARDING_SETTINGS_WRITE_REFERENCE")
    else {
        return;
    };
    let expected: Value = serde_json::from_slice(
        &std::fs::read(reference_path).expect("read Go onboarding write reference"),
    )
    .expect("decode Go onboarding write reference");
    assert_eq!(actual, expected);
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ProviderSettingsWriteCorpus {
    version: String,
    seed_document: Value,
    active_inputs: Vec<String>,
    backtest_inputs: Vec<String>,
}

#[test]
fn stage9_provider_settings_writes_match_current_go_owner() {
    let corpus: ProviderSettingsWriteCorpus = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/provider-settings-write-corpus.json"
    ))
    .expect("provider settings write corpus");
    assert_eq!(corpus.version, "stage9.provider-settings-write.v1");
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    std::fs::write(
        &path,
        serde_json::to_vec(&corpus.seed_document).expect("encode provider seed document"),
    )
    .expect("seed provider settings write document");
    let store = Arc::new(SettingsFileStore::open(&path).expect("open provider store"));
    let active = MarketDataProviderSettingsService::new(store.clone());
    let backtest = BacktestMarketDataProviderSettingsService::new(store);
    let mut active_results = Vec::with_capacity(corpus.active_inputs.len());
    for input in &corpus.active_inputs {
        let result = active.save(input);
        let error = result.is_err();
        let provider = result.unwrap_or_else(|_| {
            active
                .active_provider()
                .expect("active provider after rejection")
        });
        active_results.push(json!({
            "input": input,
            "provider": provider,
            "error": error,
        }));
    }
    let mut backtest_results = Vec::with_capacity(corpus.backtest_inputs.len());
    for input in &corpus.backtest_inputs {
        let result = backtest.save(input);
        let error = result.is_err();
        let provider = result.unwrap_or_else(|_| {
            backtest
                .active_provider()
                .expect("backtest provider after rejection")
        });
        backtest_results.push(json!({
            "input": input,
            "provider": provider,
            "error": error,
        }));
    }
    let persisted: Value =
        serde_json::from_slice(&std::fs::read(&path).expect("read persisted provider settings"))
            .expect("decode persisted provider settings");
    let actual = json!({
        "version": corpus.version,
        "activeResults": active_results,
        "backtestResults": backtest_results,
        "persisted": persisted,
    });
    let Some(reference_path) = std::env::var_os("JFTRADE_STAGE9_PROVIDER_SETTINGS_WRITE_REFERENCE")
    else {
        return;
    };
    let expected: Value = serde_json::from_slice(
        &std::fs::read(reference_path).expect("read Go provider write reference"),
    )
    .expect("decode Go provider write reference");
    assert_eq!(actual, expected);
}

#[derive(Deserialize)]
struct BrokerSettingsCorpus {
    version: String,
    cases: Vec<BrokerSettingsCase>,
}

#[derive(Deserialize)]
struct BrokerSettingsCase {
    name: String,
    document: Value,
}

#[test]
fn stage9_broker_settings_reads_match_current_go_owner() {
    let corpus: BrokerSettingsCorpus = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/broker-settings-corpus.json"
    ))
    .expect("broker settings corpus");
    assert_eq!(corpus.version, "stage9.broker-settings-read.v1");
    assert!(corpus.cases.len() >= 4);

    let directory = tempdir().expect("temporary directory");
    let mut results = Vec::with_capacity(corpus.cases.len());
    for (index, test_case) in corpus.cases.iter().enumerate() {
        let path = directory.path().join(format!("broker-{index}.json"));
        std::fs::write(
            &path,
            serde_json::to_vec(&test_case.document).expect("encode broker settings case"),
        )
        .expect("seed broker settings case");
        let service = BrokerSettingsService::new(Arc::new(
            SettingsFileStore::open_read_only(&path).expect("open broker settings case"),
        ));
        let projection = broker_settings_wire(service.inputs().expect("broker inputs"));
        results.push(json!({"name": test_case.name, "projection": projection}));
    }
    assert_eq!(
        results[1]["projection"]["brokers"][0]["integration"]["config"]["websocketKey"],
        "fixture-secret"
    );

    let Some(reference_path) = std::env::var_os("JFTRADE_STAGE9_BROKER_SETTINGS_REFERENCE") else {
        return;
    };
    let expected: Value = serde_json::from_slice(
        &std::fs::read(reference_path).expect("read Go broker settings reference"),
    )
    .expect("decode Go broker settings reference");
    assert_eq!(
        json!({"version": corpus.version, "results": results}),
        expected
    );
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct BrokerSettingsWriteCorpus {
    version: String,
    seed_document: Value,
    integration: BrokerIntegration,
    create_first: ManagedBrokerAccount,
    upsert_first: ManagedBrokerAccount,
    create_second: ManagedBrokerAccount,
    update_second: ManagedBrokerAccount,
    delete_id: String,
    missing_id: String,
}

#[test]
fn stage9_broker_settings_writes_match_current_go_owner() {
    let corpus: BrokerSettingsWriteCorpus = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/broker-settings-write-corpus.json"
    ))
    .expect("broker settings write corpus");
    assert_eq!(corpus.version, "stage9.broker-settings-write.v1");
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("settings.json");
    std::fs::write(
        &path,
        serde_json::to_vec(&corpus.seed_document).expect("encode seed document"),
    )
    .expect("seed broker settings write document");
    let store = Arc::new(SettingsFileStore::open(&path).expect("open broker write store"));
    let service = BrokerSettingsService::new(store);
    let now = "2026-08-20T04:00:00Z";
    let integration = service
        .save_integration(&corpus.integration, now)
        .expect("save integration");
    let created_first = service
        .create_account(&corpus.create_first, now)
        .expect("create first account");
    let upserted_first = service
        .create_account(&corpus.upsert_first, now)
        .expect("upsert first account");
    let created_second = service
        .create_account(&corpus.create_second, now)
        .expect("create second account");
    let updated_second = service
        .update_account(&created_second.id, &corpus.update_second, now)
        .expect("update second account");
    service
        .delete_account(&corpus.delete_id)
        .expect("delete first account");
    let update_missing = matches!(
        service.update_account(&corpus.missing_id, &corpus.update_second, now),
        Err(BrokerSettingsError::AccountNotFound)
    );
    let delete_missing = matches!(
        service.delete_account(&corpus.missing_id),
        Err(BrokerSettingsError::AccountNotFound)
    );
    let projection = broker_settings_wire(service.inputs().expect("final broker inputs"));
    let persisted: Value =
        serde_json::from_slice(&std::fs::read(&path).expect("read persisted broker settings"))
            .expect("decode persisted broker settings");
    let mut actual = json!({
        "version": corpus.version,
        "integration": integration,
        "createdFirst": created_first,
        "upsertedFirst": upserted_first,
        "createdSecond": created_second,
        "updatedSecond": updated_second,
        "updateMissing": update_missing,
        "deleteMissing": delete_missing,
        "projection": projection,
        "persisted": persisted,
    });
    normalize_broker_timestamps(&mut actual);
    assert_eq!(
        actual["persisted"]["interfaces"]["liveWebSocketConnectionLimit"],
        20
    );

    let Some(reference_path) = std::env::var_os("JFTRADE_STAGE9_BROKER_SETTINGS_WRITE_REFERENCE")
    else {
        return;
    };
    let expected: Value = serde_json::from_slice(
        &std::fs::read(reference_path).expect("read Go broker write reference"),
    )
    .expect("decode Go broker write reference");
    assert_eq!(actual, expected);
}

fn normalize_broker_timestamps(value: &mut Value) {
    match value {
        Value::Object(fields) => {
            for (key, value) in fields {
                if matches!(
                    key.as_str(),
                    "createdAt" | "updatedAt" | "completedAt" | "dismissedAt"
                ) && value.as_str().is_some_and(|value| !value.is_empty())
                {
                    *value = Value::String("<timestamp>".to_owned());
                } else {
                    normalize_broker_timestamps(value);
                }
            }
        }
        Value::Array(items) => items.iter_mut().for_each(normalize_broker_timestamps),
        Value::Null | Value::Bool(_) | Value::Number(_) | Value::String(_) => {}
    }
}

#[derive(Debug)]
struct DeliveredNotification;

impl ProductNotificationPort for DeliveredNotification {
    fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery {
        assert_eq!(request.title, "JFTrade 系统通知测试");
        assert!(request.sound_enabled);
        ProductNotificationDelivery {
            delivered: true,
            status: "delivered".to_owned(),
            message: "sent".to_owned(),
        }
    }
}

struct FixtureCalendarSource {
    responses: std::sync::Mutex<
        std::collections::VecDeque<
            Result<jftrade_calendar::CalendarSnapshot, jftrade_calendar::CalendarSourceError>,
        >,
    >,
}

impl FixtureCalendarSource {
    fn new(
        responses: impl IntoIterator<
            Item = Result<
                jftrade_calendar::CalendarSnapshot,
                jftrade_calendar::CalendarSourceError,
            >,
        >,
    ) -> Self {
        Self {
            responses: std::sync::Mutex::new(responses.into_iter().collect()),
        }
    }
}

impl jftrade_calendar::CalendarSourcePort for FixtureCalendarSource {
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
        self.responses
            .lock()
            .expect("fixture calendar responses")
            .pop_front()
            .unwrap_or_else(|| {
                Err(jftrade_calendar::CalendarSourceError::Failed(
                    "fixture exhausted".to_owned(),
                ))
            })
    }
}

fn fixture_calendar_snapshot(checksum: &str) -> jftrade_calendar::CalendarSnapshot {
    let timestamp = |value: &str| value.parse().expect("fixture calendar timestamp");
    jftrade_calendar::CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "fixture_source".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: vec![jftrade_calendar::TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-08-21T00:00:00Z"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "fixture_holiday".to_owned(),
            source_id: "fixture_source".to_owned(),
            observed: true,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-08-20T00:00:00Z"),
        valid_until: timestamp("2027-12-31T23:59:59Z"),
        checksum: checksum.to_owned(),
    }
}

fn fixture_calendar_manager(
    responses: impl IntoIterator<
        Item = Result<jftrade_calendar::CalendarSnapshot, jftrade_calendar::CalendarSourceError>,
    >,
) -> Arc<jftrade_calendar::CalendarManager> {
    let mut registry = jftrade_calendar::CalendarSourceRegistry::default();
    registry
        .register(Arc::new(FixtureCalendarSource::new(responses)))
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

#[derive(Debug)]
struct FixtureWatchlistMembershipSnapshotPort {
    memberships: std::collections::BTreeMap<String, jftrade_watchlist::Memberships>,
}

impl WatchlistMembershipSnapshotPort for FixtureWatchlistMembershipSnapshotPort {
    fn memberships(
        &self,
        instrument_id: &str,
    ) -> Result<jftrade_watchlist::Memberships, WatchlistMembershipSnapshotError> {
        Ok(self
            .memberships
            .get(instrument_id)
            .cloned()
            .unwrap_or_else(|| jftrade_watchlist::Memberships {
                instrument_id: instrument_id.to_owned(),
                revision: 0,
                groups: Vec::new(),
            }))
    }
}

#[derive(Debug)]
struct FailingWatchlistMembershipSnapshotPort;

impl WatchlistMembershipSnapshotPort for FailingWatchlistMembershipSnapshotPort {
    fn memberships(
        &self,
        _instrument_id: &str,
    ) -> Result<jftrade_watchlist::Memberships, WatchlistMembershipSnapshotError> {
        Err(WatchlistMembershipSnapshotError::Unavailable(
            "Go watchlist membership fixture unavailable".to_owned(),
        ))
    }
}

#[derive(Debug)]
struct FixturePluginUninstallGuidanceSnapshotPort {
    guidance: std::collections::BTreeMap<String, PluginUninstallGuidance>,
}

impl PluginUninstallGuidanceSnapshotPort for FixturePluginUninstallGuidanceSnapshotPort {
    fn guidance(
        &self,
        plugin_id: &str,
    ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError> {
        Ok(self.guidance.get(plugin_id).cloned())
    }
}

#[derive(Debug)]
struct FailingPluginUninstallGuidanceSnapshotPort;

impl PluginUninstallGuidanceSnapshotPort for FailingPluginUninstallGuidanceSnapshotPort {
    fn guidance(
        &self,
        _plugin_id: &str,
    ) -> Result<Option<PluginUninstallGuidance>, PluginUninstallGuidanceSnapshotError> {
        Err(PluginUninstallGuidanceSnapshotError::Unavailable(
            "Go plugin catalog fixture unavailable".to_owned(),
        ))
    }
}

#[path = "product_strategy_definitions_tests.rs"]
mod strategy_definition_tests;

#[path = "product_plugins_tests.rs"]
mod plugin_tests;

#[path = "product_alerts_write_product_tests.rs"]
mod alerts_write_product_tests;

#[path = "product_adk_chat_stream_product_tests.rs"]
mod adk_chat_stream_product_tests;
#[path = "product_plugins_write_product_tests.rs"]
mod plugins_write_product_tests;

#[path = "product_research_screen_write_product_tests.rs"]
mod research_screen_write_product_tests;
#[path = "product_strategy_research_write_product_tests.rs"]
mod strategy_research_write_product_tests;

#[path = "product_auth_session_write_product_tests.rs"]
mod auth_session_write_product_tests;

#[path = "product_watchlist_remote_write_product_tests.rs"]
mod watchlist_remote_write_product_tests;

#[path = "product_watchlist_write_product_tests.rs"]
mod watchlist_write_product_tests;

#[path = "product_backtests_write_product_tests.rs"]
mod backtests_write_product_tests;

#[path = "product_adk_mutation_product_tests.rs"]
mod adk_mutation_product_tests;

#[path = "product_strategy_runtime_write_product_tests.rs"]
mod strategy_runtime_write_product_tests;

#[path = "product_execution_write_product_tests.rs"]
mod execution_write_product_tests;

#[path = "product_system_write_product_tests.rs"]
mod system_write_product_tests;

#[path = "product_market_data_subscription_mutation_product_tests.rs"]
mod market_data_subscription_mutation_product_tests;

#[path = "product_brokers_write_product_tests.rs"]
mod brokers_write_product_tests;

#[path = "product_watchlist_tests.rs"]
mod watchlist_read_tests;

#[path = "product_portfolio_tests.rs"]
mod portfolio_tests;

#[path = "product_research_tests.rs"]
mod research_read_tests;

#[path = "product_research_preset_tests.rs"]
mod research_preset_read_tests;

#[path = "product_execution_read_tests.rs"]
mod execution_read_tests;

#[path = "product_market_data_provider_read_tests.rs"]
mod market_data_provider_read_tests;

#[path = "product_market_data_catalog_read_tests.rs"]
mod market_data_catalog_read_tests;

#[path = "product_market_data_derivative_read_tests.rs"]
mod market_data_derivative_read_tests;

#[path = "product_market_data_options_read_tests.rs"]
mod market_data_options_read_tests;

#[path = "product_market_data_news_actions_read_tests.rs"]
mod market_data_news_actions_read_tests;

#[path = "product_market_data_news_search_read_tests.rs"]
mod market_data_news_search_read_tests;

#[path = "product_market_data_quote_read_tests.rs"]
mod market_data_quote_read_tests;

#[path = "product_market_data_prediction_read_tests.rs"]
mod market_data_prediction_read_tests;

#[path = "product_adk_read_tests.rs"]
mod adk_read_tests;

#[path = "product_brokers_tests.rs"]
mod broker_read_tests;

#[path = "product_watchlists_tests.rs"]
mod remote_watchlist_tests;

#[path = "product_system_read_tests.rs"]
mod system_read_tests;

#[path = "product_appearance_read_tests.rs"]
mod appearance_read_tests;

#[path = "product_alerts_read_tests.rs"]
mod alerts_read_tests;

#[path = "product_backtests_tests.rs"]
mod backtests_read_tests;

#[path = "product_backtests_sync_tests.rs"]
mod backtests_sync_read_tests;

#[path = "product_strategies_tests.rs"]
mod strategy_read_tests;

#[path = "product_auth_session_tests.rs"]
mod auth_session_tests;

#[path = "product_ws_live_tests.rs"]
mod ws_live_tests;

#[path = "product_strategy_pine_tests.rs"]
mod strategy_pine_tests;

#[derive(Debug)]
struct FixtureAlertSnapshotPort {
    price: Value,
    option_events: Value,
}

impl AlertSnapshotPort for FixtureAlertSnapshotPort {
    fn snapshot(&self, kind: AlertKind, _raw_query: &str) -> Result<Value, AlertSnapshotError> {
        Ok(match kind {
            AlertKind::Price => self.price.clone(),
            AlertKind::OptionEvents => self.option_events.clone(),
        })
    }
}

#[tokio::test]
async fn product_server_persists_ui_settings_and_reports_actual_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let real_trade_control_path = directory.path().join("real-trade-control.json");
    std::fs::write(
            &real_trade_control_path,
            r#"{
                "riskConfig": {
                    "id": "risk-1",
                    "tradingEnvironment": "REAL",
                    "realTradingEnabled": true,
                    "maxOrderQuantity": 12.5,
                    "maxOrderNotional": 2500,
                    "operatorId": "operator",
                    "reason": "market open",
                    "activatedAt": "2026-08-20T01:00:00Z",
                    "updatedAt": "2026-08-20T01:00:00Z"
                },
                "killSwitch": {
                    "id": "kill-switch-control-plane",
                    "tradingEnvironment": "REAL",
                    "operatorId": "operator",
                    "reason": "incident",
                    "activatedAt": "2026-08-20T01:01:00Z",
                    "updatedAt": "2026-08-20T01:01:00Z"
                },
                "hardStops": [{
                    "id": "hard-stop-1",
                    "brokerId": "futu",
                    "tradingEnvironment": "REAL",
                    "accountId": "ACC-1",
                    "market": "US",
                    "symbol": "AAPL",
                    "hardStopScope": "SYMBOL",
                    "operatorId": "operator",
                    "reason": "incident",
                    "activatedAt": "2026-08-20T01:02:00Z",
                    "updatedAt": "2026-08-20T01:02:00Z"
                }],
                "events": [
                    {"id":"event-risk","eventType":"updated","action":"RISK_CONFIG_UPDATE","brokerId":"*","createdAt":"2026-08-20T01:03:00Z"},
                    {"id":"event-kill","eventType":"activated","action":"KILL_SWITCH_ACTIVATE","brokerId":"*","createdAt":"2026-08-20T01:02:00Z"},
                    {"id":"event-hard-stop","eventType":"activated","action":"HARD_STOP_ACTIVATE","brokerId":"futu","createdAt":"2026-08-20T01:01:00Z"}
                ]
            }"#,
        )
        .expect("seed real-trade control state");
    let handle = start_product(
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_notification_port(Arc::new(DeliveredNotification)),
    )
    .await
    .expect("start product");
    let address = handle.startup_record().address;
    let startup = handle.startup_record();
    assert_eq!(startup.owned_routes, 48);
    assert_eq!(startup.protocol_version, PRODUCT_REHEARSAL_PROTOCOL_VERSION);
    assert_eq!(startup.route_profile, PRODUCT_TEST_CUTOVER_ROUTE_PROFILE);
    assert_eq!(startup.capabilities.len(), startup.owned_routes);
    assert_eq!(
        startup.route_profile_digest,
        route_profile_digest(&startup.capabilities)
    );
    assert_eq!(startup.resource_sha256.len(), 64);
    assert!(
        startup
            .resource_sha256
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit())
    );

    let status = request_json(address, "GET", "/api/v1/system/status", None).await;
    assert_eq!(status["ok"], true);
    assert_eq!(status["data"]["apiPort"], address.port());
    assert_eq!(status["data"]["name"], "JFTrade");
    assert_eq!(status["data"]["realTradingEnabled"], true);
    assert_eq!(status["data"]["realTradingKillSwitch"]["active"], true);
    assert_eq!(status["data"]["realTradingRisk"]["maxOrderQuantity"], 12.5);

    let agent_templates = request_json(address, "GET", "/api/v1/adk/agent-templates", None).await;
    assert_eq!(agent_templates["ok"], true);
    assert_eq!(agent_templates["data"], agent_templates_wire());

    let mcp_rejected = request_json(
        address,
        "PUT",
        "/api/v1/settings/adk/mcp",
        Some(r#"{"enabled":true,"port":6697,"authMode":"token"}"#),
    )
    .await;
    assert_eq!(
        mcp_rejected["error"]["code"],
        "MCP_SERVER_SETTINGS_REJECTED"
    );
    let mcp_reset = request_json(
        address,
        "POST",
        "/api/v1/settings/adk/mcp/token/reset",
        None,
    )
    .await;
    let mcp_token = mcp_reset["data"]["token"]
        .as_str()
        .expect("one-time MCP token");
    assert!(mcp_token.starts_with("jft_mcp_"));
    assert_eq!(mcp_reset["data"]["settings"]["tokenConfigured"], true);
    let mcp_saved = request_json(
        address,
        "PUT",
        "/api/v1/settings/adk/mcp",
        Some(r#"{"enabled":true,"port":6697,"authMode":"token"}"#),
    )
    .await;
    assert_eq!(mcp_saved["data"]["settings"]["enabled"], true);
    assert!(!mcp_saved.to_string().contains(mcp_token));
    let mcp_readback = request_json(address, "GET", "/api/v1/settings/adk/mcp", None).await;
    assert!(!mcp_readback.to_string().contains(mcp_token));
    assert!(!mcp_readback.to_string().contains("tokenHash"));
    let persisted_settings = std::fs::read_to_string(&settings_path).expect("settings file");
    assert!(!persisted_settings.contains(mcp_token));
    assert!(persisted_settings.contains("$argon2id$v=19$m=65536,t=3,p=1$"));

    let approvals = request_json(address, "GET", "/api/v1/system/real-trade-approvals", None).await;
    assert_eq!(approvals["data"]["realTradingEnabled"], true);
    assert_eq!(approvals["data"]["entries"], json!([]));

    let hard_stops =
        request_json(address, "GET", "/api/v1/system/real-trade-hard-stops", None).await;
    assert_eq!(hard_stops["data"]["entries"][0]["id"], "hard-stop-1");

    let hard_stop_events = request_json(
        address,
        "GET",
        "/api/v1/system/real-trade-hard-stop-events",
        None,
    )
    .await;
    assert_eq!(
        hard_stop_events["data"]["entries"][0]["id"],
        "event-hard-stop"
    );

    let kill_switch = request_json(
        address,
        "GET",
        "/api/v1/system/real-trade-kill-switch",
        None,
    )
    .await;
    assert_eq!(kill_switch["data"]["killSwitchSource"], "RUNTIME");
    assert_eq!(
        kill_switch["data"]["entry"]["id"],
        "kill-switch-control-plane"
    );

    let kill_switch_events = request_json(
        address,
        "GET",
        "/api/v1/system/real-trade-kill-switch-events",
        None,
    )
    .await;
    assert_eq!(kill_switch_events["data"]["entries"][0]["id"], "event-kill");

    let risk_limits = request_json(
        address,
        "GET",
        "/api/v1/system/real-trade-risk-limits",
        None,
    )
    .await;
    assert_eq!(risk_limits["data"]["entry"]["id"], "risk-1");
    assert_eq!(risk_limits["data"]["effectiveMaxOrderNotional"], 2500.0);

    let risk_events = request_json(
        address,
        "GET",
        "/api/v1/system/real-trade-risk-events",
        None,
    )
    .await;
    assert_eq!(risk_events["data"]["entries"][0]["id"], "event-risk");
    assert_eq!(risk_events["data"]["maxOrderQuantity"], 12.5);

    let dependencies =
        request_json(address, "GET", "/api/v1/system/runtime-dependencies", None).await;
    assert_eq!(dependencies["ok"], true);
    assert_eq!(dependencies["data"]["dependencies"][0]["id"], "node");
    assert_eq!(
        dependencies["data"]["dependencies"][0]["minimumVersion"],
        "22.0.0"
    );

    let install_guide = request_json(
        address,
        "GET",
        "/api/v1/system/futu-opend/install-guide",
        None,
    )
    .await;
    assert_eq!(install_guide["data"]["settings"]["host"], "127.0.0.1");
    assert_eq!(
        install_guide["data"]["settings"]["minimumVersion"],
        "10.9.6908"
    );
    assert!(
        install_guide["data"]["settings"]
            .get("websocketKey")
            .is_none()
    );

    let storage = request_json(address, "GET", "/api/v1/system/storage/overview", None).await;
    assert_eq!(storage["data"]["pendingOutbox"], json!([]));
    assert_eq!(storage["data"]["recentExecutionCommands"], json!([]));

    let databases = request_json(
        address,
        "GET",
        "/api/v1/settings/data-management/databases?summaryOnly=TRUE&databaseId=%20strategy%20",
        None,
    )
    .await;
    assert_eq!(databases["data"]["databases"][0]["id"], "strategy");
    assert_eq!(databases["data"]["databases"][0]["expectedVersion"], 2);
    assert_eq!(
        databases["data"]["databases"][0]["storage"]["totalBytes"],
        0
    );
    assert_eq!(databases["data"]["databases"][0]["cleanable"], Value::Null);

    let security = request_json(address, "GET", "/api/v1/settings/security", None).await;
    assert_eq!(security["data"]["webAccessEnabled"], false);
    assert_eq!(security["data"]["publicAccessEnabled"], false);
    assert_eq!(security["data"]["webPort"], 6688);
    assert_eq!(security["data"]["passwordConfigured"], false);
    assert!(security["data"].get("passwordHash").is_none());
    let invalid_security = request_json(
            address,
            "PUT",
            "/api/v1/settings/security",
            Some(
                r#"{"webAccessEnabled":true,"publicAccessEnabled":true,"webPort":6688,"newPassword":"short"}"#,
            ),
        )
        .await;
    assert_eq!(
        invalid_security["error"]["code"],
        "INVALID_WEB_ACCESS_PASSWORD"
    );
    let saved_security = request_json(
            address,
            "PUT",
            "/api/v1/settings/security",
            Some(
                r#"{"webAccessEnabled":true,"publicAccessEnabled":true,"webPort":6688,"newPassword":"a sufficiently long password"}"#,
            ),
        )
        .await;
    assert_eq!(saved_security["data"]["webAccessEnabled"], true);
    assert_eq!(saved_security["data"]["publicAccessEnabled"], true);
    assert_eq!(saved_security["data"]["passwordConfigured"], true);
    assert!(saved_security["data"].get("passwordHash").is_none());
    assert!(
        !saved_security
            .to_string()
            .contains("a sufficiently long password")
    );

    let onboarding = request_json(address, "GET", "/api/v1/settings/onboarding", None).await;
    assert_eq!(onboarding["data"]["state"]["completed"], false);
    assert_eq!(onboarding["data"]["recommendedBrokerId"], "futu");
    assert_eq!(onboarding["data"]["brokers"][0]["descriptor"]["id"], "futu");
    assert_eq!(onboarding["data"]["brokers"][0]["configured"], false);

    let saved_onboarding = request_json(
        address,
        "PUT",
        "/api/v1/settings/onboarding",
        Some(r#"{"completed":true,"lastBrokerId":" futu "}"#),
    )
    .await;
    assert_eq!(saved_onboarding["data"]["state"]["completed"], true);
    assert_eq!(saved_onboarding["data"]["state"]["lastBrokerId"], "futu");
    assert!(
        saved_onboarding["data"]["state"]["completedAt"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );

    let brokers = request_json(address, "GET", "/api/v1/settings/brokers", None).await;
    assert_eq!(brokers["data"]["brokers"][0]["descriptor"]["id"], "futu");
    assert_eq!(
        brokers["data"]["brokers"][0]["integration"],
        serde_json::Value::Null
    );
    assert_eq!(brokers["data"]["brokers"][0]["defaults"]["apiPort"], 11110);
    assert_eq!(brokers["data"]["accounts"], json!([]));

    let integration = request_json(
            address,
            "PUT",
            "/api/v1/settings/brokers/ignored/integration",
            Some(
                r#"{"enabled":true,"config":{"host":" ","apiPort":0,"websocketPort":0,"maxWebSocketConnections":0,"useEncryption":true,"websocketKey":"secret"}}"#,
            ),
        )
        .await;
    assert_eq!(integration["data"]["brokerId"], "futu");
    assert_eq!(integration["data"]["config"]["host"], "127.0.0.1");
    assert_eq!(integration["data"]["config"]["useEncryption"], false);
    assert_eq!(integration["data"]["config"]["websocketKey"], "secret");
    assert!(
        integration["data"]["createdAt"]
            .as_str()
            .is_some_and(|value| !value.is_empty())
    );

    let invalid_account = request_json(
        address,
        "POST",
        "/api/v1/settings/broker-accounts",
        Some(r#"{"accountId":" "}"#),
    )
    .await;
    assert_eq!(invalid_account["ok"], false);
    assert_eq!(invalid_account["error"]["code"], "BAD_REQUEST");

    let created_account = request_json(
            address,
            "POST",
            "/api/v1/settings/broker-accounts",
            Some(
                r#"{"id":"client-id","brokerId":" FUTU ","accountId":" ACC-1 ","displayName":" ","tradingEnvironment":" real ","market":" us ","securityFirm":" ","enabled":true,"createdAt":"client","updatedAt":"client"}"#,
            ),
        )
        .await;
    assert_eq!(created_account["data"]["id"], "futu|REAL|ACC-1|US");
    assert_eq!(created_account["data"]["displayName"], "ACC-1");
    assert_eq!(created_account["data"]["securityFirm"], Value::Null);

    let updated_account = request_json(
            address,
            "PUT",
            "/api/v1/settings/broker-accounts/futu%7CREAL%7CACC-1%7CUS",
            Some(
                r#"{"accountId":"ACC-1","displayName":"Updated","tradingEnvironment":"real","market":"us","enabled":false}"#,
            ),
        )
        .await;
    assert_eq!(updated_account["data"]["id"], "futu|REAL|ACC-1|US");
    assert_eq!(updated_account["data"]["displayName"], "Updated");
    assert_eq!(updated_account["data"]["enabled"], false);

    let persisted_brokers = request_json(address, "GET", "/api/v1/settings/brokers", None).await;
    assert_eq!(
        persisted_brokers["data"]["brokers"][0]["integration"]["config"]["websocketKey"],
        "secret"
    );
    assert_eq!(
        persisted_brokers["data"]["accounts"][0]["displayName"],
        "Updated"
    );

    let deleted_account = request_json(
        address,
        "DELETE",
        "/api/v1/settings/broker-accounts/futu%7CREAL%7CACC-1%7CUS",
        None,
    )
    .await;
    assert_eq!(deleted_account["data"]["deleted"], true);
    assert_eq!(deleted_account["data"]["id"], "futu|REAL|ACC-1|US");

    let mcp = request_json(address, "GET", "/api/v1/settings/adk/mcp", None).await;
    assert_eq!(mcp["data"]["settings"]["port"], 6697);
    assert_eq!(mcp["data"]["settings"]["authMode"], "token");
    assert_eq!(mcp["data"]["settings"]["tokenConfigured"], true);
    assert_eq!(mcp["data"]["status"]["running"], false);
    assert!(mcp["data"]["settings"].get("tokenHash").is_none());

    let provider = request_json(
        address,
        "GET",
        "/api/v1/settings/market-data-provider",
        None,
    )
    .await;
    assert_eq!(provider["data"]["activeProvider"], "akshare");

    let backtest_provider = request_json(
        address,
        "GET",
        "/api/v1/settings/backtest-market-data-provider",
        None,
    )
    .await;
    assert_eq!(backtest_provider["data"]["activeProvider"], "akshare");
    assert_eq!(
        backtest_provider["data"]["availableProviders"][0]["selectionId"],
        "futu"
    );
    assert_eq!(
        backtest_provider["data"]["availableProviders"][1]["selectionId"],
        "yfinance"
    );
    assert_eq!(
        backtest_provider["data"]["availableProviders"][2]["selectionId"],
        "akshare"
    );

    let saved_provider = request_json(
        address,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(r#"{"activeProvider":" YFINANCE "}"#),
    )
    .await;
    assert_eq!(saved_provider["data"]["activeProvider"], "yfinance");
    let invalid_provider = request_json(
        address,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(r#"{"activeProvider":"invalid"}"#),
    )
    .await;
    assert_eq!(invalid_provider["ok"], false);
    assert_eq!(
        invalid_provider["error"]["code"],
        "MARKET_DATA_PROVIDER_INVALID"
    );

    let saved_backtest_provider = request_json(
        address,
        "PUT",
        "/api/v1/settings/backtest-market-data-provider",
        Some(r#"{"activeProvider":" futu "}"#),
    )
    .await;
    assert_eq!(saved_backtest_provider["data"]["activeProvider"], "futu");
    assert_eq!(
        saved_backtest_provider["data"]["availableProviders"][0]["selectionId"],
        "futu"
    );

    let calendars = request_json(address, "GET", "/api/v1/settings/exchange-calendars", None).await;
    assert_eq!(
        calendars["data"]["exchangeCalendars"]["refreshIntervalHours"],
        24
    );
    assert_eq!(
        calendars["data"]["exchangeCalendars"]["warmupMarkets"],
        json!(["US", "HK", "CN"])
    );

    let saved_calendars = request_json(
            address,
            "PUT",
            "/api/v1/settings/exchange-calendars",
            Some(
                r#"{"exchangeCalendars":{"autoRefreshEnabled":false,"errorNotificationsEnabled":false,"refreshIntervalHours":999,"warmupMarkets":[" us ","US"," hk "]}}"#,
            ),
        )
        .await;
    assert_eq!(
        saved_calendars["data"]["exchangeCalendars"]["refreshIntervalHours"],
        720
    );
    assert_eq!(
        saved_calendars["data"]["exchangeCalendars"]["warmupMarkets"],
        json!(["US", "HK"])
    );
    assert_eq!(
        saved_calendars["data"]["exchangeCalendars"]["errorNotificationsEnabled"],
        false
    );

    let execution = request_json(
            address,
            "PUT",
            "/api/v1/settings/execution",
            Some(
                r#"{"defaultTradingEnvironment":" real ","brokerOrderHistoryLookbackDays":999,"seenFillRetentionDays":0}"#,
            ),
        )
        .await;
    assert_eq!(execution["data"]["defaultTradingEnvironment"], "REAL");
    assert_eq!(execution["data"]["brokerOrderHistoryLookbackDays"], 365);
    assert_eq!(execution["data"]["seenFillRetentionDays"], 90);

    let adk = request_json(
        address,
        "PUT",
        "/api/v1/settings/adk",
        Some(r#"{"runTimeoutMs":1,"streamIdleTimeoutMs":9999999}"#),
    )
    .await;
    assert_eq!(adk["data"]["runTimeoutMs"], 60_000);
    assert_eq!(adk["data"]["streamIdleTimeoutMs"], 900_000);

    let notification_test = request_json(
        address,
        "POST",
        "/api/v1/settings/system-notifications/test",
        None,
    )
    .await;
    assert_eq!(
        notification_test["data"]["event"]["id"],
        "system-notification-1"
    );
    assert_eq!(notification_test["data"]["delivery"]["status"], "delivered");

    let notifications = request_json(
            address,
            "PUT",
            "/api/v1/settings/system-notifications",
            Some(
                r#"{"enabled":true,"mode":"custom","levels":[" WARN ","warn","Error"],"categories":["a"," a ","B"],"soundEnabled":false}"#,
            ),
        )
        .await;
    assert_eq!(notifications["data"]["levels"], json!(["warn", "error"]));
    assert_eq!(notifications["data"]["categories"], json!(["a", "B"]));

    let saved = request_json(
        address,
        "PUT",
        "/api/v1/settings/ui",
        Some(r#"{"appearance":{"upColor":" #ABCDEF ","downColor":"bad"}}"#),
    )
    .await;
    assert_eq!(saved["data"]["appearance"]["upColor"], "#abcdef");
    assert_eq!(saved["data"]["appearance"]["downColor"], "#ea3943");

    std::fs::write(&real_trade_control_path, "{").expect("corrupt real-trade control state");
    let fail_closed = request_json(
        address,
        "GET",
        "/api/v1/system/real-trade-kill-switch",
        None,
    )
    .await;
    assert_eq!(fail_closed["data"]["realTradingEnabled"], true);
    assert_eq!(fail_closed["data"]["killSwitchActive"], true);
    assert_eq!(fail_closed["data"]["entry"], serde_json::Value::Null);
    handle.shutdown().await.expect("shutdown product");

    let reloaded = SettingsFileStore::open(&settings_path).expect("reload settings");
    assert_eq!(
        reloaded
            .load_appearance()
            .expect("appearance")
            .expect("saved appearance")
            .up_color,
        "#abcdef"
    );
}

#[tokio::test]
async fn cleanup_preview_route_returns_candidates_and_rejects_bad_payloads() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let database_path = directory.path().join("backtest-runs.db");
    let connection = Connection::open(&database_path).expect("open backtest database");
    connection
            .execute_batch(
                r#"PRAGMA journal_mode = DELETE;
                 CREATE TABLE backtest_runs (
                    id TEXT PRIMARY KEY,
                    status TEXT NOT NULL DEFAULT '',
                    request_json TEXT NOT NULL DEFAULT '',
                    result_json TEXT NOT NULL DEFAULT '',
                    created_at TEXT NOT NULL DEFAULT '',
                    updated_at TEXT NOT NULL DEFAULT ''
                 );
                 CREATE INDEX idx_backtest_runs_updated_at ON backtest_runs (updated_at DESC, id ASC);
                 CREATE INDEX idx_backtest_runs_status ON backtest_runs (status, updated_at DESC);
                 CREATE TABLE jftrade_schema_meta (
                    component_id TEXT PRIMARY KEY,
                    version INTEGER NOT NULL,
                    created_at TEXT NOT NULL
                 );
                 INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                 VALUES ('backtest-runs', 1, 'test');
                 INSERT INTO backtest_runs
                    (id, status, request_json, result_json, created_at, updated_at)
                 VALUES
                    ('latest', 'completed', '{}', '{}', '2999-01-01T00:00:00Z', '2999-01-01T00:00:00Z'),
                    ('expired', 'failed', '{"x":1}', '{"y":2}', '2000-01-01T00:00:00Z', '2000-01-01T00:00:00Z');"#,
            )
            .expect("seed backtest database");
    drop(connection);

    let handle = start_product(
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config"),
    )
    .await
    .expect("start product");
    let address = handle.startup_record().address;

    let preview = request_json(
            address,
            "POST",
            "/api/v1/settings/data-management/cleanup/preview",
            Some(
                r#"{"kind":"backtest-history","databaseId":"backtest-runs","olderThanDays":1,"keepLatest":1}"#,
            ),
        )
        .await;
    assert_eq!(preview["ok"], true);
    assert_eq!(preview["data"]["databaseId"], "backtest-runs");
    assert_eq!(preview["data"]["candidateCount"], 1);
    assert_eq!(preview["data"]["items"][0]["kind"], "回测结果");
    assert_eq!(
        preview["data"]["confirmationText"],
        "CLEANUP backtest-runs 1"
    );
    assert_eq!(preview["data"]["willCompact"], true);

    let changed = Connection::open(&database_path).expect("reopen changed candidates");
    changed
        .execute(
            "INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at) VALUES ('new-expired', 'failed', '{}', '{}', '2000-01-01T00:00:00Z', '2000-01-01T00:00:00Z')",
            [],
        )
        .expect("add candidate after preview");
    drop(changed);
    let stale = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/execute",
        Some(&format!(
            r#"{{"previewId":"{}","confirmation":"CLEANUP backtest-runs 1"}}"#,
            preview["data"]["previewId"].as_str().expect("preview id")
        )),
    )
    .await;
    assert_eq!(stale["error"]["code"], "CLEANUP_PREVIEW_STALE");
    let changed = Connection::open(&database_path).expect("reopen stale database");
    let row_count: i64 = changed
        .query_row("SELECT COUNT(*) FROM backtest_runs", [], |row| row.get(0))
        .expect("count rows after stale cleanup");
    assert_eq!(row_count, 3, "stale cleanup must not mutate the database");
    changed
        .execute("DELETE FROM backtest_runs WHERE id = 'new-expired'", [])
        .expect("restore exact candidates");
    drop(changed);

    let approved = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/preview",
        Some(
            r#"{"kind":"backtest-history","databaseId":"backtest-runs","olderThanDays":1,"keepLatest":1}"#,
        ),
    )
    .await;
    let execute_body = format!(
        r#"{{"previewId":"{}","confirmation":"{}"}}"#,
        approved["data"]["previewId"].as_str().expect("preview id"),
        approved["data"]["confirmationText"]
            .as_str()
            .expect("confirmation")
    );
    let wrong_confirmation = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/execute",
        Some(&format!(
            r#"{{"previewId":"{}","confirmation":"WRONG"}}"#,
            approved["data"]["previewId"].as_str().expect("preview id")
        )),
    )
    .await;
    assert_eq!(
        wrong_confirmation["error"]["code"],
        "DATABASE_CLEANUP_FAILED"
    );
    let executed = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/execute",
        Some(&execute_body),
    )
    .await;
    assert_eq!(executed["data"]["deletedCount"], 1);
    assert_eq!(executed["data"]["compacted"], true);
    let repeated = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/execute",
        Some(&execute_body),
    )
    .await;
    assert_eq!(repeated["error"]["code"], "CLEANUP_PREVIEW_NOT_FOUND");

    let backup = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/databases/backtest-runs/backup",
        Some(r#"{"confirmation":"BACKUP backtest-runs"}"#),
    )
    .await;
    assert_eq!(backup["data"]["databaseId"], "backtest-runs");
    assert!(
        std::path::Path::new(backup["data"]["backupPath"].as_str().expect("backup path")).is_file()
    );
    let compact = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/databases/backtest-runs/compact",
        Some(r#"{"confirmation":"COMPACT backtest-runs"}"#),
    )
    .await;
    assert_eq!(compact["data"]["compacted"], true);
    let rebuild = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/databases/rebuild",
        Some(
            r#"{"databaseIds":["backtest-runs"],"mode":"single","confirmation":"REBUILD backtest-runs"}"#,
        ),
    )
    .await;
    assert_eq!(rebuild["data"]["scheduled"], true);
    let marker_path = directory.path().join("database-rebuild.json");
    assert!(marker_path.is_file());

    let source = Connection::open(&database_path).expect("reopen source after maintenance");
    let quick_check: String = source
        .query_row("PRAGMA quick_check", [], |row| row.get(0))
        .expect("check maintained source database");
    assert_eq!(quick_check, "ok");
    let remaining: Vec<String> = source
        .prepare("SELECT id FROM backtest_runs ORDER BY id")
        .expect("prepare source row query")
        .query_map([], |row| row.get(0))
        .expect("query source rows")
        .collect::<Result<_, _>>()
        .expect("collect source rows");
    assert_eq!(remaining, vec!["latest"]);
    drop(source);

    let marker: Value =
        serde_json::from_slice(&std::fs::read(&marker_path).expect("read rebuild marker"))
            .expect("decode rebuild marker");
    let rebuild_backup = marker["backups"][0]["path"]
        .as_str()
        .expect("rebuild backup path");
    std::fs::write(rebuild_backup, b"interrupted backup").expect("corrupt rebuild backup");
    let rejected_rebuild = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/databases/rebuild",
        Some(
            r#"{"databaseIds":["backtest-runs"],"mode":"single","confirmation":"REBUILD backtest-runs"}"#,
        ),
    )
    .await;
    assert_eq!(
        rejected_rebuild["error"]["code"],
        "DATABASE_REBUILD_REJECTED"
    );
    let source = Connection::open(&database_path).expect("reopen source after rejected rebuild");
    let quick_check: String = source
        .query_row("PRAGMA quick_check", [], |row| row.get(0))
        .expect("check source after rejected rebuild");
    assert_eq!(quick_check, "ok");

    let malformed = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/preview",
        Some(r#"{"kind":"#),
    )
    .await;
    assert_eq!(malformed["error"]["code"], "BAD_REQUEST");

    let rejected = request_json(
        address,
        "POST",
        "/api/v1/settings/data-management/cleanup/preview",
        Some(r#"{"kind":"unknown","databaseId":"backtest-runs"}"#),
    )
    .await;
    assert_eq!(
        rejected["error"]["code"],
        "DATABASE_CLEANUP_PREVIEW_REJECTED"
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn research_screen_catalog_route_matches_static_catalog_variants() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let handle = start_product(
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config"),
    )
    .await
    .expect("start product");
    let address = handle.startup_record().address;

    let futu = request_json(
        address,
        "GET",
        "/api/v1/research/screens/catalog?brokerId=futu&market=US",
        None,
    )
    .await;
    assert_eq!(futu["ok"], true);
    assert_eq!(futu["data"]["version"], "futu-stock-screen-v1");
    assert_eq!(futu["data"]["provider"], "futu");
    assert_eq!(futu["data"]["market"], "US");
    assert_eq!(futu["data"]["factors"].as_array().map(Vec::len), Some(402));
    assert!(futu.to_string().find("providerId").is_none());

    let embedded = request_json(
        address,
        "GET",
        "/api/v1/research/screens/catalog?brokerId=yfinance",
        None,
    )
    .await;
    assert_eq!(embedded["ok"], true);
    assert_eq!(embedded["data"]["version"], "embedded-stock-screen-v1");
    assert_eq!(embedded["data"]["provider"], "yfinance");
    assert_eq!(
        embedded["data"]["factors"].as_array().map(Vec::len),
        Some(9)
    );

    let unsupported_market = request_json(
        address,
        "GET",
        "/api/v1/research/screens/catalog?brokerId=yfinance&market=HK",
        None,
    )
    .await;
    assert_eq!(unsupported_market["error"]["code"], "BAD_REQUEST");
    let unavailable = request_json(
        address,
        "GET",
        "/api/v1/research/screens/catalog?brokerId=unknown",
        None,
    )
    .await;
    assert_eq!(
        unavailable["error"]["code"],
        "BROKER_CAPABILITY_UNAVAILABLE"
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn research_screen_catalog_route_matches_go_fixture_for_all_variants() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/research-screen-catalogs.json"
    ))
    .expect("research screen catalog fixture");
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let handle = start_product(
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config"),
    )
    .await
    .expect("start product");
    let address = handle.startup_record().address;
    for (broker, market) in [
        ("futu", ""),
        ("futu", "HK"),
        ("futu", "US"),
        ("futu", "SH"),
        ("futu", "SZ"),
        ("yfinance", ""),
        ("yfinance", "US"),
        ("akshare", ""),
        ("akshare", "SH"),
        ("akshare", "SZ"),
        ("akshare", "CN"),
        ("akshare", "HK"),
        ("akshare", "US"),
    ] {
        let query = format!("/api/v1/research/screens/catalog?brokerId={broker}&market={market}");
        let actual = request_json(address, "GET", &query, None).await;
        assert_eq!(actual["ok"], true, "catalog query {query}");
        let key = format!("{broker}|{market}");
        assert_eq!(
            actual["data"], fixture["catalogs"][&key],
            "catalog query {query}"
        );
    }
    for (query, code, message) in [
        (
            "brokerId=futu&market=SG",
            "BAD_REQUEST",
            "unsupported stock-screen market",
        ),
        (
            "brokerId=yfinance&market=HK",
            "BAD_REQUEST",
            "unsupported stock-screen market for yfinance",
        ),
        (
            "brokerId=akshare&market=MO",
            "BAD_REQUEST",
            "unsupported stock-screen market for akshare",
        ),
        (
            "brokerId=unknown",
            "BROKER_CAPABILITY_UNAVAILABLE",
            "the stock-screen factor catalog is not available for broker unknown",
        ),
    ] {
        let actual = request_json(
            address,
            "GET",
            &format!("/api/v1/research/screens/catalog?{query}"),
            None,
        )
        .await;
        assert_eq!(actual["ok"], false, "catalog error query {query}");
        assert_eq!(actual["error"]["code"], code, "catalog error {query}");
        assert_eq!(actual["error"]["message"], message, "catalog error {query}");
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn calendar_control_plane_routes_share_the_real_manager_in_cutover_only() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let manager = fixture_calendar_manager([
        Ok(fixture_calendar_snapshot("refresh-checksum")),
        Ok(fixture_calendar_snapshot("probe-checksum")),
    ]);
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_calendar_manager(manager);
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 54);
    let sources = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/exchange-calendars/sources",
        None,
    )
    .await;
    assert_eq!(sources["ok"], true);
    assert!(
        sources["data"]["sources"]
            .as_array()
            .expect("calendar sources")
            .iter()
            .any(|source| source["id"] == "fixture_source")
    );
    let refresh = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/system/exchange-calendars/refresh/US",
        None,
    )
    .await;
    assert_eq!(refresh["data"]["accepted"], true);
    assert_eq!(refresh["data"]["updated"], 1);
    let status = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/system/exchange-calendars/status",
        None,
    )
    .await;
    assert_eq!(status["ok"], true);
    assert_eq!(
        status["data"]["snapshots"][0]["checksum"],
        "refresh-checksum"
    );
    let probe = request_json(
        handle.startup_record().address,
        "POST",
        "/api/v1/system/exchange-calendars/probe/US",
        None,
    )
    .await;
    assert_eq!(probe["data"]["accepted"], true);
    assert_eq!(probe["data"]["healthy"], 1);
    assert_eq!(probe["data"]["results"][0]["checksum"], "probe-checksum");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn calendar_control_plane_routes_fail_closed_without_a_manager() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    for (method, path) in [
        ("GET", "/api/v1/system/exchange-calendars/sources"),
        ("GET", "/api/v1/system/exchange-calendars/status"),
        ("POST", "/api/v1/system/exchange-calendars/probe"),
        ("POST", "/api/v1/system/exchange-calendars/refresh/US"),
    ] {
        let actual = request_json(handle.startup_record().address, method, path, None).await;
        assert_eq!(actual["ok"], false, "{method} {path}");
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn calendar_unknown_market_control_requests_keep_the_go_noop_wire() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let manager = fixture_calendar_manager([]);
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_calendar_manager(manager);
    let handle = start_product(config).await.expect("start product");
    for operation in ["refresh", "probe"] {
        let actual = request_json(
            handle.startup_record().address,
            "POST",
            &format!("/api/v1/system/exchange-calendars/{operation}/MARS"),
            None,
        )
        .await;
        assert_eq!(actual["ok"], true, "{operation}");
        assert_eq!(actual["data"]["accepted"], true, "{operation}");
        assert_eq!(actual["data"]["market"], "MARS", "{operation}");
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn watchlist_memberships_route_matches_go_fixture_in_cutover_only() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/watchlist-memberships.json"
    ))
    .expect("watchlist membership fixture");
    let mut memberships = std::collections::BTreeMap::new();
    for case in fixture["cases"].as_array().expect("membership cases") {
        if let Some(response) = case.get("response") {
            let decoded: jftrade_watchlist::Memberships =
                serde_json::from_value(response.clone()).expect("membership response");
            memberships.insert(decoded.instrument_id.clone(), decoded);
        }
    }
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_membership_snapshot_port(Arc::new(
                FixtureWatchlistMembershipSnapshotPort { memberships },
            ));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    let address = handle.startup_record().address;
    for case in fixture["cases"].as_array().expect("membership cases") {
        let market = case["market"].as_str().expect("case market");
        let symbol = case["symbol"].as_str().expect("case symbol");
        let response = request_json(
            address,
            "GET",
            &format!("/api/v1/watchlist/instruments/{market}/{symbol}/memberships"),
            None,
        )
        .await;
        if case.get("response").is_some() {
            assert_eq!(response["ok"], true, "case {}", case["name"]);
            assert_eq!(response["data"], case["response"], "case {}", case["name"]);
        } else {
            assert_eq!(response["ok"], false, "case {}", case["name"]);
            assert_eq!(
                response["error"]["code"], "WATCHLIST_INVALID",
                "case {}",
                case["name"]
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn watchlist_memberships_route_fails_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_membership_snapshot_port(Arc::new(
                FailingWatchlistMembershipSnapshotPort,
            ));
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/watchlist/instruments/US/AAPL/memberships",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "WATCHLIST_UNAVAILABLE");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn plugin_uninstall_guidance_route_matches_go_fixture_in_cutover_only() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/plugin-uninstall-guidance.json"
    ))
    .expect("plugin uninstall guidance fixture");
    let mut guidance = std::collections::BTreeMap::new();
    for case in fixture["cases"].as_array().expect("plugin guidance cases") {
        if let Some(response) = case.get("response") {
            let decoded: PluginUninstallGuidance =
                serde_json::from_value(response.clone()).expect("plugin guidance response");
            guidance.insert(decoded.plugin_id.clone(), decoded);
        }
    }
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_plugin_uninstall_guidance_snapshot_port(Arc::new(
                FixturePluginUninstallGuidanceSnapshotPort { guidance },
            ));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    let address = handle.startup_record().address;
    for case in fixture["cases"].as_array().expect("plugin guidance cases") {
        let method = case["method"].as_str().expect("request method");
        let request_path = case["requestPath"].as_str().expect("request path");
        let (status, response) =
            request_json_with_status(address, method, request_path, None, &[]).await;
        assert_eq!(
            status,
            case["expectedStatus"].as_u64().expect("expected status") as u16,
            "case {}",
            case["name"]
        );
        if let Some(expected) = case.get("response") {
            assert_eq!(response["ok"], true, "case {}", case["name"]);
            assert_eq!(response["data"], *expected, "case {}", case["name"]);
        } else {
            assert_eq!(response["ok"], false, "case {}", case["name"]);
            assert_eq!(
                response["error"]["code"], case["errorCode"],
                "case {}",
                case["name"]
            );
            assert_eq!(
                response["error"]["message"], case["errorMessage"],
                "case {}",
                case["name"]
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn plugin_uninstall_guidance_route_fails_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_plugin_uninstall_guidance_snapshot_port(Arc::new(
                FailingPluginUninstallGuidanceSnapshotPort,
            ));
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/plugins/pine-plan/uninstall-guidance",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(
        response["error"]["code"],
        "PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE"
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn alerts_read_routes_match_go_fixture_as_cutover_only_batch() {
    let fixture: Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/alerts-read.json"
    ))
    .expect("alerts read fixture");
    let cases = fixture["cases"].as_array().expect("alerts cases");
    let price = cases
        .iter()
        .find(|case| case["featureId"] == "alerts.price.list")
        .and_then(|case| case.get("response"))
        .cloned()
        .expect("price alert response");
    let option_events = cases
        .iter()
        .find(|case| case["featureId"] == "alerts.option_event.list")
        .and_then(|case| case.get("response"))
        .cloned()
        .expect("option event alert response");
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_alert_snapshot_port(Arc::new(FixtureAlertSnapshotPort {
                price,
                option_events,
            }));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    let address = handle.startup_record().address;
    for case in cases {
        let request_path = case["requestPath"].as_str().expect("request path");
        let response = request_json(address, "GET", request_path, None).await;
        assert_eq!(response["ok"], true, "case {}", case["name"]);
        assert_eq!(response["data"], case["response"], "case {}", case["name"]);
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn alerts_read_routes_fail_closed_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/alerts/price?brokerId=futu&market=US",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn browser_authenticated_request_cannot_change_desktop_only_security_settings() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    config.access = AccessPolicy {
        session_token: Some("browser-session".to_owned()),
        csrf_token: Some("browser-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://jftrade.local".to_owned()]);
    let handle = start_product(config).await.expect("start product");
    let response = request_json_with_headers(
            handle.startup_record().address,
            "PUT",
            "/api/v1/settings/security",
            Some(
                r#"{"webAccessEnabled":true,"webPort":6688,"newPassword":"a sufficiently long password"}"#,
            ),
            &[
                ("Cookie", "jftrade_web_session=browser-session"),
                ("Origin", "https://jftrade.local"),
                ("X-CSRF-Token", "browser-csrf"),
            ],
        )
        .await;
    assert_eq!(
        response["error"]["code"],
        "WEB_ACCESS_SETTINGS_DESKTOP_ONLY"
    );
    handle.shutdown().await.expect("shutdown product");
    assert!(!settings_path.exists());
}

#[test]
fn product_config_rejects_public_bind_and_missing_path() {
    assert!(matches!(
        ProductConfig::test_cutover("0.0.0.0:3000".parse().expect("address"), "settings.json"),
        Err(ProductError::NonLoopbackBind)
    ));
    assert!(matches!(
        ProductConfig::test_cutover("127.0.0.1:3000".parse().expect("address"), Path::new("")),
        Err(ProductError::MissingSettingsPath)
    ));
    assert!(matches!(
        ProductConfig::desktop_shadow(
            "127.0.0.1:3000".parse().expect("address"),
            "settings.json",
            "weak"
        ),
        Err(ProductError::WeakDesktopToken)
    ));
}

#[test]
fn read_only_shadow_catalog_never_registers_write_or_notification_routes() {
    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct RouteOwnership {
        operations: Vec<OwnedRoute>,
    }

    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct OwnedRoute {
        method: String,
        path: String,
        implementation_status: String,
    }

    fn pairs(routes: &[RouteSpec]) -> Vec<(String, String)> {
        routes
            .iter()
            .map(|route| (route.method.clone(), route.path.clone()))
            .collect()
    }

    fn owned_pairs(routes: &[OwnedRoute], statuses: &[&str]) -> Vec<(String, String)> {
        let mut pairs = routes
            .iter()
            .filter(|route| statuses.contains(&route.implementation_status.as_str()))
            .map(|route| (route.method.clone(), route.path.clone()))
            .collect::<Vec<_>>();
        pairs.sort();
        pairs
    }

    const DEFAULT_REGISTERED_QUALIFIED_READS: &[&str] = &[
        "/api/v1/adk/agent-templates",
        "/api/v1/research/screens/catalog",
        "/api/v1/settings/ui",
    ];

    let ownership: RouteOwnership = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/route-ownership.json"
    ))
    .expect("route ownership ledger");
    let shadow = product_routes(
        &ProductCapabilities::default(),
        ProductRoutePorts::default(),
    )
    .expect("shadow routes");
    assert_eq!(shadow.routes().len(), 26);
    assert!(shadow.routes().iter().all(|route| route.method == "GET"));
    let shadow_capabilities = shadow
        .routes()
        .iter()
        .map(|route| format!("{} {}", route.method, route.path))
        .collect::<Vec<_>>();
    assert_eq!(
        route_profile_digest(&shadow_capabilities),
        "5f5654f93253a014d0ea113168bd49c88454f5c4c214ae9a72102a539ccf74cd"
    );
    let mut expected_shadow = owned_pairs(&ownership.operations, &["shadow"]);
    expected_shadow.extend(
        ownership
            .operations
            .iter()
            .filter(|route| {
                route.implementation_status == "cutover-qualified"
                    && DEFAULT_REGISTERED_QUALIFIED_READS.contains(&route.path.as_str())
            })
            .map(|route| (route.method.clone(), route.path.clone())),
    );
    expected_shadow.retain(|(_, path)| {
        !path.starts_with("/api/v1/alerts/") && !path.starts_with("/api/v1/plugins")
    });
    expected_shadow.sort();
    assert_eq!(pairs(shadow.routes()), expected_shadow);
    let appearance_only = product_routes(
        &ProductCapabilities::only(ProductCapability::AppearanceWrite),
        ProductRoutePorts::default(),
    )
    .expect("appearance-only routes");
    assert_eq!(appearance_only.routes().len(), 27);
    assert!(
        appearance_only
            .routes()
            .iter()
            .any(|route| { route.method == "PUT" && route.path == "/api/v1/settings/ui" })
    );
    assert!(
        !appearance_only
            .routes()
            .iter()
            .any(|route| { route.method == "PUT" && route.path == "/api/v1/settings/execution" })
    );
    let shadow_with_calendar_port = product_routes(
        &ProductCapabilities::default(),
        ProductRoutePorts {
            calendar_manager: true,
            watchlist_memberships: true,
            plugin_uninstall_guidance: true,
            ..ProductRoutePorts::default()
        },
    )
    .expect("shadow routes with unavailable cutover ports");
    assert_eq!(shadow_with_calendar_port.routes().len(), 26);
    assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
    }));
    assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
    }));
    assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
        route.method == "GET"
            && route.path == "/api/v1/watchlist/instruments/{market}/{symbol}/memberships"
    }));
    assert!(!shadow_with_calendar_port.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/plugins/{pluginId}/uninstall-guidance"
    }));
    let cutover_without_calendar_port = product_routes(
        &ProductCapabilities::test_cutover(),
        ProductRoutePorts::default(),
    )
    .expect("cutover routes without calendar ports");
    assert_eq!(cutover_without_calendar_port.routes().len(), 48);
    assert!(!cutover_without_calendar_port.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
    }));
    assert!(!cutover_without_calendar_port.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
    }));
    let cutover_with_calendar_manager = product_routes(
        &ProductCapabilities::test_cutover(),
        ProductRoutePorts {
            calendar_manager: true,
            ..ProductRoutePorts::default()
        },
    )
    .expect("cutover routes with calendar manager");
    assert_eq!(cutover_with_calendar_manager.routes().len(), 54);
    assert!(cutover_with_calendar_manager.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
    }));
    assert!(cutover_with_calendar_manager.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
    }));
    assert!(cutover_with_calendar_manager.routes().iter().any(|route| {
        route.method == "POST" && route.path == "/api/v1/system/exchange-calendars/probe/{market}"
    }));
    let cutover = product_routes(
        &ProductCapabilities::test_cutover(),
        ProductRoutePorts {
            auth_session: true,
            auth_session_write: true,
            alerts: true,
            alerts_write: true,
            calendar_manager: true,
            watchlist_memberships: true,
            watchlist_read: true,
            portfolio: true,
            research_read: true,
            research_preset_read: true,
            execution_read: true,
            execution_write: true,
            market_data_provider_read: true,
            market_data_subscription_mutation: true,
            market_data_catalog_read: true,
            market_data_derivative_read: true,
            market_data_options_read: true,
            market_data_news_actions_read: true,
            market_data_news_search_read: true,
            adk_read: true,
            adk_mutation: true,
            market_data_quote_read: true,
            market_data_prediction_read: true,
            broker_read: true,
            brokers_write: true,
            research_screen_write: true,
            remote_watchlist: true,
            remote_watchlist_write: true,
            watchlist_write: true,
            system_read: true,
            system_write: true,
            plugins: true,
            plugins_write: true,
            market_data_provider_actions: true,
            adk_chat_stream: true,
            plugin_uninstall_guidance: true,
            research_preset_write: true,
            strategy_definition_write: true,
            strategy_definitions: true,
            backtest_read: true,
            backtest_sync_read: true,
            backtests_write: true,
            strategy_read: true,
            strategy_runtime_write: true,
            strategy_pine_analyze: true,
            ws_live: true,
        },
    )
    .expect("cutover routes with all ports");
    assert_eq!(cutover.routes().len(), 278);
    let expected_cutover = owned_pairs(
        &ownership.operations,
        &["shadow", "cutover-test-only", "cutover-qualified"],
    );
    assert_eq!(pairs(cutover.routes()), expected_cutover);
    assert!(
        cutover
            .routes()
            .iter()
            .any(|route| { route.method == "PUT" && route.path == "/api/v1/settings/ui" })
    );
    assert!(cutover.routes().iter().any(|route| {
        route.method == "POST" && route.path == "/api/v1/settings/system-notifications/test"
    }));
    assert!(cutover.routes().iter().any(|route| {
        route.method == "POST" && route.path == "/api/v1/settings/adk/mcp/token/reset"
    }));
    assert!(
        cutover
            .routes()
            .iter()
            .any(|route| { route.method == "PUT" && route.path == "/api/v1/settings/security" })
    );
    assert!(
        cutover
            .routes()
            .iter()
            .any(|route| { route.method == "PUT" && route.path == "/api/v1/settings/onboarding" })
    );
    assert!(cutover.routes().iter().any(|route| {
        route.method == "PUT" && route.path == "/api/v1/settings/exchange-calendars"
    }));
    assert!(cutover.routes().iter().any(|route| {
        route.method == "DELETE"
            && route.path == "/api/v1/settings/broker-accounts/{accountRecordId}"
    }));
    assert!(cutover.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/sources"
    }));
    assert!(cutover.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/system/exchange-calendars/status"
    }));
    assert!(cutover.routes().iter().any(|route| {
        route.method == "GET"
            && route.path == "/api/v1/watchlist/instruments/{market}/{symbol}/memberships"
    }));
    assert!(cutover.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/plugins/{pluginId}/uninstall-guidance"
    }));
    assert!(
        cutover
            .routes()
            .iter()
            .any(|route| { route.method == "GET" && route.path == "/api/v1/plugins" })
    );
    assert!(cutover.routes().iter().any(|route| {
        route.method == "GET" && route.path == "/api/v1/plugins/operations/{operationId}"
    }));
    assert!(
        cutover.routes().iter().any(|route| {
            route.method == "POST" && route.path == "/api/v1/strategy-pine/analyze"
        })
    );
}

#[test]
fn alerts_write_capability_registers_without_alert_read_capability() {
    let routes = product_routes(
        &ProductCapabilities::only(ProductCapability::AlertsWrite),
        ProductRoutePorts {
            alerts_write: true,
            ..ProductRoutePorts::default()
        },
    )
    .expect("alerts write routes");
    let alert_routes: Vec<_> = routes
        .routes()
        .iter()
        .filter(|route| route.path.starts_with("/api/v1/alerts/"))
        .collect();
    assert_eq!(alert_routes.len(), 2);
    assert!(alert_routes.iter().all(|route| route.method == "POST"));
}

#[test]
fn market_data_provider_actions_register_only_with_explicit_test_port() {
    let without_port = product_routes(
        &ProductCapabilities::only(ProductCapability::MarketDataProviderActions),
        ProductRoutePorts::default(),
    )
    .expect("routes without provider-actions port");
    assert!(without_port.routes().iter().all(|route| {
        !MARKET_DATA_PROVIDER_ACTIONS_ROUTES
            .iter()
            .any(|(method, path)| route.method == *method && route.path == *path)
    }));

    let with_port = product_routes(
        &ProductCapabilities::only(ProductCapability::MarketDataProviderActions),
        ProductRoutePorts {
            market_data_provider_actions: true,
            ..ProductRoutePorts::default()
        },
    )
    .expect("provider-actions routes");
    let provider_routes: Vec<_> = with_port
        .routes()
        .iter()
        .filter(|route| {
            MARKET_DATA_PROVIDER_ACTIONS_ROUTES
                .iter()
                .any(|(method, path)| route.method == *method && route.path == *path)
        })
        .collect();
    assert_eq!(
        provider_routes.len(),
        MARKET_DATA_PROVIDER_ACTIONS_ROUTES.len()
    );
}

#[derive(Debug)]
struct FixtureMarketDataProviderActionsPort;

impl MarketDataProviderActionsPort for FixtureMarketDataProviderActionsPort {
    fn dispatch(
        &self,
        _request: &product_market_data_provider_actions_port::MarketDataProviderActionsRequest,
    ) -> Result<Value, product_market_data_provider_actions_port::MarketDataProviderActionsPortError>
    {
        Ok(Value::Null)
    }
}

#[tokio::test]
async fn market_data_provider_actions_product_registers_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_provider_actions_port(Arc::new(FixtureMarketDataProviderActionsPort));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 53);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/market-data/instruments/normalize" })
    );
    handle.shutdown().await.expect("shutdown product");
}

async fn request_json(address: SocketAddr, method: &str, path: &str, body: Option<&str>) -> Value {
    request_json_with_headers(address, method, path, body, &[]).await
}

async fn request_json_with_status(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: Option<&str>,
    headers: &[(&str, &str)],
) -> (u16, Value) {
    let body = body.unwrap_or_default();
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let extra_headers = headers
        .iter()
        .map(|(name, value)| format!("{name}: {value}\r\n"))
        .collect::<String>();
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\n{extra_headers}Content-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut response = Vec::new();
    stream
        .read_to_end(&mut response)
        .await
        .expect("read response");
    let response = String::from_utf8(response).expect("UTF-8 response");
    let (headers, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let status = headers
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    (status, serde_json::from_str(body).expect("JSON response"))
}

async fn request_json_with_headers(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: Option<&str>,
    headers: &[(&str, &str)],
) -> Value {
    request_json_with_status(address, method, path, body, headers)
        .await
        .1
}
