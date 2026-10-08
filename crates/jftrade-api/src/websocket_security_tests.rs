use super::*;
use crate::{
    AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, FixedClock, PortFuture, RouteSpec,
};
use std::collections::VecDeque;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

enum Reply {
    Pending,
    Failed,
    Json(Value),
}

#[derive(Default)]
struct ReadPort {
    requests: Mutex<Vec<ApiRequest>>,
    replies: Mutex<VecDeque<Reply>>,
    drops: Arc<AtomicUsize>,
    started: tokio::sync::Notify,
}

struct ReadGuard(Arc<AtomicUsize>);
impl Drop for ReadGuard {
    fn drop(&mut self) {
        self.0.fetch_add(1, Ordering::SeqCst);
    }
}

impl ApiPort for ReadPort {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async move {
            let _guard = ReadGuard(self.drops.clone());
            self.requests.lock().expect("requests").push(request);
            let reply = self
                .replies
                .lock()
                .expect("replies")
                .pop_front()
                .unwrap_or(Reply::Pending);
            self.started.notify_one();
            match reply {
                Reply::Pending => std::future::pending().await,
                Reply::Failed => Err(ApiFailure::new(503, "DETAILS_UNAVAILABLE", "offline")),
                Reply::Json(value) => Ok(ApiOutput::Json(value)),
            }
        })
    }
}

fn security(port: Arc<ReadPort>, route_enabled: bool) -> WebsocketSecurity {
    let routes = crate::RouteCatalog::new(if route_enabled {
        vec![RouteSpec {
            method: "GET".into(),
            path: "/api/v1/market-data/securities/{market}/{symbol}".into(),
        }]
    } else {
        Vec::new()
    })
    .expect("routes");
    let state = ApiState::new(routes, AccessPolicy::desktop(Some("a".repeat(32))), port)
        .with_clock(Arc::new(FixedClock("2026-06-14T00:00:00Z".into())));
    let mut headers = HeaderMap::new();
    headers.insert(
        "authorization",
        "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            .parse()
            .expect("token"),
    );
    headers.insert("origin", "http://localhost:3003".parse().expect("origin"));
    WebsocketSecurity::new(&state, &headers)
}

fn snapshot(provider: &str) -> LiveSubscriptionSnapshot {
    LiveSubscriptionSnapshot {
        provider_broker_id: provider.into(),
        security_details: vec![LiveSecuritySubscription {
            market: "US".into(),
            symbol: "AAPL".into(),
            instrument_id: "US.AAPL".into(),
        }],
        ..Default::default()
    }
}

fn payload(at: &str) -> Value {
    json!({"request":{"market":"US","symbol":"AAPL","instrumentId":"US.AAPL"},
        "security":{"name":"Apple"},"meta":{"resolvedAt":at}})
}

#[tokio::test]
async fn security_pending_reads_release_on_subscription_replacement_and_connection_owner_drop() {
    let port = Arc::new(ReadPort::default());
    let mut security = security(port.clone(), true);
    for expected in [0, 1] {
        security.replace(&snapshot("futu"));
        tokio::time::timeout(std::time::Duration::from_secs(1), async {
            tokio::select! { _=security.snapshots()=>panic!("read completed"), _=port.started.notified()=>{} }
        }).await.expect("read started");
        assert_eq!(port.drops.load(Ordering::SeqCst), expected);
        if expected == 0 {
            security.replace(&LiveSubscriptionSnapshot::default());
        }
    }
    drop(security);
    assert_eq!(port.drops.load(Ordering::SeqCst), 2);
    assert_eq!(port.requests.lock().expect("requests").len(), 2);
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:219 TestBackendReceivesExplicitBrokerSelection
#[tokio::test]
async fn security_read_forwards_explicit_broker_and_credentials_and_releases_timed_out_future() {
    let port = Arc::new(ReadPort::default());
    let mut security = security(port.clone(), true);
    security.replace(&snapshot("alpha"));
    let events = tokio::time::timeout(std::time::Duration::from_secs(3), security.snapshots())
        .await
        .expect("bounded read");
    assert!(events.is_empty());
    assert_eq!(port.drops.load(Ordering::SeqCst), 1);
    let requests = port.requests.lock().expect("requests");
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].path, "/api/v1/market-data/securities/US/AAPL");
    assert_eq!(requests[0].query, "brokerId=alpha");
    assert_eq!(requests[0].method, "GET");
    assert!(requests[0].desktop_trusted);
    assert!(requests[0].origin_provided);
    assert!(requests[0].origin_allowed);
    assert!(requests[0].body.is_empty());
    assert!(!requests[0].browser_authenticated);
    assert!(requests[0].session_cookie.is_none());
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59 TestDispatcherAuxiliarySubscriptionBranches
#[tokio::test]
async fn security_provider_error_is_skipped_and_a_new_subscription_retries_the_same_reader() {
    let port = Arc::new(ReadPort::default());
    port.replies
        .lock()
        .expect("replies")
        .extend([Reply::Failed, Reply::Json(payload("provider-time"))]);
    let mut security = security(port.clone(), true);
    security.replace(&snapshot("futu"));
    assert!(security.snapshots().await.is_empty());
    security.replace(&snapshot("futu"));
    let events = security.snapshots().await;
    assert_eq!(events.len(), 1);
    assert_eq!(events[0]["payload"]["security"]["name"], "Apple");
    assert_eq!(events[0]["payload"]["at"], "provider-time");
    assert_eq!(events[0]["serverTime"], "provider-time");
    assert_eq!(events[0]["source"], "market-data");
    assert_eq!(
        events[0]["eventId"],
        "market.security-details|US.AAPL|provider-time"
    );
    assert_eq!(port.requests.lock().expect("requests").len(), 2);
    assert_eq!(port.drops.load(Ordering::SeqCst), 2);
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59 TestDispatcherAuxiliarySubscriptionBranches
#[tokio::test]
async fn security_resolution_deduplication_does_not_rewind_pushes_and_resubscribe_forces_a_read() {
    let port = Arc::new(ReadPort::default());
    port.replies
        .lock()
        .expect("replies")
        .extend([Reply::Json(payload("old")), Reply::Json(payload("old"))]);
    let mut security = security(port.clone(), true);
    security.replace(&snapshot("futu"));
    let initial = security.snapshots().await;
    let mut push = initial[0].clone();
    push["payload"]["meta"]["resolvedAt"] = json!("new");
    assert_eq!(security.project(push.clone()), [push.clone()]);
    assert!(security.project(push.clone()).is_empty());
    assert!(security.filter_snapshots(initial).is_empty());
    let mut other = push.clone();
    other["entityId"] = json!("HK.00700");
    assert!(security.project(other).is_empty());
    let unrelated = json!({"type":"system.notification"});
    assert_eq!(security.project(unrelated.clone()), [unrelated]);
    security.replace(&snapshot("futu"));
    let forced = security.snapshots().await;
    assert_eq!(security.filter_snapshots(forced).len(), 1);
    security.replace(&LiveSubscriptionSnapshot::default());
    assert!(security.project(push).is_empty());
}

#[tokio::test]
async fn security_reads_skip_unregistered_routes_and_non_object_payloads_and_escape_provider_selection()
 {
    let port = Arc::new(ReadPort::default());
    let mut unavailable = security(port.clone(), false);
    unavailable.replace(&snapshot("futu"));
    assert!(unavailable.snapshots().await.is_empty());
    assert!(port.requests.lock().expect("requests").is_empty());
    port.replies
        .lock()
        .expect("replies")
        .extend([Reply::Json(json!(42)), Reply::Json(payload(""))]);
    let mut available = security(port.clone(), true);
    available.replace(&snapshot("alpha & beta"));
    assert!(available.snapshots().await.is_empty());
    assert_eq!(
        port.requests.lock().expect("requests")[0].query,
        "brokerId=alpha%20%26%20beta"
    );
    available.replace(&snapshot("alpha & beta"));
    let events = available.snapshots().await;
    assert_eq!(events.len(), 1);
    assert_eq!(events[0]["payload"]["at"], "2026-06-14T00:00:00Z");
    assert_eq!(events[0]["payload"]["brokerId"], "alpha & beta");
    assert_eq!(events[0]["entityId"], "US.AAPL");
    assert_eq!(
        events[0]["eventId"],
        "market.security-details|US.AAPL|2026-06-14T00:00:00Z"
    );
}
