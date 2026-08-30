use std::collections::BTreeMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Instant;

use axum::Router;
use axum::body::{Body, to_bytes};
use axum::extract::ws::{CloseFrame, Message, WebSocketUpgrade};
use axum::extract::{Request, State};
use axum::http::header::{
    ACCESS_CONTROL_ALLOW_CREDENTIALS, ACCESS_CONTROL_ALLOW_HEADERS, ACCESS_CONTROL_ALLOW_METHODS,
    ACCESS_CONTROL_ALLOW_ORIGIN, ACCESS_CONTROL_EXPOSE_HEADERS, CACHE_CONTROL, CONNECTION, VARY,
};
use axum::http::{HeaderMap, HeaderName, HeaderValue, Method, Response, StatusCode, Uri};
use axum::middleware::{self, Next};
use axum::routing::get;
use serde::Deserialize;
use serde_json::{Value, json};
use tokio_stream::wrappers::ReceiverStream;
use tower_http::trace::TraceLayer;

use crate::auth::{origin_provided, request_origin};
use crate::envelope::{body_response, empty_response, error_response, success_response};
use crate::websocket::{
    LiveDepthSubscription, LiveHub, LiveHubConnection, LiveHubLifecycle, LiveSecuritySubscription,
    LiveSubscriptionSnapshot,
};
use crate::{
    AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, AssetBundle, Clock,
    LiveConnectionMetrics, RouteCatalog, SseEvent, SystemClock, TransportMetrics, encode_event,
    encode_retry, websocket_origin_allowed,
};

const MAX_BODY_BYTES: usize = 4 * 1024 * 1024;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LiveMarketDataStatus {
    pub connected: bool,
}

pub trait LiveMarketDataStatusPort: Send + Sync + std::fmt::Debug {
    fn snapshot(&self) -> LiveMarketDataStatus;
}

#[derive(Clone)]
pub struct ApiState {
    pub routes: RouteCatalog,
    pub access: AccessPolicy,
    pub assets: AssetBundle,
    pub port: Arc<dyn ApiPort>,
    pub clock: Arc<dyn Clock>,
    pub metrics: Arc<TransportMetrics>,
    pub live_connections: Arc<LiveConnectionMetrics>,
    pub live_hub: Arc<LiveHub>,
    pub live_market_data_status: Option<Arc<dyn LiveMarketDataStatusPort>>,
    request_sequence: Arc<AtomicU64>,
}

impl ApiState {
    pub fn new(routes: RouteCatalog, access: AccessPolicy, port: Arc<dyn ApiPort>) -> Self {
        Self {
            routes,
            access,
            assets: AssetBundle::default(),
            port,
            clock: Arc::new(SystemClock),
            metrics: Arc::new(TransportMetrics::default()),
            live_connections: Arc::new(LiveConnectionMetrics::default()),
            live_hub: Arc::new(LiveHub::default()),
            live_market_data_status: None,
            request_sequence: Arc::new(AtomicU64::new(0)),
        }
    }

    pub fn with_assets(mut self, assets: AssetBundle) -> Self {
        self.assets = assets;
        self
    }

    pub fn with_clock(mut self, clock: Arc<dyn Clock>) -> Self {
        self.clock = clock;
        self
    }

    pub fn with_live_market_data_status(
        mut self,
        status: Arc<dyn LiveMarketDataStatusPort>,
    ) -> Self {
        self.live_market_data_status = Some(status);
        self
    }

    pub fn with_live_hub(mut self, hub: Arc<LiveHub>) -> Self {
        self.live_hub = hub;
        self
    }
}

pub fn build_router(state: ApiState) -> Router {
    Router::new()
        .route("/api/v1/ws/live", get(websocket_handler))
        .fallback(dispatch)
        .layer(TraceLayer::new_for_http())
        .layer(middleware::from_fn_with_state(
            state.clone(),
            transport_middleware,
        ))
        .with_state(state)
}

async fn transport_middleware(
    State(state): State<ApiState>,
    mut request: Request,
    next: Next,
) -> Response<Body> {
    state.metrics.start();
    let started_at = Instant::now();
    let method = request.method().to_string();
    let path = request.uri().path().to_owned();
    let request_id = request
        .headers()
        .get("x-request-id")
        .and_then(|value| value.to_str().ok())
        .and_then(normalize_request_id)
        .unwrap_or_else(|| {
            let sequence = state.request_sequence.fetch_add(1, Ordering::Relaxed) + 1;
            format!("rust-{sequence}")
        });
    request.extensions_mut().insert(request_id.clone());

    let origin = request_origin(request.headers());
    let origin_was_provided = origin_provided(request.headers());
    let mut response = if request.method() == Method::OPTIONS {
        if origin_was_provided && !state.access.origin_allowed(request.headers()) {
            empty_response(StatusCode::FORBIDDEN)
        } else {
            empty_response(StatusCode::NO_CONTENT)
        }
    } else if should_authenticate(request.uri().path()) {
        match authorize(&state, &request) {
            Ok(()) => next.run(request).await,
            Err(failure) => error_response(&state.clock, failure),
        }
    } else {
        next.run(request).await
    };
    apply_cors_headers(response.headers_mut(), origin.as_deref(), &state.access);
    if let Ok(value) = HeaderValue::from_str(&request_id) {
        response.headers_mut().insert("x-request-id", value);
    }
    state.metrics.finish_request(
        &method,
        &path,
        response.status().as_u16(),
        started_at.elapsed(),
        &request_id,
    );
    response
}

fn normalize_request_id(value: &str) -> Option<String> {
    let value = value.trim();
    if value.is_empty() || value.len() > 128 {
        return None;
    }
    value
        .chars()
        .all(|char| char.is_ascii_alphanumeric() || matches!(char, '-' | '_' | '.' | ':'))
        .then(|| value.to_owned())
}

fn authorize(state: &ApiState, request: &Request) -> Result<(), ApiFailure> {
    let headers = request.headers();
    if state.access.internal_proxy_protocol.is_some() {
        if state.access.internal_proxy_trusted(headers) {
            return Ok(());
        }
        return Err(ApiFailure::new(
            401,
            "INTERNAL_PROXY_AUTH_REQUIRED",
            "authenticated internal proxy access is required",
        ));
    }
    if origin_provided(headers)
        && !state.access.origin_allowed(headers)
        && request.uri().path() != "/api/v1/ws/live"
    {
        return Err(ApiFailure::new(
            403,
            "ORIGIN_FORBIDDEN",
            "request origin is not allowed",
        ));
    }
    if state.access.desktop_trusted(headers) {
        return Ok(());
    }
    if !state.access.browser_authenticated(headers) {
        return Err(ApiFailure::new(
            401,
            "WEB_AUTH_REQUIRED",
            "Web password authentication is required",
        ));
    }
    if !is_write_method(request.method()) {
        return Ok(());
    }
    if !origin_provided(headers) || !state.access.origin_allowed(headers) {
        return Err(ApiFailure::new(
            403,
            "ORIGIN_FORBIDDEN",
            "write request origin is not allowed",
        ));
    }
    if !state.access.csrf_valid(headers) {
        return Err(ApiFailure::new(
            403,
            "CSRF_FAILED",
            "valid CSRF token is required",
        ));
    }
    Ok(())
}

fn should_authenticate(path: &str) -> bool {
    if path.starts_with("/swagger") {
        return true;
    }
    path.starts_with("/api/") && path != "/api/v1/auth/login" && path != "/api/v1/auth/session"
}

fn is_write_method(method: &Method) -> bool {
    matches!(
        *method,
        Method::POST | Method::PUT | Method::PATCH | Method::DELETE
    )
}

fn apply_cors_headers(headers: &mut HeaderMap, origin: Option<&str>, policy: &AccessPolicy) {
    if let Some(origin) = origin.filter(|origin| policy.allowed_origins.contains(*origin))
        && let Ok(origin) = HeaderValue::from_str(origin)
    {
        headers.insert(ACCESS_CONTROL_ALLOW_ORIGIN, origin);
        headers.insert(VARY, HeaderValue::from_static("Origin"));
        headers.insert(
            ACCESS_CONTROL_ALLOW_CREDENTIALS,
            HeaderValue::from_static("true"),
        );
    }
    headers.insert(
        ACCESS_CONTROL_ALLOW_HEADERS,
        HeaderValue::from_static("Origin, Content-Type, Authorization, X-CSRF-Token, X-Request-ID"),
    );
    headers.insert(
        ACCESS_CONTROL_ALLOW_METHODS,
        HeaderValue::from_static("GET, POST, PUT, PATCH, DELETE, OPTIONS"),
    );
    headers.insert(
        ACCESS_CONTROL_EXPOSE_HEADERS,
        HeaderValue::from_static("X-Request-ID"),
    );
}

async fn dispatch(State(state): State<ApiState>, request: Request) -> Response<Body> {
    let method = request.method().clone();
    let uri = request.uri().clone();
    if !uri.path().starts_with("/api/") && !uri.path().starts_with("/swagger") {
        return static_response(&state.assets, &method, &uri);
    }
    if !state.routes.allows(method.as_str(), uri.path()) {
        return error_response(
            &state.clock,
            ApiFailure::new(404, "NOT_FOUND", format!("unknown endpoint {}", uri.path())),
        );
    }
    let request_id = request
        .extensions()
        .get::<String>()
        .cloned()
        .unwrap_or_default();
    let desktop_trusted = state.access.desktop_trusted(request.headers());
    let origin_provided = origin_provided(request.headers());
    let origin_allowed = !origin_provided || state.access.origin_allowed(request.headers());
    let browser_authenticated = state.access.browser_authenticated(request.headers());
    let csrf_valid = state.access.csrf_valid(request.headers());
    let session_cookie = state.access.session_cookie(request.headers());
    let body = match to_bytes(request.into_body(), MAX_BODY_BYTES).await {
        Ok(body) => body.to_vec(),
        Err(_) => {
            return error_response(
                &state.clock,
                ApiFailure::new(400, "INVALID_REQUEST", "request body is too large"),
            );
        }
    };
    let input = ApiRequest {
        method: method.to_string(),
        path: uri.path().to_owned(),
        query: uri.query().unwrap_or_default().to_owned(),
        body,
        request_id,
        desktop_trusted,
        origin_provided,
        origin_allowed,
        browser_authenticated,
        csrf_valid,
        session_cookie,
    };
    let is_auth_session = input.path == "/api/v1/auth/session";
    let mut response = match state.port.dispatch(input).await {
        Ok(ApiOutput::Json(value)) => success_response(&state.clock, value),
        Ok(ApiOutput::Sse(events)) => sse_response(events),
        Ok(ApiOutput::NoContent) => empty_response(StatusCode::NO_CONTENT),
        Ok(ApiOutput::Raw {
            status,
            content_type,
            body,
            headers,
        }) => body_response(
            StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
            &content_type,
            body,
        )
        .tap_headers(&headers),
        Ok(ApiOutput::RawStream {
            status,
            content_type,
            stream,
            headers,
        }) => {
            let body = stream
                .take_receiver()
                .map(ReceiverStream::new)
                .map(Body::from_stream)
                .unwrap_or_else(Body::empty);
            body_response(
                StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
                &content_type,
                body,
            )
            .tap_headers(&headers)
        }
        Err(failure) => error_response(&state.clock, failure),
    };
    if is_auth_session {
        response
            .headers_mut()
            .insert(CACHE_CONTROL, HeaderValue::from_static("no-store"));
    }
    response
}

trait ResponseHeaders {
    fn tap_headers(self, headers: &BTreeMap<String, String>) -> Self;
}

impl ResponseHeaders for Response<Body> {
    fn tap_headers(mut self, headers: &BTreeMap<String, String>) -> Self {
        for (name, value) in headers {
            let Ok(name) = HeaderName::from_bytes(name.as_bytes()) else {
                continue;
            };
            let Ok(value) = HeaderValue::from_str(value) else {
                continue;
            };
            self.headers_mut().insert(name, value);
        }
        self
    }
}

fn static_response(assets: &AssetBundle, method: &Method, uri: &Uri) -> Response<Body> {
    if *method != Method::GET && *method != Method::HEAD {
        return empty_response(StatusCode::NOT_FOUND);
    }
    let requested = if uri.path() == "/" {
        "index.html"
    } else {
        uri.path().trim_start_matches('/')
    };
    let asset = assets.get(requested).or_else(|| assets.spa_index());
    match asset {
        Some(asset) if *method == Method::HEAD => {
            body_response(StatusCode::OK, &asset.content_type, Body::empty())
        }
        Some(asset) => body_response(StatusCode::OK, &asset.content_type, asset.bytes.clone()),
        None => empty_response(StatusCode::NOT_FOUND),
    }
}

fn sse_response(events: Vec<SseEvent>) -> Response<Body> {
    let mut body = encode_retry(3000);
    for event in events {
        match encode_event(&event) {
            Ok(frame) => body.push_str(&frame),
            Err(_) => return empty_response(StatusCode::INTERNAL_SERVER_ERROR),
        }
    }
    let mut response = body_response(StatusCode::OK, "text/event-stream", body);
    response
        .headers_mut()
        .insert(CACHE_CONTROL, HeaderValue::from_static("no-cache"));
    response
        .headers_mut()
        .insert(CONNECTION, HeaderValue::from_static("keep-alive"));
    response
}

async fn websocket_handler(
    State(state): State<ApiState>,
    headers: HeaderMap,
    upgrade: WebSocketUpgrade,
) -> Response<Body> {
    if !state.routes.allows("GET", "/api/v1/ws/live") {
        return body_response(
            StatusCode::NOT_FOUND,
            "text/plain; charset=utf-8",
            "404 page not found\n",
        );
    }
    if matches!(
        state.live_hub.lifecycle(),
        LiveHubLifecycle::ShuttingDown | LiveHubLifecycle::Stopped
    ) {
        return error_response(
            &state.clock,
            ApiFailure::new(
                503,
                "LIVE_WS_UNAVAILABLE",
                "live websocket hub is not accepting connections",
            ),
        );
    }
    if !websocket_origin_allowed(&headers, &state.access) {
        return body_response(
            StatusCode::FORBIDDEN,
            "text/plain; charset=utf-8",
            "Forbidden\n",
        );
    }
    if state.access.enforce_access
        && !state.access.desktop_trusted(&headers)
        && !state.access.browser_authenticated(&headers)
    {
        return error_response(
            &state.clock,
            ApiFailure::new(
                401,
                "WEB_AUTH_REQUIRED",
                "Web password authentication is required",
            ),
        );
    }
    let Some(connection_permit) = state.live_connections.try_acquire() else {
        let limit = state.live_connections.snapshot().limit;
        return error_response(
            &state.clock,
            ApiFailure::new(
                503,
                "LIVE_WS_LIMIT_REACHED",
                format!("live websocket connection limit reached ({})", limit),
            ),
        );
    };
    let timestamp = state.clock.now_rfc3339();
    let live_market_data_status = state.live_market_data_status.clone();
    let Some(live_hub_connection) = state.live_hub.try_connect() else {
        return error_response(
            &state.clock,
            ApiFailure::new(
                503,
                "LIVE_WS_UNAVAILABLE",
                "live websocket hub is not accepting connections",
            ),
        );
    };
    let shutdown = state.live_hub.subscribe_shutdown();
    upgrade
        .protocols([crate::auth::DESKTOP_WEBSOCKET_PROTOCOL])
        .on_upgrade(move |socket| {
            websocket_session(
                socket,
                connection_permit,
                live_hub_connection,
                timestamp,
                live_market_data_status,
                shutdown,
            )
        })
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct LiveClientSubscriptions {
    provider_broker_id: String,
    active_instruments: Vec<String>,
    #[serde(default)]
    security_details: Vec<LiveClientSecurityDetails>,
    #[serde(default)]
    depth: Vec<LiveClientDepth>,
    #[serde(default)]
    console_refresh: bool,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct LiveClientSecurityDetails {
    market: String,
    symbol: String,
    instrument_id: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
struct LiveClientDepth {
    market: String,
    symbol: String,
    instrument_id: String,
    num: i64,
}

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
struct LiveClientMessage {
    #[serde(rename = "type")]
    event_type: String,
    subscriptions: LiveClientSubscriptions,
}

async fn websocket_session(
    mut socket: axum::extract::ws::WebSocket,
    connection_permit: crate::LiveConnectionPermit,
    mut live_hub_connection: LiveHubConnection,
    timestamp: String,
    live_market_data_status: Option<Arc<dyn LiveMarketDataStatusPort>>,
    mut shutdown: tokio::sync::watch::Receiver<bool>,
) {
    if *shutdown.borrow() {
        let _ = socket
            .send(Message::Close(Some(CloseFrame {
                code: 1001,
                reason: "server shutting down".into(),
            })))
            .await;
        return;
    }
    let heartbeat = live_heartbeat_with_subscription(
        &timestamp,
        live_market_data_status.as_deref(),
        &live_hub_connection,
    );
    if socket
        .send(Message::Text(heartbeat.to_string().into()))
        .await
        .is_err()
    {
        return;
    }
    let mut interval = tokio::time::interval(std::time::Duration::from_millis(15000));
    interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

    loop {
        tokio::select! {
            _ = interval.tick() => {
                let timestamp = crate::SystemClock.now_rfc3339();
                let hb = live_heartbeat_with_subscription(
                    &timestamp,
                    live_market_data_status.as_deref(),
                    &live_hub_connection,
                );
                if socket.send(Message::Text(hb.to_string().into())).await.is_err() {
                    break;
                }
            }
            msg = socket.recv() => {
                let Some(message) = msg else {
                    break;
                };
                let Ok(message) = message else {
                    break;
                };
                match live_subscription_snapshot(&message) {
                    Ok(Some(snapshot)) => {
                        let mut active_instruments = snapshot.active_instruments.clone();
                        active_instruments.extend(
                            snapshot
                                .security_details
                                .iter()
                                .map(|item| item.instrument_id.clone()),
                        );
                        active_instruments.extend(
                            snapshot.depth.iter().map(|item| item.instrument_id.clone()),
                        );
                        active_instruments.sort();
                        active_instruments.dedup();
                        connection_permit.set_active_instruments(&active_instruments);
                        live_hub_connection.set_subscription_snapshot(&snapshot);
                    }
                    Ok(None) => {}
                    Err(code) => {
                        let reason = match code {
                            1007 => "invalid subscription payload",
                            _ => "subscription policy violation",
                        };
                        let _ = socket
                            .send(Message::Close(Some(CloseFrame {
                                code,
                                reason: reason.into(),
                            })))
                            .await;
                        break;
                    }
                }
            }
            event = live_hub_connection.recv() => {
                let Some(event) = event else {
                    break;
                };
                if socket.send(Message::Text(event.to_string().into())).await.is_err() {
                    break;
                }
            }
            changed = shutdown.changed() => {
                if changed.is_ok() || *shutdown.borrow() {
                    let _ = socket
                        .send(Message::Close(Some(CloseFrame {
                            code: 1001,
                            reason: "server shutting down".into(),
                        })))
                        .await;
                }
                break;
            }
        }
    }
}

#[cfg(test)]
fn live_heartbeat(
    timestamp: &str,
    status: Option<&dyn LiveMarketDataStatusPort>,
) -> serde_json::Value {
    let stale = status.is_some_and(|port| !port.snapshot().connected);
    live_heartbeat_payload(timestamp, stale, "", 0)
}

fn live_heartbeat_with_subscription(
    timestamp: &str,
    status: Option<&dyn LiveMarketDataStatusPort>,
    connection: &LiveHubConnection,
) -> serde_json::Value {
    let stale = status.is_some_and(|port| !port.snapshot().connected);
    live_heartbeat_payload(
        timestamp,
        stale,
        &connection.provider_broker_id(),
        connection.active_instrument_count(),
    )
}

fn live_heartbeat_payload(
    timestamp: &str,
    stale: bool,
    provider_broker_id: &str,
    active_instruments: usize,
) -> serde_json::Value {
    // Go's live backend marks any explicitly selected non-Futu broker as a
    // polling transport, even when the shared runtime itself is healthy.
    // Keep that distinction in the heartbeat so helper-backed clients do not
    // advertise a push stream they cannot receive.
    let polling_provider =
        !provider_broker_id.trim().is_empty() && !provider_broker_id.eq_ignore_ascii_case("futu");
    let transport_mode = if polling_provider {
        "snapshot-poll-fallback"
    } else if stale {
        "idle"
    } else {
        "push-stream"
    };
    json!({
        "eventId": format!("heartbeat|live-websocket|{timestamp}"),
        "type": "heartbeat",
        "source": "system",
        "entityId": "live-websocket",
        "serverTime": timestamp,
        "payload": {
            "type": "heartbeat",
            "at": timestamp,
            "intervalMs": 15000,
            "providerBrokerId": provider_broker_id,
            "stale": stale,
            "staleReasons": if stale { vec!["provider_unavailable"] } else { Vec::<&str>::new() },
            "transport": {
                "mode": transport_mode,
                "activeInstruments": active_instruments,
                "freshInstruments": if stale { 0 } else { active_instruments },
                "staleInstruments": if stale { active_instruments } else { 0 },
            },
            "liveStream": {
                "connected": !stale,
                "backoffActive": stale,
                "retryAfter": Value::Null,
                "failureCount": if stale { 1 } else { 0 },
                "lastError": if stale { Value::String("provider unavailable".to_owned()) } else { Value::Null },
            },
        },
    })
}

#[cfg(test)]
fn live_subscription_update(message: &Message) -> Result<Option<Vec<String>>, ()> {
    Ok(live_subscription_details(message)?.map(|(_, instruments)| instruments))
}

#[cfg(test)]
fn live_subscription_details(message: &Message) -> Result<Option<(String, Vec<String>)>, ()> {
    let payload = match message {
        Message::Text(payload) => payload.as_bytes(),
        Message::Binary(payload) => payload.as_ref(),
        Message::Close(_) => return Err(()),
        _ => return Ok(None),
    };
    let Ok(message) = serde_json::from_slice::<LiveClientMessage>(payload) else {
        return Ok(None);
    };
    if message.event_type != "subscribe" {
        return Ok(None);
    }
    if message.subscriptions.provider_broker_id.trim().is_empty() {
        return Err(());
    }
    Ok(Some((
        message.subscriptions.provider_broker_id,
        message.subscriptions.active_instruments,
    )))
}

fn live_subscription_snapshot(message: &Message) -> Result<Option<LiveSubscriptionSnapshot>, u16> {
    let payload = match message {
        Message::Text(payload) => payload.as_bytes(),
        Message::Binary(payload) => payload.as_ref(),
        Message::Close(_) => return Err(1000u16),
        _ => return Ok(None),
    };
    let Ok(message) = serde_json::from_slice::<LiveClientMessage>(payload) else {
        // Go's read loop ignores malformed/non-JSON messages and keeps the
        // connection alive.  Only a valid subscribe message can replace the
        // current demand snapshot.
        return Ok(None);
    };
    if message.event_type != "subscribe" {
        return Ok(None);
    }
    if message.subscriptions.provider_broker_id.trim().is_empty() {
        return Err(1008u16);
    }
    let active_instruments = message
        .subscriptions
        .active_instruments
        .into_iter()
        .map(|value| value.trim().to_ascii_uppercase())
        .filter(|value| !value.is_empty())
        .collect::<std::collections::BTreeSet<_>>()
        .into_iter()
        .collect();
    let mut security_details = message
        .subscriptions
        .security_details
        .into_iter()
        .map(normalize_security_details)
        .flatten()
        .collect::<Vec<_>>();
    security_details.sort_by(|left, right| left.instrument_id.cmp(&right.instrument_id));
    security_details.dedup_by(|left, right| left.instrument_id == right.instrument_id);
    let mut depth = message
        .subscriptions
        .depth
        .into_iter()
        .map(normalize_depth)
        .flatten()
        .collect::<Vec<_>>();
    depth.sort_by(|left, right| {
        left.instrument_id
            .cmp(&right.instrument_id)
            .then(left.num.cmp(&right.num))
    });
    depth
        .dedup_by(|left, right| left.instrument_id == right.instrument_id && left.num == right.num);
    Ok(Some(LiveSubscriptionSnapshot {
        provider_broker_id: message
            .subscriptions
            .provider_broker_id
            .trim()
            .to_ascii_lowercase(),
        active_instruments,
        security_details,
        depth,
        console_refresh: message.subscriptions.console_refresh,
    }))
}

fn normalize_security_details(item: LiveClientSecurityDetails) -> Option<LiveSecuritySubscription> {
    let market = item.market.trim().to_ascii_uppercase();
    let symbol = item.symbol.trim().to_ascii_uppercase();
    let instrument_id = item.instrument_id.trim().to_ascii_uppercase();
    if market.is_empty() || symbol.is_empty() || instrument_id.is_empty() {
        return None;
    }
    Some(LiveSecuritySubscription {
        market,
        symbol,
        instrument_id,
    })
}

fn normalize_depth(item: LiveClientDepth) -> Option<LiveDepthSubscription> {
    let market = item.market.trim().to_ascii_uppercase();
    let symbol = item.symbol.trim().to_ascii_uppercase();
    let instrument_id = item.instrument_id.trim().to_ascii_uppercase();
    if market.is_empty() || symbol.is_empty() || instrument_id.is_empty() {
        return None;
    }
    Some(LiveDepthSubscription {
        market,
        symbol,
        instrument_id,
        num: item.num.clamp(1, 50),
    })
}

#[cfg(test)]
mod websocket_subscription_tests {
    use super::*;

    #[derive(Debug)]
    struct FixedLiveStatus(bool);

    impl LiveMarketDataStatusPort for FixedLiveStatus {
        fn snapshot(&self) -> LiveMarketDataStatus {
            LiveMarketDataStatus { connected: self.0 }
        }
    }

    #[test]
    fn heartbeat_reports_live_provider_connectivity_without_a_fixture_projection() {
        assert_eq!(
            live_heartbeat("fixture-time", Some(&FixedLiveStatus(false)))["payload"]["stale"],
            true
        );
        assert_eq!(
            live_heartbeat("fixture-time", Some(&FixedLiveStatus(true)))["payload"]["stale"],
            false
        );
        assert_eq!(
            live_heartbeat("fixture-time", None)["payload"]["stale"],
            false
        );
    }

    #[test]
    fn subscription_message_matches_go_ignore_update_and_close_rules() {
        for payload in ["not-json", r#"{"type":"other"}"#] {
            assert_eq!(
                live_subscription_update(&Message::Text(payload.into())),
                Ok(None)
            );
        }
        assert_eq!(
            live_subscription_update(&Message::Text(
                r#"{"type":"subscribe","subscriptions":{"activeInstruments":["US.AAPL"]}}"#.into(),
            )),
            Err(())
        );
        assert_eq!(
            live_subscription_update(&Message::Text(
                r#"{"type":"subscribe","subscriptions":{"providerBrokerId":" futu ","activeInstruments":[" us.aapl ","US.AAPL"]}}"#
                    .into(),
            )),
            Ok(Some(vec![" us.aapl ".to_owned(), "US.AAPL".to_owned()]))
        );
    }
}
