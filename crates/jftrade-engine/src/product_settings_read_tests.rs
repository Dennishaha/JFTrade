use std::fs;

use tempfile::tempdir;

use super::*;

const SETTINGS_READ_PATHS: &[&str] = &[
    "/api/v1/settings/adk",
    "/api/v1/settings/adk/mcp",
    "/api/v1/settings/backtest-market-data-provider",
    "/api/v1/settings/brokers",
    "/api/v1/settings/exchange-calendars",
    "/api/v1/settings/execution",
    "/api/v1/settings/market-data-provider",
    "/api/v1/settings/onboarding",
    "/api/v1/settings/pine-worker",
    "/api/v1/settings/security",
    "/api/v1/settings/system-notifications",
];

#[tokio::test]
async fn settings_read_routes_replay_go_compatible_defaults_without_file_mutation() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let before = fs::read(&settings_path).expect("read seeded settings");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("test-cutover config");
    let handle = start_product(config).await.expect("start product");

    for path in SETTINGS_READ_PATHS {
        let (status, response) =
            request_json_with_status(handle.startup_record().address, "GET", path, None, &[]).await;
        assert_eq!(status, 200, "status for {path}: {response}");
        assert_eq!(response["ok"], true, "envelope for {path}");
        assert!(
            response["data"].is_object(),
            "data projection for {path}: {response}"
        );
    }

    let security = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/security",
        None,
    )
    .await;
    assert_eq!(security["data"]["webPort"], 6688);
    assert_eq!(security["data"]["passwordConfigured"], false);
    let execution = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/execution",
        None,
    )
    .await;
    assert_eq!(execution["data"]["defaultTradingEnvironment"], "SIMULATE");
    assert_eq!(execution["data"]["brokerOrderHistoryLookbackDays"], 30);
    let provider = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/market-data-provider",
        None,
    )
    .await;
    assert_eq!(provider["data"]["activeProvider"], "akshare");
    let calendars = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/exchange-calendars",
        None,
    )
    .await;
    assert_eq!(
        calendars["data"]["exchangeCalendars"]["refreshIntervalHours"],
        24
    );
    assert_eq!(
        calendars["data"]["exchangeCalendars"]["warmupMarkets"],
        json!(["US", "HK", "CN"])
    );
    let brokers = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/brokers",
        None,
    )
    .await;
    assert_eq!(brokers["data"]["brokers"][0]["descriptor"]["id"], "futu");
    assert_eq!(brokers["data"]["accounts"], json!([]));

    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        fs::read(&settings_path).expect("read settings after replay"),
        before
    );
}

#[tokio::test]
async fn settings_read_routes_require_the_authenticated_shadow_token() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let token = "settings-read-auth-token-012345678901234567890";
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        token,
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/execution",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 401);
    assert_eq!(response["ok"], false);
    handle.shutdown().await.expect("shutdown shadow");
}

#[derive(Debug)]
struct RecordingNotificationPort;

impl ProductNotificationPort for RecordingNotificationPort {
    fn deliver(&self, request: ProductNotificationRequest) -> ProductNotificationDelivery {
        ProductNotificationDelivery {
            delivered: true,
            status: "delivered".to_owned(),
            message: format!("desktop notification sent: {}", request.title),
        }
    }
}

// Parity: go:452dea11:internal/live/notification_delivery_test.go:5 TestNotificationDeliveryKeepsHostNotificationOutcomeExplicit
#[tokio::test]
async fn system_notification_delivery_keeps_the_host_outcome_explicit() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_notification_port(std::sync::Arc::new(RecordingNotificationPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, delivered) = request_json_with_status(
        address,
        "POST",
        "/api/v1/settings/system-notifications/test",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "delivered notification: {delivered}");
    assert_eq!(delivered["data"]["delivery"]["delivered"], true);
    assert_eq!(delivered["data"]["delivery"]["status"], "delivered");
    assert!(
        delivered["data"]["delivery"]["message"]
            .as_str()
            .is_some_and(|message| !message.is_empty()),
        "delivered notification must report a host message: {delivered}"
    );

    let (status, saved) = request_json_with_status(
        address,
        "PUT",
        "/api/v1/settings/system-notifications",
        Some(r#"{"enabled":true,"mode":"custom","levels":["error"],"categories":["other"]}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200, "notification settings: {saved}");

    let (status, filtered) = request_json_with_status(
        address,
        "POST",
        "/api/v1/settings/system-notifications/test",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "filtered notification: {filtered}");
    assert_eq!(filtered["data"]["delivery"]["delivered"], false);
    assert_eq!(filtered["data"]["delivery"]["status"], "filtered");
    assert_eq!(
        filtered["data"]["delivery"]["message"],
        "notification filtered by desktop settings"
    );
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:66 TestApplyProviderSettingsAllowsUnavailableWatchlist
/// Applying provider settings must not depend on the watchlist data plane: the
/// switch succeeds while the watchlist routes are uncomposed, exactly like the
/// reference tolerating an unavailable watchlist service.
#[tokio::test]
async fn provider_switch_succeeds_while_watchlist_ports_are_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, b"{}\n").expect("seed settings");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("test-cutover config");
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, watchlist) =
        request_json_with_status(address, "GET", "/api/v1/watchlist/groups", None, &[]).await;
    assert_ne!(
        status, 200,
        "watchlist data plane must be unavailable in this composition: {watchlist}"
    );

    let (status, saved) = request_json_with_status(
        address,
        "PUT",
        "/api/v1/settings/market-data-provider",
        Some(r#"{"activeProvider":"akshare"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200, "provider switch without watchlist: {saved}");
    assert_eq!(saved["data"]["activeProvider"], "akshare");

    handle.shutdown().await.expect("shutdown product");
}
