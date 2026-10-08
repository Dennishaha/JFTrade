//! Authenticated auxiliary reads reuse the registered HTTP port and route boundary.
use std::sync::Arc;

use axum::http::HeaderMap;
use serde_json::Value;

use crate::router::ApiState;
use crate::{ApiOutput, ApiPort, ApiRequest, Clock, RouteCatalog};

#[derive(Clone)]
pub(crate) struct WebsocketRead {
    port: Arc<dyn ApiPort>,
    clock: Arc<dyn Clock>,
    routes: RouteCatalog,
    request: ApiRequest,
}

impl WebsocketRead {
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
        }
    }

    pub(crate) fn now(&self) -> String {
        self.clock.now_rfc3339()
    }

    pub(crate) async fn query(&self, path: String, query: String) -> Option<Value> {
        if !self.routes.allows("GET", &path) {
            return None;
        }
        let mut input = self.request.clone();
        input.path = path;
        input.query = query;
        let result =
            tokio::time::timeout(std::time::Duration::from_secs(2), self.port.dispatch(input))
                .await;
        match result {
            Ok(Ok(ApiOutput::Json(payload))) if payload.is_object() => Some(payload),
            _ => None,
        }
    }
}

pub(crate) fn query_component(value: &str) -> String {
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
