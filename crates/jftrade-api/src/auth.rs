use std::collections::BTreeSet;
use std::sync::Arc;

use axum::http::HeaderMap;

pub const DESKTOP_WEBSOCKET_PROTOCOL: &str = "jftrade.desktop.v1";
pub const INTERNAL_PROXY_PROTOCOL_HEADER: &str = "x-jftrade-internal-proxy";
pub const ACCESS_SURFACE_HEADER: &str = "x-jftrade-access-surface";
pub const SESSION_COOKIE: &str = "jftrade_web_session";

pub trait WebSessionValidator: Send + Sync + std::fmt::Debug {
    fn is_session_valid(&self, session_cookie: &str) -> bool;
    fn is_csrf_valid(&self, session_cookie: &str, csrf_header: &str) -> bool;
}

/// Optional runtime origin provider used by listeners whose bind port can be
/// changed without rebuilding the process. Static trusted origins remain the
/// compatibility baseline; a provider may add only narrowly scoped origins
/// for its currently active listener.
pub trait AccessOriginProvider: Send + Sync + std::fmt::Debug {
    fn allows_origin(&self, origin: &str) -> bool;
}

#[derive(Clone, Debug)]
pub struct AccessPolicy {
    pub desktop_token: Option<String>,
    pub session_token: Option<String>,
    pub csrf_token: Option<String>,
    pub allowed_origins: BTreeSet<String>,
    pub enforce_access: bool,
    pub desktop_mode: bool,
    pub internal_proxy_protocol: Option<String>,
    pub session_validator: Option<Arc<dyn WebSessionValidator>>,
    pub dynamic_origin_provider: Option<Arc<dyn AccessOriginProvider>>,
}

impl PartialEq for AccessPolicy {
    fn eq(&self, other: &Self) -> bool {
        self.desktop_token == other.desktop_token
            && self.session_token == other.session_token
            && self.csrf_token == other.csrf_token
            && self.allowed_origins == other.allowed_origins
            && self.enforce_access == other.enforce_access
            && self.desktop_mode == other.desktop_mode
            && self.internal_proxy_protocol == other.internal_proxy_protocol
            && self.session_validator.is_some() == other.session_validator.is_some()
            && self.dynamic_origin_provider.is_some() == other.dynamic_origin_provider.is_some()
    }
}

impl Eq for AccessPolicy {}

pub fn desktop_trusted_origins() -> impl IntoIterator<Item = String> {
    [
        "http://127.0.0.1:3003".to_owned(),
        "http://localhost:3003".to_owned(),
        "http://127.0.0.1:3000".to_owned(),
        "http://localhost:3000".to_owned(),
        "http://127.0.0.1:3008".to_owned(),
        "http://localhost:3008".to_owned(),
        "http://127.0.0.1:6699".to_owned(),
        "http://localhost:6699".to_owned(),
        "tauri://localhost".to_owned(),
        "http://tauri.localhost".to_owned(),
        "https://tauri.localhost".to_owned(),
    ]
}

impl Default for AccessPolicy {
    fn default() -> Self {
        Self {
            desktop_token: None,
            session_token: None,
            csrf_token: None,
            allowed_origins: BTreeSet::new(),
            enforce_access: true,
            desktop_mode: false,
            internal_proxy_protocol: None,
            session_validator: None,
            dynamic_origin_provider: None,
        }
    }
}

impl AccessPolicy {
    pub fn desktop(desktop_token: Option<String>) -> Self {
        Self {
            desktop_token,
            desktop_mode: true,
            enforce_access: true,
            ..Self::default()
        }
        .with_allowed_origins(desktop_trusted_origins())
    }

    pub fn with_allowed_origins(mut self, origins: impl IntoIterator<Item = String>) -> Self {
        self.allowed_origins = origins
            .into_iter()
            .filter_map(|value| canonical_origin(&value))
            .collect();
        self
    }

    pub fn with_session_validator(mut self, validator: Arc<dyn WebSessionValidator>) -> Self {
        self.session_validator = Some(validator);
        self
    }

    pub fn with_dynamic_origin_provider(mut self, provider: Arc<dyn AccessOriginProvider>) -> Self {
        self.dynamic_origin_provider = Some(provider);
        self
    }

    /// Browser-facing policy.  Unlike the desktop policy it never treats a
    /// bearer token as trusted; sessions are established only by the web
    /// login flow and are checked through the session cookie/CSRF pair.
    pub fn web() -> Self {
        Self {
            desktop_mode: false,
            enforce_access: true,
            ..Self::default()
        }
        .with_allowed_origins([
            "http://127.0.0.1:3003".to_owned(),
            "http://localhost:3003".to_owned(),
            "http://127.0.0.1:3000".to_owned(),
            "http://localhost:3000".to_owned(),
        ])
    }

    pub(crate) fn desktop_trusted(&self, headers: &HeaderMap) -> bool {
        let Some(expected) = self.desktop_token.as_deref() else {
            return self.desktop_mode && !self.enforce_access;
        };
        request_desktop_token(headers).is_some_and(|actual| constant_time_equal(actual, expected))
    }

    pub(crate) fn internal_proxy_trusted(&self, headers: &HeaderMap) -> bool {
        let Some(expected) = self.internal_proxy_protocol.as_deref() else {
            return false;
        };
        let protocol_matches = headers
            .get(INTERNAL_PROXY_PROTOCOL_HEADER)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|actual| constant_time_equal(actual.trim(), expected));
        let surface_valid = headers
            .get(ACCESS_SURFACE_HEADER)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| matches!(value.trim(), "desktop" | "web"));
        let bearer_matches = self.desktop_token.as_deref().is_some_and(|expected| {
            request_bearer_token(headers)
                .is_some_and(|actual| constant_time_equal(actual, expected))
        });
        protocol_matches && surface_valid && bearer_matches
    }

    pub(crate) fn browser_authenticated(&self, headers: &HeaderMap) -> bool {
        let cookie = self.session_cookie(headers);
        let validator_valid = self
            .session_validator
            .as_ref()
            .zip(cookie.as_deref())
            .is_some_and(|(validator, cookie)| validator.is_session_valid(cookie));
        if validator_valid {
            return true;
        }
        let Some(expected) = self.session_token.as_deref() else {
            return false;
        };
        cookie
            .as_deref()
            .is_some_and(|actual| constant_time_equal(actual, expected))
    }

    pub(crate) fn session_cookie(&self, headers: &HeaderMap) -> Option<String> {
        cookie_value(headers, SESSION_COOKIE).map(ToOwned::to_owned)
    }

    pub(crate) fn csrf_valid(&self, headers: &HeaderMap) -> bool {
        let cookie = self.session_cookie(headers);
        let csrf_header = headers
            .get("x-csrf-token")
            .and_then(|value| value.to_str().ok())
            .map(str::trim);
        let validator_valid = self
            .session_validator
            .as_ref()
            .zip(cookie.as_deref())
            .is_some_and(|(validator, cookie)| {
                csrf_header.is_some_and(|csrf| validator.is_csrf_valid(cookie, csrf))
            });
        if validator_valid {
            return true;
        }
        let Some(expected) = self.csrf_token.as_deref() else {
            return false;
        };
        csrf_header.is_some_and(|actual| constant_time_equal(actual, expected))
    }

    pub(crate) fn origin_allowed(&self, headers: &HeaderMap) -> bool {
        let Some(origin) = request_origin(headers) else {
            return false;
        };
        self.allowed_origins.contains(&origin)
            || self
                .dynamic_origin_provider
                .as_ref()
                .is_some_and(|provider| provider.allows_origin(&origin))
    }
}

pub fn canonical_origin(value: &str) -> Option<String> {
    let value = value.trim();
    let (scheme, rest) = value.split_once("://")?;
    if !matches!(
        scheme.to_ascii_lowercase().as_str(),
        "http" | "https" | "tauri"
    ) {
        return None;
    }
    let authority = rest.split('/').next()?.trim().to_lowercase();
    if authority.is_empty() || authority.contains('@') || authority.contains(char::is_whitespace) {
        return None;
    }
    Some(format!("{}://{authority}", scheme.to_ascii_lowercase()))
}

pub(crate) fn request_origin(headers: &HeaderMap) -> Option<String> {
    for name in ["origin", "referer"] {
        if let Some(value) = headers.get(name).and_then(|value| value.to_str().ok())
            && !value.trim().is_empty()
        {
            return canonical_origin(value);
        }
    }
    None
}

pub(crate) fn origin_provided(headers: &HeaderMap) -> bool {
    ["origin", "referer"].into_iter().any(|name| {
        headers
            .get(name)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| !value.trim().is_empty())
    })
}

fn request_desktop_token(headers: &HeaderMap) -> Option<&str> {
    if let Some(value) = request_bearer_token(headers) {
        return Some(value);
    }
    headers
        .get("sec-websocket-protocol")
        .and_then(|value| value.to_str().ok())
        .and_then(|value| {
            value
                .split(',')
                .map(str::trim)
                .find(|protocol| !protocol.is_empty() && *protocol != DESKTOP_WEBSOCKET_PROTOCOL)
        })
}

fn request_bearer_token(headers: &HeaderMap) -> Option<&str> {
    headers
        .get("authorization")
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.trim().strip_prefix("Bearer "))
        .filter(|value| !value.trim().is_empty())
        .map(str::trim)
}

fn cookie_value<'a>(headers: &'a HeaderMap, name: &str) -> Option<&'a str> {
    headers
        .get("cookie")
        .and_then(|value| value.to_str().ok())
        .and_then(|cookies| {
            cookies.split(';').find_map(|cookie| {
                let (cookie_name, value) = cookie.trim().split_once('=')?;
                (cookie_name == name && !value.trim().is_empty()).then_some(value.trim())
            })
        })
}

fn constant_time_equal(left: &str, right: &str) -> bool {
    let left = left.as_bytes();
    let right = right.as_bytes();
    let length = left.len().max(right.len());
    let mut difference = left.len() ^ right.len();
    for index in 0..length {
        difference |= usize::from(
            left.get(index).copied().unwrap_or(0) ^ right.get(index).copied().unwrap_or(0),
        );
    }
    difference == 0
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn origin_normalization_accepts_web_and_tauri_schemes() {
        assert_eq!(
            canonical_origin(" HTTPS://Example.COM/path?q=1 "),
            Some("https://example.com".into())
        );
        assert_eq!(canonical_origin("file://host/path"), None);
        assert_eq!(
            canonical_origin("tauri://localhost/index.html"),
            Some("tauri://localhost".into())
        );
        assert_eq!(canonical_origin("not-an-origin"), None);
    }
}
