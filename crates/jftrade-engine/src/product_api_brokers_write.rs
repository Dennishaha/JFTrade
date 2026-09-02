impl ProductApi {
    fn brokers_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_brokers_write(
            &BrokersWriteRequest {
                method: request.method.clone(),
                path: request_path_with_query(&request.path, &request.query),
                body: (!request.body.is_empty()).then(|| request.body.clone()),
                context: BrokersWriteContext::Normal,
            },
            self.stage9_write_ports.brokers.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        brokers_write_output(response)
    }
}

fn brokers_write_output(response: BrokersWriteResponse) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "BROKERS_WRITE_FAILED",
            "failed to encode brokers write response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_brokers_write_path(method: &str, path: &str) -> bool {
    brokers_write_routes().iter().any(|(route_method, route)| {
        *route_method == method && stage9_write_route_matches(path, route)
    })
}
