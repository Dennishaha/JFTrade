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
        if plugin_operation_path_has_invalid_escape(path) {
            return Err(ApiFailure::new(
                400,
                "BAD_REQUEST",
                "operationId is required",
            ));
        }
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

fn plugin_operation_path_has_invalid_escape(path: &str) -> bool {
    let Some(encoded) = path.strip_prefix("/api/v1/plugins/operations/") else {
        return false;
    };
    let bytes = encoded.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] == b'%' {
            if index + 2 >= bytes.len()
                || !bytes[index + 1].is_ascii_hexdigit()
                || !bytes[index + 2].is_ascii_hexdigit()
            {
                return true;
            }
            index += 3;
        } else {
            index += 1;
        }
    }
    false
}
