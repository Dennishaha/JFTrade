impl ProductApi {
    fn auth_session(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        if request.origin_provided && !request.origin_allowed {
            return Err(ApiFailure::new(
                403,
                "ORIGIN_FORBIDDEN",
                "request origin is not allowed",
            ));
        }
        let port = self.auth_session_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "AUTH_SESSION_UNAVAILABLE",
                "auth-session snapshot is not configured",
            )
        })?;
        port.session(AuthSessionSnapshotRequest {
            desktop_trusted: request.desktop_trusted,
            browser_authenticated: request.browser_authenticated,
            origin_provided: request.origin_provided,
            origin_allowed: request.origin_allowed,
        })
        .map(ApiOutput::Json)
        .map_err(auth_session_snapshot_failure)
    }
}

fn auth_session_snapshot_failure(error: AuthSessionSnapshotError) -> ApiFailure {
    let AuthSessionSnapshotError::Unavailable(message) = error;
    ApiFailure::new(503, "AUTH_SESSION_UNAVAILABLE", message)
}
