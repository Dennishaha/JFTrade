//! Initial depth reads and per-client OpenD push projection over authenticated sockets.
use super::*;
use jftrade_integration_futu::{
    MarketMicrostructureError, MarketMicrostructureOperation, MarketMicrostructureReadPort,
    OpenDSessionCoordinatorOutcome, OpenDSessionEventListener, OrderBookLevel, OrderBookPush,
    QuotePush, Security,
};
use serde_json::{Value, json};
use std::sync::Mutex;

#[derive(Debug, Default)]
struct DepthReader {
    requests: Mutex<Vec<(MarketMicrostructureOperation, String, Value)>>,
    failed: std::sync::atomic::AtomicBool,
    read: tokio::sync::Notify,
}

impl MarketMicrostructureReadPort for DepthReader {
    fn query(
        &self,
        operation: MarketMicrostructureOperation,
        instrument: &str,
        params: &Value,
    ) -> Result<Value, MarketMicrostructureError> {
        self.requests.lock().expect("requests").push((
            operation,
            instrument.into(),
            params.clone(),
        ));
        self.read.notify_one();
        if self.failed.load(std::sync::atomic::Ordering::SeqCst) {
            return Err(MarketMicrostructureError::Session(
                "depth unavailable".into(),
            ));
        }
        let (market, symbol) = instrument.split_once('.').expect("instrument");
        let bids: Vec<_> = (0..params["num"].as_u64().expect("num").min(3))
            .map(|index| json!({"price":(100-index).to_string()}))
            .collect();
        Ok(
            json!({"request":{"market":market,"symbol":symbol,"instrumentId":instrument,"num":params["num"]},
            "depth":{"bids":bids},"meta":{"resolvedAt":"2026-06-14T00:00:01Z"}}),
        )
    }
}

fn depth_config(directory: &tempfile::TempDir, reader: Arc<DepthReader>) -> ProductConfig {
    let runtime = Arc::new(product_production_ports::SharedTradeReadRuntime::default());
    runtime.set_market_microstructure(Some(reader));
    config(directory)
        .with_trade_runtime(runtime)
        .with_active_provider_state(Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Futu,
        ))))
        .with_market_data_router(Arc::new(Mutex::new(
            jftrade_marketdata::ProviderRouter::new(32),
        )))
}

async fn subscribe(socket: &mut TcpStream, nums: &[i64]) {
    let depth: Vec<_> = nums
        .iter()
        .map(|num| json!({"market":"us","symbol":"tme","instrumentId":"US.TME","num":num}))
        .collect();
    let message =
        json!({"type":"subscribe","subscriptions":{"providerBrokerId":"futu","depth":depth}});
    socket
        .write_all(&masked_text_frame(message.to_string().as_bytes()))
        .await
        .expect("subscribe");
}

async fn next_depth(socket: &mut TcpStream) -> Value {
    tokio::time::timeout(std::time::Duration::from_secs(2), async {
        loop {
            let event: Value =
                serde_json::from_str(&read_server_text_frame(socket).await).expect("wire JSON");
            if event["type"] == "market.depth" {
                return event;
            }
            assert_eq!(event["type"], "heartbeat", "unexpected {event}");
        }
    })
    .await
    .expect("depth deadline")
}

fn push(hub: Arc<jftrade_api::LiveHub>, symbol: &str, at: &str, levels: usize) {
    let listener = crate::product_runtime::LiveHubOpenDEventListener::with_reconciliation_wake(
        hub,
        Arc::new(tokio::sync::Notify::new()),
    );
    listener.on_event(&OpenDSessionCoordinatorOutcome::Push(QuotePush::OrderBook(
        OrderBookPush {
            security: Some(Security {
                market: Some(11),
                code: Some(symbol.into()),
            }),
            name: None,
            bids: (0..levels)
                .map(|index| OrderBookLevel {
                    price: Some(101.0 - index as f64),
                    volume: Some(150),
                    order_count: Some(2),
                    details: Vec::new(),
                    high_precision_volume: None,
                })
                .collect(),
            asks: Vec::new(),
            server_receive_time_bid: Some(at.into()),
            server_receive_time_bid_timestamp: None,
            server_receive_time_ask: None,
            server_receive_time_ask_timestamp: None,
            order_book_type: None,
        },
    )));
}

fn assert_depth(event: &Value, num: i64, at: &str) {
    assert_eq!(event["type"], "market.depth");
    assert_eq!(event["source"], "market-data");
    assert_eq!(event["entityId"], format!("US.TME|{num}"));
    for field in ["eventId", "serverTime"] {
        assert!(!event[field].as_str().expect(field).is_empty());
    }
    assert_eq!(event["payload"]["type"], "market.depth");
    assert_eq!(event["payload"]["request"]["instrumentId"], "US.TME");
    assert_eq!(event["payload"]["request"]["num"], num);
    assert_eq!(event["payload"]["meta"]["resolvedAt"], at);
}

// Parity: go:452dea11:internal/api/live/handler_test.go:244 TestHandlerDepthUpdatePublishesFreshPayload
#[tokio::test]
async fn production_websocket_depth_reads_initial_snapshot_then_projects_fresh_opend_push() {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(DepthReader::default());
    let handle = start_product(depth_config(&directory, reader.clone()))
        .await
        .expect("production");
    let address = handle.startup_record().address;
    let mut socket = connect(address).await;
    subscribe(&mut socket, &[50]).await;
    let initial = next_depth(&mut socket).await;
    assert_depth(&initial, 50, "2026-06-14T00:00:01Z");
    assert_eq!(initial["payload"]["depth"]["bids"][0]["price"], "100");
    assert_eq!(
        *reader.requests.lock().expect("requests"),
        [(
            MarketMicrostructureOperation::Depth,
            "US.TME".into(),
            json!({"num":50})
        )]
    );
    push(handle.live_hub(), "TME", "2026-06-14T00:00:02Z", 1);
    let fresh = next_depth(&mut socket).await;
    assert_depth(&fresh, 50, "2026-06-14T00:00:02Z");
    assert_eq!(fresh["payload"]["depth"]["bids"][0]["price"], 101.0);
    assert_eq!(fresh["payload"]["depth"]["bids"][0]["volume"], 150.0);
    assert_eq!(fresh["payload"]["depth"]["bids"][0]["orderCount"], 2);
    drop(socket);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    assert_eq!(handle.live_hub().snapshot().connected, 0);
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:295 TestDepthUpdateSubscriptionFiltersAndCoalesces
#[tokio::test]
async fn production_websocket_depth_filters_symbols_and_preserves_each_clients_requested_size() {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(DepthReader::default());
    let handle = start_product(depth_config(&directory, reader))
        .await
        .expect("production");
    let address = handle.startup_record().address;
    let mut one = connect(address).await;
    let mut both = connect(address).await;
    subscribe(&mut one, &[1]).await;
    subscribe(&mut both, &[1, 50]).await;
    assert_depth(&next_depth(&mut one).await, 1, "2026-06-14T00:00:01Z");
    for num in [1, 50] {
        let initial = next_depth(&mut both).await;
        assert_depth(&initial, num, "2026-06-14T00:00:01Z");
        assert_eq!(
            initial["payload"]["depth"]["bids"]
                .as_array()
                .expect("bids")
                .len(),
            if num == 1 { 1 } else { 3 }
        );
    }
    push(handle.live_hub(), "MSFT", "2026-06-14T00:00:02Z", 3);
    push(handle.live_hub(), "TME", "2026-06-14T00:00:02Z", 3);
    let small = next_depth(&mut one).await;
    assert_depth(&small, 1, "2026-06-14T00:00:02Z");
    assert_eq!(
        small["payload"]["depth"]["bids"]
            .as_array()
            .expect("bids")
            .len(),
        1
    );
    for (num, count) in [(1, 1), (50, 3)] {
        let event = next_depth(&mut both).await;
        assert_depth(&event, num, "2026-06-14T00:00:02Z");
        assert_eq!(
            event["payload"]["depth"]["bids"]
                .as_array()
                .expect("bids")
                .len(),
            count
        );
    }
    drop(one);
    drop(both);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59 TestDispatcherAuxiliarySubscriptionBranches
#[tokio::test]
async fn production_websocket_depth_provider_failure_keeps_session_available_for_retry() {
    let directory = tempdir().expect("directory");
    let reader = Arc::new(DepthReader::default());
    reader
        .failed
        .store(true, std::sync::atomic::Ordering::SeqCst);
    let handle = start_product(depth_config(&directory, reader.clone()))
        .await
        .expect("production");
    let address = handle.startup_record().address;
    let mut socket = connect(address).await;
    subscribe(&mut socket, &[10]).await;
    tokio::time::timeout(std::time::Duration::from_secs(2), reader.read.notified())
        .await
        .expect("actual failed owner read");
    assert!(handle.live_hub().publish(event(1)));
    assert_eq!(notification(&mut socket).await, event(1));
    reader
        .failed
        .store(false, std::sync::atomic::Ordering::SeqCst);
    subscribe(&mut socket, &[10]).await;
    assert_depth(&next_depth(&mut socket).await, 10, "2026-06-14T00:00:01Z");
    assert_eq!(reader.requests.lock().expect("requests").len(), 2);
    drop(socket);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
}

#[derive(Debug, Default)]
struct PendingQuotes {
    started: tokio::sync::Notify,
    dropped: Arc<std::sync::atomic::AtomicUsize>,
}

struct PendingReadDrop(Arc<std::sync::atomic::AtomicUsize>);
impl Drop for PendingReadDrop {
    fn drop(&mut self) {
        self.0.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
    }
}

impl MarketDataQuoteReadSnapshotPort for PendingQuotes {
    fn read<'a>(&'a self, _path: &'a str, _query: &'a str) -> MarketDataQuoteReadFuture<'a> {
        Box::pin(async move {
            let _guard = PendingReadDrop(Arc::clone(&self.dropped));
            self.started.notify_one();
            std::future::pending().await
        })
    }
}

#[tokio::test]
async fn websocket_listener_shutdown_cancels_pending_initial_depth_read_and_releases_demand() {
    let directory = tempdir().expect("directory");
    let pending = Arc::new(PendingQuotes::default());
    // Transport cancellation seam; production runtime resource injection is
    // independently exercised by the three concrete owner tests above.
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
    subscribe(&mut socket, &[50]).await;
    tokio::time::timeout(
        std::time::Duration::from_secs(1),
        pending.started.notified(),
    )
    .await
    .expect("pending port entered");
    assert_eq!(pending.dropped.load(std::sync::atomic::Ordering::SeqCst), 0);
    let shutdown = tokio::spawn(async move { handle.shutdown().await });
    let close = tokio::time::timeout(
        std::time::Duration::from_secs(1),
        read_server_close_frame(&mut socket),
    )
    .await
    .expect("close while port is pending");
    let result = shutdown.await.expect("join shutdown");
    result.expect("shutdown");
    assert_eq!(close.0, 1001);
    assert_eq!(pending.dropped.load(std::sync::atomic::Ordering::SeqCst), 1);
    assert_eq!(hub.snapshot().connected, 0);
    assert!(hub.snapshot().active_instruments.is_empty());
}
