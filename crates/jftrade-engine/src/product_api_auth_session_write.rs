impl ProductApi {
    fn auth_session_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_auth_session_write(
            &AuthSessionWriteRequest {
                method: request.method.clone(),
                path: request.path.clone(),
                body: Some(request.body.clone()),
                desktop_trusted: request.desktop_trusted,
                browser_authenticated: request.browser_authenticated,
                origin_provided: request.origin_provided,
                origin_allowed: request.origin_allowed,
                csrf_valid: request.desktop_trusted || request.browser_authenticated,
                web_access_enabled: true,
                web_auth_available: true,
                session_cookie: None,
            },
            self.auth_session_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        auth_session_write_output(response)
    }
}

fn auth_session_write_output(
    response: AuthSessionWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            500,
            "WEB_AUTH_FAILED",
            "failed to encode auth-session response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_auth_session_write_path(method: &str, path: &str) -> bool {
    auth_session_write_routes()
        .iter()
        .any(|(route_method, route_path)| *route_method == method && *route_path == path)
}
