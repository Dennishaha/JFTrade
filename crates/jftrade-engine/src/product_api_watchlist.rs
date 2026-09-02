impl ProductApi {
    fn watchlist_memberships(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let instrument_id = watchlist_membership_instrument_id(path)?;
        let port = self
            .watchlist_membership_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "WATCHLIST_UNAVAILABLE",
                    "watchlist membership snapshot is not configured",
                )
            })?;
        let memberships = port
            .memberships(&instrument_id)
            .map_err(|error| ApiFailure::new(503, "WATCHLIST_UNAVAILABLE", error.to_string()))?;
        Ok(ApiOutput::Json(json!(memberships)))
    }

    fn watchlist_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.watchlist_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "WATCHLIST_UNAVAILABLE",
                "watchlist read snapshot is not configured",
            )
        })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(watchlist_read_snapshot_failure)
    }
}
