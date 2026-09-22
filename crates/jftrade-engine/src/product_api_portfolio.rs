impl ProductApi {
    fn portfolio_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.portfolio_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "PORTFOLIO_UNAVAILABLE",
                "portfolio snapshot is not configured",
            )
        })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(|error| match error {
                PortfolioSnapshotError::Invalid(message) => {
                    ApiFailure::new(400, "BAD_REQUEST", message)
                }
                PortfolioSnapshotError::NotFound(message) => {
                    ApiFailure::new(404, "BROKER_NOT_FOUND", message)
                }
                other => ApiFailure::new(503, "PORTFOLIO_UNAVAILABLE", other.to_string()),
            })
    }
}
