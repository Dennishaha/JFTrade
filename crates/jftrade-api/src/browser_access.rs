//! Browser-navigation status page for listeners whose Web surface is closed.
//!
//! Parity baseline `go:452dea11`: `internal/app/apiserver/servercore/server_auth.go`
//! applied one access middleware to every request. When Web access was switched
//! off, a browser navigation (`GET`/`HEAD`, `Accept: text/html`, path outside
//! `/api/**` and `/swagger/**`) received a `403` HTML page telling the user to
//! enable Web access in the desktop settings, while API and WebSocket paths kept
//! their JSON error envelope. Rust keeps that split: the page is rendered here
//! and the transport only renders it when the listener reports the state.

use axum::body::Body;
use axum::http::header::{CACHE_CONTROL, X_CONTENT_TYPE_OPTIONS};
use axum::http::{HeaderValue, Response, StatusCode};

use crate::envelope::body_response;

/// Web access state reported by the listener that received the request.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum WebAccessState {
    /// Web access is switched off in the desktop settings.
    Disabled,
    /// Web access is switched on, so browser surfaces are served normally.
    Available,
}

/// Runtime source for the current Web access state.
///
/// The desktop settings can toggle Web access while the process serves, so the
/// transport asks a port instead of capturing a startup constant.
pub trait WebAccessStatePort: Send + Sync + std::fmt::Debug {
    fn web_access_state(&self) -> WebAccessState;
}

/// Go's friendly disabled page, kept byte-for-byte in spirit: same status,
/// cache and sniffing headers, same Chinese guidance text.
pub(crate) fn web_access_disabled_page(head: bool) -> Response<Body> {
    let mut response = body_response(
        StatusCode::FORBIDDEN,
        "text/html; charset=utf-8",
        if head {
            Body::empty()
        } else {
            Body::from(WEB_ACCESS_DISABLED_PAGE)
        },
    );
    response
        .headers_mut()
        .insert(CACHE_CONTROL, HeaderValue::from_static("no-store"));
    response
        .headers_mut()
        .insert(X_CONTENT_TYPE_OPTIONS, HeaderValue::from_static("nosniff"));
    response
}

const WEB_ACCESS_DISABLED_PAGE: &str = r#"<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Web 访问尚未开启 · JFTrade</title><style>body{margin:0;background:#f8fafc;color:#0f172a;font-family:system-ui,-apple-system,sans-serif}.card{max-width:34rem;margin:12vh auto;padding:2rem;border:1px solid #e2e8f0;border-radius:1rem;background:white;box-shadow:0 12px 32px #0f172a12}.brand{color:#0f766e;font-weight:700;letter-spacing:.16em}h1{font-size:1.4rem;margin:1rem 0 .7rem}p{margin:0;color:#475569;line-height:1.7}</style></head><body><main class="card"><div class="brand">JFTRADE</div><h1>Web 访问尚未开启</h1><p>请打开 JFTrade 桌面应用，在“设置 → Web 访问”中设置密码并主动开启。</p></main></body></html>"#;
