impl ProductApi {
    fn system_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_system_write(
            &SystemWriteRequest {
                method: request.method.clone(),
                path: request_path_with_query(&request.path, &request.query),
                body: request.body.clone(),
            },
            self.stage9_write_ports.system.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        system_write_output(response)
    }
}

fn system_write_output(response: SystemWriteResponse) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "SYSTEM_WRITE_FAILED",
            "failed to encode system write response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_system_write_path(method: &str, path: &str) -> bool {
    system_write_routes().iter().any(|(route_method, route)| {
        *route_method == method && stage9_write_route_matches(path, route)
    })
}

fn stage9_write_route_matches(path: &str, template: &str) -> bool {
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
