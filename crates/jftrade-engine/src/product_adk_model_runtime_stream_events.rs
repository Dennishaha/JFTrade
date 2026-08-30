//! Durable event projection helpers for the ADK stream runtime.

use jftrade_api::ApiStreamSender;
use jftrade_store_sqlite::AdkRunEvent;
use serde_json::Value;

use super::{
    AdkChatPortError, ChatExecution, ProductionAdkChatRuntime, RunLeaseGuard, storage_unavailable,
    unavailable,
};

impl ProductionAdkChatRuntime {
    pub(super) fn emit_post_terminal_event(
        &self,
        chat: &ChatExecution,
        mut event: Value,
        expected_status: &str,
        run_lease: &RunLeaseGuard,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case(expected_status) {
            return Err(self.run_state_changed(&chat.run_id));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
        let sequence = events.len() as u64 + 1;
        if let Some(object) = event.as_object_mut() {
            object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
            object.insert("sequence".to_owned(), Value::from(sequence));
            object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        }
        events.push(event.clone());
        let event_id = format!("{}:stream:{}", chat.run_id, sequence);
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                expected_status,
                &run.updated_at,
                &payload.to_string(),
                self.session_store.path(),
                &[adk_run_event(
                    &event_id,
                    &chat.session_id,
                    &chat.run_id,
                    "assistant.stream",
                    event
                        .get("message")
                        .and_then(Value::as_str)
                        .unwrap_or_default(),
                )],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        Ok(event)
    }

    pub(super) fn stream_text_prefix(&self, run_id: &str) -> Result<String, AdkChatPortError> {
        let Some(run) = self.store().get_run(run_id).map_err(storage_unavailable)? else {
            return Ok(String::new());
        };
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let mut text = String::new();
        if let Some(events) = payload.get("streamEvents").and_then(Value::as_array) {
            for event in events {
                if event.get("type").and_then(Value::as_str) != Some("timeline") {
                    continue;
                }
                if let Some(value) = event.pointer("/timeline/text").and_then(Value::as_str) {
                    text = value.to_owned();
                }
            }
        }
        Ok(text)
    }

    pub(super) fn append_provider_event(
        &self,
        chat: &ChatExecution,
        event: &Value,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let provider_events = payload
            .get_mut("providerEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| unavailable("persisted ADK run has no provider event list"))?;
        let sequence = provider_events.len() as u64 + 1;
        provider_events.push(event.clone());
        let event_id = format!("{}:provider:{}", chat.run_id, sequence);
        let event_content = event.to_string();
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                self.session_store.path(),
                &[adk_run_event(
                    &event_id,
                    &chat.session_id,
                    &chat.run_id,
                    "assistant.provider",
                    &event_content,
                )],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        Ok(())
    }

    pub(super) fn emit_stream_event(
        &self,
        chat: &ChatExecution,
        mut event: Value,
        sender: Option<&ApiStreamSender>,
        run_lease: &RunLeaseGuard,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
        let sequence = events.len() as u64 + 1;
        if let Some(object) = event.as_object_mut() {
            object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
            object.insert("sequence".to_owned(), Value::from(sequence));
            object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        }
        events.push(event.clone());
        let event_id = format!("{}:stream:{}", chat.run_id, sequence);
        let content = event
            .pointer("/timeline/text")
            .and_then(Value::as_str)
            .or_else(|| event.pointer("/response/reply").and_then(Value::as_str))
            .unwrap_or_default();
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                self.session_store.path(),
                &[adk_run_event(
                    &event_id,
                    &chat.session_id,
                    &chat.run_id,
                    "assistant.stream",
                    content,
                )],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        if sender.is_some_and(ApiStreamSender::is_closed) {
            return Err(AdkChatPortError::Failed {
                status: 499,
                code: "CLIENT_DISCONNECTED".to_owned(),
                message: "assistant chat client disconnected".to_owned(),
            });
        }
        Ok(event)
    }
}

fn adk_run_event<'a>(
    id: &'a str,
    session_id: &'a str,
    invocation_id: &'a str,
    author: &'a str,
    content: &'a str,
) -> AdkRunEvent<'a> {
    AdkRunEvent {
        id,
        session_id,
        invocation_id,
        author,
        content,
    }
}
