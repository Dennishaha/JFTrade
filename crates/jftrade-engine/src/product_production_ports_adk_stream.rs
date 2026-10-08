impl AdkChatStreamPort for ProductionAdkPort {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        // Serialize transport admission through readiness and preparation;
        // concurrent POSTs cannot both execute before the error is retained.
        let _admission = (route == AdkChatRoute::Stream)
            .then(|| self.unavailable_streams.admit())
            .transpose()?;
        if route == AdkChatRoute::Stream {
            self.check_chat_request_conflict(input)?;
            if let Some(replay) = self.unavailable_streams.replay(input)? {
                return Ok(replay);
            }
        }
        if let Some(runtime) = self
            .chat_runtime
            .as_deref()
            .filter(|runtime| runtime.runtime_ready())
        {
            return match runtime.dispatch(route, input)? {
                AdkChatPortOutput::PreRunError(error) if route == AdkChatRoute::Stream => {
                    let message = match error {
                        AdkChatPortError::Unavailable(message)
                        | AdkChatPortError::Failed { message, .. } => message,
                        error @ AdkChatPortError::Conflict(_) => return Err(error),
                    };
                    self.unavailable_streams.retain_error(input, &message)
                }
                output => Ok(output),
            };
        }
        if route == AdkChatRoute::Stream {
            return self.unavailable_streams.dispatch(input);
        }
        Err(AdkChatPortError::Unavailable(
            unavailable_stream::UNAVAILABLE.to_owned(),
        ))
    }

    fn check_chat_request_conflict(&self, input: &AdkChatInput) -> Result<(), AdkChatPortError> {
        match self.chat_runtime.as_deref() {
            Some(runtime) => runtime.check_chat_request_conflict(input),
            None => Ok(()),
        }
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
