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
