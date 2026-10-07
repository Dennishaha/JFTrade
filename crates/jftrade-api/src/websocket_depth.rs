//! Depth reads and wire projection owned by one authenticated WebSocket session.
use std::collections::BTreeMap;
use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;

use axum::http::HeaderMap;
use serde_json::{Value, json};

use crate::router::ApiState;
use crate::{
    ApiOutput, ApiPort, ApiRequest, Clock, LiveDepthSubscription, LiveSubscriptionSnapshot,
};

type DepthRead = Pin<Box<dyn Future<Output = Vec<Value>> + Send>>;

pub(crate) struct WebsocketDepth {
    port: Arc<dyn ApiPort>,
    clock: Arc<dyn Clock>,
    routes: crate::RouteCatalog,
    request: ApiRequest,
    subscriptions: Vec<LiveDepthSubscription>,
    resolved_at: BTreeMap<String, String>,
    pending: Option<DepthRead>,
}

impl WebsocketDepth {
    pub(crate) fn new(state: &ApiState, headers: &HeaderMap) -> Self {
        Self {
            port: Arc::clone(&state.port),
            clock: Arc::clone(&state.clock),
            routes: state.routes.clone(),
            request: ApiRequest {
                method: "GET".into(),
                path: String::new(),
                query: String::new(),
                body: Vec::new(),
                request_id: String::new(),
                desktop_trusted: state.access.desktop_trusted(headers),
                origin_provided: crate::auth::origin_provided(headers),
                origin_allowed: crate::websocket_origin_allowed(headers, &state.access),
                browser_authenticated: state.access.browser_authenticated(headers),
                csrf_valid: state.access.csrf_valid(headers),
                session_cookie: state.access.session_cookie(headers),
            },
            subscriptions: Vec::new(),
            resolved_at: BTreeMap::new(),
            pending: None,
        }
    }

    pub(crate) fn replace(&mut self, snapshot: &LiveSubscriptionSnapshot) {
        self.subscriptions = snapshot.depth.clone();
        self.resolved_at.clear();
        // Replacing or dropping this future releases a pending asynchronous
        // read. There is no detached producer for a previous subscription.
        let port = Arc::clone(&self.port);
        let clock = Arc::clone(&self.clock);
        let routes = self.routes.clone();
        let request = self.request.clone();
        let snapshot = snapshot.clone();
        self.pending = (!snapshot.depth.is_empty()).then(|| {
            Box::pin(async move { read_snapshots(port, clock, routes, request, snapshot).await })
                as DepthRead
        });
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
        if event["type"] != "market.depth" {
            return vec![event];
        }
        let instrument = event
            .pointer("/payload/request/instrumentId")
            .or_else(|| event.pointer("/payload/instrumentId"))
            .and_then(Value::as_str)
            .unwrap_or_default()
            .trim()
            .to_ascii_uppercase();
        let frames = self
            .subscriptions
            .iter()
            .filter(|item| item.instrument_id == instrument)
            .map(|item| depth_frame(event.clone(), item))
            .collect();
        self.filter_frames(frames)
    }

    pub(crate) fn filter_snapshots(&mut self, events: Vec<Value>) -> Vec<Value> {
        // A concrete push may win while the initial read is awaiting its
        // provider. That initial response must not rewind the client's book.
        let unseen = events
            .into_iter()
            .filter(|frame| {
                frame["entityId"]
                    .as_str()
                    .is_some_and(|key| !self.resolved_at.contains_key(key))
            })
            .collect();
        self.filter_frames(unseen)
    }

    fn filter_frames(&mut self, events: Vec<Value>) -> Vec<Value> {
        events
            .into_iter()
            .filter_map(|frame| {
                let key = frame["entityId"].as_str()?.to_owned();
                if let Some(at) = frame
                    .pointer("/payload/meta/resolvedAt")
                    .and_then(Value::as_str)
                    .filter(|at| !at.is_empty())
                {
                    if self
                        .resolved_at
                        .get(&key)
                        .is_some_and(|previous| previous == at)
                    {
                        return None;
                    }
                    self.resolved_at.insert(key, at.to_owned());
                }
                Some(frame)
            })
            .collect()
    }
}

async fn read_snapshots(
    port: Arc<dyn ApiPort>,
    clock: Arc<dyn Clock>,
    routes: crate::RouteCatalog,
    request: ApiRequest,
    snapshot: LiveSubscriptionSnapshot,
) -> Vec<Value> {
    let mut events = Vec::new();
    for item in snapshot.depth {
        let mut input = request.clone();
        input.path = format!("/api/v1/market-data/depth/{}/{}", item.market, item.symbol);
        if !routes.allows("GET", &input.path) {
            continue;
        }
        input.query = format!(
            "num={}&brokerId={}",
            item.num,
            query_component(&snapshot.provider_broker_id)
        );
        let result =
            tokio::time::timeout(std::time::Duration::from_secs(2), port.dispatch(input)).await;
        // Auxiliary provider failures skip the family; the live session stays
        // available for the next subscription and concrete provider pushes.
        if let Ok(Ok(ApiOutput::Json(mut payload))) = result
            && let Some(object) = payload.as_object_mut()
        {
            object.insert("type".into(), json!("market.depth"));
            object.insert("brokerId".into(), json!(snapshot.provider_broker_id));
            let at = clock.now_rfc3339();
            events.push(depth_frame(
                json!({"eventId":format!("market.depth|{}|{}|{}", item.instrument_id, item.num, at),
                "type":"market.depth","source":"market-data","entityId":item.instrument_id,
                "serverTime":at,"payload":payload}),
                &item,
            ));
        }
    }
    events
}

fn depth_frame(mut event: Value, item: &LiveDepthSubscription) -> Value {
    let entity = format!("{}|{}", item.instrument_id, item.num);
    event["entityId"] = json!(entity);
    event["eventId"] = json!(format!(
        "market.depth|{entity}|{}",
        event["serverTime"].as_str().unwrap_or_default()
    ));
    event["payload"]["request"] = json!({"market":item.market,"symbol":item.symbol,"instrumentId":item.instrument_id,"num":item.num});
    for side in ["bids", "asks"] {
        if let Some(levels) = event["payload"]["depth"][side].as_array_mut() {
            levels.truncate(item.num.clamp(1, 50) as usize);
        }
    }
    event
}

fn query_component(value: &str) -> String {
    let mut encoded = String::new();
    for byte in value.bytes() {
        if byte.is_ascii_alphanumeric() || b"-._~".contains(&byte) {
            encoded.push(char::from(byte));
        } else {
            encoded.push_str(&format!("%{byte:02X}"));
        }
    }
    encoded
}

#[cfg(test)]
#[path = "websocket_depth_tests.rs"]
mod tests;
