impl AdkChatStreamPort for ProductionAdkPort {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let Some(runtime) = self.chat_runtime.as_deref() else {
            return Err(AdkChatPortError::Unavailable(
                "assistant model runtime is unavailable; configure and attach a production model provider"
                    .to_owned(),
            ));
        };
        // A runtime object can be installed before its provider credentials
        // and model session are configured.  Do not let such a placeholder
        // delegate a synthetic success/stream through the production route;
        // readiness is the fail-closed boundary for both chat and stream.
        if !runtime.runtime_ready() {
            return Err(AdkChatPortError::Unavailable(
                "assistant model runtime is unavailable; configure and attach a production model provider"
                    .to_owned(),
            ));
        }
        runtime.dispatch(route, input)
    }

    fn cancel_run(&self, run_id: &str) -> bool {
        self.chat_runtime
            .as_deref()
            .is_some_and(|runtime| runtime.cancel_run(run_id))
    }

    fn resume_approval(&self, run_id: &str) -> Result<(), AdkChatPortError> {
        self.chat_runtime
            .as_deref()
            .ok_or_else(|| {
                AdkChatPortError::Unavailable(
                    "assistant approval continuation is unavailable".to_owned(),
                )
            })?
            .resume_approval(run_id)
    }

    fn runtime_ready(&self) -> bool {
        self.chat_runtime
            .as_deref()
            .is_some_and(AdkChatStreamPort::runtime_ready)
    }

    fn shutdown(&self) {
        if let Some(runtime) = self.chat_runtime.as_deref() {
            runtime.shutdown();
        }
    }
}
