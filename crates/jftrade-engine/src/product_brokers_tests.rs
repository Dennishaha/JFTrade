use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BrokerReadFixture {
    version: String,
    cases: Vec<BrokerReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BrokerReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureBrokerReadPort {
    responses: BTreeMap<String, Value>,
}

impl FixtureBrokerReadPort {
    fn from_fixture(fixture: &BrokerReadFixture) -> Self {
        Self {
            responses: fixture
                .cases
                .iter()
                .filter_map(|case| {
                    case.data
                        .clone()
                        .map(|data| (case.request_path.clone(), data))
                })
                .collect(),
        }
    }
}

impl BrokerReadSnapshotPort for FixtureBrokerReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, BrokerReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().ok_or_else(|| {
            BrokerReadSnapshotError::Unavailable("fixture response missing".to_owned())
        })
    }
}

#[derive(Debug)]
struct FailingBrokerReadPort;

impl BrokerReadSnapshotPort for FailingBrokerReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, BrokerReadSnapshotError> {
        Err(BrokerReadSnapshotError::Unavailable(
            "Go broker read snapshot unavailable".to_owned(),
        ))
    }
}

fn broker_read_fixture() -> BrokerReadFixture {
    let fixture: BrokerReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/broker-read.json"
    ))
    .expect("broker read fixture");
    assert_eq!(fixture.version, "stage9.broker-read.v1");
    fixture
}

#[tokio::test]
async fn broker_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = broker_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_broker_read_snapshot_port(Arc::new(FixtureBrokerReadPort::from_fixture(
                &fixture,
            )));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 61);
    for case in &fixture.cases {
        assert_eq!(case.method, "GET", "case {}", case.name);
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
            None,
            &[],
        )
        .await;
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
async fn broker_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_broker_read_snapshot_port(Arc::new(FailingBrokerReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/brokers/capabilities",
        "/api/v1/brokers/fixture/funds",
        "/api/v1/brokers/fixture/runtime",
    ] {
        let response = request_json(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "BROKER_READ_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn broker_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/brokers/fixture/funds",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
