use axum::http::HeaderMap;

use crate::AccessPolicy;
use crate::auth::{origin_provided, request_origin};

pub const DEFAULT_WEBSOCKET_LIMIT: usize = 20;

pub fn websocket_origin_allowed(headers: &HeaderMap, policy: &AccessPolicy) -> bool {
    if !origin_provided(headers) {
        return true;
    }
    request_origin(headers).is_some_and(|origin| policy.allowed_origins.contains(&origin))
}
