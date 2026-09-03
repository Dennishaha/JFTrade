#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AuthSessionSnapshotRequest {
    pub desktop_trusted: bool,
    pub browser_authenticated: bool,
    pub session_cookie: Option<String>,
    pub origin_provided: bool,
    pub origin_allowed: bool,
}

/// Session projection boundary owned by the Rust product composition.
/// Cookies, expiry, CSRF values, password verification, and invalidation stay
/// behind this port rather than leaking into the HTTP transport.
pub trait AuthSessionSnapshotPort: Send + Sync + std::fmt::Debug {
    fn session(
        &self,
        request: AuthSessionSnapshotRequest,
    ) -> Result<serde_json::Value, AuthSessionSnapshotError>;
}

#[derive(Clone, Debug, thiserror::Error)]
pub enum AuthSessionSnapshotError {
    #[error("auth-session snapshot is unavailable: {0}")]
    Unavailable(String),
}
