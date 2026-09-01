impl ProductionAdkChatRuntime {
    /// Check durable model configuration without issuing a network request or
    /// creating a run. Transient endpoint outages remain request-level errors.
    pub(crate) fn runtime_ready(&self) -> bool {
        let Some(recovery_supervisor) = self.recovery_supervisor.as_ref() else {
            // Short-lived continuation facades intentionally do not own the
            // process-wide scanner and are never exposed as route ports.
            return false;
        };
        if !recovery_supervisor.is_ready() {
            if let Some(error) = recovery_supervisor.startup_error() {
                eprintln!("{error}; ADK runtime readiness is unavailable");
            }
            return false;
        }
        if self
            .continuation_supervisor
            .stopping
            .load(std::sync::atomic::Ordering::Acquire)
        {
            return false;
        }
        let Ok(providers) = self.store.list_providers() else {
            return false;
        };
        let Ok(secrets) = read_secrets(&self.secrets_path) else {
            return false;
        };
        providers.iter().any(|provider| {
            let Ok(value) = serde_json::from_str::<Value>(&provider.payload_json) else {
                return false;
            };
            if !value
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(true)
            {
                return false;
            }
            let Some(base_url) = value.get("baseUrl").and_then(Value::as_str) else {
                return false;
            };
            if responses_endpoint(base_url).is_err() {
                return false;
            }
            let model = value
                .get("model")
                .and_then(Value::as_str)
                .map(str::trim)
                .unwrap_or_default();
            if model.is_empty() {
                return false;
            }
            secrets
                .get(&provider.id)
                .map(String::as_str)
                .or_else(|| value.get("apiKey").and_then(Value::as_str))
                .is_some_and(|key| !key.trim().is_empty())
        })
    }
}
