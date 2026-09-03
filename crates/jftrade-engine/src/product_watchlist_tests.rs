use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct WatchlistReadFixture {
    version: String,
    cases: Vec<WatchlistReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct WatchlistReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureWatchlistReadPort {
    responses: BTreeMap<String, FixtureWatchlistReadResponse>,
}

#[derive(Clone, Debug)]
enum FixtureWatchlistReadResponse {
    Data(Value),
    Invalid(String),
    NotFound,
    Unavailable(String),
}

impl FixtureWatchlistReadPort {
    fn from_fixture(fixture: &WatchlistReadFixture) -> Self {
        Self {
            responses: fixture
                .cases
                .iter()
                .map(|case| {
                    let response = if let Some(data) = &case.data {
                        FixtureWatchlistReadResponse::Data(data.clone())
                    } else {
                        match case.error_code.as_deref() {
                            Some("BAD_REQUEST") => FixtureWatchlistReadResponse::Invalid(
                                case.error_message.clone().expect("invalid error message"),
                            ),
                            Some("WATCHLIST_NOT_FOUND") => FixtureWatchlistReadResponse::NotFound,
                            Some("WATCHLIST_UNAVAILABLE") => {
                                FixtureWatchlistReadResponse::Unavailable(
                                    case.error_message
                                        .clone()
                                        .expect("unavailable error message"),
                                )
                            }
                            other => {
                                panic!("unsupported fixture error for {}: {other:?}", case.name)
                            }
                        }
                    };
                    (case.request_path.clone(), response)
                })
                .collect(),
        }
    }
}

impl WatchlistReadSnapshotPort for FixtureWatchlistReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, WatchlistReadSnapshotError> {
        match self.responses.get(&watchlist_read_request_key(path, query)) {
            Some(FixtureWatchlistReadResponse::Data(data)) => Ok(data.clone()),
            Some(FixtureWatchlistReadResponse::Invalid(message)) => {
                Err(WatchlistReadSnapshotError::Invalid(message.clone()))
            }
            Some(FixtureWatchlistReadResponse::NotFound) | None => {
                Err(WatchlistReadSnapshotError::NotFound)
            }
            Some(FixtureWatchlistReadResponse::Unavailable(message)) => {
                Err(WatchlistReadSnapshotError::Unavailable(message.clone()))
            }
        }
    }
}

fn watchlist_read_request_key(path: &str, query: &str) -> String {
    if query.is_empty() {
        path.to_owned()
    } else {
        format!("{path}?{query}")
    }
}

#[derive(Debug)]
struct FailingWatchlistReadPort;

impl WatchlistReadSnapshotPort for FailingWatchlistReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, WatchlistReadSnapshotError> {
        Err(WatchlistReadSnapshotError::Unavailable(
            "Go watchlist read fixture unavailable".to_owned(),
        ))
    }
}

fn watchlist_read_fixture() -> WatchlistReadFixture {
    let fixture: WatchlistReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/watchlist-read.json"
    ))
    .expect("watchlist read fixture");
    assert_eq!(fixture.version, "stage9.watchlist-read.v1");
    fixture
}

#[tokio::test]
async fn watchlist_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = watchlist_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_read_snapshot_port(Arc::new(FixtureWatchlistReadPort::from_fixture(
                &fixture,
            )));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 54);
    let address = handle.startup_record().address;
    for case in &fixture.cases {
        assert_eq!(case.method, "GET", "case {}", case.name);
        let (status, response) =
            request_json_with_status(address, &case.method, &case.request_path, None, &[]).await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        if let Some(expected) = &case.data {
            assert_eq!(response["ok"], true, "case {}", case.name);
            assert_eq!(response["data"], *expected, "case {}", case.name);
        } else {
            assert_eq!(response["ok"], false, "case {}", case.name);
            assert_eq!(
                response["error"]["code"].as_str(),
                case.error_code.as_deref(),
                "case {}",
                case.name
            );
            assert_eq!(
                response["error"]["message"].as_str(),
                case.error_message.as_deref(),
                "case {}",
                case.name
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn watchlist_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_watchlist_read_snapshot_port(Arc::new(FailingWatchlistReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/watchlist/groups",
        "/api/v1/watchlist/items",
        "/api/v1/watchlist/sources",
        "/api/v1/watchlist/sources/futu:default/groups",
        "/api/v1/watchlist/bindings",
        "/api/v1/watchlist/import-runs",
    ] {
        let response = request_json(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "WATCHLIST_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn watchlist_read_routes_are_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/watchlist/groups",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
