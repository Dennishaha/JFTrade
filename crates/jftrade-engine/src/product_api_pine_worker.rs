impl ProductApi {
    fn save_pine_worker_settings(&self, body: &[u8]) -> Result<ApiOutput, ApiFailure> {
        let input: PineWorkerSettings = serde_json::from_slice(body)
            .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "invalid Pine worker payload"))?;
        self.settings
            .pine_worker
            .save(&input)
            .map(|settings| ApiOutput::Json(json!(settings)))
            .map_err(settings_failure)
    }
}
