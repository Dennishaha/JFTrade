impl ProductApi {
    fn strategy_pine_analyze(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let response = dispatch_strategy_pine_analyze(
            self.strategy_pine_analyze_snapshot_port.as_deref(),
            "POST",
            STRATEGY_PINE_ANALYZE_PATH,
            body,
        );
        match response.error {
            Some(error) => {
                let failure = ApiFailure::new(response.status, error.code, error.message);
                if let Some(seconds) = error.retry_after_seconds {
                    Err(failure.with_retry_after(seconds))
                } else {
                    Err(failure)
                }
            }
            None => Ok(ApiOutput::Json(response.data.unwrap_or(Value::Null))),
        }
    }
}
