impl ProductApi {
    fn adk_mutation(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_adk_mutation(
            &AdkMutationRequest {
                method: request.method.clone(),
                path: request_path_with_query(&request.path, &request.query),
                body: Some(request.body.clone()),
                headers: std::collections::BTreeMap::new(),
            },
            self.adk_mutation_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        adk_mutation_output(response)
    }
}

fn adk_mutation_output(response: AdkMutationResponse) -> Result<ApiOutput, ApiFailure> {
    let content_type = response
        .headers
        .get("Content-Type")
        .cloned()
        .unwrap_or_else(|| "application/json; charset=utf-8".to_owned());
    let body = serde_json::to_vec(&response.body).map_err(|_| {
        ApiFailure::new(
            502,
            "ADK_MUTATIONS_FAILED",
            "failed to encode ADK mutation response",
        )
    })?;
    Ok(ApiOutput::Raw {
        status: response.status,
        content_type,
        body,
        headers: response.headers,
    })
}

fn is_adk_mutation_path(method: &str, path: &str) -> bool {
    adk_mutation_routes().iter().any(|(route_method, route)| {
        *route_method == method && adk_mutation_route_matches(path, route)
    })
}

fn adk_mutation_route_matches(path: &str, template: &str) -> bool {
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
