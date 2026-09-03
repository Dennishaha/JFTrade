use std::fs;

use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SettingsUiReadFixture {
    version: String,
    cases: Vec<SettingsUiReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SettingsUiReadCase {
    name: String,
    method: String,
    request_path: String,
    request_id: String,
    seed_document: Value,
    expected_status: u16,
    response: Value,
}

fn settings_ui_read_fixture() -> SettingsUiReadFixture {
    let fixture: SettingsUiReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/settings-ui-read.json"
    ))
    .expect("settings UI read fixture");
    assert_eq!(fixture.version, "stage9.settings-ui-read.v1");
    assert!(fixture.cases.len() >= 4);
    fixture
}

#[tokio::test]
async fn appearance_read_route_matches_go_fixture_for_all_seed_documents() {
    let fixture = settings_ui_read_fixture();
    for case in fixture.cases {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let seed = serde_json::to_vec(&case.seed_document).expect("encode settings seed");
        fs::write(&settings_path, &seed).expect("write settings seed");

        let token = "appearance-read-shadow-token-012345678901234567890";
        let config = ProductConfig::desktop_shadow(
            "127.0.0.1:0".parse().expect("address"),
            &settings_path,
            token,
        )
        .expect("shadow config");
        let handle = start_product(config)
            .await
            .unwrap_or_else(|error| panic!("start shadow for {}: {error}", case.name));
        let authorization = format!("Bearer {token}");
        let (status, mut response) = request_json_with_status(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
            None,
            &[
                ("Authorization", authorization.as_str()),
                ("X-Request-ID", case.request_id.as_str()),
            ],
        )
        .await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        response["timestamp"] = Value::String("fixture-time".to_owned());
        assert_eq!(response, case.response, "case {}", case.name);
        handle.shutdown().await.expect("shutdown shadow");

        assert_eq!(fs::read(&settings_path).expect("read settings"), seed);
    }
}

#[tokio::test]
async fn appearance_read_route_requires_the_authenticated_shadow_token() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    fs::write(&settings_path, br##"{"activeMarketDataProvider":"yfinance","exchangeCalendars":{"autoRefreshEnabled":false}}"##)
        .expect("write settings");
    let config = ProductConfig::desktop_shadow(
        "127.0.0.1:0".parse().expect("address"),
        &settings_path,
        "appearance-read-auth-token-012345678901234567890",
    )
    .expect("shadow config");
    let handle = start_product(config).await.expect("start shadow");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/settings/ui",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 401);
    assert_eq!(response["ok"], false);
    handle.shutdown().await.expect("shutdown shadow");
}
