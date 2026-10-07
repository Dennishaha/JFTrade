use super::*;
use crate::{AccessPolicy, FixedClock, PortFuture, RouteSpec};
use std::sync::Mutex;
use std::sync::atomic::{AtomicUsize, Ordering};

struct DropRead(Arc<AtomicUsize>);
impl Drop for DropRead {
    fn drop(&mut self) {
        self.0.fetch_add(1, Ordering::SeqCst);
    }
}

#[derive(Default)]
struct PendingPort {
    requests: Mutex<Vec<ApiRequest>>,
    drops: Arc<AtomicUsize>,
    started: tokio::sync::Notify,
}

impl ApiPort for PendingPort {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async move {
            self.requests.lock().expect("requests").push(request);
            let _read = DropRead(Arc::clone(&self.drops));
            self.started.notify_one();
            std::future::pending().await
        })
    }
}

fn depth(port: Arc<PendingPort>) -> WebsocketDepth {
    let routes = crate::RouteCatalog::new([RouteSpec {
        method: "GET".into(),
        path: "/api/v1/market-data/depth/{market}/{symbol}".into(),
    }])
    .expect("routes");
    let state = ApiState::new(routes, AccessPolicy::desktop(Some("a".repeat(32))), port)
        .with_clock(Arc::new(FixedClock("2026-06-14T00:00:00Z".into())));
    let mut headers = HeaderMap::new();
    headers.insert(
        "authorization",
        "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            .parse()
            .expect("header"),
    );
    WebsocketDepth::new(&state, &headers)
}

fn snapshot(provider: &str, nums: &[i64]) -> LiveSubscriptionSnapshot {
    LiveSubscriptionSnapshot {
        provider_broker_id: provider.into(),
        depth: nums
            .iter()
            .map(|num| LiveDepthSubscription {
                market: "US".into(),
                symbol: "TME".into(),
                instrument_id: "US.TME".into(),
                num: *num,
            })
            .collect(),
        ..Default::default()
    }
}

#[tokio::test]
async fn websocket_depth_pending_reads_release_on_subscription_replacement_and_owner_drop() {
    let port = Arc::new(PendingPort::default());
    let mut depth = depth(port.clone());
    for expected in [0, 1] {
        depth.replace(&snapshot("futu", &[50]));
        tokio::time::timeout(std::time::Duration::from_secs(1), async {
            tokio::select! { _=depth.snapshots()=>panic!("pending read completed"), _=port.started.notified()=>{} }
        }).await.expect("read started");
        assert_eq!(port.drops.load(Ordering::SeqCst), expected);
        if expected == 0 {
            depth.replace(&LiveSubscriptionSnapshot::default());
        }
    }
    drop(depth);
    assert_eq!(port.drops.load(Ordering::SeqCst), 2);
    assert_eq!(port.requests.lock().expect("requests").len(), 2);
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:219 TestBackendReceivesExplicitBrokerSelection
#[tokio::test]
async fn websocket_depth_read_forwards_explicit_broker_and_releases_timed_out_port_future() {
    let port = Arc::new(PendingPort::default());
    let mut depth = depth(port.clone());
    depth.replace(&snapshot("alpha", &[10]));
    let frames = tokio::time::timeout(std::time::Duration::from_secs(3), depth.snapshots())
        .await
        .expect("bounded read");
    assert!(frames.is_empty());
    assert_eq!(port.drops.load(Ordering::SeqCst), 1);
    let requests = port.requests.lock().expect("requests");
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].path, "/api/v1/market-data/depth/US/TME");
    assert_eq!(requests[0].query, "num=10&brokerId=alpha");
    assert_eq!(requests[0].method, "GET");
    assert!(requests[0].desktop_trusted);
    assert!(requests[0].body.is_empty());
}

fn push(at: &str) -> Value {
    json!({"type":"market.depth","source":"market-data","eventId":"original","entityId":"US.TME","serverTime":at,
        "payload":{"type":"market.depth","instrumentId":"US.TME","request":{"instrumentId":"US.TME","num":3},
            "meta":{"resolvedAt":at},"depth":{"bids":[{"price":100},{"price":99},{"price":98}],"asks":[{"price":101},{"price":102}]}}})
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59 TestDispatcherAuxiliarySubscriptionBranches
#[test]
fn websocket_depth_unchanged_resolution_is_deduplicated_per_requested_size_and_resubscribe_forces_snapshot()
 {
    let mut depth = depth(Arc::new(PendingPort::default()));
    depth.replace(&snapshot("futu", &[1, 50]));
    let event = push("2026-06-14T00:00:01Z");
    let original = event.clone();
    let frames = depth.project(event.clone());
    assert_eq!(frames.len(), 2);
    assert_eq!(
        frames[0]["payload"]["depth"]["bids"]
            .as_array()
            .expect("bids")
            .len(),
        1
    );
    assert_eq!(
        frames[1]["payload"]["depth"]["bids"]
            .as_array()
            .expect("bids")
            .len(),
        3
    );
    assert_eq!(
        frames[0]["payload"]["depth"]["asks"]
            .as_array()
            .expect("asks")
            .len(),
        1
    );
    assert_eq!(
        frames[1]["payload"]["depth"]["asks"]
            .as_array()
            .expect("asks")
            .len(),
        2
    );
    assert_ne!(frames[0]["eventId"], frames[1]["eventId"]);
    assert_eq!(event, original, "global event is not mutated");
    assert!(depth.project(event.clone()).is_empty());
    assert_eq!(depth.project(push("2026-06-14T00:00:02Z")).len(), 2);
    let delayed = depth_frame(
        push("2026-06-14T00:00:01Z"),
        &snapshot("futu", &[50]).depth[0],
    );
    assert!(
        depth.filter_snapshots(vec![delayed]).is_empty(),
        "delayed initial snapshot does not rewind a live push"
    );
    depth.replace(&snapshot("futu", &[50]));
    assert_eq!(depth.project(event).len(), 1);
    depth.replace(&LiveSubscriptionSnapshot {
        active_instruments: vec!["US.TME".into()],
        ..Default::default()
    });
    assert!(
        depth.project(push("2026-06-14T00:00:03Z")).is_empty(),
        "quote subscription does not receive depth"
    );
    assert_eq!(query_component("alpha&num=50"), "alpha%26num%3D50");
}
