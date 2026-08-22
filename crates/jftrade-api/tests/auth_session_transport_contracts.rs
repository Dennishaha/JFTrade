use std::sync::Arc;

use axum::body::Body;
use axum::http::header::CACHE_CONTROL;
use axum::http::{Request, StatusCode};
use jftrade_api::{
    AccessPolicy, ApiFailure, ApiOutput, ApiPort, ApiRequest, ApiState, FixedClock, PortFuture,
    RouteCatalog, RouteSpec, build_router,
};
use serde_json::json;
use tower::ServiceExt;

#[derive(Clone, Copy)]
enum AuthSessionOutput {
    Success,
    OriginForbidden,
}

struct AuthSessionPort {
    output: AuthSessionOutput,
}

impl ApiPort for AuthSessionPort {
    fn dispatch(&self, _request: ApiRequest) -> PortFuture<'_> {
        let output = self.output;
        Box::pin(async move {
            match output {
                AuthSessionOutput::Success => Ok(ApiOutput::Json(json!({"authenticated": true}))),
                AuthSessionOutput::OriginForbidden => Err(ApiFailure::new(
                    403,
                    "ORIGIN_FORBIDDEN",
                    "request origin is not allowed",
                )),
            }
        })
    }
}

#[tokio::test]
async fn auth_session_sets_no_store_for_success_and_error_responses() {
    for (name, output, expected_status) in [
        ("success", AuthSessionOutput::Success, StatusCode::OK),
        (
            "origin forbidden",
            AuthSessionOutput::OriginForbidden,
            StatusCode::FORBIDDEN,
        ),
    ] {
        let routes = RouteCatalog::new([RouteSpec {
            method: "GET".into(),
            path: "/api/v1/auth/session".into(),
        }])
        .expect("auth-session route");
        let state = ApiState::new(
            routes,
            AccessPolicy::default(),
            Arc::new(AuthSessionPort { output }),
        )
        .with_clock(Arc::new(FixedClock("2026-08-22T00:00:00Z".into())));
        let response = build_router(state)
            .oneshot(
                Request::builder()
                    .uri("/api/v1/auth/session")
                    .header("x-request-id", "fixture-auth-session-id")
                    .body(Body::empty())
                    .expect("request"),
            )
            .await
            .expect("response");

        assert_eq!(response.status(), expected_status, "{name}");
        assert_eq!(
            response
                .headers()
                .get(CACHE_CONTROL)
                .and_then(|value| value.to_str().ok()),
            Some("no-store"),
            "{name}"
        );
        assert_eq!(
            response
                .headers()
                .get("x-request-id")
                .and_then(|value| value.to_str().ok()),
            Some("fixture-auth-session-id"),
            "{name}"
        );
    }
}
