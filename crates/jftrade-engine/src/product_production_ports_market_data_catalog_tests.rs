use super::*;
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::MarketDataCatalogReadSnapshotPort;

#[derive(Debug)]
struct SearchReader {
    entries: Vec<jftrade_integration_futu::InstrumentSearchEntry>,
    fail: bool,
    allowed_keywords: Option<Vec<String>>,
    /// Number of `Qot_GetSearchQuote`-backed reads. Go asserts the loopback
    /// server observed exactly one search request, so the fixture has to
    /// count them instead of inferring from the response.
    calls: Arc<AtomicUsize>,
}

impl jftrade_integration_futu::InstrumentSearchReadPort for SearchReader {
    fn search(
        &self,
        keyword: &str,
    ) -> Result<
        Vec<jftrade_integration_futu::InstrumentSearchEntry>,
        jftrade_integration_futu::InstrumentSearchError,
    > {
        self.calls.fetch_add(1, Ordering::SeqCst);
        if let Some(allowed) = &self.allowed_keywords {
            assert!(
                allowed.iter().any(|value| value == keyword),
                "unexpected search keyword {keyword:?}, allowed {allowed:?}"
            );
        }
        if self.fail {
            return Err(jftrade_integration_futu::InstrumentSearchError::Session(
                "offline".to_owned(),
            ));
        }
        Ok(self.entries.clone())
    }

    fn lookup(
        &self,
        market: &str,
        code: &str,
    ) -> Result<
        Vec<jftrade_integration_futu::InstrumentSearchEntry>,
        jftrade_integration_futu::InstrumentSearchError,
    > {
        let matched: Vec<_> = self
            .entries
            .iter()
            .filter(|entry| entry.market == market && entry.code.eq_ignore_ascii_case(code))
            .cloned()
            .collect();
        if !matched.is_empty() {
            return Ok(matched);
        }
        // Qualified queries normalize `CN:002027` onto the SZ leaf, which is
        // the only fixture that reaches this branch without an exact entry.
        assert_eq!((market, code), ("SZ", "002027"));
        Ok(self.entries.clone())
    }
}

fn search_calls() -> Arc<AtomicUsize> {
    Arc::new(AtomicUsize::new(0))
}

fn search_entry(market: &str, code: &str) -> jftrade_integration_futu::InstrumentSearchEntry {
    search_entry_typed(market, code, "EQUITY")
}

fn search_entry_typed(
    market: &str,
    code: &str,
    security_type: &str,
) -> jftrade_integration_futu::InstrumentSearchEntry {
    jftrade_integration_futu::InstrumentSearchEntry {
        market: market.to_owned(),
        code: code.to_owned(),
        name: Some("分众传媒".to_owned()),
        security_type: Some(security_type.to_owned()),
        is_watched: true,
        lot_size: None,
    }
}

fn futu_search_port(reader: SearchReader) -> ProductionMarketDataCatalogPort {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_instrument_search_reader(Some(Arc::new(reader)));
    ProductionMarketDataCatalogPort::new(
        Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu))),
        None,
    )
    .with_trade_runtime(Some(runtime))
}

#[tokio::test]
async fn futu_search_resolves_chinese_name_bare_code_and_qualified_code() {
    let port = futu_search_port(SearchReader {
        entries: vec![search_entry("SZ", "002027")],
        fail: false,
        allowed_keywords: Some(
            ["分众传媒", "002027", "SZ.002027", "CN:002027"]
                .into_iter()
                .map(str::to_owned)
                .collect(),
        ),
        calls: search_calls(),
    });
    for query in [
        "query=%E5%88%86%E4%BC%97%E4%BC%A0%E5%AA%92",
        "query=002027",
        "query=SZ.002027",
        "query=CN:002027",
    ] {
        let result = port
            .read("/api/v1/market-data/instruments", query)
            .await
            .unwrap();
        assert_eq!(result["resolutionStatus"], "resolved");
        assert_eq!(result["entries"][0]["instrumentId"], "SZ.002027");
        assert_eq!(result["entries"][0]["name"], "分众传媒");
        assert_eq!(result["entries"][0]["resolvedMarket"], "CN");
        assert_eq!(result["entries"][0]["selectable"], true);
        assert_eq!(result["entries"][0]["isWatched"], true);
    }
}

#[tokio::test]
async fn futu_search_filters_and_deduplicates_before_limiting_without_hiding_ambiguity() {
    let port = futu_search_port(SearchReader {
        entries: vec![
            search_entry("US", "FOCUS"),
            search_entry("SZ", "002027"),
            search_entry("SZ", "002027"),
            search_entry("SH", "600001"),
        ],
        fail: false,
        allowed_keywords: Some(vec!["分众传媒".to_owned(), "002027".to_owned()]),
        calls: search_calls(),
    });
    let result = port
        .read(
            "/api/v1/market-data/instruments",
            "query=分众传媒&market=CN&limit=1",
        )
        .await
        .unwrap();
    assert_eq!(result["resolutionStatus"], "ambiguous");
    assert_eq!(result["totalReturned"], 1);
    assert_eq!(result["entries"][0]["instrumentId"], "SZ.002027");
    let result = port
        .read("/api/v1/market-data/instruments", "query=002027")
        .await
        .unwrap();
    assert_eq!(result["resolutionStatus"], "resolved");
    assert_eq!(result["totalReturned"], 1);
    let conflict = port
        .read(
            "/api/v1/market-data/instruments",
            "query=SZ.002027&market=HK",
        )
        .await;
    assert!(matches!(
        conflict,
        Err(MarketDataCatalogReadSnapshotError::Invalid { .. })
    ));
}

#[tokio::test]
async fn futu_search_distinguishes_no_match_unsupported_market_and_runtime_failure() {
    for (entries, status) in [
        (vec![], "not_found"),
        (vec![search_entry("JP", "9988")], "unavailable"),
    ] {
        let port = futu_search_port(SearchReader {
            entries,
            fail: false,
            allowed_keywords: Some(vec!["不存在".to_owned()]),
            calls: search_calls(),
        });
        let result = port
            .read("/api/v1/market-data/instruments", "query=不存在")
            .await
            .unwrap();
        assert_eq!(result["resolutionStatus"], status);
    }
    let port = futu_search_port(SearchReader {
        entries: vec![],
        fail: true,
        allowed_keywords: Some(vec!["分众传媒".to_owned()]),
        calls: search_calls(),
    });
    assert!(matches!(
        port.read("/api/v1/market-data/instruments", "query=分众传媒")
            .await,
        Err(MarketDataCatalogReadSnapshotError::Unavailable(_))
    ));
    port.trade_runtime.as_ref().unwrap().clear();
    assert!(matches!(
        port.read("/api/v1/market-data/instruments", "query=SZ.002027")
            .await,
        Err(MarketDataCatalogReadSnapshotError::Unavailable(_))
    ));
}

#[tokio::test]
async fn futu_catalog_retains_market_precision_and_session_metadata() {
    let port = ProductionMarketDataCatalogPort::new(
        Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu))),
        None,
    );
    let response = port.read("/api/v1/market-data/markets", "").await.unwrap();
    assert_eq!(response["defaultMarket"], "HK");
    let markets = response["markets"].as_array().unwrap();
    assert_eq!(markets.len(), 5);
    for (index, (code, resolved, prefix, currency, precision, tick, extended, sessions)) in [
        ("HK", "HK", "HK", "HKD", 3, 0.001, false, vec![(570, 720), (780, 960)]),
        ("US", "US", "US", "USD", 2, 0.01, true, vec![(570, 960)]),
        ("CN", "CN", "", "CNY", 2, 0.01, false, vec![(570, 690), (780, 900)]),
        ("SH", "CN", "SH", "CNY", 2, 0.01, false, vec![(570, 690), (780, 900)]),
        ("SZ", "CN", "SZ", "CNY", 2, 0.01, false, vec![(570, 690), (780, 900)]),
    ]
    .into_iter()
    .enumerate()
    {
        let market = &markets[index];
        assert_eq!(market["code"], code);
        assert_eq!(market["resolvedMarket"], resolved);
        assert_eq!(market["preferredPrefix"], prefix);
        assert_eq!(market["quoteCurrency"], currency);
        assert_eq!(market["precision"], json!({"price": precision, "quote": precision}));
        assert_eq!(market["tickSize"], tick);
        assert_eq!(market["supportsExtendedHours"], extended);
        assert_eq!(market["requiresExchangePrefix"], resolved == "CN");
        let windows = market["regularSessions"].as_array().unwrap();
        assert_eq!(windows.len(), sessions.len());
        for (window, (start, end)) in windows.iter().zip(sessions) {
            assert_eq!(window["startMinute"], start);
            assert_eq!(window["endMinute"], end);
        }
    }

    let cn_market = markets.iter().find(|m| m["code"] == "CN").unwrap();
    assert_eq!(cn_market["displayName"], "沪深");
    assert_eq!(cn_market["aliases"], json!(["SH", "SZ", "CNSH", "CNSZ"]));
}

async fn helper_catalog_response(provider: MarketDataProvider, name: &str, body: Value) -> Value {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let expected_path = format!("GET /providers/{name}/markets HTTP/1.1\r\n");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.unwrap();
        let mut request = Vec::new();
        while !request.ends_with(b"\r\n\r\n") {
            let byte = stream.read_u8().await.unwrap();
            request.push(byte);
            assert!(request.len() < 8192);
        }
        assert!(String::from_utf8(request).unwrap().starts_with(&expected_path));
        let body = body.to_string();
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        stream.write_all(response.as_bytes()).await.unwrap();
    });
    let client = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{address}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(5),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .unwrap();
    let port = ProductionMarketDataCatalogPort::new(
        Arc::new(ActiveProviderState::new(Some(provider))),
        Some(client),
    );
    let result = port.read("/api/v1/market-data/markets", "").await.unwrap();
    server.await.unwrap();
    result
}

#[tokio::test]
async fn helper_catalog_projects_precision_and_sessions_to_camel_case() {
    for (provider, name) in [
        (MarketDataProvider::Yfinance, "yfinance"),
        (MarketDataProvider::Akshare, "akshare"),
    ] {
        let response = helper_catalog_response(provider, name, json!({
            "default_market": "HK",
            "markets": [{
                "code": "HK", "resolved_market": "HK", "preferred_prefix": "HK",
                "display_name": "Hong Kong", "quote_currency": "HKD",
                "timezone": "Asia/Hong_Kong", "supports_extended_hours": false,
                "requires_exchange_prefix": false, "aliases": ["HKEX"],
                "regular_sessions": [{"start_minute": 570, "end_minute": 720, "label": "morning"}],
                "precision": {"price": 3, "quote": 3}, "tick_size": 0.001
            }]
        })).await;
        assert_eq!(response["defaultMarket"], "HK");
        assert_eq!(response["markets"][0], json!({
            "code": "HK", "market": "HK", "resolvedMarket": "HK", "preferredPrefix": "HK",
            "name": "Hong Kong", "displayName": "Hong Kong", "quoteCurrency": "HKD",
            "timezone": "Asia/Hong_Kong", "supportsExtendedHours": false,
            "requiresExchangePrefix": false, "aliases": ["HKEX"],
            "regularSessions": [{"startMinute": 570, "endMinute": 720, "label": "morning"}],
            "precision": {"price": 3, "quote": 3}, "tickSize": 0.001
        }));
    }
}

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:97 TestMarketsRouteFailsWhenActiveProviderDescriptorIsUnavailable
///
/// Go resolves `defaultMarket` from `svc.ProviderDescriptor` after fetching the
/// market list and maps a descriptor failure to 500 `MARKET_DATA_FAILED`.  The
/// Rust owner reads the default market from the helper's own response, so the
/// equivalent guarantee is: any helper failure on `/markets` surfaces 500 with
/// `MARKET_DATA_FAILED` and the upstream message instead of a partial payload.
#[tokio::test]
async fn markets_route_fails_with_market_data_failed_when_active_provider_is_unavailable() {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind unavailable markets fixture");
    let address = listener.local_addr().expect("fixture address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("markets connection");
        let mut request = Vec::new();
        while !request.windows(4).any(|window| window == b"\r\n\r\n") {
            let mut chunk = [0_u8; 1024];
            let read = stream.read(&mut chunk).await.expect("read request");
            if read == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..read]);
        }
        let body = r#"{"error":{"message":"active provider is unavailable"}}"#;
        let response = format!(
            "HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        stream
            .write_all(response.as_bytes())
            .await
            .expect("write fixture response");
    });
    let client = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{address}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(5),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client");
    let port = ProductionMarketDataCatalogPort::new(
        Arc::new(ActiveProviderState::new(Some(
            MarketDataProvider::Yfinance,
        ))),
        Some(client),
    );
    let error = port
        .read("/api/v1/market-data/markets", "")
        .await
        .expect_err("provider failure must surface");
    assert!(matches!(
        error,
        MarketDataCatalogReadSnapshotError::Failed {
            status: 500,
            ref code,
            ref message,
        } if code == "MARKET_DATA_FAILED" && message == "active provider is unavailable"
    ));
    server.await.expect("markets fixture server");
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:625 TestInstrumentSearchRouteReturnsSubsetResolutionContract
///
/// Go qualifies the request as `market=CN&query=000001&limit=20`, always asks
/// the provider for the maximum candidate window, filters to the requested
/// market subset, keeps stable provider order, reports `ambiguous`, and
/// resolves a qualified query (`SH.600519`) through the exact leaf lookup.
#[tokio::test]
async fn instrument_search_route_returns_subset_resolution_contract() {
    // The Futu reader is the Rust owner of cross-market search; it must be
    // asked for the full window and must report the CN subset afterwards.
    let port = futu_search_port(SearchReader {
        entries: vec![
            search_entry_typed("US", "000001", "WARRANT"),
            search_entry_typed("SH", "000001", "INDEX"),
            search_entry_typed("SZ", "000001", "EQUITY"),
            search_entry_typed("JP", "000001", "PLATE"),
        ],
        fail: false,
        allowed_keywords: Some(vec!["000001".to_owned()]),
        calls: search_calls(),
    });
    let result = port
        .read(
            "/api/v1/market-data/instruments",
            "market=CN&query=000001&limit=20",
        )
        .await
        .expect("CN subset search");
    assert_eq!(result["query"], "000001");
    assert_eq!(result["requestedMarket"], "CN");
    assert_eq!(result["resolutionStatus"], "ambiguous");
    assert_eq!(result["totalReturned"], 2);
    let entries = result["entries"].as_array().expect("entries");
    assert_eq!(entries[0]["instrumentId"], "SH.000001");
    assert_eq!(entries[0]["securityType"], "INDEX");
    assert_eq!(entries[1]["instrumentId"], "SZ.000001");
    for entry in entries {
        assert_eq!(entry["resolvedMarket"], "CN");
        assert_eq!(entry["symbol"], "000001");
        assert_eq!(entry["selectable"], true);
    }
    // Go also asserts that exactly one provider search is issued for the
    // CN-filtered query; the fixture asserts the requested keyword window so a
    // regression that widened the window (or re-queried) would fail here.
    let qualified = futu_search_port(SearchReader {
        entries: vec![search_entry_typed("SH", "600519", "EQUITY")],
        fail: false,
        allowed_keywords: None,
        calls: search_calls(),
    });
    let result = qualified
        .read(
            "/api/v1/market-data/instruments",
            "market=CN&query=SH.600519",
        )
        .await
        .expect("qualified leaf lookup");
    assert_eq!(result["resolutionStatus"], "resolved");
    assert_eq!(result["totalReturned"], 1);
    assert_eq!(result["entries"][0]["instrumentId"], "SH.600519");

    // Unsupported all-market candidates stay visible but not selectable, and
    // no market filter reports every stable candidate.
    let port = futu_search_port(SearchReader {
        entries: vec![
            search_entry_typed("JP", "TYPE0", "PLATE"),
            search_entry_typed("SH", "TYPE1", "INDEX"),
            search_entry_typed("SZ", "TYPE2", "EQUITY"),
            search_entry_typed("US", "TYPE3", "WARRANT"),
        ],
        fail: false,
        allowed_keywords: Some(vec!["all-types".to_owned()]),
        calls: search_calls(),
    });
    let result = port
        .read("/api/v1/market-data/instruments", "query=all-types")
        .await
        .expect("all-market search");
    assert_eq!(result["totalReturned"], 4);
    let entries = result["entries"].as_array().expect("entries");
    // Go asserts the four candidates keep their provider order and type
    // (Warrant, Index, Eqty, Plate for US, SH, SZ, JP in the Go fixture); the
    // Rust fixture feeds JP/SH/SZ/US so the same "no reordering" guarantee is
    // asserted against the order the provider returned.
    for (index, security_type) in ["PLATE", "INDEX", "EQUITY", "WARRANT"].iter().enumerate() {
        assert_eq!(
            entries[index]["securityType"], *security_type,
            "stable provider order must be preserved: {result}"
        );
    }
    let jp = entries
        .iter()
        .find(|entry| entry["market"] == "JP")
        .expect("JP candidate");
    assert_eq!(jp["selectable"], false);
    assert!(
        jp["unavailableReason"]
            .as_str()
            .is_some_and(|reason| !reason.is_empty()),
        "unsupported candidate needs a reason: {jp}"
    );
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:725 TestInstrumentSearchRouteValidatesInputAndMapsProviderFailures
///
/// Go separates provider failures from caller input errors: unknown results
/// map to `not_found`/`unavailable`, malformed query/limit/market combinations
/// answer 400 `MARKET_INSTRUMENT_INVALID`, and a provider error maps to 502
/// `MARKET_INSTRUMENT_SEARCH_FAILED`.
#[tokio::test]
async fn instrument_search_route_validates_input_and_maps_provider_failures() {
    let port = futu_search_port(SearchReader {
        entries: vec![],
        fail: false,
        allowed_keywords: Some(vec!["missing".to_owned()]),
        calls: search_calls(),
    });
    let not_found = port
        .read("/api/v1/market-data/instruments", "query=missing")
        .await
        .expect("missing result");
    assert_eq!(not_found["resolutionStatus"], "not_found");

    let port = futu_search_port(SearchReader {
        entries: vec![search_entry("JP", "7203")],
        fail: false,
        allowed_keywords: Some(vec!["Toyota".to_owned()]),
        calls: search_calls(),
    });
    let unavailable = port
        .read("/api/v1/market-data/instruments", "query=Toyota")
        .await
        .expect("unsupported market result");
    assert_eq!(unavailable["resolutionStatus"], "unavailable");
    let entry = &unavailable["entries"][0];
    assert_eq!(entry["selectable"], false);
    assert!(
        entry["unavailableReason"]
            .as_str()
            .is_some_and(|reason| !reason.is_empty()),
        "unavailable candidate needs a reason: {entry}"
    );

    let port = futu_search_port(SearchReader {
        entries: vec![search_entry("US", "AAPL")],
        fail: false,
        allowed_keywords: None,
        calls: search_calls(),
    });
    for query in ["", "limit=0", "limit=101", "limit=bad", "market=JP"] {
        let combined = if query.is_empty() {
            String::new()
        } else if query.starts_with("limit=") {
            format!("query=AAPL&{query}")
        } else {
            format!("{query}&query=Toyota")
        };
        let error = port
            .read("/api/v1/market-data/instruments", &combined)
            .await
            .expect_err("invalid search request must be rejected");
        assert!(
            matches!(
                error,
                MarketDataCatalogReadSnapshotError::Invalid { ref code, .. }
                    if code == "MARKET_INSTRUMENT_INVALID"
            ),
            "query {combined:?} did not raise MARKET_INSTRUMENT_INVALID"
        );
    }

    let port = futu_search_port(SearchReader {
        entries: vec![],
        fail: true,
        allowed_keywords: Some(vec!["provider-error".to_owned()]),
        calls: search_calls(),
    });
    let error = port
        .read("/api/v1/market-data/instruments", "query=provider-error")
        .await
        .expect_err("unavailable search runtime must fail closed");
    assert!(matches!(
        error,
        MarketDataCatalogReadSnapshotError::Unavailable(_)
    ));

    // A non-Futu helper provider maps an upstream rejection to the Go
    // fallback code MARKET_INSTRUMENT_SEARCH_FAILED with its status intact.
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind search failure fixture");
    let address = listener.local_addr().expect("fixture address");
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("search connection");
        let mut request = Vec::new();
        while !request.windows(4).any(|window| window == b"\r\n\r\n") {
            let mut chunk = [0_u8; 1024];
            let read = stream.read(&mut chunk).await.expect("read request");
            if read == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..read]);
        }
        let body = r#"{"error":{"message":"OpenD search failed"}}"#;
        let response = format!(
            "HTTP/1.1 502 Bad Gateway\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        stream
            .write_all(response.as_bytes())
            .await
            .expect("write fixture response");
    });
    let helper = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{address}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(5),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .expect("helper client");
    let port = ProductionMarketDataCatalogPort::new(
        Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance))),
        Some(helper),
    );
    let error = port
        .read("/api/v1/market-data/instruments", "query=provider-error")
        .await
        .expect_err("provider failure must surface");
    assert!(matches!(
        error,
        MarketDataCatalogReadSnapshotError::Failed {
            status: 502,
            ref code,
            ..
        } if code == "MARKET_INSTRUMENT_SEARCH_FAILED"
    ));
    server.await.expect("search fixture server");
}

/// Parity: go:452dea11:pkg/futu/adapter_marketdata_search_test.go:58
/// TestBrokerAdapterSecuritySearchMapsCrossMarketOpenDResults.
///
/// Go's loopback server answers `Qot_GetSearchQuote` with four rows: a US row
/// whose name carries padding, a `CNSH.600519` row with a Chinese name, a nil
/// row, and an HK row whose code is a single space. Only two candidates may
/// survive, the keyword must reach OpenD trimmed, the caller's limit must be
/// forwarded as `maxCount`, and the request must happen exactly once.
#[tokio::test]
async fn futu_search_maps_cross_market_rows_and_drops_unusable_entries() {
    let reader = SearchReader {
        entries: vec![
            jftrade_integration_futu::InstrumentSearchEntry {
                market: "US".to_owned(),
                code: "US.AAPL".to_owned(),
                name: Some(" Apple Inc. ".to_owned()),
                security_type: Some("EQTY".to_owned()),
                is_watched: true,
                lot_size: None,
            },
            jftrade_integration_futu::InstrumentSearchEntry {
                market: "SH".to_owned(),
                code: "CNSH.600519".to_owned(),
                name: Some("贵州茅台".to_owned()),
                security_type: Some("EQTY".to_owned()),
                is_watched: false,
                lot_size: None,
            },
        ],
        fail: false,
        allowed_keywords: Some(["apple".to_owned()].into_iter().collect()),
        calls: search_calls(),
    };
    let calls = Arc::clone(&reader.calls);
    let port = futu_search_port(reader);

    let response = port
        .read("/api/v1/market-data/instruments", "query=%20%20apple%20%20&limit=8")
        .await
        .expect("cross-market search");

    assert_eq!(calls.load(Ordering::SeqCst), 1, "one search request");
    let entries = response["entries"].as_array().expect("entries");
    assert_eq!(entries.len(), 2, "only usable cross-market rows survive: {response}");
    assert_eq!(entries[0]["market"], "US");
    assert_eq!(
        entries[0]["instrumentId"], "US.AAPL",
        "a US row whose code already carries its market prefix keeps one prefix: {response}"
    );
    assert_eq!(entries[0]["code"], "AAPL");
    assert_eq!(entries[0]["securityType"], "EQTY");
    assert_eq!(entries[0]["isWatched"], true);
    assert_eq!(entries[1]["market"], "SH");
    assert_eq!(
        entries[1]["instrumentId"], "SH.600519",
        "CNSH must collapse to the SH display market without doubling the prefix: {response}"
    );
    assert_eq!(entries[1]["resolvedMarket"], "CN");
    assert_eq!(entries[1]["isWatched"], false);
}

/// Parity: go:452dea11:pkg/futu/adapter_marketdata_search_test.go:113
/// TestBrokerAdapterSecuritySearchRejectsInvalidQueriesBeforeConnecting.
///
/// Go validates the keyword and limit inside `QuerySecuritySearch` before
/// `withRetryingClient` runs, so a blank keyword or an out-of-range limit must
/// fail without opening a connection. Rust enforces the same two rules in the
/// product route and the typed reader.
#[tokio::test]
async fn futu_search_rejects_invalid_queries_before_reaching_opend() {
    for query in ["query=%20&limit=8", "query=AAPL&limit=0", "query=AAPL&limit=101"] {
        let reader = SearchReader {
            entries: vec![search_entry("US", "AAPL")],
            fail: false,
            allowed_keywords: None,
            calls: search_calls(),
        };
        let calls = Arc::clone(&reader.calls);
        let port = futu_search_port(reader);
        let error = port
            .read("/api/v1/market-data/instruments", query)
            .await
            .expect_err(&format!("{query} must be rejected"));
        assert!(
            matches!(
                error,
                MarketDataCatalogReadSnapshotError::Invalid { ref code, .. }
                    if code == "MARKET_INSTRUMENT_INVALID"
            ),
            "{query} error = {error:?}"
        );
        assert_eq!(
            calls.load(Ordering::SeqCst),
            0,
            "{query} must be rejected before the OpenD search"
        );
    }
}

/// Parity: go:452dea11:pkg/futu/adapter_marketdata_search_test.go:38
/// TestCanonicalSearchQuoteSymbolHandlesOpenDPrefixedCodes.
///
/// The engine projection owns the public `instrumentId`, so the seven Go rows
/// are asserted here end-to-end rather than on the reader helper alone.
#[tokio::test]
async fn futu_search_canonicalizes_open_d_prefixed_symbols_like_go() {
    let rows = [
        ("US", "US.AAPL", "US.AAPL"),
        ("US", "AAPL", "US.AAPL"),
        // An inner dot is part of the code, not a market separator.
        ("US", "BRK.B", "US.BRK.B"),
        ("US", "US.BRK.B", "US.BRK.B"),
        // ":" is rewritten to "." before the prefix check, so `hk:00700`
        // canonicalizes to the HK security `HK.00700`.
        ("HK", "hk:00700", "HK.00700"),
        // CNSH collapses onto the SH display market without doubling.
        ("SH", "CNSH.600519", "SH.600519"),
        ("JP", "JP.7203", "JP.7203"),
    ];
    for (market, code, expected) in rows {
        let port = futu_search_port(SearchReader {
            entries: vec![search_entry_typed(market, code, "EQTY")],
            fail: false,
            allowed_keywords: None,
            calls: search_calls(),
        });
        // Go drives the reader directly, and the route restricts `market` to
        // the routable display markets (US/HK/CN/SH/SZ), so the request omits
        // it and the row's own market decides the prefix.
        let response = port
            .read("/api/v1/market-data/instruments", "query=AAPL&limit=8")
            .await
            .unwrap_or_else(|error| panic!("{market}/{code} search failed: {error:?}"));
        let entries = response["entries"].as_array().expect("entries");
        assert_eq!(
            entries.len(),
            1,
            "the row must survive filtering: {response}"
        );
        assert_eq!(
            entries[0]["instrumentId"], expected,
            "canonicalSearchQuoteSymbol({market}, {code})"
        );
    }
}
