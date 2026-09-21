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
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};
use jftrade_marketdata::{InstrumentRef, ProviderRouter};
use jftrade_settings::{MarketDataProvider, MarketDataProviderRuntimePort};
use std::time::Duration;
use tokio::net::TcpListener;

use crate::product::product_production_ports::{
    ProductionMarketDataQuotePort, SharedTradeReadRuntime,
};

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

/// Parity boundary: go:452dea11:internal/integration/futu/security_details_test.go:12 TestSecurityDetailsMapPreservesCompleteBrokerNeutralWireShape
/// Parity boundary: go:452dea11:internal/integration/futu/security_details_test.go:126 TestSecurityDetailsMapKeepsMissingOptionalAndProductBlocksNull
/// Parity boundary: go:452dea11:internal/integration/futu/security_details_test.go:160 TestSecurityRefMapUsesCanonicalIdentity
///
/// Futu's Go helper converts the full `pkg/futu.SecurityDetails` model,
/// including the extended/equity/warrant/option/index/plate/future/trust
/// research blocks.  The Rust `/api/v1/market-data/securities/{market}/{symbol}`
/// contract (`marketdata.SecurityDetailsPayload`) requires only
/// `instrumentId`/`market`/`name`/`symbol` and documents that providers may add
/// research fields, so Rust keeps the nine canonical fields and only enriches
/// the equity pe/pb pair from the live snapshot.  This test freezes that
/// boundary instead of claiming the Go research model was migrated.
#[tokio::test]
async fn futu_securities_route_projects_broker_neutral_envelope_boundary() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let port = ProductionMarketDataQuotePort::new(state, None, None, None);

    let response = port
        .read("/api/v1/market-data/securities/US/AAPL", "")
        .await
        .expect("Futu securities envelope");
    let security = response["security"]
        .as_object()
        .expect("security object projection");
    let mut keys: Vec<&str> = security.keys().map(String::as_str).collect();
    keys.sort_unstable();
    assert_eq!(
        keys,
        vec![
            "currency",
            "exchange",
            "instrumentId",
            "market",
            "name",
            "securityType",
            "supportedPeriods",
            "symbol",
            "timezone",
        ],
        "the Rust securities envelope is the OpenAPI-required field set: {response}"
    );
    assert_eq!(security["instrumentId"], "US.AAPL");
    assert_eq!(security["market"], "US");
    assert_eq!(security["symbol"], "AAPL");
    assert_eq!(security["currency"], "USD");
    assert_eq!(security["timezone"], "America/New_York");
    assert_eq!(response["meta"]["brokerId"], "futu");
    for absent in [
        "extended", "equity", "warrant", "option", "index", "plate", "future", "trust",
    ] {
        assert!(
            security.get(absent).is_none(),
            "unmigrated Go research block `{absent}` must not be fabricated: {response}"
        );
    }

    // Canonical identity is derived from the route, not from a Futu
    // `SecurityRef` model (Rust has no such type at this boundary).
    let hk = port
        .read("/api/v1/market-data/securities/HK/00700", "")
        .await
        .expect("Futu HK securities envelope");
    assert_eq!(hk["security"]["instrumentId"], "HK.00700");
    assert_eq!(hk["security"]["market"], "HK");
    assert_eq!(hk["security"]["currency"], "HKD");
    assert!(
        hk["security"].get("warrant").is_none(),
        "warrant research block stays absent: {hk}"
    );

    let invalid = port
        .read("/api/v1/market-data/securities/XX/AAPL", "")
        .await
        .expect_err("unknown market must fail closed");
    assert!(matches!(
        invalid,
        MarketDataQuoteReadSnapshotError::Failed { status: 400, code, .. } if code == "BAD_REQUEST"
    ));
}

#[tokio::test]
async fn live_read_routes_reject_malformed_instruments_before_any_provider_access() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:17
    // TestMarketDataReaderSurfacesTransportAndPayloadBoundaries. Go rejects a
    // malformed `BAD` symbol on QueryQuote/QueryKLines/QuerySecurityInfo/
    // QuerySecuritySnapshot/QueryOrderBook before any OpenD call. Rust's route
    // owner is `parse_market_symbol_path`, so an instrument that cannot be
    // split into `MARKET/CODE` must fail closed with 400 BAD_REQUEST on every
    // live read route instead of reaching a provider.
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let port = ProductionMarketDataQuotePort::new(state, None, None, None);
    for path in [
        "/api/v1/market-data/securities/BAD",
        "/api/v1/market-data/snapshots/BAD",
        "/api/v1/market-data/candles/BAD",
        "/api/v1/market-data/depth/BAD",
        "/api/v1/market-data/securities/BAD.SYMBOL",
        "/api/v1/market-data/snapshots/BAD.SYMBOL",
        "/api/v1/market-data/depth/BAD.SYMBOL",
    ] {
        let error = port
            .read(path, "")
            .await
            .expect_err("malformed instrument must be rejected");
        assert!(
            matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed { status: 400, .. }
            ),
            "path {path} produced {error:?}"
        );
    }
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
    let mut router = ProviderRouter::new(4);
    // `cache.Latest(id, TickFreshness)` only answers inside Go's 1.5s window,
    // so the fixture must stay near now rather than using a frozen epoch.
    let observed_at_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("clock")
        .as_millis() as i64
        - 500;
    let tick = jftrade_marketdata::Tick {
        instrument_id: "US.AAPL".to_owned(),
        price: "114.97".parse().expect("tick price"),
        volume: "100".parse().expect("tick volume"),
        volume_delta: None,
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
    // Go's live snapshot read requires a logical SNAPSHOT lease while a
    // subscription reconciler is installed for the push provider.
    router
        .acquire_demand(
            "quote-test",
            [InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("snapshot demand");
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

    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:371 TestMarketSnapshotResponseRejectsInvalidRefreshQuery
    let invalid_refresh_err = port
        .read("/api/v1/market-data/snapshots/US/AAPL", "refresh=sometimes")
        .await
        .expect_err("invalid refresh query must fail");
    match invalid_refresh_err {
        MarketDataQuoteReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 400);
            assert_eq!(code, "BAD_REQUEST");
            assert_eq!(message, "invalid refresh query");
        }
        other => panic!("unexpected error for invalid refresh: {other:?}"),
    }
}

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:273 TestMarketDataReadErrorsExposeProviderSwitchRetrySignal
///
/// Go's `GetSnapshot` reads `providerGeneration` before the provider query and
/// re-reads it afterwards, returning `ErrProviderChanged` (HTTP 409
/// `MARKET_DATA_PROVIDER_CHANGED`) when a switch landed while the read was in
/// flight.  The same fence must hold for the helper-backed read owner.
#[tokio::test]
async fn snapshot_read_fences_provider_generation_switch_during_helper_query() {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind snapshot helper fixture");
    let helper_address = listener.local_addr().expect("helper address");
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    let state_for_server = Arc::clone(&state);
    let helper_task = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("helper connection");
        let mut request = Vec::new();
        loop {
            let mut chunk = [0_u8; 1024];
            let read = stream.read(&mut chunk).await.expect("read helper request");
            if read == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..read]);
            if request.windows(4).any(|window| window == b"\r\n\r\n") {
                break;
            }
        }
        // The provider switch lands after the read owner captured the
        // generation but before the helper response is delivered.
        state_for_server
            .activate(MarketDataProvider::Akshare)
            .expect("activate akshare");
        let body = r#"{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","price":"188.50","observed_at":"2026-09-15T12:00:00Z","source":"yfinance"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        stream
            .write_all(response.as_bytes())
            .await
            .expect("write helper response");
    });
    let helper = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{helper_address}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(2),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client");
    let port = ProductionMarketDataQuotePort::new(state, None, Some(helper), None);
    let error = port
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect_err("provider switch must fence the in-flight snapshot read");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Failed {
            status: 409,
            ref code,
            ..
        } if code == "MARKET_DATA_PROVIDER_CHANGED"
    ));
    helper_task.await.expect("helper task");
}

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:35 TestInstrumentHandlersRejectMissingURIParameters
///
/// Go's security-details/snapshot/candles/depth handlers all reject a missing
/// URI parameter with 400 `BAD_REQUEST` and `invalid instrument` before the
/// provider is invoked.  Rust encodes the same contract in the read owner's
/// path parser, so every instrument route must fail identically.
#[tokio::test]
async fn instrument_read_routes_reject_missing_uri_parameters() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let port = ProductionMarketDataQuotePort::new(state, None, None, None);
    for path in [
        "/api/v1/market-data/securities/",
        "/api/v1/market-data/snapshots/",
        "/api/v1/market-data/candles/",
        "/api/v1/market-data/depth/",
    ] {
        let error = port
            .read(path, "")
            .await
            .expect_err("missing URI parameters must be rejected");
        assert!(
            matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 400,
                    ref code,
                    ref message,
                    ..
                } if code == "BAD_REQUEST" && message == "invalid instrument"
            ),
            "path {path} did not preserve the missing-URI contract"
        );
    }
}

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:208 TestSnapshotRejectsMalformedRefreshQuery
///
/// The malformed `refresh` query is rejected before any provider access; the
/// baseline asserts the provider stub was never called for the request.
#[tokio::test]
async fn snapshot_rejects_malformed_refresh_before_provider_access() {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind refresh helper fixture");
    let helper_address = listener.local_addr().expect("helper address");
    let helper_calls = Arc::new(AtomicUsize::new(0));
    let calls_for_server = Arc::clone(&helper_calls);
    let helper_task = tokio::spawn(async move {
        while let Ok((mut stream, _)) = listener.accept().await {
            calls_for_server.fetch_add(1, Ordering::SeqCst);
            let mut request = [0_u8; 4096];
            let _ = stream.read(&mut request).await;
            let body = r#"{"error":{"code":"UNEXPECTED","message":"provider must not be called"}}"#;
            let response = format!(
                "HTTP/1.1 502 Bad Gateway\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
            let _ = stream.write_all(response.as_bytes()).await;
        }
    });
    let helper = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{helper_address}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(2),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client");
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    let port = ProductionMarketDataQuotePort::new(state, None, Some(helper), None);
    let error = port
        .read(
            "/api/v1/market-data/snapshots/US/AAPL",
            "refresh=not-a-boolean",
        )
        .await
        .expect_err("malformed refresh must be rejected");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Failed {
            status: 400,
            ref code,
            ref message,
            ..
        } if code == "BAD_REQUEST" && message == "invalid refresh query"
    ));
    assert_eq!(
        helper_calls.load(Ordering::SeqCst),
        0,
        "malformed refresh must be rejected before provider access"
    );
    helper_task.abort();
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
async fn market_microstructure_depth_route_rejects_invalid_num_before_reader_call() {
    let reader = Arc::new(MicrostructureReaderFixture::success());
    let port = microstructure_quote_port(reader.clone());
    let error = port
        .read("/api/v1/market-data/depth/HK/00700", "num=abc")
        .await
        .expect_err("invalid depth num");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Failed {
            status: 400,
            code,
            ..
        } if code == "BAD_REQUEST"
    ));
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

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:387 TestLiveReadRoutesReturnConflictForMissingSubscriptionLease
///
/// Go binds live snapshot/candles/depth reads behind the logical lease owned by
/// the subscription reconciler: while Futu is active and no SNAPSHOT/KLINE
/// lease exists, the reads answer 409 `MARKET_DATA_SUBSCRIPTION_REQUIRED`
/// instead of touching provider state, and the same path succeeds after
/// `POST /subscriptions` acquires the lease.
#[tokio::test]
async fn live_read_routes_require_a_logical_subscription_lease() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(Mutex::new(futu_streaming_router()));
    let port = ProductionMarketDataQuotePort::new(state, Some(router.clone()), None, None);

    for (path, query) in [
        ("/api/v1/market-data/snapshots/US/AAPL", ""),
        ("/api/v1/market-data/candles/US/AAPL", "period=1m"),
        ("/api/v1/market-data/depth/US/AAPL", "num=10"),
    ] {
        let error = port
            .read(path, query)
            .await
            .expect_err("missing lease must be rejected before provider access");
        assert!(
            matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 409,
                    ref code,
                    ..
                } if code == "MARKET_DATA_SUBSCRIPTION_REQUIRED"
            ),
            "path {path} did not require a subscription lease"
        );
    }

    router
        .lock()
        .expect("router")
        .acquire_demand(
            "chart",
            [InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("snapshot lease");
    {
        let now_ms = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("clock")
            .as_millis() as i64;
        router
            .lock()
            .expect("router")
            .cache_mut()
            .insert(
                jftrade_marketdata::Tick {
                    instrument_id: "US.AAPL".to_owned(),
                    price: "101.5".parse().expect("tick price"),
                    volume: "10".parse().expect("tick volume"),
                    volume_delta: None,
                    snapshot: None,
                    observed_at_ms: now_ms,
                    provider_generation: 1,
                },
                1,
            )
            .expect("cache tick");
    }
    let response = port
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect("leased snapshot read reaches the provider path");
    assert_eq!(response["request"]["instrumentId"], "US.AAPL");
}

// Parity: go:452dea11:internal/app/apiserver/strategyapp/runtime_ports_test.go:110 TestMarketDataCapabilitiesReadsRuntimeDescriptor
/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:421 TestPollOnlyReadRoutesPrioritizeCapabilitiesAndPreserveLogicalLeases
///
/// A poll-only provider has no broker-side lease to consume, so Go reports the
/// capability gap first: tick candles and depth answer 409
/// `MARKET_DATA_CAPABILITY_UNSUPPORTED`, while snapshot reads still require a
/// logical lease when a reconciler is installed.
#[tokio::test]
async fn poll_only_read_routes_prioritize_capabilities_and_preserve_leases() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    let port = ProductionMarketDataQuotePort::new(state, None, None, None);

    for (path, query, capability) in [
        (
            "/api/v1/market-data/candles/US/AAPL",
            "period=tick",
            "tick candles",
        ),
        (
            "/api/v1/market-data/depth/US/AAPL",
            "num=10",
            "order book depth",
        ),
    ] {
        let error = port
            .read(path, query)
            .await
            .expect_err("poll-only capability must be rejected");
        assert!(
            matches!(
                error,
                MarketDataQuoteReadSnapshotError::Failed {
                    status: 409,
                    ref code,
                    ref message,
                    ..
                } if code == "MARKET_DATA_CAPABILITY_UNSUPPORTED" && message.contains(capability)
            ),
            "path {path} did not report {capability}"
        );
    }
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:245 TestMarketSnapshotResponseUsesFreshCache
///
/// A fresh cached tick answers the snapshot read with `meta.fromCache = true`
/// and the cached source, and the response echoes the sample's own observation
/// time instead of the request clock.
#[tokio::test]
async fn snapshot_route_serves_a_fresh_cache_hit_without_provider_access() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let mut router = ProviderRouter::new(4);
    // Go seeds `time.Now().UTC().Truncate(time.Second)` before asserting a
    // fresh cache hit, because `cache.Latest(id, TickFreshness)` only accepts a
    // 1.5s window.  Keep the fixture inside that window.
    let observed_at_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("clock")
        .as_millis() as i64
        - 500;
    router
        .cache_mut()
        .insert(
            jftrade_marketdata::Tick {
                instrument_id: "HK.00700".to_owned(),
                price: "321.4".parse().expect("tick price"),
                volume: "1282100".parse().expect("tick volume"),
                volume_delta: None,
                snapshot: Some(jftrade_marketdata::TradeQuoteSnapshot {
                    symbol: Some("HK.00700".to_owned()),
                    bid_price: Some("321.3".parse().expect("bid")),
                    ask_price: Some("321.5".parse().expect("ask")),
                    previous_close: Some("318.9".parse().expect("previous close")),
                    turnover: Some("411020000".parse().expect("turnover")),
                    session: Some("regular".to_owned()),
                    ..Default::default()
                }),
                observed_at_ms,
                provider_generation: 1,
            },
            1,
        )
        .expect("cache tick");
    router
        .acquire_demand(
            "snapshot-test",
            [InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "HK".to_owned(),
                symbol: "00700".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("snapshot demand");
    let port =
        ProductionMarketDataQuotePort::new(state, Some(Arc::new(Mutex::new(router))), None, None);

    let response = port
        .read("/api/v1/market-data/snapshots/HK/00700", "")
        .await
        .expect("cache-hit snapshot");
    assert_eq!(response["request"]["instrumentId"], "HK.00700");
    assert_eq!(response["meta"]["fromCache"], true);
    let snapshot = &response["snapshot"];
    assert_eq!(snapshot["price"], "321.4");
    assert_eq!(snapshot["bid"], "321.3");
    assert_eq!(snapshot["ask"], "321.5");
    assert_eq!(snapshot["previousClosePrice"], "318.9");
    assert_eq!(snapshot["turnover"], "411020000");
    assert_eq!(snapshot["volume"], "1282100");
    assert_eq!(
        snapshot["at"],
        candle_pagination_tests::format_unix_millis_rfc3339(observed_at_ms)
    );
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:330 TestMarketSnapshotResponseForceRefreshBypassesCache
///
/// `refresh=true` must ignore a retained sample even when it is fresh, and a
/// provider read is then required. Without a provider the read fails closed
/// instead of returning the stale cached price.
#[tokio::test]
async fn snapshot_route_force_refresh_bypasses_the_cache() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let mut router = ProviderRouter::new(4);
    let now_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("clock")
        .as_millis() as i64;
    router
        .cache_mut()
        .insert(
            jftrade_marketdata::Tick {
                instrument_id: "HK.00700".to_owned(),
                price: "999.9".parse().expect("tick price"),
                volume: "1".parse().expect("tick volume"),
                volume_delta: None,
                snapshot: Some(jftrade_marketdata::TradeQuoteSnapshot {
                    session: Some("regular".to_owned()),
                    ..Default::default()
                }),
                observed_at_ms: now_ms,
                provider_generation: 1,
            },
            1,
        )
        .expect("cache tick");
    router
        .acquire_demand(
            "snapshot-test",
            [InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "HK".to_owned(),
                symbol: "00700".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("snapshot demand");
    let port =
        ProductionMarketDataQuotePort::new(state, Some(Arc::new(Mutex::new(router))), None, None);

    // The cached price never leaks through a forced refresh.
    let error = port
        .read("/api/v1/market-data/snapshots/HK/00700", "refresh=true")
        .await
        .expect_err("forced refresh without a provider must fail closed");
    assert!(matches!(
        error,
        MarketDataQuoteReadSnapshotError::Unavailable(_)
    ));

    // `refresh=false` still serves the retained sample.
    let cached = port
        .read("/api/v1/market-data/snapshots/HK/00700", "refresh=false")
        .await
        .expect("cached snapshot");
    assert_eq!(cached["meta"]["fromCache"], true);
    assert_eq!(cached["snapshot"]["price"], "999.9");
}

/// The Futu descriptor used by live-read tests: streaming demand plus order
/// book capability, matching `jftrade-integration-futu::provider_descriptor`.
fn futu_streaming_router() -> ProviderRouter {
    use jftrade_marketdata::{
        ActivationMode, HealthStatus, ProviderCapabilities, ProviderConstraints,
        ProviderDescriptor, ProviderReadiness,
    };
    let mut router = ProviderRouter::new(32);
    router
        .register(
            ProviderDescriptor {
                selection_id: "futu".to_owned(),
                provider_id: "futu-opend".to_owned(),
                display_name: "Futu OpenD".to_owned(),
                broker_id: Some("futu".to_owned()),
                source: "bbgo:futu".to_owned(),
                default_market: "HK".to_owned(),
                supported_markets: vec!["HK".to_owned(), "US".to_owned()],
                transports: vec!["opend-tcp".to_owned()],
                capabilities: ProviderCapabilities {
                    snapshots: true,
                    streaming_quotes: true,
                    streaming_candles: true,
                    streaming_depth: true,
                    historical_candles: true,
                    tick_candles: true,
                    order_book_depth: true,
                    ..ProviderCapabilities::default()
                },
                constraints: ProviderConstraints::default(),
                notes: Vec::new(),
            },
            HealthStatus {
                connected: true,
                stream_mode: "push-stream".to_owned(),
                readiness: ProviderReadiness::Ready,
                ..HealthStatus::default()
            },
        )
        .expect("register futu descriptor");
    router
        .activate("futu", ActivationMode::Explicit)
        .expect("activate futu");
    router
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:563 TestReadRoutesMapProviderAndRequestFailures
///
/// Go wires one failing provider and asserts the whole read group maps each
/// failure to its transport status: provider 502, markets 500, security 502,
/// snapshot 502, malformed heartbeat and normalize bodies 400.
#[tokio::test]
async fn read_routes_map_provider_and_request_failures() {
    let provider = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    provider.set_readiness(false, true, false);
    let runtime =
        Arc::new(crate::product::product_production_ports::SharedTradeReadRuntime::default());
    let quote = ProductionMarketDataQuotePort::new(provider.clone(), None, None, None)
        .with_trade_runtime(Some(runtime.clone()));

    // Provider descriptor failure: 502 on the catalog side of the provider.
    // The Rust provider-status owner reports an unconfigured/unavailable
    // provider as a transport failure instead of fabricating health.
    let unconfigured = ProductionMarketDataQuotePort::new(
        Arc::new(ActiveProviderState::new(None)),
        None,
        None,
        None,
    );
    let security = unconfigured
        .read("/api/v1/market-data/securities/US/AAPL", "")
        .await
        .expect_err("unconfigured provider must fail closed");
    assert!(matches!(
        security,
        MarketDataQuoteReadSnapshotError::Unavailable(_)
    ));
    let snapshot = unconfigured
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect_err("unconfigured snapshot must fail closed");
    assert!(matches!(
        snapshot,
        MarketDataQuoteReadSnapshotError::Unavailable(_)
    ));

    // A Futu runtime without any cached snapshot cannot invent a quote.
    let snapshot = quote
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect_err("snapshot without cache must fail closed");
    assert!(matches!(
        snapshot,
        MarketDataQuoteReadSnapshotError::Unavailable(_)
    ));

    // Malformed heartbeat body and provider-side normalize failures are input
    // errors (400) instead of upstream failures.
    let heartbeat = crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationRequest {
        method: "POST".to_owned(),
        path: "/api/v1/market-data/subscriptions/heartbeat".to_owned(),
        query: String::new(),
        body: b"bad".to_vec(),
    };
    let subscription =
        crate::product::product_production_ports::ProductionMarketDataSubscriptionMutationPort::new(
            provider.clone(),
            None,
            None,
        );
    let error = subscription
        .dispatch(&heartbeat)
        .expect_err("malformed heartbeat must be rejected");
    assert!(matches!(
        error,
        crate::product::product_market_data_subscription_mutation_port::MarketDataSubscriptionMutationPortError::Failed {
            status: 400,
            ref code,
            ..
        } if code == "BAD_REQUEST"
    ));

    let actions =
        crate::product::product_production_ports::ProductionMarketDataProviderActionsPort::new(
            Some(Arc::new(quote)),
        );
    let normalize = actions
        .dispatch(
            &crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/instruments/normalize".to_owned(),
                query: String::new(),
                body: b"bad".to_vec(),
            },
        )
        .await
        .expect_err("malformed normalize body must be rejected");
    assert!(matches!(
        normalize,
        crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPortError::Failed {
            status: 400,
            ref code,
            ..
        } if code == "BAD_REQUEST"
    ));
    let normalize = actions
        .dispatch(
            &crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/instruments/normalize".to_owned(),
                query: String::new(),
                body: br#"{"market":"INVALID","symbol":"12345"}"#.to_vec(),
            },
        )
        .await
        .expect_err("unsupported market must be rejected");
    assert!(matches!(
        normalize,
        crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsPortError::Failed {
            status: 400,
            ref code,
            ..
        } if code == "MARKET_INSTRUMENT_INVALID"
    ));
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:306 TestMarketSnapshotResponseQueriesQuoteSnapshotOnCacheMiss
///
/// With a SNAPSHOT lease and no retained sample, the read must issue exactly
/// one provider quote query and answer from that live result rather than
/// inventing a cached value. Go asserts `BasicQuoteCallCount() == 1`; the Rust
/// equivalent is the counted `SecuritySnapshotReadPort` fixture below.
#[derive(Debug, Default)]
struct CountingSecuritySnapshotReader {
    calls: AtomicUsize,
    last_instruments: Mutex<Vec<String>>,
}

impl jftrade_integration_futu::SecuritySnapshotReadPort for CountingSecuritySnapshotReader {
    fn query(
        &self,
        instruments: &[String],
    ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        self.last_instruments
            .lock()
            .expect("snapshot instruments")
            .extend(instruments.iter().cloned());
        Ok(vec![jftrade_marketdata::BrokerSecuritySnapshot {
            symbol: Some("HK.00700".to_owned()),
            name: Some("Tencent Holdings".to_owned()),
            market: Some("HK".to_owned()),
            last_price: Some("321.4".parse().expect("last price")),
            bid_price: Some("321.3".parse().expect("bid")),
            ask_price: Some("321.5".parse().expect("ask")),
            previous_close: Some("318.9".parse().expect("previous close")),
            turnover: Some("411020000".parse().expect("turnover")),
            volume: Some("1282100".parse().expect("volume")),
            session: Some("regular".to_owned()),
            ..Default::default()
        }])
    }
}

#[tokio::test]
async fn snapshot_route_queries_the_provider_once_on_cache_miss() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let mut router = ProviderRouter::new(4);
    router
        .acquire_demand(
            "snapshot-cache-miss-test",
            [InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "HK".to_owned(),
                symbol: "00700".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("snapshot demand");
    let reader = Arc::new(CountingSecuritySnapshotReader::default());
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let port =
        ProductionMarketDataQuotePort::new(state, Some(Arc::new(Mutex::new(router))), None, None)
            .with_trade_runtime(Some(runtime));

    let response = port
        .read("/api/v1/market-data/snapshots/HK/00700", "")
        .await
        .expect("cache-miss snapshot");
    assert_eq!(response["request"]["instrumentId"], "HK.00700");
    assert_eq!(response["meta"]["fromCache"], false);
    assert_eq!(response["snapshot"]["price"], "321.4");
    assert_eq!(response["snapshot"]["volume"], "1282100");
    assert_eq!(
        reader.calls.load(Ordering::SeqCst),
        1,
        "cache miss must query the provider exactly once"
    );
    assert_eq!(
        reader
            .last_instruments
            .lock()
            .expect("instruments")
            .as_slice(),
        &["HK.00700".to_owned()],
        "the provider query must name the requested instrument"
    );
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:478 TestMarketSecurityDetailsResponseQueriesSecuritySnapshot
///
/// Go issues exactly one `GetSecuritySnapshot` and one `GetStaticInfo` call and
/// projects `name`, `exchangeType`, `currentPrice` and `equity.peRate` from the
/// provider answer. Rust has no `GetStaticInfo` step on this route (the lookup
/// reader is a separate catalog port), so the parity contract freezes the part
/// it does own: one snapshot RPC, the provider name/price, and the equity
/// projection. The missing static-info call is asserted as a boundary instead of
/// being fabricated.
#[derive(Debug, Default)]
struct CountingSecurityDetailsReader {
    calls: AtomicUsize,
    instruments: Mutex<Vec<String>>,
}

impl jftrade_integration_futu::SecuritySnapshotReadPort for CountingSecurityDetailsReader {
    fn query(
        &self,
        instruments: &[String],
    ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        self.instruments
            .lock()
            .expect("details instruments")
            .extend(instruments.iter().cloned());
        Ok(vec![jftrade_marketdata::BrokerSecuritySnapshot {
            symbol: Some("HK.00700".to_owned()),
            name: Some("Tencent Holdings".to_owned()),
            market: Some("HK".to_owned()),
            security_type: Some("EQUITY".to_owned()),
            last_price: Some("321.4".parse().expect("last price")),
            pe_rate: Some("16.7".parse().expect("pe rate")),
            pb_rate: Some("3.2".parse().expect("pb rate")),
            volume: Some("1282100".parse().expect("volume")),
            turnover: Some("411020000".parse().expect("turnover")),
            ..Default::default()
        }])
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercore/market_details_ws_test.go:11 TestMarketSecurityDetailsWebSocketSendsInitialPayload
#[tokio::test]
async fn securities_route_queries_the_security_snapshot_once() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let reader = Arc::new(CountingSecurityDetailsReader::default());
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let port = ProductionMarketDataQuotePort::new(state, None, None, None)
        .with_trade_runtime(Some(runtime));

    let response = port
        .read("/api/v1/market-data/securities/HK/00700", "")
        .await
        .expect("Futu security details");
    assert_eq!(response["request"]["instrumentId"], "HK.00700");
    assert_eq!(response["security"]["name"], "Tencent Holdings");
    assert_eq!(response["security"]["currentPrice"], "321.4");
    assert_eq!(response["security"]["equity"]["peRate"], "16.7");
    assert_eq!(
        reader.calls.load(Ordering::SeqCst),
        1,
        "security details must issue exactly one snapshot RPC"
    );
    assert_eq!(
        reader.instruments.lock().expect("instruments").as_slice(),
        &["HK.00700".to_owned()]
    );
    assert!(
        response["meta"].get("fromCache").is_none(),
        "security details do not expose a cache flag: {response}"
    );
}

/// Parity: go:452dea11:internal/app/apiserver/marketdataapp/market_http_test.go:368 TestMarketSnapshotResponseRejectsInvalidRefreshQuery
///
/// `refresh=sometimes` must be a 400 input error before any provider access,
/// matching Go's `DecodeSnapshotQuery` rejection in the adapter tests.
#[derive(Debug, Default)]
struct CountingSnapshotReaderForInvalidRefresh {
    calls: AtomicUsize,
}

impl jftrade_integration_futu::SecuritySnapshotReadPort
    for CountingSnapshotReaderForInvalidRefresh
{
    fn query(
        &self,
        _: &[String],
    ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        Ok(Vec::new())
    }
}

#[tokio::test]
async fn snapshot_route_rejects_invalid_refresh_before_provider_access() {
    let state = Arc::new(ActiveProviderState::new(Some(
        jftrade_settings::MarketDataProvider::Futu,
    )));
    let reader = Arc::new(CountingSnapshotReaderForInvalidRefresh::default());
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let port = ProductionMarketDataQuotePort::new(state, None, None, None)
        .with_trade_runtime(Some(runtime));

    let error = port
        .read(
            "/api/v1/market-data/snapshots/HK/00700",
            "refresh=sometimes",
        )
        .await
        .expect_err("invalid refresh must be rejected");
    assert!(
        matches!(
            error,
            MarketDataQuoteReadSnapshotError::Failed {
                status: 400,
                ref code,
                ref message,
                ..
            } if code == "BAD_REQUEST" && message == "invalid refresh query"
        ),
        "invalid refresh produced {error:?}"
    );
    assert_eq!(
        reader.calls.load(Ordering::SeqCst),
        0,
        "invalid refresh must be rejected before provider access"
    );
}

/// Parity: go:8a78fc78:pkg/futu/subscription_lifecycle_test.go:123
/// TestQueryKLinesWithoutLeaseReturnsExplicitErrorBeforeRealtimeRead.
///
/// A history read without an explicit KLINE lease fails with 409
/// `MARKET_DATA_SUBSCRIPTION_REQUIRED` before any provider access; the message
/// names the KLINE channel and the interval, and never leaks a raw OpenD error.
#[tokio::test]
async fn candle_read_without_a_kline_lease_fails_before_realtime_provider_access() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(Mutex::new(futu_streaming_router()));
    // Bound the history reader to a live loopback peer that must never be
    // reached while the lease is missing.
    let listener = StdTcpListener::bind("127.0.0.1:0").expect("listener");
    listener
        .set_nonblocking(true)
        .expect("nonblocking listener");
    let port = ProductionMarketDataQuotePort::new(state, Some(router), None, None);

    let error = port
        .read("/api/v1/market-data/candles/US/AAPL", "period=1m&limit=2")
        .await
        .expect_err("missing KLINE lease must be rejected");
    match error {
        MarketDataQuoteReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "MARKET_DATA_SUBSCRIPTION_REQUIRED");
            assert!(message.contains("KLINE"), "message = {message}");
            assert!(message.contains("US.AAPL:1m"), "message = {message}");
            assert!(
                !message.to_ascii_lowercase().contains("opend"),
                "the raw provider error must not leak: {message}"
            );
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // The rejected read must not have opened any provider connection.
    match listener.accept() {
        Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {}
        other => panic!("missing lease leaked a provider connection: {other:?}"),
    }
}

/// Parity: go:8a78fc78:pkg/futu/subscription_lifecycle_test.go:142
/// TestBasicQuoteReadRequiresExplicitLeaseAndNeverLeaksRawOpenDError.
///
/// QueryTicker without a Basic lease answers the broker-neutral 409 envelope
/// before any provider access; after acquiring the SNAPSHOT lease the same
/// route proceeds. The public message never contains a raw OpenD error.
#[tokio::test]
async fn basic_quote_read_requires_a_lease_and_never_leaks_the_raw_opend_error() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(Mutex::new(futu_streaming_router()));
    let port = ProductionMarketDataQuotePort::new(state, Some(router.clone()), None, None);

    let error = port
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect_err("missing lease must be rejected before provider access");
    match error {
        MarketDataQuoteReadSnapshotError::Failed {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 409);
            assert_eq!(code, "MARKET_DATA_SUBSCRIPTION_REQUIRED");
            assert!(message.contains("SNAPSHOT"), "message = {message}");
            assert!(message.contains("US.AAPL"), "message = {message}");
            assert!(
                !message.contains("retType") && !message.contains("Qot_"),
                "raw OpenD detail leaked: {message}"
            );
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Acquire the logical lease and seed the documented cache source: the same
    // read now passes the gate rather than reporting a subscription error.
    router
        .lock()
        .expect("router")
        .acquire_demand(
            "ticker",
            [InstrumentRef {
                channel: "SNAPSHOT".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: None,
            }],
            false,
            0,
        )
        .expect("snapshot lease");
    let now_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("clock")
        .as_millis() as i64;
    router
        .lock()
        .expect("router")
        .cache_mut()
        .insert(
            jftrade_marketdata::Tick {
                instrument_id: "US.AAPL".to_owned(),
                price: "188.25".parse().expect("price"),
                volume: "7".parse().expect("volume"),
                volume_delta: None,
                snapshot: None,
                observed_at_ms: now_ms,
                provider_generation: 1,
            },
            1,
        )
        .expect("cache tick");
    let response = port
        .read("/api/v1/market-data/snapshots/US/AAPL", "")
        .await
        .expect("leased read");
    assert_eq!(response["request"]["instrumentId"], "US.AAPL");
}

/// Go keeps one 3203 coordinator per Exchange in front of the raw reader
/// (`security_snapshot_coordinator.go`). This proves the production runtime
/// installs that layer rather than the bare reader: a 45-symbol HK batch must
/// reach the physical reader as three HK-sized calls, the second identical
/// query must be served from the 3s TTL cache, and the sliding 54-call gate
/// must be shared across those calls.
#[derive(Debug, Default)]
struct RecordingSnapshotBatchReader {
    batches: Mutex<Vec<Vec<String>>>,
}

impl RecordingSnapshotBatchReader {
    fn batches(&self) -> Vec<Vec<String>> {
        self.batches.lock().expect("recorded batches").clone()
    }
}

impl jftrade_integration_futu::SecuritySnapshotBatchReader for RecordingSnapshotBatchReader {
    fn query_batch(
        &self,
        symbols: &[String],
    ) -> Result<Vec<jftrade_marketdata::BrokerSecuritySnapshot>, String> {
        self.batches
            .lock()
            .expect("recorded batches")
            .push(symbols.to_vec());
        Ok(symbols
            .iter()
            .map(|symbol| jftrade_marketdata::BrokerSecuritySnapshot {
                symbol: Some(symbol.clone()),
                name: Some(symbol.clone()),
                ..Default::default()
            })
            .collect())
    }
}

#[test]
fn cached_security_snapshot_reader_batches_hk_and_serves_repeats_from_cache() {
    use jftrade_integration_futu::{CachedSecuritySnapshotReader, SECURITY_SNAPSHOT_HK_BATCH_SIZE};

    let inner = Arc::new(RecordingSnapshotBatchReader::default());
    let reader = CachedSecuritySnapshotReader::new(inner.clone());
    // 45 HK symbols: Go splits HK at 20, so this must be exactly 3 calls.
    let instruments = (0..45)
        .map(|index| format!("HK.{index:05}"))
        .collect::<Vec<_>>();

    let first = reader.query(&instruments).expect("first snapshot query");
    assert_eq!(first.len(), 45);
    let batches = inner.batches();
    assert_eq!(batches.len(), 3, "HK batches are capped at 20");
    assert_eq!(batches[0].len(), SECURITY_SNAPSHOT_HK_BATCH_SIZE);
    assert_eq!(batches[1].len(), SECURITY_SNAPSHOT_HK_BATCH_SIZE);
    assert_eq!(batches[2].len(), 5);
    // Every physical read is booked against the shared budget until the
    // window releases, which is what keeps OpenD's own quota unreachable.
    assert_eq!(reader.coordinator().admitted_calls(), 3);

    // A repeated identical query must not reach the reader at all.
    let second = reader.query(&instruments).expect("cached snapshot query");
    assert_eq!(second.len(), 45);
    assert_eq!(inner.batches().len(), 3, "cache hit must not re-read");
    assert_eq!(reader.coordinator().admitted_calls(), 3);
}
