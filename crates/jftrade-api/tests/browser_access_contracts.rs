use std::sync::Arc;

use axum::body::Body;
use axum::http::{HeaderMap, HeaderValue, Method, Request, StatusCode};
use http_body_util::BodyExt;
use jftrade_api::{
    AccessPolicy, ApiFailure, ApiPort, ApiRequest, ApiState, PortFuture, RouteCatalog, RouteSpec,
    WebAccessState, WebAccessStatePort, build_router,
};
use serde_json::Value;
use tower::ServiceExt;

struct UnroutedPort;

impl ApiPort for UnroutedPort {
    fn dispatch(&self, _request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async { Err(ApiFailure::new(404, "NOT_FOUND", "no api port")) })
    }
}

#[derive(Debug)]
struct FixedWebAccess(WebAccessState);

impl WebAccessStatePort for FixedWebAccess {
    fn web_access_state(&self) -> WebAccessState {
        self.0
    }
}

fn fixture(state: WebAccessState) -> axum::Router {
    let routes = RouteCatalog::new([RouteSpec {
        method: "GET".into(),
        path: "/api/v1/system/status".into(),
    }])
    .expect("routes");
    let access = AccessPolicy::desktop(Some("desktop-token".into()));
    let api_state = ApiState::new(routes, access, Arc::new(UnroutedPort))
        .with_browser_access(Arc::new(FixedWebAccess(state)));
    build_router(api_state)
}

async fn request(
    router: &axum::Router,
    method: Method,
    uri: &str,
    headers: &[(&str, &str)],
) -> (StatusCode, HeaderMap, Vec<u8>) {
    let mut builder = Request::builder().method(method).uri(uri);
    for (name, value) in headers {
        builder = builder.header(*name, HeaderValue::from_str(value).expect("header value"));
    }
    let response = router
        .clone()
        .oneshot(builder.body(Body::empty()).expect("request"))
        .await
        .expect("response");
    let status = response.status();
    let headers = response.headers().clone();
    let body = response
        .into_body()
        .collect()
        .await
        .expect("body")
        .to_bytes()
        .to_vec();
    (status, headers, body)
}

const BROWSER_ACCEPT: &str = "text/html,application/xhtml+xml";

fn error_code(body: &[u8]) -> String {
    let value: Value = serde_json::from_slice(body).expect("failure envelope");
    value["error"]["code"]
        .as_str()
        .expect("failure code")
        .to_owned()
}

fn content_type(headers: &HeaderMap) -> Option<&str> {
    headers
        .get("content-type")
        .and_then(|value| value.to_str().ok())
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:188 TestBrowserNavigationGetsFriendlyDisabledWebPage
#[tokio::test]
async fn browser_navigation_gets_the_friendly_disabled_page_while_web_access_is_off() {
    let router = fixture(WebAccessState::Disabled);
    let (status, headers, body) =
        request(&router, Method::GET, "/", &[("accept", BROWSER_ACCEPT)]).await;
    assert_eq!(status, StatusCode::FORBIDDEN);
    assert_eq!(headers["content-type"], "text/html; charset=utf-8");
    assert_eq!(headers["cache-control"], "no-store");
    assert_eq!(headers["x-content-type-options"], "nosniff");
    let page = String::from_utf8(body).expect("status page");
    assert!(page.contains("Web 访问尚未开启"), "{page}");
    assert!(page.contains("设置 → Web 访问"), "{page}");
}

#[tokio::test]
async fn disabled_page_covers_head_navigations_and_leaves_other_surfaces_alone() {
    let router = fixture(WebAccessState::Disabled);

    let (status, headers, body) = request(
        &router,
        Method::HEAD,
        "/strategy",
        &[("accept", BROWSER_ACCEPT)],
    )
    .await;
    assert_eq!(status, StatusCode::FORBIDDEN);
    assert_eq!(headers["content-type"], "text/html; charset=utf-8");
    assert_eq!(headers["cache-control"], "no-store");
    assert!(body.is_empty(), "HEAD navigations carry no body");

    let (status, _, body) = request(
        &router,
        Method::GET,
        "/api/v1/system/status",
        &[("accept", BROWSER_ACCEPT)],
    )
    .await;
    assert_eq!(status, StatusCode::UNAUTHORIZED);
    assert_eq!(error_code(&body), "WEB_AUTH_REQUIRED");

    let (status, _, body) = request(
        &router,
        Method::GET,
        "/swagger/parity-fixture-missing.js",
        &[("accept", BROWSER_ACCEPT)],
    )
    .await;
    assert_eq!(status, StatusCode::UNAUTHORIZED);
    assert_eq!(error_code(&body), "WEB_AUTH_REQUIRED");

    let (status, headers, body) =
        request(&router, Method::GET, "/", &[("accept", "application/json")]).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert_ne!(content_type(&headers), Some("text/html; charset=utf-8"));
    assert!(body.is_empty());

    let (status, _, body) = request(
        &router,
        Method::GET,
        "/",
        &[
            ("accept", BROWSER_ACCEPT),
            ("authorization", "Bearer desktop-token"),
        ],
    )
    .await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert!(body.is_empty());

    let (status, _, body) =
        request(&router, Method::POST, "/", &[("accept", BROWSER_ACCEPT)]).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert!(body.is_empty());
}

#[tokio::test]
async fn enabled_web_access_never_renders_the_disabled_page() {
    let router = fixture(WebAccessState::Available);
    let (status, headers, body) =
        request(&router, Method::GET, "/", &[("accept", BROWSER_ACCEPT)]).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    assert_ne!(content_type(&headers), Some("text/html; charset=utf-8"));
    assert!(body.is_empty());
}
