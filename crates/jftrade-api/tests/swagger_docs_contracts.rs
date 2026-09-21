use std::sync::Arc;

use axum::body::Body;
use axum::http::{HeaderMap, Method, Request, StatusCode};
use http_body_util::BodyExt;
use jftrade_api::{
    AccessPolicy, ApiFailure, ApiPort, ApiRequest, ApiState, PortFuture, RouteCatalog, RouteSpec,
    SwaggerDocs, build_router,
};
use serde_json::Value;
use tower::ServiceExt;

const SWAGGER_DOCUMENT: &str = include_str!("../../../contracts/openapi/openapi.json");

struct UnroutedPort;

impl ApiPort for UnroutedPort {
    fn dispatch(&self, _request: ApiRequest) -> PortFuture<'_> {
        Box::pin(async { Err(ApiFailure::new(404, "NOT_FOUND", "no api port")) })
    }
}

fn fixture() -> axum::Router {
    let routes = RouteCatalog::new([RouteSpec {
        method: "GET".into(),
        path: "/api/v1/system/status".into(),
    }])
    .expect("routes");
    let access = AccessPolicy {
        desktop_token: Some("desktop-token".into()),
        ..AccessPolicy::default()
    };
    let state = ApiState::new(routes, access, Arc::new(UnroutedPort))
        .with_swagger_docs(SwaggerDocs::new(SWAGGER_DOCUMENT));
    build_router(state)
}

async fn request(
    router: &axum::Router,
    method: Method,
    uri: &str,
    authorized: bool,
) -> (StatusCode, HeaderMap, Vec<u8>) {
    let mut builder = Request::builder().method(method).uri(uri);
    if authorized {
        builder = builder.header("authorization", "Bearer desktop-token");
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

async fn get(router: &axum::Router, uri: &str) -> (StatusCode, HeaderMap, Vec<u8>) {
    request(router, Method::GET, uri, true).await
}

// Parity: go:452dea11:internal/app/apiserver/servercore/openapi_boundary_test.go:11 TestSwaggerRoutesRedirectToBrowsableDocumentation
#[tokio::test]
async fn swagger_roots_redirect_to_the_browsable_documentation_entry() {
    let router = fixture();
    for uri in ["/swagger", "/swagger/"] {
        let (status, headers, body) = get(&router, uri).await;
        assert_eq!(status, StatusCode::TEMPORARY_REDIRECT, "{uri}");
        assert_eq!(headers["location"], "/swagger/index.html", "{uri}");
        assert!(body.is_empty(), "{uri}");
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/swagger_openapi_test.go:14 TestSwaggerUIAvailable
#[tokio::test]
async fn swagger_ui_is_available_offline_without_external_assets() {
    let router = fixture();

    let (status, headers, body) = get(&router, "/swagger/index.html").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["content-type"], "text/html; charset=utf-8");
    let html = String::from_utf8(body).expect("html document");
    assert!(html.contains("/swagger/docs.js"), "{html}");
    assert!(html.contains("/swagger/doc.json"), "{html}");
    for external in ["cdn.jsdelivr.net", "http://", "https://"] {
        assert!(
            !html.contains(external),
            "external reference {external}: {html}"
        );
    }

    let (status, headers, body) = get(&router, "/swagger/docs.css").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["content-type"], "text/css; charset=utf-8");
    assert!(!body.is_empty());

    let (status, headers, body) = get(&router, "/swagger/docs.js").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(
        headers["content-type"],
        "application/javascript; charset=utf-8"
    );
    let viewer = String::from_utf8(body).expect("viewer script");
    assert!(viewer.contains("/swagger/doc.json"), "{viewer}");

    let (status, headers, _) = get(&router, "/swagger/doc.json").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["content-type"], "application/json; charset=utf-8");

    let (status, headers, body) = request(&router, Method::HEAD, "/swagger/index.html", true).await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["content-type"], "text/html; charset=utf-8");
    assert!(body.is_empty());
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/swagger_openapi_test.go:72 TestOpenAPISpecExposesCorePaths
#[tokio::test]
async fn swagger_document_exposes_the_frozen_core_paths() {
    let router = fixture();
    let (status, _, body) = get(&router, "/swagger/doc.json").await;
    assert_eq!(status, StatusCode::OK);
    let spec: Value = serde_json::from_slice(&body).expect("swagger document");
    assert_eq!(spec["swagger"], "2.0");
    assert_eq!(spec["info"]["title"], "JFTrade Debug API");
    for path in [
        "/api/v1/system/status",
        "/api/v1/market-data/candles/{market}/{symbol}",
        "/api/v1/ws/live",
    ] {
        assert!(spec["paths"].get(path).is_some(), "missing {path}");
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercoretest/openapi_schema_compatibility_test.go:14 TestOpenAPIPreservesLegacySchemaNames
#[tokio::test]
async fn swagger_document_preserves_legacy_schema_names() {
    let router = fixture();
    let (status, _, body) = get(&router, "/swagger/doc.json").await;
    assert_eq!(status, StatusCode::OK);
    let spec: Value = serde_json::from_slice(&body).expect("swagger document");
    let definitions = spec["definitions"].as_object().expect("definitions");
    for name in [
        "adk.Agent",
        "adk.AgentWriteRequest",
        "adk.Approval",
        "adk.ApprovalResolution",
        "adk.AuditEvent",
        "adk.ChatResponse",
        "adk.InputAnswer",
        "adk.InputOption",
        "adk.InputQuestion",
        "adk.InputRequest",
        "adk.InputResolution",
        "adk.MemoryEntry",
        "adk.Provider",
        "adk.Run",
        "adk.RunOptions",
        "adk.RunUsage",
        "adk.Session",
        "adk.SessionComposerState",
        "adk.SessionContextBreakdown",
        "adk.SessionContextSnapshot",
        "adk.SessionsResponse",
        "adk.Skill",
        "adk.Task",
        "adk.TimelineEntry",
        "adk.ToolCall",
        "adk.ToolDescriptor",
        "adk.TranscriptEntry",
        "adk.WorkflowCanvasEdge",
        "adk.WorkflowCanvasGraph",
        "adk.WorkflowCanvasNode",
        "adk.WorkflowCanvasPoint",
        "adk.WorkflowDefinition",
        "adk.WorkflowNodeRun",
        "adk.WorkflowResult",
        "adk.WorkflowStepState",
        "adk.WorkflowTrigger",
        "adk.WorkflowTriggerLog",
        "servercore.WebSessionData",
        "servercore.webLoginRequest",
    ] {
        assert!(definitions.contains_key(name), "missing schema {name}");
    }
    for name in definitions.keys() {
        assert!(!name.starts_with("model."), "internal schema {name}");
        assert!(
            !name.starts_with("workflowruntime."),
            "internal schema {name}"
        );
        assert_ne!(name, "webaccess.WebSessionData", "internal schema {name}");
        assert_ne!(name, "webaccess.webLoginRequest", "internal schema {name}");
    }
}

#[tokio::test]
async fn swagger_surface_requires_access_and_keeps_the_json_envelope() {
    let router = fixture();

    let (status, _, body) = request(&router, Method::GET, "/swagger/", false).await;
    assert_eq!(status, StatusCode::UNAUTHORIZED);
    let failure: Value = serde_json::from_slice(&body).expect("failure envelope");
    assert_eq!(failure["error"]["code"], "WEB_AUTH_REQUIRED");

    let (status, _, body) = get(&router, "/swagger/missing.js").await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    let failure: Value = serde_json::from_slice(&body).expect("failure envelope");
    assert_eq!(failure["error"]["code"], "NOT_FOUND");

    let (status, _, _) = request(&router, Method::POST, "/swagger/doc.json", true).await;
    assert_eq!(status, StatusCode::NOT_FOUND);
}
