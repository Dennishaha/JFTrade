use std::sync::Arc;

use axum::body::Body;
use axum::http::header::CONTENT_TYPE;
use axum::http::{HeaderValue, Response, StatusCode};
use serde_json::{Value, json};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

pub trait Clock: Send + Sync {
    fn now_rfc3339(&self) -> String;
}

#[derive(Clone, Debug, Default)]
pub struct SystemClock;

impl Clock for SystemClock {
    fn now_rfc3339(&self) -> String {
        OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
    }
}

#[derive(Clone, Debug)]
pub struct FixedClock(pub String);

impl Clock for FixedClock {
    fn now_rfc3339(&self) -> String {
        self.0.clone()
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ApiFailure {
    pub status: u16,
    pub code: String,
    pub message: String,
}

impl ApiFailure {
    pub fn new(status: u16, code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            status,
            code: code.into(),
            message: message.into(),
        }
    }
}

pub(crate) fn success_response(clock: &Arc<dyn Clock>, data: Value) -> Response<Body> {
    json_response(
        StatusCode::OK,
        json!({
            "ok": true,
            "data": data,
            "timestamp": clock.now_rfc3339(),
        }),
    )
}

pub(crate) fn error_response(clock: &Arc<dyn Clock>, failure: ApiFailure) -> Response<Body> {
    let status = StatusCode::from_u16(failure.status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR);
    json_response(
        status,
        json!({
            "ok": false,
            "error": {
                "code": failure.code,
                "message": failure.message,
            },
            "timestamp": clock.now_rfc3339(),
        }),
    )
}

pub(crate) fn empty_response(status: StatusCode) -> Response<Body> {
    Response::builder()
        .status(status)
        .body(Body::empty())
        .expect("status-only response")
}

pub(crate) fn body_response(
    status: StatusCode,
    content_type: &str,
    body: impl Into<Body>,
) -> Response<Body> {
    let mut response = Response::builder()
        .status(status)
        .body(body.into())
        .expect("body response");
    if let Ok(content_type) = HeaderValue::from_str(content_type) {
        response.headers_mut().insert(CONTENT_TYPE, content_type);
    }
    response
}

fn json_response(status: StatusCode, value: Value) -> Response<Body> {
    let body = serde_json::to_vec(&value).unwrap_or_else(|_| b"{}".to_vec());
    body_response(status, "application/json; charset=utf-8", body)
}
