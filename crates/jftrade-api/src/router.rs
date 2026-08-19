use std::sync::Arc;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};

use axum::Router;
use axum::body::{Body, to_bytes};
use axum::extract::ws::{Message, WebSocketUpgrade};
use axum::extract::{Request, State};
use axum::http::header::{
    ACCESS_CONTROL_ALLOW_CREDENTIALS, ACCESS_CONTROL_ALLOW_HEADERS, ACCESS_CONTROL_ALLOW_METHODS,
    ACCESS_CONTROL_ALLOW_ORIGIN, ACCESS_CONTROL_EXPOSE_HEADERS, CACHE_CONTROL, CONNECTION, VARY,
};
use axum::http::{HeaderMap, HeaderValue, Method, Response, StatusCode, Uri};
use axum::middleware::{self, Next};
use axum::routing::get;
use serde_json::json;
use tower_http::trace::TraceLayer;

use crate::auth::{origin_provided, request_origin};
use crate::envelope::{body_response, empty_response, error_response, success_response};
use crate::{
    AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, AssetBundle, Clock, RouteCatalog,
    SseEvent, SystemClock, TransportMetrics, encode_event, encode_retry, websocket_origin_allowed,
};

const MAX_BODY_BYTES: usize = 4 * 1024 * 1024;

#[derive(Clone)]
pub struct ApiState {
    pub routes: RouteCatalog,
    pub access: AccessPolicy,
    pub assets: AssetBundle,
    pub port: Arc<dyn ApiPort>,
    pub clock: Arc<dyn Clock>,
    pub metrics: Arc<TransportMetrics>,
    pub websocket_limit: usize,
    request_sequence: Arc<AtomicU64>,
    websocket_connections: Arc<AtomicUsize>,
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
            websocket_limit: crate::DEFAULT_WEBSOCKET_LIMIT,
            request_sequence: Arc::new(AtomicU64::new(0)),
            websocket_connections: Arc::new(AtomicUsize::new(0)),
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
    let request_id = request
        .headers()
        .get("x-request-id")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToOwned::to_owned)
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
    let failed = response.status().is_server_error();
    state.metrics.finish(failed);
    response
}

fn authorize(state: &ApiState, request: &Request) -> Result<(), ApiFailure> {
    let headers = request.headers();
    if origin_provided(headers) && !state.access.origin_allowed(headers) {
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
    };
    match state.port.dispatch(input).await {
        Ok(ApiOutput::Json(value)) => success_response(&state.clock, value),
        Ok(ApiOutput::Sse(events)) => sse_response(events),
        Ok(ApiOutput::NoContent) => empty_response(StatusCode::NO_CONTENT),
        Ok(ApiOutput::Raw {
            status,
            content_type,
            body,
        }) => body_response(
            StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
            &content_type,
            body,
        ),
        Err(failure) => error_response(&state.clock, failure),
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
        return error_response(
            &state.clock,
            ApiFailure::new(404, "NOT_FOUND", "unknown endpoint /api/v1/ws/live"),
        );
    }
    if !websocket_origin_allowed(&headers, &state.access) {
        return error_response(
            &state.clock,
            ApiFailure::new(403, "ORIGIN_FORBIDDEN", "request origin is not allowed"),
        );
    }
    if !try_acquire_websocket(&state) {
        return error_response(
            &state.clock,
            ApiFailure::new(
                503,
                "LIVE_WS_LIMIT_REACHED",
                format!(
                    "live websocket connection limit reached ({})",
                    state.websocket_limit
                ),
            ),
        );
    }
    let connections = Arc::clone(&state.websocket_connections);
    let timestamp = state.clock.now_rfc3339();
    upgrade
        .protocols([crate::auth::DESKTOP_WEBSOCKET_PROTOCOL])
        .on_upgrade(move |mut socket| async move {
            let _guard = WebsocketGuard(connections);
            let heartbeat = json!({
                "type": "heartbeat",
                "source": "system",
                "payload": {
                    "at": timestamp,
                    "intervalMs": 15000,
                    "stale": false,
                },
            });
            let _ = socket
                .send(Message::Text(heartbeat.to_string().into()))
                .await;
            let _ = socket.send(Message::Close(None)).await;
        })
}

fn try_acquire_websocket(state: &ApiState) -> bool {
    let limit = state.websocket_limit.max(1);
    state
        .websocket_connections
        .fetch_update(Ordering::AcqRel, Ordering::Acquire, |current| {
            (current < limit).then_some(current + 1)
        })
        .is_ok()
}

struct WebsocketGuard(Arc<AtomicUsize>);

impl Drop for WebsocketGuard {
    fn drop(&mut self) {
        self.0.fetch_sub(1, Ordering::AcqRel);
    }
}
