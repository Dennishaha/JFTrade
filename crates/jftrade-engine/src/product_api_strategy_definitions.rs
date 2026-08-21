impl ProductApi {
    fn strategy_definition_list(&self) -> Result<ApiOutput, ApiFailure> {
        let port = self.strategy_definition_port()?;
        port.list()
            .map(|items| ApiOutput::Json(json!(items)))
            .map_err(strategy_definition_snapshot_failure)
    }

    fn strategy_definition_port(
        &self,
    ) -> Result<&Arc<dyn StrategyDefinitionSnapshotPort>, ApiFailure> {
        self.strategy_definition_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "STRATEGY_DEFINITIONS_UNAVAILABLE",
                "strategy definition snapshot is not configured",
            )
        })
    }
}
