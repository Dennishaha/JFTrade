impl ProductApi {
    fn alert_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let path = request_path_with_query(&request.path, &request.query);
        let response = dispatch_alert_write(
            &AlertWriteRequest {
                method: request.method.clone(),
                path,
                body: Some(request.body.clone()),
            },
            self.alert_write_port.as_deref(),
            &SystemClock.now_rfc3339(),
        );
        alert_write_output(response)
    }
}

fn request_path_with_query(path: &str, query: &str) -> String {
    if query.is_empty() {
        path.to_owned()
    } else {
        format!("{path}?{query}")
    }
}

fn alert_write_output(response: AlertWriteResponse) -> Result<ApiOutput, ApiFailure> {
    if (200..300).contains(&response.status) {
        return Ok(ApiOutput::Json(
            response.body.get("data").cloned().unwrap_or(Value::Null),
        ));
    }
    let error = response.body.get("error").cloned().unwrap_or(Value::Null);
    let code = error
        .get("code")
        .and_then(Value::as_str)
        .unwrap_or("BROKER_FEATURE_FAILED");
    let message = error
        .get("message")
        .and_then(Value::as_str)
        .unwrap_or("alert write failed");
    let mut failure = ApiFailure::new(response.status, code, message);
    if let Some(retry_after) = response
        .headers
        .get("Retry-After")
        .and_then(|value| value.parse::<u64>().ok())
    {
        failure = failure.with_retry_after(retry_after);
    }
    Err(failure)
}
