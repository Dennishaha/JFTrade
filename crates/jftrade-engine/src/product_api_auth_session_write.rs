impl ProductApi {
    fn auth_session_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let context = jftrade_api::current_request_context()
            .map(|context| {
                crate::product::product_auth_session_write_port::AuthSessionRequestContext {
                    client_key: context.client_key,
                    secure: context.secure,
                }
            })
            .unwrap_or_default();
        // Production auth state is projected from the persisted security
        // record and the listener runtime. A missing/unhealthy runtime must
        // fail closed; compatibility ports retain frozen fixture behavior so
        // transport replay remains isolated from a real listener.
        let (web_access_enabled, web_auth_available) = self
            .settings
            .security
            .web_access_projection()
            .unwrap_or_else(|_| {
                if self.production_routes.is_some() {
                    (false, false)
                } else {
                    (true, true)
                }
            });
        let response = crate::product::product_auth_session_write_port::dispatch_auth_session_write_with_context(
            &AuthSessionWriteRequest {
                method: request.method.clone(),
                path: request.path.clone(),
                body: Some(request.body.clone()),
                desktop_trusted: request.desktop_trusted,
                browser_authenticated: request.browser_authenticated,
                origin_provided: request.origin_provided,
                origin_allowed: request.origin_allowed,
                csrf_valid: request.csrf_valid,
                web_access_enabled,
                web_auth_available,
                session_cookie: request.session_cookie.clone(),
            },
            self.auth_session_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
            &context,
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
