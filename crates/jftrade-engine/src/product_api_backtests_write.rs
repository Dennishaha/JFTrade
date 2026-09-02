impl ProductApi {
    fn backtests_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_backtests_write(
            &BacktestsWriteRequest {
                method: request.method.clone(),
                path: request_path_with_query(&request.path, &request.query),
                body: Some(request.body.clone()),
            },
            self.backtests_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        backtests_write_output(response)
    }
}

fn backtests_write_output(
    response: BacktestsWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "BACKTESTS_WRITE_FAILED",
            "failed to encode backtests write response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_backtests_write_path(method: &str, path: &str) -> bool {
    backtests_write_routes().iter().any(|(route_method, route)| {
        *route_method == method && backtests_route_template_matches(path, route)
    })
}

fn backtests_route_template_matches(path: &str, template: &str) -> bool {
    let actual = path
        .split('?')
        .next()
        .unwrap_or(path)
        .split('/')
        .collect::<Vec<_>>();
    let expected = template.split('/').collect::<Vec<_>>();
    actual.len() == expected.len()
        && actual.iter().zip(expected).all(|(actual, expected)| {
            expected.starts_with('{') || actual == &expected
        })
}
