impl ProductApi {
    fn adk_chat_stream(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        let stream_idle_timeout_ms = self
            .settings
            .assistant_runtime
            .settings()
            .map_err(settings_read_failure)?
            .stream_idle_timeout_ms as u64;
        let response = product_adk_chat_stream_port::dispatch_adk_chat(
            &product_adk_chat_stream_port::AdkChatRequest {
                method: request.method.clone(),
                path: request.path.clone(),
                body: request.body.clone(),
            },
            self.adk_chat_stream_port.as_deref(),
            &SystemClock.now_rfc3339(),
            stream_idle_timeout_ms,
        );
        Ok(ApiOutput::Raw {
            status: response.status(),
            content_type: response
                .headers()
                .get("Content-Type")
                .cloned()
                .unwrap_or_else(|| "application/octet-stream".to_owned()),
            body: response.body().into_bytes(),
            headers: response.headers().clone(),
        })
    }
}
