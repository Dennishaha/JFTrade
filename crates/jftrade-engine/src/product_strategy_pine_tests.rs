use std::collections::{BTreeMap, HashMap};
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use serde::Deserialize;
use serde_json::{Value, json};
use std::sync::atomic::{AtomicUsize, Ordering};
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use super::strategy_pine::{
    StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
};
use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyPineFixture {
    version: String,
    cases: Vec<StrategyPineFixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyPineFixtureCase {
    name: String,
    method: String,
    body: String,
    expected_status: u16,
    #[serde(default)]
    headers: BTreeMap<String, String>,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug, Default)]
struct FixtureStrategyPinePort {
    projections: HashMap<String, Value>,
    calls: Mutex<Vec<StrategyPineAnalyzeInput>>,
}

impl FixtureStrategyPinePort {
    fn from_fixture(fixture: &StrategyPineFixture) -> Self {
        let projections = fixture
            .cases
            .iter()
            .filter_map(|case| {
                let data = case.data.clone()?;
                let body: Value = serde_json::from_str(&case.body).ok()?;
                let script = body
                    .get("script")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned();
                Some((script, data))
            })
            .collect();
        Self {
            projections,
            calls: Mutex::new(Vec::new()),
        }
    }
}

impl StrategyPineAnalyzeSnapshotPort for FixtureStrategyPinePort {
    fn analyze(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        self.calls
            .lock()
            .expect("strategy-pine product call lock")
            .push(input.clone());
        self.projections.get(&input.script).cloned().ok_or_else(|| {
            StrategyPineAnalyzeSnapshotError::Unavailable("fixture projection missing".to_owned())
        })
    }
}

#[derive(Debug)]
struct FailingStrategyPinePort;

impl StrategyPineAnalyzeSnapshotPort for FailingStrategyPinePort {
    fn analyze(
        &self,
        _input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        Err(StrategyPineAnalyzeSnapshotError::Unavailable(
            "Go strategy-pine owner unavailable".to_owned(),
        ))
    }
}

#[derive(Debug)]
struct RetryStrategyPinePort;

impl StrategyPineAnalyzeSnapshotPort for RetryStrategyPinePort {
    fn analyze(
        &self,
        _input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        Err(StrategyPineAnalyzeSnapshotError::Failed {
            status: 429,
            code: "STRATEGY_PINE_BUSY".to_owned(),
            message: "strategy-pine owner is busy".to_owned(),
            retry_after_seconds: Some(7),
        })
    }
}

#[derive(Debug)]
struct SequencedStrategyPinePort {
    attempts: AtomicUsize,
    projection: Value,
}

impl StrategyPineAnalyzeSnapshotPort for SequencedStrategyPinePort {
    fn analyze(
        &self,
        _input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        if self.attempts.fetch_add(1, Ordering::SeqCst) == 0 {
            return Err(StrategyPineAnalyzeSnapshotError::Failed {
                status: 504,
                code: "STRATEGY_PINE_ANALYZE_TIMEOUT".to_owned(),
                message: "analysis snapshot timed out".to_owned(),
                retry_after_seconds: None,
            });
        }
        Ok(self.projection.clone())
    }
}

#[derive(Debug)]
struct ProductHttpResponse {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Value,
}

fn strategy_pine_fixture() -> StrategyPineFixture {
    let fixture: StrategyPineFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategy-pine.json"
    ))
    .expect("strategy-pine product fixture");
    assert_eq!(fixture.version, "stage9.strategy-pine.v1");
    fixture
}

#[tokio::test]
async fn strategy_pine_routes_match_group_fixture_in_cutover_only() {
    let fixture = strategy_pine_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let port = Arc::new(FixtureStrategyPinePort::from_fixture(&fixture));
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_pine_analyze_snapshot_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/strategy-pine/analyze" })
    );

    for case in &fixture.cases {
        let response = request_strategy_pine(
            handle.startup_record().address,
            &case.method,
            STRATEGY_PINE_ANALYZE_PATH,
            &case.body,
        )
        .await;
        assert_eq!(response.status, case.expected_status, "case {}", case.name);
        for (name, expected) in &case.headers {
            assert_eq!(
                header_value(&response.headers, name),
                Some(expected.as_str()),
                "case {} header {name}",
                case.name
            );
        }
        if let Some(expected) = &case.data {
            assert_eq!(response.body["ok"], true, "case {}", case.name);
            assert_eq!(response.body["data"], *expected, "case {}", case.name);
        } else {
            assert_eq!(response.body["ok"], false, "case {}", case.name);
            assert_eq!(
                response.body["error"]["code"].as_str(),
                case.error_code.as_deref(),
                "case {}",
                case.name
            );
            assert_eq!(
                response.body["error"]["message"].as_str(),
                case.error_message.as_deref(),
                "case {}",
                case.name
            );
        }
    }

    {
        let calls = port.calls.lock().expect("strategy-pine product call lock");
        assert!(calls.iter().any(|input| input.include_ast));
        assert!(calls.iter().any(|input| input.script.is_empty()));
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn strategy_pine_routes_preserve_snapshot_failures_and_retry_after() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_pine_analyze_snapshot_port(Arc::new(FailingStrategyPinePort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let unavailable = request_strategy_pine(
        address,
        "POST",
        STRATEGY_PINE_ANALYZE_PATH,
        r#"{"script":"//@version=6\nindicator(\"unavailable\")"}"#,
    )
    .await;
    assert_eq!(unavailable.status, 503);
    assert_eq!(unavailable.body["ok"], false);
    assert_eq!(
        unavailable.body["error"]["code"],
        "STRATEGY_PINE_ANALYZE_UNAVAILABLE"
    );
    assert_eq!(
        unavailable.body["error"]["message"],
        "Go strategy-pine owner unavailable"
    );

    handle.shutdown().await.expect("shutdown product");

    let retry_config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_pine_analyze_snapshot_port(Arc::new(RetryStrategyPinePort));
    let handle = start_product(retry_config).await.expect("start product");
    let retry = request_strategy_pine(
        handle.startup_record().address,
        "POST",
        STRATEGY_PINE_ANALYZE_PATH,
        r#"{"script":"//@version=6\nindicator(\"busy\")"}"#,
    )
    .await;
    assert_eq!(retry.status, 429);
    assert_eq!(retry.body["error"]["code"], "STRATEGY_PINE_BUSY");
    assert_eq!(header_value(&retry.headers, "Retry-After"), Some("7"));
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn strategy_pine_product_recovers_after_timeout_and_restart_without_local_state_side_effects()
{
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"strategy-pine\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read seeded settings");
    let projection = json!({
        "analysis": "recovered",
        "externalEngine": {"status": "shadow_error"}
    });
    let port = Arc::new(SequencedStrategyPinePort {
        attempts: AtomicUsize::new(0),
        projection: projection.clone(),
    });
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_pine_analyze_snapshot_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let path = "/api/v1/strategy-pine/analyze";
    let body = Some(r#"{"script":"//@version=6\nstrategy(\"fixture\")"}"#);

    let (status, response) =
        request_json_with_status(handle.startup_record().address, "POST", path, body, &[]).await;
    assert_eq!(status, 504);
    assert_eq!(response["error"]["code"], "STRATEGY_PINE_ANALYZE_TIMEOUT");

    let (status, response) =
        request_json_with_status(handle.startup_record().address, "POST", path, body, &[]).await;
    assert_eq!(status, 200);
    assert_eq!(response["data"], projection);
    assert_eq!(port.attempts.load(Ordering::SeqCst), 2);
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after recovery"),
        settings_before
    );

    let restarted_port = Arc::new(SequencedStrategyPinePort {
        attempts: AtomicUsize::new(1),
        projection: projection.clone(),
    });
    let restarted = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_strategy_pine_analyze_snapshot_port(restarted_port);
    let handle = start_product(restarted).await.expect("restart product");
    let (status, response) =
        request_json_with_status(handle.startup_record().address, "POST", path, body, &[]).await;
    assert_eq!(status, 200);
    assert_eq!(response["data"], projection);
    handle.shutdown().await.expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read restarted settings"),
        settings_before
    );
}

#[tokio::test]
async fn strategy_pine_route_is_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    assert!(
        !handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/strategy-pine/analyze" })
    );
    let response = request_strategy_pine(
        handle.startup_record().address,
        "POST",
        STRATEGY_PINE_ANALYZE_PATH,
        "null",
    )
    .await;
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}

async fn request_strategy_pine(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: &str,
) -> ProductHttpResponse {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect strategy-pine product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write strategy-pine product request");
    let mut raw = Vec::new();
    stream
        .read_to_end(&mut raw)
        .await
        .expect("read strategy-pine product response");
    let response = String::from_utf8(raw).expect("strategy-pine product response UTF-8");
    let (header_text, body_text) = response
        .split_once("\r\n\r\n")
        .expect("strategy-pine HTTP body");
    let status = header_text
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("strategy-pine HTTP status");
    let headers = header_text
        .lines()
        .skip(1)
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.trim().to_ascii_lowercase(), value.trim().to_owned()))
        .collect();
    ProductHttpResponse {
        status,
        headers,
        body: serde_json::from_str(body_text).expect("strategy-pine JSON response"),
    }
}

fn header_value<'a>(headers: &'a BTreeMap<String, String>, name: &str) -> Option<&'a str> {
    headers.get(&name.to_ascii_lowercase()).map(String::as_str)
}
