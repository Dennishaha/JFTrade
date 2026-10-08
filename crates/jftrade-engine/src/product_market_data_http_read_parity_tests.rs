//! Real HTTP requests through production market-data owners.
use super::*;
use crate::product::MarketDataQuoteReadFuture;
use crate::product::product_market_data_provider_actions_port::{
    MarketDataProviderActionsFuture, MarketDataProviderActionsPort,
    MarketDataProviderActionsRequest,
};
use crate::product::product_production_ports::{
    ProductionMarketDataCatalogPort, ProductionMarketDataProviderActionsPort,
    ProductionMarketDataProviderPort, ProductionMarketDataQuotePort,
    ProductionMarketDataSubscriptionMutationPort, SharedTradeReadRuntime,
};
use jftrade_marketdata::{BrokerSecuritySnapshot, InstrumentRef, ProviderRouter, Tick};
use jftrade_settings::MarketDataProvider;
use serde_json::{Value, json};
use std::sync::Mutex;

#[derive(Debug, Default)]
struct SnapshotReader {
    calls: Mutex<Vec<Vec<String>>>,
}
impl jftrade_integration_futu::SecuritySnapshotReadPort for SnapshotReader {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String> {
        self.calls.lock().expect("calls").push(instruments.to_vec());
        Ok(vec![BrokerSecuritySnapshot {
            symbol: Some("US.AAPL".into()),
            market: Some("US".into()),
            name: Some("Apple".into()),
            last_price: Some("101.5".parse().unwrap()),
            bid_price: Some("101.4".parse().unwrap()),
            ask_price: Some("101.6".parse().unwrap()),
            session: Some("regular".into()),
            ..Default::default()
        }])
    }
}

#[derive(Debug, Default)]
struct SearchReader {
    calls: Mutex<Vec<String>>,
}
impl jftrade_integration_futu::InstrumentSearchReadPort for SearchReader {
    fn search(
        &self,
        keyword: &str,
    ) -> Result<
        Vec<jftrade_integration_futu::InstrumentSearchEntry>,
        jftrade_integration_futu::InstrumentSearchError,
    > {
        self.calls
            .lock()
            .expect("search calls")
            .push(keyword.into());
        Ok(Vec::new())
    }
    fn lookup(
        &self,
        _: &str,
        _: &str,
    ) -> Result<
        Vec<jftrade_integration_futu::InstrumentSearchEntry>,
        jftrade_integration_futu::InstrumentSearchError,
    > {
        Ok(Vec::new())
    }
}

#[derive(Debug)]
struct ObservedQuote {
    owner: ProductionMarketDataQuotePort,
    requests: Mutex<Vec<(String, String)>>,
}
impl MarketDataQuoteReadSnapshotPort for ObservedQuote {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        self.requests
            .lock()
            .expect("quote requests")
            .push((path.into(), query.into()));
        self.owner.read(path, query)
    }
}

#[derive(Debug)]
struct ObservedActions {
    owner: ProductionMarketDataProviderActionsPort,
    requests: Mutex<Vec<MarketDataProviderActionsRequest>>,
}
impl MarketDataProviderActionsPort for ObservedActions {
    fn dispatch<'a>(
        &'a self,
        request: &'a MarketDataProviderActionsRequest,
    ) -> MarketDataProviderActionsFuture<'a> {
        self.requests
            .lock()
            .expect("actions requests")
            .push(request.clone());
        self.owner.dispatch(request)
    }
}

fn state() -> Arc<ActiveProviderState> {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, false);
    state
}

async fn http(address: SocketAddr, method: &str, path: &str, body: Option<&str>) -> (u16, Value) {
    tokio::time::timeout(
        std::time::Duration::from_secs(5),
        request_json_with_status(address, method, path, body, &[]),
    )
    .await
    .expect("HTTP deadline")
}

// Parity: go:452dea11:internal/api/marketdata/routes_test.go:459 TestReadRoutesCoverMarketsSecuritySnapshotSearchHeartbeatAndNormalize
#[tokio::test]
async fn market_data_http_success_sequence_preserves_raw_security_and_normalizes_snapshot_identity()
{
    let directory = tempdir().expect("directory");
    let state = state();
    let router = Arc::new(Mutex::new(ProviderRouter::new(32)));
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    let reader = Arc::new(SnapshotReader::default());
    let search = Arc::new(SearchReader::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    runtime.set_instrument_search_reader(Some(search.clone()));
    // The frozen route fixture has no physical reconciler. Keep its ungated
    // quote reads while the real router owns subscription acquire/heartbeat.
    let quote = Arc::new(ObservedQuote {
        owner: ProductionMarketDataQuotePort::new(state.clone(), None, None, None)
            .with_trade_runtime(Some(runtime.clone())),
        requests: Mutex::new(Vec::new()),
    });
    let actions = Arc::new(ObservedActions {
        owner: ProductionMarketDataProviderActionsPort::new(Some(quote.clone())),
        requests: Mutex::new(Vec::new()),
    });
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        directory.path().join("settings.json"),
    )
    .expect("config")
    .with_market_data_provider_read_snapshot_port(Arc::new(ProductionMarketDataProviderPort {
        active_provider_state: state.clone(),
        runtime_status: None,
        router: Some(router.clone()),
        physical: None,
    }))
    .with_market_data_catalog_read_snapshot_port(Arc::new(
        ProductionMarketDataCatalogPort::new(state.clone(), None).with_trade_runtime(Some(runtime)),
    ))
    .with_market_data_quote_read_snapshot_port(quote.clone())
    .with_market_data_subscription_mutation_port(Arc::new(
        ProductionMarketDataSubscriptionMutationPort::new(state, Some(router.clone()), None),
    ))
    .with_market_data_provider_actions_port(actions.clone());
    let handle = start_product(config).await.expect("product");
    let address = handle.startup_record().address;
    let provider = http(address, "GET", "/api/v1/market-data/provider", None).await;
    let markets = http(address, "GET", "/api/v1/market-data/markets", None).await;
    let security = http(
        address,
        "GET",
        "/api/v1/market-data/securities/us/aapl",
        None,
    )
    .await;
    let snapshot = http(
        address,
        "GET",
        "/api/v1/market-data/snapshots/us/aapl?refresh=true",
        None,
    )
    .await;
    let acquired = http(
        address,
        "POST",
        "/api/v1/market-data/subscriptions",
        Some(r#"{"consumerId":"chart-main","instruments":[{"market":"US","symbol":"AAPL"}]}"#),
    )
    .await;
    let heartbeat = http(
        address,
        "POST",
        "/api/v1/market-data/subscriptions/heartbeat",
        Some(r#"{"consumerId":"chart-main"}"#),
    )
    .await;
    let searched = http(
        address,
        "GET",
        "/api/v1/market-data/instruments?market=US&query=nvda",
        None,
    )
    .await;
    let normalized = http(
        address,
        "POST",
        "/api/v1/market-data/instruments/normalize",
        Some(r#"{"market":"us","symbol":"aapl"}"#),
    )
    .await;
    handle.shutdown().await.expect("shutdown");
    assert_eq!(
        [
            provider.0,
            markets.0,
            security.0,
            snapshot.0,
            acquired.0,
            heartbeat.0,
            searched.0,
            normalized.0
        ],
        [200; 8],
        "security={security:?}, snapshot={snapshot:?}"
    );
    assert_eq!(provider.1["data"]["descriptor"]["providerId"], "futu-opend");
    assert_eq!(
        provider.1["data"]["descriptor"]["capabilities"]["orderBookDepth"],
        true
    );
    assert_eq!(markets.1["data"]["defaultMarket"], "HK");
    assert_eq!(
        quote.requests.lock().unwrap().as_slice(),
        &[
            ("/api/v1/market-data/securities/us/aapl".into(), "".into()),
            (
                "/api/v1/market-data/snapshots/us/aapl".into(),
                "refresh=true".into()
            )
        ]
    );
    assert_eq!(
        reader.calls.lock().unwrap().as_slice(),
        &[vec!["US.aapl".to_owned()], vec!["US.AAPL".to_owned()]]
    );
    assert_eq!(snapshot.1["data"]["request"]["instrumentId"], "US.AAPL");
    assert_eq!(snapshot.1["data"]["meta"]["instrumentId"], "US.AAPL");
    assert_eq!(snapshot.1["data"]["meta"]["fromCache"], false);
    assert_eq!(heartbeat.1["data"]["totalActiveSubscriptions"], 1);
    assert_eq!(router.lock().unwrap().demand().logical_count, 1);
    assert_eq!(searched.1["data"]["query"], "nvda");
    assert_eq!(search.calls.lock().unwrap().as_slice(), &["nvda"]);
    let calls = actions.requests.lock().unwrap();
    assert_eq!(calls.len(), 1);
    assert_eq!(
        serde_json::from_slice::<Value>(&calls[0].body).unwrap(),
        json!({"market":"us","symbol":"aapl"})
    );
    assert_eq!(normalized.1["data"]["instrumentId"], "US.AAPL");
}

#[tokio::test]
async fn snapshot_http_case_aliases_share_cache_and_refresh_uses_canonical_provider_identity() {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(SnapshotReader::default());
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let router = Arc::new(Mutex::new(ProviderRouter::new(32)));
    let quote = Arc::new(
        ProductionMarketDataQuotePort::new(state(), Some(router.clone()), None, None)
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
    .expect("product");
    let address = handle.startup_record().address;
    let rejected = http(
        address,
        "GET",
        "/api/v1/market-data/snapshots/us/aapl?refresh=true",
        None,
    )
    .await;
    assert_eq!(rejected.0, 409);
    assert_eq!(
        rejected.1["error"]["code"],
        "MARKET_DATA_SUBSCRIPTION_REQUIRED"
    );
    assert!(reader.calls.lock().unwrap().is_empty());
    router
        .lock()
        .unwrap()
        .acquire_demand(
            "chart-main",
            [InstrumentRef {
                channel: "SNAPSHOT".into(),
                market: "US".into(),
                symbol: "AAPL".into(),
                interval: None,
            }],
            false,
            0,
        )
        .unwrap();
    let live = http(
        address,
        "GET",
        "/api/v1/market-data/snapshots/us/aapl?refresh=true",
        None,
    )
    .await;
    assert_eq!(live.0, 200, "{live:?}");
    assert_eq!(
        live.1["data"]["request"],
        json!({"instrumentId":"US.AAPL","market":"US","symbol":"AAPL"})
    );
    assert_eq!(
        reader.calls.lock().unwrap().as_slice(),
        &[vec!["US.AAPL".to_owned()]]
    );
    let observed_at_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis() as i64;
    router
        .lock()
        .unwrap()
        .cache_mut()
        .insert(
            Tick {
                instrument_id: "US.AAPL".into(),
                price: "99".parse().unwrap(),
                volume: "1".parse().unwrap(),
                volume_delta: None,
                snapshot: None,
                observed_at_ms,
                provider_generation: 1,
            },
            1,
        )
        .unwrap();
    for path in [
        "/api/v1/market-data/snapshots/us/aapl",
        "/api/v1/market-data/snapshots/US/AAPL",
    ] {
        let cached = http(address, "GET", path, None).await;
        assert_eq!(cached.0, 200, "{cached:?}");
        assert_eq!(cached.1["data"]["meta"]["fromCache"], true);
        assert_eq!(cached.1["data"]["meta"]["instrumentId"], "US.AAPL");
        assert_eq!(cached.1["data"]["snapshot"]["price"], "99");
    }
    assert_eq!(reader.calls.lock().unwrap().len(), 1);
    let refreshed = http(
        address,
        "GET",
        "/api/v1/market-data/snapshots/us/aapl?refresh=true",
        None,
    )
    .await;
    assert_eq!(refreshed.0, 200);
    assert_eq!(refreshed.1["data"]["meta"]["fromCache"], false);
    assert_eq!(refreshed.1["data"]["snapshot"]["price"], "101.5");
    assert_eq!(
        reader.calls.lock().unwrap().as_slice(),
        &[vec!["US.AAPL".to_owned()], vec!["US.AAPL".to_owned()]]
    );
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/marketdata/routes_test.go:147 TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback
#[tokio::test]
async fn market_data_http_unknown_broker_rejects_all_four_reads_without_touching_active_reader() {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(SnapshotReader::default());
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader.clone()));
    let quote = Arc::new(
        ProductionMarketDataQuotePort::new(state(), None, None, None)
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
    .expect("product");
    for path in [
        "/api/v1/market-data/securities/us/aapl?brokerId=alpha",
        "/api/v1/market-data/snapshots/us/aapl?brokerId=alpha&refresh=true",
        "/api/v1/market-data/candles/us/aapl?brokerId=alpha&period=5m&limit=20",
        "/api/v1/market-data/depth/us/aapl?brokerId=alpha&num=12",
    ] {
        let response = http(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response.0, 409, "{path}: {response:?}");
        assert_eq!(
            response.1["error"]["code"],
            "MARKET_DATA_CAPABILITY_UNSUPPORTED"
        );
    }
    assert!(reader.calls.lock().unwrap().is_empty());
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/marketdata/routes_test.go:563 TestReadRoutesMapProviderAndRequestFailures
#[tokio::test]
async fn market_data_http_malformed_mutations_and_unavailable_snapshot_keep_owner_error_boundaries()
{
    let directory = tempdir().expect("directory");
    let state = state();
    let quote = Arc::new(ProductionMarketDataQuotePort::new(
        state.clone(),
        None,
        None,
        None,
    ));
    let actions = Arc::new(ObservedActions {
        owner: ProductionMarketDataProviderActionsPort::new(Some(quote.clone())),
        requests: Mutex::new(Vec::new()),
    });
    let router = Arc::new(Mutex::new(ProviderRouter::new(32)));
    let handle = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().unwrap(),
            directory.path().join("settings.json"),
        )
        .unwrap()
        .with_market_data_quote_read_snapshot_port(quote)
        .with_market_data_provider_actions_port(actions.clone())
        .with_market_data_subscription_mutation_port(Arc::new(
            ProductionMarketDataSubscriptionMutationPort::new(state, Some(router.clone()), None),
        )),
    )
    .await
    .expect("product");
    let address = handle.startup_record().address;
    for path in [
        "/api/v1/market-data/subscriptions/heartbeat",
        "/api/v1/market-data/instruments/normalize",
    ] {
        let failed = http(address, "POST", path, Some("bad")).await;
        assert_eq!(failed.0, 400);
        assert_eq!(failed.1["error"]["code"], "BAD_REQUEST");
    }
    assert!(actions.requests.lock().unwrap().is_empty());
    assert_eq!(router.lock().unwrap().demand().logical_count, 0);
    let absent = http(
        address,
        "GET",
        "/api/v1/market-data/snapshots/us/aapl",
        None,
    )
    .await;
    assert_eq!(absent.0, 503);
    assert_eq!(
        absent.1["error"]["code"],
        "MARKET_DATA_QUOTE_READ_UNAVAILABLE"
    );
    // Local normalization accepts a valid US symbol; there is no injected
    // provider normalization failure seam equivalent to the frozen fixture.
    let valid = http(
        address,
        "POST",
        "/api/v1/market-data/instruments/normalize",
        Some(r#"{"market":"us","symbol":"bad"}"#),
    )
    .await;
    assert_eq!(valid.0, 200);
    assert_eq!(valid.1["data"]["instrumentId"], "US.BAD");
    handle.shutdown().await.expect("shutdown");
}
