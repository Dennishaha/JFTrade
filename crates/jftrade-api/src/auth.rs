use std::collections::BTreeSet;

use axum::http::HeaderMap;

pub const DESKTOP_WEBSOCKET_PROTOCOL: &str = "jftrade.desktop.v1";
pub const INTERNAL_PROXY_PROTOCOL_HEADER: &str = "x-jftrade-internal-proxy";
pub const ACCESS_SURFACE_HEADER: &str = "x-jftrade-access-surface";
const SESSION_COOKIE: &str = "jftrade_web_session";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AccessPolicy {
    pub desktop_token: Option<String>,
    pub session_token: Option<String>,
    pub csrf_token: Option<String>,
    pub allowed_origins: BTreeSet<String>,
    pub enforce_access: bool,
    pub desktop_mode: bool,
    pub internal_proxy_protocol: Option<String>,
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
        }
    }
}

impl AccessPolicy {
    pub fn with_allowed_origins(mut self, origins: impl IntoIterator<Item = String>) -> Self {
        self.allowed_origins = origins
            .into_iter()
            .filter_map(|value| canonical_origin(&value))
            .collect();
        self
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
        let Some(expected) = self.session_token.as_deref() else {
            return false;
        };
        cookie_value(headers, SESSION_COOKIE)
            .is_some_and(|actual| constant_time_equal(actual, expected))
    }

    pub(crate) fn csrf_valid(&self, headers: &HeaderMap) -> bool {
        let Some(expected) = self.csrf_token.as_deref() else {
            return false;
        };
        headers
            .get("x-csrf-token")
            .and_then(|value| value.to_str().ok())
            .is_some_and(|actual| constant_time_equal(actual.trim(), expected))
    }

    pub(crate) fn origin_allowed(&self, headers: &HeaderMap) -> bool {
        let Some(origin) = request_origin(headers) else {
            return false;
        };
        self.allowed_origins.contains(&origin)
    }
}

pub fn canonical_origin(value: &str) -> Option<String> {
    let value = value.trim();
    let (scheme, rest) = value.split_once("://")?;
    if !matches!(
        scheme.to_ascii_lowercase().as_str(),
        "http" | "https" | "tauri" | "wails"
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
    fn origin_normalization_matches_go_scheme_and_authority_rules() {
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
