impl AdkChatStreamPort for ProductionAdkPort {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        if let Some(runtime) = self
            .chat_runtime
            .as_deref()
            .filter(|runtime| runtime.runtime_ready())
        {
            return runtime.dispatch(route, input);
        }
        if route == AdkChatRoute::Stream {
            return self.unavailable_streams.dispatch(input);
        }
        Err(AdkChatPortError::Unavailable(
            unavailable_stream::UNAVAILABLE.to_owned(),
        ))
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

    fn shutdown_with_error(&self) -> Result<(), AdkChatPortError> {
        match self.chat_runtime.as_deref() {
            Some(runtime) => runtime.shutdown_with_error(),
            None => Ok(()),
        }
    }
}
