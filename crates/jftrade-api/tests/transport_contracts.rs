use std::sync::{Arc, Mutex};

use axum::body::Body;
use axum::http::{Method, Request, StatusCode};
use http_body_util::BodyExt;
use jftrade_api::{
    ACCESS_SURFACE_HEADER, AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, ApiState,
    Asset, AssetBundle, FixedClock, INTERNAL_PROXY_PROTOCOL_HEADER, PortFuture, RouteCatalog,
    RouteSpec, TransportMetrics, build_router,
};
use serde_json::{Value, json};
use tower::ServiceExt;

#[derive(Default)]
struct RecordingPort {
    requests: Mutex<Vec<ApiRequest>>,
}

#[tokio::test]
async fn managed_sidecar_requires_bearer_protocol_and_verified_surface() {
    let port = Arc::new(RecordingPort::default());
    let routes = RouteCatalog::new([RouteSpec {
        method: "GET".into(),
        path: "/api/v1/settings/ui".into(),
    }])
    .expect("routes");
    let access = AccessPolicy {
        desktop_token: Some("private-rust-bearer".into()),
        internal_proxy_protocol: Some("jftrade-go-rehearsal.v1".into()),
        ..AccessPolicy::default()
    };
    let router = build_router(ApiState::new(routes, access, port.clone()));
    let request = |protocol: Option<&str>, surface: Option<&str>| {
        let mut builder = Request::builder()
            .uri("/api/v1/settings/ui")
            .header("authorization", "Bearer private-rust-bearer");
        if let Some(protocol) = protocol {
            builder = builder.header(INTERNAL_PROXY_PROTOCOL_HEADER, protocol);
        }
        if let Some(surface) = surface {
            builder = builder.header(ACCESS_SURFACE_HEADER, surface);
        }
        builder.body(Body::empty()).expect("request")
    };

    for denied in [
        request(None, Some("desktop")),
        request(Some("wrong"), Some("desktop")),
        request(Some("jftrade-go-rehearsal.v1"), None),
        request(Some("jftrade-go-rehearsal.v1"), Some("unverified")),
    ] {
        let response = router.clone().oneshot(denied).await.expect("denied");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
    }
    let websocket_token_substitution = Request::builder()
        .uri("/api/v1/settings/ui")
        .header("sec-websocket-protocol", "private-rust-bearer")
        .header(INTERNAL_PROXY_PROTOCOL_HEADER, "jftrade-go-rehearsal.v1")
        .header(ACCESS_SURFACE_HEADER, "desktop")
        .body(Body::empty())
        .expect("request");
    let denied = router
        .clone()
        .oneshot(websocket_token_substitution)
        .await
        .expect("denied");
    assert_eq!(denied.status(), StatusCode::UNAUTHORIZED);
    let accepted = router
        .oneshot(request(Some("jftrade-go-rehearsal.v1"), Some("desktop")))
        .await
        .expect("accepted");
    assert_eq!(accepted.status(), StatusCode::OK);
    assert_eq!(port.requests.lock().expect("requests").len(), 1);
}

impl ApiPort for RecordingPort {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async move {
            self.requests.lock().expect("requests").push(request);
            Ok(ApiOutput::Json(json!({"theme": "system"})))
        })
    }
}

struct RetryAfterPort;

impl ApiPort for RetryAfterPort {
    fn dispatch(&self, _request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async {
            Err(ApiFailure::new(
                StatusCode::SERVICE_UNAVAILABLE.as_u16(),
                "PROVIDER_BUSY",
                "provider is busy",
            )
            .with_retry_after(2))
        })
    }
}

#[tokio::test]
async fn error_envelope_preserves_optional_retry_after_header() {
    let routes = RouteCatalog::new([RouteSpec {
        method: "GET".into(),
        path: "/api/v1/settings/ui".into(),
    }])
    .expect("routes");
    let metrics = Arc::new(TransportMetrics::default());
    let mut state = ApiState::new(
        routes,
        AccessPolicy {
            desktop_token: Some("desktop-token".into()),
            ..AccessPolicy::default()
        },
        std::sync::Arc::new(RetryAfterPort),
    )
    .with_clock(std::sync::Arc::new(FixedClock(
        "2026-08-22T00:00:00Z".into(),
    )));
    state.metrics = Arc::clone(&metrics);
    let router = build_router(state);
    let response = router
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/v1/settings/ui")
                .header("authorization", "Bearer desktop-token")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::SERVICE_UNAVAILABLE);
    assert_eq!(response.headers()["retry-after"], "2");
    let observed = metrics.request_observability_snapshot();
    assert_eq!(observed.recent_errors.len(), 1);
    assert_eq!(observed.recent_errors[0].method, "GET");
    assert_eq!(observed.recent_errors[0].path, "/api/v1/settings/ui");
    assert_eq!(observed.recent_errors[0].status, 503);
    assert_eq!(observed.recent_errors[0].error.as_deref(), Some("HTTP 503"));

    let unknown = router
        .oneshot(
            Request::builder()
                .uri("/api/v1/unknown")
                .header("authorization", "Bearer desktop-token")
                .body(Body::empty())
                .expect("unknown request"),
        )
        .await
        .expect("unknown response");
    assert_eq!(unknown.status(), StatusCode::NOT_FOUND);
    assert!(unknown.headers().get("retry-after").is_none());
}

fn fixture() -> (axum::Router, Arc<RecordingPort>) {
    let port = Arc::new(RecordingPort::default());
    let routes = RouteCatalog::new([
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/settings/ui".into(),
        },
        RouteSpec {
            method: "PUT".into(),
            path: "/api/v1/settings/ui".into(),
        },
        RouteSpec {
            method: "GET".into(),
            path: "/api/v1/ws/live".into(),
        },
    ])
    .expect("routes");
    let access = AccessPolicy {
        desktop_token: Some("desktop-token".into()),
        session_token: Some("session-token".into()),
        csrf_token: Some("csrf-token".into()),
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://jftrade.local".into()]);
    let assets = AssetBundle::new([(
        "index.html".into(),
        Asset {
            content_type: "text/html; charset=utf-8".into(),
            bytes: b"<html>JFTrade</html>".to_vec(),
        },
    )]);
    let state = ApiState::new(routes, access, port.clone())
        .with_assets(assets)
        .with_clock(Arc::new(FixedClock("2026-08-19T00:00:00Z".into())));
    (build_router(state), port)
}

#[tokio::test]
async fn desktop_token_reaches_port_with_stable_envelope_and_request_id() {
    let (router, port) = fixture();
    let response = router
        .oneshot(
            Request::builder()
                .uri("/api/v1/settings/ui")
                .header("authorization", "Bearer desktop-token")
                .header("x-request-id", "request-7")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("response");
    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(response.headers()["x-request-id"], "request-7");
    let body = response
        .into_body()
        .collect()
        .await
        .expect("body")
        .to_bytes();
    let value: Value = serde_json::from_slice(&body).expect("json");
    assert_eq!(
        value,
        json!({
            "ok": true,
            "data": {"theme": "system"},
            "timestamp": "2026-08-19T00:00:00Z",
        })
    );
    assert_eq!(
        port.requests.lock().expect("requests")[0].request_id,
        "request-7"
    );
    assert!(port.requests.lock().expect("requests")[0].desktop_trusted);
}

#[tokio::test]
async fn invalid_request_id_is_replaced_before_observation_and_dispatch() {
    let (router, port) = fixture();
    let response = router
        .oneshot(
            Request::builder()
                .uri("/api/v1/settings/ui")
                .header("authorization", "Bearer desktop-token")
                .header("x-request-id", "invalid request id")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("response");

    let request_id = response.headers()["x-request-id"]
        .to_str()
        .expect("request id");
    assert!(request_id.starts_with("rust-"));
    assert_eq!(
        port.requests.lock().expect("requests")[0].request_id,
        request_id
    );
}

#[tokio::test]
async fn browser_write_requires_allowed_origin_and_csrf() {
    let (router, port) = fixture();
    let request = |csrf: Option<&str>| {
        let mut builder = Request::builder()
            .method(Method::PUT)
            .uri("/api/v1/settings/ui")
            .header("cookie", "jftrade_web_session=session-token")
            .header("origin", "https://jftrade.local");
        if let Some(csrf) = csrf {
            builder = builder.header("x-csrf-token", csrf);
        }
        builder.body(Body::from("{}")).expect("request")
    };
    let denied = router.clone().oneshot(request(None)).await.expect("denied");
    assert_eq!(denied.status(), StatusCode::FORBIDDEN);
    let accepted = router
        .oneshot(request(Some("csrf-token")))
        .await
        .expect("accepted");
    assert_eq!(accepted.status(), StatusCode::OK);
    assert_eq!(
        accepted.headers()["access-control-allow-origin"],
        "https://jftrade.local"
    );
    assert!(
        !port
            .requests
            .lock()
            .expect("requests")
            .last()
            .expect("accepted request")
            .desktop_trusted
    );
}

#[tokio::test]
async fn unknown_api_is_json_but_frontend_uses_spa_fallback() {
    let (router, _) = fixture();
    let unknown = router
        .clone()
        .oneshot(
            Request::builder()
                .uri("/api/v1/not-found")
                .header("authorization", "Bearer desktop-token")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("unknown");
    assert_eq!(unknown.status(), StatusCode::NOT_FOUND);
    assert_eq!(
        unknown.headers()["content-type"],
        "application/json; charset=utf-8"
    );

    let frontend = router
        .clone()
        .oneshot(
            Request::builder()
                .uri("/strategy")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("frontend");
    assert_eq!(frontend.status(), StatusCode::OK);
    assert_eq!(
        frontend.headers()["content-type"],
        "text/html; charset=utf-8"
    );

    let head = router
        .oneshot(
            Request::builder()
                .method(Method::HEAD)
                .uri("/strategy")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("head");
    assert_eq!(head.status(), StatusCode::OK);
    assert!(
        head.into_body()
            .collect()
            .await
            .expect("head body")
            .to_bytes()
            .is_empty()
    );
}
