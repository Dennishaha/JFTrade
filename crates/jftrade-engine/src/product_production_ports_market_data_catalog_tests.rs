use super::*;
use jftrade_integration_marketdata_helper::HelperClientConfig;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

#[derive(Debug)]
struct SearchReader {
    entries: Vec<jftrade_integration_futu::InstrumentSearchEntry>,
    fail: bool,
}

impl jftrade_integration_futu::InstrumentSearchReadPort for SearchReader {
    fn search(
        &self,
        keyword: &str,
    ) -> Result<
        Vec<jftrade_integration_futu::InstrumentSearchEntry>,
        jftrade_integration_futu::InstrumentSearchError,
    > {
        assert!(["分众传媒", "002027", "不存在"].contains(&keyword));
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
        assert_eq!((market, code), ("SZ", "002027"));
        Ok(self.entries.clone())
    }
}

fn search_entry(market: &str, code: &str) -> jftrade_integration_futu::InstrumentSearchEntry {
    jftrade_integration_futu::InstrumentSearchEntry {
        market: market.to_owned(),
        code: code.to_owned(),
        name: Some("分众传媒".to_owned()),
        security_type: Some("EQUITY".to_owned()),
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
