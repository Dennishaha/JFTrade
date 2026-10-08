use super::*;
use crate::product::product_production_ports::{
    ProductionMarketDataQuotePort, SharedTradeReadRuntime,
};
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig};
use jftrade_marketdata::{
    BrokerSecuritySnapshot, InstrumentRef, ProviderRouter, Tick, TradeQuoteSnapshot,
};
use jftrade_settings::MarketDataProvider;
use jftrade_settings::MarketDataProviderRuntimePort;
use serde_json::{Value, json};
use std::sync::Mutex;
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

// Parity: go:452dea11:internal/marketdata/service_facade_test.go:136 TestServiceRejectsSnapshotCompletedAfterProviderChange
#[tokio::test]
async fn snapshot_http_provider_change_rejects_late_success_and_keeps_cache_empty() {
    let directory = tempdir().unwrap();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Yfinance)));
    let server_state = state.clone();
    let server = tokio::spawn(async move {
        let (mut stream, _) = tokio::time::timeout(Duration::from_secs(3), listener.accept())
            .await
            .expect("accept deadline")
            .unwrap();
        let mut request = Vec::new();
        tokio::time::timeout(Duration::from_secs(3), async {
            while !request.windows(4).any(|value| value == b"\r\n\r\n") {
                let mut chunk = [0; 1024];
                let read = stream.read(&mut chunk).await.unwrap();
                assert_ne!(read, 0, "request ended before headers");
                request.extend_from_slice(&chunk[..read]);
                assert!(request.len() <= 8192, "bounded fixture headers");
            }
        })
        .await
        .expect("header deadline");
        assert!(
            String::from_utf8(request)
                .unwrap()
                .starts_with("GET /providers/yfinance/snapshot/US/AAPL HTTP/1.1\r\n")
        );
        // The owner's query is now in flight. Advance the actual active
        // provider generation before delivering the old successful response.
        server_state.activate(MarketDataProvider::Akshare).unwrap();
        let body = r#"{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","price":"188.5","volume":"10","observed_at":"2026-10-08T00:00:00Z","source":"yfinance"}"#;
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        );
        tokio::time::timeout(
            Duration::from_secs(3),
            stream.write_all(response.as_bytes()),
        )
        .await
        .expect("response deadline")
        .unwrap();
    });
    let helper = HelperClient::new(HelperClientConfig {
        base_url: format!("http://{address}"),
        bearer_token: None,
        request_timeout: Duration::from_secs(2),
        max_attempts: 1,
        retry_delay: Duration::ZERO,
    })
    .unwrap();
    let router = Arc::new(Mutex::new(ProviderRouter::new(32)));
    let quote = Arc::new(ProductionMarketDataQuotePort::new(
        state.clone(),
        Some(router.clone()),
        Some(helper),
        None,
    ));
    let handle = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().unwrap(),
            directory.path().join("settings.json"),
        )
        .unwrap()
        .with_market_data_quote_read_snapshot_port(quote),
    )
    .await
    .unwrap();
    let response = get(
        handle.startup_record().address,
        "/api/v1/market-data/snapshots/US/AAPL?refresh=true",
    )
    .await;
    handle.shutdown().await.unwrap();
    server.await.expect("join helper fixture");
    assert_eq!(response.0, 409, "{response:?}");
    assert_eq!(response.1["error"]["code"], "MARKET_DATA_PROVIDER_CHANGED");
    assert_eq!(state.snapshot().provider, Some(MarketDataProvider::Akshare));
    assert_eq!(state.snapshot().generation, 1);
    assert!(router.lock().unwrap().cache().history("US.AAPL").is_empty());
    assert_eq!(router.lock().unwrap().cache().instrument_count(), 0);
}

#[derive(Debug)]
struct Reader {
    calls: Mutex<Vec<Vec<String>>>,
    result: Result<Vec<BrokerSecuritySnapshot>, String>,
}
impl jftrade_integration_futu::SecuritySnapshotReadPort for Reader {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String> {
        self.calls.lock().unwrap().push(instruments.to_vec());
        self.result.clone()
    }
}
fn futu_state() -> Arc<ActiveProviderState> {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, false);
    state
}
async fn get(address: SocketAddr, path: &str) -> (u16, Value) {
    tokio::time::timeout(
        Duration::from_secs(5),
        request_json_with_status(address, "GET", path, None, &[]),
    )
    .await
    .expect("HTTP deadline")
}
fn instrument() -> InstrumentRef {
    InstrumentRef {
        channel: "SNAPSHOT".into(),
        market: "US".into(),
        symbol: "AAPL".into(),
        interval: None,
    }
}
async fn cached_response(authoritative: bool) -> (u16, Value) {
    let directory = tempdir().unwrap();
    let router = Arc::new(Mutex::new(ProviderRouter::new(32)));
    {
        let mut router = router.lock().unwrap();
        router
            .acquire_demand("chart", [instrument()], false, 0)
            .unwrap();
    }
    let quote = Arc::new(ProductionMarketDataQuotePort::new(
        futu_state(),
        Some(router.clone()),
        None,
        None,
    ));
    let handle = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().unwrap(),
            directory.path().join("settings.json"),
        )
        .unwrap()
        .with_market_data_quote_read_snapshot_port(quote),
    )
    .await
    .unwrap();
    router
        .lock()
        .unwrap()
        .cache_mut()
        .insert(
            Tick {
                instrument_id: "US.AAPL".into(),
                price: "100".parse().unwrap(),
                volume: "0".parse().unwrap(),
                volume_delta: None,
                snapshot: authoritative.then(|| TradeQuoteSnapshot {
                    authoritative: true,
                    ..Default::default()
                }),
                observed_at_ms: std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap()
                    .as_millis() as i64,
                provider_generation: 1,
            },
            1,
        )
        .unwrap();
    let response = get(
        handle.startup_record().address,
        "/api/v1/market-data/snapshots/US/AAPL",
    )
    .await;
    handle.shutdown().await.unwrap();
    assert_eq!(router.lock().unwrap().cache().history("US.AAPL").len(), 1);
    response
}

// Parity: go:452dea11:internal/marketdata/quote_availability_test.go:35 TestSnapshotSerializationKeepsLegacyZeroValuesAvailable
#[tokio::test]
async fn snapshot_http_legacy_quote_keeps_zero_strings_available() {
    let response = cached_response(false).await;
    assert_eq!(response.0, 200, "{response:?}");
    assert_eq!(response.1["data"]["snapshot"]["price"], "100");
    assert_eq!(response.1["data"]["meta"]["fromCache"], true);
    for field in ["bid", "ask", "volume", "turnover"] {
        assert_eq!(
            response.1["data"]["snapshot"][field], "0",
            "{field}: {response:?}"
        );
    }
}

// Parity: go:452dea11:internal/marketdata/quote_availability_test.go:9 TestSnapshotSerializationPreservesAuthoritativeMissingQuoteFields
#[tokio::test]
async fn snapshot_http_authoritative_quote_keeps_missing_fields_null() {
    let response = cached_response(true).await;
    assert_eq!(response.0, 200, "{response:?}");
    assert_eq!(response.1["data"]["snapshot"]["price"], "100");
    for field in ["bid", "ask", "volume", "turnover"] {
        assert_eq!(
            response.1["data"]["snapshot"][field],
            Value::Null,
            "{field}: {response:?}"
        );
        assert!(
            response.1["data"]["snapshot"]
                .as_object()
                .unwrap()
                .contains_key(field)
        );
    }
    // The actual cache route is Futu; this proves its nullable snapshot wire,
    // not the frozen AKShare live/latest aggregate envelope.
    assert_eq!(response.1["data"]["meta"]["brokerId"], "futu");
}

// Parity: go:452dea11:internal/marketdata/service_facade_test.go:83 TestServiceSnapshotResolvesChinaAggregateToExchangeLeaf
#[tokio::test]
async fn snapshot_http_china_aggregate_uses_exchange_leaf_for_actual_reader_and_request() {
    let directory = tempdir().unwrap();
    let reader = Arc::new(Reader {
        calls: Mutex::new(Vec::new()),
        result: Ok(vec![BrokerSecuritySnapshot {
            symbol: Some("SH.600519".into()),
            market: Some("SH".into()),
            last_price: Some("1338.5".parse().unwrap()),
            volume: Some("10".parse().unwrap()),
            session: Some("regular".into()),
            ..Default::default()
        }]),
    });
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let quote = Arc::new(
        ProductionMarketDataQuotePort::new(futu_state(), None, None, None)
            .with_trade_runtime(Some(runtime)),
    );
    let handle = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().unwrap(),
            directory.path().join("settings.json"),
        )
        .unwrap()
        .with_market_data_quote_read_snapshot_port(quote),
    )
    .await
    .unwrap();
    let response = get(
        handle.startup_record().address,
        "/api/v1/market-data/snapshots/CN/SH.600519?refresh=true",
    )
    .await;
    handle.shutdown().await.unwrap();
    assert_eq!(response.0, 200, "{response:?}");
    assert_eq!(
        reader.calls.lock().unwrap().as_slice(),
        &[vec!["SH.600519".to_owned()]]
    );
    assert_eq!(
        response.1["data"]["request"],
        json!({"market":"SH","symbol":"600519","instrumentId":"SH.600519"})
    );
    assert_eq!(response.1["data"]["snapshot"]["price"], "1338.5");
}

// Parity: go:452dea11:internal/marketdata/service_facade_test.go:69 TestServiceSnapshotErrorsAreBusinessVisible
#[tokio::test]
async fn snapshot_http_denied_and_missing_reader_results_keep_the_current_unavailable_boundary() {
    for result in [Err("provider denied snapshot".to_owned()), Ok(Vec::new())] {
        let directory = tempdir().unwrap();
        let reader = Arc::new(Reader {
            calls: Mutex::new(Vec::new()),
            result,
        });
        let runtime = Arc::new(SharedTradeReadRuntime::default());
        runtime.set_security_snapshots(Some(reader.clone()));
        let quote = Arc::new(
            ProductionMarketDataQuotePort::new(futu_state(), None, None, None)
                .with_trade_runtime(Some(runtime)),
        );
        let handle = start_product(
            ProductConfig::test_cutover(
                "127.0.0.1:0".parse().unwrap(),
                directory.path().join("settings.json"),
            )
            .unwrap()
            .with_market_data_quote_read_snapshot_port(quote),
        )
        .await
        .unwrap();
        let response = get(
            handle.startup_record().address,
            "/api/v1/market-data/snapshots/HK/00700?refresh=true",
        )
        .await;
        handle.shutdown().await.unwrap();
        assert_eq!(response.0, 503, "{response:?}");
        assert_eq!(
            response.1["error"]["code"],
            "MARKET_DATA_QUOTE_READ_UNAVAILABLE"
        );
        assert_eq!(
            response.1["error"]["message"],
            "no cached snapshot available for HK.00700"
        );
        assert_eq!(
            reader.calls.lock().unwrap().as_slice(),
            &[vec!["HK.00700".to_owned()]]
        );
    }
}

#[tokio::test]
async fn security_http_market_case_aliases_keep_currency_and_timezone_equal() {
    let directory = tempdir().unwrap();
    let reader = Arc::new(Reader {
        calls: Mutex::new(Vec::new()),
        result: Ok(vec![BrokerSecuritySnapshot {
            name: Some("Fixture Security".into()),
            last_price: Some("100".parse().unwrap()),
            ..Default::default()
        }]),
    });
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let quote = Arc::new(
        ProductionMarketDataQuotePort::new(futu_state(), None, None, None)
            .with_trade_runtime(Some(runtime)),
    );
    let handle = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().unwrap(),
            directory.path().join("settings.json"),
        )
        .unwrap()
        .with_market_data_quote_read_snapshot_port(quote),
    )
    .await
    .unwrap();
    let mut comparisons = Vec::new();
    for (market, symbol) in [
        ("US", "aapl"),
        ("HK", "00700"),
        ("SH", "600519"),
        ("SZ", "000001"),
    ] {
        let upper = get(
            handle.startup_record().address,
            &format!("/api/v1/market-data/securities/{market}/{symbol}"),
        )
        .await;
        let lower = get(
            handle.startup_record().address,
            &format!(
                "/api/v1/market-data/securities/{}/{symbol}",
                market.to_ascii_lowercase()
            ),
        )
        .await;
        comparisons.push((market, upper, lower));
    }
    handle.shutdown().await.unwrap();
    assert_eq!(reader.calls.lock().unwrap().len(), 8);
    let mut mismatches = Vec::new();
    for (market, upper, lower) in comparisons {
        assert_eq!((upper.0, lower.0), (200, 200));
        for field in ["currency", "timezone"] {
            if lower.1["data"]["security"][field] != upper.1["data"]["security"][field] {
                mismatches.push(format!(
                    "{market} {field}: lower={}, upper={}",
                    lower.1["data"]["security"][field], upper.1["data"]["security"][field]
                ));
            }
        }
    }
    assert!(mismatches.is_empty(), "{}", mismatches.join("; "));
}
