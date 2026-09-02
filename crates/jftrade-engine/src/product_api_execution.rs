impl ProductApi {
    fn execution_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.execution_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "EXECUTION_UNAVAILABLE",
                "execution read snapshot is not configured",
            )
        })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(execution_read_snapshot_failure)
    }
}

fn is_execution_read_path(path: &str) -> bool {
    path == "/api/v1/execution/orders"
        || path
            .strip_prefix("/api/v1/execution/orders/")
            .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
        || path
            .strip_prefix("/api/v1/execution/orders/")
            .and_then(|suffix| suffix.strip_suffix("/events"))
            .is_some_and(|id| !id.is_empty() && !id.contains('/'))
}

fn execution_read_snapshot_failure(error: ExecutionReadSnapshotError) -> ApiFailure {
    match error {
        ExecutionReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "EXECUTION_UNAVAILABLE", message)
        }
        ExecutionReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "BAD_REQUEST", message)
        }
        ExecutionReadSnapshotError::NotFound => {
            ApiFailure::new(404, "ORDER_NOT_FOUND", "execution order not found")
        }
        ExecutionReadSnapshotError::Failed { code, message } => ApiFailure::new(500, code, message),
    }
}
