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
use std::thread;
use std::time::Duration;

use jftrade_api::{ApiStream, ApiStreamSender};
use jftrade_store_sqlite::AdkRunEvent;
use serde_json::{Value, json};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatLiveStream, AdkChatPortError, AdkChatPortOutput, AdkChatRoute,
    AdkChatStreamFrame, AdkChatStreamSnapshot,
};

use super::{
    ChatExecution, ModelResponse, ProductionAdkChatRuntime, RunLeaseGuard, run_cancelled,
    storage_unavailable, stream_from_payload, unavailable,
};

#[path = "product_adk_model_runtime_stream_events.rs"]
mod stream_events;

impl ProductionAdkChatRuntime {
    pub(super) fn start_live_stream(
        &self,
        input: AdkChatInput,
        stream: ApiStream,
        sender: ApiStreamSender,
        started: SyncSender<Result<AdkChatPortOutput, AdkChatPortError>>,
        supervisor_cancel: Arc<AtomicBool>,
    ) {
        let prepared = self.prepare_chat(AdkChatRoute::Stream, &input);
        let chat = match prepared {
            Ok(super::PreparedChat::Existing(AdkChatPortOutput::Stream(snapshot)))
                if !snapshot.terminal =>
            {
                let output = AdkChatPortOutput::LiveStream(AdkChatLiveStream {
                    headers: snapshot.headers.clone(),
                    stream,
                });
                if started.send(Ok(output)).is_ok() {
                    self.tail_existing_stream(snapshot, sender, supervisor_cancel);
                }
                return;
            }
            Ok(super::PreparedChat::Existing(output)) => {
                let _ = started.send(Ok(output));
                return;
            }
            Ok(super::PreparedChat::New(chat, run_lease)) => (chat, run_lease),
            Err(error) => {
                let _ = started.send(Err(error));
                return;
            }
        };
        let (chat, run_lease) = chat;
        if sender.send(b"retry: 3000\n\n".to_vec()).is_err() {
            let disconnect = client_disconnected();
            let _ = self.persist_cancelled(&chat, &disconnect, &run_lease);
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
        let initial_event = match self.emit_stream_event(&chat, initial, Some(&sender), &run_lease)
        {
            Ok(event) => event,
            Err(error) => {
                if super::is_client_disconnect(&error) {
                    let _ = self.persist_cancelled(&chat, &error, &run_lease);
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
            let _ = self.persist_cancelled(&chat, &disconnect, &run_lease);
            let _ = started.send(Err(disconnect));
            return;
        }
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let output = AdkChatPortOutput::LiveStream(AdkChatLiveStream {
            headers: BTreeMap::from([(String::from("X-ADK-Stream-ID"), chat.run_id.clone())]),
            stream,
        });
        if started.send(Ok(output)).is_err() {
            self.cancellation_registry
                .unregister(&chat.run_id, &cancellation);
            let _ = self.persist_cancelled(&chat, &client_disconnected(), &run_lease);
            return;
        }
        self.run_live_stream(chat, sender, cancellation, run_lease);
    }

    fn tail_existing_stream(
        &self,
        snapshot: AdkChatStreamSnapshot,
        sender: ApiStreamSender,
        cancellation: Arc<AtomicBool>,
    ) {
        if sender.send(b"retry: 3000\n\n".to_vec()).is_err() {
            return;
        }
        let mut seen = 0usize;
        for frame in snapshot.frames {
            let bytes = match frame {
                AdkChatStreamFrame::Event { data, .. } => super::encode_sse_event(&data),
                AdkChatStreamFrame::Comment(comment) => format!("{comment}\n\n").into_bytes(),
            };
            if sender.send(bytes).is_err() {
                return;
            }
            seen = seen.saturating_add(1);
        }
        let Some(run_id) = snapshot.headers.get("X-ADK-Stream-ID").cloned() else {
            return;
        };
        let mut sent_current = false;
        loop {
            if cancellation.load(Ordering::Acquire) || sender.is_closed() {
                return;
            }
            let run = match self.store.get_run(&run_id) {
                Ok(Some(run)) => run,
                Ok(None) => return,
                Err(error) => {
                    let event = json!({
                        "type": "error",
                        "message": format!("assistant stream replay failed: {error}"),
                    });
                    let _ = sender.send(super::encode_sse_event(&event));
                    return;
                }
            };
            let payload: Value = match serde_json::from_str(&run.payload_json) {
                Ok(payload) => payload,
                Err(error) => {
                    let event = json!({
                        "type": "error",
                        "message": format!("assistant stream replay is corrupt: {error}"),
                    });
                    let _ = sender.send(super::encode_sse_event(&event));
                    return;
                }
            };
            if !sent_current {
                let current = json!({"type": "run", "run": payload.clone()});
                if sender.send(super::encode_sse_event(&current)).is_err() {
                    return;
                }
                sent_current = true;
            }
            let events = payload
                .get("streamEvents")
                .and_then(Value::as_array)
                .cloned()
                .unwrap_or_default();
            for event in events.iter().skip(seen) {
                if sender.send(super::encode_sse_event(event)).is_err() {
                    return;
                }
            }
            seen = events.len();
            if matches!(
                run.status.to_ascii_uppercase().as_str(),
                "COMPLETED" | "FAILED" | "TIMED_OUT" | "CANCELLED" | "DENIED" | "PENDING"
            ) {
                if !events.last().is_some_and(|event| {
                    event
                        .get("type")
                        .and_then(Value::as_str)
                        .is_some_and(|kind| matches!(kind, "final" | "error"))
                }) {
                    let current = json!({"type": "run", "run": payload});
                    let _ = sender.send(super::encode_sse_event(&current));
                }
                return;
            }
            thread::sleep(Duration::from_millis(100));
        }
    }

    fn run_live_stream(
        &self,
        chat: super::ChatExecution,
        sender: ApiStreamSender,
        cancellation: Arc<AtomicBool>,
        run_lease: RunLeaseGuard,
    ) {
        let _guard = super::CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
            token: Arc::clone(&cancellation),
        };
        let result = super::stream_adapter::execute_model_stream(
            chat.request.clone(),
            |event| self.forward_provider_event(&chat, event, &sender, &run_lease),
            || {
                sender.is_closed()
                    || cancellation.load(Ordering::Acquire)
                    || self.run_is_cancelled(&chat.run_id)
            },
        );
        // A takeover may happen after the provider returns but before the
        // terminal projection is written.  Never let a stale stream worker
        // publish a late success/failure over the new owner's run state.
        if run_lease.is_lost() {
            return;
        }
        match result {
            Ok(model_response) if !model_response.tool_calls.is_empty() => {
                match self.persist_tool_calls(&chat, &model_response, &run_lease) {
                    Ok(AdkChatPortOutput::Json(response)) => {
                        let event = self
                            .emit_post_terminal_event(
                                &chat,
                                // The browser stream contract has no
                                // `pending` event.  Approval waits are
                                // terminal from the transport perspective;
                                // the embedded run status and
                                // pendingApprovals carry the resumable state.
                                json!({"type": "final", "response": response}),
                                "PENDING",
                                &run_lease,
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
                                &run_lease,
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
            Ok(model_response) => match self.persist_success(&chat, model_response, &run_lease) {
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
                        let _ = self.persist_cancelled(&chat, &error, &run_lease);
                        return;
                    }
                    let persisted = match self.persist_failure(&chat, &error, &run_lease) {
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
                    let _ = self.persist_cancelled(&chat, &error, &run_lease);
                    return;
                }
                let persisted = if super::is_provider_retryable_error(&error) {
                    match self.persist_provider_retry(&chat, &error, &run_lease) {
                        Ok(()) => true,
                        Err(_) => false,
                    }
                } else {
                    match self.persist_failure(&chat, &error, &run_lease) {
                        Ok(()) => true,
                        Err(_) => false,
                    }
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

    pub(super) fn forward_provider_event(
        &self,
        chat: &super::ChatExecution,
        provider_event: &Value,
        sender: &ApiStreamSender,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        if run_lease.is_lost() {
            return Err(unavailable("assistant run execution lease was lost"));
        }
        let prior_text = self.stream_text_prefix(&chat.run_id)?;
        self.append_provider_event(chat, provider_event, run_lease)?;
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
        if run_lease.is_lost() {
            return Err(unavailable("assistant run execution lease was lost"));
        }
        let event = self.emit_stream_event(chat, value, Some(sender), run_lease)?;
        let _ = sender.send(super::encode_sse_event(&event));
        Ok(())
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
        run_lease: &RunLeaseGuard,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match result {
            Ok(model_response) => {
                let response = self.persist_success(chat, model_response, run_lease)?;
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
                if super::is_provider_retryable_error(&error) {
                    let _ = self.persist_provider_retry(chat, &error, run_lease);
                } else {
                    let _ = self.persist_failure(chat, &error, run_lease);
                }
                Err(error)
            }
        }
    }

    pub(super) fn persist_success(
        &self,
        chat: &ChatExecution,
        model_response: ModelResponse,
        run_lease: &RunLeaseGuard,
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
        if let Some(object) = payload.as_object_mut() {
            object.remove("providerRetry");
        }
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
            .update_run_state_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                "COMPLETED",
                &payload.to_string(),
                self.session_store.as_ref(),
                &events,
                run_lease.owner_id(),
                run_lease.token(),
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
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let (error_status, error_code, error_message, message) = match error {
            AdkChatPortError::Unavailable(message) => (
                503,
                "ADK_UNAVAILABLE".to_owned(),
                message.clone(),
                message.clone(),
            ),
            AdkChatPortError::Conflict(message) => (
                409,
                "ADK_CHAT_IDEMPOTENCY_CONFLICT".to_owned(),
                message.clone(),
                message.clone(),
            ),
            AdkChatPortError::Failed {
                status,
                code,
                message,
            } => (
                *status,
                code.clone(),
                message.clone(),
                format!("{code}: {message}"),
            ),
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
        payload["errorStatus"] = Value::from(error_status);
        payload["errorCode"] = Value::String(error_code);
        payload["errorMessage"] = Value::String(error_message);
        if let Some(object) = payload.as_object_mut() {
            object.remove("providerRetry");
        }
        let mut stream_event_id = None;
        let mut stream_event_content = None;
        if chat.route == AdkChatRoute::Stream {
            let has_terminal = payload
                .get("streamEvents")
                .and_then(Value::as_array)
                .and_then(|events| events.last())
                .and_then(|event| event.get("type"))
                .and_then(Value::as_str)
                .is_some_and(|kind| matches!(kind, "final" | "error"));
            if !has_terminal {
                let sequence = payload
                    .get("streamEvents")
                    .and_then(Value::as_array)
                    .map_or(1, |events| events.len() as u64 + 1);
                let mut event = json!({"type":"error","message":message});
                if let Some(object) = event.as_object_mut() {
                    object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
                    object.insert("sequence".to_owned(), Value::from(sequence));
                    object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
                }
                payload
                    .get_mut("streamEvents")
                    .and_then(Value::as_array_mut)
                    .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?
                    .push(event);
                stream_event_id = Some(format!("{}:stream:{}", chat.run_id, sequence));
                stream_event_content = Some(message.clone());
            }
        }
        payload["status"] = Value::String(status.to_owned());
        payload["message"] = Value::String(message);
        let updated = match (stream_event_id.as_ref(), stream_event_content.as_ref()) {
            (Some(event_id), Some(content)) => self
                .store()
                .update_run_state_if_status_and_revision_with_events_with_lease(
                    &chat.run_id,
                    "RUNNING",
                    &run.updated_at,
                    status,
                    &payload.to_string(),
                    self.session_store.as_ref(),
                    &[AdkRunEvent {
                        id: event_id,
                        session_id: &chat.session_id,
                        invocation_id: &chat.run_id,
                        author: "assistant.stream",
                        content,
                    }],
                    run_lease.owner_id(),
                    run_lease.token(),
                ),
            _ => self
                .store()
                .update_run_state_if_status_and_revision_with_lease(
                    &chat.run_id,
                    "RUNNING",
                    &run.updated_at,
                    status,
                    &payload.to_string(),
                    run_lease.owner_id(),
                    run_lease.token(),
                ),
        }
        .map_err(storage_unavailable)?;
        if !updated {
            let current = self
                .store()
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
            if current.status.eq_ignore_ascii_case("CANCELLED") {
                return Err(run_cancelled());
            }
            if current.status.eq_ignore_ascii_case(status) {
                return Ok(());
            }
            return Err(unavailable(
                "assistant chat run or execution lease changed before failure",
            ));
        }
        Ok(())
    }

    fn store(&self) -> &std::sync::Arc<jftrade_store_sqlite::AdkStore> {
        &self.store
    }
}

fn client_disconnected() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "CLIENT_DISCONNECTED".to_owned(),
        message: "assistant chat client disconnected".to_owned(),
    }
}
