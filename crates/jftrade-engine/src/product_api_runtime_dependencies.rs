impl ProductApi {
    async fn runtime_dependency_snapshot(
        &self,
    ) -> Result<runtime_dependencies::RuntimeDependencies, ApiFailure> {
        let settings = self
            .settings
            .pine_worker
            .settings()
            .map_err(settings_read_failure)?;
        Ok(runtime_dependencies::inspect(
            SystemClock.now_rfc3339(),
            &settings.node_binary_path,
        )
        .await)
    }

    async fn runtime_dependencies(&self) -> Result<ApiOutput, ApiFailure> {
        let dependencies = self.runtime_dependency_snapshot().await?;
        Ok(ApiOutput::Json(
            serde_json::to_value(dependencies)
                .expect("runtime dependency projection must be serializable"),
        ))
    }
}
