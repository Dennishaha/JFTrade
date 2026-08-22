use std::collections::BTreeMap;
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataQuoteReadFixture {
    version: String,
    cases: Vec<MarketDataQuoteReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataQuoteReadCase {
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
struct FixtureMarketDataQuoteReadPort {
    responses: Mutex<BTreeMap<String, Vec<Result<Value, MarketDataQuoteReadSnapshotError>>>>,
}

impl FixtureMarketDataQuoteReadPort {
    fn from_fixture(fixture: &MarketDataQuoteReadFixture) -> Self {
        let mut responses = BTreeMap::new();
        for case in &fixture.cases {
            let response = match &case.data {
                Some(data) => Ok(data.clone()),
                None => Err(MarketDataQuoteReadSnapshotError::Failed {
                    status: case.expected_status,
                    code: case.error_code.clone().unwrap_or_default(),
                    message: case.error_message.clone().unwrap_or_default(),
                    retry_after_seconds: case
                        .headers
                        .get("Retry-After")
                        .and_then(|value| value.parse().ok()),
                }),
            };
            responses
                .entry(case.request_path.clone())
                .or_insert_with(Vec::new)
                .push(response);
        }
        Self {
            responses: Mutex::new(responses),
        }
    }
}

impl MarketDataQuoteReadSnapshotPort for FixtureMarketDataQuoteReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        let mut responses = self.responses.lock().expect("quote fixture response lock");
        let Some(values) = responses.get_mut(&key) else {
            return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ));
        };
        if values.is_empty() {
            return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                "fixture response exhausted".to_owned(),
            ));
        }
        values.remove(0)
    }
}

#[derive(Debug)]
struct FailingMarketDataQuoteReadPort;

impl MarketDataQuoteReadSnapshotPort for FailingMarketDataQuoteReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataQuoteReadSnapshotError> {
        Err(MarketDataQuoteReadSnapshotError::Unavailable(
            "Go market-data quote-read owner unavailable".to_owned(),
        ))
    }
}

fn market_data_quote_read_fixture() -> MarketDataQuoteReadFixture {
    let fixture: MarketDataQuoteReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/market-data-quote-read.json"
    ))
    .expect("market-data quote-read fixture");
    assert_eq!(fixture.version, "stage9.market-data-quote-read.v1");
    fixture
}

#[tokio::test]
async fn market_data_quote_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = market_data_quote_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_quote_read_snapshot_port(Arc::new(
                FixtureMarketDataQuoteReadPort::from_fixture(&fixture),
            ));
    let handle = start_product(config).await.expect("start product");
    for case in &fixture.cases {
        let (status, headers, response) = request_market_data_quote_read_json_response(
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
async fn market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_quote_read_snapshot_port(Arc::new(FailingMarketDataQuoteReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/market-data/broker-queue/US.AAPL",
        "/api/v1/market-data/candles/US/AAPL",
        "/api/v1/market-data/capital-flow/US.AAPL",
        "/api/v1/market-data/depth/US/AAPL",
        "/api/v1/market-data/instruments/US.AAPL/profile",
        "/api/v1/market-data/intraday/US.AAPL",
        "/api/v1/market-data/securities/US/AAPL",
        "/api/v1/market-data/snapshots/US/AAPL",
        "/api/v1/market-data/subscriptions",
        "/api/v1/market-data/ticks/US.AAPL",
    ] {
        let (status, _headers, response) = request_market_data_quote_read_json_response(
            handle.startup_record().address,
            "GET",
            path,
        )
        .await;
        assert_eq!(status, 503, "path {path}");
        assert_eq!(
            response["error"]["code"], "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn market_data_quote_read_routes_are_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let (status, _headers, response) = request_market_data_quote_read_json_response(
        handle.startup_record().address,
        "GET",
        "/api/v1/market-data/snapshots/US/AAPL",
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}

async fn request_market_data_quote_read_json_response(
    address: SocketAddr,
    method: &str,
    path: &str,
) -> (u16, BTreeMap<String, String>, Value) {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nX-Request-ID: fixture-market-data-quote-read\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
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
