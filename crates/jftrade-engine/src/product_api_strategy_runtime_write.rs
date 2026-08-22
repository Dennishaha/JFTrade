impl ProductApi {
    fn strategy_runtime_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_strategy_runtime_write(
            request,
            self.strategy_runtime_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        strategy_runtime_write_output(response)
    }
}

fn strategy_runtime_write_output(
    response: StrategyRuntimeWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "STRATEGY_FAILED",
            "failed to encode strategy runtime response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_strategy_runtime_write_path(method: &str, path: &str) -> bool {
    strategy_runtime_write_routes()
        .iter()
        .any(|(route_method, route)| {
            *route_method == method && strategy_runtime_route_matches(path, route)
        })
}

fn strategy_runtime_route_matches(path: &str, template: &str) -> bool {
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
