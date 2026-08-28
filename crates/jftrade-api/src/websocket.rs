use std::collections::{BTreeMap, BTreeSet};
use std::sync::Arc;
use std::sync::RwLock;
use std::sync::atomic::{AtomicUsize, Ordering};

use axum::http::HeaderMap;
use serde_json::Value;
use tokio::sync::broadcast;

use crate::AccessPolicy;
use crate::auth::{origin_provided, request_origin};

pub const DEFAULT_WEBSOCKET_LIMIT: usize = 20;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LiveConnectionSnapshot {
    pub connected: usize,
    pub limit: usize,
    pub at_limit: bool,
    pub active_instruments: Vec<String>,
}

#[derive(Debug)]
pub struct LiveConnectionMetrics {
    limit: usize,
    connected: AtomicUsize,
    next_client_id: AtomicUsize,
    active_instruments: RwLock<BTreeMap<usize, Vec<String>>>,
}

impl Default for LiveConnectionMetrics {
    fn default() -> Self {
        Self::new(DEFAULT_WEBSOCKET_LIMIT)
    }
}

impl LiveConnectionMetrics {
    pub fn new(limit: usize) -> Self {
        Self {
            limit: limit.max(1),
            connected: AtomicUsize::new(0),
            next_client_id: AtomicUsize::new(0),
            active_instruments: RwLock::new(BTreeMap::new()),
        }
    }

    pub fn snapshot(&self) -> LiveConnectionSnapshot {
        let connected = self.connected.load(Ordering::Acquire);
        let active_instruments = self
            .active_instruments
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .values()
            .flatten()
            .cloned()
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect();
        LiveConnectionSnapshot {
            connected,
            limit: self.limit,
            at_limit: connected >= self.limit,
            active_instruments,
        }
    }

    pub fn try_acquire(self: &Arc<Self>) -> Option<LiveConnectionPermit> {
        self.connected
            .fetch_update(Ordering::AcqRel, Ordering::Acquire, |current| {
                (current < self.limit).then_some(current + 1)
            })
            .ok()
            .map(|_| {
                let client_id = self.next_client_id.fetch_add(1, Ordering::Relaxed) + 1;
                self.active_instruments
                    .write()
                    .unwrap_or_else(std::sync::PoisonError::into_inner)
                    .insert(client_id, Vec::new());
                LiveConnectionPermit {
                    client_id,
                    metrics: Arc::clone(self),
                }
            })
    }
}

#[derive(Debug)]
pub struct LiveConnectionPermit {
    client_id: usize,
    metrics: Arc<LiveConnectionMetrics>,
}

impl LiveConnectionPermit {
    pub fn set_active_instruments(&self, values: &[String]) {
        let normalized = values
            .iter()
            .map(|value| value.trim().to_uppercase())
            .filter(|value| !value.is_empty())
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect();
        self.metrics
            .active_instruments
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(self.client_id, normalized);
    }
}

impl Drop for LiveConnectionPermit {
    fn drop(&mut self) {
        self.metrics
            .active_instruments
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(&self.client_id);
        self.metrics.connected.fetch_sub(1, Ordering::AcqRel);
    }
}

pub fn websocket_origin_allowed(headers: &HeaderMap, policy: &AccessPolicy) -> bool {
    if !origin_provided(headers) {
        return true;
    }
    request_origin(headers).is_some_and(|origin| policy.allowed_origins.contains(&origin))
}

pub trait LiveDemandListener: Send + Sync + std::fmt::Debug {
    fn on_subscription_change(
        &self,
        connection_id: u64,
        provider_broker_id: &str,
        instruments: &[String],
    );
    fn on_disconnect(&self, connection_id: u64);
}

/// Rust-owned live event hub.  Provider runtimes publish wire-shaped events
/// here and each websocket connection receives only events matching its
/// active instrument subscription.  The hub deliberately does not synthesize
/// quote data; a missing provider therefore remains a stale/error state.
#[derive(Debug)]
pub struct LiveHub {
    sender: broadcast::Sender<Value>,
    subscriptions: Arc<RwLock<BTreeMap<u64, LiveSubscription>>>,
    next_connection_id: AtomicUsize,
    demand_listener: RwLock<Option<Arc<dyn LiveDemandListener>>>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
struct LiveSubscription {
    provider_broker_id: String,
    active_instruments: BTreeSet<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LiveHubSnapshot {
    pub connected: usize,
    pub active_instruments: Vec<String>,
}

/// A registered websocket's view of the shared hub.  Dropping it releases
/// the subscription even when the socket closes without a close frame.
pub struct LiveHubConnection {
    id: u64,
    hub: Arc<LiveHub>,
    receiver: broadcast::Receiver<Value>,
}

impl std::fmt::Debug for LiveHubConnection {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("LiveHubConnection")
            .field("id", &self.id)
            .finish_non_exhaustive()
    }
}

impl Default for LiveHub {
    fn default() -> Self {
        Self::new(256)
    }
}

impl LiveHub {
    pub fn new(capacity: usize) -> Self {
        let (sender, _) = broadcast::channel(capacity.max(1));
        Self {
            sender,
            subscriptions: Arc::new(RwLock::new(BTreeMap::new())),
            next_connection_id: AtomicUsize::new(0),
            demand_listener: RwLock::new(None),
        }
    }

    pub fn set_demand_listener(&self, listener: Arc<dyn LiveDemandListener>) {
        *self
            .demand_listener
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner) = Some(listener);
    }

    pub fn connect(self: &Arc<Self>) -> LiveHubConnection {
        let id = self.next_connection_id.fetch_add(1, Ordering::Relaxed) as u64 + 1;
        self.subscriptions
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(id, LiveSubscription::default());
        LiveHubConnection {
            id,
            hub: Arc::clone(self),
            receiver: self.sender.subscribe(),
        }
    }

    /// Publish an already encoded live event.  Returns false when no client
    /// is currently subscribed; callers may use that to avoid unnecessary
    /// provider work without changing the wire contract.
    pub fn publish(&self, event: Value) -> bool {
        self.sender.send(event).is_ok()
    }

    pub fn snapshot(&self) -> LiveHubSnapshot {
        let subscriptions = self
            .subscriptions
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let active_instruments = subscriptions
            .values()
            .flat_map(|subscription| subscription.active_instruments.iter().cloned())
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect();
        LiveHubSnapshot {
            connected: subscriptions.len(),
            active_instruments,
        }
    }

    fn update_subscription(&self, id: u64, provider_broker_id: &str, instruments: &[String]) {
        let normalized = instruments
            .iter()
            .map(|instrument| instrument.trim().to_ascii_uppercase())
            .filter(|instrument| !instrument.is_empty())
            .collect::<BTreeSet<_>>();
        if let Some(subscription) = self
            .subscriptions
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .get_mut(&id)
        {
            subscription.provider_broker_id = provider_broker_id.trim().to_owned();
            subscription.active_instruments = normalized;
        }
        if let Some(listener) = self
            .demand_listener
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .as_ref()
        {
            listener.on_subscription_change(id, provider_broker_id, instruments);
        }
    }

    fn event_matches(&self, id: u64, event: &Value) -> bool {
        let subscriptions = self
            .subscriptions
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let Some(subscription) = subscriptions.get(&id) else {
            return false;
        };
        if let Some(provider_broker_id) = event_provider_broker_id(event)
            && !subscription.provider_broker_id.is_empty()
            && subscription.provider_broker_id.to_ascii_lowercase() != provider_broker_id
        {
            return false;
        }
        let instrument = event_instrument_id(event);
        match instrument {
            Some(instrument) => subscription.active_instruments.contains(&instrument),
            // Heartbeats, stale/error and notification events are not tied to
            // one instrument and must remain visible to every authenticated
            // client.
            None => true,
        }
    }

    fn disconnect(&self, id: u64) {
        self.subscriptions
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(&id);
        if let Some(listener) = self
            .demand_listener
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .as_ref()
        {
            listener.on_disconnect(id);
        }
    }
}

impl LiveHubConnection {
    pub fn set_subscription(&self, provider_broker_id: &str, instruments: &[String]) {
        self.hub
            .update_subscription(self.id, provider_broker_id, instruments);
    }

    pub async fn recv(&mut self) -> Option<Value> {
        loop {
            let event = match self.receiver.recv().await {
                Ok(event) => event,
                Err(broadcast::error::RecvError::Lagged(_)) => continue,
                Err(broadcast::error::RecvError::Closed) => return None,
            };
            if self.hub.event_matches(self.id, &event) {
                return Some(event);
            }
        }
    }
}

impl Drop for LiveHubConnection {
    fn drop(&mut self) {
        self.hub.disconnect(self.id);
    }
}

fn event_instrument_id(event: &Value) -> Option<String> {
    let candidate = event
        .pointer("/payload/instrumentId")
        .or_else(|| event.pointer("/payload/instrument"))
        .or_else(|| event.pointer("/payload/symbol"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    Some(candidate.to_ascii_uppercase())
}

fn event_provider_broker_id(event: &Value) -> Option<String> {
    event
        .pointer("/payload/providerBrokerId")
        .or_else(|| event.pointer("/payload/brokerId"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_ascii_lowercase)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn live_connection_snapshot_tracks_concurrency_and_limit() {
        let metrics = Arc::new(LiveConnectionMetrics::new(2));
        assert_eq!(
            metrics.snapshot(),
            LiveConnectionSnapshot {
                connected: 0,
                limit: 2,
                at_limit: false,
                active_instruments: Vec::new(),
            }
        );

        let first = metrics.try_acquire().expect("first connection");
        assert_eq!(metrics.snapshot().connected, 1);
        assert!(!metrics.snapshot().at_limit);

        let second = metrics.try_acquire().expect("second connection");
        assert_eq!(
            metrics.snapshot(),
            LiveConnectionSnapshot {
                connected: 2,
                limit: 2,
                at_limit: true,
                active_instruments: Vec::new(),
            }
        );
        assert!(metrics.try_acquire().is_none());

        drop(first);
        assert_eq!(metrics.snapshot().connected, 1);
        assert!(!metrics.snapshot().at_limit);
        drop(second);
        assert_eq!(metrics.snapshot().connected, 0);
    }

    #[test]
    fn live_connection_limit_is_never_zero() {
        let metrics = Arc::new(LiveConnectionMetrics::new(0));
        let permit = metrics.try_acquire().expect("effective first connection");
        assert_eq!(metrics.snapshot().limit, 1);
        assert!(metrics.snapshot().at_limit);
        assert!(metrics.try_acquire().is_none());
        drop(permit);
    }

    #[test]
    fn active_instruments_are_normalized_unioned_replaced_and_released() {
        let metrics = Arc::new(LiveConnectionMetrics::new(2));
        let first = metrics.try_acquire().expect("first connection");
        let second = metrics.try_acquire().expect("second connection");
        first.set_active_instruments(&[
            " us.aapl ".to_owned(),
            "HK.00700".to_owned(),
            "US.AAPL".to_owned(),
        ]);
        second.set_active_instruments(&[" cn.600000 ".to_owned(), "us.aapl".to_owned()]);
        assert_eq!(
            metrics.snapshot().active_instruments,
            ["CN.600000", "HK.00700", "US.AAPL"]
        );

        first.set_active_instruments(&["US.MSFT".to_owned()]);
        assert_eq!(
            metrics.snapshot().active_instruments,
            ["CN.600000", "US.AAPL", "US.MSFT"]
        );
        drop(second);
        assert_eq!(metrics.snapshot().active_instruments, ["US.MSFT"]);
        drop(first);
        assert!(metrics.snapshot().active_instruments.is_empty());
    }

    #[tokio::test]
    async fn live_hub_filters_instrument_events_and_releases_subscriptions() {
        let hub = Arc::new(LiveHub::new(8));
        let mut connection = hub.connect();
        assert_eq!(hub.snapshot().connected, 1);
        connection.set_subscription(" futu ", &[" us.aapl ".to_owned()]);
        assert_eq!(
            hub.snapshot().active_instruments,
            vec!["US.AAPL".to_owned()]
        );

        assert!(hub.publish(serde_json::json!({
            "type": "tick",
            "payload": {"instrumentId": "US.MSFT", "price": 1}
        })));
        assert!(hub.publish(serde_json::json!({
            "type": "tick",
            "payload": {"instrumentId": "US.AAPL", "price": 2}
        })));
        assert_eq!(
            connection.recv().await,
            Some(serde_json::json!({
                "type": "tick",
                "payload": {"instrumentId": "US.AAPL", "price": 2}
            }))
        );

        assert!(hub.publish(serde_json::json!({
            "type": "stale",
            "payload": {"reason": "provider unavailable"}
        })));
        assert_eq!(
            connection.recv().await,
            Some(serde_json::json!({
                "type": "stale",
                "payload": {"reason": "provider unavailable"}
            }))
        );
        drop(connection);
        assert_eq!(
            hub.snapshot(),
            LiveHubSnapshot {
                connected: 0,
                active_instruments: Vec::new(),
            }
        );
    }

    #[test]
    fn live_hub_demand_listener_tracks_subscriptions_and_disconnect() {
        #[derive(Debug, Eq, PartialEq)]
        struct SubscriptionEvent {
            connection_id: u64,
            provider_broker_id: String,
            instruments: Vec<String>,
        }

        #[derive(Debug, Default)]
        struct TestListener {
            changes: Arc<RwLock<Vec<SubscriptionEvent>>>,
            disconnects: Arc<RwLock<Vec<u64>>>,
        }

        impl LiveDemandListener for TestListener {
            fn on_subscription_change(
                &self,
                connection_id: u64,
                provider_broker_id: &str,
                instruments: &[String],
            ) {
                self.changes.write().unwrap().push(SubscriptionEvent {
                    connection_id,
                    provider_broker_id: provider_broker_id.to_owned(),
                    instruments: instruments.to_vec(),
                });
            }

            fn on_disconnect(&self, connection_id: u64) {
                self.disconnects.write().unwrap().push(connection_id);
            }
        }

        let changes = Arc::new(RwLock::new(Vec::new()));
        let disconnects = Arc::new(RwLock::new(Vec::new()));
        let listener = Arc::new(TestListener {
            changes: Arc::clone(&changes),
            disconnects: Arc::clone(&disconnects),
        });

        let hub = Arc::new(LiveHub::new(8));
        hub.set_demand_listener(listener);

        let connection = hub.connect();
        connection.set_subscription("futu", &["US.AAPL".to_owned()]);

        assert_eq!(changes.read().unwrap().len(), 1);
        assert_eq!(
            changes.read().unwrap()[0],
            SubscriptionEvent {
                connection_id: 1,
                provider_broker_id: "futu".to_owned(),
                instruments: vec!["US.AAPL".to_owned()],
            }
        );

        drop(connection);
        assert_eq!(disconnects.read().unwrap().len(), 1);
    }
}
