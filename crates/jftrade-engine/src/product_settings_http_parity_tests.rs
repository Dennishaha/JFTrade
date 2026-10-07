//! Settings transport failures and durable state through the production file owner.

use super::*;
use std::path::Path;
use std::sync::Mutex;

const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];

fn config(directory: &tempfile::TempDir, seed: Value) -> ProductConfig {
    let path = directory.path().join("settings.json");
    std::fs::write(&path, serde_json::to_vec(&seed).expect("seed JSON")).expect("settings");
    product_data_management::initialize_production_databases(&path).expect("databases");
    ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("address"),
        &path,
        "a".repeat(32),
    )
    .expect("production config")
}

async fn request(
    handle: &ProductHandle,
    method: &str,
    path: &str,
    body: Option<&str>,
) -> (u16, Value) {
    request_json_with_status(handle.startup_record().address, method, path, body, AUTH).await
}

fn document(directory: &Path) -> Value {
    serde_json::from_slice(&std::fs::read(directory.join("settings.json")).expect("file"))
        .expect("document")
}

fn seed() -> Value {
    json!({"accounts":[{"id":"record-1","brokerId":"futu","accountId":"acc-1",
        "displayName":"Original","tradingEnvironment":"SIMULATE","market":"US",
        "enabled":true,"createdAt":"2026-01-02T03:04:05Z"},
        {"id":"account-1","brokerId":"futu","accountId":"acc-2",
        "tradingEnvironment":"SIMULATE","market":"US","enabled":true}],
        "onboarding":{"completed":true,"completedAt":"2026-01-02T03:04:05Z",
        "dismissedAt":"2026-01-02T03:04:05Z","lastBrokerId":"futu"}})
}

// Parity: go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:48 TestSettingWriteRoutesMapPersistenceFailures
#[tokio::test]
async fn production_http_settings_failed_saves_preserve_state_and_pine_validation_precedes_persistence()
 {
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory, seed()))
        .await
        .expect("production");
    let paths = [
        "ui",
        "onboarding",
        "execution",
        "security",
        "system-notifications",
        "adk",
        "pine-worker",
        "exchange-calendars",
        "brokers",
    ];
    let mut before = Vec::new();
    for path in paths {
        let (status, value) =
            request(&handle, "GET", &format!("/api/v1/settings/{path}"), None).await;
        assert_eq!(status, 200, "{path}: {value}");
        before.push(value["data"].clone());
    }
    let file = directory.path().join("settings.json");
    let backup = directory.path().join("settings.saved.json");
    let bytes = std::fs::read(&file).expect("before");
    std::fs::rename(&file, &backup).expect("move file");
    std::fs::create_dir(&file).expect("block atomic replacement");
    for (method, path, body) in [
        (
            "PUT",
            "ui",
            Some(r##"{"appearance":{"upColor":"#00ff00"}}"##),
        ),
        ("PUT", "onboarding", Some(r#"{"completed":true}"#)),
        ("PUT", "execution", Some("{}")),
        ("PUT", "security", Some("{}")),
        ("PUT", "system-notifications", Some("{}")),
        ("PUT", "adk", Some("{}")),
        ("PUT", "pine-worker", Some("{}")),
        (
            "PUT",
            "pine-worker",
            Some(r#"{"backtestWorkerLimit":2,"instanceWorkerLimit":10,"nodeBinaryPath":""}"#),
        ),
        (
            "PUT",
            "exchange-calendars",
            Some(r#"{"exchangeCalendars":{}}"#),
        ),
        ("PUT", "brokers/futu/integration", Some("{}")),
        ("POST", "broker-accounts", Some(r#"{"accountId":"acc-1"}"#)),
        ("DELETE", "broker-accounts/account-1", None),
    ] {
        let (status, value) =
            request(&handle, method, &format!("/api/v1/settings/{path}"), body).await;
        // The declared Pine wire contract requires all three fields. Keep the
        // original Go {} counterexample distinct from the valid save failure.
        let pine_missing_fields = path == "pine-worker" && body == Some("{}");
        assert_eq!(
            status,
            if pine_missing_fields { 400 } else { 500 },
            "{method} {path}: {value}"
        );
        assert_eq!(
            value["error"]["code"],
            if pine_missing_fields {
                "BAD_REQUEST"
            } else {
                "SETTINGS_SAVE_FAILED"
            },
            "{path}: {value}"
        );
        assert_eq!(std::fs::read(&backup).expect("backup"), bytes);
        assert!(file.is_dir());
        for (index, read_path) in paths.iter().enumerate() {
            let (status, value) = request(
                &handle,
                "GET",
                &format!("/api/v1/settings/{read_path}"),
                None,
            )
            .await;
            assert_eq!(status, 200, "{read_path}: {value}");
            assert_eq!(
                value["data"], before[index],
                "runtime state after {path}: {read_path}"
            );
        }
    }
    std::fs::remove_dir(&file).expect("unblock file");
    std::fs::rename(&backup, &file).expect("restore file");
    handle.shutdown().await.expect("shutdown");
    assert_eq!(std::fs::read(file).expect("after"), bytes);
}

// Parity: go:452dea11:internal/api/settings/routes_accounts_validation_test.go:18 TestManagedAccountUpdateUsesPathIDAndSurfacesServerErrors
#[tokio::test]
async fn production_http_managed_account_path_id_wins_and_failed_update_rolls_back() {
    let directory = tempdir().expect("directory");
    let config = config(&directory, seed());
    let handle = start_product(config.clone()).await.expect("production");
    let (status, response) = request(
        &handle,
        "PUT",
        "/api/v1/settings/broker-accounts/record-1",
        Some(r#"{"id":"client-id","accountId":"acct-9","displayName":"Primary"}"#),
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["id"], "record-1");
    assert!(!response.to_string().contains("client-id"));
    assert_eq!(document(directory.path())["accounts"][0]["id"], "record-1");
    let file = directory.path().join("settings.json");
    let bytes = std::fs::read(&file).expect("before");
    let backup = directory.path().join("settings.saved.json");
    std::fs::rename(&file, &backup).expect("move file");
    std::fs::create_dir(&file).expect("block save");
    let (status, response) = request(
        &handle,
        "PUT",
        "/api/v1/settings/broker-accounts/record-1",
        Some(r#"{"accountId":"acct-9"}"#),
    )
    .await;
    assert_eq!(status, 500, "{response}");
    assert_eq!(response["error"]["code"], "SETTINGS_SAVE_FAILED");
    let (status, response) = request(&handle, "GET", "/api/v1/settings/brokers", None).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["accounts"][0]["displayName"], "Primary");
    assert_eq!(std::fs::read(&backup).expect("backup"), bytes);
    std::fs::remove_dir(&file).expect("unblock file");
    std::fs::rename(&backup, &file).expect("restore file");
    handle.shutdown().await.expect("shutdown");
    let handle = start_product(config).await.expect("restart");
    let (status, response) = request(&handle, "GET", "/api/v1/settings/brokers", None).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["accounts"][0]["id"], "record-1");
    assert_eq!(response["data"]["accounts"][0]["displayName"], "Primary");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:125 TestOnboardingCanBeResetWithoutLosingLastBroker
#[tokio::test]
async fn production_http_onboarding_reset_clears_timestamps_and_keeps_last_broker_after_restart() {
    let directory = tempdir().expect("directory");
    let config = config(&directory, seed());
    let handle = start_product(config.clone()).await.expect("production");
    let (status, response) = request(
        &handle,
        "PUT",
        "/api/v1/settings/onboarding",
        Some(r#"{"completed":false,"dismissed":false,"lastBrokerId":" "}"#),
    )
    .await;
    assert_eq!(status, 200, "{response}");
    for state in [
        &response["data"]["state"],
        &document(directory.path())["onboarding"],
    ] {
        assert_eq!(state["completed"], false);
        assert!(state.get("completedAt").is_none());
        assert!(state.get("dismissedAt").is_none());
        let decoded: jftrade_settings::OnboardingSettings =
            serde_json::from_value(state.clone()).expect("state");
        assert!(decoded.completed_at.is_empty());
        assert!(decoded.dismissed_at.is_empty());
        assert_eq!(state["lastBrokerId"], "futu");
    }
    handle.shutdown().await.expect("shutdown");
    let handle = start_product(config).await.expect("restart");
    let (status, response) = request(&handle, "GET", "/api/v1/settings/onboarding", None).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["state"]["completed"], false);
    assert_eq!(response["data"]["state"]["lastBrokerId"], "futu");
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/settings/routes_failure_boundaries_test.go:84 TestSystemNotificationRoutesReadAndSaveSettings
#[tokio::test]
async fn production_http_notification_settings_expose_normalization_and_persist_disabled_state() {
    let directory = tempdir().expect("directory");
    let config = config(
        &directory,
        json!({"systemNotifications":{"enabled":true,
        "mode":"important","levels":["error"],"categories":["trading"],"soundEnabled":true}}),
    );
    let handle = start_product(config.clone()).await.expect("production");
    let (status, response) = request(
        &handle,
        "GET",
        "/api/v1/settings/system-notifications",
        None,
    )
    .await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["mode"], "important");
    // Go's route stub keeps [trading]; the production domain expands important defaults.
    assert_eq!(
        response["data"]["categories"],
        json!([
            "broker.connection",
            "strategy.order.signal",
            "execution.order",
            "execution.fill"
        ])
    );
    let (status, response) = request(&handle, "PUT", "/api/v1/settings/system-notifications",
        Some(r#"{"enabled":false,"mode":"off","levels":["warn"],"categories":["system"],"soundEnabled":false}"#)).await;
    assert_eq!(status, 200, "{response}");
    assert_eq!(response["data"]["enabled"], false);
    assert_eq!(response["data"]["soundEnabled"], false);
    assert_eq!(response["data"]["mode"], "important");
    assert_eq!(
        document(directory.path())["systemNotifications"],
        response["data"]
    );
    let (_, read) = request(
        &handle,
        "GET",
        "/api/v1/settings/system-notifications",
        None,
    )
    .await;
    assert_eq!(read["data"], response["data"]);
    handle.shutdown().await.expect("shutdown");
    let handle = start_product(config).await.expect("restart");
    let (status, read) = request(
        &handle,
        "GET",
        "/api/v1/settings/system-notifications",
        None,
    )
    .await;
    assert_eq!(status, 200, "{read}");
    assert_eq!(read["data"], response["data"]);
    handle.shutdown().await.expect("shutdown");
}

#[derive(Debug, Default)]
struct Notifications(Mutex<Vec<ProductNotificationRequest>>);

impl ProductNotificationPort for Notifications {
    fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery {
        self.0.lock().expect("calls").push(request);
        ProductNotificationDelivery {
            delivered: true,
            status: "delivered".into(),
            message: "sent".into(),
        }
    }
}

// Parity: go:452dea11:internal/api/settings/routes_test.go:935 TestSystemNotificationTestRouteUsesSettingsServicePort
#[tokio::test]
async fn production_http_notification_test_forwards_host_request_and_missing_host_fails_closed() {
    let directory = tempdir().expect("directory");
    let config = config(&directory, json!({}));
    let owner = Arc::new(Notifications::default());
    let handle = start_product(config.clone().with_notification_port(owner.clone()))
        .await
        .expect("production");
    for sequence in 1..=3 {
        let (status, response) = request(
            &handle,
            "POST",
            "/api/v1/settings/system-notifications/test",
            None,
        )
        .await;
        assert_eq!(status, 200, "{response}");
        assert_eq!(
            response["data"]["event"]["id"],
            format!("system-notification-{sequence}")
        );
        assert_eq!(response["data"]["delivery"]["status"], "delivered");
    }
    let calls = owner.0.lock().expect("calls").clone();
    assert_eq!(calls.len(), 3);
    for call in calls {
        assert_eq!(call.title, "JFTrade 系统通知测试");
        assert_eq!(call.body, "系统通知通道已连接。");
        assert!(call.sound_enabled);
    }
    handle.shutdown().await.expect("shutdown");
    let handle = start_product(config).await.expect("without host");
    let (status, response) = request(
        &handle,
        "POST",
        "/api/v1/settings/system-notifications/test",
        None,
    )
    .await;
    assert_eq!(status, 503, "{response}");
    assert_eq!(response["error"]["code"], "SYSTEM_NOTIFICATION_UNAVAILABLE");
    assert_eq!(owner.0.lock().expect("calls").len(), 3);
    handle.shutdown().await.expect("shutdown");
}
