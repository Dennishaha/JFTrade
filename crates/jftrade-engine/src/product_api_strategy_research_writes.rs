impl ProductApi {
    fn product_write_mutation(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        if is_research_screen_write_path(&request.method, &request.path) {
            let response = dispatch_research_screen_write(
                &ResearchScreenWriteRequest {
                    method: request.method.clone(),
                    path: request_path_with_query(&request.path, &request.query),
                    body: Some(request.body.clone()),
                },
                self.research_screen_write_port.as_deref(),
                &SystemClock.now_rfc3339(),
            );
            return write_mutation_output(
                response.status,
                response.body,
                "research screen write failed",
            );
        }
        if is_research_preset_write_path(&request.method, &request.path) {
            let response = dispatch_research_preset_write(
                &ResearchPresetWriteRequest {
                    method: request.method.clone(),
                    path: request_path_with_query(&request.path, &request.query),
                    body: Some(request.body.clone()),
                },
                self.research_preset_write_port.as_deref(),
                &SystemClock.now_rfc3339(),
            );
            return research_preset_write_output(response);
        }
        if is_strategy_definition_write_path(&request.method, &request.path) {
            let response = dispatch_strategy_definition_write(
                request,
                self.strategy_definition_write_port.as_deref(),
                &SystemClock.now_rfc3339(),
            );
            return strategy_definition_write_output(response);
        }
        Err(ApiFailure::new(404, "NOT_FOUND", "resource not found"))
    }
}

fn research_preset_write_output(
    response: ResearchPresetWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    write_mutation_output(
        response.status,
        response.body,
        "research preset write failed",
    )
}

fn strategy_definition_write_output(
    response: StrategyDefinitionWriteResponse,
) -> Result<ApiOutput, ApiFailure> {
    write_mutation_output(
        response.status,
        response.body,
        "strategy definition write failed",
    )
}

fn write_mutation_output(
    status: u16,
    body: serde_json::Value,
    fallback_message: &str,
) -> Result<ApiOutput, ApiFailure> {
    if (200..300).contains(&status) {
        return Ok(ApiOutput::Json(body.get("data").cloned().unwrap_or(body)));
    }
    let error = body
        .get("error")
        .cloned()
        .unwrap_or(serde_json::Value::Null);
    let code = error
        .get("code")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("INTERNAL_ERROR");
    let message = error
        .get("message")
        .and_then(serde_json::Value::as_str)
        .unwrap_or(fallback_message);
    Err(ApiFailure::new(status, code, message))
}

fn is_product_write_path(method: &str, path: &str) -> bool {
    is_research_screen_write_path(method, path)
        || is_research_preset_write_path(method, path)
        || is_strategy_definition_write_path(method, path)
}

fn is_research_screen_write_path(method: &str, path: &str) -> bool {
    method == "POST" && path == RESEARCH_SCREEN_PATH
}

fn is_research_preset_write_path(method: &str, path: &str) -> bool {
    if method == "POST" && path == "/api/v1/research/screens/presets" {
        return true;
    }
    if !matches!(method, "PATCH" | "DELETE") {
        return false;
    }
    path.strip_prefix("/api/v1/research/screens/presets/")
        .is_some_and(|preset_id| !preset_id.is_empty() && !preset_id.contains('/'))
}

fn is_strategy_definition_write_path(method: &str, path: &str) -> bool {
    if method == "POST" && path == "/api/v1/strategy-definitions" {
        return true;
    }
    let Some(raw_id) = path.strip_prefix("/api/v1/strategy-definitions/") else {
        return false;
    };
    if method == "POST" {
        return ["/apply-linked-instances", "/instantiate"]
            .into_iter()
            .any(|suffix| {
                raw_id.strip_suffix(suffix).is_some_and(|definition_id| {
                    !definition_id.is_empty() && !definition_id.contains('/')
                })
            });
    }
    matches!(method, "PUT" | "DELETE") && !raw_id.is_empty() && !raw_id.contains('/')
}
