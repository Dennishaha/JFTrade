use std::collections::BTreeMap;
use std::net::SocketAddr;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataNewsActionsReadFixture {
    version: String,
    cases: Vec<MarketDataNewsActionsReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataNewsActionsReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    #[serde(default)]
    headers: BTreeMap<String, String>,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureMarketDataNewsActionsReadPort {
    responses: BTreeMap<String, Result<Value, MarketDataNewsActionsReadSnapshotError>>,
}

impl FixtureMarketDataNewsActionsReadPort {
    fn from_fixture(fixture: &MarketDataNewsActionsReadFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .map(|case| {
                let response = match &case.data {
                    Some(data) => Ok(data.clone()),
                    None => Err(MarketDataNewsActionsReadSnapshotError::Failed {
                        status: case.expected_status,
                        code: case.error_code.clone().unwrap_or_default(),
                        message: case.error_message.clone().unwrap_or_default(),
                        retry_after_seconds: case
                            .headers
                            .get("Retry-After")
                            .and_then(|value| value.parse().ok()),
                    }),
                };
                (case.request_path.clone(), response)
            })
            .collect();
        Self { responses }
    }
}

impl MarketDataNewsActionsReadSnapshotPort for FixtureMarketDataNewsActionsReadPort {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        })
    }
}

#[derive(Debug)]
struct FailingMarketDataNewsActionsReadPort;

impl MarketDataNewsActionsReadSnapshotPort for FailingMarketDataNewsActionsReadPort {
    fn read(
        &self,
        _path: &str,
        _query: &str,
    ) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
        Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
            "Go market-data news/actions owner unavailable".to_owned(),
        ))
    }
}

fn market_data_news_actions_read_fixture() -> MarketDataNewsActionsReadFixture {
    let fixture: MarketDataNewsActionsReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/market-data-news-actions-read.json"
    ))
    .expect("market-data news/actions fixture");
    assert_eq!(fixture.version, "stage9.market-data-news-actions-read.v1");
    fixture
}

#[tokio::test]
async fn market_data_news_actions_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = market_data_news_actions_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_news_actions_read_snapshot_port(Arc::new(
                FixtureMarketDataNewsActionsReadPort::from_fixture(&fixture),
            ));
    let handle = start_product(config).await.expect("start product");
    assert!(
        handle
            .startup_record()
            .capabilities
            .contains(&"GET /api/v1/market-data/news/{market}/{symbol}".to_owned())
    );
    assert!(
        handle
            .startup_record()
            .capabilities
            .contains(&"GET /api/v1/market-data/corporate-actions/{market}/{symbol}".to_owned())
    );
    for case in &fixture.cases {
        let (status, headers, response) = request_news_actions_json_response(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
        )
        .await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        assert_eq!(
            headers.get("retry-after"),
            case.headers.get("Retry-After"),
            "case {} retry header",
            case.name
        );
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
async fn market_data_news_actions_read_routes_fail_closed_when_snapshot_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_news_actions_read_snapshot_port(Arc::new(
                FailingMarketDataNewsActionsReadPort,
            ));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/market-data/news/US/AAPL",
        "/api/v1/market-data/corporate-actions/US/AAPL",
    ] {
        let (status, _headers, response) =
            request_news_actions_json_response(handle.startup_record().address, "GET", path).await;
        assert_eq!(status, 503, "path {path}");
        assert_eq!(
            response["error"]["code"], "MARKET_DATA_NEWS_ACTIONS_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn market_data_news_actions_read_routes_are_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/market-data/news/US/AAPL",
        "/api/v1/market-data/corporate-actions/US/AAPL",
    ] {
        let (status, _headers, response) =
            request_news_actions_json_response(handle.startup_record().address, "GET", path).await;
        assert_eq!(status, 404, "path {path}");
        assert_eq!(response["error"]["code"], "NOT_FOUND", "path {path}");
    }
    handle.shutdown().await.expect("shutdown product");
}

async fn request_news_actions_json_response(
    address: SocketAddr,
    method: &str,
    path: &str,
) -> (u16, BTreeMap<String, String>, Value) {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nX-Request-ID: fixture-market-data-news-actions\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut raw_response = Vec::new();
    stream
        .read_to_end(&mut raw_response)
        .await
        .expect("read response");
    let response = String::from_utf8(raw_response).expect("UTF-8 response");
    let (head, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let mut lines = head.lines();
    let status = lines
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    let headers = lines
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.trim().to_ascii_lowercase(), value.trim().to_owned()))
        .collect();
    let value = serde_json::from_str(body).expect("JSON response");
    (status, headers, value)
}
