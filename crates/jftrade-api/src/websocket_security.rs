//! Security-detail reads and resolution deduplication owned by one live connection.
use std::collections::BTreeMap;
use std::future::Future;
use std::pin::Pin;

use axum::http::HeaderMap;
use serde_json::{Value, json};

use crate::router::ApiState;
use crate::websocket_read::{WebsocketRead, query_component};
use crate::{LiveSecuritySubscription, LiveSubscriptionSnapshot};

type SecurityRead = Pin<Box<dyn Future<Output = Vec<Value>> + Send>>;

pub(crate) struct WebsocketSecurity {
    read: WebsocketRead,
    subscriptions: Vec<LiveSecuritySubscription>,
    resolved_at: BTreeMap<String, String>,
    pending: Option<SecurityRead>,
}

impl WebsocketSecurity {
    pub(crate) fn new(state: &ApiState, headers: &HeaderMap) -> Self {
        Self {
            read: WebsocketRead::new(state, headers),
            subscriptions: Vec::new(),
            resolved_at: BTreeMap::new(),
            pending: None,
        }
    }

    pub(crate) fn replace(&mut self, snapshot: &LiveSubscriptionSnapshot) {
        self.subscriptions = snapshot.security_details.clone();
        self.resolved_at.clear();
        let read = self.read.clone();
        let snapshot = snapshot.clone();
        // Dropping the previous future cancels its read, including on unsubscribe.
        self.pending = (!snapshot.security_details.is_empty())
            .then(|| Box::pin(async move { read_snapshots(read, snapshot).await }) as SecurityRead);
    }

    pub(crate) async fn snapshots(&mut self) -> Vec<Value> {
        let Some(read) = self.pending.as_mut() else {
            return std::future::pending().await;
        };
        let events = read.await;
        self.pending = None;
        events
    }

    pub(crate) fn project(&mut self, event: Value) -> Vec<Value> {
        if event["type"] != "market.security-details" {
            return vec![event];
        }
        self.filter_frames(vec![event])
    }

    pub(crate) fn filter_snapshots(&mut self, events: Vec<Value>) -> Vec<Value> {
        // A slow initial read must not overwrite a push already delivered to this client.
        let unseen = events
            .into_iter()
            .filter(|event| {
                event["entityId"]
                    .as_str()
                    .is_some_and(|key| !self.resolved_at.contains_key(key))
            })
            .collect();
        self.filter_frames(unseen)
    }

    fn filter_frames(&mut self, events: Vec<Value>) -> Vec<Value> {
        events
            .into_iter()
            .filter_map(|event| {
                let instrument = event["entityId"].as_str()?.to_owned();
                if !self
                    .subscriptions
                    .iter()
                    .any(|item| item.instrument_id == instrument)
                {
                    return None;
                }
                if let Some(at) = event
                    .pointer("/payload/meta/resolvedAt")
                    .or_else(|| event.pointer("/payload/at"))
                    .and_then(Value::as_str)
                    .filter(|at| !at.is_empty())
                {
                    if self
                        .resolved_at
                        .get(&instrument)
                        .is_some_and(|previous| previous == at)
                    {
                        return None;
                    }
                    self.resolved_at.insert(instrument, at.to_owned());
                }
                Some(event)
            })
            .collect()
    }
}

async fn read_snapshots(read: WebsocketRead, snapshot: LiveSubscriptionSnapshot) -> Vec<Value> {
    let mut events = Vec::new();
    for item in snapshot.security_details {
        let path = format!(
            "/api/v1/market-data/securities/{}/{}",
            item.market, item.symbol
        );
        let query = format!("brokerId={}", query_component(&snapshot.provider_broker_id));
        if let Some(mut payload) = read.query(path, query).await {
            let at = payload
                .pointer("/meta/resolvedAt")
                .and_then(Value::as_str)
                .filter(|at| !at.is_empty())
                .map(str::to_owned)
                .unwrap_or_else(|| read.now());
            let object = payload
                .as_object_mut()
                .expect("object-shaped auxiliary read");
            object.insert("type".into(), json!("market.security-details"));
            object.insert("at".into(), json!(at));
            object.insert("brokerId".into(), json!(snapshot.provider_broker_id));
            events.push(json!({
                "eventId":format!("market.security-details|{}|{at}", item.instrument_id),
                "type":"market.security-details","source":"market-data",
                "entityId":item.instrument_id,"serverTime":at,"payload":payload,
            }));
        }
    }
    events
}

#[cfg(test)]
#[path = "websocket_security_tests.rs"]
mod tests;
