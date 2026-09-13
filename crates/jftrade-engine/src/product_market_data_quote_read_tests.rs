use std::collections::BTreeMap;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

use serde::Deserialize;
use serde_json::{Value, json};
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use jftrade_integration_futu::{
    MarketMicrostructureError, MarketMicrostructureOperation, MarketMicrostructureReadPort,
};
use jftrade_marketdata::{InstrumentRef, ProviderRouter};

use crate::product::product_production_ports::ProductionMarketDataQuotePort;

use super::*;

#[path = "product_market_data_candle_pagination_tests.rs"]
mod candle_pagination_tests;

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
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        let mut responses = self.responses.lock().expect("quote fixture response lock");
        let Some(values) = responses.get_mut(&key) else {
            return Box::pin(std::future::ready(Err(
                MarketDataQuoteReadSnapshotError::Unavailable(
                    "fixture response missing".to_owned(),
                ),
            )));
        };
        if values.is_empty() {
            return Box::pin(std::future::ready(Err(
                MarketDataQuoteReadSnapshotError::Unavailable(
                    "fixture response exhausted".to_owned(),
                ),
            )));
        }
        let res = values.remove(0);
        Box::pin(std::future::ready(res))
    }
}

#[derive(Debug)]
struct FailingMarketDataQuoteReadPort;

impl MarketDataQuoteReadSnapshotPort for FailingMarketDataQuoteReadPort {
    fn read<'a>(&'a self, _path: &'a str, _query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        Box::pin(std::future::ready(Err(
            MarketDataQuoteReadSnapshotError::Unavailable(
                "Go market-data quote-read owner unavailable".to_owned(),
            ),
        )))
    }
}

fn market_data_quote_read_fixture() -> MarketDataQuoteReadFixture {
    let fixture: MarketDataQuoteReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/market-data-quote-read.json"
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

#[tokio::test]
async fn futu_snapshot_route_projects_cached_extended_quote_contract() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let router = ProviderRouter::new(4);
    let observed_at_ms = 1_750_000_000_000;
    let tick = jftrade_marketdata::Tick {
        instrument_id: "US.AAPL".to_owned(),
        price: "114.97".parse().expect("tick price"),
        volume: "100".parse().expect("tick volume"),
        snapshot: Some(jftrade_marketdata::TradeQuoteSnapshot {
            symbol: Some("US.AAPL".to_owned()),
            last_price: Some("114.97".parse().expect("regular price")),
            previous_close: Some("112.50".parse().expect("previous close")),
            last_close: Some("111.25".parse().expect("last close")),
            session: Some("after".to_owned()),
            after_market: Some(jftrade_marketdata::ExtendedQuoteSnapshot {
                price: Some("118.40".parse().expect("after price")),
                high_price: Some("121.20".parse().expect("after high")),
                low_price: Some("115.50".parse().expect("after low")),
                volume: Some("0".parse().expect("after volume")),
                turnover: Some("67722995.69".parse().expect("after turnover")),
                trading_date: Some("2026-07-18".to_owned()),
                exchange_timezone: Some("America/New_York".to_owned()),
                session_start_at: Some("2026-07-18T16:00:00Z".to_owned()),
                session_end_at: Some("2026-07-19T00:00:00Z".to_owned()),
                ..Default::default()
            }),
            ..Default::default()
        }),
        observed_at_ms,
        provider_generation: 1,
    };
    router.cache_mut().insert(tick, 1).expect("cache tick");
    let port =
        ProductionMarketDataQuotePort::new(state, Some(Arc::new(Mutex::new(router))), None, None);

    let response = port
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect("Futu snapshot route");
    let snapshot = &response["snapshot"];
    assert_eq!(snapshot["price"], "118.40");
    assert_eq!(snapshot["session"], "after");
    assert_eq!(snapshot["extendedHours"], true);
    assert_eq!(snapshot["volume"], "0");
    assert_eq!(snapshot["previousClosePrice"], "114.97");
    assert_eq!(snapshot["lastClosePrice"], "111.25");
    assert_eq!(
        snapshot["extended"]["afterMarket"]["sessionStartAt"],
        "2026-07-18T16:00:00Z"
    );
    assert_eq!(response["meta"]["fromCache"], true);
}

#[derive(Debug)]
struct MicrostructureReaderFixture {
    calls: AtomicUsize,
    requests: Mutex<Vec<(MarketMicrostructureOperation, String, Value)>>,
    result: MicrostructureReaderResult,
}

#[derive(Debug)]
enum MicrostructureReaderResult {
    Success(Value),
    Invalid(String),
    Session(String),
    Decode {
        operation: &'static str,
        message: String,
    },
    Rejected {
        operation: &'static str,
        ret_type: i32,
        err_code: i32,
        message: String,
    },
}

impl MicrostructureReaderFixture {
    fn success() -> Self {
        Self {
            calls: AtomicUsize::new(0),
            requests: Mutex::new(Vec::new()),
            result: MicrostructureReaderResult::Success(json!({"reader": "microstructure"})),
        }
    }

    fn failure(error: MarketMicrostructureError) -> Self {
        let result = match error {
            MarketMicrostructureError::Session(message) => {
                MicrostructureReaderResult::Session(message)
            }
            MarketMicrostructureError::Decode { operation, message } => {
                MicrostructureReaderResult::Decode { operation, message }
            }
            MarketMicrostructureError::Rejected {
                operation,
                ret_type,
                err_code,
                message,
            } => MicrostructureReaderResult::Rejected {
                operation,
                ret_type,
                err_code,
                message,
            },
            MarketMicrostructureError::Invalid(message) => {
                MicrostructureReaderResult::Invalid(message)
            }
        };
        Self {
            calls: AtomicUsize::new(0),
            requests: Mutex::new(Vec::new()),
            result,
        }
    }
}

impl MarketMicrostructureReadPort for MicrostructureReaderFixture {
    fn query(
        &self,
        operation: MarketMicrostructureOperation,
        instrument_id: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        self.requests
            .lock()
            .expect("microstructure requests")
            .push((operation, instrument_id.to_owned(), params.clone()));
        match &self.result {
            MicrostructureReaderResult::Success(value) => Ok(value.clone()),
            MicrostructureReaderResult::Invalid(message) => {
                Err(MarketMicrostructureError::Invalid(message.clone()))
            }
            MicrostructureReaderResult::Session(message) => {
                Err(MarketMicrostructureError::Session(message.clone()))
            }
            MicrostructureReaderResult::Decode { operation, message } => {
                Err(MarketMicrostructureError::Decode {
                    operation,
                    message: message.clone(),
                })
            }
            MicrostructureReaderResult::Rejected {
                operation,
                ret_type,
                err_code,
                message,
            } => Err(MarketMicrostructureError::Rejected {
                operation,
                ret_type: *ret_type,
                err_code: *err_code,
                message: message.clone(),
            }),
        }
    }
}

fn microstructure_quote_port(
    reader: Arc<MicrostructureReaderFixture>,
) -> ProductionMarketDataQuotePort {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    ProductionMarketDataQuotePort::new(state, None, None, None).with_microstructure(Some(reader))
}

#[tokio::test]
async fn market_microstructure_quote_routes_call_installed_reader_without_fixtures() {
    let reader = Arc::new(MicrostructureReaderFixture::success());
    let port = microstructure_quote_port(reader.clone());
    let cases = [
        (
            "/api/v1/market-data/ticks/US.AAPL",
            "pageSize=3",
            MarketMicrostructureOperation::Ticks,
        ),
        (
            "/api/v1/market-data/broker-queue/US.AAPL",
            "pageSize=5",
            MarketMicrostructureOperation::BrokerQueue,
        ),
        (
            "/api/v1/market-data/capital-flow/US.AAPL",
            "periodType=1&beginTime=2026-08-01&endTime=2026-08-31",
            MarketMicrostructureOperation::CapitalFlow,
        ),
        (
            "/api/v1/market-data/capital-flow/US.AAPL",
            "operation=distribution",
            MarketMicrostructureOperation::CapitalDistribution,
        ),
        (
            "/api/v1/market-data/intraday/US.AAPL",
            "pageSize=3",
            MarketMicrostructureOperation::Intraday,
        ),
        (
            "/api/v1/market-data/instruments/US.AAPL/profile",
            "pageSize=3",
            MarketMicrostructureOperation::Profile,
        ),
    ];

    for (path, query, operation) in cases {
        let response = port.read(path, query).await.expect("reader response");
        assert_eq!(response, json!({"reader": "microstructure"}));
        let request = reader
            .requests
            .lock()
            .expect("microstructure requests")
            .last()
            .cloned()
            .expect("recorded microstructure request");
        assert_eq!(request.0, operation);
        assert_eq!(request.1, "US.AAPL");
    }
    assert_eq!(reader.calls.load(Ordering::SeqCst), cases.len());
    let requests = reader.requests.lock().expect("microstructure requests");
    assert_eq!(requests[0].2["pageSize"], 3);
    assert_eq!(requests[2].2["periodType"], 1);
    assert_eq!(requests[2].2["beginTime"], "2026-08-01");
    assert_eq!(requests[2].2["endTime"], "2026-08-31");
}

#[tokio::test]
async fn market_microstructure_depth_forwards_maximum_supported_level() {
    let reader = Arc::new(MicrostructureReaderFixture::success());
    let mut router = ProviderRouter::new(8);
    router
        .acquire_demand(
            "quote-test",
            [InstrumentRef {
                channel: "ORDER_BOOK".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("order-book demand");
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let port =
        ProductionMarketDataQuotePort::new(state, Some(Arc::new(Mutex::new(router))), None, None)
            .with_microstructure(Some(reader.clone()));
    port.read("/api/v1/market-data/depth/US/AAPL", "num=50")
        .await
        .expect("depth response");
    let requests = reader.requests.lock().expect("microstructure requests");
    assert_eq!(requests[0].0, MarketMicrostructureOperation::Depth);
    assert_eq!(requests[0].1, "US.AAPL");
    assert_eq!(requests[0].2["num"], 50);
}

#[tokio::test]
async fn market_microstructure_quote_routes_reject_invalid_queries_before_reader_call() {
    let reader = Arc::new(MicrostructureReaderFixture::success());
    let port = microstructure_quote_port(reader.clone());
    let cases = [
        ("/api/v1/market-data/ticks/US.AAPL", "pageSize=bad"),
        ("/api/v1/market-data/ticks/US.AAPL", "pageSize=0"),
        ("/api/v1/market-data/ticks/US.AAPL", "pageSize=1001"),
        ("/api/v1/market-data/broker-queue/US.AAPL", "pageSize=101"),
        (
            "/api/v1/market-data/capital-flow/US.AAPL",
            "periodType=intraday",
        ),
        (
            "/api/v1/market-data/capital-flow/US.AAPL",
            "beginTime=not-a-time",
        ),
        (
            "/api/v1/market-data/capital-flow/US.AAPL",
            "endTime=not-a-time",
        ),
        ("/api/v1/market-data/depth/US/AAPL", "num=bad"),
        ("/api/v1/market-data/depth/US/AAPL", "num=0"),
        ("/api/v1/market-data/depth/US/AAPL", "num=51"),
    ];

    // Parity: internal/api/marketdata/routes_test.go:450 TestReadRoutesCoverMarketsSecuritySnapshotSearchHeartbeatAndNormalize
    // Verifies invalid num/pageSize/date format queries are rejected with BAD_REQUEST before dispatching to provider
    for (path, query) in cases {
        let error = port.read(path, query).await.expect_err("invalid query");
        assert!(matches!(
            error,
            MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                code,
                ..
            } if code == "BAD_REQUEST"
        ));
    }
    assert_eq!(reader.calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn market_microstructure_quote_routes_preserve_provider_error_mapping() {
    // Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:63 TestCandlesAndDepthRoutesMapProviderFailures
    // Verifies provider failure classification and HTTP status mappings for ticks and depth routes
    let errors = [
        (
            MarketMicrostructureError::Session("OpenD is offline".to_owned()),
            503,
            "MARKET_DATA_PROVIDER_UNAVAILABLE",
            None,
        ),
        (
            MarketMicrostructureError::Decode {
                operation: "Qot_GetTicker",
                message: "malformed payload".to_owned(),
            },
            502,
            "OPEND_TICKS_FAILED",
            None,
        ),
        (
            MarketMicrostructureError::Rejected {
                operation: "Qot_GetTicker",
                ret_type: 1,
                err_code: 429,
                message: "retry after 7 seconds".to_owned(),
            },
            429,
            "MARKET_DATA_RATE_LIMITED",
            Some(7),
        ),
    ];

    for (error, status, code, retry_after) in errors {
        let reader = Arc::new(MicrostructureReaderFixture::failure(error));
        let port = microstructure_quote_port(reader.clone());
        let error = port
            .read("/api/v1/market-data/ticks/US.AAPL", "")
            .await
            .expect_err("ticks provider failure");
        assert!(matches!(
            error,
            MarketDataQuoteReadSnapshotError::Failed {
                status: actual_status,
                code: ref actual_code,
                retry_after_seconds: actual_retry,
                ..
            } if actual_status == status && actual_code == code && actual_retry == retry_after
        ));

        // Depth route preserves provider failure mapping parity (OPEND_DEPTH_FAILED for decode errors)
        let depth_expected_code = if code == "OPEND_TICKS_FAILED" {
            "OPEND_DEPTH_FAILED"
        } else {
            code
        };
        let mut router = ProviderRouter::new(8);
        router
            .acquire_demand(
                "quote-test",
                [InstrumentRef {
                    channel: "ORDER_BOOK".to_owned(),
                    market: "US".to_owned(),
                    symbol: "AAPL".to_owned(),
                    interval: None,
                }],
                false,
                0,
            )
            .expect("order-book demand");
        let state = Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Futu,
        )));
        let depth_port = ProductionMarketDataQuotePort::new(
            state,
            Some(Arc::new(Mutex::new(router))),
            None,
            None,
        )
        .with_microstructure(Some(reader));
        let error = depth_port
            .read("/api/v1/market-data/depth/US/AAPL", "")
            .await
            .expect_err("depth provider failure");
        match &error {
            MarketDataQuoteReadSnapshotError::Failed {
                status: actual_status,
                code: actual_code,
                retry_after_seconds: actual_retry,
                ..
            } => {
                assert_eq!(*actual_status, status, "status mismatch");
                assert_eq!(actual_code.as_str(), depth_expected_code, "code mismatch");
                assert_eq!(*actual_retry, retry_after, "retry mismatch");
            }
            other => panic!("expected Failed, got {other:?}"),
        }
    }
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
