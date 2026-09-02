impl ProductApi {
    fn execution_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_execution_write(
            &ExecutionWriteRequest {
                method: request.method.clone(),
                path: request_path_with_query(&request.path, &request.query),
                body: (!request.body.is_empty()).then(|| request.body.clone()),
                context: ExecutionWriteContext::Normal,
            },
            self.stage9_write_ports.execution.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        execution_write_output(response)
    }
}

fn execution_write_output(
    response: ExecutionWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "EXECUTION_WRITE_FAILED",
            "failed to encode execution write response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_execution_write_path(method: &str, path: &str) -> bool {
    execution_write_routes().iter().any(|(route_method, route)| {
        *route_method == method && stage9_write_route_matches(path, route)
    })
}
