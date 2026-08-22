impl ProductApi {
    fn remote_watchlist_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_remote_watchlist_write(
            &RemoteWatchlistWriteRequest {
                method: request.method.clone(),
                path: request_path_with_query(&request.path, &request.query),
                body: Some(request.body.clone()),
            },
            self.remote_watchlist_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        remote_watchlist_write_output(response)
    }
}

fn remote_watchlist_write_output(
    response: RemoteWatchlistWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "BROKER_FEATURE_FAILED",
            "failed to encode remote watchlist response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_remote_watchlist_write_path(method: &str, path: &str) -> bool {
    remote_watchlist_write_routes()
        .iter()
        .any(|(route_method, route_path)| *route_method == method && *route_path == path)
}
