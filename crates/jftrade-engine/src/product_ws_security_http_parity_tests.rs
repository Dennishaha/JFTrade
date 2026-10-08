//! Security details through production subscription admission and authenticated sockets.
use super::*;
use jftrade_integration_futu::SecuritySnapshotReadPort;
use jftrade_marketdata::BrokerSecuritySnapshot;
use serde_json::{Value, json};
use std::sync::Mutex;
use std::sync::atomic::{AtomicUsize, Ordering};

#[derive(Debug, Default)]
struct PendingDetails {
    started: tokio::sync::Notify,
    dropped: Arc<AtomicUsize>,
}

struct PendingReadDrop(Arc<AtomicUsize>);
impl Drop for PendingReadDrop {
    fn drop(&mut self) {
        self.0.fetch_add(1, Ordering::SeqCst);
    }
}

impl MarketDataQuoteReadSnapshotPort for PendingDetails {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        Box::pin(async move {
            assert_eq!(path, "/api/v1/market-data/securities/HK/00700");
            assert_eq!(query, "brokerId=futu");
            let _guard = PendingReadDrop(self.dropped.clone());
            self.started.notify_one();
            std::future::pending().await
        })
    }
}

#[tokio::test]
async fn websocket_listener_shutdown_cancels_pending_security_read_and_releases_connection_demand()
{
    let directory = tempdir().expect("directory");
    let pending = Arc::new(PendingDetails::default());
    // This fixture isolates the real listener cancellation boundary. The
    // concrete production reader is exercised by the tests below.
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        directory.path().join("settings.json"),
    )
    .expect("config")
    .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort))
    .with_market_data_quote_read_snapshot_port(pending.clone());
    let handle = start_product(config).await.expect("product");
    let hub = handle.live_hub();
    let mut socket = connect(handle.startup_record().address).await;
    subscribe(&mut socket, "futu", "HK.00700").await;
    tokio::time::timeout(
        std::time::Duration::from_secs(1),
        pending.started.notified(),
    )
    .await
    .expect("pending reader entered");
    assert_eq!(pending.dropped.load(Ordering::SeqCst), 0);
    let shutdown = tokio::spawn(async move { handle.shutdown().await });
    let close = tokio::time::timeout(
        std::time::Duration::from_secs(1),
        read_server_close_frame(&mut socket),
    )
    .await
    .expect("close while details reader is pending");
    tokio::time::timeout(std::time::Duration::from_secs(3), shutdown)
        .await
        .expect("bounded shutdown")
        .expect("shutdown join")
        .expect("shutdown");
    assert_eq!(close, (1001, "server shutting down".into()));
    assert_eq!(pending.dropped.load(Ordering::SeqCst), 1);
    assert_eq!(hub.snapshot().connected, 0);
    assert!(hub.snapshot().active_instruments.is_empty());
}

#[derive(Debug, Default)]
struct DetailsReader {
    requests: Mutex<Vec<Vec<String>>>,
}

impl SecuritySnapshotReadPort for DetailsReader {
    fn query(&self, instruments: &[String]) -> Result<Vec<BrokerSecuritySnapshot>, String> {
        self.requests
            .lock()
            .expect("requests")
            .push(instruments.to_vec());
        Ok(instruments
            .iter()
            .map(|instrument| {
                let (market, symbol) = instrument.split_once('.').expect("identity");
                BrokerSecuritySnapshot {
                    symbol: Some(format!("{market}.{symbol}")),
                    market: Some(market.into()),
                    name: Some(
                        if instrument == "HK.00700" {
                            "Tencent Holdings"
                        } else {
                            "Apple"
                        }
                        .into(),
                    ),
                    security_type: Some("EQUITY".into()),
                    last_price: Some("321.4".parse().expect("price")),
                    ..Default::default()
                }
            })
            .collect())
    }
}

fn details_config(directory: &tempfile::TempDir, reader: Arc<DetailsReader>) -> ProductConfig {
    let runtime = Arc::new(product_production_ports::SharedTradeReadRuntime::default());
    runtime.set_security_snapshots(Some(reader));
    config(directory)
        .with_trade_runtime(runtime)
        .with_active_provider_state(Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Futu,
        ))))
        .with_market_data_router(Arc::new(Mutex::new(
            jftrade_marketdata::ProviderRouter::new(32),
        )))
}

async fn subscribe(socket: &mut TcpStream, provider: &str, instrument: &str) {
    let (market, symbol) = instrument.split_once('.').expect("identity");
    let message = json!({"type":"subscribe","subscriptions":{"providerBrokerId":provider,
        "securityDetails":[{"market":market,"symbol":symbol,"instrumentId":instrument}]}});
    socket
        .write_all(&masked_text_frame(message.to_string().as_bytes()))
        .await
        .expect("subscribe");
}

async fn next_details(
    socket: &mut TcpStream,
    deadline: std::time::Duration,
) -> Result<Value, tokio::time::error::Elapsed> {
    tokio::time::timeout(deadline, async {
        loop {
            let event: Value =
                serde_json::from_str(&read_server_text_frame(socket).await).expect("wire JSON");
            if event["type"] == "market.security-details" {
                return event;
            }
            assert_eq!(event["type"], "heartbeat", "unexpected {event}");
        }
    })
    .await
}

fn assert_details(event: &Value, instrument: &str, name: &str) {
    assert_eq!(event["type"], "market.security-details");
    assert_eq!(event["source"], "market-data");
    assert_eq!(event["entityId"], instrument);
    assert!(!event["eventId"].as_str().expect("event id").is_empty());
    assert!(
        !event["serverTime"]
            .as_str()
            .expect("server time")
            .is_empty()
    );
    assert_eq!(event["payload"]["type"], "market.security-details");
    assert_eq!(event["payload"]["brokerId"], "futu");
    assert_eq!(event["payload"]["request"]["instrumentId"], instrument);
    assert_eq!(event["payload"]["security"]["name"], name);
}

// Parity: go:452dea11:internal/app/apiserver/servercore/market_details_ws_test.go:11 TestMarketSecurityDetailsWebSocketSendsInitialPayload
#[tokio::test]
async fn production_websocket_security_subscription_sends_provider_details_as_initial_frame() {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(DetailsReader::default());
    let handle = start_product(details_config(&directory, reader.clone()))
        .await
        .expect("production");
    let address = handle.startup_record().address;
    let mut socket = connect(address).await;
    subscribe(&mut socket, "futu", "HK.00700").await;
    let initial = next_details(&mut socket, std::time::Duration::from_secs(2)).await;
    drop(socket);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
    assert_details(
        &initial.expect("initial security frame"),
        "HK.00700",
        "Tencent Holdings",
    );
    assert_eq!(
        *reader.requests.lock().expect("requests"),
        [vec!["HK.00700".to_owned()]]
    );
}

#[tokio::test]
async fn production_websocket_security_wrong_provider_cannot_read_active_snapshot_and_resubscribe_recovers()
 {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(DetailsReader::default());
    let handle = start_product(details_config(&directory, reader.clone()))
        .await
        .expect("production");
    let address = handle.startup_record().address;
    let mut socket = connect(address).await;
    subscribe(&mut socket, "ibkr", "HK.00700").await;
    wait_for_live_projection_with_headers(address, 1, &["HK.00700"], AUTH).await;
    assert!(
        next_details(&mut socket, std::time::Duration::from_millis(100))
            .await
            .is_err()
    );
    assert!(reader.requests.lock().expect("requests").is_empty());
    subscribe(&mut socket, "futu", "HK.00700").await;
    let recovered = next_details(&mut socket, std::time::Duration::from_secs(2)).await;
    drop(socket);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
    assert_details(
        &recovered.expect("recovered frame"),
        "HK.00700",
        "Tencent Holdings",
    );
    assert_eq!(reader.requests.lock().expect("requests").len(), 1);
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59 TestDispatcherAuxiliarySubscriptionBranches
#[tokio::test]
async fn production_websocket_security_updates_are_isolated_and_unchanged_resolution_is_deduplicated()
 {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(DetailsReader::default());
    let handle = start_product(details_config(&directory, reader.clone()))
        .await
        .expect("production");
    let address = handle.startup_record().address;
    let mut hong_kong = connect(address).await;
    let mut united_states = connect(address).await;
    subscribe(&mut hong_kong, "futu", "HK.00700").await;
    subscribe(&mut united_states, "futu", "US.AAPL").await;
    let first = next_details(&mut hong_kong, std::time::Duration::from_secs(2)).await;
    let second = next_details(&mut united_states, std::time::Duration::from_secs(2)).await;
    if first.is_ok() && second.is_ok() {
        let event = json!({"type":"market.security-details","source":"market-data","entityId":"HK.00700",
            "eventId":"details|HK.00700|revision","serverTime":"2026-06-14T00:00:01Z",
            "payload":{"type":"market.security-details","brokerId":"futu","request":{"instrumentId":"HK.00700"},
                "security":{"name":"Tencent revision"},"meta":{"resolvedAt":"2026-06-14T00:00:01Z"}}});
        assert!(handle.live_hub().publish(event.clone()));
        assert_details(
            &next_details(&mut hong_kong, std::time::Duration::from_secs(2))
                .await
                .expect("updated"),
            "HK.00700",
            "Tencent revision",
        );
        assert!(
            next_details(&mut united_states, std::time::Duration::from_millis(100))
                .await
                .is_err()
        );
        assert!(handle.live_hub().publish(event));
        assert!(
            next_details(&mut hong_kong, std::time::Duration::from_millis(100))
                .await
                .is_err()
        );
        subscribe(&mut hong_kong, "futu", "HK.00700").await;
        assert_details(
            &next_details(&mut hong_kong, std::time::Duration::from_secs(2))
                .await
                .expect("forced"),
            "HK.00700",
            "Tencent Holdings",
        );
    }
    drop(hong_kong);
    drop(united_states);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
    assert_details(&first.expect("HK initial"), "HK.00700", "Tencent Holdings");
    assert_details(&second.expect("US initial"), "US.AAPL", "Apple");
    assert_eq!(reader.requests.lock().expect("requests").len(), 3);
}
