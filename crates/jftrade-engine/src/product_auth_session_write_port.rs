//! Product boundary for browser authentication session mutations.
//!
//! This leaf parses the two public route shapes and delegates password
//! verification, browser-session storage, CSRF values, expiry, and cookie
//! invalidation to the Rust-owned state port installed by product composition.

use std::collections::BTreeMap;

use serde::Deserialize;
use serde_json::{Value, json};
use thiserror::Error;

pub const AUTH_LOGIN_PATH: &str = "/api/v1/auth/login";
pub const AUTH_LOGOUT_PATH: &str = "/api/v1/auth/logout";

pub const AUTH_SESSION_WRITE_ROUTES: [(&str, &str); 2] =
    [("POST", AUTH_LOGIN_PATH), ("POST", AUTH_LOGOUT_PATH)];

/// Transport-only request metadata used by the production auth owner.
///
/// It intentionally lives outside `AuthSessionWriteRequest`: the latter is
/// the frozen compatibility route fixture shape. Runtime callers can supply the
/// peer/client key and whether the request arrived through a trusted TLS
/// proxy without changing the public HTTP or fixture contract.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct AuthSessionRequestContext {
    pub client_key: String,
    pub secure: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AuthSessionWriteRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
    pub desktop_trusted: bool,
    pub browser_authenticated: bool,
    pub origin_provided: bool,
    pub origin_allowed: bool,
    pub csrf_valid: bool,
    pub web_access_enabled: bool,
    pub web_auth_available: bool,
    pub session_cookie: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AuthSessionWriteInput {
    Login { password: String },
    Logout { session_cookie: Option<String> },
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AuthSessionWritePortResult {
    pub data: Value,
    pub set_cookie: Option<String>,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum AuthSessionWritePortError {
    #[error("web password authentication is unavailable")]
    Unavailable(String),
    #[error("invalid Web access password")]
    InvalidPassword(String),
    #[error("login request was canceled")]
    Canceled(String),
    #[error("Web access settings changed during login; try again")]
    ConfigurationChanged(String),
    #[error("failed to create session")]
    Failed(String),
    #[error("too many failed login attempts")]
    RateLimited { retry_after: u64, message: String },
}

/// Consumer-owned state boundary for login and logout.
pub trait AuthSessionWritePort: Send + Sync + std::fmt::Debug {
    fn login_rate_limit(&self) -> Option<AuthSessionWritePortError> {
        None
    }

    fn login_rate_limit_with_context(
        &self,
        _context: &AuthSessionRequestContext,
    ) -> Option<AuthSessionWritePortError> {
        self.login_rate_limit()
    }

    fn mutate(
        &self,
        input: &AuthSessionWriteInput,
    ) -> Result<AuthSessionWritePortResult, AuthSessionWritePortError>;

    fn mutate_with_context(
        &self,
        input: &AuthSessionWriteInput,
        _context: &AuthSessionRequestContext,
    ) -> Result<AuthSessionWritePortResult, AuthSessionWritePortError> {
        self.mutate(input)
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AuthSessionWriteResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn auth_session_write_routes() -> &'static [(&'static str, &'static str); 2] {
    &AUTH_SESSION_WRITE_ROUTES
}

#[allow(dead_code)]
pub fn dispatch_auth_session_write(
    request: &AuthSessionWriteRequest,
    port: Option<&dyn AuthSessionWritePort>,
    timestamp: &str,
) -> AuthSessionWriteResponse {
    dispatch_auth_session_write_with_context(
        request,
        port,
        timestamp,
        &AuthSessionRequestContext::default(),
    )
}

pub fn dispatch_auth_session_write_with_context(
    request: &AuthSessionWriteRequest,
    port: Option<&dyn AuthSessionWritePort>,
    timestamp: &str,
    context: &AuthSessionRequestContext,
) -> AuthSessionWriteResponse {
    let (path, _) = split_path_query(&request.path);
    match (request.method.as_str(), path) {
        ("POST", AUTH_LOGIN_PATH) => dispatch_login(request, port, timestamp, context),
        ("POST", AUTH_LOGOUT_PATH) => dispatch_logout(request, port, timestamp, context),
        _ => error_response(404, "NOT_FOUND", "resource not found", timestamp),
    }
}

fn dispatch_login(
    request: &AuthSessionWriteRequest,
    port: Option<&dyn AuthSessionWritePort>,
    timestamp: &str,
    context: &AuthSessionRequestContext,
) -> AuthSessionWriteResponse {
    if request.origin_provided && !request.origin_allowed {
        return error_response(
            403,
            "ORIGIN_FORBIDDEN",
            "request origin is not allowed",
            timestamp,
        );
    }
    if request.desktop_trusted {
        return success_response(
            json!({"authenticated": true, "csrfToken": ""}),
            None,
            timestamp,
        );
    }
    if !request.web_access_enabled {
        return error_response(
            403,
            "WEB_ACCESS_DISABLED",
            "Web access is disabled; enable it in the desktop settings",
            timestamp,
        );
    }
    if !request.web_auth_available {
        return error_response(
            503,
            "WEB_AUTH_UNAVAILABLE",
            "Web password authentication is unavailable",
            timestamp,
        );
    }
    let Some(port) = port else {
        return error_response(
            503,
            "AUTH_SESSION_WRITE_UNAVAILABLE",
            "auth-session write port is unavailable",
            timestamp,
        );
    };
    if let Some(error) = port.login_rate_limit_with_context(context) {
        return port_error_response(error, timestamp);
    }
    let Some(password) = parse_login_password(request.body.as_deref()) else {
        return error_response(400, "BAD_REQUEST", "invalid login payload", timestamp);
    };
    match port.mutate_with_context(&AuthSessionWriteInput::Login { password }, context) {
        Ok(result) => success_response(result.data, result.set_cookie, timestamp),
        Err(error) => port_error_response(error, timestamp),
    }
}

fn dispatch_logout(
    request: &AuthSessionWriteRequest,
    port: Option<&dyn AuthSessionWritePort>,
    timestamp: &str,
    context: &AuthSessionRequestContext,
) -> AuthSessionWriteResponse {
    if request.origin_provided && !request.origin_allowed {
        return middleware_error_response(
            403,
            "ORIGIN_FORBIDDEN",
            "request origin is not allowed",
            timestamp,
        );
    }
    if !request.desktop_trusted && !request.web_access_enabled {
        return middleware_error_response(
            403,
            "WEB_ACCESS_DISABLED",
            "Web access is disabled; enable it in the desktop settings",
            timestamp,
        );
    }
    if !request.desktop_trusted && !request.browser_authenticated {
        return middleware_error_response(
            401,
            "WEB_AUTH_REQUIRED",
            "Web password authentication is required",
            timestamp,
        );
    }
    if !request.desktop_trusted && !request.origin_provided {
        return middleware_error_response(
            403,
            "ORIGIN_FORBIDDEN",
            "write request origin is not allowed",
            timestamp,
        );
    }
    if !request.desktop_trusted && !request.csrf_valid {
        return middleware_error_response(
            403,
            "CSRF_FAILED",
            "valid CSRF token is required",
            timestamp,
        );
    }
    let Some(port) = port else {
        return error_response(
            503,
            "AUTH_SESSION_WRITE_UNAVAILABLE",
            "auth-session write port is unavailable",
            timestamp,
        );
    };
    let input = AuthSessionWriteInput::Logout {
        session_cookie: request.session_cookie.clone(),
    };
    match port.mutate_with_context(&input, context) {
        Ok(result) => success_response(result.data, result.set_cookie, timestamp),
        Err(error) => port_error_response(error, timestamp),
    }
}

fn parse_login_password(body: Option<&[u8]>) -> Option<String> {
    let body = body?;
    let mut deserializer = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut deserializer).ok()?;
    let Value::Object(object) = value else {
        return value.is_null().then(String::new);
    };
    match object.get("password") {
        None | Some(Value::Null) => Some(String::new()),
        Some(Value::String(password)) => Some(password.clone()),
        Some(_) => None,
    }
}

fn port_error_response(
    error: AuthSessionWritePortError,
    timestamp: &str,
) -> AuthSessionWriteResponse {
    match error {
        AuthSessionWritePortError::Unavailable(message) => {
            error_response(503, "WEB_AUTH_UNAVAILABLE", &message, timestamp)
        }
        AuthSessionWritePortError::InvalidPassword(message) => {
            error_response(401, "INVALID_PASSWORD", &message, timestamp)
        }
        AuthSessionWritePortError::Canceled(message) => {
            error_response(408, "REQUEST_CANCELED", &message, timestamp)
        }
        AuthSessionWritePortError::ConfigurationChanged(message) => {
            error_response(409, "WEB_AUTH_CONFIGURATION_CHANGED", &message, timestamp)
        }
        AuthSessionWritePortError::Failed(message) => {
            error_response(500, "WEB_AUTH_FAILED", &message, timestamp)
        }
        AuthSessionWritePortError::RateLimited {
            retry_after,
            message,
        } => rate_limited_response(retry_after, &message, timestamp),
    }
}

fn rate_limited_response(
    retry_after: u64,
    message: &str,
    timestamp: &str,
) -> AuthSessionWriteResponse {
    let mut response = error_response(429, "LOGIN_RATE_LIMITED", message, timestamp);
    response
        .headers
        .insert("Retry-After".to_owned(), retry_after.max(1).to_string());
    response
}

fn success_response(
    data: Value,
    set_cookie: Option<String>,
    timestamp: &str,
) -> AuthSessionWriteResponse {
    let mut response = AuthSessionWriteResponse {
        status: 200,
        headers: json_headers(true),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    };
    if let Some(set_cookie) = set_cookie {
        response.headers.insert("Set-Cookie".to_owned(), set_cookie);
    }
    response
}

fn error_response(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
) -> AuthSessionWriteResponse {
    error_response_with_cache(status, code, message, timestamp, true)
}

fn middleware_error_response(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
) -> AuthSessionWriteResponse {
    error_response_with_cache(status, code, message, timestamp, false)
}

fn error_response_with_cache(
    status: u16,
    code: &str,
    message: &str,
    timestamp: &str,
    cache_control: bool,
) -> AuthSessionWriteResponse {
    AuthSessionWriteResponse {
        status,
        headers: json_headers(cache_control),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers(cache_control: bool) -> BTreeMap<String, String> {
    let mut headers = BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )]);
    if cache_control {
        headers.insert("Cache-Control".to_owned(), "no-store".to_owned());
    }
    headers
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}
