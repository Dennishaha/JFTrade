use super::*;
use jftrade_integration_marketdata_helper::HelperClientConfig;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

#[tokio::test]
async fn futu_catalog_retains_market_precision_and_session_metadata() {
    let port = ProductionMarketDataCatalogPort::new(
        Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu))),
        None,
    );
    let response = port.read("/api/v1/market-data/markets", "").await.unwrap();
    assert_eq!(response["defaultMarket"], "HK");
    let markets = response["markets"].as_array().unwrap();
    assert_eq!(markets.len(), 4);
    for (index, (code, resolved, currency, precision, tick, extended, sessions)) in [
        ("HK", "HK", "HKD", 3, 0.001, false, vec![(570, 720), (780, 960)]),
        ("US", "US", "USD", 2, 0.01, true, vec![(570, 960)]),
        ("SH", "CN", "CNY", 2, 0.01, false, vec![(570, 690), (780, 900)]),
        ("SZ", "CN", "CNY", 2, 0.01, false, vec![(570, 690), (780, 900)]),
    ]
    .into_iter()
    .enumerate()
    {
        let market = &markets[index];
        assert_eq!(market["code"], code);
        assert_eq!(market["resolvedMarket"], resolved);
        assert_eq!(market["preferredPrefix"], code);
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
