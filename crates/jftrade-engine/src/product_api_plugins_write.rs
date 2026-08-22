impl ProductApi {
    fn plugin_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_plugin_write(
            &PluginWriteRequest {
                method: request.method.clone(),
                path: request.path.clone(),
                body: Some(request.body.clone()),
            },
            self.plugin_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        plugin_write_output(response)
    }
}

fn plugin_write_output(response: PluginWriteResponse) -> Result<ApiOutput, ApiFailure> {
    if (200..300).contains(&response.status) {
        return Ok(ApiOutput::Json(
            response.body.get("data").cloned().unwrap_or(Value::Null),
        ));
    }
    let error = response.body.get("error").cloned().unwrap_or(Value::Null);
    let code = error
        .get("code")
        .and_then(Value::as_str)
        .unwrap_or("INTERNAL_ERROR");
    let message = error
        .get("message")
        .and_then(Value::as_str)
        .unwrap_or("plugin write failed");
    Err(ApiFailure::new(response.status, code, message))
}

fn is_plugin_write_path(path: &str) -> bool {
    ["/install", "/uninstall"].into_iter().any(|suffix| {
        path.strip_prefix("/api/v1/plugins/")
            .and_then(|value| value.strip_suffix(suffix))
            .is_some_and(|plugin_id| !plugin_id.is_empty() && !plugin_id.contains('/'))
    })
}
