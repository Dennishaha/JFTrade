//! Streaming lifecycle and terminal persistence for the production ADK model runtime.
//!
//! Keeping this implementation separate from provider selection and the
//! transport adapter keeps the runtime module below the repository's file-size
//! limit.  Run projections are committed with status compare-and-swap (CAS),
//! so a concurrent user cancellation cannot be overwritten by a late model
//! completion.

use std::collections::BTreeMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::SyncSender;

use jftrade_api::{ApiStream, ApiStreamSender};
use jftrade_store_sqlite::AdkRunEvent;
use serde_json::{Value, json};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatLiveStream, AdkChatPortError, AdkChatPortOutput, AdkChatRoute,
};

use super::{
    ChatExecution, ModelResponse, ProductionAdkChatRuntime, format_adk_error, storage_unavailable,
    stream_from_payload, unavailable,
};

impl ProductionAdkChatRuntime {
    pub(super) fn start_live_stream(
        &self,
        input: AdkChatInput,
        stream: ApiStream,
        sender: ApiStreamSender,
        started: SyncSender<Result<AdkChatPortOutput, AdkChatPortError>>,
    ) {
        let prepared = self.prepare_chat(AdkChatRoute::Stream, &input);
        let chat = match prepared {
            Ok(super::PreparedChat::Existing(output)) => {
                let _ = started.send(Ok(output));
                return;
            }
            Ok(super::PreparedChat::New(chat)) => chat,
            Err(error) => {
                let _ = started.send(Err(error));
                return;
            }
        };
        if sender.send(b"retry: 3000\n\n".to_vec()).is_err() {
            let disconnect = client_disconnected();
            let _ = self.persist_cancelled(&chat, &disconnect);
            let _ = started.send(Err(AdkChatPortError::Failed {
                status: 499,
                code: "CLIENT_DISCONNECTED".to_owned(),
                message: "assistant chat client disconnected".to_owned(),
            }));
            return;
        }
        let initial = json!({
            "type": "run",
            "run": {"id": chat.run_id, "sessionId": chat.session_id, "agentId": chat.agent_id, "status": "RUNNING"},
        });
        let initial_event = match self.emit_stream_event(&chat, initial, Some(&sender)) {
            Ok(event) => event,
            Err(error) => {
                if super::is_client_disconnect(&error) {
                    let _ = self.persist_cancelled(&chat, &error);
                }
                let _ = started.send(Err(error));
                return;
            }
        };
        if sender
            .send(super::encode_sse_event(&initial_event))
            .is_err()
        {
            let disconnect = client_disconnected();
            let _ = self.persist_cancelled(&chat, &disconnect);
            let _ = started.send(Err(disconnect));
            return;
        }
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let output = AdkChatPortOutput::LiveStream(AdkChatLiveStream {
            headers: BTreeMap::from([(String::from("X-ADK-Stream-ID"), chat.run_id.clone())]),
            stream,
        });
        if started.send(Ok(output)).is_err() {
            self.cancellation_registry.unregister(&chat.run_id);
            let _ = self.persist_cancelled(&chat, &client_disconnected());
            return;
        }
        self.run_live_stream(chat, sender, cancellation);
    }

    fn run_live_stream(
        &self,
        chat: super::ChatExecution,
        sender: ApiStreamSender,
        cancellation: Arc<AtomicBool>,
    ) {
        let _guard = super::CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
        };
        let result = super::stream_adapter::execute_model_stream(
            chat.request.clone(),
            |event| self.forward_provider_event(&chat, event, &sender),
            || {
                sender.is_closed()
                    || cancellation.load(Ordering::Acquire)
                    || self.run_is_cancelled(&chat.run_id)
            },
        );
        match result {
            Ok(model_response) if !model_response.tool_calls.is_empty() => {
                match self.persist_tool_calls(&chat, &model_response) {
                    Ok(AdkChatPortOutput::Json(response)) => {
                        let event = self
                            .emit_post_terminal_event(
                                &chat,
                                json!({"type": "pending", "response": response}),
                                "PENDING",
                            )
                            .unwrap_or_else(|_| {
                                json!({"type": "error", "message": "assistant tool call staging failed"})
                            });
                        let _ = sender.send(super::encode_sse_event(&event));
                    }
                    Ok(_) => {}
                    Err(error) => {
                        let event = self
                            .emit_post_terminal_event(
                                &chat,
                                json!({
                                    "type": "error",
                                    "message": super::format_adk_error(&error),
                                }),
                                "FAILED",
                            )
                            .unwrap_or_else(|_| {
                                json!({
                                    "type": "error",
                                    "message": super::format_adk_error(&error),
                                })
                            });
                        let _ = sender.send(super::encode_sse_event(&event));
                    }
                }
            }
            Ok(model_response) => match self.persist_success(&chat, model_response) {
                Ok(response) => {
                    if let Ok(Some(event)) = self.latest_stream_event(&chat.run_id) {
                        let _ = sender.send(super::encode_sse_event(&event));
                    } else {
                        let _ = sender.send(super::encode_sse_event(&json!({
                            "type": "final",
                            "response": response,
                        })));
                    }
                }
                Err(error) => {
                    if super::is_client_disconnect(&error)
                        || self.run_is_cancelled(&chat.run_id)
                        || super::is_run_cancelled(&error)
                    {
                        let _ = self.persist_cancelled(&chat, &error);
                        return;
                    }
                    let persisted = match self.persist_failure(&chat, &error) {
                        Ok(()) => true,
                        Err(persist_error)
                            if super::is_run_cancelled(&persist_error)
                                || self.run_is_cancelled(&chat.run_id) =>
                        {
                            return;
                        }
                        Err(_) => false,
                    };
                    let event = if persisted {
                        self.latest_stream_event(&chat.run_id)
                            .ok()
                            .flatten()
                            .unwrap_or_else(|| {
                                json!({"type":"error","message":super::format_adk_error(&error)})
                            })
                    } else {
                        json!({"type":"error","message":super::format_adk_error(&error)})
                    };
                    let _ = sender.send(super::encode_sse_event(&event));
                }
            },
            Err(error) => {
                if super::is_client_disconnect(&error)
                    || self.run_is_cancelled(&chat.run_id)
                    || super::is_run_cancelled(&error)
                {
                    let _ = self.persist_cancelled(&chat, &error);
                    return;
                }
                let persisted = match self.persist_failure(&chat, &error) {
                    Ok(()) => true,
                    Err(persist_error)
                        if super::is_run_cancelled(&persist_error)
                            || self.run_is_cancelled(&chat.run_id) =>
                    {
                        return;
                    }
                    Err(_) => false,
                };
                let event = if persisted {
                    self.latest_stream_event(&chat.run_id)
                        .ok()
                        .flatten()
                        .unwrap_or_else(
                            || json!({"type":"error","message":super::format_adk_error(&error)}),
                        )
                } else {
                    json!({"type":"error","message":super::format_adk_error(&error)})
                };
                let _ = sender.send(super::encode_sse_event(&event));
            }
        }
    }

    fn emit_post_terminal_event(
        &self,
        chat: &super::ChatExecution,
        mut event: Value,
        expected_status: &str,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(super::storage_unavailable)?
            .ok_or_else(|| super::unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case(expected_status) {
            return Err(self.run_state_changed(&chat.run_id));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| super::unavailable("persisted ADK run has no stream event list"))?;
        let sequence = events.len() as u64 + 1;
        if let Some(object) = event.as_object_mut() {
            object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
            object.insert("sequence".to_owned(), Value::from(sequence));
            object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        }
        events.push(event.clone());
        let updated = self
            .store()
            .update_run_payload_if_status(&chat.run_id, expected_status, &payload.to_string())
            .map_err(super::storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        self.record_event_with_id(
            &format!("{}:stream:{}", chat.run_id, sequence),
            &chat.run_id,
            &chat.session_id,
            "assistant.stream",
            event
                .get("message")
                .and_then(Value::as_str)
                .unwrap_or_default(),
        )?;
        Ok(event)
    }

    pub(super) fn forward_provider_event(
        &self,
        chat: &super::ChatExecution,
        provider_event: &Value,
        sender: &ApiStreamSender,
    ) -> Result<(), AdkChatPortError> {
        let prior_text = self.stream_text_prefix(&chat.run_id)?;
        self.append_provider_event(chat, provider_event)?;
        if provider_event.get("type").and_then(Value::as_str) != Some("response.output_text.delta")
        {
            return Ok(());
        }
        let Some(delta) = provider_event.get("delta").and_then(Value::as_str) else {
            return Ok(());
        };
        if delta.is_empty() {
            return Ok(());
        }
        let value = json!({
            "type": "timeline",
            "timeline": {
                "id": format!("{}:assistant", chat.run_id),
                "kind": "assistant_message",
                "status": "streaming",
                "text": format!("{prior_text}{delta}"),
            },
        });
        let event = self.emit_stream_event(chat, value, Some(sender))?;
        let _ = sender.send(super::encode_sse_event(&event));
        Ok(())
    }

    fn stream_text_prefix(&self, run_id: &str) -> Result<String, AdkChatPortError> {
        let Some(run) = self
            .store()
            .get_run(run_id)
            .map_err(super::storage_unavailable)?
        else {
            return Ok(String::new());
        };
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
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

    fn append_provider_event(
        &self,
        chat: &super::ChatExecution,
        event: &Value,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(super::storage_unavailable)?
            .ok_or_else(|| super::unavailable("persisted ADK run disappeared"))?;
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
        payload
            .get_mut("providerEvents")
            .and_then(Value::as_array_mut)
            .map(|events| events.push(event.clone()));
        let updated = self
            .store()
            .update_run_payload_if_status(&chat.run_id, "RUNNING", &payload.to_string())
            .map_err(super::storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        Ok(())
    }

    pub(super) fn emit_stream_event(
        &self,
        chat: &super::ChatExecution,
        mut event: Value,
        sender: Option<&ApiStreamSender>,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(super::storage_unavailable)?
            .ok_or_else(|| super::unavailable("persisted ADK run disappeared"))?;
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| super::unavailable("persisted ADK run has no stream event list"))?;
        let sequence = events.len() as u64 + 1;
        if let Some(object) = event.as_object_mut() {
            object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
            object.insert("sequence".to_owned(), Value::from(sequence));
            object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        }
        events.push(event.clone());
        let updated = self
            .store()
            .update_run_payload_if_status(&chat.run_id, "RUNNING", &payload.to_string())
            .map_err(super::storage_unavailable)?;
        if !updated {
            return Err(self.run_state_changed(&chat.run_id));
        }
        let content = event
            .pointer("/timeline/text")
            .and_then(Value::as_str)
            .or_else(|| event.pointer("/response/reply").and_then(Value::as_str))
            .unwrap_or_default();
        self.record_event_with_id(
            &format!("{}:stream:{}", chat.run_id, sequence),
            &chat.run_id,
            &chat.session_id,
            "assistant.stream",
            content,
        )?;
        if sender.is_some_and(ApiStreamSender::is_closed) {
            return Err(AdkChatPortError::Failed {
                status: 499,
                code: "CLIENT_DISCONNECTED".to_owned(),
                message: "assistant chat client disconnected".to_owned(),
            });
        }
        Ok(event)
    }

    pub(super) fn run_is_cancelled(&self, run_id: &str) -> bool {
        self.store()
            .get_run(run_id)
            .ok()
            .flatten()
            .is_some_and(|run| run.status.eq_ignore_ascii_case("CANCELLED"))
    }

    pub(super) fn run_state_changed(&self, run_id: &str) -> AdkChatPortError {
        if self.run_is_cancelled(run_id) {
            AdkChatPortError::Failed {
                status: 499,
                code: "RUN_CANCELLED".to_owned(),
                message: "assistant chat run was cancelled".to_owned(),
            }
        } else {
            super::unavailable("assistant chat run state changed while streaming")
        }
    }

    pub(super) fn latest_stream_event(
        &self,
        run_id: &str,
    ) -> Result<Option<Value>, AdkChatPortError> {
        let Some(run) = self
            .store()
            .get_run(run_id)
            .map_err(super::storage_unavailable)?
        else {
            return Ok(None);
        };
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(super::storage_unavailable)?;
        Ok(payload
            .get("streamEvents")
            .and_then(Value::as_array)
            .and_then(|events| events.last().cloned()))
    }

    pub(super) fn finish_chat(
        &self,
        chat: &ChatExecution,
        result: Result<ModelResponse, AdkChatPortError>,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match result {
            Ok(model_response) => {
                let response = self.persist_success(chat, model_response)?;
                if chat.route == AdkChatRoute::Chat {
                    Ok(AdkChatPortOutput::Json(response))
                } else {
                    let run = self
                        .store()
                        .get_run(&chat.run_id)
                        .map_err(storage_unavailable)?
                        .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
                    stream_from_payload(&run.payload_json)
                }
            }
            Err(error) => {
                let _ = self.persist_failure(chat, &error);
                Err(error)
            }
        }
    }

    pub(super) fn persist_success(
        &self,
        chat: &ChatExecution,
        model_response: ModelResponse,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Err(run_cancelled());
        }
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            if let Some(response) = super::persisted_response(&run.payload_json)? {
                return Ok(response);
            }
            return Err(unavailable(format!("run is already {}", run.status)));
        }
        let now = run.updated_at.clone();
        let session = self
            .store()
            .get_session(&chat.session_id)
            .map_err(storage_unavailable)?
            .map(|session| {
                json!({"id": session.id, "agentId": chat.agent_id, "createdAt": session.created_at, "updatedAt": session.updated_at})
            })
            .unwrap_or_else(|| json!({"id": chat.session_id, "agentId": chat.agent_id}));
        let text = model_response.text;
        let run_value = json!({
            "id": chat.run_id,
            "sessionId": chat.session_id,
            "agentId": chat.agent_id,
            "status": "COMPLETED",
            "message": text.clone(),
            "reply": text.clone(),
            "pendingApprovals": [],
            "createdAt": run.created_at,
            "updatedAt": now,
        });
        let timeline = json!({
            "id": format!("{}:assistant", chat.run_id),
            "kind": "assistant_message",
            "status": "final",
            "text": text.clone(),
        });
        let response = json!({
            "reply": text.clone(),
            "session": session,
            "run": run_value,
            "pendingApprovals": [],
            "timeline": [timeline],
        });
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        payload["id"] = Value::String(chat.run_id.clone());
        payload["sessionId"] = Value::String(chat.session_id.clone());
        payload["agentId"] = Value::String(chat.agent_id.clone());
        payload["status"] = Value::String("COMPLETED".to_owned());
        payload["reply"] = Value::String(text.clone());
        payload["response"] = response.clone();
        let mut final_sequence = None;
        if chat.route == AdkChatRoute::Stream {
            let events = payload
                .get_mut("streamEvents")
                .and_then(Value::as_array_mut)
                .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
            let sequence = events.len() as u64 + 1;
            let mut final_event = json!({"type":"final","response":response.clone()});
            if let Some(object) = final_event.as_object_mut() {
                object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
                object.insert("sequence".to_owned(), Value::from(sequence));
                object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
            }
            events.push(final_event);
            final_sequence = Some(sequence);
        }
        let assistant_event_id = format!("{}:{}", chat.run_id, chat.agent_id);
        let assistant_event = AdkRunEvent {
            id: &assistant_event_id,
            session_id: &chat.session_id,
            invocation_id: &chat.run_id,
            author: &chat.agent_id,
            content: &text,
        };
        let stream_event_id =
            final_sequence.map(|sequence| format!("{}:stream:{}", chat.run_id, sequence));
        let stream_event = stream_event_id.as_ref().map(|id| AdkRunEvent {
            id,
            session_id: &chat.session_id,
            invocation_id: &chat.run_id,
            author: "assistant.stream",
            content: response
                .get("reply")
                .and_then(Value::as_str)
                .unwrap_or_default(),
        });
        let mut events = vec![assistant_event];
        if let Some(stream_event) = stream_event.as_ref() {
            events.push(stream_event.clone());
        }
        let updated = self
            .store()
            .update_run_state_if_status_and_revision_with_events(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                "COMPLETED",
                &payload.to_string(),
                self.session_store.path(),
                &events,
            )
            .map_err(storage_unavailable)?;
        if !updated {
            let current = self
                .store()
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?;
            let Some(current) = current else {
                return Err(unavailable("persisted ADK run disappeared"));
            };
            if current.status.eq_ignore_ascii_case("CANCELLED") {
                return Err(run_cancelled());
            }
            if !current.status.eq_ignore_ascii_case("COMPLETED") {
                return Err(unavailable(
                    "assistant chat run state changed before completion",
                ));
            }
            return super::persisted_response(&current.payload_json)?
                .ok_or_else(|| unavailable("completed ADK run has no persisted response"));
        }
        Ok(response)
    }

    pub(super) fn persist_failure(
        &self,
        chat: &ChatExecution,
        error: &AdkChatPortError,
    ) -> Result<(), AdkChatPortError> {
        let message = match error {
            AdkChatPortError::Unavailable(message) | AdkChatPortError::Conflict(message) => {
                message.clone()
            }
            AdkChatPortError::Failed { code, message, .. } => format!("{code}: {message}"),
        };
        let status = if matches!(
            error,
            AdkChatPortError::Failed { code, .. } if code == "MODEL_CALL_TIMEOUT"
        ) {
            "TIMED_OUT"
        } else {
            "FAILED"
        };
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Err(run_cancelled());
        }
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            if matches!(
                run.status.to_ascii_uppercase().as_str(),
                "COMPLETED" | "FAILED" | "TIMED_OUT"
            ) {
                return Ok(());
            }
            return Err(unavailable(
                "assistant chat run state changed before failure",
            ));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        payload["id"] = Value::String(chat.run_id.clone());
        payload["status"] = Value::String(status.to_owned());
        payload["message"] = Value::String(message.clone());
        if chat.route == AdkChatRoute::Stream {
            let has_terminal = payload
                .get("streamEvents")
                .and_then(Value::as_array)
                .and_then(|events| events.last())
                .and_then(|event| event.get("type"))
                .and_then(Value::as_str)
                .is_some_and(|kind| matches!(kind, "final" | "error"));
            if !has_terminal {
                let event = json!({"type":"error","message":message});
                let _ = self.emit_stream_event(chat, event, None)?;
                payload = self
                    .store()
                    .get_run(&chat.run_id)
                    .map_err(storage_unavailable)?
                    .ok_or_else(|| unavailable("persisted ADK run disappeared"))
                    .and_then(|run| {
                        serde_json::from_str(&run.payload_json).map_err(storage_unavailable)
                    })?;
            }
            payload["status"] = Value::String(status.to_owned());
            payload["message"] = Value::String(message);
        }
        let updated = self
            .store()
            .update_run_state_if_status(&chat.run_id, "RUNNING", status, &payload.to_string())
            .map_err(storage_unavailable)?;
        if !updated
            && self
                .store()
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?
                .is_some_and(|run| run.status.eq_ignore_ascii_case("CANCELLED"))
        {
            return Err(run_cancelled());
        }
        Ok(())
    }

    pub(super) fn persist_cancelled(
        &self,
        chat: &ChatExecution,
        error: &AdkChatPortError,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Ok(());
        }
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Ok(());
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let has_terminal = payload
            .get("streamEvents")
            .and_then(Value::as_array)
            .and_then(|events| events.last())
            .and_then(|event| event.get("type"))
            .and_then(Value::as_str)
            .is_some_and(|kind| matches!(kind, "final" | "error"));
        if !has_terminal && chat.route == AdkChatRoute::Stream {
            let _ = self.emit_stream_event(
                chat,
                json!({"type":"error","message":format_adk_error(error)}),
                None,
            )?;
            payload = self
                .store()
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| unavailable("persisted ADK run disappeared"))
                .and_then(|run| {
                    serde_json::from_str(&run.payload_json).map_err(storage_unavailable)
                })?;
        }
        payload["status"] = Value::String("CANCELLED".to_owned());
        payload["message"] = Value::String(format_adk_error(error));
        let updated = self
            .store()
            .update_run_state_if_status(&chat.run_id, "RUNNING", "CANCELLED", &payload.to_string())
            .map_err(storage_unavailable)?;
        if !updated {
            return Ok(());
        }
        Ok(())
    }

    fn store(&self) -> &std::sync::Arc<jftrade_store_sqlite::AdkStore> {
        &self.store
    }
}

fn run_cancelled() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "RUN_CANCELLED".to_owned(),
        message: "assistant chat run was cancelled".to_owned(),
    }
}

fn client_disconnected() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "CLIENT_DISCONNECTED".to_owned(),
        message: "assistant chat client disconnected".to_owned(),
    }
}
