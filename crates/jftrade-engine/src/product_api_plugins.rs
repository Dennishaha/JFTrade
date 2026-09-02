impl ProductApi {
    fn plugin_uninstall_guidance(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let plugin_id = plugin_uninstall_guidance_plugin_id(path)?;
        let port = self
            .plugin_uninstall_guidance_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE",
                    "plugin uninstall guidance snapshot is not configured",
                )
            })?;
        let guidance = port
            .guidance(&plugin_id)
            .map_err(|error| {
                ApiFailure::new(
                    503,
                    "PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE",
                    error.to_string(),
                )
            })?
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "plugin not found"))?;
        Ok(ApiOutput::Json(json!(guidance)))
    }

    fn plugin_catalog(&self) -> Result<ApiOutput, ApiFailure> {
        let port = self.plugin_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "PLUGINS_UNAVAILABLE",
                "plugin snapshot port is not configured",
            )
        })?;
        port.catalog()
            .map(ApiOutput::Json)
            .map_err(plugin_snapshot_failure)
    }

    fn plugin_operation(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let operation_id = plugin_operation_id(path)?;
        let port = self.plugin_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "PLUGINS_UNAVAILABLE",
                "plugin snapshot port is not configured",
            )
        })?;
        let operation = port
            .operation(&operation_id)
            .map_err(plugin_snapshot_failure)?
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "plugin operation not found"))?;
        Ok(ApiOutput::Json(operation))
    }
}
